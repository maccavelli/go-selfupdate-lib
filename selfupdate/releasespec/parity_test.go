package releasespec

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// verifierScript is the publish workflow's file-set verifier.
var verifierScript = filepath.Join("..", "..", "scripts", "verify-selfupdate-release.sh")

// verifier finds python3 and sh, which the verifier needs. It skips when
// either is absent, unless SELFUPDATE_REQUIRE_PYTHON=1, as
// TestManifestDifferential does.
func verifier(t *testing.T) string {
	t.Helper()
	sh, err := exec.LookPath("sh")
	if err == nil {
		_, err = exec.LookPath("python3")
	}
	if err != nil {
		if os.Getenv("SELFUPDATE_REQUIRE_PYTHON") == "1" {
			t.Fatalf("python3 and sh are required (SELFUPDATE_REQUIRE_PYTHON=1): %v", err)
		}
		t.Skip("python3 or sh not found; the parity test runs the release verifier")
	}
	return sh
}

type verifierInput struct {
	products, platforms, extras, channels string
}

func inputsOf(t *testing.T, s Spec, tag string) verifierInput {
	t.Helper()
	extras, err := s.ExtrasJSON(tag)
	if err != nil {
		t.Fatal(err)
	}
	return verifierInput{s.ProductsJSON(), s.PlatformsJSON(), extras, s.ChannelsJSON()}
}

