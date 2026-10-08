// Package systemd is the reference systemd lifecycle for a managed update
// (docs/decisions/0011-MADR-reference-service-lifecycles.md): a
// selfupdate.Lifecycle, selfupdate.EnabledLifecycle, selfupdate.Reconciler
// and service.Detacher for one service unit, plus Notify, the sd_notify
// protocol for the service itself.
//
// It runs systemctl and systemd-run at absolute paths with a built
// environment, and parses only `systemctl show`, which systemd documents as
// stable. It compiles on every OS; New returns service.ErrUnsupported off
// Linux.
package systemd

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// Scope selects the service manager.
type Scope int

const (
	// System is the system manager, PID 1.
	System Scope = iota
	// User is the calling user's manager, systemctl --user.
	User
)

// Options configure a Unit.
type Options struct {
	// Unit is the service unit, such as "relay.service". Empty means
	// "<product>.service", when that is a valid unit name.
	Unit string
	// Scope selects the system or the user manager. It is never switched
	// automatically (0011-MADR §2).
	Scope Scope
	// Systemctl and SystemdRun are the tools' absolute paths. Empty means
	// /usr/bin, then /bin.
	Systemctl  string
	SystemdRun string
	// Runner runs the tools. Nil means service.ExecRunner.
	Runner service.Runner
	// Probe, when set, is an application readiness check WaitHealthy runs
	// once systemd reports the unit healthy.
	Probe service.HealthProbe
	// Poll bounds stop, start and health waits. The stop wait is at least
	// the unit's TimeoutStopUSec plus 30 s.
	Poll service.PollOptions
	// RewritePath lets Reconcile point the unit at a binary that moved, with
	// a drop-in. Without it, a unit running another binary is an error. A
	// drop-in that loads later and sets ExecStart again makes the rewrite
	// an error.
	RewritePath bool
	// DropInDir is where Reconcile writes its drop-in directory. Empty means
	// /etc/systemd/system, or $XDG_CONFIG_HOME/systemd/user (by default
	// ~/.config/systemd/user) in user scope.
	DropInDir string
}

// Unit manages one systemd service unit.
type Unit struct {
	o         Options
	systemctl string
	run       string
	env       []string
	getenv    func(string) string

	mu       sync.Mutex
	baseline map[string]baseline
}

// baseline is what a unit ran before this update started it.
type baseline struct {
	invocation string
	restarts   string
}

var (
	_ selfupdate.Lifecycle        = (*Unit)(nil)
	_ selfupdate.EnabledLifecycle = (*Unit)(nil)
	_ selfupdate.Reconciler       = (*Unit)(nil)
	_ service.Detacher            = (*Unit)(nil)
)

// toolDirs are where the tools are looked for, never PATH.
var toolDirs = []string{"/usr/bin", "/bin"}

// statFile is os.Stat, replaced in tests.
var statFile = os.Stat

// New returns a Unit for o. Off Linux it returns service.ErrUnsupported. In
// user scope it needs XDG_RUNTIME_DIR, which the user manager's bus lives
// under.
func New(o Options) (*Unit, error) {
	return newUnit(o, runtime.GOOS, os.Getenv)
}

func newUnit(o Options, goos string, getenv func(string) string) (*Unit, error) {
	if goos != "linux" {
		return nil, fmt.Errorf("%w: systemd runs on Linux, not %s", service.ErrUnsupported, goos)
	}
	if o.Unit != "" {
		if err := ValidUnitName(o.Unit); err != nil {
			return nil, err
		}
	}
	if o.Scope != System && o.Scope != User {
		return nil, fmt.Errorf("selfupdate: systemd: unknown scope %d", o.Scope)
	}
	if err := o.Poll.Validate(); err != nil {
		return nil, fmt.Errorf("selfupdate: systemd: %w", err)
	}
	u := &Unit{o: o, getenv: getenv, baseline: map[string]baseline{}}
	var err error
	if u.systemctl, err = tool(o.Systemctl, "systemctl"); err != nil {
		return nil, err
	}
	if u.run, err = tool(o.SystemdRun, "systemd-run"); err != nil {
		return nil, err
	}
	if o.Runner == nil {
		u.o.Runner = service.ExecRunner()
	}
	u.env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LC_ALL=C", "SYSTEMD_PAGER=cat", "SYSTEMD_COLORS=0"}
	if o.Scope == User {
		runtimeDir := getenv("XDG_RUNTIME_DIR")
		if runtimeDir == "" {
			return nil, fmt.Errorf("selfupdate: systemd: user scope needs XDG_RUNTIME_DIR")
		}
		u.env = append(u.env, "XDG_RUNTIME_DIR="+runtimeDir)
		if bus := getenv("DBUS_SESSION_BUS_ADDRESS"); bus != "" {
			u.env = append(u.env, "DBUS_SESSION_BUS_ADDRESS="+bus)
		}
	}
	return u, nil
}

