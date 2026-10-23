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
	// register registers a definition as name in folder
	// (ITaskFolder::RegisterTask, no user), with TASK_CREATION flags and a
	// TASK_LOGON_TYPE. TASK_CREATE on an existing task is hrAlreadyExists,
	// and a missing folder is hrFileNotFound (0032-MADR Amendment A1,
	// P1-4).
	register(folder, name, xml string, flags, logon int32) error
	// deleteTask deletes a task (ITaskFolder::DeleteTask). A missing task,
	// or one in a missing folder, is hrFileNotFound. A running instance is
	// not ended (0032-MADR Amendment A1, P1-4 and P1-5).
	deleteTask(folder, name string) error
	// createFolder creates a folder, such as `\Vendor\Product`, with every
	// missing level (ITaskFolder::CreateFolder on the root). An existing
	// folder is hrAlreadyExists.
	createFolder(path string) error
	// list lists a folder's tasks, hidden ones included
	// (ITaskFolder::GetTasks with TASK_ENUM_HIDDEN).
	list(folder string) ([]listed, error)
}

// listed is one task of a folder's listing.
type listed struct {
	Name       string
	State      int32
	LastResult int32
	LastRun    time.Time
}

// principal is a task's principal.
type principal struct {
	// UserID is IPrincipal::get_UserId: the account's name without its
	// domain, though the stored definition holds its SID (0032-MADR
	// Amendment A1, P2-6).
	UserID    string
	LogonType int32
	// RunLevel is TASK_RUNLEVEL_TYPE: 0 least privilege, 1 highest.
	RunLevel int32
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
	// principal is the task's principal.
	principal() (principal, error)
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
