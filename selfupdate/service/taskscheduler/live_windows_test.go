//go:build windows

package taskscheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/maccavelli/go-selfupdate-lib/internal/winproc"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// The live test against this host's Task Scheduler
// (0021-PLAN-task-scheduler-lifecycle-and-windowless-children.md T6).

const (
	requireEnv = "SELFUPDATE_REQUIRE_TASKSCHED"
	holdWait   = 70 * time.Second
	secretVal  = `say "hi" %PATH% \n` + "`x`\nsecond line"
)

type liveConfig struct {
	Name   string
	Dir    string
	Target string
	New    string
	Path   string
}

func liveMode(mode string, args []string) int {
	if len(args) != 1 {
		return 2
	}
	if mode == "probe" {
		return liveProbe(args[0])
	}
	cfg, err := readLiveConfig(args[0])
	if err != nil {
		return 3
	}
	switch mode {
	case "serve":
		return liveServe(cfg)
	case "agent":
		return liveAgent(cfg)
	case "update":
		return liveUpdate(cfg)
	}
	return 2
}

func readLiveConfig(path string) (liveConfig, error) {
	body, err := os.ReadFile(path) //nolint:gosec // the live test's own file
	if err != nil {
		return liveConfig{}, err
	}
	var cfg liveConfig
	err = json.Unmarshal(body, &cfg)
	cfg.Path = path
	return cfg, err
}

func report(cfg liveConfig, name, text string) {
	_ = os.WriteFile(filepath.Join(cfg.Dir, name), []byte(text), 0o600)
}

func liveServe(cfg liveConfig) int {
	_ = os.Remove(filepath.Join(cfg.Dir, "please-stop"))
	_ = appendFile(filepath.Join(cfg.Dir, "starts"), strconv.Itoa(os.Getpid())+"\n")
	report(cfg, "ready", strconv.Itoa(os.Getpid()))
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for range tick.C {
		if _, err := os.Stat(filepath.Join(cfg.Dir, "please-stop")); err == nil {
			_ = os.Remove(filepath.Join(cfg.Dir, "please-stop"))
			report(cfg, "graceful", "ok")
			return 0
		}
		if _, err := os.Stat(filepath.Join(cfg.Dir, "trigger")); err == nil {
			_ = os.Remove(filepath.Join(cfg.Dir, "trigger"))
			startAgent(cfg)
		}
	}
	return 0
}

func startAgent(cfg liveConfig) {
	exe, err := os.Executable()
	if err != nil {
		report(cfg, "serve-error", err.Error())
		return
	}
	cmd := exec.Command(exe, liveArg, "agent", cfg.Path) //nolint:gosec // the test binary
	if err := cmd.Start(); err != nil {
		report(cfg, "serve-error", "agent: "+err.Error())
		return
	}
	report(cfg, "agent", strconv.Itoa(cmd.Process.Pid))
	_ = cmd.Process.Release()
}

func liveAgent(cfg liveConfig) int {
	s, err := liveBackend(cfg)
	if err != nil {
		report(cfg, "inside-error", err.Error())
		return 1
	}
	if err := s.Stop(context.Background(), "demo"); !errors.Is(err, service.ErrInsideService) {
		report(cfg, "inside-error", fmt.Sprintf("Stop from inside: %v", err))
		return 1
	}
	report(cfg, "backstop", "refused")
	det, handed, err := service.HandOffIfInside(context.Background(), s, service.HandOff{
		Args: []string{liveArg, "update", cfg.Path},
		Env:  []string{"FAKE_SECRET=" + secretVal},
	})
	if err != nil || !handed {
		report(cfg, "inside-error", fmt.Sprintf("handoff: handed %t, %v", handed, err))
		return 1
	}
	report(cfg, "handed", det.Where)
	return 0
}

func liveUpdate(cfg liveConfig) int {
	done := service.ReportFunc()
	if err := writeConsole(os.Getenv("SELFUPDATE_HANDOFF_RESULT") + ".console"); err != nil {
		_ = done(selfupdate.Result{}, err)
		return 1
	}
	if err := writeIntegrity(os.Getenv("SELFUPDATE_HANDOFF_RESULT") + ".integrity"); err != nil {
		_ = done(selfupdate.Result{}, err)
		return 1
	}
	if err := writeOpen(cfg, os.Getenv("SELFUPDATE_HANDOFF_RESULT")+".open"); err != nil {
		_ = done(selfupdate.Result{}, err)
		return 1
	}
	if err := spawnProbe(); err != nil {
		_ = done(selfupdate.Result{}, err)
		return 1
	}
	res, err := runLiveUpdate()
	if rerr := done(res, err); rerr != nil {
		return 4
	}
	if err != nil {
		return 1
	}
	return 0
}

