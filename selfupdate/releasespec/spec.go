package releasespec

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/archive"
)

// SchemaVersion is the only schema this package reads.
const SchemaVersion = 1

// MaxSize is the largest spec Parse reads, in bytes.
const MaxSize = 64 << 10

// TagPlaceholder is replaced by the release tag in an extra asset's name.
const TagPlaceholder = "{tag}"

// Packaging is how a release ships its programs.
type Packaging string

// The packagings.
const (
	// PackagingBinary ships each program as a raw binary,
	// <product>-<os>-<arch>, plus ".exe" on Windows. It is the default.
	PackagingBinary Packaging = "binary"
	// PackagingArchive ships each program in an archive, named by
	// archive.FleetName.
	PackagingArchive Packaging = "archive"
)

// goosWindows is the GOOS whose programs end in ".exe" and whose default
// archive is a zip.
const goosWindows = "windows"

// Spec is a release spec.
type Spec struct {
	// Schema is SchemaVersion.
	Schema int `json:"schema"`
	// Products are the programs the release ships, 1 to 16.
	Products []Product `json:"products"`
	// Platforms are the targets every product is built for, 1 to 32.
	Platforms []Platform `json:"platforms"`
	// Packaging is PackagingBinary, PackagingArchive, or empty for
	// PackagingBinary.
	Packaging Packaging `json:"packaging,omitempty"`
	// Extras are further release assets, at most 32. SHA256SUMS does not
	// list them.
	Extras []Extra `json:"extras,omitempty"`
	// PrereleaseChannels name the prerelease channels the release workflow
	// may publish vX.Y.Z-NAME.N tags for, most stable first, in strictly
	// descending ASCII order.
	PrereleaseChannels []string `json:"prerelease_channels,omitempty"`
	// Installer, when present, asks the build workflow to generate
	// install.sh and install.ps1 (docs/decisions/0014-MADR-shared-installer-templates.md).
	Installer *Installer `json:"installer,omitempty"`
}

// Product is one program.
type Product struct {
	// Name is the product name: the asset name's prefix and the
	// selfupdate.Request product.
	Name string `json:"name"`
	// Package is the main package, relative to the module directory: "."
	// or "./" followed by a slash path.
	Package string `json:"package"`
	// Tags are build tags, at most 16.
	Tags []string `json:"tags,omitempty"`
	// IdentityArgs, when present, are the arguments that make the program
	// print buildinfo.Identity().String() as the first line of its
	// stdout. The workflow then runs it on each platform that has a
	// runner. At most 16, and never an empty list.
	IdentityArgs []string `json:"identity_args,omitempty"`
}

// Platform is one target.
type Platform struct {
	// OS is a GOOS value.
	OS string `json:"os"`
	// Arch is a GOARCH value.
	Arch string `json:"arch"`
	// Format is the archive format under PackagingArchive. Empty means zip
	// on Windows and tar.gz elsewhere. It must be empty under
	// PackagingBinary.
	Format archive.Format `json:"format,omitempty"`
}

// Target returns the platform as a selfupdate.Platform.
func (p Platform) Target() selfupdate.Platform {
	return selfupdate.Platform{OS: p.OS, Arch: p.Arch}
}

// Extra is a further release asset.
type Extra struct {
	// Name is the asset name. TagPlaceholder in it is replaced by the
	// release tag.
	Name string `json:"name"`
	// Path, when set, is the file in the calling repository, as a slash
	// path relative to its root. When empty, the file comes from the
	// caller's extras artifact.
	Path string `json:"path,omitempty"`
}

