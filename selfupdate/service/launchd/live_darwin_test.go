//go:build darwin

package launchd

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service/servicetest"
)

// The live test against this Mac's launchd (0011-PLAN V3 step 8): a
// throwaway KeepAlive LaunchAgent in gui/<uid>, running a copy of this test
// binary. It runs only with SELFUPDATE_REQUIRE_LAUNCHD=1, and needs a GUI
// login session ("Aqua").

const (
	requireEnv  = "SELFUPDATE_REQUIRE_LAUNCHD"
	secretValue = `say "hi" $HOME \n` + "`x`\nsecond line"
)

func liveMode(mode string) int {
	switch mode {
	case "service":
		return liveService()
	case "update":
		return liveUpdate()
	}
	return 2
}

// liveService is the job's process: it waits for SIGTERM, or for
// FAKE_TRIGGER, which makes it run the update from inside the job. With
// FAKE_IGNORE_TERM=1 it is slow to exit: it notes a SIGTERM in the
// "termed" file, for the lab to see the stop under way, and runs on until
// launchd kills it after ExitTimeOut.
func liveService() int {
	term := make(chan os.Signal, 1)
	signal.Notify(term, syscall.SIGTERM)
	ignore := os.Getenv("FAKE_IGNORE_TERM") == "1"
	// The signal disposition is set: a PID in list may still be xpcproxy's.
	report("ready", strconv.Itoa(os.Getpid()))
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-term:
			if ignore {
				report("termed", strconv.Itoa(os.Getpid()))
				continue
			}
			return 0
		case <-tick.C:
			if trigger := os.Getenv("FAKE_TRIGGER"); trigger != "" {
				if _, err := os.Stat(trigger); err == nil {
					_ = os.Remove(trigger)
					insideUpdate()
				}
			}
		}
	}
}

func report(name, text string) {
	_ = os.WriteFile(filepath.Join(os.Getenv("FAKE_DIR"), name), []byte(text), 0o600)
}

// insideUpdate is the update run from inside the job: Stop is refused, and
// the update hands off.
func insideUpdate() {
	j, err := liveJob()
	if err != nil {
		report("inside-error", err.Error())
		return
	}
	if err := j.Stop(context.Background(), "demo"); !errors.Is(err, service.ErrInsideService) {
		report("inside-error", fmt.Sprintf("Stop from inside: %v", err))
		return
	}
	report("backstop", "refused")
	// The job's own name, for the run to compare with its own (0033-PLAN
	// K4).
	det, handed, err := service.HandOffIfInside(context.Background(), j, service.HandOff{
		Env: []string{"FAKE_SECRET=" + secretValue, "CALLER_XPC_SERVICE_NAME=" + os.Getenv("XPC_SERVICE_NAME")},
	})
	if err != nil || !handed {
		report("inside-error", fmt.Sprintf("handoff: handed %t, %v", handed, err))
		return
	}
	report("handed", det.Where)
}

// liveUpdate is the detached run. ReportFunc comes first: it loads the
// private environment file.
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
		return selfupdate.Result{}, fmt.Errorf("the environment file changed FAKE_SECRET to %q", got)
	}
	// The run reports as its own job, not the caller's (0033-MADR D1-C,
	// Amendment A1 K1-4).
	if mine, caller := os.Getenv("XPC_SERVICE_NAME"), os.Getenv("CALLER_XPC_SERVICE_NAME"); caller == "" || mine == caller {
		return selfupdate.Result{}, fmt.Errorf("the run's XPC_SERVICE_NAME %q, the caller's %q", mine, caller)
	}
	j, err := liveJob()
	if err != nil {
		return selfupdate.Result{}, err
	}
	installed, err := managedInstall(j, os.Getenv("FAKE_TARGET"), os.Getenv("FAKE_NEW"))
	return selfupdate.Result{
		Product: "demo", Applied: installed.Applied, ServiceInstalled: installed.ServiceInstalled,
		ServiceStarted: installed.ServiceStarted, RolledBack: installed.RolledBack,
	}, err
}

func liveJob() (*Job, error) {
	return New(Options{
		Label: os.Getenv("FAKE_LABEL"), Domain: GUI(os.Getuid()), Plist: os.Getenv("FAKE_PLIST"),
		JobDir: filepath.Join(os.Getenv("FAKE_DIR"), "handoff"),
		Poll:   service.PollOptions{Interval: 200 * time.Millisecond, Timeout: 40 * time.Second},
	})
}

