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
	_, exe := withTempHome(t)
	inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}})
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

// setSeam replaces a package-level seam for one test and restores it at
// cleanup. Every test that swaps a seam goes through it. Seams are package
// state, so no test in this package calls t.Parallel: two tests swapping
// the same seam at once would see each other's values.
func setSeam[T any](t *testing.T, seam *T, v T) {
	t.Helper()
	prev := *seam
	*seam = v
	t.Cleanup(func() { *seam = prev })
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
	sess, exe := standaloneSession(t)
	path := stageNew(t, sess)
	syncErr := errors.New("injected dir sync failure")
	restoreErr := errors.New("injected restore failure")
	setSeam(t, &syncDirFn, func(string) error { return syncErr })
	real := replacePath
	calls := 0
	setSeam(t, &replacePath, func(ctx context.Context, oldpath, newpath string) error {
		calls++
		if calls == 1 {
			return real(ctx, oldpath, newpath)
		}
		return restoreErr
	})
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
	life := &fakeLife{installed: true, running: true}
	_, _, sess, exe := managedEnv(t, life, &fakeRec{})
	path := stageNew(t, sess)
	syncCalls := 0
	realSync := syncDirFn
	setSeam(t, &syncDirFn, func(dir string) error {
		syncCalls++
		if syncCalls == 1 {
			return errors.New("injected dir sync failure")
		}
		return realSync(dir)
	})
	real := replacePath
	calls := 0
	setSeam(t, &replacePath, func(ctx context.Context, oldpath, newpath string) error {
		calls++
		if calls == 2 {
			return errors.New("injected restore failure")
		}
		return real(ctx, oldpath, newpath)
	})
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
	if runtime.GOOS == goosWindows {
		t.Skip("POSIX permission bits: Windows keeps only a read-only flag")
	}
	setSeam(t, &osLink, func(string, string) error { return errors.New("injected link failure") })
	life := &fakeLife{installed: true, running: true, healthErr: errors.New("unhealthy")}
	_, _, sess, exe := managedEnv(t, life, &fakeRec{})
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
	sess, exe := standaloneSession(t)
	path := stageNew(t, sess)
	setSeam(t, &replacePath, func(context.Context, string, string) error { return errors.New("injected rename failure") })
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
	home, exe := withTempHome(t)
	victim := filepath.Join(home, "victim")
	if err := os.WriteFile(victim, []byte("v"), 0o600); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, "victim", filepath.Join(home, lockBasename("demo")))
	inst, _ := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}})
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
	home, exe := withTempHome(t)
	symlinkOrSkip(t, "created-by-lock", filepath.Join(home, lockBasename("demo")))
	inst, _ := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}})
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
	failing := errors.New("injected")
	cases := []struct {
		name       string
		inject     func(t *testing.T)
		wantTarget string
		wantApply  bool
	}{
		{"chmod", func(t *testing.T) {
			setSeam(t, &osChmod, func(string, os.FileMode) error { return failing })
		}, "old-bytes", false},
		{"rename", func(t *testing.T) {
			setSeam(t, &replacePath, func(context.Context, string, string) error { return failing })
		}, "old-bytes", false},
		{"dir sync, restored", func(t *testing.T) {
			setSeam(t, &syncDirFn, func(string) error { return failing })
		}, "old-bytes", false},
		{"commit remove backup", func(t *testing.T) {
			setSeam(t, &osRemove, func(string) error { return failing })
		}, "new-bytes", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sess, exe := standaloneSession(t)
			path := stageNew(t, sess)
			tc.inject(t)
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
		})
	}
}

// TestInstallPermissionDenied: an unwritable directory fails at staging and
// leaves the target intact.
func TestInstallPermissionDenied(t *testing.T) {
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
	for name, body := range map[string][]byte{"short": []byte("hello"), "long": []byte("hello-bin-and-more")} {
		t.Run(name, func(t *testing.T) {
			rel, bodies, plats := fixtureRelease(t, "demo")
			bodies[2] = body
			_, exe := withTempHome(t)
			inst, _ := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}})
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
		real := syncDirFn
		calls := 0
		setSeam(t, &syncDirFn, func(dir string) error {
			calls++
			if calls == 1 {
				return errors.New("injected dir sync failure")
			}
			return real(dir)
		})
		res, err := sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: path}})
		check(t, res, err, exe)
	})
	t.Run("rolled back in the locked directory, unsynced", func(t *testing.T) {
		sess, exe := standaloneSession(t)
		path := stageNew(t, sess)
		moved := swapAfterFirstReplace(t, filepath.Dir(exe), false)
		setSeam(t, &syncRootFn, func(*os.Root) error { return errors.New("injected root sync failure") })
		res, err := sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: path}})
		check(t, res, err, filepath.Join(moved, filepath.Base(exe)))
	})
}
