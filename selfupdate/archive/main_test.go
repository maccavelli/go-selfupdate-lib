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
