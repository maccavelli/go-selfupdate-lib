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

// TestRunEndsAtSelected: a check, and a run that finds the program up to
// date, install nothing. Their last event is EventSelected, the result's
// Operation is the outcome, and no complete, failed or declined follows
// (docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md C5,
// owner answer Q2).
func TestRunEndsAtSelected(t *testing.T) {
	for _, c := range []struct {
		name    string
		current string
		check   bool
		wantErr error
		wantOp  Operation
	}{
		{"check finds an update", "v1.0.0", true, ErrUpdateAvailable, OperationUpgrade},
		{"up-to-date check", "v1.1.0", true, nil, OperationNone},
		{"up-to-date apply", "v1.1.0", false, nil, OperationNone},
	} {
		t.Run(c.name, func(t *testing.T) {
			env := newContractEnv(t)
			req := applyReq()
			req.CurrentVersion, req.CheckOnly, req.Yes = c.current, c.check, !c.check
			res, err := env.build(t).Run(context.Background(), req)
			if !errors.Is(err, c.wantErr) || (c.wantErr == nil && err != nil) || res.Operation != c.wantOp {
				t.Fatalf("Run = %v, %v; want %v, %v", res.Operation, err, c.wantOp, c.wantErr)
			}
			kinds := env.rep.kinds
			if len(kinds) == 0 || kinds[len(kinds)-1] != EventSelected {
				t.Fatalf("events %v: want the last to be selected", kinds)
			}
			for _, k := range kinds {
				if k == EventComplete || k == EventFailed || k == EventDeclined {
					t.Fatalf("events %v: %v follows selected", kinds, k)
				}
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
