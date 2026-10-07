package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate/releasespec"
)

// Tests for docs/decisions/0014-PLAN-shared-installer-templates.md I2: the
// templates and the renderer (0014-MADR §3).

// installerSpec is a spec with installers for the given platforms.
func installerSpec(t *testing.T, platforms []map[string]string, installer map[string]any) releasespec.Spec {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"schema": 1,
		"products": []any{
			map[string]any{"name": "relay", "package": "./cmd/relay", "identity_args": []string{"version"}},
			map[string]any{"name": "relayctl", "package": "./cmd/relay"},
		},
		"platforms":           platforms,
		"prerelease_channels": []string{"rc"},
		"installer":           installer,
	})
	if err != nil {
		t.Fatal(err)
	}
	spec, err := releasespec.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return spec
}

var mixedPlatforms = []map[string]string{
	{"os": "linux", "arch": "amd64"}, {"os": "darwin", "arch": "arm64"}, {"os": "windows", "arch": "amd64"},
}

func template(t *testing.T, name string) []byte {
	t.Helper()
	data, err := installerTemplates.ReadFile("installer/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// outsideBlock is the text before the start marker and after the end
// marker: what the renderer must not touch.
func outsideBlock(t *testing.T, data []byte) string {
	t.Helper()
	s := string(data)
	start := strings.Index(s, blockStart)
	end := strings.Index(s, blockEnd)
	if start < 0 || end < start {
		t.Fatalf("no value block in %q…", s[:min(len(s), 80)])
	}
	return s[:start] + s[end:]
}

func TestRenderInstallers(t *testing.T) {
	spec := installerSpec(t, mixedPlatforms, map[string]any{
		"hooks": []any{map[string]any{"when": "after_install", "product": "relay", "args": []string{"configure", "--encrypt-db=true"}}},
	})
	files, err := renderInstallers(spec, "maccavelli/relay-suite", "v1.2.3-rc.1")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("rendered %d files", len(files))
	}
	sh := string(files[releasespec.InstallerScript])
	for _, want := range []string{
		"REPOSITORY='maccavelli/relay-suite'\n",
		"TAG='v1.2.3-rc.1'\n",
		"CHANNELS='rc'\n",
		"INSTALLER_NAME='relay-suite'\n",
		"ENV_PREFIX='RELAY_SUITE'\n",
		"PRODUCTS='relay relayctl'\n",
		"ASSETS='relay linux amd64 relay-linux-amd64 binary\nrelay darwin arm64 relay-darwin-arm64 binary\nrelayctl linux amd64 relayctl-linux-amd64 binary\nrelayctl darwin arm64 relayctl-darwin-arm64 binary'\n",
		"IDENTITY='relay version'\n",
		"HOOKS='after_install relay configure --encrypt-db=true'\n",
	} {
		if !strings.Contains(sh, want) {
			t.Errorf("install.sh lacks %q", want)
		}
	}
	if strings.Contains(sh, "windows") {
		t.Error("install.sh lists a Windows asset")
	}
	ps := string(files[releasespec.InstallerPowerShell])
	for _, want := range []string{
		"        $Repository = 'maccavelli/relay-suite'\n",
		"        $Channels = @('rc')\n",
		"        $Products = @('relay', 'relayctl')\n",
		"            @{ Product = 'relay'; Arch = 'amd64'; Name = 'relay-windows-amd64.exe'; Format = 'binary' }\n",
		"        $Identity = @{ 'relay' = @('version') }\n",
		"            @{ When = 'after_install'; Product = 'relay'; Args = @('configure', '--encrypt-db=true') }\n",
	} {
		if !strings.Contains(ps, want) {
			t.Errorf("install.ps1 lacks %q", want)
		}
	}
	if strings.Contains(ps, "darwin") || strings.Contains(ps, "linux") {
		t.Error("install.ps1 lists a Unix asset")
	}
	for name, data := range files {
		if outsideBlock(t, data) != outsideBlock(t, template(t, name)) {
			t.Errorf("%s: the renderer changed text outside the value block", name)
		}
		if bytes.ContainsRune(data, '\r') || bytes.HasPrefix(data, []byte("\xef\xbb\xbf")) {
			t.Errorf("%s: CR or BOM in the output", name)
		}
	}
}

func TestRenderPicksScriptsByPlatform(t *testing.T) {
	for _, tc := range []struct {
		platforms []map[string]string
		want      []string
	}{
		{[]map[string]string{{"os": "linux", "arch": "amd64"}}, []string{"install.sh"}},
		{[]map[string]string{{"os": "windows", "arch": "arm64"}}, []string{"install.ps1"}},
	} {
		files, err := renderInstallers(installerSpec(t, tc.platforms, map[string]any{}), "o/r", "v1.0.0")
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for name := range files {
			got = append(got, name)
		}
		if len(got) != 1 || got[0] != tc.want[0] {
			t.Errorf("%v: rendered %v, want %v", tc.platforms, got, tc.want)
		}
	}
	files, err := renderInstallers(installerSpec(t, mixedPlatforms, nil), "o/r", "v1.0.0")
	if err != nil || len(files) != 0 {
		t.Errorf("a spec without installer rendered %d files: %v", len(files), err)
	}
}

func TestRenderRefuses(t *testing.T) {
	spec := installerSpec(t, mixedPlatforms, map[string]any{})
	for _, tc := range []struct{ repository, tag, want string }{
		{"maccavelli", "v1.0.0", "must be owner/name"},
		{"o/r'x", "v1.0.0", "must be owner/name"},
		{"o/r x", "v1.0.0", "must be owner/name"},
		{"o/r", "v1.0.0'", "tag"},
		{"o/r", "v1 0", "tag"},
	} {
		if _, err := renderInstallers(spec, tc.repository, tc.tag); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("render(%q, %q): %v; want %q", tc.repository, tc.tag, err, tc.want)
		}
	}
	// A value the spec would admit but the scripts could not carry is
	// refused by the renderer's own check, whatever validated it before.
	v := installerValues{repository: "o/r", tag: "v1.0.0", name: "r", envPrefix: "R", products: []string{"a b"}}
	if err := v.check(); err == nil || !strings.Contains(err.Error(), `"a b"`) {
		t.Errorf("check: %v", err)
	}
	// The prefix made from a repository name that starts with a digit.
	if _, err := renderInstallers(spec, "o/1password", "v1.0.0"); err == nil || !strings.Contains(err.Error(), "set installer.env_prefix") {
		t.Errorf("render with a digit-led name: %v", err)
	}
}

