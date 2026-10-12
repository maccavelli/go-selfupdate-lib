package taskscheduler

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// TestNotInstalled: the probes and Inside report a missing task as absent;
// every other method returns service.ErrNotInstalled.
func TestNotInstalled(t *testing.T) {
	ctx := context.Background()
	task, _ := testTask(t, newFake(), Options{Poll: service.PollOptions{Timeout: 20 * time.Millisecond}})
	if ok, err := task.Installed(ctx, "demo"); ok || err != nil {
		t.Errorf("Installed: %t, %v", ok, err)
	}
	if ok, err := task.Running(ctx, "demo"); ok || err != nil {
		t.Errorf("Running: %t, %v", ok, err)
	}
	if ok, err := task.Inside(ctx); ok || err != nil {
		t.Errorf("Inside: %t, %v", ok, err)
	}
	if _, err := task.Enabled(ctx, "demo"); !errors.Is(err, service.ErrNotInstalled) {
		t.Errorf("Enabled: %v", err)
	}
	if err := task.Stop(ctx, "demo"); !errors.Is(err, service.ErrNotInstalled) {
		t.Errorf("Stop: %v", err)
	}
	if err := task.Start(ctx, "demo"); !errors.Is(err, service.ErrNotInstalled) {
		t.Errorf("Start: %v", err)
	}
	if err := task.WaitHealthy(ctx, "demo"); !errors.Is(err, service.ErrNotInstalled) {
		t.Errorf("WaitHealthy: %v", err)
	}
	if _, err := task.Reconcile(ctx, "demo", demoExe); !errors.Is(err, service.ErrNotInstalled) {
		t.Errorf("Reconcile: %v", err)
	}
}

// TestPermission: a refused open is service.ErrPermission.
func TestPermission(t *testing.T) {
	f := newFake()
	f.openErr = hrAccessDenied
	task, _ := testTask(t, f, Options{})
	if _, err := task.Installed(context.Background(), "demo"); !errors.Is(err, service.ErrPermission) {
		t.Fatalf("Installed: %v", err)
	}
}

// TestRunningByState: Running is TASK_STATE_RUNNING, or any instance.
func TestRunningByState(t *testing.T) {
	for _, c := range []struct {
		st   int32
		pids []uint32
		want bool
	}{
		{stateUnknown, nil, false},
		{stateDisabled, nil, false},
		{stateQueued, nil, false},
		{stateReady, nil, false},
		{stateRunning, nil, true},
		{stateReady, []uint32{4242}, true},
	} {
		f := newFake()
		ft := f.add(`\demo`, 0)
		ft.st, ft.pids = c.st, c.pids
		task, _ := testTask(t, f, Options{})
		if got, err := task.Running(context.Background(), "demo"); err != nil || got != c.want {
			t.Errorf("%s, %d instances: %t, %v; want %t", stateName(c.st), len(c.pids), got, err, c.want)
		}
	}
}

// TestEnabledRule: an enabled task with an enabled logon, boot or
// repeating time trigger counts; nothing else does (0021-MADR D3).
func TestEnabledRule(t *testing.T) {
	for _, c := range []struct {
		name string
		on   bool
		trig []trigger
		want bool
	}{
		{"logon", true, []trigger{{Type: triggerLogon, Enabled: true}}, true},
		{"boot", true, []trigger{{Type: triggerBoot, Enabled: true}}, true},
		{"repeating time", true, []trigger{{Type: triggerTime, Enabled: true, Repeats: true}}, true},
		{"one-off time", true, []trigger{{Type: triggerTime, Enabled: true}}, false},
		{"disabled trigger", true, []trigger{{Type: triggerLogon}}, false},
		{"no trigger", true, nil, false},
		{"idle trigger", true, []trigger{{Type: 6, Enabled: true}}, false},
		{"disabled task", false, []trigger{{Type: triggerLogon, Enabled: true}}, false},
		{"one-off and logon", true, []trigger{{Type: triggerTime, Enabled: true}, {Type: triggerLogon, Enabled: true}}, true},
	} {
		f := newFake()
		ft := f.add(`\demo`, 0)
		ft.on, ft.trig = c.on, c.trig
		task, _ := testTask(t, f, Options{})
		if got, err := task.Enabled(context.Background(), "demo"); err != nil || got != c.want {
			t.Errorf("%s: %t, %v; want %t", c.name, got, err, c.want)
		}
	}
}

