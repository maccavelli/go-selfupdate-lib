//go:build !darwin

package launchd

// liveMode exists only on macOS, where the live test runs.
func liveMode(string) int { return 2 }
