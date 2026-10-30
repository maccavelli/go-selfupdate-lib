package scm

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// fakeEnv makes the test binary the detached run of TestDetach: it writes
// "<ppid> <hop marker seen> <FAKE_SECRET>" to FAKE_OUT, and, when
// FAKE_PLAN_OUT is set, service.EnvHandOffPlan's value to it first.
const fakeEnv = "SELFUPDATE_SCM_FAKE"

func fakeRun() int {
	if planOut := os.Getenv("FAKE_PLAN_OUT"); planOut != "" {
		if err := os.WriteFile(planOut, []byte(os.Getenv(service.EnvHandOffPlan)), 0o600); err != nil {
			return 5
		}
	}
	out := os.Getenv("FAKE_OUT")
	_, marked := os.LookupEnv(service.EnvHandOffHop)
	body := strconv.Itoa(os.Getppid()) + " " + strconv.FormatBool(marked) + " " + os.Getenv("FAKE_SECRET")
	if err := os.WriteFile(out+".tmp", []byte(body), 0o600); err != nil {
		return 3
	}
	if err := os.Rename(out+".tmp", out); err != nil {
		return 4
	}
	return 0
}

// TestDetach: Detach starts a hop, and the hop the real run, which is not
// this process's child, does not see the marker, and has the handoff's
// environment (0011-MADR amendment A4).
func TestDetach(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "run")
	s := testService(t, newFake(), Options{})
	det, err := s.Detach(context.Background(), service.HandOff{
		ID: "abc", Executable: exe, ResultPath: filepath.Join(t.TempDir(), "result"),
		Env: []string{fakeEnv + "=1", "FAKE_OUT=" + out, "FAKE_SECRET=s3cret"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(det.Where, "hop") || det.ID != "abc" {
		t.Fatalf("detached %+v", det)
	}
	var body []byte
	for deadline := time.Now().Add(30 * time.Second); ; {
		if body, err = os.ReadFile(out); err == nil { //nolint:gosec // the test's own file, written by rename
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the real run never reported")
		}
		time.Sleep(20 * time.Millisecond)
	}
	f := strings.Fields(string(body))
	if len(f) != 3 || f[0] == strconv.Itoa(os.Getpid()) || f[1] != "false" || f[2] != "s3cret" {
		t.Fatalf("the real run reported %q", body)
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

// TestDetachCarriesPlan: the real run's environment holds the plan's
// entry, which reads back as the same plan.
func TestDetachCarriesPlan(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	out, planOut := filepath.Join(dir, "run"), filepath.Join(dir, "plan")
	p := carriedPlan()
	s := testService(t, newFake(), Options{})
	if _, err := s.Detach(context.Background(), service.HandOff{
		ID: "abc", Executable: exe, ResultPath: filepath.Join(dir, "result"),
		Env: []string{fakeEnv + "=1", "FAKE_OUT=" + out, "FAKE_PLAN_OUT=" + planOut, planEnv(t, p)},
	}); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(30 * time.Second); ; {
		if _, err := os.Stat(out); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the real run never reported")
		}
		time.Sleep(20 * time.Millisecond)
	}
	doc, err := os.ReadFile(planOut) //nolint:gosec // the test's own file
	if err != nil {
		t.Fatal(err)
	}
	if got := planFrom(t, []string{service.EnvHandOffPlan + "=" + string(doc)}); got != p {
		t.Fatalf("the real run's environment carries %+v, want %+v", got, p)
	}
}
