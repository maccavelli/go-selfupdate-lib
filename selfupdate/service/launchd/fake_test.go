package launchd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// fakeLaunchd answers launchctl, plutil and ps from an in-memory job, and
// records every call (0011-PLAN V3 step 8).
type fakeLaunchd struct {
	mu       sync.Mutex
	calls    [][]string
	loaded   bool
	pid      int
	state    string
	manager  string
	disabled string // print-disabled's value for the label, or ""
	keys     map[string]string
	// goneAfter: bootout leaves the job loaded for this many prints.
	goneAfter int
	// bootstrapCodes are bootstrap's exit codes, in turn; then 0.
	bootstrapCodes []int
	// onStart runs on kickstart, as the job starting.
	onStart func(f *fakeLaunchd)
	// parents maps a PID to its parent, for ps.
	parents map[int]int
	// groups maps a PID to its process group; this process is in group 1.
	// A PID it does not list has no group, as a process that has gone.
	groups map[int]int
	// fail maps a verb to its output.
	fail map[string]service.Output
	// handOffs answers print for one-shot jobs, by label.
	handOffs map[string]service.Output
	// types answers plutil -type for a key; a key in keys without one is a
	// bool for true or false, an integer for digits, else a dictionary.
	types map[string]string
	// corrupt makes the plist convert to a root that is not a dictionary,
	// as a file holding a bare word does.
	corrupt bool
	// onBootout runs on bootout, before the job starts going.
	onBootout func(f *fakeLaunchd)
	// printDeadlines records, for each print of the job, how long its
	// context had left: zero when it had no deadline.
	printDeadlines []time.Duration
}

func newFake() *fakeLaunchd {
	return &fakeLaunchd{
		loaded: true, pid: 100, state: "running", manager: "Aqua",
		keys: map[string]string{"RunAtLoad": "true", "ProgramArguments.0": "/opt/demo/demo"},
		onStart: func(f *fakeLaunchd) {
			f.loaded, f.pid, f.state = true, 200, "running"
		},
		parents: map[int]int{}, groups: map[int]int{}, fail: map[string]service.Output{}, handOffs: map[string]service.Output{},
	}
}

// processGroups answers the Job's group lookup from f.groups, never from
// the system (0014-PLAN deviation D9).
func (f *fakeLaunchd) processGroups(pid int) (mine, theirs int, ok bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	g, ok := f.groups[pid]
	return 1, g, ok
}

func (f *fakeLaunchd) Run(ctx context.Context, c service.Command) (service.Output, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, append([]string{c.Path}, c.Args...))
	switch {
	case strings.HasSuffix(c.Path, "plutil"):
		return f.plutil(c.Args), nil
	case strings.HasSuffix(c.Path, "/ps"):
		var pid int
		fmt.Sscan(c.Args[len(c.Args)-1], &pid)
		if parent, ok := f.parents[pid]; ok {
			return service.Output{Stdout: []byte(fmt.Sprintf("%5d\n", parent))}, nil
		}
		return service.Output{Stdout: []byte("    1\n")}, nil
	}
	verb := c.Args[0]
	if out, ok := f.fail[verb]; ok {
		return out, nil
	}
	switch verb {
	case "managername":
		return service.Output{Stdout: []byte(f.manager + "\n")}, nil
	case "list":
		if !f.loaded {
			return service.Output{ExitCode: exitNotFound}, nil
		}
		body := "{\n\t\"LimitLoadToSessionType\" = \"Aqua\";\n\t\"Label\" = \"com.example.demo\";\n"
		if f.pid > 0 {
			body += fmt.Sprintf("\t\"PID\" = %d;\n", f.pid)
		}
		return service.Output{Stdout: []byte(body + "\t\"LastExitStatus\" = 0;\n};\n")}, nil
	case "print":
		if out, ok := f.handOffs[c.Args[1]]; ok {
			return out, nil
		}
		var left time.Duration
		if dl, ok := ctx.Deadline(); ok {
			left = time.Until(dl)
		}
		f.printDeadlines = append(f.printDeadlines, left)
		if !f.loaded {
			return service.Output{ExitCode: exitNotFound, Stderr: []byte("Could not find service")}, nil
		}
		if f.goneAfter > 0 {
			f.goneAfter--
			if f.goneAfter == 0 {
				f.loaded = false
			}
		}
		return service.Output{Stdout: []byte(printOutput(f.pid, f.state))}, nil
	case "print-disabled":
		body := "disabled services = {\n\t\"com.apple.other\" => enabled\n"
		if f.disabled != "" {
			body += "\t\"com.example.demo\" => " + f.disabled + "\n"
		}
		return service.Output{Stdout: []byte(body + "}\n")}, nil
	case "bootout":
		if f.onBootout != nil {
			f.onBootout(f)
		}
		if f.goneAfter == 0 {
			f.loaded = false
		}
		f.pid, f.state = 0, "not running"
	case "bootstrap":
		if len(f.bootstrapCodes) > 0 {
			code := f.bootstrapCodes[0]
			f.bootstrapCodes = f.bootstrapCodes[1:]
			if code != 0 {
				return service.Output{ExitCode: code, Stderr: []byte(fmt.Sprintf("Bootstrap failed: %d", code))}, nil
			}
		}
		f.loaded = true
	case "kickstart":
		if f.onStart != nil {
			f.onStart(f)
		}
	}
	return service.Output{}, nil
}

