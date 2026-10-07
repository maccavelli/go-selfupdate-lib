//go:build unix

package main

import (
	"bytes"
	"context"
	"errors"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
)

// Tests for docs/decisions/0014-PLAN-shared-installer-templates.md I3:
// install.sh's behaviour (0014-MADR §4), run as `curl … | sh -s -- …` is,
// under every shell the host has.

// shell runs a script read from its standard input.
type shell struct {
	name string
	argv []string
	// stubbable is whether PATH decides which uname runs. A BusyBox built
	// with FEATURE_SH_STANDALONE, as Ubuntu's busybox-static is, runs its
	// own applets first.
	stubbable bool
}

// shells are sh, dash, bash and BusyBox's ash, each once: on Debian sh is
// dash, and on Alpine it is BusyBox. Each is named for the binary it is.
func shells(t *testing.T) []shell {
	t.Helper()
	seen := map[string]bool{}    // real paths
	honours := map[string]bool{} // binary names: whether PATH is honoured
	var out []shell
	for _, c := range [][]string{{"sh"}, {"dash"}, {"bash"}, {"busybox", "sh"}} {
		path, err := exec.LookPath(c[0])
		if err != nil {
			continue
		}
		real, err := filepath.EvalSymlinks(path)
		if err != nil {
			t.Fatal(err)
		}
		if seen[real] {
			continue
		}
		seen[real] = true
		name := strings.Join(c, " ")
		if base := filepath.Base(real); base != c[0] {
			name += " (" + base + ")"
		}
		sh := shell{name: name, argv: append([]string{path}, c[1:]...)}
		sh.stubbable = runsStub(t, sh)
		out = append(out, sh)
		honours[filepath.Base(real)] = sh.stubbable
	}
	if len(out) == 0 {
		t.Fatal("no POSIX shell found")
	}
	for _, want := range strings.FieldsFunc(os.Getenv(requireShellsEnv), func(r rune) bool { return r == ',' }) {
		if !honours[want] {
			t.Fatalf("%s: no %s that honours PATH among %v", requireShellsEnv, want, out)
		}
	}
	return out
}

// runsStub is whether sh runs a uname stub put ahead of PATH.
func runsStub(t *testing.T, sh shell) bool {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "uname"), []byte("#!/bin/sh\necho stub\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(sh.argv[0], append(slices.Clone(sh.argv[1:]), "-c", "uname")...)
	cmd.Env = []string{"PATH=" + dir + string(os.PathListSeparator) + os.Getenv("PATH")}
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%s -c uname: %v", sh.name, err)
	}
	return strings.TrimSpace(string(out)) == "stub"
}

// needStubs skips a case that replaces commands through PATH under a shell
// that runs its own first (0014-PLAN deviation D3). Alpine's BusyBox,
// built without FEATURE_SH_STANDALONE, runs these cases in the container
// job.
func (c *shCase) needStubs() {
	c.t.Helper()
	if !c.sh.stubbable {
		c.t.Skipf("%s runs its own uname, id, wget and sha256sum ahead of PATH, so no stub can stand in for them", c.sh.name)
	}
}

// shCase is one run's world: a home holding the default install
// directory, a release server, and a directory of stubs ahead of PATH.
type shCase struct {
	t      *testing.T
	sh     shell
	r      installReleases
	srv    *releaseServer
	script []byte
	env    []string
	path   string // replaces PATH when set
	home   string
	dir    string // ~/.local/bin
	stubs  string
	log    string // HOOK_LOG, where stand-ins record their runs
}

func newShCase(t *testing.T, sh shell, r installReleases) *shCase {
	t.Helper()
	home := t.TempDir()
	c := &shCase{t: t, sh: sh, r: r, srv: newReleaseServer(t, r), script: r.file(t, "raw", "install.sh"),
		home: home, dir: filepath.Join(home, ".local", "bin"), stubs: filepath.Join(home, "stubs"), log: filepath.Join(home, "hooks.log")}
	if err := os.Mkdir(c.stubs, 0o755); err != nil {
		t.Fatal(err)
	}
	return c
}

