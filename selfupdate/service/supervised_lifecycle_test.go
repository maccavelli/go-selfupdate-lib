package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/internal/procfs"
	"github.com/maccavelli/go-selfupdate-lib/internal/testproc"
)

// Tests for Stop and Start
// (docs/decisions/0035-PLAN-serve-supervisors-through-a-supervised-adapter.md
// M4), over a fake supervisor, in synctest bubbles: Supervised keeps its
// production clock, and its context deadlines and its waits share the
// bubble's fake time
// (docs/decisions/0038-MADR-pinned-plans-lifecycle-conformance-and-bubble-tests.md
// D3-A).

// fakeSup models a supervisor and its one service. After Down, the
// service's process exits downAfter later, or never when downAfter is
// negative (a program that ignores TERM); Kill ends it at once. It moves
// when it is observed, by the bubble's clock, so the tests keep no clock
// of their own. Like a command run through ExecRunner, a call on an ended
// context fails and changes nothing (0035-MADR Amendment A3).
type fakeSup struct {
	mu         sync.Mutex
	calls      []string
	installed  bool
	supervised bool
	up, wantUp bool
	pid        int
	start      uint64
	procs      fakeProcs
	downAfter  time.Duration
	downAt     time.Time
	downing    bool
	// reuse, when set, gives the exited process's PID to an unrelated
	// process with another start time.
	reuse   bool
	upErr   error
	downErr error
	killErr error
	killNop bool
	killAt  time.Time
}

func newFakeSup() *fakeSup {
	return &fakeSup{installed: true, supervised: true, up: true, wantUp: true, pid: 100, start: 7, procs: table()}
}

func (f *fakeSup) record(call string) { f.calls = append(f.calls, call) }

// advance ends the process once downAfter has passed since Down; f.mu is
// held.
func (f *fakeSup) advance() {
	if f.downing && f.downAfter >= 0 && !time.Now().Before(f.downAt.Add(f.downAfter)) {
		f.exit()
	}
}

func (f *fakeSup) State(ctx context.Context) (SupervisedState, error) {
	if err := ctx.Err(); err != nil {
		return SupervisedState{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.advance()
	st := SupervisedState{Installed: f.installed, Supervised: f.supervised, Up: f.up, WantUp: f.wantUp, NormallyUp: true}
	if f.up {
		st.PID = f.pid
	}
	if !f.supervised {
		st.Supervised = false
		return st, fmt.Errorf("fake: %w", ErrNoSupervisor)
	}
	return st, nil
}

func (f *fakeSup) Up(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.advance()
	f.record("up")
	if f.upErr != nil {
		return f.upErr
	}
	f.wantUp, f.downing = true, false
	if !f.up {
		f.pid, f.start = f.pid+1, f.start+100
		f.procs[f.pid] = procfs.Stat{Parent: 10, StartTime: f.start, State: 'S'}
		f.up = true
	}
	return nil
}

func (f *fakeSup) Down(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("down")
	if f.downErr != nil {
		return f.downErr
	}
	f.wantUp, f.downing, f.downAt = false, f.up, time.Now()
	return nil
}

func (f *fakeSup) Kill(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("kill")
	f.killAt = time.Now()
	if !f.killNop {
		f.exit()
	}
	return f.killErr
}

// exit ends the process; f.mu is held.
func (f *fakeSup) exit() {
	delete(f.procs, f.pid)
	if f.reuse {
		f.procs[f.pid] = procfs.Stat{Parent: 1, StartTime: f.start + 1000, State: 'S'}
	}
	f.up, f.downing = false, false
}

func (f *fakeSup) read(pid int) (procfs.Stat, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.advance()
	return f.procs.read(pid)
}

func (f *fakeSup) history() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

// supervisedOver is a Supervised over f, as process self (400: unrelated),
// with a 4 s stop bound polled every 100 ms. Call it inside the bubble.
func supervisedOver(t *testing.T, f *fakeSup, kill bool, self int) *Supervised {
	t.Helper()
	o := SupervisedOptions{
		Name: "demo", State: f.State, Up: f.Up, Down: f.Down,
		Poll: PollOptions{Timeout: 4 * time.Second, Interval: 100 * time.Millisecond, Settle: -1},
	}
	if kill {
		o.Kill, o.KillAfter = f.Kill, time.Second
	}
	s, err := supervisedOn(o, goosLinux)
	if err != nil {
		t.Fatal(err)
	}
	s.readStat, s.self = f.read, func() int { return self }
	s.exe = func(pid int) (string, error) { return fmt.Sprintf("/opt/demo/demo-%d", pid), nil }
	return s
}

func TestSupervisedStop(t *testing.T) {
	testproc.CheckNoLeak(t)
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		f := newFakeSup()
		f.downAfter = 300 * time.Millisecond
		s := supervisedOver(t, f, false, 400)
		if err := s.Stop(context.Background(), "demo"); err != nil {
			t.Fatal(err)
		}
		if got := f.history(); !slices.Equal(got, []string{"down"}) {
			t.Fatalf("calls %q, want only the down", got)
		}
		if s.recorded != (instance{pid: 100, start: 7}) || s.binary != "/opt/demo/demo-100" {
			t.Fatalf("recorded %v, binary %q", s.recorded, s.binary)
		}
		if took := time.Since(start); took != 300*time.Millisecond {
			t.Fatalf("Stop took %s, want the 300 ms the process took", took)
		}
	})
}

