//go:build darwin

package launchd

import (
	"context"
	"errors"
	"fmt"
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
// FAKE_TRIGGER, which makes it run the update from inside the job.
func liveService() int {
	term := make(chan os.Signal, 1)
	if os.Getenv("FAKE_IGNORE_TERM") == "1" {
		// Slow to exit: launchd kills it after ExitTimeOut.
		signal.Ignore(syscall.SIGTERM)
	} else {
		signal.Notify(term, syscall.SIGTERM)
	}
	// The signal disposition is set: a PID in list may still be xpcproxy's.
	report("ready", strconv.Itoa(os.Getpid()))
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-term:
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
	det, handed, err := service.HandOffIfInside(context.Background(), j, service.HandOff{
		Env: []string{"FAKE_SECRET=" + secretValue},
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
	ctx := context.Background()
	inner, err := selfupdate.NewStandaloneInstaller(selfupdate.InstallOptions{
		TargetPolicy: selfupdate.TargetPolicy{ExecutablePath: target, AllowedRoots: []string{filepath.Dir(target)}},
	})
	if err != nil {
		return selfupdate.InstallResult{}, err
	}
	m, err := selfupdate.NewManagedInstaller(inner, j, j)
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
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(self) //nolint:gosec // the test binary
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, body, 0o755); err != nil { //nolint:gosec // an executable fixture
		t.Fatal(err)
	}
	if out, err := exec.Command("/usr/bin/codesign", "--force", "--sign", "-", "--identifier", marker, dst).CombinedOutput(); err != nil {
		t.Fatalf("codesign: %v: %s", err, out)
	}
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
	for _, line := range strings.Split(string(out), "\n") {
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
// SIGTERM, and launchd kills it that many seconds after a bootout.
func newSlowLiveJob(t *testing.T, exitTimeOut int) *liveEnv {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	e := &liveEnv{label: liveLabel, dir: dir}
	e.target = filepath.Join(dir, "demo")
	e.plist = filepath.Join(dir, e.label+".plist")
	build(t, e.target, "selfupdate.live.old")
	env := map[string]string{
		liveModeEnv: "service", "FAKE_LABEL": e.label, "FAKE_PLIST": e.plist, "FAKE_DIR": dir,
		"FAKE_TARGET": e.target, "FAKE_NEW": filepath.Join(dir, "new"), "FAKE_TRIGGER": filepath.Join(dir, "trigger"),
	}
	keys := "<key>KeepAlive</key>\n\t<true/>\n\t"
	if exitTimeOut > 0 {
		env["FAKE_IGNORE_TERM"] = "1"
		keys += fmt.Sprintf("<key>ExitTimeOut</key>\n\t<integer>%d</integer>\n\t", exitTimeOut)
	}
	body := strings.Replace(string(oneShotPlist(e.label, []string{e.target}, env)),
		"<key>RunAtLoad</key>", keys+"<key>RunAtLoad</key>", 1)
	body = strings.Replace(body, "\t<key>AbandonProcessGroup</key>\n\t<true/>\n", "", 1)
	if err := os.WriteFile(e.plist, []byte(body), 0o644); err != nil { //nolint:gosec // a LaunchAgent plist
		t.Fatal(err)
	}
	domain := liveDomain()
	// A run that crashed may have left the job loaded, or disabled
	// (TestLiveExitCodes).
	_ = exec.Command("/bin/launchctl", "bootout", domain+"/"+e.label).Run()
	if out, err := exec.Command("/bin/launchctl", "enable", domain+"/"+e.label).CombinedOutput(); err != nil {
		t.Fatalf("enable: %v: %s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("/bin/launchctl", "bootout", domain+"/"+e.label).Run()
		if j, err := liveJobFor(e); err == nil {
			_ = j.CleanupHandOffs(context.Background())
		}
	})
	if out, err := exec.Command("/bin/launchctl", "bootstrap", domain, e.plist).CombinedOutput(); err != nil {
		t.Fatalf("bootstrap: %v: %s", err, out)
	}
	return e
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
