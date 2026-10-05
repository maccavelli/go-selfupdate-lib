//go:build unix

package service

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSameExecutableThroughSymlink: a path through a symlinked directory
// names the same binary (0011-MADR amendment A5).
func TestSameExecutableThroughSymlink(t *testing.T) {
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
	if !SameExecutable(filepath.Join(dir, "link", "demo"), filepath.Join(real, "demo")) {
		t.Fatal("the symlinked path did not match")
	}
}
