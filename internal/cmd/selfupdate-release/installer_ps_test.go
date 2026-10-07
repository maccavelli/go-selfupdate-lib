//go:build windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate/releasespec"
)

// Tests for docs/decisions/0014-PLAN-shared-installer-templates.md I4:
// install.ps1's behaviour (0014-MADR §4) under Windows PowerShell 5.1 and
// PowerShell 7, run three ways: as a file, through `irm … | iex` with its
// options in environment variables, and through
// `& ([scriptblock]::Create((irm …))) -Option …` as a user types it.

// psHost is a PowerShell: Windows PowerShell 5.1 or PowerShell 7.
type psHost struct {
	name, path string
	major      int
}

func psHosts(t *testing.T) []psHost {
	t.Helper()
	var out []psHost
	for _, c := range []psHost{{name: "powershell", path: "powershell.exe", major: 5}, {name: "pwsh", path: "pwsh.exe", major: 7}} {
		path, err := exec.LookPath(c.path)
		if err != nil {
			continue
		}
		c.path = path
		out = append(out, c)
	}
	if len(out) == 0 {
		t.Fatal("no PowerShell found")
	}
	for _, want := range strings.FieldsFunc(os.Getenv(requireShellsEnv), func(r rune) bool { return r == ',' }) {
		if !slices.ContainsFunc(out, func(h psHost) bool { return h.name == want }) {
			t.Fatalf("%s: no %s among %v", requireShellsEnv, want, out)
		}
	}
	return out
}

// psMode is how the script is run.
type psMode string

const (
	psFile  psMode = "file"
	psIex   psMode = "iex"
	psBlock psMode = "scriptblock"
)

// psDriver runs the script in the iex and scriptblock forms, as a user's
// session would, and reports what the run left in that session: new
// variables and functions, the error preference, strict mode and the TLS
// protocols. A failure under these forms is the error "install failed (exit
// N)"; the driver exits N, and 99 for any other error. The driver's own
// catch block sets $_ and $PSItem, so they are not counted. The scriptblock
// form's arguments arrive as PowerShell source in
// SELFUPDATE_INSTALL_TEST_ARGS: on the command line, a value that starts
// with "-" would be read as one of the driver's own options.
const psDriver = `param([string]$Mode, [string]$Url)
$ArgsText = $env:SELFUPDATE_INSTALL_TEST_ARGS
$LASTEXITCODE = 0
$sb = $null
$failure = $null
$newVars = $null
$newFuncs = $null
$tlsAfter = $null
$tlsBefore = [Net.ServicePointManager]::SecurityProtocol
$eapBefore = $ErrorActionPreference
$varsBefore = $null
$funcsBefore = $null
$varsBefore = @(Get-Variable | ForEach-Object { $_.Name })
$funcsBefore = @(Get-ChildItem function: | ForEach-Object { $_.Name })
try {
    if ($Mode -eq 'iex') {
        Invoke-RestMethod $Url | Invoke-Expression
    } else {
        $sb = [scriptblock]::Create((Invoke-RestMethod $Url))
        Invoke-Expression ('& $sb ' + $ArgsText)
    }
} catch {
    $failure = $_.Exception.Message
}
$newVars = @(Get-Variable | ForEach-Object { $_.Name } | Where-Object { $varsBefore -notcontains $_ -and $_ -ne '_' -and $_ -ne 'PSItem' })
$newFuncs = @(Get-ChildItem function: | ForEach-Object { $_.Name } | Where-Object { $funcsBefore -notcontains $_ })
$tlsAfter = [Net.ServicePointManager]::SecurityProtocol
[Console]::Out.WriteLine('driver: new variables [' + ($newVars -join ',') + ']')
[Console]::Out.WriteLine('driver: new functions [' + ($newFuncs -join ',') + ']')
[Console]::Out.WriteLine("driver: ErrorActionPreference $eapBefore -> $ErrorActionPreference")
try { $null = $NoSuchVariableAnywhere; [Console]::Out.WriteLine('driver: strict mode off') } catch { [Console]::Out.WriteLine('driver: strict mode on') }
[Console]::Out.WriteLine('driver: TLS ' + [int]$tlsBefore + ' -> ' + [int]$tlsAfter)
[Console]::Out.WriteLine('driver: session alive')
if ($null -eq $failure) { exit 0 }
$m = [regex]::Match($failure, '^install failed \(exit ([0-9]+)\)$')
if ($m.Success) { exit [int]$m.Groups[1].Value }
[Console]::Out.WriteLine("driver: unexpected error: $failure")
exit 99
`

