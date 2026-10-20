// Package testproc is test support: it builds a Go fixture program without
// reading the git state around the build, and starts a helper process and
// waits for it to be ready without a fixed time budget
// (docs/decisions/0023-MADR-robust-test-fixtures-and-process-waits.md).
// Only _test.go files import it.
//
// A fixture built in a temporary directory with Go's default -buildvcs=auto
// fails when a .git above that directory cannot be read, though no fixture
// uses VCS information. A helper's first start can take seconds on a
// loaded machine, which a fixed budget turns into a failure, while a helper
// that crashes should be reported at once, with its exit status.
package testproc

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// GoBuild builds the main package in dir to out, with
// "go build -buildvcs=false args… -o out .", env appended to this
// process's environment. It fails t, with the go command's output, when
// the build fails.
func GoBuild(t testing.TB, dir, out string, env []string, args ...string) {
	t.Helper()
	if err := Build(dir, out, env, args...); err != nil {
		t.Fatal(err)
	}
}

// Build is GoBuild for a caller with no testing.TB, such as TestMain or a
// fixture built once: it returns the error, with the go command's output.
func Build(dir, out string, env []string, args ...string) error {
	goBin, err := exec.LookPath("go")
	if err != nil {
		return fmt.Errorf("the go command is required to build a fixture: %w", err)
	}
	full := append([]string{"build", "-buildvcs=false"}, args...)
	full = append(full, "-o", out, ".")
	cmd := exec.Command(goBin, full...) //nolint:gosec // the go command, building a test fixture
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	if b, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go %s in %s: %w\n%s", strings.Join(full, " "), dir, err, b)
	}
	return nil
}

// Proc is a helper process Start started.
type Proc struct {
	cmd  *exec.Cmd
	out  *tail
	done chan struct{}
	err  error
}

// Start starts cmd and waits until the file ready exists. It fails t at
// once when the process exits first, with its exit status and the last
// lines of its output, and when Budget(t) runs out, after it kills the
// helper and waits for it. cmd's standard output
// and error, when unset, are kept for that message. The caller ends the
// process its own way, and waits for it with Proc.Wait, never cmd.Wait.
func Start(t testing.TB, cmd *exec.Cmd, ready string) *Proc {
	t.Helper()
	p, err := startWithin(cmd, ready, Budget(t))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// tailLines is how many of a helper's last output lines a failure quotes.
const tailLines = 20

// startWithin is Start with an explicit budget, which the package's tests use.
func startWithin(cmd *exec.Cmd, ready string, budget time.Duration) (*Proc, error) {
	p := &Proc{cmd: cmd, out: &tail{max: 64 << 10}, done: make(chan struct{})}
	if cmd.Stdout == nil {
		cmd.Stdout = p.out
	}
	if cmd.Stderr == nil {
		cmd.Stderr = p.out
	}
	name := filepath.Base(cmd.Path)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", name, err)
	}
	go func() {
		p.err = cmd.Wait()
		close(p.done)
	}()
	began := time.Now()
	timer := time.NewTimer(budget)
	defer timer.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		if _, err := os.Stat(ready); err == nil {
			return p, nil
		}
		select {
		case <-p.done:
			if _, err := os.Stat(ready); err == nil {
				return p, nil
			}
			return nil, fmt.Errorf("%s exited (%s) before it was ready, after %s; its output ended:\n%s",
				name, status(p.err), time.Since(began).Round(time.Millisecond), p.out.last(tailLines))
		case <-timer.C:
			_ = cmd.Process.Kill() //nolint:errcheck // ending a helper the test gives up on
			<-p.done
			return nil, fmt.Errorf("%s not ready after %s; its output ended:\n%s",
				name, time.Since(began).Round(time.Millisecond), p.out.last(tailLines))
		case <-tick.C:
		}
	}
}

// Wait waits for the process to exit and returns cmd.Wait's error. Every
// call returns the same.
func (p *Proc) Wait() error {
	<-p.done
	return p.err
}

