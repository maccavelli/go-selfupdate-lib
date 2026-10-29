//go:build linux

package systemd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service/servicetest"
)

// The live test against a real systemd (0011-PLAN V2 step 8). It runs only
// with SELFUPDATE_REQUIRE_SYSTEMD=1, and fails rather than skips when it
// then cannot run. SELFUPDATE_SYSTEMD_SCOPE=user runs it against the
// calling user's manager; otherwise it needs root.
//
// The unit runs a copy of this test binary in "service" mode: a
// Type=notify service that sends READY=1, and that can run an update from
// inside the unit, as an agent the service spawned would.

const (
	requireEnv = "SELFUPDATE_REQUIRE_SYSTEMD"
	scopeEnv   = "SELFUPDATE_SYSTEMD_SCOPE"
	// secretValue round-trips through the handoff's environment file.
	secretValue = `say "hi" $HOME \n` + "`x`\nsecond line"
)

// liveMode runs the binary as the service ("service") or as the detached
// update ("update").
func liveMode(mode string) int {
	switch mode {
	case "service":
		return liveService()
	case "update":
		return liveUpdate()
	case "ignore-term":
		return liveIgnoreTerm()
	case "slow-stop":
		return liveSlowStopMode()
	case "crash":
		// Exits at once, before it reports ready.
		return 3
	case "never-ready":
		// Runs, and never reports ready: a start that never completes.
		for {
			time.Sleep(time.Hour)
		}
	}
	return 2
}

// liveSlowStopMode reports ready, then exits liveSlowStop after SIGTERM.
func liveSlowStopMode() int {
	term := make(chan os.Signal, 1)
	signal.Notify(term, syscall.SIGTERM)
	if _, err := Ready(); err != nil {
		return 3
	}
	<-term
	_, _ = Stopping()
	time.Sleep(liveSlowStop)
	return 0
}

// liveIgnoreTerm reports ready, then ignores SIGTERM, as a program that
// hangs on stop does
// (docs/decisions/0037-PLAN-honour-contexts-in-backend-fakes-and-stop-commands.md
// N3).
func liveIgnoreTerm() int {
	signal.Ignore(syscall.SIGTERM)
	if _, err := Ready(); err != nil {
		return 3
	}
	for {
		time.Sleep(time.Hour)
	}
}