// Parse reads and validates a spec. It refuses more than MaxSize bytes, any
// field it does not know, a key that is not exactly a field's name, a
// duplicate key, and anything after the one JSON value.
func Parse(data []byte) (Spec, error) {
	if len(data) > MaxSize {
		return Spec{}, fmt.Errorf("releasespec: the spec is %d bytes, more than %d", len(data), MaxSize)
	}
	if err := checkKeys(data); err != nil {
		return Spec{}, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var s Spec
	if err := dec.Decode(&s); err != nil {
		return Spec{}, fmt.Errorf("releasespec: %w", err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Spec{}, errors.New("releasespec: data after the spec")
	}
	if err := s.Validate(); err != nil {
		return Spec{}, err
	}
	s.normalize()
	return s, nil
}

// normalize makes an empty optional list nil, so a spec re-encodes to the
// same value.
func (s *Spec) normalize() {
	if len(s.Extras) == 0 {
		s.Extras = nil
	}
	if len(s.PrereleaseChannels) == 0 {
		s.PrereleaseChannels = nil
	}
	for i := range s.Products {
		if len(s.Products[i].Tags) == 0 {
			s.Products[i].Tags = nil
		}
	}
	if s.Installer != nil && len(s.Installer.Hooks) == 0 {
		s.Installer.Hooks = nil
	}
}

// checkKeys walks the JSON and refuses a duplicate key, or a key that is
// not lowercase letters and underscores. encoding/json matches keys
// case-insensitively and keeps the last of two, so either would otherwise
// pass silently. It refuses null too, which encoding/json reads as an
// absent field: "installer": null is not "installer": {} (0015-MADR E7).
func checkKeys(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var walk func(key string) error
	walk = func(key string) error {
		tok, err := dec.Token()
		if err != nil {
			return fmt.Errorf("releasespec: %w", err)
		}
		if tok == nil {
			if key == "" {
				return errors.New("releasespec: null is not allowed")
			}
			return fmt.Errorf("releasespec: %q: null is not allowed; leave the field out", key)
		}
		switch tok {
		case json.Delim('{'):
			seen := map[string]bool{}
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return fmt.Errorf("releasespec: %w", err)
				}
				key, ok := kt.(string)
				if !ok || !keyRe.MatchString(key) {
					return fmt.Errorf("releasespec: key %q is not a field name", key)
				}
				if seen[key] {
					return fmt.Errorf("releasespec: duplicate key %q", key)
				}
				seen[key] = true
				if err := walk(key); err != nil {
					return err
				}
			}
			_, err = dec.Token()
		case json.Delim('['):
			for dec.More() {
				if err := walk(key); err != nil {
					return err
				}
			}
			_, err = dec.Token()
		}
		if err != nil {
			return fmt.Errorf("releasespec: %w", err)
		}
		return nil
	}
	return walk("")
}

// Product returns the product named name, or an error naming the products
// the spec lists. A program that calls it with its own product name fails
// at startup if the two differ.
func (s Spec) Product(name string) (Product, error) {
	for _, p := range s.Products {
		if p.Name == name {
			return p, nil
		}
	}
	names := make([]string, len(s.Products))
	for i, p := range s.Products {
		names[i] = p.Name
	}
	return Product{}, fmt.Errorf("releasespec: product %q is not in the spec (products: %s)", name, strings.Join(names, ", "))
}

// Targets returns the platforms, in spec order.
func (s Spec) Targets() []selfupdate.Platform {
	out := make([]selfupdate.Platform, len(s.Platforms))
	for i, p := range s.Platforms {
		out[i] = p.Target()
	}
	return out
}

// platform returns the spec's entry for p.
func (s Spec) platform(p selfupdate.Platform) (Platform, bool) {
	for _, sp := range s.Platforms {
		if sp.OS == p.OS && sp.Arch == p.Arch {
			return sp, true
		}
	}
	return Platform{}, false
}

// FormatFor returns the archive format for p, and true, when the spec is
// PackagingArchive and lists p. Otherwise it returns "" and false.
func (s Spec) FormatFor(p selfupdate.Platform) (archive.Format, bool) {
	if s.Packaging != PackagingArchive {
		return "", false
	}
	sp, ok := s.platform(p)
	if !ok {
		return "", false
	}
	if sp.Format != "" {
		return sp.Format, true
	}
	if p.OS == goosWindows {
		return archive.Zip, true
	}
	return archive.TarGz, true
}

// AssetName returns the canonical asset name of product on p:
// selfupdate.ExactAssetName, or archive.FleetName for p's format. Both
// must be in the spec.
func (s Spec) AssetName(product string, p selfupdate.Platform) (string, error) {
	if _, err := s.Product(product); err != nil {
		return "", err
	}
	if _, ok := s.platform(p); !ok {
		return "", fmt.Errorf("releasespec: platform %s/%s is not in the spec", p.OS, p.Arch)
	}
	if f, ok := s.FormatFor(p); ok {
		name, err := archive.FleetName(product, "", p, f)
		if err != nil {
			return "", fmt.Errorf("releasespec: %w", err)
		}
		return name, nil
	}
	return selfupdate.ExactAssetName(product, p), nil
}

// assetNames returns every canonical asset name, products in spec order,
// then platforms in spec order.
func (s Spec) assetNames() ([]string, error) {
	var out []string
	for _, prod := range s.Products {
		for _, p := range s.Platforms {
			name, err := s.AssetName(prod.Name, p.Target())
			if err != nil {
				return nil, err
			}
			out = append(out, name)
		}
	}
	return out, nil
}

