package scm

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// Tests for docs/decisions/0011-PLAN-reference-service-lifecycles.md V4
// steps 1 to 7, over the fake SCM.

func TestNewRefuses(t *testing.T) {
	if _, err := newService(Options{Name: "demo"}, "linux", newFake()); !errors.Is(err, service.ErrUnsupported) {
		t.Fatalf("off Windows: %v", err)
	}
	for _, name := range []string{`a\b`, "a/b", strings.Repeat("x", 257)} {
		if _, err := newService(Options{Name: name}, "windows", newFake()); err == nil {
			t.Errorf("name %q accepted", name)
		}
	}
	// 0015-MADR D10.
	if _, err := newService(Options{Poll: service.PollOptions{Timeout: 5 * time.Second}}, "windows", newFake()); err == nil ||
		!strings.Contains(err.Error(), "Poll.Settle") {
		t.Errorf("a settle as long as the timeout: %v", err)
	}
	s, _ := testService(t, newFake(), Options{Name: ""})
	s.o.Name = ""
	if _, err := s.Installed(context.Background(), "a/b"); err == nil {
		t.Fatal("an invalid product name was used as the service name")
	}
}

func TestProbes(t *testing.T) {
	ctx := context.Background()
	f := newFake()
	svc := f.add("demo", 4242)
	s, _ := testService(t, f, Options{})
	if ok, err := s.Installed(ctx, "demo"); err != nil || !ok {
		t.Fatalf("Installed %t, %v", ok, err)
	}
	for state, want := range map[uint32]bool{
		stateStopped: false, stateStartPending: true, stateStopPending: true, stateRunning: true,
		stateContinuePending: true, statePausePending: true, statePaused: true,
	} {
		svc.st.State = state
		if got, err := s.Running(ctx, "demo"); err != nil || got != want {
			t.Errorf("Running in %s: %t, %v", stateName(state), got, err)
		}
	}
	delete(f.services, "demo")
	if ok, err := s.Installed(ctx, "demo"); err != nil || ok {
		t.Fatalf("a missing service: Installed %t, %v", ok, err)
	}
	if ok, err := s.Running(ctx, "demo"); err != nil || ok {
		t.Fatalf("a missing service: Running %t, %v", ok, err)
	}
	f.openErr["demo"] = errAccessDenied
	if _, err := s.Installed(ctx, "demo"); !errors.Is(err, service.ErrPermission) {
		t.Fatalf("access denied: %v", err)
	}
}

func TestDriverRefused(t *testing.T) {
	f := newFake()
	f.add("demo", 1).st.Type = 1 // SERVICE_KERNEL_DRIVER
	s, _ := testService(t, f, Options{})
	if _, err := s.Running(context.Background(), "demo"); !errors.Is(err, service.ErrUnsupported) {
		t.Fatalf("a driver: %v", err)
	}
}

func TestEnabled(t *testing.T) {
	for _, c := range []struct {
		name          string
		start         uint32
		delayed       bool
		triggers      uint32
		triggerOption bool
		want          bool
	}{
		{"auto", startAuto, false, 0, false, true},
		{"delayed auto", startAuto, true, 0, false, true},
		{"demand", startDemand, false, 0, false, false},
		{"trigger start", startDemand, false, 2, false, false},
		{"trigger start, counted", startDemand, false, 2, true, true},
		{"disabled", 4, false, 0, false, false},
	} {
		f := newFake()
		svc := f.add("demo", 1)
		svc.cfg.StartType, svc.cfg.DelayedAutoStart, svc.cfg.Triggers = c.start, c.delayed, c.triggers
		s, _ := testService(t, f, Options{TriggerStartEnabled: c.triggerOption})
		if got, err := s.Enabled(context.Background(), "demo"); err != nil || got != c.want {
			t.Errorf("%s: enabled %t, %v; want %t", c.name, got, err, c.want)
		}
	}
}

