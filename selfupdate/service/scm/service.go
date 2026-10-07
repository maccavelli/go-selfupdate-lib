// Package scm is the reference Windows Service Control Manager lifecycle
// for a managed update
// (docs/decisions/0011-MADR-reference-service-lifecycles.md): a
// selfupdate.Lifecycle, selfupdate.EnabledLifecycle, selfupdate.Reconciler
// and service.Detacher for one Win32 service.
//
// It calls the SCM through x/sys/windows, wrapping its handles in mgr.Mgr
// and mgr.Service, and opens each with only the rights its call needs, so a
// non-administrator can probe a service whose ACL allows it. It never
// calls mgr.Connect or mgr.Service.UpdateConfig. It compiles on every OS;
// New returns service.ErrUnsupported off Windows.
package scm

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// Options configure a Service.
type Options struct {
	// Name is the service's name, not its display name. Empty means the
	// product.
	Name string
	// Probe, when set, is an application readiness check WaitHealthy runs
	// once the SCM reports the service healthy.
	Probe service.HealthProbe
	// Poll bounds stop, start and health waits. Once a stop is sent, its
	// wait runs to Poll's timeout whatever the caller's context does.
	Poll service.PollOptions
	// RewritePath lets Reconcile point the service at a binary that moved.
	// Without it, a service running another binary is an error. The
	// rewritten command line quotes the program; an unquoted one that
	// could name two programs is refused.
	RewritePath bool
	// StopDependents lets Stop stop running dependent services first.
	// Without it, a running dependent is an error.
	StopDependents bool
	// TriggerStartEnabled counts a demand-start service with start triggers
	// as enabled. Without it, only an automatic start type does.
	TriggerStartEnabled bool
}

// Service manages one Windows service.
type Service struct {
	o     Options
	m     manager
	now   func() time.Time
	sleep func(context.Context, time.Duration) error
	self  func() uint32

	mu       sync.Mutex
	previous map[string]uint32
}

var (
	_ selfupdate.Lifecycle        = (*Service)(nil)
	_ selfupdate.EnabledLifecycle = (*Service)(nil)
	_ selfupdate.Reconciler       = (*Service)(nil)
	_ service.Detacher            = (*Service)(nil)
)

// New returns a Service for o. Off Windows it returns
// service.ErrUnsupported.
func New(o Options) (*Service, error) {
	return newService(o, runtime.GOOS, systemManager())
}

func newService(o Options, goos string, m manager) (*Service, error) {
	if goos != "windows" {
		return nil, fmt.Errorf("%w: the SCM runs on Windows, not %s", service.ErrUnsupported, goos)
	}
	if o.Name != "" {
		if err := validName(o.Name); err != nil {
			return nil, err
		}
	}
	return &Service{
		o: o, m: m, now: time.Now, sleep: sleepCtx, self: currentPID,
		previous: map[string]uint32{},
	}, nil
}

// validName refuses what the SCM refuses in a service name: empty, longer
// than 256 characters, or holding / or \ (CreateService).
func validName(name string) error {
	if name == "" || len(name) > 256 || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("selfupdate: scm: invalid service name %q", name)
	}
	return nil
}

// name is Options.Name, or the product.
func (s *Service) name(product string) (string, error) {
	if s.o.Name != "" {
		return s.o.Name, nil
	}
	if err := validName(product); err != nil {
		return "", err
	}
	return product, nil
}

// open opens the named service with access, mapping the SCM's errors to
// the typed ones, and refuses a driver.
func (s *Service) open(name string, access uint32) (handle, error) {
	h, err := s.m.open(name, access|accessQueryStatus)
	if err != nil {
		return nil, mapError("open", name, err)
	}
	st, err := h.status()
	if err != nil {
		return nil, errors.Join(mapError("query", name, err), h.close())
	}
	if st.Type&typeDriver != 0 {
		return nil, errors.Join(fmt.Errorf("%w: %s is a driver, not a Win32 service", service.ErrUnsupported, name), h.close())
	}
	return h, nil
}

// mapError wraps an SCM error with its step, as a typed error where one
// fits.
func mapError(step, name string, err error) error {
	e := fmt.Errorf("selfupdate: scm: %s %s: %w", step, name, err)
	switch {
	case errors.Is(err, errDoesNotExist):
		return fmt.Errorf("%w: %w", service.ErrNotInstalled, e)
	case errors.Is(err, errAccessDenied):
		return fmt.Errorf("%w: %w", service.ErrPermission, e)
	}
	return e
}

// exitDetail describes a stopped service's exit code: the service-specific
// code when the service set one.
func exitDetail(st status) string {
	if st.Win32ExitCode == errServiceSpecificErr {
		return fmt.Sprintf("service-specific exit code %d", st.SpecificExit)
	}
	return fmt.Sprintf("exit code %d", st.Win32ExitCode)
}

func stateName(state uint32) string {
	switch state {
	case stateStopped:
		return "STOPPED"
	case stateStartPending:
		return "START_PENDING"
	case stateStopPending:
		return "STOP_PENDING"
	case stateRunning:
		return "RUNNING"
	case stateContinuePending:
		return "CONTINUE_PENDING"
	case statePausePending:
		return "PAUSE_PENDING"
	case statePaused:
		return "PAUSED"
	}
	return fmt.Sprintf("state %d", state)
}

// sleepCtx sleeps for d, or until ctx ends.
func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
