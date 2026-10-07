package launchd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// Tests for docs/decisions/0011-PLAN-reference-service-lifecycles.md V3,
// over the scripted fake.

func TestNewRefuses(t *testing.T) {
	ok := Options{Label: "com.example.demo", Domain: System(), Plist: filepath.Join(t.TempDir(), "x.plist")}
	if _, err := newJob(ok, "linux", 0, 0); !errors.Is(err, service.ErrUnsupported) {
		t.Fatalf("off macOS: %v", err)
	}
	for name, mutate := range map[string]func(*Options){
		"no label":      func(o *Options) { o.Label = "" },
		"slash":         func(o *Options) { o.Label = "com/example" },
		"leading dash":  func(o *Options) { o.Label = "-com.example" },
		"no domain":     func(o *Options) { o.Domain = Domain{} },
		"relative":      func(o *Options) { o.Plist = "x.plist" },
		"relative tool": func(o *Options) { o.Launchctl = "launchctl" },
	} {
		o := ok
		mutate(&o)
		if _, err := newJob(o, "darwin", 0, 0); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if j, err := newJob(ok, "darwin", 0, 0); err != nil || j.launchctl != "/bin/launchctl" || j.plutil != "/usr/bin/plutil" {
		t.Fatalf("defaults: %+v, %v", j, err)
	}
}

func TestDomainString(t *testing.T) {
	for d, want := range map[Domain]string{System(): "system", GUI(501): "gui/501", User(0): "user/0"} {
		if d.String() != want {
			t.Errorf("%v = %q, want %q", d, d.String(), want)
		}
	}
}

// TestProbeUsesListWhenItSees: list for the caller's own GUI domain;
// print, and only its top-level lines, otherwise.
func TestProbeUsesListWhenItSees(t *testing.T) {
	f := newFake()
	j := testJob(t, f, Options{})
	if running, err := j.Running(context.Background(), "demo"); err != nil || !running {
		t.Fatalf("running %t, %v", running, err)
	}
	if v := f.verbs(); !slices.Contains(v, "list") || slices.Contains(v, "print") {
		t.Fatalf("own GUI domain: verbs %q, want list", v)
	}

	for name, o := range map[string]Options{
		"background session": {},
		"another user":       {Domain: GUI(501)},
		"system, not root":   {Domain: System()},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFake()
			if name == "background session" {
				f.manager = "Background"
			}
			j := testJob(t, f, o)
			s, err := j.probe(context.Background())
			if err != nil || s.pid != 100 || !s.running {
				t.Fatalf("state %+v, %v; want pid 100 from the top level, not the nested 999", s, err)
			}
			if v := f.verbs(); !slices.Contains(v, "print") || slices.Contains(v, "list") {
				t.Fatalf("verbs %q, want print", v)
			}
		})
	}
}

func TestInstalledAndRunning(t *testing.T) {
	f := newFake()
	j := testJob(t, f, Options{})
	ctx := context.Background()
	if ok, err := j.Installed(ctx, "demo"); err != nil || !ok {
		t.Fatalf("installed %t, %v", ok, err)
	}
	j.o.Plist = filepath.Join(t.TempDir(), "missing.plist")
	if ok, err := j.Installed(ctx, "demo"); err != nil || ok {
		t.Fatalf("missing plist: installed %t, %v", ok, err)
	}
	f.pid = 0
	if running, err := j.Running(ctx, "demo"); err != nil || running {
		t.Fatalf("no PID: running %t, %v", running, err)
	}
	f.loaded = false
	if running, err := j.Running(ctx, "demo"); err != nil || running {
		t.Fatalf("not loaded: running %t, %v", running, err)
	}
}

func TestEnabled(t *testing.T) {
	for _, c := range []struct {
		name     string
		keys     map[string]string
		disabled string
		want     bool
	}{
		{"RunAtLoad", map[string]string{"RunAtLoad": "true"}, "", true},
		{"KeepAlive true", map[string]string{"KeepAlive": "true"}, "", true},
		// raw prints a dictionary's keys, or nothing for an empty one.
		{"KeepAlive dict", map[string]string{"KeepAlive": "SuccessfulExit"}, "", true},
		{"KeepAlive empty dict", map[string]string{"KeepAlive": ""}, "", true},
		{"KeepAlive false", map[string]string{"KeepAlive": "false"}, "", false},
		{"neither", map[string]string{"RunAtLoad": "false"}, "", false},
		{"no keys", map[string]string{}, "", false},
		{"disabled override", map[string]string{"RunAtLoad": "true"}, "disabled", false},
		{"old disabled override", map[string]string{"RunAtLoad": "true"}, "true", false},
		{"enabled override", map[string]string{"RunAtLoad": "true"}, "enabled", true},
	} {
		f := newFake()
		f.keys, f.disabled = c.keys, c.disabled
		got, err := testJob(t, f, Options{}).Enabled(context.Background(), "demo")
		if err != nil || got != c.want {
			t.Errorf("%s: enabled %t, %v; want %t", c.name, got, err, c.want)
		}
	}
}

