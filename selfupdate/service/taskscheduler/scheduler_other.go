package taskscheduler

// systemScheduler has no Task Scheduler transport yet; New refuses before
// using it. The COM transport replaces this on Windows
// (0021-PLAN-task-scheduler-lifecycle-and-windowless-children.md T5).
func systemScheduler() scheduler { return nil }
