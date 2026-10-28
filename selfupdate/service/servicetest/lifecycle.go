package servicetest

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// State is a state a Lab puts the service in. New states may be added
// within v1: a Lab returns an error wrapping errors.ErrUnsupported for one
// it cannot arrange, and the checks that need it are skipped.
type State int

// The states a Lab arranges.
const (
	// Absent: the service is not defined.
	Absent State = iota
	// Stopped: defined, and not running.
	Stopped
	// Running: running, and it stops when asked.
	Running
	// SlowStop: running, and it goes some time after a stop, within the
	// bound the lifecycle documents.
	SlowStop
	// Stubborn: running, and it ignores a stop; its manager does not end
	// it within the bound.
	Stubborn
	// Crashing: defined and not running; once started, it exits at once.
	Crashing
	// StartHangs: defined and not running; its manager never completes a
	// start.
	StartHangs
)

// String names the state.
func (s State) String() string {
	names := [...]string{"Absent", "Stopped", "Running", "SlowStop", "Stubborn", "Crashing", "StartHangs"}
	if s < 0 || int(s) >= len(names) {
		return fmt.Sprintf("State(%d)", int(s))
	}
	return names[s]
}

// Arrangement is what Lab.Arrange sets up.
type Arrangement struct {
	// State is the service's state.
	State State `json:"state"`
	// Enabled is whether the service is configured to start, in
	// selfupdate.EnabledLifecycle's sense.
	Enabled bool `json:"enabled"`
}

// Observation is the service as the lab sees it, without the Lifecycle.
type Observation struct {
	// Installed is whether the service is defined.
	Installed bool `json:"installed"`
	// Running is whether a process of the service runs.
	Running bool `json:"running"`
	// Stopping is whether a stop has been taken, and the service has not
	// gone.
	Stopping bool `json:"stopping"`
	// Enabled is whether the service is configured to start.
	Enabled bool `json:"enabled"`
	// Wanted is whether the manager will run the service again if it
	// exits: it wants it up.
	Wanted bool `json:"wanted"`
	// Detail describes the observation, for a violation's message.
	Detail string `json:"detail,omitempty"`
}

// Lab arranges and observes the service a Lifecycle controls. A program
// implements it over its fake, or over a real service manager. Arrange
// replaces whatever it arranged before, ending any process that state
// left. Lab never gains a method within v1; new states are new State
// values.
type Lab interface {
	Arrange(ctx context.Context, a Arrangement) error
	Observe(ctx context.Context) (Observation, error)
}

// Method names Lifecycle methods, as a set.
type Method uint8

// The methods a lifecycle may document as ignoring their context.
const (
	MethodInstalled Method = 1 << iota
	MethodRunning
	MethodEnabled
	MethodStart
)

// LifecycleCase names what VerifyLifecycle expects of one lifecycle.
type LifecycleCase struct {
	// Product is passed to every method.
	Product string
	// Lab arranges and observes the service.
	Lab Lab
	// StopBound is the longest Stop may take, as the implementation
	// documents it. It is required.
	StopBound time.Duration
	// StartBound is the longest Start may take when the manager never
	// completes it. Zero skips that check.
	StartBound time.Duration
	// Slack is the allowance over a bound before a call counts as hung;
	// zero means one second.
	Slack time.Duration
	// ContextFree are the methods the implementation documents as
	// ignoring their context. Stop and WaitHealthy may never be.
	ContextFree Method
	// RestoresAfterFailedStop says that after a Stop that fails with the
	// service still running, the manager still wants it up.
	RestoresAfterFailedStop bool
	// RequireAll makes a skipped check a violation, as a live CI run
	// needs.
	RequireAll bool
	// Logf, when set, receives each skipped check.
	Logf func(format string, args ...any)
}

