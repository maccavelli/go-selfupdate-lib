package service

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/internal/procfs"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
)

// SupervisedState is one observation of a service under a supervisor this
// module has no backend for, as the program's SupervisedOptions.State reads
// it, such as from runit's `sv status` or s6's `s6-svstat -o`
// (docs/decisions/0035-MADR-serve-supervisors-through-a-supervised-adapter.md
// D2-A).
type SupervisedState struct {
	// Installed is whether the service's definition exists: a service
	// directory or an init script.
	Installed bool `json:"installed"`
	// Supervised is whether a supervisor serves the service now.
	Supervised bool `json:"supervised"`
	// Up is whether the service's process runs.
	Up bool `json:"up"`
	// PID is the service's own process while it is up; zero otherwise.
	PID int `json:"pid,omitempty"`
	// Instance is the running process as "<pid>@<start time>", which a
	// reused PID cannot match. The adapter fills it; State leaves it empty
	// (D3-A).
	Instance string `json:"instance,omitempty"`
	// WantUp is whether the supervisor will start the service when it is
	// down.
	WantUp bool `json:"want_up"`
	// NormallyUp is whether the service starts when its supervisor starts:
	// for runit and s6, no down file; for OpenRC, in the default runlevel.
	// It is Enabled's answer.
	NormallyUp bool `json:"normally_up"`
	// Restarting is a service that is down while the supervisor wants it
	// up: it exited and is being started again, such as runit's
	// "down: …, want up" or s6's wantedup with up false.
	Restarting bool `json:"restarting"`
	// Failed is a service the supervisor reports as failed for good, such
	// as OpenRC's "crashed" (status 32; Amendment A1, P4).
	Failed bool `json:"failed"`
	// Ready is the service's own readiness, where the supervisor reports
	// it, such as s6 with a notification-fd. Nil means the supervisor
	// does not report readiness, so Up alone counts; s6 reports false for
	// a service with no notification-fd, so a program sets Ready only when
	// the service has one (Amendment A1, P2).
	Ready *bool `json:"ready,omitempty"`
	// Detail describes the observation, for an error message.
	Detail string `json:"detail,omitempty"`
}

// SupervisedOptions describe a service under a supervisor this module has
// no backend for. State, Up and Down are required; the rest are optional
// (D2-A).
type SupervisedOptions struct {
	// Name names the service in errors. Empty means "the service".
	Name string
	// State observes the service once. When nothing supervises it, State
	// returns an error wrapping ErrNoSupervisor, with a state that still
	// says whether the service is installed and normally up.
	State func(ctx context.Context) (SupervisedState, error)
	// Up asks the supervisor to start the service and keep it up, such as
	// `sv up <dir>` or `s6-svc -u <dir>`. It may first start a supervisor
	// for the session. It does not wait: the adapter does.
	Up func(ctx context.Context) error
	// Down asks the supervisor to stop the service and not restart it,
	// such as `sv down <dir>` or `s6-svc -d <dir>`. It does not wait.
	Down func(ctx context.Context) error
	// Kill, when set, asks the supervisor to kill the service's process,
	// such as `sv kill <dir>` or `s6-svc -k <dir>`. Stop calls it once,
	// when its wait reaches KillAfter (D4-C).
	Kill func(ctx context.Context) error
	// KillAfter is how long Stop waits before it calls Kill. Zero means
	// half of Poll's timeout. It must be shorter than that timeout, and
	// is refused without a Kill.
	KillAfter time.Duration
	// Poll bounds Stop's wait and the health wait.
	Poll PollOptions
	// Probe, when set, is the program's application check, which
	// WaitHealthy runs once the supervisor reports a new instance settled.
	Probe HealthProbe
	// DetachCheck, when set, runs before a handoff's detached start. An
	// error refuses the handoff, such as for an OpenRC system service that
	// kills its cgroup on stop (D8-A; Amendment A1, P4).
	DetachCheck func(ctx context.Context) error
}