func managedInstall(j *Job, target, newPath string) (selfupdate.InstallResult, error) {
	return managedInstallWith(j, j, target, newPath, selfupdate.ManagedOptions{})
}

// managedInstallWith is managedInstall with a lifecycle, a reconciler and
// options of the test's choosing (docs/decisions/0020-MADR-precheck-gofmt-errors-and-replace-before-stop.md
// 4B).
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

type liveEnv struct {
	label, dir, target, plist string
}

func requireLive(t *testing.T) {
	t.Helper()
	if os.Getenv(requireEnv) != "1" {
		t.Skipf("set %s=1 to run against this Mac's launchd", requireEnv)
	}
	out, err := exec.Command("/bin/launchctl", "managername").Output()
	if err != nil || strings.TrimSpace(string(out)) != "Aqua" {
		t.Fatalf("the live test needs a GUI login session; launchctl managername: %q, %v", out, err)
	}
}

// build copies the test binary to dst and signs it again under the
// identifier marker, which tells the builds apart: the identifier is in
// the signature's code directory. Appending bytes instead fails codesign's
// strict validation, and an Apple Silicon binary with a stale signature is
// killed.
func build(t *testing.T, dst, marker string) {
	t.Helper()
	if err := buildTo(dst, marker); err != nil {
		t.Fatal(err)
	}
}

// buildTo is build, returning its error.
func buildTo(dst, marker string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	body, err := os.ReadFile(self) //nolint:gosec // the test binary
	if err != nil {
		return err
	}
	if err := os.WriteFile(dst, body, 0o755); err != nil { //nolint:gosec // an executable fixture
		return err
	}
	if out, err := exec.Command("/usr/bin/codesign", "--force", "--sign", "-", "--identifier", marker, dst).CombinedOutput(); err != nil {
		return fmt.Errorf("codesign: %w: %s", err, out)
	}
	return nil
}

// dump describes the live test's files and the handoff jobs, for a failure.
func (e *liveEnv) dump() string {
	var b strings.Builder
	_ = filepath.WalkDir(e.dir, func(path string, _ os.DirEntry, _ error) error {
		fmt.Fprintf(&b, "file %s\n", path)
		return nil
	})
	for _, name := range []string{"inside-error", "backstop", "handed"} {
		if body, err := os.ReadFile(filepath.Join(e.dir, name)); err == nil { //nolint:gosec // the live test's own file
			fmt.Fprintf(&b, "%s: %s\n", name, body)
		}
	}
	plists, _ := filepath.Glob(filepath.Join(e.dir, "handoff", "*.plist"))
	for _, p := range plists {
		label := strings.TrimSuffix(filepath.Base(p), ".plist")
		out, _ := exec.Command("/bin/launchctl", "print", liveDomain()+"/"+label).CombinedOutput()
		fmt.Fprintf(&b, "print %s:\n%s\n", label, out)
	}
	return b.String()
}

// signedAs returns the identifier path is signed under, which names the
// build (build).
func signedAs(t *testing.T, path string) string {
	t.Helper()
	out, err := exec.Command("/usr/bin/codesign", "-d", "-v", path).CombinedOutput()
	if err != nil {
		t.Fatalf("codesign -d: %v: %s", err, out)
	}
	for line := range strings.SplitSeq(string(out), "\n") {
		if id, ok := strings.CutPrefix(line, "Identifier="); ok {
			return id
		}
	}
	t.Fatalf("codesign -d gives no identifier: %s", out)
	return ""
}

// liveLabel is the one label every live test uses. Start runs
// `launchctl enable`, which keeps an override for the label in launchd's
// database for good, and launchctl cannot delete one: a fixed label leaves
// one entry, where a label per run would leave one per run.
const liveLabel = "io.github.maccavelli.selfupdate.livetest"

func liveDomain() string { return "gui/" + strconv.Itoa(os.Getuid()) }

// newLiveJob loads a throwaway KeepAlive LaunchAgent running a copy of
// this test binary, and boots it out when the test ends.
func newLiveJob(t *testing.T) *liveEnv {
	t.Helper()
	return newSlowLiveJob(t, 0)
}