func spawnProbe() error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	dir := filepath.Dir(os.Getenv("SELFUPDATE_HANDOFF_RESULT"))
	cmd, err := winproc.Command(context.Background(), self, liveArg, "probe", dir)
	if err != nil {
		return err
	}
	return cmd.Run()
}

func liveProbe(dir string) int {
	if err := writeConsole(filepath.Join(dir, "probe-console")); err != nil {
		return 1
	}
	return 0
}

func writeConsole(path string) error {
	var pids [8]uint32
	n, _, _ := procConsoleList.Call(uintptr(unsafe.Pointer(&pids[0])), uintptr(len(pids))) //nolint:errcheck,gosec // console snapshot
	hwnd, _, _ := procConsoleWin.Call()                                                    //nolint:errcheck // HWND
	visible := uintptr(0)
	if hwnd != 0 {
		visible, _, _ = procIsWinVisible.Call(hwnd) //nolint:errcheck // BOOL
	}
	line := fmt.Sprintf("procs=%d hwnd=%#x visible=%t", n, hwnd, visible != 0)
	return os.WriteFile(path, []byte(line), 0o600)
}

// mediumRID is SECURITY_MANDATORY_MEDIUM_RID, the integrity of an
// unelevated process.
const mediumRID = 0x2000

// writeIntegrity records this process's integrity RID, the last
// subauthority of its token's mandatory label: the detached update runs
// the backend's calls, and the owner asked for them at medium integrity
// (0021-PLAN Deviation D3).
func writeIntegrity(path string) error {
	tok := windows.GetCurrentProcessToken()
	var n uint32
	_ = windows.GetTokenInformation(tok, windows.TokenIntegrityLevel, nil, 0, &n) //nolint:errcheck // sizes the buffer
	if n == 0 {
		return errors.New("TokenIntegrityLevel: no size")
	}
	buf := make([]byte, n)
	if err := windows.GetTokenInformation(tok, windows.TokenIntegrityLevel, &buf[0], n, &n); err != nil {
		return err
	}
	label := (*windows.Tokenmandatorylabel)(unsafe.Pointer(&buf[0])) //nolint:gosec // TOKEN_MANDATORY_LABEL in its own buffer
	sid := label.Label.Sid
	rid := sid.SubAuthority(uint32(sid.SubAuthorityCount()) - 1)
	return os.WriteFile(path, fmt.Appendf(nil, "%#x", rid), 0o600)
}

// writeOpen records whether this process, the detached update, may open
// the task's program, before it stops it, with the access winproc.Alive
// asks for, and what the wait reads while the program runs. A refused
// open would send Alive to the process snapshot unseen, since the
// snapshot also sees the program exit (0022-MADR Amendment A3).
func writeOpen(cfg liveConfig, path string) error {
	body, err := os.ReadFile(filepath.Join(cfg.Dir, "starts")) //nolint:gosec // the live test's own file
	if err != nil {
		return err
	}
	pids := strings.Fields(string(body))
	if len(pids) == 0 {
		return errors.New("starts: no PID")
	}
	pid, err := strconv.ParseUint(pids[len(pids)-1], 10, 32)
	if err != nil {
		return err
	}
	h, err := windows.OpenProcess(winproc.AliveAccess, false, uint32(pid))
	if err != nil {
		return os.WriteFile(path, fmt.Appendf(nil, "pid=%d open=%v", pid, err), 0o600)
	}
	defer windows.CloseHandle(h) //nolint:errcheck // a query handle: nothing to report
	var wait string
	switch ev, err := windows.WaitForSingleObject(h, 0); {
	case err != nil:
		wait = "err: " + err.Error()
	case ev == uint32(windows.WAIT_TIMEOUT):
		wait = "timeout"
	case ev == windows.WAIT_OBJECT_0:
		wait = "signaled"
	default:
		wait = fmt.Sprintf("%#x", ev)
	}
	return os.WriteFile(path, fmt.Appendf(nil, "pid=%d open=ok wait=%s", pid, wait), 0o600)
}

var (
	kernel32Live     = windows.NewLazySystemDLL("kernel32.dll")
	procConsoleList  = kernel32Live.NewProc("GetConsoleProcessList")
	procConsoleWin   = kernel32Live.NewProc("GetConsoleWindow")
	procIsWinVisible = windows.NewLazySystemDLL("user32.dll").NewProc("IsWindowVisible")
)

