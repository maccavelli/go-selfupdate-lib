//go:build windows

package scm

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
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// The live test against this host's SCM (0011-PLAN V4 step 8): a
// throwaway service running a copy of this test binary from a directory
// whose name has a space. It runs only with SELFUPDATE_REQUIRE_SCM=1, and
// needs an elevated session to create the service.

const (
	requireEnv  = "SELFUPDATE_REQUIRE_SCM"
	liveName    = "selfupdate-livetest"
	secretValue = `say "hi" %PATH% \n` + "`x`\nsecond line"
)

// liveConfig is what every live mode reads, from the file its last
// argument names: the SCM starts a service with its command line, not with
// this process's environment.
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
	body, err := os.ReadFile(args[0]) //nolint:gosec // the live test's own file
	if err != nil {
		return 3
	}
	var cfg liveConfig
	if err := json.Unmarshal(body, &cfg); err != nil {
		return 3
	}
	cfg.Path = args[0]
	switch mode {
	case "service":
		if err := svc.Run(cfg.Name, &liveHandler{cfg: cfg}); err != nil {
			report(cfg, "service-error", err.Error())
			return 1
		}
		return 0
	case "agent":
		return liveAgent(cfg)
	case "update":
		return liveUpdate()
	}
	return 2
}

func report(cfg liveConfig, name, text string) {
	_ = os.WriteFile(filepath.Join(cfg.Dir, name), []byte(text), 0o600)
}

// liveHandler is the service. It puts itself in a job that kills every
// process in it when the service exits, as an agent's host may: its agent
// dies with it, and only a run that broke away survives.
type liveHandler struct{ cfg liveConfig }

func (h *liveHandler) Execute(_ []string, r <-chan svc.ChangeRequest, s chan<- svc.Status) (bool, uint32) {
	s <- svc.Status{State: svc.StartPending, WaitHint: 5000}
	if err := killWithMe(); err != nil {
		report(h.cfg, "service-error", "job: "+err.Error())
		return true, 1
	}
	report(h.cfg, "ready", strconv.Itoa(os.Getpid()))
	s <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				s <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				s <- svc.Status{State: svc.StopPending, WaitHint: 5000}
				return false, 0
			}
		case <-tick.C:
			trigger := filepath.Join(h.cfg.Dir, "trigger")
			if _, err := os.Stat(trigger); err == nil {
				_ = os.Remove(trigger)
				h.startAgent()
			}
		}
	}
}

// startAgent starts the agent, which inherits the service's job.
func (h *liveHandler) startAgent() {
	exe, err := os.Executable()
	if err != nil {
		report(h.cfg, "service-error", err.Error())
		return
	}
	cmd := exec.Command(exe, liveArg, "agent", h.cfg.Path) //nolint:gosec // the test binary
	if err := cmd.Start(); err != nil {
		report(h.cfg, "service-error", "agent: "+err.Error())
		return
	}
	report(h.cfg, "agent", strconv.Itoa(cmd.Process.Pid))
	_ = cmd.Process.Release()
}

// killWithMe puts this process in a new job that kills its processes when
// its last handle, this process's, closes, and lets a child break away.
func killWithMe() error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE | windows.JOB_OBJECT_LIMIT_BREAKAWAY_OK
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		return err
	}
	return windows.AssignProcessToJobObject(job, windows.CurrentProcess())
}

// liveAgent is a process the service started, running the update: Stop is
// refused, and the update hands off.
func liveAgent(cfg liveConfig) int {
	s, err := liveService(cfg)
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
		Env:  []string{"FAKE_SECRET=" + secretValue},
	})
	if err != nil || !handed {
		report(cfg, "inside-error", fmt.Sprintf("handoff: handed %t, %v", handed, err))
		return 1
	}
	report(cfg, "handed", det.Where)
	time.Sleep(10 * time.Minute) // until the service's job kills it
	return 0
}

// liveUpdate is the detached run. ReportFunc comes first, as a program's
// start-up does; the hop has already been through service.HandOffHop.
func liveUpdate() int {
	done := service.ReportFunc()
	res, err := runLiveUpdate()
	if rerr := done(res, err); rerr != nil {
		return 4
	}
	if err != nil {
		return 1
	}
	return 0
}

func runLiveUpdate() (selfupdate.Result, error) {
	if got := os.Getenv("FAKE_SECRET"); got != secretValue {
		return selfupdate.Result{}, fmt.Errorf("the handoff changed FAKE_SECRET to %q", got)
	}
	cfg, err := readConfig(os.Args[len(os.Args)-1])
	if err != nil {
		return selfupdate.Result{}, err
	}
	s, err := liveService(cfg)
	if err != nil {
		return selfupdate.Result{}, err
	}
	installed, err := managedInstall(s, cfg.Target, cfg.New)
	return selfupdate.Result{
		Product: "demo", Applied: installed.Applied, ServiceInstalled: installed.ServiceInstalled,
		ServiceStarted: installed.ServiceStarted, RolledBack: installed.RolledBack,
	}, err
}

