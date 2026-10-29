package service

import (
	"context"
	"fmt"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/internal/procfs"
)

// Exported for conformance_test.go, which runs servicetest over Supervised.
// servicetest imports this package, so that test is external, and its lab
// arranges fakeSup through SupervisedFake
// (docs/decisions/0038-PLAN-pinned-plans-lifecycle-conformance-and-bubble-tests.md
// Q11).

// SupervisedFake is fakeSup with its process table, behind a Supervised on
// the production clock. Its Down takes one second before the supervisor
// takes it, as a command such as rc-service's does, so a stop's bound is
// spent partly before the wait (0035-MADR Amendment A3).
type SupervisedFake struct {
	f *fakeSup
	// normallyUp and crash are what Set arranged; f.mu guards them.
	normallyUp, crash bool
}

// SupervisedFakeState is what SupervisedFake.Set arranges.
type SupervisedFakeState struct {
	Installed, Up, NormallyUp bool
	// DownAfter is how long the process takes to exit after Down;
	// negative is never.
	DownAfter time.Duration
	// Crash makes the process exit as soon as Up starts it, the
	// supervisor still wanting it up.
	Crash bool
}

// SupervisedFakeView is the fake as it is.
type SupervisedFakeView struct {
	Installed, Up, Downing, WantUp, NormallyUp bool
	PID                                        int
}

// NewSupervisedFake returns a Supervised named "demo" over a new fake, as
// process 400, unrelated to the service, with poll.
func NewSupervisedFake(poll PollOptions) (*Supervised, *SupervisedFake, error) {
	x := &SupervisedFake{f: newFakeSup()}
	state := func(ctx context.Context) (SupervisedState, error) {
		st, err := x.f.State(ctx)
		x.f.mu.Lock()
		st.NormallyUp = x.normallyUp
		x.f.mu.Unlock()
		return st, err
	}
	up := func(ctx context.Context) error {
		if err := x.f.Up(ctx); err != nil {
			return err
		}
		x.f.mu.Lock()
		defer x.f.mu.Unlock()
		if x.crash {
			x.f.exit()
			x.f.wantUp = true
		}
		return nil
	}
	down := func(ctx context.Context) error {
		timer := time.NewTimer(time.Second)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
		return x.f.Down(ctx)
	}
	s, err := supervisedOn(SupervisedOptions{Name: "demo", State: state, Up: up, Down: down, Poll: poll}, goosLinux)
	if err != nil {
		return nil, nil, err
	}
	s.readStat, s.self = x.f.read, func() int { return 400 }
	s.exe = func(pid int) (string, error) { return fmt.Sprintf("/opt/demo/demo-%d", pid), nil }
	return s, x, nil
}

// Set arranges the fake, ending the process it had.
func (x *SupervisedFake) Set(st SupervisedFakeState) {
	f := x.f
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.up {
		f.exit()
	}
	f.installed, f.wantUp, f.downing, f.downAfter = st.Installed, false, false, st.DownAfter
	x.normallyUp, x.crash = st.NormallyUp, st.Crash
	if st.Up {
		f.pid, f.start = f.pid+1, f.start+100
		f.procs[f.pid] = procfs.Stat{Parent: 10, StartTime: f.start, State: 'S'}
		f.up, f.wantUp = true, true
	}
}

// View is the fake as it is, once time has moved it.
func (x *SupervisedFake) View() SupervisedFakeView {
	f := x.f
	f.mu.Lock()
	defer f.mu.Unlock()
	f.advance()
	return SupervisedFakeView{
		Installed: f.installed, Up: f.up, Downing: f.downing, WantUp: f.wantUp,
		NormallyUp: x.normallyUp, PID: f.pid,
	}
}
