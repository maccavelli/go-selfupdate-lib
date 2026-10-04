package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
	"testing/iotest"
	"time"
)

// Tests for docs/decisions/0004-PLAN-v1-1-0-core-api.md Step 5.

// TestEventKindValues pins every EventKind's number and name: appending is
// compatible, renumbering is not.
func TestEventKindValues(t *testing.T) {
	want := []string{"unknown", "resolving-target", "fetching-release", "selected", "downloading-manifest",
		"downloading-binary", "verified", "transforming", "installing", "complete",
		"progress", "declined", "failed", "rolled-back"}
	kinds := []EventKind{EventUnknown, EventResolvingTarget, EventFetchingRelease, EventSelected, EventDownloadingManifest,
		EventDownloadingBinary, EventVerified, EventTransforming, EventInstalling, EventComplete,
		EventProgress, EventDeclined, EventFailed, EventRolledBack}
	for i, k := range kinds {
		if int(k) != i || k.String() != want[i] {
			t.Errorf("kind %d: value %d, name %q; want %d, %q", i, k, k.String(), i, want[i])
		}
	}
}

// oneByteSource serves asset bodies a byte per Read, so every byte is a
// separate write to the staging file.
type oneByteSource struct{ *scriptSource }

func (s oneByteSource) OpenAsset(ctx context.Context, rel Release, a Asset) (io.ReadCloser, error) {
	rc, err := s.scriptSource.OpenAsset(ctx, rel, a)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(iotest.OneByteReader(rc)), nil
}

// progressRun runs an apply with the given interval and a clock that moves
// step on every reading.
func progressRun(t *testing.T, interval, step time.Duration, rep *recReporter) (Result, int64, error) {
	t.Helper()
	clock := cacheNow
	setSeam(t, &timeNow, func() time.Time { clock = clock.Add(step); return clock })
	env := newContractEnv(t)
	rel, _, plats := fixtureRelease(t, "demo")
	sel, err := NewExactAssetSelector(plats)
	if err != nil {
		t.Fatal(err)
	}
	u, err := New(Config{Source: oneByteSource{env.src}, Versions: NewStrictVersionPolicy(), Assets: sel,
		Installer: env.inst, Reporter: rep, Confirmer: env.conf, Limits: env.lim, ProgressInterval: interval})
	if err != nil {
		t.Fatal(err)
	}
	req := applyReq()
	req.Yes = true
	res, err := u.Run(context.Background(), req)
	return res, rel.Assets[0].Size, err
}

func progressEvents(rep *recReporter) []Event {
	var out []Event
	for _, ev := range rep.events {
		if ev.Kind == EventProgress {
			out = append(out, ev)
		}
	}
	return out
}

// TestProgressEvents: a first event at zero, throttled events in between,
// and a final event at the total, all between downloading-binary and
// verified.
func TestProgressEvents(t *testing.T) {
	rep := &recReporter{}
	_, size, err := progressRun(t, 100*time.Millisecond, 100*time.Millisecond, rep)
	if err != nil {
		t.Fatal(err)
	}
	evs := progressEvents(rep)
	if int64(len(evs)) != size+1 {
		t.Fatalf("%d progress events for %d bytes with every write past the interval; want %d", len(evs), size, size+1)
	}
	for i, ev := range evs {
		if ev.Bytes != int64(i) || ev.Total != size || ev.Asset == "" || ev.Target != "v1.1.0" {
			t.Fatalf("event %d = %+v", i, ev)
		}
	}
	first := slices.Index(rep.kinds, EventProgress)
	if rep.kinds[first-1] != EventDownloadingBinary || rep.kinds[first+len(evs)] != EventVerified {
		t.Fatalf("progress is not between downloading-binary and verified: %v", rep.kinds)
	}

	throttled := &recReporter{}
	if _, size, err := progressRun(t, time.Hour, time.Millisecond, throttled); err != nil {
		t.Fatal(err)
	} else if evs := progressEvents(throttled); len(evs) != 2 || evs[0].Bytes != 0 || evs[1].Bytes != size {
		t.Fatalf("throttled progress = %+v, want only the first and final events", evs)
	}
}

func TestProgressDisabledByDefault(t *testing.T) {
	rep := &recReporter{}
	if _, _, err := progressRun(t, 0, time.Hour, rep); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(rep.kinds, EventProgress) {
		t.Fatalf("progress reported with a zero interval: %v", rep.kinds)
	}
}