func readConfig(path string) (liveConfig, error) {
	body, err := os.ReadFile(path) //nolint:gosec // the live test's own file
	if err != nil {
		return liveConfig{}, err
	}
	var cfg liveConfig
	err = json.Unmarshal(body, &cfg)
	cfg.Path = path
	return cfg, err
}

func liveService(cfg liveConfig) (*Service, error) {
	return New(Options{
		Name: cfg.Name,
		Poll: service.PollOptions{Interval: 200 * time.Millisecond, Timeout: 60 * time.Second, Settle: 3 * time.Second},
	})
}

func managedInstall(s *Service, target, newPath string) (selfupdate.InstallResult, error) {
	ctx := context.Background()
	inner, err := selfupdate.NewStandaloneInstaller(selfupdate.InstallOptions{
		TargetPolicy: selfupdate.TargetPolicy{ExecutablePath: target, AllowedRoots: []string{filepath.Dir(target)}},
	})
	if err != nil {
		return selfupdate.InstallResult{}, err
	}
	m, err := selfupdate.NewManagedInstaller(inner, s, s)
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

// build copies the test binary to dst with marker appended: a PE file
// runs with data after its last section.
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

func requireLive(t *testing.T) {
	t.Helper()
	if os.Getenv(requireEnv) != "1" {
		t.Skipf("set %s=1 to run against this host's SCM", requireEnv)
	}
}

// removeService stops and deletes the named service, if there is one, and
// waits until the SCM has let it go.
func removeService(t *testing.T, name string) {
	t.Helper()
	m, err := mgr.Connect()
	if err != nil {
		t.Fatalf("connect to the SCM (the live test needs an elevated session): %v", err)
	}
	defer func() { _ = m.Disconnect() }()
	s, err := m.OpenService(name)
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	_, _ = s.Control(svc.Stop)
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if st, err := s.Query(); err != nil || st.State == svc.Stopped {
			break
		}
	}
	if err := s.Delete(); err != nil && !errors.Is(err, windows.ERROR_SERVICE_MARKED_FOR_DELETE) {
		t.Fatal(err)
	}
	_ = s.Close()
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		s, err := m.OpenService(name)
		if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			return
		}
		if err == nil {
			_ = s.Close()
		}
	}
	t.Fatalf("service %s was never deleted", name)
}

// registeredPath is path's 8.3 short form, or path itself when the volume
// gives it none.
func registeredPath(t *testing.T, path string) string {
	t.Helper()
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n, err := windows.GetShortPathName(p, &buf[0], uint32(len(buf)))
	if err != nil {
		t.Fatal(err)
	}
	short := windows.UTF16ToString(buf[:n])
	t.Logf("the service is registered as %s", short)
	return short
}

// newLiveService creates and starts the throwaway service, and deletes it
// when the test ends.
func newLiveService(t *testing.T) liveConfig {
	t.Helper()
	requireLive(t)
	removeService(t, liveName) // a run that crashed may have left it
	dir := filepath.Join(t.TempDir(), "scm live")
	if err := os.Mkdir(dir, 0o755); err != nil { //nolint:gosec // the service, LocalSystem, writes here too
		t.Fatal(err)
	}
	cfg := liveConfig{Name: liveName, Dir: dir, Target: filepath.Join(dir, "demo.exe"), New: filepath.Join(dir, "new.exe")}
	cfg.Path = filepath.Join(dir, "live.json")
	body, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.Path, body, 0o644); err != nil { //nolint:gosec // read by the service
		t.Fatal(err)
	}
	build(t, cfg.Target, "\nold build\n")
	m, err := mgr.Connect()
	if err != nil {
		t.Fatalf("connect to the SCM (the live test needs an elevated session): %v", err)
	}
	defer func() { _ = m.Disconnect() }()
	// Registered by its 8.3 short path when the volume gives one, as CI's
	// %TEMP% does: Reconcile must still recognise the binary (0011-MADR
	// amendment A5).
	s, err := m.CreateService(liveName, registeredPath(t, cfg.Target), mgr.Config{
		StartType: mgr.StartAutomatic, DisplayName: "go-selfupdate-lib live test",
	}, liveArg, "service", cfg.Path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { removeService(t, liveName) })
	if err := s.Start(); err != nil {
		_ = s.Close()
		t.Fatal(err)
	}
	_ = s.Close()
	waitReady(t, cfg, 0)
	return cfg
}

