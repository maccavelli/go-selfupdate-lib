//go:build !windows

package taskscheduler

// liveMode exists only on Windows, where the live test runs.
func liveMode(string, []string) int { return 2 }
