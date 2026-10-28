package selfupdate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

type scriptSource struct {
	rel     Release
	bodies  map[int64][]byte
	calls   []string
	mu      sync.Mutex
	block   chan struct{}
	entered chan struct{} // closed when Latest is first entered
	err     error
	openErr map[int64]error
	log     *[]string // optional shared call log
}

func (s *scriptSource) note(call string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, call)
	if s.log != nil {
		*s.log = append(*s.log, call)
	}
}

func (s *scriptSource) Latest(ctx context.Context) (Release, error) {
	s.note("Latest")
	s.mu.Lock()
	if s.entered != nil {
		close(s.entered)
		s.entered = nil
	}
	s.mu.Unlock()
	if s.block != nil {
		select {
		case <-ctx.Done():
			return Release{}, ctx.Err()
		case <-s.block:
		}
	}
	return s.rel, s.err
}
func (s *scriptSource) ByTag(context.Context, string) (Release, error) {
	s.note("ByTag")
	return s.rel, s.err
}

// ListReleases lists the one scripted release, so a channel request
// reaches discovery (0005-PLAN Step 5).
func (s *scriptSource) ListReleases(context.Context, ListOptions) ([]Release, error) {
	s.note("ListReleases")
	return []Release{s.rel}, s.err
}
func (s *scriptSource) OpenAsset(_ context.Context, _ Release, a Asset) (io.ReadCloser, error) {
	s.note(fmt.Sprintf("OpenAsset:%d", a.ID))
	if err := s.openErr[a.ID]; err != nil {
		return nil, err
	}
	body, ok := s.bodies[a.ID]
	if !ok {
		return nil, fmt.Errorf("missing asset %d", a.ID)
	}
	return io.NopCloser(bytes.NewReader(body)), nil
}

type recReporter struct {
	kinds  []EventKind
	events []Event
	errAt  EventKind
	fail   error
	log    *[]string // optional shared call log
}

func (r *recReporter) Report(_ context.Context, ev Event) error {
	r.kinds = append(r.kinds, ev.Kind)
	r.events = append(r.events, ev)
	if r.log != nil {
		*r.log = append(*r.log, "event:"+ev.Kind.String())
	}
	if r.errAt != EventUnknown && ev.Kind == r.errAt {
		if r.fail == nil {
			return errors.New("report failed")
		}
		return r.fail
	}
	return nil
}

type recConfirmer struct {
	ok    bool
	err   error
	calls int
}

func (c *recConfirmer) Confirm(context.Context, Prompt) (bool, error) {
	c.calls++
	return c.ok, c.err
}

type recVerifier struct {
	opened int
	closed int
}

func (v *recVerifier) Verify(_ context.Context, ver Verification) error {
	if ver.Open == nil {
		return errors.New("missing open")
	}
	rc, err := ver.Open()
	if err != nil {
		return err
	}
	v.opened++
	_, _ = io.Copy(io.Discard, rc)
	if err := rc.Close(); err != nil {
		return err
	}
	v.closed++
	return nil
}

func fixtureRelease(t *testing.T, product string) (Release, map[int64][]byte, []Platform) {
	t.Helper()
	plat := Platform{OS: runtime.GOOS, Arch: runtime.GOARCH}
	if plat.OS == "windows" {
		plat.OS = goosWindows
	}
	bin := []byte("hello-bin")
	sum := sha256.Sum256(bin)
	hexsum := hex.EncodeToString(sum[:])
	name := ExactAssetName(product, plat)
	manifest := []byte(hexsum + "  " + name + "\n")
	rel := Release{
		ID: 1, Tag: "v1.1.0", URL: "https://example.invalid/v1.1.0", Immutable: true,
		Assets: []Asset{
			{ID: 2, Name: name, State: "uploaded", Size: int64(len(bin)), Digest: "sha256:" + hexsum},
			{ID: 3, Name: manifestAssetName, State: "uploaded", Size: int64(len(manifest))},
		},
	}
	return rel, map[int64][]byte{2: bin, 3: manifest}, []Platform{plat}
}

type nilPtrVerifier struct{}

func (*nilPtrVerifier) Verify(context.Context, Verification) error { return nil }

type nilPtrReporter struct{}

func (*nilPtrReporter) Report(context.Context, Event) error { return nil }

type nilPtrTransformer struct{}

func (*nilPtrTransformer) Transform(context.Context, TransformRequest) error { return nil }

