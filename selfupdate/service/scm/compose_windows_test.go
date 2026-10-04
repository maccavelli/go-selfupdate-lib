//go:build windows

package scm

import (
	"context"
	"slices"
	"testing"
)

// realParse is the fake SCM with the real command-line split and quoting.
type realParse struct{ *fakeSCM }

func (realParse) decompose(line string) ([]string, error) { return sysManager{}.decompose(line) }
func (realParse) compose(args []string) string            { return sysManager{}.compose(args) }

// TestReconcileRealCommandLine: paths compare cleaned and
// case-insensitively, an unquoted path with spaces is read as the SCM
// reads it, and a rewrite quotes the program and keeps every argument
// (0011-MADR §5).
func TestReconcileRealCommandLine(t *testing.T) {
	ctx := context.Background()
	const exe = `C:\Program Files\Demo\demo.exe`
	for _, line := range []string{
		`"C:\Program Files\Demo\demo.exe" run`,
		`"c:\program files\demo\.\DEMO.EXE"`,
		`C:\Program Files\Demo\demo.exe run --flag`,
		`C:\Program Files\Demo\demo.exe`,
	} {
		f := newFake()
		f.add("demo", 1).cfg.BinaryPathName = line
		s, _ := testService(t, f, Options{})
		s.m = realParse{f}
		if res, err := s.Reconcile(ctx, "demo", exe); err != nil || res.Changed {
			t.Errorf("%s: %+v, %v", line, res, err)
		}
	}
	f := newFake()
	svc := f.add("demo", 1)
	svc.cfg.BinaryPathName = `"C:\Old\demo.exe" run "a b" "x\"y"`
	s, _ := testService(t, f, Options{RewritePath: true})
	s.m = realParse{f}
	if _, err := s.Reconcile(ctx, "demo", exe); err != nil {
		t.Fatal(err)
	}
	args, err := sysManager{}.decompose(svc.cfg.BinaryPathName)
	if err != nil || !slices.Equal(args, []string{exe, "run", "a b", `x"y`}) {
		t.Fatalf("rewrote %s, which splits as %q, %v", svc.cfg.BinaryPathName, args, err)
	}
	if svc.cfg.BinaryPathName[0] != '"' {
		t.Fatalf("the program is not quoted: %s", svc.cfg.BinaryPathName)
	}
}
