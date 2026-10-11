// Package taskscheduler is the reference Windows Task Scheduler lifecycle
// for a managed update
// (docs/decisions/0021-MADR-task-scheduler-lifecycle-and-windowless-children.md):
// a selfupdate.Lifecycle, selfupdate.EnabledLifecycle, selfupdate.Reconciler
// and service.Detacher for one registered task, such as a per-user task
// that starts a program at logon and needs no administrator.
//
// It calls Task Scheduler through its COM API, so states are numeric and
// definitions are UTF-16, and it never parses a tool's output. It manages a
// task the program's setup registered: it never creates, renders or deletes
// one. A task has no polite stop of its own, so Stop disables the task
// first, so that a repeating trigger cannot start it mid-update, and then
// asks Options.RequestStop, when set, before ending the task's process. It
// compiles on every OS; New returns service.ErrUnsupported off Windows.
package taskscheduler

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

// DefaultStopGrace is how long Stop waits, by default, for the program to
// exit after Options.RequestStop before ending it.
const DefaultStopGrace = 10 * time.Second

// ErrTaskDisabled is returned, wrapped, when the task is disabled where the
// update needs it to run: Task Scheduler refuses to run a disabled task
// (SCHED_E_TASK_DISABLED), so Stop refuses a disabled task whose program
// runs before it changes anything, and Start returns it for a disabled task
// that Stop did not disable
// (docs/decisions/0022-MADR-documentation-pass-after-v1-14-0.md Amendment
// A1).
var ErrTaskDisabled = errors.New("selfupdate: taskscheduler: the task is disabled, and Task Scheduler does not run a disabled task")

// Options configure a Task.
type Options struct {
	// Name is the task's name, without its folder. Empty means the
	// product, except for the handoff: Inside, and so Detach through
	// service.HandOffIfInside, needs it set.
	Name string
	// Folder is the task's folder, from the root: `\` or `\Vendor\Product`.
	// Empty means the root folder, `\`.
	Folder string
	// Probe, when set, is an application readiness check WaitHealthy runs
	// once Task Scheduler reports a new instance running.
	Probe service.HealthProbe
	// Poll bounds stop and health waits. Once Stop ends the task, its wait
	// runs to Poll's timeout whatever the caller's context does.
	Poll service.PollOptions
	// RewritePath lets Reconcile point the task's Exec action at a binary
	// that moved. Without it, a task running another binary is an error.
	// Only a task whose principal logs on with an interactive token is
	// rewritten (0021-MADR Amendment A1, Q6).
	RewritePath bool
	// RequestStop, when set, asks the task's program to exit on its own
	// before Stop ends it, such as by a shutdown request on its admin
	// socket. pid is the running instance's process. Stop then waits up to
	// StopGrace for it to exit. An error from RequestStop ends the grace at
	// once; it is part of Stop's error only when ending the task fails too
	// (0021-MADR D4).
	RequestStop func(ctx context.Context, pid uint32) error
	// StopGrace bounds the wait after RequestStop. Zero means
	// DefaultStopGrace.
	StopGrace time.Duration
}

// instance is one running instance of a task: its process and that
// process's creation time, which tells a reused process ID apart.
type instance struct {
	pid  uint32
	born uint64
}

// Task manages one registered task.
type Task struct {
	o     Options
	s     scheduler
	now   func() time.Time
	sleep func(context.Context, time.Duration) error
	self  func() uint32

	mu sync.Mutex
	// previous is, per task, the instance Stop saw, which WaitHealthy will
	// not count.
	previous map[string]instance
	// disabled is, per task, whether Stop disabled it, for Start to enable
	// it again.
	disabled map[string]bool
	// started is, per task, when Start ran it, so WaitHealthy can tell a run
	// that ended from one that has not begun.
	started map[string]time.Time
}

var (
	_ selfupdate.Lifecycle        = (*Task)(nil)
	_ selfupdate.EnabledLifecycle = (*Task)(nil)
	_ selfupdate.Reconciler       = (*Task)(nil)
	_ service.Detacher            = (*Task)(nil)
)

// New returns a Task for o. Off Windows it returns service.ErrUnsupported.
func New(o Options) (*Task, error) {
	return newTask(o, runtime.GOOS, systemScheduler())
}

