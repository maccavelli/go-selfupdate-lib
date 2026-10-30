package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/selfupdatetest"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// Tests for docs/decisions/0011-PLAN-reference-service-lifecycles.md V5
// step 1: cli.Options.HandOff (0011-MADR §9). The text and JSON goldens of
// a handoff are the handed-off and handoff-failed scenarios.

// TestHandOffOnlyForApply: a check and a dry run never ask to hand off;
// an apply does, with its request.
func TestHandOffOnlyForApply(t *testing.T) {
	for _, c := range []struct {
		name  string
		flags Flags
		want  bool
	}{
		{"check", Flags{Check: true}, false},
		{"dry run", Flags{DryRun: true}, false},
		{"apply", Flags{Yes: true}, true},
	} {
		var asked *selfupdate.Request
		sc := scenario{latest: "v1.1.0", id: releaseID, flags: c.flags, handOff: HandOff{
			Detach: func(_ context.Context, req selfupdate.Request) (bool, string, error) {
				asked = &req
				return false, "", nil
			},
		}}
		out := sc.run(t, false)
		if (asked != nil) != c.want {
			t.Fatalf("%s: Detach called %t, want %t", c.name, asked != nil, c.want)
		}
		if asked != nil && (asked.Product != "demo" || !asked.Yes) {
			t.Fatalf("%s: Detach saw %+v", c.name, *asked)
		}
		// Not handed off: the update ran here.
		if c.want && (out.code != 0 || !out.res.Applied) {
			t.Fatalf("%s: exit %d, %+v", c.name, out.code, out.res)
		}
	}
}

// insideDetacher is a service.Detacher for a process inside the service;
// it records whether Detach was called.
type insideDetacher struct{ called bool }

func (*insideDetacher) Inside(context.Context) (bool, error) { return true, nil }

func (d *insideDetacher) Detach(context.Context, service.HandOff) (service.Detached, error) {
	d.called = true
	return service.Detached{ID: "abc", Where: "fake"}, nil
}

// TestHandOffNeedsYes: an update from inside the service runs detached,
// where nobody can answer a prompt, so without --yes it is refused, even
// when this run could ask
// (docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md C3).
func TestHandOffNeedsYes(t *testing.T) {
	t.Setenv(service.EnvHandOff, "")
	d := &insideDetacher{}
	out := scenario{latest: "v1.1.0", id: releaseID, stdin: "y\n", interactive: true, handOff: HandOff{
		Detach: service.HandOffFunc(d, service.HandOff{ID: "abc", ResultPath: t.TempDir() + "/result"}),
	}}.run(t, false)
	if d.called || out.code != 1 || !errors.Is(out.err, selfupdate.ErrConfirmationRequired) || out.res.Applied {
		t.Fatalf("Detach called=%t exit=%d err=%v applied=%t; want a refusal", d.called, out.code, out.err, out.res.Applied)
	}
}

// TestHandOffOnlyWhenUpdateFound: with nothing to install there is
// nothing to hand off; the run reports "up to date" here (0015-MADR C3).
func TestHandOffOnlyWhenUpdateFound(t *testing.T) {
	called := false
	out := scenario{latest: "v1.0.0", id: releaseID, flags: Flags{Yes: true}, handOff: HandOff{
		Detach: func(context.Context, selfupdate.Request) (bool, string, error) {
			called = true
			return true, "abc", nil
		},
	}}.run(t, false)
	if called || out.code != 0 || !strings.Contains(out.stderr, "up to date") {
		t.Fatalf("Detach called=%t exit=%d stderr %q", called, out.code, out.stderr)
	}
}

// failing is the scenario source whose discovery fails.
func failing() func(*selfupdatetest.FakeSource) selfupdate.ReleaseSource {
	return func(f *selfupdatetest.FakeSource) selfupdate.ReleaseSource { return failSource{f} }
}

// TestHandOffReport: Report sees the outcome of an update that ran here,
// never runs for one that was handed off, and its error fails the run.
func TestHandOffReport(t *testing.T) {
	type seen struct {
		res selfupdate.Result
		err error
	}
	var got []seen
	report := func(res selfupdate.Result, err error) error {
		got = append(got, seen{res, err})
		return nil
	}
	out := scenario{latest: "v1.1.0", id: releaseID, flags: Flags{Yes: true}, handOff: HandOff{Report: report}}.run(t, false)
	if len(got) != 1 || !got[0].res.Applied || got[0].err != nil || out.code != 0 {
		t.Fatalf("applied: Report saw %+v; exit %d", got, out.code)
	}
	got = nil
	scenario{latest: "v1.1.0", id: releaseID, flags: Flags{Check: true}, src: failing(), handOff: HandOff{Report: report}}.run(t, false)
	if len(got) != 1 || got[0].err == nil {
		t.Fatalf("failed: Report saw %+v", got)
	}
	got = nil
	scenario{latest: "v1.1.0", id: releaseID, flags: Flags{Yes: true}, handOff: HandOff{
		Detach: func(context.Context, selfupdate.Request) (bool, string, error) { return true, "abc", nil },
		Report: report,
	}}.run(t, false)
	if len(got) != 0 {
		t.Fatalf("handed off: Report saw %+v", got)
	}
	out = scenario{latest: "v1.1.0", id: releaseID, flags: Flags{Yes: true}, handOff: HandOff{
		Report: func(selfupdate.Result, error) error { return errors.New("result file: read-only file system") },
	}}.run(t, false)
	if out.code != 1 || !strings.Contains(out.stderr, "update failed:") || !strings.Contains(out.stderr, "read-only file system") {
		t.Fatalf("a failed Report: exit %d, stderr %q", out.code, out.stderr)
	}
}

