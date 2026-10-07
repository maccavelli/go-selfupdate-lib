package archive_test

import (
	"fmt"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/archive"
)

// ExampleNewSelector selects a release published with GoReleaser's
// defaults: tar.gz on every OS, and a checksums file named for the
// version.
func ExampleNewSelector() {
	sel, err := archive.NewSelector(archive.SelectorOptions{
		Platforms: []selfupdate.Platform{{OS: "linux", Arch: "amd64"}, {OS: "windows", Arch: "amd64"}},
		Format:    func(selfupdate.Platform) archive.Format { return archive.TarGz },
		Name:      archive.GoReleaserName,
		Manifest:  archive.GoReleaserChecksums,
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	rel := selfupdate.Release{Tag: "v1.2.3", Assets: []selfupdate.Asset{
		{Name: "relay_1.2.3_linux_amd64.tar.gz"},
		{Name: "relay_1.2.3_windows_amd64.tar.gz"},
		{Name: "relay_1.2.3_checksums.txt"},
	}}
	got, err := sel.Select(rel, "relay", selfupdate.Platform{OS: "windows", Arch: "amd64"})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(got.Binary.Name, got.Manifest.Name, got.Packed)
	// Output: relay_1.2.3_windows_amd64.tar.gz relay_1.2.3_checksums.txt true
}

// ExampleNewUnpacker pairs the unpacker with the selector in a Config. The
// run downloads and verifies the archive, extracts the program into the
// session's staging, and installs the program.
func ExampleNewUnpacker() {
	platforms := []selfupdate.Platform{{OS: "linux", Arch: "amd64"}, {OS: "darwin", Arch: "arm64"}}
	sel, err := archive.NewSelector(archive.SelectorOptions{Platforms: platforms})
	if err != nil {
		fmt.Println(err)
		return
	}
	unp, err := archive.NewUnpacker(archive.UnpackOptions{})
	if err != nil {
		fmt.Println(err)
		return
	}
	cfg := selfupdate.Config{Assets: sel, Unpacker: unp}
	// Source, Versions, Installer, Reporter, Confirmer and Limits are set
	// as for any update, then selfupdate.New(cfg).
	rel := selfupdate.Release{Tag: "v1.2.3", Assets: []selfupdate.Asset{
		{Name: "relay-linux-amd64.tar.gz"}, {Name: "SHA256SUMS"},
	}}
	got, err := cfg.Assets.Select(rel, "relay", platforms[0])
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(got.Binary.Name, got.Packed, cfg.Unpacker != nil)
	// Output: relay-linux-amd64.tar.gz true true
}