// psCutsDriver runs every cut of the script at Path, at each line end, the
// way a dropped connection would leave it: through Invoke-Expression and
// through [scriptblock]::Create. A parse error is expected; anything the
// cut does is checked by the test.
const psCutsDriver = `param([string]$Path)
$text = [IO.File]::ReadAllText($Path)
$ends = 0
for ($i = 0; $i -lt $text.Length - 1; $i++) {
    if ($text[$i] -ne "` + "`" + `n") { continue }
    $cut = $text.Substring(0, $i + 1)
    $ends++
    try { Invoke-Expression $cut } catch { }
    try { & ([scriptblock]::Create($cut)) } catch { }
}
[Console]::Out.WriteLine("driver: ran $ends cuts")
`

// psOptionRe is an option as a user types it: -Name, or -Name:$true.
var psOptionRe = regexp.MustCompile(`^-[A-Za-z]+(:\$(true|false))?$`)

// psArgsText is the file form's arguments as PowerShell source: options
// as they are, values single-quoted.
func psArgsText(args []string) string {
	words := make([]string, len(args))
	for i, a := range args {
		if psOptionRe.MatchString(a) {
			words[i] = a
			continue
		}
		words[i] = "'" + strings.ReplaceAll(a, "'", "''") + "'"
	}
	return strings.Join(words, " ")
}

// offer serves script at scriptPath, for the forms that fetch it.
func (s *releaseServer) offer(script []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.script = script
}

var regKeys atomic.Int64

// psCase is one run's world: LOCALAPPDATA, which holds the default install
// directory, a scratch registry key for PATH updates, and a release server.
type psCase struct {
	t      *testing.T
	ps     psHost
	mode   psMode
	r      installReleases
	srv    *releaseServer
	script []byte
	env    []string
	local  string // LOCALAPPDATA
	dir    string // %LOCALAPPDATA%\Programs\relay
	key    string // under HKCU
	log    string // HOOK_LOG
	work   string // the case's own files
}

