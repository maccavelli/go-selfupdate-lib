//go:build windows

package service

import (
	"context"
	"time"

	"golang.org/x/sys/windows"

	"github.com/maccavelli/go-selfupdate-lib/internal/winproc"
)

// hopWaitSlice is the longest single wait on the hop's handle, so a context
// that ends is seen within it.
const hopWaitSlice = 50 * time.Millisecond

// waitHopExit waits, up to d, for process pid to exit. A process created
// after this one holds a reused ID, not the hop, and is not waited for. It
// waits in slices of hopWaitSlice and returns ctx's error when ctx ends
// first (docs/decisions/0025-MADR-align-with-go-sdk-design-practices.md D3).
func waitHopExit(ctx context.Context, pid int, d time.Duration) error {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid)) //nolint:gosec // a process ID fits in a DWORD
	if err != nil {
		return nil // gone, or not ours to wait for
	}
	defer func() { _ = windows.CloseHandle(h) }() //nolint:errcheck // a query handle: nothing to report
	if created(h) > created(windows.CurrentProcess()) {
		return nil
	}
	for deadline := time.Now().Add(d); ; {
		if err := ctx.Err(); err != nil {
			return err
		}
		left := time.Until(deadline)
		if left <= 0 {
			return nil // a timeout lets the run go on
		}
		ev, err := windows.WaitForSingleObject(h, uint32(min(left, hopWaitSlice).Milliseconds())) //nolint:gosec // at most hopWaitSlice
		if err != nil || ev != uint32(windows.WAIT_TIMEOUT) {
			return nil // the hop exited, or the handle cannot be waited on
		}
	}
}

// created is a process's creation time, or 0 when it cannot be read.
func created(h windows.Handle) uint64 {
	var c, e, k, u windows.Filetime
	if err := windows.GetProcessTimes(h, &c, &e, &k, &u); err != nil {
		return 0
	}
	return uint64(c.HighDateTime)<<32 | uint64(c.LowDateTime)
}

// processRunning reports whether process pid exists and has not exited.
func processRunning(pid int) bool {
	return winproc.Alive(uint32(pid), 0) //nolint:gosec // a process ID fits in a DWORD
}
