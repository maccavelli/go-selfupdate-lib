package systemd

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// Installed reports whether the unit is loaded or loadable: its LoadState
// is anything but not-found (0011-MADR §7).
func (u *Unit) Installed(ctx context.Context, product string) (bool, error) {
	unit, err := u.unit(product)
	if err != nil {
		return false, err
	}
	p, err := u.show(ctx, unit, "LoadState")
	if err != nil {
		return false, err
	}
	return p["LoadState"] != "" && p["LoadState"] != "not-found", nil
}

// runningStates are the ActiveStates in which a process may hold the
// image, so Stop must cover them (0011-MADR §2).
var runningStates = map[string]bool{
	"active": true, "reloading": true, "refreshing": true, "activating": true, "deactivating": true,
}

// Running reports whether the unit is in a state where a process may hold
// its binary: active, reloading, refreshing, activating or deactivating.
func (u *Unit) Running(ctx context.Context, product string) (bool, error) {
	unit, err := u.unit(product)
	if err != nil {
		return false, err
	}
	p, err := u.show(ctx, unit, "ActiveState")
	if err != nil {
		return false, err
	}
	return runningStates[p["ActiveState"]], nil
}

// Enabled implements selfupdate.EnabledLifecycle: the unit is configured
// to start, which only UnitFileState enabled or enabled-runtime means.
// static, indirect, generated, alias and transient do not, though
// `systemctl is-enabled` exits 0 for them (systemctl(1)).
func (u *Unit) Enabled(ctx context.Context, product string) (bool, error) {
	unit, err := u.unit(product)
	if err != nil {
		return false, err
	}
	p, err := u.show(ctx, unit, "UnitFileState")
	if err != nil {
		return false, err
	}
	s := p["UnitFileState"]
	return s == "enabled" || s == "enabled-runtime", nil
}

// Stop stops the unit and waits until it is inactive or failed. It
// refuses with service.ErrInsideService when this process would die with
// the unit: the update must be handed off (0011-MADR §3).
func (u *Unit) Stop(ctx context.Context, product string) error {
	unit, err := u.unit(product)
	if err != nil {
		return err
	}
	inside, err := u.insideUnit(ctx, unit)
	if err != nil {
		return err
	}
	if inside {
		return fmt.Errorf("%w: %s", service.ErrInsideService, unit)
	}
	bound := u.stopBound(ctx, unit)
	if err := ctx.Err(); err != nil {
		return err
	}
	// Once the stop is issued the unit goes whatever the caller does, so
	// neither the command nor the wait ends with the caller's context; its
	// error is returned after (0015-MADR B3).
	sctx := context.WithoutCancel(ctx)
	out, err := u.systemctlRun(sctx, "stop", "--no-ask-password", "--quiet", "--", unit)
	if err != nil {
		return err
	}
	if out.ExitCode != 0 {
		return commandError("stop", unit, out)
	}
	// The client's exit is not the job's result: wait on the unit.
	err = u.waitState(sctx, unit, "stop", bound, func(p properties) (bool, bool) {
		s := p["ActiveState"]
		return s == "inactive" || s == "failed", false
	})
	return errors.Join(err, ctx.Err())
}

// stopGrace is how much longer than the unit's own TimeoutStopUSec the stop
// wait allows: systemd kills the unit then (0011-MADR §7).
const stopGrace = 30 * time.Second

// stopBound is how long Stop waits for the unit once the stop is issued: the
// longer of Options.Poll's timeout and TimeoutStopUSec plus stopGrace. A
// value that is "infinity" or cannot be read leaves the poll timeout
// (0015-MADR B3).
func (u *Unit) stopBound(ctx context.Context, unit string) time.Duration {
	bound := u.o.Poll.Timeout
	if bound <= 0 {
		bound = service.DefaultPollTimeout
	}
	p, err := u.show(ctx, unit, "TimeoutStopUSec")
	if err != nil {
		return bound
	}
	if t, ok := parseTimespan(p["TimeoutStopUSec"]); ok {
		return max(bound, t+stopGrace)
	}
	return bound
}

