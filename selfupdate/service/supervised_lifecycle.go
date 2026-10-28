package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
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
//     process has gone, bounded by Poll's timeout, counted from before
//     Down, whatever the caller's context does, and returns that
//     context's error after
//     (docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md
//     B3). Kill, when set, is called once when the wait reaches KillAfter;
//  6. when the stop fails with the recorded process still running, it
//     calls Up, so the supervisor wants the service up again, as it did
//     before (docs/decisions/0024-MADR-documentation-pass-after-v1-15-1.md
//     D4-A). Up has a bound of its own, Poll's timeout, so a failed Stop
//     can take twice that (0035-MADR Amendment A3). Kill's error is part
//     of Stop's only when the stop fails.
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
	// One bound, from before Down, for Down and the wait
	// (0035-MADR Amendment A3).
	begin := time.Now()
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()
	defer func() { err = errors.Join(err, caller.Err()) }()

	var killErr error
	werr := s.o.Down(ctx)
	if werr != nil {
		werr = s.fail("stop", werr)
	} else {
		killErr, werr = s.waitDown(ctx, rec, begin, timeout)
	}
	if werr == nil {
		return nil
	}
	werr = errors.Join(werr, killErr)
	if alive, aerr := s.still(rec); aerr == nil && alive {
		// The wait may have spent ctx, so Up gets a bound of its own.
		uctx, ucancel := context.WithTimeout(context.WithoutCancel(caller), timeout)
		defer ucancel()
		if uerr := s.o.Up(uctx); uerr != nil {
			werr = errors.Join(werr, s.fail("start again after the failed stop of", uerr))
		}
	}
	return werr
}

