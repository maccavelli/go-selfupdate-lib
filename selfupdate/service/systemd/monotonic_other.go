//go:build !linux

package systemd

// monotonicMicros is 0 off Linux, where no systemd reads it.
func monotonicMicros() int64 { return 0 }
