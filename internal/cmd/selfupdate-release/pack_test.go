package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/archive"
)

var packTime = time.Date(2026, 10, 5, 12, 34, 56, 789, time.UTC)

// hostProgram is the fixture built for this host: a real executable the
// unpacker's image check accepts.
func hostProgram(t *testing.T) []byte {
	t.Helper()
	b := sharedBuild(t)
	data, err := os.ReadFile(filepath.Join(b.bin, selfupdate.ExactAssetName("relay", host)))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func packed(t *testing.T, f archive.Format, name string, prog []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := pack(&buf, f, name, prog, packTime); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestPackMetadata(t *testing.T) {
	prog := []byte("\x7fELF not really")
	t.Run("tar.gz", func(t *testing.T) {
		data := packed(t, archive.TarGz, "relay", prog)
		gr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		if gr.Name != "" || !gr.ModTime.IsZero() || gr.OS != 255 {
			t.Errorf("gzip header: name %q time %v os %d", gr.Name, gr.ModTime, gr.OS)
		}
		tr := tar.NewReader(gr)
		hdr, err := tr.Next()
		if err != nil {
			t.Fatal(err)
		}
		if hdr.Name != "relay" || hdr.Mode != 0o755 || hdr.Typeflag != tar.TypeReg || hdr.Uid != 0 || hdr.Gid != 0 ||
			hdr.Uname != "" || hdr.Gname != "" || !hdr.ModTime.Equal(packTime.Truncate(time.Second)) || hdr.Format != tar.FormatUSTAR {
			t.Errorf("tar header: %+v", hdr)
		}
		if got, _ := io.ReadAll(tr); !bytes.Equal(got, prog) {
			t.Error("tar member is not the program")
		}
		if _, err := tr.Next(); err != io.EOF {
			t.Errorf("a second member: %v", err)
		}
	})
	t.Run("zip", func(t *testing.T) {
		data := packed(t, archive.Zip, "relay.exe", prog)
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Fatal(err)
		}
		if len(zr.File) != 1 {
			t.Fatalf("%d members", len(zr.File))
		}
		f := zr.File[0]
		if f.Name != "relay.exe" || f.Mode().Perm() != 0o755 || !f.Mode().IsRegular() || f.CreatorVersion>>8 != 3 ||
			f.Method != zip.Deflate || !f.Modified.Equal(packTime.Truncate(time.Second)) {
			t.Errorf("zip header: name %q mode %v creator %d method %d modified %v", f.Name, f.Mode(), f.CreatorVersion>>8, f.Method, f.Modified)
		}
	})
	t.Run("gz", func(t *testing.T) {
		data := packed(t, archive.Gz, "relay", prog)
		gr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		if gr.Name != "" || !gr.ModTime.IsZero() || gr.OS != 255 {
			t.Errorf("gzip header: name %q time %v os %d", gr.Name, gr.ModTime, gr.OS)
		}
		if got, _ := io.ReadAll(gr); !bytes.Equal(got, prog) {
			t.Error("gz is not the program")
		}
	})
	t.Run("repeatable", func(t *testing.T) {
		for _, f := range []archive.Format{archive.TarGz, archive.Zip, archive.Gz} {
			if !bytes.Equal(packed(t, f, "relay", prog), packed(t, f, "relay", prog)) {
				t.Errorf("%s packs differently twice", f)
			}
		}
	})
	t.Run("unknown format", func(t *testing.T) {
		if err := pack(io.Discard, "tar.xz", "relay", prog, packTime); err == nil {
			t.Fatal("pack accepted tar.xz")
		}
	})
}

func TestRoundTripRefusesACorruptArchive(t *testing.T) {
	prog := hostProgram(t)
	dir := t.TempDir()
	name := programName("relay", host)
	data := packed(t, archive.TarGz, name, prog)
	path := filepath.Join(dir, "relay.tar.gz")
	if err := os.WriteFile(path, data[:len(data)/2], 0o644); err != nil {
		t.Fatal(err)
	}
	if err := roundTrip(context.Background(), path, "relay", host, prog); err == nil || !strings.Contains(err.Error(), "the client's unpacker refuses it") {
		t.Fatalf("roundTrip: %v", err)
	}
	// A byte in the middle, in the code: the image stays valid, so only
	// the comparison catches it. The end of an ELF file is its section
	// header table, which the unpacker's image check reads.
	other := append([]byte{}, prog...)
	other[len(other)/2] ^= 0xff
	if err := os.WriteFile(path, packed(t, archive.TarGz, name, other), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := roundTrip(context.Background(), path, "relay", host, prog); err == nil || !strings.Contains(err.Error(), "differs from the built binary") {
		t.Fatalf("roundTrip: %v", err)
	}
}

// tarOf builds a tar.gz from raw headers and bodies.
func tarOf(t *testing.T, members ...func(tw *tar.Writer)) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	for _, m := range members {
		m(tw)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestCheck(t *testing.T) {
	prog := hostProgram(t)
	name := programName("relay", host)
	regular := func(n string) func(tw *tar.Writer) {
		return func(tw *tar.Writer) {
			if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: n, Mode: 0o755, Size: int64(len(prog))}); err != nil {
				t.Fatal(err)
			}
			if _, err := tw.Write(prog); err != nil {
				t.Fatal(err)
			}
		}
	}
	var symlinkZip bytes.Buffer
	zw := zip.NewWriter(&symlinkZip)
	hdr := &zip.FileHeader{Name: name}
	hdr.SetMode(os.ModeSymlink | 0o777)
	w, err := zw.CreateHeader(hdr)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("/bin/sh")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	good := packed(t, archive.TarGz, name, prog)
	goodGz := packed(t, archive.Gz, name, prog)
	// Changed only in gzip's trailer, so the program inside is intact:
	// the client's unpacker accepts these (0012-MADR §4), and gzip and tar,
	// which the installers use, refuse them (0014-PLAN deviation D8).
	badCRC := slices.Clone(good)
	badCRC[len(badCRC)-8] ^= 0xff

	for _, tc := range []struct {
		name   string
		format archive.Format
		body   []byte
		want   string // empty: accepted
	}{
		{"a good tar.gz", archive.TarGz, good, ""},
		{"a good zip", archive.Zip, packed(t, archive.Zip, name, prog), ""},
		{"a good gz", archive.Gz, packed(t, archive.Gz, name, prog), ""},
		{"the program twice", archive.TarGz, tarOf(t, regular(name), regular(name)), "the client's unpacker refuses it"},
		{"the program as a symlink", archive.Zip, symlinkZip.Bytes(), "the client's unpacker refuses it"},
		{"a truncated gzip", archive.TarGz, good[:len(good)-12], "the gzip stream is not whole: unexpected EOF"},
		{"a tar.gz without its gzip trailer", archive.TarGz, good[:len(good)-8], "the gzip stream is not whole: unexpected EOF"},
		{"a tar.gz with a wrong gzip checksum", archive.TarGz, badCRC, "the gzip stream is not whole: gzip: invalid checksum"},
		{"data after the tar.gz member", archive.TarGz, append(slices.Clone(good), "junk"...), "the gzip stream is not whole: data follows the gzip member"},
		{"a second gzip member after a tar.gz", archive.TarGz, append(slices.Clone(good), goodGz...), "data follows the gzip member"},
		{"a gz without its trailer", archive.Gz, goodGz[:len(goodGz)-8], "the gzip stream is not whole: unexpected EOF"},
		{"a tar.gz cut in half", archive.TarGz, good[:len(good)/2], "the gzip stream is not whole: unexpected EOF"},
		{"a script, not an executable", archive.Gz, packed(t, archive.Gz, name, []byte("#!/bin/sh\n")), "the client's unpacker refuses it"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			asset, _ := archive.FleetName("relay", "", host, tc.format)
			if err := os.WriteFile(filepath.Join(dir, asset), tc.body, 0o644); err != nil {
				t.Fatal(err)
			}
			platforms := `[{"os":"` + host.OS + `","arch":"` + host.Arch + `","format":"` + string(tc.format) + `"}]`
			checked, err := check(context.Background(), dir, `["relay"]`, platforms)
			if tc.want == "" {
				if err != nil || len(checked) != 1 {
					t.Fatalf("check: %v, %v", checked, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("check: %v; want %q", err, tc.want)
			}
		})
	}
	t.Run("a raw release", func(t *testing.T) {
		checked, err := check(context.Background(), t.TempDir(), `["relay"]`, `[{"os":"linux","arch":"amd64"}]`)
		if err != nil || len(checked) != 0 {
			t.Fatalf("check: %v, %v", checked, err)
		}
	})
	t.Run("bad JSON", func(t *testing.T) {
		if _, err := check(context.Background(), t.TempDir(), `[`, `[]`); err == nil {
			t.Fatal("check accepted bad JSON")
		}
	})
}
