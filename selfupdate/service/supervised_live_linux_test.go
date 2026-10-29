//go:build linux

package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The live test of the supervised adapter over a real s6 tree
// (docs/decisions/0035-PLAN-serve-supervisors-through-a-supervised-adapter.md
// M6; 0035-MADR D14-A). It runs only with SELFUPDATE_REQUIRE_SUPERVISED=1,
// and fails rather than skips when it then cannot run. The test starts its
// own s6-svscan, as the current user, over a scan directory in its
// temporary directory; the service runs a copy of this test binary in the
// "s6service" mode.

const liveSupervisedEnv = "SELFUPDATE_REQUIRE_SUPERVISED"

// liveTimeout bounds the adapter's waits in the live test.
var livePoll = PollOptions{Interval: 100 * time.Millisecond, Timeout: 30 * time.Second, Settle: 2 * time.Second}

// s6Tool finds an s6 tool where s6 installs it, never on PATH.
func s6Tool(name string) (string, error) {
	for _, dir := range []string{"/usr/bin", "/bin", "/command", "/usr/local/bin"} {
		p := filepath.Join(dir, name)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("%s is not installed", name)
}

// LiveS6Conformance runs servicetest over s, the test's adapter with no
// Kill, over the service directory svc, which it creates and links into
// an s6 tree of its own; s's Poll timeout is pollTimeout, and tool is
// s6Tool. supervised_live_conformance_linux_test.go sets it: this package
// cannot import servicetest, which imports it
// (docs/decisions/0038-PLAN-pinned-plans-lifecycle-conformance-and-bubble-tests.md
// Deviation D8).
var LiveS6Conformance func(t *testing.T, s *Supervised, svc string, pollTimeout time.Duration, tool func(name string) (string, error))

// s6Supervised is the adapter over the s6 service directory svc, with the
// test's reference State over s6-svstat -o, the one documented machine
// form (0035-MADR F17; Amendment A1, P2), and Up, Down and Kill over
// s6-svc.
func s6Supervised(svc string) (*Supervised, error) {
	return s6SupervisedWith(svc, livePoll, true)
}

// s6SupervisedWith is s6Supervised with poll, and with Kill only when kill
// is set. A service directory that does not exist is not installed.
func s6SupervisedWith(svc string, poll PollOptions, kill bool) (*Supervised, error) {
	svstat, err := s6Tool("s6-svstat")
	if err != nil {
		return nil, err
	}
	s6svc, err := s6Tool("s6-svc")
	if err != nil {
		return nil, err
	}
	run := ExecRunner()
	state := func(ctx context.Context) (SupervisedState, error) {
		_, serr := os.Stat(svc)
		_, derr := os.Stat(filepath.Join(svc, "down"))
		st := SupervisedState{Installed: serr == nil, NormallyUp: errors.Is(derr, fs.ErrNotExist)}
		if errors.Is(serr, fs.ErrNotExist) {
			return SupervisedState{}, ctx.Err()
		}
		out, err := run.Run(ctx, Command{Path: svstat, Args: []string{"-o", "up,wantedup,pid", svc}, Env: os.Environ()})
		if err != nil {
			return st, err
		}
		switch out.ExitCode {
		case 0:
		case 1:
			return st, fmt.Errorf("s6-svstat: %s: %w", bytes.TrimSpace(out.Stderr), ErrNoSupervisor)
		default:
			return st, fmt.Errorf("s6-svstat exit %d: %s", out.ExitCode, bytes.TrimSpace(out.Stderr))
		}
		f := strings.Fields(string(out.Stdout))
		if len(f) != 3 {
			return st, fmt.Errorf("s6-svstat printed %q", out.Stdout)
		}
		st.Supervised, st.Up, st.WantUp = true, f[0] == "true", f[1] == "true"
		st.Restarting = !st.Up && st.WantUp
		if st.Up {
			if st.PID, err = strconv.Atoi(f[2]); err != nil {
				return st, err
			}
		}
		return st, nil
	}
	ctl := func(flag string) func(context.Context) error {
		return func(ctx context.Context) error {
			out, err := run.Run(ctx, Command{Path: s6svc, Args: []string{flag, svc}, Env: os.Environ()})
			if err == nil && out.ExitCode != 0 {
				err = fmt.Errorf("s6-svc %s exit %d: %s", flag, out.ExitCode, bytes.TrimSpace(out.Stderr))
			}
			return err
		}
	}
	o := SupervisedOptions{Name: "s6 live", State: state, Up: ctl("-u"), Down: ctl("-d"), Poll: poll}
	if kill {
		o.Kill = ctl("-k")
	}
	return NewSupervised(o)
}

// supervisedLiveMode runs this binary as the live service ("s6service") or
// as the detached update a handoff starts ("s6update").
func supervisedLiveMode(mode string) (int, bool) {
	switch mode {
	case "s6service":
		return s6Service(), true
	case "s6update":
		return s6Update(), true
	}
	return 0, false
}

// s6Service is the service's process. It exits 3 at once while FAKE_DIR/crash
// exists. Otherwise it runs until it is stopped, and on FAKE_DIR/trigger
// hands an update off from inside the service, as an agent it ran would.
func s6Service() int {
	dir := os.Getenv("FAKE_DIR")
	if _, err := os.Stat(filepath.Join(dir, "crash")); err == nil {
		return 3
	}
	for {
		trigger := filepath.Join(dir, "trigger")
		if _, err := os.Stat(trigger); err == nil {
			_ = os.Remove(trigger)
			note := "handed: "
			s, err := s6Supervised(filepath.Join(dir, "scan", "demo"))
			if err == nil {
				var det Detached
				var handed bool
				det, handed, err = HandOffIfInside(context.Background(), s, HandOff{
					Env:        []string{fakeEnv + "=s6update", "FAKE_DIR=" + dir},
					ResultPath: filepath.Join(dir, "handoff-result"),
				})
				note += strconv.FormatBool(handed) + " " + det.Where
			}
			if err != nil {
				note = "error: " + err.Error()
			}
			_ = os.WriteFile(filepath.Join(dir, "handed"), []byte(note), 0o600)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// s6Update is the detached run: it stops the service from outside, swaps
// the binary for FAKE_DIR/next, starts it, and waits for it to be healthy,
// writing "ok" or the error to FAKE_DIR/update-result.
func s6Update() int {
	dir := os.Getenv("FAKE_DIR")
	result := func(text string, code int) int {
		_ = os.WriteFile(filepath.Join(dir, "update-result"), []byte(text), 0o600)
		return code
	}
	s, err := s6Supervised(filepath.Join(dir, "scan", "demo"))
	if err != nil {
		return result("error: "+err.Error(), 1)
	}
	ctx := context.Background()
	if in, err := s.Inside(ctx); err != nil || in {
		return result(fmt.Sprintf("error: the detached run is inside: %t, %v", in, err), 1)
	}
	target := filepath.Join(dir, "demo")
	steps := []func() error{
		func() error { return s.Stop(ctx, "demo") },
		func() error { return replace(filepath.Join(dir, "next"), target) },
		func() error { _, err := s.Reconcile(ctx, "demo", target); return err },
		func() error { return s.Start(ctx, "demo") },
		func() error { return s.WaitHealthy(ctx, "demo") },
	}
	for i, step := range steps {
		if err := step(); err != nil {
			return result(fmt.Sprintf("error: step %d: %v", i+1, err), 1)
		}
	}
	return result("ok", 0)
}

// replace copies src over dst by rename, as an update does.
func replace(src, dst string) error {
	body, err := os.ReadFile(src) //nolint:gosec // the live test's own file
	if err != nil {
		return err
	}
	if err := os.WriteFile(dst+".tmp", body, 0o755); err != nil { //nolint:gosec // an executable fixture
		return err
	}
	return os.Rename(dst+".tmp", dst)
}

// copyBuild writes this test binary to dst with marker appended, so a
// process's binary can be told apart by its tail.
func copyBuild(t *testing.T, dst, marker string) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(self) //nolint:gosec // the test binary
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, append(body, marker...), 0o755); err != nil { //nolint:gosec // an executable fixture
		t.Fatal(err)
	}
}

// runsBuild reports whether process pid runs the build marked marker, read
// through /proc/<pid>/exe, which names the binary even once it is replaced.
func runsBuild(t *testing.T, pid int, marker string) bool {
	t.Helper()
	f, err := os.Open(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	tail := make([]byte, len(marker))
	if _, err := f.ReadAt(tail, fi.Size()-int64(len(marker))); err != nil && !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
	return string(tail) == marker
}

// waitFor polls cond every 100 ms until it holds, or fails t after limit.
func waitFor(t *testing.T, limit time.Duration, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(limit); !cond(); time.Sleep(100 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("%s: not within %s", what, limit)
		}
	}
}

func TestLiveSupervisedS6(t *testing.T) {
	if os.Getenv(liveSupervisedEnv) != "1" {
		t.Skipf("set %s=1 to run the adapter over a real s6 tree; CI sets it on its Linux runner", liveSupervisedEnv)
	}
	svscan, err := s6Tool("s6-svscan")
	if err != nil {
		t.Fatal(err)
	}
	s6svc, err := s6Tool("s6-svc")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	dir := t.TempDir()
	scan := filepath.Join(dir, "scan")
	svc := filepath.Join(scan, "demo")
	if err := os.MkdirAll(svc, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "demo")
	copyBuild(t, target, "\nold build\n")
	run := fmt.Sprintf("#!/bin/sh\nexec env %s=s6service FAKE_DIR=%s %s\n", fakeEnv, dir, target)
	if err := os.WriteFile(filepath.Join(svc, "run"), []byte(run), 0o755); err != nil { //nolint:gosec // the service's run script
		t.Fatal(err)
	}
	tree := exec.Command(svscan, scan) //nolint:gosec // s6-svscan at its installed path, over the test's scan directory
	if err := tree.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// -k first: s6-svscan does not exit while a service it supervises
		// ignores TERM (0035-MADR Amendment A1, P2).
		for _, flag := range []string{"-k", "-dx"} {
			_ = exec.Command(s6svc, flag, svc).Run() //nolint:gosec // s6-svc at its installed path
		}
		time.Sleep(time.Second)
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
	s, err := s6Supervised(svc)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, 15*time.Second, "the service running", func() bool {
		up, err := s.Running(ctx, "demo")
		return err == nil && up
	})
	first, err := s.Status(ctx)
	if err != nil || first.Instance == "" {
		t.Fatalf("Status: %+v, %v", first, err)
	}
	if in, err := s.Inside(ctx); err != nil || in {
		t.Fatalf("the test process reads as inside: %t, %v", in, err)
	}

	// An update's cycle, from outside.
	if err := s.Stop(ctx, "demo"); err != nil {
		t.Fatal(err)
	}
	copyBuild(t, filepath.Join(dir, "next"), "\nnew build\n")
	if err := replace(filepath.Join(dir, "next"), target); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Reconcile(ctx, "demo", target); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if err := s.Start(ctx, "demo"); err != nil {
		t.Fatal(err)
	}
	if err := s.WaitHealthy(ctx, "demo"); err != nil {
		t.Fatalf("WaitHealthy after the cycle: %v", err)
	}
	second, err := s.Status(ctx)
	if err != nil || second.Instance == first.Instance || !runsBuild(t, second.PID, "\nnew build\n") {
		t.Fatalf("after the cycle: %+v, %v; want a new instance of the new build", second, err)
	}

	// A crash loop fails the health wait.
	if err := s.Stop(ctx, "demo"); err != nil {
		t.Fatal(err)
	}
	crash := filepath.Join(dir, "crash")
	if err := os.WriteFile(crash, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(ctx, "demo"); err != nil {
		t.Fatal(err)
	}
	if err := s.WaitHealthy(ctx, "demo"); !errors.Is(err, ErrUnhealthy) {
		t.Fatalf("WaitHealthy over a crash loop: %v, want ErrUnhealthy", err)
	}
	if err := os.Remove(crash); err != nil {
		t.Fatal(err)
	}
	if err := s.WaitHealthy(ctx, "demo"); err != nil {
		t.Fatalf("WaitHealthy once the crash ended: %v", err)
	}

	// An update from inside the service hands off, and the detached run
	// stops, replaces, starts and checks the service from outside.
	copyBuild(t, filepath.Join(dir, "next"), "\nhanded-off build\n")
	if err := os.WriteFile(filepath.Join(dir, "trigger"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	result := filepath.Join(dir, "update-result")
	waitFor(t, 90*time.Second, "the handed-off update's result", func() bool {
		_, err := os.Stat(result)
		return err == nil
	})
	body, _ := os.ReadFile(result)                         //nolint:gosec // the live test's own file
	handed, _ := os.ReadFile(filepath.Join(dir, "handed")) //nolint:gosec // the live test's own file
	if string(body) != "ok" || !strings.Contains(string(handed), "handed: true") || !strings.Contains(string(handed), "through the hop") {
		t.Fatalf("handoff: result %q, service %q", body, handed)
	}
	third, err := s.Status(ctx)
	if err != nil || !third.Up || !runsBuild(t, third.PID, "\nhanded-off build\n") {
		t.Fatalf("after the handoff: %+v, %v; want the handed-off build running", third, err)
	}

	// The adapter meets the Lifecycle contract (0038-PLAN Q12).
	t.Run("conformance", func(t *testing.T) {
		if LiveS6Conformance == nil {
			t.Fatal("LiveS6Conformance is not set: supervised_live_conformance_linux_test.go sets it")
		}
		svc := filepath.Join(t.TempDir(), "demo")
		poll := PollOptions{Interval: 100 * time.Millisecond, Timeout: 10 * time.Second, Settle: 2 * time.Second}
		s, err := s6SupervisedWith(svc, poll, false)
		if err != nil {
			t.Fatal(err)
		}
		LiveS6Conformance(t, s, svc, poll.Timeout, s6Tool)
	})
}
