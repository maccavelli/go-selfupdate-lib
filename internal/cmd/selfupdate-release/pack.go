package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/archive"
)

const goosWindows = "windows"

// programName is the program's name inside an archive: the product, plus
// ".exe" on Windows, the unpacker's default lookup.
func programName(product string, p selfupdate.Platform) string {
	if p.OS == goosWindows {
		return product + ".exe"
	}
	return product
}

// pack writes prog as the one member of an archive in format f, with fixed
// metadata, so the same input always packs to the same bytes (0013-MADR
// §6): mode 0755, owner 0, no user or group names, mtime, and a gzip header
// with no name, no time and OS 255.
func pack(w io.Writer, f archive.Format, name string, prog []byte, mtime time.Time) error {
	mtime = mtime.UTC().Truncate(time.Second)
	switch f {
	case archive.TarGz:
		gw := newGzip(w)
		tw := tar.NewWriter(gw)
		hdr := &tar.Header{
			Typeflag: tar.TypeReg, Name: name, Mode: 0o755, Size: int64(len(prog)),
			ModTime: mtime, Format: tar.FormatUSTAR,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if _, err := tw.Write(prog); err != nil {
			return err
		}
		return errors.Join(tw.Close(), gw.Close())
	case archive.Zip:
		zw := zip.NewWriter(w)
		hdr := &zip.FileHeader{Name: name, Method: zip.Deflate, Modified: mtime}
		hdr.SetMode(0o755)
		fw, err := zw.CreateHeader(hdr)
		if err != nil {
			return err
		}
		if _, err := fw.Write(prog); err != nil {
			return err
		}
		return zw.Close()
	case archive.Gz:
		gw := newGzip(w)
		if _, err := gw.Write(prog); err != nil {
			return err
		}
		return gw.Close()
	}
	return fmt.Errorf("unknown archive format %q", f)
}

// newGzip is a gzip writer whose header holds nothing that varies: no name,
// a zero modification time, and OS 255, Go's default.
func newGzip(w io.Writer) *gzip.Writer {
	gw := gzip.NewWriter(w)
	gw.Header = gzip.Header{OS: 255}
	return gw
}

// roundTrip unpacks the archive at path with the library's own unpacker,
// the client's code, and requires the program to equal want byte for byte.
func roundTrip(ctx context.Context, path, product string, p selfupdate.Platform, want []byte) error {
	got, err := unpackProgram(ctx, path, product, p)
	if err != nil {
		return err
	}
	if !bytes.Equal(got, want) {
		return fmt.Errorf("%s: the unpacked program differs from the built binary", filepath.Base(path))
	}
	return nil
}

// unpackProgram extracts the program from the archive at path with
// archive.NewUnpacker, which also checks it is an executable for p.
func unpackProgram(ctx context.Context, path, product string, p selfupdate.Platform) ([]byte, error) {
	u, err := archive.NewUnpacker(archive.UnpackOptions{})
	if err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp("", "selfupdate-release-unpack-*")
	if err != nil {
		return nil, err
	}
	prog := tmp.Name()
	defer os.Remove(prog) //nolint:errcheck // a temporary file
	if err := tmp.Close(); err != nil {
		return nil, err
	}
	req := selfupdate.UnpackRequest{
		Product: product, Platform: p, AssetName: filepath.Base(path),
		Archive: path, Program: prog, Limit: selfupdate.DefaultLimits().Executable,
	}
	if err := u.Unpack(ctx, req); err != nil {
		return nil, fmt.Errorf("%s: the client's unpacker refuses it: %w", filepath.Base(path), err)
	}
	return os.ReadFile(prog) //nolint:gosec // the temporary file made above
}
