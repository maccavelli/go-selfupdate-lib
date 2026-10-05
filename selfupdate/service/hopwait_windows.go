//go:build windows

package service

import (
	"errors"
	"time"

	"golang.org/x/sys/windows"
)

// waitHopExit waits, up to d, for process pid to exit. A process created
// after this one holds a reused ID, not the hop, and is not waited for.
func waitHopExit(pid int, d time.Duration) {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid)) //nolint:gosec // a process ID fits in a DWORD
	if err != nil {
		return // gone, or not ours to wait for
	}
	defer func() { _ = windows.CloseHandle(h) }() //nolint:errcheck // a query handle: nothing to report
	if created(h) > created(windows.CurrentProcess()) {
		return
	}
	_, _ = windows.WaitForSingleObject(h, uint32(d.Milliseconds())) //nolint:gosec,errcheck // bounded by hopWait; a timeout lets the run go on
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
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid)) //nolint:gosec // a process ID fits in a DWORD
	if err != nil {
		return false
	}
	ev, err := windows.WaitForSingleObject(h, 0)
	return errors.Join(err, windows.CloseHandle(h)) == nil && ev == uint32(windows.WAIT_TIMEOUT)
}