func runLiveUpdate() (selfupdate.Result, error) {
	if got := os.Getenv("FAKE_SECRET"); got != secretVal {
		return selfupdate.Result{}, fmt.Errorf("the handoff changed FAKE_SECRET to %q", got)
	}
	cfg, err := readLiveConfig(os.Args[len(os.Args)-1])
	if err != nil {
		return selfupdate.Result{}, err
	}
	s, err := liveBackend(cfg)
	if err != nil {
		return selfupdate.Result{}, err
	}
	installed, err := managedInstall(s, cfg.Target, cfg.New)
	return selfupdate.Result{
		Product: "demo", Applied: installed.Applied, ServiceInstalled: installed.ServiceInstalled,
		ServiceStarted: installed.ServiceStarted, RolledBack: installed.RolledBack,
	}, err
}

func liveBackend(cfg liveConfig) (*Task, error) {
	task, err := New(Options{
		Name:        cfg.Name,
		RewritePath: true,
		Poll:        service.PollOptions{Interval: 200 * time.Millisecond, Timeout: 90 * time.Second, Settle: 3 * time.Second},
		StopGrace:   15 * time.Second,
		RequestStop: func(context.Context, uint32) error {
			return os.WriteFile(filepath.Join(cfg.Dir, "please-stop"), nil, 0o600)
		},
	})
	return task, err
}

func managedInstall(s *Task, target, newPath string) (selfupdate.InstallResult, error) {
	return managedInstallWith(s, s, target, newPath, selfupdate.ManagedOptions{})
}

func managedInstallWith(life selfupdate.Lifecycle, rec selfupdate.Reconciler, target, newPath string, opts selfupdate.ManagedOptions) (selfupdate.InstallResult, error) {
	ctx := context.Background()
	inner, err := selfupdate.NewStandaloneInstaller(selfupdate.InstallOptions{
		TargetPolicy: selfupdate.TargetPolicy{ExecutablePath: target, AllowedRoots: []string{filepath.Dir(target)}},
	})
	if err != nil {
		return selfupdate.InstallResult{}, err
	}
	m, err := selfupdate.NewManagedInstallerWith(inner, life, rec, opts)
	if err != nil {
		return selfupdate.InstallResult{}, err
	}
	tgt, err := m.ResolveTarget(ctx)
	if err != nil {
		return selfupdate.InstallResult{}, err
	}
	sess, err := m.Begin(ctx, tgt)
	if err != nil {
		return selfupdate.InstallResult{}, err
	}
	defer func() { _ = sess.Close() }()
	f, path, err := sess.CreateStaging(ctx)
	if err != nil {
		return selfupdate.InstallResult{}, err
	}
	body, err := os.ReadFile(newPath) //nolint:gosec // the live test's own file
	if err != nil {
		return selfupdate.InstallResult{}, errors.Join(err, f.Close())
	}
	if _, err := f.Write(body); err != nil {
		return selfupdate.InstallResult{}, errors.Join(err, f.Close())
	}
	if err := f.Close(); err != nil {
		return selfupdate.InstallResult{}, err
	}
	return sess.Install(ctx, selfupdate.InstallRequest{Product: "demo", Artifact: selfupdate.StagedArtifact{Path: path, Size: int64(len(body))}})
}

func requireLive(t *testing.T) {
	t.Helper()
	if os.Getenv(requireEnv) != "1" {
		t.Skipf("set %s=1 to run against this host's Task Scheduler; GitHub runners have no interactive console session", requireEnv)
	}
}

func newLiveTask(t *testing.T) liveConfig {
	t.Helper()
	return newLiveTaskIn(t, "")
}

