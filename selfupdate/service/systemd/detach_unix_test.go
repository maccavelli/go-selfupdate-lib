//go:build unix

package systemd

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// Tests for docs/decisions/0011-PLAN-reference-service-lifecycles.md V2
// step 6 and amendment A2: Detach and its environment file. They exercise
// the Linux paths and file modes, so they run on Unix; New refuses
// elsewhere.

func handOffSpec(t *testing.T) service.HandOff {
	t.Helper()
	return service.HandOff{
		ID: "abc123", Executable: "/opt/demo/demo", Args: []string{"update", "-y"},
		Env: []string{service.EnvHandOff + "=abc123", "SECRET_TOKEN=s3cret"}, ResultPath: "/opt/demo/.demo.selfupdate.handoff",
	}
}

func TestDetach(t *testing.T) {
	f := newFake()
	u := testUnit(t, f, Options{Scope: User, Unit: "relay@eu.service"})
	var envFile string
	det, err := u.Detach(context.Background(), handOffSpec(t))
	if err != nil {
		t.Fatal(err)
	}
	run := f.calls[len(f.calls)-1]
	want := []string{"/usr/bin/systemd-run", "--user", "--unit=relay-eu-selfupdate-abc123.service", "--collect", "--quiet",
		"--description=selfupdate handoff for relay@eu.service"}
	if !slices.Equal(run[:len(want)], want) {
		t.Fatalf("systemd-run %q", run)
	}
	for i, a := range run {
		if a == "-p" && strings.HasPrefix(run[i+1], "EnvironmentFile=") {
			envFile = strings.TrimPrefix(run[i+1], "EnvironmentFile=")
		}
	}
	if !slices.Contains(run, "Type=exec") || slices.Contains(run, "--no-block") ||
		!slices.Equal(run[len(run)-4:], []string{"--", "/opt/demo/demo", "update", "-y"}) {
		t.Fatalf("systemd-run %q", run)
	}
	for _, a := range run {
		if strings.Contains(a, "s3cret") {
			t.Fatalf("a secret reached the command line: %q", run)
		}
	}
	// Each unit's handoffs keep to its own directory (0015-MADR D8).
	if !strings.HasPrefix(envFile, u.getenv("XDG_RUNTIME_DIR")+"/selfupdate/relay@eu.service/handoff-abc123.env") {
		t.Fatalf("environment file %q", envFile)
	}
	if _, err := os.Stat(envFile); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the environment file was kept after a Type=exec start: %v", err)
	}
	if det.Where != "transient unit relay-eu-selfupdate-abc123.service" || det.ResultPath != handOffSpec(t).ResultPath {
		t.Fatalf("detached %+v", det)
	}
}

// TestDetachOldSystemd: before 240 there is no Type=exec, so the run is
// started without blocking and the file stays; before 236, no handoff.
func TestDetachOldSystemd(t *testing.T) {
	f := newFake()
	f.version = "systemd 239 (239-68.el8)"
	u := testUnit(t, f, Options{Scope: User})
	if _, err := u.Detach(context.Background(), handOffSpec(t)); err != nil {
		t.Fatal(err)
	}
	run := f.calls[len(f.calls)-1]
	if !slices.Contains(run, "--no-block") || slices.Contains(run, "Type=exec") {
		t.Fatalf("systemd-run %q", run)
	}
	file := filepath.Join(u.getenv("XDG_RUNTIME_DIR"), "selfupdate", "demo.service", "handoff-abc123.env")
	body := readFile(t, file)
	if !strings.Contains(body, `SECRET_TOKEN="s3cret"`) || !strings.Contains(body, service.EnvHandOff+`="abc123"`) {
		t.Fatalf("environment file %q", body)
	}
	info, err := os.Stat(file)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("environment file mode %v, %v", info.Mode(), err)
	}
	dir, err := os.Stat(filepath.Dir(file))
	if err != nil || dir.Mode().Perm() != 0o700 {
		t.Fatalf("directory mode %v, %v", dir.Mode(), err)
	}
	// The next handoff removes the file the last one left.
	spec := handOffSpec(t)
	spec.ID = "def456"
	if _, err := u.Detach(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("an earlier handoff's file was kept: %v", err)
	}

	f.version = "systemd 235"
	if _, err := u.Detach(context.Background(), handOffSpec(t)); !errors.Is(err, service.ErrUnsupported) {
		t.Fatalf("systemd 235: %v", err)
	}
	f.version = "garbage"
	if _, err := u.Detach(context.Background(), handOffSpec(t)); err == nil {
		t.Fatal("an unreadable version was accepted")
	}
}

func TestDetachFailureRemovesFile(t *testing.T) {
	f := newFake()
	f.fail["systemd-run"] = service.Output{ExitCode: 1, Stderr: []byte("Failed to start transient service unit: Access denied")}
	u := testUnit(t, f, Options{Scope: User})
	if _, err := u.Detach(context.Background(), handOffSpec(t)); !errors.Is(err, service.ErrPermission) {
		t.Fatalf("err = %v", err)
	}
	entries, _ := os.ReadDir(filepath.Join(u.getenv("XDG_RUNTIME_DIR"), "selfupdate", "demo.service"))
	if len(entries) != 0 {
		t.Fatalf("a failed handoff left %v", entries)
	}
}

