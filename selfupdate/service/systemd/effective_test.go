package systemd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestReconcileVerifiesEffectiveExecStart: drop-ins apply in file-name
// order across directories, so an override.conf that resets ExecStart wins
// over this package's 90-selfupdate.conf. Once reloaded, the unit must run
// the new binary, or Reconcile fails, returning the receipt, so recovery's
// Restore removes the drop-in
// (docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md D4).
func TestReconcileVerifiesEffectiveExecStart(t *testing.T) {
	dir := t.TempDir()
	fragment := filepath.Join(dir, "demo.service")
	writeUnit(t, fragment, "[Service]\nExecStart=/opt/old/demo run --flag\n")
	override := filepath.Join(dir, "override.conf")
	writeUnit(t, override, "[Service]\nExecStart=\nExecStart=/opt/old/demo run --flag\n")
	f := newFake()
	f.props["ExecStart"] = "{ path=/opt/old/demo ; argv[]=/opt/old/demo run --flag ; }"
	f.props["FragmentPath"], f.props["DropInPaths"] = fragment, override
	// The override still wins after the reload.
	f.onReload = func(map[string]string) {}
	dropIns := filepath.Join(dir, "etc")
	u := testUnit(t, f, Options{RewritePath: true, DropInDir: dropIns})
	res, err := u.Reconcile(context.Background(), "demo", "/opt/new/demo")
	if err == nil || !res.Changed || !strings.Contains(err.Error(), "/opt/old/demo") {
		t.Fatalf("changed=%t err=%v; want the receipt and an error naming what still runs", res.Changed, err)
	}
	path := filepath.Join(dropIns, "demo.service.d", dropInName)
	if err := u.Restore(context.Background(), "demo", res); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("the drop-in was kept after Restore: %v", err)
	}
}
