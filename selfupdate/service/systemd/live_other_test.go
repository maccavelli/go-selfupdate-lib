//go:build !linux

package systemd

// liveMode exists only on Linux, where the live test runs.
func liveMode(string) int { return 2 }