// TestNewRejectsNilCollaborators covers 0003-MADR C3 and part of C13: every
// required seam nil or typed-nil, a nil or typed-nil verifier, a typed-nil
// transformer, and invalid limits.
func TestNewRejectsNilCollaborators(t *testing.T) {
	t.Parallel()
	rel, bodies, plats := fixtureRelease(t, "demo")
	sel, err := NewExactAssetSelector(plats)
	if err != nil {
		t.Fatal(err)
	}
	valid := func() Config {
		return Config{
			Source: &scriptSource{rel: rel, bodies: bodies}, Versions: NewStrictVersionPolicy(), Assets: sel,
			Installer: pendingInstaller{}, Reporter: &recReporter{}, Confirmer: &recConfirmer{}, Limits: DefaultLimits(),
		}
	}
	if _, err := New(valid()); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	var typedNilSource *scriptSource
	var typedNilVerifier *nilPtrVerifier
	var typedNilReporter *nilPtrReporter
	var typedNilTransformer *nilPtrTransformer
	cases := map[string]func(*Config){
		"empty":              func(c *Config) { *c = Config{} },
		"nil source":         func(c *Config) { c.Source = nil },
		"typed-nil source":   func(c *Config) { c.Source = typedNilSource },
		"nil versions":       func(c *Config) { c.Versions = nil },
		"nil assets":         func(c *Config) { c.Assets = nil },
		"nil installer":      func(c *Config) { c.Installer = nil },
		"nil reporter":       func(c *Config) { c.Reporter = nil },
		"typed-nil reporter": func(c *Config) { c.Reporter = typedNilReporter },
		"nil confirmer":      func(c *Config) { c.Confirmer = nil },
		"nil verifier":       func(c *Config) { c.Verifiers = []Verifier{&recVerifier{}, nil} },
		"typed-nil verifier": func(c *Config) { c.Verifiers = []Verifier{typedNilVerifier} },
		"typed-nil transformer": func(c *Config) {
			c.Transformer = typedNilTransformer
		},
		"zero limits":     func(c *Config) { c.Limits = Limits{} },
		"negative limits": func(c *Config) { c.Limits.Executable = -1 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := valid()
			mutate(&cfg)
			if u, err := New(cfg); err == nil {
				t.Fatalf("accepted %s: %+v", name, u)
			}
		})
	}
}

func TestRunCheckAndApply(t *testing.T) {
	t.Parallel()
	rel, bodies, plats := fixtureRelease(t, "demo")
	src := &scriptSource{rel: rel, bodies: bodies}
	_, exe := tempTarget(t)
	inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: targetPolicy(exe)})
	if err != nil {
		t.Fatal(err)
	}
	rep := &recReporter{}
	conf := &recConfirmer{ok: true}
	sel, err := NewExactAssetSelector(plats)
	if err != nil {
		t.Fatal(err)
	}
	u, err := New(Config{
		Source: src, Versions: NewStrictVersionPolicy(), Assets: sel,
		Installer: inst, Reporter: rep, Confirmer: conf, Limits: DefaultLimits(),
	})
	if err != nil {
		t.Fatal(err)
	}
	req := Request{Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: ReleaseBuild, CheckOnly: true}
	res, err := u.Run(context.Background(), req)
	if !errors.Is(err, ErrUpdateAvailable) || res.Operation != OperationUpgrade {
		t.Fatalf("check: %+v %v", res, err)
	}
	if ExitCode(res, err) != 10 {
		t.Fatalf("exit %d", ExitCode(res, err))
	}
	if containsKind(rep.kinds, EventDownloadingBinary) {
		t.Fatal("check mode downloaded binary")
	}

	req.CheckOnly = false
	req.Yes = true
	res, err = u.Run(context.Background(), req)
	if err != nil || !res.Applied {
		t.Fatalf("apply: %+v %v", res, err)
	}
	got, _ := os.ReadFile(exe)
	if string(got) != "hello-bin" {
		t.Fatalf("installed %q", got)
	}
}