// newSlowLiveJob is newLiveJob; with exitTimeOut > 0, the job ignores
// SIGTERM, and launchd kills it that many seconds after a bootout. The
// lab arranges it, Running or SlowStop.
func newSlowLiveJob(t *testing.T, exitTimeOut int) *liveEnv {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	l, err := newLiveLab(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = l.Arrange(context.Background(), servicetest.Arrangement{State: servicetest.Absent})
		if j, err := liveJobFor(&l.liveEnv); err == nil {
			_ = j.CleanupHandOffs(context.Background())
		}
	})
	state := servicetest.Running
	if exitTimeOut > 0 {
		l.exitTimeOut, state = exitTimeOut, servicetest.SlowStop
	}
	if err := l.Arrange(context.Background(), servicetest.Arrangement{State: state, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	return &l.liveEnv
}

// labExitTimeOut is the ExitTimeOut, in seconds, of the lab's SlowStop
// job: it ignores SIGTERM, and launchd kills it that long after its
// bootout.
const labExitTimeOut = 3

// liveLab is a servicetest.Lab over this Mac's launchd: a LaunchAgent
// under liveLabel in gui/<uid>, whose plist the lab writes for each state,
// running a copy of this test binary in "service" mode
// (docs/decisions/0038-PLAN-pinned-plans-lifecycle-conformance-and-bubble-tests.md
// Q12). launchd ends every job within its ExitTimeOut, so the lab cannot
// arrange Stubborn (0038-MADR Amendment A3); a plist bootstrap refuses
// with 5 every time is StartHangs.
type liveLab struct {
	liveEnv
	// exitTimeOut is a SlowStop job's ExitTimeOut, in seconds.
	exitTimeOut int
	// enabled is the arrangement's Enabled: the plist's RunAtLoad and
	// KeepAlive.
	enabled bool
	// pid is the job's process when the lab last saw one, so Arrange can
	// wait for it to go.
	pid int
}

// newLiveLab builds the job's binary in dir and enables the label: a run
// that crashed may have left the job loaded, or disabled
// (TestLiveExitCodes). It loads nothing.
func newLiveLab(dir string) (*liveLab, error) {
	l := &liveLab{
		liveEnv:     liveEnv{label: liveLabel, dir: dir, target: filepath.Join(dir, "demo"), plist: filepath.Join(dir, liveLabel+".plist")},
		exitTimeOut: labExitTimeOut,
	}
	if err := buildTo(l.target, "selfupdate.live.old"); err != nil {
		return nil, err
	}
	_ = exec.Command("/bin/launchctl", "bootout", l.service()).Run()
	if out, err := exec.Command("/bin/launchctl", "enable", l.service()).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("enable: %w: %s", err, out)
	}
	return l, nil
}

func (l *liveLab) service() string { return liveDomain() + "/" + l.label }