// TestStopOrder: Stop disables the task, asks the program to exit, ends
// the task, and waits; Start then enables it again and runs it.
func TestStopOrder(t *testing.T) {
	f := newFake()
	f.add(`\demo`, 4242).stopKeeps = true
	var asked []uint32
	task, _ := testTask(t, f, Options{
		StopGrace: 5 * time.Millisecond,
		RequestStop: func(_ context.Context, pid uint32) error {
			f.mu.Lock()
			f.record("request %d", pid)
			f.mu.Unlock()
			asked = append(asked, pid)
			return nil
		},
	})
	if err := task.Stop(context.Background(), "demo"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"enable false", "request 4242", "stop"}; !slices.Equal(f.verbs(), want) {
		t.Fatalf("Stop calls %q, want %q", f.verbs(), want)
	}
	if f.alive[4242] || len(f.tasks[`\demo`].pids) != 0 {
		t.Fatal("Stop returned with the instance still running")
	}
	if err := task.Start(context.Background(), "demo"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"enable false", "request 4242", "stop", "enable true", "run"}; !slices.Equal(f.verbs(), want) {
		t.Fatalf("Stop and Start calls %q, want %q", f.verbs(), want)
	}
}

// TestStopRequestHonoured: a program that exits within StopGrace is never
// ended by Task Scheduler.
func TestStopRequestHonoured(t *testing.T) {
	f := newFake()
	ft := f.add(`\demo`, 4242)
	task, clock := testTask(t, f, Options{
		StopGrace: time.Second,
		RequestStop: func(context.Context, uint32) error {
			f.mu.Lock()
			ft.exitIn = 3
			f.mu.Unlock()
			return nil
		},
	})
	start := clock.now()
	if err := task.Stop(context.Background(), "demo"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"enable false"}; !slices.Equal(f.verbs(), want) {
		t.Fatalf("calls %q, want %q: the program exited on its own", f.verbs(), want)
	}
	if waited := clock.now().Sub(start); waited >= time.Second {
		t.Fatalf("waited %s, the whole grace", waited)
	}
}

// TestStopGraceRunsOut: a program that does not exit within StopGrace is
// ended once the grace has passed.
func TestStopGraceRunsOut(t *testing.T) {
	f := newFake()
	f.add(`\demo`, 4242)
	task, clock := testTask(t, f, Options{
		StopGrace:   50 * time.Millisecond,
		RequestStop: func(context.Context, uint32) error { return nil },
	})
	start := clock.now()
	if err := task.Stop(context.Background(), "demo"); err != nil {
		t.Fatal(err)
	}
	if waited := clock.now().Sub(start); waited < 50*time.Millisecond {
		t.Fatalf("stopped after %s, before the grace ran out", waited)
	}
	if want := []string{"enable false", "stop"}; !slices.Equal(f.verbs(), want) {
		t.Fatalf("calls %q, want %q", f.verbs(), want)
	}
}

