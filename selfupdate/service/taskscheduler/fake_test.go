package taskscheduler

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/internal/winproc"
)

// fakeSched answers for Task Scheduler and the process table from memory,
// and records every call that changes something.
type fakeSched struct {
	mu      sync.Mutex
	calls   []string
	tasks   map[string]*fakeTask // by path
	procs   []winproc.Process
	born    map[uint32]uint64 // creation times; a missing PID reads as 0
	alive   map[uint32]bool
	openErr error
	nextPID uint32
	now     func() time.Time
	// folders are the task folders that exist; `\` always does.
	folders map[string]bool
	// fail makes a call that changes something return its error, by verb:
	// register, delete, mkfolder or list.
	fail map[string]error
	// onRegister shapes each task register creates, before it is stored.
	onRegister func(ft *fakeTask)
}

// fakeTask is one registered task.
type fakeTask struct {
	st    int32
	on    bool
	pids  []uint32
	trig  []trigger
	acts  []action
	logon int32
	// user and runLevel are the principal's, beside logon.
	user     string
	runLevel int32
	last     lastRun
	def      string
	// exitIn, when positive, ends the running instance after that many
	// virtual sleeps, as a program that was asked to exit does.
	exitIn int
	// stopKeeps makes a stop take effect only after a few virtual sleeps.
	stopKeeps bool
	// staleList keeps an ended instance listed, its process gone, as Task
	// Scheduler can (0021-MADR Amendment A4).
	staleList bool
	stopErr   error
	runErr    error
	// runExits makes a run end at once with this result.
	runExits *int32
	// readErr makes a read fail, by method: state, enabled,
	// instances, triggers or lastRun.
	readErr map[string]error
}

const (
	demoExe = `C:\Program Files\Demo\demo.exe`
	demoXML = `<Task><Actions><Exec><Command>C:\Program Files\Demo\demo.exe</Command></Exec></Actions></Task>`
)

