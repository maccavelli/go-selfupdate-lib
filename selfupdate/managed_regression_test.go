package selfupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Regression tests for docs/decisions/0010-MADR-remediate-second-debugging-pass-findings.md
// B4, B7 and C5, and the managed test gaps
// (docs/decisions/0010-PLAN-v1-5-1-contract-preserving-fixes.md P4).

// seqRecoveryLife records the lifecycle calls in order. Health fails the first
// healthFails times; onHealth runs inside the first WaitHealthy. atStop
// records what the target held at each Stop.
type seqRecoveryLife struct {
	exe         string
	healthFails int
	onHealth    func()
	log         []string
	atStop      []string
}

func (l *seqRecoveryLife) Installed(context.Context, string) (bool, error) { return true, nil }
func (l *seqRecoveryLife) Running(context.Context, string) (bool, error)   { return true, nil }
func (l *seqRecoveryLife) Stop(context.Context, string) error {
	l.log = append(l.log, "stop")
	b, _ := os.ReadFile(l.exe)
	l.atStop = append(l.atStop, string(b))
	return nil
}
func (l *seqRecoveryLife) Start(context.Context, string) error {
	l.log = append(l.log, "start")
	return nil
}
func (l *seqRecoveryLife) WaitHealthy(context.Context, string) error {
	l.log = append(l.log, "health")
	if l.onHealth != nil {
		f := l.onHealth
		l.onHealth = nil
		f()
	}
	if l.healthFails > 0 {
		l.healthFails--
		return errors.New("unhealthy")
	}
	return nil
}

func seqRecoveryEnv(t *testing.T, life *seqRecoveryLife) (InstallSession, string) {
	t.Helper()
	_, exe := withTempHome(t)
	life.exe = exe
	inner, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}})
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewManagedInstaller(inner, life, &fakeRec{changed: true})
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
	return sess, exe
}

// TestManagedStopsBeforeRollback: when the new binary started but never
// became healthy, recovery stops it before restoring the old one, then
// starts the old one (B4).
func TestManagedStopsBeforeRollback(t *testing.T) {
	life := &seqRecoveryLife{healthFails: 1}
	sess, exe := seqRecoveryEnv(t, life)
	path := stageNew(t, sess)
	_, err := sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: path}})
	if !errors.Is(err, ErrManagedInstall) {
		t.Fatalf("err = %v", err)
	}
	if got := strings.Join(life.log, " "); got != "stop start health stop start health" {
		t.Fatalf("lifecycle %q, want stop start health stop start health", got)
	}
	if len(life.atStop) != 2 || life.atStop[1] != "new-bytes" {
		t.Fatalf("target at each stop %q: the second stop must come before the rollback", life.atStop)
	}
	if got := readString(t, exe); got != "old-bytes" {
		t.Fatalf("target = %q, want the old binary", got)
	}
}

// TestManagedCommitSwapRecovers: a directory swapped while the new binary
// was being health-checked makes Commit refuse; the managed session then
// recovers in the locked directory instead of leaving the new binary live
// (B7).
func TestManagedCommitSwapRecovers(t *testing.T) {
	if runtime.GOOS == goosWindows {
		t.Skip("a directory swap is refused on Windows")
	}
	life := &seqRecoveryLife{}
	sess, exe := seqRecoveryEnv(t, life)
	dir := filepath.Dir(exe)
	moved := dir + ".moved"
	life.onHealth = func() {
		if err := os.Rename(dir, moved); err != nil {
			t.Fatalf("swap: %v", err)
		}
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
		_ = os.Rename(moved, dir)
	})
	path := stageNew(t, sess)
	res, err := sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: path}})
	if !errors.Is(err, ErrManagedInstall) || !errors.Is(err, ErrConcurrentUpdate) {
		t.Fatalf("err = %v, want a managed failure from the concurrent update", err)
	}
	if got := readString(t, filepath.Join(moved, "demo")); got != "old-bytes" {
		t.Fatalf("locked directory's target = %q, want the old binary back", got)
	}
	if !res.RolledBack {
		t.Fatalf("RolledBack = false: %+v", res)
	}
	if got := strings.Join(life.log, " "); got != "stop start health stop start health" {
		t.Fatalf("lifecycle %q", got)
	}
}

