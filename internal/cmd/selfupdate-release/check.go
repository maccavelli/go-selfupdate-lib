package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/archive"
)

// publishPlatform is one object of the publish workflow's platforms-json.
type publishPlatform struct {
	OS     string         `json:"os"`
	Arch   string         `json:"arch"`
	Format archive.Format `json:"format"`
}

// check is the publish workflow's archive check (0013-MADR §8): every
// archive in a packed release must unpack with the client's own unpacker
// to an executable for its platform. A release with no format is not
// packed, and has nothing to check. It returns the archives checked.
func check(ctx context.Context, dir, productsJSON, platformsJSON string) ([]string, error) {
	var products []string
	if err := json.Unmarshal([]byte(productsJSON), &products); err != nil {
		return nil, usagef("-products-json: %v", err)
	}
	var platforms []publishPlatform
	if err := json.Unmarshal([]byte(platformsJSON), &platforms); err != nil {
		return nil, usagef("-platforms-json: %v", err)
	}
	var checked []string
	for _, prod := range products {
		for _, pp := range platforms {
			if pp.Format == "" {
				continue
			}
			p := selfupdate.Platform{OS: pp.OS, Arch: pp.Arch}
			name, err := archive.FleetName(prod, "", p, pp.Format)
			if err != nil {
				return nil, err
			}
			path := filepath.Join(dir, name)
			prog, err := unpackProgram(ctx, path, prod, p)
			if err != nil {
				return nil, err
			}
			if err := selfupdate.CheckImage(bytes.NewReader(prog), p); err != nil {
				return nil, fmt.Errorf("%s: %w", name, err)
			}
			checked = append(checked, name)
		}
	}
	return checked, nil
}

func runCheck(ctx context.Context, args []string, stdout io.Writer) error {
	f := newFlags("check")
	dir := f.str("dir", true)
	products := f.str("products-json", true)
	platforms := f.str("platforms-json", true)
	if err := f.parse(args); err != nil {
		return err
	}
	checked, err := check(ctx, *dir, *products, *platforms)
	if err != nil {
		return err
	}
	if len(checked) == 0 {
		return printf(stdout, "selfupdate-release check: no archives; nothing to check\n")
	}
	for _, name := range checked {
		if err := printf(stdout, "selfupdate-release check: %s unpacks to an executable for its platform\n", name); err != nil {
			return err
		}
	}
	return nil
}