// stub puts a script named name ahead of PATH.
func (c *shCase) stub(name, body string) {
	c.t.Helper()
	if err := os.WriteFile(filepath.Join(c.stubs, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		c.t.Fatal(err)
	}
}

// standIn is a program for hooks and identity checks: it reports version
// as a release, logs any other run as "<role> <args>", and exits with
// $<ROLE>_EXIT.
func standIn(role, version string) []byte {
	return []byte("#!/bin/sh\nif [ \"$1\" = version ]; then echo '" + version + " (release)'; exit 0; fi\n" +
		"echo \"" + role + " $*\" >>\"$HOOK_LOG\"\nexit \"${" + strings.ToUpper(role) + "_EXIT:-0}\"\n")
}

// place installs data as name in the install directory, as an earlier
// install would have.
func (c *shCase) place(name string, data []byte) {
	c.t.Helper()
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		c.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(c.dir, name), data, 0o755); err != nil {
		c.t.Fatal(err)
	}
}

func (c *shCase) command(ctx context.Context, args ...string) *exec.Cmd {
	argv := append(append(slices.Clone(c.sh.argv[1:]), "-s", "--"), args...)
	cmd := exec.CommandContext(ctx, c.sh.argv[0], argv...)
	cmd.Stdin = bytes.NewReader(c.script)
	path := c.path
	if path == "" {
		path = c.stubs + string(os.PathListSeparator) + os.Getenv("PATH")
	}
	cmd.Env = append([]string{"PATH=" + path, "HOME=" + c.home, "HOOK_LOG=" + c.log,
		"SELFUPDATE_INSTALL_BASE_URL=" + c.srv.URL}, c.env...)
	// Ctrl-C reaches the whole job, curl included.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	return cmd
}

func (c *shCase) run(args ...string) runResult {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	out, err := c.command(ctx, args...).CombinedOutput()
	return runResult{exitCode(c.t, err), string(out)}
}

// expect checks the exit code, and that the output has each of wants.
func (c *shCase) expect(r runResult, code int, wants ...string) {
	c.t.Helper()
	if r.code != code {
		c.t.Fatalf("exit %d, want %d; output:\n%s", r.code, code, r.out)
	}
	for _, w := range wants {
		if !strings.Contains(r.out, w) {
			c.t.Fatalf("the output lacks %q:\n%s", w, r.out)
		}
	}
}

func (c *shCase) hookLog() string {
	c.t.Helper()
	data, err := os.ReadFile(c.log)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		c.t.Fatal(err)
	}
	return string(data)
}

func (c *shCase) asset(kind, product string) string {
	return c.r.assetName(c.t, kind, product, host)
}

func (c *shCase) installed(name string) string {
	return "installed " + filepath.Join(c.dir, name) + " (" + fixtureTag + ")"
}

// unchanged runs the installer expecting exit code and wants, with an
// earlier relay in place, and checks that it is all that is there after.
func unchanged(c *shCase, edit func(), code int, wants ...string) {
	c.t.Helper()
	old := standIn("old", fixtureTag)
	c.place("relay", old)
	edit()
	c.expect(c.run(), code, wants...)
	expectFiles(c.t, c.dir, map[string][]byte{"relay": old})
}

