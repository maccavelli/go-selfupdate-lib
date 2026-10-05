//go:build unix

package systemd

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestReconcileThroughSymlink: a unit that names the binary through a
// symlinked directory already runs it (0011-MADR amendment A5).
func TestReconcileThroughSymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	if err := os.Mkdir(real, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real, "demo"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	via := filepath.Join(dir, "link", "demo")
	f := newFake()
	f.props["ExecStart"] = "{ path=" + via + " ; argv[]=" + via + " serve ; ignore_errors=no ; start_time=[n/a] }"
	u := testUnit(t, f, Options{})
	res, err := u.Reconcile(context.Background(), "demo", filepath.Join(real, "demo"))
	if err != nil || res.Changed {
		t.Fatalf("res %+v, err %v", res, err)
	}
}
