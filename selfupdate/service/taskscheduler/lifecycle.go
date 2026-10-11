package taskscheduler

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// Installed reports whether Task Scheduler knows the task.
func (t *Task) Installed(_ context.Context, product string) (bool, error) {
	name, path, err := t.task(product)
	if err != nil {
		return false, err
	}
	r, err := t.open(name, path)
	if errors.Is(err, service.ErrNotInstalled) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, r.close()
}

// Running reports whether a process may hold the task's binary: Task
// Scheduler reports it running, or lists a running instance (0011-MADR §2).
func (t *Task) Running(_ context.Context, product string) (bool, error) {
	name, path, err := t.task(product)
	if err != nil {
		return false, err
	}
	r, err := t.open(name, path)
	if errors.Is(err, service.ErrNotInstalled) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	st, pids, err := status(r)
	if err = errors.Join(err, r.close()); err != nil {
		return false, mapError("query", path, err)
	}
	return st == stateRunning || len(pids) > 0, nil
}

// status reads a task's state and running instances.
func status(r registered) (int32, []uint32, error) {
	st, err := r.state()
	if err != nil {
		return 0, nil, err
	}
	pids, err := r.instances()
	return st, pids, err
}

// Enabled implements selfupdate.EnabledLifecycle: the task is enabled and
// has an enabled trigger that starts it with nobody acting: at logon, at
// boot, or on a time trigger that repeats. A task with only a one-off time
// trigger does not count (0021-MADR D3).
func (t *Task) Enabled(_ context.Context, product string) (bool, error) {
	name, path, err := t.task(product)
	if err != nil {
		return false, err
	}
	r, err := t.open(name, path)
	if err != nil {
		return false, err
	}
	on, triggers, err := switches(r)
	if err = errors.Join(err, r.close()); err != nil {
		return false, mapError("query the triggers of", path, err)
	}
	if !on {
		return false, nil
	}
	for _, tr := range triggers {
		if tr.Enabled && (tr.Type == triggerLogon || tr.Type == triggerBoot || (tr.Type == triggerTime && tr.Repeats)) {
			return true, nil
		}
	}
	return false, nil
}

func switches(r registered) (bool, []trigger, error) {
	on, err := r.enabled()
	if err != nil {
		return false, nil, err
	}
	triggers, err := r.triggers()
	return on, triggers, err
}

// Stop ends the task's running instance and waits until it has gone
// (0021-MADR D4):
//
//  1. it refuses with service.ErrInsideService when this process descends
//     from the instance: the update must be handed off (0011-MADR §3);
//  2. it records the instance, so WaitHealthy can require a new one;
//  3. it disables an enabled task, and records that it did, so a
//     repeating trigger cannot start the task while its binary is replaced:
//     ending the task alone does not stop the trigger (magic-cli-remote 0159
//     probe 8);
//  4. it calls Options.RequestStop, when set, and waits up to
//     Options.StopGrace for the program to exit on its own;
//  5. it ends the task (IRegisteredTask::Stop), and waits until no instance
//     runs and the recorded process has exited.
//
// Once the task is being stopped, the wait is bounded by Options.Poll's
// timeout alone, and the caller's context error is returned after
// (0015-MADR B3). A task that is not running is still disabled.
func (t *Task) Stop(ctx context.Context, product string) (err error) {
	name, path, err := t.task(product)
	if err != nil {
		return err
	}
	inside, err := t.insideTask(name, path)
	if err != nil {
		return err
	}
	if inside {
		return fmt.Errorf("%w: %s", service.ErrInsideService, path)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	caller := ctx
	ctx, cancel := t.deadline(context.WithoutCancel(ctx))
	defer cancel()
	defer func() { err = errors.Join(err, caller.Err()) }()
	r, err := t.open(name, path)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, r.close()) }()
	pids, err := r.instances()
	if err != nil {
		return mapError("list the instances of", path, err)
	}
	var inst instance
	if len(pids) > 0 {
		inst = instance{pid: pids[0], born: t.s.created(pids[0])}
		t.mu.Lock()
		t.previous[path] = inst
		t.mu.Unlock()
	}
	if err := t.disable(r, path); err != nil {
		return err
	}
	if len(pids) == 0 {
		return nil
	}
	var asked error
	if t.o.RequestStop != nil {
		if asked = t.o.RequestStop(ctx, inst.pid); asked == nil {
			gone, err := t.waitGone(ctx, r, path, inst, t.o.StopGrace)
			if err != nil || gone {
				return err
			}
		}
	}
	if err := r.stop(); err != nil {
		return errors.Join(mapError("stop", path, err), requestError(asked))
	}
	gone, err := t.waitGone(ctx, r, path, inst, t.o.Poll.Timeout)
	if err == nil && !gone {
		err = fmt.Errorf("%w: %s still runs after it was stopped", service.ErrTimeout, path)
	}
	if err != nil {
		return errors.Join(err, requestError(asked))
	}
	return nil
}

