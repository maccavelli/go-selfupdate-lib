package selfupdate

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

type fakeLife struct {
	installed    bool
	running      bool
	installedErr error
	runningErr   error
	stopErr      error
	startErr     error
	healthErr    error
	onHealth     func() // runs inside every WaitHealthy call
	stops        int
	starts       int
	healths      int
	startCtxErrs []error // ctx.Err() seen by each Start
	// stopTakesEffect stops the service even when Stop returns stopErr, as
	// a stop whose wait timed out after the service stopped does.
	stopTakesEffect bool
}

func (f *fakeLife) Installed(context.Context, string) (bool, error) {
	return f.installed, f.installedErr
}
func (f *fakeLife) Running(context.Context, string) (bool, error) { return f.running, f.runningErr }
func (f *fakeLife) Stop(context.Context, string) error {
	f.stops++
	if f.stopErr == nil || f.stopTakesEffect {
		f.running = false
	}
	return f.stopErr
}
func (f *fakeLife) Start(ctx context.Context, _ string) error {
	f.starts++
	f.startCtxErrs = append(f.startCtxErrs, ctx.Err())
	return f.startErr
}
func (f *fakeLife) WaitHealthy(context.Context, string) error {
	f.healths++
	if f.onHealth != nil {
		f.onHealth()
	}
	return f.healthErr
}

type fakeRec struct {
	changed  bool
	err      error
	restores int
	restoreE error
}

func (f *fakeRec) Reconcile(context.Context, string, string) (ReconcileResult, error) {
	return ReconcileResult{Changed: f.changed, State: "unit"}, f.err
}
func (f *fakeRec) Restore(context.Context, string, ReconcileResult) error {
	f.restores++
	return f.restoreE
}

func managedEnv(t *testing.T, life *fakeLife, rec *fakeRec) (*ManagedInstaller, Target, InstallSession, string) {
	t.Helper()
	_, exe := tempTarget(t)
	inner, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: targetPolicy(exe)})
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewManagedInstaller(inner, life, rec)
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
	t.Cleanup(func() { _ = sess.Close() })
	return m, target, sess, exe
}