func TestReplaceBlock(t *testing.T) {
	good := "#!/bin/sh\n  " + blockStart + "\n  X='sample'\n  " + blockEnd + "\nmain\n"
	got, err := replaceBlock([]byte(good), []string{"X='real'", "Y='a\nb'"})
	if err != nil {
		t.Fatal(err)
	}
	want := "#!/bin/sh\n  " + blockStart + "\n  # Generated by go-selfupdate-lib's build workflow from the release spec.\n  X='real'\n  Y='a\n  b'\n  " + blockEnd + "\nmain\n"
	if string(got) != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	for _, tc := range []struct{ name, text, want string }{
		{"no block", "#!/bin/sh\nmain\n", "no value block"},
		{"two starts, one end", blockStart + "\n" + blockStart + "\n" + blockEnd + "\n", "two value blocks"},
		{"one start, two ends", blockStart + "\n" + blockEnd + "\n" + blockEnd + "\n", "two value block ends"},
		{"end first", blockEnd + "\n" + blockStart + "\n", "no value block"},
		{"CR endings", strings.ReplaceAll(good, "\n", "\r\n"), "CR line endings"},
	} {
		if _, err := replaceBlock([]byte(tc.text), nil); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: replaceBlock: %v; want %q", tc.name, err, tc.want)
		}
	}
}

// localRe finds `local` used as a command, which POSIX does not define and
// ksh lacks (0014-MADR §4), and not `.local/bin`.
var localRe = regexp.MustCompile(`(?m)(^|[;&|{(]|\bthen|\bdo|\belse)\s*local\s`)

// iwrRe finds each line that calls Invoke-WebRequest, by name or by its
// aliases, outside a comment.
var iwrRe = regexp.MustCompile(`(?im)^[^#\n]*\b(Invoke-WebRequest|iwr|curl|wget)\b[^\n]*$`)

