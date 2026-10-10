package taskscheduler

import "fmt"

// The Task Scheduler values this package uses, as taskschd.h defines them.
// They are declared here, not taken from x/sys/windows, which has none, so
// the logic and its fakes compile and run on every OS.

// Task states (TASK_STATE): Task Scheduler's own numbers, which no locale
// changes (0021-MADR D2).
const (
	stateUnknown  = 0 // TASK_STATE_UNKNOWN
	stateDisabled = 1 // TASK_STATE_DISABLED
	stateQueued   = 2 // TASK_STATE_QUEUED
	stateReady    = 3 // TASK_STATE_READY
	stateRunning  = 4 // TASK_STATE_RUNNING
)

// Trigger types (TASK_TRIGGER_TYPE2) that start a task with nobody acting
// (0021-MADR D3).
const (
	triggerTime  = 1 // TASK_TRIGGER_TIME
	triggerBoot  = 8 // TASK_TRIGGER_BOOT
	triggerLogon = 9 // TASK_TRIGGER_LOGON
)

// actionExec is TASK_ACTION_EXEC.
const actionExec = 0

// Logon types (TASK_LOGON_TYPE), named in errors.
const (
	logonNone             = 0 // TASK_LOGON_NONE
	logonPassword         = 1 // TASK_LOGON_PASSWORD
	logonS4U              = 2 // TASK_LOGON_S4U
	logonInteractiveToken = 3 // TASK_LOGON_INTERACTIVE_TOKEN
	logonGroup            = 4 // TASK_LOGON_GROUP
	logonServiceAccount   = 5 // TASK_LOGON_SERVICE_ACCOUNT
)

// hresult is a COM error code.
type hresult uint32

func (h hresult) Error() string { return fmt.Sprintf("HRESULT 0x%08X", uint32(h)) }

// The HRESULTs that map to typed errors, measured on the Windows test host
// (0021-MADR Amendment A1, probe 5).
const (
	hrFileNotFound = hresult(0x80070002) // HRESULT_FROM_WIN32(ERROR_FILE_NOT_FOUND): no such task
	hrAccessDenied = hresult(0x80070005) // HRESULT_FROM_WIN32(ERROR_ACCESS_DENIED)
)

func logonName(t int32) string {
	switch t {
	case logonNone:
		return "none"
	case logonPassword:
		return "a stored password"
	case logonS4U:
		return "S4U"
	case logonInteractiveToken:
		return "an interactive token"
	case logonGroup:
		return "a group"
	case logonServiceAccount:
		return "a service account"
	}
	return fmt.Sprintf("logon type %d", t)
}