// Arrange implements servicetest.Lab. It boots the job out and waits for
// its process to go, then writes the plist the state needs. A Running or
// SlowStop job is bootstrapped, and Arrange returns once it reports ready.
func (l *liveLab) Arrange(ctx context.Context, a servicetest.Arrangement) error {
	if err := l.remove(ctx); err != nil {
		return err
	}
	for _, path := range []string{l.plist, filepath.Join(l.dir, "ready"), filepath.Join(l.dir, "termed")} {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	l.enabled = a.Enabled
	env := map[string]string{
		liveModeEnv: "service", "FAKE_LABEL": l.label, "FAKE_PLIST": l.plist, "FAKE_DIR": l.dir,
		"FAKE_TARGET": l.target, "FAKE_NEW": filepath.Join(l.dir, "new"), "FAKE_TRIGGER": filepath.Join(l.dir, "trigger"),
	}
	argv, keys := []string{l.target}, ""
	if a.Enabled {
		keys = "<key>KeepAlive</key>\n\t<true/>\n\t"
	}
	switch a.State {
	case servicetest.Absent:
		return nil
	case servicetest.Stopped, servicetest.Running, servicetest.StartHangs:
	case servicetest.SlowStop:
		env["FAKE_IGNORE_TERM"] = "1"
		keys += fmt.Sprintf("<key>ExitTimeOut</key>\n\t<integer>%d</integer>\n\t", l.exitTimeOut)
	case servicetest.Crashing:
		crash := filepath.Join(l.dir, "crash")
		if err := os.WriteFile(crash, []byte("#!/bin/sh\nexit 3\n"), 0o755); err != nil { //nolint:gosec // an executable fixture
			return err
		}
		argv = []string{crash}
	case servicetest.Stubborn:
		return fmt.Errorf("launchd live lab: %s: launchd ends a job within its ExitTimeOut, which Stop's bound exceeds: %w", a.State, errors.ErrUnsupported)
	default:
		return fmt.Errorf("launchd live lab: %s: %w", a.State, errors.ErrUnsupported)
	}
	body := strings.Replace(string(oneShotPlist(l.label, argv, env)),
		"<key>RunAtLoad</key>\n\t<true/>", keys+"<key>RunAtLoad</key>\n\t<"+strconv.FormatBool(a.Enabled)+"/>", 1)
	body = strings.Replace(body, "\t<key>AbandonProcessGroup</key>\n\t<true/>\n", "", 1)
	if a.State == servicetest.StartHangs {
		// launchctl bootstrap refuses it with 5, which Start retries
		// (0038-MADR Amendment A3).
		body = `<?xml version="1.0"?><plist><dict><key>Label</key>`
	}
	if err := os.WriteFile(l.plist, []byte(body), 0o644); err != nil { //nolint:gosec // a LaunchAgent plist
		return err
	}
	if a.State != servicetest.Running && a.State != servicetest.SlowStop {
		return nil
	}
	for _, args := range [][]string{{"bootstrap", liveDomain(), l.plist}, {"kickstart", l.service()}} {
		if out, err := exec.CommandContext(ctx, "/bin/launchctl", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("launchctl %s: %w: %s", args[0], err, out)
		}
	}
	return l.waitReady(ctx)
}

// waitReady waits, up to 20 s, until the job's process has reported ready,
// its signal disposition set.
func (l *liveLab) waitReady(ctx context.Context) error {
	for deadline := time.Now().Add(20 * time.Second); ; {
		_, pid, err := l.print(ctx)
		if err != nil {
			return err
		}
		if b, err := os.ReadFile(filepath.Join(l.dir, "ready")); err == nil && pid > 0 && string(b) == strconv.Itoa(pid) { //nolint:gosec // the live test's own file
			l.pid = pid
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("the job never reported ready (pid %d)", pid)
		}
		if err := sleepCtx(ctx, 100*time.Millisecond); err != nil {
			return err
		}
	}
}

// remove boots the job out and waits, up to 20 s, until launchd no longer
// knows it and its last process has gone: launchd kills a job within its
// ExitTimeOut, 5 s by default.
func (l *liveLab) remove(ctx context.Context) error {
	_ = exec.CommandContext(ctx, "/bin/launchctl", "bootout", l.service()).Run()
	for deadline := time.Now().Add(20 * time.Second); ; {
		loaded, pid, err := l.print(ctx)
		if err != nil {
			return err
		}
		if pid > 0 {
			l.pid = pid
		}
		if !loaded && (l.pid <= 0 || !pidAlive(l.pid)) {
			l.pid = 0
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("20 s after its bootout the job is loaded=%t, its process %d alive=%t", loaded, l.pid, pidAlive(l.pid))
		}
		if err := sleepCtx(ctx, 100*time.Millisecond); err != nil {
			return err
		}
	}
}

// print reads the job from `launchctl print`: loaded, and its process.
func (l *liveLab) print(ctx context.Context) (loaded bool, pid int, err error) {
	out, err := exec.CommandContext(ctx, "/bin/launchctl", "print", l.service()).Output()
	var ee *exec.ExitError
	switch {
	case err == nil:
		pid, _ = printState(out)
		return true, pid, nil
	case errors.As(err, &ee) && ee.ExitCode() == exitNotFound:
		return false, 0, nil
	}
	return false, 0, errors.Join(err, ctx.Err())
}

// Observe implements servicetest.Lab. A stop is under way once the job's
// process has noted a SIGTERM it ignores (liveService).
func (l *liveLab) Observe(ctx context.Context) (servicetest.Observation, error) {
	_, err := os.Stat(l.plist)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return servicetest.Observation{}, err
	}
	installed := err == nil
	loaded, pid, err := l.print(ctx)
	if err != nil {
		return servicetest.Observation{}, err
	}
	if pid > 0 {
		l.pid = pid
	}
	b, _ := os.ReadFile(filepath.Join(l.dir, "termed")) //nolint:gosec // the live test's own file
	termed := pid > 0 && string(b) == strconv.Itoa(pid)
	return servicetest.Observation{
		Installed: installed,
		Running:   loaded && pid > 0,
		Stopping:  loaded && termed,
		Enabled:   installed && l.enabled,
		Wanted:    loaded && l.enabled && !termed,
		Detail:    fmt.Sprintf("loaded=%t pid=%d termed=%t", loaded, pid, termed),
	}, nil
}

