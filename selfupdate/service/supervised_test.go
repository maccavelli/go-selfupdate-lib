package service

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"runtime"
	"testing"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/internal/procfs"
	"github.com/maccavelli/go-selfupdate-lib/internal/testproc"
)

// Tests for the supervised adapter's construction, reads and inside check
// (docs/decisions/0035-PLAN-serve-supervisors-through-a-supervised-adapter.md
// M3), over a fake process table on every OS.

// fakeProcs is a process table by PID; a missing PID is fs.ErrNotExist.
type fakeProcs map[int]procfs.Stat

func (f fakeProcs) read(pid int) (procfs.Stat, error) {
	s, ok := f[pid]
	if !ok {
		return procfs.Stat{}, fmt.Errorf("fake /proc/%d: %w", pid, fs.ErrNotExist)
	}
	return s, nil
}

// table is a supervisor (10) serving a service (100), whose agent (200) runs
// an update (300); 400 is unrelated.
func table() fakeProcs {
	return fakeProcs{
		1:   {Parent: 0, StartTime: 1, State: 'S'},
		10:  {Parent: 1, StartTime: 5, State: 'S'},
		100: {Parent: 10, StartTime: 7, State: 'S'},
		200: {Parent: 100, StartTime: 8, State: 'S'},
		300: {Parent: 200, StartTime: 9, State: 'S'},
		400: {Parent: 1, StartTime: 6, State: 'S'},
	}
}

func nop(context.Context) error { return nil }

func states(st SupervisedState, err error) func(context.Context) (SupervisedState, error) {
	return func(context.Context) (SupervisedState, error) { return st, err }
}

// fakeSupervised is a Supervised on "linux" over procs, as process self.
func fakeSupervised(t *testing.T, o SupervisedOptions, procs fakeProcs, self int) *Supervised {
	t.Helper()
	if o.Up == nil {
		o.Up = nop
	}
	if o.Down == nil {
		o.Down = nop
	}
	s, err := supervisedOn(o, goosLinux)
	if err != nil {
		t.Fatal(err)
	}
	s.readStat = procs.read
	s.self = func() int { return self }
	return s
}

var up100 = SupervisedState{Installed: true, Supervised: true, Up: true, PID: 100, WantUp: true, NormallyUp: true}

func TestNewSupervisedRefuses(t *testing.T) {
	testproc.CheckNoLeak(t)
	st := states(up100, nil)
	kill := func(context.Context) error { return nil }
	for name, c := range map[string]struct {
		o    SupervisedOptions
		goos string
	}{
		"off Linux":                   {SupervisedOptions{State: st, Up: nop, Down: nop}, "darwin"},
		"no State":                    {SupervisedOptions{Up: nop, Down: nop}, goosLinux},
		"no Up":                       {SupervisedOptions{State: st, Down: nop}, goosLinux},
		"no Down":                     {SupervisedOptions{State: st, Up: nop}, goosLinux},
		"settle not under timeout":    {SupervisedOptions{State: st, Up: nop, Down: nop, Poll: PollOptions{Timeout: time.Second, Settle: 2 * time.Second}}, goosLinux},
		"negative KillAfter":          {SupervisedOptions{State: st, Up: nop, Down: nop, Kill: kill, KillAfter: -time.Second}, goosLinux},
		"KillAfter without Kill":      {SupervisedOptions{State: st, Up: nop, Down: nop, KillAfter: time.Second}, goosLinux},
		"KillAfter not under timeout": {SupervisedOptions{State: st, Up: nop, Down: nop, Kill: kill, KillAfter: 60 * time.Second}, goosLinux},
	} {
		if _, err := supervisedOn(c.o, c.goos); err == nil {
			t.Errorf("%s: accepted", name)
		} else if name == "off Linux" && !errors.Is(err, ErrUnsupported) {
			t.Errorf("off Linux: %v, want ErrUnsupported", err)
		}
	}
	s, err := supervisedOn(SupervisedOptions{State: st, Up: nop, Down: nop, Kill: kill, Poll: PollOptions{Timeout: 20 * time.Second}}, goosLinux)
	if err != nil || s.o.KillAfter != 10*time.Second {
		t.Fatalf("default KillAfter: %v, %v; want half of the timeout", s, err)
	}
	if _, err := NewSupervised(SupervisedOptions{State: st, Up: nop, Down: nop}); runtime.GOOS != goosLinux && !errors.Is(err, ErrUnsupported) {
		t.Fatalf("NewSupervised on %s: %v, want ErrUnsupported", runtime.GOOS, err)
	}
}

func TestSupervisedStatus(t *testing.T) {
	testproc.CheckNoLeak(t)
	ctx := context.Background()
	gone := table()
	delete(gone, 100)
	zombie := table()
	zombie[100] = procfs.Stat{Parent: 10, StartTime: 7, State: 'Z'}
	for name, c := range map[string]struct {
		st    SupervisedState
		procs fakeProcs
		want  string
	}{
		"running":      {up100, table(), "100@7"},
		"process gone": {up100, gone, ""},
		"zombie":       {up100, zombie, ""},
		"down":         {SupervisedState{Installed: true, Supervised: true}, table(), ""},
		"up, no pid":   {SupervisedState{Installed: true, Up: true}, table(), ""},
	} {
		s := fakeSupervised(t, SupervisedOptions{State: states(c.st, nil)}, c.procs, 300)
		got, err := s.Status(ctx)
		if err != nil || got.Instance != c.want {
			t.Errorf("%s: Instance %q, %v; want %q", name, got.Instance, err, c.want)
		}
	}
	boom := errors.New("boom")
	s := fakeSupervised(t, SupervisedOptions{State: states(SupervisedState{}, boom)}, table(), 300)
	if _, err := s.Status(ctx); !errors.Is(err, boom) {
		t.Fatalf("State's error: %v", err)
	}
	s = fakeSupervised(t, SupervisedOptions{State: states(up100, nil)}, table(), 300)
	s.readStat = func(int) (procfs.Stat, error) { return procfs.Stat{}, fs.ErrPermission }
	if _, err := s.Status(ctx); !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("an unreadable process: %v, want its error", err)
	}
}