func TestRunDeclineAndNone(t *testing.T) {
	t.Parallel()
	rel, bodies, plats := fixtureRelease(t, "demo")
	src := &scriptSource{rel: rel, bodies: bodies}
	_, exe := tempTarget(t)
	inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: targetPolicy(exe)})
	if err != nil {
		t.Fatal(err)
	}
	sel, _ := NewExactAssetSelector(plats)
	u, err := New(Config{
		Source: src, Versions: NewStrictVersionPolicy(), Assets: sel,
		Installer: inst, Reporter: &recReporter{}, Confirmer: &recConfirmer{ok: false}, Limits: DefaultLimits(),
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := u.Run(context.Background(), Request{Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: ReleaseBuild})
	if err != nil || !res.Declined || res.Applied {
		t.Fatalf("%+v %v", res, err)
	}

	rel.Tag = "v1.0.0"
	src.rel = rel
	res, err = u.Run(context.Background(), Request{Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: ReleaseBuild, CheckOnly: true})
	if err != nil || res.Operation != OperationNone {
		t.Fatalf("none: %+v %v", res, err)
	}
}

// TestOverlappingRun holds the first Run inside Source.Latest with a channel
// handshake (no sleep), then asserts the overlapping Run is rejected without
// invoking any collaborator (0003-MADR C13).
func TestOverlappingRun(t *testing.T) {
	t.Parallel()
	block := make(chan struct{})
	entered := make(chan struct{})
	rel, bodies, plats := fixtureRelease(t, "demo")
	src := &scriptSource{rel: rel, bodies: bodies, block: block, entered: entered}
	_, exe := tempTarget(t)
	inst, _ := NewStandaloneInstaller(InstallOptions{TargetPolicy: targetPolicy(exe)})
	sel, _ := NewExactAssetSelector(plats)
	rep := &recReporter{}
	conf := &recConfirmer{ok: true}
	u, err := New(Config{
		Source: src, Versions: NewStrictVersionPolicy(), Assets: sel,
		Installer: inst, Reporter: rep, Confirmer: conf, Limits: DefaultLimits(),
	})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := u.Run(context.Background(), Request{Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: ReleaseBuild, CheckOnly: true})
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("first Run never reached Source.Latest")
	}
	src.mu.Lock()
	callsBefore := len(src.calls)
	src.mu.Unlock()
	eventsBefore := len(rep.kinds)
	// Bounded, so a missing guard fails fast (the second Run would block in
	// Latest) instead of hanging the test binary.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	_, err = u.Run(ctx2, Request{Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: ReleaseBuild, CheckOnly: true})
	close(block)
	firstErr := <-done
	if !errors.Is(err, ErrConcurrentUpdate) {
		t.Fatalf("overlap err = %v", err)
	}
	if !errors.Is(firstErr, ErrUpdateAvailable) {
		t.Fatalf("first run err = %v", firstErr)
	}
	src.mu.Lock()
	callsAfter := len(src.calls)
	src.mu.Unlock()
	// The first Run continues after unblocking, but the rejected Run made
	// no source call of its own: exactly one Latest was ever recorded.
	if n := strings.Count(strings.Join(src.calls, ","), "Latest"); n != 1 || callsBefore != 1 {
		t.Fatalf("source calls before=%d after=%d: %v", callsBefore, callsAfter, src.calls)
	}
	if eventsBefore == 0 || conf.calls != 0 {
		t.Fatalf("events before=%d confirmer calls=%d", eventsBefore, conf.calls)
	}
}

