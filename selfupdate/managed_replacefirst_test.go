package selfupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// The opt-in replace-before-stop order
// (docs/decisions/0020-MADR-precheck-gofmt-errors-and-replace-before-stop.md
// 4B, 0020-PLAN S2 step 9).

// firstLife is a running service whose Stop records what the target held,
// and whether the service ran, at that moment.
type firstLife struct {
	target     string
	running    bool
	log        []string
	atStop     string
	ranAtStop  bool
	stopErr    error
	stopDown   bool // a failed Stop still took the service down
	healthErrs int  // how many WaitHealthy calls fail first
}

func (l *firstLife) Installed(context.Context, string) (bool, error) { return true, nil }
func (l *firstLife) Running(context.Context, string) (bool, error)   { return l.running, nil }
func (l *firstLife) Stop(context.Context, string) error {
	l.log = append(l.log, "stop")
	if b, err := os.ReadFile(l.target); err == nil {
		l.atStop = string(b)
	}
	l.ranAtStop = l.running
	if l.stopErr != nil {
		if l.stopDown {
			l.running = false
		}
		return l.stopErr
	}
	l.running = false
	return nil
}
func (l *firstLife) Start(context.Context, string) error {
	l.log = append(l.log, "start")
	l.running = true
	return nil
}
func (l *firstLife) WaitHealthy(context.Context, string) error {
	l.log = append(l.log, "health")
	if l.healthErrs > 0 {
		l.healthErrs--
		return errors.New("fixture: unhealthy")
	}
	return nil
}

// enabledFirstLife is an firstLife that reports itself configured to start.
type enabledFirstLife struct{ firstLife }

func (l *enabledFirstLife) Enabled(context.Context, string) (bool, error) { return true, nil }

// replaceFirstOpts configure replaceFirst.
type replaceFirstOpts struct {
	opts        ManagedOptions
	postInstall Prober
	// before runs after Begin, with the target, before Install.
	before func(t *testing.T, target string)
}

// replaceFirst runs one managed install of "new-bytes" over "old-bytes",
// and returns its result, the target's bytes afterwards, and its error.
// life's target must be set by the caller through setTarget.
func replaceFirst(t *testing.T, life Lifecycle, rec Reconciler, o replaceFirstOpts, setTarget func(string)) (InstallResult, string, error) {
	t.Helper()
	_, exe := withTempHome(t)
	if setTarget != nil {
		setTarget(exe)
	}
	inner, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}, PostInstall: o.postInstall})
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewManagedInstallerWith(inner, life, rec, o.opts)
	if err != nil {
		t.Fatal(err)
	}
	target, err := m.ResolveTarget(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := m.Begin(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sess.Close() }()
	if o.before != nil {
		o.before(t, exe)
	}
	res, err := sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: stageNew(t, sess)}})
	got := ""
	if b, rerr := os.ReadFile(exe); rerr == nil {
		got = string(b)
	}
	return res, got, err
}

var early = replaceFirstOpts{opts: ManagedOptions{ReplaceBeforeStop: true}}

// TestReplaceBeforeStopOrder: with the option, the binary is replaced while
// the service runs, then the service is stopped, started and checked.
func TestReplaceBeforeStopOrder(t *testing.T) {
	life := &firstLife{running: true}
	res, got, err := replaceFirst(t, life, &fakeRec{}, early, func(p string) { life.target = p })
	if err != nil || !res.Applied || !res.ReplacedBeforeStop || got != "new-bytes" {
		t.Fatalf("res = %+v, err = %v, target %q", res, err, got)
	}
	if life.atStop != "new-bytes" || !life.ranAtStop {
		t.Fatalf("at Stop the target held %q, running %v; want the new binary under a running service", life.atStop, life.ranAtStop)
	}
	if want := []string{"stop", "start", "health"}; !slices.Equal(life.log, want) {
		t.Fatalf("lifecycle %v, want %v", life.log, want)
	}
}

// TestManagedDefaultOrderUnchanged: without the option, a running service
// is stopped before its binary is replaced, and the result says so.
func TestManagedDefaultOrderUnchanged(t *testing.T) {
	life := &firstLife{running: true}
	_, exe := withTempHome(t)
	life.target = exe
	inner, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}})
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewManagedInstallerFor(inner, life, &fakeRec{})
	if err != nil {
		t.Fatal(err)
	}
	target, err := m.ResolveTarget(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := m.Begin(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sess.Close() }()
	res, err := sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: stageNew(t, sess)}})
	if err != nil || !res.Applied || res.ReplacedBeforeStop {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	if life.atStop != "old-bytes" {
		t.Fatalf("at Stop the target held %q; want the old binary", life.atStop)
	}
}

