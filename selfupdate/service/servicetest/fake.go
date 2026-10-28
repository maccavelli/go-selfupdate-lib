package servicetest

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// FakeOptions shape a FakeService. A zero field takes its default.
type FakeOptions struct {
	// StopBound is how long Stop waits for the service to go before it is
	// service.ErrTimeout. Default service.DefaultPollTimeout.
	StopBound time.Duration
	// StartBound is how long Start waits on a start that never completes
	// before it is service.ErrTimeout. Default service.DefaultPollTimeout.
	StartBound time.Duration
	// StopLatency is how long a SlowStop service takes to go after a
	// stop. Default one second.
	StopLatency time.Duration
	// Interval is how often Stop looks whether the service has gone.
	// Default service.DefaultPollInterval.
	Interval time.Duration
}

func (o FakeOptions) withDefaults() FakeOptions {
	if o.StopBound <= 0 {
		o.StopBound = service.DefaultPollTimeout
	}
	if o.StartBound <= 0 {
		o.StartBound = service.DefaultPollTimeout
	}
	if o.StopLatency <= 0 {
		o.StopLatency = time.Second
	}
	if o.Interval <= 0 {
		o.Interval = service.DefaultPollInterval
	}
	return o
}

// FakeService is an in-memory service: a selfupdate.Lifecycle and
// selfupdate.EnabledLifecycle that meets the contract VerifyLifecycle
// checks, and its own Lab. It keeps time with the time package, so in a
// testing/synctest bubble its bounds cost no wall time. Every method
// honours its context, and a failed Stop leaves the service wanted up, so
// its LifecycleCase sets RestoresAfterFailedStop. It is safe for
// concurrent use.
type FakeService struct {
	o  FakeOptions
	mu sync.Mutex
	st fakeState
}

// fakeState is the service; f.mu guards it.
type fakeState struct {
	installed, running, enabled, wanted, stopping, crashed bool
	// mode is the state Arrange set, which decides how a stop or a start
	// goes.
	mode State
	// goneAt is when a stop under way ends the process; zero is never.
	goneAt time.Time
}

var (
	_ selfupdate.Lifecycle        = (*FakeService)(nil)
	_ selfupdate.EnabledLifecycle = (*FakeService)(nil)
	_ Lab                         = (*FakeService)(nil)
)

// NewFakeService returns an absent FakeService.
func NewFakeService(o FakeOptions) *FakeService {
	return &FakeService{o: o.withDefaults()}
}

// StopBound is the bound its Stop documents, for LifecycleCase.StopBound.
func (f *FakeService) StopBound() time.Duration { return f.o.StopBound }

// StartBound is the bound its Start documents, for
// LifecycleCase.StartBound.
func (f *FakeService) StartBound() time.Duration { return f.o.StartBound }

// advance ends a stop whose time has come; f.mu is held.
func (f *FakeService) advance() {
	if f.st.stopping && !f.st.goneAt.IsZero() && !time.Now().Before(f.st.goneAt) {
		f.st.running, f.st.stopping = false, false
	}
}

// Installed implements selfupdate.Lifecycle.
func (f *FakeService) Installed(ctx context.Context, _ string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.st.installed, nil
}

// Running implements selfupdate.Lifecycle.
func (f *FakeService) Running(ctx context.Context, _ string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.advance()
	return f.st.running, nil
}

// Enabled implements selfupdate.EnabledLifecycle.
func (f *FakeService) Enabled(ctx context.Context, _ string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.st.enabled, nil
}

// Stop implements selfupdate.Lifecycle. Once the stop is issued, its wait
// runs to StopBound whatever ctx does, and ctx's error is returned after
// (0015-MADR B3). A stop the service ignores is service.ErrTimeout, and
// leaves the service wanted up.
func (f *FakeService) Stop(ctx context.Context, product string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return f.issueStop(ctx, context.WithoutCancel(ctx), product)
}

