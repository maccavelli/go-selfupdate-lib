package taskscheduler

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// Tests for Status (0032-MADR D4-A,
// 0032-PLAN-task-scheduler-registration-and-one-shot-handoff.md J5).

func TestStatus(t *testing.T) {
	ran := time.Unix(1_700_000_000, 0)
	for _, c := range []struct {
		name string
		edit func(ft *fakeTask)
		pid  uint32
		want Status
	}{
		{"running", func(ft *fakeTask) { ft.last = lastRun{Result: 0x41301, Time: ran} }, 4242,
			Status{Installed: true, Enabled: true, State: "running", Running: true, PIDs: []uint32{4242}, LastRunTime: ran, LastResult: 0x41301}},
		{"ready after a run", func(ft *fakeTask) { ft.last = lastRun{Result: 7, Time: ran} }, 0,
			Status{Installed: true, Enabled: true, State: "ready", LastRunTime: ran, LastResult: 7}},
		{"never run", func(ft *fakeTask) {
			ft.last = lastRun{Result: hasNotRun, Time: time.Date(1999, 11, 30, 0, 0, 0, 0, time.UTC)}
		}, 0,
			Status{Installed: true, Enabled: true, State: "ready", LastResult: hasNotRun}},
		{"disabled", func(ft *fakeTask) { ft.on, ft.st = false, stateDisabled }, 0,
			Status{State: "disabled", Installed: true}},
		{"no trigger that starts it", func(ft *fakeTask) { ft.trig = []trigger{{Type: triggerTime, Enabled: true}} }, 0,
			Status{Installed: true, State: "ready"}},
		{"listed while ready", func(ft *fakeTask) { ft.pids = []uint32{99} }, 0,
			Status{Installed: true, Enabled: true, State: "ready", Running: true, PIDs: []uint32{99}}},
	} {
		f := newFake()
		c.edit(f.add(`\demo`, c.pid))
		task, _ := testTask(t, f, Options{})
		got, err := task.Status(context.Background(), "demo")
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: Status = %+v, %v; want %+v", c.name, got, err, c.want)
		}
	}
}

func TestStatusMissingAndErrors(t *testing.T) {
	ctx := context.Background()
	task, _ := testTask(t, newFake(), Options{})
	if got, err := task.Status(ctx, "demo"); err != nil || !reflect.DeepEqual(got, Status{}) {
		t.Fatalf("missing: %+v, %v; want a zero Status", got, err)
	}
	for _, method := range []string{"state", "enabled", "instances", "triggers", "lastRun"} {
		f := newFake()
		f.add(`\demo`, 0).readErr = map[string]error{method: hrAccessDenied}
		task, _ := testTask(t, f, Options{})
		got, err := task.Status(ctx, "demo")
		if !errors.Is(err, service.ErrPermission) || !reflect.DeepEqual(got, Status{}) {
			t.Errorf("%s fails: %+v, %v; want a zero Status and ErrPermission", method, got, err)
		}
	}
	f := newFake()
	f.openErr = hrAccessDenied
	task, _ = testTask(t, f, Options{})
	if _, err := task.Status(ctx, "demo"); !errors.Is(err, service.ErrPermission) {
		t.Fatalf("open denied: %v, want ErrPermission", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := task.Status(cancelled, "demo"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled: %v", err)
	}
}

func TestStatusJSON(t *testing.T) {
	ran := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	b, err := json.Marshal(Status{Installed: true, Enabled: true, State: "running", Running: true, PIDs: []uint32{4242}, LastRunTime: ran, LastResult: 0})
	want := `{"installed":true,"enabled":true,"state":"running","running":true,"pids":[4242],"last_run_time":"2026-10-23T12:00:00Z","last_result":0}`
	if err != nil || string(b) != want {
		t.Fatalf("%s, %v; want %s", b, err, want)
	}
	b, err = json.Marshal(Status{})
	if want := `{"installed":false,"enabled":false,"state":"","running":false,"pids":null,"last_result":0}`; err != nil || string(b) != want {
		t.Fatalf("zero: %s, %v; want %s", b, err, want)
	}
	var back Status
	if err := json.Unmarshal([]byte(`{"installed":true,"state":"ready","pids":[1,2],"last_result":7}`), &back); err != nil ||
		!reflect.DeepEqual(back, Status{Installed: true, State: "ready", PIDs: []uint32{1, 2}, LastResult: 7}) {
		t.Fatalf("round trip %+v, %v", back, err)
	}
}
