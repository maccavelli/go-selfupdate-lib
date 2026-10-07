package scm

import (
	"context"
	"io/fs"
	"os"
	"strings"
	"testing"
	"time"
)

// fakeInfo is a regular file for statFile.
type fakeInfo struct{ name string }

func (i fakeInfo) Name() string     { return i.name }
func (fakeInfo) Size() int64        { return 1 }
func (fakeInfo) Mode() fs.FileMode  { return 0o755 }
func (fakeInfo) ModTime() time.Time { return time.Time{} }
func (fakeInfo) IsDir() bool        { return false }
func (fakeInfo) Sys() any           { return nil }

// existing makes statFile report exactly the paths in files, compared
// case-insensitively, as on Windows.
func existing(t *testing.T, files ...string) {
	t.Helper()
	prev := statFile
	statFile = func(p string) (os.FileInfo, error) {
		for _, f := range files {
			if strings.EqualFold(f, p) {
				return fakeInfo{name: p}, nil
			}
		}
		return nil, os.ErrNotExist
	}
	t.Cleanup(func() { statFile = prev })
}

// TestReconcileUnquotedPathWithSpaces: an unquoted command line whose
// program path holds a space is read as the SCM reads it: the first prefix
// ending at a space, or the whole line, that names an existing file, with
// .exe appended to one that has no extension; failing that, the first
// ending in .exe. A rewrite keeps the arguments, and refuses a line two
// readings disagree on
// (docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md D5).
func TestReconcileUnquotedPathWithSpaces(t *testing.T) {
	ctx := context.Background()
	const exe = `C:\Program Files\New\demo.exe`
	const line = `C:\Program Files\Old\demo.exe run`
	existing(t)
	f := newFake()
	svc := f.add("demo", 1)
	svc.cfg.BinaryPathName = line
	s, _ := testService(t, f, Options{})
	_, err := s.Reconcile(ctx, "demo", exe)
	if err == nil || !strings.Contains(err.Error(), `runs C:\Program Files\Old\demo.exe,`) {
		t.Fatalf("err = %v; want it to name the old binary", err)
	}
	s.o.RewritePath = true
	if _, err := s.Reconcile(ctx, "demo", exe); err != nil {
		t.Fatal(err)
	}
	if want := `"C:\Program Files\New\demo.exe" run`; svc.cfg.BinaryPathName != want {
		t.Fatalf("rewrote to %s, want %s", svc.cfg.BinaryPathName, want)
	}

	// C:\Program.exe exists: the SCM would run it, though the line names
	// a .exe further on. A rewrite would guess, so it is refused.
	existing(t, `C:\Program.exe`)
	svc.cfg.BinaryPathName = line
	if _, err := s.Reconcile(ctx, "demo", exe); err == nil || !strings.Contains(err.Error(), "quote it") {
		t.Fatalf("err = %v; want an ambiguous line refused", err)
	}
	if svc.cfg.BinaryPathName != line {
		t.Fatalf("the ambiguous line was rewritten to %s", svc.cfg.BinaryPathName)
	}

	// Nothing exists and nothing ends in .exe: there is no telling.
	existing(t)
	svc.cfg.BinaryPathName = `C:\My Tools\demo run`
	if _, err := s.Reconcile(ctx, "demo", exe); err == nil || !strings.Contains(err.Error(), "quote it") {
		t.Fatalf("err = %v; want the line refused", err)
	}
}
