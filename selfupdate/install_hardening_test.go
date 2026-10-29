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

// Tests for the install path, added by
// docs/decisions/0003-PLAN-remediate-debugging-pass-findings.md Phase 3.

// standaloneSession begins a standalone session on a fresh temp home and
// returns it with the target path.
func standaloneSession(t *testing.T) (InstallSession, string) {
	t.Helper()
	_, exe := tempTarget(t)
	inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: targetPolicy(exe)})
	if err != nil {
		t.Fatal(err)
	}
	target, err := inst.ResolveTarget(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := inst.Begin(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	return sess, exe
}

// fsOf is the file-system effects sess uses, or the standalone session
// under a managed one uses; a test that changes them changes that session
// alone
// (docs/decisions/0038-MADR-pinned-plans-lifecycle-conformance-and-bubble-tests.md
// D3-A).
func fsOf(t *testing.T, sess InstallSession) *fsOps {
	t.Helper()
	switch s := sess.(type) {
	case *installSession:
		return s.fs
	case *managedSession:
		return fsOf(t, s.inner)
	}
	t.Fatalf("no file-system effects in a %T", sess)
	return nil
}

// fsFor gives inst file-system effects of its own, which every session it
// begins shares, and returns them for the test to change.
func fsFor(inst *StandaloneInstaller) *fsOps {
	inst.fs = realFS()
	return inst.fs
}

// syncsAfterJournal sends fx's directory syncs after the journal is
// written to sync, numbered from 1: the replacement's, then a commit's or
// a rollback's. The syncs before it, the journal's own included, are real.
// Every flush goes through the session's root
// (docs/decisions/0038-PLAN-pinned-plans-lifecycle-conformance-and-bubble-tests.md
// Q13), so the number tells them apart.
func syncsAfterJournal(fx *fsOps, sync func(n int, root *os.Root) error) {
	realJournal := fx.writeJournalFn
	journaled, n := false, 0
	fx.writeJournalFn = func(root *os.Root, target Target, j pendingJournal) error {
		err := realJournal(root, target, j)
		journaled = true
		return err
	}
	fx.syncRootFn = func(root *os.Root) error {
		if !journaled {
			return syncRoot(root)
		}
		n++
		return sync(n, root)
	}
}

func readString(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func stagingLeft(t *testing.T, dir, base string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var left []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "."+base+".selfupdate-") {
			left = append(left, e.Name())
		}
	}
	return left
}

// TestInstallSyncAndRollbackFailureReported: when the directory sync and
// the restore both fail, both errors are returned and the live backup is
// reported (B1).
func TestInstallSyncAndRollbackFailureReported(t *testing.T) {
	t.Parallel()
	sess, exe := standaloneSession(t)
	path := stageNew(t, sess)
	syncErr := errors.New("injected dir sync failure")
	restoreErr := errors.New("injected restore failure")
	syncsAfterJournal(fsOf(t, sess), func(int, *os.Root) error { return syncErr })
	fx := fsOf(t, sess)
	real := fx.replacePath
	calls := 0
	fx.replacePath = func(ctx context.Context, root *os.Root, oldname, newname string) error {
		calls++
		if calls == 1 {
			return real(ctx, root, oldname, newname)
		}
		return restoreErr
	}
	res, err := sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: path}})
	if !errors.Is(err, syncErr) || !errors.Is(err, restoreErr) {
		t.Fatalf("err = %v, want both the sync and the restore failure", err)
	}
	if res.Applied || res.Backup == "" {
		t.Fatalf("res = %+v, want the live backup reported", res)
	}
	if got := readString(t, exe); got != "new-bytes" {
		t.Fatalf("target %q", got)
	}
	if got := readString(t, res.Backup); got != "old-bytes" {
		t.Fatalf("backup %q", got)
	}
}

