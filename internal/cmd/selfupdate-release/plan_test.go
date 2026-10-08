package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate/releasespec"
)

const testSHA = "0123456789abcdef0123456789abcdef01234567"

func planSpec(t *testing.T) string {
	t.Helper()
	data := `{"schema":1,
"products":[{"name":"relay","package":"./cmd/relay","identity_args":["version","--short"]},{"name":"helper","package":"./cmd/helper"}],
"platforms":[{"os":"linux","arch":"amd64"},{"os":"darwin","arch":"amd64"},{"os":"darwin","arch":"arm64"},{"os":"windows","arch":"arm64"}],
"extras":[{"name":"notes-{tag}.txt"}],
"prerelease_channels":["rc"]}`
	path := filepath.Join(t.TempDir(), "spec.json")
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPlanTag(t *testing.T) {
	r, err := plan(planInput{SpecPath: planSpec(t), RefType: "tag", RefName: "v1.4.0-rc.2", SHA: testSHA, RunAttempt: "2"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Tag != "v1.4.0-rc.2" || r.Rehearsal || r.StampVersion != "v1.4.0-rc.2" || r.StampKind != "release" ||
		r.ArtifactName != "selfupdate-release-v1.4.0-rc.2-2" || r.ExtrasJSON != `["notes-v1.4.0-rc.2.txt"]` {
		t.Fatalf("plan: %+v", r)
	}
	want := []identityLeg{
		{Product: "relay", OS: "linux", Arch: "amd64", Runner: "ubuntu-24.04", Asset: "relay-linux-amd64", Args: `["version","--short"]`},
		{Product: "relay", OS: "darwin", Arch: "arm64", Runner: "macos-15", Asset: "relay-darwin-arm64", Args: `["version","--short"]`},
		{Product: "relay", OS: "windows", Arch: "arm64", Runner: "windows-11-arm", Asset: "relay-windows-arm64.exe", Args: `["version","--short"]`},
	}
	if len(r.IdentityMatrix) != len(want) {
		t.Fatalf("matrix: %+v", r.IdentityMatrix)
	}
	for i := range want {
		if r.IdentityMatrix[i] != want[i] {
			t.Errorf("leg %d = %+v, want %+v", i, r.IdentityMatrix[i], want[i])
		}
	}
	if strings.Join(r.ToolTargets, ",") != "linux/amd64,darwin/arm64,windows/arm64" {
		t.Errorf("tool targets %v", r.ToolTargets)
	}
	summary := r.summary()
	for _, s := range []string{
		"| relay | linux/amd64 | `relay-linux-amd64` | ubuntu-24.04 |",
		"| relay | darwin/amd64 | `relay-darwin-amd64` | not run: no runner for darwin/amd64 |",
		"| helper | linux/amd64 | `helper-linux-amd64` | not run: no identity_args |",
	} {
		if !strings.Contains(summary, s) {
			t.Errorf("summary lacks %q:\n%s", s, summary)
		}
	}
	if n := strings.Count(summary, "\n| "); n != 2+8 {
		t.Errorf("summary has %d table lines, want a header, its rule and 8 rows:\n%s", n, summary)
	}
}

func TestPlanRehearsal(t *testing.T) {
	r, err := plan(planInput{SpecPath: planSpec(t), RefType: "branch", RefName: "main", SHA: testSHA, RunAttempt: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Tag != "" || !r.Rehearsal || r.StampVersion != "rehearsal-0123456789ab" || r.StampKind != "local" ||
		r.ArtifactName != "selfupdate-rehearsal-0123456789ab-1" || r.ExtrasJSON != `["notes-rehearsal-0123456789ab.txt"]` {
		t.Fatalf("plan: %+v", r)
	}
	r, err = plan(planInput{SpecPath: planSpec(t), RefType: "branch", RefName: "main", SHA: testSHA, RunAttempt: "1", ArtifactName: "mine"})
	if err != nil || r.ArtifactName != "mine" {
		t.Fatalf("an overridden name: %+v, %v", r, err)
	}
}

func TestPlanRefuses(t *testing.T) {
	spec := planSpec(t)
	for _, tc := range []struct {
		name string
		in   planInput
		want string
	}{
		{"an unlisted channel", planInput{RefType: "tag", RefName: "v1.4.0-beta.1"}, "is not an admitted release tag"},
		{"a loose tag", planInput{RefType: "tag", RefName: "1.4.0"}, "is not an admitted release tag"},
		{"a short SHA", planInput{RefType: "branch", SHA: "0123456"}, "is not a full commit SHA"},
		{"a zero attempt", planInput{RefType: "branch", RunAttempt: "0"}, "-run-attempt"},
		{"a bad artifact name", planInput{RefType: "branch", ArtifactName: "a/b"}, "artifact name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := tc.in
			in.SpecPath = spec
			if in.SHA == "" {
				in.SHA = testSHA
			}
			if in.RunAttempt == "" {
				in.RunAttempt = "1"
			}
			if _, err := plan(in); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("plan: %v; want %q", err, tc.want)
			}
		})
	}
}

// readOutputs parses a GITHUB_OUTPUT file the way the runner does.
func readOutputs(t *testing.T, path string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := sc.Text()
		name, delim, ok := strings.Cut(line, "<<")
		if !ok {
			t.Fatalf("not a multiline output: %q", line)
		}
		var value []string
		for sc.Scan() && sc.Text() != delim {
			value = append(value, sc.Text())
		}
		out[name] = strings.Join(value, "\n")
	}
	return out
}

func TestWriteOutputs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "output")
	tricky := "line one\nEOF\nghadelimiter_\nname<<x\nlast"
	if err := writeOutputs(path, []output{{"plain", "v1.2.3"}, {"tricky", tricky}, {"empty", ""}}); err != nil {
		t.Fatal(err)
	}
	got := readOutputs(t, path)
	if got["plain"] != "v1.2.3" || got["tricky"] != tricky || got["empty"] != "" || len(got) != 3 {
		t.Fatalf("outputs: %#v", got)
	}
	if err := writeOutputs("", []output{{"x", "y"}}); err != nil {
		t.Fatal(err)
	}
}

