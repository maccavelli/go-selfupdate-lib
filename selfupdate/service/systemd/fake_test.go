package systemd

import (
	"context"
	"os"
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

func (f *fakeSystemd) Run(_ context.Context, c service.Command) (service.Output, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
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
