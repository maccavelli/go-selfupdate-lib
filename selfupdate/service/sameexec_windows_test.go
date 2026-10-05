//go:build windows

package service

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

// shortPath returns path's 8.3 form, skipping the test when the volume
// gives it none.
func shortPath(t *testing.T, path string) string {
	t.Helper()
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n, err := windows.GetShortPathName(p, &buf[0], uint32(len(buf)))
	if err != nil {
		t.Fatal(err)
	}
	short := windows.UTF16ToString(buf[:n])
	if short == path {
		t.Skipf("this volume gives %s no 8.3 name", path)
	}
	return short
}

// TestSameExecutableShortName: an 8.3 short name names the same binary,
// the case CI's Windows runner found (0011-MADR amendment A5).
func TestSameExecutableShortName(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a long directory name")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	long := filepath.Join(dir, "demo.exe")
	if err := os.WriteFile(long, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	short := filepath.Join(shortPath(t, dir), "demo.exe")
	if !SameExecutable(short, long) {
		t.Fatalf("%s and %s did not match", short, long)
	}
}
