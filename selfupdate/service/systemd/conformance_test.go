package systemd

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/internal/testproc"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service/servicetest"
)

// Unit checked by servicetest over fakeSystemd, in synctest bubbles
// (docs/decisions/0038-PLAN-pinned-plans-lifecycle-conformance-and-bubble-tests.md
// Q11).

// labSlowStop is how long a SlowStop unit takes to go once its stop job
// is queued.
const labSlowStop = 5 * time.Second

// fakeLab is a servicetest.Lab over fakeSystemd's unit. It moves the unit
// as time passes, when the unit is next read: a SlowStop unit goes
// labSlowStop after its stop job is queued, and a Stubborn one stays
// deactivating and holds a stop that waits for its job. f.mu guards its
// fields.
type fakeLab struct {
	f *fakeSystemd
	// goneAt is when a SlowStop unit goes; zero is never.
	goneAt time.Time
	// starts counts the unit's starts, for a new InvocationID each.
	starts int
}

func newFakeLab(f *fakeSystemd) *fakeLab {
	l := &fakeLab{f: f}
	f.onRun = l.advance
	return l
}

// advance ends a slow stop whose time has come; f.mu is held.
func (l *fakeLab) advance(p map[string]string) {
	if !l.goneAt.IsZero() && !time.Now().Before(l.goneAt) {
		p["ActiveState"], p["SubState"], l.goneAt = "inactive", "dead", time.Time{}
	}
}

// started is a start that succeeds; f.mu is held.
func (l *fakeLab) started(p map[string]string) {
	l.starts++
	p["ActiveState"], p["SubState"], p["Result"] = "active", "running", "success"
	p["InvocationID"] = fmt.Sprintf("inv-%d", l.starts)
}

func deactivating(p map[string]string) {
	p["ActiveState"], p["SubState"] = "deactivating", "stop-sigterm"
}

// Arrange implements servicetest.Lab. It releases any call the last
// arrangement held.
func (l *fakeLab) Arrange(ctx context.Context, a servicetest.Arrangement) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f := l.f
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, block := range []*chan struct{}{&f.stopBlocks, &f.startBlocks} {
		if *block != nil {
			close(*block)
			*block = nil
		}
	}
	l.goneAt = time.Time{}
	delete(f.fail, "start")
	delete(f.fail, "stop")
	f.onStop, f.onStart = nil, l.started
	p := f.props
	p["LoadState"], p["ActiveState"], p["SubState"], p["Result"], p["NRestarts"] = "loaded", "inactive", "dead", "success", "0"
	p["InvocationID"] = fmt.Sprintf("inv-%d", l.starts)
	p["UnitFileState"] = "disabled"
	if a.Enabled {
		p["UnitFileState"] = "enabled"
	}
	switch a.State {
	case servicetest.Absent:
		p["LoadState"], p["UnitFileState"] = "not-found", ""
		f.fail["start"] = service.Output{ExitCode: 5, Stderr: []byte("Failed to start demo.service: Unit demo.service not found.")}
		f.fail["stop"] = service.Output{ExitCode: 5, Stderr: []byte("Failed to stop demo.service: Unit demo.service not loaded.")}
	case servicetest.Stopped:
	case servicetest.Running:
		p["ActiveState"], p["SubState"] = "active", "running"
	case servicetest.SlowStop:
		p["ActiveState"], p["SubState"] = "active", "running"
		f.onStop = func(p map[string]string) {
			deactivating(p)
			l.goneAt = time.Now().Add(labSlowStop)
		}
	case servicetest.Stubborn:
		p["ActiveState"], p["SubState"] = "active", "running"
		f.onStop, f.stopBlocks = deactivating, make(chan struct{})
	case servicetest.Crashing:
		f.onStart = func(p map[string]string) {
			l.started(p)
			p["ActiveState"], p["SubState"], p["Result"] = "failed", "failed", "exit-code"
		}
	case servicetest.StartHangs:
		f.startBlocks = make(chan struct{})
	default:
		return fmt.Errorf("systemd fake lab: %s: %w", a.State, errors.ErrUnsupported)
	}
	return nil
}

// Observe implements servicetest.Lab.
func (l *fakeLab) Observe(ctx context.Context) (servicetest.Observation, error) {
	if err := ctx.Err(); err != nil {
		return servicetest.Observation{}, err
	}
	l.f.mu.Lock()
	defer l.f.mu.Unlock()
	p := l.f.props
	l.advance(p)
	s := p["ActiveState"]
	return servicetest.Observation{
		Installed: p["LoadState"] != "not-found",
		Running:   runningStates[s],
		Stopping:  s == "deactivating",
		Enabled:   p["UnitFileState"] == "enabled",
		Wanted:    s == "active" || s == "activating" || s == "reloading",
		Detail:    "ActiveState=" + s + " SubState=" + p["SubState"],
	}, nil
}

// TestConformance: Unit meets the Lifecycle contract over the fake. Its
// Stop is bounded by Poll's timeout when the unit sets no
// TimeoutStopUSec. Start documents no bound of its own, so the start-hangs
// check is skipped.
func TestConformance(t *testing.T) {
	testproc.CheckNoLeak(t)
	synctest.Test(t, func(t *testing.T) {
		f := newFake()
		lab := newFakeLab(f)
		u := testUnit(t, f, Options{Poll: bubblePoll})
		var skipped []string
		err := servicetest.VerifyLifecycle(context.Background(), u, servicetest.LifecycleCase{
			Product: "demo", Lab: lab, StopBound: service.DefaultPollTimeout,
			Logf: func(format string, args ...any) { skipped = append(skipped, fmt.Sprintf(format, args...)) },
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(skipped) != 1 || !strings.Contains(skipped[0], "start hangs") {
			t.Fatalf("skipped %q; want only the start-hangs check", skipped)
		}
	})
}

// TestRunnerConformance: the fake handles its context as ExecRunner does,
// a stop that waits for its job included.
func TestRunnerConformance(t *testing.T) {
	testproc.CheckNoLeak(t)
	synctest.Test(t, func(t *testing.T) {
		f := newFake()
		f.stopBlocks = make(chan struct{})
		err := servicetest.VerifyRunner(context.Background(), f, servicetest.RunnerCase{
			Quick:    service.Command{Path: "/usr/bin/systemctl", Args: []string{"show", "-p", "Id", "--", "demo.service"}},
			Blocking: service.Command{Path: "/usr/bin/systemctl", Args: []string{"stop", "--no-ask-password", "--", "demo.service"}},
			Calls: func() int {
				f.mu.Lock()
				defer f.mu.Unlock()
				return len(f.calls)
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if v := f.verbs(); !slices.Equal(v, []string{"show"}) {
			t.Fatalf("verbs %q; want only the quick show", v)
		}
	})
}
