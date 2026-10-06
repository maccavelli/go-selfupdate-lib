package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/maccavelli/go-selfupdate-lib/buildinfo"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
)

// buildinfoPkg is the package whose variables the stamp sets.
const buildinfoPkg = "github.com/maccavelli/go-selfupdate-lib/buildinfo"

type buildInput struct {
	SpecPath     string
	ModuleDir    string
	StampVersion string
	StampKind    string
	Out          string
}

// ldflags is the recipe's -ldflags (0013-MADR §4).
func ldflags(version, kind string) string {
	return "-s -w -X " + buildinfo.VersionVar + "=" + version + " -X " + buildinfo.KindVar + "=" + kind
}

// build compiles every product for every platform into in.Out, each named
// by selfupdate.ExactAssetName, after layer 1 of 0013-MADR §5.
func build(ctx context.Context, in buildInput, log io.Writer) error {
	if in.StampKind != kindRelease && in.StampKind != kindLocal {
		return usagef("-stamp-kind %q is not release or local", in.StampKind)
	}
	if !nameRe.MatchString(in.StampVersion) {
		return usagef("-stamp-version %q must match %s", in.StampVersion, nameRe)
	}
	spec, err := loadSpec(in.SpecPath)
	if err != nil {
		return err
	}
	g, err := newGoTool()
	if err != nil {
		return err
	}
	// go build runs in the module directory; -o must not resolve there.
	if in.Out, err = filepath.Abs(in.Out); err != nil {
		return err
	}
	if err := emptyDir(in.Out); err != nil {
		return err
	}
	targets, err := g.run(ctx, in.ModuleDir, nil, "tool", "dist", "list")
	if err != nil {
		return err
	}
	known := map[string]bool{}
	for _, t := range strings.Fields(targets) {
		known[t] = true
	}
	flags := ldflags(in.StampVersion, in.StampKind)
	for _, prod := range spec.Products {
		tags := []string{}
		if len(prod.Tags) > 0 {
			tags = []string{"-tags", strings.Join(prod.Tags, ",")}
		}
		for _, p := range spec.Targets() {
			if !known[p.OS+"/"+p.Arch] {
				return fmt.Errorf("%s/%s is not a target of this toolchain (go tool dist list)", p.OS, p.Arch)
			}
			env := []string{"GOOS=" + p.OS, "GOARCH=" + p.Arch}
			if err := requireBuildinfo(ctx, g, in.ModuleDir, env, tags, prod.Name, prod.Package, p); err != nil {
				return err
			}
			out := filepath.Join(in.Out, selfupdate.ExactAssetName(prod.Name, p))
			args := append([]string{"build", "-trimpath", "-buildvcs=true"}, tags...)
			args = append(args, "-ldflags", flags, "-o", out, prod.Package)
			if _, err := g.run(ctx, in.ModuleDir, env, args...); err != nil {
				return err
			}
			if err := printf(log, "built %s\n", filepath.Base(out)); err != nil {
				return err
			}
		}
	}
	return nil
}

// requireBuildinfo is layer 1: the product links buildinfo for p, so the
// stamp has a variable to set. The linker ignores -X for a missing one.
func requireBuildinfo(ctx context.Context, g goTool, dir string, env, tags []string, product, pkg string, p selfupdate.Platform) error {
	args := append(append([]string{"list", "-deps"}, tags...), pkg)
	out, err := g.run(ctx, dir, env, args...)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == buildinfoPkg {
			return nil
		}
	}
	return fmt.Errorf("product %s (%s) for %s/%s does not import %s; its release stamp would be lost", product, pkg, p.OS, p.Arch, buildinfoPkg)
}

// emptyDir creates dir, or requires an existing one to be empty.
func emptyDir(dir string) error {
	entries, err := os.ReadDir(dir)
	switch {
	case os.IsNotExist(err):
		return os.MkdirAll(dir, 0o755) //nolint:gosec // an output directory the workflow reads back
	case err != nil:
		return err
	case len(entries) > 0:
		return fmt.Errorf("output directory %s is not empty", dir)
	}
	return nil
}

func runBuild(ctx context.Context, args []string, stdout io.Writer) error {
	f := newFlags("build")
	spec := f.str("spec", true)
	moduleDir := f.str("module-dir", true)
	version := f.str("stamp-version", true)
	kind := f.str("stamp-kind", true)
	out := f.str("out", true)
	if err := f.parse(args); err != nil {
		return err
	}
	return build(ctx, buildInput{SpecPath: *spec, ModuleDir: *moduleDir, StampVersion: *version, StampKind: *kind, Out: *out}, stdout)
}