// liveService is the unit's process. It reports ready, then waits for
// SIGTERM, or for FAKE_TRIGGER, which makes it run the update from inside
// the unit.
func liveService() int {
	if d, err := time.ParseDuration(os.Getenv("FAKE_READY_DELAY")); err == nil {
		time.Sleep(d)
	}
	if _, err := Ready(); err != nil {
		return 3
	}
	term := make(chan os.Signal, 1)
	signal.Notify(term, syscall.SIGTERM)
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-term:
			_, _ = Stopping()
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

// insideUpdate is the agent's update, from inside the unit. It first shows
// the backstop: a plain Stop is refused. Then it hands the update off.
func insideUpdate() {
	report := func(name, text string) {
		_ = os.WriteFile(filepath.Join(os.Getenv("FAKE_DIR"), name), []byte(text), 0o600)
	}
	u, err := liveUnit()
	if err != nil {
		report("inside-error", err.Error())
		return
	}
	if err := u.Stop(context.Background(), "demo"); !errors.Is(err, service.ErrInsideService) {
		report("inside-error", fmt.Sprintf("Stop from inside: %v", err))
		return
	}
	report("backstop", "refused")
	// The unit's own invocation, for the run to compare with its own
	// (0033-PLAN K4).
	det, handed, err := service.HandOffIfInside(context.Background(), u, service.HandOff{
		Env: []string{liveModeEnv + "=update", "FAKE_SECRET=" + secretValue, "CALLER_INVOCATION_ID=" + os.Getenv("INVOCATION_ID")},
	})
	if err != nil || !handed {
		report("inside-error", fmt.Sprintf("handoff: handed %t, %v", handed, err))
		return
	}
	report("handed", det.Where)
}

// liveUpdate is the detached run: the managed update, outside the unit.
func liveUpdate() int {
	report := service.ReportFunc()
	res, err := runLiveUpdate()
	if rerr := report(res, err); rerr != nil {
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
	// The run is its own unit's, not the caller's: its own invocation, and
	// no channel to the caller's manager (0033-MADR D3-A).
	if mine, caller := os.Getenv("INVOCATION_ID"), os.Getenv("CALLER_INVOCATION_ID"); caller == "" || mine == caller {
		return selfupdate.Result{}, fmt.Errorf("the run's INVOCATION_ID %q, the caller's %q", mine, caller)
	}
	if v, ok := os.LookupEnv("NOTIFY_SOCKET"); ok {
		return selfupdate.Result{}, fmt.Errorf("the run has NOTIFY_SOCKET=%q", v)
	}
	u, err := liveUnit()
	if err != nil {
		return selfupdate.Result{}, err
	}
	installed, err := managedInstall(u, os.Getenv("FAKE_TARGET"), os.Getenv("FAKE_NEW"))
	return selfupdate.Result{
		Product: "demo", Applied: installed.Applied, ServiceInstalled: installed.ServiceInstalled,
		ServiceWasRunning: installed.ServiceWasRunning, ServiceStarted: installed.ServiceStarted,
		RolledBack: installed.RolledBack,
	}, err
}

// liveUnit is a Unit for the live unit, in the live test's scope.
func liveUnit() (*Unit, error) {
	scope := System
	if os.Getenv(scopeEnv) == "user" {
		scope = User
	}
	return New(Options{
		Unit: os.Getenv("FAKE_UNIT"), Scope: scope,
		Poll: service.PollOptions{Interval: 100 * time.Millisecond, Timeout: 30 * time.Second, Settle: time.Second},
	})
}

// managedInstall replaces target with the bytes at newPath through a
// ManagedInstaller over u.
func managedInstall(u *Unit, target, newPath string) (selfupdate.InstallResult, error) {
	return managedInstallWith(u, u, target, newPath, selfupdate.ManagedOptions{})
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

// liveEnv is one live unit: its name, directory, target binary, and the
// files the service and the test exchange.
type liveEnv struct {
	unit, dir, target, unitFile string
	scope                       Scope
}

func requireLive(t *testing.T) Scope {
	t.Helper()
	if os.Getenv(requireEnv) != "1" {
		t.Skipf("set %s=1 to run against a real systemd", requireEnv)
	}
	if !Available() {
		t.Fatal("this host does not run systemd")
	}
	if os.Getenv(scopeEnv) == "user" {
		return User
	}
	if os.Geteuid() != 0 {
		t.Fatal("the system-scope live test needs root")
	}
	return System
}

// newLiveUnit installs a throwaway Type=notify unit running a copy of this
// test binary, starts it, and removes it when the test ends. extra is
// added to its environment. The lab arranges it, Running.
func newLiveUnit(t *testing.T, scope Scope, extra ...string) *liveEnv {
	t.Helper()
	l := newLiveLabFor(t, scope)
	l.extra = extra
	if err := l.Arrange(context.Background(), servicetest.Arrangement{State: servicetest.Running}); err != nil {
		t.Fatal(err)
	}
	return l.liveEnv
}

// liveUnitOpts add lines to the throwaway unit's [Unit] and [Service]
// sections, and can leave it stopped.
type liveUnitOpts struct {
	unit, service string
	noStart       bool
}

// newLiveUnitWith is newLiveUnit with o's lines, started unless o.noStart.
func newLiveUnitWith(t *testing.T, scope Scope, o liveUnitOpts, extra ...string) *liveEnv {
	t.Helper()
	e := newLiveLabFor(t, scope).liveEnv
	if err := e.writeUnit(o, extra...); err != nil {
		t.Fatal(err)
	}
	if o.noStart {
		return e
	}
	if err := e.systemctl("start", e.unit); err != nil {
		t.Fatal(err)
	}
	return e
}

// newLiveEnv names a throwaway unit and copies this test binary into its
// directory. It writes no unit.
func newLiveEnv(scope Scope) (*liveEnv, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return nil, err
	}
	e := &liveEnv{unit: "selfupdate-live-" + hex.EncodeToString(b[:]) + ".service", scope: scope}
	// The binary's directory must be one the service can execute from: not
	// a noexec /tmp, and not a 0700 test directory another user cannot
	// enter, so it is made world-readable.
	dir, err := os.MkdirTemp(os.Getenv("FAKE_LIVE_ROOT"), "selfupdate-live-")
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0o755); err != nil { //nolint:gosec // the unit's binary lives here
		return nil, errors.Join(err, os.RemoveAll(dir))
	}
	e.dir = dir
	e.target = filepath.Join(dir, "demo")
	self, err := os.Executable()
	if err != nil {
		return nil, errors.Join(err, os.RemoveAll(dir))
	}
	body, err := os.ReadFile(self) //nolint:gosec // the test binary
	if err != nil {
		return nil, errors.Join(err, os.RemoveAll(dir))
	}
	if err := os.WriteFile(e.target, body, 0o755); err != nil { //nolint:gosec // an executable fixture
		return nil, errors.Join(err, os.RemoveAll(dir))
	}
	unitDir := "/etc/systemd/system"
	if scope == User {
		home, _ := os.UserHomeDir()
		unitDir = filepath.Join(home, ".config", "systemd", "user")
	}
	e.unitFile = filepath.Join(unitDir, e.unit)
	return e, nil
}

// writeUnit writes e's unit, with o's lines, the service mode's
// environment and extra, a later assignment winning, and reloads systemd.
func (e *liveEnv) writeUnit(o liveUnitOpts, extra ...string) error {
	env := []string{
		liveModeEnv + "=service", "FAKE_UNIT=" + e.unit, "FAKE_DIR=" + e.dir,
		"FAKE_TARGET=" + e.target, "FAKE_NEW=" + filepath.Join(e.dir, "new"), "FAKE_TRIGGER=" + filepath.Join(e.dir, "trigger"),
	}
	if e.scope == User {
		env = append(env, scopeEnv+"=user", "XDG_RUNTIME_DIR="+os.Getenv("XDG_RUNTIME_DIR"))
	}
	env = append(env, extra...)
	var unit strings.Builder
	unit.WriteString("[Unit]\nDescription=go-selfupdate-lib live test\n" + o.unit + "[Service]\nType=notify\n" + o.service)
	for _, kv := range env {
		fmt.Fprintf(&unit, "Environment=%q\n", kv)
	}
	fmt.Fprintf(&unit, "ExecStart=%q\n[Install]\nWantedBy=default.target\n", e.target)
	if err := os.MkdirAll(filepath.Dir(e.unitFile), 0o755); err != nil { //nolint:gosec // a unit directory
		return err
	}
	if err := os.WriteFile(e.unitFile, []byte(unit.String()), 0o644); err != nil { //nolint:gosec // systemd reads units as any user
		return err
	}
	return e.systemctl("daemon-reload")
}

// remove ends the unit and deletes it: a stop, which also cancels a
// restart, then a kill if the unit is still there 3 s later, as one that
// ignores TERM under TimeoutStopSec=infinity is; then it clears the
// unit's failure, disables it, deletes its file and reloads systemd.
func (e *liveEnv) remove() error {
	if _, err := os.Stat(e.unitFile); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	ended := func() bool {
		p, err := e.props("ActiveState", "MainPID")
		return err == nil && (p["ActiveState"] == "inactive" || p["ActiveState"] == "failed") && p["MainPID"] == "0"
	}
	wait := func(limit time.Duration) bool {
		for deadline := time.Now().Add(limit); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
			if ended() {
				return true
			}
		}
		return ended()
	}
	_ = e.systemctl("stop", "--no-block", e.unit)
	if !wait(3 * time.Second) {
		_ = e.systemctl("kill", "-s", "KILL", e.unit)
		if !wait(20 * time.Second) {
			return fmt.Errorf("%s did not go after a stop and a kill", e.unit)
		}
	}
	_ = e.systemctl("reset-failed", e.unit)
	_ = e.systemctl("disable", e.unit)
	if err := os.Remove(e.unitFile); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return e.systemctl("daemon-reload")
}