func TestSupervisedStopRefusesFirst(t *testing.T) {
	testproc.CheckNoLeak(t)
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		f := newFakeSup()
		f.procs[300] = procfs.Stat{Parent: 100, StartTime: 9, State: 'S'}
		s := supervisedOver(t, f, false, 300)
		if err := s.Stop(ctx, "demo"); !errors.Is(err, ErrInsideService) {
			t.Fatalf("inside: %v, want ErrInsideService", err)
		}
		f2 := newFakeSup()
		f2.installed = false
		s2 := supervisedOver(t, f2, false, 400)
		if err := s2.Stop(ctx, "demo"); !errors.Is(err, ErrNotInstalled) {
			t.Fatalf("not installed: %v, want ErrNotInstalled", err)
		}
		for _, x := range []*fakeSup{f, f2} {
			if got := x.history(); len(got) != 0 {
				t.Fatalf("a refused stop called %q", got)
			}
		}
		// Down, not wanted up: nothing to do.
		f3 := newFakeSup()
		f3.up, f3.wantUp = false, false
		s3 := supervisedOver(t, f3, false, 400)
		if err := s3.Stop(ctx, "demo"); err != nil || len(f3.history()) != 0 {
			t.Fatalf("a stopped service: %v, calls %q", err, f3.history())
		}
	})
}

// TestSupervisedStopTimeout: a process that ignores the stop times out at
// the bound, and Up runs, so the supervisor wants it up again.
func TestSupervisedStopTimeout(t *testing.T) {
	testproc.CheckNoLeak(t)
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		f := newFakeSup()
		f.downAfter = -1
		s := supervisedOver(t, f, false, 400)
		err := s.Stop(context.Background(), "demo")
		if !errors.Is(err, ErrTimeout) {
			t.Fatalf("Stop: %v, want ErrTimeout", err)
		}
		if got := f.history(); !slices.Equal(got, []string{"down", "up"}) {
			t.Fatalf("calls %q, want down then up", got)
		}
		if took := time.Since(start); !f.wantUp || took != 4*time.Second {
			t.Fatalf("want up %t after %s; want true after the 4 s bound", f.wantUp, took)
		}
	})
}

// TestSupervisedStopRestoresInBubble: at the documented default bound, a
// stop that times out reports its last observation, and Up then runs on a
// context with time left, whether Down returns at once or spends half the
// bound (0035-MADR Amendment A3). The code before 0035-PLAN M11 fails it
// (0038-MADR F6, probe P1).
func TestSupervisedStopRestoresInBubble(t *testing.T) {
	testproc.CheckNoLeak(t)
	const timeout = DefaultPollTimeout
	for _, downFor := range []time.Duration{0, timeout / 2} {
		synctest.Test(t, func(t *testing.T) {
			f := newFakeSup()
			f.downAfter = -1
			down := func(ctx context.Context) error {
				time.Sleep(downFor)
				return f.Down(ctx)
			}
			o := SupervisedOptions{
				Name: "demo", State: f.State, Up: f.Up, Down: down,
				Poll: PollOptions{Timeout: timeout, Interval: time.Second, Settle: -1},
			}
			s, err := supervisedOn(o, goosLinux)
			if err != nil {
				t.Fatal(err)
			}
			s.readStat, s.self = f.read, func() int { return 400 }
			s.exe = func(pid int) (string, error) { return fmt.Sprintf("/opt/demo/demo-%d", pid), nil }
			err = s.Stop(context.Background(), "demo")
			if !errors.Is(err, ErrTimeout) || !strings.Contains(err.Error(), "still runs") {
				t.Fatalf("Down for %s: Stop: %v, want ErrTimeout with the last observation", downFor, err)
			}
			if got := f.history(); !slices.Equal(got, []string{"down", "up"}) || !f.wantUp {
				t.Fatalf("Down for %s: calls %q, want up %t; want down, then up with time left", downFor, got, f.wantUp)
			}
		})
	}
}