// TestOpenRights: each call opens the service with only the rights it
// needs (0011-MADR §7).
func TestOpenRights(t *testing.T) {
	ctx := context.Background()
	for _, c := range []struct {
		name string
		call func(s *Service) error
		want []uint32
	}{
		{"Installed", func(s *Service) error { _, err := s.Installed(ctx, "demo"); return err }, []uint32{accessQueryStatus}},
		{"Enabled", func(s *Service) error { _, err := s.Enabled(ctx, "demo"); return err }, []uint32{accessQueryConfig | accessQueryStatus}},
		{"Start", func(s *Service) error { return s.Start(ctx, "demo") }, []uint32{accessStart | accessQueryStatus}},
		{"Stop", func(s *Service) error { return s.Stop(ctx, "demo") }, []uint32{accessQueryStatus, accessStop | accessEnumerateDependent | accessQueryStatus}},
		{"Reconcile", func(s *Service) error {
			_, err := s.Reconcile(ctx, "demo", `C:\Program Files\Demo\demo.exe`)
			return err
		}, []uint32{accessQueryConfig | accessQueryStatus}},
	} {
		f := newFake()
		f.add("demo", 4242)
		s, _ := testService(t, f, Options{})
		if err := c.call(s); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		var got []uint32
		for _, call := range f.calls {
			var name string
			var access uint32
			if n, _ := sscanOpen(call, &name, &access); n == 2 {
				got = append(got, access)
			}
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("%s opened with %#x, want %#x", c.name, got, c.want)
		}
	}
}

func TestStopWaits(t *testing.T) {
	f := newFake()
	svc := f.add("demo", 4242)
	svc.onStop = func(s *fakeSvc) {
		for cp := uint32(1); cp <= 5; cp++ {
			s.queue = append(s.queue, status{Type: 0x10, State: stateStopPending, CheckPoint: cp, WaitHint: 4000, ProcessID: 4242})
		}
		s.queue = append(s.queue, status{Type: 0x10, State: stateStopped})
	}
	s, clock := testService(t, f, Options{})
	start := clock.now()
	if err := s.Stop(context.Background(), "demo"); err != nil {
		t.Fatal(err)
	}
	// Five sleeps, from the first pending status to STOPPED, each a tenth
	// of the 4 s hint, clamped up to 1 s.
	if took := clock.now().Sub(start); took != 5*time.Second {
		t.Fatalf("waited %v of virtual time, want 5s", took)
	}
	if s.previous["demo"] != 4242 {
		t.Fatalf("previous pid %d", s.previous["demo"])
	}
}

func TestStopWaitHintClamp(t *testing.T) {
	for _, c := range []struct {
		hint uint32
		want time.Duration
	}{{0, time.Second}, {5000, time.Second}, {50_000, 5 * time.Second}, {500_000, 10 * time.Second}} {
		f := newFake()
		f.add("demo", 1).onStop = func(s *fakeSvc) {
			s.queue = append(s.queue, status{State: stateStopPending, CheckPoint: 1, WaitHint: c.hint}, status{State: stateStopped})
		}
		s, clock := testService(t, f, Options{})
		start := clock.now()
		if err := s.Stop(context.Background(), "demo"); err != nil {
			t.Fatal(err)
		}
		if took := clock.now().Sub(start); took != c.want {
			t.Errorf("hint %d ms: slept %v, want %v", c.hint, took, c.want)
		}
	}
}

// TestStopStalledCheckpoint: a checkpoint that does not advance for the
// wait hint, at least stallFloor, is a hang.
func TestStopStalledCheckpoint(t *testing.T) {
	f := newFake()
	f.add("demo", 1).onStop = func(s *fakeSvc) {
		s.queue = append(s.queue, status{State: stateStopPending, CheckPoint: 3, WaitHint: 2000})
	}
	s, clock := testService(t, f, Options{Poll: service.PollOptions{Timeout: time.Hour}})
	start := clock.now()
	err := s.Stop(context.Background(), "demo")
	if !errors.Is(err, service.ErrTimeout) || !strings.Contains(err.Error(), "hung") {
		t.Fatalf("err = %v", err)
	}
	if took := clock.now().Sub(start); took <= stallFloor || took > stallFloor+2*time.Second {
		t.Fatalf("declared the hang after %v", took)
	}
}