var (
	_ selfupdate.Lifecycle        = (*Supervised)(nil)
	_ selfupdate.EnabledLifecycle = (*Supervised)(nil)
	_ selfupdate.Reconciler       = (*Supervised)(nil)
	_ Detacher                    = (*Supervised)(nil)
)

// goosLinux is runtime.GOOS on Linux, the one OS the adapter runs on.
const goosLinux = "linux"

// maxAncestry bounds Inside's walk up the process tree, in case a parent
// chain loops.
const maxAncestry = 4096

// instance is a process by PID and start time.
type instance struct {
	pid   int
	start uint64
}

func (i instance) String() string { return fmt.Sprintf("%d@%d", i.pid, i.start) }

// Supervised is a selfupdate.Lifecycle, selfupdate.EnabledLifecycle,
// selfupdate.Reconciler and Detacher for a service under a supervisor this
// module has no backend for: runit, s6, OpenRC, or any other the program
// can observe and command through SupervisedOptions
// (docs/decisions/0035-MADR-serve-supervisors-through-a-supervised-adapter.md).
// It is safe for concurrent use; the updater drives one product's calls in
// order, and a program that calls them itself must do the same.
// NewSupervised returns ErrUnsupported off Linux.
type Supervised struct {
	o    SupervisedOptions
	name string

	// readStat, exe and self read /proc and this process; tests replace
	// them.
	readStat func(pid int) (procfs.Stat, error)
	exe      func(pid int) (string, error)
	self     func() int
	detach   func(context.Context, HandOff) (Detached, error)

	mu sync.Mutex
	// recorded is the instance Stop saw, and binary the binary it ran.
	recorded instance
	binary   string
}

// NewSupervised returns a Supervised for o. It refuses a missing State, Up
// or Down; a Poll that PollOptions.Validate refuses; and a KillAfter that is
// negative, set without a Kill, or not shorter than Poll's timeout. Off
// Linux it returns ErrUnsupported (D12-A).
func NewSupervised(o SupervisedOptions) (*Supervised, error) {
	return supervisedOn(o, runtime.GOOS)
}

func supervisedOn(o SupervisedOptions, goos string) (*Supervised, error) {
	if goos != goosLinux {
		return nil, fmt.Errorf("%w: the supervised adapter reads /proc, which %s does not have", ErrUnsupported, goos)
	}
	switch {
	case o.State == nil:
		return nil, errors.New("selfupdate: service: SupervisedOptions.State is required")
	case o.Up == nil:
		return nil, errors.New("selfupdate: service: SupervisedOptions.Up is required")
	case o.Down == nil:
		return nil, errors.New("selfupdate: service: SupervisedOptions.Down is required")
	}
	if err := o.Poll.Validate(); err != nil {
		return nil, fmt.Errorf("selfupdate: service: %w", err)
	}
	timeout := o.Poll.withDefaults().Timeout
	switch {
	case o.KillAfter < 0:
		return nil, fmt.Errorf("selfupdate: service: SupervisedOptions.KillAfter %s is negative", o.KillAfter)
	case o.KillAfter > 0 && o.Kill == nil:
		return nil, errors.New("selfupdate: service: SupervisedOptions.KillAfter is set, but Kill is not")
	case o.KillAfter >= timeout:
		return nil, fmt.Errorf("selfupdate: service: SupervisedOptions.KillAfter %s is not shorter than Poll.Timeout %s, which it is spent inside", o.KillAfter, timeout)
	case o.Kill != nil && o.KillAfter == 0:
		o.KillAfter = timeout / 2
	}
	name := o.Name
	if name == "" {
		name = "the service"
	}
	return &Supervised{
		o: o, name: name, readStat: procfs.ReadStat, exe: procfs.Exe, self: os.Getpid,
		detach: DetachProcess,
	}, nil
}

// fail wraps err with the step and the service's name.
func (s *Supervised) fail(step string, err error) error {
	return fmt.Errorf("selfupdate: service: %s %s: %w", step, s.name, err)
}