func TestReporterFailureBeforeInstall(t *testing.T) {
	t.Parallel()
	rel, bodies, plats := fixtureRelease(t, "demo")
	src := &scriptSource{rel: rel, bodies: bodies}
	_, exe := tempTarget(t)
	orig, _ := os.ReadFile(exe)
	inst, _ := NewStandaloneInstaller(InstallOptions{TargetPolicy: targetPolicy(exe)})
	sel, _ := NewExactAssetSelector(plats)
	rep := &recReporter{errAt: EventInstalling}
	u, err := New(Config{
		Source: src, Versions: NewStrictVersionPolicy(), Assets: sel,
		Installer: inst, Reporter: rep, Confirmer: &recConfirmer{ok: true}, Limits: DefaultLimits(),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = u.Run(context.Background(), Request{Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: ReleaseBuild, Yes: true})
	if err == nil {
		t.Fatal("expected report error")
	}
	got, _ := os.ReadFile(exe)
	if string(got) != string(orig) {
		t.Fatal("installed after reporter failure")
	}
}

func TestVerifierClosesDescriptor(t *testing.T) {
	t.Parallel()
	rel, bodies, plats := fixtureRelease(t, "demo")
	src := &scriptSource{rel: rel, bodies: bodies}
	_, exe := tempTarget(t)
	inst, _ := NewStandaloneInstaller(InstallOptions{TargetPolicy: targetPolicy(exe)})
	sel, _ := NewExactAssetSelector(plats)
	v := &recVerifier{}
	u, err := New(Config{
		Source: src, Versions: NewStrictVersionPolicy(), Assets: sel,
		Verifiers: []Verifier{v},
		Installer: inst, Reporter: &recReporter{}, Confirmer: &recConfirmer{ok: true}, Limits: DefaultLimits(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := u.Run(context.Background(), Request{Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: ReleaseBuild, Yes: true}); err != nil {
		t.Fatal(err)
	}
	if v.opened != 1 || v.closed != 1 {
		t.Fatalf("opened=%d closed=%d", v.opened, v.closed)
	}
}

func TestTransformerRejectsSymlink(t *testing.T) {
	t.Parallel()
	rel, bodies, plats := fixtureRelease(t, "demo")
	src := &scriptSource{rel: rel, bodies: bodies}
	_, exe := tempTarget(t)
	inst, _ := NewStandaloneInstaller(InstallOptions{TargetPolicy: targetPolicy(exe)})
	sel, _ := NewExactAssetSelector(plats)
	xf := transformerFunc(func(_ context.Context, req TransformRequest) error {
		dir := filepath.Dir(req.Path)
		other := filepath.Join(dir, "other")
		if err := os.WriteFile(other, []byte("x"), 0o600); err != nil {
			return err
		}
		if err := os.Remove(req.Path); err != nil {
			return err
		}
		return os.Symlink(other, req.Path)
	})
	u, err := New(Config{
		Source: src, Versions: NewStrictVersionPolicy(), Assets: sel,
		Transformer: xf,
		Installer:   inst, Reporter: &recReporter{}, Confirmer: &recConfirmer{ok: true}, Limits: DefaultLimits(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := u.Run(context.Background(), Request{Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: ReleaseBuild, Yes: true}); err == nil {
		t.Fatal("accepted symlink staging")
	}
}

func TestPendingBackupEvent(t *testing.T) {
	t.Parallel()
	rel, bodies, plats := fixtureRelease(t, "demo")
	src := &scriptSource{rel: rel, bodies: bodies}
	inst := pendingInstaller{}
	sel, _ := NewExactAssetSelector(plats)
	var buf bytes.Buffer
	u, err := New(Config{
		Source: src, Versions: NewStrictVersionPolicy(), Assets: sel,
		Installer: inst, Reporter: NewTextReporter(&buf), Confirmer: &recConfirmer{ok: true}, Limits: DefaultLimits(),
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := u.Run(context.Background(), Request{Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: ReleaseBuild, Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.PendingBackup != "old.bak" {
		t.Fatalf("%+v", res)
	}
	if !strings.Contains(buf.String(), "old.bak") {
		t.Fatalf("%s", buf.String())
	}
}

type transformerFunc func(context.Context, TransformRequest) error

func (f transformerFunc) Transform(ctx context.Context, req TransformRequest) error {
	return f(ctx, req)
}

type pendingInstaller struct{}

func (pendingInstaller) ResolveTarget(context.Context) (Target, error) {
	return Target{Path: "/tmp/demo", Dir: "/tmp", Base: "demo"}, nil
}
func (pendingInstaller) Begin(context.Context, Target) (InstallSession, error) {
	return pendingSession{}, nil
}

type pendingSession struct{}

func (pendingSession) Target() Target { return Target{Path: "/tmp/demo", Dir: "/tmp", Base: "demo"} }
func (pendingSession) CreateStaging(context.Context) (*os.File, string, error) {
	f, err := os.CreateTemp("", "stage")
	if err != nil {
		return nil, "", err
	}
	return f, f.Name(), nil
}
func (pendingSession) Install(context.Context, InstallRequest) (InstallResult, error) {
	return InstallResult{Applied: true, PendingBackup: "old.bak"}, nil
}
func (pendingSession) Close() error { return nil }

func containsKind(kinds []EventKind, want EventKind) bool {
	return slices.Contains(kinds, want)
}

func TestStateMachineStopsAfterFailure(t *testing.T) {
	t.Parallel()
	rel, bodies, plats := fixtureRelease(t, "demo")
	src := &scriptSource{rel: rel, bodies: bodies, err: errors.New("fetch failed")}
	_, exe := tempTarget(t)
	inst, _ := NewStandaloneInstaller(InstallOptions{TargetPolicy: targetPolicy(exe)})
	sel, _ := NewExactAssetSelector(plats)
	rep := &recReporter{}
	conf := &recConfirmer{ok: true}
	u, err := New(Config{
		Source: src, Versions: NewStrictVersionPolicy(), Assets: sel,
		Installer: inst, Reporter: rep, Confirmer: conf, Limits: DefaultLimits(),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = u.Run(context.Background(), Request{Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: ReleaseBuild, Yes: true})
	if err == nil {
		t.Fatal("expected fetch failure")
	}
	if conf.calls != 0 {
		t.Fatal("confirmed after fetch failure")
	}
	if containsKind(rep.kinds, EventInstalling) {
		t.Fatal("installed after fetch failure")
	}
}
