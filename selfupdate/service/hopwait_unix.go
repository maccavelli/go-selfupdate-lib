//go:build unix

package service

import (
	"errors"
	"os"
	"syscall"
	"time"
)

// waitHopExit waits, up to d, until this process's parent is no longer
// pid: Unix gives an orphan to a new parent as its parent exits, so a
// reused ID is never mistaken for the hop.
func waitHopExit(pid int, d time.Duration) {
	for deadline := time.Now().Add(d); os.Getppid() == pid && time.Now().Before(deadline); {
		time.Sleep(5 * time.Millisecond)
	}
}

// processRunning reports whether a process with ID pid exists.
func processRunning(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
