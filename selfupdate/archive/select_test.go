package archive

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
)

// Tests for docs/decisions/0012-PLAN-archive-assets-and-macos-codesign.md
// U2: selecting an archive asset (0012-MADR §3).

var (
	linuxAMD64   = selfupdate.Platform{OS: "linux", Arch: "amd64"}
	darwinARM64  = selfupdate.Platform{OS: "darwin", Arch: "arm64"}
	windowsAMD64 = selfupdate.Platform{OS: "windows", Arch: "amd64"}
	fleet        = []selfupdate.Platform{linuxAMD64, darwinARM64, windowsAMD64}
)

func release(tag string, names ...string) selfupdate.Release {
	rel := selfupdate.Release{Tag: tag}
	for i, n := range names {
		rel.Assets = append(rel.Assets, selfupdate.Asset{ID: int64(i + 1), Name: n, State: selfupdate.AssetStateUploaded, Size: 1})
	}
	return rel
}

func TestFormatExtension(t *testing.T) {
	for f, want := range map[Format]string{TarGz: ".tar.gz", Zip: ".zip", Gz: ".gz", "tar.xz": "", "": ""} {
		if got := f.Extension(); got != want {
			t.Errorf("%q.Extension() = %q, want %q", f, got, want)
		}
	}
}

func TestFleetName(t *testing.T) {
	for _, c := range []struct {
		p    selfupdate.Platform
		f    Format
		want string
	}{
		{linuxAMD64, TarGz, "relay-linux-amd64.tar.gz"},
		{windowsAMD64, Zip, "relay-windows-amd64.zip"},
		{darwinARM64, Gz, "relay-darwin-arm64.gz"},
	} {
		got, err := FleetName("relay", "v1.2.3", c.p, c.f)
		if err != nil || got != c.want {
			t.Errorf("FleetName(%v, %s) = %q, %v; want %q", c.p, c.f, got, err, c.want)
		}
	}
	if _, err := FleetName("relay", "v1.2.3", linuxAMD64, "tar.xz"); err == nil {
		t.Error("FleetName accepted an unknown format")
	}
}

func TestGoReleaserNames(t *testing.T) {
	for _, tag := range []string{"v1.2.3", "1.2.3"} {
		got, err := GoReleaserName("relay", tag, linuxAMD64, TarGz)
		if err != nil || got != "relay_1.2.3_linux_amd64.tar.gz" {
			t.Errorf("GoReleaserName(%q) = %q, %v", tag, got, err)
		}
		sums, err := GoReleaserChecksums("relay", tag)
		if err != nil || sums != "relay_1.2.3_checksums.txt" {
			t.Errorf("GoReleaserChecksums(%q) = %q, %v", tag, sums, err)
		}
	}
	got, err := GoReleaserName("relay", "v1.2.3", windowsAMD64, Zip)
	if err != nil || got != "relay_1.2.3_windows_amd64.zip" {
		t.Errorf("windows: %q, %v", got, err)
	}
	// Architectures whose default GoReleaser name carries a variant that
	// Platform does not hold.
	for _, arch := range []string{"arm", "mips", "mipsle", "mips64", "mips64le"} {
		if got, err := GoReleaserName("relay", "v1.2.3", selfupdate.Platform{OS: "linux", Arch: arch}, TarGz); err == nil {
			t.Errorf("%s: accepted as %q", arch, got)
		}
	}
	if _, err := GoReleaserName("relay", "v1.2.3", linuxAMD64, "tar.xz"); err == nil {
		t.Error("GoReleaserName accepted an unknown format")
	}
}

// TestNewSelectorRefusesPlatforms: the platform list is refused exactly as
// NewExactAssetSelector refuses it.
func TestNewSelectorRefusesPlatforms(t *testing.T) {
	for name, ps := range map[string][]selfupdate.Platform{
		"empty":     nil,
		"duplicate": {linuxAMD64, linuxAMD64},
		"upper":     {{OS: "Linux", Arch: "amd64"}},
		"slash":     {{OS: "linux", Arch: "amd64/x"}},
	} {
		_, want := selfupdate.NewExactAssetSelector(ps)
		_, got := NewSelector(SelectorOptions{Platforms: ps})
		if want == nil || got == nil || !strings.Contains(got.Error(), want.Error()) {
			t.Errorf("%s: NewSelector = %v; the exact selector says %v", name, got, want)
		}
	}
}