func newFake() *fakeSched {
	return &fakeSched{
		tasks: map[string]*fakeTask{}, born: map[uint32]uint64{}, alive: map[uint32]bool{}, nextPID: 9000,
		now:     func() time.Time { return time.Unix(1_700_000_000, 0) },
		folders: map[string]bool{`\`: true}, fail: map[string]error{},
	}
}

// taskPath is a task's full path from its folder and name.
func taskPath(folder, name string) string {
	if folder == `\` {
		return `\` + name
	}
	return folder + `\` + name
}

// register creates or updates a task as RegisterTask does, by flags
// (0032-MADR Amendment A1, P1-4): TASK_CREATE alone on an existing task
// is hrAlreadyExists, TASK_UPDATE alone on a missing one hrFileNotFound,
// and a missing folder hrFileNotFound.
func (f *fakeSched) register(folder, name, xml string, flags, logon int32) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("register %s flags=%d logon=%d", taskPath(folder, name), flags, logon)
	if err := f.fail["register"]; err != nil {
		return err
	}
	if !f.folders[folder] {
		return hrFileNotFound
	}
	path := taskPath(folder, name)
	ft, exists := f.tasks[path]
	switch {
	case exists && flags&taskCreateOrUpdate == taskCreate:
		return hrAlreadyExists
	case !exists && flags&taskCreate == 0:
		return hrFileNotFound
	case !exists:
		ft = &fakeTask{st: stateReady, on: true, user: "user", last: lastRun{Result: 0x41303}} // SCHED_S_TASK_HAS_NOT_RUN
		if f.onRegister != nil {
			f.onRegister(ft)
		}
		f.tasks[path] = ft
	}
	ft.logon, ft.def = logon, xml
	return nil
}

func (f *fakeSched) deleteTask(folder, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	path := taskPath(folder, name)
	f.record("delete %s", path)
	if err := f.fail["delete"]; err != nil {
		return err
	}
	if _, ok := f.tasks[path]; !ok {
		return hrFileNotFound
	}
	delete(f.tasks, path)
	return nil
}

func (f *fakeSched) createFolder(path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("mkfolder %s", path)
	if err := f.fail["mkfolder"]; err != nil {
		return err
	}
	if f.folders[path] {
		return hrAlreadyExists
	}
	for p := path; p != `\` && p != ""; p = p[:max(strings.LastIndex(p, `\`), 1)] {
		f.folders[p] = true
	}
	return nil
}

func (f *fakeSched) list(folder string) ([]listed, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail["list"]; err != nil {
		return nil, err
	}
	if !f.folders[folder] {
		return nil, hrFileNotFound
	}
	var out []listed
	for path, ft := range f.tasks {
		dir, name := path[:max(strings.LastIndex(path, `\`), 1)], path[strings.LastIndex(path, `\`)+1:]
		if dir == folder {
			out = append(out, listed{Name: name, State: ft.st, LastResult: ft.last.Result, LastRun: ft.last.Time})
		}
	}
	return out, nil
}

// add registers an enabled task with a logon trigger and one Exec action,
// running as pid, or not running when pid is 0.
func (f *fakeSched) add(path string, pid uint32) *fakeTask {
	ft := &fakeTask{
		st:    stateReady,
		on:    true,
		trig:  []trigger{{Type: triggerLogon, Enabled: true}},
		acts:  []action{{Type: actionExec, Path: demoExe, Args: "serve"}},
		logon: logonInteractiveToken,
		def:   demoXML,
	}
	if pid != 0 {
		ft.st, ft.pids = stateRunning, []uint32{pid}
		f.alive[pid], f.born[pid] = true, uint64(pid)*10
	}
	f.tasks[path] = ft
	f.folders[path[:max(strings.LastIndex(path, `\`), 1)]] = true
	return ft
}

func (f *fakeSched) record(format string, args ...any) {
	f.calls = append(f.calls, fmt.Sprintf(format, args...))
}

// verbs are the recorded calls.
func (f *fakeSched) verbs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *fakeSched) open(folder, name string) (registered, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.openErr != nil {
		return nil, f.openErr
	}
	path := `\` + name
	if folder != `\` {
		path = folder + `\` + name
	}
	ft, ok := f.tasks[path]
	if !ok {
		return nil, hrFileNotFound
	}
	return &fakeReg{f: f, t: ft}, nil
}

func (f *fakeSched) processes() ([]winproc.Process, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]winproc.Process(nil), f.procs...), nil
}

func (f *fakeSched) created(pid uint32) uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.born[pid]
}

func (f *fakeSched) running(pid uint32, born uint64) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.alive[pid] && (born == 0 || f.born[pid] == born)
}

// tick is time passing: a task asked to exit counts down.
func (f *fakeSched) tick() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, ft := range f.tasks {
		if ft.exitIn > 0 {
			if ft.exitIn--; ft.exitIn == 0 {
				f.end(ft)
			}
		}
	}
}

// end ends a task's instance. f.mu is held.
func (f *fakeSched) end(ft *fakeTask) {
	for _, pid := range ft.pids {
		f.alive[pid] = false
	}
	if ft.staleList {
		return
	}
	ft.pids = nil
	ft.st = stateReady
	if !ft.on {
		ft.st = stateDisabled
	}
}

type fakeReg struct {
	f *fakeSched
	t *fakeTask
}

func (r *fakeReg) lock() func() { r.f.mu.Lock(); return r.f.mu.Unlock }

func (r *fakeReg) state() (int32, error) { defer r.lock()(); return r.t.st, r.t.readErr["state"] }

func (r *fakeReg) enabled() (bool, error) { defer r.lock()(); return r.t.on, r.t.readErr["enabled"] }

func (r *fakeReg) setEnabled(on bool) error {
	defer r.lock()()
	r.f.record("enable %t", on)
	r.t.on = on
	switch {
	case !on && r.t.st != stateRunning:
		r.t.st = stateDisabled
	case on && r.t.st == stateDisabled:
		r.t.st = stateReady
	}
	return nil
}

func (r *fakeReg) run() error {
	defer r.lock()()
	r.f.record("run")
	if r.t.runErr != nil {
		return r.t.runErr
	}
	if !r.t.on {
		// Task Scheduler refuses to run a disabled task
		// (0022-PLAN-documentation-pass-after-v1-14-0.md P1).
		return hrTaskDisabled
	}
	r.t.last.Time = r.f.now()
	if r.t.runExits != nil {
		r.t.last.Result = *r.t.runExits
		return nil
	}
	pid := r.f.nextPID
	r.f.nextPID++
	r.t.pids, r.t.st = []uint32{pid}, stateRunning
	r.f.alive[pid], r.f.born[pid] = true, uint64(pid)*10
	r.t.last.Result = 0x41301 // SCHED_S_TASK_RUNNING
	return nil
}

func (r *fakeReg) stop() error {
	defer r.lock()()
	r.f.record("stop")
	if r.t.stopErr != nil {
		return r.t.stopErr
	}
	if r.t.stopKeeps {
		if r.t.exitIn == 0 {
			r.t.exitIn = 3
		}
		return nil
	}
	r.f.end(r.t)
	return nil
}

func (r *fakeReg) instances() ([]uint32, error) {
	defer r.lock()()
	return append([]uint32(nil), r.t.pids...), r.t.readErr["instances"]
}

func (r *fakeReg) triggers() ([]trigger, error) {
	defer r.lock()()
	return append([]trigger(nil), r.t.trig...), r.t.readErr["triggers"]
}

func (r *fakeReg) actions() ([]action, error) {
	defer r.lock()()
	return append([]action(nil), r.t.acts...), nil
}

func (r *fakeReg) logonType() (int32, error) { defer r.lock()(); return r.t.logon, nil }

func (r *fakeReg) principal() (principal, error) {
	defer r.lock()()
	return principal{UserID: r.t.user, LogonType: r.t.logon, RunLevel: r.t.runLevel}, nil
}

func (r *fakeReg) lastRun() (lastRun, error) {
	defer r.lock()()
	return r.t.last, r.t.readErr["lastRun"]
}

func (r *fakeReg) xml() (string, error) { defer r.lock()(); return r.t.def, nil }

// setPath changes the one Exec action's path and nothing else, as the
// real registration was measured to (Amendment A1, probe 4).
func (r *fakeReg) setPath(path string) error {
	defer r.lock()()
	r.f.record("setpath %s", path)
	r.t.def = strings.Replace(r.t.def, r.t.acts[0].Path, path, 1)
	r.t.acts[0].Path = path
	return nil
}

func (r *fakeReg) restore(xml string) error {
	defer r.lock()()
	r.f.record("restore")
	r.t.def = xml
	return nil
}

func (*fakeReg) close() error { return nil }

// virtualClock is the Task's now and sleep: a sleep advances it at once,
// and runs onSleep.
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

// selfPID is the PID the fake process table gives this process; 6000 is
// its parent and 4 its grandparent.
const selfPID = 7000

// testTask is a Task for the task `\demo` over f, on a virtual clock, with
// this process at selfPID and no settle window.
func testTask(t *testing.T, f *fakeSched, o Options) (*Task, *virtualClock) {
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
	task, err := newTask(o, "windows", f)
	if err != nil {
		t.Fatal(err)
	}
	clock := &virtualClock{t: time.Unix(1_700_000_000, 0), onSleep: f.tick}
	task.now, task.sleep = clock.now, clock.sleep
	f.now = clock.now
	task.self = func() uint32 { return selfPID }
	if len(f.procs) == 0 {
		f.procs = []winproc.Process{{PID: selfPID, ParentPID: 6000}, {PID: 6000, ParentPID: 4}, {PID: 4}}
	}
	return task, clock
}
