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

// backupsIn lists the selfupdate backups left in dir.
func backupsIn(t *testing.T, dir string) []string {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), backupPrefix("demo")) {
			out = append(out, e.Name())
		}
	}
	return out
}

// failRestore lets the first replacement through and fails every later one,
// so the restore after an injected directory-sync failure fails too.
func failRestore(t *testing.T) {
	t.Helper()
	real := replacePath
	calls := 0
	setSeam(t, &replacePath, func(ctx context.Context, oldpath, newpath string) error {
		calls++
		if calls == 1 {
			return real(ctx, oldpath, newpath)
		}
		return errors.New("injected restore failure")
	})
}

// TestRunReportsKeptBackup: when the new binary is live and restoring the
// previous one fails, Run names the backup in Result.PendingBackup and in
// the error, so the only copy of the previous binary is not lost
// (0004-MADR R1).
func TestRunReportsKeptBackup(t *testing.T) {
	_, exe := withTempHome(t)
	inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}})
	if err != nil {
		t.Fatal(err)
	}
	rel, bodies, plats := fixtureRelease(t, "demo")
	sel, err := NewExactAssetSelector(plats)
	if err != nil {
		t.Fatal(err)
	}
	setSeam(t, &syncDirFn, func(string) error { return errors.New("injected directory sync failure") })
	failRestore(t)
	u, err := New(Config{
		Source: &scriptSource{rel: rel, bodies: bodies}, Versions: NewStrictVersionPolicy(), Assets: sel,
		Installer: inst, Reporter: &recReporter{}, Confirmer: &recConfirmer{}, Limits: DefaultLimits(),
	})
	if err != nil {
		t.Fatal(err)
	}
	req := applyReq()
	req.Yes = true
	res, err := u.Run(context.Background(), req)
	if err == nil || res.Applied {
		t.Fatalf("Applied=%v err=%v; want a failed, unapplied run", res.Applied, err)
	}
	if res.PendingBackup == "" {
		t.Fatalf("PendingBackup is empty; the kept backup was not reported (err=%v)", err)
	}
	if got := readString(t, res.PendingBackup); got != "old-bytes" {
		t.Fatalf("PendingBackup %s holds %q, want the previous binary", res.PendingBackup, got)
	}
	if !strings.Contains(err.Error(), "previous binary was kept at "+sanitizeText(res.PendingBackup)) {
		t.Fatalf("error does not name the kept backup: %v", err)
	}
}

// TestManagedReportsKeptBackup: the managed installer reports the backup
// when recovery cannot restore it (0004-MADR R1).
func TestManagedReportsKeptBackup(t *testing.T) {
	life := &fakeLife{installed: true, running: true, healthErr: errors.New("unhealthy")}
	rec := &fakeRec{}
	_, _, sess, exe := managedEnv(t, life, rec)
	path := stageNew(t, sess)
	failRestore(t)
	res, err := sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: path}})
	if !errors.Is(err, ErrManagedInstall) || res.Applied {
		t.Fatalf("Applied=%v err=%v", res.Applied, err)
	}
	if res.Backup == "" {
		t.Fatalf("Backup is empty after a failed rollback (err=%v)", err)
	}
	if got := readString(t, res.Backup); got != "old-bytes" {
		t.Fatalf("Backup holds %q", got)
	}
	if got := readString(t, exe); got != "new-bytes" {
		t.Fatalf("target holds %q; the rollback was injected to fail", got)
	}
}

