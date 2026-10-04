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
	code := hop(func() (string, error) { return "/opt/demo/demo", nil }, []string{"update", "-y"}, environ, start)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if gotExe != "/opt/demo/demo" || !slices.Equal(gotArgs, []string{"update", "-y"}) {
		t.Fatalf("started %s %q", gotExe, gotArgs)
	}
	if !slices.Equal(gotEnv, []string{"A=1", EnvHandOff + "=id", "SECRET=s"}) {
		t.Fatalf("the run's environment %q; want the hop's without %s", gotEnv, EnvHandOffHop)
	}
}

func TestHandOffHopReportsFailure(t *testing.T) {
	result := filepath.Join(t.TempDir(), "result")
	environ := []string{EnvHandOffHop + "=1", EnvHandOff + "=abc", EnvHandOffResult + "=" + result}
	start := func(string, []string, []string) (int, error) { return 0, errors.New("no such file") }
	if code := hop(func() (string, error) { return "/opt/demo/demo", nil }, nil, environ, start); code != 1 {
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
	if code := hop(func() (string, error) { return "/opt/demo/demo", nil }, nil, gone, start); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	// No result path: still exit 1, and nothing to write.
	if code := hop(func() (string, error) { return "", errors.New("no executable") }, nil, []string{EnvHandOffHop + "=1"}, start); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
}

func TestHandOffHopIdleWithoutMarker(t *testing.T) {
	t.Setenv(EnvHandOffHop, "")
	HandOffHop() // returns: no marker
	if err := LoadHandOffEnv(); err != nil {
		t.Fatal(err)
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
	goAhead := filepath.Join(dir, "go")
	cmd.Env = append(os.Environ(), EnvHandOffHop+"=1", fakeEnv+"=hoprun", "FAKE_OUT="+out, "FAKE_GO="+goAhead)
	start := time.Now()
	if err := cmd.Run(); err != nil {
		t.Fatalf("the hop: %v", err)
	}
	hopPID := cmd.Process.Pid
	if took := time.Since(start); took > 10*time.Second {
		t.Fatalf("the hop took %v; it must exit at once", took)
	}
	// The hop is reaped, so Unix has already given its child to a new
	// parent: only now may the run read its own.
	if err := os.WriteFile(goAhead, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if !waitForFile(out) {
		t.Fatal("the real run never reported")
	}
	body, err := os.ReadFile(out) //nolint:gosec // the test's own file, written by rename
	if err != nil {
		t.Fatal(err)
	}
	ppid, marked, _ := strings.Cut(string(body), " ")
	if marked != "false" {
		t.Fatalf("the real run sees %s: %q", EnvHandOffHop, body)
	}
	if ppid == strconv.Itoa(os.Getpid()) {
		t.Fatal("the real run is this process's child")
	}
	// Its recorded parent is the hop, which has exited: Windows keeps the
	// link, and Unix gives the run to whoever adopts orphans.
	hop := strconv.Itoa(hopPID)
	if (runtime.GOOS == "windows") != (ppid == hop) {
		t.Fatalf("on %s the real run's parent is %s, the hop %s", runtime.GOOS, ppid, hop)
	}
}
