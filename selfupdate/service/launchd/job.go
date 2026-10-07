// Package launchd is the reference launchd lifecycle for a managed update
// (docs/decisions/0011-MADR-reference-service-lifecycles.md): a
// selfupdate.Lifecycle, selfupdate.EnabledLifecycle, selfupdate.Reconciler
// and service.Detacher for one LaunchDaemon or LaunchAgent.
//
// It runs launchctl and plutil at absolute paths, and relies on launchctl's
// exit codes and its documented-stable `list` output. `print` output, which
// launchctl(1) says is not an interface, is read only for a domain `list`
// cannot see, and only its "pid =" and "state =" lines. It compiles on every
// OS; New returns service.ErrUnsupported off macOS.
package launchd

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// Domain is a launchd domain: system, a user's GUI login (gui/<uid>), or a
// user's background domain (user/<uid>). There is no default: the zero
// Domain is invalid (0011-MADR §2).
type Domain struct {
	kind string
	uid  int
}

// The domain kinds, as launchctl names them.
const (
	kindSystem = "system"
	kindGUI    = "gui"
	kindUser   = "user"
)

// System is the system domain, for LaunchDaemons. Changing it needs root.
func System() Domain { return Domain{kind: kindSystem} }

// GUI is uid's GUI login domain, for a LaunchAgent in a logged-in session.
func GUI(uid int) Domain { return Domain{kind: kindGUI, uid: uid} }

// User is uid's background domain, which exists without a login session.
func User(uid int) Domain { return Domain{kind: kindUser, uid: uid} }

// String is the domain target, such as "gui/501".
func (d Domain) String() string {
	if d.kind == kindSystem {
		return kindSystem
	}
	return d.kind + "/" + strconv.Itoa(d.uid)
}

func (d Domain) valid() bool {
	return d.kind == kindSystem || (d.kind == kindGUI || d.kind == kindUser) && d.uid >= 0
}

// launchctl's exit codes, as `launchctl error <n>` decodes them on macOS
// 26 (0011-MADR, Probe evidence).
const (
	exitNotPermitted = 1   // Operation not permitted
	exitNoProcess    = 3   // No such process
	exitIO           = 5   // Input/output error
	exitInProgress   = 36  // Operation now in progress
	exitAlready      = 37  // Operation already in progress
	exitNotFound     = 113 // Could not find specified service
	exitDisabled     = 119 // Service is disabled
	exitSIP          = 150 // Operation not permitted while SIP is engaged
)

// Options configure a Job.
type Options struct {
	// Label is the job's label, such as "com.example.relay". Required.
	Label string
	// Domain is the job's domain. Required.
	Domain Domain
	// Plist is the job's property list, such as
	// "/Library/LaunchDaemons/com.example.relay.plist". Required.
	Plist string
	// Launchctl, Plutil and Ps are the tools' absolute paths. Empty means
	// /bin/launchctl, /usr/bin/plutil and /bin/ps.
	Launchctl, Plutil, Ps string
	// Runner runs the tools. Nil means service.ExecRunner.
	Runner service.Runner
	// Probe, when set, is an application readiness check WaitHealthy runs
	// once launchd reports the job healthy.
	Probe service.HealthProbe
	// Poll bounds stop, start and health waits. The health settle window
	// is at least launchd's default ThrottleInterval, 10 s, unless Settle
	// is negative. The stop wait is at least the job's ExitTimeOut, 5 s
	// when the plist does not set it, plus 30 s.
	Poll service.PollOptions
	// RewritePath lets Reconcile point the plist at a binary that moved.
	// A job loaded but not running is reloaded so launchd reads it; a
	// running job is refused.
	RewritePath bool
	// JobDir is where Detach writes its one-shot jobs' property lists and
	// environment files. Empty means /Library/Application
	// Support/selfupdate/handoff in the system domain, and
	// ~/Library/Application Support/selfupdate/handoff otherwise.
	JobDir string
}

// Job manages one launchd job.
type Job struct {
	o                     Options
	launchctl, plutil, ps string
	uid, euid             int
	managerOnce           sync.Once
	manager               string
	mu                    sync.Mutex
	previous              int // the job's PID before this update
	// reloaded marks a job Reconcile or Restore reloaded: a process it
	// started at the bootstrap runs the new definition, and Start does not
	// take it for the one before the update (0015-MADR D3).
	reloaded bool
	// groups is processGroups; the tests answer it themselves, so that no
	// real process can share a fixture PID's group.
	groups func(pid int) (mine, theirs int, ok bool)
}

var (
	_ selfupdate.Lifecycle        = (*Job)(nil)
	_ selfupdate.EnabledLifecycle = (*Job)(nil)
	_ selfupdate.Reconciler       = (*Job)(nil)
	_ service.Detacher            = (*Job)(nil)
)

// statFile is os.Stat, replaced in tests.
var statFile = os.Stat

// New returns a Job for o. Off macOS it returns service.ErrUnsupported.
func New(o Options) (*Job, error) {
	return newJob(o, runtime.GOOS, os.Getuid(), os.Geteuid())
}

