package ghattest_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/selfupdatetest"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/verify/ghattest"
)

// A program refuses a release its own workflow did not attest: the check
// runs on SHA256SUMS, before any binary is downloaded, and needs gh logged
// in on the machine that updates.
func ExampleNewManifestVerifier() {
	v, err := ghattest.NewManifestVerifier(ghattest.Options{
		Policy: ghattest.Policy{
			// The program's own repository; the signer is this module's
			// publish workflow, which its release workflow calls.
			Repository: selfupdate.Repository{Owner: "example", Name: "relay"},
		},
		// gh's absolute path: nothing is looked up on PATH.
		GH: "/usr/local/bin/gh",
	})
	if err != nil {
		panic(err)
	}
	cfg := selfupdate.Config{
		// Source, Versions, Assets, Installer, Reporter and Confirmer as usual.
		ManifestVerifiers: []selfupdate.ManifestVerifier{v},
	}
	_ = cfg
}

// TestUpdaterRefusesUnattested: an update whose SHA256SUMS gh does not
// verify fails with ErrIntegrity, and downloads no binary
// (docs/decisions/0017-PLAN-verify-build-provenance-and-close-0015-open-items.md
// Q2). It sits with the example, in the external test package, as the
// program's own code would.
func TestUpdaterRefusesUnattested(t *testing.T) {
	plat := selfupdate.Platform{OS: runtime.GOOS, Arch: runtime.GOARCH}
	src := selfupdatetest.NewFakeSource("v1.1.0", selfupdatetest.NewRelease("demo", "v1.1.0",
		[]selfupdate.Platform{plat}, func(selfupdate.Platform) []byte { return []byte("demo v1.1.0\n") }))
	assets, err := selfupdate.NewExactAssetSelector([]selfupdate.Platform{plat})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, "demo")
	if err := os.WriteFile(exe, []byte("demo v1.0.0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	inst, err := selfupdate.NewStandaloneInstaller(selfupdate.InstallOptions{
		TargetPolicy: selfupdate.TargetPolicy{ExecutablePath: exe, AllowedRoots: []string{dir}},
	})
	if err != nil {
		t.Fatal(err)
	}
	gh := "/usr/local/bin/gh"
	if runtime.GOOS == "windows" {
		gh = `C:\gh.exe`
	}
	v, err := ghattest.NewManifestVerifier(ghattest.Options{
		Policy: ghattest.Policy{Repository: selfupdate.Repository{Owner: "example", Name: "demo"}},
		GH:     gh,
		Env:    []string{},
		Runner: service.RunnerFunc(func(context.Context, service.Command) (service.Output, error) {
			return service.Output{ExitCode: 1, Stderr: []byte("Error: no attestation")}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	u, err := selfupdate.New(selfupdate.Config{
		Source: src, Versions: selfupdate.NewStrictVersionPolicy(), Assets: assets, Installer: inst,
		Reporter: &selfupdatetest.RecordingReporter{}, Confirmer: &selfupdatetest.ScriptedConfirmer{},
		Limits: selfupdate.DefaultLimits(), ManifestVerifiers: []selfupdate.ManifestVerifier{v},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = u.Run(context.Background(), selfupdate.Request{
		Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: selfupdate.ReleaseBuild, Yes: true,
	})
	if !errors.Is(err, selfupdate.ErrIntegrity) || !strings.Contains(err.Error(), "attestation verification failed (exit 1)") {
		t.Fatalf("Run = %v, want an ErrIntegrity from the attestation check", err)
	}
	for _, call := range src.Calls() {
		if strings.HasPrefix(call, "OpenAsset ") && call != "OpenAsset SHA256SUMS" {
			t.Fatalf("calls %q: a binary was downloaded", src.Calls())
		}
	}
	if !slices.Contains(src.Calls(), "OpenAsset SHA256SUMS") {
		t.Fatalf("calls %q: SHA256SUMS was not fetched", src.Calls())
	}
	if b, _ := os.ReadFile(exe); string(b) != "demo v1.0.0\n" {
		t.Fatalf("the target holds %q", b)
	}
}
