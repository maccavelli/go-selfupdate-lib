//go:build darwin

package codesign

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
)

// Live tests for 0012-PLAN U4 step 8, against this Mac's /usr/bin/codesign.
// SELFUPDATE_REQUIRE_CODESIGN=1 runs the ad-hoc tests, as CI does on
// macOS. SELFUPDATE_CODESIGN_IDENTITY names a certificate identity for
// TestLiveSignIdentity, which CI never sets: it is the test for when the
// owner has one (0012-MADR §10). Every fixture is darwin/arm64.

const (
	requireEnv  = "SELFUPDATE_REQUIRE_CODESIGN"
	identityEnv = "SELFUPDATE_CODESIGN_IDENTITY"
	keychainEnv = "SELFUPDATE_CODESIGN_KEYCHAIN"
	liveID      = "com.example.selfupdate.live"
)

var darwinARM64 = selfupdate.Platform{OS: "darwin", Arch: "arm64"}

func requireLive(t *testing.T) {
	t.Helper()
	if os.Getenv(requireEnv) != "1" {
		t.Skipf("set %s=1 to run against this Mac's codesign", requireEnv)
	}
}

// fixture cross-builds a darwin/arm64 program, which Go's linker signs
// ad hoc, into a file named as the module names staging.
func fixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	if err := os.MkdirAll(src, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "go.mod"), []byte("module fixture\n\ngo 1.27\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("the go command is required to build the fixture: %v", err)
	}
	out := filepath.Join(dir, ".relay.selfupdate-12345")
	cmd := exec.Command(goBin, "build", "-o", out, ".") //nolint:gosec // the go command, building a fixture
	cmd.Dir = src
	cmd.Env = append(os.Environ(), "GOOS=darwin", "GOARCH=arm64", "CGO_ENABLED=0", "GOWORK=off", "GOFLAGS=")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, b)
	}
	return out
}

// describe is codesign -d -vv's description of path.
func describe(t *testing.T, path string) string {
	t.Helper()
	out, err := exec.Command("/usr/bin/codesign", "-d", "-vv", path).CombinedOutput()
	if err != nil {
		t.Fatalf("codesign -d: %v: %s", err, out)
	}
	return string(out)
}

func staging(path string) selfupdate.ProbeRequest {
	return selfupdate.ProbeRequest{Product: "relay", Path: path, Phase: selfupdate.ProbeStaged}
}

// TestLiveSignAdHoc: an ad-hoc signer gives the staging file the configured
// identifier, not one from its name, and the checker accepts it.
func TestLiveSignAdHoc(t *testing.T) {
	requireLive(t)
	path := fixture(t)
	if d := describe(t, path); !strings.Contains(d, "Identifier=a.out") {
		t.Fatalf("the linker's signature:\n%s", d)
	}
	s, err := NewSigner(SignOptions{Identity: "-", Identifier: liveID})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Transform(context.Background(), selfupdate.TransformRequest{Product: "relay", Platform: darwinARM64, Path: path}); err != nil {
		t.Fatal(err)
	}
	if d := describe(t, path); !strings.Contains(d, "Identifier="+liveID+"\n") || !strings.Contains(d, "Signature=adhoc") {
		t.Fatalf("after signing:\n%s", d)
	}
	for _, req := range []string{"", `identifier "` + liveID + `"`} {
		c, err := NewChecker(CheckOptions{Requirement: req})
		if err != nil {
			t.Fatal(err)
		}
		if err := c.Probe(context.Background(), staging(path)); err != nil {
			t.Fatalf("requirement %q: %v", req, err)
		}
	}
}

// TestLiveCheckerExitCodes: the checker accepts the linker's signature and
// refuses no signature, a changed byte and an unmet requirement.
func TestLiveCheckerExitCodes(t *testing.T) {
	requireLive(t)
	signed := fixture(t)
	dir := filepath.Dir(signed)
	body, err := os.ReadFile(signed) //nolint:gosec // the fixture
	if err != nil {
		t.Fatal(err)
	}
	stripped := filepath.Join(dir, "stripped")
	if err := os.WriteFile(stripped, body, 0o700); err != nil { //nolint:gosec // an executable fixture
		t.Fatal(err)
	}
	if out, err := exec.Command("/usr/bin/codesign", "--remove-signature", stripped).CombinedOutput(); err != nil {
		t.Fatalf("remove the signature: %v: %s", err, out)
	}
	tampered := filepath.Join(dir, "tampered")
	changed := append([]byte(nil), body...)
	changed[len(changed)/4] ^= 0xff
	if err := os.WriteFile(tampered, changed, 0o700); err != nil { //nolint:gosec // an executable fixture
		t.Fatal(err)
	}
	check := func(req, path string) error {
		t.Helper()
		c, err := NewChecker(CheckOptions{Requirement: req})
		if err != nil {
			t.Fatal(err)
		}
		return c.Probe(context.Background(), staging(path))
	}
	if err := check("", signed); err != nil {
		t.Fatalf("the linker's signature: %v", err)
	}
	for _, c := range []struct {
		name, req, path, want string
	}{
		{"no signature", "", stripped, "(exit 1)"},
		{"a changed byte", "", tampered, "(exit 1)"},
		{"an unmet requirement", "anchor apple generic", signed, "(exit 3)"},
	} {
		err := check(c.req, c.path)
		if err == nil || !strings.Contains(err.Error(), c.want) || !errors.Is(err, selfupdate.ErrIntegrity) {
			t.Errorf("%s: %v, want %s and ErrIntegrity", c.name, err, c.want)
		}
	}
}

// TestLiveSignIdentity signs with the certificate identity
// SELFUPDATE_CODESIGN_IDENTITY names, on a Mac that has one.
func TestLiveSignIdentity(t *testing.T) {
	identity := os.Getenv(identityEnv)
	if identity == "" {
		t.Skipf("set %s to a signing identity in this Mac's keychains to sign with a certificate", identityEnv)
	}
	path := fixture(t)
	s, err := NewSigner(SignOptions{Identity: identity, Identifier: liveID, Keychain: os.Getenv(keychainEnv)})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Transform(context.Background(), selfupdate.TransformRequest{Product: "relay", Platform: darwinARM64, Path: path}); err != nil {
		t.Fatal(err)
	}
	if d := describe(t, path); !strings.Contains(d, "Identifier="+liveID+"\n") || !strings.Contains(d, "Authority=") {
		t.Fatalf("after signing:\n%s", d)
	}
	c, err := NewChecker(CheckOptions{Requirement: "anchor apple generic"})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Probe(context.Background(), staging(path)); err != nil {
		t.Fatal(err)
	}
}