// TestInstallRollsBackWhenDirectoryMovesAfterRename: a directory swapped
// right after the rename is detected, and the rename is undone in the
// directory that was locked, through its handle (0004-MADR R3).
func TestInstallRollsBackWhenDirectoryMovesAfterRename(t *testing.T) {
	sess, exe := standaloneSession(t)
	path := stageNew(t, sess)
	dir := filepath.Dir(exe)
	moved := dir + ".moved"
	real := replacePath
	var swapErr error
	setSeam(t, &replacePath, func(ctx context.Context, oldpath, newpath string) error {
		if err := real(ctx, oldpath, newpath); err != nil {
			return err
		}
		// A refused swap is recorded, not returned: the replacement itself
		// succeeded.
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
	res, err := sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: path}})
	if swapErr != nil {
		// Windows refuses to rename a directory while the session holds
		// handles inside it, so the swap cannot happen there at all.
		if runtime.GOOS != "windows" {
			t.Fatalf("directory swap failed: %v", swapErr)
		}
		if err != nil || !res.Applied {
			t.Fatalf("no swap happened, yet Applied=%v err=%v", res.Applied, err)
		}
		t.Logf("the OS refused the swap: %v", swapErr)
		return
	}
	if !errors.Is(err, ErrConcurrentUpdate) || res.Applied {
		t.Fatalf("Applied=%v err=%v; want an unapplied concurrent-update failure", res.Applied, err)
	}
	if got := readString(t, filepath.Join(moved, "demo")); got != "old-bytes" {
		t.Fatalf("locked directory's target holds %q; the rename was not undone", got)
	}
	if left := backupsIn(t, moved); len(left) != 0 {
		t.Fatalf("backups left in the locked directory: %v", left)
	}
}

// TestReplaceRestoreSurvivesCancellation: the restore after a failed
// directory sync runs even when the caller's context was cancelled during
// the replacement (0004-MADR R4).
func TestReplaceRestoreSurvivesCancellation(t *testing.T) {
	sess, exe := standaloneSession(t)
	path := stageNew(t, sess)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	real := replacePath
	var restoreCtxErr error
	calls := 0
	setSeam(t, &replacePath, func(c context.Context, oldpath, newpath string) error {
		calls++
		if calls == 1 {
			err := real(c, oldpath, newpath)
			cancel()
			return err
		}
		restoreCtxErr = c.Err()
		return real(c, oldpath, newpath)
	})
	setSeam(t, &syncDirFn, func(string) error { return errors.New("injected directory sync failure") })
	_, err := sess.Install(ctx, InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: path}})
	if err == nil {
		t.Fatal("want the injected sync failure")
	}
	if calls != 2 {
		t.Fatalf("replacePath calls = %d, want the replacement and the restore", calls)
	}
	if restoreCtxErr != nil {
		t.Fatalf("the restore ran on a cancelled context: %v", restoreCtxErr)
	}
	if got := readString(t, exe); got != "old-bytes" {
		t.Fatalf("target holds %q after the restore", got)
	}
}

// TestBeginChecksDirectoryBeforeReceipt: a directory swapped after the lock
// is refused before anything in it is read or removed, cleanup receipt
// included (0004-MADR R5).
func TestBeginChecksDirectoryBeforeReceipt(t *testing.T) {
	_, exe := withTempHome(t)
	dir := filepath.Dir(exe)
	moved := dir + ".moved"
	inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}})
	if err != nil {
		t.Fatal(err)
	}
	target, err := inst.ResolveTarget(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	receipt := cleanupReceiptName("demo")
	// A receipt in the locked directory is what makes the unfixed code act:
	// it finds this one through the root, then removed by path whatever the
	// path named by then.
	if err := os.WriteFile(filepath.Join(dir, receipt), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	var swapErr error
	setSeam(t, &afterLockHook, func() {
		if swapErr = os.Rename(dir, moved); swapErr != nil {
			// No swap: the placeholder receipt is not a real one, so take it
			// away before Begin would validate it.
			if err := os.Remove(filepath.Join(dir, receipt)); err != nil {
				t.Error(err)
			}
			return
		}
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Error(err)
			return
		}
		for name, body := range map[string]string{"demo": "old-bytes", receipt: "{}"} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
				t.Error(err)
			}
		}
	})
	t.Cleanup(func() {
		if swapErr == nil {
			_ = os.RemoveAll(dir)
			_ = os.Rename(moved, dir)
		}
	})
	sess, err := inst.Begin(context.Background(), target)
	if swapErr != nil {
		if runtime.GOOS != "windows" {
			t.Fatalf("directory swap failed: %v", swapErr)
		}
		if err != nil {
			t.Fatalf("no swap happened, yet Begin failed: %v", err)
		}
		t.Logf("the OS refused the swap: %v", swapErr)
		_ = sess.Close()
		return
	}
	if err == nil {
		_ = sess.Close()
		t.Fatal("Begin accepted a swapped directory")
	}
	if !errors.Is(err, ErrConcurrentUpdate) {
		t.Fatalf("err = %v", err)
	}
	if _, serr := os.Lstat(filepath.Join(dir, receipt)); serr != nil {
		t.Fatalf("the receipt in the swapped-in directory was touched: %v", serr)
	}
}