func TestSelectDefaults(t *testing.T) {
	sel, err := NewSelector(SelectorOptions{Platforms: fleet})
	if err != nil {
		t.Fatal(err)
	}
	rel := release("v1.2.3", "relay-linux-amd64.tar.gz", "relay-darwin-arm64.tar.gz",
		"relay-windows-amd64.zip", "SHA256SUMS", "install.sh", "relay-linux-amd64")
	for _, c := range []struct {
		p    selfupdate.Platform
		want string
	}{
		{linuxAMD64, "relay-linux-amd64.tar.gz"},
		{darwinARM64, "relay-darwin-arm64.tar.gz"},
		{windowsAMD64, "relay-windows-amd64.zip"},
	} {
		got, err := sel.Select(rel, "relay", c.p)
		if err != nil {
			t.Fatalf("%v: %v", c.p, err)
		}
		if got.Binary.Name != c.want || got.Manifest.Name != "SHA256SUMS" || got.ManifestName != c.want || !got.Packed {
			t.Errorf("%v: %+v", c.p, got)
		}
	}
}

func TestSelectGoReleaser(t *testing.T) {
	sel, err := NewSelector(SelectorOptions{
		Platforms: fleet,
		Format:    func(selfupdate.Platform) Format { return TarGz },
		Name:      GoReleaserName,
		Manifest:  GoReleaserChecksums,
	})
	if err != nil {
		t.Fatal(err)
	}
	rel := release("v1.2.3", "relay_1.2.3_linux_amd64.tar.gz", "relay_1.2.3_windows_amd64.tar.gz",
		"relay_1.2.3_checksums.txt")
	got, err := sel.Select(rel, "relay", windowsAMD64)
	if err != nil {
		t.Fatal(err)
	}
	if got.Binary.Name != "relay_1.2.3_windows_amd64.tar.gz" || got.Manifest.Name != "relay_1.2.3_checksums.txt" ||
		got.ManifestName != got.Binary.Name || !got.Packed {
		t.Fatalf("%+v", got)
	}
}

func TestSelectRefusals(t *testing.T) {
	good := release("v1.2.3", "relay-linux-amd64.tar.gz", "SHA256SUMS")
	named := func(name string) func(string, string, selfupdate.Platform, Format) (string, error) {
		return func(string, string, selfupdate.Platform, Format) (string, error) { return name, nil }
	}
	for _, c := range []struct {
		name    string
		opts    SelectorOptions
		rel     selfupdate.Release
		product string
		p       selfupdate.Platform
		want    string
	}{
		{"platform outside the list", SelectorOptions{}, good, "relay", selfupdate.Platform{OS: "linux", Arch: "riscv64"}, "not in the product platform matrix"},
		{"invalid product", SelectorOptions{}, good, "../relay", linuxAMD64, "invalid product name"},
		{"no archive", SelectorOptions{}, release("v1.2.3", "SHA256SUMS"), "relay", linuxAMD64, `no asset "relay-linux-amd64.tar.gz"`},
		{"no manifest", SelectorOptions{}, release("v1.2.3", "relay-linux-amd64.tar.gz"), "relay", linuxAMD64, `no asset "SHA256SUMS"`},
		{"duplicate archive", SelectorOptions{}, release("v1.2.3", "relay-linux-amd64.tar.gz", "relay-linux-amd64.tar.gz", "SHA256SUMS"), "relay", linuxAMD64, "duplicate asset"},
		{"duplicate manifest", SelectorOptions{}, release("v1.2.3", "relay-linux-amd64.tar.gz", "SHA256SUMS", "SHA256SUMS"), "relay", linuxAMD64, "duplicate asset"},
		{"name with a slash", SelectorOptions{Name: named("dir/relay.tar.gz")}, good, "relay", linuxAMD64, "invalid asset name"},
		{"name with dots", SelectorOptions{Name: named("..")}, good, "relay", linuxAMD64, "invalid asset name"},
		{"empty name", SelectorOptions{Name: named("")}, good, "relay", linuxAMD64, "invalid asset name"},
		{"long name", SelectorOptions{Name: named(strings.Repeat("a", 129))}, good, "relay", linuxAMD64, "invalid asset name"},
		{"name is SHA256SUMS", SelectorOptions{Name: named("SHA256SUMS")}, good, "relay", linuxAMD64, "names the manifest"},
		{"bad manifest name", SelectorOptions{Manifest: func(string, string) (string, error) { return "sums/x", nil }}, good, "relay", linuxAMD64, "invalid asset name"},
		{"name error", SelectorOptions{Name: func(string, string, selfupdate.Platform, Format) (string, error) {
			return "", errors.New("no name for you")
		}}, good, "relay", linuxAMD64, "no name for you"},
		{"unknown format", SelectorOptions{Format: func(selfupdate.Platform) Format { return "tar.xz" }}, good, "relay", linuxAMD64, "unknown archive format"},
	} {
		t.Run(c.name, func(t *testing.T) {
			c.opts.Platforms = []selfupdate.Platform{linuxAMD64}
			sel, err := NewSelector(c.opts)
			if err != nil {
				t.Fatal(err)
			}
			_, err = sel.Select(c.rel, c.product, c.p)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("Select = %v, want %q", err, c.want)
			}
			if c.name == "platform outside the list" && !errors.Is(err, selfupdate.ErrUnsupportedPlatform) {
				t.Fatalf("%v is not ErrUnsupportedPlatform", err)
			}
		})
	}
}

