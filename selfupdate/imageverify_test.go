package selfupdate

import (
	"bytes"
	"context"
	"debug/macho"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Tests for docs/decisions/0004-PLAN-v1-1-0-core-api.md Step 7 (image
// verifier). The fixtures are real Go executables, cross-built here.

func buildFixture(t *testing.T, dir, goos, goarch string) string {
	t.Helper()
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("the go command is required to build fixtures: %v", err)
	}
	out := filepath.Join(dir, goos+"-"+goarch)
	cmd := exec.Command(goBin, "build", "-o", out, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOOS="+goos, "GOARCH="+goarch, "CGO_ENABLED=0", "GOWORK=off", "GOFLAGS=")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build %s/%s: %v\n%s", goos, goarch, err, b)
	}
	return out
}

// writeFat assembles a fat Mach-O from thin ones, in the layout
// debug/macho/fat.go reads: a big-endian header, one arch record each, and
// each slice aligned to 2^14.
func writeFat(t *testing.T, out string, thin ...string) {
	t.Helper()
	const alignBits = 14
	var slices [][]byte
	var hdrs []macho.FatArchHeader
	offset := uint32(1 << alignBits)
	for _, p := range thin {
		f, err := macho.Open(p)
		if err != nil {
			t.Fatal(err)
		}
		cpu, sub := f.Cpu, f.SubCpu
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		hdrs = append(hdrs, macho.FatArchHeader{Cpu: cpu, SubCpu: sub, Offset: offset, Size: uint32(len(b)), Align: alignBits})
		slices = append(slices, b)
		offset += uint32(len(b))
		offset = (offset + (1<<alignBits - 1)) &^ (1<<alignBits - 1)
	}
	var buf bytes.Buffer
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(binary.Write(&buf, binary.BigEndian, macho.MagicFat))
	must(binary.Write(&buf, binary.BigEndian, uint32(len(hdrs))))
	for _, h := range hdrs {
		must(binary.Write(&buf, binary.BigEndian, h))
	}
	for i, h := range hdrs {
		buf.Write(make([]byte, int(h.Offset)-buf.Len()))
		buf.Write(slices[i])
	}
	must(os.WriteFile(out, buf.Bytes(), 0o600))
}

func verifyImage(t *testing.T, p Platform, path string) error {
	t.Helper()
	v, err := NewImageVerifier(p)
	if err != nil {
		t.Fatal(err)
	}
	return v.Verify(context.Background(), Verification{Open: func() (io.ReadCloser, error) { return os.Open(path) }})
}

