//go:build windows

package scm

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

// TestReconcileShortName: a service registered with the binary's 8.3 short
// path already runs it, the case CI's Windows runner found (0011-MADR
// amendment A5).
func TestReconcileShortName(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a long directory name")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	long := filepath.Join(dir, "demo.exe")
	if err := os.WriteFile(long, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n, err := windows.GetShortPathName(p, &buf[0], uint32(len(buf)))
	if err != nil {
		t.Fatal(err)
	}
	if windows.UTF16ToString(buf[:n]) == dir {
		t.Skipf("this volume gives %s no 8.3 name", dir)
	}
	short := filepath.Join(windows.UTF16ToString(buf[:n]), "demo.exe")
	f := newFake()
	f.add("demo", 1).cfg.BinaryPathName = `"` + short + `" run`
	s, _ := testService(t, f, Options{})
	s.m = realParse{f}
	if res, err := s.Reconcile(context.Background(), "demo", long); err != nil || res.Changed {
		t.Fatalf("%s for %s: %+v, %v", short, long, res, err)
	}
}