// TestManagedApplyFailureRetriesRollback: the managed path receives the
// live backup and restores it during recovery (B1).
func TestManagedApplyFailureRetriesRollback(t *testing.T) {
	t.Parallel()
	life := &fakeLife{installed: true, running: true}
	_, _, sess, exe := managedEnv(t, life, &fakeRec{})
	path := stageNew(t, sess)
	fx := fsOf(t, sess)
	syncsAfterJournal(fx, func(n int, root *os.Root) error {
		if n == 1 {
			return errors.New("injected dir sync failure")
		}
		return syncRoot(root)
	})
	real := fx.replacePath
	calls := 0
	fx.replacePath = func(ctx context.Context, root *os.Root, oldname, newname string) error {
		calls++
		if calls == 2 {
			return errors.New("injected restore failure")
		}
		return real(ctx, root, oldname, newname)
	}
	_, err := sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: path}})
	if !errors.Is(err, ErrManagedInstall) {
		t.Fatalf("err = %v", err)
	}
	if got := readString(t, exe); got != "old-bytes" {
		t.Fatalf("recovery did not restore the backup: target %q", got)
	}
	if calls != 3 || life.starts != 1 {
		t.Fatalf("replace calls=%d starts=%d", calls, life.starts)
	}
}

// TestManagedRecoveryIgnoresCancelledContext: recovery runs on a live
// context even when the caller's was cancelled (B2).
func TestManagedRecoveryIgnoresCancelledContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	life := &fakeLife{installed: true, running: true, healthErr: context.Canceled, onHealth: cancel}
	_, _, sess, exe := managedEnv(t, life, &fakeRec{})
	path := stageNew(t, sess)
	_, err := sess.Install(ctx, InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: path}})
	if !errors.Is(err, ErrManagedInstall) {
		t.Fatalf("err = %v", err)
	}
	if life.starts != 2 {
		t.Fatalf("starts = %d, want the service restarted after rollback", life.starts)
	}
	if life.startCtxErrs[1] != nil {
		t.Fatalf("recovery Start saw a done context: %v", life.startCtxErrs[1])
	}
	if got := readString(t, exe); got != "old-bytes" {
		t.Fatalf("target %q", got)
	}
}

// TestBackupCopyFallbackPreservesMode: a copied backup keeps the
// executable's mode, so a rollback restores a runnable binary (B3).
func TestBackupCopyFallbackPreservesMode(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == goosWindows {
		t.Skip("POSIX permission bits: Windows keeps only a read-only flag")
	}
	life := &fakeLife{installed: true, running: true, healthErr: errors.New("unhealthy")}
	_, _, sess, exe := managedEnv(t, life, &fakeRec{})
	fsOf(t, sess).rootLink = func(*os.Root, string, string) error { return errors.New("injected link failure") }
	before, err := os.Stat(exe)
	if err != nil {
		t.Fatal(err)
	}
	path := stageNew(t, sess)
	if _, err := sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: path}}); err == nil {
		t.Fatal("expected the health failure")
	}
	after, err := os.Stat(exe)
	if err != nil {
		t.Fatal(err)
	}
	if got := readString(t, exe); got != "old-bytes" {
		t.Fatalf("target %q", got)
	}
	if after.Mode().Perm() != before.Mode().Perm() {
		t.Fatalf("mode after rollback %v, want %v", after.Mode().Perm(), before.Mode().Perm())
	}
}

// TestInstallFailureBeforeRenameRemovesStagingOnClose (B4).
func TestInstallFailureBeforeRenameRemovesStagingOnClose(t *testing.T) {
	t.Parallel()
	sess, exe := standaloneSession(t)
	path := stageNew(t, sess)
	fsOf(t, sess).replacePath = func(context.Context, *os.Root, string, string) error { return errors.New("injected rename failure") }
	if _, err := sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: path}}); err == nil {
		t.Fatal("expected the rename failure")
	}
	if err := sess.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("staging left after Close: %v", err)
	}
	if left := stagingLeft(t, filepath.Dir(exe), "demo"); len(left) != 0 {
		t.Fatalf("leftovers: %v", left)
	}
}

func symlinkOrSkip(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		if runtime.GOOS == goosWindows {
			t.Skipf("symlink creation not permitted on this Windows host: %v", err)
		}
		t.Fatal(err)
	}
}

// TestLockRejectsRelativeSymlink: a lock name that is a relative symlink
// inside the directory is refused, not followed (B5).
func TestLockRejectsRelativeSymlink(t *testing.T) {
	t.Parallel()
	home, exe := tempTarget(t)
	victim := filepath.Join(home, "victim")
	if err := os.WriteFile(victim, []byte("v"), 0o600); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, "victim", filepath.Join(home, lockBasename("demo")))
	inst, _ := NewStandaloneInstaller(InstallOptions{TargetPolicy: targetPolicy(exe)})
	target, err := inst.ResolveTarget(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if sess, err := inst.Begin(context.Background(), target); err == nil {
		_ = sess.Close()
		t.Fatal("Begin followed a relative lock symlink")
	}
}

