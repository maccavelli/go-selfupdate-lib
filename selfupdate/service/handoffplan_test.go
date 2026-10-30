package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/internal/testproc"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
)

// Tests for the plan a handoff carries
// (docs/decisions/0038-MADR-pinned-plans-lifecycle-conformance-and-bubble-tests.md
// D1-A; 0038-PLAN Q16).

// handOffPlan is a valid plan, as Updater.Plan would make one.
func handOffPlan() selfupdate.Plan {
	sum := sha256.Sum256([]byte("demo v1.1.0\n"))
	digest := hex.EncodeToString(sum[:])
	return selfupdate.Plan{
		Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: selfupdate.ReleaseBuild,
		Platform: selfupdate.Platform{OS: "linux", Arch: "amd64"}, TargetVersion: "v1.1.0", ReleaseID: 7,
		ReleaseURL: `https://example.invalid/r?a="b"&c=$d`, Latest: true, Operation: selfupdate.OperationUpgrade,
		Selection: selfupdate.Selection{
			Binary:       selfupdate.Asset{ID: 8, Name: "demo-linux-amd64", State: selfupdate.AssetStateUploaded, Size: 12, Digest: "sha256:" + digest},
			Manifest:     selfupdate.Asset{ID: 9, Name: "SHA256SUMS", State: selfupdate.AssetStateUploaded, Size: 83},
			ManifestName: "demo-linux-amd64",
		},
		ManifestSHA256: digest,
	}
}

// TestHandOffPlanFuncAddsThePlan: inside the service, the hook hands off
// with the plan's document in EnvHandOffPlan, after spec's own Env, and
// leaves spec's slice as it was.
func TestHandOffPlanFuncAddsThePlan(t *testing.T) {
	testproc.CheckNoLeak(t)
	t.Setenv(EnvHandOff, "")
	d := &fakeDetacher{inside: true}
	env := []string{"KEEP=1"}
	spec := HandOff{ID: "abc", Env: env[:1:1], ResultPath: t.TempDir() + "/result"}
	p := handOffPlan()
	handed, detail, err := HandOffPlanFunc(d, spec)(context.Background(), selfupdate.Request{Yes: true}, p)
	if err != nil || !handed || !strings.Contains(detail, "abc") {
		t.Fatalf("handed %t, detail %q, err %v", handed, detail, err)
	}
	got := d.got.Env
	if n := len(got); n < 2 || got[n-2] != "KEEP=1" || !strings.HasPrefix(got[n-1], EnvHandOffPlan+"=") {
		t.Fatalf("Env %q, want KEEP=1 then the plan", got)
	}
	doc := strings.TrimPrefix(got[len(got)-1], EnvHandOffPlan+"=")
	if strings.ContainsAny(doc, "\n\t") {
		t.Fatalf("the plan document is not compact: %q", doc)
	}
	back, err := selfupdate.ParsePlan([]byte(doc))
	if err != nil || back != p {
		t.Fatalf("the carried plan parses to %+v, %v; want %+v", back, err, p)
	}
	if !slices.Equal(env, []string{"KEEP=1"}) || len(spec.Env) != 1 {
		t.Fatalf("spec's Env changed: %q", spec.Env)
	}
}

// TestHandOffPlanFuncRules: HandOffFunc's rules hold: outside the service
// nothing is handed off, and inside a request without Yes is refused.
func TestHandOffPlanFuncRules(t *testing.T) {
	testproc.CheckNoLeak(t)
	t.Setenv(EnvHandOff, "")
	outside := &fakeDetacher{}
	if handed, _, err := HandOffPlanFunc(outside, HandOff{})(context.Background(), selfupdate.Request{Yes: true}, handOffPlan()); err != nil || handed || outside.got != nil {
		t.Fatalf("outside: handed %t, err %v", handed, err)
	}
	inside := &fakeDetacher{inside: true}
	_, _, err := HandOffPlanFunc(inside, HandOff{})(context.Background(), selfupdate.Request{}, handOffPlan())
	if !errors.Is(err, selfupdate.ErrConfirmationRequired) || inside.got != nil {
		t.Fatalf("inside without Yes: err %v, detached %t", err, inside.got != nil)
	}
}

// TestLoadHandOffPlan: no variable is no plan; a valid one is the plan,
// and is unset; an invalid one is an error, and is unset too.
func TestLoadHandOffPlan(t *testing.T) {
	t.Setenv(EnvHandOffPlan, "")
	if err := os.Unsetenv(EnvHandOffPlan); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := LoadHandOffPlan(); ok || err != nil {
		t.Fatalf("unset: ok %t, err %v", ok, err)
	}
	t.Setenv(EnvHandOffPlan, "")
	if _, ok, err := LoadHandOffPlan(); ok || err != nil {
		t.Fatalf("empty: ok %t, err %v", ok, err)
	}
	if _, set := os.LookupEnv(EnvHandOffPlan); set {
		t.Fatal("the empty variable is still set")
	}
	p := handOffPlan()
	doc, err := json.Marshal(p.Document())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvHandOffPlan, string(doc))
	got, ok, err := LoadHandOffPlan()
	if err != nil || !ok || got != p {
		t.Fatalf("LoadHandOffPlan = %+v, %t, %v; want %+v", got, ok, err, p)
	}
	if _, set := os.LookupEnv(EnvHandOffPlan); set {
		t.Fatal("the variable is still set")
	}
	tampered := strings.Replace(string(doc), `"schema_version":1`, `"schema_version":2`, 1)
	t.Setenv(EnvHandOffPlan, tampered)
	if _, ok, err := LoadHandOffPlan(); ok || err == nil || !strings.Contains(err.Error(), "schema_version") {
		t.Fatalf("a tampered plan: ok %t, err %v", ok, err)
	}
	if _, set := os.LookupEnv(EnvHandOffPlan); set {
		t.Fatal("the tampered variable is still set")
	}
}

// TestHandOffPlanThroughEnvFile: the private environment file launchd and
// Task Scheduler carry HandOff.Env in brings the plan to the detached run,
// where LoadHandOffEnv then LoadHandOffPlan read it back unchanged.
func TestHandOffPlanThroughEnvFile(t *testing.T) {
	p := handOffPlan()
	doc, err := json.Marshal(p.Document())
	if err != nil {
		t.Fatal(err)
	}
	file, err := WriteHandOffEnv(t.TempDir(), "abc", []string{EnvHandOffPlan + "=" + string(doc)})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvHandOffPlan, "")
	if err := os.Unsetenv(EnvHandOffPlan); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvHandOffEnv, file)
	if err := LoadHandOffEnv(); err != nil {
		t.Fatal(err)
	}
	got, ok, err := LoadHandOffPlan()
	if err != nil || !ok || got != p {
		t.Fatalf("LoadHandOffPlan after the file = %+v, %t, %v; want %+v", got, ok, err, p)
	}
}