func (e *liveEnv) systemctl(args ...string) error {
	if e.scope == User {
		args = append([]string{"--user"}, args...)
	}
	out, err := exec.Command("/usr/bin/systemctl", args...).CombinedOutput() //nolint:gosec // the live test's own unit
	if err != nil {
		return fmt.Errorf("systemctl %s: %w: %s", strings.Join(args, " "), err, out)
	}
	return nil
}

// props reads the unit's properties through systemctl show.
func (e *liveEnv) props(names ...string) (map[string]string, error) {
	args := []string{"show", "--property=" + strings.Join(names, ","), "--", e.unit}
	if e.scope == User {
		args = append([]string{"--user"}, args...)
	}
	out, err := exec.Command("/usr/bin/systemctl", args...).Output() //nolint:gosec // the live test's own unit
	if err != nil {
		return nil, fmt.Errorf("systemctl %s: %w", strings.Join(args, " "), err)
	}
	p := map[string]string{}
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if k, v, ok := strings.Cut(line, "="); ok {
			p[k] = v
		}
	}
	return p, nil
}

// liveStopTimeout is the lab's units' TimeoutStopSec. Stop's bound is it
// plus stopGrace, or Poll's timeout when that is longer.
const liveStopTimeout = 5 * time.Second

// liveSlowStop is how long a SlowStop unit's program takes to exit after
// TERM.
const liveSlowStop = 3 * time.Second