// VerifyLifecycle checks l against the contract the managed installer
// relies on, through c.Lab, and returns every violation joined, or nil. It
// leaves the service Absent. It refuses a nil lifecycle, a nil lab, an
// empty product and a zero StopBound before any check.
//
// The checks:
//
//   - absent: Installed is false; a Stop or Start either fails with an
//     error wrapping service.ErrNotInstalled or leaves the service absent;
//   - ended context: a method not in ContextFree, and Stop and WaitHealthy
//     always, fail with an error wrapping the context's; Stop changes
//     nothing, nor does Start unless it is context-free;
//   - enabled: Enabled, when l implements selfupdate.EnabledLifecycle,
//     agrees with the lab;
//   - cycle: Stop, then Start, leaves the service running, and WaitHealthy
//     succeeds;
//   - stubborn: a stop the service ignores is service.ErrTimeout within
//     StopBound and Slack; Running then agrees with the lab; and, with
//     RestoresAfterFailedStop, the manager still wants it up;
//   - cancel during stop: a cancellation once the stop is under way does
//     not end the wait: Stop returns after the service has gone, with the
//     cancellation's error;
//   - crash: a service that exits at once fails WaitHealthy, with
//     service.ErrUnhealthy or service.ErrTimeout;
//   - start hangs: a start the manager never completes is
//     service.ErrTimeout within StartBound and Slack.
func VerifyLifecycle(ctx context.Context, l selfupdate.Lifecycle, c LifecycleCase) error {
	switch {
	case l == nil:
		return errors.New("servicetest: the Lifecycle is nil")
	case c.Lab == nil:
		return errors.New("servicetest: LifecycleCase.Lab is nil")
	case c.Product == "":
		return errors.New("servicetest: LifecycleCase.Product is empty")
	case c.StopBound <= 0:
		return errors.New("servicetest: LifecycleCase.StopBound is required")
	}
	if c.Slack <= 0 {
		c.Slack = time.Second
	}
	v := &verifier{ctx: ctx, l: l, c: c}
	for _, check := range []struct {
		name string
		run  func() error
	}{
		{"absent", v.absent},
		{"ended context", v.endedContext},
		{"enabled", v.enabled},
		{"cycle", v.cycle},
		{"stubborn", v.stubborn},
		{"cancel during stop", v.cancelDuringStop},
		{"crash", v.crash},
		{"start hangs", v.startHangs},
	} {
		err := check.run()
		switch {
		case err == nil:
		case errors.Is(err, errors.ErrUnsupported) && !c.RequireAll:
			v.logf("servicetest: %s: skipped: %v", check.name, err)
		default:
			v.errs = append(v.errs, fmt.Errorf("servicetest: %s: %w", check.name, err))
		}
	}
	if err := c.Lab.Arrange(context.WithoutCancel(ctx), Arrangement{State: Absent}); err != nil {
		v.errs = append(v.errs, fmt.Errorf("servicetest: Arrange(Absent) at the end: %w", err))
	}
	v.drain()
	return errors.Join(v.errs...)
}

// verifier holds one VerifyLifecycle run.
type verifier struct {
	ctx  context.Context
	l    selfupdate.Lifecycle
	c    LifecycleCase
	errs []error
	// strays are the calls that outlived their bound, drained once the
	// lab has removed the service.
	strays []chan error
}

func (v *verifier) logf(format string, args ...any) {
	if v.c.Logf != nil {
		v.c.Logf(format, args...)
	}
}

func (v *verifier) contextFree(m Method) bool { return v.c.ContextFree&m != 0 }

// withArrange is a check's result when Arrange failed after the check had
// found errs: the arrange error alone when there are none, so a lab's
// errors.ErrUnsupported can skip the check, and otherwise the violations,
// with the arrange error as text, so a skip never hides them.
func withArrange(errs []error, err error) error {
	if len(errs) == 0 {
		return err
	}
	return errors.Join(append(errs, errors.New(show(err)))...)
}

// show is err's text for a violation's message, or "no error". A
// lifecycle's error is shown, never wrapped, so it can never make a
// violation look like a skip.
func show(err error) string {
	if err == nil {
		return "no error"
	}
	return err.Error()
}

func (v *verifier) arrange(s State, enabled bool) error {
	if err := v.c.Lab.Arrange(v.ctx, Arrangement{State: s, Enabled: enabled}); err != nil {
		return fmt.Errorf("arrange %s: %w", s, err)
	}
	return nil
}

func (v *verifier) observe() (Observation, error) {
	o, err := v.c.Lab.Observe(v.ctx)
	if err != nil {
		return o, fmt.Errorf("observe: %w", err)
	}
	return o, nil
}

// call runs f in its own goroutine and waits for it at most bound and
// Slack. returned is false when it did not return; it is then a stray.
func (v *verifier) call(bound time.Duration, f func() error) (took time.Duration, returned bool, err error) {
	done := make(chan error, 1)
	start := time.Now()
	go func() { done <- f() }()
	timer := time.NewTimer(bound + v.c.Slack)
	defer timer.Stop()
	select {
	case err = <-done:
		return time.Since(start), true, err
	case <-timer.C:
		v.strays = append(v.strays, done)
		return time.Since(start), false, nil
	}
}

// drain waits for the calls that outlived their bound, now that the lab
// has removed the service, so none outlives the run.
func (v *verifier) drain() {
	for _, done := range v.strays {
		timer := time.NewTimer(v.c.StopBound + v.c.Slack)
		select {
		case <-done:
		case <-timer.C:
			v.errs = append(v.errs, errors.New("servicetest: a call that outlived its bound still had not returned after the service was removed"))
		}
		timer.Stop()
	}
}

