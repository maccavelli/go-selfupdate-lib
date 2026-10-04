package selfupdate

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyIntegrity(t *testing.T) {
	err := verifyIntegrity(Verification{
		SHA256:         testDigest,
		ManifestSHA256: testDigest,
		GitHubSHA256:   testDigest,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = verifyIntegrity(Verification{
		SHA256:         testDigest,
		ManifestSHA256: strings.Repeat("ab", 32),
	})
	if !errors.Is(err, ErrIntegrity) {
		t.Fatalf("manifest mismatch err = %v", err)
	}
	err = verifyIntegrity(Verification{
		SHA256:         testDigest,
		ManifestSHA256: testDigest,
		GitHubSHA256:   strings.Repeat("cd", 32),
	})
	if !errors.Is(err, ErrIntegrity) {
		t.Fatalf("github mismatch err = %v", err)
	}
}

// TestManifestTestdata reads the testdata manifests through the parser and
// lookup the Updater uses. verifyManifest, which wrapped them, had no
// caller outside its test and was removed (0010-MADR A15); a mismatch is
// TestVerifyIntegrity's.
func TestManifestTestdata(t *testing.T) {
	valid, err := os.ReadFile(filepath.Join("testdata", "SHA256SUMS.valid"))
	if err != nil {
		t.Fatal(err)
	}
	entries, err := ParseSHA256SUMS(valid)
	if err != nil {
		t.Fatal(err)
	}
	if want, err := checksumFor(entries, "demo-linux-amd64"); err != nil || want != testDigest {
		t.Fatalf("checksumFor = %q, %v; want %q", want, err, testDigest)
	}
	invalid, err := os.ReadFile(filepath.Join("testdata", "SHA256SUMS.invalid"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseSHA256SUMS(invalid); err == nil {
		t.Fatal("accepted invalid testdata manifest")
	}
}

// TestEqualDigestLength: digests of different lengths never match.
func TestEqualDigestLength(t *testing.T) {
	if equalDigest(testDigest, testDigest[:len(testDigest)-1]) {
		t.Fatal("digests of different lengths matched")
	}
	if err := verifyIntegrity(Verification{SHA256: testDigest, ManifestSHA256: testDigest[:10]}); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("short manifest digest: err = %v", err)
	}
}
