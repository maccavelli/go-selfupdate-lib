package archive

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
)

// Tests for docs/decisions/0017-PLAN-verify-build-provenance-and-close-0015-open-items.md
// P1: the parts of the tiling check a whole archive cannot reach.

// infoZip is an archive Info-ZIP's zip 3.0 wrote with -X: hello.txt, a
// docs/ directory, docs/readme and relay, deflated and stored, with data
// descriptors where zip chose them.
var infoZip = mustHex(
	"504b03040a000000000000909f5b20303a360600000006000000090000006865" +
		"6c6c6f2e74787468656c6c6f0a504b03040a000000000000909f5b0000000000" +
		"0000000000000005000000646f63732f504b03040a000000000000909f5bbca2" +
		"8fc90c0000000c0000000b000000646f63732f726561646d65726561646d6520" +
		"746578740a504b03040a000000000000909f5bddac90d8080000000800000005" +
		"00000072656c617970726f6772616d0a504b01021e030a000000000000909f5b" +
		"20303a360600000006000000090000000000000001000000a481000000006865" +
		"6c6c6f2e747874504b01021e030a000000000000909f5b000000000000000000" +
		"000000050000000000000000001000ed412d000000646f63732f504b01021e03" +
		"0a000000000000909f5bbca28fc90c0000000c0000000b000000000000000100" +
		"0000a48150000000646f63732f726561646d65504b01021e030a000000000000" +
		"909f5bddac90d80800000008000000050000000000000001000000a481850000" +
		"0072656c6179504b05060000000004000400d6000000b00000000000",
)

func mustHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

// TestOtherWritersTile: a zip another tool wrote passes the zip checks.
// extract runs them without the image check, which a text file would fail.
func TestOtherWritersTile(t *testing.T) {
	u := &unpacker{maxEntries: 64}
	var out bytes.Buffer
	isProgram := programMatcher("relay", selfupdate.Platform{OS: "linux", Arch: "amd64"}, "")
	if err := u.extract(context.Background(), Zip, bytes.NewReader(infoZip), int64(len(infoZip)), &out, isProgram, 1<<20); err != nil {
		t.Fatal(err)
	}
	if out.String() != "program\n" {
		t.Fatalf("extracted %q", out.String())
	}
}

// TestDirectoryBoundsZip64: 70,000 entries make archive/zip's writer write
// zip64 end records, which directoryBounds reads. It is called directly,
// since 70,000 entries are more than an unpacker takes.
func TestDirectoryBoundsZip64(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for i := range 70000 {
		if _, err := zw.CreateRaw(&zip.FileHeader{Name: fmt.Sprintf("f%d", i), Method: zip.Store}); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()
	if !bytes.Contains(b[len(b)-200:], []byte("PK\x06\x07")) {
		t.Fatal("the writer wrote no zip64 locator")
	}
	cdOff, cdSize, err := directoryBounds(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	if len(zr.File) != 70000 || cdOff <= 0 || cdSize <= 0 {
		t.Fatalf("%d entries, directory [%d, +%d)", len(zr.File), cdOff, cdSize)
	}
}

// TestDescriptorLenWidths: each of the four descriptor forms is read, and
// exactly one reading matches.
func TestDescriptorLenWidths(t *testing.T) {
	for _, dd := range []int{12, 16, 20, 24} {
		t.Run(fmt.Sprint(dd), func(t *testing.T) {
			b := handZip(handEntry{"a", []byte("aa"), dd}, handEntry{"relay", []byte("p"), 0})
			zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
			if err != nil {
				t.Fatal(err)
			}
			zf := zr.File[0]
			off, err := zf.DataOffset()
			if err != nil {
				t.Fatal(err)
			}
			n, err := descriptorLen(bytes.NewReader(b), zf, off+2, int64(len(b)))
			if err != nil {
				t.Fatal(err)
			}
			if n != int64(dd) {
				t.Fatalf("descriptorLen = %d, want %d", n, dd)
			}
		})
	}
}