// newLiveTaskIn registers and starts the live task. With sub set, the
// task runs a copy of the program in that directory, not the install
// target, so an install rewrites the task's path.
func newLiveTaskIn(t *testing.T, sub string) liveConfig {
	t.Helper()
	requireLive(t)
	name := "selfupdate-live-" + randomHex()
	dir := t.TempDir()
	cfg := liveConfig{Name: name, Dir: dir, Target: filepath.Join(dir, "demo.exe"), New: filepath.Join(dir, "new.exe")}
	cfg.Path = filepath.Join(dir, "live.json")
	body, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.Path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	build(t, cfg.Target, "\nold build\n")
	command := cfg.Target
	if sub != "" {
		if err := os.Mkdir(filepath.Join(dir, sub), 0o700); err != nil {
			t.Fatal(err)
		}
		command = filepath.Join(dir, sub, "demo.exe")
		build(t, command, "\nold build\n")
	}
	xml := liveXML(command, `tasksched-live serve "`+cfg.Path+`"`)
	if err := comRegister(name, xml); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = exec.Command("schtasks", "/end", "/tn", name).CombinedOutput()
		time.Sleep(500 * time.Millisecond)
		_, _ = exec.Command("schtasks", "/delete", "/tn", name, "/f").CombinedOutput()
		_ = comDelete(name)
	})
	s := liveSvc(t, cfg)
	if err := s.Start(context.Background(), "demo"); err != nil {
		t.Fatal(err)
	}
	waitReady(t, cfg, 0)
	return cfg
}

