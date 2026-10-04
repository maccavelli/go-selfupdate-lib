//go:build windows

package service

import (
	"os/exec"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// newKillJob makes a job object that kills its processes when terminated,
// allowing breakaway or not.
func newKillJob(t *testing.T, breakaway bool) windows.Handle {
	t.Helper()
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = windows.CloseHandle(job) })
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if breakaway {
		info.BasicLimitInformation.LimitFlags |= windows.JOB_OBJECT_LIMIT_BREAKAWAY_OK
	}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		t.Fatal(err)
	}
	return job
}

// TestDetachProcessLeavesJob: a child DetachProcess started outlives its
// parent's job when the job allows breakaway. When it does not, the start
// is retried inside the job, and the child dies with it: that case is
// pinned as found (0011-PLAN V1 step 8).
func TestDetachProcessLeavesJob(t *testing.T) {
	for _, c := range []struct {
		name      string
		breakaway bool
		wantAlive bool
	}{
		{"breakaway allowed", true, true},
		{"breakaway refused", false, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			wait, goFile, alive := filepath.Join(dir, "wait"), filepath.Join(dir, "go"), filepath.Join(dir, "alive")
			job := newKillJob(t, c.breakaway)
			cmd := exec.Command(selfExe(t))
			childPID := startParent(t, cmd, "FAKE_PARENT_WAIT="+wait, "FAKE_GO="+goFile, "FAKE_ALIVE="+alive)
			h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = windows.CloseHandle(h) }()
			if err := windows.AssignProcessToJobObject(job, h); err != nil {
				t.Fatal(err)
			}
			if err := writeEmpty(wait); err != nil {
				t.Fatal(err)
			}
			pid := childPID()
			t.Cleanup(func() { killPID(pid) })
			if err := windows.TerminateJobObject(job, 1); err != nil {
				t.Fatal(err)
			}
			_ = cmd.Wait()
			if got := childOutlived(t, goFile, alive); got != c.wantAlive {
				t.Fatalf("child alive after the job ended: %t, want %t", got, c.wantAlive)
			}
		})
	}
}

func writeEmpty(path string) error {
	f, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	h, err := windows.CreateFile(f, windows.GENERIC_WRITE, 0, nil, windows.CREATE_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return err
	}
	return windows.CloseHandle(h)
}

func killPID(pid int) {
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return
	}
	_ = windows.TerminateProcess(h, 1)
	_ = windows.CloseHandle(h)
}
