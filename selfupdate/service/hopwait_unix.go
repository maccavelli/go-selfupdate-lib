//go:build unix

package service

import (
	"context"
	"errors"
	"os"
	"syscall"
	"time"
)

// waitHopExit waits, up to d, until this process's parent is no longer
// pid: Unix gives an orphan to a new parent as its parent exits, so a
// reused ID is never mistaken for the hop. It returns ctx's error when ctx
// ends first (docs/decisions/0025-MADR-align-with-go-sdk-design-practices.md D3).
func waitHopExit(ctx context.Context, pid int, d time.Duration) error {
	for deadline := time.Now().Add(d); os.Getppid() == pid && time.Now().Before(deadline); {
		tick := time.NewTimer(5 * time.Millisecond)
		select {
		case <-ctx.Done():
			tick.Stop()
			return ctx.Err()
		case <-tick.C:
		}
	}
	return nil
}

// processRunning reports whether a process with ID pid exists.
func processRunning(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
