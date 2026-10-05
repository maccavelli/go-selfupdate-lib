package archive

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

// gnuSparseTarGz is a tar.gz whose one entry has the GNU sparse type.
func gnuSparseTarGz(t testing.TB) []byte {
	t.Helper()
	raw := append(tarBlock("relay", 'S', 0), make([]byte, 1024)...)
	return gzipBytes(t, raw)
}
