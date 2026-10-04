//go:build unix

package service

import (
	"os/exec"
	"syscall"
)

// startDetached starts the program in a new session, with its standard
// streams on the null device and / as its directory, and does not wait
// for it. It exits as an orphan of init once this process has gone.
func startDetached(path string, args, env []string) (int, error) {
	cmd := exec.Command(path, args...) //nolint:gosec // HandOffIfInside resolved path to this program
	cmd.Env = env
	cmd.Dir = "/"
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	return pid, cmd.Process.Release()
}
