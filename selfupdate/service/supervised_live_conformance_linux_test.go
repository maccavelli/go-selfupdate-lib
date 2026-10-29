//go:build linux

package service_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service/servicetest"
)

// The s6 run of servicetest, which TestLiveSupervisedS6 runs as its
// "conformance" subtest through service.LiveS6Conformance: package service
// cannot import servicetest
// (docs/decisions/0038-PLAN-pinned-plans-lifecycle-conformance-and-bubble-tests.md
// Q12, Deviation D8).

func init() {
	service.LiveS6Conformance = liveS6Conformance
}

// s6SlowStop is how long a SlowStop service's run script takes to exit
// after TERM.
const s6SlowStop = 3 * time.Second

// s6Lab is a servicetest.Lab over an s6 tree the test runs: the service
// directory svc, linked into the scan directory scan as link, whose run
// script the lab writes for each state. A Stubborn script ignores TERM:
// the adapter it is checked with has no Kill, so its stop fails as the lab
// arranges.
type s6Lab struct {
	dir, scan, svc, link string
	tool                 func(name string) (string, error)
}

// s6 runs an s6 tool with args.
func (l *s6Lab) s6(name string, args ...string) ([]byte, error) {
	path, err := l.tool(name)
	if err != nil {
		return nil, err
	}
	out, err := exec.Command(path, args...).CombinedOutput() //nolint:gosec // an s6 tool at its installed path
	if err != nil {
		return out, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, out)
	}
	return out, nil
}

// waitFor polls cond every 50 ms until it holds, or fails after limit.
func waitFor(limit time.Duration, what string, cond func() bool) error {
	for deadline := time.Now().Add(limit); !cond(); time.Sleep(50 * time.Millisecond) {
		if time.Now().After(deadline) {
			return fmt.Errorf("%s: not within %s", what, limit)
		}
	}
	return nil
}

// remove ends the service and its s6-supervise, and deletes its
// directory: down, then a kill for a script that ignores TERM; then the
// link leaves the scan directory, and s6-svscan, told to rescan and nuke,
// ends the supervisor it would otherwise start again.
func (l *s6Lab) remove() error {
	if _, err := os.Stat(l.svc); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	for _, flag := range []string{"-d", "-k"} {
		_, _ = l.s6("s6-svc", flag, l.svc)
	}
	if err := os.Remove(l.link); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if _, err := l.s6("s6-svscanctl", "-an", l.scan); err != nil {
		return err
	}
	if err := waitFor(10*time.Second, "s6-supervise exits", func() bool {
		_, err := l.s6("s6-svok", l.svc)
		return err != nil
	}); err != nil {
		return err
	}
	return os.RemoveAll(l.svc)
}

// Arrange implements servicetest.Lab. It removes the service, then writes
// its directory, down, with the state's run script, has s6-svscan
// supervise it, removes the down file when enabled, and brings it up for a
// state that runs, returning once its script has reported ready.
func (l *s6Lab) Arrange(_ context.Context, a servicetest.Arrangement) error {
	if err := l.remove(); err != nil {
		return err
	}
	ready := filepath.Join(l.dir, "ready")
	if err := os.Remove(ready); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	report := "echo $$ > " + ready + "\n"
	var script string
	switch a.State {
	case servicetest.Absent:
		return nil
	case servicetest.Stopped, servicetest.Running:
		script = report + "exec sleep 100000\n"
	case servicetest.SlowStop:
		script = fmt.Sprintf("trap 'sleep %d; exit 0' TERM\n", int(s6SlowStop.Seconds())) + report + "while :; do sleep 0.1; done\n"
	case servicetest.Stubborn:
		script = "trap '' TERM\n" + report + "while :; do sleep 0.1; done\n"
	case servicetest.Crashing:
		script = "exit 3\n"
	default:
		return fmt.Errorf("s6 live lab: %s: %w", a.State, errors.ErrUnsupported)
	}
	if err := os.MkdirAll(l.svc, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(l.svc, "down"), nil, 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(l.svc, "run"), []byte("#!/bin/sh\n"+script), 0o755); err != nil { //nolint:gosec // the service's run script
		return err
	}
	if err := os.Symlink(l.svc, l.link); err != nil {
		return err
	}
	if _, err := l.s6("s6-svscanctl", "-a", l.scan); err != nil {
		return err
	}
	if err := waitFor(10*time.Second, "s6-supervise runs", func() bool {
		_, err := l.s6("s6-svok", l.svc)
		return err == nil
	}); err != nil {
		return err
	}
	if a.Enabled {
		if err := os.Remove(filepath.Join(l.svc, "down")); err != nil {
			return err
		}
	}
	switch a.State {
	case servicetest.Running, servicetest.SlowStop, servicetest.Stubborn:
	default:
		return nil
	}
	if _, err := l.s6("s6-svc", "-u", l.svc); err != nil {
		return err
	}
	return waitFor(10*time.Second, "the run script reports ready", func() bool {
		b, err := os.ReadFile(ready) //nolint:gosec // the lab's own file
		if err != nil {
			return false
		}
		st, err := l.status()
		return err == nil && st.pid > 0 && strings.TrimSpace(string(b)) == strconv.Itoa(st.pid)
	})
}

