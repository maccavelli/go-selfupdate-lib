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

// Enabled implements selfupdate.EnabledLifecycle: the plist sets RunAtLoad
// or KeepAlive (a KeepAlive dictionary counts), and the label is not
// disabled in launchd's override database (0011-MADR §7).
func (j *Job) Enabled(ctx context.Context, _ string) (bool, error) {
	atLoad, err := j.plistBool(ctx, "RunAtLoad")
	if err != nil {
		return false, err
	}
	keepAlive, err := j.plistBool(ctx, "KeepAlive")
	if err != nil {
		return false, err
	}
	if !atLoad && !keepAlive {
		return false, nil
	}
	disabled, err := j.disabled(ctx)
	return !disabled, err
}

// plistBool reads key from the plist with `plutil -extract <key> raw`,
// which prints a boolean as true or false. A missing key is false. Any
// other value counts as true: launchd takes KeepAlive as a boolean or a
// dictionary, and raw prints a dictionary's keys, or nothing for an empty
// one. (The json format refuses a boolean on its own: "Invalid object in
// plist for JSON format".)
func (j *Job) plistBool(ctx context.Context, key string) (bool, error) {
	out, err := j.o.Runner.Run(ctx, service.Command{
		Path: j.plutil, Args: []string{"-extract", key, "raw", "-o", "-", "--", j.o.Plist}, Env: env,
	})
	if err != nil {
		return false, err
	}
	if out.ExitCode != 0 {
		// plutil exits 1 for a missing key: "No value at that key path".
		return false, nil
	}
	return strings.TrimSpace(string(out.Stdout)) != "false", nil
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
// bootout-wait magic-cli-remote measured (0011-MADR §7). Once bootout is
// issued, the wait runs to stopBound whatever ctx does, and ctx's error is
// returned after it (0015-MADR B3). It refuses with
// service.ErrInsideService when this process would die with the job.
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
		if typ != "integer" || perr != nil || n <= 0 {
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
// transient 5 and 37 within the deadline, then kickstarts it.
func (j *Job) Start(ctx context.Context, _ string) error {
	s, err := j.probe(ctx)
	if err != nil {
		return err
	}
	j.mu.Lock()
	if s.pid > 0 {
		j.previous = s.pid
	}
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
