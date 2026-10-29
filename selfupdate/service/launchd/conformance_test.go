package launchd

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strconv"
	"testing"
	"testing/synctest"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/internal/testproc"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service/servicetest"
)

// Job checked by servicetest over fakeLaunchd, in synctest bubbles
// (docs/decisions/0038-PLAN-pinned-plans-lifecycle-conformance-and-bubble-tests.md
// Q11).

const (
	// labBootout is how long launchctl bootout takes to return, launchd
	// having taken the request at once.
	labBootout = 2 * time.Second
	// labSlowStop is how long a SlowStop job takes to go after its
	// bootout.
	labSlowStop = 5 * time.Second
	// labPID is the first process ID the lab gives a job: one no host
	// runs, so pidAlive is false once the job has gone.
	labPID = 50_000_000
)

// fakeLab is a servicetest.Lab over fakeLaunchd's job, and the Runner the
// Job runs through: it passes each command to the fake, after moving the
// job as time passes. A bootout returns labBootout after launchd took it;
// a context that ends meanwhile kills launchctl, and the job goes on
// going (0037-MADR Amendment A1, P2). A SlowStop job goes labSlowStop
// after its bootout, keeping its process until then; a Stubborn one
// never goes. The fake's mu guards the lab's fields.
type fakeLab struct {
	f     *fakeLaunchd
	plist string
	mode  servicetest.State
	// bootedOut is whether launchd took a bootout since Arrange.
	bootedOut bool
	// goneAt is when a SlowStop job goes; zero is never.
	goneAt time.Time
	// pids counts the processes the lab started.
	pids int
}

func newFakeLab(f *fakeLaunchd, plist string) *fakeLab {
	return &fakeLab{f: f, plist: plist}
}

// nextPID is a new process ID; f.mu is held.
func (l *fakeLab) nextPID() int {
	l.pids++
	return labPID + l.pids
}

// advance ends a slow stop whose time has come; f.mu is held.
func (l *fakeLab) advance() {
	if !l.goneAt.IsZero() && !time.Now().Before(l.goneAt) {
		f := l.f
		f.loaded, f.pid, f.state, f.goneAfter, l.goneAt = false, 0, "not running", 0, time.Time{}
	}
}

// Run implements service.Runner.
func (l *fakeLab) Run(ctx context.Context, c service.Command) (service.Output, error) {
	l.f.mu.Lock()
	l.advance()
	l.f.mu.Unlock()
	out, err := l.f.Run(ctx, c)
	if err != nil || len(c.Args) == 0 || c.Args[0] != "bootout" {
		return out, err
	}
	timer := time.NewTimer(labBootout)
	defer timer.Stop()
	select {
	case <-timer.C:
		return out, nil
	case <-ctx.Done():
		return service.Output{}, ctx.Err()
	}
}

// Arrange implements servicetest.Lab.
func (l *fakeLab) Arrange(ctx context.Context, a servicetest.Arrangement) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f := l.f
	f.mu.Lock()
	defer f.mu.Unlock()
	l.mode, l.bootedOut, l.goneAt = a.State, false, time.Time{}
	f.goneAfter, f.pidUntilGone, f.bootstrapHangs, f.disabled = 0, true, false, ""
	f.loaded, f.pid, f.state = false, 0, "not running"
	f.keys = map[string]string{"ProgramArguments.0": "/opt/demo/demo", "RunAtLoad": strconv.FormatBool(a.Enabled)}
	f.onStart = func(f *fakeLaunchd) { f.loaded, f.pid, f.state = true, l.nextPID(), "running" }
	f.onBootout = func(f *fakeLaunchd) {
		l.bootedOut = true
		switch l.mode {
		case servicetest.SlowStop:
			f.goneAfter, l.goneAt = 1<<30, time.Now().Add(labSlowStop)
		case servicetest.Stubborn:
			f.goneAfter = 1 << 30
		}
	}
	running := func() { f.loaded, f.pid, f.state = true, l.nextPID(), "running" }
	switch a.State {
	case servicetest.Absent:
		if err := os.Remove(l.plist); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	case servicetest.Stopped:
	case servicetest.Running, servicetest.SlowStop:
		running()
	case servicetest.Stubborn:
		// No kill bound: launchd leaves a job that ignores TERM running.
		running()
		f.keys["ExitTimeOut"] = "0"
	case servicetest.Crashing:
		f.onStart = func(f *fakeLaunchd) { f.loaded, f.pid, f.state = true, 0, "not running" }
	case servicetest.StartHangs:
		f.bootstrapHangs = true
	default:
		return fmt.Errorf("launchd fake lab: %s: %w", a.State, errors.ErrUnsupported)
	}
	return os.WriteFile(l.plist, []byte("<plist/>"), 0o600)
}

// Observe implements servicetest.Lab.
func (l *fakeLab) Observe(ctx context.Context) (servicetest.Observation, error) {
	if err := ctx.Err(); err != nil {
		return servicetest.Observation{}, err
	}
	_, err := os.Stat(l.plist)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return servicetest.Observation{}, err
	}
	installed := err == nil
	f := l.f
	f.mu.Lock()
	defer f.mu.Unlock()
	l.advance()
	return servicetest.Observation{
		Installed: installed,
		Running:   f.loaded && f.pid > 0,
		Stopping:  f.loaded && l.bootedOut,
		Enabled:   installed && f.keys["RunAtLoad"] == "true" && f.disabled != "disabled",
		Wanted:    f.loaded && !l.bootedOut,
		Detail:    fmt.Sprintf("loaded=%t pid=%d bootedOut=%t", f.loaded, f.pid, l.bootedOut),
	}, nil
}

// TestConformance: Job meets the Lifecycle contract over the fake. Its
// Stop is bounded by Poll's timeout, longer than ExitTimeOut's 5 s and the
// 30 s grace, and its Start by the bootstrap's, Poll's timeout too.
// Installed reads the property list and ignores its context.
func TestConformance(t *testing.T) {
	testproc.CheckNoLeak(t)
	synctest.Test(t, func(t *testing.T) {
		f := newFake()
		j := testJob(t, f, Options{Poll: bubblePoll})
		lab := newFakeLab(f, j.o.Plist)
		j.o.Runner = lab
		var skipped []string
		err := servicetest.VerifyLifecycle(context.Background(), j, servicetest.LifecycleCase{
			Product: "demo", Lab: lab, StopBound: service.DefaultPollTimeout, StartBound: service.DefaultPollTimeout,
			ContextFree: servicetest.MethodInstalled,
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

// TestRunnerConformance: the fake handles its context as ExecRunner does,
// a bootstrap that hangs included.
func TestRunnerConformance(t *testing.T) {
	testproc.CheckNoLeak(t)
	synctest.Test(t, func(t *testing.T) {
		f := newFake()
		f.bootstrapHangs = true
		err := servicetest.VerifyRunner(context.Background(), f, servicetest.RunnerCase{
			Quick:    service.Command{Path: "/bin/launchctl", Args: []string{"list", "com.example.demo"}},
			Blocking: service.Command{Path: "/bin/launchctl", Args: []string{"bootstrap", "gui/503", "/x.plist"}},
			Calls: func() int {
				f.mu.Lock()
				defer f.mu.Unlock()
				return len(f.calls)
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if v := f.verbs(); !slices.Equal(v, []string{"list"}) {
			t.Fatalf("verbs %q; want only the quick list", v)
		}
	})
}
