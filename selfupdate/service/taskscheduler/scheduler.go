package taskscheduler

import (
	"os"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/internal/winproc"
)

// scheduler is Task Scheduler and the process table, faked in tests. The
// real one, over the COM API, is in scheduler_windows.go.
type scheduler interface {
	// open opens a registered task. A task that does not exist is
	// hrFileNotFound.
	open(folder, name string) (registered, error)
	// processes is a snapshot of every process.
	processes() ([]winproc.Process, error)
	// created is a process's creation time in 100 ns units since 1601, or
	// 0 when it cannot be read.
	created(pid uint32) uint64
	// running reports whether pid still runs as the process created at
	// born. A born of 0 is matched by the process ID alone.
	running(pid uint32, born uint64) bool
}

// registered is one open registered task.
type registered interface {
	// state is the task's TASK_STATE.
	state() (int32, error)
	// enabled and setEnabled read and set the task's switch.
	enabled() (bool, error)
	setEnabled(on bool) error
	// run starts the task (IRegisteredTask::Run).
	run() error
	// stop ends every running instance (IRegisteredTask::Stop). It ends
	// the action's process only, not the processes it started (0021-MADR
	// Amendment A1, probe 2).
	stop() error
	// instances are the running instances' engine process IDs, which are
	// the Exec action's own processes (0021-MADR Amendment A1, probe 1).
	instances() ([]uint32, error)
	// triggers are the task's triggers.
	triggers() ([]trigger, error)
	// actions are the task's actions.
	actions() ([]action, error)
	// logonType is the principal's TASK_LOGON_TYPE.
	logonType() (int32, error)
	// lastRun is the last run's result and start time.
	lastRun() (lastRun, error)
	// xml is the registered definition (IRegisteredTask::get_Xml).
	xml() (string, error)
	// setPath sets the one Exec action's Path on the definition object and
	// registers it again (TASK_UPDATE, no user), changing nothing else
	// (0021-MADR D5; Amendment A1, probe 4).
	setPath(path string) error
	// restore registers a saved definition over the task (TASK_UPDATE, no
	// user).
	restore(xml string) error
	close() error
}

// trigger is one of a task's triggers.
type trigger struct {
	Type    int32
	Enabled bool
	// Repeats reports a repetition interval.
	Repeats bool
}

// action is one of a task's actions; Path and Args are an Exec action's.
type action struct {
	Type int32
	Path string
	Args string
}

// lastRun describes the task's last run.
type lastRun struct {
	// Result is the run's result: the program's exit code for an Exec
	// action (0021-MADR Amendment A1, probe 3).
	Result int32
	// Time is when the run began.
	Time time.Time
}

// currentPID is this process's ID.
func currentPID() uint32 {
	return uint32(os.Getpid()) //nolint:gosec // a process ID fits in a DWORD
}