// waitDown waits, until limit after begin, until State reports the
// service down and rec has gone, calling Kill once at KillAfter. Its error is ErrTimeout, with the last observation,
// when the clock or ctx's deadline, the same bound, ends it; killErr is
// Kill's, which Stop reports only when the stop fails.
func (s *Supervised) waitDown(ctx context.Context, rec instance, begin time.Time, limit time.Duration) (killErr, err error) {
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
		now := time.Now()
		if s.o.Kill != nil && !killed && !now.Before(killAt) {
			killed = true
			if kerr := s.o.Kill(ctx); kerr != nil {
				killErr = s.fail("kill", kerr)
			}
		}
		if now.Before(end) && sleepCtx(ctx, min(s.o.Poll.withDefaults().Interval, end.Sub(now))) == nil {
			continue
		}
		seen := fmt.Sprintf("up=%t pid=%d", last.Up, last.PID)
		if last.Detail != "" {
			seen += " " + last.Detail
		}
		terr := fmt.Errorf("%w: %s still runs %s after it was stopped (last: %s)", ErrTimeout, s.name, limit, seen)
		return killErr, errors.Join(terr, lastErr)
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

// WaitHealthy implements selfupdate.Lifecycle: PollHealthy, with the
// instance Stop recorded as Previous, over State (D6-A). An observation
// counts when the service is up as an instance other than the recorded
// one and, where the supervisor reports readiness (Ready set), ready. The
// wait fails at once with ErrUnhealthy when:
//
//   - State reports the service Failed;
//   - the instance changes after one has counted: it restarted during the
//     wait;
//   - State reports it Restarting after a new instance was up: it
//     exited; or Restarting has lasted, without a break, longer than the
//     settle window (DefaultPollSettle when there is none) with no new
//     instance ever up: it keeps exiting too fast to be seen up. One
//     restart before the first up is tolerated, though a supervisor such
//     as s6 shows it for a second (0035-PLAN Deviation D3).
//
// Then Options.Probe, the program's application check, runs, as for the
// other backends. Errors, matched with errors.Is: ErrUnhealthy and
// ErrTimeout. When ctx ends, its error is returned.
func (s *Supervised) WaitHealthy(ctx context.Context, _ string) error {
	s.mu.Lock()
	rec := s.recorded
	s.mu.Unlock()
	poll := s.o.Poll
	if rec.pid != 0 {
		poll.Previous = rec.String()
	}
	var counted string
	var restartingSince time.Time
	wasUp := false
	loop := poll.withDefaults().Settle
	if loop <= 0 {
		loop = DefaultPollSettle
	}
	probe := func(ctx context.Context) (Health, error) {
		st, err := s.state(ctx)
		if err != nil {
			return Health{}, err
		}
		h := Health{Instance: st.Instance, Detail: describeState(s.name, st)}
		if st.Failed {
			h.Failed = true
			return h, nil
		}
		if st.Restarting {
			now := time.Now()
			if restartingSince.IsZero() {
				restartingSince = now
			}
			switch {
			case wasUp:
				h.Failed = true
				h.Detail = fmt.Sprintf("%s exited after it was up, and its supervisor is starting it again (%s)", s.name, h.Detail)
			case now.Sub(restartingSince) > loop:
				h.Failed = true
				h.Detail = fmt.Sprintf("%s keeps exiting while its supervisor starts it again, for over %s (%s)", s.name, loop, h.Detail)
			}
			return h, nil
		}
		restartingSince = time.Time{}
		if !st.Up || st.Instance == "" || st.Instance == poll.Previous {
			return h, nil
		}
		wasUp = true
		if counted != "" && st.Instance != counted {
			h.Failed = true
			h.Detail = fmt.Sprintf("%s restarted during the health wait (%s, then %s)", s.name, counted, st.Instance)
			return h, nil
		}
		if st.Ready == nil || *st.Ready {
			counted = st.Instance
			h.Ready = true
		}
		return h, nil
	}
	if err := PollHealthy(ctx, probe, poll); err != nil {
		return err
	}
	if s.o.Probe == nil {
		return nil
	}
	app := s.o.Poll
	app.Settle = -1
	return PollHealthy(ctx, s.o.Probe, app)
}

// describeState is one observation, for an error message.
func describeState(name string, st SupervisedState) string {
	d := fmt.Sprintf("%s up=%t instance=%q want_up=%t", name, st.Up, st.Instance, st.WantUp)
	if st.Ready != nil {
		d += fmt.Sprintf(" ready=%t", *st.Ready)
	}
	if st.Detail != "" {
		d += ": " + st.Detail
	}
	return d
}

// Reconcile implements selfupdate.Reconciler, verifying only (D9-A): the
// binary the service ran when Stop recorded it must be executable, by
// SameExecutable. With nothing recorded, because the service was not
// running, it reports no change and a warning that it could not check. A
// mismatch is an error: a program whose service runs another binary, or
// which moves its binary, passes NewExecReconciler instead. Nothing is
// ever rewritten, so Restore does nothing.
func (s *Supervised) Reconcile(_ context.Context, _, executable string) (selfupdate.ReconcileResult, error) {
	s.mu.Lock()
	bin := s.binary
	s.mu.Unlock()
	if bin == "" {
		return selfupdate.ReconcileResult{
			Detail:   s.name + " was not running when it was stopped; its binary was not checked",
			Warnings: selfupdate.NewWarnings(s.name + "'s binary could not be checked: the service was not running"),
		}, nil
	}
	// A binary replaced before the stop reads "<path> (deleted)"
	// (0035-MADR Amendment A1, P1).
	ran := strings.TrimSuffix(bin, " (deleted)")
	if !SameExecutable(ran, executable) {
		return selfupdate.ReconcileResult{}, s.fail("reconcile",
			fmt.Errorf("the service ran %s, not %s; a program whose service runs another binary passes service.NewExecReconciler", ran, executable))
	}
	return selfupdate.ReconcileResult{Detail: s.name + " runs " + executable}, nil
}

// Restore implements selfupdate.Reconciler. Reconcile rewrites nothing, so
// there is nothing to restore.
func (*Supervised) Restore(context.Context, string, selfupdate.ReconcileResult) error {
	return nil
}

// Detach implements Detacher: Options.DetachCheck, when set, runs first,
// and its error refuses the handoff, wrapped with ErrUnsupported. Then the
// run starts through the hop (EnvHandOffHop): DetachProcess starts a hop in
// a new session, with the service manager's per-unit variables left out of
// its environment, and the hop starts the real run and exits. The real run
// waits for the hop to exit, so its parent chain no longer reaches the
// service, and its Stop is not refused as inside
// (0035-PLAN-serve-supervisors-through-a-supervised-adapter.md Deviation
// D2, as scm's since docs/decisions/0011-MADR-reference-service-lifecycles.md
// amendment A4; docs/decisions/0033-MADR-scrub-service-manager-environment-from-handoffs.md).
// The program completes the hop by calling ReportFunc or LoadHandOffEnv
// first in main, as a detached run does anyway.
func (s *Supervised) Detach(ctx context.Context, spec HandOff) (Detached, error) {
	if s.o.DetachCheck != nil {
		if err := s.o.DetachCheck(ctx); err != nil {
			return Detached{}, fmt.Errorf("%w: %w", ErrUnsupported, s.fail("hand an update off from", err))
		}
	}
	spec.Env = append(append([]string(nil), spec.Env...), EnvHandOffHop+"=1")
	det, err := s.detach(ctx, spec)
	if err != nil {
		return Detached{}, err
	}
	det.Where = "a detached run, started through the hop " + det.Where
	return det, nil
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
