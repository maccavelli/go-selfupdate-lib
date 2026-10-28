package service

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/internal/procfs"
	"github.com/maccavelli/go-selfupdate-lib/internal/testproc"
)

// Tests for WaitHealthy, Reconcile, Restore and Detach
// (docs/decisions/0035-PLAN-serve-supervisors-through-a-supervised-adapter.md
// M5), over scripted observations. The WaitHealthy tests run in synctest
// bubbles on the production clock, so the restart window counts bubble
// time, however slow the runner. That replaces the stepping clock
// 0035-PLAN-serve-supervisors-through-a-supervised-adapter.md D4 chose
// (docs/decisions/0038-MADR-pinned-plans-lifecycle-conformance-and-bubble-tests.md
// D3-A).

// healthProcs adds the instances 101@107 and 102@207 to table().
func healthProcs() fakeProcs {
	p := table()
	p[101] = procfs.Stat{Parent: 10, StartTime: 107, State: 'S'}
	p[102] = procfs.Stat{Parent: 10, StartTime: 207, State: 'S'}
	return p
}

// observations returns each one in turn, then the last one forever.
func observations(seq ...SupervisedState) func(context.Context) (SupervisedState, error) {
	var mu sync.Mutex
	i := 0
	return func(context.Context) (SupervisedState, error) {
		mu.Lock()
		defer mu.Unlock()
		st := seq[min(i, len(seq)-1)]
		i++
		return st, nil
	}
}

func upAs(pid int) SupervisedState {
	return SupervisedState{Installed: true, Supervised: true, Up: true, PID: pid, WantUp: true, NormallyUp: true}
}

var restarting = SupervisedState{Installed: true, Supervised: true, WantUp: true, Restarting: true, NormallyUp: true}

func boolPtr(b bool) *bool { return &b }

// healthOver is a Supervised over seq, after a Stop that recorded 100@7,
// polling every 100 ms with a 3 s settle window and the default 60 s
// bound. Call it inside the bubble.
func healthOver(t *testing.T, probe HealthProbe, seq ...SupervisedState) *Supervised {
	t.Helper()
	s := fakeSupervised(t, SupervisedOptions{
		Name: "demo", State: observations(seq...), Probe: probe,
		Poll: PollOptions{Interval: 100 * time.Millisecond, Settle: 3 * time.Second},
	}, healthProcs(), 400)
	s.recorded = instance{pid: 100, start: 7}
	return s
}

func TestSupervisedWaitHealthy(t *testing.T) {
	testproc.CheckNoLeak(t)
	readyFalse, readyTrue := upAs(101), upAs(101)
	readyFalse.Ready, readyTrue.Ready = boolPtr(false), boolPtr(true)
	failed := upAs(101)
	failed.Up, failed.PID, failed.Failed = false, 0, true
	for name, c := range map[string]struct {
		seq  []SupervisedState
		want error
		// took is how long the wait takes, in bubble time.
		took time.Duration
	}{
		"a new instance settles":                   {[]SupervisedState{upAs(101)}, nil, 3 * time.Second},
		"ready where the supervisor reports it":    {[]SupervisedState{readyTrue}, nil, 3 * time.Second},
		"one restart before it comes up":           {[]SupervisedState{restarting, upAs(101)}, nil, 3100 * time.Millisecond},
		"failed":                                   {[]SupervisedState{upAs(101), failed}, ErrUnhealthy, 100 * time.Millisecond},
		"restarted during the wait":                {[]SupervisedState{upAs(101), upAs(101), upAs(102)}, ErrUnhealthy, 200 * time.Millisecond},
		"restarting after it was up":               {[]SupervisedState{upAs(101), restarting}, ErrUnhealthy, 100 * time.Millisecond},
		"several readings of one restart":          {[]SupervisedState{restarting, restarting, restarting, restarting, upAs(101)}, nil, 3400 * time.Millisecond},
		"restarting longer than the settle window": {[]SupervisedState{restarting}, ErrUnhealthy, 3100 * time.Millisecond},
		"not ready, where the supervisor says":     {[]SupervisedState{readyFalse}, ErrTimeout, DefaultPollTimeout},
		"only the instance the stop recorded":      {[]SupervisedState{upAs(100)}, ErrTimeout, DefaultPollTimeout},
	} {
		synctest.Test(t, func(t *testing.T) {
			start := time.Now()
			err := healthOver(t, nil, c.seq...).WaitHealthy(context.Background(), "demo")
			if c.want == nil && err != nil || c.want != nil && !errors.Is(err, c.want) {
				t.Errorf("%s: %v, want %v", name, err, c.want)
			}
			if took := time.Since(start); took != c.took {
				t.Errorf("%s: took %s, want %s", name, took, c.took)
			}
		})
	}
}