// dryRunUpdater is an Updater over the fixture release that installs with
// inst, for a dry run beside a real target.
func dryRunUpdater(t *testing.T, inst Installer) *Updater {
	t.Helper()
	rel, bodies, plats := fixtureRelease(t, "demo")
	sel, err := NewExactAssetSelector(plats)
	if err != nil {
		t.Fatal(err)
	}
	u, err := New(Config{
		Source: &scriptSource{rel: rel, bodies: bodies}, Versions: NewStrictVersionPolicy(), Assets: sel,
		Installer: inst, Reporter: &recReporter{}, Confirmer: &recConfirmer{}, Limits: DefaultLimits(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// TestKeptBackupSurvivesLaterSessions: a backup reported as the only copy
// of the previous binary, because restoring it failed, is still there after
// any later session: the startup CleanupPending, another update, a dry run,
// or a managed update's
// (docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md B1).
func TestKeptBackupSurvivesLaterSessions(t *testing.T) {
	realSync, realReplace := syncDirFn, replacePath
	// keep fails the restore after an injected sync failure, so the backup
	// is kept and reported; then it puts the real seams back for the later
	// session.
	keep := func(t *testing.T, install func() (InstallResult, error)) string {
		t.Helper()
		setSeam(t, &syncDirFn, func(string) error { return errors.New("injected directory sync failure") })
		failRestore(t)
		res, err := install()
		if err == nil || res.Applied || res.Backup == "" {
			t.Fatalf("Applied=%v Backup=%q err=%v; want a kept backup", res.Applied, res.Backup, err)
		}
		setSeam(t, &syncDirFn, realSync)
		setSeam(t, &replacePath, realReplace)
		return res.Backup
	}
	survives := func(t *testing.T, backup string) {
		t.Helper()
		got, err := os.ReadFile(backup)
		if err != nil || string(got) != "old-bytes" {
			t.Fatalf("backup %s after a later session: %q, %v; want the previous binary", filepath.Base(backup), got, err)
		}
		if !strings.HasPrefix(filepath.Base(backup), ".demo.selfupdate-kept-") {
			t.Fatalf("backup %s is not named as kept", filepath.Base(backup))
		}
	}
	standalone := func(t *testing.T) (*StandaloneInstaller, Target, string) {
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
		backup := keep(t, func() (InstallResult, error) {
			return sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: stageNew(t, sess)}})
		})
		if err := sess.Close(); err != nil {
			t.Fatal(err)
		}
		return inst, target, backup
	}
	t.Run("CleanupPending", func(t *testing.T) {
		inst, _, backup := standalone(t)
		if err := inst.CleanupPending(context.Background()); err != nil {
			t.Fatal(err)
		}
		survives(t, backup)
	})
	t.Run("Run", func(t *testing.T) {
		inst, _, backup := standalone(t)
		// A later run resolves the target afresh: the first one replaced it.
		target, err := inst.ResolveTarget(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		sess, err := inst.Begin(context.Background(), target)
		if err != nil {
			t.Fatal(err)
		}
		if err := sess.Close(); err != nil {
			t.Fatal(err)
		}
		survives(t, backup)
	})
	t.Run("DryRun", func(t *testing.T) {
		inst, _, backup := standalone(t)
		req := applyReq()
		req.DryRun = true
		if _, err := dryRunUpdater(t, inst).Run(context.Background(), req); err != nil {
			t.Fatal(err)
		}
		survives(t, backup)
	})
	t.Run("Managed", func(t *testing.T) {
		life := &fakeLife{installed: true, running: true, healthErr: errors.New("unhealthy")}
		m, _, sess, _ := managedEnv(t, life, &fakeRec{})
		backup := keep(t, func() (InstallResult, error) {
			return sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: stageNew(t, sess)}})
		})
		if err := sess.Close(); err != nil {
			t.Fatal(err)
		}
		target, err := m.ResolveTarget(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		later, err := m.Begin(context.Background(), target)
		if err != nil {
			t.Fatal(err)
		}
		if err := later.Close(); err != nil {
			t.Fatal(err)
		}
		survives(t, backup)
	})
}
