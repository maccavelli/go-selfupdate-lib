package ghattest_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/verify/ghattest"
)

// Live tests for docs/decisions/0017-PLAN-verify-build-provenance-and-close-0015-open-items.md
// Q2, against the real gh and a real attested release. They need gh logged
// in, so CI, which has no token, skips them.
//
// SELFUPDATE_REQUIRE_GHATTEST=1 runs them. SELFUPDATE_GHATTEST_GH names gh's
// path (default: gh on PATH). The fixture is aquaproj/aqua v2.64.0's
// checksums, which a reusable workflow in another repository attests, as
// this module's publish workflow attests its callers' releases. Once a
// release from this module's workflow exists, SELFUPDATE_GHATTEST_REPOSITORY
// (owner/name), SELFUPDATE_GHATTEST_TAG and SELFUPDATE_GHATTEST_ASSET (its
// SHA256SUMS's download URL) check it instead, with the publish workflow
// as the signer.

const (
	requireEnv = "SELFUPDATE_REQUIRE_GHATTEST"
	aquaURL    = "https://github.com/aquaproj/aqua/releases/download/v2.64.0/aqua_2.64.0_checksums.txt"
	aquaSHA256 = "651d4378614e8506c5814d3bed0c5643eea74b957e491601427ad8ebf241fa02"
	aquaSigner = "suzuki-shunsuke/go-release-workflow/.github/workflows/release.yaml"
	aquaCommit = "cf03c29d97518871efb36bbb80bcc01a19645b49"
)

func requireLive(t *testing.T) string {
	t.Helper()
	if os.Getenv(requireEnv) != "1" {
		t.Skipf("set %s=1 to verify a real attestation with gh", requireEnv)
	}
	gh := os.Getenv("SELFUPDATE_GHATTEST_GH")
	if gh == "" {
		p, err := exec.LookPath("gh")
		if err != nil {
			t.Fatalf("%s=1, and no gh: %v", requireEnv, err)
		}
		gh = p
	}
	abs, err := filepath.Abs(gh)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

func fetch(t *testing.T, url string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: %s", url, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestLiveVerify(t *testing.T) {
	gh := requireLive(t)
	repo, tag, signer := selfupdate.Repository{Owner: "aquaproj", Name: "aqua"}, "v2.64.0", aquaSigner
	var sums []byte
	if r := os.Getenv("SELFUPDATE_GHATTEST_REPOSITORY"); r != "" {
		owner, name, _ := strings.Cut(r, "/")
		repo, tag, signer = selfupdate.Repository{Owner: owner, Name: name}, os.Getenv("SELFUPDATE_GHATTEST_TAG"), ghattest.PublishWorkflow
		sums = fetch(t, os.Getenv("SELFUPDATE_GHATTEST_ASSET"))
	} else {
		sums = fetch(t, aquaURL)
		if d := sha256.Sum256(sums); hex.EncodeToString(d[:]) != aquaSHA256 {
			t.Fatalf("the fixture changed: %s has SHA-256 %x", aquaURL, d)
		}
	}
	check := func(p ghattest.Policy, tag string) error {
		t.Helper()
		v, err := ghattest.NewManifestVerifier(ghattest.Options{Policy: p, GH: gh})
		if err != nil {
			t.Fatal(err)
		}
		return v.VerifyManifest(context.Background(), selfupdate.ManifestVerification{
			Release: selfupdate.Release{Tag: tag}, Manifest: sums,
		})
	}
	pass := ghattest.Policy{Repository: repo, SignerWorkflow: signer}
	if err := check(pass, tag); err != nil {
		t.Fatalf("the attested release: %v", err)
	}
	if os.Getenv("SELFUPDATE_GHATTEST_REPOSITORY") == "" {
		for name, digests := range map[string][]string{
			"one digest":  {aquaCommit},
			"two digests": {strings.Repeat("a", 40), aquaCommit},
		} {
			p := pass
			p.SignerDigests = digests
			if err := check(p, tag); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
		}
		other := pass
		other.SignerDigests = []string{strings.Repeat("a", 40)}
		if err := check(other, tag); err == nil {
			t.Fatal("another signer commit passed")
		}
	}
	wrongSigner := pass
	wrongSigner.SignerWorkflow = repo.Owner + "/" + repo.Name + "/.github/workflows/not-the-signer.yml"
	if err := check(wrongSigner, tag); err == nil || !strings.Contains(err.Error(), "exit 1") {
		t.Fatalf("another signer workflow: %v", err)
	}
	if err := check(pass, "v0.0.0-not-this-release"); err == nil || !strings.Contains(err.Error(), "exit 1") {
		t.Fatalf("another tag: %v", err)
	}
}