func TestSupervisedReads(t *testing.T) {
	testproc.CheckNoLeak(t)
	ctx := context.Background()
	s := fakeSupervised(t, SupervisedOptions{State: states(up100, nil)}, table(), 300)
	for name, call := range map[string]func(context.Context, string) (bool, error){
		"Installed": s.Installed, "Enabled": s.Enabled, "Running": s.Running,
	} {
		if got, err := call(ctx, "demo"); err != nil || !got {
			t.Errorf("%s: %t, %v; want true", name, got, err)
		}
	}
	// No supervisor: still installed and enabled, not running.
	none := SupervisedState{Installed: true, NormallyUp: true}
	s = fakeSupervised(t, SupervisedOptions{State: states(none, fmt.Errorf("no runsv: %w", ErrNoSupervisor))}, table(), 300)
	if got, err := s.Installed(ctx, "demo"); err != nil || !got {
		t.Errorf("Installed with no supervisor: %t, %v", got, err)
	}
	if got, err := s.Enabled(ctx, "demo"); err != nil || !got {
		t.Errorf("Enabled with no supervisor: %t, %v", got, err)
	}
	if got, err := s.Running(ctx, "demo"); err != nil || got {
		t.Errorf("Running with no supervisor: %t, %v", got, err)
	}
	// ... unless the instance a Stop recorded still runs (D10-A).
	s.recorded = instance{pid: 100, start: 7}
	if got, err := s.Running(ctx, "demo"); err != nil || !got {
		t.Errorf("Running with no supervisor, recorded instance alive: %t, %v", got, err)
	}
	s.recorded = instance{pid: 100, start: 99}
	if got, err := s.Running(ctx, "demo"); err != nil || got {
		t.Errorf("Running with no supervisor, PID reused: %t, %v", got, err)
	}
	boom := errors.New("boom")
	s = fakeSupervised(t, SupervisedOptions{State: states(SupervisedState{}, boom)}, table(), 300)
	for name, call := range map[string]func(context.Context, string) (bool, error){
		"Installed": s.Installed, "Enabled": s.Enabled, "Running": s.Running,
	} {
		if _, err := call(ctx, "demo"); !errors.Is(err, boom) {
			t.Errorf("%s: %v, want State's error", name, err)
		}
	}
}

func TestSupervisedInside(t *testing.T) {
	testproc.CheckNoLeak(t)
	ctx := context.Background()
	for name, c := range map[string]struct {
		st   SupervisedState
		err  error
		self int
		want bool
	}{
		"the update descends from the service": {up100, nil, 300, true},
		"the service itself":                   {up100, nil, 100, true},
		"an unrelated process":                 {up100, nil, 400, false},
		"the supervisor, above the service":    {up100, nil, 10, false},
		"the service is down":                  {SupervisedState{Installed: true, Supervised: true}, nil, 300, false},
		"no supervisor, the process still up":  {up100, ErrNoSupervisor, 300, true},
	} {
		s := fakeSupervised(t, SupervisedOptions{State: states(c.st, c.err)}, table(), c.self)
		if got, err := s.Inside(ctx); err != nil || got != c.want {
			t.Errorf("%s: %t, %v; want %t", name, got, err, c.want)
		}
	}

	// The service's PID is reused between the observation and the walk:
	// the new process is not the service (D7-A).
	s := fakeSupervised(t, SupervisedOptions{State: states(up100, nil)}, table(), 300)
	reads := 0
	procs := table()
	s.readStat = func(pid int) (procfs.Stat, error) {
		if pid == 100 {
			reads++
			if reads > 1 {
				return procfs.Stat{Parent: 10, StartTime: 70, State: 'S'}, nil
			}
		}
		return procs.read(pid)
	}
	if got, err := s.Inside(ctx); err != nil || got {
		t.Errorf("a reused PID: %t, %v; want false", got, err)
	}

	// An unreadable step is an error, never "outside".
	broken := table()
	broken[300] = procfs.Stat{Parent: 250, StartTime: 9, State: 'S'}
	s = fakeSupervised(t, SupervisedOptions{State: states(up100, nil)}, broken, 300)
	if got, err := s.Inside(ctx); err == nil || got {
		t.Errorf("an unreadable parent: %t, %v; want an error", got, err)
	}

	// A loop in the parent links ends in an error.
	loop := table()
	loop[500] = procfs.Stat{Parent: 501, StartTime: 3, State: 'S'}
	loop[501] = procfs.Stat{Parent: 500, StartTime: 4, State: 'S'}
	s = fakeSupervised(t, SupervisedOptions{State: states(up100, nil)}, loop, 500)
	if got, err := s.Inside(ctx); err == nil || got {
		t.Errorf("a parent loop: %t, %v; want an error", got, err)
	}

	boom := errors.New("boom")
	s = fakeSupervised(t, SupervisedOptions{State: states(SupervisedState{}, boom)}, table(), 300)
	if _, err := s.Inside(ctx); !errors.Is(err, boom) {
		t.Errorf("State's error: %v", err)
	}
}