var shCases = []struct {
	name string
	run  func(c *shCase)
}{
	{"installs every product", func(c *shCase) {
		c.expect(c.run(), 0, c.installed("relay"), c.installed("relayctl"), "add "+c.dir+" to your PATH")
		expectFiles(c.t, c.dir, map[string][]byte{"relay": c.r.program(c.t, "relay"), "relayctl": c.r.program(c.t, "relayctl")})
	}},
	{"--product installs a subset", func(c *shCase) {
		c.expect(c.run("--product", "relayctl"), 0, c.installed("relayctl"))
		expectFiles(c.t, c.dir, map[string][]byte{"relayctl": c.r.program(c.t, "relayctl")})
		c.expect(c.run("--product", "nope"), 1, "nope is not a product of this release (relay relayctl)")
	}},
	{"a reinstall keeps the previous copy", func(c *shCase) {
		old := standIn("old", fixtureTag)
		c.place("relay", old)
		c.expect(c.run(), 0, c.installed("relay"))
		expectFiles(c.t, c.dir, map[string][]byte{"relay": c.r.program(c.t, "relay"), "relay.prev": old, "relayctl": c.r.program(c.t, "relayctl")})
	}},
	{"--dir and RELAY_INSTALL_DIR", func(c *shCase) {
		a, b := filepath.Join(c.home, "a"), filepath.Join(c.home, "b")
		c.expect(c.run("--dir", a, "--product", "relayctl"), 0, "installed "+filepath.Join(a, "relayctl"))
		expectFiles(c.t, a, map[string][]byte{"relayctl": c.r.program(c.t, "relayctl")})
		c.env = append(c.env, "RELAY_INSTALL_DIR="+b)
		c.expect(c.run("--product", "relayctl"), 0, "installed "+filepath.Join(b, "relayctl"))
		expectFiles(c.t, b, map[string][]byte{"relayctl": c.r.program(c.t, "relayctl")})
		expectFiles(c.t, c.dir, nil)
	}},
	{"--version and RELAY_VERSION install another release", func(c *shCase) {
		// An installer from v1.3.0, which this server does not have.
		c.script = c.r.render(c.t, "raw", "install.sh", "v1.3.0")
		c.expect(c.run(), 2, "download of SHA256SUMS for v1.3.0 failed")
		c.expect(c.run("--version", "1.2.3", "--product", "relay"), 0, c.installed("relay"))
		c.expect(c.run("--version", "v1.2.3", "--product", "relay"), 0, c.installed("relay"))
		c.env = append(c.env, "RELAY_VERSION=1.2.3")
		c.expect(c.run("--product", "relay"), 0, c.installed("relay"))
	}},
	{"--version outside the tag rule or the channels", func(c *shCase) {
		c.expect(c.run("--version", "v1.2.4-beta.1"), 1, "v1.2.4-beta.1 is a prerelease on channel beta, which this release does not publish")
		c.expect(c.run("--version", "latest"), 1, "vlatest is not a release tag")
		c.expect(c.run("--version", "v1.2.4-rc.1"), 2, "download of SHA256SUMS for v1.2.4-rc.1 failed")
		expectFiles(c.t, c.dir, nil)
	}},
	{"--version of a release with other asset names", func(c *shCase) {
		// An archive release's installer, asked for a raw release.
		c.script = c.r.render(c.t, "tgz", "install.sh", "v1.3.0")
		c.srv.route("fixture/relay-tgz", fixtureTag, c.r.staged("raw"))
		c.expect(c.run("--version", fixtureTag), 1, "release v1.2.3 has no "+c.asset("tgz", "relay")+"; its own installer is "+
			c.srv.URL+"/fixture/relay-tgz/releases/download/v1.2.3/install.sh")
		if got := c.srv.got(); !slices.Equal(got, []string{"/fixture/relay-tgz/releases/download/v1.2.3/SHA256SUMS"}) {
			c.t.Fatalf("requested %v, want only SHA256SUMS", got)
		}
		expectFiles(c.t, c.dir, nil)
	}},
	{"an altered asset", func(c *shCase) {
		unchanged(c, func() {
			c.srv.replace(rawRepo, fixtureTag, c.asset("raw", "relay"), append(c.r.program(c.t, "relay"), 'x'))
		}, 2, c.asset("raw", "relay")+": SHA-256 ", "does not match SHA256SUMS")
	}},
	{"SHA256SUMS with CR line endings", func(c *shCase) {
		unchanged(c, func() {
			c.srv.editSums(c.t, rawRepo, fixtureTag, c.asset("raw", "relay"), func(lines []string, _ int) []string {
				for i := range lines {
					lines[i] += "\r"
				}
				return lines
			})
		}, 2, "SHA256SUMS has no entry for "+c.asset("raw", "relay"))
	}},
	{"SHA256SUMS with a 63-character hash", func(c *shCase) {
		unchanged(c, func() {
			c.srv.editSums(c.t, rawRepo, fixtureTag, c.asset("raw", "relay"), func(lines []string, i int) []string {
				lines[i] = lines[i][1:]
				return lines
			})
		}, 2, "SHA256SUMS has no entry for "+c.asset("raw", "relay"))
	}},
	{"SHA256SUMS with an upper-case hash", func(c *shCase) {
		unchanged(c, func() {
			c.srv.editSums(c.t, rawRepo, fixtureTag, c.asset("raw", "relay"), func(lines []string, i int) []string {
				lines[i] = strings.ToUpper(lines[i][:64]) + lines[i][64:]
				return lines
			})
		}, 2, "SHA256SUMS has a malformed or duplicate entry for "+c.asset("raw", "relay"))
	}},
	{"SHA256SUMS listing the asset twice", func(c *shCase) {
		unchanged(c, func() {
			c.srv.editSums(c.t, rawRepo, fixtureTag, c.asset("raw", "relay"), func(lines []string, i int) []string {
				return append(lines, lines[i])
			})
		}, 2, "SHA256SUMS has a malformed or duplicate entry for "+c.asset("raw", "relay"))
	}},
	{"SHA256SUMS without the asset", func(c *shCase) {
		unchanged(c, func() {
			c.srv.editSums(c.t, rawRepo, fixtureTag, c.asset("raw", "relay"), func(lines []string, i int) []string {
				return slices.Delete(lines, i, i+1)
			})
		}, 2, "SHA256SUMS has no entry for "+c.asset("raw", "relay"))
	}},
	{"an identity mismatch restores the previous copy", func(c *shCase) {
		unchanged(c, func() {
			c.srv.replaceAsset(c.t, rawRepo, fixtureTag, c.asset("raw", "relay"), standIn("new", "v9.9.9"))
		}, 2, `relay reports "v9.9.9 (release)", not v1.2.3 (release)`, "the previous ones were restored")
	}},
	{"tar.gz archives", func(c *shCase) {
		c.script = c.r.file(c.t, "tgz", "install.sh")
		c.expect(c.run(), 0, c.installed("relay"), c.installed("relayctl"))
		expectFiles(c.t, c.dir, map[string][]byte{"relay": c.r.program(c.t, "relay"), "relayctl": c.r.program(c.t, "relayctl")})
	}},
	{"gz archives", func(c *shCase) {
		c.script = c.r.file(c.t, "gz", "install.sh")
		c.expect(c.run(), 0, c.installed("relay"), c.installed("relayctl"))
		expectFiles(c.t, c.dir, map[string][]byte{"relay": c.r.program(c.t, "relay"), "relayctl": c.r.program(c.t, "relayctl")})
	}},
	{"before_install runs the installed copy, and only that", func(c *shCase) {
		c.expect(c.run(), 0, "running relay hook-after --mark=1")
		if log := c.hookLog(); log != "" {
			c.t.Fatalf("a fresh install ran a hook through a stand-in: %q", log)
		}
		c.place("relay", standIn("old", fixtureTag))
		c.expect(c.run(), 0, "running relay hook-before", "running relay hook-after --mark=1")
		if log := c.hookLog(); log != "old hook-before\n" {
			c.t.Fatalf("hook log %q", log)
		}
	}},
	{"a failed before_install changes nothing", func(c *shCase) {
		c.env = append(c.env, "OLD_EXIT=1")
		unchanged(c, func() {}, 1, "before_install hook failed: relay hook-before", "a before_install hook failed; nothing was changed")
	}},
	{"a failed after_install exits 3, installed", func(c *shCase) {
		next := standIn("new", fixtureTag)
		c.srv.replaceAsset(c.t, rawRepo, fixtureTag, c.asset("raw", "relay"), next)
		c.env = append(c.env, "NEW_EXIT=1")
		c.expect(c.run(), 3, "after_install hook failed: relay hook-after --mark=1", "installed, but an after_install hook failed")
		expectFiles(c.t, c.dir, map[string][]byte{"relay": next, "relayctl": c.r.program(c.t, "relayctl")})
		if log := c.hookLog(); log != "new hook-after --mark=1\n" {
			c.t.Fatalf("hook log %q", log)
		}
	}},
	{"--no-hooks and RELAY_NO_HOOKS", func(c *shCase) {
		c.place("relay", standIn("old", fixtureTag))
		c.srv.replaceAsset(c.t, rawRepo, fixtureTag, c.asset("raw", "relay"), standIn("new", fixtureTag))
		c.env = append(c.env, "OLD_EXIT=1", "NEW_EXIT=1")
		c.expect(c.run("--no-hooks"), 0, c.installed("relay"))
		c.place("relay", standIn("old", fixtureTag))
		c.env = append(c.env, "RELAY_NO_HOOKS=1")
		c.expect(c.run(), 0, c.installed("relay"))
		if log := c.hookLog(); log != "" {
			c.t.Fatalf("hooks ran: %q", log)
		}
	}},
	{"--uninstall", func(c *shCase) {
		c.expect(c.run(), 0)
		c.expect(c.run(), 0)
		before := filesIn(c.t, c.dir)
		c.expect(c.run("--uninstall", "--dry-run"), 0, "would remove "+filepath.Join(c.dir, "relay"))
		expectFiles(c.t, c.dir, before)
		c.expect(c.run("--uninstall", "--product", "relayctl"), 0, "removed "+filepath.Join(c.dir, "relayctl"))
		expectFiles(c.t, c.dir, map[string][]byte{"relay": before["relay"], "relay.prev": before["relay.prev"]})
		c.expect(c.run("--uninstall"), 0, "removed "+filepath.Join(c.dir, "relay"), "configuration, if any, is left in place")
		expectFiles(c.t, c.dir, nil)
	}},
	{"--dry-run changes nothing", func(c *shCase) {
		c.expect(c.run("--dry-run"), 0, "would download "+c.srv.URL+"/fixture/relay/releases/download/v1.2.3/"+c.asset("raw", "relay")+
			" and install "+filepath.Join(c.dir, "relay"))
		if _, err := os.Stat(filepath.Join(c.home, ".local")); !errors.Is(err, os.ErrNotExist) {
			c.t.Fatalf("--dry-run made the install directory: %v", err)
		}
		if got := c.srv.got(); len(got) != 0 {
			c.t.Fatalf("--dry-run downloaded %v", got)
		}
	}},
	{"an unsupported platform lists the supported ones", func(c *shCase) {
		c.needStubs()
		c.stub("uname", `case $1 in -m) echo amd64 ;; *) echo FreeBSD ;; esac`)
		c.expect(c.run("--dry-run"), 1, "freebsd/amd64 is not a platform of this release (it ships darwin/arm64 linux/amd64 linux/arm64")
	}},
	{"aarch64 is arm64", func(c *shCase) {
		c.needStubs()
		c.stub("uname", `case $1 in -m) echo aarch64 ;; *) echo Linux ;; esac`)
		c.expect(c.run("--dry-run"), 0, "/"+c.r.assetName(c.t, "raw", "relay", selfupdate.Platform{OS: "linux", Arch: "arm64"})+" and install")
	}},
	{"Rosetta is corrected to arm64", func(c *shCase) {
		c.needStubs()
		c.stub("uname", `case $1 in -m) echo x86_64 ;; *) echo Darwin ;; esac`)
		c.stub("sysctl", `[ "$*" = "-n sysctl.proc_translated" ] && echo 1`)
		c.expect(c.run("--dry-run"), 0, "/"+c.r.assetName(c.t, "raw", "relay", selfupdate.Platform{OS: "darwin", Arch: "arm64"})+" and install")
		// An Intel Mac has no such sysctl.
		c.stub("sysctl", `exit 1`)
		c.expect(c.run("--dry-run"), 1, "darwin/amd64 is not a platform of this release")
	}},
	{"root is refused without --allow-root", func(c *shCase) {
		c.needStubs()
		c.stub("id", `echo 0`)
		c.expect(c.run(), 1, "refusing to run as root")
		c.expect(c.run("--allow-root", "--dry-run"), 0, "would download")
		expectFiles(c.t, c.dir, nil)
	}},
	{"SELFUPDATE_INSTALL_BASE_URL is https or loopback", func(c *shCase) {
		for _, base := range []string{"http://example.invalid", "ftp://127.0.0.1:1", "file:///etc",
			"http://localhost:@example.invalid", "http://127.0.0.1:1@example.invalid", "http://127.0.0.1:1/x"} {
			c.env = []string{"SELFUPDATE_INSTALL_BASE_URL=" + base}
			c.expect(c.run(), 1, "SELFUPDATE_INSTALL_BASE_URL must be an https:// URL, or a loopback http:// one")
		}
		expectFiles(c.t, c.dir, nil)
	}},
	{"--verify-attestation verifies each download", func(c *shCase) {
		c.stub("gh", ghStub)
		c.expect(c.run("--verify-attestation"), 0, c.installed("relay"), c.installed("relayctl"))
		log := c.hookLog()
		for _, product := range []string{"relay", "relayctl"} {
			want := "gh attestation verify " + c.asset("raw", product) + " --repo fixture/relay --signer-workflow " + publishWorkflow + "\n"
			if !strings.Contains(log, want) {
				c.t.Fatalf("gh calls %q lack %q", log, want)
			}
		}
	}},
	{"a failed attestation changes nothing", func(c *shCase) {
		c.stub("gh", ghStub)
		c.env = append(c.env, "GH_VERIFY_EXIT=1")
		old := standIn("old", fixtureTag)
		c.place("relay", old)
		c.expect(c.run("--verify-attestation"), 2, c.asset("raw", "relay")+": attestation verification failed")
		expectFiles(c.t, c.dir, map[string][]byte{"relay": old})
	}},
	{"--verify-attestation needs gh, logged in", func(c *shCase) {
		c.stub("gh", ghStub)
		c.env = append(c.env, "GH_AUTH_EXIT=1")
		c.expect(c.run("--verify-attestation"), 1, "--verify-attestation needs gh to be logged in (gh auth login)")
		// No gh at all: the stub goes, and PATH holds only basic tools.
		c.path = c.stubs + string(os.PathListSeparator) + linkTools(c.t, "awk", "cat", "grep", "id", "mkdir", "sort", "sysctl", "tr", "uname")
		if err := os.Remove(filepath.Join(c.stubs, "gh")); err != nil {
			c.t.Fatal(err)
		}
		c.expect(c.run("--verify-attestation"), 1, "--verify-attestation needs gh")
		if got := c.srv.got(); len(got) != 0 {
			c.t.Fatalf("downloaded %v before the attestation check", got)
		}
		expectFiles(c.t, c.dir, nil)
	}},
}

