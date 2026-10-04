package selfupdate

import (
	"context"
	"errors"
	"testing"
)

// Tests for docs/decisions/0010-PLAN-v1-6-0-owner-contracts.md S3 (Q3, C2,
// C11): a run has one terminal event. Once EventComplete is reported, a
// later error does not fail the run. These use only the v1.5.1 API, so they
// run against the unfixed code too.

// lateErrorCases are the MADR's four probes: an error that arrives after
// the run has done what it set out to do.
func lateErrorCases() map[string]func(*contractEnv, *Request) {
	return map[string]func(*contractEnv, *Request){
		"close fails": func(e *contractEnv, _ *Request) { e.inst.closeErr = errors.New("fixture: unlock failed") },
		"complete report fails": func(e *contractEnv, _ *Request) {
			e.rep.errAt, e.rep.fail = EventComplete, errors.New("fixture: reporter closed")
		},
		"applied with an error": func(e *contractEnv, _ *Request) {
			e.inst.installErr = errors.New("fixture: backup removal failed")
		},
		"dry run, close fails": func(e *contractEnv, r *Request) {
			r.DryRun = true
			e.inst.closeErr = errors.New("fixture: unlock failed")
		},
	}
}

// terminalKinds counts the run's terminal events.
func terminalKinds(kinds []EventKind) (complete, failed int) {
	for _, k := range kinds {
		switch k {
		case EventComplete:
			complete++
		case EventFailed:
			failed++
		}
	}
	return complete, failed
}

func TestLateErrorIsNotAFailure(t *testing.T) {
	for name, mutate := range lateErrorCases() {
		t.Run(name, func(t *testing.T) {
			env := newContractEnv(t)
			req := applyReq()
			req.Yes = true
			mutate(env, &req)
			res, err := env.build(t).Run(context.Background(), req)
			if err != nil || ExitCode(res, err) != 0 {
				t.Fatalf("err = %v, exit %d; want no failure", err, ExitCode(res, err))
			}
			if res.Applied == req.DryRun {
				t.Fatalf("Applied = %t on a dry run %t", res.Applied, req.DryRun)
			}
			if complete, failed := terminalKinds(env.rep.kinds); complete != 1 || failed != 0 {
				t.Fatalf("events %v: %d complete, %d failed; want one complete", env.rep.kinds, complete, failed)
			}
		})
	}
}

// TestDryRunCompleteDetail: a dry run's EventComplete says so in its own
// Detail (C11).
func TestDryRunCompleteDetail(t *testing.T) {
	env := newContractEnv(t)
	req := applyReq()
	req.DryRun = true
	if _, err := env.build(t).Run(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	for _, ev := range env.rep.events {
		if ev.Kind == EventComplete && ev.Detail != "dry-run" {
			t.Fatalf("complete Detail = %q, want dry-run", ev.Detail)
		}
	}
}