func TestStopNotActiveAndStopped(t *testing.T) {
	f := newFake()
	f.add("demo", 1).controlErr = []error{errNotActive}
	s, _ := testService(t, f, Options{})
	if err := s.Stop(context.Background(), "demo"); err != nil {
		t.Fatalf("not active: %v", err)
	}
	f = newFake()
	f.add("demo", 0).st.State = stateStopped
	s, _ = testService(t, f, Options{})
	if err := s.Stop(context.Background(), "demo"); err != nil || slices.Contains(f.verbs(), "control") {
		t.Fatalf("stopped: %v, verbs %q", err, f.verbs())
	}
}

// TestStopWhileStarting: a service still starting cannot take the stop;
// it is waited out and asked again.
func TestStopWhileStarting(t *testing.T) {
	f := newFake()
	svc := f.add("demo", 4242)
	svc.st.State = stateStartPending
	svc.controlErr = []error{errCannotAcceptCtrl}
	svc.queue = []status{{State: stateStartPending, CheckPoint: 1, ProcessID: 4242}, {State: stateRunning, ProcessID: 4242}}
	s, _ := testService(t, f, Options{})
	if err := s.Stop(context.Background(), "demo"); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(strings.Join(f.verbs(), " "), "control"); n != 2 {
		t.Fatalf("%d stop controls, want 2", n)
	}
	// A service that never takes the stop is an error, not a loop.
	f = newFake()
	f.add("demo", 4242).controlErr = []error{errCannotAcceptCtrl, errCannotAcceptCtrl, errCannotAcceptCtrl}
	s, _ = testService(t, f, Options{})
	if err := s.Stop(context.Background(), "demo"); err == nil || !strings.Contains(err.Error(), "does not take the stop") {
		t.Fatalf("err = %v", err)
	}
}

func TestStopDependents(t *testing.T) {
	f := newFake()
	f.add("demo", 1).deps = []string{"child", "grandchild"}
	f.add("child", 2)
	f.add("grandchild", 3)
	s, _ := testService(t, f, Options{})
	err := s.Stop(context.Background(), "demo")
	if !errors.Is(err, errDependentsRunning) || !strings.Contains(err.Error(), "child, grandchild") {
		t.Fatalf("err = %v", err)
	}
	if slices.Contains(f.verbs(), "control") {
		t.Fatal("stopped with dependents running")
	}
	f.calls = nil
	s.o.StopDependents = true
	if err := s.Stop(context.Background(), "demo"); err != nil {
		t.Fatal(err)
	}
	var stops []string
	for _, c := range f.calls {
		if strings.HasPrefix(c, "control ") {
			stops = append(stops, strings.Fields(c)[1])
		}
	}
	if !slices.Equal(stops, []string{"child", "grandchild", "demo"}) {
		t.Fatalf("stopped %q", stops)
	}
}

// TestStopDependentsAreRestarted: the dependents Stop stopped are started
// again by Start, in reverse stop order, and so is one stopped before the
// service's own stop failed
// (docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md D6).
func TestStopDependentsAreRestarted(t *testing.T) {
	ctx := context.Background()
	f := newFake()
	f.add("demo", 1).deps = []string{"child", "grandchild"}
	f.add("child", 2)
	f.add("grandchild", 3)
	s, _ := testService(t, f, Options{StopDependents: true})
	if err := s.Stop(ctx, "demo"); err != nil {
		t.Fatal(err)
	}
	f.calls = nil
	if err := s.Start(ctx, "demo"); err != nil {
		t.Fatal(err)
	}
	var starts []string
	for _, c := range f.calls {
		if strings.HasPrefix(c, "start ") {
			starts = append(starts, strings.Fields(c)[1])
		}
	}
	if !slices.Equal(starts, []string{"demo", "grandchild", "child"}) {
		t.Fatalf("started %q", starts)
	}
	for _, name := range []string{"child", "grandchild"} {
		if st := f.services[name].st.State; st != stateRunning {
			t.Fatalf("%s %s after Start", name, stateName(st))
		}
	}

	// demo's own stop fails after child stopped: Start brings child back.
	f = newFake()
	demo := f.add("demo", 1)
	demo.deps = []string{"child"}
	demo.controlErr = []error{errors.New("access denied")}
	f.add("child", 2)
	s, _ = testService(t, f, Options{StopDependents: true})
	if err := s.Stop(ctx, "demo"); err == nil || f.services["child"].st.State != stateStopped {
		t.Fatalf("Stop = %v, child %s; want demo's stop to fail after child stopped", err, stateName(f.services["child"].st.State))
	}
	if err := s.Start(ctx, "demo"); err != nil {
		t.Fatal(err)
	}
	if st := f.services["child"].st.State; st != stateRunning {
		t.Fatalf("child %s after Start; want it running again", stateName(st))
	}
}