// AssetSelector returns the selector for the spec's packaging:
// selfupdate.NewExactAssetSelector, or archive.NewSelector with the spec's
// formats. Pair it with Unpacker.
func (s Spec) AssetSelector() (selfupdate.AssetSelector, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	if s.Packaging != PackagingArchive {
		return selfupdate.NewExactAssetSelector(s.Targets())
	}
	return archive.NewSelector(archive.SelectorOptions{
		Platforms: s.Targets(),
		Format: func(p selfupdate.Platform) archive.Format {
			f, _ := s.FormatFor(p)
			return f
		},
	})
}

// Unpacker returns nil under PackagingBinary, and archive.NewUnpacker under
// PackagingArchive, so it always matches AssetSelector.
func (s Spec) Unpacker() (selfupdate.Unpacker, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	if s.Packaging != PackagingArchive {
		return nil, nil
	}
	return archive.NewUnpacker(archive.UnpackOptions{})
}

// ExtraNames returns the extras' asset names for the release tag, in spec
// order, with TagPlaceholder replaced, then the generated installers
// (InstallerScripts). Each extra must still be a valid, unique asset name
// that is not SHA256SUMS, a canonical asset's name or an installer's.
func (s Spec) ExtraNames(tag string) ([]string, error) {
	canonical, err := s.assetNames()
	if err != nil {
		return nil, err
	}
	installers := s.InstallerScripts()
	names := make([]string, len(s.Extras), len(s.Extras)+len(installers))
	taken := map[string]string{}
	for _, c := range canonical {
		taken[strings.ToLower(c)] = "a canonical asset"
	}
	if s.Installer != nil {
		// Both names are reserved whenever the installers are on, even the
		// one this spec's platforms do not generate.
		taken[InstallerScript] = "an installer"
		taken[InstallerPowerShell] = "an installer"
	}
	for i, e := range s.Extras {
		name := strings.ReplaceAll(e.Name, TagPlaceholder, tag)
		if err := checkExtraName(name, taken); err != nil {
			return nil, fmt.Errorf("releasespec: extras[%d].name: %w", i, err)
		}
		taken[strings.ToLower(name)] = "another extra"
		names[i] = name
	}
	return append(names, installers...), nil
}

func checkExtraName(name string, taken map[string]string) error {
	if !assetNameRe.MatchString(name) {
		return fmt.Errorf("%q must match %s", name, assetNameRe)
	}
	if name == "SHA256SUMS" || strings.HasPrefix(name, "SHA256SUMS-") {
		return fmt.Errorf("%q is reserved for the checksum manifest", name)
	}
	if what, ok := taken[strings.ToLower(name)]; ok {
		return fmt.Errorf("%q is already %s's name", name, what)
	}
	return nil
}

// ProductsJSON returns the product names as the publish workflow's
// products-json.
func (s Spec) ProductsJSON() string {
	names := make([]string, len(s.Products))
	for i, p := range s.Products {
		names[i] = p.Name
	}
	return mustJSON(names)
}

// publishPlatform is a platforms-json object. Format is set, resolved to
// its default, under PackagingArchive.
type publishPlatform struct {
	OS     string         `json:"os"`
	Arch   string         `json:"arch"`
	Format archive.Format `json:"format,omitempty"`
}

// PlatformsJSON returns the platforms as the publish workflow's
// platforms-json, each with its resolved format under PackagingArchive.
func (s Spec) PlatformsJSON() string {
	out := make([]publishPlatform, len(s.Platforms))
	for i, p := range s.Platforms {
		f, _ := s.FormatFor(p.Target())
		out[i] = publishPlatform{OS: p.OS, Arch: p.Arch, Format: f}
	}
	return mustJSON(out)
}

// ExtrasJSON returns ExtraNames(tag) as the publish workflow's
// extra-assets-json.
func (s Spec) ExtrasJSON(tag string) (string, error) {
	names, err := s.ExtraNames(tag)
	if err != nil {
		return "", err
	}
	return mustJSON(names), nil
}

// ChannelsJSON returns the prerelease channels as the publish workflow's
// prerelease-channels-json; "[]" when there are none.
func (s Spec) ChannelsJSON() string {
	if len(s.PrereleaseChannels) == 0 {
		return "[]"
	}
	return mustJSON(s.PrereleaseChannels)
}

// mustJSON encodes v, which is always a list of strings or of
// publishPlatform. An empty list is "[]".
func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic("releasespec: " + err.Error())
	}
	return string(b)
}
