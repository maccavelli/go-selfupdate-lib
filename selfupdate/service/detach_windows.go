//go:build windows

package service

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"

	"github.com/maccavelli/go-selfupdate-lib/internal/winproc"
)

// startDetached starts the program in a new process group, with a console
// of its own and no window, and out of this process's job. A job that does
// not allow breakaway refuses that with ERROR_ACCESS_DENIED; the start is
// then retried inside the job (process-creation flags, 0011-MADR §9, as
// amended by 0021-MADR D7).
func startDetached(path string, args, env []string) (int, error) {
	pid, err := startWith(path, args, env, true)
	if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		pid, err = startWith(path, args, env, false)
	}
	return pid, err
}

func startWith(path string, args, env []string, breakaway bool) (int, error) {
	cmd, err := winproc.Detached(path, args, breakaway)
	if err != nil {
		return 0, err
	}
	cmd.Env = env
	cmd.Dir = os.TempDir()
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	return pid, cmd.Process.Release()
}
