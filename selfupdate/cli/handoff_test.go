package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/selfupdatetest"
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