// printOutput is a `launchctl print` with a nested pid line, which must
// not be read.
func printOutput(pid int, state string) string {
	return fmt.Sprintf("gui/503/com.example.demo = {\n\tactive count = 1\n\tpath = /x.plist\n\ttype = LaunchAgent\n"+
		"\tstate = %s\n\n\tprogram = /opt/demo/demo\n\tendpoints = {\n\t\tpid = 999\n\t}\n\tpid = %d\n\timmediate reason = speculative\n}\n", state, pid)
}

func (f *fakeLaunchd) plutil(args []string) service.Output {
	switch args[0] {
	case "-convert":
		root := "<dict>\n</dict>"
		if f.corrupt {
			root = "<string>garbage</string>"
		}
		return service.Output{Stdout: []byte("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<plist version=\"1.0\">\n" + root + "\n</plist>\n")}
	case "-type":
		v, ok := f.keys[args[1]]
		if !ok {
			return service.Output{ExitCode: 1, Stderr: []byte("No value at that key path")}
		}
		typ, ok := f.types[args[1]]
		switch {
		case ok:
		case v == "true" || v == "false":
			typ = "bool"
		case v != "" && strings.Trim(v, "0123456789") == "":
			typ = "integer"
		default:
			typ = "dictionary"
		}
		return service.Output{Stdout: []byte(typ + "\n")}
	case "-extract":
		if args[2] != "raw" {
			return service.Output{ExitCode: 1, Stderr: []byte("the fake answers only raw, as Enabled asks")}
		}
		v, ok := f.keys[args[1]]
		if !ok {
			return service.Output{ExitCode: 1, Stderr: []byte("No value at that key path")}
		}
		return service.Output{Stdout: []byte(v + "\n")}
	case "-replace":
		path := args[len(args)-1]
		if err := os.WriteFile(path, []byte("replaced "+args[1]+" with "+args[3]), 0o600); err != nil {
			return service.Output{ExitCode: 1, Stderr: []byte(err.Error())}
		}
	}
	return service.Output{}
}

func (f *fakeLaunchd) verbs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, c := range f.calls {
		if strings.HasSuffix(c[0], "launchctl") {
			out = append(out, c[1])
		}
	}
	return out
}

// testJob is a Job over f for macOS, in gui/503 as uid 503, with fast
// polling, and a plist in a temporary directory.
func testJob(t *testing.T, f *fakeLaunchd, o Options) *Job {
	t.Helper()
	dir := t.TempDir()
	if o.Label == "" {
		o.Label = "com.example.demo"
	}
	if o.Domain == (Domain{}) {
		o.Domain = GUI(503)
	}
	if o.Plist == "" {
		o.Plist = filepath.Join(dir, "com.example.demo.plist")
		if err := os.WriteFile(o.Plist, []byte("<plist/>"), 0o644); err != nil { //nolint:gosec // a fixture plist
			t.Fatal(err)
		}
	}
	if o.JobDir == "" {
		o.JobDir = filepath.Join(dir, "handoff")
	}
	o.Runner = f
	if o.Poll.Interval == 0 {
		o.Poll.Interval = time.Millisecond
	}
	if o.Poll.Settle == 0 {
		o.Poll.Settle = -1
	}
	if o.Poll.Timeout == 0 {
		o.Poll.Timeout = 2 * time.Second
	}
	j, err := newJob(o, "darwin", 503, 503)
	if err != nil {
		t.Fatal(err)
	}
	j.groups = f.processGroups
	return j
}
