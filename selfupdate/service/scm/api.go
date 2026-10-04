package scm

import "syscall"

// The SCM values this package uses, as Win32 defines them. They are
// declared here, not taken from x/sys/windows, so the logic and its fakes
// compile and run on every OS.

// Access rights: each call asks for only what it needs (0011-MADR §7).
const (
	accessQueryConfig        = 0x0001 // SERVICE_QUERY_CONFIG
	accessChangeConfig       = 0x0002 // SERVICE_CHANGE_CONFIG
	accessQueryStatus        = 0x0004 // SERVICE_QUERY_STATUS
	accessEnumerateDependent = 0x0008 // SERVICE_ENUMERATE_DEPENDENTS
	accessStart              = 0x0010 // SERVICE_START
	accessStop               = 0x0020 // SERVICE_STOP
)

// Service states (SERVICE_STATUS_PROCESS.dwCurrentState).
const (
	stateStopped         = 1
	stateStartPending    = 2
	stateStopPending     = 3
	stateRunning         = 4
	stateContinuePending = 5
	statePausePending    = 6
	statePaused          = 7
)

// Service types and start types.
const (
	typeDriver = 0x0B // SERVICE_KERNEL_DRIVER | SERVICE_FILE_SYSTEM_DRIVER | SERVICE_RECOGNIZER_DRIVER

	startAuto   = 2 // SERVICE_AUTO_START
	startDemand = 3 // SERVICE_DEMAND_START
)

// controlStop is SERVICE_CONTROL_STOP.
const controlStop = 1

// Win32 errors, which x/sys/windows declares as these syscall.Errno values.
const (
	errAccessDenied       = syscall.Errno(5)    // ERROR_ACCESS_DENIED
	errDependentsRunning  = syscall.Errno(1051) // ERROR_DEPENDENT_SERVICES_RUNNING
	errAlreadyRunning     = syscall.Errno(1056) // ERROR_SERVICE_ALREADY_RUNNING
	errDoesNotExist       = syscall.Errno(1060) // ERROR_SERVICE_DOES_NOT_EXIST
	errCannotAcceptCtrl   = syscall.Errno(1061) // ERROR_SERVICE_CANNOT_ACCEPT_CTRL
	errNotActive          = syscall.Errno(1062) // ERROR_SERVICE_NOT_ACTIVE
	errServiceSpecificErr = 1066                // ERROR_SERVICE_SPECIFIC_ERROR
)

// status is one QueryServiceStatusEx(SC_STATUS_PROCESS_INFO) result.
type status struct {
	Type          uint32
	State         uint32
	CheckPoint    uint32
	WaitHint      uint32 // milliseconds
	ProcessID     uint32
	Win32ExitCode uint32
	SpecificExit  uint32
}

// config is what QueryServiceConfig and QueryServiceConfig2 report.
type config struct {
	Type             uint32
	StartType        uint32
	DelayedAutoStart bool
	BinaryPathName   string
	// Triggers is the number of start triggers
	// (SERVICE_CONFIG_TRIGGER_INFO).
	Triggers uint32
}

// process is one entry of a process snapshot.
type process struct {
	PID       uint32
	ParentPID uint32
}

// manager is the SCM and the process table, faked in tests. The real one
// wraps its handles in x/sys's mgr.Mgr and mgr.Service.
type manager interface {
	// open opens the named service with exactly access.
	open(name string, access uint32) (handle, error)
	// processes is a snapshot of every process.
	processes() ([]process, error)
	// created is a process's creation time in 100 ns units since 1601, or
	// 0 when it cannot be read.
	created(pid uint32) uint64
	// decompose splits a command line as CommandLineToArgv does, and
	// compose quotes arguments so decompose returns them.
	decompose(cmdline string) ([]string, error)
	compose(args []string) string
}

// handle is an open service.
type handle interface {
	status() (status, error)
	control(code uint32) (status, error)
	start() error
	config() (config, error)
	// setBinaryPathName is ChangeServiceConfig with SERVICE_NO_CHANGE for
	// everything but the path.
	setBinaryPathName(path string) error
	// dependents are the active dependent services, direct or not, in the
	// order they must stop (EnumDependentServices).
	dependents() ([]string, error)
	close() error
}
