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
	name := ".demo.selfupdate-bak-busy"
	target, root := receiptEnv(t, name, cleanupReceipt{Version: 1, Backup: name, Digest: oldBytesDigest(t)}, true)
	release := holdBusy(t, filepath.Join(target.Dir, name))
	if err := processCleanupReceipt(target, root); err != nil {
		t.Fatalf("a busy pending backup failed the session: %v", err)
	}
	if got := receiptBackups(t, target); len(got) != 1 || got[0] != name {
		t.Fatalf("receipt lists %v, want the busy backup kept", got)
	}
	release()
	if err := processCleanupReceipt(target, root); err != nil {
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
	if err := processCleanupReceipt(target, root); err != nil {
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
	first := ".demo.selfupdate-bak-first"
	target, _ := receiptEnv(t, first, cleanupReceipt{Version: 1, Backup: first, Digest: oldBytesDigest(t)}, true)
	second := filepath.Join(target.Dir, ".demo.selfupdate-bak-second")
	if err := os.WriteFile(second, []byte("old-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	holdBusy(t, second)
	pending, err := commitReplacement(target, applyResult{backup: second, oldDigest: oldBytesDigest(t)})
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
	_, exe := withTempHome(t)
	inst, err := NewStandaloneInstaller(InstallOptions{
		TargetPolicy: TargetPolicy{ExecutablePath: exe}, KeepPrevious: true, LockTimeout: 300 * time.Millisecond,
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
	home, _ := withTempHome(t)
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
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	stopped := false
	stop := func() {
		if !stopped {
			stopped = true
			_ = os.WriteFile(done, []byte("x"), 0o600)
			_ = cmd.Wait()
		}
	}
	defer stop()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("helper did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}

	inst, err := NewStandaloneInstaller(InstallOptions{
		TargetPolicy: TargetPolicy{ExecutablePath: exe}, KeepPrevious: true, LockTimeout: 300 * time.Millisecond,
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