// s6Status is s6-svstat's view of the service.
type s6Status struct {
	up, wantedUp, normallyUp bool
	pid                      int
}

func (l *s6Lab) status() (s6Status, error) {
	out, err := l.s6("s6-svstat", "-o", "up,wantedup,normallyup,pid", l.svc)
	if err != nil {
		return s6Status{}, err
	}
	f := strings.Fields(string(out))
	if len(f) != 4 {
		return s6Status{}, fmt.Errorf("s6-svstat printed %q", out)
	}
	st := s6Status{up: f[0] == "true", wantedUp: f[1] == "true", normallyUp: f[2] == "true"}
	if st.up {
		if st.pid, err = strconv.Atoi(f[3]); err != nil {
			return s6Status{}, err
		}
	}
	return st, nil
}

// Observe implements servicetest.Lab, through s6-svstat: a stop is under
// way while the service is up and no longer wanted up.
func (l *s6Lab) Observe(context.Context) (servicetest.Observation, error) {
	if _, err := os.Stat(l.svc); errors.Is(err, fs.ErrNotExist) {
		return servicetest.Observation{Detail: "no service directory"}, nil
	}
	st, err := l.status()
	if err != nil {
		return servicetest.Observation{}, err
	}
	return servicetest.Observation{
		Installed: true,
		Running:   st.up,
		Stopping:  st.up && !st.wantedUp,
		Enabled:   st.normallyUp,
		Wanted:    st.wantedUp,
		Detail:    fmt.Sprintf("up=%t wantedup=%t normallyup=%t pid=%d", st.up, st.wantedUp, st.normallyUp, st.pid),
	}, nil
}

// liveS6Conformance: the adapter s, with no Kill, meets the Lifecycle
// contract over the service directory svc, linked into an s6 tree of its
// own. A failed Stop's Up has a bound of its own, so Stop's is twice
// Poll's timeout (0035-MADR Amendment A3); Up documents none, so "start
// hangs" is the one check skipped (0038-PLAN Deviation D7).
func liveS6Conformance(t *testing.T, s *service.Supervised, svc string, pollTimeout time.Duration, tool func(name string) (string, error)) {
	dir := t.TempDir()
	l := &s6Lab{dir: dir, scan: filepath.Join(dir, "scan"), svc: svc, link: filepath.Join(dir, "scan", "demo"), tool: tool}
	if err := os.Mkdir(l.scan, 0o700); err != nil {
		t.Fatal(err)
	}
	svscan, err := tool("s6-svscan")
	if err != nil {
		t.Fatal(err)
	}
	tree := exec.Command(svscan, l.scan) //nolint:gosec // s6-svscan at its installed path, over the test's scan directory
	if err := tree.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := l.remove(); err != nil {
			t.Error(err)
		}
		_ = tree.Process.Signal(os.Interrupt)
		done := make(chan error, 1)
		go func() { done <- tree.Wait() }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = tree.Process.Kill()
			<-done
		}
	})
	var skipped []string
	err = servicetest.VerifyLifecycle(context.Background(), s, servicetest.LifecycleCase{
		Product: "demo", Lab: l, StopBound: 2 * pollTimeout, RestoresAfterFailedStop: true,
		Logf: func(format string, args ...any) { skipped = append(skipped, fmt.Sprintf(format, args...)) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(skipped) != 1 || !strings.HasPrefix(skipped[0], "servicetest: start hangs: skipped") {
		t.Fatalf("skipped %q; want only start hangs", skipped)
	}
}
