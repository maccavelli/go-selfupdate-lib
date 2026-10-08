package archive

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
)

// Fixtures for 0012-PLAN U3: real Go executables, cross-built once, as
// selfupdate's imageverify_test.go builds them. None is darwin/amd64
// (0012-MADR §10).

var (
	// hostProgram runs here; foreignProgram is linux, on another
	// architecture; windowsProgram is a windows/amd64 PE.
	hostProgram, foreignProgram, windowsProgram []byte
	hostPlatform                                = selfupdate.Platform{OS: runtime.GOOS, Arch: runtime.GOARCH}
	// prog is the program's name for hostPlatform: the product, plus
	// ".exe" on windows.
	prog = func() string {
		if runtime.GOOS == "windows" {
			return "relay.exe"
		}
		return "relay"
	}()
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "archive-fixtures-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	code := func() int {
		defer func() { _ = os.RemoveAll(dir) }()
		foreign := selfupdate.Platform{OS: "linux", Arch: "arm64"}
		if hostPlatform == foreign {
			foreign.Arch = "amd64"
		}
		for _, f := range []struct {
			p   selfupdate.Platform
			out *[]byte
		}{
			{hostPlatform, &hostProgram},
			{foreign, &foreignProgram},
			{selfupdate.Platform{OS: "windows", Arch: "amd64"}, &windowsProgram},
		} {
			b, err := buildFixture(dir, f.p)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 2
			}
			*f.out = b
		}
		return m.Run()
	}()
	os.Exit(code)
}

func buildFixture(dir string, p selfupdate.Platform) ([]byte, error) {
	src := filepath.Join(dir, "src")
	if err := os.MkdirAll(src, 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(src, "go.mod"), []byte("module fixture\n\ngo 1.27\n"), 0o600); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(src, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o600); err != nil {
		return nil, err
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		return nil, fmt.Errorf("the go command is required to build fixtures: %w", err)
	}
	out := filepath.Join(dir, p.OS+"-"+p.Arch)
	cmd := exec.Command(goBin, "build", "-o", out, ".") //nolint:gosec // the go command, building a fixture
	cmd.Dir = src
	cmd.Env = append(os.Environ(), "GOOS="+p.OS, "GOARCH="+p.Arch, "CGO_ENABLED=0", "GOWORK=off", "GOFLAGS=")
	if b, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("build %s/%s: %w\n%s", p.OS, p.Arch, err, b)
	}
	return os.ReadFile(out) //nolint:gosec // the fixture just built
}

// tarEntry is one entry for tarGz.
type tarEntry struct {
	hdr  tar.Header
	body []byte
}

func file(name string, body []byte) tarEntry {
	return tarEntry{tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o755, Size: int64(len(body))}, body}
}

func dir(name string) tarEntry {
	return tarEntry{tar.Header{Name: name, Typeflag: tar.TypeDir, Mode: 0o755}, nil}
}

