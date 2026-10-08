package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
)

// Tests for docs/decisions/0010-PLAN-v1-6-0-owner-contracts.md S2 and its
// deviation D1: ServiceStarted, on InstallResult, on Result and in the
// document.

func TestManagedServiceStarted(t *testing.T) {
	for _, tc := range []struct {
		name string
		life Lifecycle
		want bool
	}{
		{"stopped, not enabled", &seqEnabledLife{}, false},
		{"stopped, enabled", &seqEnabledLife{enabled: true}, true},
		{"running", &seqLife{running: true}, true},
		{"stopped, no Enabled", &seqLife{}, false},
	} {
		res, _, err := installWith(t, tc.life, &fakeRec{})
		if err != nil || res.ServiceStarted != tc.want {
			t.Errorf("%s: ServiceStarted = %t, err = %v; want %t", tc.name, res.ServiceStarted, err, tc.want)
		}
	}
}

// TestRunReportsServiceStarted: Run copies ServiceStarted into Result, and
// the document carries it.
func TestRunReportsServiceStarted(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		_, exe := withTempHome(t)
		inner, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}})
		if err != nil {
			t.Fatal(err)
		}
		m, err := NewManagedInstaller(inner, &seqEnabledLife{enabled: enabled}, &fakeRec{})
		if err != nil {
			t.Fatal(err)
		}
		rel, bodies, plats := probeRelease(t)
		sel, err := NewExactAssetSelector(plats)
		if err != nil {
			t.Fatal(err)
		}
		u, err := New(Config{Source: &scriptSource{rel: rel, bodies: bodies}, Versions: NewStrictVersionPolicy(), Assets: sel,
			Installer: m, Reporter: &recReporter{}, Confirmer: &recConfirmer{}, Limits: DefaultLimits()})
		if err != nil {
			t.Fatal(err)
		}
		req := applyReq()
		req.Yes = true
		res, err := u.Run(context.Background(), req)
		if err != nil || !res.Applied || !res.ServiceInstalled || res.ServiceWasRunning || res.ServiceStarted != enabled {
			t.Fatalf("enabled %t: Run = %+v, %v", enabled, res, err)
		}
		b, err := json.Marshal(res.Document())
		if err != nil {
			t.Fatal(err)
		}
		want := `"service_was_running":false,"service_started":false`
		if enabled {
			want = `"service_was_running":false,"service_started":true`
		}
		if !strings.Contains(string(b), want) {
			t.Fatalf("enabled %t: document %s", enabled, b)
		}
	}
}

// TestRunReportsRolledBack: a managed update whose new binary fails its
// health check is rolled back, and the result says so, as the event does
// (docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md G3).
func TestRunReportsRolledBack(t *testing.T) {
	_, exe := withTempHome(t)
	inner, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}})
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewManagedInstaller(inner, &fakeLife{installed: true, running: true, healthErr: errors.New("unhealthy")}, &fakeRec{})
	if err != nil {
		t.Fatal(err)
	}
	rel, bodies, plats := probeRelease(t)
	sel, err := NewExactAssetSelector(plats)
	if err != nil {
		t.Fatal(err)
	}
	rep := &recReporter{}
	u, err := New(Config{Source: &scriptSource{rel: rel, bodies: bodies}, Versions: NewStrictVersionPolicy(), Assets: sel,
		Installer: m, Reporter: rep, Confirmer: &recConfirmer{}, Limits: DefaultLimits()})
	if err != nil {
		t.Fatal(err)
	}
	req := applyReq()
	req.Yes = true
	res, err := u.Run(context.Background(), req)
	if !errors.Is(err, ErrManagedInstall) || res.Applied || !res.RolledBack {
		t.Fatalf("Run = %+v, %v; want a rollback reported", res, err)
	}
	b, err := json.Marshal(res.Document())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"rolled_back":true`) {
		t.Fatalf("document %s", b)
	}
	if !slices.ContainsFunc(rep.events, func(ev Event) bool { return ev.Kind == EventRolledBack }) {
		t.Fatalf("events %v lack EventRolledBack", rep.events)
	}
}

// warnRec is a Reconciler that changes nothing and has warnings.
type warnRec struct{ warnings Warnings }

func (r warnRec) Reconcile(context.Context, string, string) (ReconcileResult, error) {
	return ReconcileResult{Warnings: r.warnings}, nil
}
func (warnRec) Restore(context.Context, string, ReconcileResult) error { return nil }

// TestRunReportsReconcileWarnings: a Reconciler's warnings, on an update
// that applied, are the run's warnings: one EventWarning each after
// complete, listed in Result.Warnings, and no error
// (docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md G4).
func TestRunReportsReconcileWarnings(t *testing.T) {
	_, exe := withTempHome(t)
	inner, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}})
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewManagedInstaller(inner, &fakeLife{installed: true, running: true},
		warnRec{warnings: NewWarnings("unit is masked", "not reloaded")})
	if err != nil {
		t.Fatal(err)
	}
	rel, bodies, plats := probeRelease(t)
	sel, err := NewExactAssetSelector(plats)
	if err != nil {
		t.Fatal(err)
	}
	rep := &recReporter{}
	u, err := New(Config{Source: &scriptSource{rel: rel, bodies: bodies}, Versions: NewStrictVersionPolicy(), Assets: sel,
		Installer: m, Reporter: rep, Confirmer: &recConfirmer{}, Limits: DefaultLimits()})
	if err != nil {
		t.Fatal(err)
	}
	req := applyReq()
	req.Yes = true
	res, err := u.Run(context.Background(), req)
	if err != nil || !res.Applied || !slices.Equal(res.Warnings.List(), []string{"unit is masked", "not reloaded"}) {
		t.Fatalf("Run = %+v, %v; want applied with two warnings", res, err)
	}
	if ExitCode(res, err) != 0 {
		t.Fatalf("exit %d, want 0", ExitCode(res, err))
	}
	var warnings []string
	complete := -1
	for i, ev := range rep.events {
		switch ev.Kind {
		case EventComplete:
			complete = i
		case EventWarning:
			if complete < 0 {
				t.Fatalf("a warning before complete: %v", rep.events)
			}
			warnings = append(warnings, ev.Detail)
		}
	}
	if !slices.Equal(warnings, []string{"unit is masked", "not reloaded"}) {
		t.Fatalf("warning events %q", warnings)
	}
}