// TestReplaceBeforeStopStoppedService: a stopped service has nothing to
// stop, so the order does not apply.
func TestReplaceBeforeStopStoppedService(t *testing.T) {
	life := &enabledFirstLife{}
	res, got, err := replaceFirst(t, life, &fakeRec{}, early, func(p string) { life.target = p })
	if err != nil || !res.Applied || res.ReplacedBeforeStop || got != "new-bytes" {
		t.Fatalf("res = %+v, err = %v, target %q", res, err, got)
	}
	if want := []string{"start", "health"}; !slices.Equal(life.log, want) {
		t.Fatalf("lifecycle %v, want %v", life.log, want)
	}
}

// TestReplaceBeforeStopNotInstalled: with no service, the install is the
// standalone one.
func TestReplaceBeforeStopNotInstalled(t *testing.T) {
	res, got, err := replaceFirst(t, &fakeLife{}, &fakeRec{}, early, nil)
	if err != nil || !res.Applied || res.ServiceInstalled || res.ReplacedBeforeStop || got != "new-bytes" {
		t.Fatalf("res = %+v, err = %v, target %q", res, err, got)
	}
}

// TestReplaceBeforeStopProbeFails: a new binary that fails its post-install
// probe is rolled back under the running service, which is never stopped.
func TestReplaceBeforeStopProbeFails(t *testing.T) {
	life := &firstLife{running: true}
	o := early
	o.postInstall = ProberFunc(func(context.Context, ProbeRequest) error { return errors.New("fixture: bad binary") })
	res, got, err := replaceFirst(t, life, &fakeRec{}, o, func(p string) { life.target = p })
	if !errors.Is(err, ErrManagedInstall) || res.Applied || !res.RolledBack || !res.ReplacedBeforeStop || got != "old-bytes" {
		t.Fatalf("res = %+v, err = %v, target %q", res, err, got)
	}
	if len(life.log) != 0 || !life.running {
		t.Fatalf("lifecycle %v, running %v; want the service untouched", life.log, life.running)
	}
}

