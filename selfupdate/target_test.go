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
	home, exe := withTempHome(t)
	got, err := resolveTarget(TargetPolicy{ExecutablePath: exe})
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
	home, exe := withTempHome(t)
	link := filepath.Join(home, "alias")
	if err := os.Symlink(exe, link); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveTarget(TargetPolicy{ExecutablePath: link}); err == nil {
		t.Fatal("accepted symlink invocation")
	}
}

func TestResolveTargetRejectsOutsideHome(t *testing.T) {
	_, _ = withTempHome(t)
	outside := filepath.Join(t.TempDir(), "other")
	if err := os.WriteFile(outside, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveTarget(TargetPolicy{ExecutablePath: outside}); err == nil {
		t.Fatal("accepted target outside home")
	}
}

func TestResolveTargetExtraRoot(t *testing.T) {
	_, _ = withTempHome(t)
	extra := t.TempDir()
	exe := filepath.Join(extra, "demo")
	if err := os.WriteFile(exe, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := resolveTarget(TargetPolicy{ExecutablePath: exe, AllowedRoots: []string{extra}})
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
	if _, err := canonicalizeRoot("rel"); err == nil {
		t.Fatal("accepted relative root")
	}
}

func withTempHome(t *testing.T) (home, exe string) {
	t.Helper()
	home = t.TempDir()
	setSeam(t, &userHomeDir, func() (string, error) { return home, nil })
	exe = filepath.Join(home, "demo")
	if err := os.WriteFile(exe, []byte("old-bytes"), 0o755); err != nil {
		t.Fatal(err)
	}
	return home, exe
}

// TestResolveTargetDefaultExecutable: with no ExecutablePath the target is
// the running executable; a failure to find it, or an empty path, is an
// error (docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md
// B10).
func TestResolveTargetDefaultExecutable(t *testing.T) {
	_, exe := withTempHome(t)
	setSeam(t, &osExecutable, func() (string, error) { return exe, nil })
	got, err := resolveTarget(TargetPolicy{})
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
	setSeam(t, &osExecutable, func() (string, error) { return "", errors.New("injected: no executable") })
	if _, err := resolveTarget(TargetPolicy{}); err == nil || !strings.Contains(err.Error(), "locate executable") {
		t.Fatalf("err = %v, want the failure to locate it", err)
	}
	setSeam(t, &osExecutable, func() (string, error) { return "", nil })
	if _, err := resolveTarget(TargetPolicy{}); err == nil || !strings.Contains(err.Error(), "executable path is empty") {
		t.Fatalf("err = %v, want an empty path refused", err)
	}
}

// TestRawExecutablePathRelative: a relative ExecutablePath is made absolute
// against the working directory (0015-MADR B10).
func TestRawExecutablePathRelative(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	got, err := rawExecutablePath("demo")
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