func (v *verifier) absent() error {
	if err := v.arrange(Absent, false); err != nil {
		return err
	}
	var errs []error
	if ok, err := v.l.Installed(v.ctx, v.c.Product); err != nil || ok {
		errs = append(errs, fmt.Errorf("installed = %t, %s; want false and no error", ok, show(err)))
	}
	for _, m := range []struct {
		name string
		call func(context.Context, string) error
	}{{"Stop", v.l.Stop}, {"Start", v.l.Start}} {
		_, returned, err := v.call(v.c.StopBound, func() error { return m.call(v.ctx, v.c.Product) })
		if !returned {
			errs = append(errs, fmt.Errorf("%s of an absent service did not return", m.name))
			continue
		}
		if err != nil && !errors.Is(err, service.ErrNotInstalled) {
			errs = append(errs, fmt.Errorf("%s of an absent service = %s; want nil or service.ErrNotInstalled", m.name, show(err)))
		}
		if o, oerr := v.observe(); oerr != nil || o.Installed || o.Running {
			errs = append(errs, fmt.Errorf("after %s of an absent service the lab sees installed=%t running=%t (%s)", m.name, o.Installed, o.Running, show(oerr)))
		}
	}
	return errors.Join(errs...)
}

func (v *verifier) endedContext() error {
	if err := v.arrange(Running, true); err != nil {
		return err
	}
	ended, cancel := context.WithCancel(v.ctx)
	cancel()
	var errs []error
	check := func(name string, free bool, err error) {
		switch {
		case err != nil && !errors.Is(err, context.Canceled):
			errs = append(errs, fmt.Errorf("%s on an ended context = %s; want an error wrapping context.Canceled", name, show(err)))
		case err == nil && !free:
			errs = append(errs, fmt.Errorf("%s on an ended context returned no error", name))
		}
	}
	_, err := v.l.Installed(ended, v.c.Product)
	check("Installed", v.contextFree(MethodInstalled), err)
	_, err = v.l.Running(ended, v.c.Product)
	check("Running", v.contextFree(MethodRunning), err)
	if el, ok := v.l.(selfupdate.EnabledLifecycle); ok {
		_, err = el.Enabled(ended, v.c.Product)
		check("Enabled", v.contextFree(MethodEnabled), err)
	}
	check("WaitHealthy", false, v.l.WaitHealthy(ended, v.c.Product))
	check("Stop", false, v.l.Stop(ended, v.c.Product))
	if o, oerr := v.observe(); oerr != nil || !o.Running || o.Stopping {
		errs = append(errs, fmt.Errorf("after Stop on an ended context the lab sees running=%t stopping=%t (%s); want the service untouched", o.Running, o.Stopping, show(oerr)))
	}
	if err := v.arrange(Stopped, true); err != nil {
		return withArrange(errs, err)
	}
	check("Start", v.contextFree(MethodStart), v.l.Start(ended, v.c.Product))
	if !v.contextFree(MethodStart) {
		if o, oerr := v.observe(); oerr != nil || o.Running {
			errs = append(errs, fmt.Errorf("after Start on an ended context the lab sees running=%t (%s); want the service untouched", o.Running, show(oerr)))
		}
	}
	return errors.Join(errs...)
}

func (v *verifier) enabled() error {
	el, ok := v.l.(selfupdate.EnabledLifecycle)
	if !ok {
		return nil
	}
	var errs []error
	for _, on := range []bool{true, false} {
		if err := v.arrange(Stopped, on); err != nil {
			return withArrange(errs, err)
		}
		o, oerr := v.observe()
		got, err := el.Enabled(v.ctx, v.c.Product)
		if err != nil || oerr != nil || got != o.Enabled {
			errs = append(errs, fmt.Errorf("arranged enabled=%t: Enabled = %t, %s; the lab sees %t (%s)", on, got, show(err), o.Enabled, show(oerr)))
		}
	}
	return errors.Join(errs...)
}

