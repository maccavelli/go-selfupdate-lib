package launchd

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// throttleInterval is launchd's default minimum time between a job's
// starts (launchd.plist(5) ThrottleInterval), and the least settle window.
const throttleInterval = 10 * time.Second

// Installed reports whether the job's property list exists.
func (j *Job) Installed(context.Context, string) (bool, error) {
	if _, err := statFile(j.o.Plist); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Running reports whether the job has a running process.
func (j *Job) Running(ctx context.Context, _ string) (bool, error) {
	s, err := j.probe(ctx)
	return s.running, err
}

// Loaded reports whether launchd knows the job in its domain, read by
// `launchctl list` when that sees the domain, otherwise by `launchctl
// print` (0011-MADR §7). A job can be loaded and not running, and its plist
// can exist while it is not loaded. Exit 113 is false and no error; any
// other failure is an error, matched as Running's are (0031-MADR D4).
func (j *Job) Loaded(ctx context.Context, _ string) (bool, error) {
	s, err := j.probe(ctx)
	return s.loaded, err
}

// Status is a launchd job's state at one moment (0031-MADR D4).
type Status struct {
	// Installed is Installed's answer: the plist exists.
	Installed bool `json:"installed"`
	// Loaded is Loaded's answer: launchd knows the job in its domain.
	Loaded bool `json:"loaded"`
	// Enabled is Enabled's answer; false when the plist does not exist.
	Enabled bool `json:"enabled"`
	// Running is Running's answer: the job has a running process.
	Running bool `json:"running"`
	// PID is that process's ID, or 0 when the job is not running.
	PID int `json:"pid"`
}

// Status reads every state of the job with one launchd probe: Installed,
// then Loaded, Running and the PID, which it reads whether or not the
// plist exists, since a loaded job can outlive its plist, then Enabled,
// only when the plist exists. Any error returns a zero Status with it, as
// the per-state methods return their errors.
func (j *Job) Status(ctx context.Context, product string) (Status, error) {
	installed, err := j.Installed(ctx, product)
	if err != nil {
		return Status{}, err
	}
	s, err := j.probe(ctx)
	if err != nil {
		return Status{}, err
	}
	st := Status{Installed: installed, Loaded: s.loaded, Running: s.running}
	if s.running {
		st.PID = s.pid
	}
	if installed {
		if st.Enabled, err = j.Enabled(ctx, product); err != nil {
			return Status{}, err
		}
	}
	return st, nil
}

// Enabled implements selfupdate.EnabledLifecycle: the plist sets RunAtLoad
// to true, or KeepAlive to true or a dictionary, and the label is not
// disabled in launchd's override database (0011-MADR §7). launchd runs a
// job at load for a boolean true only; an integer, even 1, is not
// (0015-MADR D7). A plist plutil cannot read as a dictionary is an error,
// not "not enabled".
func (j *Job) Enabled(ctx context.Context, _ string) (bool, error) {
	typ, raw, _, err := j.plistValue(ctx, "RunAtLoad")
	if err != nil {
		return false, err
	}
	atLoad := isTrue(typ, raw)
	typ, raw, _, err = j.plistValue(ctx, "KeepAlive")
	if err != nil {
		return false, err
	}
	keepAlive := isTrue(typ, raw) || typ == typeDictionary
	if !atLoad && !keepAlive {
		return false, nil
	}
	disabled, err := j.disabled(ctx)
	return !disabled, err
}

// disabled reads the label's override from `launchctl print-disabled`: a
// line `"label" => disabled` (or `=> true` on older releases).
func (j *Job) disabled(ctx context.Context) (bool, error) {
	out, err := j.launchctlRun(ctx, "print-disabled", j.o.Domain.String())
	if err != nil {
		return false, err
	}
	if out.ExitCode != 0 {
		return false, launchctlError("print-disabled", j.o.Domain.String(), out)
	}
	want := `"` + j.o.Label + `" =>`
	sc := bufio.NewScanner(bytes.NewReader(out.Stdout))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if v, ok := strings.CutPrefix(line, want); ok {
			v = strings.TrimSpace(v)
			return v == "disabled" || v == "true", nil
		}
	}
	return false, nil
}

