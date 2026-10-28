package servicetest

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// RunnerCase names the commands VerifyRunner runs.
type RunnerCase struct {
	// Quick is a command the runner answers at once. It is required.
	Quick service.Command
	// Blocking is a command that runs until its context ends. A zero Path
	// skips the check that needs it.
	Blocking service.Command
	// Calls, when set, reports how many runs the runner has recorded, so
	// a run on an ended context can be seen to record nothing.
	Calls func() int
	// KillBound is how long a blocking run may take to end once its
	// context has; zero means ten seconds.
	KillBound time.Duration
}

// VerifyRunner checks r against ExecRunner's handling of its context
// (docs/decisions/0037-MADR-honour-contexts-in-backend-fakes-and-stop-commands.md
// D1-A), and returns every violation joined, or nil:
//
//   - Quick runs on a live context with no error;
//   - on a cancelled context, and on one past its deadline, Run fails
//     with an error wrapping the context's, and, when Calls is set,
//     records nothing;
//   - a Blocking run ends, with an error wrapping context.Canceled, within
//     KillBound of its context's cancellation.
//
// It does not wait for a blocking run that ignores its cancellation.
func VerifyRunner(ctx context.Context, r service.Runner, c RunnerCase) error {
	switch {
	case r == nil:
		return errors.New("servicetest: the Runner is nil")
	case c.Quick.Path == "":
		return errors.New("servicetest: RunnerCase.Quick has no Path")
	}
	if c.KillBound <= 0 {
		c.KillBound = 10 * time.Second
	}
	if _, err := r.Run(ctx, c.Quick); err != nil {
		return fmt.Errorf("servicetest: Quick on a live context = %s; want no error", show(err))
	}
	var errs []error
	before := 0
	if c.Calls != nil {
		before = c.Calls()
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := r.Run(cancelled, c.Quick); !errors.Is(err, context.Canceled) {
		errs = append(errs, fmt.Errorf("servicetest: Run on a cancelled context = %s; want an error wrapping context.Canceled", show(err)))
	}
	expired, cancelExpired := context.WithDeadline(ctx, time.Now().Add(-time.Second))
	defer cancelExpired()
	if _, err := r.Run(expired, c.Quick); !errors.Is(err, context.DeadlineExceeded) {
		errs = append(errs, fmt.Errorf("servicetest: Run past its deadline = %s; want an error wrapping context.DeadlineExceeded", show(err)))
	}
	if c.Calls != nil {
		if after := c.Calls(); after != before {
			errs = append(errs, fmt.Errorf("servicetest: a Run on an ended context was recorded: %d calls, then %d", before, after))
		}
	}
	if c.Blocking.Path != "" {
		errs = append(errs, verifyBlocking(ctx, r, c))
	}
	return errors.Join(errs...)
}

// verifyBlocking starts c.Blocking, cancels it once it has had time to
// start, and requires it to end with the cancellation within KillBound.
func verifyBlocking(ctx context.Context, r service.Runner, c RunnerCase) error {
	bctx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := r.Run(bctx, c.Blocking)
		done <- err
	}()
	pause := time.NewTimer(100 * time.Millisecond)
	select {
	case err := <-done:
		pause.Stop()
		return fmt.Errorf("servicetest: Blocking returned before its context ended: %s", show(err))
	case <-pause.C:
	}
	cancel()
	bound := time.NewTimer(c.KillBound)
	defer bound.Stop()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			return fmt.Errorf("servicetest: a run whose context was cancelled = %s; want an error wrapping context.Canceled", show(err))
		}
		return nil
	case <-bound.C:
		return fmt.Errorf("servicetest: a run whose context was cancelled still ran after %s", c.KillBound)
	}
}
