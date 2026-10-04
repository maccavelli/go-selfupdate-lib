package scm

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeSCM answers for the SCM and the process table from memory, and
// records every call (0011-PLAN V4 step 8).
type fakeSCM struct {
	mu       sync.Mutex
	calls    []string
	services map[string]*fakeSvc
	procs    []process
	// born are creation times; a missing PID reads as 0, unknown.
	born    map[uint32]uint64
	openErr map[string]error
}

// fakeSvc is one service. A control or a start moves it to the head of
// its queue at once; each virtual sleep then moves it one entry on
// (fakeSCM.tick).
type fakeSvc struct {
	st         status
	queue      []status
	cfg        config
	deps       []string
	controlErr []error // per control call, in turn; then nil
	startErr   error
	// onStop and onStart queue what the service does after the control or
	// the start; nil means a short pending phase, then STOPPED or RUNNING.
	onStop, onStart func(f *fakeSvc)
	paths           []string // setBinaryPathName calls
}

func newFake() *fakeSCM {
	return &fakeSCM{services: map[string]*fakeSvc{}, born: map[uint32]uint64{}, openErr: map[string]error{}}
}

// add registers a running own-process service.
func (f *fakeSCM) add(name string, pid uint32) *fakeSvc {
	s := &fakeSvc{
		st:  status{Type: 0x10, State: stateRunning, ProcessID: pid},
		cfg: config{Type: 0x10, StartType: startAuto, BinaryPathName: `"C:\Program Files\Demo\demo.exe" run`},
	}
	f.services[name] = s
	return s
}

func (f *fakeSCM) record(format string, args ...any) {
	f.calls = append(f.calls, fmt.Sprintf(format, args...))
}

func (f *fakeSCM) open(name string, access uint32) (handle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("open %s %#x", name, access)
	if err := f.openErr[name]; err != nil {
		return nil, err
	}
	s, ok := f.services[name]
	if !ok {
		return nil, errDoesNotExist
	}
	return &fakeHandle{f: f, name: name, s: s}, nil
}

func (f *fakeSCM) processes() ([]process, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("processes")
	return append([]process(nil), f.procs...), nil
}

func (f *fakeSCM) created(pid uint32) uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.born[pid]
}

// decompose and compose are simple stand-ins for CommandLineToArgv and
// ComposeCommandLine: double quotes group, nothing escapes. The real pair
// is tested on Windows (compose_windows_test.go).
func (*fakeSCM) decompose(line string) ([]string, error) {
	var args []string
	for line = strings.TrimSpace(line); line != ""; line = strings.TrimSpace(line) {
		if rest, ok := strings.CutPrefix(line, `"`); ok {
			arg, after, found := strings.Cut(rest, `"`)
			if !found {
				return nil, fmt.Errorf("unterminated quote in %q", line)
			}
			args, line = append(args, arg), after
			continue
		}
		arg, after, _ := strings.Cut(line, " ")
		args, line = append(args, arg), after
	}
	return args, nil
}

func (*fakeSCM) compose(args []string) string {
	q := make([]string, len(args))
	for i, a := range args {
		q[i] = a
		if a == "" || strings.ContainsAny(a, " \t") {
			q[i] = `"` + a + `"`
		}
	}
	return strings.Join(q, " ")
}

// sscanOpen parses an "open <name> <access>" call.
func sscanOpen(call string, name *string, access *uint32) (int, error) {
	return fmt.Sscanf(call, "open %s %v", name, access)
}

func (f *fakeSCM) verbs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var v []string
	for _, c := range f.calls {
		v = append(v, strings.Fields(c)[0])
	}
	return v
}

type fakeHandle struct {
	f    *fakeSCM
	name string
	s    *fakeSvc
}

func (h *fakeHandle) status() (status, error) {
	h.f.mu.Lock()
	defer h.f.mu.Unlock()
	return h.s.st, nil
}

// next moves s to its queue's head, if any.
func (s *fakeSvc) next() {
	if len(s.queue) > 0 {
		s.st, s.queue = s.queue[0], s.queue[1:]
	}
}

