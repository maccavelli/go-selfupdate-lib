package service

import (
	"os"
	"runtime"
	"slices"
	"strings"
)

// managerEnvNames are the variables a service manager sets for one unit or
// job, which describe it and not the user: its run, its channels to the
// manager, the manager, why it ran, two of its directories, and launchd's
// per-job names
// (docs/decisions/0033-MADR-scrub-service-manager-environment-from-handoffs.md
// D1-C and D2-C).
var managerEnvNames = []string{
	// systemd.exec(5), "Environment Variables in Spawned Processes": the
	// unit's run.
	"INVOCATION_ID", "SYSTEMD_EXEC_PID", "MAINPID", "MAINPIDFDID", "PIDFILE",
	"LOG_NAMESPACE", "DEBUG_INVOCATION",
	// Its channels to the manager: sd_notify(3), sd_listen_fds(3), the
	// watchdog, the file descriptor store, memory pressure and the journal.
	"NOTIFY_SOCKET", "WATCHDOG_PID", "WATCHDOG_USEC",
	"LISTEN_FDS", "LISTEN_PID", "LISTEN_PIDFDID", "LISTEN_FDNAMES",
	"FDSTORE", "MEMORY_PRESSURE_WATCH", "MEMORY_PRESSURE_WRITE", "JOURNAL_STREAM",
	// The manager.
	"MANAGERPID", "MANAGERPIDFDID",
	// Why the unit ran.
	"SERVICE_RESULT", "EXIT_CODE", "EXIT_STATUS",
	"MONITOR_SERVICE_RESULT", "MONITOR_EXIT_CODE", "MONITOR_EXIT_STATUS",
	"MONITOR_INVOCATION_ID", "MONITOR_UNIT",
	"TRIGGER_UNIT", "TRIGGER_PATH", "TRIGGER_TIMER_REALTIME_USEC", "TRIGGER_TIMER_MONOTONIC_USEC",
	"REMOTE_ADDR", "REMOTE_PORT", "SO_COOKIE",
	// Directories that go with the unit: removed when it stops, or private
	// to it (Amendment A1, K1-3).
	"RUNTIME_DIRECTORY", "CREDENTIALS_DIRECTORY",
	// launchd's per-job names (Amendment A1, K1-4).
	"XPC_SERVICE_NAME", "XPC_FLAGS",
}

// ManagerEnvNames returns the names of the variables a service manager sets
// for one unit or job, which ScrubManagerEnv leaves out: systemd's that
// describe the unit's run, its channels to the manager, the manager and
// why it ran, its runtime and credentials directories, and launchd's
// XPC_SERVICE_NAME and XPC_FLAGS. Each call returns a new slice. The list
// grows as the managers document new variables
// (docs/decisions/0033-MADR-scrub-service-manager-environment-from-handoffs.md).
func ManagerEnvNames() []string {
	return slices.Clone(managerEnvNames)
}

// ScrubManagerEnv returns env, a list of NAME=value entries such as
// os.Environ gives, without the variables ManagerEnvNames names, so that a
// process started from a service does not act for its unit: notify its
// socket, claim its invocation, or report as its job. Names compare
// exactly, and case-insensitively on Windows; an entry that is not
// NAME=value is kept. The library applies it to every environment it passes
// on; a program may apply it to the processes it starts itself.
func ScrubManagerEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		name, _, ok := strings.Cut(kv, "=")
		if ok && isManagerEnv(name) {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// goosWindows is runtime.GOOS on Windows, where environment names compare
// case-insensitively.
const goosWindows = "windows"

func isManagerEnv(name string) bool {
	if runtime.GOOS == goosWindows {
		return slices.ContainsFunc(managerEnvNames, func(n string) bool { return strings.EqualFold(n, name) })
	}
	return slices.Contains(managerEnvNames, name)
}

// passOn is this process's environment, scrubbed, then extra: a caller's
// explicit variables pass on whatever their names (0033-MADR D5-A).
func passOn(extra []string) []string {
	return append(ScrubManagerEnv(os.Environ()), extra...)
}