// ghStub stands in for gh: auth status exits $GH_AUTH_EXIT, and attestation
// verify logs its arguments, with the downloaded file's base name, and
// exits $GH_VERIFY_EXIT (0014-PLAN deviation D10).
const ghStub = `case "$1 $2" in
"auth status") exit "${GH_AUTH_EXIT:-0}" ;;
"attestation verify")
	file=$3
	shift 3
	echo "gh attestation verify ${file##*/} $*" >>"$HOOK_LOG"
	exit "${GH_VERIFY_EXIT:-0}"
	;;
esac
exit 9`

// publishWorkflow is the signer the installers require.
const publishWorkflow = "maccavelli/go-selfupdate-lib/.github/workflows/publish-selfupdate-release.yml"

// linkTools is a directory of links to the named tools, as found on PATH.
func linkTools(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range names {
		if path, err := exec.LookPath(name); err == nil {
			if err := os.Symlink(path, filepath.Join(dir, name)); err != nil {
				t.Fatal(err)
			}
		}
	}
	return dir
}

func TestInstallSh(t *testing.T) {
	r := sharedReleases(t)
	for _, sh := range shells(t) {
		t.Run(sh.name, func(t *testing.T) {
			t.Parallel()
			for _, tc := range shCases {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()
					tc.run(newShCase(t, sh, r))
				})
			}
		})
	}
}

