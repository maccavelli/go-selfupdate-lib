package selfupdate

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestManifestParityFixtures holds the client's SHA256SUMS parser to the same
// accept/reject outcome as the publish gate: scripts/verify-selfupdate-release.sh
// runs the same fixtures (0003-MADR D1). A manifest the gate accepts but the
// client refuses would become an immutable release nobody can install.
func TestManifestParityFixtures(t *testing.T) {
	t.Parallel()
	dir := filepath.Join("testdata", "manifest-parity")
	cases, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) < 20 {
		t.Fatalf("only %d parity fixtures; the set is incomplete", len(cases))
	}
	for _, c := range cases {
		if !c.IsDir() {
			continue
		}
		t.Run(c.Name(), func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(dir, c.Name(), "SHA256SUMS"))
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(filepath.Join(dir, c.Name(), "expect"))
			if err != nil {
				t.Fatal(err)
			}
			entries, perr := ParseSHA256SUMS(data)
			if perr == nil {
				_, e1 := checksumFor(entries, "demo-linux-amd64")
				_, e2 := checksumFor(entries, "demo-windows-amd64.exe")
				perr = errors.Join(e1, e2)
			}
			accepted := perr == nil
			switch strings.TrimSpace(string(want)) {
			case "accept":
				if !accepted {
					t.Fatalf("client rejects a manifest the gate accepts: %v", perr)
				}
			case "reject":
				if accepted {
					t.Fatal("client accepts a manifest the gate rejects")
				}
			default:
				t.Fatalf("bad expect file %q", want)
			}
		})
	}
}
