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

// Tests for defences 0003 added that no earlier test reached
// (docs/decisions/0004-MADR-evolve-selfupdate-api-and-tui-support.md,
// "Harness gaps").

const lockName = ".demo.selfupdate.lock"

func lockRoot(t *testing.T) (*os.Root, string) {
	t.Helper()
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return root, dir
}

func wantErrContaining(t *testing.T, err error, text string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), text) {
		t.Fatalf("err = %v, want one containing %q", err, text)
	}
}

// TestOpenLockFileRetriesAfterCreateRace: a lock created between the Lstat
// and the exclusive create is examined on the second attempt, not refused.
func TestOpenLockFileRetriesAfterCreateRace(t *testing.T) {
	root, dir := lockRoot(t)
	path := filepath.Join(dir, lockName)
	created := false
	setSeam(t, &lockOpenHook, func(stage string) {
		if stage == "after-lstat" && !created {
			created = true
			if err := os.WriteFile(path, nil, 0o600); err != nil {
				t.Error(err)
			}
		}
	})
	f, err := openLockFile(root, lockName)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	defer f.Close()
	got, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(got, want) {
		t.Fatal("opened a file other than the lock created in the race")
	}
}

// TestOpenLockFileRefusesReplacedLock: a lock replaced between the Lstat and
// the open is refused (0003-MADR B5).
func TestOpenLockFileRefusesReplacedLock(t *testing.T) {
	root, dir := lockRoot(t)
	path := filepath.Join(dir, lockName)
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	replaced := false
	setSeam(t, &lockOpenHook, func(stage string) {
		if stage == "after-lstat" && !replaced {
			replaced = true
			other := filepath.Join(dir, "other")
			if err := os.WriteFile(other, []byte("x"), 0o600); err != nil {
				t.Error(err)
			}
			if err := os.Rename(other, path); err != nil {
				t.Error(err)
			}
		}
	})
	f, err := openLockFile(root, lockName)
	if err == nil {
		_ = f.Close()
	}
	wantErrContaining(t, err, "lock changed while opening")
}

// TestOpenLockFileGivesUpWhenLockKeepsChanging: a lock that appears after
// every Lstat is refused after the second attempt, not looped on.
func TestOpenLockFileGivesUpWhenLockKeepsChanging(t *testing.T) {
	root, dir := lockRoot(t)
	path := filepath.Join(dir, lockName)
	setSeam(t, &lockOpenHook, func(stage string) {
		switch stage {
		case "before-lstat":
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				t.Error(err)
			}
		case "after-lstat":
			if err := os.WriteFile(path, nil, 0o600); err != nil {
				t.Error(err)
			}
		}
	})
	f, err := openLockFile(root, lockName)
	if err == nil {
		_ = f.Close()
	}
	wantErrContaining(t, err, "lock kept changing while opening")
}

// TestOpenLockFileRefusesNonRegularLock: a directory in the lock's place is
// refused.
func TestOpenLockFileRefusesNonRegularLock(t *testing.T) {
	root, dir := lockRoot(t)
	if err := os.Mkdir(filepath.Join(dir, lockName), 0o700); err != nil {
		t.Fatal(err)
	}
	f, err := openLockFile(root, lockName)
	if err == nil {
		_ = f.Close()
	}
	wantErrContaining(t, err, "lock is not a regular file")
}

// TestManagedCommitRefusesMovedDirectory: the managed commit re-checks the
// locked directory before it removes the backup (0003-MADR B10).
func TestManagedCommitRefusesMovedDirectory(t *testing.T) {
	life := &fakeLife{installed: true, running: true}
	_, _, sess, exe := managedEnv(t, life, &fakeRec{})
	path := stageNew(t, sess)
	dir := filepath.Dir(exe)
	moved := dir + ".moved"
	var swapErr error
	swapped := false
	life.onHealth = func() {
		// Only the first health check swaps: the recovery checks health
		// again (0010-MADR B7).
		if swapped {
			return
		}
		swapped = true
		if swapErr = os.Rename(dir, moved); swapErr == nil {
			if err := os.Mkdir(dir, 0o755); err != nil {
				t.Error(err)
			}
		}
	}
	t.Cleanup(func() {
		if swapErr == nil {
			_ = os.RemoveAll(dir)
			_ = os.Rename(moved, dir)
		}
	})
	res, err := sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: path}})
	if swapErr != nil {
		if runtime.GOOS != "windows" {
			t.Fatalf("directory swap failed: %v", swapErr)
		}
		if err != nil || !res.Applied {
			t.Fatalf("no swap happened, yet Applied=%v err=%v", res.Applied, err)
		}
		t.Logf("the OS refused the swap: %v", swapErr)
		return
	}
	// The refused commit is recovered in the locked directory, not left
	// live (0010-MADR B7; it used to be Applied with the error).
	if !errors.Is(err, ErrConcurrentUpdate) || !errors.Is(err, ErrManagedInstall) || res.Applied || !res.RolledBack {
		t.Fatalf("Applied=%v RolledBack=%v err=%v; want the refused commit rolled back", res.Applied, res.RolledBack, err)
	}
	if got := readString(t, filepath.Join(moved, filepath.Base(exe))); got != "old-bytes" {
		t.Fatalf("locked directory's target = %q, want the old binary back", got)
	}
}