// TestSupervisedStopBoundIncludesDown: Poll's timeout and KillAfter count
// from before Down, so a Down that blocks, as OpenRC's rc-service stop
// does, leaves Kill and the timeout where they were
// (0035-MADR Amendment A3).
func TestSupervisedStopBoundIncludesDown(t *testing.T) {
	testproc.CheckNoLeak(t)
	for _, killNop := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			start := time.Now()
			f := newFakeSup()
			f.downAfter, f.killNop = -1, killNop
			s := supervisedOver(t, f, true, 400)
			s.o.Down = func(ctx context.Context) error {
				time.Sleep(3 * time.Second)
				return f.Down(ctx)
			}
			err := s.Stop(context.Background(), "demo")
			if at := f.killAt.Sub(start); at != 3*time.Second {
				t.Fatalf("killed at %s, want at once after Down's 3 s, past KillAfter's 1 s", at)
			}
			took := time.Since(start)
			switch {
			case !killNop && err != nil:
				t.Fatalf("a stop the kill ends: %v", err)
			case killNop && (!errors.Is(err, ErrTimeout) || took != 4*time.Second):
				t.Fatalf("a kill that ends nothing: %v after %s, want ErrTimeout at the 4 s bound", err, took)
			}
		})
	}
}

// TestSupervisedStopOutlivesContext: once the stop is issued, a cancelled
// context does not end the wait; its error comes after.
func TestSupervisedStopOutlivesContext(t *testing.T) {
	testproc.CheckNoLeak(t)
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		f := newFakeSup()
		f.downAfter = 500 * time.Millisecond
		s := supervisedOver(t, f, false, 400)
		time.AfterFunc(150*time.Millisecond, cancel)
		err := s.Stop(ctx, "demo")
		if !errors.Is(err, context.Canceled) || errors.Is(err, ErrTimeout) {
			t.Fatalf("Stop: %v, want context.Canceled and no timeout", err)
		}
		if f.up || !slices.Equal(f.history(), []string{"down"}) || time.Since(start) != 500*time.Millisecond {
			t.Fatalf("up %t, calls %q after %s: the wait ended early", f.up, f.history(), time.Since(start))
		}
	})
}

// TestSupervisedStopKill: Kill runs once, at KillAfter, and ends a process
// that ignores the stop.
func TestSupervisedStopKill(t *testing.T) {
	testproc.CheckNoLeak(t)
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		f := newFakeSup()
		f.downAfter = -1
		s := supervisedOver(t, f, true, 400)
		if err := s.Stop(context.Background(), "demo"); err != nil {
			t.Fatal(err)
		}
		if got := f.history(); !slices.Equal(got, []string{"down", "kill"}) {
			t.Fatalf("calls %q, want down then kill", got)
		}
		if at := f.killAt.Sub(start); at != time.Second {
			t.Fatalf("killed at %s, want at KillAfter, 1 s", at)
		}
	})
}

// TestSupervisedStopKillError: Kill's error is reported only when the stop
// fails.
func TestSupervisedStopKillError(t *testing.T) {
	testproc.CheckNoLeak(t)
	boom := errors.New("kill refused")
	synctest.Test(t, func(t *testing.T) {
		f := newFakeSup()
		f.downAfter, f.killErr, f.killNop = 1500*time.Millisecond, boom, true
		s := supervisedOver(t, f, true, 400)
		if err := s.Stop(context.Background(), "demo"); err != nil {
			t.Fatalf("a stop that ends after a failed kill: %v", err)
		}
		f = newFakeSup()
		f.downAfter, f.killErr, f.killNop = -1, boom, true
		s = supervisedOver(t, f, true, 400)
		if err := s.Stop(context.Background(), "demo"); !errors.Is(err, ErrTimeout) || !errors.Is(err, boom) {
			t.Fatalf("a failed stop: %v, want ErrTimeout and Kill's error", err)
		}
	})
}

