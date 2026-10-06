package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// runInstaller renders the spec's installers into a directory, for a local
// look or a test (0014-MADR §1).
func runInstaller(_ context.Context, args []string, stdout io.Writer) error {
	f := newFlags("installer")
	specPath := f.str("spec", true)
	repository := f.str("repository", true)
	tag := f.str("tag", true)
	out := f.str("out", true)
	if err := f.parse(args); err != nil {
		return err
	}
	spec, err := loadSpec(*specPath)
	if err != nil {
		return err
	}
	if spec.Installer == nil {
		return usagef("the spec has no installer field")
	}
	files, err := renderInstallers(spec, *repository, *tag)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(*out, 0o755); err != nil { //nolint:gosec // an output directory the caller reads
		return err
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		path := filepath.Join(*out, name)
		if err := os.WriteFile(path, files[name], 0o644); err != nil { //nolint:gosec // a script the caller reads
			return err
		}
		if err := printf(stdout, "rendered %s\n", path); err != nil {
			return err
		}
	}
	return nil
}