// Stop boots the job out, the only stop that holds against KeepAlive, and
// waits until launchd has let it go and its process has exited: the
// bootout-wait 0011-MADR §7 records. Once bootout is issued, the wait runs
// whatever ctx does, for the longer of Options.Poll's timeout (60 s by
// default) and the plist's ExitTimeOut (5 s by default) plus 30 s, and
// ctx's error is returned after it (0015-MADR B3). It refuses with
// service.ErrInsideService when this process would die with the job.
//
// A job that is not loaded is already stopped, and Stop returns nil.
//
// Errors, matched with errors.Is: service.ErrInsideService when this
// process would stop with the job; service.ErrPermission;
// service.ErrTimeout when the job outlives the stop's bound; and
// service.ErrUnsupported where the backend cannot run. errors.As finds
// the failed command as a *service.CommandError. When ctx ends, its
// error is returned.
func (j *Job) Stop(ctx context.Context, _ string) error {
	inside, err := j.Inside(ctx)
	if err != nil {
		return err
	}
	if inside {
		return fmt.Errorf("%w: %s", service.ErrInsideService, j.target())
	}
	s, err := j.probe(ctx)
	if err != nil {
		return err
	}
	j.mu.Lock()
	j.previous = s.pid
	j.mu.Unlock()
	if !s.loaded {
		return nil
	}
	bound := j.stopBound(ctx)
	out, err := j.launchctlRun(ctx, "bootout", j.target())
	if err != nil {
		return err
	}
	switch out.ExitCode {
	case 0, exitNotFound, exitNoProcess, exitInProgress:
	default:
		return launchctlError("bootout", j.target(), out)
	}
	return j.waitGone(ctx, s.pid, bound)
}

// stopGrace and defaultExitTimeOut make the stop wait's bound. launchd
// kills a job ExitTimeOut seconds after SIGTERM, 5 when the plist does not
// set it (0011-MADR's probe evidence), and the wait allows stopGrace more
// (0011-MADR §7). Tests shorten them.
var (
	stopGrace          = 30 * time.Second
	defaultExitTimeOut = 5 * time.Second
)

// stopBound is how long Stop waits for the job to go once bootout is
// issued: the longer of Options.Poll's timeout and launchd's own kill
// bound, ExitTimeOut plus stopGrace (0015-MADR B3). An ExitTimeOut of 0,
// which launchd reads as no kill, or one that is not an integer or cannot
// be read, leaves the poll timeout.
func (j *Job) stopBound(ctx context.Context) time.Duration {
	bound := j.o.Poll.Timeout
	if bound <= 0 {
		bound = service.DefaultPollTimeout
	}
	typ, raw, present, err := j.plistValue(ctx, "ExitTimeOut")
	if err != nil {
		return bound
	}
	exit := defaultExitTimeOut
	if present {
		n, perr := strconv.Atoi(raw)
		if typ != typeInteger || perr != nil || n <= 0 {
			return bound
		}
		exit = time.Duration(n) * time.Second
	}
	return max(bound, exit+stopGrace)
}

// waitGone polls until `print` exits 113 and pid has exited, for at most
// bound. Once bootout is issued the job goes whatever the caller does, so
// the wait does not end with the caller's context; its error is returned
// after (0015-MADR B3).
func (j *Job) waitGone(ctx context.Context, pid int, bound time.Duration) error {
	probe := func(ctx context.Context) (service.Health, error) {
		out, err := j.launchctlRun(ctx, "print", j.target())
		if err != nil {
			return service.Health{}, err
		}
		gone := out.ExitCode == exitNotFound && (pid <= 0 || !pidAlive(pid))
		return service.Health{Ready: gone, Detail: fmt.Sprintf("%s print exit %d, pid %d", j.target(), out.ExitCode, pid)}, nil
	}
	// PollHealthy bounds the wait by poll.Timeout itself.
	poll := j.o.Poll
	poll.Interval, poll.Settle, poll.Previous, poll.Timeout = 50*time.Millisecond, -1, "", bound
	if err := service.PollHealthy(context.WithoutCancel(ctx), probe, poll); err != nil {
		return errors.Join(fmt.Errorf("selfupdate: launchd: bootout %s: %w", j.target(), err), ctx.Err())
	}
	return ctx.Err()
}

