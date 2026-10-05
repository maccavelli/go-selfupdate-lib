package archive

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/flate"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
)

// Tests for docs/decisions/0012-PLAN-archive-assets-and-macos-codesign.md
// U3: extracting the program (0012-MADR §4).

const testLimit = 64 << 20

// unpack runs u on data, as asset, for p, and returns what it wrote.
func unpack(t *testing.T, u selfupdate.Unpacker, asset string, data []byte, p selfupdate.Platform, limit int64) ([]byte, error) {
	t.Helper()
	return unpackCtx(context.Background(), t, u, asset, data, p, limit)
}

func unpackCtx(ctx context.Context, t *testing.T, u selfupdate.Unpacker, asset string, data []byte, p selfupdate.Platform, limit int64) ([]byte, error) {
	t.Helper()
	d := t.TempDir()
	archive, program := filepath.Join(d, "archive"), filepath.Join(d, "program")
	if err := os.WriteFile(archive, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(program, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	err := u.Unpack(ctx, selfupdate.UnpackRequest{
		Product: "relay", Platform: p, AssetName: asset, Archive: archive, Program: program, Limit: limit,
	})
	got, rerr := os.ReadFile(program) //nolint:gosec // the test's own file
	if rerr != nil {
		t.Fatal(rerr)
	}
	return got, err
}

func mustUnpacker(t *testing.T, o UnpackOptions) selfupdate.Unpacker {
	t.Helper()
	u, err := NewUnpacker(o)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestNewUnpackerRefuses(t *testing.T) {
	for _, o := range []UnpackOptions{
		{Member: "/abs/relay"}, {Member: "../relay"}, {Member: "a/"}, {Member: "a//relay"}, {MaxEntries: -1},
	} {
		if _, err := NewUnpacker(o); err == nil {
			t.Errorf("%+v accepted", o)
		}
	}
}

// TestUnpackAccepts: each format and layout the MADR accepts writes the
// program exactly.
func TestUnpackAccepts(t *testing.T) {
	windows := selfupdate.Platform{OS: "windows", Arch: "amd64"}
	readme := []byte("# relay\n")
	cases := []struct {
		name   string
		opts   UnpackOptions
		asset  string
		data   []byte
		p      selfupdate.Platform
		wanted []byte
	}{
		{"tar.gz, top level, beside other files", UnpackOptions{}, "relay-x.tar.gz",
			tarGz(t, dir("./"), file("README.md", readme), file("LICENSE", readme), dir("docs/"), file("docs/x.md", readme), file("./"+prog, hostProgram)),
			hostPlatform, hostProgram},
		{"tar.gz, one directory down", UnpackOptions{}, "relay-x.tar.gz",
			tarGz(t, dir("relay_1.2.3/"), file("relay_1.2.3/README.md", readme), file("relay_1.2.3/"+prog, hostProgram)),
			hostPlatform, hostProgram},
		{".tgz", UnpackOptions{}, "relay-x.tgz", tarGz(t, file(prog, hostProgram)), hostPlatform, hostProgram},
		{"Member, two directories down", UnpackOptions{Member: "a/b/relay"}, "relay-x.tar.gz",
			tarGz(t, file("a/b/relay", hostProgram), file("relay.sh", readme)), hostPlatform, hostProgram},
		{"zip", UnpackOptions{}, "relay-x.zip", zipBytes(t, zfile("README.md", readme), zfile(prog, hostProgram)), hostPlatform, hostProgram},
		{"zip, .exe on windows", UnpackOptions{}, "relay-windows-amd64.zip",
			zipBytes(t, zfile("relay.exe", windowsProgram), zfile("relay", readme)), windows, windowsProgram},
		{"gz", UnpackOptions{}, "relay-x.gz", gzipBytes(t, hostProgram), hostPlatform, hostProgram},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := unpack(t, mustUnpacker(t, c.opts), c.asset, c.data, c.p, testLimit)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, c.wanted) {
				t.Fatalf("wrote %d bytes, want the %d-byte program", len(got), len(c.wanted))
			}
		})
	}
}

// overlapZip is a zip whose second entry's central-directory record points
// at the first entry's local header, so their data overlap.
func overlapZip(t *testing.T) []byte {
	t.Helper()
	b := zipBytes(t, zfile("relay", []byte("one")), zfile("other", []byte("two")))
	cd := []byte("PK\x01\x02")
	first := bytes.Index(b, cd)
	second := first + 1 + bytes.Index(b[first+1:], cd)
	if first < 0 || second <= first {
		t.Fatal("central directory records not found")
	}
	binary.LittleEndian.PutUint32(b[second+42:], 0)
	return b
}

