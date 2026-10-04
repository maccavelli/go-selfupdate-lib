package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
)

// Tests for docs/decisions/0011-PLAN-reference-service-lifecycles.md V1
// step 4: ExecReconciler and the version 1 receipt.

// fakeReconciler runs the test binary as the product, printing out and
// exiting with code.
func fakeReconciler(t *testing.T, out string, code string, extra ...string) *ExecReconciler {
	t.Helper()
	env := append([]string{fakeEnv + "=receipt", "FAKE_OUT=" + out, "FAKE_EXIT=" + code, "FAKE_ERR=fixture stderr"}, extra...)
	r, err := NewExecReconciler(ExecOptions{
		Reconcile: []string{"setup-service", "--refresh", "--json"},
		Restore:   []string{"setup-service", "--restore", "--json"},
		Env:       env,
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestExecReconcilerVerdicts(t *testing.T) {
	for _, c := range []struct {
		out     string
		changed bool
		verdict string
	}{
		{`{"schema_version":1,"verdict":"refreshed","path":"/u/demo.service","backup":"/u/demo.service.prev","changed":true,"reloaded":true}`, true, VerdictRefreshed},
		{`{"schema_version":1,"verdict":"unchanged","path":"/u/demo.service","changed":false}`, false, VerdictUnchanged},
		{`{"schema_version":1,"verdict":"kept","path":"/u/demo.service","changed":false,"reason":"hand-edited"}`, false, VerdictKept},
		{`{"schema_version":1,"verdict":"none","changed":false}`, false, VerdictNone},
		// magic-cli-remote's current output: no schema_version, and a field
		// this module does not know.
		{`{"verdict":"refreshed","path":"/u/x","backup":"/u/x.prev","changed":true,"reloaded":true,"warnings":["w"],"future":1}` + "\n", true, VerdictRefreshed},
	} {
		res, err := fakeReconciler(t, c.out, "0").Reconcile(context.Background(), "demo", selfExe(t))
		if err != nil {
			t.Fatalf("%s: %v", c.out, err)
		}
		st, ok := res.State.(ExecState)
		if !ok || res.Changed != c.changed || st.Receipt.Verdict != c.verdict || st.Receipt.SchemaVersion != 1 || st.Executable != selfExe(t) {
			t.Fatalf("%s: result %+v", c.out, res)
		}
	}
}

// TestExecReconcilerKeepsReceiptOnFailure: a child that wrote, then failed,
// still hands back its receipt, so Restore can run (0011-MADR §6).
func TestExecReconcilerKeepsReceiptOnFailure(t *testing.T) {
	out := `{"verdict":"refreshed","path":"/u/x","backup":"/u/x.prev","changed":true,"reloaded":false,"reason":"daemon-reload failed"}`
	res, err := fakeReconciler(t, out, "1").Reconcile(context.Background(), "demo", selfExe(t))
	if err == nil || !strings.Contains(err.Error(), "exit 1: fixture stderr") {
		t.Fatalf("err = %v", err)
	}
	if st, ok := res.State.(ExecState); !ok || !res.Changed || st.Receipt.Backup != "/u/x.prev" {
		t.Fatalf("the receipt was lost: %+v", res)
	}
}

func TestExecReconcilerNoReceipt(t *testing.T) {
	for _, c := range []struct{ out, code, want string }{
		{"", "3", "exit 3"},
		{"not json", "0", "malformed receipt"},
		{`{"changed":true}`, "0", "no verdict"},
		{`{"verdict":"none"}{"verdict":"none"}`, "0", "more than one"},
	} {
		res, err := fakeReconciler(t, c.out, c.code).Reconcile(context.Background(), "demo", selfExe(t))
		if err == nil || !strings.Contains(err.Error(), c.want) || res.State != nil || res.Changed {
			t.Errorf("%q exit %s: res %+v err %v; want %q and nothing to restore", c.out, c.code, res, err, c.want)
		}
	}
}

func TestExecReconcilerRestoreByNewBinary(t *testing.T) {
	stdin := filepath.Join(t.TempDir(), "stdin")
	out := `{"verdict":"refreshed","path":"/u/x","backup":"/u/x.prev","changed":true}`
	r := fakeReconciler(t, out, "0", "FAKE_STDIN="+stdin)
	res, err := r.Reconcile(context.Background(), "demo", selfExe(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Restore(context.Background(), "demo", res); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(stdin)
	if err != nil {
		t.Fatal(err)
	}
	var got Receipt
	if err := json.Unmarshal(body, &got); err != nil || got.Backup != "/u/x.prev" || got.SchemaVersion != 1 {
		t.Fatalf("restore read %q, %v", body, err)
	}
}

func TestExecReconcilerRestoreFunc(t *testing.T) {
	var got Receipt
	r, err := NewExecReconciler(ExecOptions{
		Reconcile: []string{"refresh"},
		RestoreFunc: func(_ context.Context, product string, rec Receipt) error {
			got = rec
			return nil
		},
		Env: []string{fakeEnv + "=receipt", `FAKE_OUT={"verdict":"refreshed","backup":"/u/b","changed":true}`},
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := r.Reconcile(context.Background(), "demo", selfExe(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Restore(context.Background(), "demo", res); err != nil || got.Backup != "/u/b" {
		t.Fatalf("RestoreFunc saw %+v, err %v", got, err)
	}
}

func TestExecReconcilerRestoreNothing(t *testing.T) {
	r := fakeReconciler(t, `{"verdict":"unchanged","changed":false}`, "0")
	unchanged := selfupdate.ReconcileResult{State: ExecState{Receipt: Receipt{Verdict: VerdictUnchanged}}}
	for name, res := range map[string]selfupdate.ReconcileResult{"unchanged": unchanged, "empty": {}} {
		if err := r.Restore(context.Background(), "demo", res); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if err := r.Restore(context.Background(), "demo", selfupdate.ReconcileResult{Changed: true, State: "foreign"}); err == nil {
		t.Error("a foreign receipt was restored")
	}
}

func TestNewExecReconcilerValidates(t *testing.T) {
	for name, o := range map[string]ExecOptions{
		"no reconcile": {Restore: []string{"r"}},
		"no restore":   {Reconcile: []string{"r"}},
	} {
		if _, err := NewExecReconciler(o); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if _, err := NewExecReconciler(ExecOptions{Reconcile: []string{"r"}, Restore: []string{"s"}}); err != nil {
		t.Fatal(err)
	}
}