// TestHandOffWriteFails: a handoff whose notice cannot be written fails
// the command, as any other unwritten outcome does.
func TestHandOffWriteFails(t *testing.T) {
	src, tg := scenario{latest: "v1.1.0"}.fixture(t, "demo")
	u, err := buildUpdater(src, tg)
	if err != nil {
		t.Fatal(err)
	}
	stderr := &failOn{marker: "update handed off"}
	req, err := Flags{Yes: true}.Request("demo", releaseID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Run(context.Background(), u, req, Options{
		Stderr: stderr, Stdout: &bytes.Buffer{}, Signals: []os.Signal{},
		HandOff: HandOff{Detach: func(context.Context, selfupdate.Request) (bool, string, error) { return true, "abc", nil }},
	})
	if err == nil {
		t.Fatal("an unwritten handoff notice succeeded")
	}
}

// TestBatchErrorReachesCaller: a handoff refused as a batch file, as
// service.DetachProcess refuses one on Windows, fails Run with an error
// that wraps selfupdate.ErrBatchFile (0022-MADR Amendment A2, D5-A).
func TestBatchErrorReachesCaller(t *testing.T) {
	ran := false
	sc := scenario{latest: "v1.1.0", id: releaseID, flags: Flags{Yes: true}, handOff: HandOff{
		Detach: func(context.Context, selfupdate.Request) (bool, string, error) {
			ran = true
			return false, "", fmt.Errorf("fake: %w", selfupdate.ErrBatchFile)
		},
	}}
	out := sc.run(t, false)
	if !ran || !errors.Is(out.err, selfupdate.ErrBatchFile) || out.code != 1 {
		t.Fatalf("Detach ran %t, exit %d, err %v; want a run, exit 1 and ErrBatchFile", ran, out.code, out.err)
	}
}

// Tests for the plan a handoff carries
// (docs/decisions/0038-MADR-pinned-plans-lifecycle-conformance-and-bubble-tests.md
// D1-A; 0038-PLAN Q16).

// recordingDetacher is a service.Detacher inside the service that records
// the spec it was asked to detach.
type recordingDetacher struct{ spec *service.HandOff }

func (*recordingDetacher) Inside(context.Context) (bool, error) { return true, nil }

func (d *recordingDetacher) Detach(_ context.Context, spec service.HandOff) (service.Detached, error) {
	d.spec = &spec
	return service.Detached{ID: spec.ID, Where: "fake", ResultPath: spec.ResultPath}, nil
}

// publishing is a scenario source on which publish puts a newer release.
func publishing(publish *func()) func(*selfupdatetest.FakeSource) selfupdate.ReleaseSource {
	return func(f *selfupdatetest.FakeSource) selfupdate.ReleaseSource {
		*publish = func() { f.Publish(release("demo", "v1.2.0")) }
		return f
	}
}

// TestHandOffCarriesThePlan: the run inside the service plans v1.1.0 and
// hands off with the plan through service.HandOffPlanFunc; the detached
// run, through service.LoadHandOffPlan, installs v1.1.0 although v1.2.0
// has been published since.
func TestHandOffCarriesThePlan(t *testing.T) {
	t.Setenv(service.EnvHandOff, "")
	t.Setenv(service.EnvHandOffPlan, "")
	d := &recordingDetacher{}
	parent := scenario{latest: "v1.1.0", id: releaseID, flags: Flags{Yes: true}, handOff: HandOff{
		DetachPlan: service.HandOffPlanFunc(d, service.HandOff{ID: "abc", ResultPath: t.TempDir() + "/result"}),
		Planned:    service.LoadHandOffPlan,
	}}.run(t, false)
	if parent.code != 0 || parent.res.Applied || d.spec == nil || !strings.Contains(parent.stderr, "update handed off: abc") {
		t.Fatalf("parent: exit %d, %+v, stderr %q", parent.code, parent.res, parent.stderr)
	}
	parent.tg.unchanged(t)
	var doc string
	for _, kv := range d.spec.Env {
		if v, ok := strings.CutPrefix(kv, service.EnvHandOffPlan+"="); ok {
			doc = v
		}
	}
	if doc == "" {
		t.Fatalf("the handoff's Env %q carries no plan", d.spec.Env)
	}

	t.Setenv(service.EnvHandOffPlan, doc)
	var publish func()
	child := scenario{latest: "v1.1.0", id: releaseID, flags: Flags{Yes: true}, src: publishing(&publish), handOff: HandOff{
		DetachPlan: service.HandOffPlanFunc(d, service.HandOff{}),
		Planned: func() (selfupdate.Plan, bool, error) {
			publish()
			return service.LoadHandOffPlan()
		},
	}}.run(t, false)
	if child.code != 0 || !child.res.Applied || child.res.TargetVersion != "v1.1.0" {
		t.Fatalf("child: exit %d, %+v, stderr %q", child.code, child.res, child.stderr)
	}
	if got, err := os.ReadFile(child.tg.path); err != nil || string(got) != "demo v1.1.0\n" {
		t.Fatalf("child's target = %q, %v; want the parent's v1.1.0", got, err)
	}
}

// TestDetachPlanAppliesHere: outside the service, the run applies the plan
// it made, not a release published after it.
func TestDetachPlanAppliesHere(t *testing.T) {
	var publish func()
	asked := 0
	out := scenario{latest: "v1.1.0", id: releaseID, flags: Flags{Yes: true}, src: publishing(&publish), handOff: HandOff{
		DetachPlan: func(_ context.Context, _ selfupdate.Request, p selfupdate.Plan) (bool, string, error) {
			asked++
			if p.TargetVersion != "v1.1.0" {
				t.Errorf("DetachPlan saw a plan for %s", p.TargetVersion)
			}
			publish()
			return false, "", nil
		},
	}}.run(t, false)
	if asked != 1 || out.code != 0 || !out.res.Applied || out.res.TargetVersion != "v1.1.0" {
		t.Fatalf("asked %d, exit %d, %+v", asked, out.code, out.res)
	}
}

// TestDetachPlanOnlyForAnInstall: a check, a dry run and an up-to-date run
// never call DetachPlan.
func TestDetachPlanOnlyForAnInstall(t *testing.T) {
	for _, sc := range []scenario{
		{name: "check", latest: "v1.1.0", flags: Flags{Check: true}},
		{name: "dry run", latest: "v1.1.0", flags: Flags{DryRun: true}},
		{name: "up to date", latest: "v1.0.0", flags: Flags{Yes: true}},
	} {
		called := false
		sc.id = releaseID
		sc.handOff = HandOff{DetachPlan: func(context.Context, selfupdate.Request, selfupdate.Plan) (bool, string, error) {
			called = true
			return true, "abc", nil
		}}
		if out := sc.run(t, false); called || strings.Contains(out.stderr, "handed off") {
			t.Fatalf("%s: DetachPlan called %t, stderr %q", sc.name, called, out.stderr)
		}
	}
}

// TestDetachPlanFallsBack: when Plan fails, nothing is handed off, and the
// run reports exactly what it reports without DetachPlan.
func TestDetachPlanFallsBack(t *testing.T) {
	without := scenario{latest: "v1.1.0", id: releaseID, flags: Flags{Yes: true}, src: failing()}.run(t, false)
	called := false
	with := scenario{latest: "v1.1.0", id: releaseID, flags: Flags{Yes: true}, src: failing(), handOff: HandOff{
		DetachPlan: func(context.Context, selfupdate.Request, selfupdate.Plan) (bool, string, error) {
			called = true
			return true, "abc", nil
		},
	}}.run(t, false)
	if called || with.code != without.code || with.stderr != without.stderr || with.code != 1 {
		t.Fatalf("DetachPlan called %t; exit %d, stderr %q; without it exit %d, stderr %q",
			called, with.code, with.stderr, without.code, without.stderr)
	}
}

// TestPlannedRefusals: a plan the release no longer matches is
// ErrPlanStale, and a plan that cannot be loaded fails the run; each
// reaches Report, and nothing is installed.
func TestPlannedRefusals(t *testing.T) {
	stale := scenario{latest: "v1.1.0", id: releaseID, flags: Flags{Yes: true}}
	src, tg := stale.fixture(t, "demo")
	plan, err := newUpdater(t, src, tg).Plan(context.Background(), selfupdate.Request{
		Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: selfupdate.ReleaseBuild,
	})
	if err != nil {
		t.Fatal(err)
	}
	plan.ReleaseID++
	loadErr := errors.New("fixture: malformed plan")
	for name, c := range map[string]struct {
		planned func() (selfupdate.Plan, bool, error)
		want    error
	}{
		"stale":        {func() (selfupdate.Plan, bool, error) { return plan, true, nil }, selfupdate.ErrPlanStale},
		"load failure": {func() (selfupdate.Plan, bool, error) { return selfupdate.Plan{}, false, loadErr }, loadErr},
	} {
		var reported error
		sc := stale
		sc.handOff = HandOff{Planned: c.planned, Report: func(_ selfupdate.Result, err error) error {
			reported = err
			return nil
		}}
		out := sc.run(t, false)
		if out.code != 1 || !errors.Is(out.err, c.want) || !errors.Is(reported, c.want) || out.res.Applied {
			t.Fatalf("%s: exit %d, err %v, Report saw %v, %+v", name, out.code, out.err, reported, out.res)
		}
		out.tg.unchanged(t)
	}
}
