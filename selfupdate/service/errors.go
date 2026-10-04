package service

import "errors"

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
)
