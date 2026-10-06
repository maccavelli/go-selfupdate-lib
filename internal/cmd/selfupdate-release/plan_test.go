package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	code := run(context.Background(), []string{"plan", "-spec", planSpec(t), "-ref-type", "tag", "-ref-name", "v1.4.0",
		"-sha", testSHA, "-run-attempt", "1", "-github-output", ghOut, "-summary", summary}, &stdout, &stderr)
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
		{"a missing flag", []string{"plan", "-spec", "x"}, 2, "-ref-type is required"},
		{"an unknown flag", []string{"check", "-nope"}, 2, "flag provided but not defined"},
		{"an extra argument", []string{"check", "-dir", "d", "-products-json", "[]", "-platforms-json", "[]", "extra"}, 2, `unexpected argument "extra"`},
		{"a failed check", []string{"plan", "-spec", filepath.Join(t.TempDir(), "missing.json"), "-ref-type", "tag", "-ref-name", "v1.0.0",
			"-sha", testSHA, "-run-attempt", "1"}, 1, "selfupdate-release plan:"},
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
