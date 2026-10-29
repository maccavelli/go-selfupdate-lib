package taskscheduler

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/maccavelli/go-selfupdate-lib/internal/testproc"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service/servicetest"
)

// Task checked by servicetest over fakeSched, in synctest bubbles
// (docs/decisions/0038-PLAN-pinned-plans-lifecycle-conformance-and-bubble-tests.md
// Q11).

// labTask is the task the lab arranges.
const labTask = `\demo`

// fakeLab is a servicetest.Lab over fakeSched's task labTask. A SlowStop
// task's program exits a few observations after it is ended; a Stubborn
// one never does. Task Scheduler wants a task up while it is enabled: its
// triggers start it again. The fake's mu guards its fields.
type fakeLab struct {
	f *fakeSched
	// mark is how many calls the fake had recorded at Arrange, so a stop
	// since can be told.
	mark int
}

// Arrange implements servicetest.Lab. A StartHangs task cannot be
// arranged: Start runs the task without waiting for it.
func (l *fakeLab) Arrange(ctx context.Context, a servicetest.Arrangement) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f := l.f
	f.mu.Lock()
	defer f.mu.Unlock()
	if ft, ok := f.tasks[labTask]; ok {
		for _, pid := range ft.pids {
			f.alive[pid] = false
		}
		delete(f.tasks, labTask)
	}
	l.mark = len(f.calls)
	var pid uint32
	switch a.State {
	case servicetest.Absent:
		return nil
	case servicetest.Running, servicetest.SlowStop, servicetest.Stubborn:
		pid = f.nextPID
		f.nextPID++
	case servicetest.Stopped, servicetest.Crashing:
	default:
		return fmt.Errorf("taskscheduler fake lab: %s: %w", a.State, errors.ErrUnsupported)
	}
	ft := f.add(labTask, pid)
	ft.on = a.Enabled
	if !ft.on && pid == 0 {
		ft.st = stateDisabled
	}
	switch a.State {
	case servicetest.SlowStop:
		ft.stopKeeps = true
	case servicetest.Stubborn:
		ft.stopKeeps, ft.exitIn = true, -1
	case servicetest.Crashing:
		code := int32(1)
		ft.runExits = &code
	}
	return nil
}

// Observe implements servicetest.Lab. Like any read, it is time passing.
func (l *fakeLab) Observe(ctx context.Context) (servicetest.Observation, error) {
	if err := ctx.Err(); err != nil {
		return servicetest.Observation{}, err
	}
	f := l.f
	f.mu.Lock()
	defer f.mu.Unlock()
	f.observe()
	ft, ok := f.tasks[labTask]
	if !ok {
		return servicetest.Observation{Detail: "no task"}, nil
	}
	running := slices.ContainsFunc(ft.pids, func(pid uint32) bool { return f.alive[pid] })
	return servicetest.Observation{
		Installed: true,
		Running:   running,
		Stopping:  running && slices.Contains(f.calls[l.mark:], "stop"),
		Enabled:   enabledBy(ft.on, ft.trig),
		Wanted:    ft.on,
		Detail:    fmt.Sprintf("%s pids %v on=%t", stateName(ft.st), ft.pids, ft.on),
	}, nil
}

// TestTaskConformance: Task meets the Lifecycle contract over the fake.
// Stop is bounded by Poll's timeout, and a Stop that fails with the
// program running enables the task again (0024-MADR D4-A). Installed,
// Running, Enabled and Start ignore their context (Task's documentation);
// Start runs the task without waiting for it, so the start-hangs check is
// skipped.
func TestTaskConformance(t *testing.T) {
	testproc.CheckNoLeak(t)
	synctest.Test(t, func(t *testing.T) {
		f := newFake()
		task := testTask(t, f, Options{})
		var skipped []string
		err := servicetest.VerifyLifecycle(context.Background(), task, servicetest.LifecycleCase{
			Product: "demo", Lab: &fakeLab{f: f}, StopBound: service.DefaultPollTimeout,
			ContextFree:             servicetest.MethodInstalled | servicetest.MethodRunning | servicetest.MethodEnabled | servicetest.MethodStart,
			RestoresAfterFailedStop: true,
			Logf:                    func(format string, args ...any) { skipped = append(skipped, fmt.Sprintf(format, args...)) },
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(skipped) != 1 || !strings.Contains(skipped[0], "start hangs") {
			t.Fatalf("skipped %q; want only the start-hangs check", skipped)
		}
	})
}