// sleepCtx sleeps for d, or until ctx ends.
func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func liveJobFor(e *liveEnv) (*Job, error) {
	t := map[string]string{"FAKE_LABEL": e.label, "FAKE_PLIST": e.plist, "FAKE_DIR": e.dir}
	for k, v := range t {
		if err := os.Setenv(k, v); err != nil {
			return nil, err
		}
	}
	return liveJob()
}

func (e *liveEnv) pid(t *testing.T) int {
	t.Helper()
	j, err := liveJobFor(e)
	if err != nil {
		t.Fatal(err)
	}
	s, err := j.probe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return s.pid
}

func waitRunning(t *testing.T, e *liveEnv) int {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if pid := e.pid(t); pid > 0 {
			return pid
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("the job never ran")
	return 0
}

func TestLiveManagedUpdate(t *testing.T) {
	requireLive(t)
	e := newLiveJob(t)
	before := waitRunning(t, e)
	build(t, filepath.Join(e.dir, "new"), "selfupdate.live.new")
	j, err := liveJobFor(e)
	if err != nil {
		t.Fatal(err)
	}
	res, err := managedInstall(j, e.target, filepath.Join(e.dir, "new"))
	if err != nil || !res.Applied || !res.ServiceStarted {
		t.Fatalf("Install = %+v, %v", res, err)
	}
	if after := e.pid(t); after == before || after <= 0 {
		t.Fatalf("pid %d -> %d", before, after)
	}
	if got := signedAs(t, e.target); got != "selfupdate.live.new" {
		t.Fatalf("the job's binary is signed as %q, not the new build", got)
	}
	// Pin the output this package reads: print's pid and state lines.
	out, err := exec.Command("/bin/launchctl", "print", liveDomain()+"/"+e.label).Output()
	if err != nil {
		t.Fatal(err)
	}
	if pid, st := printState(out); pid != e.pid(t) || st != "running" {
		t.Fatalf("print gives pid %d state %q", pid, st)
	}
}

// launchctlExit runs launchctl and returns its exit code.
func launchctlExit(t *testing.T, args ...string) int {
	t.Helper()
	err := exec.Command("/bin/launchctl", args...).Run()
	var ee *exec.ExitError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &ee):
		return ee.ExitCode()
	}
	t.Fatal(err)
	return -1
}

// TestLiveExitCodes pins the launchctl exit codes in 0011-MADR's probe
// evidence, and that Start brings back a job someone disabled.
func TestLiveExitCodes(t *testing.T) {
	requireLive(t)
	e := newLiveJob(t)
	waitRunning(t, e)
	target := liveDomain() + "/" + e.label
	missing := target + ".absent"
	for _, c := range []struct {
		args []string
		want int
	}{
		{[]string{"print", missing}, exitNotFound},
		{[]string{"list", e.label + ".absent"}, exitNotFound},
		{[]string{"kickstart", missing}, exitNotFound},
		{[]string{"bootout", missing}, exitNoProcess},
		// Loaded already: the same code as a job still leaving.
		{[]string{"bootstrap", liveDomain(), e.plist}, exitIO},
		{[]string{"bootout", target}, 0},
		{[]string{"disable", target}, 0},
		// Disabled: 5 again, not 119.
		{[]string{"bootstrap", liveDomain(), e.plist}, exitIO},
	} {
		if got := launchctlExit(t, c.args...); got != c.want {
			t.Fatalf("launchctl %q exited %d, want %d", c.args, got, c.want)
		}
	}
	out, err := exec.Command("/bin/launchctl", "error", strconv.Itoa(exitDisabled)).Output()
	if err != nil || !strings.Contains(string(out), "Service is disabled") {
		t.Fatalf("launchctl error %d: %q, %v", exitDisabled, out, err)
	}
	j, err := liveJobFor(e)
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Start(context.Background(), "demo"); err != nil {
		t.Fatalf("Start of a disabled job: %v", err)
	}
	if enabled, err := j.Enabled(context.Background(), "demo"); err != nil || !enabled {
		t.Fatalf("after Start: enabled %t, %v", enabled, err)
	}
	waitRunning(t, e)
}