// stage writes a release the spec describes: each canonical asset, a
// SHA256SUMS listing exactly those, and each extra.
func stage(t *testing.T, s Spec, tag string) string {
	t.Helper()
	dir := t.TempDir()
	names, err := s.assetNames()
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(names)
	var sums bytes.Buffer
	for _, n := range names {
		body := []byte("binary " + n)
		if err := os.WriteFile(filepath.Join(dir, n), body, 0o600); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(body)
		sums.WriteString(hex.EncodeToString(sum[:]) + "  " + n + "\n")
	}
	if err := os.WriteFile(filepath.Join(dir, "SHA256SUMS"), sums.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	extras, err := s.ExtraNames(tag)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range extras {
		if err := os.WriteFile(filepath.Join(dir, e), []byte("extra"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// runVerifier runs the verifier with the inputs in the environment, and sh
// expands them into its arguments. On Windows, Git's sh.exe parses a
// command line by other rules than the ones Go quotes it by, and a JSON
// argument arrives mangled; an environment value arrives intact.
func runVerifier(t *testing.T, sh, dir, tag string, in verifierInput) (string, error) {
	t.Helper()
	cmd := exec.Command(sh, "-c", `exec sh "$V_SCRIPT" --dir "$V_DIR" --products "$V_PRODUCTS" `+
		`--platforms "$V_PLATFORMS" --extras "$V_EXTRAS" --channels "$V_CHANNELS" --tag "$V_TAG"`)
	cmd.Env = append(os.Environ(),
		"V_SCRIPT="+filepath.ToSlash(verifierScript), "V_DIR="+filepath.ToSlash(dir),
		"V_PRODUCTS="+in.products, "V_PLATFORMS="+in.platforms, "V_EXTRAS="+in.extras,
		"V_CHANNELS="+in.channels, "V_TAG="+tag)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// TestVerifierParity holds the spec and the publish verifier to the same
// rules (0013-PLAN B1). A release built from a spec Parse accepts must pass
// the verifier, and an input the spec refuses must not reach the verifier
// in a form it accepts. B1 covered binary specs; the archive spec joined
// in B3, when the verifier learned "format" (0013-PLAN deviation D1).
func TestVerifierParity(t *testing.T) {
	sh := verifier(t)
	const tag = "v1.2.3-rc.1"
	for _, fixture := range []string{"fleet.json", "minimal.json", "archive.json", "installer.json"} {
		t.Run(fixture, func(t *testing.T) {
			s := mustParse(t, readFixture(t, fixture))
			if len(s.PrereleaseChannels) == 0 {
				s.PrereleaseChannels = []string{"rc"}
			}
			dir := stage(t, s, tag)
			if out, err := runVerifier(t, sh, dir, tag, inputsOf(t, s, tag)); err != nil {
				t.Fatalf("the verifier refused a release built from %s: %v\n%s", fixture, err, out)
			}
		})
	}

	// Each refused spec, and the verifier input it would have produced: the
	// verifier must refuse that input too.
	good := mustParse(t, readFixture(t, "minimal.json"))
	goodIn := inputsOf(t, good, "v1.2.3")
	refused := []struct {
		name string
		edit func(m map[string]any)
		in   func(in verifierInput) verifierInput
		want string // the verifier's own refusal
	}{
		{"bad product name", func(m map[string]any) { product(m)["name"] = "-relay" },
			func(in verifierInput) verifierInput { in.products = `["-relay"]`; return in }, "invalid product"},
		{"duplicate platform", func(m map[string]any) {
			m["platforms"] = repeat(2, func(int) any { return map[string]any{"os": "linux", "arch": "amd64"} })
		}, func(in verifierInput) verifierInput {
			in.platforms = `[{"os":"linux","arch":"amd64"},{"os":"linux","arch":"amd64"}]`
			return in
		}, "duplicate platform"},
		{"uppercase os", func(m map[string]any) { m["platforms"] = []any{map[string]any{"os": "Linux", "arch": "amd64"}} },
			func(in verifierInput) verifierInput { in.platforms = `[{"os":"Linux","arch":"amd64"}]`; return in }, "invalid platform os"},
		{"extra SHA256SUMS", func(m map[string]any) { m["extras"] = []any{map[string]any{"name": "SHA256SUMS-1"}} },
			func(in verifierInput) verifierInput { in.extras = `["SHA256SUMS-1"]`; return in }, "invalid or duplicate extra asset"},
		{"extra bad name", func(m map[string]any) { m["extras"] = []any{map[string]any{"name": "-notes"}} },
			func(in verifierInput) verifierInput { in.extras = `["-notes"]`; return in }, "invalid extra asset"},
		{"ascending channels", func(m map[string]any) { m["prerelease_channels"] = []any{"beta", "rc"} },
			func(in verifierInput) verifierInput { in.channels = `["beta","rc"]`; return in }, "must come after"},
		{"bad channel", func(m map[string]any) { m["prerelease_channels"] = []any{"RC"} },
			func(in verifierInput) verifierInput { in.channels = `["RC"]`; return in }, "must match"},
		{"unknown archive format", func(m map[string]any) {
			m["packaging"] = "archive"
			m["platforms"] = []any{map[string]any{"os": "linux", "arch": "amd64", "format": "tar.xz"}}
		}, func(in verifierInput) verifierInput {
			in.platforms = `[{"os":"linux","arch":"amd64","format":"tar.xz"}]`
			return in
		}, "unknown archive format"},
		{"extra named like an archive", func(m map[string]any) {
			m["packaging"] = "archive"
			m["extras"] = []any{map[string]any{"name": "relay-linux-amd64.tar.gz"}}
		}, func(in verifierInput) verifierInput {
			in.platforms = `[{"os":"linux","arch":"amd64","format":"tar.gz"}]`
			in.extras = `["relay-linux-amd64.tar.gz"]`
			return in
		}, "is named like a canonical asset"},
	}
	for _, tc := range refused {
		t.Run("refused/"+tc.name, func(t *testing.T) {
			m := base()
			tc.edit(m)
			if _, err := Parse(encode(t, m)); err == nil {
				t.Fatal("Parse accepted the spec")
			}
			dir := stage(t, good, "v1.2.3")
			out, err := runVerifier(t, sh, dir, "v1.2.3", tc.in(goodIn))
			if err == nil {
				t.Fatalf("the verifier accepted what the spec refuses:\n%s", out)
			}
			if !strings.Contains(out, tc.want) || strings.Contains(out, "usage:") {
				t.Fatalf("the verifier failed, but not with %q: %v\n%s", tc.want, err, out)
			}
		})
	}
}