// Start enables the label (bootstrap of a disabled label fails with 5),
// bootstraps the plist when the job is not loaded, retrying launchd's
// transient 5 and 37 within the deadline, then kickstarts it. A process
// that Reconcile's reload started is not taken for the one before the
// update (0015-MADR D3).
//
// Errors, matched with errors.Is: service.ErrPermission,
// service.ErrNotInstalled and service.ErrUnsupported; and
// service.ErrTimeout when launchd still refuses to load the job at the
// bootstrap's bound. errors.As finds the failed command as a
// *service.CommandError. When ctx ends, its error is returned.
func (j *Job) Start(ctx context.Context, _ string) error {
	s, err := j.probe(ctx)
	if err != nil {
		return err
	}
	j.mu.Lock()
	if s.pid > 0 && !j.reloaded {
		j.previous = s.pid
	}
	j.reloaded = false
	j.mu.Unlock()
	out, err := j.launchctlRun(ctx, "enable", j.target())
	if err != nil {
		return err
	}
	if out.ExitCode != 0 {
		return launchctlError("enable", j.target(), out)
	}
	if !s.loaded {
		if err := j.bootstrap(ctx); err != nil {
			return err
		}
	}
	out, err = j.launchctlRun(ctx, "kickstart", j.target())
	if err != nil {
		return err
	}
	if out.ExitCode != 0 {
		return launchctlError("kickstart", j.target(), out)
	}
	return nil
}

// bootstrapRetries bound the retry of a bootstrap launchd refused while it
// was still letting the job go.
const (
	bootstrapFirst = 100 * time.Millisecond
	bootstrapMax   = time.Second
)

func (j *Job) bootstrap(ctx context.Context) error {
	timeout := j.o.Poll.Timeout
	if timeout <= 0 {
		timeout = service.DefaultPollTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	wait := bootstrapFirst
	for {
		out, err := j.launchctlRun(ctx, "bootstrap", j.o.Domain.String(), j.o.Plist)
		if err != nil {
			return err
		}
		switch out.ExitCode {
		case 0:
			return nil
		case exitIO, exitAlready, exitInProgress:
			// The job may have loaded after all, or launchd may still be
			// letting the old one go.
			if s, err := j.probe(ctx); err == nil && s.loaded {
				return nil
			}
		default:
			return launchctlError("bootstrap", j.o.Plist, out)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%w: bootstrap %s: %w", service.ErrTimeout, j.o.Plist, launchctlError("bootstrap", j.o.Plist, out))
		case <-time.After(wait):
		}
		wait = min(wait*2, bootstrapMax)
	}
}

// WaitHealthy waits until the job runs as a new process, the same one for
// the settle window, at least launchd's ThrottleInterval, so a job crashing
// and being relaunched does not pass; then runs Options.Probe.
//
// Errors, matched with errors.Is: service.ErrUnhealthy when the job
// failed, crashed or restarted, and service.ErrTimeout when it was not
// healthy in time. When ctx ends, its error is returned.
func (j *Job) WaitHealthy(ctx context.Context, _ string) error {
	j.mu.Lock()
	previous := j.previous
	j.mu.Unlock()
	probe := func(ctx context.Context) (service.Health, error) {
		s, err := j.probe(ctx)
		if err != nil {
			return service.Health{}, err
		}
		if !s.loaded {
			return service.Health{Failed: true, Detail: j.target() + " is not loaded"}, nil
		}
		h := service.Health{Ready: s.running, Detail: fmt.Sprintf("%s pid %d running %t", j.target(), s.pid, s.running)}
		if s.pid > 0 {
			h.Instance = strconv.Itoa(s.pid)
		}
		return h, nil
	}
	poll := j.o.Poll
	if poll.Settle >= 0 && poll.Settle < throttleInterval {
		poll.Settle = throttleInterval
	}
	if previous > 0 {
		poll.Previous = strconv.Itoa(previous)
	}
	if err := service.PollHealthy(ctx, probe, poll); err != nil {
		return err
	}
	if j.o.Probe == nil {
		return nil
	}
	app := j.o.Poll
	app.Settle = -1
	return service.PollHealthy(ctx, j.o.Probe, app)
}