func newPsCase(t *testing.T, ps psHost, mode psMode, r installReleases) *psCase {
	t.Helper()
	local := t.TempDir()
	c := &psCase{t: t, ps: ps, mode: mode, r: r, srv: newReleaseServer(t, r), script: r.file(t, "raw", "install.ps1"),
		local: local, dir: filepath.Join(local, "Programs", "relay"), work: t.TempDir(),
		key: fmt.Sprintf(`Software\SelfupdateInstallTest\%d-%d`, os.Getpid(), regKeys.Add(1))}
	c.log = filepath.Join(c.work, "hooks.log")
	t.Cleanup(func() { _ = exec.Command("reg", "delete", `HKCU\`+c.key, "/f").Run() })
	return c
}

func (c *psCase) envFor() []string {
	var env []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		switch {
		case strings.EqualFold(name, "PSModulePath"), strings.EqualFold(name, "LOCALAPPDATA"),
			strings.HasPrefix(strings.ToUpper(name), "RELAY_"), strings.HasPrefix(strings.ToUpper(name), "SELFUPDATE_INSTALL_"):
			continue
		}
		env = append(env, kv)
	}
	return append(append(env, "LOCALAPPDATA="+c.local, "HOOK_LOG="+c.log, "SELFUPDATE_INSTALL_BASE_URL="+c.srv.URL,
		"SELFUPDATE_INSTALL_TEST_ENV_KEY="+c.key), c.env...)
}

func (c *psCase) writeFile(name string, data []byte) string {
	c.t.Helper()
	path := filepath.Join(c.work, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		c.t.Fatal(err)
	}
	return path
}

func (c *psCase) command(ctx context.Context, args ...string) *exec.Cmd {
	c.t.Helper()
	argv := []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File"}
	switch c.mode {
	case psFile:
		argv = append(append(argv, c.writeFile("install.ps1", c.script)), args...)
	case psIex:
		if len(args) > 0 {
			c.t.Fatalf("the iex form takes no arguments: %v", args)
		}
		c.srv.offer(c.script)
		argv = append(argv, c.writeFile("driver.ps1", []byte(psDriver)), "-Mode", "iex", "-Url", c.srv.URL+scriptPath)
	case psBlock:
		c.srv.offer(c.script)
		argv = append(argv, c.writeFile("driver.ps1", []byte(psDriver)), "-Mode", "block", "-Url", c.srv.URL+scriptPath)
	}
	cmd := exec.CommandContext(ctx, c.ps.path, argv...)
	cmd.Env = append(c.envFor(), "SELFUPDATE_INSTALL_TEST_ARGS="+psArgsText(args))
	return cmd
}

var tlsRe = regexp.MustCompile(`driver: TLS ([0-9]+) -> ([0-9]+)`)

const tls12 = 3072 // [Net.SecurityProtocolType]::Tls12

func (c *psCase) run(args ...string) runResult {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	out, err := c.command(ctx, args...).CombinedOutput()
	if ctx.Err() != nil {
		c.t.Fatalf("timed out:\n%s", out)
	}
	r := runResult{exitCode(c.t, err), string(out)}
	if c.mode != psFile {
		c.session(r)
	}
	return r
}

// session checks what an iex or scriptblock run left in the session: at
// most TLS 1.2, added on Windows PowerShell 5.1, unless the options were
// refused before the script ran.
func (c *psCase) session(r runResult) {
	c.t.Helper()
	for _, want := range []string{"driver: new variables []", "driver: new functions []", "driver: ErrorActionPreference Continue -> Continue",
		"driver: strict mode off", "driver: session alive"} {
		if !strings.Contains(r.out, want) {
			c.t.Fatalf("the session after the run: no %q in:\n%s", want, r.out)
		}
	}
	m := tlsRe.FindStringSubmatch(r.out)
	if m == nil {
		c.t.Fatalf("no TLS line in:\n%s", r.out)
	}
	before, _ := strconv.Atoi(m[1])
	after, _ := strconv.Atoi(m[2])
	if after != before && (c.ps.major >= 6 || after != before|tls12) {
		c.t.Fatalf("TLS protocols %d -> %d", before, after)
	}
	if c.ps.major < 6 && after != before|tls12 && !strings.Contains(r.out, "parameter") {
		c.t.Fatalf("Windows PowerShell ran the script without adding TLS 1.2 (%d -> %d)", before, after)
	}
}

func (c *psCase) expect(r runResult, code int, wants ...string) {
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

func (c *psCase) place(name string, data []byte) {
	c.t.Helper()
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		c.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(c.dir, name), data, 0o755); err != nil {
		c.t.Fatal(err)
	}
}

func (c *psCase) hookLog() string {
	c.t.Helper()
	data, err := os.ReadFile(c.log)
	if err != nil && !os.IsNotExist(err) {
		c.t.Fatal(err)
	}
	return strings.ReplaceAll(string(data), "\r\n", "\n")
}

func (c *psCase) asset(kind, product string) string {
	return c.r.assetName(c.t, kind, product, host)
}

func (c *psCase) installed(name string) string {
	return "installed " + filepath.Join(c.dir, name+".exe") + " (" + fixtureTag + ")"
}

func (c *psCase) programs() map[string][]byte {
	return map[string][]byte{"relay.exe": c.r.program(c.t, "relay"), "relayctl.exe": c.r.program(c.t, "relayctl")}
}

// pathValue is the scratch key's Path: its kind, its raw value, and
// whether it exists.
func (c *psCase) pathValue() (string, string, bool) {
	c.t.Helper()
	out, err := exec.Command("reg", "query", `HKCU\`+c.key, "/v", "Path").CombinedOutput()
	if err != nil {
		return "", "", false
	}
	for _, line := range strings.Split(string(out), "\r\n") {
		f := strings.SplitN(strings.TrimSpace(line), "    ", 3)
		if len(f) >= 2 && f[0] == "Path" {
			if len(f) == 2 {
				return f[1], "", true
			}
			return f[1], f[2], true
		}
	}
	c.t.Fatalf("reg query: no Path in:\n%s", out)
	return "", "", false
}

func (c *psCase) setPath(kind, value string) {
	c.t.Helper()
	if out, err := exec.Command("reg", "add", `HKCU\`+c.key, "/v", "Path", "/t", kind, "/d", value, "/f").CombinedOutput(); err != nil {
		c.t.Fatalf("reg add: %v\n%s", err, out)
	}
}

func (c *psCase) expectPath(kind, value string) {
	c.t.Helper()
	k, v, ok := c.pathValue()
	if !ok || k != kind || v != value {
		c.t.Fatalf("PATH is %s %q (exists %v), want %s %q", k, v, ok, kind, value)
	}
}

func (c *psCase) expectNoPath() {
	c.t.Helper()
	if k, v, ok := c.pathValue(); ok {
		c.t.Fatalf("PATH was written: %s %q", k, v)
	}
}

// unchanged runs expecting code and wants with an earlier relay.exe in
// place, and checks that it is all there is after, and PATH untouched.
func (c *psCase) unchanged(edit func(), code int, wants ...string) {
	c.t.Helper()
	old := standInExe(c.t, "old", fixtureTag)
	c.place("relay.exe", old)
	edit()
	c.expect(c.run(), code, wants...)
	expectFiles(c.t, c.dir, map[string][]byte{"relay.exe": old})
	c.expectNoPath()
}

// standInSource is a program for hooks and identity checks: it reports its
// version as a release, holds when asked, and otherwise prints and logs
// "<role> <args>" and exits with $<ROLE>_EXIT.
const standInSource = `package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

var role, version string

func main() {
	args := strings.Join(os.Args[1:], " ")
	switch args {
	case "version":
		fmt.Println(version + " (release)")
		return
	case "hold":
		time.Sleep(10 * time.Minute)
		return
	}
	fmt.Println(role + " ran " + args)
	if f, err := os.OpenFile(os.Getenv("HOOK_LOG"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
		fmt.Fprintln(f, role+" "+args)
		f.Close()
	}
	if os.Getenv(strings.ToUpper(role)+"_EXIT") == "1" {
		os.Exit(1)
	}
}
`

var (
	standInsOnce sync.Once
	standInsDir  string
	standInsErr  error
)

var standInVariants = [][2]string{{"old", fixtureTag}, {"new", fixtureTag}, {"new", "v9.9.9"}}

// standInExe is the stand-in built for role and version.
func standInExe(t *testing.T, role, version string) []byte {
	t.Helper()
	standInsOnce.Do(func() {
		dir, err := os.MkdirTemp("", "selfupdate-standin-*")
		if err != nil {
			standInsErr = err
			return
		}
		keepUntilExit(dir)
		standInsDir = dir
		for name, data := range map[string]string{"go.mod": "module standin\n\ngo 1.27.1\n", "main.go": standInSource} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
				standInsErr = err
				return
			}
		}
		for _, v := range standInVariants {
			cmd := exec.Command("go", "build", "-trimpath", "-ldflags", "-X main.role="+v[0]+" -X main.version="+v[1],
				"-o", v[0]+"-"+v[1]+".exe", ".")
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOWORK=off", "GOFLAGS=")
			if out, err := cmd.CombinedOutput(); err != nil {
				standInsErr = fmt.Errorf("building the stand-in: %w\n%s", err, out)
				return
			}
		}
	})
	if standInsErr != nil {
		t.Fatal(standInsErr)
	}
	data, err := os.ReadFile(filepath.Join(standInsDir, role+"-"+version+".exe"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// otherArchScript is an installer for a release that ships Windows only
// on an architecture this host is not.
func otherArchScript(t *testing.T) ([]byte, string) {
	t.Helper()
	arch := "arm64"
	if host.Arch == "arm64" {
		arch = "amd64"
	}
	data, err := json.Marshal(map[string]any{
		"schema":    1,
		"products":  []any{map[string]any{"name": "relay", "package": "./cmd/relay"}},
		"platforms": []any{map[string]string{"os": "windows", "arch": arch}},
		"installer": map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	spec, err := releasespec.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	files, err := renderInstallers(spec, rawRepo, fixtureTag)
	if err != nil {
		t.Fatal(err)
	}
	return files["install.ps1"], arch
}

var (
	allModes  = []psMode{psFile, psIex, psBlock}
	argModes  = []psMode{psFile, psBlock}
	fileModes = []psMode{psFile}
)

var psCases = []struct {
	name  string
	modes []psMode
	run   func(c *psCase)
}{
	{"installs every product", allModes, func(c *psCase) {
		c.expect(c.run(), 0, c.installed("relay"), c.installed("relayctl"), "added "+c.dir+" to your user PATH")
		expectFiles(c.t, c.dir, c.programs())
		c.expectPath("REG_EXPAND_SZ", c.dir)
	}},
	{"-Product installs a subset", argModes, func(c *psCase) {
		c.expect(c.run("-Product", "relayctl"), 0, c.installed("relayctl"))
		expectFiles(c.t, c.dir, map[string][]byte{"relayctl.exe": c.r.program(c.t, "relayctl")})
		c.expect(c.run("-Product", "nope"), 1, "nope is not a product of this release (relay relayctl)")
	}},
	{"a reinstall keeps the previous copy", argModes, func(c *psCase) {
		old := standInExe(c.t, "old", fixtureTag)
		c.place("relay.exe", old)
		c.expect(c.run(), 0, c.installed("relay"))
		want := c.programs()
		want["relay.exe.prev"] = old
		expectFiles(c.t, c.dir, want)
	}},
	{"-InstallDir", argModes, func(c *psCase) {
		a := filepath.Join(c.work, "a")
		c.expect(c.run("-InstallDir", a, "-Product", "relayctl"), 0, "installed "+filepath.Join(a, "relayctl.exe"))
		expectFiles(c.t, a, map[string][]byte{"relayctl.exe": c.r.program(c.t, "relayctl")})
		c.expectPath("REG_EXPAND_SZ", a)
	}},
	{"RELAY_INSTALL_DIR", allModes, func(c *psCase) {
		b := filepath.Join(c.work, "b")
		c.env = append(c.env, "RELAY_INSTALL_DIR="+b)
		c.expect(c.run(), 0, "installed "+filepath.Join(b, "relayctl.exe"))
		expectFiles(c.t, b, c.programs())
		expectFiles(c.t, c.dir, nil)
	}},
	{"-Version installs another release", argModes, func(c *psCase) {
		c.script = c.r.render(c.t, "raw", "install.ps1", "v1.3.0")
		c.expect(c.run(), 2, "download of SHA256SUMS for v1.3.0 failed")
		c.expect(c.run("-Version", "1.2.3", "-Product", "relay"), 0, c.installed("relay"))
		c.expect(c.run("-Version", "v1.2.3", "-Product", "relay"), 0, c.installed("relay"))
	}},
	{"RELAY_VERSION installs another release", allModes, func(c *psCase) {
		c.script = c.r.render(c.t, "raw", "install.ps1", "v1.3.0")
		c.env = append(c.env, "RELAY_VERSION=1.2.3")
		c.expect(c.run(), 0, c.installed("relay"), c.installed("relayctl"))
	}},
	{"-Version outside the tag rule or the channels", argModes, func(c *psCase) {
		c.expect(c.run("-Version", "v1.2.4-beta.1"), 1, "v1.2.4-beta.1 is a prerelease on channel beta, which this release does not publish")
		c.expect(c.run("-Version", "latest"), 1, "vlatest is not a release tag")
		c.expect(c.run("-Version", "v1.2.4-rc.1"), 2, "download of SHA256SUMS for v1.2.4-rc.1 failed")
		expectFiles(c.t, c.dir, nil)
		c.expectNoPath()
	}},
	{"-Version of a release with other asset names", argModes, func(c *psCase) {
		c.script = c.r.render(c.t, "tgz", "install.ps1", "v1.3.0")
		c.srv.route("fixture/relay-tgz", fixtureTag, c.r.staged("raw"))
		c.expect(c.run("-Version", fixtureTag), 1, "release v1.2.3 has no "+c.asset("tgz", "relay")+"; its own installer is "+
			c.srv.URL+"/fixture/relay-tgz/releases/download/v1.2.3/install.ps1")
		if got := c.srv.got(); !slices.Equal(got, []string{"/fixture/relay-tgz/releases/download/v1.2.3/SHA256SUMS"}) {
			c.t.Fatalf("requested %v, want only SHA256SUMS", got)
		}
		expectFiles(c.t, c.dir, nil)
	}},
	{"an altered asset", allModes, func(c *psCase) {
		c.unchanged(func() {
			c.srv.replace(rawRepo, fixtureTag, c.asset("raw", "relay"), append(c.r.program(c.t, "relay"), 'x'))
		}, 2, c.asset("raw", "relay")+": SHA-256 ", "does not match SHA256SUMS")
	}},
	{"SHA256SUMS with CR line endings", argModes, func(c *psCase) {
		c.unchanged(func() {
			c.srv.editSums(c.t, rawRepo, fixtureTag, c.asset("raw", "relay"), func(lines []string, _ int) []string {
				for i := range lines {
					lines[i] += "\r"
				}
				return lines
			})
		}, 2, "SHA256SUMS has no entry for "+c.asset("raw", "relay"))
	}},
	{"SHA256SUMS with a 63-character hash", argModes, func(c *psCase) {
		c.unchanged(func() {
			c.srv.editSums(c.t, rawRepo, fixtureTag, c.asset("raw", "relay"), func(lines []string, i int) []string {
				lines[i] = lines[i][1:]
				return lines
			})
		}, 2, "SHA256SUMS has no entry for "+c.asset("raw", "relay"))
	}},
	{"SHA256SUMS with an upper-case hash", argModes, func(c *psCase) {
		c.unchanged(func() {
			c.srv.editSums(c.t, rawRepo, fixtureTag, c.asset("raw", "relay"), func(lines []string, i int) []string {
				lines[i] = strings.ToUpper(lines[i][:64]) + lines[i][64:]
				return lines
			})
		}, 2, "SHA256SUMS has a malformed entry for "+c.asset("raw", "relay"))
	}},
	{"SHA256SUMS listing the asset twice", argModes, func(c *psCase) {
		c.unchanged(func() {
			c.srv.editSums(c.t, rawRepo, fixtureTag, c.asset("raw", "relay"), func(lines []string, i int) []string {
				return append(lines, lines[i])
			})
		}, 2, "SHA256SUMS has 2 entries for "+c.asset("raw", "relay"))
	}},
	{"SHA256SUMS without the asset", argModes, func(c *psCase) {
		c.unchanged(func() {
			c.srv.editSums(c.t, rawRepo, fixtureTag, c.asset("raw", "relay"), func(lines []string, i int) []string {
				return slices.Delete(lines, i, i+1)
			})
		}, 2, "SHA256SUMS has no entry for "+c.asset("raw", "relay"))
	}},
	{"an identity mismatch restores the previous copy", allModes, func(c *psCase) {
		c.unchanged(func() {
			c.srv.replaceAsset(c.t, rawRepo, fixtureTag, c.asset("raw", "relay"), standInExe(c.t, "new", "v9.9.9"))
		}, 2, `relay reports "v9.9.9 (release)", not v1.2.3 (release)`, "the previous ones were restored")
	}},
	{"zip archives", argModes, func(c *psCase) {
		c.script = c.r.file(c.t, "tgz", "install.ps1")
		c.expect(c.run(), 0, c.installed("relay"), c.installed("relayctl"))
		expectFiles(c.t, c.dir, c.programs())
	}},
	{"before_install runs the installed copy, and only that", argModes, func(c *psCase) {
		c.expect(c.run(), 0, "running relay hook-after --mark=1")
		if log := c.hookLog(); log != "" {
			c.t.Fatalf("a fresh install ran a hook through a stand-in: %q", log)
		}
		c.place("relay.exe", standInExe(c.t, "old", fixtureTag))
		c.expect(c.run(), 0, "running relay hook-before", "old ran hook-before", "running relay hook-after --mark=1")
		if log := c.hookLog(); log != "old hook-before\n" {
			c.t.Fatalf("hook log %q", log)
		}
	}},
	{"a failed before_install changes nothing", allModes, func(c *psCase) {
		c.env = append(c.env, "OLD_EXIT=1")
		c.unchanged(func() {}, 1, "before_install hook failed: relay hook-before", "a before_install hook failed; nothing was changed")
	}},
	{"a failed after_install exits 3, installed", allModes, func(c *psCase) {
		next := standInExe(c.t, "new", fixtureTag)
		c.srv.replaceAsset(c.t, rawRepo, fixtureTag, c.asset("raw", "relay"), next)
		c.env = append(c.env, "NEW_EXIT=1")
		c.expect(c.run(), 3, "after_install hook failed: relay hook-after --mark=1", "installed, but an after_install hook failed")
		want := c.programs()
		want["relay.exe"] = next
		expectFiles(c.t, c.dir, want)
		if log := c.hookLog(); log != "new hook-after --mark=1\n" {
			c.t.Fatalf("hook log %q", log)
		}
	}},
	{"-NoHooks", argModes, func(c *psCase) {
		c.place("relay.exe", standInExe(c.t, "old", fixtureTag))
		c.srv.replaceAsset(c.t, rawRepo, fixtureTag, c.asset("raw", "relay"), standInExe(c.t, "new", fixtureTag))
		c.env = append(c.env, "OLD_EXIT=1", "NEW_EXIT=1")
		c.expect(c.run("-NoHooks"), 0, c.installed("relay"))
		if log := c.hookLog(); log != "" {
			c.t.Fatalf("hooks ran: %q", log)
		}
	}},
	{"RELAY_NO_HOOKS", allModes, func(c *psCase) {
		c.place("relay.exe", standInExe(c.t, "old", fixtureTag))
		c.srv.replaceAsset(c.t, rawRepo, fixtureTag, c.asset("raw", "relay"), standInExe(c.t, "new", fixtureTag))
		c.env = append(c.env, "OLD_EXIT=1", "NEW_EXIT=1", "RELAY_NO_HOOKS=1")
		c.expect(c.run(), 0, c.installed("relay"))
		if log := c.hookLog(); log != "" {
			c.t.Fatalf("hooks ran: %q", log)
		}
	}},
	{"-Uninstall", argModes, func(c *psCase) {
		c.expect(c.run(), 0)
		c.expect(c.run(), 0)
		before := filesIn(c.t, c.dir)
		c.expect(c.run("-Uninstall", "-DryRun"), 0, "would remove "+filepath.Join(c.dir, "relay.exe"))
		expectFiles(c.t, c.dir, before)
		c.expectPath("REG_EXPAND_SZ", c.dir)
		c.expect(c.run("-Uninstall", "-Product", "relayctl"), 0, "removed "+filepath.Join(c.dir, "relayctl.exe"))
		expectFiles(c.t, c.dir, map[string][]byte{"relay.exe": before["relay.exe"], "relay.exe.prev": before["relay.exe.prev"]})
		// relay.exe is still there, so its folder stays on PATH.
		c.expectPath("REG_EXPAND_SZ", c.dir)
		c.expect(c.run("-Uninstall"), 0, "removed "+filepath.Join(c.dir, "relay.exe"), "configuration, if any, is left in place")
		expectFiles(c.t, c.dir, nil)
		c.expectPath("REG_EXPAND_SZ", "")
	}},
	{"-DryRun changes nothing", argModes, func(c *psCase) {
		c.expect(c.run("-DryRun"), 0, "would download "+c.srv.URL+"/fixture/relay/releases/download/v1.2.3/"+c.asset("raw", "relay")+
			" and install "+filepath.Join(c.dir, "relay.exe"))
		if _, err := os.Stat(filepath.Join(c.local, "Programs")); !os.IsNotExist(err) {
			c.t.Fatalf("-DryRun made the install directory: %v", err)
		}
		if got := c.srv.got(); len(got) != 0 {
			c.t.Fatalf("-DryRun downloaded %v", got)
		}
		c.expectNoPath()
	}},
	{"an unsupported platform lists the supported ones", argModes, func(c *psCase) {
		script, arch := otherArchScript(c.t)
		c.script = script
		c.expect(c.run("-DryRun"), 1, "windows/"+host.Arch+" is not a platform of this release (it ships windows/"+arch+")")
	}},
	{"SELFUPDATE_INSTALL_BASE_URL is https or loopback", argModes, func(c *psCase) {
		for _, base := range []string{"http://example.invalid", "ftp://127.0.0.1:1", "file:///C:/Windows",
			"http://localhost:@example.invalid", "http://127.0.0.1:1@example.invalid", "http://127.0.0.1:1/x"} {
			c.env = []string{"SELFUPDATE_INSTALL_BASE_URL=" + base}
			c.expect(c.run("-DryRun"), 1, "SELFUPDATE_INSTALL_BASE_URL must be an https:// URL, or a loopback http:// one")
		}
	}},
	{"an unknown option", argModes, func(c *psCase) {
		c.expect(c.run("-Bogus"), 1, "Bogus")
		expectFiles(c.t, c.dir, nil)
	}},
	{"a switch given a value", argModes, func(c *psCase) {
		// @args passes -Uninstall:$false on as -Uninstall and a stray False,
		// which must not become -InstallDir.
		c.expect(c.run("-Uninstall:$false", "-Version", fixtureTag), 1, "A positional parameter cannot be found")
		if _, err := os.Stat(filepath.Join(c.local, "Programs")); !os.IsNotExist(err) {
			c.t.Fatalf("the install directory was made: %v", err)
		}
	}},
	{"the user PATH: added once, its kind and %VAR% entries kept", argModes, func(c *psCase) {
		c.setPath("REG_EXPAND_SZ", `%USERPROFILE%\bin;C:\Elsewhere`)
		c.expect(c.run(), 0, "added "+c.dir+" to your user PATH")
		c.expectPath("REG_EXPAND_SZ", `%USERPROFILE%\bin;C:\Elsewhere;`+c.dir)
		r := c.run()
		c.expect(r, 0, c.installed("relay"))
		if strings.Contains(r.out, "added ") {
			c.t.Fatalf("a second install changed PATH:\n%s", r.out)
		}
		c.expectPath("REG_EXPAND_SZ", `%USERPROFILE%\bin;C:\Elsewhere;`+c.dir)
	}},
	{"a REG_SZ PATH stays REG_SZ, and an entry already there is kept", fileModes, func(c *psCase) {
		c.setPath("REG_SZ", `C:\Elsewhere`)
		c.expect(c.run(), 0, "added "+c.dir+" to your user PATH")
		c.expectPath("REG_SZ", `C:\Elsewhere;`+c.dir)
		c.setPath("REG_SZ", c.dir+`\;C:\Elsewhere`)
		r := c.run()
		c.expect(r, 0)
		if strings.Contains(r.out, "added ") {
			c.t.Fatalf("PATH already had the folder, with a trailing backslash:\n%s", r.out)
		}
		c.expectPath("REG_SZ", c.dir+`\;C:\Elsewhere`)
	}},
	{"-NoPathUpdate writes nothing", argModes, func(c *psCase) {
		c.expect(c.run("-NoPathUpdate"), 0, "add "+c.dir+" to your PATH to run the programs by name")
		c.expectNoPath()
	}},
	{"RELAY_NO_PATH_UPDATE writes nothing", allModes, func(c *psCase) {
		c.env = append(c.env, "RELAY_NO_PATH_UPDATE=1")
		c.expect(c.run(), 0, "add "+c.dir+" to your PATH to run the programs by name")
		c.expectNoPath()
	}},
	{"a running copy is replaced", fileModes, func(c *psCase) {
		old := standInExe(c.t, "old", fixtureTag)
		c.place("relay.exe", old)
		hold := exec.Command(filepath.Join(c.dir, "relay.exe"), "hold")
		if err := hold.Start(); err != nil {
			c.t.Fatal(err)
		}
		c.t.Cleanup(func() {
			_ = hold.Process.Kill()
			_ = hold.Wait()
		})
		c.expect(c.run("-Product", "relay"), 0, c.installed("relay"))
		expectFiles(c.t, c.dir, map[string][]byte{"relay.exe": c.r.program(c.t, "relay"), "relay.exe.prev": old})
		// Again: relay.exe.prev, still running, cannot be deleted, so it is
		// renamed aside.
		c.expect(c.run("-Product", "relay"), 0, c.installed("relay"))
		got := filesIn(c.t, c.dir)
		var aside []string
		for n, data := range got {
			if strings.HasPrefix(n, "relay.exe.old-") && string(data) == string(old) {
				aside = append(aside, n)
			}
		}
		if len(got) != 3 || len(aside) != 1 || string(got["relay.exe.prev"]) != string(c.r.program(c.t, "relay")) {
			c.t.Fatalf("after replacing a running copy twice the folder holds %v", slices.Sorted(maps.Keys(got)))
		}
	}},
}

func TestInstallPs1(t *testing.T) {
	r := sharedReleases(t)
	for _, ps := range psHosts(t) {
		t.Run(ps.name, func(t *testing.T) {
			t.Parallel()
			for _, mode := range allModes {
				t.Run(string(mode), func(t *testing.T) {
					t.Parallel()
					for _, tc := range psCases {
						if !slices.Contains(tc.modes, mode) {
							continue
						}
						t.Run(tc.name, func(t *testing.T) {
							t.Parallel()
							tc.run(newPsCase(t, ps, mode, r))
						})
					}
				})
			}
		})
	}
}

var (
	kernel32                = syscall.NewLazyDLL("kernel32.dll")
	procGenerateConsoleCtrl = kernel32.NewProc("GenerateConsoleCtrlEvent")
	procGetConsoleWindow    = kernel32.NewProc("GetConsoleWindow")
	procAllocConsole        = kernel32.NewProc("AllocConsole")
	consoleOnce             sync.Once
	consoleErr              error
)

const ctrlBreakEvent = 1 // CTRL_BREAK_EVENT

// ensureConsole gives the test process a console when it has none, as
// under a CI runner: a console control event reaches only processes that
// share the sender's console.
func ensureConsole() error {
	consoleOnce.Do(func() {
		if w, _, _ := procGetConsoleWindow.Call(); w != 0 {
			return
		}
		if r, _, err := procAllocConsole.Call(); r == 0 {
			consoleErr = fmt.Errorf("AllocConsole: %w", err)
		}
	})
	return consoleErr
}

// TestInstallPs1Interrupt sends Ctrl+Break during a download: PowerShell 7
// stops, its finally blocks remove the temporary folder, and nothing is
// installed. Windows PowerShell 5.1 does not act on Ctrl+Break, or on a
// Ctrl-C sent with GenerateConsoleCtrlEvent, in a non-interactive process,
// so it is not run here (0014-PLAN deviation D5).
func TestInstallPs1Interrupt(t *testing.T) {
	r := sharedReleases(t)
	if err := ensureConsole(); err != nil {
		t.Fatal(err)
	}
	ran := false
	for _, ps := range psHosts(t) {
		if ps.major < 7 {
			continue
		}
		ran = true
		t.Run(ps.name, func(t *testing.T) {
			c := newPsCase(t, ps, psFile, r)
			stalled := c.srv.stall(c.asset("raw", "relay"))
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			cmd := c.command(ctx)
			cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
			var out strings.Builder
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
				t.Fatalf("during the download the install folder holds %v, want only its temporary folder", during)
			}
			if r, _, err := procGenerateConsoleCtrl.Call(ctrlBreakEvent, uintptr(cmd.Process.Pid)); r == 0 {
				t.Fatalf("GenerateConsoleCtrlEvent: %v", err)
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			select {
			case err := <-done:
				if code := exitCode(t, err); code == 0 {
					t.Fatalf("exit 0 after Ctrl+Break:\n%s", out.String())
				}
			case <-time.After(time.Minute):
				t.Fatalf("still running a minute after Ctrl+Break:\n%s", out.String())
			}
			expectFiles(t, c.dir, nil)
			c.expectNoPath()
		})
	}
	if !ran {
		t.Fatal("no PowerShell 7 to interrupt")
	}
}

// TestInstallPs1Truncated runs the script cut at each line end through
// iex and [scriptblock]::Create, and cut in half as a file: nothing may
// run.
func TestInstallPs1Truncated(t *testing.T) {
	r := sharedReleases(t)
	full := r.file(t, "raw", "install.ps1")
	for _, ps := range psHosts(t) {
		t.Run(ps.name, func(t *testing.T) {
			t.Parallel()
			c := newPsCase(t, ps, psFile, r)
			script := c.writeFile("full.ps1", full)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			cmd := exec.CommandContext(ctx, ps.path, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
				"-File", c.writeFile("cuts.ps1", []byte(psCutsDriver)), "-Path", script)
			cmd.Env = c.envFor()
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("the cuts driver: %v\n%s", err, out)
			}
			if want := fmt.Sprintf("driver: ran %d cuts", strings.Count(string(full[:len(full)-1]), "\n")); !strings.Contains(string(out), want) {
				t.Fatalf("want %q in:\n%s", want, out)
			}
			c.script = full[:len(full)/2]
			if res := c.run(); res.code == 0 {
				t.Fatalf("half the script, as a file, exited 0:\n%s", res.out)
			}
			if got := c.srv.got(); len(got) != 0 {
				t.Fatalf("a cut script requested %v", got)
			}
			if entries, err := os.ReadDir(c.local); err != nil || len(entries) != 0 {
				t.Fatalf("a cut script wrote to LOCALAPPDATA: %v %v", entries, err)
			}
			c.expectNoPath()
		})
	}
}