// liveLab is a servicetest.Lab over this host's systemd: the throwaway
// unit, rewritten for each state, running a copy of this test binary
// (docs/decisions/0038-PLAN-pinned-plans-lifecycle-conformance-and-bubble-tests.md
// Q12).
type liveLab struct {
	*liveEnv
	// extra is added to the unit's environment.
	extra []string
}

// newLiveLabFor is a lab over a new throwaway unit, removed with its
// directory when the test ends.
func newLiveLabFor(t *testing.T, scope Scope) *liveLab {
	t.Helper()
	e, err := newLiveEnv(scope)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := e.remove(); err != nil {
			t.Error(err)
		}
		_ = os.RemoveAll(e.dir)
	})
	return &liveLab{liveEnv: e}
}

// Arrange implements servicetest.Lab. It removes the unit, then writes it
// for the state, its program in the state's mode, enables it if asked,
// and starts it for a state that runs. A Stubborn unit ignores TERM under
// TimeoutStopSec=infinity; a StartHangs one never reports ready under
// TimeoutStartSec=infinity.
func (l *liveLab) Arrange(_ context.Context, a servicetest.Arrangement) error {
	if err := l.remove(); err != nil {
		return err
	}
	o := liveUnitOpts{service: fmt.Sprintf("TimeoutStopSec=%d\n", int(liveStopTimeout.Seconds()))}
	mode := "service"
	switch a.State {
	case servicetest.Absent:
		return nil
	case servicetest.Stopped, servicetest.Running:
	case servicetest.SlowStop:
		mode = "slow-stop"
	case servicetest.Stubborn:
		mode, o.service = "ignore-term", o.service+"TimeoutStopSec=infinity\n"
	case servicetest.Crashing:
		mode = "crash"
	case servicetest.StartHangs:
		mode, o.service = "never-ready", o.service+"TimeoutStartSec=infinity\n"
	default:
		return fmt.Errorf("systemd live lab: %s: %w", a.State, errors.ErrUnsupported)
	}
	if err := l.writeUnit(o, append(slices.Clone(l.extra), liveModeEnv+"="+mode)...); err != nil {
		return err
	}
	// remove disabled it; enable, like disable, reloads systemd.
	if a.Enabled {
		if err := l.systemctl("enable", l.unit); err != nil {
			return err
		}
	}
	switch a.State {
	case servicetest.Running, servicetest.SlowStop, servicetest.Stubborn:
		return l.systemctl("start", l.unit)
	}
	return nil
}

// Observe implements servicetest.Lab.
func (l *liveLab) Observe(context.Context) (servicetest.Observation, error) {
	p, err := l.props("LoadState", "ActiveState", "SubState", "UnitFileState", "MainPID")
	if err != nil {
		return servicetest.Observation{}, err
	}
	s := p["ActiveState"]
	return servicetest.Observation{
		Installed: p["LoadState"] != "" && p["LoadState"] != "not-found",
		Running:   p["MainPID"] != "" && p["MainPID"] != "0",
		Stopping:  s == "deactivating",
		Enabled:   p["UnitFileState"] == "enabled",
		Wanted:    s == "active" || s == "activating" || s == "reloading",
		Detail:    fmt.Sprintf("LoadState=%s ActiveState=%s SubState=%s MainPID=%s", p["LoadState"], s, p["SubState"], p["MainPID"]),
	}, nil
}

