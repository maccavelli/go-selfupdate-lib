package scm

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"testing/synctest"

	"github.com/maccavelli/go-selfupdate-lib/internal/testproc"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service/servicetest"
)

// Service checked by servicetest over fakeSCM, in synctest bubbles
// (docs/decisions/0038-PLAN-pinned-plans-lifecycle-conformance-and-bubble-tests.md
// Q11).

// labPending is how many STOP_PENDING reports a SlowStop service makes,
// its checkpoint advancing, before it is STOPPED.
const labPending = 5

// fakeLab is a servicetest.Lab over fakeSCM's service "demo". A SlowStop
// service reports STOP_PENDING labPending times, then STOPPED; a Stubborn
// one stays STOP_PENDING, its checkpoint never advancing; a StartHangs one
// stays START_PENDING the same way. The fake's mu guards its fields.
type fakeLab struct {
	f *fakeSCM
	// pids counts the processes the lab started.
	pids uint32
}

// nextPID is a new process ID; f.mu is held.
func (l *fakeLab) nextPID() uint32 {
	l.pids++
	return 5000 + l.pids
}

func pendingStop(pid, checkpoint uint32) status {
	return status{Type: 0x10, State: stateStopPending, CheckPoint: checkpoint, WaitHint: 3000, ProcessID: pid}
}

// Arrange implements servicetest.Lab.
func (l *fakeLab) Arrange(ctx context.Context, a servicetest.Arrangement) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f := l.f
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.services, "demo")
	if a.State == servicetest.Absent {
		return nil
	}
	s := &fakeSvc{
		st:  status{Type: 0x10, State: stateStopped},
		cfg: config{Type: 0x10, StartType: startDemand, BinaryPathName: `"C:\Program Files\Demo\demo.exe" run`},
	}
	if a.Enabled {
		s.cfg.StartType = startAuto
	}
	s.onStart = func(s *fakeSvc) {
		pid := l.nextPID()
		s.queue = append(s.queue,
			status{Type: 0x10, State: stateStartPending, CheckPoint: 1, WaitHint: 3000, ProcessID: pid},
			status{Type: 0x10, State: stateRunning, ProcessID: pid})
	}
	running := func() { s.st = status{Type: 0x10, State: stateRunning, ProcessID: l.nextPID()} }
	switch a.State {
	case servicetest.Stopped:
	case servicetest.Running:
		running()
	case servicetest.SlowStop:
		running()
		s.onStop = func(s *fakeSvc) {
			for cp := range uint32(labPending) {
				s.queue = append(s.queue, pendingStop(s.st.ProcessID, cp+1))
			}
			s.queue = append(s.queue, status{Type: 0x10, State: stateStopped})
		}
	case servicetest.Stubborn:
		running()
		s.onStop = func(s *fakeSvc) { s.queue = append(s.queue, pendingStop(s.st.ProcessID, 1)) }
	case servicetest.Crashing:
		s.onStart = func(s *fakeSvc) {
			s.queue = append(s.queue,
				status{Type: 0x10, State: stateStartPending, CheckPoint: 1, WaitHint: 3000, ProcessID: l.nextPID()},
				status{Type: 0x10, State: stateStopped, Win32ExitCode: 1})
		}
	case servicetest.StartHangs:
		s.onStart = func(s *fakeSvc) {
			s.queue = append(s.queue, status{Type: 0x10, State: stateStartPending, CheckPoint: 1, WaitHint: 3000, ProcessID: l.nextPID()})
		}
	default:
		return fmt.Errorf("scm fake lab: %s: %w", a.State, errors.ErrUnsupported)
	}
	f.services["demo"] = s
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
	s, ok := f.services["demo"]
	if !ok {
		return servicetest.Observation{Detail: "no service"}, nil
	}
	st := s.st.State
	return servicetest.Observation{
		Installed: true,
		Running:   st != stateStopped,
		Stopping:  st == stateStopPending,
		Enabled:   s.cfg.StartType == startAuto,
		Wanted:    st == stateRunning || st == stateStartPending,
		Detail:    fmt.Sprintf("%s checkpoint %d pid %d", stateName(st), s.st.CheckPoint, s.st.ProcessID),
	}, nil
}

// TestConformance: Service meets the Lifecycle contract over the fake. Stop
// and Start are bounded by Poll's timeout. Installed, Running and Enabled
// ignore their context (Service's documentation).
func TestConformance(t *testing.T) {
	testproc.CheckNoLeak(t)
	synctest.Test(t, func(t *testing.T) {
		f := newFake()
		s := testService(t, f, Options{})
		var skipped []string
		err := servicetest.VerifyLifecycle(context.Background(), s, servicetest.LifecycleCase{
			Product: "demo", Lab: &fakeLab{f: f}, StopBound: service.DefaultPollTimeout, StartBound: service.DefaultPollTimeout,
			ContextFree: servicetest.MethodInstalled | servicetest.MethodRunning | servicetest.MethodEnabled,
			Logf:        func(format string, args ...any) { skipped = append(skipped, fmt.Sprintf(format, args...)) },
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(skipped) != 0 {
			t.Fatalf("skipped %q; the lab arranges every state", skipped)
		}
	})
}