// TestCommitReportsBackupRemovalFailure: a backup that cannot be removed
// after a healthy replacement is an error on an applied result.
func TestCommitReportsBackupRemovalFailure(t *testing.T) {
	sess, _ := standaloneSession(t)
	path := stageNew(t, sess)
	setSeam(t, &osRemove, func(string) error { return errors.New("injected remove failure") })
	res, err := sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: path}})
	if !res.Applied {
		t.Fatalf("Applied=false err=%v; the replacement itself succeeded", err)
	}
	wantErrContaining(t, err, "remove backup")
}

// TestCommitReportsDirectorySyncFailure: the directory sync after the backup
// is removed is not ignored.
func TestCommitReportsDirectorySyncFailure(t *testing.T) {
	sess, _ := standaloneSession(t)
	path := stageNew(t, sess)
	syncs := 0
	setSeam(t, &syncDirFn, func(string) error {
		syncs++
		if syncs == 1 {
			return nil // the sync inside the replacement
		}
		return errors.New("injected commit sync failure")
	})
	res, err := sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: path}})
	if !res.Applied {
		t.Fatalf("Applied=false err=%v", err)
	}
	wantErrContaining(t, err, "injected commit sync failure")
}

// TestRollbackReplacementFailures: a rollback with no backup, a failed
// restore, and a failed sync after the restore are each reported.
func TestRollbackReplacementFailures(t *testing.T) {
	sess, exe := standaloneSession(t)
	target := sess.Target()
	backup := filepath.Join(filepath.Dir(exe), backupPrefix("demo")+"x")
	if err := os.WriteFile(backup, []byte("old-bytes"), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	wantErrContaining(t, rollbackReplacement(ctx, target, applyResult{}), "no backup to restore")

	t.Run("restore", func(t *testing.T) {
		setSeam(t, &replacePath, func(context.Context, string, string) error { return errors.New("injected restore failure") })
		wantErrContaining(t, rollbackReplacement(ctx, target, applyResult{backup: backup}), "restore backup")
	})
	t.Run("sync", func(t *testing.T) {
		setSeam(t, &syncDirFn, func(string) error { return errors.New("injected rollback sync failure") })
		wantErrContaining(t, rollbackReplacement(ctx, target, applyResult{backup: backup}), "injected rollback sync failure")
		if got := readString(t, exe); got != "old-bytes" {
			t.Fatalf("target holds %q; the restore ran before the sync failed", got)
		}
	})
}

// TestCopyFileFailuresRemoveTheCopy: a backup copy that fails at chmod, sync
// or close is reported and does not leave a partial file (0003-MADR B3).
func TestCopyFileFailuresRemoveTheCopy(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	if err := os.WriteFile(src, []byte("bytes"), 0o755); err != nil {
		t.Fatal(err)
	}
	injected := errors.New("injected")
	cases := map[string]func(t *testing.T){
		"chmod": func(t *testing.T) {
			setSeam(t, &fileChmod, func(*os.File, os.FileMode) error { return injected })
		},
		"sync": func(t *testing.T) {
			setSeam(t, &fileSync, func(*os.File) error { return injected })
		},
		"close": func(t *testing.T) {
			setSeam(t, &fileClose, func(f *os.File) error { return errors.Join(f.Close(), injected) })
		},
	}
	for name, inject := range cases {
		t.Run(name, func(t *testing.T) {
			inject(t)
			dst := filepath.Join(dir, "dst-"+name)
			if err := copyFile(src, dst); !errors.Is(err, injected) {
				t.Fatalf("err = %v, want the injected %s failure", err, name)
			}
			if _, err := os.Lstat(dst); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("a failed copy left %s behind: %v", dst, err)
			}
		})
	}
}

// TestReadTruncated: bodies are cut at the limit, never refused, and a
// non-positive limit is an error.
func TestReadTruncated(t *testing.T) {
	got, err := readTruncated(strings.NewReader("0123456789"), 4)
	if err != nil || string(got) != "0123" {
		t.Fatalf("over the limit: %q, %v", got, err)
	}
	got, err = readTruncated(strings.NewReader("01"), 4)
	if err != nil || string(got) != "01" {
		t.Fatalf("under the limit: %q, %v", got, err)
	}
	if _, err := readTruncated(strings.NewReader("x"), 0); err == nil {
		t.Fatal("a zero limit was accepted")
	}
}
