package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
)

// The plan a handoff carries
// (docs/decisions/0038-MADR-pinned-plans-lifecycle-conformance-and-bubble-tests.md
// D1-A, F12). Without it, a detached run resolves the latest release again,
// in another process, later: not necessarily the release that justified
// the handoff. With it, the detached run applies the plan the run that
// handed off made, through Updater.Apply, which refuses it with
// selfupdate.ErrPlanStale when the release no longer matches.

// EnvHandOffPlan holds, in a detached run, the plan the run that handed off
// made, as a compact selfupdate.PlanDocument. Every backend carries it as
// it carries HandOff.Env.
const EnvHandOffPlan = "SELFUPDATE_HANDOFF_PLAN"

// HandOffPlanFunc returns the cli.HandOff DetachPlan hook for d and spec.
// It is HandOffFunc's hook, with the same rules, refusing inside the
// service a request without Yes with selfupdate.ErrConfirmationRequired,
// and with EnvHandOffPlan, the plan's document, added to a copy of
// spec.Env. A plan carries no approval: the detached run applies it with
// the request's Yes.
func HandOffPlanFunc(d Detacher, spec HandOff) func(context.Context, selfupdate.Request, selfupdate.Plan) (bool, string, error) {
	return func(ctx context.Context, req selfupdate.Request, p selfupdate.Plan) (bool, string, error) {
		doc, err := json.Marshal(p.Document())
		if err != nil {
			return false, "", fmt.Errorf("selfupdate: service: encode the plan: %w", err)
		}
		withPlan := spec
		withPlan.Env = append(append([]string(nil), spec.Env...), EnvHandOffPlan+"="+string(doc))
		return HandOffFunc(d, withPlan)(ctx, req)
	}
}

// LoadHandOffPlan returns the plan a detached run was handed, and unsets
// EnvHandOffPlan, so nothing the run starts inherits it. ok is false, with
// no error, when the variable is unset or empty: the run was not handed a
// plan. A document selfupdate.ParsePlan refuses is an error. Call it after
// ReportFunc or LoadHandOffEnv, which apply the private environment file a
// backend may carry the variable in; it is the cli.HandOff Planned hook. It
// takes no context: it reads one variable.
func LoadHandOffPlan() (p selfupdate.Plan, ok bool, err error) {
	doc, set := os.LookupEnv(EnvHandOffPlan)
	if !set {
		return selfupdate.Plan{}, false, nil
	}
	if err := os.Unsetenv(EnvHandOffPlan); err != nil {
		return selfupdate.Plan{}, false, err
	}
	if doc == "" {
		return selfupdate.Plan{}, false, nil
	}
	p, err = selfupdate.ParsePlan([]byte(doc))
	if err != nil {
		return selfupdate.Plan{}, false, fmt.Errorf("selfupdate: service: the handed-off plan: %w", err)
	}
	return p, true, nil
}
