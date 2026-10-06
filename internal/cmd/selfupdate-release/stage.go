package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/releasespec"
)

type stageInput struct {
	SpecPath     string
	ModuleDir    string
	Src          string // the repository root
	Bin          string // build's output
	Out          string
	SHA          string
	Tag          string // empty in a rehearsal
	StampVersion string // replaces {tag} in the extras' names
	ExtrasDir    string // the extras that have no path
	Repository   string // owner/name, for the installers
}

// binaryWant is what layer 2 of 0013-MADR §5 requires of one binary's build
// information.
type binaryWant struct {
	Platform  selfupdate.Platform
	Tags      string // comma-separated, empty for none
	SHA       string
	GoVersion string
	Module    string
	Package   string // the main package's import path
	Tag       string // checked as the main module version when set
}

// checkBuildInfo is layer 2 of 0013-MADR §5 on one binary's build
// information.
func checkBuildInfo(bi *buildinfo.BuildInfo, w binaryWant) error {
	settings := map[string]string{}
	for _, s := range bi.Settings {
		settings[s.Key] = s.Value
	}
	checks := []struct{ what, got, want string }{
		{"GOOS", settings["GOOS"], w.Platform.OS},
		{"GOARCH", settings["GOARCH"], w.Platform.Arch},
		{"-tags", settings["-tags"], w.Tags},
		{"CGO_ENABLED", settings["CGO_ENABLED"], "0"},
		{"-trimpath", settings["-trimpath"], "true"},
		{"vcs.revision", settings["vcs.revision"], w.SHA},
		{"vcs.modified", settings["vcs.modified"], "false"},
		{"the Go version", bi.GoVersion, w.GoVersion},
		{"the main module", bi.Main.Path, w.Module},
		{"the main package", bi.Path, w.Package},
	}
	if w.Tag != "" {
		checks = append(checks, struct{ what, got, want string }{"the main module version", bi.Main.Version, w.Tag})
	}
	for _, c := range checks {
		if c.got != c.want {
			return fmt.Errorf("%s is %q, want %q", c.what, c.got, c.want)
		}
	}
	return nil
}

// stage checks each binary, packs it when the spec says so, writes
// SHA256SUMS and copies the extras into in.Out (0013-MADR §5, §6).
func stage(ctx context.Context, in stageInput, log io.Writer) (map[string]string, error) {
	if !shaRe.MatchString(in.SHA) {
		return nil, usagef("-sha %q is not a full commit SHA", in.SHA)
	}
	spec, err := loadSpec(in.SpecPath)
	if err != nil {
		return nil, err
	}
	g, err := newGoTool()
	if err != nil {
		return nil, err
	}
	goVersion, err := g.version(ctx, in.ModuleDir)
	if err != nil {
		return nil, err
	}
	module, err := g.modulePath(ctx, in.ModuleDir)
	if err != nil {
		return nil, err
	}
	tagAtRoot, err := sameDir(in.ModuleDir, in.Src)
	if err != nil {
		return nil, err
	}
	if err := emptyDir(in.Out); err != nil {
		return nil, err
	}
	sums := map[string]string{}
	for _, prod := range spec.Products {
		want := binaryWant{
			Tags: strings.Join(prod.Tags, ","), SHA: in.SHA, GoVersion: goVersion, Module: module,
			Package: path.Join(module, strings.TrimPrefix(prod.Package, "./")),
		}
		if tagAtRoot {
			want.Tag = in.Tag
		}
		for _, p := range spec.Targets() {
			want.Platform = p
			name, sum, err := stageOne(ctx, spec, in, prod.Name, want)
			if err != nil {
				return nil, err
			}
			sums[name] = sum
			if err := printf(log, "staged %s %s\n", sum, name); err != nil {
				return nil, err
			}
		}
	}
	if err := writeSums(ctx, in.Out, sums); err != nil {
		return nil, err
	}
	if err := copyExtras(spec, in); err != nil {
		return nil, err
	}
	if err := writeInstallers(spec, in); err != nil {
		return nil, err
	}
	return sums, nil
}

// writeInstallers renders the spec's installers into the staging
// directory (0014-MADR §2, §3). They are extras: SHA256SUMS does not list
// them, and ExtraNames does.
func writeInstallers(spec releasespec.Spec, in stageInput) error {
	if spec.Installer == nil {
		return nil
	}
	if in.Repository == "" {
		return usagef("the spec asks for installers; -repository is required")
	}
	files, err := renderInstallers(spec, in.Repository, in.StampVersion)
	if err != nil {
		return err
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(in.Out, name), data, 0o644); err != nil { //nolint:gosec // a release asset
			return err
		}
	}
	return nil
}

