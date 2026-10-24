package service

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// Tests for leaving the service manager's per-unit variables out
// (docs/decisions/0033-MADR-scrub-service-manager-environment-from-handoffs.md,
// 0033-PLAN K2).

// wantManagerEnv is the MADR's list: D1-C, with Amendment A1's launchd
// names, and D2-C's two directories.
var wantManagerEnv = []string{
	"INVOCATION_ID", "SYSTEMD_EXEC_PID", "MAINPID", "MAINPIDFDID", "PIDFILE", "LOG_NAMESPACE", "DEBUG_INVOCATION",
	"NOTIFY_SOCKET", "WATCHDOG_PID", "WATCHDOG_USEC", "LISTEN_FDS", "LISTEN_PID", "LISTEN_PIDFDID", "LISTEN_FDNAMES",
	"FDSTORE", "MEMORY_PRESSURE_WATCH", "MEMORY_PRESSURE_WRITE", "JOURNAL_STREAM",
	"MANAGERPID", "MANAGERPIDFDID",
	"SERVICE_RESULT", "EXIT_CODE", "EXIT_STATUS", "MONITOR_SERVICE_RESULT", "MONITOR_EXIT_CODE", "MONITOR_EXIT_STATUS",
	"MONITOR_INVOCATION_ID", "MONITOR_UNIT", "TRIGGER_UNIT", "TRIGGER_PATH", "TRIGGER_TIMER_REALTIME_USEC",
	"TRIGGER_TIMER_MONOTONIC_USEC", "REMOTE_ADDR", "REMOTE_PORT", "SO_COOKIE",
	"RUNTIME_DIRECTORY", "CREDENTIALS_DIRECTORY",
	"XPC_SERVICE_NAME", "XPC_FLAGS",
}

// kept are variables of the user or the session, and the unit's persistent
// directories, which pass on (0033-MADR D1-C, D2-C).
var kept = []string{"PATH=/usr/bin", "HOME=/home/u", "XDG_RUNTIME_DIR=/run/user/1000", "STATE_DIRECTORY=/var/lib/demo",
	"CACHE_DIRECTORY=/var/cache/demo", "TMPDIR=/tmp", "OSLogRateLimit=64", "LANG=C.UTF-8", "malformed", "=x"}

func TestManagerEnvNames(t *testing.T) {
	got := ManagerEnvNames()
	if !slices.Equal(got, wantManagerEnv) {
		t.Fatalf("ManagerEnvNames() = %q\nwant %q", got, wantManagerEnv)
	}
	got[0] = "CHANGED"
	if ManagerEnvNames()[0] != "INVOCATION_ID" {
		t.Fatal("ManagerEnvNames returns the package's own slice")
	}
}

func TestScrubManagerEnv(t *testing.T) {
	var in []string
	for _, n := range wantManagerEnv {
		in = append(in, n+"=caller")
	}
	in = append(in, kept...)
	if got := ScrubManagerEnv(in); !slices.Equal(got, kept) {
		t.Fatalf("ScrubManagerEnv = %q\nwant %q", got, kept)
	}
	lower := ScrubManagerEnv([]string{"notify_socket=x", "Invocation_Id=y"})
	if runtime.GOOS == "windows" {
		if len(lower) != 0 {
			t.Fatalf("on Windows, names compare case-insensitively: %q kept", lower)
		}
	} else if len(lower) != 2 {
		t.Fatalf("names compare exactly: %q", lower)
	}
}

// setManagerEnv sets three of the caller's per-unit variables for the test.
func setManagerEnv(t *testing.T) {
	t.Helper()
	t.Setenv("NOTIFY_SOCKET", "/run/caller/notify")
	t.Setenv("INVOCATION_ID", "caller0000")
	t.Setenv("RUNTIME_DIRECTORY", "/run/caller")
}

func lookup(env []string, name string) (string, bool) {
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, name+"="); ok {
			return v, true
		}
	}
	return "", false
}