func TestSupervisedWaitHealthyProbe(t *testing.T) {
	testproc.CheckNoLeak(t)
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		calls := 0
		ok := func(context.Context) (Health, error) { calls++; return Health{Ready: true}, nil }
		if err := healthOver(t, ok, upAs(101)).WaitHealthy(ctx, "demo"); err != nil || calls == 0 {
			t.Fatalf("a passing application probe: %v, %d calls", err, calls)
		}
		bad := func(context.Context) (Health, error) {
			return Health{Failed: true, Detail: "admin socket refused"}, nil
		}
		if err := healthOver(t, bad, upAs(101)).WaitHealthy(ctx, "demo"); !errors.Is(err, ErrUnhealthy) {
			t.Fatalf("a failing application probe: %v, want ErrUnhealthy", err)
		}
		unused := false
		never := func(context.Context) (Health, error) { unused = true; return Health{Ready: true}, nil }
		if err := healthOver(t, never, upAs(101), restarting).WaitHealthy(ctx, "demo"); !errors.Is(err, ErrUnhealthy) || unused {
			t.Fatalf("the application probe ran after a failed wait: %v, %t", err, unused)
		}
	})
}

func TestSupervisedReconcile(t *testing.T) {
	testproc.CheckNoLeak(t)
	ctx := context.Background()
	s := fakeSupervised(t, SupervisedOptions{State: states(up100, nil)}, table(), 400)
	r, err := s.Reconcile(ctx, "demo", "/opt/demo/demo")
	if err != nil || r.Changed || r.Warnings == "" {
		t.Fatalf("nothing recorded: %+v, %v; want no change and a warning", r, err)
	}
	for _, ran := range []string{"/opt/demo/demo", "/opt/demo/demo (deleted)", "/opt/demo/./demo"} {
		s.binary = ran
		if r, err := s.Reconcile(ctx, "demo", "/opt/demo/demo"); err != nil || r.Changed {
			t.Errorf("ran %q: %+v, %v; want a match", ran, r, err)
		}
	}
	s.binary = "/opt/other/demo"
	if _, err := s.Reconcile(ctx, "demo", "/opt/demo/demo"); err == nil || !strings.Contains(err.Error(), "NewExecReconciler") {
		t.Fatalf("another binary: %v, want an error naming NewExecReconciler", err)
	}
	if err := s.Restore(ctx, "demo", r); err != nil {
		t.Fatalf("Restore: %v", err)
	}
}

func TestSupervisedDetachCheck(t *testing.T) {
	testproc.CheckNoLeak(t)
	ctx := context.Background()
	refused := errors.New("cgroup cleanup is on")
	s := fakeSupervised(t, SupervisedOptions{
		State:       states(up100, nil),
		DetachCheck: func(context.Context) error { return refused },
	}, table(), 400)
	// A relative executable would make DetachProcess fail on its own; the
	// check must refuse first.
	_, err := s.Detach(ctx, HandOff{Executable: "demo"})
	if !errors.Is(err, ErrUnsupported) || !errors.Is(err, refused) {
		t.Fatalf("Detach with a refusing check: %v, want ErrUnsupported and the check's error", err)
	}
	s = fakeSupervised(t, SupervisedOptions{State: states(up100, nil)}, table(), 400)
	if _, err := s.Detach(ctx, HandOff{Executable: "demo"}); err == nil || errors.Is(err, ErrUnsupported) {
		t.Fatalf("Detach with no check: %v, want DetachProcess's own error", err)
	}
}

// TestSupervisedDetachHop: the run starts through the hop, so it does not
// descend from the service once the hop has exited (0035-PLAN Deviation
// D2), and the caller's environment is not changed.
func TestSupervisedDetachHop(t *testing.T) {
	testproc.CheckNoLeak(t)
	s := fakeSupervised(t, SupervisedOptions{State: states(up100, nil)}, table(), 400)
	var got HandOff
	s.detach = func(_ context.Context, spec HandOff) (Detached, error) {
		got = spec
		return Detached{ID: spec.ID, Where: "process 4242"}, nil
	}
	env := []string{"A=1"}
	det, err := s.Detach(context.Background(), HandOff{ID: "x", Executable: "/opt/demo/demo", Env: env})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(got.Env, EnvHandOffHop+"=1") || !strings.Contains(det.Where, "through the hop") {
		t.Fatalf("spec env %q, where %q; want the hop", got.Env, det.Where)
	}
	if len(env) != 1 {
		t.Fatalf("the caller's environment slice was changed: %q", env)
	}
}
