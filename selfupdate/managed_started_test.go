package selfupdate

import (
	"context"
	"encoding/json"
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