func gzipBytes(t testing.TB, raw []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func tarGz(t testing.TB, es ...tarEntry) []byte {
	t.Helper()
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	for _, e := range es {
		h := e.hdr
		if err := tw.WriteHeader(&h); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(e.body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return gzipBytes(t, raw.Bytes())
}

// zipEntry is one entry for zipBytes.
type zipEntry struct {
	hdr  zip.FileHeader
	body []byte
	raw  bool // write body as already compressed, with hdr's sizes
}

func zfile(name string, body []byte) zipEntry {
	h := zip.FileHeader{Name: name, Method: zip.Deflate}
	h.SetMode(0o755)
	return zipEntry{hdr: h, body: body}
}

// storedRaw is a stored entry written with CreateRaw: its sizes in the
// local header, and no data descriptor.
func storedRaw(name string, body []byte) zipEntry {
	h := zip.FileHeader{Name: name, Method: zip.Store, CRC32: crc32.ChecksumIEEE(body),
		CompressedSize64: uint64(len(body)), UncompressedSize64: uint64(len(body))}
	h.SetMode(0o755)
	return zipEntry{hdr: h, body: body, raw: true}
}

// zdir is a directory entry.
func zdir(name string) zipEntry {
	h := zip.FileHeader{Name: name}
	h.SetMode(os.ModeDir | 0o755)
	return zipEntry{hdr: h}
}

func zipBytes(t testing.TB, es ...zipEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range es {
		h := e.hdr
		var w io.Writer
		var err error
		if e.raw {
			w, err = zw.CreateRaw(&h)
		} else {
			w, err = zw.CreateHeader(&h)
		}
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(e.body); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// tarBlock is one hand-built ustar header block, for entries archive/tar's
// writer will not write.
func tarBlock(name string, typeflag byte, size int64) []byte {
	b := make([]byte, 512)
	copy(b[0:100], name)
	copy(b[100:108], "0000755\x00")
	copy(b[108:116], "0000000\x00")
	copy(b[116:124], "0000000\x00")
	copy(b[124:136], fmt.Sprintf("%011o\x00", size))
	copy(b[136:148], "00000000000\x00")
	b[156] = typeflag
	copy(b[257:263], "ustar\x00")
	copy(b[263:265], "00")
	for i := 148; i < 156; i++ {
		b[i] = ' '
	}
	var sum int64
	for _, c := range b {
		sum += int64(c)
	}
	copy(b[148:156], fmt.Sprintf("%06o\x00 ", sum))
	return b
}

func padBlock(b []byte) []byte {
	if r := len(b) % 512; r != 0 {
		b = append(b, make([]byte, 512-r)...)
	}
	return b
}

func paxRecord(k, v string) string {
	l := len(k) + len(v) + 3
	for {
		s := fmt.Sprintf("%d %s=%s\n", l, k, v)
		if len(s) == l {
			return s
		}
		l = len(s)
	}
}

// paxSparseTarGz is a tar.gz whose one entry, named relay, is a GNU 1.0
// sparse file in PAX records, which archive/tar's writer will not write.
func paxSparseTarGz(t testing.TB) []byte {
	t.Helper()
	pax := paxRecord("GNU.sparse.major", "1") + paxRecord("GNU.sparse.minor", "0") +
		paxRecord("GNU.sparse.name", "relay") + paxRecord("GNU.sparse.realsize", "4")
	data := append(padBlock([]byte("1\n0\n4\n")), padBlock([]byte("ELF!"))...)
	var raw bytes.Buffer
	raw.Write(tarBlock("PaxHeaders/relay", 'x', int64(len(pax))))
	raw.Write(padBlock([]byte(pax)))
	raw.Write(tarBlock("GNUSparseFile.0/relay", '0', int64(len(data))))
	raw.Write(data)
	raw.Write(make([]byte, 1024))
	return gzipBytes(t, raw.Bytes())
}

// slashRegTarGz is a tar.gz whose one entry is a regular file (type 0)
// named name, a name that ends in "/" (0015-MADR E3).
func slashRegTarGz(t testing.TB, name string, body []byte) []byte {
	t.Helper()
	raw := append(tarBlock(name, '0', int64(len(body))), padBlock(append([]byte(nil), body...))...)
	return gzipBytes(t, append(raw, make([]byte, 1024)...))
}

// localNameZip is a zip of one entry, named central in the central
// directory and local, of the same length, in its local header
// (0015-MADR E1).
func localNameZip(t testing.TB, central, local string, body []byte) []byte {
	t.Helper()
	if len(central) != len(local) {
		t.Fatalf("%q and %q differ in length", central, local)
	}
	b := zipBytes(t, zfile(central, body))
	if string(b[:4]) != "PK\x03\x04" || string(b[30:30+len(central)]) != central {
		t.Fatal("the local header was not found")
	}
	copy(b[30:], local)
	return b
}

// unicodePathExtra is an Info-ZIP Unicode Path extra field (0x7075) that
// names the entry u, for a header named name (0015-MADR E1).
func unicodePathExtra(name, u string) []byte {
	b := binary.LittleEndian.AppendUint16(nil, 0x7075)
	b = binary.LittleEndian.AppendUint16(b, uint16(5+len(u))) //nolint:gosec // a test's short name
	b = append(b, 1)
	b = binary.LittleEndian.AppendUint32(b, crc32.ChecksumIEEE([]byte(name)))
	return append(b, u...)
}

// renamedZip is zipBytes with from, which must occur exactly twice (the
// local header and the central record), renamed to to, of the same
// length: a name the zip writer would not write with that data.
func renamedZip(t testing.TB, from, to string, es ...zipEntry) []byte {
	t.Helper()
	b := zipBytes(t, es...)
	if len(from) != len(to) || bytes.Count(b, []byte(from)) != 2 {
		t.Fatalf("%q is not in the zip exactly twice, or %q differs in length", from, to)
	}
	return bytes.ReplaceAll(b, []byte(from), []byte(to))
}

// paxGlobalTarGz is a tar.gz that opens with a PAX global header holding
// records, as git archive's does, then holds the program as name
// (0015-MADR E8).
func paxGlobalTarGz(t testing.TB, records map[string]string, name string, body []byte) []byte {
	t.Helper()
	var pax string
	for _, k := range slices.Sorted(maps.Keys(records)) {
		pax += paxRecord(k, records[k])
	}
	var raw bytes.Buffer
	raw.Write(tarBlock("pax_global_header", 'g', int64(len(pax))))
	raw.Write(padBlock([]byte(pax)))
	raw.Write(tarBlock(name, '0', int64(len(body))))
	raw.Write(padBlock(append([]byte(nil), body...)))
	raw.Write(make([]byte, 1024))
	return gzipBytes(t, raw.Bytes())
}

// gnuSparseTarGz is a tar.gz whose one entry has the GNU sparse type.
func gnuSparseTarGz(t testing.TB) []byte {
	t.Helper()
	raw := append(tarBlock("relay", 'S', 0), make([]byte, 1024)...)
	return gzipBytes(t, raw)
}

// Zips for 0017-PLAN P1 (0017-MADR 2B): bytes a streaming reader sees that
// the central directory does not name, and the ways a zip's records can
// disagree about where an entry ends.

// eocdOffset is the offset of b's last end-of-central-directory record.
func eocdOffset(tb testing.TB, b []byte) int {
	tb.Helper()
	i := bytes.LastIndex(b, []byte("PK\x05\x06"))
	if i < 0 {
		tb.Fatal("no end of central directory")
	}
	return i
}

// hiddenGapZip is good's zip with evil's local entry, under the same name,
// inserted before good's central directory, whose offset moves past it: no
// central record names evil, and a streaming reader extracts it.
func hiddenGapZip(tb testing.TB, good, evil []byte) []byte {
	tb.Helper()
	le := binary.LittleEndian
	g := zipBytes(tb, zfile(prog, good))
	e := zipBytes(tb, zfile(prog, evil))
	gCD := int(le.Uint32(g[eocdOffset(tb, g)+16:]))
	eCD := int(le.Uint32(e[eocdOffset(tb, e)+16:]))
	out := append(append(append([]byte(nil), g[:gCD]...), e[:eCD]...), g[gCD:]...)
	le.PutUint32(out[eocdOffset(tb, out)+16:], uint32(gCD+eCD))
	return out
}

// hiddenBetweenZip is a zip of README and good, with evil's local entry,
// under the program's name, inserted between them, and the offsets after it
// moved: the hole lies between two entries a central record names.
func hiddenBetweenZip(tb testing.TB, good, evil []byte) []byte {
	tb.Helper()
	le := binary.LittleEndian
	g := zipBytes(tb, zfile("README", []byte("# relay\n")), zfile(prog, good))
	e := zipBytes(tb, zfile(prog, evil))
	eCD := int(le.Uint32(e[eocdOffset(tb, e)+16:]))
	end := eocdOffset(tb, g)
	cd := int(le.Uint32(g[end+16:]))
	second := cd + 1 + bytes.Index(g[cd+1:], []byte("PK\x01\x02"))
	at := int(le.Uint32(g[second+42:]))
	out := append(append(append([]byte(nil), g[:at]...), e[:eCD]...), g[at:]...)
	le.PutUint32(out[second+eCD+42:], uint32(at+eCD))
	le.PutUint32(out[end+eCD+16:], uint32(cd+eCD))
	return out
}

// hiddenPrefixZip is good's zip with evil's local entry prepended and no
// offset changed: archive/zip reads past it as its base offset.
func hiddenPrefixZip(tb testing.TB, good, evil []byte) []byte {
	tb.Helper()
	e := zipBytes(tb, zfile(prog, evil))
	eCD := int(binary.LittleEndian.Uint32(e[eocdOffset(tb, e)+16:]))
	return append(append([]byte(nil), e[:eCD]...), zipBytes(tb, zfile(prog, good))...)
}

// trailingZip is a zip with bytes after its end record's comment.
func trailingZip(tb testing.TB, body []byte) []byte {
	tb.Helper()
	return append(zipBytes(tb, zfile(prog, body)), "trailing junk"...)
}

// badDescriptorZip is a zip whose first entry's data descriptor holds a CRC
// that is not its central record's.
func badDescriptorZip(tb testing.TB, body []byte) []byte {
	tb.Helper()
	b := zipBytes(tb, zfile("README", []byte("# relay\n")), zfile(prog, body))
	i := bytes.Index(b, []byte("PK\x07\x08"))
	if i < 0 {
		tb.Fatal("no data descriptor")
	}
	b[i+4] ^= 0xff
	return b
}

// clearedFlagZip is a zip whose local header no longer says a data
// descriptor follows, though its central record does.
func clearedFlagZip(tb testing.TB, body []byte) []byte {
	tb.Helper()
	b := zipBytes(tb, zfile(prog, body))
	b[6] &^= 0x8
	return b
}

// handEntry is one stored entry for handZip. dd is its data descriptor's
// length: 0 (none), 12, 16 (signed), 20 (64-bit) or 24 (signed, 64-bit).
type handEntry struct {
	name string
	body []byte
	dd   int
}

// handZip writes a zip of stored entries byte by byte, so that each data
// descriptor form, which archive/zip's writer does not choose, can be made.
func handZip(es ...handEntry) []byte {
	le := binary.LittleEndian
	var out, cd []byte
	for _, e := range es {
		off := uint32(len(out))
		crc := crc32.ChecksumIEEE(e.body)
		n := uint32(len(e.body))
		var flags uint16
		if e.dd != 0 {
			flags = 0x8
		}
		out = le.AppendUint32(out, 0x04034b50)
		out = le.AppendUint16(out, 20)
		out = le.AppendUint16(out, flags)
		out = le.AppendUint16(out, 0) // stored
		out = le.AppendUint32(out, 0) // time and date
		if e.dd != 0 {
			out = append(out, make([]byte, 12)...)
		} else {
			out = le.AppendUint32(out, crc)
			out = le.AppendUint32(out, n)
			out = le.AppendUint32(out, n)
		}
		out = le.AppendUint16(out, uint16(len(e.name)))
		out = le.AppendUint16(out, 0)
		out = append(out, e.name...)
		out = append(out, e.body...)
		if e.dd == 16 || e.dd == 24 {
			out = le.AppendUint32(out, 0x08074b50)
		}
		switch e.dd {
		case 12, 16:
			out = le.AppendUint32(out, crc)
			out = le.AppendUint32(out, n)
			out = le.AppendUint32(out, n)
		case 20, 24:
			out = le.AppendUint32(out, crc)
			out = le.AppendUint64(out, uint64(n))
			out = le.AppendUint64(out, uint64(n))
		}
		cd = le.AppendUint32(cd, 0x02014b50)
		cd = le.AppendUint16(cd, 3<<8|20)
		cd = le.AppendUint16(cd, 20)
		cd = le.AppendUint16(cd, flags)
		cd = le.AppendUint16(cd, 0)
		cd = le.AppendUint32(cd, 0)
		cd = le.AppendUint32(cd, crc)
		cd = le.AppendUint32(cd, n)
		cd = le.AppendUint32(cd, n)
		cd = le.AppendUint16(cd, uint16(len(e.name)))
		cd = append(cd, make([]byte, 8)...) // extra, comment, disk, internal attributes
		cd = le.AppendUint32(cd, 0o100755<<16)
		cd = le.AppendUint32(cd, off)
		cd = append(cd, e.name...)
	}
	cdOff := uint32(len(out))
	out = append(out, cd...)
	out = le.AppendUint32(out, 0x06054b50)
	out = append(out, 0, 0, 0, 0)
	out = le.AppendUint16(out, uint16(len(es)))
	out = le.AppendUint16(out, uint16(len(es)))
	out = le.AppendUint32(out, uint32(len(cd)))
	out = le.AppendUint32(out, cdOff)
	return le.AppendUint16(out, 0)
}

// twoWayDescriptorZip is a zip whose first, empty, entry's descriptor reads
// as 24 bytes and as 16 followed by 8 zero bytes: a reader cannot tell where
// the entry ends (You et al., USENIX Security 2025).
func twoWayDescriptorZip(body []byte) []byte {
	return handZip(handEntry{"empty", nil, 24}, handEntry{prog, body, 0})
}

// zip64DisagreeZip is a zip with a zip64 end record and locator before its
// end record, the zip64 record counting one more entry than the end record:
// archive/zip ignores it, and Python's zipfile and Info-ZIP read it.
func zip64DisagreeZip(tb testing.TB, body []byte) []byte {
	tb.Helper()
	le := binary.LittleEndian
	b := zipBytes(tb, zfile(prog, body))
	e := eocdOffset(tb, b)
	records, cdSize, cdOff := le.Uint16(b[e+10:]), le.Uint32(b[e+12:]), le.Uint32(b[e+16:])
	var rec []byte
	rec = le.AppendUint32(rec, 0x06064b50)
	rec = le.AppendUint64(rec, 44)
	rec = le.AppendUint16(rec, 45)
	rec = le.AppendUint16(rec, 45)
	rec = le.AppendUint32(rec, 0)
	rec = le.AppendUint32(rec, 0)
	rec = le.AppendUint64(rec, uint64(records)+1)
	rec = le.AppendUint64(rec, uint64(records)+1)
	rec = le.AppendUint64(rec, uint64(cdSize))
	rec = le.AppendUint64(rec, uint64(cdOff))
	rec = le.AppendUint32(rec, 0x07064b50)
	rec = le.AppendUint32(rec, 0)
	rec = le.AppendUint64(rec, uint64(e))
	rec = le.AppendUint32(rec, 1)
	return append(append(append([]byte(nil), b[:e]...), rec...), b[e:]...)
}

// slackZip is a zip with 4 bytes inside its central directory, after the
// records, which the end record's size counts: archive/zip reads records
// by count and never looks at them.
func slackZip(tb testing.TB, body []byte) []byte {
	tb.Helper()
	le := binary.LittleEndian
	b := zipBytes(tb, zfile(prog, body))
	e := eocdOffset(tb, b)
	out := append(append(append([]byte(nil), b[:e]...), "SLAK"...), b[e:]...)
	le.PutUint32(out[e+4+12:], le.Uint32(out[e+4+12:])+4)
	return out
}

// commentZip is a zip with an archive comment.
func commentZip(tb testing.TB, comment string, es ...zipEntry) []byte {
	tb.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range es {
		h := e.hdr
		w, err := zw.CreateHeader(&h)
		if err != nil {
			tb.Fatal(err)
		}
		if _, err := w.Write(e.body); err != nil {
			tb.Fatal(err)
		}
	}
	if err := zw.SetComment(comment); err != nil {
		tb.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		tb.Fatal(err)
	}
	return buf.Bytes()
}