// TestInstallShInterrupt sends Ctrl-C during a download: the script must
// stop, exit 130 and leave nothing behind.
func TestInstallShInterrupt(t *testing.T) {
	r := sharedReleases(t)
	for _, sh := range shells(t) {
		t.Run(sh.name, func(t *testing.T) {
			t.Parallel()
			c := newShCase(t, sh, r)
			stalled := c.srv.stall(c.asset("raw", "relay"))
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			cmd := c.command(ctx)
			var out bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &out
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			select {
			case <-stalled:
			case <-time.After(time.Minute):
				cancel()
				_ = cmd.Wait()
				t.Fatalf("the download never started:\n%s", out.String())
			}
			during := filesIn(t, c.dir)
			if len(during) != 1 || !slices.ContainsFunc(slices.Collect(maps.Keys(during)), func(n string) bool {
				return strings.HasPrefix(n, ".relay-install.") && strings.HasSuffix(n, "/")
			}) {
				t.Fatalf("during the download the install directory holds %v, want only its temporary directory", during)
			}
			if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGINT); err != nil {
				t.Fatal(err)
			}
			if code := exitCode(t, cmd.Wait()); code != 130 {
				t.Fatalf("exit %d after SIGINT, want 130:\n%s", code, out.String())
			}
			expectFiles(t, c.dir, nil)
		})
	}
}

