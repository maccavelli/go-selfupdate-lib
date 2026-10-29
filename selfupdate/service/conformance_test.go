package service_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/internal/testproc"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service/servicetest"
)

// Supervised checked by servicetest over a fake supervisor, in a synctest
// bubble
// (docs/decisions/0038-PLAN-pinned-plans-lifecycle-conformance-and-bubble-tests.md
// Q11).

// labSlowStop is how long a SlowStop service's process takes to exit
// after Down.
const labSlowStop = 5 * time.Second

// supervisedLab is a servicetest.Lab over service.SupervisedFake. A
// StartHangs service cannot be arranged: Start runs Up on the caller's
// context, with no bound of its own.
type supervisedLab struct{ x *service.SupervisedFake }

// Arrange implements servicetest.Lab.
func (l supervisedLab) Arrange(ctx context.Context, a servicetest.Arrangement) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	st := service.SupervisedFakeState{Installed: true, NormallyUp: a.Enabled}
	switch a.State {
	case servicetest.Absent:
		st = service.SupervisedFakeState{}
	case servicetest.Stopped:
	case servicetest.Running:
		st.Up = true
	case servicetest.SlowStop:
		st.Up, st.DownAfter = true, labSlowStop
	case servicetest.Stubborn:
		st.Up, st.DownAfter = true, -1
	case servicetest.Crashing:
		st.Crash = true
	default:
		return fmt.Errorf("supervised fake lab: %s: %w", a.State, errors.ErrUnsupported)
	}
	l.x.Set(st)
	return nil
}

// Observe implements servicetest.Lab.
func (l supervisedLab) Observe(ctx context.Context) (servicetest.Observation, error) {
	if err := ctx.Err(); err != nil {
		return servicetest.Observation{}, err
	}
	v := l.x.View()
	return servicetest.Observation{
		Installed: v.Installed,
		Running:   v.Up,
		Stopping:  v.Downing,
		Enabled:   v.Installed && v.NormallyUp,
		Wanted:    v.WantUp,
		Detail:    fmt.Sprintf("up=%t pid=%d downing=%t wantUp=%t", v.Up, v.PID, v.Downing, v.WantUp),
	}, nil
}

// TestSupervisedConformance: Supervised meets the Lifecycle contract over
// a fake supervisor whose Down takes a second. A failed Stop calls Up
// again, with a bound of its own, so Stop may take twice Poll's timeout
// (0035-MADR Amendment A3).
func TestSupervisedConformance(t *testing.T) {
	testproc.CheckNoLeak(t)
	synctest.Test(t, func(t *testing.T) {
		s, x, err := service.NewSupervisedFake(service.PollOptions{})
		if err != nil {
			t.Fatal(err)
		}
		var skipped []string
		err = servicetest.VerifyLifecycle(context.Background(), s, servicetest.LifecycleCase{
			Product: "demo", Lab: supervisedLab{x}, StopBound: 2 * service.DefaultPollTimeout,
			RestoresAfterFailedStop: true,
			Logf:                    func(format string, args ...any) { skipped = append(skipped, fmt.Sprintf(format, args...)) },
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(skipped) != 1 || !strings.Contains(skipped[0], "start hangs") {
			t.Fatalf("skipped %q; want only the start-hangs check", skipped)
		}
	})
}