// TestLockRejectsDanglingRelativeSymlink: a dangling relative symlink does
// not make Begin create the file it points at (B5).
func TestLockRejectsDanglingRelativeSymlink(t *testing.T) {
	t.Parallel()
	home, exe := tempTarget(t)
	symlinkOrSkip(t, "created-by-lock", filepath.Join(home, lockBasename("demo")))
	inst, _ := NewStandaloneInstaller(InstallOptions{TargetPolicy: targetPolicy(exe)})
	target, err := inst.ResolveTarget(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if sess, err := inst.Begin(context.Background(), target); err == nil {
		_ = sess.Close()
		t.Fatal("Begin followed a dangling lock symlink")
	}
	if _, err := os.Lstat(filepath.Join(home, "created-by-lock")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the symlink's target was created: %v", err)
	}
}

// TestInstallDetectsSwappedDirectory: a directory swapped in after Begin is
// refused with ErrConcurrentUpdate (B10).
func TestInstallDetectsSwappedDirectory(t *testing.T) {
	t.Parallel()
	sess, exe := standaloneSession(t)
	path := stageNew(t, sess)
	dir := filepath.Dir(exe)
	if err := os.Rename(dir, dir+".old"); err != nil {
		if runtime.GOOS == goosWindows {
			t.Skipf("Windows refuses to rename a directory with open handles, which prevents the swap: %v", err)
		}
		t.Fatal(err)
	}
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("other-bytes"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Plant a regular file at the staging name in the swapped-in directory,
	// so only the directory identity check can refuse the install.
	if err := os.WriteFile(path, []byte("planted-bytes"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: path}})
	if !errors.Is(err, ErrConcurrentUpdate) {
		t.Fatalf("err = %v, want ErrConcurrentUpdate", err)
	}
	if got := readString(t, exe); got != "other-bytes" {
		t.Fatalf("swapped-in target changed: %q", got)
	}
}

// --- B9: required failure tests ---------------------------------------

// TestStagingRejectsPlantedSymlink: a symlink swapped in at the staging
// path is refused, and the target is untouched.
func TestStagingRejectsPlantedSymlink(t *testing.T) {
	t.Parallel()
	sess, exe := standaloneSession(t)
	path := stageNew(t, sess)
	other := filepath.Join(filepath.Dir(exe), "planted")
	if err := os.WriteFile(other, []byte("planted-bytes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, "planted", path)
	if _, err := sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: path}}); err == nil {
		t.Fatal("installed a planted staging symlink")
	}
	if got := readString(t, exe); got != "old-bytes" {
		t.Fatalf("target %q", got)
	}
}

// TestInstallInjectedFailures: each injected failure is reported and leaves
// the target as the case requires.
func TestInstallInjectedFailures(t *testing.T) {
	t.Parallel()
	failing := errors.New("injected")
	cases := []struct {
		name       string
		inject     func(t *testing.T, fx *fsOps)
		wantTarget string
		wantApply  bool
	}{
		{"chmod", func(t *testing.T, fx *fsOps) {
			fx.fileChmod = func(*os.File, os.FileMode) error { return failing }
		}, "old-bytes", false},
		{"rename", func(t *testing.T, fx *fsOps) {
			fx.replacePath = func(context.Context, *os.Root, string, string) error { return failing }
		}, "old-bytes", false},
		{"dir sync, restored", func(t *testing.T, fx *fsOps) {
			syncsAfterJournal(fx, func(int, *os.Root) error { return failing })
		}, "old-bytes", false},
		{"commit remove backup", func(t *testing.T, fx *fsOps) {
			fx.rootRemove = func(*os.Root, string) error { return failing }
		}, "new-bytes", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sess, exe := standaloneSession(t)
			path := stageNew(t, sess)
			tc.inject(t, fsOf(t, sess))
			res, err := sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: path}})
			if !errors.Is(err, failing) {
				t.Fatalf("err = %v", err)
			}
			if res.Applied != tc.wantApply {
				t.Fatalf("applied = %v", res.Applied)
			}
			if got := readString(t, exe); got != tc.wantTarget {
				t.Fatalf("target %q, want %q", got, tc.wantTarget)
			}
			noJournalIn(t, filepath.Dir(exe))
		})
	}
}

