package releasespec

import (
	"fmt"
	"io/fs"
	"regexp"
	"strings"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate/archive"
)

// The limits of 0013-MADR §2.
const (
	maxProducts  = 16
	maxPlatforms = 32
	maxExtras    = 32
	maxList      = 16 // tags and identity_args, each
	// maxTarName is a USTAR entry name's length, as pack writes the
	// program, with no directory.
	maxTarName = 100
	// maxAssetName is the archive selector's asset-name length.
	maxAssetName = 128
)

var (
	// keyRe is every field name's shape.
	keyRe = regexp.MustCompile(`^[a-z_]+$`)
	// assetNameRe is the release workflow's product and extra name check
	// (scripts/verify-selfupdate-release.sh), and selfupdate's product
	// rule.
	assetNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	// platformFieldRe is NewExactAssetSelector's GOOS and GOARCH rule.
	platformFieldRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_]*$`)
	// tagRe is a build tag.
	tagRe = regexp.MustCompile(`^[A-Za-z0-9_.]+$`)
	// channelRe is a prerelease channel name (0005-MADR §5).
	channelRe = regexp.MustCompile(`^[a-z][a-z0-9]{0,15}$`)
)

// exampleTag stands in for the release tag when Validate checks the extras'
// names; ExtraNames checks them again with the real tag.
const exampleTag = "v1.2.3"

// Validate checks every rule of 0013-MADR §2. Parse calls it; a program
// that builds a Spec in Go can too. Errors name the field:
// "releasespec: <field>: <reason>".
func (s Spec) Validate() error {
	if s.Schema != SchemaVersion {
		return fmt.Errorf("releasespec: schema: %d is not supported; this library reads schema %d", s.Schema, SchemaVersion)
	}
	if err := s.validateProducts(); err != nil {
		return err
	}
	if err := s.validatePlatforms(); err != nil {
		return err
	}
	if err := s.validateArchiveNames(); err != nil {
		return err
	}
	if err := s.validateInstaller(); err != nil {
		return err
	}
	if err := s.validateExtras(); err != nil {
		return err
	}
	return validateChannels(s.PrereleaseChannels)
}

func (s Spec) validateProducts() error {
	if len(s.Products) == 0 || len(s.Products) > maxProducts {
		return fmt.Errorf("releasespec: products: %d listed; 1 to %d are allowed", len(s.Products), maxProducts)
	}
	seen := map[string]bool{}
	for i, p := range s.Products {
		at := fmt.Sprintf("releasespec: products[%d]", i)
		if !assetNameRe.MatchString(p.Name) {
			return fmt.Errorf("%s.name: %q must match %s", at, p.Name, assetNameRe)
		}
		if seen[strings.ToLower(p.Name)] {
			return fmt.Errorf("%s.name: %q is listed twice (names are compared ignoring case)", at, p.Name)
		}
		seen[strings.ToLower(p.Name)] = true
		if err := validatePackage(p.Package); err != nil {
			return fmt.Errorf("%s.package: %w", at, err)
		}
		if len(p.Tags) > maxList {
			return fmt.Errorf("%s.tags: %d listed; at most %d are allowed", at, len(p.Tags), maxList)
		}
		for j, tag := range p.Tags {
			if !tagRe.MatchString(tag) {
				return fmt.Errorf("%s.tags[%d]: %q must match %s", at, j, tag, tagRe)
			}
		}
		if p.IdentityArgs != nil && len(p.IdentityArgs) == 0 {
			return fmt.Errorf("%s.identity_args: an empty list; leave the field out instead", at)
		}
		if len(p.IdentityArgs) > maxList {
			return fmt.Errorf("%s.identity_args: %d listed; at most %d are allowed", at, len(p.IdentityArgs), maxList)
		}
		for j, arg := range p.IdentityArgs {
			if arg == "" || strings.ContainsRune(arg, 0) {
				return fmt.Errorf("%s.identity_args[%d]: must be non-empty and hold no NUL", at, j)
			}
		}
	}
	return nil
}

// validatePackage accepts "." and "./" followed by a slash path with no
// empty, "." or ".." element and no backslash.
func validatePackage(pkg string) error {
	if pkg == "." {
		return nil
	}
	rest, ok := strings.CutPrefix(pkg, "./")
	if !ok {
		return fmt.Errorf("%q must be \".\" or start with \"./\"", pkg)
	}
	if err := validatePath(rest); err != nil {
		return fmt.Errorf("%q: %w", pkg, err)
	}
	return nil
}

// validatePath accepts a slash path relative to a root: fs.ValidPath, not
// ".", and no element that is "." or holds a backslash, a colon or NUL.
func validatePath(p string) error {
	if p == "." || !fs.ValidPath(p) {
		return fmt.Errorf("not a clean relative slash path")
	}
	for _, elem := range strings.Split(p, "/") {
		if elem == "." || strings.ContainsAny(elem, "\\:\x00") {
			return fmt.Errorf("element %q is not allowed", elem)
		}
	}
	return nil
}

func (s Spec) validatePlatforms() error {
	if len(s.Platforms) == 0 || len(s.Platforms) > maxPlatforms {
		return fmt.Errorf("releasespec: platforms: %d listed; 1 to %d are allowed", len(s.Platforms), maxPlatforms)
	}
	switch s.Packaging {
	case "", PackagingBinary, PackagingArchive:
	default:
		return fmt.Errorf("releasespec: packaging: %q is not %q or %q", s.Packaging, PackagingBinary, PackagingArchive)
	}
	seen := map[[2]string]bool{}
	for i, p := range s.Platforms {
		at := fmt.Sprintf("releasespec: platforms[%d]", i)
		if !platformFieldRe.MatchString(p.OS) {
			return fmt.Errorf("%s.os: %q must match %s", at, p.OS, platformFieldRe)
		}
		if !platformFieldRe.MatchString(p.Arch) {
			return fmt.Errorf("%s.arch: %q must match %s", at, p.Arch, platformFieldRe)
		}
		key := [2]string{p.OS, p.Arch}
		if seen[key] {
			return fmt.Errorf("%s: %s/%s is listed twice", at, p.OS, p.Arch)
		}
		seen[key] = true
		if p.Format == "" {
			continue
		}
		if s.Packaging != PackagingArchive {
			return fmt.Errorf("%s.format: %q needs \"packaging\": %q; a release is all archives or all binaries", at, p.Format, PackagingArchive)
		}
		switch p.Format {
		case archive.TarGz, archive.Zip, archive.Gz:
		default:
			return fmt.Errorf("%s.format: %q is not %q, %q or %q", at, p.Format, archive.TarGz, archive.Zip, archive.Gz)
		}
	}
	return nil
}

// validateArchiveNames checks, under archive packaging, that the client's
// archive selector accepts every composed asset name, and that a tar.gz
// program's name fits the USTAR header pack writes (0015-MADR E5).
func (s Spec) validateArchiveNames() error {
	if s.Packaging != PackagingArchive {
		return nil
	}
	for i, prod := range s.Products {
		at := fmt.Sprintf("releasespec: products[%d]", i)
		for _, p := range s.Platforms {
			t := p.Target()
			name, err := s.AssetName(prod.Name, t)
			if err != nil {
				return err
			}
			if !assetNameRe.MatchString(name) {
				return fmt.Errorf("%s: the asset name %q is %d characters; the archive selector accepts at most %d",
					at, name, len(name), maxAssetName)
			}
			if f, _ := s.FormatFor(t); f != archive.TarGz {
				continue
			}
			prog := prod.Name
			if t.OS == "windows" {
				prog += ".exe"
			}
			if len(prog) > maxTarName {
				return fmt.Errorf("%s: %q is %d characters; a tar.gz entry name holds at most %d", at, prog, len(prog), maxTarName)
			}
		}
	}
	return nil
}

func (s Spec) validateExtras() error {
	if len(s.Extras) > maxExtras {
		return fmt.Errorf("releasespec: extras: %d listed; at most %d are allowed", len(s.Extras), maxExtras)
	}
	for i, e := range s.Extras {
		if e.Path == "" {
			continue
		}
		if err := validatePath(e.Path); err != nil {
			return fmt.Errorf("releasespec: extras[%d].path: %q: %w", i, e.Path, err)
		}
	}
	_, err := s.ExtraNames(exampleTag)
	return err
}

// validateChannels applies check-release-tag.sh's channel rule.
func validateChannels(channels []string) error {
	for i, name := range channels {
		if !channelRe.MatchString(name) {
			return fmt.Errorf("releasespec: prerelease_channels[%d]: %q must match %s", i, name, channelRe)
		}
		if i > 0 && channels[i-1] <= name {
			return fmt.Errorf("releasespec: prerelease_channels[%d]: %q must come after %q: list channels in descending ASCII order, most stable first", i, channels[i-1], name)
		}
	}
	return nil
}