func TestStopWaitsUntilGone(t *testing.T) {
	f := newFake()
	f.pid = 99999999 // a PID no process has
	f.goneAfter = 3  // bootout returns while the job is still loaded
	j := testJob(t, f, Options{})
	if err := j.Stop(context.Background(), "demo"); err != nil {
		t.Fatal(err)
	}
	v := f.verbs()
	bootout := slices.Index(v, "bootout")
	if bootout < 0 || strings.Count(strings.Join(v[bootout:], " "), "print") < 3 {
		t.Fatalf("verbs %q; want prints until the job was gone", v)
	}
}

func TestStopTimesOut(t *testing.T) {
	f := newFake()
	f.pid = 99999999
	f.goneAfter = 1 << 30
	// ExitTimeOut 0 is no kill bound, so the wait is Poll.Timeout's
	// (0015-MADR B3).
	f.keys["ExitTimeOut"] = "0"
	j := testJob(t, f, Options{Poll: service.PollOptions{Timeout: 100 * time.Millisecond}})
	if err := j.Stop(context.Background(), "demo"); !errors.Is(err, service.ErrTimeout) {
		t.Fatalf("err = %v, want a timeout", err)
	}
}

func TestStopNotLoadedAndRefused(t *testing.T) {
	f := newFake()
	f.loaded, f.pid = false, 0
	if err := testJob(t, f, Options{}).Stop(context.Background(), "demo"); err != nil || slices.Contains(f.verbs(), "bootout") {
		t.Fatalf("not loaded: err %v, verbs %q", err, f.verbs())
	}
	f = newFake()
	f.pid = 99999999
	f.fail["bootout"] = service.Output{ExitCode: exitNotPermitted, Stderr: []byte("Boot-out failed: 1: Operation not permitted")}
	if err := testJob(t, f, Options{}).Stop(context.Background(), "demo"); !errors.Is(err, service.ErrPermission) {
		t.Fatalf("err = %v, want ErrPermission", err)
	}
}

func TestStopRefusesInsideJob(t *testing.T) {
	f := newFake()
	f.pid = os.Getpid() // this process is the job
	j := testJob(t, f, Options{})
	if err := j.Stop(context.Background(), "demo"); !errors.Is(err, service.ErrInsideService) {
		t.Fatalf("err = %v", err)
	}
	if slices.Contains(f.verbs(), "bootout") {
		t.Fatal("the job was booted out from inside it")
	}
}

func TestInsideByAncestry(t *testing.T) {
	f := newFake()
	f.pid = 4242
	f.parents = map[int]int{os.Getpid(): 777, 777: 4242}
	if inside, err := testJob(t, f, Options{}).Inside(context.Background()); err != nil || !inside {
		t.Fatalf("an ancestor job: inside %t, %v", inside, err)
	}
	f.parents = map[int]int{os.Getpid(): 777}
	if inside, err := testJob(t, f, Options{}).Inside(context.Background()); err != nil || inside {
		t.Fatalf("an unrelated job: inside %t, %v", inside, err)
	}
	f.pid = 0
	if inside, err := testJob(t, f, Options{}).Inside(context.Background()); err != nil || inside {
		t.Fatalf("a job with no process: inside %t, %v", inside, err)
	}
	// Not an ancestor, but in this process's group, which launchd reaps
	// with the job.
	f.pid = 4242
	f.parents = map[int]int{os.Getpid(): 777}
	f.groups = map[int]int{4242: 1}
	if inside, err := testJob(t, f, Options{}).Inside(context.Background()); err != nil || !inside {
		t.Fatalf("a job in this process's group: inside %t, %v", inside, err)
	}
	f.groups = map[int]int{4242: 2}
	if inside, err := testJob(t, f, Options{}).Inside(context.Background()); err != nil || inside {
		t.Fatalf("a job in another group: inside %t, %v", inside, err)
	}
}

// TestLaunchctlError: the exit codes of 0011-MADR §5 map to the typed
// errors; others pass through with launchctl's message.
func TestLaunchctlError(t *testing.T) {
	for _, c := range []struct {
		code int
		is   error
		text string
	}{
		{exitNotPermitted, service.ErrPermission, "exit 1: denied"},
		{exitSIP, service.ErrPermission, "exit 150: denied"},
		{exitNotFound, service.ErrNotInstalled, "exit 113: denied"},
		{exitDisabled, nil, "the job is disabled"},
		{exitIO, nil, "bootstrap /p: exit 5: denied"},
	} {
		err := launchctlError("bootstrap", "/p", service.Output{ExitCode: c.code, Stderr: []byte("denied\n")})
		if !strings.Contains(err.Error(), c.text) {
			t.Fatalf("exit %d: %v lacks %q", c.code, err, c.text)
		}
		for _, sentinel := range []error{service.ErrPermission, service.ErrNotInstalled} {
			if errors.Is(err, sentinel) != errors.Is(sentinel, c.is) {
				t.Fatalf("exit %d: %v, errors.Is(%v) wrong", c.code, err, sentinel)
			}
		}
	}
}