// TestManagedRefusesTypedNil: a typed nil Lifecycle or Reconciler is
// refused at construction, not found by a panic in Install (C5).
func TestManagedRefusesTypedNil(t *testing.T) {
	_, exe := withTempHome(t)
	inner, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}})
	if err != nil {
		t.Fatal(err)
	}
	var nilLife *fakeLife
	var nilRec *fakeRec
	if _, err := NewManagedInstaller(inner, nilLife, &fakeRec{}); err == nil {
		t.Fatal("a typed nil Lifecycle was accepted")
	}
	if _, err := NewManagedInstaller(inner, &fakeLife{}, nilRec); err == nil {
		t.Fatal("a typed nil Reconciler was accepted")
	}
}

// TestManagedWithTransformer: a Transformer runs through the managed
// session, which owns the transformed staging and names the target, and the
// service is stopped, updated and started (the P4 test gap).
func TestManagedWithTransformer(t *testing.T) {
	_, exe := withTempHome(t)
	life := &fakeLife{installed: true, running: true}
	inner, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}})
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewManagedInstaller(inner, life, &fakeRec{})
	if err != nil {
		t.Fatal(err)
	}
	rel, bodies, plats := probeRelease(t)
	sel, err := NewExactAssetSelector(plats)
	if err != nil {
		t.Fatal(err)
	}
	u, err := New(Config{Source: &scriptSource{rel: rel, bodies: bodies}, Versions: NewStrictVersionPolicy(), Assets: sel,
		Transformer: growTransformer{extra: []byte("-signed")},
		Installer:   m, Reporter: &recReporter{}, Confirmer: &recConfirmer{}, Limits: DefaultLimits()})
	if err != nil {
		t.Fatal(err)
	}
	req := applyReq()
	req.Yes = true
	res, err := u.Run(context.Background(), req)
	if err != nil || !res.Applied || !res.ServiceInstalled || !res.ServiceWasRunning {
		t.Fatalf("Run = %+v, %v", res, err)
	}
	if got := readString(t, exe); !strings.HasSuffix(got, "-signed") {
		t.Fatalf("target = %q, want the transformed binary", got)
	}
	if life.stops != 1 || life.starts != 1 || life.healths != 1 {
		t.Fatalf("lifecycle counts %+v", life)
	}
}

// TestManagedRecoveryReportsRestoredBinary: when recovery leaves the
// previous binary in place, the result says it was rolled back and names
// no backup, whether the restore's directory sync failed or Apply itself
// undid the replacement
// (docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md B4).
func TestManagedRecoveryReportsRestoredBinary(t *testing.T) {
	check := func(t *testing.T, res InstallResult, err error, target string) {
		t.Helper()
		if !errors.Is(err, ErrManagedInstall) || res.Applied {
			t.Fatalf("Applied=%v err=%v", res.Applied, err)
		}
		if !res.RolledBack || res.Backup != "" {
			t.Fatalf("RolledBack=%v Backup=%q; the previous binary is back (err=%v)", res.RolledBack, res.Backup, err)
		}
		if got := readString(t, target); got != "old-bytes" {
			t.Fatalf("target holds %q", got)
		}
	}
	t.Run("unsynced rollback", func(t *testing.T) {
		life := &fakeLife{installed: true, running: true, healthErr: errors.New("unhealthy")}
		_, _, sess, exe := managedEnv(t, life, &fakeRec{})
		path := stageNew(t, sess)
		real := syncDirFn
		calls := 0
		setSeam(t, &syncDirFn, func(dir string) error {
			calls++
			if calls == 2 {
				return errors.New("injected sync failure after the restore")
			}
			return real(dir)
		})
		res, err := sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: path}})
		check(t, res, err, exe)
	})
	t.Run("undone apply", func(t *testing.T) {
		life := &fakeLife{installed: true, running: true}
		_, _, sess, exe := managedEnv(t, life, &fakeRec{})
		path := stageNew(t, sess)
		moved := swapAfterFirstReplace(t, filepath.Dir(exe), false)
		res, err := sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: path}})
		check(t, res, err, filepath.Join(moved, filepath.Base(exe)))
	})
}