func TestProgressIntervalNegativeRejected(t *testing.T) {
	env := newContractEnv(t)
	_, _, plats := fixtureRelease(t, "demo")
	sel, _ := NewExactAssetSelector(plats)
	if _, err := New(Config{Source: env.src, Versions: NewStrictVersionPolicy(), Assets: sel, Installer: env.inst,
		Reporter: env.rep, Confirmer: env.conf, Limits: env.lim, ProgressInterval: -time.Second}); err == nil {
		t.Fatal("a negative progress interval was accepted")
	}
}

func TestProgressReporterErrorIgnored(t *testing.T) {
	rep := &recReporter{errAt: EventProgress}
	res, _, err := progressRun(t, time.Millisecond, time.Second, rep)
	if err != nil || !res.Applied {
		t.Fatalf("a progress reporter error failed the run: %v", err)
	}
}

func TestDeclinedEvent(t *testing.T) {
	env := newContractEnv(t)
	env.conf.err = nil
	u := env.build(t)
	decline := &recReporter{errAt: EventDeclined}
	u.reporter = decline
	u.confirmer = ConfirmerFunc(func(context.Context, Prompt) (bool, error) { return false, nil })
	res, err := u.Run(context.Background(), applyReq())
	if err != nil || !res.Declined {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if last := decline.kinds[len(decline.kinds)-1]; last != EventDeclined {
		t.Fatalf("last event = %v, want declined: %v", last, decline.kinds)
	}
}

func TestFailureClass(t *testing.T) {
	for err, want := range map[error]string{
		context.Canceled: "canceled",
		errors.Join(context.Canceled, ErrIntegrity):  "canceled",
		context.DeadlineExceeded:                     "deadline-exceeded",
		ErrConfirmationRequired:                      "confirmation-required",
		ErrForceRequired:                             "force-required",
		ErrLatestOlder:                               "latest-older",
		ErrMutableRelease:                            "mutable-release",
		&RateLimitError{StatusCode: 429}:             "rate-limited",
		ErrUnsupportedPlatform:                       "unsupported-platform",
		ErrConcurrentUpdate:                          "concurrent-update",
		errors.Join(ErrManagedInstall, ErrIntegrity): "managed-install",
		ErrIntegrity:                                 "integrity",
		errors.New("anything else"):                  "error",
	} {
		if got := failureClass(err); got != want {
			t.Errorf("%v: %q, want %q", err, got, want)
		}
	}
}

// TestFailedEventClasses: a failed run ends with EventFailed and its class;
// check mode finding an update and an unsafe product name do not.
func TestFailedEventClasses(t *testing.T) {
	env := newContractEnv(t)
	env.src.rel.Immutable = false
	u := env.build(t)
	req := applyReq()
	req.Yes = true
	if _, err := u.Run(context.Background(), req); !errors.Is(err, ErrMutableRelease) {
		t.Fatalf("err = %v", err)
	}
	last := env.rep.events[len(env.rep.events)-1]
	if last.Kind != EventFailed || last.Detail != "mutable-release" || last.Product != "demo" {
		t.Fatalf("last event = %+v", last)
	}

	found := newContractEnv(t)
	check := applyReq()
	check.CheckOnly = true
	if _, err := found.build(t).Run(context.Background(), check); !errors.Is(err, ErrUpdateAvailable) {
		t.Fatalf("check: %v", err)
	}
	if slices.Contains(found.rep.kinds, EventFailed) {
		t.Fatalf("check mode reported failed: %v", found.rep.kinds)
	}

	unsafe := newContractEnv(t)
	if _, err := unsafe.build(t).Run(context.Background(), Request{Product: "../x", CurrentBuild: ReleaseBuild, CurrentVersion: "v1.0.0"}); err == nil {
		t.Fatal("unsafe product accepted")
	}
	if len(unsafe.rep.kinds) != 0 {
		t.Fatalf("events reported for an unsafe product: %v", unsafe.rep.kinds)
	}
}

func TestFailedEventReporterErrorIgnored(t *testing.T) {
	env := newContractEnv(t)
	env.src.rel.Immutable = false
	env.rep.errAt = EventFailed
	u := env.build(t)
	req := applyReq()
	req.Yes = true
	_, err := u.Run(context.Background(), req)
	if !errors.Is(err, ErrMutableRelease) || strings.Contains(err.Error(), "report failed") {
		t.Fatalf("err = %v; the reporter error must not reach the caller", err)
	}
}

// rolledBackInstaller's session restores the previous binary itself.
type rolledBackInstaller struct{ *logInstaller }

func (i rolledBackInstaller) Begin(ctx context.Context, t Target) (InstallSession, error) {
	s, err := i.logInstaller.Begin(ctx, t)
	if err != nil {
		return nil, err
	}
	return rolledBackSession{s.(*logSession)}, nil
}

type rolledBackSession struct{ *logSession }

func (rolledBackSession) Install(context.Context, InstallRequest) (InstallResult, error) {
	return InstallResult{RolledBack: true}, errors.New("post-install check failed")
}

func TestRolledBackEvent(t *testing.T) {
	env := newContractEnv(t)
	_, _, plats := fixtureRelease(t, "demo")
	sel, _ := NewExactAssetSelector(plats)
	u, err := New(Config{Source: env.src, Versions: NewStrictVersionPolicy(), Assets: sel,
		Installer: rolledBackInstaller{env.inst}, Reporter: env.rep, Confirmer: env.conf, Limits: env.lim})
	if err != nil {
		t.Fatal(err)
	}
	req := applyReq()
	req.Yes = true
	if _, err := u.Run(context.Background(), req); err == nil {
		t.Fatal("want the install failure")
	}
	rb := slices.Index(env.rep.kinds, EventRolledBack)
	if rb < 0 || env.rep.kinds[len(env.rep.kinds)-1] != EventFailed || rb != len(env.rep.kinds)-2 {
		t.Fatalf("events = %v, want rolled-back then failed", env.rep.kinds)
	}
}

// TestInstallersReportRolledBack: the standalone swap rollback and managed
// recovery both set InstallResult.RolledBack.
func TestInstallersReportRolledBack(t *testing.T) {
	life := &fakeLife{installed: true, running: true, healthErr: errors.New("unhealthy")}
	_, _, sess, _ := managedEnv(t, life, &fakeRec{})
	res, err := sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: stageNew(t, sess)}})
	if !errors.Is(err, ErrManagedInstall) || !res.RolledBack {
		t.Fatalf("managed: res=%+v err=%v", res, err)
	}

	standalone, exe := standaloneSession(t)
	path := stageNew(t, standalone)
	dir := filepath.Dir(exe)
	moved := dir + ".moved"
	real := replacePath
	var swapErr error
	setSeam(t, &replacePath, func(ctx context.Context, oldpath, newpath string) error {
		if err := real(ctx, oldpath, newpath); err != nil {
			return err
		}
		if swapErr = os.Rename(dir, moved); swapErr == nil {
			return os.Mkdir(dir, 0o755)
		}
		return nil
	})
	t.Cleanup(func() {
		if swapErr == nil {
			_ = os.RemoveAll(dir)
			_ = os.Rename(moved, dir)
		}
	})
	res, _ = standalone.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: path}})
	if swapErr != nil {
		if runtime.GOOS != goosWindows {
			t.Fatalf("directory swap failed: %v", swapErr)
		}
		t.Logf("the OS refused the swap: %v", swapErr)
		return
	}
	if !res.RolledBack {
		t.Fatalf("standalone swap rollback: %+v", res)
	}
}