// TestInstallShTruncated runs the script cut at every line, and at half its
// bytes, as a dropped connection would leave it: nothing may run.
func TestInstallShTruncated(t *testing.T) {
	r := sharedReleases(t)
	full := r.file(t, "raw", "install.sh")
	cuts := []int{len(full) / 2}
	for i, b := range full[:len(full)-1] {
		if b == '\n' {
			cuts = append(cuts, i+1)
		}
	}
	for _, sh := range shells(t) {
		t.Run(sh.name, func(t *testing.T) {
			t.Parallel()
			c := newShCase(t, sh, r)
			for _, cut := range cuts {
				c.script = full[:cut]
				res := c.run()
				if got := c.srv.got(); len(got) != 0 || strings.Contains(res.out, "would") {
					t.Fatalf("cut at byte %d: requested %v; output:\n%s", cut, got, res.out)
				}
			}
			entries, err := os.ReadDir(c.home)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 || entries[0].Name() != "stubs" {
				t.Fatalf("a truncated script wrote to its home: %v", entries)
			}
		})
	}
}

// TestInstallShFetchers runs with a PATH that has no curl: a wget that
// cannot refuse plain http is refused, one that can is told to, and with
// neither the script says so.
func TestInstallShFetchers(t *testing.T) {
	r := sharedReleases(t)
	tools := t.TempDir()
	for _, name := range []string{"awk", "cat", "chmod", "cut", "grep", "gzip", "head", "id", "mkdir", "mktemp", "mv",
		"openssl", "rm", "sha256sum", "shasum", "sort", "sysctl", "tar", "tr", "uname"} {
		if path, err := exec.LookPath(name); err == nil {
			if err := os.Symlink(path, filepath.Join(tools, name)); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, sh := range shells(t) {
		t.Run(sh.name, func(t *testing.T) {
			t.Parallel()
			c := newShCase(t, sh, r)
			c.needStubs()
			c.path = c.stubs + string(os.PathListSeparator) + tools
			c.expect(c.run(), 1, "curl or wget is required")
			// The https check happens before any connection, so the
			// address need not answer.
			c.env = []string{"SELFUPDATE_INSTALL_BASE_URL=https://127.0.0.1:9"}
			c.stub("wget", `[ "$1" = --help ] && { echo 'Usage: wget [-q] [-O FILE] URL'; exit 1; }
echo "wget $*" >>"$HOOK_LOG"; exit 1`)
			c.expect(c.run(), 1, "this wget cannot refuse plain http (no --https-only); install curl")
			if log := c.hookLog(); log != "" {
				t.Fatalf("the refused wget was run: %q", log)
			}
			c.stub("wget", `[ "$1" = --help ] && { echo '  --https-only  only follow secure HTTPS links'; exit 0; }
echo "wget $*" >>"$HOOK_LOG"; exit 1`)
			c.expect(c.run(), 2, "download of SHA256SUMS for v1.2.3 failed")
			if log := c.hookLog(); !strings.HasPrefix(log, "wget --https-only -q -O ") {
				t.Fatalf("wget ran as %q", log)
			}
		})
	}
}

// TestInstallShHashTools checks that a host with no SHA-256 tool fails
// rather than installing unchecked.
func TestInstallShHashTools(t *testing.T) {
	r := sharedReleases(t)
	tools := t.TempDir()
	for _, name := range []string{"awk", "cat", "chmod", "curl", "cut", "grep", "gzip", "head", "id", "mkdir", "mktemp", "mv",
		"rm", "sort", "sysctl", "tar", "tr", "uname", "wget"} {
		if path, err := exec.LookPath(name); err == nil {
			if err := os.Symlink(path, filepath.Join(tools, name)); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, sh := range shells(t) {
		t.Run(sh.name, func(t *testing.T) {
			t.Parallel()
			c := newShCase(t, sh, r)
			c.needStubs()
			c.path = tools
			c.expect(c.run(), 1, "sha256sum, shasum or openssl is required to verify the download")
			expectFiles(t, c.dir, nil)
		})
	}
}
