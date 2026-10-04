//go:build windows

package service

import (
	"errors"
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// detachFlags start a process with no console and in a new process group.
const detachFlags = windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP

// startDetached starts the program with no console, in a new process group,
// and out of this process's job. A job that does not allow breakaway
// refuses that with ERROR_ACCESS_DENIED; the start is then retried inside
// the job (process-creation flags, 0011-MADR §9).
func startDetached(path string, args, env []string) (int, error) {
	pid, err := startWith(path, args, env, detachFlags|windows.CREATE_BREAKAWAY_FROM_JOB)
	if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		pid, err = startWith(path, args, env, detachFlags)
	}
	return pid, err
}

func startWith(path string, args, env []string, flags uint32) (int, error) {
	cmd := exec.Command(path, args...) //nolint:gosec // HandOffIfInside resolved path to this program
	cmd.Env = env
	cmd.Dir = os.TempDir()
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: flags}
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	return pid, cmd.Process.Release()
}
