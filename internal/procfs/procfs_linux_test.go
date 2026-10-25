//go:build linux

package procfs

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/internal/testproc"
)

// Tests for ReadStat and Exe against this host's /proc
// (docs/decisions/0035-PLAN-serve-supervisors-through-a-supervised-adapter.md
// M2; 0035-MADR Amendment A1, P1).

func TestReadStatSelf(t *testing.T) {
	s, err := ReadStat(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if s.Parent != os.Getppid() || s.StartTime == 0 || s.Zombie() {
		t.Fatalf("this process: %+v; want parent %d, a start time, and not a zombie", s, os.Getppid())
	}
	time.Sleep(20 * time.Millisecond)
	again, err := ReadStat(os.Getpid())
	if err != nil || again.StartTime != s.StartTime {
		t.Fatalf("the start time moved: %d, then %+v, %v", s.StartTime, again, err)
	}
}

func TestExeSelf(t *testing.T) {
	got, err := Exe(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if want, err = filepath.EvalSymlinks(want); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("Exe = %q, want %q", got, want)
	}
}

// TestReadStatChild: a child's parent is this process, and its start time
// is its own.
func TestReadStatChild(t *testing.T) {
	ready := filepath.Join(t.TempDir(), "ready")
	cmd := exec.Command(os.Args[0]) //nolint:gosec // the test binary as a helper
	cmd.Env = append(os.Environ(), helperEnv+"=wait", "PROCFS_READY="+ready)
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	p := testproc.Start(t, cmd, ready)
	defer func() {
		_ = in.Close()
		_ = p.Wait()
	}()
	child, err := ReadStat(cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	self, err := ReadStat(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	// Start time is in clock ticks, so a child started within this
	// process's first tick has the same one; it is never earlier
	// (0035-PLAN-serve-supervisors-through-a-supervised-adapter.md M10).
	if child.Parent != os.Getpid() || child.StartTime == 0 || child.StartTime < self.StartTime || child.Zombie() {
		t.Fatalf("child %+v; this process %+v", child, self)
	}
}

// TestReadStatZombieAndGone: a child that has exited reads as a zombie until
// it is reaped, and then as fs.ErrNotExist, for ReadStat and Exe alike.
func TestReadStatZombieAndGone(t *testing.T) {
	cmd := exec.Command(os.Args[0]) //nolint:gosec // the test binary as a helper
	cmd.Env = append(os.Environ(), helperEnv+"=exit")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	deadline := time.Now().Add(testproc.Budget(t))
	for {
		s, err := ReadStat(pid)
		if err == nil && s.Zombie() {
			break
		}
		if err != nil || time.Now().After(deadline) {
			_ = cmd.Wait()
			t.Fatalf("the exited child never read as a zombie: %+v, %v", s, err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadStat(pid); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("ReadStat of a reaped process: %v, want fs.ErrNotExist", err)
	}
	if _, err := Exe(pid); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Exe of a reaped process: %v, want fs.ErrNotExist", err)
	}
}

func TestInvalidPID(t *testing.T) {
	for _, pid := range []int{0, -1} {
		if _, err := ReadStat(pid); err == nil {
			t.Errorf("ReadStat(%d) succeeded", pid)
		}
		if _, err := Exe(pid); err == nil {
			t.Errorf("Exe(%d) succeeded", pid)
		}
	}
}