func TestStopRefusesInside(t *testing.T) {
	f := newFake()
	f.add("demo", 6000) // this process's parent
	s, _ := testService(t, f, Options{})
	if err := s.Stop(context.Background(), "demo"); !errors.Is(err, service.ErrInsideService) {
		t.Fatalf("err = %v", err)
	}
	if slices.Contains(f.verbs(), "control") {
		t.Fatal("the service was stopped from inside it")
	}
}

func TestStart(t *testing.T) {
	ctx := context.Background()
	f := newFake()
	svc := f.add("demo", 0)
	svc.st = status{Type: 0x10, State: stateStopPending, CheckPoint: 1, WaitHint: 1000}
	svc.queue = []status{{Type: 0x10, State: stateStopPending, CheckPoint: 2}, {Type: 0x10, State: stateStopped}}
	s, _ := testService(t, f, Options{})
	if err := s.Start(ctx, "demo"); err != nil {
		t.Fatal(err)
	}
	if v := f.verbs(); v[len(v)-1] != "start" {
		t.Fatalf("verbs %q: a pending stop must be waited out, then started", v)
	}
	// Already running: left alone. ERROR_SERVICE_ALREADY_RUNNING is
	// success.
	f = newFake()
	f.add("demo", 1)
	s, _ = testService(t, f, Options{})
	if err := s.Start(ctx, "demo"); err != nil || slices.Contains(f.verbs(), "start") {
		t.Fatalf("running: %v, verbs %q", err, f.verbs())
	}
	f = newFake()
	svc = f.add("demo", 0)
	svc.st.State, svc.startErr = stateStopped, errAlreadyRunning
	svc.queue = []status{{Type: 0x10, State: stateRunning, ProcessID: 7}}
	s, _ = testService(t, f, Options{})
	if err := s.Start(ctx, "demo"); err != nil {
		t.Fatalf("already running: %v", err)
	}
}

func TestStartStopsWhileStarting(t *testing.T) {
	f := newFake()
	svc := f.add("demo", 0)
	svc.st.State = stateStopped
	svc.onStart = func(s *fakeSvc) {
		s.queue = append(s.queue, status{State: stateStartPending, ProcessID: 9}, status{State: stateStopped, Win32ExitCode: 1067})
	}
	s, _ := testService(t, f, Options{})
	err := s.Start(context.Background(), "demo")
	if !errors.Is(err, service.ErrUnhealthy) || !strings.Contains(err.Error(), "exit code 1067") {
		t.Fatalf("err = %v", err)
	}
}

func TestWaitHealthy(t *testing.T) {
	ctx := context.Background()
	f := newFake()
	svc := f.add("demo", 4242)
	s, _ := testService(t, f, Options{})
	if err := s.Stop(ctx, "demo"); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(ctx, "demo"); err != nil {
		t.Fatal(err)
	}
	if err := s.WaitHealthy(ctx, "demo"); err != nil {
		t.Fatalf("a new process: %v", err)
	}
	// The process Stop saw never counts.
	svc.st = status{Type: 0x10, State: stateRunning, ProcessID: 4242}
	if err := s.WaitHealthy(ctx, "demo"); !errors.Is(err, service.ErrTimeout) {
		t.Fatalf("the old process: %v", err)
	}
	// A stop fails at once, with the service-specific code.
	svc.st = status{Type: 0x10, State: stateStopped, Win32ExitCode: errServiceSpecificErr, SpecificExit: 42}
	err := s.WaitHealthy(ctx, "demo")
	if !errors.Is(err, service.ErrUnhealthy) || !strings.Contains(err.Error(), "service-specific exit code 42") {
		t.Fatalf("stopped: %v", err)
	}
}