// status describes how a process ended.
func status(err error) string {
	if err == nil {
		return "exit status 0"
	}
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		return exit.Error()
	}
	return err.Error()
}

// The budget's limits.
const (
	// noDeadline is the budget for a test run without a deadline
	// (go test -timeout 0).
	noDeadline = 5 * time.Minute
	// minGrace is the least time left to the test after a budget, to
	// report and clean up.
	minGrace = time.Second
)

// Budget is how long a test may wait on a helper: until the test's
// deadline, less a grace period of 5 % of the time left and at least 1 s,
// as Go's internal/testenv bounds a test's subprocesses; without a
// deadline, 5 minutes. A helper that crashes is reported at once whatever
// the budget, by Start.
func Budget(t testing.TB) time.Duration {
	if d, ok := t.(interface{ Deadline() (time.Time, bool) }); ok {
		deadline, set := d.Deadline()
		return budgetAt(deadline, set, time.Now())
	}
	return noDeadline
}

func budgetAt(deadline time.Time, set bool, now time.Time) time.Duration {
	if !set {
		return noDeadline
	}
	left := deadline.Sub(now)
	grace := max(left/20, minGrace)
	return max(left-grace, 0)
}

// tail keeps the last max bytes written to it, for any number of writers.
type tail struct {
	mu  sync.Mutex
	max int
	buf []byte
}

func (b *tail) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	if over := len(b.buf) - b.max; over > 0 {
		b.buf = b.buf[over:]
	}
	return len(p), nil
}

// last is the last n lines written, or "(no output)".
func (b *tail) last(n int) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	text := strings.TrimRight(string(b.buf), "\r\n")
	if text == "" {
		return "(no output)"
	}
	lines := strings.Split(text, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// CheckNoLeak fails the test when a goroutine that started after it was
// called still runs. It checks once the test has cleaned up, and also
// whenever the returned function is called. It compares goroutine IDs, not
// a count: a goroutine that ran before the call and ends during the test
// would bring a count back down and hide a leak (0025-MADR Amendment A6).
// It polls for up to 2 s, because a goroutine that is ending is still
// listed for a moment (0004-MADR H6). The sleep only paces that poll; it
// orders nothing. Every package's tests share it
// (docs/decisions/0025-MADR-align-with-go-sdk-design-practices.md D4).
//
// Call it first. A test whose cleanup would release a leaked goroutine,
// such as closing the pipe a reader blocks on, checks before that cleanup:
//
//	defer testproc.CheckNoLeak(t)()
func CheckNoLeak(t testing.TB) (checkNow func()) {
	t.Helper()
	base := goroutines()
	check := func() {
		deadline := time.Now().Add(2 * time.Second)
		for {
			var started []string
			for id, g := range goroutines() {
				if _, ok := base[id]; !ok {
					started = append(started, g)
				}
			}
			if len(started) == 0 {
				return
			}
			if time.Now().After(deadline) {
				slices.Sort(started)
				t.Errorf("goroutines: %d started during the test still run:\n%s", len(started), strings.Join(started, "\n"))
				return
			}
			runtime.Gosched()
			time.Sleep(10 * time.Millisecond)
		}
	}
	t.Cleanup(check)
	return check
}

// goroutines returns the running goroutines from runtime.Stack, keyed by
// ID, each as its header line and the line naming what created it. The
// runtime never reuses an ID within a process.
func goroutines() map[string]string {
	buf := make([]byte, 64<<10)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		buf = make([]byte, 2*len(buf))
	}
	gs := make(map[string]string)
	for _, g := range strings.Split(string(buf), "\n\n") {
		head, rest, _ := strings.Cut(g, "\n")
		id, ok := strings.CutPrefix(head, "goroutine ")
		if !ok {
			continue
		}
		id, _, _ = strings.Cut(id, " ")
		desc := head
		for _, line := range strings.Split(rest, "\n") {
			if strings.HasPrefix(line, "created by ") {
				desc += " " + line
				break
			}
		}
		gs[id] = desc
	}
	return gs
}