func TestStartBootstrapsAndRetries(t *testing.T) {
	f := newFake()
	f.loaded, f.pid = false, 0
	f.bootstrapCodes = []int{exitIO, exitAlready, 0}
	j := testJob(t, f, Options{})
	if err := j.Start(context.Background(), "demo"); err != nil {
		t.Fatal(err)
	}
	v := f.verbs()
	if strings.Count(strings.Join(v, " "), "bootstrap") != 3 || v[len(v)-1] != "kickstart" || !slices.Contains(v, "enable") {
		t.Fatalf("verbs %q", v)
	}
	f = newFake()
	f.loaded, f.pid = false, 0
	f.bootstrapCodes = []int{exitNotPermitted}
	if err := testJob(t, f, Options{}).Start(context.Background(), "demo"); !errors.Is(err, service.ErrPermission) {
		t.Fatalf("err = %v, want ErrPermission", err)
	}
}

// TestStartLoadedSkipsBootstrap: a job still loaded, such as an on-demand
// one, is only kickstarted.
func TestStartLoadedSkipsBootstrap(t *testing.T) {
	f := newFake()
	f.pid, f.state = 0, "not running"
	if err := testJob(t, f, Options{}).Start(context.Background(), "demo"); err != nil || slices.Contains(f.verbs(), "bootstrap") {
		t.Fatalf("err %v, verbs %q", err, f.verbs())
	}
}

func TestWaitHealthy(t *testing.T) {
	f := newFake()
	f.pid = 99999999
	j := testJob(t, f, Options{})
	ctx := context.Background()
	if err := j.Stop(ctx, "demo"); err != nil {
		t.Fatal(err)
	}
	if err := j.Start(ctx, "demo"); err != nil {
		t.Fatal(err)
	}
	if err := j.WaitHealthy(ctx, "demo"); err != nil {
		t.Fatal(err)
	}

	// The process from before the update never counts.
	f = newFake()
	f.onStart = func(f *fakeLaunchd) { f.loaded, f.state = true, "running" }
	j = testJob(t, f, Options{Poll: service.PollOptions{Timeout: 50 * time.Millisecond}})
	if err := j.Start(ctx, "demo"); err != nil {
		t.Fatal(err)
	}
	if err := j.WaitHealthy(ctx, "demo"); !errors.Is(err, service.ErrTimeout) {
		t.Fatalf("same PID: %v, want a timeout", err)
	}

	f.loaded = false
	if err := j.WaitHealthy(ctx, "demo"); !errors.Is(err, service.ErrUnhealthy) {
		t.Fatalf("not loaded: %v, want ErrUnhealthy", err)
	}
}

// TestWaitHealthySettlesAtLeastThrottle: a settle window shorter than
// launchd's ThrottleInterval is raised to it.
func TestWaitHealthySettlesAtLeastThrottle(t *testing.T) {
	f := newFake()
	j := testJob(t, f, Options{Poll: service.PollOptions{Settle: time.Millisecond, Timeout: 50 * time.Millisecond}})
	if err := j.WaitHealthy(context.Background(), "demo"); !errors.Is(err, service.ErrTimeout) {
		t.Fatalf("err = %v; a 1 ms settle must have become 10 s, and timed out", err)
	}
}

func TestReconcile(t *testing.T) {
	f := newFake()
	j := testJob(t, f, Options{})
	ctx := context.Background()
	if res, err := j.Reconcile(ctx, "demo", "/opt/demo/./demo"); err != nil || res.Changed {
		t.Fatalf("same binary: %+v, %v", res, err)
	}
	f.keys["Program"] = "/opt/demo/demo"
	if res, err := j.Reconcile(ctx, "demo", "/opt/demo/demo"); err != nil || res.Changed {
		t.Fatalf("Program: %+v, %v", res, err)
	}
	if _, err := j.Reconcile(ctx, "demo", "/opt/new/demo"); err == nil || !strings.Contains(err.Error(), "RewritePath") {
		t.Fatalf("moved binary without RewritePath: %v", err)
	}
	f.keys = map[string]string{}
	if _, err := j.Reconcile(ctx, "demo", "/opt/new/demo"); err == nil {
		t.Fatal("a plist with no program was reconciled")
	}
}

func readString(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