// TestSupervisedStopReusedPID: the recorded process is gone once its PID
// belongs to another process.
func TestSupervisedStopReusedPID(t *testing.T) {
	testproc.CheckNoLeak(t)
	synctest.Test(t, func(t *testing.T) {
		f := newFakeSup()
		f.downAfter, f.reuse = 200*time.Millisecond, true
		s := supervisedOver(t, f, false, 400)
		if err := s.Stop(context.Background(), "demo"); err != nil {
			t.Fatalf("Stop with the PID reused: %v", err)
		}
	})
}

// TestSupervisedStopDownFails: a refused Down is an error, and Up keeps the
// supervisor wanting the running process up.
func TestSupervisedStopDownFails(t *testing.T) {
	testproc.CheckNoLeak(t)
	boom := errors.New("down refused")
	synctest.Test(t, func(t *testing.T) {
		f := newFakeSup()
		f.downErr = boom
		s := supervisedOver(t, f, false, 400)
		if err := s.Stop(context.Background(), "demo"); !errors.Is(err, boom) {
			t.Fatalf("Stop: %v, want Down's error", err)
		}
		if got := f.history(); !slices.Equal(got, []string{"down", "up"}) {
			t.Fatalf("calls %q, want down then up", got)
		}
	})
}

// TestSupervisedStopNoSupervisor: with no supervisor, nothing running is
// stopped already, and a running process cannot be stopped.
func TestSupervisedStopNoSupervisor(t *testing.T) {
	testproc.CheckNoLeak(t)
	synctest.Test(t, func(t *testing.T) {
		f := newFakeSup()
		f.supervised = false
		s := supervisedOver(t, f, false, 400)
		if err := s.Stop(context.Background(), "demo"); !errors.Is(err, ErrNoSupervisor) {
			t.Fatalf("a running process with no supervisor: %v", err)
		}
		f = newFakeSup()
		f.supervised, f.up = false, false
		s = supervisedOver(t, f, false, 400)
		if err := s.Stop(context.Background(), "demo"); err != nil {
			t.Fatalf("nothing running, no supervisor: %v", err)
		}
	})
}

func TestSupervisedStart(t *testing.T) {
	testproc.CheckNoLeak(t)
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		f := newFakeSup()
		s := supervisedOver(t, f, false, 400)
		if err := s.Start(ctx, "demo"); err != nil || len(f.history()) != 0 {
			t.Fatalf("a running instance: %v, calls %q; want it left alone", err, f.history())
		}
		f.downAfter = 0
		if err := s.Stop(ctx, "demo"); err != nil {
			t.Fatal(err)
		}
		if err := s.Start(ctx, "demo"); err != nil {
			t.Fatal(err)
		}
		if got := f.history(); !slices.Equal(got, []string{"down", "up"}) || !f.up || f.pid == 100 {
			t.Fatalf("calls %q, up %t pid %d; want a new process", got, f.up, f.pid)
		}
		// The recorded instance still running counts as not started.
		f2 := newFakeSup()
		s2 := supervisedOver(t, f2, false, 400)
		s2.recorded = instance{pid: 100, start: 7}
		if err := s2.Start(ctx, "demo"); err != nil || !slices.Equal(f2.history(), []string{"up"}) {
			t.Fatalf("the recorded instance: %v, calls %q; want up", err, f2.history())
		}
		// A service the supervisor reports as not installed is
		// ErrNotInstalled, and Up does not run (0038-MADR D2-A, question 5).
		f4 := newFakeSup()
		f4.installed, f4.up = false, false
		s4 := supervisedOver(t, f4, false, 400)
		if err := s4.Start(ctx, "demo"); !errors.Is(err, ErrNotInstalled) || len(f4.history()) != 0 {
			t.Fatalf("not installed: Start = %v, calls %q; want ErrNotInstalled and no Up", err, f4.history())
		}
		// Up's ErrNoSupervisor is passed on.
		f3 := newFakeSup()
		f3.supervised, f3.up = false, false
		f3.upErr = fmt.Errorf("no session supervisor: %w", ErrNoSupervisor)
		s3 := supervisedOver(t, f3, false, 400)
		if err := s3.Start(ctx, "demo"); !errors.Is(err, ErrNoSupervisor) {
			t.Fatalf("Start: %v, want ErrNoSupervisor", err)
		}
	})
}
