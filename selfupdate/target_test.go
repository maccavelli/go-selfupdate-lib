package selfupdate

import (
	"context"
	"encoding/json"
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

// TestResolveTargetRejectsSymlink: an ExecutablePath that is a link is
// refused as the policy's, stating the fact with no instruction
// (0041-MADR Amendment A1 D3-A, D4-A).
func TestResolveTargetRejectsSymlink(t *testing.T) {
	t.Parallel()
	home, exe := tempTarget(t)
	link := filepath.Join(home, "alias")
	if err := os.Symlink(exe, link); err != nil {
		t.Fatal(err)
	}
	_, err := homeFS(home).resolveTarget(TargetPolicy{ExecutablePath: link})
	wantSymlinkError(t, err, link, realPath(t, exe), SymlinkSourcePolicy)
	want := "selfupdate: refusing to update through a symlink: the configured executable path " +
		link + " links to " + realPath(t, exe)
	if err.Error() != want {
		t.Fatalf("err = %q\nwant  %q", err, want)
	}
}

// TestResolveTargetExecutableSymlink: os.Executable returning the link, as
// it does on macOS and Windows, is refused as the executable's, with the
// instruction to start the target (0041-MADR Amendment A1 D3-A, D4-A).
func TestResolveTargetExecutableSymlink(t *testing.T) {
	t.Parallel()
	home, exe := tempTarget(t)
	link := filepath.Join(home, "alias")
	symlinkOrSkip(t, exe, link)
	fx := homeFS(home)
	fx.osExecutable = func() (string, error) { return link, nil }
	fx.osArgs0 = func() string { return "" }
	_, err := fx.resolveTarget(TargetPolicy{})
	wantSymlinkError(t, err, link, realPath(t, exe), SymlinkSourceExecutable)
	if got, want := err.Error(), startText(link, realPath(t, exe)); got != want {
		t.Fatalf("err = %q\nwant  %q", got, want)
	}
}

// TestSymlinkInvocationErrorText: a link that does not resolve is named
// alone; no text names a Go identifier; a nil error prints the sentinel and
// unwraps to it; and the JSON form names the source (0041-MADR Amendment A1
// D3-A, D4-A).
func TestSymlinkInvocationErrorText(t *testing.T) {
	t.Parallel()
	home, _ := tempTarget(t)
	dangling := filepath.Join(home, "dangling")
	symlinkOrSkip(t, filepath.Join(home, "missing"), dangling)
	_, err := homeFS(home).resolveTarget(TargetPolicy{ExecutablePath: dangling})
	wantSymlinkError(t, err, dangling, "", SymlinkSourcePolicy)
	if got, want := err.Error(), "selfupdate: refusing to update through a symlink: "+dangling+" is a symlink"; got != want {
		t.Fatalf("err = %q, want %q", got, want)
	}
	fx := homeFS(home)
	fx.osExecutable = func() (string, error) { return dangling, nil }
	_, err = fx.resolveTarget(TargetPolicy{})
	wantSymlinkError(t, err, dangling, "", SymlinkSourceExecutable)

	for _, e := range []*SymlinkInvocationError{
		{Link: "/a/link", Target: "/b/real", Source: SymlinkSourceExecutable},
		{Link: "/a/link", Target: "/b/real", Source: SymlinkSourceInvocation},
		{Link: "/a/link", Target: "/b/real", Source: SymlinkSourcePolicy},
		{Link: "/a/link", Source: SymlinkSourceInvocation},
	} {
		if strings.Contains(e.Error(), "TargetPolicy") || strings.Contains(e.Error(), "ExecutablePath") {
			t.Errorf("%+v: %q names a Go identifier", *e, e.Error())
		}
	}
	var nilErr *SymlinkInvocationError
	if nilErr.Error() != ErrSymlinkInvocation.Error() || !errors.Is(nilErr, ErrSymlinkInvocation) {
		t.Fatalf("nil: %q, Is = %t", nilErr.Error(), errors.Is(nilErr, ErrSymlinkInvocation))
	}
	for e, want := range map[SymlinkInvocationError]string{
		{Link: "/a/link", Target: "/b/real", Source: SymlinkSourcePolicy}: `{"link":"/a/link","target":"/b/real","source":"policy"}`,
		{Link: "/a/link", Source: SymlinkSourceExecutable}:                `{"link":"/a/link","source":"executable"}`,
	} {
		b, err := json.Marshal(e)
		if err != nil || string(b) != want {
			t.Errorf("json = %s, %v; want %s", b, err, want)
		}
	}
}

// wantSymlinkError fails t unless err is a *SymlinkInvocationError with
// these fields, matching ErrSymlinkInvocation.
func wantSymlinkError(t *testing.T, err error, link, target string, source SymlinkSource) {
	t.Helper()
	var got *SymlinkInvocationError
	if !errors.As(err, &got) || !errors.Is(err, ErrSymlinkInvocation) {
		t.Fatalf("err = %v (%T), want a *SymlinkInvocationError", err, err)
	}
	if got.Link != link || got.Target != target || got.Source != source {
		t.Fatalf("got %+v, want {Link:%s Target:%s Source:%s}", *got, link, target, source)
	}
}

// startText is the refusal of a program started through link.
func startText(link, target string) string {
	return "selfupdate: refusing to update through a symlink: " + link + " links to " + target +
		"; start " + target + " itself to update it"
}

// realPath is path with every link resolved, as resolveTarget names it.
func realPath(t *testing.T, path string) string {
	t.Helper()
	p, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return p
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
	if err := testproc.WriteExecutable(other, []byte("other-bytes"), 0o755); err != nil {
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
				wantSymlinkError(t, err, link, want, SymlinkSourceInvocation)
				if got := err.Error(); got != startText(link, want) {
					t.Fatalf("err = %q\nwant  %q", got, startText(link, want))
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
// binary through a symlink, and it calls ResolveTarget, CleanupPending and
// KeptBackups with the real os.Executable and os.Args[0]. On Linux
// os.Executable has resolved the link, so only the os.Args[0] check
// refuses it; the test written with ExecutablePath could not see that
// (0041-MADR F4, D1-A). Through the link, ResolveTarget is refused,
// CleanupPending writes nothing, and KeptBackups lists the target's kept
// backup (Amendment A1 D3-A, D5-A).
func TestResolveTargetRealExecutableThroughSymlink(t *testing.T) {
	t.Parallel()
	src, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	in, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	linkSource := SymlinkSourceExecutable
	if runtime.GOOS == "linux" {
		linkSource = SymlinkSourceInvocation
	}
	refused := "symlink " + string(linkSource)
	for _, op := range []struct{ name, link, real string }{
		{"resolve", refused, "ok"},
		{"cleanup", "ok", "ok"},
		{"kept", "ok 1", "ok 1"},
	} {
		t.Run(op.name, func(t *testing.T) {
			t.Parallel()
			c := symlinkedCopy(t, in)
			if err := testproc.WriteExecutable(filepath.Join(c.realDir, "."+c.name+".selfupdate-kept-1"), []byte("kept"), 0o755); err != nil {
				t.Fatal(err)
			}
			before := dirNames(t, c.realDir)
			// The link starts run first: a real-path cleanup leaves a lock
			// file, which would hide one written through the link.
			for _, s := range []struct{ name, path, arg0, dir, want string }{
				{"absolute link", c.link, c.link, "", op.link},
				{"relative link", c.link, "." + string(os.PathSeparator) + c.name, c.linkDir, op.link},
				{"bare name on PATH", c.link, c.name, "", op.link},
				{"real path", c.exe, c.exe, "", op.real},
			} {
				got := runResolveHelper(t, c, op.name, s.path, s.arg0, s.dir)
				if got != s.want {
					t.Fatalf("%s: helper printed %q, want %q", s.name, got, s.want)
				}
				if s.path == c.link {
					if after := dirNames(t, c.realDir); after != before {
						t.Fatalf("%s: the target's directory changed through the link:\nbefore %s\nafter  %s", s.name, before, after)
					}
				}
			}
		})
	}
}

// symlinked is a copy of a binary in <root>/real, and a symlink to it in
// <root>/link.
type symlinked struct {
	root, realDir, linkDir, exe, link, name string
}

// symlinkedCopy writes in as an executable under a new temporary root and
// links to it, skipping where a symlink cannot be made.
func symlinkedCopy(t *testing.T, in []byte) symlinked {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c := symlinked{root: root, realDir: filepath.Join(root, "real"), linkDir: filepath.Join(root, "link"), name: "demo"}
	if runtime.GOOS == goosWindows {
		c.name += ".exe"
	}
	for _, d := range []string{c.realDir, c.linkDir} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	c.exe, c.link = filepath.Join(c.realDir, c.name), filepath.Join(c.linkDir, c.name)
	if err := testproc.WriteExecutable(c.exe, in, 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, c.exe, c.link)
	return c
}

// runResolveHelper starts path with arg0 as os.Args[0], in dir, to run op
// in resolveTargetHelper, and returns what it printed.
func runResolveHelper(t *testing.T, c symlinked, op, path, arg0, dir string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testproc.Budget(t))
	defer cancel()
	cmd := exec.CommandContext(ctx, path)
	cmd.Args = []string{arg0}
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"SELFUPDATE_TEST_RESOLVE_TARGET=1",
		"SELFUPDATE_TEST_OP="+op,
		"SELFUPDATE_TEST_ROOT="+c.root,
		"PATH="+c.linkDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("helper: %v: %s", err, out)
	}
	return strings.TrimSpace(string(out))
}

// dirNames is dir's entry names, sorted and joined by spaces.
func dirNames(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return strings.Join(names, " ")
}

func TestResolveTargetRejectsOutsideHome(t *testing.T) {
	t.Parallel()
	home, _ := tempTarget(t)
	outside := filepath.Join(t.TempDir(), "other")
	if err := testproc.WriteExecutable(outside, []byte("x"), 0o755); err != nil {
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
	if err := testproc.WriteExecutable(exe, []byte("x"), 0o755); err != nil {
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
	if err := testproc.WriteExecutable(exe, []byte("old-bytes"), 0o755); err != nil {
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
