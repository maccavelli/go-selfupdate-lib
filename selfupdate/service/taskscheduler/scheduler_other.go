//go:build !windows

package taskscheduler

// systemScheduler has no Task Scheduler transport off Windows; New refuses
// before using it.
func systemScheduler() scheduler { return nil }
