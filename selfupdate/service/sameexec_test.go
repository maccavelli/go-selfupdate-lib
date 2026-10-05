package service

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Tests for docs/decisions/0011-MADR-reference-service-lifecycles.md
// amendment A5: a binary is recognised by its text or by file identity.

func TestSameExecutableByText(t *testing.T) {
	// No file needs to exist for a textual match.
	if !SameExecutable("/opt/demo/./demo", "/opt/demo/demo") {
		t.Fatal("cleaned paths differ")
	}
	if SameExecutable("/opt/demo/demo", "/opt/other/demo") {
		t.Fatal("two missing paths matched")
	}
	upper := strings.ToUpper("/opt/demo/demo")
	if got, want := SameExecutable("/opt/demo/demo", upper), runtime.GOOS == "windows"; got != want {
		t.Fatalf("case differs: %t, want %t on %s", got, want, runtime.GOOS)
	}
}

func TestSameExecutableByIdentity(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	for _, p := range []string{a, b} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if SameExecutable(a, b) {
		t.Fatal("two files matched")
	}
	link := filepath.Join(dir, "hard")
	if err := os.Link(a, link); err != nil {
		t.Skipf("no hard link here: %v", err)
	}
	if !SameExecutable(a, link) {
		t.Fatal("a hard link to the file did not match")
	}
	if SameExecutable(a, filepath.Join(dir, "missing")) {
		t.Fatal("a missing path matched")
	}
}
