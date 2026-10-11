//go:build windows

package taskscheduler

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unsafe"
)

func TestVARIANTSize(t *testing.T) {
	if n := unsafe.Sizeof(variant{}); n != 24 {
		t.Fatalf("sizeof(variant) = %d, want 24 on amd64/arm64", n)
	}
}

func TestCOMLayout(t *testing.T) {
	exe, name, dir, r := newLayoutTask(t)
	_, err := systemScheduler().open(`\`, "no-such-"+name)
	if !errors.Is(err, hrFileNotFound) {
		t.Fatalf("missing: %v, want hrFileNotFound (init HRESULT 0x%08X)", err, uint32(lastCoInit))
	}
	t.Logf("CoInitializeEx HRESULT 0x%08X", uint32(lastCoInit))
	never, err := r.lastRun()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("never-run DATE %v LastTaskResult 0x%08X", never.Time, uint32(never.Result)) //nolint:gosec // log the HRESULT-shaped result
	saved := checkDefinition(t, r, exe)
	// An idle task's instances (0021-MADR Amendment A4).
	t.Logf("never-run GetInstances HRESULT 0x%08X", uint32(rawTaskCall(t, name, taskGetInstances)))
	if pids, err := r.instances(); err != nil || len(pids) != 0 {
		t.Fatalf("instances of a never-run task: %v, %v; want none", pids, err)
	}
	waitInstances(t, r, 1)
	toggleEnabled(t, r)
	if err := r.stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	waitInstances(t, r, 0)
	// Stop with nothing left to stop (0021-MADR Amendment A4).
	t.Logf("idle Stop HRESULT 0x%08X", uint32(rawTaskCall(t, name, taskStop)))
	if err := r.stop(); err != nil {
		t.Fatalf("stop of an idle task: %v", err)
	}
	rewriteAndRestore(t, r, dir, exe, saved)
}

// rawTaskCall calls GetInstances or Stop on the task with flags 0, and
// returns its HRESULT unmapped.
func rawTaskCall(t *testing.T, name string, slot int) hresult {
	t.Helper()
	var hr hresult
	err := withTask(`\`, name, func(task, _ *comObj) error {
		if slot == taskStop {
			hr = task.call(taskStop, 0)
			return nil
		}
		var col *comObj
		hr = task.call(slot, 0, ptr(&col))
		col.release()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hr
}

func newLayoutTask(t *testing.T) (exe, name, dir string, r registered) {
	t.Helper()
	exe = filepath.Join(os.Getenv("WINDIR"), "System32", "timeout.exe")
	if _, err := os.Stat(exe); err != nil {
		t.Fatal(err)
	}
	name = "selfupdate-probe-" + randomHex()
	dir = t.TempDir()
	xml := taskXML(exe, "/t 30 /nobreak")
	if err := comRegister(name, xml); err != nil {
		if errors.Is(err, hrAccessDenied) && os.Getenv("CI") != "" {
			t.Skipf("register %s: %v", name, err)
		}
		t.Fatalf("create %s: %v", name, err)
	}
	t.Cleanup(func() { _ = comDelete(name) })
	s := systemScheduler()
	if s == nil {
		t.Fatal("no transport")
	}
	var err error
	r, err = s.open(`\`, name)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = r.close() })
	return exe, name, dir, r
}

func checkDefinition(t *testing.T, r registered, exe string) string {
	t.Helper()
	st, err := r.state()
	if err != nil || (st != stateReady && st != stateDisabled) {
		t.Fatalf("get_State: %d, %v", st, err)
	}
	on, err := r.enabled()
	if err != nil || !on {
		t.Fatalf("enabled %t, %v", on, err)
	}
	trigs, err := r.triggers()
	if err != nil || len(trigs) != 1 || trigs[0].Type != triggerLogon || !trigs[0].Enabled {
		t.Fatalf("triggers %+v, %v", trigs, err)
	}
	acts, err := r.actions()
	if err != nil || len(acts) != 1 || acts[0].Type != actionExec || !strings.EqualFold(acts[0].Path, exe) {
		t.Fatalf("actions %+v, %v", acts, err)
	}
	logon, err := r.logonType()
	if err != nil || logon != logonInteractiveToken {
		t.Fatalf("logon %d, %v", logon, err)
	}
	xml, err := r.xml()
	if err != nil || !strings.Contains(strings.ToLower(xml), strings.ToLower(exe)) {
		t.Fatalf("xml %q, %v", xml, err)
	}
	return xml
}

func waitInstances(t *testing.T, r registered, want int) {
	t.Helper()
	if want > 0 {
		if err := r.run(); err != nil {
			t.Fatalf("run: %v", err)
		}
	}
	deadline := time.Now().Add(10 * time.Second)
	var pids []uint32
	var st int32
	var err error
	for time.Now().Before(deadline) {
		pids, err = r.instances()
		if err != nil {
			t.Fatal(err)
		}
		st, err = r.state()
		if err != nil {
			t.Fatal(err)
		}
		if want == 0 && len(pids) == 0 && st != stateRunning {
			return
		}
		if want > 0 && len(pids) == want && pids[0] != 0 {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("instances want %d: pids %v state %d", want, pids, st)
}

func toggleEnabled(t *testing.T, r registered) {
	t.Helper()
	if err := r.setEnabled(false); err != nil {
		t.Fatal(err)
	}
	on, err := r.enabled()
	if err != nil || on {
		t.Fatalf("disabled: %t, %v", on, err)
	}
	if err := r.setEnabled(true); err != nil {
		t.Fatal(err)
	}
	on, err = r.enabled()
	if err != nil || !on {
		t.Fatalf("enabled again: %t, %v", on, err)
	}
}

func rewriteAndRestore(t *testing.T, r registered, dir, exe, saved string) {
	t.Helper()
	nonASCII := filepath.Join(dir, "café-über-任务")
	if err := os.Mkdir(nonASCII, 0o700); err != nil {
		t.Fatal(err)
	}
	copied := filepath.Join(nonASCII, "timeout.exe")
	body, err := os.ReadFile(exe) //nolint:gosec // Windows System32 timeout.exe
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(copied, body, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := r.setPath(copied); err != nil {
		t.Fatalf("setPath: %v", err)
	}
	xml, err := r.xml()
	if err != nil || !strings.Contains(xml, "café") {
		t.Fatalf("xml after setPath %q, %v", xml, err)
	}
	acts, err := r.actions()
	if err != nil || len(acts) != 1 || !strings.EqualFold(acts[0].Path, copied) {
		t.Fatalf("path after setPath %+v, %v", acts, err)
	}
	if logon, err := r.logonType(); err != nil || logon != logonInteractiveToken {
		t.Fatalf("logon after setPath %d, %v", logon, err)
	}
	if err := r.restore(saved); err != nil {
		t.Fatalf("restore: %v", err)
	}
	xml, err = r.xml()
	if err != nil || xml != saved {
		t.Fatalf("xml after restore differs: len %d vs %d, %v", len(xml), len(saved), err)
	}
}

func comRegister(name, xml string) error {
	return withFolder(`\`, func(f *comObj) error {
		nb, err := sysString(name)
		if err != nil {
			return err
		}
		defer freeBSTR(nb)
		xb, err := sysString(xml)
		if err != nil {
			return err
		}
		defer freeBSTR(xb)
		var empty [3]variant
		var out *comObj
		hr := f.call(folderRegisterTask, nb, xb, uintptr(taskCreateOrUpdate),
			ptr(&empty[0]), ptr(&empty[1]), uintptr(logonInteractiveToken),
			ptr(&empty[2]), ptr(&out))
		out.release()
		return checkHR(hr)
	})
}

func comDelete(name string) error {
	return withFolder(`\`, func(f *comObj) error {
		nb, err := sysString(name)
		if err != nil {
			return err
		}
		defer freeBSTR(nb)
		return checkHR(f.call(folderDeleteTask, nb, 0))
	})
}

func taskXML(exe, args string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <Principals>
    <Principal id="Author">
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>LeastPrivilege</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <Enabled>true</Enabled>
  </Settings>
  <Triggers>
    <LogonTrigger>
      <Enabled>true</Enabled>
    </LogonTrigger>
  </Triggers>
  <Actions Context="Author">
    <Exec>
      <Command>%s</Command>
      <Arguments>%s</Arguments>
    </Exec>
  </Actions>
</Task>
`, exe, args)
}

func randomHex() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