// issueStop issues the stop and waits on wait for the service to go.
func (f *FakeService) issueStop(caller, wait context.Context, product string) error {
	f.mu.Lock()
	f.advance()
	switch {
	case !f.st.installed:
		f.mu.Unlock()
		return fmt.Errorf("servicetest: fake %s: %w", product, service.ErrNotInstalled)
	case !f.st.running:
		f.mu.Unlock()
		return nil
	}
	f.st.wanted, f.st.stopping = false, true
	switch f.st.mode {
	case Stubborn:
		f.st.goneAt = time.Time{}
	case SlowStop:
		f.st.goneAt = time.Now().Add(f.o.StopLatency)
	default:
		f.st.running, f.st.stopping = false, false
	}
	f.mu.Unlock()
	deadline := time.Now().Add(f.o.StopBound)
	for {
		f.mu.Lock()
		f.advance()
		running := f.st.running
		f.mu.Unlock()
		if !running {
			return caller.Err()
		}
		now := time.Now()
		if !now.Before(deadline) {
			f.mu.Lock()
			f.st.wanted = true
			f.mu.Unlock()
			return errors.Join(fmt.Errorf("servicetest: fake %s still runs %s after it was stopped: %w", product, f.o.StopBound, service.ErrTimeout), caller.Err())
		}
		timer := time.NewTimer(min(f.o.Interval, deadline.Sub(now)))
		select {
		case <-wait.Done():
			timer.Stop()
			return errors.Join(fmt.Errorf("servicetest: fake %s: the stop's wait ended: %w", product, wait.Err()), caller.Err())
		case <-timer.C:
		}
	}
}

// Start implements selfupdate.Lifecycle. A start the manager never
// completes is service.ErrTimeout after StartBound.
func (f *FakeService) Start(ctx context.Context, product string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	f.advance()
	switch {
	case !f.st.installed:
		f.mu.Unlock()
		return fmt.Errorf("servicetest: fake %s: %w", product, service.ErrNotInstalled)
	case f.st.running:
		f.mu.Unlock()
		return nil
	case f.st.mode == StartHangs:
		f.mu.Unlock()
		timer := time.NewTimer(f.o.StartBound)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return fmt.Errorf("servicetest: fake %s: start: %w", product, ctx.Err())
		case <-timer.C:
			return fmt.Errorf("servicetest: fake %s did not start within %s: %w", product, f.o.StartBound, service.ErrTimeout)
		}
	case f.st.mode == Crashing:
		f.st.crashed, f.st.wanted = true, true
	default:
		f.st.running, f.st.wanted, f.st.stopping, f.st.crashed = true, true, false, false
	}
	f.mu.Unlock()
	return nil
}

// WaitHealthy implements selfupdate.Lifecycle: a service that exited at
// once is service.ErrUnhealthy.
func (f *FakeService) WaitHealthy(ctx context.Context, product string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.advance()
	switch {
	case f.st.crashed:
		return fmt.Errorf("servicetest: fake %s exited at once: %w", product, service.ErrUnhealthy)
	case !f.st.running:
		return fmt.Errorf("servicetest: fake %s is not running: %w", product, service.ErrUnhealthy)
	}
	return nil
}

// Arrange implements Lab. It ends any stop under way.
func (f *FakeService) Arrange(ctx context.Context, a Arrangement) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if a.State < Absent || a.State > StartHangs {
		return fmt.Errorf("servicetest: fake: %s: %w", a.State, errors.ErrUnsupported)
	}
	running := a.State == Running || a.State == SlowStop || a.State == Stubborn
	f.mu.Lock()
	defer f.mu.Unlock()
	f.st = fakeState{
		installed: a.State != Absent,
		running:   running,
		enabled:   a.Enabled && a.State != Absent,
		wanted:    running,
		mode:      a.State,
	}
	return nil
}

// Observe implements Lab.
func (f *FakeService) Observe(ctx context.Context) (Observation, error) {
	if err := ctx.Err(); err != nil {
		return Observation{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.advance()
	return Observation{
		Installed: f.st.installed,
		Running:   f.st.running,
		Stopping:  f.st.stopping,
		Enabled:   f.st.enabled,
		Wanted:    f.st.wanted,
		Detail:    "mode " + f.st.mode.String(),
	}, nil
}
