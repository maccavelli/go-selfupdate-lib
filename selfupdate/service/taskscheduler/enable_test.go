package taskscheduler

import (
	"context"
	"errors"
	"slices"
	"testing"
	"testing/synctest"

	"github.com/maccavelli/go-selfupdate-lib/internal/testproc"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// Tests for Enable and Disable
// (docs/decisions/0034-MADR-export-task-scheduler-enable-and-disable.md
// D1-A to D4-A, 0034-PLAN L2).

// TestEnableDisableWrites: each writes the switch once, and only when it
// differs (0034-MADR Amendment A1, P3).
func TestEnableDisableWrites(t *testing.T) {
	testproc.CheckNoLeak(t)
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		for _, c := range []struct {
			name string
			on   bool
			call func(*Task) error
			want []string
			end  bool
		}{
			{"enable a disabled task", false, func(task *Task) error { return task.Enable(ctx, "demo") }, []string{"enable true"}, true},
			{"disable an enabled task", true, func(task *Task) error { return task.Disable(ctx, "demo") }, []string{"enable false"}, false},
			{"enable an enabled task", true, func(task *Task) error { return task.Enable(ctx, "demo") }, nil, true},
			{"disable a disabled task", false, func(task *Task) error { return task.Disable(ctx, "demo") }, nil, false},
		} {
			f := newFake()
			ft := f.add(`\demo`, 0)
			ft.on = c.on
			task := testTask(t, f, Options{})
			if err := c.call(task); err != nil {
				t.Errorf("%s: %v", c.name, err)
			}
			if got := f.verbs(); !slices.Equal(got, c.want) {
				t.Errorf("%s: calls %q, want %q", c.name, got, c.want)
			}
			if ft.on != c.end {
				t.Errorf("%s: switch %t, want %t", c.name, ft.on, c.end)
			}
		}
	})
}

// TestStartAfterStopElsewhere: a Stop made by another Task, as in an
// earlier process, leaves the task disabled; Start fails until Enable
// (issue #8; 0034-MADR D2-A).
func TestStartAfterStopElsewhere(t *testing.T) {
	testproc.CheckNoLeak(t)
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		f := newFake()
		f.add(`\demo`, 4242)
		first := testTask(t, f, Options{})
		if err := first.Stop(ctx, "demo"); err != nil {
			t.Fatal(err)
		}
		second := testTask(t, f, Options{})
		if err := second.Start(ctx, "demo"); !errors.Is(err, ErrTaskDisabled) {
			t.Fatalf("Start in another Task after Stop: %v, want ErrTaskDisabled", err)
		}
		if err := second.Enable(ctx, "demo"); err != nil {
			t.Fatal(err)
		}
		if err := second.Start(ctx, "demo"); err != nil {
			t.Fatalf("Start after Enable: %v", err)
		}
		if v := f.verbs(); v[len(v)-1] != "run" {
			t.Fatalf("calls %q: the task was not run", v)
		}
	})
}

// TestSwitchClearsStopRecord: after Enable or Disable, Start does not
// change the switch Stop left (0034-MADR D3-A).
func TestSwitchClearsStopRecord(t *testing.T) {
	testproc.CheckNoLeak(t)
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		for _, c := range []struct {
			name  string
			calls func(*Task) error
		}{
			{"Stop, Disable, Start", func(task *Task) error { return task.Disable(ctx, "demo") }},
			{"Stop, Enable, Disable, Start", func(task *Task) error {
				return errors.Join(task.Enable(ctx, "demo"), task.Disable(ctx, "demo"))
			}},
		} {
			f := newFake()
			f.add(`\demo`, 4242)
			task := testTask(t, f, Options{})
			if err := task.Stop(ctx, "demo"); err != nil {
				t.Fatal(err)
			}
			if err := c.calls(task); err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
			before := len(f.verbs())
			if err := task.Start(ctx, "demo"); !errors.Is(err, ErrTaskDisabled) {
				t.Errorf("%s: Start = %v, want ErrTaskDisabled", c.name, err)
			}
			if after := f.verbs()[before:]; slices.Contains(after, "enable true") {
				t.Errorf("%s: Start enabled the task: %q", c.name, after)
			}
		}
	})
}

// TestDisableLeavesProgramRunning: Disable does not end a running
// instance (0034-MADR D4-A).
func TestDisableLeavesProgramRunning(t *testing.T) {
	testproc.CheckNoLeak(t)
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		f := newFake()
		f.add(`\demo`, 4242)
		task := testTask(t, f, Options{})
		if err := task.Disable(ctx, "demo"); err != nil {
			t.Fatal(err)
		}
		if v := f.verbs(); !slices.Equal(v, []string{"enable false"}) {
			t.Fatalf("calls %q, want only the disable", v)
		}
		s, err := task.Status(ctx, "demo")
		if err != nil || !s.Running || !s.Disabled || s.State != "running" {
			t.Fatalf("Status %+v, %v; want running and disabled", s, err)
		}
	})
}

// TestEnableDisableErrors: the typed errors, and a done context before any
// call.
func TestEnableDisableErrors(t *testing.T) {
	testproc.CheckNoLeak(t)
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		task := testTask(t, newFake(), Options{})
		for name, call := range map[string]func(context.Context) error{
			"Enable":  func(c context.Context) error { return task.Enable(c, "demo") },
			"Disable": func(c context.Context) error { return task.Disable(c, "demo") },
		} {
			if err := call(ctx); !errors.Is(err, service.ErrNotInstalled) {
				t.Errorf("%s of a missing task: %v, want ErrNotInstalled", name, err)
			}
		}
		f := newFake()
		ft := f.add(`\demo`, 0)
		ft.on = false
		ft.setErr = hrAccessDenied
		task = testTask(t, f, Options{})
		if err := task.Enable(ctx, "demo"); !errors.Is(err, service.ErrPermission) {
			t.Errorf("Enable denied: %v, want ErrPermission", err)
		}
		f = newFake()
		f.add(`\demo`, 0).readErr = map[string]error{"enabled": hrAccessDenied}
		task = testTask(t, f, Options{})
		if err := task.Disable(ctx, "demo"); !errors.Is(err, service.ErrPermission) {
			t.Errorf("Disable, the read denied: %v, want ErrPermission", err)
		}
		f = newFake()
		ft = f.add(`\demo`, 0)
		task = testTask(t, f, Options{})
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		if err := task.Disable(cancelled, "demo"); !errors.Is(err, context.Canceled) {
			t.Errorf("Disable, cancelled: %v", err)
		}
		if v := f.verbs(); len(v) != 0 || !ft.on {
			t.Errorf("a cancelled Disable changed the task: calls %q, switch %t", v, ft.on)
		}
	})
}