// tool resolves a tool's absolute path: the one given, or the first of
// toolDirs that has it.
func tool(given, name string) (string, error) {
	if given != "" {
		if !strings.HasPrefix(given, "/") {
			return "", fmt.Errorf("selfupdate: systemd: %s path %q is not absolute", name, given)
		}
		return given, nil
	}
	for _, dir := range toolDirs {
		p := dir + "/" + name
		if _, err := statFile(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("%w: %s is not in %s", service.ErrUnsupported, name, strings.Join(toolDirs, " or "))
}

// Available reports whether this host runs systemd (sd_booted(3): the
// directory /run/systemd/system exists) and has systemctl.
func Available() bool {
	if _, err := statFile("/run/systemd/system"); err != nil {
		return false
	}
	_, err := tool("", "systemctl")
	return err == nil
}

// unit names product's unit: Options.Unit, or <product>.service.
func (u *Unit) unit(product string) (string, error) {
	if u.o.Unit != "" {
		return u.o.Unit, nil
	}
	name := product + ".service"
	if err := ValidUnitName(name); err != nil {
		return "", fmt.Errorf("selfupdate: systemd: no Options.Unit, and %w", err)
	}
	return name, nil
}

// systemctl runs systemctl with the scope's flag and args.
func (u *Unit) systemctlRun(ctx context.Context, args ...string) (service.Output, error) {
	full := make([]string, 0, len(args)+1)
	if u.o.Scope == User {
		full = append(full, "--user")
	}
	full = append(full, args...)
	return u.o.Runner.Run(ctx, service.Command{Path: u.systemctl, Args: full, Env: u.env})
}

// properties is `systemctl show` output, by key.
type properties map[string]string

// showProps are the properties every probe reads (0011-MADR §7).
var showProps = []string{
	"Id", "LoadState", "ActiveState", "SubState", "UnitFileState", "InvocationID",
	"MainPID", "NRestarts", "Result", "NeedDaemonReload", "ControlGroup",
}

// show reads unit's properties, parsed by key. systemd documents `show`
// as the machine-readable interface; a unit that does not exist reports
// LoadState=not-found.
func (u *Unit) show(ctx context.Context, unit string, props ...string) (properties, error) {
	if len(props) == 0 {
		props = showProps
	}
	out, err := u.systemctlRun(ctx, "show", "--no-pager", "-p", strings.Join(props, ","), "--", unit)
	if err != nil {
		return nil, err
	}
	if out.ExitCode != 0 {
		return nil, commandError("show", unit, out)
	}
	p := properties{}
	sc := bufio.NewScanner(bytes.NewReader(out.Stdout))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		if k, v, ok := strings.Cut(sc.Text(), "="); ok {
			p[k] = v
		}
	}
	return p, sc.Err()
}

// commandError classifies a failed systemctl run by the message systemd
// gave: the exit codes are not to be relied on (systemctl(1)).
func commandError(step, unit string, out service.Output) error {
	msg := strings.TrimSpace(string(out.Stderr))
	if msg == "" {
		msg = strings.TrimSpace(string(out.Stdout))
	}
	err := fmt.Errorf("selfupdate: systemd: %s %s: exit %d: %s", step, unit, out.ExitCode, msg)
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "access denied"), strings.Contains(lower, "authentication required"),
		strings.Contains(lower, "permission denied"):
		return fmt.Errorf("%w: %w", service.ErrPermission, err)
	case unitMissing(lower, strings.ToLower(unit)):
		return fmt.Errorf("%w: %w", service.ErrNotInstalled, err)
	}
	return err
}

// unitMissing reports systemd saying this unit, not another, is missing: a
// start that fails on a missing dependency names the dependency
// (0015-MADR D11). lower and unit are lowercased.
func unitMissing(lower, unit string) bool {
	for _, phrase := range []string{
		"unit " + unit + " not found",
		"unit " + unit + " not loaded",
		"unit " + unit + " could not be found",
		"unit file " + unit + " does not exist",
	} {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}