// waitReady waits for the service to report ready as a process other than
// not, and returns its process ID.
func waitReady(t *testing.T, cfg liveConfig, not int) int {
	t.Helper()
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if msg, err := os.ReadFile(filepath.Join(cfg.Dir, "service-error")); err == nil { //nolint:gosec // the live test's own file
			t.Fatalf("the service: %s", msg)
		}
		body, err := os.ReadFile(filepath.Join(cfg.Dir, "ready")) //nolint:gosec // the live test's own file
		if err != nil {
			continue
		}
		if pid, err := strconv.Atoi(string(body)); err == nil && pid != not {
			return pid
		}
	}
	t.Fatal("the service never reported ready")
	return 0
}

func liveSvc(t *testing.T, cfg liveConfig) *Service {
	t.Helper()
	s, err := liveService(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestLiveManagedUpdate(t *testing.T) {
	cfg := newLiveService(t)
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
	if res, err := s.Reconcile(ctx, "demo", cfg.Target); err != nil || res.Changed {
		t.Fatalf("Reconcile of the service's own binary: %+v, %v", res, err)
	}
	before := waitReady(t, cfg, 0)
	build(t, cfg.New, "\nnew build\n")
	res, err := managedInstall(s, cfg.Target, cfg.New)
	if err != nil || !res.Applied || !res.ServiceStarted {
		t.Fatalf("Install = %+v, %v", res, err)
	}
	waitReady(t, cfg, before)
	if got, err := os.ReadFile(cfg.Target); err != nil || !strings.HasSuffix(string(got), "\nnew build\n") { //nolint:gosec // the live test's own file
		t.Fatal("the service's binary is not the new build")
	}
}

// TestLiveHealthFailureRollsBack: the new binary is not a service program,
// so the SCM's start fails; the old binary comes back and runs.
func TestLiveHealthFailureRollsBack(t *testing.T) {
	cfg := newLiveService(t)
	before := waitReady(t, cfg, 0)
	whoami, err := os.ReadFile(filepath.Join(os.Getenv("SystemRoot"), "System32", "whoami.exe"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.New, whoami, 0o755); err != nil { //nolint:gosec // an executable fixture
		t.Fatal(err)
	}
	res, err := managedInstall(liveSvc(t, cfg), cfg.Target, cfg.New)
	if !errors.Is(err, selfupdate.ErrManagedInstall) || res.Applied || !res.RolledBack {
		t.Fatalf("Install = %+v, %v; want a rollback", res, err)
	}
	// The rollback must be the start's: a failure earlier, such as
	// Reconcile's, rolls back too (0011-PLAN deviation D7).
	if !strings.Contains(err.Error(), "scm: start "+liveName) {
		t.Fatalf("the install failed before the start: %v", err)
	}
	waitReady(t, cfg, before)
}

// TestLiveHandOff: a process the service started runs the update. Stop
// refuses; the update hands off through the hop; the agent dies with the
// service's job; the service restarts on the new binary; the result file
// says so.
func TestLiveHandOff(t *testing.T) {
	cfg := newLiveService(t)
	before := waitReady(t, cfg, 0)
	build(t, cfg.New, "\nhanded-off build\n")
	if err := os.WriteFile(filepath.Join(cfg.Dir, "trigger"), nil, 0o644); err != nil { //nolint:gosec // read by the service
		t.Fatal(err)
	}
	result := service.DefaultResultPath(cfg.Target)
	for deadline := time.Now().Add(120 * time.Second); ; time.Sleep(200 * time.Millisecond) {
		for _, name := range []string{"inside-error", "service-error"} {
			if msg, err := os.ReadFile(filepath.Join(cfg.Dir, name)); err == nil { //nolint:gosec // the live test's own file
				t.Fatalf("%s: %s", name, msg)
			}
		}
		if r, err := service.ReadHandOffResult(result); err == nil {
			if r.ExitCode != 0 || !r.Result.Applied || !r.Result.ServiceStarted {
				t.Fatalf("handoff result %+v", r)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no handoff result within 120 s")
		}
	}
	if _, err := os.Stat(filepath.Join(cfg.Dir, "backstop")); err != nil {
		t.Fatal("Stop from inside the service was not refused")
	}
	waitReady(t, cfg, before)
	got, err := os.ReadFile(cfg.Target) //nolint:gosec // the live test's own file
	if err != nil || !strings.HasSuffix(string(got), "\nhanded-off build\n") {
		t.Fatal("the service's binary is not the handed-off build")
	}
	// The agent died with the old service's job: what killed the agent
	// session in magic-cli-remote did not reach the handed-off run.
	body, err := os.ReadFile(filepath.Join(cfg.Dir, "agent")) //nolint:gosec // the live test's own file
	if err != nil {
		t.Fatal(err)
	}
	agent, err := strconv.Atoi(string(body))
	if err != nil {
		t.Fatal(err)
	}
	if h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(agent)); err == nil { //nolint:gosec // a process ID
		ev, _ := windows.WaitForSingleObject(h, 0)
		_ = windows.CloseHandle(h)
		if ev == uint32(windows.WAIT_TIMEOUT) {
			t.Fatalf("the agent %d outlived the service", agent)
		}
	}
}