func TestImageVerifier(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module fixture\n\ngo 1.27\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bin := map[string]string{}
	for _, p := range []string{"linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64", "windows/amd64"} {
		osArch := strings.SplitN(p, "/", 2)
		bin[p] = buildFixture(t, dir, osArch[0], osArch[1])
	}
	fat := filepath.Join(dir, "darwin-fat")
	writeFat(t, fat, bin["darwin/amd64"], bin["darwin/arm64"])
	// A DLL also sets IMAGE_FILE_EXECUTABLE_IMAGE; IMAGE_FILE_DLL (0x2000)
	// in the COFF header's Characteristics, 18 bytes past the PE signature
	// at e_lfanew, makes the Windows fixture one (0015-MADR B9).
	dll := filepath.Join(dir, "windows-dll")
	img, err := os.ReadFile(bin["windows/amd64"])
	if err != nil {
		t.Fatal(err)
	}
	at := int(binary.LittleEndian.Uint32(img[0x3c:])) + 4 + 18
	binary.LittleEndian.PutUint16(img[at:], binary.LittleEndian.Uint16(img[at:])|0x2000)
	if err := os.WriteFile(dll, img, 0o600); err != nil {
		t.Fatal(err)
	}
	notExec := filepath.Join(dir, "not-an-executable")
	if err := os.WriteFile(notExec, []byte("hello, world"), 0o600); err != nil {
		t.Fatal(err)
	}
	plat := func(s string) Platform {
		p := strings.SplitN(s, "/", 2)
		return Platform{OS: p[0], Arch: p[1]}
	}
	rows := []struct {
		file, platform string
		ok             bool
	}{
		{bin["linux/amd64"], "linux/amd64", true},
		{bin["linux/amd64"], "linux/arm64", false},
		{bin["linux/amd64"], "darwin/amd64", false},
		{bin["linux/amd64"], "windows/amd64", false},
		{bin["linux/arm64"], "linux/arm64", true},
		{bin["darwin/arm64"], "darwin/arm64", true},
		{bin["darwin/arm64"], "darwin/amd64", false},
		{bin["windows/amd64"], "windows/amd64", true},
		{bin["windows/amd64"], "windows/arm64", false},
		{dll, "windows/amd64", false},
		{fat, "darwin/amd64", true},
		{fat, "darwin/arm64", true},
		{fat, "darwin/386", false},
		{notExec, "linux/amd64", false},
		{notExec, "darwin/arm64", false},
		{notExec, "windows/amd64", false},
	}
	for _, r := range rows {
		err := verifyImage(t, plat(r.platform), r.file)
		if r.ok && err != nil {
			t.Errorf("%s as %s: %v", filepath.Base(r.file), r.platform, err)
		}
		if !r.ok && !errors.Is(err, ErrIntegrity) {
			t.Errorf("%s as %s: %v, want ErrIntegrity", filepath.Base(r.file), r.platform, err)
		}
		// CheckImage gives the same answer (0012-PLAN U1). darwin/amd64
		// is not a target of that work (0012-MADR §10), so its rows stay
		// with the verifier alone.
		if r.platform == "darwin/amd64" || r.file == bin["darwin/amd64"] || r.file == fat {
			continue
		}
		err = checkImageFile(t, plat(r.platform), r.file)
		if r.ok && err != nil {
			t.Errorf("CheckImage %s as %s: %v", filepath.Base(r.file), r.platform, err)
		}
		if !r.ok && !errors.Is(err, ErrIntegrity) {
			t.Errorf("CheckImage %s as %s: %v, want ErrIntegrity", filepath.Base(r.file), r.platform, err)
		}
	}
}

func checkImageFile(t *testing.T, p Platform, path string) (err error) {
	t.Helper()
	f, err := os.Open(path) //nolint:gosec // a fixture this test built
	if err != nil {
		t.Fatal(err)
	}
	defer func() { err = joinClose(err, f) }()
	return CheckImage(f, p)
}

// TestCheckImageUnsupportedPlatform: as NewImageVerifier, a platform with
// no image check is ErrUnsupportedPlatform, and a zero platform is the
// running one.
func TestCheckImageUnsupportedPlatform(t *testing.T) {
	for _, p := range []Platform{{OS: "plan9", Arch: "amd64"}, {OS: "linux", Arch: "riscv64"}} {
		if err := CheckImage(strings.NewReader("x"), p); !errors.Is(err, ErrUnsupportedPlatform) {
			t.Errorf("%v: %v", p, err)
		}
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := checkImageFile(t, Platform{}, exe); err != nil {
		t.Fatalf("the test binary as the running platform: %v", err)
	}
}

func TestImageVerifierUnsupportedPlatform(t *testing.T) {
	for _, p := range []Platform{{OS: "plan9", Arch: "amd64"}, {OS: "linux", Arch: "riscv64"}} {
		if _, err := NewImageVerifier(p); !errors.Is(err, ErrUnsupportedPlatform) {
			t.Errorf("%v: %v", p, err)
		}
	}
	if _, err := NewImageVerifier(Platform{}); err != nil {
		t.Errorf("runtime platform: %v", err)
	}
	v, err := NewImageVerifier(Platform{OS: "linux", Arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	err = v.Verify(context.Background(), Verification{Open: func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("x")), nil
	}})
	if err == nil || !strings.Contains(err.Error(), "seekable") {
		t.Fatalf("a non-seekable body: %v", err)
	}
}