// TestInstallPermissionDenied: an unwritable directory fails at staging and
// leaves the target intact.
func TestInstallPermissionDenied(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == goosWindows {
		t.Skip("POSIX directory permissions")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	sess, exe := standaloneSession(t)
	dir := filepath.Dir(exe)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	if f, _, err := sess.CreateStaging(context.Background()); err == nil {
		_ = f.Close()
		t.Fatal("created staging in a read-only directory")
	}
	if got := readString(t, exe); got != "old-bytes" {
		t.Fatalf("target %q", got)
	}
}

// TestInstallCancelledBeforeAndAfterStaging.
func TestInstallCancelledBeforeAndAfterStaging(t *testing.T) {
	t.Parallel()
	sess, exe := standaloneSession(t)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if f, _, err := sess.CreateStaging(cancelled); !errors.Is(err, context.Canceled) {
		if f != nil {
			_ = f.Close()
		}
		t.Fatalf("CreateStaging err = %v", err)
	}
	path := stageNew(t, sess)
	if _, err := sess.Install(cancelled, InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: path}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Install err = %v", err)
	}
	if err := sess.Close(); err != nil {
		t.Fatal(err)
	}
	if got := readString(t, exe); got != "old-bytes" {
		t.Fatalf("target %q", got)
	}
	if left := stagingLeft(t, filepath.Dir(exe), "demo"); len(left) != 0 {
		t.Fatalf("leftovers: %v", left)
	}
}

// TestRunBadBodyLeavesNoStaging: a body shorter or longer than its declared
// size fails the run and leaves no staging file.
func TestRunBadBodyLeavesNoStaging(t *testing.T) {
	t.Parallel()
	for name, body := range map[string][]byte{"short": []byte("hello"), "long": []byte("hello-bin-and-more")} {
		t.Run(name, func(t *testing.T) {
			rel, bodies, plats := fixtureRelease(t, "demo")
			bodies[2] = body
			_, exe := tempTarget(t)
			inst, _ := NewStandaloneInstaller(InstallOptions{TargetPolicy: targetPolicy(exe)})
			sel, _ := NewExactAssetSelector(plats)
			u, err := New(Config{
				Source: &scriptSource{rel: rel, bodies: bodies}, Versions: NewStrictVersionPolicy(), Assets: sel,
				Installer: inst, Reporter: &recReporter{}, Confirmer: &recConfirmer{ok: true}, Limits: DefaultLimits(),
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := u.Run(context.Background(), Request{Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: ReleaseBuild, Yes: true}); !errors.Is(err, ErrIntegrity) {
				t.Fatalf("err = %v", err)
			}
			if left := stagingLeft(t, filepath.Dir(exe), "demo"); len(left) != 0 {
				t.Fatalf("leftovers: %v", left)
			}
			if got := readString(t, exe); got != "old-bytes" {
				t.Fatalf("target %q", got)
			}
		})
	}
}

// TestManagedFailureMatrix: stop, start, probe and restart failures are
// each reported, recovery runs, and the old binary is back.
func TestManagedFailureMatrix(t *testing.T) {
	t.Parallel()
	boom := errors.New("injected")
	cases := []struct {
		name       string
		life       fakeLife
		wantStarts int
		wantTarget string
	}{
		{"installed probe fails", fakeLife{installedErr: boom}, 0, "old-bytes"},
		{"running probe fails", fakeLife{installed: true, runningErr: boom}, 0, "old-bytes"},
		{"stop fails", fakeLife{installed: true, running: true, stopErr: boom}, 0, "old-bytes"},
		{"start fails, restart fails", fakeLife{installed: true, running: true, startErr: boom}, 2, "old-bytes"},
		{"health fails, restart ok", fakeLife{installed: true, running: true, healthErr: boom}, 2, "old-bytes"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			life := tc.life
			_, _, sess, exe := managedEnv(t, &life, &fakeRec{})
			path := stageNew(t, sess)
			_, err := sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: path}})
			if !errors.Is(err, ErrManagedInstall) || !errors.Is(err, boom) {
				t.Fatalf("err = %v", err)
			}
			if life.starts != tc.wantStarts {
				t.Fatalf("starts = %d, want %d", life.starts, tc.wantStarts)
			}
			if got := readString(t, exe); got != tc.wantTarget {
				t.Fatalf("target %q", got)
			}
		})
	}
}

