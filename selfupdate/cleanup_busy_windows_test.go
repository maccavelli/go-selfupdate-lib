//go:build windows

package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"

	"github.com/maccavelli/go-selfupdate-lib/internal/testproc"
)

// Regression tests for docs/decisions/0010-MADR-remediate-second-debugging-pass-findings.md
// B5 and B6, and amendment A1 (docs/decisions/0010-PLAN-v1-5-1-contract-preserving-fixes.md P5).

// holdBusy opens path without FILE_SHARE_DELETE, as a running image holds
// its file: until release, Windows refuses to delete or replace it.
func holdBusy(t *testing.T, path string) (release func()) {
	t.Helper()
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatalf("hold %s: %v", path, err)
	}
	done := false
	release = func() {
		if !done {
			done = true
			_ = windows.CloseHandle(h)
		}
	}
	t.Cleanup(release)
	return release
}

// receiptBackups reads the receipt's backup names, whatever its version.
func receiptBackups(t *testing.T, target Target) []string {
	t.Helper()
	data, err := os.ReadFile(cleanupReceiptPath(target))
	if err != nil {
		t.Fatalf("read receipt: %v", err)
	}
	var raw struct {
		Backup  string `json:"backup"`
		Backups []struct {
			Backup string `json:"backup"`
		} `json:"backups"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	var names []string
	if raw.Backup != "" {
		names = append(names, raw.Backup)
	}
	for _, b := range raw.Backups {
		names = append(names, b.Backup)
	}
	return names
}

// TestWindowsBusyPendingBackupKept: a pending backup that a running image
// still holds keeps its receipt, and does not fail the session; once
// released, the next session removes both (B5).
func TestWindowsBusyPendingBackupKept(t *testing.T) {
	t.Parallel()
	name := ".demo.selfupdate-bak-busy"
	target, root := receiptEnv(t, name, cleanupReceipt{Version: 1, Backup: name, Digest: oldBytesDigest(t)}, true)
	release := holdBusy(t, filepath.Join(target.Dir, name))
	if err := realFS().processCleanupReceipt(context.Background(), target, root); err != nil {
		t.Fatalf("a busy pending backup failed the session: %v", err)
	}
	if got := receiptBackups(t, target); len(got) != 1 || got[0] != name {
		t.Fatalf("receipt lists %v, want the busy backup kept", got)
	}
	release()
	if err := realFS().processCleanupReceipt(context.Background(), target, root); err != nil {
		t.Fatalf("after release: %v", err)
	}
	for _, p := range []string{cleanupReceiptPath(target), filepath.Join(target.Dir, name)} {
		if _, err := os.Lstat(p); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s left after release: %v", filepath.Base(p), err)
		}
	}
}

// TestWindowsReceiptKeepsOnlyBusy: of two pending backups, the free one is
// removed and the busy one stays listed (amendment A1).
func TestWindowsReceiptKeepsOnlyBusy(t *testing.T) {
	t.Parallel()
	busy, free := ".demo.selfupdate-bak-busy", ".demo.selfupdate-bak-free"
	target, root := receiptEnv(t, busy, cleanupReceipt{Version: 1, Backup: busy, Digest: oldBytesDigest(t)}, true)
	if err := os.WriteFile(filepath.Join(target.Dir, free), []byte("old-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	two := `{"version":2,"backups":[{"backup":"` + busy + `","digest":"` + oldBytesDigest(t) + `"},` +
		`{"backup":"` + free + `","digest":"` + oldBytesDigest(t) + `"}]}`
	if err := os.WriteFile(cleanupReceiptPath(target), []byte(two), 0o600); err != nil {
		t.Fatal(err)
	}
	holdBusy(t, filepath.Join(target.Dir, busy))
	if err := realFS().processCleanupReceipt(context.Background(), target, root); err != nil {
		t.Fatalf("process: %v", err)
	}
	if got := receiptBackups(t, target); len(got) != 1 || got[0] != busy {
		t.Fatalf("receipt lists %v, want only the busy backup", got)
	}
	if _, err := os.Lstat(filepath.Join(target.Dir, free)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the free backup was not removed: %v", err)
	}
}

// TestWindowsCommitAddsToPendingReceipt: a commit whose backup is busy,
// while an earlier receipt is still pending, adds its backup to the list
// instead of failing on the existing receipt (amendment A1).
func TestWindowsCommitAddsToPendingReceipt(t *testing.T) {
	t.Parallel()
	first := ".demo.selfupdate-bak-first"
	target, _ := receiptEnv(t, first, cleanupReceipt{Version: 1, Backup: first, Digest: oldBytesDigest(t)}, true)
	second := filepath.Join(target.Dir, ".demo.selfupdate-bak-second")
	if err := os.WriteFile(second, []byte("old-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	holdBusy(t, second)
	pending, err := realFS().commitReplacement(context.Background(), target, applyResult{backup: second, oldDigest: oldBytesDigest(t)})
	if err != nil {
		t.Fatalf("commit with a pending receipt: %v", err)
	}
	if pending != second {
		t.Fatalf("pending = %q, want %q", pending, second)
	}
	if got := receiptBackups(t, target); len(got) != 2 || got[0] != first || got[1] != filepath.Base(second) {
		t.Fatalf("receipt lists %v, want both backups", got)
	}
}

// TestWindowsKeepPreviousBusy: with KeepPrevious, a .previous that a
// running image holds puts the new backup on the receipt, and the commit
// stands (B6).
func TestWindowsKeepPreviousBusy(t *testing.T) {
	t.Parallel()
	_, exe := tempTarget(t)
	inst, err := NewStandaloneInstaller(InstallOptions{
		TargetPolicy: targetPolicy(exe), KeepPrevious: true, LockTimeout: 300 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	target, err := inst.ResolveTarget(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	previous := previousPath(target)
	if err := os.WriteFile(previous, []byte("older-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	holdBusy(t, previous)
	sess, err := inst.Begin(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	path := stageNew(t, sess)
	res, err := sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: path}})
	if err != nil {
		t.Fatalf("Install with a busy .previous: %v", err)
	}
	if !res.Applied || res.PendingBackup == "" || res.Previous != "" {
		t.Fatalf("res = %+v, want applied, the backup pending, no previous", res)
	}
	if got := receiptBackups(t, target); len(got) != 1 || got[0] != filepath.Base(res.PendingBackup) {
		t.Fatalf("receipt lists %v, want the new backup", got)
	}
	if got := readString(t, exe); got != "new-bytes" {
		t.Fatalf("target = %q", got)
	}
}

// TestWindowsKeepPreviousRunningPrevious: a program runs; one update makes
// its image the .previous; a second update, while it still runs, cannot
// replace that .previous. The second backup goes on the receipt, the
// commit stands, and once the program exits CleanupPending removes the
// backup and the receipt and leaves the .previous (B6, with a running
// image rather than a held handle).
func TestWindowsKeepPreviousRunningPrevious(t *testing.T) {
	t.Parallel()
	home, _ := tempTarget(t)
	src, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	in, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(home, "helper.exe")
	if err := os.WriteFile(exe, in, 0o755); err != nil {
		t.Fatal(err)
	}
	ready, done := filepath.Join(home, "ready"), filepath.Join(home, "done")
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(), "SELFUPDATE_NATIVE_HELPER=1", "SELFUPDATE_READY="+ready, "SELFUPDATE_DONE="+done)
	proc := testproc.Start(t, cmd, ready)
	stopped := false
	stop := func() {
		if !stopped {
			stopped = true
			_ = os.WriteFile(done, []byte("x"), 0o600)
			_ = proc.Wait()
		}
	}
	defer stop()

	inst, err := NewStandaloneInstaller(InstallOptions{
		TargetPolicy: targetPolicy(exe), KeepPrevious: true, LockTimeout: 300 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	var target Target
	install := func(suffix string) InstallResult {
		t.Helper()
		// Each update resolves the target afresh, as a caller does: a
		// Target records the file it named when it was resolved.
		if target, err = inst.ResolveTarget(context.Background()); err != nil {
			t.Fatal(err)
		}
		sess, err := inst.Begin(context.Background(), target)
		if err != nil {
			t.Fatalf("Begin before %s: %v", suffix, err)
		}
		defer func() { _ = sess.Close() }()
		f, path, err := sess.CreateStaging(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		replacement := append(append([]byte{}, in...), suffix...)
		if _, err := f.Write(replacement); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		res, err := sess.Install(context.Background(), InstallRequest{Product: "helper", Artifact: StagedArtifact{Path: path, Size: int64(len(replacement))}})
		if err != nil {
			t.Fatalf("install %s: res = %+v, err = %v", suffix, res, err)
		}
		return res
	}

	first := install("FIRST")
	if !first.Applied || first.Previous != previousPath(target) || first.PendingBackup != "" {
		t.Fatalf("first = %+v, want the running image kept as previous", first)
	}
	second := install("SECOND")
	if !second.Applied || second.Previous != "" || second.PendingBackup == "" {
		t.Fatalf("second = %+v, want applied, the backup pending, no previous", second)
	}
	if got := receiptBackups(t, target); len(got) != 1 || got[0] != filepath.Base(second.PendingBackup) {
		t.Fatalf("receipt lists %v, want the second backup", got)
	}
	if got := readString(t, exe); !strings.HasSuffix(got, "SECOND") {
		t.Fatal("the target does not hold the second replacement")
	}

	stop()
	if err := inst.CleanupPending(context.Background()); err != nil {
		t.Fatalf("CleanupPending after the program exited: %v", err)
	}
	for _, p := range []string{cleanupReceiptPath(target), second.PendingBackup} {
		if _, err := os.Lstat(p); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s left after cleanup: %v", filepath.Base(p), err)
		}
	}
	if got := readString(t, previousPath(target)); got != string(in) {
		t.Fatal("the previous no longer holds the first program")
	}
}

// Regression tests for docs/decisions/0025-MADR-align-with-go-sdk-design-practices.md
// Amendment A2: the receipt's replace takes the caller's retry budget, and
// its cancellation only before anything has changed.

// TestWindowsReceiptRewriteHonoursContext: at session start a rewrite of
// the receipt that cannot replace the old one stops when the context ends,
// instead of retrying for DefaultLockTimeout.
func TestWindowsReceiptRewriteHonoursContext(t *testing.T) {
	t.Parallel()
	busy, free := ".demo.selfupdate-bak-busy", ".demo.selfupdate-bak-free"
	target, root := receiptEnv(t, busy, cleanupReceipt{Version: 1, Backup: busy, Digest: oldBytesDigest(t)}, true)
	if err := os.WriteFile(filepath.Join(target.Dir, free), []byte("old-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	two := `{"version":2,"backups":[{"backup":"` + busy + `","digest":"` + oldBytesDigest(t) + `"},` +
		`{"backup":"` + free + `","digest":"` + oldBytesDigest(t) + `"}]}`
	if err := os.WriteFile(cleanupReceiptPath(target), []byte(two), 0o600); err != nil {
		t.Fatal(err)
	}
	holdBusy(t, filepath.Join(target.Dir, busy))
	holdBusy(t, cleanupReceiptPath(target))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	err := realFS().processCleanupReceipt(ctx, target, root)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("returned after %v, want under 1s", d)
	}
}

// lockedSession begins a standalone session on a fresh target with
// lockTimeout, closed when t ends.
func lockedSession(t *testing.T, lockTimeout time.Duration) (*installSession, Target) {
	t.Helper()
	_, exe := tempTarget(t)
	inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: targetPolicy(exe), LockTimeout: lockTimeout})
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
	return sess.(*installSession), target
}

// busyBackup writes a backup beside target and holds it as a running
// image would.
func busyBackup(t *testing.T, target Target, name string) string {
	t.Helper()
	path := filepath.Join(target.Dir, name)
	if err := os.WriteFile(path, []byte("old-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	holdBusy(t, path)
	return path
}

// TestWindowsCommitReceiptNotCancelled: the commit path writes the receipt
// after the binary is replaced, so a cancelled context does not stop its
// retries: a receipt held for a moment is still replaced.
func TestWindowsCommitReceiptNotCancelled(t *testing.T) {
	t.Parallel()
	sess, target := lockedSession(t, 5*time.Second)
	first := busyBackup(t, target, ".demo.selfupdate-bak-first")
	if err := realFS().writeCleanupReceipt(context.Background(), target, applyResult{backup: first, oldDigest: oldBytesDigest(t)}); err != nil {
		t.Fatal(err)
	}
	release := holdBusy(t, cleanupReceiptPath(target))
	released := make(chan struct{})
	go func() {
		defer close(released)
		time.Sleep(200 * time.Millisecond)
		release()
	}()
	defer func() { <-released }()
	backup := busyBackup(t, target, ".demo.selfupdate-bak-commit")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	sess.mu.Lock()
	pending, _, err := sess.commitLocked(ctx, applyResult{backup: backup, oldDigest: oldBytesDigest(t)})
	sess.mu.Unlock()
	if err != nil || pending != backup {
		t.Fatalf("commit with a cancelled context: pending = %q, err = %v; want %q, nil", pending, err, backup)
	}
	if got := receiptBackups(t, target); len(got) != 2 || got[1] != filepath.Base(backup) {
		t.Fatalf("receipt lists %v, want both busy backups", got)
	}
}

// TestWindowsCommitReceiptBoundedByLockTimeout: a receipt the commit path
// cannot replace is given up after the session's LockTimeout, not
// DefaultLockTimeout.
func TestWindowsCommitReceiptBoundedByLockTimeout(t *testing.T) {
	t.Parallel()
	sess, target := lockedSession(t, 200*time.Millisecond)
	first := busyBackup(t, target, ".demo.selfupdate-bak-first")
	if err := realFS().writeCleanupReceipt(context.Background(), target, applyResult{backup: first, oldDigest: oldBytesDigest(t)}); err != nil {
		t.Fatal(err)
	}
	holdBusy(t, cleanupReceiptPath(target))
	backup := busyBackup(t, target, ".demo.selfupdate-bak-second")
	start := time.Now()
	sess.mu.Lock()
	_, _, err := sess.commitLocked(context.Background(), applyResult{backup: backup, oldDigest: oldBytesDigest(t)})
	sess.mu.Unlock()
	if err == nil {
		t.Fatal("commit replaced a held receipt")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("gave up after %v, want about the 200ms LockTimeout, not %v", d, DefaultLockTimeout)
	}
}