func newJob(o Options, goos string, uid, euid int) (*Job, error) {
	if goos != "darwin" {
		return nil, fmt.Errorf("%w: launchd runs on macOS, not %s", service.ErrUnsupported, goos)
	}
	if err := validLabel(o.Label); err != nil {
		return nil, err
	}
	if !o.Domain.valid() {
		return nil, errors.New("selfupdate: launchd: Options.Domain is required: System(), GUI(uid) or User(uid)")
	}
	if !filepath.IsAbs(o.Plist) {
		return nil, fmt.Errorf("selfupdate: launchd: plist path %q is not absolute", o.Plist)
	}
	j := &Job{o: o, uid: uid, euid: euid, groups: processGroups}
	for _, t := range []struct {
		dst          *string
		given, deflt string
	}{{&j.launchctl, o.Launchctl, "/bin/launchctl"}, {&j.plutil, o.Plutil, "/usr/bin/plutil"}, {&j.ps, o.Ps, "/bin/ps"}} {
		*t.dst = t.deflt
		if t.given != "" {
			if !filepath.IsAbs(t.given) {
				return nil, fmt.Errorf("selfupdate: launchd: tool path %q is not absolute", t.given)
			}
			*t.dst = t.given
		}
	}
	if o.Runner == nil {
		j.o.Runner = service.ExecRunner()
	}
	return j, nil
}

// validLabel accepts a reverse-DNS label: letters, digits, ".", "-" and "_",
// with no "/" and no leading "-" or ".".
func validLabel(label string) error {
	if label == "" || len(label) > 255 || label[0] == '-' || label[0] == '.' {
		return fmt.Errorf("selfupdate: launchd: label %q is not a reverse-DNS label", label)
	}
	for i := 0; i < len(label); i++ {
		if !labelChar(label[i]) {
			return fmt.Errorf("selfupdate: launchd: label %q has the character %q", label, label[i])
		}
	}
	return nil
}

func labelChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '-' || c == '_'
}

// env is the tools' environment: built, never inherited.
var env = []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "LC_ALL=C"}

func (j *Job) launchctlRun(ctx context.Context, args ...string) (service.Output, error) {
	return j.o.Runner.Run(ctx, service.Command{Path: j.launchctl, Args: args, Env: env})
}

func (j *Job) target() string { return j.o.Domain.String() + "/" + j.o.Label }

// state is what launchd knows of the job.
type state struct {
	loaded  bool
	pid     int
	running bool
}

// managerName is `launchctl managername`, cached: "Aqua" in a GUI login,
// "Background" in a background session, "System" for root.
func (j *Job) managerName(ctx context.Context) string {
	j.managerOnce.Do(func() {
		if out, err := j.launchctlRun(ctx, "managername"); err == nil && out.ExitCode == 0 {
			j.manager = strings.TrimSpace(string(out.Stdout))
		}
	})
	return j.manager
}

// listSees reports whether `launchctl list`, which reads the caller's own
// domain, sees the job's domain.
func (j *Job) listSees(ctx context.Context) bool {
	d := j.o.Domain
	if d.kind == kindSystem {
		return j.euid == 0
	}
	if d.uid != j.uid || j.euid != j.uid {
		return false
	}
	m := j.managerName(ctx)
	return d.kind == kindGUI && m == "Aqua" || d.kind == kindUser && m == "Background"
}

// probe reads the job's state: from `launchctl list <label>` when it sees
// the domain, its documented-stable output; otherwise from `launchctl print`,
// reading only its "pid =" and "state =" lines (0011-MADR §7). Exit 113
// means the job is not loaded.
func (j *Job) probe(ctx context.Context) (state, error) {
	if j.listSees(ctx) {
		out, err := j.launchctlRun(ctx, "list", j.o.Label)
		if err != nil {
			return state{}, err
		}
		switch out.ExitCode {
		case 0:
			pid := listPID(out.Stdout)
			return state{loaded: true, pid: pid, running: pid > 0}, nil
		case exitNotFound:
			return state{}, nil
		}
		return state{}, launchctlError("list", j.o.Label, out)
	}
	out, err := j.launchctlRun(ctx, "print", j.target())
	if err != nil {
		return state{}, err
	}
	switch out.ExitCode {
	case 0:
		pid, st := printState(out.Stdout)
		return state{loaded: true, pid: pid, running: st == "running" && pid > 0}, nil
	case exitNotFound:
		return state{}, nil
	}
	return state{}, launchctlError("print", j.target(), out)
}

// listPID reads `"PID" = 123;` from `launchctl list <label>`. A job that is
// not running has no PID line.
func listPID(out []byte) int {
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if v, ok := strings.CutPrefix(line, `"PID" = `); ok {
			n, err := strconv.Atoi(strings.TrimSuffix(v, ";"))
			if err == nil {
				return n
			}
		}
	}
	return 0
}

// printState reads the top-level "pid = " and "state = " lines of
// `launchctl print`, the only ones this package reads.
func printState(out []byte) (pid int, st string) {
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		// Top-level properties are indented by exactly one tab.
		if !strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "\t\t") {
			continue
		}
		line = strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(line, "pid = "); ok {
			if n, err := strconv.Atoi(v); err == nil {
				pid = n
			}
		}
		if v, ok := strings.CutPrefix(line, "state = "); ok {
			st = v
		}
	}
	return pid, st
}

// launchctlError classifies a failed launchctl run by its exit code.
func launchctlError(step, what string, out service.Output) error {
	msg := strings.TrimSpace(string(out.Stderr))
	if msg == "" {
		msg = strings.TrimSpace(string(out.Stdout))
	}
	err := fmt.Errorf("selfupdate: launchd: %s %s: exit %d: %s", step, what, out.ExitCode, msg)
	switch out.ExitCode {
	case exitNotPermitted, exitSIP:
		return fmt.Errorf("%w: %w", service.ErrPermission, err)
	case exitNotFound:
		return fmt.Errorf("%w: %w", service.ErrNotInstalled, err)
	case exitDisabled:
		return fmt.Errorf("%w (the job is disabled; launchctl enable clears it)", err)
	}
	return err
}