func TestRunPlanWritesOutputs(t *testing.T) {
	dir := t.TempDir()
	ghOut, summary := filepath.Join(dir, "out"), filepath.Join(dir, "summary")
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"plan", "-spec", planSpec(t), "-module-dir", libModule(t, "example.com/relay", "require "+libraryPath+" v1.10.0"),
		"-ref-type", "tag", "-ref-name", "v1.4.0", "-sha", testSHA, "-run-attempt", "1", "-github-output", ghOut, "-summary", summary}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	got := readOutputs(t, ghOut)
	var matrix []identityLeg
	if err := json.Unmarshal([]byte(got["identity-matrix"]), &matrix); err != nil || len(matrix) != 3 {
		t.Fatalf("identity-matrix %q: %v", got["identity-matrix"], err)
	}
	for name, want := range map[string]string{
		"tag": "v1.4.0", "rehearsal": "false", "stamp-kind": "release", "products-json": `["relay","helper"]`,
		"prerelease-channels-json": `["rc"]`, "tool-targets": `["linux/amd64","darwin/arm64","windows/arm64"]`,
	} {
		if got[name] != want {
			t.Errorf("%s = %q, want %q", name, got[name], want)
		}
	}
	if s, _ := os.ReadFile(summary); !bytes.Contains(s, []byte("## Self-update release plan: release v1.4.0")) {
		t.Errorf("summary: %s", s)
	}
}

func TestRunExitCodes(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		code int
		want string
	}{
		{"no subcommand", nil, 2, "usage: selfupdate-release"},
		{"unknown subcommand", []string{"publish"}, 2, `unknown subcommand "publish"`},
		{"a missing flag", []string{"plan", "-spec", "x"}, 2, "-module-dir is required"},
		{"an unknown flag", []string{"check", "-nope"}, 2, "flag provided but not defined"},
		{"an extra argument", []string{"check", "-dir", "d", "-products-json", "[]", "-platforms-json", "[]", "extra"}, 2, `unexpected argument "extra"`},
		{"a failed check", []string{"plan", "-spec", filepath.Join(t.TempDir(), "missing.json"), "-module-dir", t.TempDir(),
			"-ref-type", "tag", "-ref-name", "v1.0.0", "-sha", testSHA, "-run-attempt", "1"}, 1, "selfupdate-release plan:"},
		{"a usage error from a subcommand", []string{"build", "-spec", "s", "-module-dir", ".", "-stamp-version", "v1", "-stamp-kind", "beta", "-out", "o"}, 2, "-stamp-kind"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(context.Background(), tc.args, &stdout, &stderr); code != tc.code || !strings.Contains(stderr.String(), tc.want) {
				t.Fatalf("exit %d, stderr %q; want %d, %q", code, stderr.String(), tc.code, tc.want)
			}
		})
	}
}

