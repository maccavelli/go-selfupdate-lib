package selfupdate

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

// Tests for docs/decisions/0004-PLAN-v1-2-0-interaction-stream.md Step 2.

func yesReq() Request {
	req := applyReq()
	req.Yes = true
	return req
}

func TestRunWithOverridesReporter(t *testing.T) {
	t.Parallel()
	env := newContractEnv(t)
	u := env.build(t)
	override := &recReporter{}
	if _, err := u.RunWith(context.Background(), yesReq(), WithReporter(override)); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(override.kinds, EventComplete) || len(env.rep.kinds) != 0 {
		t.Fatalf("override got %v, configured got %v", override.kinds, env.rep.kinds)
	}
	seen := len(override.kinds)
	if _, err := u.Run(context.Background(), yesReq()); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(env.rep.kinds, EventComplete) || len(override.kinds) != seen {
		t.Fatalf("after RunWith, Run reported to %v (override %d → %d)", env.rep.kinds, seen, len(override.kinds))
	}
}

func TestRunWithOverridesConfirmer(t *testing.T) {
	t.Parallel()
	env := newContractEnv(t)
	u := env.build(t)
	override := &recConfirmer{ok: true}
	res, err := u.RunWith(context.Background(), applyReq(), WithConfirmer(override))
	if err != nil || !res.Applied {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if override.calls != 1 || slices.Contains(calls(*env.log), "Confirm") {
		t.Fatalf("override calls = %d, configured log = %v", override.calls, calls(*env.log))
	}
}

// progressUpdater is progressRun's Updater: a one-byte-per-read source and a
// clock that advances step per reading.
func progressUpdater(t *testing.T, interval time.Duration, rep *recReporter) *Updater {
	t.Helper()
	clock := cacheNow
	moving := ClockFunc(func() time.Time { clock = clock.Add(time.Millisecond); return clock })
	env := newContractEnv(t)
	_, _, plats := fixtureRelease(t, "demo")
	sel, err := NewExactAssetSelector(plats)
	if err != nil {
		t.Fatal(err)
	}
	u, err := New(Config{Source: oneByteSource{env.src}, Versions: NewStrictVersionPolicy(), Assets: sel,
		Installer: env.inst, Reporter: rep, Confirmer: env.conf, Limits: env.lim, ProgressInterval: interval, Clock: moving})
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestRunWithProgressInterval(t *testing.T) {
	t.Parallel()
	on := &recReporter{}
	if _, err := progressUpdater(t, 0, on).RunWith(context.Background(), yesReq(), WithProgressInterval(time.Nanosecond)); err != nil {
		t.Fatal(err)
	}
	if len(progressEvents(on)) == 0 {
		t.Fatal("WithProgressInterval(1ns) reported no progress on an Updater configured with zero")
	}
	off := &recReporter{}
	if _, err := progressUpdater(t, time.Nanosecond, off).RunWith(context.Background(), yesReq(), WithProgressInterval(0)); err != nil {
		t.Fatal(err)
	}
	if n := len(progressEvents(off)); n != 0 {
		t.Fatalf("WithProgressInterval(0) still reported %d progress events", n)
	}
}

func TestRunWithLastOptionWins(t *testing.T) {
	t.Parallel()
	env := newContractEnv(t)
	u := env.build(t)
	first, last := &recReporter{}, &recReporter{}
	if _, err := u.RunWith(context.Background(), yesReq(), WithReporter(first), WithReporter(last)); err != nil {
		t.Fatal(err)
	}
	if len(first.kinds) != 0 || !slices.Contains(last.kinds, EventComplete) {
		t.Fatalf("first got %v, last got %v", first.kinds, last.kinds)
	}
}

func TestRunWithRejectsInvalidOptions(t *testing.T) {
	t.Parallel()
	var typedNil *recReporter
	cases := []struct {
		name string
		opt  RunOption
		want string
	}{
		{"nil reporter", WithReporter(nil), "selfupdate: WithReporter: reporter is nil"},
		{"typed-nil reporter", WithReporter(typedNil), "selfupdate: WithReporter: reporter is nil"},
		{"nil confirmer", WithConfirmer(nil), "selfupdate: WithConfirmer: confirmer is nil"},
		{"negative interval", WithProgressInterval(-time.Second), "selfupdate: WithProgressInterval: interval must not be negative"},
		{"nil option", nil, "selfupdate: run option is nil"},
	}
	for _, c := range cases {
		env := newContractEnv(t)
		u := env.build(t)
		_, err := u.RunWith(context.Background(), yesReq(), c.opt)
		if err == nil || err.Error() != c.want {
			t.Errorf("%s: err = %v, want %q", c.name, err, c.want)
		}
		if len(*env.log) != 0 {
			t.Errorf("%s: the run started: %v", c.name, *env.log)
		}
	}
}

func TestRunWithSharesRunGuard(t *testing.T) {
	t.Parallel()
	env := newContractEnv(t)
	u := env.build(t)
	u.running.Store(true)
	defer u.running.Store(false)
	if _, err := u.RunWith(context.Background(), yesReq()); !errors.Is(err, ErrConcurrentUpdate) {
		t.Fatalf("RunWith during a run = %v, want ErrConcurrentUpdate", err)
	}
	if len(*env.log) != 0 {
		t.Fatalf("the second run started: %v", *env.log)
	}
}
