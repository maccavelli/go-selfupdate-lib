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
	"sync"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestVARIANTSize(t *testing.T) {
	if n := unsafe.Sizeof(variant{}); n != 24 {
		t.Fatalf("sizeof(variant) = %d, want 24 on amd64/arm64", n)
	}
}

// TestWithCOMConcurrent runs withCOM from several goroutines at once, so
// that go test -race sees any unsynchronised state it writes
// (0025-MADR-align-with-go-sdk-design-practices.md D4).
func TestWithCOMConcurrent(t *testing.T) {
	var wg sync.WaitGroup
	errs := make(chan error, 8*50)
	for range 8 {
		wg.Go(func() {
			for range 50 {
				errs <- withCOM(func() error { return nil })
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("withCOM: %v", err)
		}
	}
}

func TestCOMLayout(t *testing.T) {
	exe, name, dir, r := newLayoutTask(t)
	_, err := systemScheduler().open(`\`, "no-such-"+name)
	if !errors.Is(err, hrFileNotFound) {
		t.Fatalf("missing: %v, want hrFileNotFound (init HRESULT 0x%08X)", err, lastCoInit.Load())
	}
	t.Logf("CoInitializeEx HRESULT 0x%08X", lastCoInit.Load())
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
	s := systemScheduler()
	if s == nil {
		t.Fatal("no transport")
	}
	if err := s.register(`\`, name, xml, taskCreateOrUpdate, logonInteractiveToken); err != nil {
		if errors.Is(err, hrAccessDenied) && os.Getenv("CI") != "" {
			t.Skipf("register %s: %v", name, err)
		}
		t.Fatalf("create %s: %v", name, err)
	}
	t.Cleanup(func() { _ = s.deleteTask(`\`, name) })
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

// folderDeleteFolder is ITaskFolder::DeleteFolder, which only this test's
// cleanup calls.
const folderDeleteFolder = 12

// TestCOMRegistration pins the calls 0032-MADR adds, and the HRESULTs
// Amendment A1 measured: register, list, the principal, a folder, and
// delete (0032-PLAN J2).
func TestCOMRegistration(t *testing.T) {
	s := systemScheduler()
	exe := filepath.Join(os.Getenv("WINDIR"), "System32", "timeout.exe")
	name := "selfupdate-probe-" + randomHex()
	xml := taskXML(exe, "/t 30 /nobreak")
	if err := s.register(`\`, name, xml, taskCreate, logonInteractiveToken); err != nil {
		if errors.Is(err, hrAccessDenied) && os.Getenv("CI") != "" {
			t.Skipf("register %s: %v", name, err)
		}
		t.Fatalf("register: %v", err)
	}
	t.Cleanup(func() { _ = s.deleteTask(`\`, name) })
	if err := s.register(`\`, name, xml, taskCreate, logonInteractiveToken); !errors.Is(err, hrAlreadyExists) {
		t.Fatalf("TASK_CREATE again: %v, want 0x800700B7", err)
	}
	if err := s.register(`\`, name, xml, taskCreateOrUpdate, logonInteractiveToken); err != nil {
		t.Fatalf("TASK_CREATE_OR_UPDATE: %v", err)
	}
	if err := s.register(`\`, name+"-missing", xml, taskUpdate, logonInteractiveToken); !errors.Is(err, hrFileNotFound) {
		t.Fatalf("TASK_UPDATE of a missing task: %v, want 0x80070002", err)
	}

	entries, err := s.list(`\`)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Name == name {
			found = true
			if e.State != stateReady || uint32(e.LastResult) != 0x00041303 { //nolint:gosec // an HRESULT-shaped result
				t.Fatalf("listed %+v, want ready and never run (SCHED_S_TASK_HAS_NOT_RUN)", e)
			}
		}
	}
	if !found {
		t.Fatalf("%s not listed among %d tasks", name, len(entries))
	}

	r, err := s.open(`\`, name)
	if err != nil {
		t.Fatal(err)
	}
	p, err := r.principal()
	if err != nil {
		t.Fatalf("principal: %v", err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	account, _, _, err := user.User.Sid.LookupAccount("")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(p.UserID, account) || p.LogonType != logonInteractiveToken || p.RunLevel != 0 {
		t.Fatalf("principal %+v; want this account's name, InteractiveToken, least privilege", p)
	}

	folder := `\selfupdate-probe-` + randomHex()
	switch err := s.createFolder(folder); {
	case errors.Is(err, hrAccessDenied):
		t.Logf("createFolder %s: %v; the folder steps are skipped", folder, err)
	case err != nil:
		t.Fatalf("createFolder: %v", err)
	default:
		t.Cleanup(func() { _ = comDeleteFolder(folder) })
		if err := s.createFolder(folder); !errors.Is(err, hrAlreadyExists) {
			t.Fatalf("createFolder again: %v, want 0x800700B7", err)
		}
		if err := s.register(folder, "t", xml, taskCreate, logonInteractiveToken); err != nil {
			t.Fatalf("register in %s: %v", folder, err)
		}
		if in, err := s.list(folder); err != nil || len(in) != 1 || in[0].Name != "t" {
			t.Fatalf("list %s: %+v, %v", folder, in, err)
		}
		if err := s.deleteTask(folder, "t"); err != nil {
			t.Fatalf("delete in %s: %v", folder, err)
		}
	}

	if err := s.deleteTask(`\`, name); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := s.deleteTask(`\`, name); !errors.Is(err, hrFileNotFound) {
		t.Fatalf("delete again: %v, want 0x80070002", err)
	}
	if err := s.deleteTask(`\selfupdate-missing-`+randomHex(), "t"); !errors.Is(err, hrFileNotFound) {
		t.Fatalf("delete in a missing folder: %v, want 0x80070002", err)
	}
}

func comDeleteFolder(path string) error {
	return withFolder(`\`, func(f *comObj) error {
		b, err := sysString(strings.TrimPrefix(path, `\`))
		if err != nil {
			return err
		}
		defer freeBSTR(b)
		return checkHR(f.call(folderDeleteFolder, b, 0))
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
