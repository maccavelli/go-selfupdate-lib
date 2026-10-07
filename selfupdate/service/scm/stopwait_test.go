package scm

import (
	"context"
	"errors"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// Tests for docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md
// B3: once the stop control is sent, Stop waits for STOPPED whatever the
// caller's context does, bounded by Options.Poll's timeout.

// TestStopFinishesAfterCancel: a caller's context that ends once the stop
// is sent does not cut the wait short. Stop waits until the service is
// STOPPED, then reports the cancellation.
func TestStopFinishesAfterCancel(t *testing.T) {
	f := newFake()
	svc := f.add("demo", 4242)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.onStop = func(s *fakeSvc) {
		cancel()
		s.queue = append(s.queue,
			status{Type: 0x10, State: stateStopPending, CheckPoint: 1, WaitHint: 3000, ProcessID: 4242},
			status{Type: 0x10, State: stateStopPending, CheckPoint: 2, WaitHint: 3000, ProcessID: 4242},
			status{Type: 0x10, State: stateStopped})
	}
	s, _ := testService(t, f, Options{})
	err := s.Stop(ctx, "demo")
	if !errors.Is(err, context.Canceled) || errors.Is(err, service.ErrTimeout) {
		t.Fatalf("Stop = %v, want the caller's cancellation alone", err)
	}
	if svc.st.State != stateStopped {
		t.Fatalf("Stop returned with the service %s", stateName(svc.st.State))
	}
}