// Start records the unit's invocation, so WaitHealthy can require a new
// one, then starts it. For Type=notify or Type=exec, systemctl returns once
// the start succeeded or failed.
func (u *Unit) Start(ctx context.Context, product string) error {
	unit, err := u.unit(product)
	if err != nil {
		return err
	}
	p, err := u.show(ctx, unit, "InvocationID")
	if err != nil {
		return err
	}
	out, err := u.systemctlRun(ctx, "start", "--no-ask-password", "--quiet", "--", unit)
	if err != nil {
		return err
	}
	if out.ExitCode != 0 {
		return commandError("start", unit, out)
	}
	// The restart count is read once the start returned: what the start
	// itself does to it is not a restart. A start during the RestartSec
	// wait is counted as one, so a count from before would differ
	// (0015-MADR D2, A3).
	after, err := u.show(ctx, unit, "NRestarts")
	if err != nil {
		return err
	}
	u.mu.Lock()
	u.baseline[unit] = baseline{invocation: p["InvocationID"], restarts: after["NRestarts"]}
	u.mu.Unlock()
	return nil
}

// WaitHealthy waits until the unit is active as a new invocation, with no
// automatic restart, for the settle window; then runs Options.Probe. It
// fails at once on failed, on a return to inactive, or on a restart
// (0011-MADR §7).
func (u *Unit) WaitHealthy(ctx context.Context, product string) error {
	unit, err := u.unit(product)
	if err != nil {
		return err
	}
	u.mu.Lock()
	base := u.baseline[unit]
	u.mu.Unlock()
	probe := func(ctx context.Context) (service.Health, error) {
		p, err := u.show(ctx, unit, "ActiveState", "SubState", "InvocationID", "NRestarts", "Result")
		if err != nil {
			return service.Health{}, err
		}
		detail := fmt.Sprintf("%s ActiveState=%s SubState=%s Result=%s NRestarts=%s",
			unit, p["ActiveState"], p["SubState"], p["Result"], p["NRestarts"])
		switch {
		case p["ActiveState"] == "failed", p["ActiveState"] == "inactive",
			p["SubState"] == "auto-restart",
			base.restarts != "" && p["NRestarts"] != base.restarts:
			return service.Health{Failed: true, Detail: detail}, nil
		}
		return service.Health{
			Ready:    p["ActiveState"] == "active",
			Instance: p["InvocationID"],
			Detail:   detail,
		}, nil
	}
	poll := u.o.Poll
	poll.Previous = base.invocation
	if err := service.PollHealthy(ctx, probe, poll); err != nil {
		return err
	}
	if u.o.Probe == nil {
		return nil
	}
	app := u.o.Poll
	app.Settle = -1
	return service.PollHealthy(ctx, u.o.Probe, app)
}

// waitState polls the unit until done reports true, within timeout, or
// Options.Poll's when it is zero.
func (u *Unit) waitState(ctx context.Context, unit, step string, timeout time.Duration, done func(properties) (ok, failed bool)) error {
	probe := func(ctx context.Context) (service.Health, error) {
		p, err := u.show(ctx, unit, "ActiveState", "SubState")
		if err != nil {
			return service.Health{}, err
		}
		ok, failed := done(p)
		return service.Health{
			Ready: ok, Failed: failed,
			Detail: fmt.Sprintf("%s ActiveState=%s SubState=%s", unit, p["ActiveState"], p["SubState"]),
		}, nil
	}
	poll := u.o.Poll
	poll.Settle, poll.Previous = -1, ""
	if timeout > 0 {
		poll.Timeout = timeout
	}
	if err := service.PollHealthy(ctx, probe, poll); err != nil {
		if errors.Is(err, service.ErrTimeout) {
			return fmt.Errorf("selfupdate: systemd: %s %s: %w", step, unit, err)
		}
		return err
	}
	return nil
}
