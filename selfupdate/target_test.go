package selfupdate

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/internal/testproc"
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
	if _, err := homeFS(home).resolveTarget(TargetPolicy{ExecutablePath: link}); !errors.Is(err, ErrSymlinkInvocation) {
		t.Fatalf("err = %v, want ErrSymlinkInvocation", err)
	}
}

// TestResolveTargetInvokedSymlink: with no ExecutablePath, os.Executable's
// result is the binary itself, as on Linux, and os.Args[0] decides. A link
// to the running binary, by absolute, relative or bare name, is refused;
// anything else os.Args[0] names is ignored
// (docs/decisions/0041-MADR-refuse-symlink-invocation-on-every-platform.md
// D1-A).
func TestResolveTargetInvokedSymlink(t *testing.T) {
	t.Parallel()
	home, exe := tempTarget(t)
	link := filepath.Join(home, "alias")
	symlinkOrSkip(t, exe, link)
	other := filepath.Join(home, "other")
	if err := os.WriteFile(other, []byte("other-bytes"), 0o755); err != nil {
		t.Fatal(err)
	}
	otherLink := filepath.Join(home, "other-alias")
	symlinkOrSkip(t, other, otherLink)
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relLink, err := filepath.Rel(wd, link)
	if err != nil {
		t.Skipf("no relative path from %s to the link: %v", wd, err)
	}
	want, err := filepath.EvalSymlinks(exe)
	if err != nil {
		t.Fatal(err)
	}
	found := func(string) (string, error) { return link, nil }
	unused := func(name string) (string, error) {
		t.Errorf("lookPath(%q) called", name)
		return "", exec.ErrNotFound
	}
	notFound := func(string) (string, error) { return "", exec.ErrNotFound }
	dot := func(string) (string, error) { return "", exec.ErrDot }
	for _, c := range []struct {
		name       string
		arg0       string
		lookPath   func(string) (string, error)
		executable string
		refused    bool
	}{
		{"absolute link", link, unused, "", true},
		{"relative link", relLink, unused, "", true},
		{"bare name", "alias", found, "", true},
		{"bare name not found", "alias", notFound, "", false},
		{"bare name ErrDot", "alias", dot, "", false},
		{"link to another file", otherLink, unused, "", false},
		{"missing path", filepath.Join(home, "missing"), unused, "", false},
		{"empty", "", unused, "", false},
		{"real path", exe, unused, "", false},
		{"override", link, unused, exe, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			fx := homeFS(home)
			fx.osExecutable = func() (string, error) { return exe, nil }
			fx.osArgs0 = func() string { return c.arg0 }
			fx.lookPath = c.lookPath
			got, err := fx.resolveTarget(TargetPolicy{ExecutablePath: c.executable})
			if c.refused {
				if !errors.Is(err, ErrSymlinkInvocation) {
					t.Fatalf("err = %v, want ErrSymlinkInvocation", err)
				}
				if !strings.Contains(err.Error(), want) {
					t.Errorf("err = %v, want it to name %s", err, want)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Path != want {
				t.Fatalf("Path = %s, want %s", got.Path, want)
			}
		})
	}
}

// TestResolveTargetRealExecutableThroughSymlink runs a copy of this test
// binary through a symlink, and it resolves its target with the real
// os.Executable and os.Args[0]. On Linux os.Executable has resolved the
// link, so only the os.Args[0] check refuses it; the test written with
// ExecutablePath could not see that (0041-MADR F4, D1-A).
func TestResolveTargetRealExecutableThroughSymlink(t *testing.T) {
	t.Parallel()
	src, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	name := "demo"
	if runtime.GOOS == goosWindows {
		name += ".exe"
	}
	realDir, linkDir := filepath.Join(root, "real"), filepath.Join(root, "link")
	for _, d := range []string{realDir, linkDir} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	exe, link := filepath.Join(realDir, name), filepath.Join(linkDir, name)
	in, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, in, 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, exe, link)
	env := append(os.Environ(),
		"SELFUPDATE_TEST_RESOLVE_TARGET=1",
		"SELFUPDATE_TEST_ROOT="+root,
		"PATH="+linkDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, c := range []struct {
		name, path, arg0, dir, want string
	}{
		{"real path", exe, exe, "", "ok"},
		{"absolute link", link, link, "", "symlink"},
		{"relative link", link, "." + string(os.PathSeparator) + name, linkDir, "symlink"},
		{"bare name on PATH", link, name, "", "symlink"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(context.Background(), testproc.Budget(t))
			defer cancel()
			cmd := exec.CommandContext(ctx, c.path)
			cmd.Args = []string{c.arg0}
			cmd.Dir = c.dir
			cmd.Env = env
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("helper: %v: %s", err, out)
			}
			if got := strings.TrimSpace(string(out)); got != c.want {
				t.Fatalf("helper printed %q, want %q", got, c.want)
			}
		})
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
