package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
)

// Verification hooks around the SHA256SUMS manifest
// (docs/decisions/0004-MADR-evolve-selfupdate-api-and-tui-support.md G9, and
// "Release signing: deferred, with the hook built").

// ManifestVerification is the input to a ManifestVerifier.
type ManifestVerification struct {
	// Product is the requested product name.
	Product string
	// Release is the selected release metadata.
	Release Release
	// Selection is the selected binary and manifest.
	Selection Selection
	// Manifest is a copy of the SHA256SUMS bytes.
	Manifest []byte
	// OpenAsset opens another asset of the release by exact name, within
	// limit bytes, enforcing its advertised size and digest.
	OpenAsset func(ctx context.Context, name string, limit int64) (io.ReadCloser, error)
}

// ManifestVerifier checks the manifest before any binary byte is fetched:
// the place a signature over SHA256SUMS is verified.
type ManifestVerifier interface {
	VerifyManifest(context.Context, ManifestVerification) error
}

// ManifestVerifierFunc adapts a function to ManifestVerifier.
type ManifestVerifierFunc func(context.Context, ManifestVerification) error

// VerifyManifest implements ManifestVerifier.
func (f ManifestVerifierFunc) VerifyManifest(ctx context.Context, v ManifestVerification) error {
	return f(ctx, v)
}

// runManifestVerifiers runs each verifier in order. A failure always
// matches ErrIntegrity.
func (u *run) runManifestVerifiers(ctx context.Context, req Request, rel Release, sel Selection, manifest []byte) error {
	for _, v := range u.manifestVfy {
		err := v.VerifyManifest(ctx, ManifestVerification{
			Product:   req.Product,
			Release:   rel,
			Selection: sel,
			Manifest:  append([]byte(nil), manifest...),
			OpenAsset: u.openAsset(rel),
		})
		if err != nil {
			return fmt.Errorf("selfupdate: manifest verification failed: %w", errors.Join(ErrIntegrity, err))
		}
	}
	return nil
}

// openAsset returns the OpenAsset function for rel: an exact, unique name;
// a positive limit capped at Limits.Executable; and a body that must match
// the advertised size and, when present, the GitHub digest.
func (u *run) openAsset(rel Release) func(ctx context.Context, name string, limit int64) (io.ReadCloser, error) {
	return func(ctx context.Context, name string, limit int64) (io.ReadCloser, error) {
		if limit <= 0 {
			return nil, fmt.Errorf("selfupdate: asset limit must be positive")
		}
		if limit > u.limits.Executable {
			limit = u.limits.Executable
		}
		var found []Asset
		for _, a := range rel.Assets {
			if a.Name == name {
				found = append(found, a)
			}
		}
		switch len(found) {
		case 0:
			return nil, fmt.Errorf("selfupdate: release %s has no asset %q", sanitizeText(rel.Tag), sanitizeText(name))
		case 1:
		default:
			return nil, fmt.Errorf("selfupdate: release %s has duplicate asset %q", sanitizeText(rel.Tag), sanitizeText(name))
		}
		asset := found[0]
		if err := validateAssetMetadata(asset, limit); err != nil {
			return nil, err
		}
		want := ""
		if asset.Digest != "" {
			d, err := parseGitHubDigest(asset.Digest)
			if err != nil {
				return nil, err
			}
			want = d
		}
		rc, err := u.source.OpenAsset(ctx, rel, asset)
		if err != nil {
			return nil, err
		}
		return &checkedAsset{
			rc: rc, r: &io.LimitedReader{R: rc, N: asset.Size + 1},
			name: asset.Name, size: asset.Size, digest: want, h: sha256.New(),
		}, nil
	}
}

// checkedAsset fails with ErrIntegrity when the body is longer or shorter
// than advertised, or its SHA-256 does not match the GitHub digest.
type checkedAsset struct {
	rc      io.ReadCloser
	r       io.Reader
	name    string
	size    int64
	digest  string
	h       hash.Hash
	n       int64
	checked bool
}

func (c *checkedAsset) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	c.h.Write(p[:n])
	if c.n > c.size {
		return n, fmt.Errorf("selfupdate: asset %s is longer than advertised: %w", sanitizeText(c.name), ErrIntegrity)
	}
	// The digest is compared on the read that reaches the advertised size:
	// a caller that reads exactly Size bytes (io.ReadFull, io.CopyN) never
	// sees io.EOF. A mismatch returns no bytes, because io.ReadFull drops an
	// error that comes with a full buffer (0010-MADR A1).
	if c.n == c.size && !c.checked {
		c.checked = true
		if c.digest != "" && hex.EncodeToString(c.h.Sum(nil)) != c.digest {
			return 0, fmt.Errorf("selfupdate: asset %s does not match its github digest: %w", sanitizeText(c.name), ErrIntegrity)
		}
	}
	if errors.Is(err, io.EOF) && c.n != c.size {
		return n, fmt.Errorf("selfupdate: asset %s is shorter than advertised: %w", sanitizeText(c.name), ErrIntegrity)
	}
	return n, err
}

func (c *checkedAsset) Close() error {
	return c.rc.Close()
}
