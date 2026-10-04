package systemd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
)

// Tests for docs/decisions/0011-PLAN-reference-service-lifecycles.md V2
// step 5: the verified no-op and the opt-in drop-in.

func TestReconcileNoOp(t *testing.T) {
	f := newFake()
	f.props["ExecStart"] = "{ path=/opt/demo/demo ; argv[]=/opt/demo/demo serve --port 1 ; ignore_errors=no ; start_time=[n/a] }"
	u := testUnit(t, f, Options{})
	res, err := u.Reconcile(context.Background(), "demo", "/opt/demo/./demo")
	if err != nil || res.Changed || res.State != nil {
		t.Fatalf("res %+v, err %v", res, err)
	}
	if err := u.Restore(context.Background(), "demo", res); err != nil {
		t.Fatal(err)
	}
}

func TestReconcileMismatchNeedsRewritePath(t *testing.T) {
	f := newFake()
	f.props["ExecStart"] = "{ path=/opt/old/demo ; argv[]=/opt/old/demo ; }"
	u := testUnit(t, f, Options{})
	if _, err := u.Reconcile(context.Background(), "demo", "/opt/new/demo"); err == nil ||
		!strings.Contains(err.Error(), "RewritePath") {
		t.Fatalf("err = %v", err)
	}
	f.props["ExecStart"] = ""
	if _, err := u.Reconcile(context.Background(), "demo", "/opt/new/demo"); err == nil {
		t.Fatal("a unit with no ExecStart was reconciled")
	}
}

func TestReconcileRewrite(t *testing.T) {
	dir := t.TempDir()
	fragment := filepath.Join(dir, "demo.service")
	writeUnit(t, fragment, "[Unit]\nDescription=x\n[Service]\nType=notify\nExecStart=-/opt/old/demo serve \\\n  --name \"a b\"\n")
	override := filepath.Join(dir, "override.conf")
	writeUnit(t, override, "[Service]\nEnvironment=X=1\n")
	f := newFake()
	f.props["ExecStart"] = "{ path=/opt/old/demo ; argv[]=/opt/old/demo serve --name a b ; }"
	f.props["FragmentPath"], f.props["DropInPaths"] = fragment, override
	f.props["NeedDaemonReload"] = "yes"
	dropIns := filepath.Join(dir, "etc")
	u := testUnit(t, f, Options{RewritePath: true, DropInDir: dropIns})

	res, err := u.Reconcile(context.Background(), "demo", "/opt/new/demo")
	if err != nil || !res.Changed {
		t.Fatalf("res %+v, err %v", res, err)
	}
	path := filepath.Join(dropIns, "demo.service.d", dropInName)
	body := readFile(t, path)
	if !strings.Contains(body, "[Service]\nExecStart=\nExecStart=-\"/opt/new/demo\" serve    --name \"a b\"\n") {
		t.Fatalf("drop-in:\n%s", body)
	}
	if v := f.verbs(); v[len(v)-2] != "daemon-reload" {
		t.Fatalf("verbs %q", v)
	}

	// Restore removes the drop-in this package created.
	if err := u.Restore(context.Background(), "demo", res); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("the drop-in was kept: %v", err)
	}

	// A drop-in that was there before is put back as it was.
	writeUnit(t, path, "# before\n")
	res, err = u.Reconcile(context.Background(), "demo", "/opt/new/demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := u.Restore(context.Background(), "demo", res); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != "# before\n" {
		t.Fatalf("restored %q", got)
	}
}

func TestReconcileStillNeedsReload(t *testing.T) {
	dir := t.TempDir()
	fragment := filepath.Join(dir, "demo.service")
	writeUnit(t, fragment, "[Service]\nExecStart=/opt/old/demo\n")
	f := newFake()
	f.props["ExecStart"] = "{ path=/opt/old/demo ; }"
	f.props["FragmentPath"] = fragment
	f.props["NeedDaemonReload"] = "yes"
	f.reloadIgnored = true
	u := testUnit(t, f, Options{RewritePath: true, DropInDir: dir})
	res, err := u.Reconcile(context.Background(), "demo", "/opt/new/demo")
	if err == nil || !strings.Contains(err.Error(), "still needs a daemon-reload") || !res.Changed {
		t.Fatalf("res %+v, err %v; want the receipt with the error", res, err)
	}
}

func TestEffectiveExecStart(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	writeUnit(t, a, "[Service]\nExecStart=/x one\n[Install]\nExecStart=/not/service\n")
	writeUnit(t, b, "[Service]\nExecStart=\nExecStart=/y two\n")
	if got, err := effectiveExecStart([]string{a, b}); err != nil || got != "/y two" {
		t.Fatalf("got %q, %v", got, err)
	}
	writeUnit(t, b, "[Service]\nExecStart=/y two\n")
	if _, err := effectiveExecStart([]string{a, b}); err == nil {
		t.Fatal("two ExecStart commands were accepted")
	}
	if _, err := effectiveExecStart([]string{filepath.Join(dir, "missing")}); err == nil {
		t.Fatal("a missing file was accepted")
	}
}

func TestReplaceProgram(t *testing.T) {
	for line, want := range map[string]string{
		"/x a b":       `"/n/p" a b`,
		"@-/x a":       `@-"/n/p" a`,
		`"/x y/z" a`:   `"/n/p" a`,
		"/x":           `"/n/p"`,
		"!!/x\t--flag": "!!\"/n/p\"\t--flag",
	} {
		if got, err := replaceProgram(line, "/n/p"); err != nil || got != want {
			t.Errorf("replaceProgram(%q) = %q, %v; want %q", line, got, err, want)
		}
	}
	if _, err := replaceProgram(`"/x a`, "/n/p"); err == nil {
		t.Error("an unterminated quote was accepted")
	}
	if _, err := replaceProgram("/x", `/n/"p`); err == nil {
		t.Error("a quote in the new path was accepted")
	}
}

func TestRestoreRefusesForeignState(t *testing.T) {
	u := testUnit(t, newFake(), Options{})
	if err := u.Restore(context.Background(), "demo", selfupdate.ReconcileResult{Changed: true, State: "x"}); err == nil {
		t.Fatal("a foreign receipt was restored")
	}
}

func writeUnit(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
