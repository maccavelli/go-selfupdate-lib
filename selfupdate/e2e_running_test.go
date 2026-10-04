package selfupdate_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/selfupdatetest"
)

// Tests for docs/decisions/0004-PLAN-h4-running-copy-end-to-end.md (0004-MADR
// §8 H4, amendment D1): Updater.Run replaces a program while it runs. The
// channel case is docs/decisions/0005-PLAN-opt-in-prerelease-channels.md
// Step 7.

// helperSource is the program being updated. Its version is stamped at build
// time, so the old and new builds differ in their bytes and say which they
// are.
const helperSource = `package main

import (
	"fmt"
	"os"
	"time"
)

var version = "dev"

func main() {
	switch {
	case len(os.Args) == 2 && os.Args[1] == "--version":
		fmt.Println("demo", version)
	case len(os.Args) == 4 && os.Args[1] == "serve":
		if err := os.WriteFile(os.Args[2], []byte(version), 0o600); err != nil {
			os.Exit(3)
		}
		deadline := time.Now().Add(60 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(os.Args[3]); err == nil {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		os.Exit(4)
	default:
		os.Exit(2)
	}
}
`

// stampFlag stamps the helper's version.
const stampFlag = "-X main.version="

var runtimePlatform = selfupdate.Platform{OS: runtime.GOOS, Arch: runtime.GOARCH}

