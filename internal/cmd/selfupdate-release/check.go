package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
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
	// The client's selector, over the packed platforms: it must choose
	// each archive by the name it is staged under (0015-MADR E5).
	var packed []selfupdate.Platform
	formats := map[selfupdate.Platform]archive.Format{}
	for _, pp := range platforms {
		if pp.Format != "" {
			p := selfupdate.Platform{OS: pp.OS, Arch: pp.Arch}
			packed = append(packed, p)
			formats[p] = pp.Format
		}
	}
	var sel selfupdate.AssetSelector
	if len(packed) > 0 {
		var err error
		sel, err = archive.NewSelector(archive.SelectorOptions{
			Platforms: packed,
			Format:    func(p selfupdate.Platform) archive.Format { return formats[p] },
		})
		if err != nil {
			return nil, err
		}
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
			rel := selfupdate.Release{Assets: []selfupdate.Asset{{Name: name}, {Name: manifestName}}}
			if _, err := sel.Select(rel, prod, p); err != nil {
				return nil, fmt.Errorf("%s: the client's selector refuses it: %w", name, err)
			}
			path := filepath.Join(dir, name)
			if pp.Format == archive.TarGz || pp.Format == archive.Gz {
				if err := wholeGzip(path); err != nil {
					return nil, fmt.Errorf("%s: the gzip stream is not whole: %w", name, err)
				}
			}
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

// wholeGzip reads a gzip asset to its end: one member, its trailer's CRC
// and length checked, and nothing after it. The client's unpacker stops at
// the program and leaves the rest unread (0012-MADR §4); the installers
// unpack with gzip and tar, which read to the end, so the publish check
// does too (docs/decisions/0014-PLAN-shared-installer-templates.md
// deviation D8).
func wholeGzip(path string) error {
	f, err := os.Open(path) //nolint:gosec // a staged asset
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck // read only
	br := bufio.NewReader(f)
	gz, err := gzip.NewReader(br)
	if err != nil {
		return err
	}
	gz.Multistream(false)
	limit := selfupdate.DefaultLimits().Executable + maxGzipOverhead
	n, err := io.Copy(io.Discard, io.LimitReader(gz, limit+1))
	if err != nil {
		return err
	}
	if n > limit {
		return fmt.Errorf("it expands past %d bytes", limit)
	}
	if _, err := br.ReadByte(); !errors.Is(err, io.EOF) {
		return errors.New("data follows the gzip member")
	}
	return nil
}

// maxGzipOverhead is what a release archive may expand to beyond its
// program: tar's headers and padding.
const maxGzipOverhead = 8 << 20

// manifestName is the checksum asset the client's selector requires beside
// each archive, as stage writes it.
const manifestName = "SHA256SUMS"

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