// TestPassOnExplicitWins: the scrubbed environment, then the caller's
// explicit variables, whatever their names (0033-MADR D5-A).
func TestPassOnExplicitWins(t *testing.T) {
	setManagerEnv(t)
	env := passOn([]string{"NOTIFY_SOCKET=/run/explicit"})
	if v, _ := lookup(env, "NOTIFY_SOCKET"); v != "/run/explicit" {
		t.Fatalf("NOTIFY_SOCKET = %q, want the explicit value", v)
	}
	for _, n := range []string{"INVOCATION_ID", "RUNTIME_DIRECTORY"} {
		if v, ok := lookup(env, n); ok {
			t.Fatalf("%s = %q passed on", n, v)
		}
	}
}

// TestDetachProcessScrubs: the detached run has none of the caller's
// per-unit variables, and an explicit one (0033-MADR D3-A, D5-A).
func TestDetachProcessScrubs(t *testing.T) {
	setManagerEnv(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "env")
	if _, err := DetachProcess(context.Background(), HandOff{
		Executable: exe,
		Env:        []string{fakeEnv + "=envfile", "FAKE_OUT=" + out, "LISTEN_FDS=3"},
	}); err != nil {
		t.Fatal(err)
	}
	var body []byte
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if body, err = os.ReadFile(out); err == nil && len(body) > 0 { //nolint:gosec // the test's own file
			break
		}
	}
	env := strings.Split(string(body), "\n")
	if len(body) == 0 {
		t.Fatal("the detached run wrote no environment")
	}
	for _, n := range []string{"NOTIFY_SOCKET", "INVOCATION_ID", "RUNTIME_DIRECTORY"} {
		if v, ok := lookup(env, n); ok {
			t.Errorf("the detached run has %s=%q", n, v)
		}
	}
	if v, _ := lookup(env, "LISTEN_FDS"); v != "3" {
		t.Errorf("the explicit LISTEN_FDS = %q, want 3", v)
	}
}

// envRunner records each command's environment.
type envRunner struct{ envs [][]string }

func (r *envRunner) Run(_ context.Context, c Command) (Output, error) {
	r.envs = append(r.envs, c.Env)
	return Output{Stdout: []byte(`{"verdict":"unchanged"}`)}, nil
}

// TestExecReconcilerScrubs: the reconcile command has none of the caller's
// per-unit variables, and an explicit one (0033-MADR D3-A, D5-A).
func TestExecReconcilerScrubs(t *testing.T) {
	setManagerEnv(t)
	run := &envRunner{}
	r, err := NewExecReconciler(ExecOptions{
		Reconcile: []string{"setup-service", "--refresh"},
		Restore:   []string{"setup-service", "--restore"},
		Env:       []string{"INVOCATION_ID=explicit"},
		Runner:    run,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = r.Reconcile(context.Background(), "demo", "/opt/demo/demo") //nolint:errcheck // the environment is what is tested
	if len(run.envs) != 1 {
		t.Fatalf("%d commands run, want 1", len(run.envs))
	}
	env := run.envs[0]
	for _, n := range []string{"NOTIFY_SOCKET", "RUNTIME_DIRECTORY"} {
		if v, ok := lookup(env, n); ok {
			t.Errorf("the reconcile command has %s=%q", n, v)
		}
	}
	if v, _ := lookup(env, "INVOCATION_ID"); v != "explicit" {
		t.Errorf("the explicit INVOCATION_ID = %q", v)
	}
}

// TestHopKeepsEnvironment: the hop passes its environment on as it is. It
// came scrubbed from DetachProcess or a handoff file, with the caller's
// explicit variables after, which it must not drop (0033-PLAN Deviation
// D1).
func TestHopKeepsEnvironment(t *testing.T) {
	var got []string
	start := func(_ string, _ []string, env []string) (int, error) {
		got = env
		return 1, nil
	}
	environ := []string{EnvHandOffHop + "=1", "NOTIFY_SOCKET=/run/explicit", "PATH=/usr/bin"}
	exe := func() (string, error) { return "/opt/demo/demo", nil }
	if code := hop(exe, nil, environ, 42, start); code != 0 {
		t.Fatalf("hop exit %d", code)
	}
	if v, _ := lookup(got, "NOTIFY_SOCKET"); v != "/run/explicit" {
		t.Fatalf("the hop dropped an explicit NOTIFY_SOCKET: %q", got)
	}
}