// lyingZip is a zip whose program entry declares 16 bytes and holds 1 MiB.
func lyingZip(t *testing.T) []byte {
	t.Helper()
	body := make([]byte, 1<<20)
	var comp bytes.Buffer
	fw, err := flate.NewWriter(&comp, flate.BestCompression)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := fw.Close(); err != nil {
		t.Fatal(err)
	}
	h := zip.FileHeader{Name: prog, Method: zip.Deflate, CRC32: crc32.ChecksumIEEE(body),
		CompressedSize64: uint64(comp.Len()), UncompressedSize64: 16}
	h.SetMode(0o755)
	return zipBytes(t, zipEntry{hdr: h, body: comp.Bytes(), raw: true})
}

// TestUnpackRefuses: one case for each MADR §4 rule. Every refusal wraps
// ErrIntegrity, and none is accepted.
func TestUnpackRefuses(t *testing.T) {
	small := []byte("small")
	mib := make([]byte, 1<<20)
	link := func(name string, typ byte) tarEntry {
		return tarEntry{tar.Header{Name: name, Typeflag: typ, Linkname: "/bin/sh", Mode: 0o755}, nil}
	}
	special := func(name string, typ byte) tarEntry {
		return tarEntry{tar.Header{Name: name, Typeflag: typ, Mode: 0o644}, nil}
	}
	zlink := zfile(prog, []byte("/bin/sh"))
	zlink.hdr.SetMode(os.ModeSymlink | 0o777)
	odd := zipEntry{hdr: zip.FileHeader{Name: "README", Method: 99, CompressedSize64: 1, UncompressedSize64: 1}, body: []byte("x"), raw: true}
	cases := []struct {
		name  string
		opts  UnpackOptions
		asset string
		data  []byte
		limit int64
		want  string
	}{
		// Names.
		{"absolute", UnpackOptions{}, "a.tar.gz", tarGz(t, file("/relay", small)), 0, "not safe"},
		{"dot-dot", UnpackOptions{}, "a.tar.gz", tarGz(t, file("../relay", small)), 0, "not safe"},
		{"dot-dot inside", UnpackOptions{}, "a.tar.gz", tarGz(t, file("a/../../relay", small)), 0, "not safe"},
		{"backslash", UnpackOptions{}, "a.tar.gz", tarGz(t, file(`a\relay`, small)), 0, "not safe"},
		{"NUL", UnpackOptions{}, "a.zip", zipBytes(t, zfile("re\x00lay", small)), 0, "not safe"},
		{"zip absolute", UnpackOptions{}, "a.zip", zipBytes(t, zfile("/relay", small)), 0, "not safe"},
		{"zip dot-dot", UnpackOptions{}, "a.zip", zipBytes(t, zfile("../relay", small)), 0, "not safe"},
		// Duplicates.
		{"duplicate", UnpackOptions{}, "a.tar.gz", tarGz(t, file("relay", small), file("relay", small)), 0, "repeats a name"},
		{"duplicate after cleaning", UnpackOptions{}, "a.tar.gz", tarGz(t, file("relay", small), file("./relay", small)), 0, "repeats a name"},
		{"differs only in case", UnpackOptions{}, "a.tar.gz", tarGz(t, file("README", small), file("readme", small), file("relay", small)), 0, "differs from another only in case"},
		{"zip duplicate", UnpackOptions{}, "a.zip", zipBytes(t, zfile("relay", small), zfile("relay", small)), 0, "repeats a name"},
		// Links, devices, FIFOs and sparse files.
		{"symlink named as the program", UnpackOptions{}, "a.tar.gz", tarGz(t, link(prog, tar.TypeSymlink)), 0, "not a regular file"},
		{"hard link named as the program", UnpackOptions{}, "a.tar.gz", tarGz(t, link(prog, tar.TypeLink)), 0, "not a regular file"},
		{"symlink elsewhere", UnpackOptions{}, "a.tar.gz", tarGz(t, file("relay", small), link("docs/x", tar.TypeSymlink)), 0, "not a regular file"},
		{"character device", UnpackOptions{}, "a.tar.gz", tarGz(t, special("dev", tar.TypeChar), file("relay", small)), 0, "not a regular file"},
		{"block device", UnpackOptions{}, "a.tar.gz", tarGz(t, special("dev", tar.TypeBlock), file("relay", small)), 0, "not a regular file"},
		{"FIFO", UnpackOptions{}, "a.tar.gz", tarGz(t, special("fifo", tar.TypeFifo), file("relay", small)), 0, "not a regular file"},
		{"GNU sparse", UnpackOptions{}, "a.tar.gz", gnuSparseTarGz(t), 0, "tar:"},
		{"PAX sparse", UnpackOptions{}, "a.tar.gz", paxSparseTarGz(t), 0, "is a sparse file"},
		{"zip symlink", UnpackOptions{}, "a.zip", zipBytes(t, zlink), 0, "not a regular file"},
		// Counts and sizes.
		{"too many entries", UnpackOptions{MaxEntries: 3}, "a.tar.gz", tarGz(t, file("a", small), file("b", small), file("c", small), file("relay", small)), 0, "more than 3 entries"},
		{"zip too many entries", UnpackOptions{MaxEntries: 3}, "a.zip", zipBytes(t, zfile("a", small), zfile("b", small), zfile("c", small), zfile("relay", small)), 0, "more than 3 entries"},
		{"declared over the limit", UnpackOptions{}, "a.tar.gz", tarGz(t, file("README", make([]byte, 2<<20)), file("relay", small)), 1 << 20, "declare more than"},
		{"entries together over the limit", UnpackOptions{}, "a.tar.gz", tarGz(t, file("a", mib[:400<<10]), file("b", mib[:400<<10]), file("relay", mib[:400<<10])), 1 << 20, "declare more than"},
		{"zip declared over the limit", UnpackOptions{}, "a.zip", zipBytes(t, zfile("relay", make([]byte, 2<<20))), 1 << 20, "declares more than"},
		{"zip declaring less than it holds", UnpackOptions{}, "a.zip", lyingZip(t), 0, "reading the program"},
		{"gz bomb", UnpackOptions{}, "a.gz", gzipBytes(t, make([]byte, 4<<20)), 1 << 20, "larger than"},
		// The zip layout.
		{"overlapping entries", UnpackOptions{}, "a.zip", overlapZip(t), 0, "overlap"},
		{"unknown method", UnpackOptions{}, "a.zip", zipBytes(t, odd, zfile("relay", small)), 0, "compression method 99"},
		// The format.
		{"zip named tar.gz", UnpackOptions{}, "a.tar.gz", zipBytes(t, zfile("relay", small)), 0, "not gzip"},
		{"gzip named zip", UnpackOptions{}, "a.zip", gzipBytes(t, small), 0, "not zip"},
		{"unknown suffix", UnpackOptions{}, "a.tar.xz", []byte("x"), 0, "is not a .tar.gz"},
		{"not a tar inside the gzip", UnpackOptions{}, "a.tar.gz", gzipBytes(t, []byte("not a tar archive at all")), 0, "tar:"},
		{"data after the gz member", UnpackOptions{}, "a.gz", append(gzipBytes(t, small), "junk"...), 0, "data follows"},
		{"a second gz member", UnpackOptions{}, "a.gz", append(gzipBytes(t, small), gzipBytes(t, small)...), 0, "data follows"},
		// The program.
		{"no program", UnpackOptions{}, "a.tar.gz", tarGz(t, file("README", small)), 0, "no program"},
		{"two programs", UnpackOptions{}, "a.tar.gz", tarGz(t, file(prog, small), file("x/"+prog, small)), 0, "more than one program"},
		{"zip two programs", UnpackOptions{}, "a.zip", zipBytes(t, zfile(prog, small), zfile("x/"+prog, small)), 0, "more than one program"},
		{"two directories down", UnpackOptions{}, "a.tar.gz", tarGz(t, file("a/b/"+prog, small)), 0, "no program"},
		{"wrong architecture", UnpackOptions{}, "a.tar.gz", tarGz(t, file(prog, foreignProgram)), 0, "executable"},
		{"not an executable", UnpackOptions{}, "a.gz", gzipBytes(t, small), 0, "executable"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			limit := c.limit
			if limit == 0 {
				limit = testLimit
			}
			_, err := unpack(t, mustUnpacker(t, c.opts), c.asset, c.data, hostPlatform, limit)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("Unpack = %v, want %q", err, c.want)
			}
			if !errors.Is(err, selfupdate.ErrIntegrity) {
				t.Fatalf("%v does not wrap ErrIntegrity", err)
			}
		})
	}
}

func TestUnpackContextAndLimit(t *testing.T) {
	u := mustUnpacker(t, UnpackOptions{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := unpackCtx(ctx, t, u, "a.tar.gz", tarGz(t, file(prog, hostProgram)), hostPlatform, testLimit); !errors.Is(err, context.Canceled) {
		t.Fatalf("a cancelled context: %v", err)
	}
	if _, err := unpack(t, u, "a.tar.gz", tarGz(t, file(prog, hostProgram)), hostPlatform, 0); err == nil {
		t.Fatal("a zero limit was accepted")
	}
	if _, err := unpack(t, u, "a.tar.gz", tarGz(t, file("relay", hostProgram)), selfupdate.Platform{OS: "plan9", Arch: "amd64"}, testLimit); !errors.Is(err, selfupdate.ErrUnsupportedPlatform) {
		t.Fatalf("a platform with no image check: %v", err)
	}
}
