//go:build linux

package systemd

import (
	"syscall"
	"unsafe"
)

// clockMonotonic is CLOCK_MONOTONIC's ID on Linux.
const clockMonotonic = 1

// monotonicMicros is CLOCK_MONOTONIC in microseconds, the clock systemd
// compares MONOTONIC_USEC with. x/sys/unix is not among this package's
// imports (0011-MADR §1), so it calls clock_gettime directly.
func monotonicMicros() int64 {
	var ts syscall.Timespec
	_, _, errno := syscall.Syscall(syscall.SYS_CLOCK_GETTIME, clockMonotonic, uintptr(unsafe.Pointer(&ts)), 0) //nolint:gosec // a Timespec for clock_gettime
	if errno != 0 {
		return 0
	}
	return int64(ts.Sec)*1_000_000 + int64(ts.Nsec)/1_000 //nolint:unconvert // Sec and Nsec are 32-bit on some architectures
}