func stageNew(t *testing.T, sess InstallSession) string {
	t.Helper()
	f, path, err := sess.CreateStaging(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("new-bytes")); err != nil {
		t.Fatal(err)
	}
	if err := f.Sync(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestManagedAbsentIsBinaryOnly(t *testing.T) {
	t.Parallel()
	life := &fakeLife{}
	rec := &fakeRec{}
	_, _, sess, exe := managedEnv(t, life, rec)
	path := stageNew(t, sess)
	res, err := sess.Install(context.Background(), InstallRequest{
		Product:  "demo",
		Artifact: StagedArtifact{Path: path},
	})
	if err != nil {
		t.Fatal(err)
	}
	if life.stops != 0 || life.starts != 0 || rec.restores != 0 {
		t.Fatalf("service called on absent definition: %+v %+v", life, rec)
	}
	if !res.Applied || res.ServiceInstalled {
		t.Fatalf("%+v", res)
	}
	got, _ := os.ReadFile(exe)
	if string(got) != "new-bytes" {
		t.Fatalf("%q", got)
	}
}

func TestManagedRunningStopReconcileStart(t *testing.T) {
	t.Parallel()
	life := &fakeLife{installed: true, running: true}
	rec := &fakeRec{changed: true}
	_, _, sess, exe := managedEnv(t, life, rec)
	path := stageNew(t, sess)
	res, err := sess.Install(context.Background(), InstallRequest{
		Product:  "demo",
		Artifact: StagedArtifact{Path: path},
	})
	if err != nil {
		t.Fatal(err)
	}
	if life.stops != 1 || life.starts != 1 || life.healths != 1 {
		t.Fatalf("lifecycle counts %+v", life)
	}
	if !res.ServiceInstalled || !res.ServiceWasRunning {
		t.Fatalf("%+v", res)
	}
	got, _ := os.ReadFile(exe)
	if string(got) != "new-bytes" {
		t.Fatalf("%q", got)
	}
}

// TestManagedStoppedStaysStopped: a stopped service whose Lifecycle cannot
// say it is configured to start is updated and left stopped. It was
// TestManagedDownHeals, which started it: the owner's answer to 0010-MADR
// Q2 reversed that.
func TestManagedStoppedStaysStopped(t *testing.T) {
	t.Parallel()
	life := &fakeLife{installed: true, running: false}
	rec := &fakeRec{changed: true}
	_, _, sess, exe := managedEnv(t, life, rec)
	path := stageNew(t, sess)
	res, err := sess.Install(context.Background(), InstallRequest{
		Product:  "demo",
		Artifact: StagedArtifact{Path: path},
	})
	if err != nil {
		t.Fatal(err)
	}
	if life.stops != 0 || life.starts != 0 || life.healths != 0 {
		t.Fatalf("lifecycle counts %+v", life)
	}
	if !res.Applied || !res.ServiceInstalled || res.ServiceWasRunning || res.ServiceStarted || readString(t, exe) != "new-bytes" {
		t.Fatalf("res = %+v", res)
	}
}

func TestManagedHealthFailureRollsBack(t *testing.T) {
	t.Parallel()
	life := &fakeLife{installed: true, running: true, healthErr: errors.New("unhealthy")}
	rec := &fakeRec{changed: true}
	_, _, sess, exe := managedEnv(t, life, rec)
	orig, _ := os.ReadFile(exe)
	path := stageNew(t, sess)
	_, err := sess.Install(context.Background(), InstallRequest{
		Product:  "demo",
		Artifact: StagedArtifact{Path: path},
	})
	if !errors.Is(err, ErrManagedInstall) {
		t.Fatalf("err = %v", err)
	}
	if rec.restores != 1 {
		t.Fatalf("restores = %d", rec.restores)
	}
	got, _ := os.ReadFile(exe)
	if string(got) != string(orig) {
		t.Fatalf("binary not restored: %q", got)
	}
	if life.starts < 2 {
		t.Fatalf("expected restart of prior service, starts=%d", life.starts)
	}
}

func TestManagedReconcileFailureRollsBack(t *testing.T) {
	t.Parallel()
	life := &fakeLife{installed: true, running: true}
	rec := &fakeRec{changed: true, err: errors.New("partial write")}
	_, _, sess, exe := managedEnv(t, life, rec)
	orig, _ := os.ReadFile(exe)
	path := stageNew(t, sess)
	_, err := sess.Install(context.Background(), InstallRequest{
		Product:  "demo",
		Artifact: StagedArtifact{Path: path},
	})
	if !errors.Is(err, ErrManagedInstall) {
		t.Fatalf("err = %v", err)
	}
	if rec.restores != 1 {
		t.Fatalf("must restore even when Reconcile errors, restores=%d", rec.restores)
	}
	got, _ := os.ReadFile(exe)
	if string(got) != string(orig) {
		t.Fatalf("binary not restored: %q", got)
	}
}

func TestManagedRollbackErrorJoined(t *testing.T) {
	t.Parallel()
	life := &fakeLife{installed: true, running: true, healthErr: errors.New("unhealthy")}
	rec := &fakeRec{changed: true, restoreE: errors.New("restore failed")}
	_, _, sess, _ := managedEnv(t, life, rec)
	path := stageNew(t, sess)
	err := error(nil)
	_, err = sess.Install(context.Background(), InstallRequest{
		Product:  "demo",
		Artifact: StagedArtifact{Path: path},
	})
	if err == nil || !errors.Is(err, ErrManagedInstall) {
		t.Fatalf("err = %v", err)
	}
	// Unconditional: the restore failure must be joined into the error, and
	// Restore must have run exactly once (0003-MADR B8).
	if !errors.Is(err, rec.restoreE) {
		t.Fatalf("restore error not joined: %v", err)
	}
	if rec.restores != 1 {
		t.Fatalf("restores = %d", rec.restores)
	}
}

// TestManagedStopFailsAfterStoppingRestarts: a Stop that fails after the
// service stopped is recovered: the service is started again and checked,
// and the update fails with ErrManagedInstall and the stop's error. The
// binary was never replaced
// (docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md B3).
func TestManagedStopFailsAfterStoppingRestarts(t *testing.T) {
	t.Parallel()
	stopErr := errors.New("injected: stop wait timed out")
	life := &fakeLife{installed: true, running: true, stopErr: stopErr, stopTakesEffect: true}
	_, _, sess, exe := managedEnv(t, life, &fakeRec{})
	res, err := sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: stageNew(t, sess)}})
	if !errors.Is(err, ErrManagedInstall) || !errors.Is(err, stopErr) || res.Applied {
		t.Fatalf("Applied=%v err=%v", res.Applied, err)
	}
	if life.stops != 1 || life.starts != 1 || life.healths != 1 {
		t.Fatalf("stops=%d starts=%d healths=%d; want the stopped service started and checked", life.stops, life.starts, life.healths)
	}
	if got := readString(t, exe); got != "old-bytes" {
		t.Fatalf("target holds %q", got)
	}
}