// TestDetachScrubsManagerEnv: the environment file leaves out the caller's
// unit's own variables, keeps the rest, and keeps one spec.Env names
// (0033-MADR D3-A, D5-A). systemd 239 keeps the file to read.
func TestDetachScrubsManagerEnv(t *testing.T) {
	t.Setenv("NOTIFY_SOCKET", "/run/caller/notify")
	t.Setenv("INVOCATION_ID", "caller0000")
	t.Setenv("RUNTIME_DIRECTORY", "/run/caller")
	t.Setenv("STATE_DIRECTORY", "/var/lib/demo")
	f := newFake()
	f.version = "systemd 239 (239-68.el8)"
	u := testUnit(t, f, Options{Scope: User})
	spec := handOffSpec(t)
	spec.Env = append(spec.Env, "NOTIFY_SOCKET=/run/explicit")
	if _, err := u.Detach(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	body := readFile(t, filepath.Join(u.getenv("XDG_RUNTIME_DIR"), "selfupdate", "demo.service", "handoff-abc123.env"))
	lines := strings.Split(body, "\n")
	for _, want := range []string{`STATE_DIRECTORY="/var/lib/demo"`, `NOTIFY_SOCKET="/run/explicit"`} {
		if !slices.Contains(lines, want) {
			t.Errorf("environment file lacks %s:\n%s", want, body)
		}
	}
	for _, gone := range []string{"INVOCATION_ID=", "RUNTIME_DIRECTORY=", `NOTIFY_SOCKET="/run/caller`} {
		if strings.Contains(body, gone) {
			t.Errorf("environment file holds the caller's %s:\n%s", gone, body)
		}
	}
}

// carriedPlan is a plan whose document holds quotes, a dollar sign and an
// ampersand, as a carrier must keep them
// (docs/decisions/0038-PLAN-pinned-plans-lifecycle-conformance-and-bubble-tests.md
// Deviation D15).
func carriedPlan() selfupdate.Plan {
	digest := strings.Repeat("ab", 32)
	return selfupdate.Plan{
		Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: selfupdate.ReleaseBuild,
		Platform: selfupdate.Platform{OS: "linux", Arch: "amd64"}, TargetVersion: "v1.1.0", ReleaseID: 7,
		ReleaseURL: `https://example.invalid/r?a="b"&c=$d\e`, Latest: true, Operation: selfupdate.OperationUpgrade,
		Selection: selfupdate.Selection{
			Binary:       selfupdate.Asset{ID: 8, Name: "demo-linux-amd64", State: selfupdate.AssetStateUploaded, Size: 12, Digest: "sha256:" + digest},
			Manifest:     selfupdate.Asset{ID: 9, Name: "SHA256SUMS", State: selfupdate.AssetStateUploaded, Size: 83},
			ManifestName: "demo-linux-amd64",
		},
		ManifestSHA256: digest,
	}
}

// planEnv is the HandOff.Env entry service.HandOffPlanFunc adds for p.
func planEnv(t *testing.T, p selfupdate.Plan) string {
	t.Helper()
	doc, err := json.Marshal(p.Document())
	if err != nil {
		t.Fatal(err)
	}
	return service.EnvHandOffPlan + "=" + string(doc)
}

// planFrom finds the plan's entry in vars and parses it.
func planFrom(t *testing.T, vars []string) selfupdate.Plan {
	t.Helper()
	for _, kv := range vars {
		if doc, ok := strings.CutPrefix(kv, service.EnvHandOffPlan+"="); ok {
			p, err := selfupdate.ParsePlan([]byte(doc))
			if err != nil {
				t.Fatalf("the carried plan: %v", err)
			}
			return p
		}
	}
	t.Fatalf("no %s among %d variables", service.EnvHandOffPlan, len(vars))
	return selfupdate.Plan{}
}

// unquoteEnvFile reads the NAME="value" lines envFileBody writes, undoing
// its backslash escapes, as systemd reads an EnvironmentFile.
func unquoteEnvFile(t *testing.T, body string) []string {
	t.Helper()
	var vars []string
	for line := range strings.Lines(body) {
		k, q, ok := strings.Cut(strings.TrimSuffix(line, "\n"), "=")
		if !ok || len(q) < 2 || q[0] != '"' || q[len(q)-1] != '"' {
			t.Fatalf("environment file line %q", line)
		}
		var v strings.Builder
		for i := 1; i < len(q)-1; i++ {
			if q[i] == '\\' && i+1 < len(q)-1 {
				i++
			}
			v.WriteByte(q[i])
		}
		vars = append(vars, k+"="+v.String())
	}
	return vars
}

// TestDetachCarriesPlan: the plan's entry reaches the EnvironmentFile, and
// reads back as the same plan.
func TestDetachCarriesPlan(t *testing.T) {
	f := newFake()
	f.version = "systemd 239 (239-68.el8)"
	u := testUnit(t, f, Options{Scope: User})
	spec := handOffSpec(t)
	p := carriedPlan()
	spec.Env = append(slices.Clone(spec.Env), planEnv(t, p))
	if _, err := u.Detach(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(u.getenv("XDG_RUNTIME_DIR"), "selfupdate", "demo.service", "handoff-abc123.env")
	if got := planFrom(t, unquoteEnvFile(t, readFile(t, file))); got != p {
		t.Fatalf("the EnvironmentFile carries %+v, want %+v", got, p)
	}
}
