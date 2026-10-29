package systemd

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// fakeSystemd answers systemctl and systemd-run from an in-memory unit,
// and records every call (0011-PLAN V2 step 8).
type fakeSystemd struct {
	mu      sync.Mutex
	props   map[string]string // the unit's properties
	calls   [][]string
	envs    [][]string
	version string
	// onStart and onStop change props when the job runs.
	onStart, onStop func(map[string]string)
	// fail maps a verb to its failure output.
	fail map[string]service.Output
	// showQueue, when non-empty, answers show calls in turn first.
	showQueue []map[string]string
	// reloadIgnored keeps NeedDaemonReload as it is on daemon-reload.
	reloadIgnored bool
	// onReload changes props on daemon-reload, as systemd re-reading the
	// unit's files does.
	onReload func(map[string]string)
	// stopBlocks, when set, holds a stop without --no-block until it is
	// closed or the call's context ends.
	stopBlocks chan struct{}
	// startBlocks, when set, holds a start without --no-block the same
	// way, as a start job that never completes.
	startBlocks chan struct{}
	// onRun, when set, runs before each call is answered, as time
	// passing changes props.
	onRun func(map[string]string)
}

// waitsFor is the job a systemctl call waits for, "stop" or "start",
// when it has no --no-block; otherwise "".
func waitsFor(args []string) string {
	if len(args) > 0 && args[0] == "--user" {
		args = args[1:]
	}
	if len(args) == 0 || slices.Contains(args, "--no-block") || (args[0] != "stop" && args[0] != "start") {
		return ""
	}
	return args[0]
}

func newFake() *fakeSystemd {
	return &fakeSystemd{
		props: map[string]string{
			"Id": "demo.service", "LoadState": "loaded", "ActiveState": "active", "SubState": "running",
			"UnitFileState": "enabled", "InvocationID": "aaa", "MainPID": "100", "NRestarts": "0",
			"Result": "success", "NeedDaemonReload": "no", "ControlGroup": "/system.slice/demo.service",
		},
		version: "systemd 255 (255.4-1ubuntu8)",
		fail:    map[string]service.Output{},
	}
}

// Run answers as systemctl would. Like ExecRunner, it refuses a call on an
// ended context and records nothing
// (docs/decisions/0037-MADR-honour-contexts-in-backend-fakes-and-stop-commands.md D1-A).
func (f *fakeSystemd) Run(ctx context.Context, c service.Command) (service.Output, error) {
	if err := ctx.Err(); err != nil {
		return service.Output{}, err
	}
	f.mu.Lock()
	block := map[string]chan struct{}{"stop": f.stopBlocks, "start": f.startBlocks}[waitsFor(c.Args)]
	f.mu.Unlock()
	if block != nil {
		// A stop or start without --no-block waits for its job, as
		// systemctl does (0037-MADR Amendment A1, P1): until the test
		// releases it, or until ctx ends, when ExecRunner would kill it.
		select {
		case <-block:
		case <-ctx.Done():
			return service.Output{}, ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.onRun != nil {
		f.onRun(f.props)
	}
	f.calls = append(f.calls, append([]string{c.Path}, c.Args...))
	f.envs = append(f.envs, c.Env)
	args := c.Args
	if len(args) > 0 && args[0] == "--user" {
		args = args[1:]
	}
	if strings.HasSuffix(c.Path, "systemd-run") {
		if out, ok := f.fail["systemd-run"]; ok {
			return out, nil
		}
		return service.Output{}, nil
	}
	if len(args) == 0 {
		return service.Output{ExitCode: 1}, nil
	}
	verb := args[0]
	if out, ok := f.fail[verb]; ok {
		return out, nil
	}
	switch verb {
	case "--version":
		return service.Output{Stdout: []byte(f.version + "\n+PAM +AUDIT\n")}, nil
	case "show":
		return service.Output{Stdout: []byte(f.showLocked(args))}, nil
	case "stop":
		if f.onStop != nil {
			f.onStop(f.props)
		} else {
			f.props["ActiveState"], f.props["SubState"] = "inactive", "dead"
		}
	case "start":
		if f.onStart != nil {
			f.onStart(f.props)
		} else {
			f.props["ActiveState"], f.props["SubState"], f.props["InvocationID"] = "active", "running", "bbb"
		}
	case "daemon-reload":
		if !f.reloadIgnored {
			f.props["NeedDaemonReload"] = "no"
		}
		if f.onReload != nil {
			f.onReload(f.props)
		}
	}
	return service.Output{}, nil
}

// showLocked prints the -p properties asked for, as systemctl show does.
func (f *fakeSystemd) showLocked(args []string) string {
	props := f.props
	if len(f.showQueue) > 0 {
		props, f.showQueue = f.showQueue[0], f.showQueue[1:]
	}
	var names []string
	for i, a := range args {
		if a == "-p" && i+1 < len(args) {
			names = strings.Split(args[i+1], ",")
		}
	}
	var b strings.Builder
	for _, n := range names {
		b.WriteString(n + "=" + props[n] + "\n")
	}
	return b.String()
}

// verbs lists the systemctl verbs called, without --user.
func (f *fakeSystemd) verbs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, c := range f.calls {
		args := c[1:]
		if len(args) > 0 && args[0] == "--user" {
			args = args[1:]
		}
		if len(args) > 0 {
			out = append(out, args[0])
		}
	}
	return out
}

// testUnit is a Unit over f, for Linux, with fast polling and the cgroup
// seam saying this process is outside the unit.
func testUnit(t *testing.T, f *fakeSystemd, o Options) *Unit {
	t.Helper()
	if o.Unit == "" {
		o.Unit = "demo.service"
	}
	o.Systemctl, o.SystemdRun, o.Runner = "/usr/bin/systemctl", "/usr/bin/systemd-run", f
	if o.Poll.Interval == 0 {
		o.Poll.Interval = 1
	}
	if o.Poll.Settle == 0 {
		o.Poll.Settle = -1
	}
	if o.Poll.Timeout == 0 {
		o.Poll.Timeout = 2e9
	}
	env := map[string]string{"XDG_RUNTIME_DIR": t.TempDir()}
	u, err := newUnit(o, "linux", func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	setCgroup(t, "0::/user.slice/user-1000.slice/session-3.scope\n")
	return u
}

// bubblePoll is the poll of a test that runs in a synctest bubble: the
// documented defaults, with no settle window. testUnit's 1 ns interval
// suits the real clock, where the fake answers at once; in a bubble it
// would be billions of steps of fake time
// (docs/decisions/0038-MADR-pinned-plans-lifecycle-conformance-and-bubble-tests.md
// D3-A).
var bubblePoll = service.PollOptions{Interval: service.DefaultPollInterval, Timeout: service.DefaultPollTimeout, Settle: -1}

func setCgroup(t *testing.T, content string) {
	t.Helper()
	prev := readSelfCgroup
	readSelfCgroup = func() ([]byte, error) { return []byte(content), nil }
	t.Cleanup(func() { readSelfCgroup = prev })
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestFakeHonoursContext: the fake refuses a call on an ended context, and
// records nothing, as ExecRunner does
// (docs/decisions/0037-MADR-honour-contexts-in-backend-fakes-and-stop-commands.md D1-A).
func TestFakeHonoursContext(t *testing.T) {
	f := newFake()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := f.Run(ctx, service.Command{Path: "/usr/bin/systemctl", Args: []string{"show"}}); !errors.Is(err, context.Canceled) || len(f.calls) != 0 {
		t.Fatalf("Run on a cancelled context: %v, calls %q; want context.Canceled and none", err, f.calls)
	}
}