// disable disables an enabled task and records that it did.
func (t *Task) disable(r registered, path string) error {
	on, err := r.enabled()
	if err != nil {
		return mapError("query", path, err)
	}
	if !on {
		return nil
	}
	if err := r.setEnabled(false); err != nil {
		return mapError("disable", path, err)
	}
	t.mu.Lock()
	t.disabled[path] = true
	t.mu.Unlock()
	return nil
}

// requestError describes a failed Options.RequestStop, for Stop's error
// when ending the task failed too.
func requestError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("selfupdate: taskscheduler: Options.RequestStop: %w", err)
}

// waitGone waits, for at most limit by this Task's clock, until the task
// has no running instance and inst's process has exited. It reports
// whether they had; the error is a query failure, or ctx ending.
func (t *Task) waitGone(ctx context.Context, r registered, path string, inst instance, limit time.Duration) (bool, error) {
	if limit <= 0 {
		limit = service.DefaultPollTimeout
	}
	end := t.now().Add(limit)
	for {
		pids, err := r.instances()
		if err != nil {
			var hr hresult
			if errors.As(err, &hr) && hr == hrTaskNotRunning {
				pids = nil
			} else {
				return false, mapError("list the instances of", path, err)
			}
		}
		// The recorded process is what Stop must wait for. GetInstances can
		// still list a PID after the program has exited (T6 live run).
		if inst.pid != 0 {
			if !t.s.running(inst.pid, inst.born) {
				return true, nil
			}
		} else if len(pids) == 0 {
			return true, nil
		}
		if !t.now().Before(end) {
			return false, nil
		}
		if err := t.sleep(ctx, t.interval()); err != nil {
			return false, fmt.Errorf("%w: %s: %w", service.ErrTimeout, path, err)
		}
	}
}

// Start enables the task again when Stop disabled it, then runs it. A task
// that already runs is left alone.
func (t *Task) Start(_ context.Context, product string) (err error) {
	name, path, err := t.task(product)
	if err != nil {
		return err
	}
	r, err := t.open(name, path)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, r.close()) }()
	t.mu.Lock()
	wasDisabled := t.disabled[path]
	t.mu.Unlock()
	if wasDisabled {
		if err := r.setEnabled(true); err != nil {
			return mapError("enable", path, err)
		}
		t.mu.Lock()
		delete(t.disabled, path)
		t.mu.Unlock()
	}
	pids, err := r.instances()
	if err != nil {
		return mapError("list the instances of", path, err)
	}
	if len(pids) > 0 {
		return nil
	}
	at := t.now()
	if err := r.run(); err != nil {
		return mapError("run", path, err)
	}
	t.mu.Lock()
	t.started[path] = at
	t.mu.Unlock()
	return nil
}

// runClockSlack allows for Task Scheduler's last-run time being recorded
// to the second.
const runClockSlack = time.Second

// WaitHealthy waits until the task runs as an instance other than the one
// Stop saw, as that instance for the settle window; then runs
// Options.Probe. It fails at once with service.ErrUnhealthy, and the
// program's exit code, when no instance runs and the last run began at or
// after Start ran the task (0021-MADR D3).
func (t *Task) WaitHealthy(ctx context.Context, product string) error {
	name, path, err := t.task(product)
	if err != nil {
		return err
	}
	t.mu.Lock()
	previous, started := t.previous[path], t.started[path]
	t.mu.Unlock()
	probe := func(context.Context) (service.Health, error) {
		return t.health(name, path, started)
	}
	poll := t.o.Poll
	if previous.pid != 0 {
		poll.Previous = strconv.FormatUint(uint64(previous.pid), 10)
	}
	if err := service.PollHealthy(ctx, probe, poll); err != nil {
		return err
	}
	if t.o.Probe == nil {
		return nil
	}
	app := t.o.Poll
	app.Settle = -1
	return service.PollHealthy(ctx, t.o.Probe, app)
}

// health observes the task once.
func (t *Task) health(name, path string, started time.Time) (h service.Health, err error) {
	r, err := t.open(name, path)
	if err != nil {
		return service.Health{}, err
	}
	defer func() { err = errors.Join(err, r.close()) }()
	st, pids, err := status(r)
	if err != nil {
		return service.Health{}, mapError("query", path, err)
	}
	if len(pids) > 0 {
		return service.Health{
			Ready:    true,
			Instance: strconv.FormatUint(uint64(pids[0]), 10),
			Detail:   fmt.Sprintf("%s %s pid %d", path, stateName(st), pids[0]),
		}, nil
	}
	last, err := r.lastRun()
	if err != nil {
		return service.Health{}, mapError("query the last run of", path, err)
	}
	if !started.IsZero() && !last.Time.Before(started.Add(-runClockSlack)) {
		return service.Health{Failed: true, Detail: fmt.Sprintf("%s ran and exited with code %d", path, last.Result)}, nil
	}
	return service.Health{Detail: fmt.Sprintf("%s %s, no instance yet", path, stateName(st))}, nil
}