// TestLiveStopWaitsForSlowExit: bootout returns while a job that ignores
// SIGTERM is still loaded, in state SIGTERMed, until launchd kills it at
// ExitTimeOut. Stop returns only once it is gone: the bug magic-cli-remote
// fixed (0011-MADR §7).
func TestLiveStopWaitsForSlowExit(t *testing.T) {
	requireLive(t)
	// launchd kills the job ExitTimeOut after the bootout; the poll timeout
	// is shorter, so Stop must wait by the job's own bound
	// (docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md B3).
	const exitTimeOut, pollTimeout = 8, 3 * time.Second
	e := newSlowLiveJob(t, exitTimeOut)
	pid := waitRunning(t, e)
	for deadline := time.Now().Add(20 * time.Second); ; {
		if b, err := os.ReadFile(filepath.Join(e.dir, "ready")); err == nil && string(b) == strconv.Itoa(pid) { //nolint:gosec // the live test's own file
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("pid %d never reported ready", pid)
		}
		time.Sleep(50 * time.Millisecond)
	}
	j, err := liveJobFor(e)
	if err != nil {
		t.Fatal(err)
	}
	j.o.Poll.Timeout = pollTimeout
	start := time.Now()
	if err := j.Stop(context.Background(), "demo"); err != nil {
		t.Fatalf("Stop after %v: %v", time.Since(start), err)
	}
	took, alive := time.Since(start), pidAlive(pid)
	printed := launchctlExit(t, "print", liveDomain()+"/"+e.label)
	if took < (exitTimeOut-1)*time.Second || alive || printed != exitNotFound {
		t.Fatalf("Stop returned after %v, pid %d alive %t, print exit %d; want at least %d s, gone, %d",
			took, pid, alive, printed, exitTimeOut-1, exitNotFound)
	}
}

// TestLiveStatus: Status and Loaded agree with launchd for a running job,
// and after Stop, when the job is no longer loaded (0031-MADR D4).
func TestLiveStatus(t *testing.T) {
	requireLive(t)
	e := newLiveJob(t)
	pid := waitRunning(t, e)
	j, err := liveJobFor(e)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	check := func(when string, want Status) {
		t.Helper()
		got, err := j.Status(ctx, "demo")
		if err != nil || got != want {
			t.Fatalf("%s: Status = %+v, %v; want %+v", when, got, err, want)
		}
		if loaded, err := j.Loaded(ctx, "demo"); err != nil || loaded != want.Loaded {
			t.Fatalf("%s: Loaded = %t, %v; want %t", when, loaded, err, want.Loaded)
		}
	}
	check("running", Status{Installed: true, Loaded: true, Enabled: true, Running: true, PID: pid})
	if err := j.Stop(ctx, "demo"); err != nil {
		t.Fatal(err)
	}
	check("after Stop", Status{Installed: true, Enabled: true})
}

func TestLiveHealthFailureRollsBack(t *testing.T) {
	requireLive(t)
	e := newLiveJob(t)
	waitRunning(t, e)
	bad := filepath.Join(e.dir, "bad")
	if err := os.WriteFile(bad, []byte("#!/bin/sh\nexit 3\n"), 0o755); err != nil { //nolint:gosec // an executable fixture
		t.Fatal(err)
	}
	j, err := liveJobFor(e)
	if err != nil {
		t.Fatal(err)
	}
	j.o.Poll.Timeout = 15 * time.Second
	res, err := managedInstall(j, e.target, bad)
	if !errors.Is(err, selfupdate.ErrManagedInstall) || res.Applied || !res.RolledBack {
		t.Fatalf("Install = %+v, %v; want a rollback", res, err)
	}
	if pid := waitRunning(t, e); pid <= 0 {
		t.Fatal("after the rollback the job is not running")
	}
}

