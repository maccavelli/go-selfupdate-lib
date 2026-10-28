package selfupdate

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestResolveTargetHomeRoot(t *testing.T) {
	t.Parallel()
	home, exe := tempTarget(t)
	got, err := homeFS(home).resolveTarget(TargetPolicy{ExecutablePath: exe})
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(exe)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != want || got.Base != "demo" || got.Dir != filepath.Dir(want) {
		t.Fatalf("%+v want %s under %s (home %s)", got, want, filepath.Dir(want), home)
	}
}

func TestResolveTargetRejectsSymlink(t *testing.T) {
	t.Parallel()
	home, exe := tempTarget(t)
	link := filepath.Join(home, "alias")
	if err := os.Symlink(exe, link); err != nil {
		t.Fatal(err)
	}
	if _, err := homeFS(home).resolveTarget(TargetPolicy{ExecutablePath: link}); err == nil {
		t.Fatal("accepted symlink invocation")
	}
}

func TestResolveTargetRejectsOutsideHome(t *testing.T) {
	t.Parallel()
	home, _ := tempTarget(t)
	outside := filepath.Join(t.TempDir(), "other")
	if err := os.WriteFile(outside, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := homeFS(home).resolveTarget(TargetPolicy{ExecutablePath: outside}); err == nil {
		t.Fatal("accepted target outside home")
	}
}

func TestResolveTargetExtraRoot(t *testing.T) {
	t.Parallel()
	home, _ := tempTarget(t)
	extra := t.TempDir()
	exe := filepath.Join(extra, "demo")
	if err := os.WriteFile(exe, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := homeFS(home).resolveTarget(TargetPolicy{ExecutablePath: exe, AllowedRoots: []string{extra}})
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(exe)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != want {
		t.Fatalf("%s", got.Path)
	}
}

func TestCanonicalizeRootRejectsFilesystemRoot(t *testing.T) {
	t.Parallel()
	root := "/"
	if runtime.GOOS == "windows" {
		// The root of the drive the test's temporary directory is on. Not
		// %SystemDrive%: an environment read is a gosec G703 taint source,
		// which would surface at canonicalizeRoot's os.Lstat
		// (0006-MADR-adopt-golangci-lint-v2-14.md).
		root = filepath.VolumeName(t.TempDir()) + `\`
	}
	if _, err := canonicalizeRoot(root); err == nil {
		t.Fatal("accepted filesystem root")
	}
}

func TestCanonicalizeRootRejectsRelative(t *testing.T) {
	t.Parallel()
	if _, err := canonicalizeRoot("rel"); err == nil {
		t.Fatal("accepted relative root")
	}
}

// tempTarget is a temporary directory holding a target, demo, with
// "old-bytes". targetPolicy(exe) allows it; it touches no package state
// (docs/decisions/0038-MADR-pinned-plans-lifecycle-conformance-and-bubble-tests.md
// D3-A).
func tempTarget(t *testing.T) (dir, exe string) {
	t.Helper()
	dir = t.TempDir()
	exe = filepath.Join(dir, "demo")
	if err := os.WriteFile(exe, []byte("old-bytes"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir, exe
}

// targetPolicy is the policy for exe, with its directory an allowed root.
func targetPolicy(exe string) TargetPolicy {
	return TargetPolicy{ExecutablePath: exe, AllowedRoots: []string{filepath.Dir(exe)}}
}

// homeFS is the real effects with home as the home directory, the
// default allowed root.
func homeFS(home string) *fsOps {
	fx := realFS()
	fx.userHomeDir = func() (string, error) { return home, nil }
	return fx
}

// TestResolveTargetDefaultExecutable: with no ExecutablePath the target is
// the running executable; a failure to find it, or an empty path, is an
// error (docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md
// B10).
func TestResolveTargetDefaultExecutable(t *testing.T) {
	t.Parallel()
	home, exe := tempTarget(t)
	fx := homeFS(home)
	fx.osExecutable = func() (string, error) { return exe, nil }
	got, err := fx.resolveTarget(TargetPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(exe)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != want {
		t.Fatalf("target %q, want the running executable %q", got.Path, want)
	}
	fx.osExecutable = func() (string, error) { return "", errors.New("injected: no executable") }
	if _, err := fx.resolveTarget(TargetPolicy{}); err == nil || !strings.Contains(err.Error(), "locate executable") {
		t.Fatalf("err = %v, want the failure to locate it", err)
	}
	fx.osExecutable = func() (string, error) { return "", nil }
	if _, err := fx.resolveTarget(TargetPolicy{}); err == nil || !strings.Contains(err.Error(), "executable path is empty") {
		t.Fatalf("err = %v, want an empty path refused", err)
	}
}

// TestRawExecutablePathRelative: a relative ExecutablePath is made absolute
// against the working directory (0015-MADR B10).
func TestRawExecutablePathRelative(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	got, err := realFS().rawExecutablePath("demo")
	if err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(cwd, "demo"); got != want {
		t.Fatalf("raw path %q, want %q", got, want)
	}
}