// tick is time passing: every service moves one entry on.
func (f *fakeSCM) tick() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.services {
		s.next()
	}
}

func (h *fakeHandle) control(code uint32) (status, error) {
	h.f.mu.Lock()
	defer h.f.mu.Unlock()
	h.f.record("control %s %d", h.name, code)
	if len(h.s.controlErr) > 0 {
		err := h.s.controlErr[0]
		h.s.controlErr = h.s.controlErr[1:]
		if err != nil {
			return h.s.st, err
		}
	}
	if h.s.onStop != nil {
		h.s.onStop(h.s)
	} else {
		pid := h.s.st.ProcessID
		h.s.queue = append(h.s.queue,
			status{Type: 0x10, State: stateStopPending, CheckPoint: 1, WaitHint: 3000, ProcessID: pid},
			status{Type: 0x10, State: stateStopped})
	}
	h.s.next()
	return h.s.st, nil
}

func (h *fakeHandle) start() error {
	h.f.mu.Lock()
	defer h.f.mu.Unlock()
	h.f.record("start %s", h.name)
	if h.s.startErr != nil {
		// Someone else's start, which the error reports, still moves it.
		h.s.next()
		return h.s.startErr
	}
	if h.s.onStart != nil {
		h.s.onStart(h.s)
	} else {
		h.s.queue = append(h.s.queue,
			status{Type: 0x10, State: stateStartPending, CheckPoint: 1, WaitHint: 3000, ProcessID: 5000},
			status{Type: 0x10, State: stateRunning, ProcessID: 5000})
	}
	h.s.next()
	return nil
}

func (h *fakeHandle) config() (config, error) {
	h.f.mu.Lock()
	defer h.f.mu.Unlock()
	return h.s.cfg, nil
}

func (h *fakeHandle) setBinaryPathName(path string) error {
	h.f.mu.Lock()
	defer h.f.mu.Unlock()
	h.f.record("setpath %s", h.name)
	h.s.paths = append(h.s.paths, path)
	h.s.cfg.BinaryPathName = path
	return nil
}

func (h *fakeHandle) dependents() ([]string, error) {
	h.f.mu.Lock()
	defer h.f.mu.Unlock()
	var running []string
	for _, d := range h.s.deps {
		if s, ok := h.f.services[d]; ok && s.st.State != stateStopped {
			running = append(running, d)
		}
	}
	return running, nil
}

func (*fakeHandle) close() error { return nil }

// virtualClock is the Service's now and sleep: a sleep advances it at
// once, and runs onSleep.
type virtualClock struct {
	mu      sync.Mutex
	t       time.Time
	onSleep func()
}

func (c *virtualClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *virtualClock) sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
	if c.onSleep != nil {
		c.onSleep()
	}
	return nil
}

// selfPID is the PID the fake process table gives this process.
const selfPID = 7000

// testService is a Service named "demo" over f, on a virtual clock, with
// this process at selfPID and no settle window.
func testService(t *testing.T, f *fakeSCM, o Options) (*Service, *virtualClock) {
	t.Helper()
	if o.Name == "" {
		o.Name = "demo"
	}
	if o.Poll.Settle == 0 {
		o.Poll.Settle = -1
	}
	if o.Poll.Interval == 0 {
		o.Poll.Interval = time.Millisecond
	}
	if o.Poll.Timeout == 0 {
		o.Poll.Timeout = 2 * time.Second
	}
	s, err := newService(o, "windows", f)
	if err != nil {
		t.Fatal(err)
	}
	clock := &virtualClock{t: time.Unix(1_700_000_000, 0), onSleep: f.tick}
	s.now, s.sleep = clock.now, clock.sleep
	s.self = func() uint32 { return selfPID }
	if len(f.procs) == 0 {
		f.procs = []process{{PID: selfPID, ParentPID: 6000}, {PID: 6000, ParentPID: 4}, {PID: 4}}
	}
	return s, clock
}
