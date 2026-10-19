package signednote

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/mod/semver"
	"golang.org/x/mod/sumdb/note"
)

const (
	// Header is the statement's first line.
	Header = "selfupdate-release/v1"
	// Suffix names the note: the manifest asset's name plus Suffix, such as
	// SHA256SUMS.note.
	Suffix = ".note"
	// MaxNoteSize bounds the note the verifier reads.
	MaxNoteSize = 16 << 10
)

var (
	repositoryRe = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*/[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$`)
	nameRe       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	digestRe     = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// Statement is the signed release statement. Text renders it, and
// ParseStatement reads it back; the two are exact inverses.
type Statement struct {
	// Repository is <host>/<owner>/<name>, as the publish workflow's
	// github.server_url host and github.repository give it.
	Repository string `json:"repository"`
	// Tag is the release tag, a canonical semantic version.
	Tag string `json:"tag"`
	// Products are the release's products, ascending and unique.
	Products []string `json:"products"`
	// Manifest is the manifest asset's name, such as SHA256SUMS.
	Manifest string `json:"manifest"`
	// ManifestSHA256 is the lowercase hex SHA-256 of the manifest's bytes.
	ManifestSHA256 string `json:"manifest_sha256"`
}

// Text validates s and renders it: five lines, each ending in LF, in a fixed
// order, with no timestamp.
func (s Statement) Text() (string, error) {
	if err := s.validate(); err != nil {
		return "", err
	}
	return Header + "\n" +
		"repository " + s.Repository + "\n" +
		"tag " + s.Tag + "\n" +
		"products " + strings.Join(s.Products, " ") + "\n" +
		"manifest " + s.Manifest + " sha256:" + s.ManifestSHA256 + "\n", nil
}

// ParseStatement reads a statement strictly: exactly Text's five lines, in
// order, LF only, with a final LF, single spaces, and products ascending and
// unique.
func ParseStatement(text string) (Statement, error) {
	body, ok := strings.CutSuffix(text, "\n")
	if !ok || strings.ContainsRune(text, '\r') {
		return Statement{}, errors.New("selfupdate: signednote: statement must be LF lines ending in LF")
	}
	lines := strings.Split(body, "\n")
	if len(lines) != 5 || lines[0] != Header {
		return Statement{}, fmt.Errorf("selfupdate: signednote: statement must be %s and four lines", Header)
	}
	var s Statement
	fields := []struct {
		prefix string
		set    func(string) error
	}{
		{"repository ", func(v string) error { s.Repository = v; return nil }},
		{"tag ", func(v string) error { s.Tag = v; return nil }},
		{"products ", func(v string) error { s.Products = strings.Split(v, " "); return nil }},
		{"manifest ", func(v string) error {
			name, digest, ok := strings.Cut(v, " sha256:")
			if !ok {
				return errors.New("manifest line must be <name> sha256:<hex>")
			}
			s.Manifest, s.ManifestSHA256 = name, digest
			return nil
		}},
	}
	for i, f := range fields {
		v, ok := strings.CutPrefix(lines[i+1], f.prefix)
		if !ok {
			return Statement{}, fmt.Errorf("selfupdate: signednote: line %d must start %q", i+2, f.prefix)
		}
		if err := f.set(v); err != nil {
			return Statement{}, fmt.Errorf("selfupdate: signednote: %w", err)
		}
	}
	got, err := s.Text()
	if err != nil {
		return Statement{}, err
	}
	if got != text {
		return Statement{}, errors.New("selfupdate: signednote: statement is not in canonical form")
	}
	return s, nil
}

func (s Statement) validate() error {
	switch {
	case !repositoryRe.MatchString(s.Repository):
		return fmt.Errorf("selfupdate: signednote: repository %q is not <host>/<owner>/<name>", s.Repository)
	case !canonicalTag(s.Tag):
		return fmt.Errorf("selfupdate: signednote: tag %q is not a canonical semantic version", s.Tag)
	case len(s.Products) == 0:
		return errors.New("selfupdate: signednote: a statement needs a product")
	case !nameRe.MatchString(s.Manifest):
		return fmt.Errorf("selfupdate: signednote: manifest name %q is invalid", s.Manifest)
	case !digestRe.MatchString(s.ManifestSHA256):
		return errors.New("selfupdate: signednote: manifest digest must be 64 lowercase hex characters")
	}
	for i, p := range s.Products {
		if !nameRe.MatchString(p) {
			return fmt.Errorf("selfupdate: signednote: product %q is invalid", p)
		}
		if i > 0 && s.Products[i-1] >= p {
			return errors.New("selfupdate: signednote: products must be ascending and unique")
		}
	}
	return nil
}

// canonicalTag reports a semantic version tag in canonical form, with no
// build metadata.
func canonicalTag(tag string) bool {
	return semver.IsValid(tag) && semver.Canonical(tag) == tag
}

// Open verifies msg as a signed note against keys, note verifier keys of
// the form name+hash+base64, and parses its statement. It fails unless a
// key in keys signed it; a signature by an unknown key is ignored, and an
// invalid one by a known key fails it.
func Open(msg []byte, keys []string) (Statement, error) {
	vs, err := verifiers(keys)
	if err != nil {
		return Statement{}, err
	}
	n, err := note.Open(msg, note.VerifierList(vs...))
	if err != nil {
		return Statement{}, fmt.Errorf("selfupdate: signednote: open note: %w", err)
	}
	return ParseStatement(n.Text)
}

// verifiers parses keys, refusing none and a key given twice.
func verifiers(keys []string) ([]note.Verifier, error) {
	if len(keys) == 0 {
		return nil, errors.New("selfupdate: signednote: no verifier keys")
	}
	vs := make([]note.Verifier, 0, len(keys))
	var seen []string
	for _, k := range keys {
		v, err := note.NewVerifier(k)
		if err != nil {
			return nil, fmt.Errorf("selfupdate: signednote: verifier key: %w", err)
		}
		id := fmt.Sprintf("%s+%08x", v.Name(), v.KeyHash())
		if slices.Contains(seen, id) {
			return nil, fmt.Errorf("selfupdate: signednote: verifier key %s is given twice", id)
		}
		seen = append(seen, id)
		vs = append(vs, v)
	}
	return vs, nil
}