func (e *liveEnv) show(t *testing.T, prop string) string {
	t.Helper()
	args := []string{"show", "-P", prop, e.unit}
	if e.scope == User {
		args = append([]string{"--user"}, args...)
	}
	out, err := exec.Command("/usr/bin/systemctl", args...).Output() //nolint:gosec // the live test's own unit
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

// copyFile copies src to dst, mode 0755, with suffix appended so the copy
// differs from the original.
func copyFile(t *testing.T, src, dst string, suffix []byte) {
	t.Helper()
	body, err := os.ReadFile(src) //nolint:gosec // the test binary
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, append(body, suffix...), 0o755); err != nil { //nolint:gosec // an executable fixture
		t.Fatal(err)
	}
}

func TestLiveManagedUpdate(t *testing.T) {
	scope := requireLive(t)
	start := time.Now()
	e := newLiveUnit(t, scope, "FAKE_READY_DELAY=1s")
	if took := time.Since(start); took < time.Second {
		t.Fatalf("systemctl start returned after %s, before READY=1", took)
	}
	t.Setenv("FAKE_UNIT", e.unit)
	if scope == User {
		t.Setenv(scopeEnv, "user")
	}
	before := e.show(t, "InvocationID")
	newBin := filepath.Join(e.dir, "new")
	self, _ := os.Executable()
	copyFile(t, self, newBin, []byte("\nnew build\n"))
	u, err := liveUnit()
	if err != nil {
		t.Fatal(err)
	}
	res, err := managedInstall(u, e.target, newBin)
	if err != nil || !res.Applied || !res.ServiceStarted {
		t.Fatalf("Install = %+v, %v", res, err)
	}
	if after := e.show(t, "InvocationID"); after == before || e.show(t, "ActiveState") != "active" {
		t.Fatalf("invocation %s -> %s, state %s", before, after, e.show(t, "ActiveState"))
	}
	// Pin the property formats this package parses.
	if p := execStartPath(e.show(t, "ExecStart")); p != e.target {
		t.Fatalf("ExecStart path %q, want %q", p, e.target)
	}
	if cg := e.show(t, "ControlGroup"); !strings.HasSuffix(cg, "/"+e.unit) {
		t.Fatalf("ControlGroup %q", cg)
	}
}

func TestLiveHealthFailureRollsBack(t *testing.T) {
	scope := requireLive(t)
	e := newLiveUnit(t, scope)
	t.Setenv("FAKE_UNIT", e.unit)
	if scope == User {
		t.Setenv(scopeEnv, "user")
	}
	bad := filepath.Join(e.dir, "bad")
	if err := os.WriteFile(bad, []byte("#!/bin/sh\nexit 3\n"), 0o755); err != nil { //nolint:gosec // an executable fixture
		t.Fatal(err)
	}
	u, err := liveUnit()
	if err != nil {
		t.Fatal(err)
	}
	res, err := managedInstall(u, e.target, bad)
	if !errors.Is(err, selfupdate.ErrManagedInstall) || res.Applied || !res.RolledBack {
		t.Fatalf("Install = %+v, %v; want a rollback", res, err)
	}
	if e.show(t, "ActiveState") != "active" {
		t.Fatalf("after the rollback the unit is %s", e.show(t, "ActiveState"))
	}
}