func TestTemplatesAsWritten(t *testing.T) {
	sh := template(t, releasespec.InstallerScript)
	if m := localRe.Find(sh); m != nil {
		t.Errorf("install.sh uses local: %q", m)
	}
	if !bytes.HasSuffix(bytes.TrimRight(sh, "\n"), []byte("\nmain \"$@\"")) {
		t.Error(`install.sh does not end by calling main "$@"`)
	}
	ps := template(t, releasespec.InstallerPowerShell)
	if !bytes.HasSuffix(bytes.TrimRight(ps, "\n"), []byte("\n} @args")) {
		t.Error("install.ps1 does not end by invoking its one scriptblock")
	}
	// Windows PowerShell 5.1 prompts, or fails non-interactively, when
	// Invoke-WebRequest keeps a response in memory without
	// -UseBasicParsing; -OutFile avoids it today, so no run shows the
	// flag missing (0014-PLAN deviation D6).
	calls := iwrRe.FindAll(ps, -1)
	if len(calls) == 0 {
		t.Error("install.ps1 has no Invoke-WebRequest")
	}
	for _, call := range calls {
		if !bytes.Contains(call, []byte("-UseBasicParsing")) {
			t.Errorf("install.ps1: %q lacks -UseBasicParsing", bytes.TrimSpace(call))
		}
	}
	for name, data := range map[string][]byte{"install.sh": sh, "install.ps1": ps} {
		if bytes.ContainsRune(data, '\r') || bytes.HasPrefix(data, []byte("\xef\xbb\xbf")) {
			t.Errorf("%s: CR or BOM in the template", name)
		}
	}
	// The shells the host has must parse both the template and a render.
	files, err := renderInstallers(installerSpec(t, mixedPlatforms, map[string]any{}), "o/r", "v1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	rendered := filepath.Join(dir, "install.sh")
	if err := os.WriteFile(rendered, files["install.sh"], 0o644); err != nil {
		t.Fatal(err)
	}
	ran := 0
	for _, shell := range []string{"sh", "dash", "bash"} {
		path, err := exec.LookPath(shell)
		if err != nil {
			continue
		}
		ran++
		for _, f := range []string{filepath.Join("installer", "install.sh"), rendered} {
			if out, err := exec.Command(path, "-n", f).CombinedOutput(); err != nil {
				t.Errorf("%s -n %s: %v\n%s", shell, f, err, out)
			}
		}
	}
	if ran == 0 && runtime.GOOS != "windows" {
		t.Fatal("no POSIX shell found")
	}
}

func TestStageWritesInstallers(t *testing.T) {
	b := sharedBuild(t)
	data, err := specJSON("binary", platformObjects(testPlatforms(), nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	raw["installer"] = map[string]any{}
	data, err = json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	spec := filepath.Join(t.TempDir(), "spec.json")
	if err := os.WriteFile(spec, data, 0o644); err != nil {
		t.Fatal(err)
	}
	_, out, err := stageWith(t, b, spec, b.bin, func(in *stageInput) { in.Repository = "maccavelli/relay" })
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"install.sh", "install.ps1"} {
		got, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatalf("%s not staged: %v", name, err)
		}
		if !bytes.Contains(got, []byte("maccavelli/relay")) || !bytes.Contains(got, []byte(fixtureTag)) {
			t.Errorf("%s does not name the repository and tag", name)
		}
	}
	sums, err := os.ReadFile(filepath.Join(out, "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sums, []byte("install.")) {
		t.Error("SHA256SUMS lists an installer")
	}
	if _, _, err := stageWith(t, b, spec, b.bin, nil); err == nil || !strings.Contains(err.Error(), "-repository is required") {
		t.Errorf("stage without -repository: %v", err)
	}
}

func TestRunInstaller(t *testing.T) {
	specPath := filepath.Join("..", "..", "..", "selfupdate", "releasespec", "testdata", "installer.json")
	out := t.TempDir()
	if code := run(context.Background(), []string{"installer", "-spec", specPath, "-repository", "o/r", "-tag", "v1.0.0", "-out", out}, io.Discard, io.Discard); code != 0 {
		t.Fatalf("exit %d", code)
	}
	for _, name := range []string{"install.sh", "install.ps1"} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	noInstaller := filepath.Join("..", "..", "..", "selfupdate", "releasespec", "testdata", "minimal.json")
	var stderr bytes.Buffer
	if code := run(context.Background(), []string{"installer", "-spec", noInstaller, "-repository", "o/r", "-tag", "v1.0.0", "-out", out}, io.Discard, &stderr); code != 2 {
		t.Fatalf("a spec without installer: exit %d, %s", code, stderr.String())
	}
}