// TestReplaceBeforeStopApplyRefused: an Apply refused before its rename
// replaced nothing, and leaves the running service untouched.
func TestReplaceBeforeStopApplyRefused(t *testing.T) {
	life := &firstLife{running: true}
	o := early
	o.before = func(t *testing.T, target string) {
		t.Helper()
		// A journal no session resolved: no update replaces the target
		// (0017-PLAN N9).
		if err := os.WriteFile(filepath.Join(filepath.Dir(target), journalName(filepath.Base(target))), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	res, got, err := replaceFirst(t, life, &fakeRec{}, o, func(p string) { life.target = p })
	if !errors.Is(err, ErrManagedInstall) || err == nil || !strings.Contains(err.Error(), "journal") ||
		res.Applied || res.ReplacedBeforeStop || got != "old-bytes" {
		t.Fatalf("res = %+v, err = %v, target %q", res, err, got)
	}
	if len(life.log) != 0 || !life.running {
		t.Fatalf("lifecycle %v, running %v; want the service untouched", life.log, life.running)
	}
}

// TestReplaceBeforeStopStopFailsStillRunning: a stop that fails with the
// service still up rolls the binary back under it, and starts nothing.
func TestReplaceBeforeStopStopFailsStillRunning(t *testing.T) {
	life := &firstLife{running: true, stopErr: errors.New("fixture: stop refused")}
	res, got, err := replaceFirst(t, life, &fakeRec{}, early, func(p string) { life.target = p })
	if !errors.Is(err, ErrManagedInstall) || res.Applied || !res.RolledBack || !res.ReplacedBeforeStop || got != "old-bytes" {
		t.Fatalf("res = %+v, err = %v, target %q", res, err, got)
	}
	if want := []string{"stop"}; !slices.Equal(life.log, want) {
		t.Fatalf("lifecycle %v, want %v", life.log, want)
	}
}

// TestReplaceBeforeStopStopFailsWentDown: a stop that fails after the
// service went down rolls the binary back, then starts the service on it.
func TestReplaceBeforeStopStopFailsWentDown(t *testing.T) {
	life := &firstLife{running: true, stopErr: errors.New("fixture: stop timed out"), stopDown: true}
	res, got, err := replaceFirst(t, life, &fakeRec{}, early, func(p string) { life.target = p })
	if !errors.Is(err, ErrManagedInstall) || res.Applied || !res.RolledBack || !res.ReplacedBeforeStop || got != "old-bytes" {
		t.Fatalf("res = %+v, err = %v, target %q", res, err, got)
	}
	if want := []string{"stop", "start", "health"}; !slices.Equal(life.log, want) {
		t.Fatalf("lifecycle %v, want %v", life.log, want)
	}
}

// TestReplaceBeforeStopReconcileFails: a failed reconcile after the stop
// rolls back and starts the old binary, as without the option.
func TestReplaceBeforeStopReconcileFails(t *testing.T) {
	life := &firstLife{running: true}
	res, got, err := replaceFirst(t, life, &fakeRec{err: errors.New("fixture: reconcile")}, early, func(p string) { life.target = p })
	if !errors.Is(err, ErrManagedInstall) || res.Applied || !res.RolledBack || !res.ReplacedBeforeStop || got != "old-bytes" {
		t.Fatalf("res = %+v, err = %v, target %q", res, err, got)
	}
	if want := []string{"stop", "start", "health"}; !slices.Equal(life.log, want) {
		t.Fatalf("lifecycle %v, want %v", life.log, want)
	}
}

// TestReplaceBeforeStopHealthFails: an unhealthy new binary is stopped,
// rolled back, and the old one started and checked.
func TestReplaceBeforeStopHealthFails(t *testing.T) {
	life := &firstLife{running: true, healthErrs: 1}
	res, got, err := replaceFirst(t, life, &fakeRec{}, early, func(p string) { life.target = p })
	if !errors.Is(err, ErrManagedInstall) || res.Applied || !res.RolledBack || !res.ReplacedBeforeStop || got != "old-bytes" {
		t.Fatalf("res = %+v, err = %v, target %q", res, err, got)
	}
	if want := []string{"stop", "start", "health", "stop", "start", "health"}; !slices.Equal(life.log, want) {
		t.Fatalf("lifecycle %v, want %v", life.log, want)
	}
}

// TestReplaceBeforeStopCommitRefused: a commit refused after the directory
// was swapped is undone, as without the option.
func TestReplaceBeforeStopCommitRefused(t *testing.T) {
	life := &firstLife{running: true}
	rec := &swapRec{}
	res, _, err := replaceFirst(t, life, rec, early, func(p string) {
		life.target = p
		rec.dir = filepath.Dir(p)
		rec.moved = rec.dir + ".moved"
		t.Cleanup(func() {
			if _, err := os.Lstat(rec.moved); err == nil {
				_ = os.RemoveAll(rec.dir)
				_ = os.Rename(rec.moved, rec.dir)
			}
		})
	})
	if rec.swapErr != nil {
		if runtime.GOOS != goosWindows {
			t.Fatalf("directory swap failed: %v", rec.swapErr)
		}
		t.Skipf("the OS refused the swap: %v", rec.swapErr)
	}
	if !errors.Is(err, ErrConcurrentUpdate) || res.Applied || !res.RolledBack || !res.ReplacedBeforeStop {
		t.Fatalf("res = %+v, err = %v; want the refused commit rolled back", res, err)
	}
	if want := []string{"stop", "start", "health", "stop", "start", "health"}; !slices.Equal(life.log, want) {
		t.Fatalf("lifecycle %v, want %v", life.log, want)
	}
}

// TestNewManagedInstallerWithRefusesNil: the options constructor refuses
// what NewManagedInstallerFor refuses.
func TestNewManagedInstallerWithRefusesNil(t *testing.T) {
	_, exe := withTempHome(t)
	inner, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}})
	if err != nil {
		t.Fatal(err)
	}
	var nilInner *StandaloneInstaller
	var nilLife *fakeLife
	var nilRec *fakeRec
	opts := ManagedOptions{ReplaceBeforeStop: true}
	for name, try := range map[string]func() (*ManagedInstaller, error){
		"nil installer": func() (*ManagedInstaller, error) { return NewManagedInstallerWith(nil, &fakeLife{}, &fakeRec{}, opts) },
		"typed nil installer": func() (*ManagedInstaller, error) {
			return NewManagedInstallerWith(nilInner, &fakeLife{}, &fakeRec{}, opts)
		},
		"nil lifecycle":        func() (*ManagedInstaller, error) { return NewManagedInstallerWith(inner, nil, &fakeRec{}, opts) },
		"typed nil lifecycle":  func() (*ManagedInstaller, error) { return NewManagedInstallerWith(inner, nilLife, &fakeRec{}, opts) },
		"nil reconciler":       func() (*ManagedInstaller, error) { return NewManagedInstallerWith(inner, &fakeLife{}, nil, opts) },
		"typed nil reconciler": func() (*ManagedInstaller, error) { return NewManagedInstallerWith(inner, &fakeLife{}, nilRec, opts) },
	} {
		if m, err := try(); err == nil || m != nil {
			t.Errorf("%s: accepted (%v, %v)", name, m, err)
		}
	}
}