// TestLiveHandOff: the service, from inside its unit, runs the update, as
// an agent it spawned would. Stop refuses; the update hands off; the unit
// restarts on the new binary; the result file says so.
func TestLiveHandOff(t *testing.T) {
	scope := requireLive(t)
	e := newLiveUnit(t, scope)
	before := e.show(t, "InvocationID")
	self, _ := os.Executable()
	copyFile(t, self, filepath.Join(e.dir, "new"), []byte("\nhanded-off build\n"))
	if err := os.WriteFile(filepath.Join(e.dir, "trigger"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	result := service.DefaultResultPath(e.target)
	deadline := time.Now().Add(90 * time.Second)
	for {
		if msg, err := os.ReadFile(filepath.Join(e.dir, "inside-error")); err == nil { //nolint:gosec // the live test's own file
			t.Fatalf("inside the unit: %s", msg)
		}
		if r, err := service.ReadHandOffResult(result); err == nil {
			if r.ExitCode != 0 || !r.Result.Applied || !r.Result.ServiceStarted {
				t.Fatalf("handoff result %+v", r)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no handoff result within 90 s")
		}
		time.Sleep(200 * time.Millisecond)
	}
	if _, err := os.Stat(filepath.Join(e.dir, "backstop")); err != nil {
		t.Fatal("Stop from inside the unit was not refused")
	}
	if after := e.show(t, "InvocationID"); after == before || e.show(t, "ActiveState") != "active" {
		t.Fatalf("invocation %s -> %s, state %s", before, after, e.show(t, "ActiveState"))
	}
	got, err := os.ReadFile(e.target) //nolint:gosec // the live test's own file
	if err != nil || !strings.HasSuffix(string(got), "handed-off build\n") {
		t.Fatal("the unit's binary is not the handed-off build")
	}
}

// stopCheck records, at the first Stop, whether the target already held
// the new build and whether the service still ran
// (docs/decisions/0020-PLAN-precheck-gofmt-errors-and-replace-before-stop.md
// S3).
type stopCheck struct {
	*Unit
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
	return c.Unit.Stop(ctx, product)
}

// replaceFirst runs a managed install of newPath with ReplaceBeforeStop.
func replaceFirst(b *Unit, target, newPath string) (*stopCheck, selfupdate.InstallResult, error) {
	want, err := os.ReadFile(newPath) //nolint:gosec // the live test's own file
	if err != nil {
		return nil, selfupdate.InstallResult{}, err
	}
	c := &stopCheck{Unit: b, target: target, want: want}
	res, err := managedInstallWith(c, c, target, newPath, selfupdate.ManagedOptions{ReplaceBeforeStop: true})
	return c, res, err
}

// TestLiveReplaceBeforeStop: with ReplaceBeforeStop, the running unit's
// binary is replaced before the stop, and the unit restarts on it
// (docs/decisions/0020-MADR-precheck-gofmt-errors-and-replace-before-stop.md
// 4B).
func TestLiveReplaceBeforeStop(t *testing.T) {
	scope := requireLive(t)
	e := newLiveUnit(t, scope)
	t.Setenv("FAKE_UNIT", e.unit)
	if scope == User {
		t.Setenv(scopeEnv, "user")
	}
	before := e.show(t, "InvocationID")
	newBin := filepath.Join(e.dir, "new")
	self, _ := os.Executable()
	copyFile(t, self, newBin, []byte("\nnew build\n"))
	u, err := liveUnit()
	if err != nil {
		t.Fatal(err)
	}
	c, res, err := replaceFirst(u, e.target, newBin)
	if err != nil || !res.Applied || !res.ServiceStarted || !res.ReplacedBeforeStop {
		t.Fatalf("Install = %+v, %v", res, err)
	}
	if !c.newAtStop || !c.ranAtStop {
		t.Fatalf("at Stop: target held the new build %t, unit running %t; want both", c.newAtStop, c.ranAtStop)
	}
	if after := e.show(t, "InvocationID"); after == before || e.show(t, "ActiveState") != "active" {
		t.Fatalf("invocation %s -> %s, state %s", before, after, e.show(t, "ActiveState"))
	}
}

// TestLiveReplaceBeforeStopHealthFailure: a new binary replaced before the
// stop that cannot start is rolled back, and the unit runs the previous one.
func TestLiveReplaceBeforeStopHealthFailure(t *testing.T) {
	scope := requireLive(t)
	e := newLiveUnit(t, scope)
	t.Setenv("FAKE_UNIT", e.unit)
	if scope == User {
		t.Setenv(scopeEnv, "user")
	}
	orig, err := os.ReadFile(e.target)
	if err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(e.dir, "bad")
	if err := os.WriteFile(bad, []byte("#!/bin/sh\nexit 3\n"), 0o755); err != nil { //nolint:gosec // an executable fixture
		t.Fatal(err)
	}
	u, err := liveUnit()
	if err != nil {
		t.Fatal(err)
	}
	c, res, err := replaceFirst(u, e.target, bad)
	if !errors.Is(err, selfupdate.ErrManagedInstall) || res.Applied || !res.RolledBack || !res.ReplacedBeforeStop {
		t.Fatalf("Install = %+v, %v; want a rollback after the early replace", res, err)
	}
	if !c.newAtStop || !c.ranAtStop {
		t.Fatalf("at Stop: target held the new build %t, unit running %t; want both", c.newAtStop, c.ranAtStop)
	}
	if got, err := os.ReadFile(e.target); err != nil || string(got) != string(orig) {
		t.Fatal("after the rollback the target is not the previous binary")
	}
	if e.show(t, "ActiveState") != "active" {
		t.Fatalf("after the rollback the unit is %s", e.show(t, "ActiveState"))
	}
}

// TestLiveStopInfinityTimesOut: under TimeoutStopSec=infinity a program
// that ignores TERM never stops. Stop returns ErrTimeout at Poll's
// timeout, with the unit still deactivating, where a blocking systemctl
// stop never returned
// (docs/decisions/0037-MADR-honour-contexts-in-backend-fakes-and-stop-commands.md
// D2-A; Amendment A1, P1).
func TestLiveStopInfinityTimesOut(t *testing.T) {
	scope := requireLive(t)
	// The lab's cleanup kills the unit, which no stop ends.
	l := newLiveLabFor(t, scope)
	if err := l.Arrange(context.Background(), servicetest.Arrangement{State: servicetest.Stubborn}); err != nil {
		t.Fatal(err)
	}
	e := l.liveEnv
	const timeout = 3 * time.Second
	u, err := New(Options{Unit: e.unit, Scope: scope, Poll: service.PollOptions{Interval: 100 * time.Millisecond, Timeout: timeout, Settle: -1}})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	done := make(chan error, 1)
	go func() { done <- u.Stop(context.Background(), "demo") }()
	select {
	case err = <-done:
	case <-time.After(30 * time.Second):
		// Killing the unit ends a blocked systemctl stop, so Stop returns.
		_ = e.systemctl("kill", "-s", "KILL", e.unit)
		t.Fatalf("Stop still blocked after 30 s (then: %v)", <-done)
	}
	took := time.Since(start)
	if !errors.Is(err, service.ErrTimeout) || took < timeout || took > timeout+7*time.Second {
		t.Fatalf("Stop = %v after %s; want ErrTimeout at the %s bound", err, took, timeout)
	}
	if s := e.show(t, "ActiveState"); s != "deactivating" {
		t.Fatalf("after Stop the unit is %s, want deactivating: the stop job still running", s)
	}
}

// TestLiveConformance: Unit meets the Lifecycle contract over this host's
// systemd, in the scope SELFUPDATE_SYSTEMD_SCOPE names
// (docs/decisions/0038-PLAN-pinned-plans-lifecycle-conformance-and-bubble-tests.md
// Q12). Stop's bound is the longer of Poll's timeout and the lab's
// TimeoutStopSec plus stopGrace; under TimeoutStopSec=infinity it is
// Poll's timeout. Start documents no bound, so "start hangs" is the one
// check skipped (0038-PLAN Deviation D7).
func TestLiveConformance(t *testing.T) {
	scope := requireLive(t)
	lab := newLiveLabFor(t, scope)
	poll := service.PollOptions{Interval: 100 * time.Millisecond, Timeout: 10 * time.Second, Settle: time.Second}
	u, err := New(Options{Unit: lab.unit, Scope: scope, Poll: poll})
	if err != nil {
		t.Fatal(err)
	}
	var skipped []string
	err = servicetest.VerifyLifecycle(context.Background(), u, servicetest.LifecycleCase{
		Product: "demo", Lab: lab, StopBound: max(poll.Timeout, liveStopTimeout+stopGrace),
		Logf: func(format string, args ...any) { skipped = append(skipped, fmt.Sprintf(format, args...)) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(skipped) != 1 || !strings.HasPrefix(skipped[0], "servicetest: start hangs: skipped") {
		t.Fatalf("skipped %q; want only start hangs", skipped)
	}
}
