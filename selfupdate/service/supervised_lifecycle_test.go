package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/internal/procfs"
	"github.com/maccavelli/go-selfupdate-lib/internal/testproc"
)

// Tests for Stop and Start
// (docs/decisions/0035-PLAN-serve-supervisors-through-a-supervised-adapter.md
// M4), over a fake supervisor on a virtual clock.

// fakeSup models a supervisor and its one service. After Down, the
// service's process exits on the clock tick downAfter ticks later, or
// never when downAfter is negative (a program that ignores TERM); Kill
// ends it at once.
type fakeSup struct {
	mu         sync.Mutex
	calls      []string
	installed  bool
	supervised bool
	up, wantUp bool
	pid        int
	start      uint64
	procs      fakeProcs
	downAfter  int
	pending    int
	downing    bool
	// reuse, when set, gives the exited process's PID to an unrelated
	// process with another start time.
	reuse   bool
	upErr   error
	downErr error
	killErr error
	killNop bool
	killAt  time.Time
	clock   *vclock
}

func newFakeSup(c *vclock) *fakeSup {
	f := &fakeSup{installed: true, supervised: true, up: true, wantUp: true, pid: 100, start: 7, procs: table(), clock: c}
	return f
}

func (f *fakeSup) record(call string) { f.calls = append(f.calls, call) }

func (f *fakeSup) State(context.Context) (SupervisedState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
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

func (f *fakeSup) Up(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
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

func (f *fakeSup) Down(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("down")
	if f.downErr != nil {
		return f.downErr
	}
	f.wantUp, f.downing, f.pending = false, f.up, f.downAfter
	return nil
}

func (f *fakeSup) Kill(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("kill")
	f.killAt = f.clock.now()
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

// tick advances the process one clock tick.
func (f *fakeSup) tick() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.downing || f.pending < 0 {
		return
	}
	if f.pending == 0 {
		f.exit()
		return
	}
	f.pending--
}

func (f *fakeSup) read(pid int) (procfs.Stat, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.procs.read(pid)
}

func (f *fakeSup) history() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

// vclock is a virtual clock: each sleep advances it and runs onSleep.
type vclock struct {
	mu      sync.Mutex
	t       time.Time
	onSleep func()
}

func (c *vclock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *vclock) sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
	if c.onSleep != nil {
		c.onSleep()
	}
	return nil
}

// supervisedOver is a Supervised over f, as process self (400: unrelated),
// on a virtual clock that ticks f, with a 4 s stop bound polled every
// 100 ms.
func supervisedOver(t *testing.T, f *fakeSup, kill bool, self int) (*Supervised, *vclock) {
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
	c := f.clock
	c.onSleep = f.tick
	s.readStat, s.self, s.now, s.sleep = f.read, func() int { return self }, c.now, c.sleep
	s.exe = func(pid int) (string, error) { return fmt.Sprintf("/opt/demo/demo-%d", pid), nil }
	return s, c
}

func TestSupervisedStop(t *testing.T) {
	testproc.CheckNoLeak(t)
	ctx := context.Background()
	f := newFakeSup(&vclock{})
	f.downAfter = 3
	s, _ := supervisedOver(t, f, false, 400)
	if err := s.Stop(ctx, "demo"); err != nil {
		t.Fatal(err)
	}
	if got := f.history(); !slices.Equal(got, []string{"down"}) {
		t.Fatalf("calls %q, want only the down", got)
	}
	if s.recorded != (instance{pid: 100, start: 7}) || s.binary != "/opt/demo/demo-100" {
		t.Fatalf("recorded %v, binary %q", s.recorded, s.binary)
	}
}

func TestSupervisedStopRefusesFirst(t *testing.T) {
	testproc.CheckNoLeak(t)
	ctx := context.Background()
	f := newFakeSup(&vclock{})
	f.procs[300] = procfs.Stat{Parent: 100, StartTime: 9, State: 'S'}
	s, _ := supervisedOver(t, f, false, 300)
	if err := s.Stop(ctx, "demo"); !errors.Is(err, ErrInsideService) {
		t.Fatalf("inside: %v, want ErrInsideService", err)
	}
	f2 := newFakeSup(&vclock{})
	f2.installed = false
	s2, _ := supervisedOver(t, f2, false, 400)
	if err := s2.Stop(ctx, "demo"); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("not installed: %v, want ErrNotInstalled", err)
	}
	for _, x := range []*fakeSup{f, f2} {
		if got := x.history(); len(got) != 0 {
			t.Fatalf("a refused stop called %q", got)
		}
	}
	// Down, not wanted up: nothing to do.
	f3 := newFakeSup(&vclock{})
	f3.up, f3.wantUp = false, false
	s3, _ := supervisedOver(t, f3, false, 400)
	if err := s3.Stop(ctx, "demo"); err != nil || len(f3.history()) != 0 {
		t.Fatalf("a stopped service: %v, calls %q", err, f3.history())
	}
}

// TestSupervisedStopTimeout: a process that ignores the stop times out,
// and Up runs, so the supervisor wants it up again.
func TestSupervisedStopTimeout(t *testing.T) {
	testproc.CheckNoLeak(t)
	f := newFakeSup(&vclock{})
	f.downAfter = -1
	s, c := supervisedOver(t, f, false, 400)
	err := s.Stop(context.Background(), "demo")
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("Stop: %v, want ErrTimeout", err)
	}
	if got := f.history(); !slices.Equal(got, []string{"down", "up"}) {
		t.Fatalf("calls %q, want down then up", got)
	}
	if !f.wantUp || c.now().Sub(time.Time{}) < 4*time.Second {
		t.Fatalf("want up %t after %s", f.wantUp, c.now().Sub(time.Time{}))
	}
}

