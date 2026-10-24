package service

import (
	"errors"
	"fmt"
)

// The errors every backend returns, so a caller can tell them apart with
// errors.Is (0011-MADR §2). The managed installer wraps a lifecycle error
// in selfupdate.ErrManagedInstall as well.
var (
	// ErrUnsupported is returned on an OS, or a service manager version,
	// the backend does not support.
	ErrUnsupported = errors.New("selfupdate: service: not supported here")
	// ErrNotInstalled is returned when the service's definition does not
	// exist.
	ErrNotInstalled = errors.New("selfupdate: service: not installed")
	// ErrPermission is returned when the service manager refused for lack
	// of rights.
	ErrPermission = errors.New("selfupdate: service: permission denied")
	// ErrInsideService is returned by Stop when this process would die
	// with the service it was asked to stop. The update must be handed off
	// (HandOffIfInside).
	ErrInsideService = errors.New("selfupdate: service: this process runs inside the service it would stop")
	// ErrTimeout is returned when a stop, a start or a health wait outlived
	// its deadline.
	ErrTimeout = errors.New("selfupdate: service: timed out")
	// ErrUnhealthy is returned when the service failed, crashed or restarted
	// during a health wait.
	ErrUnhealthy = errors.New("selfupdate: service: not healthy")
	// ErrNoSupervisor is returned when the service exists, but nothing
	// supervises it, such as a runit or s6 service directory that no
	// runsvdir or s6-svscan serves after a logout. A program can start a
	// supervisor and try again
	// (docs/decisions/0035-MADR-serve-supervisors-through-a-supervised-adapter.md
	// D10-A).
	ErrNoSupervisor = errors.New("selfupdate: service: nothing supervises the service")
)

// CommandError is a service manager command that exited non-zero: a
// systemctl, systemd-run or launchctl run, or the exec reconciler's child.
// A failure the backend classifies wraps it with ErrPermission or
// ErrNotInstalled, so errors.Is sees the class and errors.As the command
// (docs/decisions/0025-MADR-align-with-go-sdk-design-practices.md D2). The
// scm and taskscheduler backends fail through Windows API errors, not
// commands, and return none.
type CommandError struct {
	// Backend names the backend: "systemd", "launchd", or "service" for the
	// exec reconciler.
	Backend string
	// Step is what was run, such as "stop" or "bootout".
	Step string
	// Name is what was acted on: the unit; the job's label, plist or
	// domain; or the product.
	Name string
	// ExitCode is the command's exit status.
	ExitCode int
	// Message is the command's standard error, or its standard output when
	// that is empty, trimmed. The Runner keeps at most 1 MiB of each
	// stream, and the exec reconciler quotes at most 4 KiB.
	Message string
}

// Error implements the error interface:
// "selfupdate: <backend>: <step> <name>: exit <code>: <message>".
func (e *CommandError) Error() string {
	if e == nil {
		return "selfupdate: service: command failed"
	}
	return fmt.Sprintf("selfupdate: %s: %s %s: exit %d: %s", e.Backend, e.Step, e.Name, e.ExitCode, e.Message)
}
