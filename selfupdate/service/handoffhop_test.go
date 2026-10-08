package service

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Tests for docs/decisions/0011-MADR-reference-service-lifecycles.md
// amendment A4: the hop that starts the real detached run and exits.

func TestHandOffHopStartsRun(t *testing.T) {
	var gotExe string
	var gotArgs, gotEnv []string
	start := func(exe string, args, env []string) (int, error) {
		gotExe, gotArgs, gotEnv = exe, args, env
		return 4242, nil
	}
	environ := []string{"A=1", EnvHandOffHop + "=1", EnvHandOff + "=id", "SECRET=s"}
	code := hop(func() (string, error) { return "/opt/demo/demo", nil }, []string{"update", "-y"}, environ, 321, start)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if gotExe != "/opt/demo/demo" || !slices.Equal(gotArgs, []string{"update", "-y"}) {
		t.Fatalf("started %s %q", gotExe, gotArgs)
	}
	if !slices.Equal(gotEnv, []string{"A=1", EnvHandOff + "=id", "SECRET=s", EnvHandOffHopParent + "=321"}) {
		t.Fatalf("the run's environment %q; want the hop's without %s, with its PID", gotEnv, EnvHandOffHop)
	}
}

func TestHandOffHopReportsFailure(t *testing.T) {
	result := filepath.Join(t.TempDir(), "result")
	environ := []string{EnvHandOffHop + "=1", EnvHandOff + "=abc", EnvHandOffResult + "=" + result}
	start := func(string, []string, []string) (int, error) { return 0, errors.New("no such file") }
	if code := hop(func() (string, error) { return "/opt/demo/demo", nil }, nil, environ, 1, start); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	r, err := ReadHandOffResult(result)
	if err != nil {
		t.Fatal(err)
	}
	if r.ID != "abc" || r.ExitCode == 0 || !strings.Contains(r.Error, "hop") || !strings.Contains(r.Error, "no such file") {
		t.Fatalf("result %+v", r)
	}
	// A result that cannot be written either: exit 2.
	gone := []string{EnvHandOffHop + "=1", EnvHandOffResult + "=" + filepath.Join(t.TempDir(), "missing", "result")}
	if code := hop(func() (string, error) { return "/opt/demo/demo", nil }, nil, gone, 1, start); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	// No result path: still exit 1, and nothing to write.
	if code := hop(func() (string, error) { return "", errors.New("no executable") }, nil, []string{EnvHandOffHop + "=1"}, 1, start); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
}

func TestHandOffHopIdleWithoutMarker(t *testing.T) {
	t.Setenv(EnvHandOffHop, "")
	if err := HandOffHop(); err != nil { // returns: no marker
		t.Fatal(err)
	}
	if err := LoadHandOffEnv(); err != nil {
		t.Fatal(err)
	}
}

// TestWaitHopExit: the real run waits for a hop that has not exited yet,
// here one that lives 300 ms after the run says it is waiting, however
// long the run took to start
// (docs/decisions/0017-PLAN-verify-build-provenance-and-close-0015-open-items.md
// Q0).
func TestWaitHopExit(t *testing.T) {
	out := filepath.Join(t.TempDir(), "child")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(), fakeEnv+"=hopparent", "FAKE_OUT="+out)
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if !waitForFile(out) {
		t.Fatal("the child never reported")
	}
	body, err := os.ReadFile(out) //nolint:gosec // the test's own file, written by rename
	if err != nil {
		t.Fatal(err)
	}
	ms, running, _ := strings.Cut(string(body), " ")
	waited, err := strconv.Atoi(ms)
	if err != nil || waited < 200 || running != "false" {
		t.Fatalf("the child waited %s ms, its parent still running %s; want at least 200 ms, and gone", ms, running)
	}
}

// TestHandOffHopProcess runs the test binary as a hop (TestMain calls
// HandOffHop first, as a program would). The hop exits at once; the real
// run is not this process's child, and its environment lacks the marker.
func TestHandOffHopProcess(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "run")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(), EnvHandOffHop+"=1", fakeEnv+"=hoprun", "FAKE_OUT="+out)
	start := time.Now()
	if err := cmd.Run(); err != nil {
		t.Fatalf("the hop: %v", err)
	}
	hopPID := cmd.Process.Pid
	if took := time.Since(start); took > 10*time.Second {
		t.Fatalf("the hop took %v; it must exit at once", took)
	}
	if !waitForFile(out) {
		t.Fatal("the real run never reported")
	}
	body, err := os.ReadFile(out) //nolint:gosec // the test's own file, written by rename
	if err != nil {
		t.Fatal(err)
	}
	f := strings.Fields(string(body))
	if len(f) != 3 {
		t.Fatalf("the real run reported %q", body)
	}
	ppid, marked, running := f[0], f[1], f[2]
	if marked != "false" {
		t.Fatalf("the real run sees the hop's variables: %q", body)
	}
	if ppid == strconv.Itoa(os.Getpid()) {
		t.Fatal("the real run is this process's child")
	}
	// Its recorded parent is the hop, which had exited before the run
	// went on: Windows keeps the link, and Unix gives the run to whoever
	// adopts orphans. The run reported at once, so its LoadHandOffEnv
	// waited for that (0011-MADR amendment A4).
	hop := strconv.Itoa(hopPID)
	if (runtime.GOOS == "windows") != (ppid == hop) {
		t.Fatalf("on %s the real run's parent is %s, the hop %s", runtime.GOOS, ppid, hop)
	}
	if runtime.GOOS == "windows" && running != "false" {
		t.Fatalf("the hop %s was still running when the real run went on", hop)
	}
}
