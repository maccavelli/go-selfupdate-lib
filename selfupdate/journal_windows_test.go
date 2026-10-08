//go:build windows

package selfupdate

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestWindowsInterruptedApplyRunningImage: the previous binary still runs
// while an update is interrupted between Apply and Commit. Its image is
// then named only by the backup, which the next session renames to its
// kept name while the image runs
// (docs/decisions/0017-PLAN-verify-build-provenance-and-close-0015-open-items.md
// Q1).
func TestWindowsInterruptedApplyRunningImage(t *testing.T) {
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
	defer func() {
		_ = os.WriteFile(done, []byte("x"), 0o600)
		_ = cmd.Wait()
	}()
	for deadline := time.Now().Add(10 * time.Second); ; {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("helper did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}

	inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}, LockTimeout: 300 * time.Millisecond})
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
	f, path, err := sess.CreateStaging(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	replacement := append(append([]byte{}, in...), "new"...)
	if _, err := f.Write(replacement); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := sess.(TwoPhaseSession).Apply(context.Background(), InstallRequest{Product: "helper", Artifact: StagedArtifact{Path: path}}); err != nil {
		t.Fatal(err)
	}
	if err := sess.Close(); err != nil {
		t.Fatal(err)
	}

	// The helper still runs: its image is the backup's.
	if err := inst.CleanupPending(context.Background()); err != nil {
		t.Fatal(err)
	}
	kept, err := inst.KeptBackups(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(kept) != 1 {
		t.Fatalf("KeptBackups = %+v, want the running image's backup", kept)
	}
	got, err := os.ReadFile(kept[0].Path)
	if err != nil || !bytes.Equal(got, in) {
		t.Fatalf("kept backup: %d bytes, %v; want the previous binary", len(got), err)
	}
	if left, _ := filepath.Glob(filepath.Join(home, ".helper.exe.selfupdate.pending*")); len(left) != 0 {
		t.Fatalf("journal files left: %q", left)
	}
}
