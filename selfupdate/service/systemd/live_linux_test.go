//go:build linux

package systemd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
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
	}
	return 2
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
	det, handed, err := service.HandOffIfInside(context.Background(), u, service.HandOff{
		Env: []string{liveModeEnv + "=update", "FAKE_SECRET=" + secretValue},
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
	u, err := liveUnit()
	if err != nil {
		return selfupdate.Result{}, err
	}
	installed, err := managedInstall(u, os.Getenv("FAKE_TARGET"), os.Getenv("FAKE_NEW"))
	return selfupdate.Result{
		Product: "demo", Applied: installed.Applied, ServiceInstalled: installed.ServiceInstalled,
		ServiceWasRunning: installed.ServiceWasRunning, ServiceStarted: installed.ServiceStarted,
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
	ctx := context.Background()
	inner, err := selfupdate.NewStandaloneInstaller(selfupdate.InstallOptions{
		TargetPolicy: selfupdate.TargetPolicy{ExecutablePath: target, AllowedRoots: []string{filepath.Dir(target)}},
	})
	if err != nil {
		return selfupdate.InstallResult{}, err
	}
	m, err := selfupdate.NewManagedInstaller(inner, u, u)
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
// test binary, starts it, and removes it when the test ends.
func newLiveUnit(t *testing.T, scope Scope, extra ...string) *liveEnv {
	t.Helper()
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	e := &liveEnv{unit: "selfupdate-live-" + hex.EncodeToString(b[:]) + ".service", scope: scope}
	// The binary's directory must be one the service can execute from: not
	// a noexec /tmp, and not a 0700 test directory another user cannot
	// enter, so it is made world-readable.
	dir, err := os.MkdirTemp(os.Getenv("FAKE_LIVE_ROOT"), "selfupdate-live-")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil { //nolint:gosec // the unit's binary lives here
		t.Fatal(err)
	}
	e.dir = dir
	e.target = filepath.Join(dir, "demo")
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	copyFile(t, self, e.target, nil)
	unitDir := "/etc/systemd/system"
	if scope == User {
		home, _ := os.UserHomeDir()
		unitDir = filepath.Join(home, ".config", "systemd", "user")
	}
	e.unitFile = filepath.Join(unitDir, e.unit)
	env := []string{
		liveModeEnv + "=service", "FAKE_UNIT=" + e.unit, "FAKE_DIR=" + dir,
		"FAKE_TARGET=" + e.target, "FAKE_NEW=" + filepath.Join(dir, "new"), "FAKE_TRIGGER=" + filepath.Join(dir, "trigger"),
	}
	if scope == User {
		env = append(env, scopeEnv+"=user", "XDG_RUNTIME_DIR="+os.Getenv("XDG_RUNTIME_DIR"))
	}
	env = append(env, extra...)
	var unit strings.Builder
	unit.WriteString("[Unit]\nDescription=go-selfupdate-lib live test\n[Service]\nType=notify\n")
	for _, kv := range env {
		fmt.Fprintf(&unit, "Environment=%q\n", kv)
	}
	fmt.Fprintf(&unit, "ExecStart=%q\n", e.target)
	if err := os.MkdirAll(unitDir, 0o755); err != nil { //nolint:gosec // a unit directory
		t.Fatal(err)
	}
	if err := os.WriteFile(e.unitFile, []byte(unit.String()), 0o644); err != nil { //nolint:gosec // systemd reads units as any user
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = e.systemctl("stop", e.unit)
		_ = os.Remove(e.unitFile)
		_ = e.systemctl("daemon-reload")
		_ = os.RemoveAll(dir)
	})
	if err := e.systemctl("daemon-reload"); err != nil {
		t.Fatal(err)
	}
	if err := e.systemctl("start", e.unit); err != nil {
		t.Fatal(err)
	}
	return e
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
