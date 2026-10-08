package archive

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
)

// Fuzz targets for 0012-PLAN U3 step 10. They call extract, which has no
// image check, on arbitrary bytes. Whatever the input: no panic, never more
// than the limit plus the one byte that shows an overrun written, nothing
// over the limit accepted, and every error a refusal.

const fuzzLimit = 1 << 20

func fuzzExtract(t *testing.T, f Format, data []byte) {
	u := &unpacker{maxEntries: 64}
	var out bytes.Buffer
	isProgram := programMatcher("relay", selfupdate.Platform{OS: "linux", Arch: "amd64"}, "")
	err := u.extract(context.Background(), f, bytes.NewReader(data), int64(len(data)), &out, isProgram, fuzzLimit)
	if out.Len() > fuzzLimit+1 {
		t.Fatalf("wrote %d bytes past a %d-byte limit", out.Len(), fuzzLimit)
	}
	if err == nil && out.Len() > fuzzLimit {
		t.Fatalf("accepted %d bytes over a %d-byte limit", out.Len(), fuzzLimit)
	}
	if err != nil && !errors.Is(err, selfupdate.ErrIntegrity) {
		t.Fatalf("an error that is not a refusal: %v", err)
	}
}

func FuzzUnpackTarGz(f *testing.F) {
	small := []byte("small")
	f.Add(tarGz(f, file("relay", small)))
	f.Add(tarGz(f, dir("relay_1/"), file("relay_1/relay", small), file("README", small)))
	f.Add(tarGz(f, file("../relay", small)))
	f.Add(tarGz(f, file("relay", small), file("RELAY", small)))
	f.Add(tarGz(f, tarEntry{tar.Header{Name: "relay", Typeflag: tar.TypeSymlink, Linkname: "/bin/sh"}, nil}))
	f.Add(paxSparseTarGz(f))
	f.Add(gnuSparseTarGz(f))
	f.Fuzz(func(t *testing.T, data []byte) { fuzzExtract(t, TarGz, data) })
}

func FuzzUnpackZip(f *testing.F) {
	small := []byte("small")
	f.Add(zipBytes(f, zfile("relay", small)))
	f.Add(zipBytes(f, zfile("README", small), zfile("x/relay", small)))
	f.Add(zipBytes(f, zfile("relay", small), zfile("relay", small)))
	f.Add(zipBytes(f, zfile("re\x00lay", small)))
	// A local entry no central record names (0017-MADR 2B).
	f.Add(hiddenGapZip(f, small, []byte("evil")))
	f.Add(hiddenPrefixZip(f, small, []byte("evil")))
	f.Fuzz(func(t *testing.T, data []byte) { fuzzExtract(t, Zip, data) })
}

func FuzzUnpackGz(f *testing.F) {
	f.Add(gzipBytes(f, []byte("small")))
	f.Add(append(gzipBytes(f, []byte("small")), "junk"...))
	f.Add(gzipBytes(f, make([]byte, 2*fuzzLimit)))
	f.Fuzz(func(t *testing.T, data []byte) { fuzzExtract(t, Gz, data) })
}