// liveXML is the live task: a logon trigger, and a watchdog time trigger
// that starts now and repeats every minute with no end, so a task left
// enabled while stopped is started again within the hold wait.
func liveXML(exe, args string) string {
	start := time.Now().Format("2006-01-02T15:04:05")
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <Triggers>
    <LogonTrigger>
      <Enabled>true</Enabled>
    </LogonTrigger>
    <TimeTrigger>
      <Repetition>
        <Interval>PT1M</Interval>
      </Repetition>
      <StartBoundary>%s</StartBoundary>
      <Enabled>true</Enabled>
    </TimeTrigger>
  </Triggers>
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
  <Actions Context="Author">
    <Exec>
      <Command>%s</Command>
      <Arguments>%s</Arguments>
    </Exec>
  </Actions>
</Task>
`, start, exe, args)
}

func build(t *testing.T, dst, marker string) {
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

func waitReady(t *testing.T, cfg liveConfig, not int) int {
	t.Helper()
	for deadline := time.Now().Add(45 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if msg, err := os.ReadFile(filepath.Join(cfg.Dir, "serve-error")); err == nil { //nolint:gosec // the live test's own file
			t.Fatalf("the task: %s", msg)
		}
		if msg, err := os.ReadFile(filepath.Join(cfg.Dir, "inside-error")); err == nil { //nolint:gosec // the live test's own file
			t.Fatalf("inside: %s", msg)
		}
		body, err := os.ReadFile(filepath.Join(cfg.Dir, "ready")) //nolint:gosec // the live test's own file
		if err != nil {
			continue
		}
		if pid, err := strconv.Atoi(strings.TrimSpace(string(body))); err == nil && pid != not {
			return pid
		}
	}
	t.Fatal("the task never reported ready")
	return 0
}

func liveSvc(t *testing.T, cfg liveConfig) *Task {
	t.Helper()
	s, err := liveBackend(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func starts(t *testing.T, cfg liveConfig) []string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(cfg.Dir, "starts")) //nolint:gosec // the live test's own file
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(string(body)), "\n") {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func appendFile(path, text string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // the live test's own file
	if err != nil {
		return err
	}
	_, err = f.WriteString(text)
	return errors.Join(err, f.Close())
}

func assertNoWindow(t *testing.T, path string) {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // the live test's own file
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	if strings.HasPrefix(got, "procs=0 ") {
		t.Errorf("%s has no console: %s", path, got)
	}
	if !strings.Contains(got, " hwnd=0x0 ") {
		t.Errorf("%s has a console window: %s", path, got)
	}
}

func TestLiveTaskUpdate(t *testing.T) {
	cfg := newLiveTask(t)
	ctx := context.Background()
	s := liveSvc(t, cfg)
	for name, probe := range map[string]func() (bool, error){
		"Installed": func() (bool, error) { return s.Installed(ctx, "demo") },
		"Running":   func() (bool, error) { return s.Running(ctx, "demo") },
		"Enabled":   func() (bool, error) { return s.Enabled(ctx, "demo") },
	} {
		if ok, err := probe(); err != nil || !ok {
			t.Fatalf("%s: %t, %v", name, ok, err)
		}
	}
	before := waitReady(t, cfg, 0)
	_ = os.Remove(filepath.Join(cfg.Dir, "ready"))
	build(t, cfg.New, "\nnew build\n")
	if err := os.WriteFile(filepath.Join(cfg.Dir, "trigger"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	result := service.DefaultResultPath(cfg.Target)
	waitHandOff(t, cfg, result)
	on, err := s.Enabled(ctx, "demo")
	if err != nil || !on {
		t.Fatalf("enabled after update: %t, %v", on, err)
	}
	after := waitReady(t, cfg, before)
	if after == before {
		t.Fatalf("instance PID did not change: %d", after)
	}
	if got, err := os.ReadFile(cfg.Target); err != nil || !strings.HasSuffix(string(got), "\nnew build\n") { //nolint:gosec // the live test's own file
		t.Fatal("the task's binary is not the new build")
	}
	assertNoWindow(t, result+".console")
	assertNoWindow(t, filepath.Join(filepath.Dir(result), "probe-console"))
	rid, err := os.ReadFile(result + ".integrity") //nolint:gosec // the live test's own file
	if err != nil {
		t.Fatal(err)
	}
	if want := fmt.Sprintf("%#x", mediumRID); string(rid) != want {
		t.Fatalf("the update ran at integrity %s, want %s (medium)", rid, want)
	}
	open, err := os.ReadFile(result + ".open") //nolint:gosec // the live test's own file
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(open), " open=ok wait=timeout") {
		t.Fatalf("the update's open of the task's program with winproc.AliveAccess: %s; want open=ok wait=timeout (0022-MADR A3)", open)
	}
	t.Logf("the update's integrity RID %s; %s", rid, open)
}

func waitHandOff(t *testing.T, cfg liveConfig, result string) {
	t.Helper()
	for deadline := time.Now().Add(120 * time.Second); ; time.Sleep(200 * time.Millisecond) {
		for _, name := range []string{"inside-error", "serve-error"} {
			if msg, err := os.ReadFile(filepath.Join(cfg.Dir, name)); err == nil { //nolint:gosec // the live test's own file
				t.Fatalf("%s: %s", name, msg)
			}
		}
		if r, err := service.ReadHandOffResult(result); err == nil {
			if r.ExitCode != 0 || !r.Result.Applied || !r.Result.ServiceStarted {
				t.Fatalf("handoff result %+v", r)
			}
			return
		}
		if time.Now().After(deadline) {
			ents, _ := os.ReadDir(cfg.Dir)
			var names []string
			for _, e := range ents {
				names = append(names, e.Name())
			}
			t.Fatalf("no handoff result within 120 s; dir %v", names)
		}
	}
}

func TestLiveTaskStopHoldsDisabled(t *testing.T) {
	cfg := newLiveTask(t)
	s := liveSvc(t, cfg)
	got := starts(t, cfg)
	if len(got) != 1 {
		t.Fatalf("starts before Stop: %v", got)
	}
	first, _ := strconv.Atoi(got[0])
	t.Logf("instance pids before Stop: %v", instancePIDs(t, s, cfg.Name))
	if err := s.Stop(context.Background(), "demo"); err != nil {
		t.Fatal(err)
	}
	t.Logf("start pid %d running after Stop: %t; pids %v", first, s.s.running(uint32(first), 0), instancePIDs(t, s, cfg.Name)) //nolint:gosec // test PID
	time.Sleep(holdWait)
	if got = starts(t, cfg); len(got) != 1 {
		t.Fatalf("a start ran while disabled: %v", got)
	}
	_ = os.Remove(filepath.Join(cfg.Dir, "ready"))
	if err := s.Start(context.Background(), "demo"); err != nil {
		t.Fatal(err)
	}
	waitReady(t, cfg, first)
	if got = starts(t, cfg); len(got) != 2 {
		t.Fatalf("starts after Start: %v, want 2", got)
	}
}

func instancePIDs(t *testing.T, s *Task, name string) []uint32 {
	t.Helper()
	r, err := s.s.open(`\`, name)
	if err != nil {
		t.Fatal(err)
	}
	pids, err := r.instances()
	_ = r.close()
	if err != nil {
		t.Fatal(err)
	}
	return pids
}

// TestLiveTaskRollback: the task runs a copy under old\, so the install
// rewrites its path to the target; the new build fails, and the rollback
// must restore the definition as it was.
func TestLiveTaskRollback(t *testing.T) {
	cfg := newLiveTaskIn(t, "old")
	s := liveSvc(t, cfg)
	r, err := s.s.open(`\`, cfg.Name)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := r.xml()
	_ = r.close()
	if err != nil {
		t.Fatal(err)
	}
	before := waitReady(t, cfg, 0)
	whoami, err := os.ReadFile(filepath.Join(os.Getenv("SystemRoot"), "System32", "whoami.exe"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.New, whoami, 0o755); err != nil { //nolint:gosec // an executable fixture
		t.Fatal(err)
	}
	res, err := managedInstall(s, cfg.Target, cfg.New)
	if !errors.Is(err, selfupdate.ErrManagedInstall) || res.Applied || !res.RolledBack {
		t.Fatalf("Install = %+v, %v; want a rollback", res, err)
	}
	waitReady(t, cfg, before)
	on, err := s.Enabled(context.Background(), "demo")
	if err != nil || !on {
		t.Fatalf("enabled after rollback: %t, %v", on, err)
	}
	r, err = s.s.open(`\`, cfg.Name)
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.xml()
	_ = r.close()
	if err != nil || got != saved {
		t.Fatalf("xml after rollback differs: len %d vs %d, %v", len(got), len(saved), err)
	}
}

func TestLiveTaskRequestStop(t *testing.T) {
	cfg := newLiveTask(t)
	s := liveSvc(t, cfg)
	s.o.RequestStop = func(context.Context, uint32) error {
		return os.WriteFile(filepath.Join(cfg.Dir, "please-stop"), nil, 0o600)
	}
	s.o.StopGrace = 5 * time.Second
	var stops atomic.Int32
	s.s = stopCounter{scheduler: s.s, stops: &stops}
	if err := s.Stop(context.Background(), "demo"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(cfg.Dir, "graceful")); err != nil {
		t.Fatal("the program did not exit on RequestStop")
	}
	if n := stops.Load(); n != 0 {
		t.Fatalf("IRegisteredTask::Stop called %d times after the program exited on RequestStop", n)
	}
}

// stopCounter counts the transport's stop calls.
type stopCounter struct {
	scheduler
	stops *atomic.Int32
}

func (c stopCounter) open(folder, name string) (registered, error) {
	r, err := c.scheduler.open(folder, name)
	if err != nil {
		return nil, err
	}
	return countedStop{registered: r, stops: c.stops}, nil
}

type countedStop struct {
	registered
	stops *atomic.Int32
}

func (r countedStop) stop() error {
	r.stops.Add(1)
	return r.registered.stop()
}

// TestLiveTaskStopRefusesDisabled: Task Scheduler refuses to run a
// disabled task (0x80041326, 0022-PLAN P1). So Stop refuses a task the user
// disabled while its program runs, and leaves the program running and the
// task disabled; and Start, on the disabled task once its program has
// ended, reports ErrTaskDisabled and starts nothing (0022-MADR Amendment
// A1). The task is disabled and ended with schtasks, as a user would.
func TestLiveTaskStopRefusesDisabled(t *testing.T) {
	cfg := newLiveTask(t)
	s := liveSvc(t, cfg)
	ctx := context.Background()
	pid := waitReady(t, cfg, 0)
	if out, err := exec.Command("schtasks", "/change", "/tn", cfg.Name, "/disable").CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	err := s.Stop(ctx, "demo")
	if !errors.Is(err, ErrTaskDisabled) {
		t.Fatalf("Stop: %v, want ErrTaskDisabled", err)
	}
	if !s.s.running(uint32(pid), 0) { //nolint:gosec // test PID; 0 reads the process snapshot
		t.Fatal("Stop ended the program of a disabled task")
	}
	r, err := s.s.open(`\`, cfg.Name)
	if err != nil {
		t.Fatal(err)
	}
	on, err := r.enabled()
	_ = r.close()
	if err != nil || on {
		t.Fatalf("enabled after the refusal: %t, %v; want the task still disabled", on, err)
	}
	if out, err := exec.Command("schtasks", "/end", "/tn", cfg.Name).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	for deadline := time.Now().Add(15 * time.Second); s.s.running(uint32(pid), 0); time.Sleep(100 * time.Millisecond) { //nolint:gosec // test PID
		if time.Now().After(deadline) {
			t.Fatal("the program did not end on schtasks /end")
		}
	}
	err = s.Start(ctx, "demo")
	if !errors.Is(err, ErrTaskDisabled) {
		t.Fatalf("Start: %v, want ErrTaskDisabled", err)
	}
	t.Logf("Start on the disabled task: %v", err)
	time.Sleep(2 * time.Second)
	if got := starts(t, cfg); len(got) != 1 {
		t.Fatalf("starts: %v, want only the first", got)
	}
}

func TestLiveTaskzCleanup(t *testing.T) {
	requireLive(t)
	out, err := exec.Command("schtasks", "/query", "/fo", "csv", "/nh").CombinedOutput()
	if err != nil {
		t.Fatal(err, string(out))
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(strings.ToLower(line), "selfupdate-live-") {
			t.Fatalf("leftover task: %s", line)
		}
	}
}
