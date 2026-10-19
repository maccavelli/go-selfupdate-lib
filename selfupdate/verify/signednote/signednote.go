package signednote

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"slices"

	"golang.org/x/mod/semver"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
)

// Config configures New.
type Config struct {
	// Repository is the repository the statement must name, as
	// <host>/<owner>/<name>, such as github.com/owner/repo. Required.
	Repository string
	// Keys are the note verifier keys the program trusts, of the form
	// name+hash+base64, each at most once. Required. A note signed by any
	// of them is accepted; listing an old and a new key rotates from one to
	// the other.
	Keys []string
	// RequiredFrom is the first tag that must carry a note. Empty means
	// every tag. A release below it may have none.
	RequiredFrom string
}

type verifier struct {
	repository   string
	keys         []string
	requiredFrom string
}

// New returns the verifier for c. It refuses an invalid repository, no
// keys, a key it cannot parse or one given twice, and a RequiredFrom that is
// not a canonical semantic version.
func New(c Config) (selfupdate.ManifestVerifier, error) {
	if !repositoryRe.MatchString(c.Repository) {
		return nil, fmt.Errorf("selfupdate: signednote: repository %q is not <host>/<owner>/<name>", c.Repository)
	}
	if _, err := verifiers(c.Keys); err != nil {
		return nil, err
	}
	if c.RequiredFrom != "" && !canonicalTag(c.RequiredFrom) {
		return nil, fmt.Errorf("selfupdate: signednote: RequiredFrom %q is not a canonical semantic version", c.RequiredFrom)
	}
	return &verifier{repository: c.Repository, keys: slices.Clone(c.Keys), requiredFrom: c.RequiredFrom}, nil
}

// VerifyManifest implements selfupdate.ManifestVerifier.
func (v *verifier) VerifyManifest(ctx context.Context, m selfupdate.ManifestVerification) error {
	tag := m.Release.Tag
	name := m.Selection.Manifest.Name + Suffix
	if !slices.ContainsFunc(m.Release.Assets, func(a selfupdate.Asset) bool { return a.Name == name }) {
		if v.requiredFrom != "" && semver.Compare(tag, v.requiredFrom) < 0 {
			return nil
		}
		return fmt.Errorf("selfupdate: signednote: release %q has no %s", tag, name)
	}
	rc, err := m.OpenAsset(ctx, name, MaxNoteSize)
	if err != nil {
		return fmt.Errorf("selfupdate: signednote: open %s: %w", name, err)
	}
	msg, err := io.ReadAll(io.LimitReader(rc, MaxNoteSize+1))
	if cerr := rc.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("selfupdate: signednote: read %s: %w", name, err)
	}
	if len(msg) > MaxNoteSize {
		return fmt.Errorf("selfupdate: signednote: %s exceeds %d bytes", name, MaxNoteSize)
	}
	s, err := Open(msg, v.keys)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(m.Manifest)
	var errs []error
	if s.Repository != v.repository {
		errs = append(errs, fmt.Errorf("names repository %q, want %q", s.Repository, v.repository))
	}
	if s.Tag != tag {
		errs = append(errs, fmt.Errorf("names tag %q, want %q", s.Tag, tag))
	}
	if !slices.Contains(s.Products, m.Product) {
		errs = append(errs, fmt.Errorf("does not name product %q", m.Product))
	}
	if s.Manifest != m.Selection.Manifest.Name {
		errs = append(errs, fmt.Errorf("names manifest %q, want %q", s.Manifest, m.Selection.Manifest.Name))
	}
	if s.ManifestSHA256 != hex.EncodeToString(sum[:]) {
		errs = append(errs, errors.New("names another manifest digest"))
	}
	if len(errs) > 0 {
		return fmt.Errorf("selfupdate: signednote: the statement in %s %w", name, errors.Join(errs...))
	}
	return nil
}
