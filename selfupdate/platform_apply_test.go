package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"runtime"
	"slices"
	"testing"
)

// Tests for docs/decisions/0010-PLAN-v1-6-0-owner-contracts.md S4 (Q4, C7):
// an apply for a platform other than the running one is refused, before
// any download. A check or a dry run may still ask about another platform.
// These use only the v1.5.1 API, so they run against the unfixed code too.

// foreignPlatform is a real platform that is not the running one.
func foreignPlatform() Platform {
	if runtime.GOOS == "linux" {
		return Platform{OS: goosWindows, Arch: "amd64"}
	}
	return Platform{OS: "linux", Arch: "amd64"}
}

// platformEnv is a contract env whose release publishes plat only.
func platformEnv(t *testing.T, plat Platform) (*contractEnv, *Updater) {
	t.Helper()
	env := newContractEnv(t)
	bin := []byte("hello-bin")
	sum := sha256.Sum256(bin)
	hexsum := hex.EncodeToString(sum[:])
	name := ExactAssetName("demo", plat)
	manifest := []byte(hexsum + "  " + name + "\n")
	env.src.rel = Release{
		ID: 1, Tag: "v1.1.0", URL: "https://example.invalid/v1.1.0", Immutable: true,
		Assets: []Asset{
			{ID: 2, Name: name, State: "uploaded", Size: int64(len(bin)), Digest: "sha256:" + hexsum},
			{ID: 3, Name: manifestAssetName, State: "uploaded", Size: int64(len(manifest))},
		},
	}
	env.src.bodies = map[int64][]byte{2: bin, 3: manifest}
	sel, err := NewExactAssetSelector([]Platform{plat})
	if err != nil {
		t.Fatal(err)
	}
	u, err := New(Config{
		Source: env.src, Versions: NewStrictVersionPolicy(), Assets: sel, Verifiers: []Verifier{env.ver},
		Installer: env.inst, Reporter: env.rep, Confirmer: env.conf, Limits: env.lim,
	})
	if err != nil {
		t.Fatal(err)
	}
	return env, u
}

func TestApplyForeignPlatformRefused(t *testing.T) {
	foreign := foreignPlatform()
	env, u := platformEnv(t, foreign)
	req := applyReq()
	req.Yes = true
	req.Platform = foreign
	res, err := u.Run(context.Background(), req)
	if !errors.Is(err, ErrUnsupportedPlatform) || res.Applied {
		t.Fatalf("res = %+v, err = %v; want the platform refused", res, err)
	}
	for _, call := range []string{"Latest", "ByTag", "ResolveTarget", "Begin", "Install"} {
		if slices.Contains(*env.log, call) {
			t.Fatalf("%s ran before the refusal: %v", call, *env.log)
		}
	}
}

func TestCheckAndDryRunForeignPlatform(t *testing.T) {
	foreign := foreignPlatform()
	_, u := platformEnv(t, foreign)
	check := applyReq()
	check.CheckOnly = true
	check.Platform = foreign
	if res, err := u.Run(context.Background(), check); !errors.Is(err, ErrUpdateAvailable) || res.AssetName != ExactAssetName("demo", foreign) {
		t.Fatalf("check: res = %+v, err = %v", res, err)
	}
	env, u := platformEnv(t, foreign)
	dry := applyReq()
	dry.DryRun = true
	dry.Platform = foreign
	if res, err := u.Run(context.Background(), dry); err != nil || !res.DryRun || res.Applied || slices.Contains(*env.log, "Install") {
		t.Fatalf("dry run: res = %+v, err = %v, calls %v", res, err, *env.log)
	}
}

// TestApplyRunningPlatformNamed: naming the running platform is the same
// as leaving it zero.
func TestApplyRunningPlatformNamed(t *testing.T) {
	running := Platform{OS: runtime.GOOS, Arch: runtime.GOARCH}
	_, u := platformEnv(t, running)
	req := applyReq()
	req.Yes = true
	req.Platform = running
	if res, err := u.Run(context.Background(), req); err != nil || !res.Applied {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
}