// TestNameCheckMatchesReleaseWorkflow: the name check is the release
// workflow's own expression, read from its verifier, so the two cannot
// drift.
func TestNameCheckMatchesReleaseWorkflow(t *testing.T) {
	script, err := os.ReadFile(filepath.Join("..", "..", "scripts", "verify-selfupdate-release.sh"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^product_re = re\.compile\(r"([^"]+)"\)$`).FindSubmatch(script)
	if m == nil {
		t.Fatal("product_re not found in scripts/verify-selfupdate-release.sh")
	}
	if got := assetNameRe.String(); got != string(m[1]) {
		t.Fatalf("archive name check %q, release workflow %q", got, m[1])
	}
}

// TestGoReleaserChecksumsFile: a real GoReleaser checksum file parses as
// SHA256SUMS, and a consumer's own Name selects from the names it lists.
// testdata/goreleaser-v2.18.2-checksums.txt is checksums.txt of the
// immutable release v2.18.2 of github.com/goreleaser/goreleaser, fetched
// unchanged on 2026-10-05 (0012-PLAN U2 step 9). GoReleaser's own archives
// use the uname style ("goreleaser_Darwin_arm64.tar.gz",
// "goreleaser_Linux_x86_64.tar.gz", zip on Windows), not the default
// template, which is why that style is a Name the consumer writes
// (0012-MADR §3).
func TestGoReleaserChecksumsFile(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "goreleaser-v2.18.2-checksums.txt"))
	if err != nil {
		t.Fatal(err)
	}
	sums, err := selfupdate.ParseSHA256SUMS(data)
	if err != nil {
		t.Fatalf("GoReleaser's checksums.txt does not parse as SHA256SUMS: %v", err)
	}
	if len(sums) != 53 {
		t.Fatalf("%d entries, want 53", len(sums))
	}
	uname := func(product, _ string, p selfupdate.Platform, f Format) (string, error) {
		arch := map[string]string{"amd64": "x86_64", "386": "i386"}[p.Arch]
		if arch == "" {
			arch = p.Arch
		}
		return product + "_" + strings.ToUpper(p.OS[:1]) + p.OS[1:] + "_" + arch + f.Extension(), nil
	}
	sel, err := NewSelector(SelectorOptions{
		Platforms: fleet, Name: uname,
		Manifest: func(string, string) (string, error) { return "checksums.txt", nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	names := []string{"checksums.txt"}
	for n := range sums {
		names = append(names, n)
	}
	rel := release("v2.18.2", names...)
	for p, want := range map[selfupdate.Platform]string{
		darwinARM64:  "goreleaser_Darwin_arm64.tar.gz",
		linuxAMD64:   "goreleaser_Linux_x86_64.tar.gz",
		windowsAMD64: "goreleaser_Windows_x86_64.zip",
	} {
		got, err := sel.Select(rel, "goreleaser", p)
		if err != nil || got.Binary.Name != want || sums[got.ManifestName] == "" {
			t.Errorf("%v: %+v, %v; want %s with a checksum", p, got, err, want)
		}
	}
}

// TestSelectorWithUnpacker: New accepts this selector beside an Unpacker,
// unlike the exact one.
func TestSelectorWithUnpacker(t *testing.T) {
	sel, err := NewSelector(SelectorOptions{Platforms: fleet})
	if err != nil {
		t.Fatal(err)
	}
	_, err = selfupdate.New(selfupdate.Config{
		Source: nopSource{}, Versions: selfupdate.NewStrictVersionPolicy(), Assets: sel,
		Unpacker:  selfupdate.UnpackerFunc(func(context.Context, selfupdate.UnpackRequest) error { return nil }),
		Installer: nopInstaller{}, Reporter: selfupdate.DiscardReporter(),
		Confirmer: selfupdate.NonInteractiveConfirmer(), Limits: selfupdate.DefaultLimits(),
	})
	if err != nil {
		t.Fatal(err)
	}
}

type nopSource struct{ selfupdate.ReleaseSource }

type nopInstaller struct{ selfupdate.Installer }