// libModule is a new module directory: path, and the go.mod lines after
// its go line.
func libModule(t *testing.T, path string, lines ...string) string {
	t.Helper()
	dir := t.TempDir()
	body := "module " + path + "\n\ngo 1.27.1\n\n" + strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestPlanRefusesAnOldLibrary: a spec with a field this library's older
// releases refuse needs a module that requires one that reads it; a
// program built against an older one could not parse the spec it embeds
// (docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md F3).
func TestPlanRefusesAnOldLibrary(t *testing.T) {
	g, err := newGoTool()
	if err != nil {
		t.Fatal(err)
	}
	installer := releasespec.Spec{Installer: &releasespec.Installer{}}
	req := func(v string) string { return "require " + libraryPath + " " + v }
	for _, c := range []struct {
		name string
		spec releasespec.Spec
		dir  string
		want string // empty: accepted
		note bool
	}{
		{"v1.9.0 with installer", installer, libModule(t, "example.com/relay", req("v1.9.0")),
			`module example.com/relay requires go-selfupdate-lib v1.9.0; this spec's "installer" needs v1.10.0 or later`, false},
		{"v1.10.0", installer, libModule(t, "example.com/relay", req("v1.10.0")), "", false},
		{"v1.9.0 without installer", releasespec.Spec{}, libModule(t, "example.com/relay", req("v1.9.0")), "", false},
		{"a pseudo-version above v1.10.0", installer, libModule(t, "example.com/relay", req("v1.10.1-0.20261008000000-0123456789ab")), "", false},
		{"v1.10.0-rc.1", installer, libModule(t, "example.com/relay", req("v1.10.0-rc.1")), "requires go-selfupdate-lib v1.10.0-rc.1", false},
		{"a directory replace", installer, libModule(t, "example.com/relay", req("v0.0.0"), "replace "+libraryPath+" => ../lib"), "", true},
		{"a module replace at v1.9.0", installer, libModule(t, "example.com/relay", req("v1.11.0"), "replace "+libraryPath+" => example.com/fork v1.9.0"),
			"requires go-selfupdate-lib v1.9.0", false},
		{"the library itself", installer, libModule(t, libraryPath), "", false},
		{"no requirement", installer, libModule(t, "example.com/relay"), "does not require " + libraryPath, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			note, err := checkLibraryFloor(context.Background(), g, c.dir, c.spec)
			switch {
			case c.want == "" && err != nil:
				t.Fatalf("refused: %v", err)
			case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
				t.Fatalf("err = %v; want %q", err, c.want)
			case (note != "") != c.note:
				t.Fatalf("note %q; want one %t", note, c.note)
			}
		})
	}
}

// v190SpecPaths are the JSON paths of v1.9.0's Spec, from its source
// (git show v1.9.0:selfupdate/releasespec/spec.go).
var v190SpecPaths = []string{
	"schema", "products", "products.name", "products.package", "products.tags", "products.identity_args",
	"platforms", "platforms.os", "platforms.arch", "platforms.format", "packaging",
	"extras", "extras.name", "extras.path", "prerelease_channels",
}

// jsonPaths lists a struct type's JSON field paths, into nested structs,
// pointers to them and slices of them.
func jsonPaths(t reflect.Type, prefix string) []string {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}
	var out []string
	for i := range t.NumField() {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
		if name == "" || name == "-" {
			continue
		}
		path := prefix + name
		out = append(out, path)
		out = append(out, jsonPaths(t.Field(i).Type, path+".")...)
	}
	return out
}

// TestSpecFloorsCoverEveryField: every field v1.9.0's Spec lacks has a
// floor, so a spec using it is checked against the module's requirement
// (0015-MADR F3).
func TestSpecFloorsCoverEveryField(t *testing.T) {
	for _, p := range jsonPaths(reflect.TypeFor[releasespec.Spec](), "") {
		if slices.Contains(v190SpecPaths, p) {
			continue
		}
		if !slices.ContainsFunc(specFloors, func(f specFloor) bool { return p == f.field || strings.HasPrefix(p, f.field+".") }) {
			t.Errorf("%s is not in v1.9.0's Spec and has no floor", p)
		}
	}
}
