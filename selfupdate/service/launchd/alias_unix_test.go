//go:build unix

package launchd

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestReconcileThroughSymlink: a plist that names the binary through a
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
	f := newFake()
	f.keys["ProgramArguments.0"] = filepath.Join(dir, "link", "demo")
	j := testJob(t, f, Options{})
	res, err := j.Reconcile(context.Background(), "demo", filepath.Join(real, "demo"))
	if err != nil || res.Changed {
		t.Fatalf("res %+v, err %v", res, err)
	}
}
