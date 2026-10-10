//go:build unix

package service

import "github.com/maccavelli/go-selfupdate-lib/internal/winproc"

// startDetached starts the program in a new session, with its standard
// streams on the null device and / as its directory, and does not wait
// for it. It exits as an orphan of init once this process has gone.
func startDetached(path string, args, env []string) (int, error) {
	cmd, err := winproc.Detached(path, args, false)
	if err != nil {
		return 0, err
	}
	cmd.Env = env
	cmd.Dir = "/"
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	return pid, cmd.Process.Release()
}