// state calls the program's State, and sets Instance for a running
// process. An error that is not ErrNoSupervisor is returned wrapped; with
// ErrNoSupervisor the state is returned too.
func (s *Supervised) state(ctx context.Context) (SupervisedState, error) {
	st, err := s.o.State(ctx)
	if err != nil {
		return st, s.fail("observe", err)
	}
	st.Instance = ""
	if st.Up && st.PID > 0 {
		inst, ok, err := s.alive(st.PID)
		if err != nil {
			return st, s.fail("observe", err)
		}
		if ok {
			st.Instance = inst.String()
		}
	}
	return st, nil
}

// alive reads pid's start time, and reports whether it runs: it exists and
// is not a zombie. A process that does not exist is not an error.
func (s *Supervised) alive(pid int) (instance, bool, error) {
	ps, err := s.readStat(pid)
	if errors.Is(err, fs.ErrNotExist) {
		return instance{}, false, nil
	}
	if err != nil {
		return instance{}, false, err
	}
	return instance{pid: pid, start: ps.StartTime}, !ps.Zombie(), nil
}

// still reports whether in still runs: its PID exists with the same start
// time, and is not a zombie.
func (s *Supervised) still(in instance) (bool, error) {
	if in.pid <= 0 {
		return false, nil
	}
	now, ok, err := s.alive(in.pid)
	if err != nil || !ok {
		return false, err
	}
	return now.start == in.start, nil
}

// Status observes the service once, with Instance set for a running
// process (D11-A).
func (s *Supervised) Status(ctx context.Context) (SupervisedState, error) {
	return s.state(ctx)
}

// Installed implements selfupdate.Lifecycle: the program's
// SupervisedState.Installed, which a service with no supervisor still
// reports.
func (s *Supervised) Installed(ctx context.Context, _ string) (bool, error) {
	st, err := s.state(ctx)
	if err != nil && !errors.Is(err, ErrNoSupervisor) {
		return false, err
	}
	return st.Installed, nil
}

// Enabled implements selfupdate.EnabledLifecycle: the service starts when
// its supervisor starts (SupervisedState.NormallyUp).
func (s *Supervised) Enabled(ctx context.Context, _ string) (bool, error) {
	st, err := s.state(ctx)
	if err != nil && !errors.Is(err, ErrNoSupervisor) {
		return false, err
	}
	return st.NormallyUp, nil
}

// Running implements selfupdate.Lifecycle: the service's process is up.
// With no supervisor it is not, unless the instance a Stop recorded still
// runs (D10-A).
func (s *Supervised) Running(ctx context.Context, _ string) (bool, error) {
	st, err := s.state(ctx)
	if errors.Is(err, ErrNoSupervisor) {
		s.mu.Lock()
		rec := s.recorded
		s.mu.Unlock()
		return s.still(rec)
	}
	if err != nil {
		return false, err
	}
	return st.Up, nil
}

// Inside implements Detacher: this process is the service's process, or
// descends from it, by the parent links in /proc. The service's process
// counts only with the start time it had when it was observed, so a reused
// PID does not. An unreadable step is an error, never "outside" (D7-A).
func (s *Supervised) Inside(ctx context.Context) (bool, error) {
	st, err := s.o.State(ctx)
	if err != nil && !errors.Is(err, ErrNoSupervisor) {
		return false, s.fail("observe", err)
	}
	if !st.Up || st.PID <= 0 {
		return false, nil
	}
	target, ok, err := s.alive(st.PID)
	if err != nil {
		return false, s.fail("read the process of", err)
	}
	if !ok {
		return false, nil
	}
	cur := s.self()
	for range maxAncestry {
		if cur == target.pid {
			now, ok, err := s.alive(cur)
			if err != nil {
				return false, s.fail("read the process of", err)
			}
			return ok && now.start == target.start, nil
		}
		if cur <= 1 {
			return false, nil
		}
		ps, err := s.readStat(cur)
		if err != nil {
			return false, s.fail(fmt.Sprintf("read the parent of process %d for", cur), err)
		}
		cur = ps.Parent
	}
	return false, s.fail("walk the process tree for", fmt.Errorf("the ancestry of process %d is longer than %d", s.self(), maxAncestry))
}
