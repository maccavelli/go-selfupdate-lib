package scm

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// Installed reports whether the SCM knows the service.
func (s *Service) Installed(_ context.Context, product string) (bool, error) {
	name, err := s.name(product)
	if err != nil {
		return false, err
	}
	h, err := s.open(name, accessQueryStatus)
	if errors.Is(err, service.ErrNotInstalled) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, h.close()
}

// Running reports whether a process may hold the service's binary: any
// state but STOPPED, so Stop covers a service that is starting, pausing or
// paused (0011-MADR §2).
func (s *Service) Running(_ context.Context, product string) (bool, error) {
	name, err := s.name(product)
	if err != nil {
		return false, err
	}
	st, err := s.query(name)
	if errors.Is(err, service.ErrNotInstalled) {
		return false, nil
	}
	return err == nil && st.State != stateStopped, err
}

// Enabled implements selfupdate.EnabledLifecycle: the start type is
// automatic, delayed or not. A demand-start service with start triggers
// counts only with Options.TriggerStartEnabled.
func (s *Service) Enabled(_ context.Context, product string) (bool, error) {
	name, err := s.name(product)
	if err != nil {
		return false, err
	}
	h, err := s.open(name, accessQueryConfig)
	if err != nil {
		return false, err
	}
	c, err := h.config()
	if err = errors.Join(err, h.close()); err != nil {
		return false, mapError("query the configuration of", name, err)
	}
	switch {
	case c.StartType == startAuto:
		return true, nil
	case c.StartType == startDemand && c.Triggers > 0:
		return s.o.TriggerStartEnabled, nil
	}
	return false, nil
}

// query reads the service's status once.
func (s *Service) query(name string) (status, error) {
	h, err := s.open(name, accessQueryStatus)
	if err != nil {
		return status{}, err
	}
	st, err := h.status()
	if err = errors.Join(err, h.close()); err != nil {
		return status{}, mapError("query", name, err)
	}
	return st, nil
}

// Stop stops the service and waits until it is STOPPED, by Microsoft's
// wait-hint and checkpoint loop. It records the process ID first, so
// WaitHealthy can require a new one. A running dependent service is an
// error, unless Options.StopDependents, when it is stopped first and
// recorded, for Start to start again (0015-MADR D6). Stop
// refuses with service.ErrInsideService when this process descends from
// the service: the update must be handed off (0011-MADR §3).
func (s *Service) Stop(ctx context.Context, product string) (err error) {
	name, err := s.name(product)
	if err != nil {
		return err
	}
	inside, err := s.insideService(name)
	if err != nil {
		return err
	}
	if inside {
		return fmt.Errorf("%w: %s", service.ErrInsideService, name)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Once a stop is sent the service goes whatever the caller does, so the
	// wait is bounded by Options.Poll's timeout alone, and the caller's
	// context error is returned after (0015-MADR B3).
	caller := ctx
	ctx, cancel := s.deadline(context.WithoutCancel(ctx))
	defer cancel()
	defer func() { err = errors.Join(err, caller.Err()) }()
	h, err := s.open(name, accessStop|accessEnumerateDependent)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, h.close()) }()
	st, err := h.status()
	if err != nil {
		return mapError("query", name, err)
	}
	if st.ProcessID != 0 {
		s.mu.Lock()
		s.previous[name] = st.ProcessID
		s.mu.Unlock()
	}
	if st.State == stateStopped {
		return nil
	}
	deps, err := h.dependents()
	if err != nil {
		return mapError("list the dependents of", name, err)
	}
	if len(deps) > 0 {
		if !s.o.StopDependents {
			return mapError("stop", name, fmt.Errorf("%w: %s", errDependentsRunning, strings.Join(deps, ", ")))
		}
		for _, d := range deps {
			if err := s.stopOne(ctx, d); err != nil {
				return err
			}
			s.mu.Lock()
			if !slices.Contains(s.stoppedDeps[name], d) {
				s.stoppedDeps[name] = append(s.stoppedDeps[name], d)
			}
			s.mu.Unlock()
		}
	}
	return s.stopHandle(ctx, name, h, st)
}

// stopOne stops a dependent service.
func (s *Service) stopOne(ctx context.Context, name string) (err error) {
	h, err := s.open(name, accessStop)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, h.close()) }()
	st, err := h.status()
	if err != nil {
		return mapError("query", name, err)
	}
	return s.stopHandle(ctx, name, h, st)
}

// stopHandle asks the service to stop, unless a stop is already pending,
// and waits for STOPPED. A service that cannot take the control yet, being
// pending, is waited out and asked once more.
func (s *Service) stopHandle(ctx context.Context, name string, h handle, st status) error {
	for asked := 0; st.State != stateStopped; {
		if st.State != stateStopPending {
			if asked == 2 {
				return fmt.Errorf("selfupdate: scm: stop %s: the service is %s and does not take the stop", name, stateName(st.State))
			}
			asked++
			_, err := h.control(controlStop)
			switch {
			case errors.Is(err, errNotActive):
				return nil
			case errors.Is(err, errCannotAcceptCtrl):
				// Pending: let it settle, then ask again.
			case err != nil:
				return mapError("stop", name, err)
			}
		}
		var err error
		if st, err = s.waitPending(ctx, name, h); err != nil {
			return err
		}
	}
	return nil
}