func TestTextReporterSkipsProgress(t *testing.T) {
	var b strings.Builder
	r := NewTextReporter(&b)
	if err := r.Report(context.Background(), Event{Kind: EventProgress, Product: "demo", Bytes: 5, Total: 9}); err != nil {
		t.Fatal(err)
	}
	if err := r.Report(context.Background(), Event{Kind: EventFailed, Product: "demo", Detail: "integrity"}); err != nil {
		t.Fatal(err)
	}
	if got := b.String(); got != "selfupdate: failed product=demo integrity\n" {
		t.Fatalf("text = %q", got)
	}
}

// countingWriter records every Write call.
type countingWriter struct {
	strings.Builder
	writes int
}

func (w *countingWriter) Write(p []byte) (int, error) {
	w.writes++
	return w.Builder.Write(p)
}

func TestJSONReporterLines(t *testing.T) {
	w := &countingWriter{}
	r := NewJSONReporter(w)
	ctx := context.Background()
	evs := []Event{
		{Kind: EventProgress, Product: "demo", Target: "v1.1.0", Asset: "demo-linux-amd64", Bytes: 4, Total: 9},
		{Kind: EventFailed, Product: "demo\x1b[31m", Detail: "a <b> & c"},
		{Kind: EventResolvingTarget},
	}
	for _, ev := range evs {
		if err := r.Report(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
	want := `{"kind":"progress","product":"demo","target":"v1.1.0","asset":"demo-linux-amd64","bytes":4,"total":9}` + "\n" +
		`{"kind":"failed","product":"demo?[31m","detail":"a <b> & c"}` + "\n" +
		`{"kind":"resolving-target"}` + "\n"
	if w.String() != want {
		t.Fatalf("lines:\n%s\nwant:\n%s", w.String(), want)
	}
	if w.writes != len(evs) {
		t.Fatalf("%d writes for %d events", w.writes, len(evs))
	}
	if err := NewJSONReporter(nil).Report(ctx, evs[0]); err == nil {
		t.Fatal("nil writer accepted")
	}
}

func TestResultDocumentJSON(t *testing.T) {
	res := Result{Product: "demo", CurrentVersion: "v1.0.0", TargetVersion: "v1.1.0", ReleaseURL: "https://example.invalid/r",
		AssetName: "demo-linux-amd64", Operation: OperationUpgrade, Applied: true, ReleaseDigest: "aa", InstalledDigest: "bb"}
	got, err := json.Marshal(res.Document())
	if err != nil {
		t.Fatal(err)
	}
	want := `{"schema_version":2,"product":"demo","current_version":"v1.0.0","target_version":"v1.1.0",` +
		`"release_url":"https://example.invalid/r","asset_name":"demo-linux-amd64","operation":"upgrade",` +
		`"checked":false,"applied":true,"declined":false,"dry_run":false,"release_digest":"aa","installed_digest":"bb",` +
		`"service_installed":false,"service_was_running":false,"service_started":false}`
	if string(got) != want {
		t.Fatalf("document:\n%s\nwant:\n%s", got, want)
	}

	// The Step 10 fields (0004-PLAN-v1-1-0-core-api.md Step 13, D5).
	res = Result{Product: "demo", CurrentVersion: "v1.0.0", Operation: OperationUpgrade, Applied: true, DryRun: true,
		Previous: "/opt/demo/.demo.previous"}
	if got, err = json.Marshal(res.Document()); err != nil {
		t.Fatal(err)
	}
	want = `{"schema_version":2,"product":"demo","current_version":"v1.0.0","operation":"upgrade",` +
		`"checked":false,"applied":true,"declined":false,"dry_run":true,` +
		`"service_installed":false,"service_was_running":false,"service_started":false,"previous":"/opt/demo/.demo.previous"}`
	if string(got) != want {
		t.Fatalf("document:\n%s\nwant:\n%s", got, want)
	}
}

// TestResultDocumentCoversEveryField: a Result with every field set gives
// a document with every field set, so a field added to Result cannot be
// left out of Document unnoticed (0004-PLAN-v1-1-0-core-api.md Step 13,
// D5).
func TestResultDocumentCoversEveryField(t *testing.T) {
	var res Result
	rv := reflect.ValueOf(&res).Elem()
	for i := range rv.NumField() {
		f := rv.Field(i)
		switch f.Kind() {
		case reflect.String:
			f.SetString("x")
		case reflect.Bool:
			f.SetBool(true)
		case reflect.Uint8:
			f.SetUint(uint64(OperationUpgrade))
		default:
			t.Fatalf("Result.%s has kind %s; extend this test", rv.Type().Field(i).Name, f.Kind())
		}
	}
	if n, d := rv.NumField(), reflect.TypeFor[ResultDocument]().NumField(); d != n+1 {
		t.Fatalf("ResultDocument has %d fields, want Result's %d plus schema_version", d, n)
	}
	doc := reflect.ValueOf(res.Document())
	for i := range doc.NumField() {
		if doc.Field(i).IsZero() {
			t.Errorf("Document leaves ResultDocument.%s unset", doc.Type().Field(i).Name)
		}
	}
}