// TestStopRequestFails: a RequestStop error ends the grace at once. When
// ending the task then works, Stop succeeds; when it fails, both errors
// are reported (0021-MADR D4).
func TestStopRequestFails(t *testing.T) {
	refused := errors.New("admin socket refused")
	f := newFake()
	f.add(`\demo`, 4242)
	o := Options{StopGrace: 1500 * time.Millisecond, RequestStop: func(context.Context, uint32) error { return refused }}
	task, clock := testTask(t, f, o)
	start := clock.now()
	if err := task.Stop(context.Background(), "demo"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if waited := clock.now().Sub(start); waited >= o.StopGrace {
		t.Fatalf("waited the grace after a failed request: %s", waited)
	}

	f = newFake()
	f.add(`\demo`, 4242).stopErr = hresult(0x80004005)
	task, _ = testTask(t, f, o)
	err := task.Stop(context.Background(), "demo")
	if !errors.Is(err, refused) || !strings.Contains(err.Error(), "HRESULT 0x80004005") {
		t.Fatalf("Stop: %v, want both errors", err)
	}
}

// TestStopFailureEnablesAgain: a Stop that fails after it disabled the task,
// with the program still running, enables the task again, so its triggers
// still start the program; one whose program has gone leaves it disabled,
// for Start (0024-MADR D4-A).
func TestStopFailureEnablesAgain(t *testing.T) {
	f := newFake()
	ft := f.add(`\demo`, 4242)
	ft.stopErr = hresult(0x80004005)
	task, _ := testTask(t, f, Options{})
	if err := task.Stop(context.Background(), "demo"); err == nil {
		t.Fatal("Stop: nil, want the failed end")
	}
	if want := []string{"enable false", "stop", "enable true"}; !slices.Equal(f.verbs(), want) || !ft.on {
		t.Fatalf("calls %q, enabled %t; want %q and the task enabled", f.verbs(), ft.on, want)
	}
	if task.disabled[`\demo`] {
		t.Fatal("Stop still records that it disabled the task")
	}

	f = newFake()
	ft = f.add(`\demo`, 4242)
	ft.stopErr = hresult(0x80004005)
	f.alive[4242] = false
	task, _ = testTask(t, f, Options{})
	if err := task.Stop(context.Background(), "demo"); err == nil {
		t.Fatal("Stop: nil, want the failed end")
	}
	if want := []string{"enable false", "stop"}; !slices.Equal(f.verbs(), want) || ft.on {
		t.Fatalf("calls %q, enabled %t; want %q and the task disabled, its program gone", f.verbs(), ft.on, want)
	}
}

// TestStopWithoutRequest: without RequestStop, Stop ends the task at once,
// after disabling it, and Start enables it again and runs it.
func TestStopWithoutRequest(t *testing.T) {
	f := newFake()
	ft := f.add(`\demo`, 4242)
	task, _ := testTask(t, f, Options{})
	if err := task.Stop(context.Background(), "demo"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"enable false", "stop"}; !slices.Equal(f.verbs(), want) {
		t.Fatalf("calls %q, want %q", f.verbs(), want)
	}
	err := task.Start(context.Background(), "demo")
	if want := []string{"enable false", "stop", "enable true", "run"}; err != nil || !slices.Equal(f.verbs(), want) {
		t.Fatalf("Start: %v, calls %q, want %q", err, f.verbs(), want)
	}
	if !ft.on {
		t.Fatal("Start left disabled the task Stop disabled")
	}
}

// TestStopRefusesDisabled: a task the user disabled while its program runs
// is refused before anything changes, and the program keeps running:
// Task Scheduler would not run the task again after the update
// (0022-MADR Amendment A1).
func TestStopRefusesDisabled(t *testing.T) {
	f := newFake()
	ft := f.add(`\demo`, 4242)
	ft.on = false
	task, _ := testTask(t, f, Options{})
	err := task.Stop(context.Background(), "demo")
	if !errors.Is(err, ErrTaskDisabled) || !strings.Contains(err.Error(), "enable the task") {
		t.Fatalf("Stop: %v, want ErrTaskDisabled with the remedy", err)
	}
	if len(f.verbs()) != 0 || !f.alive[4242] || ft.on {
		t.Fatalf("calls %q, alive %t, enabled %t; want no call, the program running, the task disabled", f.verbs(), f.alive[4242], ft.on)
	}
}

// TestStopDisabledIdle: a disabled task with no program running is left
// as it is; the update starts nothing, since Enabled reports false.
func TestStopDisabledIdle(t *testing.T) {
	f := newFake()
	ft := f.add(`\demo`, 0)
	ft.on = false
	task, _ := testTask(t, f, Options{})
	if err := task.Stop(context.Background(), "demo"); err != nil {
		t.Fatal(err)
	}
	if len(f.verbs()) != 0 || ft.on {
		t.Fatalf("calls %q, enabled %t; want no call, the task disabled", f.verbs(), ft.on)
	}
}

// TestStartDisabled: Start on a task Stop did not disable does not enable
// it, and Task Scheduler's refusal is ErrTaskDisabled (0022-MADR
// Amendment A1).
func TestStartDisabled(t *testing.T) {
	f := newFake()
	ft := f.add(`\demo`, 0)
	ft.on = false
	task, _ := testTask(t, f, Options{})
	err := task.Start(context.Background(), "demo")
	if !errors.Is(err, ErrTaskDisabled) || !strings.Contains(err.Error(), "HRESULT 0x80041326") {
		t.Fatalf("Start: %v, want ErrTaskDisabled", err)
	}
	if want := []string{"run"}; !slices.Equal(f.verbs(), want) || ft.on {
		t.Fatalf("calls %q, enabled %t; want %q and the task disabled", f.verbs(), ft.on, want)
	}
}

// TestStopNotRunning: a task that is not running is still disabled, so its
// trigger cannot start it mid-update, and nothing is ended.
func TestStopNotRunning(t *testing.T) {
	f := newFake()
	f.add(`\demo`, 0)
	task, _ := testTask(t, f, Options{})
	if err := task.Stop(context.Background(), "demo"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"enable false"}; !slices.Equal(f.verbs(), want) {
		t.Fatalf("calls %q, want %q", f.verbs(), want)
	}
}

// TestStopFinishesAfterCancel: once the task is being stopped, a caller's
// context that ends does not cut the wait short; Stop waits until the
// instance has gone, then reports the cancellation (0015-MADR B3).
func TestStopFinishesAfterCancel(t *testing.T) {
	f := newFake()
	ft := f.add(`\demo`, 4242)
	ft.stopKeeps = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	task, _ := testTask(t, f, Options{RequestStop: func(context.Context, uint32) error {
		cancel()
		return errors.New("not listening")
	}})
	err := task.Stop(ctx, "demo")
	if !errors.Is(err, context.Canceled) || errors.Is(err, service.ErrTimeout) {
		t.Fatalf("Stop = %v, want the caller's cancellation alone", err)
	}
	if f.alive[4242] {
		t.Fatal("Stop returned with the instance running")
	}
}

// TestStopTimesOut: an instance that never goes is service.ErrTimeout once
// Poll's timeout has passed.
func TestStopTimesOut(t *testing.T) {
	f := newFake()
	ft := f.add(`\demo`, 4242)
	ft.stopKeeps, ft.exitIn = true, -1
	task, _ := testTask(t, f, Options{Poll: service.PollOptions{Timeout: 50 * time.Millisecond}})
	if err := task.Stop(context.Background(), "demo"); !errors.Is(err, service.ErrTimeout) {
		t.Fatalf("Stop: %v, want ErrTimeout", err)
	}
}

// TestStopInside: Stop from inside the instance is refused, before
// anything changes.
func TestStopInside(t *testing.T) {
	f := newFake()
	f.add(`\demo`, 6000)
	task, _ := testTask(t, f, Options{})
	if err := task.Stop(context.Background(), "demo"); !errors.Is(err, service.ErrInsideService) {
		t.Fatalf("Stop: %v, want ErrInsideService", err)
	}
	if len(f.verbs()) != 0 {
		t.Fatalf("calls %q, want none", f.verbs())
	}
}

// TestStartRunning: Start leaves a running task alone.
func TestStartRunning(t *testing.T) {
	f := newFake()
	f.add(`\demo`, 4242)
	task, _ := testTask(t, f, Options{})
	if err := task.Start(context.Background(), "demo"); err != nil || len(f.verbs()) != 0 {
		t.Fatalf("Start: %v, calls %q", err, f.verbs())
	}
}

// TestStopStaleInstance: Stop waits for the recorded process, not for Task
// Scheduler's list, which can still name it after it has exited
// (0021-MADR Amendment A4).
func TestStopStaleInstance(t *testing.T) {
	f := newFake()
	f.add(`\demo`, 4242).staleList = true
	task, _ := testTask(t, f, Options{Poll: service.PollOptions{Timeout: 50 * time.Millisecond}})
	if err := task.Stop(context.Background(), "demo"); err != nil {
		t.Fatalf("Stop: %v, want nil once the process has exited", err)
	}
}

// TestStartStaleInstance: after Stop, Start runs the task though Task
// Scheduler still lists the stopped instance, whose process has exited.
// A recorded instance whose process still runs is counted (0021-MADR
// Amendment A4, Q7).
func TestStartStaleInstance(t *testing.T) {
	f := newFake()
	f.add(`\demo`, 4242).staleList = true
	task, _ := testTask(t, f, Options{})
	if err := task.Stop(context.Background(), "demo"); err != nil {
		t.Fatal(err)
	}
	err := task.Start(context.Background(), "demo")
	if want := []string{"enable false", "stop", "enable true", "run"}; err != nil || !slices.Equal(f.verbs(), want) {
		t.Fatalf("Start after a stale listing: %v, calls %q, want %q", err, f.verbs(), want)
	}

	f = newFake()
	ft := f.add(`\demo`, 4242)
	ft.stopKeeps, ft.exitIn = true, -1
	task, _ = testTask(t, f, Options{Poll: service.PollOptions{Timeout: 50 * time.Millisecond}})
	if err := task.Stop(context.Background(), "demo"); !errors.Is(err, service.ErrTimeout) {
		t.Fatalf("Stop: %v, want ErrTimeout", err)
	}
	err = task.Start(context.Background(), "demo")
	if want := []string{"enable false", "stop", "enable true"}; err != nil || !slices.Equal(f.verbs(), want) {
		t.Fatalf("Start with the recorded process alive: %v, calls %q, want %q", err, f.verbs(), want)
	}
}

// TestWaitHealthyNewInstance: after Stop and Start, the new instance is
// healthy; the probe runs after it.
func TestWaitHealthyNewInstance(t *testing.T) {
	f := newFake()
	f.add(`\demo`, 4242)
	probed := false
	task, _ := testTask(t, f, Options{Probe: func(context.Context) (service.Health, error) {
		probed = true
		return service.Health{Ready: true}, nil
	}})
	ctx := context.Background()
	if err := task.Stop(ctx, "demo"); err != nil {
		t.Fatal(err)
	}
	if err := task.Start(ctx, "demo"); err != nil {
		t.Fatal(err)
	}
	if err := task.WaitHealthy(ctx, "demo"); err != nil || !probed {
		t.Fatalf("WaitHealthy: %v, probed %t", err, probed)
	}
}

// TestWaitHealthyOldInstance: the instance Stop saw never counts.
func TestWaitHealthyOldInstance(t *testing.T) {
	f := newFake()
	ft := f.add(`\demo`, 4242)
	ft.stopKeeps, ft.exitIn = true, -1 // stop does nothing
	task, _ := testTask(t, f, Options{Poll: service.PollOptions{Timeout: 50 * time.Millisecond}})
	ctx := context.Background()
	_ = task.Stop(ctx, "demo") // times out, leaving 4242 running
	if err := task.WaitHealthy(ctx, "demo"); !errors.Is(err, service.ErrTimeout) {
		t.Fatalf("WaitHealthy: %v, want ErrTimeout", err)
	}
}

// TestWaitHealthyFailedRun: a run that ended after Start fails the wait at
// once, with the program's exit code (Amendment A1, probe 3).
func TestWaitHealthyFailedRun(t *testing.T) {
	f := newFake()
	ft := f.add(`\demo`, 4242)
	code := int32(7)
	task, _ := testTask(t, f, Options{})
	ctx := context.Background()
	if err := task.Stop(ctx, "demo"); err != nil {
		t.Fatal(err)
	}
	ft.runExits = &code
	if err := task.Start(ctx, "demo"); err != nil {
		t.Fatal(err)
	}
	err := task.WaitHealthy(ctx, "demo")
	if !errors.Is(err, service.ErrUnhealthy) || !strings.Contains(err.Error(), "exited with code 7") {
		t.Fatalf("WaitHealthy: %v", err)
	}
}

// TestWaitHealthyBeforeRun: a last run from before Start is not a failure.
func TestWaitHealthyBeforeRun(t *testing.T) {
	f := newFake()
	ft := f.add(`\demo`, 0)
	ft.last = lastRun{Result: 1, Time: time.Unix(1_600_000_000, 0)}
	task, _ := testTask(t, f, Options{Poll: service.PollOptions{Timeout: 20 * time.Millisecond}})
	task.started[`\demo`] = time.Unix(1_700_000_000, 0)
	if err := task.WaitHealthy(context.Background(), "demo"); !errors.Is(err, service.ErrTimeout) {
		t.Fatalf("WaitHealthy: %v, want ErrTimeout", err)
	}
}
