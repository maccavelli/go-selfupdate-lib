//go:build linux

package service

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/internal/procfs"
	"github.com/maccavelli/go-selfupdate-lib/internal/testproc"
)

// Tests for the supervised adapter against this host's real processes
// (docs/decisions/0035-PLAN-serve-supervisors-through-a-supervised-adapter.md
// M3): this test binary descends from its parent, and not from a child of
// its own.

func realSupervised(t *testing.T, pid int) *Supervised {
	t.Helper()
	s, err := NewSupervised(SupervisedOptions{
		Name:  "real",
		State: states(SupervisedState{Installed: true, Supervised: true, Up: true, PID: pid}, nil),
		Up:    nop,
		Down:  nop,
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSupervisedInsideReal(t *testing.T) {
	testproc.CheckNoLeak(t)
	ctx := context.Background()
	child := exec.Command(os.Args[0]) //nolint:gosec // the test binary as a helper
	child.Env = append(os.Environ(), fakeEnv+"=sleep")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = child.Process.Kill()
		_ = child.Wait()
	})
	for name, c := range map[string]struct {
		pid  int
		want bool
	}{
		"this process":       {os.Getpid(), true},
		"its parent":         {os.Getppid(), true},
		"a child of its own": {child.Process.Pid, false},
	} {
		if got, err := realSupervised(t, c.pid).Inside(ctx); err != nil || got != c.want {
			t.Errorf("%s (pid %d): %t, %v; want %t", name, c.pid, got, err, c.want)
		}
	}
}

// TestSupervisedHandOffReal: this test process plays the service's
// process, so HandOffIfInside finds itself inside and detaches a real run,
// which outlives the call and writes its environment.
func TestSupervisedHandOffReal(t *testing.T) {
	testproc.CheckNoLeak(t)
	out := filepath.Join(t.TempDir(), "env")
	det, handed, err := HandOffIfInside(context.Background(), realSupervised(t, os.Getpid()), HandOff{
		Executable: os.Args[0],
		Env:        []string{fakeEnv + "=envfile", "FAKE_OUT=" + out},
		ResultPath: filepath.Join(t.TempDir(), "result"),
	})
	if err != nil || !handed || !strings.Contains(det.Where, "through the hop process ") {
		t.Fatalf("HandOffIfInside: %+v, %t, %v", det, handed, err)
	}
	var body []byte
	for deadline := time.Now().Add(testproc.Budget(t)); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if body, err = os.ReadFile(out); err == nil && len(body) > 0 { //nolint:gosec // the test's own file
			break
		}
	}
	if !strings.Contains(string(body), EnvHandOff+"=") {
		t.Fatalf("the detached run's environment lacks %s: %q", EnvHandOff, body)
	}
}

func TestSupervisedStatusReal(t *testing.T) {
	testproc.CheckNoLeak(t)
	st, err := realSupervised(t, os.Getpid()).Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ps, err := procfs.ReadStat(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	want := strconv.Itoa(os.Getpid()) + "@" + strconv.FormatUint(ps.StartTime, 10)
	if st.Instance != want || !strings.Contains(st.Instance, "@") {
		t.Fatalf("Instance %q, want %q", st.Instance, want)
	}
}