func (v *verifier) cycle() error {
	if err := v.arrange(Running, true); err != nil {
		return err
	}
	var errs []error
	if ok, err := v.l.Installed(v.ctx, v.c.Product); err != nil || !ok {
		errs = append(errs, fmt.Errorf("installed = %t, %s; want true", ok, show(err)))
	}
	if ok, err := v.l.Running(v.ctx, v.c.Product); err != nil || !ok {
		errs = append(errs, fmt.Errorf("running = %t, %s; want true", ok, show(err)))
	}
	_, returned, err := v.call(v.c.StopBound, func() error { return v.l.Stop(v.ctx, v.c.Product) })
	switch {
	case !returned:
		return errors.Join(append(errs, fmt.Errorf("stop did not return within %s", v.c.StopBound+v.c.Slack))...)
	case err != nil:
		return errors.Join(append(errs, fmt.Errorf("stop = %s", show(err)))...)
	}
	if o, oerr := v.observe(); oerr != nil || o.Running {
		errs = append(errs, fmt.Errorf("after Stop the lab sees running=%t (%s)", o.Running, show(oerr)))
	}
	if ok, err := v.l.Running(v.ctx, v.c.Product); err != nil || ok {
		errs = append(errs, fmt.Errorf("running after Stop = %t, %s; want false", ok, show(err)))
	}
	if err := v.l.Start(v.ctx, v.c.Product); err != nil {
		return errors.Join(append(errs, fmt.Errorf("start = %s", show(err)))...)
	}
	if err := v.l.WaitHealthy(v.ctx, v.c.Product); err != nil {
		errs = append(errs, fmt.Errorf("WaitHealthy after Stop and Start = %s", show(err)))
	}
	if o, oerr := v.observe(); oerr != nil || !o.Running {
		errs = append(errs, fmt.Errorf("after Start the lab sees running=%t (%s)", o.Running, show(oerr)))
	}
	return errors.Join(errs...)
}

func (v *verifier) stubborn() error {
	if err := v.arrange(Stubborn, true); err != nil {
		return err
	}
	took, returned, err := v.call(v.c.StopBound, func() error { return v.l.Stop(v.ctx, v.c.Product) })
	switch {
	case !returned:
		return fmt.Errorf("stop of a service that ignores it did not return within %s", v.c.StopBound+v.c.Slack)
	case !errors.Is(err, service.ErrTimeout):
		return fmt.Errorf("stop of a service that ignores it = %s after %s; want service.ErrTimeout", show(err), took)
	}
	ok, rerr := v.l.Running(v.ctx, v.c.Product)
	o, oerr := v.observe()
	switch {
	case rerr != nil || oerr != nil || ok != o.Running:
		return fmt.Errorf("after the timed-out Stop, Running = %t, %s; the lab sees %t (%s)", ok, show(rerr), o.Running, show(oerr))
	case v.c.RestoresAfterFailedStop && o.Running && !o.Wanted:
		return errors.New("after the timed-out Stop the service runs, but its manager no longer wants it up")
	}
	return nil
}

func (v *verifier) cancelDuringStop() error {
	if err := v.arrange(SlowStop, true); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(v.ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- v.l.Stop(ctx, v.c.Product) }()
	deadline := time.Now().Add(v.c.StopBound)
	for {
		if o, err := v.observe(); err == nil && o.Stopping {
			break
		}
		if !time.Now().Before(deadline) {
			cancel()
			v.strays = append(v.strays, done)
			return errors.New("the lab never saw the stop under way")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	timer := time.NewTimer(v.c.StopBound + v.c.Slack)
	defer timer.Stop()
	var err error
	select {
	case err = <-done:
	case <-timer.C:
		v.strays = append(v.strays, done)
		return errors.New("stop did not return after its context was cancelled")
	}
	o, oerr := v.observe()
	switch {
	case !errors.Is(err, context.Canceled):
		return fmt.Errorf("stop cancelled while under way = %s; want the cancellation's error, after the stop", show(err))
	case oerr != nil || o.Running:
		return fmt.Errorf("stop returned on its cancellation while the service still ran (running=%t, %s): the cancellation cut the wait short", o.Running, show(oerr))
	}
	return nil
}

func (v *verifier) crash() error {
	if err := v.arrange(Crashing, true); err != nil {
		return err
	}
	if err := v.l.Start(v.ctx, v.c.Product); err != nil {
		if errors.Is(err, service.ErrUnhealthy) {
			return nil
		}
		return fmt.Errorf("start of a service that exits at once = %s", show(err))
	}
	err := v.l.WaitHealthy(v.ctx, v.c.Product)
	if !errors.Is(err, service.ErrUnhealthy) && !errors.Is(err, service.ErrTimeout) {
		return fmt.Errorf("WaitHealthy of a service that exits at once = %s; want service.ErrUnhealthy or service.ErrTimeout", show(err))
	}
	return nil
}

func (v *verifier) startHangs() error {
	if v.c.StartBound <= 0 {
		return fmt.Errorf("no LifecycleCase.StartBound: %w", errors.ErrUnsupported)
	}
	if err := v.arrange(StartHangs, true); err != nil {
		return err
	}
	took, returned, err := v.call(v.c.StartBound, func() error { return v.l.Start(v.ctx, v.c.Product) })
	switch {
	case !returned:
		return fmt.Errorf("start the manager never completes did not return within %s", v.c.StartBound+v.c.Slack)
	case !errors.Is(err, service.ErrTimeout):
		return fmt.Errorf("start the manager never completes = %s after %s; want service.ErrTimeout", show(err), took)
	}
	return nil
}