func newTask(o Options, goos string, s scheduler) (*Task, error) {
	if goos != "windows" {
		return nil, fmt.Errorf("%w: Task Scheduler runs on Windows, not %s", service.ErrUnsupported, goos)
	}
	if s == nil {
		return nil, fmt.Errorf("%w: no Task Scheduler transport in this build", service.ErrUnsupported)
	}
	if o.Name != "" {
		if err := validName(o.Name); err != nil {
			return nil, err
		}
	}
	if o.Folder == "" {
		o.Folder = `\`
	}
	if err := validFolder(o.Folder); err != nil {
		return nil, err
	}
	if o.StopGrace < 0 {
		return nil, fmt.Errorf("selfupdate: taskscheduler: Options.StopGrace %s is negative", o.StopGrace)
	}
	if o.StopGrace == 0 {
		o.StopGrace = DefaultStopGrace
	}
	if err := o.Poll.Validate(); err != nil {
		return nil, fmt.Errorf("selfupdate: taskscheduler: %w", err)
	}
	return &Task{
		o: o, s: s, now: time.Now, sleep: sleepCtx, self: currentPID,
		previous: map[string]instance{}, disabled: map[string]bool{}, started: map[string]time.Time{},
	}, nil
}

// validName refuses an empty task name, or one holding a folder separator.
func validName(name string) error {
	if name == "" || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("selfupdate: taskscheduler: invalid task name %q", name)
	}
	return nil
}

// validFolder accepts `\` or a path of non-empty elements under it, such
// as `\Vendor\Product`.
func validFolder(folder string) error {
	if folder == `\` {
		return nil
	}
	if !strings.HasPrefix(folder, `\`) || strings.Contains(folder, "/") || strings.HasSuffix(folder, `\`) || strings.Contains(folder, `\\`) {
		return fmt.Errorf("selfupdate: taskscheduler: invalid task folder %q: want `\\` or a path such as `\\Vendor\\Product`", folder)
	}
	return nil
}

// task is the task's name for product, and its full path, which keys the
// per-task state and names the task in errors.
func (t *Task) task(product string) (name, path string, err error) {
	name = t.o.Name
	if name == "" {
		if err := validName(product); err != nil {
			return "", "", err
		}
		name = product
	}
	if t.o.Folder == `\` {
		return name, `\` + name, nil
	}
	return name, t.o.Folder + `\` + name, nil
}

// open opens the task, mapping Task Scheduler's errors to the typed ones.
func (t *Task) open(name, path string) (registered, error) {
	r, err := t.s.open(t.o.Folder, name)
	if err != nil {
		return nil, mapError("open", path, err)
	}
	return r, nil
}

// mapError wraps a Task Scheduler error with its step, as a typed error
// where one fits (0021-MADR D2, measured in Amendment A1 probe 5).
func mapError(step, path string, err error) error {
	e := fmt.Errorf("selfupdate: taskscheduler: %s %s: %w", step, path, err)
	switch {
	case errors.Is(err, hrFileNotFound):
		return fmt.Errorf("%w: %w", service.ErrNotInstalled, e)
	case errors.Is(err, hrAccessDenied):
		return fmt.Errorf("%w: %w", service.ErrPermission, e)
	case errors.Is(err, hrTaskDisabled):
		return fmt.Errorf("%w: %w", ErrTaskDisabled, e)
	}
	return e
}

// deadline bounds ctx by Options.Poll's timeout.
func (t *Task) deadline(ctx context.Context) (context.Context, context.CancelFunc) {
	timeout := t.o.Poll.Timeout
	if timeout <= 0 {
		timeout = service.DefaultPollTimeout
	}
	return context.WithTimeout(ctx, timeout)
}

// interval is Options.Poll's interval, or its default.
func (t *Task) interval() time.Duration {
	if t.o.Poll.Interval > 0 {
		return t.o.Poll.Interval
	}
	return service.DefaultPollInterval
}

func stateName(state int32) string {
	switch state {
	case stateUnknown:
		return "unknown"
	case stateDisabled:
		return "disabled"
	case stateQueued:
		return "queued"
	case stateReady:
		return "ready"
	case stateRunning:
		return "running"
	}
	return fmt.Sprintf("state %d", state)
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