// TestLiveHandOff: the job's own process runs the update. Stop refuses;
// the update hands off to a one-shot job; the job restarts on the new
// binary; the result file says so.
func TestLiveHandOff(t *testing.T) {
	requireLive(t)
	e := newLiveJob(t)
	before := waitRunning(t, e)
	build(t, filepath.Join(e.dir, "new"), "selfupdate.live.handedoff")
	if err := os.WriteFile(filepath.Join(e.dir, "trigger"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	result := service.DefaultResultPath(e.target)
	deadline := time.Now().Add(120 * time.Second)
	for {
		if msg, err := os.ReadFile(filepath.Join(e.dir, "inside-error")); err == nil { //nolint:gosec // the live test's own file
			t.Fatalf("inside the job: %s", msg)
		}
		if r, err := service.ReadHandOffResult(result); err == nil {
			if r.ExitCode != 0 || !r.Result.Applied || !r.Result.ServiceStarted {
				t.Fatalf("handoff result %+v", r)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no handoff result within 120 s\n%s", e.dump())
		}
		time.Sleep(200 * time.Millisecond)
	}
	if _, err := os.Stat(filepath.Join(e.dir, "backstop")); err != nil {
		t.Fatal("Stop from inside the job was not refused")
	}
	if after := e.pid(t); after == before || after <= 0 {
		t.Fatalf("pid %d -> %d", before, after)
	}
	if got := signedAs(t, e.target); got != "selfupdate.live.handedoff" {
		t.Fatalf("the job's binary is signed as %q, not the handed-off build", got)
	}
	if entries, _ := filepath.Glob(filepath.Join(e.dir, "handoff", "handoff-*.env")); len(entries) != 0 {
		t.Fatalf("the environment file was left: %v", entries)
	}
}

// TestLiveHandOffHealthFailureReportsRollback: the handed-off build exits
// at once, so the detached run's health check fails. Its result reports
// the rollback, and the job runs the previous build afterwards
// (docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md G3).
func TestLiveHandOffHealthFailureReportsRollback(t *testing.T) {
	requireLive(t)
	e := newLiveJob(t)
	waitRunning(t, e)
	if err := os.WriteFile(filepath.Join(e.dir, "new"), []byte("#!/bin/sh\nexit 3\n"), 0o755); err != nil { //nolint:gosec // an executable fixture
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.dir, "trigger"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	result := service.DefaultResultPath(e.target)
	for deadline := time.Now().Add(150 * time.Second); ; time.Sleep(200 * time.Millisecond) {
		if msg, err := os.ReadFile(filepath.Join(e.dir, "inside-error")); err == nil { //nolint:gosec // the live test's own file
			t.Fatalf("inside the job: %s", msg)
		}
		if r, err := service.ReadHandOffResult(result); err == nil {
			if r.ExitCode != 1 || r.Result.Applied || !r.Result.RolledBack {
				t.Fatalf("handoff result %+v; want exit 1, not applied, rolled back", r)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no handoff result within 150 s\n%s", e.dump())
		}
	}
	if pid := waitRunning(t, e); pid <= 0 {
		t.Fatal("after the rollback the job is not running")
	}
	if got := signedAs(t, e.target); got != "selfupdate.live.old" {
		t.Fatalf("the job's binary is signed as %q, not the previous build", got)
	}
}

// stopCheck records, at the first Stop, whether the target already held
// the new build and whether the service still ran
// (docs/decisions/0020-PLAN-precheck-gofmt-errors-and-replace-before-stop.md
// S3).
type stopCheck struct {
	*Job
	target    string
	want      []byte
	stops     int
	newAtStop bool
	ranAtStop bool
}

func (c *stopCheck) Stop(ctx context.Context, product string) error {
	if c.stops == 0 {
		if b, err := os.ReadFile(c.target); err == nil { //nolint:gosec // the live test's own file
			c.newAtStop = string(b) == string(c.want)
		}
		c.ranAtStop, _ = c.Running(ctx, product)
	}
	c.stops++
	return c.Job.Stop(ctx, product)
}

// replaceFirst runs a managed install of newPath with ReplaceBeforeStop.
func replaceFirst(b *Job, target, newPath string) (*stopCheck, selfupdate.InstallResult, error) {
	want, err := os.ReadFile(newPath) //nolint:gosec // the live test's own file
	if err != nil {
		return nil, selfupdate.InstallResult{}, err
	}
	c := &stopCheck{Job: b, target: target, want: want}
	res, err := managedInstallWith(c, c, target, newPath, selfupdate.ManagedOptions{ReplaceBeforeStop: true})
	return c, res, err
}

// TestLiveReplaceBeforeStop: with ReplaceBeforeStop, the running job's
// binary is replaced before the stop, and the job restarts on it
// (docs/decisions/0020-MADR-precheck-gofmt-errors-and-replace-before-stop.md
// 4B).
func TestLiveReplaceBeforeStop(t *testing.T) {
	requireLive(t)
	e := newLiveJob(t)
	before := waitRunning(t, e)
	build(t, filepath.Join(e.dir, "new"), "selfupdate.live.new")
	j, err := liveJobFor(e)
	if err != nil {
		t.Fatal(err)
	}
	c, res, err := replaceFirst(j, e.target, filepath.Join(e.dir, "new"))
	if err != nil || !res.Applied || !res.ServiceStarted || !res.ReplacedBeforeStop {
		t.Fatalf("Install = %+v, %v", res, err)
	}
	if !c.newAtStop || !c.ranAtStop {
		t.Fatalf("at Stop: target held the new build %t, job running %t; want both", c.newAtStop, c.ranAtStop)
	}
	if after := e.pid(t); after == before || after <= 0 {
		t.Fatalf("pid %d -> %d", before, after)
	}
	if got := signedAs(t, e.target); got != "selfupdate.live.new" {
		t.Fatalf("the job's binary is signed as %q, not the new build", got)
	}
}

// TestLiveReplaceBeforeStopHealthFailure: a new binary replaced before the
// stop that cannot run is rolled back, and the job runs the previous one.
func TestLiveReplaceBeforeStopHealthFailure(t *testing.T) {
	requireLive(t)
	e := newLiveJob(t)
	waitRunning(t, e)
	orig, err := os.ReadFile(e.target)
	if err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(e.dir, "bad")
	if err := os.WriteFile(bad, []byte("#!/bin/sh\nexit 3\n"), 0o755); err != nil { //nolint:gosec // an executable fixture
		t.Fatal(err)
	}
	j, err := liveJobFor(e)
	if err != nil {
		t.Fatal(err)
	}
	j.o.Poll.Timeout = 15 * time.Second
	c, res, err := replaceFirst(j, e.target, bad)
	if !errors.Is(err, selfupdate.ErrManagedInstall) || res.Applied || !res.RolledBack || !res.ReplacedBeforeStop {
		t.Fatalf("Install = %+v, %v; want a rollback after the early replace", res, err)
	}
	if !c.newAtStop || !c.ranAtStop {
		t.Fatalf("at Stop: target held the new build %t, job running %t; want both", c.newAtStop, c.ranAtStop)
	}
	if got, err := os.ReadFile(e.target); err != nil || string(got) != string(orig) {
		t.Fatal("after the rollback the target is not the previous binary")
	}
	if pid := waitRunning(t, e); pid <= 0 {
		t.Fatal("after the rollback the job is not running")
	}
}

// TestLiveConformance: Job meets the Lifecycle contract over this Mac's
// launchd (docs/decisions/0038-PLAN-pinned-plans-lifecycle-conformance-and-bubble-tests.md
// Q12). Stop's bound is the longer of Poll's timeout and the default
// ExitTimeOut plus stopGrace; Start's, the bootstrap's, is Poll's timeout.
// launchd ends every job within its ExitTimeOut, so "stubborn" is the one
// check skipped (0038-MADR Amendment A3; 0038-PLAN Deviation D7).
func TestLiveConformance(t *testing.T) {
	requireLive(t)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	lab, err := newLiveLab(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lab.Arrange(context.Background(), servicetest.Arrangement{State: servicetest.Absent}) })
	poll := service.PollOptions{Interval: 100 * time.Millisecond, Timeout: 20 * time.Second}
	j, err := New(Options{Label: lab.label, Domain: GUI(os.Getuid()), Plist: lab.plist, Poll: poll})
	if err != nil {
		t.Fatal(err)
	}
	var skipped []string
	err = servicetest.VerifyLifecycle(context.Background(), j, servicetest.LifecycleCase{
		Product: "demo", Lab: lab,
		StopBound: max(poll.Timeout, defaultExitTimeOut+stopGrace), StartBound: poll.Timeout,
		ContextFree: servicetest.MethodInstalled,
		Logf:        func(format string, args ...any) { skipped = append(skipped, fmt.Sprintf(format, args...)) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(skipped) != 1 || !strings.HasPrefix(skipped[0], "servicetest: stubborn: skipped") {
		t.Fatalf("skipped %q; want only stubborn", skipped)
	}
}
