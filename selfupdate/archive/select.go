package archive

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
)

// Format is an archive format.
type Format string

// The formats this package reads.
const (
	// TarGz is a gzip-compressed tar archive, .tar.gz (.tgz is also read).
	TarGz Format = "tar.gz"
	// Zip is a zip archive, .zip.
	Zip Format = "zip"
	// Gz is one gzip-compressed binary, .gz, as GoReleaser's "gz" format.
	Gz Format = "gz"
)

// Extension returns the format's file extension with its dot, such as
// ".tar.gz", or "" for a format this package does not read.
func (f Format) Extension() string {
	switch f {
	case TarGz:
		return ".tar.gz"
	case Zip:
		return ".zip"
	case Gz:
		return ".gz"
	}
	return ""
}

// manifestName is the module's checksum asset, the Manifest default.
const manifestName = "SHA256SUMS"

// assetNameRe is the release workflow's asset name check
// (scripts/verify-selfupdate-release.sh), so a name this package builds is
// one the workflow could publish.
var assetNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// SelectorOptions configure NewSelector.
type SelectorOptions struct {
	// Platforms is the product's platform list, validated as
	// selfupdate.NewExactAssetSelector validates it.
	Platforms []selfupdate.Platform
	// Format picks the format for a platform. Nil means Zip on windows and
	// TarGz elsewhere.
	Format func(selfupdate.Platform) Format
	// Name builds the archive's asset name from the product, the release
	// tag, the platform and the format. Nil means FleetName.
	Name func(product, tag string, p selfupdate.Platform, f Format) (string, error)
	// Manifest names the checksum asset from the product and the release
	// tag. Nil means "SHA256SUMS".
	Manifest func(product, tag string) (string, error)
}

type selector struct {
	exact    selfupdate.AssetSelector
	format   func(selfupdate.Platform) Format
	name     func(product, tag string, p selfupdate.Platform, f Format) (string, error)
	manifest func(product, tag string) (string, error)
}

// NewSelector returns an AssetSelector that chooses the archive holding the
// program and the checksum asset, and marks the selection Packed. It
// refuses a platform list as NewExactAssetSelector refuses it. Pair it
// with NewUnpacker (0012-MADR §3).
func NewSelector(o SelectorOptions) (selfupdate.AssetSelector, error) {
	exact, err := selfupdate.NewExactAssetSelector(o.Platforms)
	if err != nil {
		return nil, fmt.Errorf("selfupdate: archive: %w", err)
	}
	s := &selector{exact: exact, format: o.Format, name: o.Name, manifest: o.Manifest}
	if s.format == nil {
		s.format = defaultFormat
	}
	if s.name == nil {
		s.name = FleetName
	}
	if s.manifest == nil {
		s.manifest = func(string, string) (string, error) { return manifestName, nil }
	}
	return s, nil
}

func defaultFormat(p selfupdate.Platform) Format {
	if p.OS == "windows" {
		return Zip
	}
	return TarGz
}

// Select implements selfupdate.AssetSelector.
func (s *selector) Select(rel selfupdate.Release, product string, p selfupdate.Platform) (selfupdate.Selection, error) {
	// The exact selector checks the product, the platform's fields and the
	// platform list, so both selectors refuse them alike. It is asked
	// about a release holding just the names it wants.
	probe := selfupdate.Release{Tag: rel.Tag, Assets: []selfupdate.Asset{
		{Name: selfupdate.ExactAssetName(product, p)}, {Name: manifestName},
	}}
	if _, err := s.exact.Select(probe, product, p); err != nil {
		return selfupdate.Selection{}, err
	}
	f := s.format(p)
	if f.Extension() == "" {
		return selfupdate.Selection{}, fmt.Errorf("selfupdate: archive: unknown archive format %q", f)
	}
	name, err := s.name(product, rel.Tag, p, f)
	if err != nil {
		return selfupdate.Selection{}, fmt.Errorf("selfupdate: archive: %w", err)
	}
	manifest, err := s.manifest(product, rel.Tag)
	if err != nil {
		return selfupdate.Selection{}, fmt.Errorf("selfupdate: archive: %w", err)
	}
	for _, n := range []string{name, manifest} {
		if !assetNameRe.MatchString(n) {
			return selfupdate.Selection{}, fmt.Errorf("selfupdate: archive: invalid asset name %q", n)
		}
	}
	if name == manifest || name == manifestName {
		return selfupdate.Selection{}, fmt.Errorf("selfupdate: archive: the archive name %q names the manifest", name)
	}
	archive, err := exactlyOne(rel, name)
	if err != nil {
		return selfupdate.Selection{}, err
	}
	sums, err := exactlyOne(rel, manifest)
	if err != nil {
		return selfupdate.Selection{}, err
	}
	return selfupdate.Selection{Binary: archive, Manifest: sums, ManifestName: name, Packed: true}, nil
}

// exactlyOne returns rel's one asset named name.
func exactlyOne(rel selfupdate.Release, name string) (selfupdate.Asset, error) {
	var found []selfupdate.Asset
	for _, a := range rel.Assets {
		if a.Name == name {
			found = append(found, a)
		}
	}
	switch len(found) {
	case 0:
		return selfupdate.Asset{}, fmt.Errorf("selfupdate: archive: release %s has no asset %q", rel.Tag, name)
	case 1:
		return found[0], nil
	}
	return selfupdate.Asset{}, fmt.Errorf("selfupdate: archive: release %s has a duplicate asset %q", rel.Tag, name)
}

// FleetName is this module's asset name with the format's extension:
// <product>-<os>-<arch>.tar.gz, .zip or .gz. The tag is not used.
func FleetName(product, _ string, p selfupdate.Platform, f Format) (string, error) {
	ext := f.Extension()
	if ext == "" {
		return "", fmt.Errorf("unknown archive format %q", f)
	}
	return product + "-" + p.OS + "-" + p.Arch + ext, nil
}

// goReleaserVariants are architectures whose default GoReleaser name adds a
// variant, such as GOARM, that selfupdate.Platform does not hold.
var goReleaserVariants = map[string]bool{"arm": true, "mips": true, "mipsle": true, "mips64": true, "mips64le": true}

// GoReleaserName is GoReleaser's default archive name,
// <product>_<version>_<os>_<arch>.<ext>, where version is the tag without
// its leading "v". It refuses arm and the mips architectures, whose default
// names carry a variant Platform does not hold. GoReleaser's default format
// is tar.gz on every OS, Windows included, so a selector using this name
// usually sets SelectorOptions.Format to return TarGz always.
func GoReleaserName(product, tag string, p selfupdate.Platform, f Format) (string, error) {
	ext := f.Extension()
	if ext == "" {
		return "", fmt.Errorf("unknown archive format %q", f)
	}
	if goReleaserVariants[p.Arch] {
		return "", fmt.Errorf("GoReleaser's default name for %s/%s carries a variant that Platform does not hold", p.OS, p.Arch)
	}
	return product + "_" + strings.TrimPrefix(tag, "v") + "_" + p.OS + "_" + p.Arch + ext, nil
}

// GoReleaserChecksums is GoReleaser's default checksum asset name,
// <product>_<version>_checksums.txt, where version is the tag without its
// leading "v".
func GoReleaserChecksums(product, tag string) (string, error) {
	if tag == "" {
		return "", errors.New("GoReleaser's checksum name needs a tag")
	}
	return product + "_" + strings.TrimPrefix(tag, "v") + "_checksums.txt", nil
}