// TestInstallReportsRestoreAfterSyncFailure: Install reports RolledBack,
// and no backup, when the previous binary is back in place: after the
// directory sync failed and the restore succeeded, and after the rollback
// in the locked directory renamed the backup back and only its sync failed
// (docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md B4).
func TestInstallReportsRestoreAfterSyncFailure(t *testing.T) {
	t.Parallel()
	check := func(t *testing.T, res InstallResult, err error, target string) {
		t.Helper()
		if err == nil || res.Applied {
			t.Fatalf("Applied=%v err=%v; want a failed, unapplied install", res.Applied, err)
		}
		if !res.RolledBack || res.Backup != "" {
			t.Fatalf("RolledBack=%v Backup=%q; the previous binary is back (err=%v)", res.RolledBack, res.Backup, err)
		}
		if got := readString(t, target); got != "old-bytes" {
			t.Fatalf("target holds %q", got)
		}
	}
	t.Run("restored after a failed sync", func(t *testing.T) {
		sess, exe := standaloneSession(t)
		path := stageNew(t, sess)
		syncsAfterJournal(fsOf(t, sess), func(n int, root *os.Root) error {
			if n == 1 {
				return errors.New("injected dir sync failure")
			}
			return syncRoot(root)
		})
		res, err := sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: path}})
		check(t, res, err, exe)
	})
	t.Run("rolled back in the locked directory, unsynced", func(t *testing.T) {
		sess, exe := standaloneSession(t)
		path := stageNew(t, sess)
		moved := swapAfterFirstReplace(t, fsOf(t, sess), filepath.Dir(exe), false)
		// The replacement's own sync, the first after the journal's
		// (docs/decisions/0017-PLAN-verify-build-provenance-and-close-0015-open-items.md
		// Q1), passes; the rollback's, after it, fails.
		syncsAfterJournal(fsOf(t, sess), func(n int, root *os.Root) error {
			if n == 1 {
				return syncRoot(root)
			}
			return errors.New("injected root sync failure")
		})
		res, err := sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: path}})
		check(t, res, err, filepath.Join(moved, filepath.Base(exe)))
		// The rollback's own sync failed, and says so
		// (docs/decisions/0038-PLAN-pinned-plans-lifecycle-conformance-and-bubble-tests.md
		// Deviation D3).
		if !errors.Is(err, errRestoredUnsynced) {
			t.Fatalf("err = %v, want the rollback's failed sync reported", err)
		}
	})
}

// TestInstallRefusesTargetReplacedAfterBegin: the target is checked again at
// the replace. One replaced after Begin, by another file or by a symlink, is
// ErrConcurrentUpdate, and neither it nor what a symlink names is touched; a
// symlink's mode never reaches the new binary
// (docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md B2).
func TestInstallRefusesTargetReplacedAfterBegin(t *testing.T) {
	t.Parallel()
	check := func(t *testing.T, sess InstallSession, path string) {
		t.Helper()
		res, err := sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: path}})
		if !errors.Is(err, ErrConcurrentUpdate) || res.Applied {
			t.Fatalf("applied=%t err=%v; want ErrConcurrentUpdate", res.Applied, err)
		}
	}
	t.Run("file", func(t *testing.T) {
		sess, exe := standaloneSession(t)
		path := stageNew(t, sess)
		other := exe + ".other"
		if err := os.WriteFile(other, []byte("swapped-in"), 0o755); err != nil { //nolint:gosec // an executable fixture
			t.Fatal(err)
		}
		if err := os.Rename(other, exe); err != nil {
			t.Fatal(err)
		}
		check(t, sess, path)
		if got := readString(t, exe); got != "swapped-in" {
			t.Fatalf("the swapped-in file holds %q", got)
		}
	})
	t.Run("symlink", func(t *testing.T) {
		sess, exe := standaloneSession(t)
		path := stageNew(t, sess)
		other := filepath.Join(filepath.Dir(exe), "elsewhere")
		if err := os.WriteFile(other, []byte("elsewhere"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(other, 0o777); err != nil { //nolint:gosec // the symlink case needs a world-writable file
			t.Fatal(err)
		}
		if err := os.Remove(exe); err != nil {
			t.Fatal(err)
		}
		symlinkOrSkip(t, other, exe)
		check(t, sess, path)
		info, err := os.Lstat(exe)
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("the target is no longer the symlink: %v", err)
		}
		if got := readString(t, other); got != "elsewhere" {
			t.Fatalf("the symlink's file holds %q", got)
		}
	})
}
