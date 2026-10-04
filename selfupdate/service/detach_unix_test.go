//go:build unix

package service

import (
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
)

// TestDetachProcessOutlivesProcessGroup: a child DetachProcess started
// survives when its parent's whole process group is killed, as a launchd
// job's group is reaped (0011-PLAN V1 step 8).
func TestDetachProcessOutlivesProcessGroup(t *testing.T) {
	dir := t.TempDir()
	goFile, alive := filepath.Join(dir, "go"), filepath.Join(dir, "alive")
	cmd := exec.Command(selfExe(t))
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	childPID := startParent(t, cmd, "FAKE_GO="+goFile, "FAKE_ALIVE="+alive)
	pid := childPID()
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })

	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	if !childOutlived(t, goFile, alive) {
		t.Fatal("the detached child died with its parent's process group")
	}
}