// buildHelper builds the helper for goos and the runtime architecture, with
// version stamped, and returns its bytes.
func buildHelper(t *testing.T, version, goos string) []byte {
	t.Helper()
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("the go command is required to build the helper: %v", err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/demo\n\ngo 1.27.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(helperSource), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "demo.bin")
	cmd := exec.Command(goBin, "build", "-ldflags", stampFlag+version, "-o", out, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOOS="+goos, "GOARCH="+runtime.GOARCH, "CGO_ENABLED=0", "GOWORK=off", "GOFLAGS=")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build the %s helper for %s: %v\n%s", version, goos, err, b)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// runningCopy is the old program, started from the target and still running.
type runningCopy struct {
	cmd  *exec.Cmd
	done string
	once sync.Once
	err  error
}

func startCopy(t *testing.T, path string) *runningCopy {
	t.Helper()
	signals := t.TempDir()
	ready, done := filepath.Join(signals, "ready"), filepath.Join(signals, "done")
	cmd := exec.Command(path, "serve", ready, done)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	c := &runningCopy{cmd: cmd, done: done}
	t.Cleanup(func() { _ = c.stop() })
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			return c
		}
		if time.Now().After(deadline) {
			t.Fatal("the running copy never signalled ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// stop asks the copy to exit and returns its exit error, once.
func (c *runningCopy) stop() error {
	c.once.Do(func() {
		if err := os.WriteFile(c.done, []byte("x"), 0o600); err != nil {
			c.err = err
			return
		}
		c.err = c.cmd.Wait()
	})
	return c.err
}

// e2e is one target with its running copy, served release and Updater.
type e2e struct {
	dir, exe, base string
	gh             *selfupdatetest.GitHubServer
	inst           *selfupdate.StandaloneInstaller
	rec            *selfupdatetest.RecordingReporter
	u              *selfupdate.Updater
	running        *runningCopy
}

type e2eOptions struct {
	release     func(selfupdatetest.ReleaseSpec) selfupdatetest.ReleaseSpec
	postInstall selfupdate.Prober
	// releases, when set, are served in place of the one v1.1.0 release
	// built from served.
	releases []selfupdatetest.ReleaseSpec
	// versions, when set, replaces NewStrictVersionPolicy.
	versions selfupdate.VersionPolicy
}

const e2eToken = "e2e-token"

func newE2E(t *testing.T, v1, served []byte, o e2eOptions) *e2e {
	t.Helper()
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	base := "demo"
	if runtime.GOOS == "windows" {
		base = "demo.exe"
	}
	exe := filepath.Join(dir, base)
	if err := os.WriteFile(exe, v1, 0o755); err != nil {
		t.Fatal(err)
	}
	spec := selfupdatetest.NewRelease("demo", "v1.1.0", []selfupdate.Platform{runtimePlatform},
		func(selfupdate.Platform) []byte { return served })
	if o.release != nil {
		spec = o.release(spec)
	}
	releases := []selfupdatetest.ReleaseSpec{spec}
	if o.releases != nil {
		releases = o.releases
	}
	gh := selfupdatetest.NewGitHubServer(t, "owner", "demo", releases...)
	gh.RequireToken(e2eToken)
	src, err := selfupdate.NewGitHubSource(selfupdate.GitHubOptions{
		Repository: selfupdate.Repository{Owner: "owner", Name: "demo"},
		Client:     gh.Client, APIBaseURL: gh.APIBase, UserAgent: "demo/v1.0.0",
		Token: e2eToken, Limits: selfupdate.DefaultLimits(),
	})
	if err != nil {
		t.Fatal(err)
	}
	prober, err := selfupdate.NewVersionProber([]string{"--version"}, nil, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	post := prober
	if o.postInstall != nil {
		post = o.postInstall
	}
	inst, err := selfupdate.NewStandaloneInstaller(selfupdate.InstallOptions{
		TargetPolicy: selfupdate.TargetPolicy{ExecutablePath: exe, AllowedRoots: []string{dir}},
		PostInstall:  post,
	})
	if err != nil {
		t.Fatal(err)
	}
	image, err := selfupdate.NewImageVerifier(runtimePlatform)
	if err != nil {
		t.Fatal(err)
	}
	sel, err := selfupdate.NewExactAssetSelector([]selfupdate.Platform{runtimePlatform})
	if err != nil {
		t.Fatal(err)
	}
	versions := o.versions
	if versions == nil {
		versions = selfupdate.NewStrictVersionPolicy()
	}
	rec := &selfupdatetest.RecordingReporter{}
	u, err := selfupdate.New(selfupdate.Config{
		Source: src, Versions: versions, Assets: sel,
		Verifiers: []selfupdate.Verifier{image}, Probes: []selfupdate.Prober{prober},
		Installer: inst, Reporter: rec, Confirmer: selfupdate.NonInteractiveConfirmer(),
		Limits: selfupdate.DefaultLimits(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return &e2e{dir: dir, exe: exe, base: base, gh: gh, inst: inst, rec: rec, u: u, running: startCopy(t, exe)}
}

func (e *e2e) run() (selfupdate.Result, error) {
	return e.runOn("")
}

// runOn runs the update on channel; "" is the stable channel.
func (e *e2e) runOn(channel string) (selfupdate.Result, error) {
	return e.u.Run(context.Background(), selfupdate.Request{
		Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: selfupdate.ReleaseBuild,
		Platform: runtimePlatform, Yes: true, Channel: channel,
	})
}

// version runs the target as a user would.
func (e *e2e) version(t *testing.T) string {
	t.Helper()
	out, err := exec.Command(e.exe, "--version").Output()
	if err != nil {
		t.Fatalf("run %s --version: %v", e.base, err)
	}
	return strings.TrimSpace(string(out))
}

func (e *e2e) bytes(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(e.exe)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// leftovers lists staging and backup files beside the target. The lock file
// is not one: it stays, by design.
func (e *e2e) leftovers(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(e.dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, ent := range entries {
		if strings.HasPrefix(ent.Name(), "."+e.base+".selfupdate-") {
			out = append(out, ent.Name())
		}
	}
	return out
}

func (e *e2e) receipt() string {
	return filepath.Join(e.dir, "."+e.base+".selfupdate.cleanup")
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// TestE2EUpdateRunningCopy: the whole update path against a program that is
// running, on each OS.
func TestE2EUpdateRunningCopy(t *testing.T) {
	selfupdate.CheckNoLeak(t)
	v1 := buildHelper(t, "v1.0.0", runtime.GOOS)
	v2 := buildHelper(t, "v1.1.0", runtime.GOOS)
	e := newE2E(t, v1, v2, e2eOptions{})
	if got := e.version(t); got != "demo v1.0.0" {
		t.Fatalf("before the update the target reports %q", got)
	}

	res, err := e.run()
	if code := selfupdate.ExitCode(res, err); code != 0 || !res.Applied {
		t.Fatalf("ExitCode = %d, res = %+v, err = %v", code, res, err)
	}
	if !slices.Equal(e.bytes(t), v2) {
		t.Fatal("the target does not hold the v1.1.0 build")
	}
	if got := e.version(t); got != "demo v1.1.0" {
		t.Fatalf("after the update the target reports %q", got)
	}
	if kinds := e.rec.Kinds(); !slices.Contains(kinds, selfupdate.EventInstalling) || !slices.Contains(kinds, selfupdate.EventComplete) {
		t.Fatalf("events = %v", kinds)
	}

	if runtime.GOOS != "windows" {
		if err := e.running.stop(); err != nil {
			t.Fatalf("the old process did not exit cleanly: %v", err)
		}
		if res.PendingBackup != "" || len(e.leftovers(t)) != 0 || exists(e.receipt()) {
			t.Fatalf("pending %q, leftovers %v, receipt %v", res.PendingBackup, e.leftovers(t), exists(e.receipt()))
		}
		return
	}

	// Windows: the running image keeps its backup until it exits.
	if res.PendingBackup == "" || !exists(e.receipt()) {
		t.Fatalf("pending %q, receipt %v; want both while the old image runs", res.PendingBackup, exists(e.receipt()))
	}
	// A backup the old image still holds is kept, and is not an error: it
	// used to fail every later update until the old process exited
	// (0010-MADR B5).
	if err := e.inst.CleanupPending(context.Background()); err != nil {
		t.Fatalf("CleanupPending while the old image runs = %v, want the busy backup kept without an error", err)
	}
	if !exists(e.receipt()) || !exists(res.PendingBackup) {
		t.Fatalf("receipt %v, backup %v; want both kept while the old image runs", exists(e.receipt()), exists(res.PendingBackup))
	}
	if err := e.running.stop(); err != nil {
		t.Fatalf("the old process did not exit cleanly: %v", err)
	}
	if err := e.inst.CleanupPending(context.Background()); err != nil {
		t.Fatalf("CleanupPending after the old image exited = %v", err)
	}
	if exists(res.PendingBackup) || exists(e.receipt()) || len(e.leftovers(t)) != 0 {
		t.Fatalf("after cleanup: backup %v, receipt %v, leftovers %v", exists(res.PendingBackup), exists(e.receipt()), e.leftovers(t))
	}
}

// TestE2EUpdateRunningCopyOnChannel: a running program on the rc channel is
// replaced by the prerelease, and on the stable channel, from the same
// releases, by the stable build. Both pass the image check and the version
// probes, so the installed program reports the prerelease tag itself.
func TestE2EUpdateRunningCopyOnChannel(t *testing.T) {
	selfupdate.CheckNoLeak(t)
	v1 := buildHelper(t, "v1.0.0", runtime.GOOS)
	stable := buildHelper(t, "v1.2.0", runtime.GOOS)
	rc := buildHelper(t, "v1.3.0-rc.1", runtime.GOOS)
	policy, err := selfupdate.NewSemverPolicy(selfupdate.SemverOptions{AllowPrerelease: true, Channels: []string{"rc"}})
	if err != nil {
		t.Fatal(err)
	}
	plats := []selfupdate.Platform{runtimePlatform}
	pre := selfupdatetest.NewRelease("demo", "v1.3.0-rc.1", plats, func(selfupdate.Platform) []byte { return rc })
	pre.Prerelease = true
	releases := []selfupdatetest.ReleaseSpec{
		selfupdatetest.NewRelease("demo", "v1.2.0", plats, func(selfupdate.Platform) []byte { return stable }),
		pre,
	}
	for _, c := range []struct {
		name, channel, want string
		body                []byte
	}{
		{"rc", "rc", "v1.3.0-rc.1", rc},
		{"stable", "", "v1.2.0", stable},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := newE2E(t, v1, nil, e2eOptions{releases: releases, versions: policy})
			res, err := e.runOn(c.channel)
			if code := selfupdate.ExitCode(res, err); code != 0 || !res.Applied || res.TargetVersion != c.want {
				t.Fatalf("ExitCode = %d, res = %+v, err = %v; want %s applied", code, res, err, c.want)
			}
			if !slices.Equal(e.bytes(t), c.body) {
				t.Fatalf("the target does not hold the %s build", c.want)
			}
			if got := e.version(t); got != "demo "+c.want {
				t.Fatalf("after the update the target reports %q", got)
			}
			if err := e.running.stop(); err != nil {
				t.Fatalf("the old process did not exit cleanly: %v", err)
			}
			if err := e.inst.CleanupPending(context.Background()); err != nil {
				t.Fatalf("CleanupPending after the old process exited = %v", err)
			}
			if left := e.leftovers(t); len(left) != 0 || exists(e.receipt()) {
				t.Fatalf("leftovers %v, receipt %v", left, exists(e.receipt()))
			}
		})
	}
}

// errCrashes is a post-install probe's failure, as when the installed binary
// crashes on start.
var errCrashes = errors.New("crashes on start")

// TestE2ERunningCopyRefusals: each refused update leaves the running program
// as it was, byte for byte.
func TestE2ERunningCopyRefusals(t *testing.T) {
	selfupdate.CheckNoLeak(t)
	v1 := buildHelper(t, "v1.0.0", runtime.GOOS)
	v2 := buildHelper(t, "v1.1.0", runtime.GOOS)
	foreignOS := "linux"
	if runtime.GOOS == "linux" {
		foreignOS = "windows"
	}
	foreign := buildHelper(t, "v1.1.0", foreignOS)

	cases := []struct {
		name   string
		served []byte
		opts   e2eOptions
		before func(*e2e)
		check  func(*testing.T, *e2e, selfupdate.Result, error)
	}{
		{
			name:   "the binary does not match SHA256SUMS",
			served: v2,
			opts: e2eOptions{release: func(s selfupdatetest.ReleaseSpec) selfupdatetest.ReleaseSpec {
				s.Assets[0].Body = v1 // the manifest still lists v2's digest
				return s
			}},
			check: wantErrIs(selfupdate.ErrIntegrity),
		},
		{
			name: "the binary is for another OS", served: foreign,
			check: wantErrIs(selfupdate.ErrIntegrity),
		},
		{
			name: "the new binary reports the wrong version", served: v1,
			check: wantErrText("staged binary failed a probe"),
		},
		{
			name: "the installed binary fails its post-install probe", served: v2,
			opts: e2eOptions{postInstall: selfupdate.ProberFunc(func(context.Context, selfupdate.ProbeRequest) error {
				return errCrashes
			})},
			check: func(t *testing.T, e *e2e, res selfupdate.Result, err error) {
				t.Helper()
				wantErrText("installed binary failed its probe")(t, e, res, err)
				if !errors.Is(err, errCrashes) || !slices.Contains(e.rec.Kinds(), selfupdate.EventRolledBack) {
					t.Fatalf("err = %v, events = %v; want the probe's error and rolled-back", err, e.rec.Kinds())
				}
			},
		},
		{
			name: "the download is cut short", served: v2,
			before: func(e *e2e) { e.gh.TruncateAssets(true) },
			check:  func(*testing.T, *e2e, selfupdate.Result, error) {},
		},
		{
			name: "the API refuses the token", served: v2,
			before: func(e *e2e) { e.gh.RequireToken("other") },
			check:  wantErrText("github http 401"),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newE2E(t, v1, c.served, c.opts)
			if c.before != nil {
				c.before(e)
			}
			res, err := e.run()
			if err == nil || res.Applied {
				t.Fatalf("res = %+v, err = %v; want a refusal", res, err)
			}
			c.check(t, e, res, err)
			if !slices.Equal(e.bytes(t), v1) {
				t.Fatal("the target is not byte-identical to the v1.0.0 build")
			}
			if got := e.version(t); got != "demo v1.0.0" {
				t.Fatalf("the target reports %q", got)
			}
			if left := e.leftovers(t); len(left) != 0 || exists(e.receipt()) {
				t.Fatalf("leftovers %v, receipt %v", left, exists(e.receipt()))
			}
			if err := e.running.stop(); err != nil {
				t.Fatalf("the old process did not exit cleanly: %v", err)
			}
		})
	}
}

func wantErrIs(target error) func(*testing.T, *e2e, selfupdate.Result, error) {
	return func(t *testing.T, _ *e2e, _ selfupdate.Result, err error) {
		t.Helper()
		if !errors.Is(err, target) {
			t.Fatalf("err = %v, want %v", err, target)
		}
	}
}

func wantErrText(text string) func(*testing.T, *e2e, selfupdate.Result, error) {
	return func(t *testing.T, _ *e2e, _ selfupdate.Result, err error) {
		t.Helper()
		if !strings.Contains(err.Error(), text) {
			t.Fatalf("err = %v, want one mentioning %q", err, text)
		}
	}
}
