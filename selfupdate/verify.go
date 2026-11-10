package selfupdate

import (
	"crypto/subtle"

	"github.com/maccavelli/go-selfupdate-lib/internal/errprefix"
)

func verifyIntegrity(v Verification) error {
	if v.SHA256 == "" || v.ManifestSHA256 == "" {
		return errprefix.Wrap("selfupdate: missing digest", ErrIntegrity)
	}
	if !equalDigest(v.SHA256, v.ManifestSHA256) {
		return errprefix.Wrap("selfupdate: staged digest does not match SHA256SUMS", ErrIntegrity)
	}
	if v.GitHubSHA256 != "" && !equalDigest(v.SHA256, v.GitHubSHA256) {
		return errprefix.Wrap("selfupdate: staged digest does not match github asset digest", ErrIntegrity)
	}
	return nil
}

func equalDigest(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