// TestManagedStopFailsStillRunning: a Stop that failed and left the
// service running, such as one refused from inside the service, ends the
// update as before: nothing is started (0015-MADR B3).
func TestManagedStopFailsStillRunning(t *testing.T) {
	t.Parallel()
	stopErr := errors.New("injected: refused inside the service")
	life := &fakeLife{installed: true, running: true, stopErr: stopErr}
	_, _, sess, exe := managedEnv(t, life, &fakeRec{})
	res, err := sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: stageNew(t, sess)}})
	if !errors.Is(err, ErrManagedInstall) || !errors.Is(err, stopErr) || res.Applied {
		t.Fatalf("Applied=%v err=%v", res.Applied, err)
	}
	if life.stops != 1 || life.starts != 0 {
		t.Fatalf("stops=%d starts=%d; a running service is not started again", life.stops, life.starts)
	}
	if got := readString(t, exe); got != "old-bytes" {
		t.Fatalf("target holds %q", got)
	}
}

// blockingStop is a fakeLife whose Stop waits for release, so a test can
// call another session method while Install is inside it.
type blockingStop struct {
	*fakeLife
	entered, release chan struct{}
}

func (b *blockingStop) Stop(ctx context.Context, product string) error {
	close(b.entered)
	<-b.release
	return b.fakeLife.Stop(ctx, product)
}

// TestManagedSessionSerializes: Close, called while Install is inside the
// service's Stop, waits for Install to return, as InstallSession's doc
// says; Install then completes
// (docs/decisions/0027-MADR-documentation-pass-after-v1-17-0.md C3). The
// 200 ms wait only bounds how long the test looks for a Close that
// returned early; it orders nothing.
func TestManagedSessionSerializes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, exe := tempTarget(t)
	inner, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: targetPolicy(exe)})
	if err != nil {
		t.Fatal(err)
	}
	life := &blockingStop{fakeLife: &fakeLife{installed: true, running: true}, entered: make(chan struct{}), release: make(chan struct{})}
	m, err := NewManagedInstaller(inner, life, &fakeRec{})
	if err != nil {
		t.Fatal(err)
	}
	target, err := m.ResolveTarget(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := m.Begin(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	path := stageNew(t, sess)
	installed := make(chan error, 1)
	go func() {
		_, err := sess.Install(ctx, InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: path}})
		installed <- err
	}()
	<-life.entered
	closed := make(chan error, 1)
	go func() { closed <- sess.Close() }()
	select {
	case err := <-closed:
		close(life.release)
		<-installed
		t.Fatalf("Close returned (%v) while Install was inside Stop", err)
	case <-time.After(200 * time.Millisecond):
	}
	close(life.release)
	if err := <-installed; err != nil {
		t.Fatalf("Install: %v", err)
	}
	if err := <-closed; err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got, _ := os.ReadFile(exe); string(got) != "new-bytes" {
		t.Fatalf("target holds %q", got)
	}
}

// TestManagedSessionTarget: a managed session reports the target its inner
// session locked (0015-MADR B10).
func TestManagedSessionTarget(t *testing.T) {
	t.Parallel()
	_, target, sess, _ := managedEnv(t, &fakeLife{installed: true}, &fakeRec{})
	if got := sess.Target(); got.Path != target.Path || got.Dir != target.Dir || got.Base != target.Base {
		t.Fatalf("Target() = %+v, want %+v", got, target)
	}
}