func TestInside(t *testing.T) {
	for _, c := range []struct {
		name     string
		procs    []process
		born     map[uint32]uint64
		pid      uint32
		state    uint32
		want     bool
		noTarget bool
	}{
		{"self", nil, nil, selfPID, stateRunning, true, false},
		{"parent", nil, nil, 6000, stateRunning, true, false},
		{"grandparent", nil, nil, 4, stateRunning, true, false},
		{"unrelated", nil, nil, 5555, stateRunning, false, false},
		{"stopped", nil, nil, 0, stateStopped, false, false},
		{"not installed", nil, nil, 0, 0, false, true},
		// The hop has exited: the walk stops at the missing parent.
		{"parent gone", []process{{PID: selfPID, ParentPID: 6100}, {PID: 6000, ParentPID: 4}}, nil, 6000, stateRunning, false, false},
		// 6000 was created after this process: a reused PID.
		{"reused pid", nil, map[uint32]uint64{selfPID: 100, 6000: 200}, 6000, stateRunning, false, false},
		{"older parent", nil, map[uint32]uint64{selfPID: 200, 6000: 100}, 6000, stateRunning, true, false},
		// A time that cannot be read counts the link.
		{"unreadable time", nil, map[uint32]uint64{selfPID: 200}, 6000, stateRunning, true, false},
	} {
		f := newFake()
		f.procs = c.procs
		if c.born != nil {
			f.born = c.born
		}
		if !c.noTarget {
			svc := f.add("demo", c.pid)
			svc.st.State = c.state
		}
		s, _ := testService(t, f, Options{})
		if got, err := s.Inside(context.Background()); err != nil || got != c.want {
			t.Errorf("%s: inside %t, %v; want %t", c.name, got, err, c.want)
		}
	}
	s, _ := testService(t, newFake(), Options{})
	s.o.Name = ""
	if _, err := s.Inside(context.Background()); err == nil {
		t.Fatal("Inside without Options.Name")
	}
}

func TestReconcile(t *testing.T) {
	ctx := context.Background()
	const exe = `C:\Program Files\Demo\demo.exe`
	for _, line := range []string{
		`"C:\Program Files\Demo\demo.exe" run`,
		`"c:\program files\demo\DEMO.EXE"`,
		`C:\Program Files\Demo\demo.exe run --flag`,
		`C:\Program Files\Demo\demo.exe`,
	} {
		f := newFake()
		f.add("demo", 1).cfg.BinaryPathName = line
		s, _ := testService(t, f, Options{})
		res, err := s.Reconcile(ctx, "demo", exe)
		if err != nil || res.Changed {
			t.Errorf("%s: %+v, %v", line, res, err)
		}
	}
	f := newFake()
	svc := f.add("demo", 1)
	svc.cfg.BinaryPathName = `"C:\Old Place\demo.exe" run --flag`
	s, _ := testService(t, f, Options{})
	if _, err := s.Reconcile(ctx, "demo", exe); err == nil || !strings.Contains(err.Error(), "RewritePath") {
		t.Fatalf("another binary without RewritePath: %v", err)
	}
	if len(svc.paths) != 0 {
		t.Fatal("rewrote without RewritePath")
	}
	s.o.RewritePath = true
	res, err := s.Reconcile(ctx, "demo", exe)
	if err != nil || !res.Changed {
		t.Fatalf("%+v, %v", res, err)
	}
	if want := `"C:\Program Files\Demo\demo.exe" run --flag`; svc.cfg.BinaryPathName != want {
		t.Fatalf("rewrote to %s, want %s", svc.cfg.BinaryPathName, want)
	}
	if err := s.Restore(ctx, "demo", res); err != nil {
		t.Fatal(err)
	}
	if svc.cfg.BinaryPathName != `"C:\Old Place\demo.exe" run --flag` {
		t.Fatalf("restored %s", svc.cfg.BinaryPathName)
	}
	if _, err := s.Reconcile(ctx, "demo", `C:\a"b\demo.exe`); err == nil {
		t.Fatal("a quote in the path was accepted")
	}
	if err := s.Restore(ctx, "demo", selfupdate.ReconcileResult{Changed: true, State: "x"}); err == nil {
		t.Fatal("a foreign receipt was restored")
	}
}