// TestSupervisedStopOutlivesContext: once the stop is issued, a cancelled
// context does not end the wait; its error comes after.
func TestSupervisedStopOutlivesContext(t *testing.T) {
	testproc.CheckNoLeak(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := newFakeSup(&vclock{})
	f.downAfter = 5
	s, c := supervisedOver(t, f, false, 400)
	ticks := 0
	c.onSleep = func() {
		ticks++
		if ticks == 2 {
			cancel()
		}
		f.tick()
	}
	err := s.Stop(ctx, "demo")
	if !errors.Is(err, context.Canceled) || errors.Is(err, ErrTimeout) {
		t.Fatalf("Stop: %v, want context.Canceled and no timeout", err)
	}
	if f.up || !slices.Equal(f.history(), []string{"down"}) {
		t.Fatalf("up %t, calls %q: the wait ended early", f.up, f.history())
	}
}

// TestSupervisedStopKill: Kill runs once, at KillAfter, and ends a process
// that ignores the stop.
func TestSupervisedStopKill(t *testing.T) {
	testproc.CheckNoLeak(t)
	f := newFakeSup(&vclock{})
	f.downAfter = -1
	s, _ := supervisedOver(t, f, true, 400)
	if err := s.Stop(context.Background(), "demo"); err != nil {
		t.Fatal(err)
	}
	if got := f.history(); !slices.Equal(got, []string{"down", "kill"}) {
		t.Fatalf("calls %q, want down then kill", got)
	}
	if at := f.killAt.Sub(time.Time{}); at < time.Second || at > 1100*time.Millisecond {
		t.Fatalf("killed at %s, want at KillAfter, 1 s", at)
	}
}

// TestSupervisedStopKillError: Kill's error is reported only when the stop
// fails.
func TestSupervisedStopKillError(t *testing.T) {
	testproc.CheckNoLeak(t)
	boom := errors.New("kill refused")
	f := newFakeSup(&vclock{})
	f.downAfter, f.killErr, f.killNop = 15, boom, true
	s, _ := supervisedOver(t, f, true, 400)
	if err := s.Stop(context.Background(), "demo"); err != nil {
		t.Fatalf("a stop that ends after a failed kill: %v", err)
	}
	f = newFakeSup(&vclock{})
	f.downAfter, f.killErr, f.killNop = -1, boom, true
	s, _ = supervisedOver(t, f, true, 400)
	if err := s.Stop(context.Background(), "demo"); !errors.Is(err, ErrTimeout) || !errors.Is(err, boom) {
		t.Fatalf("a failed stop: %v, want ErrTimeout and Kill's error", err)
	}
}

// TestSupervisedStopReusedPID: the recorded process is gone once its PID
// belongs to another process.
func TestSupervisedStopReusedPID(t *testing.T) {
	testproc.CheckNoLeak(t)
	f := newFakeSup(&vclock{})
	f.downAfter, f.reuse = 2, true
	s, _ := supervisedOver(t, f, false, 400)
	if err := s.Stop(context.Background(), "demo"); err != nil {
		t.Fatalf("Stop with the PID reused: %v", err)
	}
}

// TestSupervisedStopDownFails: a refused Down is an error, and Up keeps the
// supervisor wanting the running process up.
func TestSupervisedStopDownFails(t *testing.T) {
	testproc.CheckNoLeak(t)
	boom := errors.New("down refused")
	f := newFakeSup(&vclock{})
	f.downErr = boom
	s, _ := supervisedOver(t, f, false, 400)
	if err := s.Stop(context.Background(), "demo"); !errors.Is(err, boom) {
		t.Fatalf("Stop: %v, want Down's error", err)
	}
	if got := f.history(); !slices.Equal(got, []string{"down", "up"}) {
		t.Fatalf("calls %q, want down then up", got)
	}
}

// TestSupervisedStopNoSupervisor: with no supervisor, nothing running is
// stopped already, and a running process cannot be stopped.
func TestSupervisedStopNoSupervisor(t *testing.T) {
	testproc.CheckNoLeak(t)
	f := newFakeSup(&vclock{})
	f.supervised = false
	s, _ := supervisedOver(t, f, false, 400)
	if err := s.Stop(context.Background(), "demo"); !errors.Is(err, ErrNoSupervisor) {
		t.Fatalf("a running process with no supervisor: %v", err)
	}
	f = newFakeSup(&vclock{})
	f.supervised, f.up = false, false
	s, _ = supervisedOver(t, f, false, 400)
	if err := s.Stop(context.Background(), "demo"); err != nil {
		t.Fatalf("nothing running, no supervisor: %v", err)
	}
}

func TestSupervisedStart(t *testing.T) {
	testproc.CheckNoLeak(t)
	ctx := context.Background()
	f := newFakeSup(&vclock{})
	s, _ := supervisedOver(t, f, false, 400)
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
	f2 := newFakeSup(&vclock{})
	s2, _ := supervisedOver(t, f2, false, 400)
	s2.recorded = instance{pid: 100, start: 7}
	if err := s2.Start(ctx, "demo"); err != nil || !slices.Equal(f2.history(), []string{"up"}) {
		t.Fatalf("the recorded instance: %v, calls %q; want up", err, f2.history())
	}
	// Up's ErrNoSupervisor is passed on.
	f3 := newFakeSup(&vclock{})
	f3.supervised, f3.up = false, false
	f3.upErr = fmt.Errorf("no session supervisor: %w", ErrNoSupervisor)
	s3, _ := supervisedOver(t, f3, false, 400)
	if err := s3.Start(ctx, "demo"); !errors.Is(err, ErrNoSupervisor) {
		t.Fatalf("Start: %v, want ErrNoSupervisor", err)
	}
}