// stageOne checks one binary and stages its asset. It returns the asset's
// name and SHA-256.
func stageOne(ctx context.Context, spec releasespec.Spec, in stageInput, product string, want binaryWant) (string, string, error) {
	p := want.Platform
	binPath := filepath.Join(in.Bin, selfupdate.ExactAssetName(product, p))
	prog, err := os.ReadFile(binPath) //nolint:gosec // build's own output
	if err != nil {
		return "", "", err
	}
	if len(prog) == 0 {
		return "", "", fmt.Errorf("%s is empty", filepath.Base(binPath))
	}
	bi, err := buildinfo.Read(bytes.NewReader(prog))
	if err != nil {
		return "", "", fmt.Errorf("%s: no Go build information: %w", filepath.Base(binPath), err)
	}
	if err := checkBuildInfo(bi, want); err != nil {
		return "", "", fmt.Errorf("%s: %w", filepath.Base(binPath), err)
	}
	if err := selfupdate.CheckImage(bytes.NewReader(prog), p); err != nil {
		return "", "", fmt.Errorf("%s: %w", filepath.Base(binPath), err)
	}
	name, err := spec.AssetName(product, p)
	if err != nil {
		return "", "", err
	}
	asset := prog
	if f, packed := spec.FormatFor(p); packed {
		mtime, err := vcsTime(bi)
		if err != nil {
			return "", "", fmt.Errorf("%s: %w", filepath.Base(binPath), err)
		}
		var buf bytes.Buffer
		if err := pack(&buf, f, programName(product, p), prog, mtime); err != nil {
			return "", "", err
		}
		asset = buf.Bytes()
	}
	dst := filepath.Join(in.Out, name)
	if err := os.WriteFile(dst, asset, 0o644); err != nil { //nolint:gosec // a release asset, published to everyone
		return "", "", err
	}
	if _, packed := spec.FormatFor(p); packed {
		if err := roundTrip(ctx, dst, product, p, prog); err != nil {
			return "", "", err
		}
	}
	sum := sha256.Sum256(asset)
	return name, hex.EncodeToString(sum[:]), nil
}

// vcsTime is the commit time the build recorded, the archives'
// modification time.
func vcsTime(bi *buildinfo.BuildInfo) (time.Time, error) {
	for _, s := range bi.Settings {
		if s.Key == "vcs.time" {
			return time.Parse(time.RFC3339, s.Value)
		}
	}
	return time.Time{}, fmt.Errorf("no vcs.time in the build information")
}

// writeSums writes SHA256SUMS, "<hex>  <name>" lines sorted by name in byte
// order, and parses it back with the client's parser.
func writeSums(_ context.Context, dir string, sums map[string]string) error {
	names := make([]string, 0, len(sums))
	for n := range sums {
		names = append(names, n)
	}
	sort.Strings(names)
	var b bytes.Buffer
	for _, n := range names {
		fmt.Fprintf(&b, "%s  %s\n", sums[n], n)
	}
	parsed, err := selfupdate.ParseSHA256SUMS(b.Bytes())
	if err != nil {
		return fmt.Errorf("the client refuses the SHA256SUMS written: %w", err)
	}
	if len(parsed) != len(sums) {
		return fmt.Errorf("SHA256SUMS parses to %d entries, want %d", len(parsed), len(sums))
	}
	for n, s := range sums {
		if parsed[n] != s {
			return fmt.Errorf("SHA256SUMS parses %s as %q, want %q", n, parsed[n], s)
		}
	}
	return os.WriteFile(filepath.Join(dir, "SHA256SUMS"), b.Bytes(), 0o644) //nolint:gosec // a release asset
}

// copyExtras copies each extra: from its path in the repository, or from
// the extras directory. Only regular files are accepted, and the extras
// directory may hold nothing else.
func copyExtras(spec releasespec.Spec, in stageInput) error {
	names, err := spec.ExtraNames(in.StampVersion)
	if err != nil {
		return err
	}
	fromArtifact := map[string]bool{}
	for i, e := range spec.Extras {
		src := filepath.Join(in.ExtrasDir, names[i])
		if e.Path != "" {
			src = filepath.Join(in.Src, filepath.FromSlash(e.Path))
		} else {
			if in.ExtrasDir == "" {
				return fmt.Errorf("extra %s has no path, and no extras directory was given", names[i])
			}
			fromArtifact[names[i]] = true
		}
		if err := copyRegular(src, filepath.Join(in.Out, names[i])); err != nil {
			return fmt.Errorf("extra %s: %w", names[i], err)
		}
	}
	if in.ExtrasDir == "" {
		return nil
	}
	entries, err := os.ReadDir(in.ExtrasDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !fromArtifact[e.Name()] {
			return fmt.Errorf("the extras directory holds %s, which the spec does not list", e.Name())
		}
	}
	return nil
}

func copyRegular(src, dst string) error {
	fi, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", src)
	}
	data, err := os.ReadFile(src) //nolint:gosec // a path the spec names, checked to be a regular file
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644) //nolint:gosec // a release asset
}

// sameDir reports whether a and b are the same directory.
func sameDir(a, b string) (bool, error) {
	fa, err := os.Stat(a)
	if err != nil {
		return false, err
	}
	fb, err := os.Stat(b)
	if err != nil {
		return false, err
	}
	return os.SameFile(fa, fb), nil
}

func runStage(ctx context.Context, args []string, stdout io.Writer) error {
	f := newFlags("stage")
	spec := f.str("spec", true)
	moduleDir := f.str("module-dir", true)
	src := f.str("src", true)
	bin := f.str("bin", true)
	out := f.str("out", true)
	sha := f.str("sha", true)
	version := f.str("stamp-version", true)
	tag := f.str("tag", false)
	extras := f.str("extras-dir", false)
	repository := f.str("repository", false)
	summary := f.str("summary", false)
	if err := f.parse(args); err != nil {
		return err
	}
	in := stageInput{SpecPath: *spec, ModuleDir: *moduleDir, Src: *src, Bin: *bin, Out: *out,
		SHA: *sha, Tag: *tag, StampVersion: *version, ExtrasDir: *extras, Repository: *repository}
	sums, err := stage(ctx, in, stdout)
	if err != nil {
		return err
	}
	return appendSummary(*summary, stageSummary(sums))
}

func stageSummary(sums map[string]string) string {
	names := make([]string, 0, len(sums))
	for n := range sums {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("## Staged assets\n\n| Asset | SHA-256 |\n| :--- | :--- |\n")
	for _, n := range names {
		fmt.Fprintf(&b, "| `%s` | `%s` |\n", n, sums[n])
	}
	return b.String() + "\n"
}