// Start starts the service, after waiting out a pending stop, and waits
// until it leaves START_PENDING. A service already running is left alone.
// Then it starts the dependents Stop stopped, in reverse stop order; one
// that fails is the error (0015-MADR D6).
func (s *Service) Start(ctx context.Context, product string) error {
	name, err := s.name(product)
	if err != nil {
		return err
	}
	ctx, cancel := s.deadline(ctx)
	defer cancel()
	if err := s.startOne(ctx, name); err != nil {
		return err
	}
	return s.startDependents(ctx, name)
}

// startDependents starts, in reverse stop order, the dependents Stop
// stopped for name, and forgets them once all have started.
func (s *Service) startDependents(ctx context.Context, name string) error {
	s.mu.Lock()
	deps := slices.Clone(s.stoppedDeps[name])
	s.mu.Unlock()
	for _, dep := range slices.Backward(deps) {
		if err := s.startOne(ctx, dep); err != nil {
			return fmt.Errorf("selfupdate: scm: start %s, which stopping %s stopped: %w", dep, name, err)
		}
	}
	s.mu.Lock()
	delete(s.stoppedDeps, name)
	s.mu.Unlock()
	return nil
}

// startOne starts one service, after waiting out a pending stop, and waits
// until it leaves START_PENDING. A service already running is left alone.
func (s *Service) startOne(ctx context.Context, name string) (err error) {
	h, err := s.open(name, accessStart)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, h.close()) }()
	st, err := s.waitPending(ctx, name, h)
	if err != nil {
		return err
	}
	if st.State != stateStopped {
		return nil
	}
	if err := h.start(); err != nil && !errors.Is(err, errAlreadyRunning) {
		return mapError("start", name, err)
	}
	st, err = s.waitPending(ctx, name, h)
	if err != nil {
		return err
	}
	if st.State == stateStopped {
		return fmt.Errorf("%w: %s stopped while starting, %s", service.ErrUnhealthy, name, exitDetail(st))
	}
	return nil
}

// WaitHealthy waits until the service is RUNNING as a process other than
// the one Stop saw, as that process for the settle window; then runs
// Options.Probe. It fails at once with service.ErrUnhealthy, and the exit
// code, when the service stops.
func (s *Service) WaitHealthy(ctx context.Context, product string) error {
	name, err := s.name(product)
	if err != nil {
		return err
	}
	s.mu.Lock()
	previous := s.previous[name]
	s.mu.Unlock()
	probe := func(context.Context) (service.Health, error) {
		st, err := s.query(name)
		if err != nil {
			return service.Health{}, err
		}
		detail := fmt.Sprintf("%s %s pid %d", name, stateName(st.State), st.ProcessID)
		if st.State == stateStopped {
			return service.Health{Failed: true, Detail: detail + ", " + exitDetail(st)}, nil
		}
		h := service.Health{Ready: st.State == stateRunning, Detail: detail}
		if st.ProcessID != 0 {
			h.Instance = strconv.FormatUint(uint64(st.ProcessID), 10)
		}
		return h, nil
	}
	poll := s.o.Poll
	if previous != 0 {
		poll.Previous = strconv.FormatUint(uint64(previous), 10)
	}
	if err := service.PollHealthy(ctx, probe, poll); err != nil {
		return err
	}
	if s.o.Probe == nil {
		return nil
	}
	app := s.o.Poll
	app.Settle = -1
	return service.PollHealthy(ctx, s.o.Probe, app)
}

// stallFloor is the least time a pending service may go without advancing
// its checkpoint before it counts as hung. Microsoft's loop uses the wait
// hint alone; a service built on x/sys/windows/svc reports pending states
// with a zero hint and checkpoint unless it sets them.
const stallFloor = 10 * time.Second

// waitPending polls a pending service until it settles, by Microsoft's
// loop: sleep a tenth of the wait hint, within 1 to 10 s, and declare a
// hang when the checkpoint has not advanced for the wait hint, at least
// stallFloor (0011-MADR §7). It returns the settled status.
func (s *Service) waitPending(ctx context.Context, name string, h handle) (status, error) {
	st, err := h.status()
	if err != nil {
		return status{}, mapError("query", name, err)
	}
	checkpoint, progressAt := st.CheckPoint, s.now()
	for pending(st.State) {
		wait := min(max(time.Duration(st.WaitHint)*time.Millisecond/10, time.Second), 10*time.Second)
		if err := s.sleep(ctx, wait); err != nil {
			return st, fmt.Errorf("%w: %s %s: %w", service.ErrTimeout, name, stateName(st.State), err)
		}
		if st, err = h.status(); err != nil {
			return status{}, mapError("query", name, err)
		}
		if st.CheckPoint != checkpoint {
			checkpoint, progressAt = st.CheckPoint, s.now()
			continue
		}
		if stall := max(time.Duration(st.WaitHint)*time.Millisecond, stallFloor); pending(st.State) && s.now().Sub(progressAt) > stall {
			return st, fmt.Errorf("%w: %s hung in %s: checkpoint %d unchanged for %v",
				service.ErrTimeout, name, stateName(st.State), st.CheckPoint, stall)
		}
	}
	return st, nil
}

// pending reports a state the service is moving out of on its own.
func pending(state uint32) bool {
	switch state {
	case stateStartPending, stateStopPending, stateContinuePending, statePausePending:
		return true
	}
	return false
}

// deadline bounds ctx by Options.Poll's timeout.
func (s *Service) deadline(ctx context.Context) (context.Context, context.CancelFunc) {
	timeout := s.o.Poll.Timeout
	if timeout <= 0 {
		timeout = service.DefaultPollTimeout
	}
	return context.WithTimeout(ctx, timeout)
}
