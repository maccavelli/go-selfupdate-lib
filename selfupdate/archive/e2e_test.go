package archive

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/buildinfo"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/cli"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/selfupdatetest"
)

// End to end, for 0012-PLAN U3 step 9: releases whose assets are archives,
// through Updater and cli.Command.

// archiveRelease is a release of relay at tag whose assets are the
// archives in bodies, listed in a checksum asset named manifest.
func archiveRelease(tag, manifest string, bodies map[string][]byte) selfupdatetest.ReleaseSpec {
	spec := selfupdatetest.ReleaseSpec{Tag: tag, Immutable: true}
	var sums bytes.Buffer
	for name, b := range bodies {
		sum := sha256.Sum256(b)
		fmt.Fprintf(&sums, "%s  %s\n", hex.EncodeToString(sum[:]), name)
		spec.Assets = append(spec.Assets, selfupdatetest.AssetSpec{Name: name, Body: b})
	}
	spec.Assets = append(spec.Assets, selfupdatetest.AssetSpec{Name: manifest, Body: sums.Bytes()})
	return spec
}

type e2e struct {
	exe        string
	newUpdater func() (*selfupdate.Updater, error)
}

func newE2E(t *testing.T, opts SelectorOptions, spec selfupdatetest.ReleaseSpec) *e2e {
	t.Helper()
	d, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(d, "relay")
	if hostPlatform.OS == "windows" {
		exe += ".exe"
	}
	if err := os.WriteFile(exe, []byte("old"), 0o700); err != nil { //nolint:gosec // a stand-in for the installed program
		t.Fatal(err)
	}
	opts.Platforms = []selfupdate.Platform{hostPlatform}
	return &e2e{exe: exe, newUpdater: func() (*selfupdate.Updater, error) {
		sel, err := NewSelector(opts)
		if err != nil {
			return nil, err
		}
		unp, err := NewUnpacker(UnpackOptions{})
		if err != nil {
			return nil, err
		}
		inst, err := selfupdate.NewStandaloneInstaller(selfupdate.InstallOptions{
			TargetPolicy: selfupdate.TargetPolicy{ExecutablePath: exe, AllowedRoots: []string{d}},
		})
		if err != nil {
			return nil, err
		}
		return selfupdate.New(selfupdate.Config{
			Source: selfupdatetest.NewFakeSource(spec.Tag, spec), Versions: selfupdate.NewStrictVersionPolicy(),
			Assets: sel, Unpacker: unp, Installer: inst, Reporter: selfupdate.DiscardReporter(),
			Confirmer: selfupdate.NonInteractiveConfirmer(), Limits: selfupdate.DefaultLimits(),
		})
	}}
}

func hostArchive(t *testing.T, f Format) []byte {
	t.Helper()
	name := "relay"
	if hostPlatform.OS == "windows" {
		name += ".exe"
	}
	switch f {
	case Zip:
		return zipBytes(t, zfile("README.md", []byte("# relay\n")), zfile(name, hostProgram))
	case Gz:
		return gzipBytes(t, hostProgram)
	}
	return tarGz(t, file("README.md", []byte("# relay\n")), dir("relay_1.1.0/"), file("relay_1.1.0/"+name, hostProgram))
}

// TestUpdaterInstallsFromArchive: each format, in fleet and GoReleaser
// naming, applied and dry-run.
func TestUpdaterInstallsFromArchive(t *testing.T) {
	for _, f := range []Format{TarGz, Zip, Gz} {
		for _, naming := range []string{"fleet", "goreleaser"} {
			for _, dry := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/dry=%v", f, naming, dry), func(t *testing.T) {
					updateFromArchive(t, f, naming == "goreleaser", dry)
				})
			}
		}
	}
}

func updateFromArchive(t *testing.T, f Format, goreleaser, dry bool) {
	t.Helper()
	opts := SelectorOptions{Format: func(selfupdate.Platform) Format { return f }}
	manifest, nameFn := "SHA256SUMS", FleetName
	if goreleaser {
		opts.Name, opts.Manifest = GoReleaserName, GoReleaserChecksums
		manifest, nameFn = "relay_1.1.0_checksums.txt", GoReleaserName
	}
	asset, err := nameFn("relay", "v1.1.0", hostPlatform, f)
	if err != nil {
		t.Fatal(err)
	}
	archive := hostArchive(t, f)
	env := newE2E(t, opts, archiveRelease("v1.1.0", manifest, map[string][]byte{asset: archive}))
	u, err := env.newUpdater()
	if err != nil {
		t.Fatal(err)
	}
	res, err := u.Run(context.Background(), selfupdate.Request{
		Product: "relay", CurrentVersion: "v1.0.0", CurrentBuild: selfupdate.ReleaseBuild, Yes: true, DryRun: dry,
	})
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(archive)
	progSum := sha256.Sum256(hostProgram)
	if res.AssetName != asset || res.ReleaseDigest != hex.EncodeToString(sum[:]) || res.InstalledDigest != hex.EncodeToString(progSum[:]) {
		t.Fatalf("result %+v", res)
	}
	got, err := os.ReadFile(env.exe)
	if err != nil {
		t.Fatal(err)
	}
	want := hostProgram
	if dry {
		want = []byte("old")
	}
	if res.Applied == dry || !bytes.Equal(got, want) {
		t.Fatalf("applied=%v, installed %d bytes", res.Applied, len(got))
	}
}

// TestCommandShowsUnpacking: through cli.Command, the unpacking event shows
// in text and in --json.
func TestCommandShowsUnpacking(t *testing.T) {
	asset, err := FleetName("relay", "v1.1.0", hostPlatform, TarGz)
	if err != nil {
		t.Fatal(err)
	}
	id := buildinfo.Info{Version: "v1.0.0", Kind: buildinfo.KindRelease}
	for _, args := range [][]string{{"--yes"}, {"--yes", "--json"}} {
		env := newE2E(t, SelectorOptions{Format: func(selfupdate.Platform) Format { return TarGz }},
			archiveRelease("v1.1.0", "SHA256SUMS", map[string][]byte{asset: hostArchive(t, TarGz)}))
		var stdout, stderr bytes.Buffer
		o := cli.Options{Stdout: &stdout, Stderr: &stderr, Signals: []os.Signal{}}
		if code := cli.Command(context.Background(), args, "relay", id, env.newUpdater, o); code != 0 {
			t.Fatalf("%v: exit %d\n%s%s", args, code, stdout.String(), stderr.String())
		}
		out := stderr.String()
		if len(args) == 2 {
			out = stdout.String()
		}
		if !strings.Contains(out, "unpacking") {
			t.Fatalf("%v: no unpacking event in\n%s", args, out)
		}
		if got, _ := os.ReadFile(env.exe); !bytes.Equal(got, hostProgram) {
			t.Fatalf("%v: the program was not installed", args)
		}
	}
}
