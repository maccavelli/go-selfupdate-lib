package service

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Stop implements selfupdate.Lifecycle
// (docs/decisions/0035-MADR-serve-supervisors-through-a-supervised-adapter.md
// D4-C):
//
//  1. a service that is not installed is ErrNotInstalled;
//  2. it refuses with ErrInsideService when this process is inside the
//     service (Inside), before anything changes;
//  3. it records the running instance, by PID and start time, and the
//     binary it runs;
//  4. it calls Down;
//  5. it waits until State reports the service down and the recorded
//     process has gone, bounded by Poll's timeout whatever the caller's
//     context does, and returns that context's error after
//     (docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md
//     B3). Kill, when set, is called once when the wait reaches KillAfter;
//  6. when the stop fails with the recorded process still running, it
//     calls Up, so the supervisor wants the service up again, as it did
//     before (docs/decisions/0024-MADR-documentation-pass-after-v1-15-1.md
//     D4-A). Kill's error is part of Stop's only when the stop fails.
//
// A service that is down and not wanted up is left as it is. With no
// supervisor, nothing can be stopped: Stop returns nil when nothing runs,
// and an error wrapping ErrNoSupervisor when the recorded process still
// does.
//
// Errors, matched with errors.Is: ErrNotInstalled, ErrInsideService,
// ErrNoSupervisor and ErrTimeout, and the program's own errors. When ctx
// ends, its error is returned.
func (s *Supervised) Stop(ctx context.Context, _ string) (err error) {
	st, serr := s.o.State(ctx)
	if serr != nil && !errors.Is(serr, ErrNoSupervisor) {
		return s.fail("observe", serr)
	}
	if !st.Installed {
		return s.fail("stop", ErrNotInstalled)
	}
	inside, err := s.Inside(ctx)
	if err != nil {
		return err
	}
	if inside {
		return s.fail("stop", ErrInsideService)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	var rec instance
	var bin string
	if st.Up && st.PID > 0 {
		in, ok, err := s.alive(st.PID)
		if err != nil {
			return s.fail("read the process of", err)
		}
		if ok {
			rec = in
			// Best-effort: Reconcile warns when there is no binary to check.
			bin, _ = s.exe(in.pid) //nolint:errcheck // an unreadable binary is recorded as none
		}
	}
	s.mu.Lock()
	s.recorded, s.binary = rec, bin
	s.mu.Unlock()
	if serr != nil {
		if rec.pid == 0 {
			return nil
		}
		return s.fail("stop", serr)
	}
	if rec.pid == 0 && !st.WantUp {
		return nil
	}

	caller := ctx
	timeout := s.o.Poll.withDefaults().Timeout
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()
	defer func() { err = errors.Join(err, caller.Err()) }()

	var killErr error
	werr := s.o.Down(ctx)
	if werr != nil {
		werr = s.fail("stop", werr)
	} else {
		killErr, werr = s.waitDown(ctx, rec, timeout)
	}
	if werr == nil {
		return nil
	}
	werr = errors.Join(werr, killErr)
	if alive, aerr := s.still(rec); aerr == nil && alive {
		if uerr := s.o.Up(ctx); uerr != nil {
			werr = errors.Join(werr, s.fail("start again after the failed stop of", uerr))
		}
	}
	return werr
}

// waitDown waits, for at most limit by this Supervised's clock, until State
// reports the service down and rec has gone, calling Kill once at
// KillAfter. Its error is ErrTimeout, with the last observation; killErr
// is Kill's, which Stop reports only when the stop fails.
func (s *Supervised) waitDown(ctx context.Context, rec instance, limit time.Duration) (killErr, err error) {
	begin := s.now()
	end, killAt := begin.Add(limit), begin.Add(s.o.KillAfter)
	killed := false
	var last SupervisedState
	var lastErr error
	for {
		st, err := s.o.State(ctx)
		last, lastErr = st, err
		if err == nil || errors.Is(err, ErrNoSupervisor) {
			alive, aerr := s.still(rec)
			if aerr != nil {
				lastErr = aerr
			} else if !st.Up && !alive {
				return killErr, nil
			}
		}
		now := s.now()
		if s.o.Kill != nil && !killed && !now.Before(killAt) {
			killed = true
			if kerr := s.o.Kill(ctx); kerr != nil {
				killErr = s.fail("kill", kerr)
			}
		}
		if !now.Before(end) {
			seen := fmt.Sprintf("up=%t pid=%d", last.Up, last.PID)
			if last.Detail != "" {
				seen += " " + last.Detail
			}
			terr := fmt.Errorf("%w: %s still runs %s after it was stopped (last: %s)", ErrTimeout, s.name, limit, seen)
			return killErr, errors.Join(terr, lastErr)
		}
		if err := s.sleep(ctx, min(s.o.Poll.withDefaults().Interval, end.Sub(now))); err != nil {
			return killErr, fmt.Errorf("%w: %s: %w", ErrTimeout, s.name, err)
		}
	}
}

// Start implements selfupdate.Lifecycle: a running instance other than the
// one Stop recorded is left alone; otherwise Up runs, which may start a
// supervisor first (D5-A). Its errors, such as ErrNoSupervisor, are
// returned wrapped.
func (s *Supervised) Start(ctx context.Context, _ string) error {
	st, err := s.state(ctx)
	if err != nil && !errors.Is(err, ErrNoSupervisor) {
		return err
	}
	s.mu.Lock()
	rec := s.recorded
	s.mu.Unlock()
	if err == nil && st.Up && st.Instance != "" && st.Instance != rec.String() {
		return nil
	}
	if err := s.o.Up(ctx); err != nil {
		return s.fail("start", err)
	}
	return nil
}

// sleepCtx sleeps for d, or until ctx ends.
func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
