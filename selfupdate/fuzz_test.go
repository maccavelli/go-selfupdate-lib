package selfupdate

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

// Fuzz targets from the round-2 review
// (docs/decisions/0004-MADR-evolve-selfupdate-api-and-tui-support.md,
// "Harness gaps"). go test runs each seed corpus on every platform; run one
// target longer with, for example,
//
//	go test -run '^$' -fuzz '^FuzzParseSHA256SUMS$' -fuzztime 30s ./selfupdate

// FuzzParseSHA256SUMS: no panic; every accepted entry is a lowercase 64-hex
// digest and a validated basename; a canonical re-serialisation parses back
// to the same entries.
func FuzzParseSHA256SUMS(f *testing.F) {
	d := strings.Repeat("ab", 32)
	for _, s := range []string{
		d + "  demo-linux-amd64\n",
		d + " *demo.exe\r\n# c\n\n",
		d + "\tx\n" + d + "  y",
		"\xef\xbb\xbf" + d + "  x\n",
		d + "\xc2\x85x\n",
		d + "  x\r\r\n",
		strings.Repeat("AB", 32) + "  upper\n",
		strings.Repeat("#", 5000) + "\n" + d + " x\n",
	} {
		f.Add([]byte(s))
	}
	// Every parity fixture the release gate and the client must agree on.
	dirs, err := os.ReadDir(filepath.Join("testdata", "manifest-parity"))
	if err != nil {
		f.Fatal(err)
	}
	for _, e := range dirs {
		data, err := os.ReadFile(filepath.Join("testdata", "manifest-parity", e.Name(), "SHA256SUMS"))
		if err != nil {
			f.Fatal(err)
		}
		f.Add(data)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		entries, err := ParseSHA256SUMS(data)
		if err != nil {
			return
		}
		if len(entries) == 0 {
			t.Fatal("accepted with no entries")
		}
		names := make([]string, 0, len(entries))
		for name, dig := range entries {
			if len(dig) != 64 || strings.ToLower(dig) != dig {
				t.Fatalf("digest %q is not lowercase 64-hex", dig)
			}
			if _, err := hex.DecodeString(dig); err != nil {
				t.Fatalf("digest %q: %v", dig, err)
			}
			if err := validateChecksumName(name); err != nil {
				t.Fatalf("accepted name %q: %v", name, err)
			}
			if strings.IndexFunc(name, unicode.IsSpace) >= 0 || strings.HasPrefix(name, "*") {
				t.Fatalf("name %q keeps a space or the binary marker", name)
			}
			names = append(names, name)
		}
		sort.Strings(names)
		var b bytes.Buffer
		for _, n := range names {
			fmt.Fprintf(&b, "%s  %s\n", entries[n], n)
		}
		again, err := ParseSHA256SUMS(b.Bytes())
		if err != nil {
			t.Fatalf("round trip rejected: %v\n%q", err, b.String())
		}
		if len(again) != len(entries) {
			t.Fatal("round trip lost entries")
		}
		for n, dig := range entries {
			if again[n] != dig {
				t.Fatalf("round trip changed %q: %q -> %q", n, dig, again[n])
			}
		}
	})
}

// FuzzGitHubReleaseJSON: decoding and mapping never panic, and every
// accepted asset name is a plain file name (0004-MADR R9).
func FuzzGitHubReleaseJSON(f *testing.F) {
	f.Add([]byte(`{"id":1,"tag_name":"v1.0.0","immutable":true,"assets":[{"id":2,"name":"a","state":"uploaded","size":1}]}`))
	f.Add([]byte(`{"id":1}}`))
	f.Add([]byte(`{"assets":[{"id":1,"name":"..\\x"}]}`))
	f.Add([]byte(`{"id":1,"tag_name":"v1.0.0","assets":[{"id":1,"nAme":"."}]}`))
	f.Add([]byte(`{"id":1,"tag_name":"v1.0.0","assets":[{"id":1,"name":".."}]}`))
	f.Add([]byte("{\"assets\":[{\"id\":1,\"name\":\"\xe2\x80\xaex\"}]}"))
	f.Fuzz(func(t *testing.T, data []byte) {
		var raw githubReleaseJSON
		if err := decodeJSON(data, &raw); err != nil {
			return
		}
		rel, err := mapRelease(raw)
		if err != nil {
			return
		}
		for _, a := range rel.Assets {
			if a.ID <= 0 || a.Name == "" || strings.ContainsAny(a.Name, `/\`) {
				t.Fatalf("unsafe asset accepted: %+v", a)
			}
			if a.Name == "." || a.Name == ".." {
				t.Fatalf("dot asset name accepted: %q", a.Name)
			}
			if filepath.Base(a.Name) != a.Name {
				t.Fatalf("non-basename accepted: %q", a.Name)
			}
		}
	})
}

// FuzzSanitize: output is valid UTF-8 with no C0, C1 or DEL control, no
// format character (Cf), and no line or paragraph separator; sanitizeText
// also keeps no whitespace other than a plain space.
func FuzzSanitize(f *testing.F) {
	f.Add("a\x1b[31mb\xe2\x80\xaec d\xff")
	f.Add("\xc2\x85\xc2\xad\xe2\x80\x8b\xef\xbb\xbf")
	f.Add("line\xe2\x80\xa8para\xe2\x80\xa9end\ttab\r\n")
	f.Fuzz(func(t *testing.T, s string) {
		for name, out := range map[string]string{"text": sanitizeText(s), "diag": sanitizeDiagnostic(s, 4096)} {
			if !utf8.ValidString(out) {
				t.Fatalf("%s: invalid UTF-8 %q", name, out)
			}
			for _, r := range out {
				if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) || unicode.Is(unicode.Cf, r) || r == 0x2028 || r == 0x2029 {
					t.Fatalf("%s: control %U survived in %q", name, r, out)
				}
				if name == "text" && unicode.IsSpace(r) && r != ' ' {
					t.Fatalf("%s: whitespace %U survived in %q", name, r, out)
				}
			}
		}
	})
}

// FuzzParsePlan: no panic, and a plan ParsePlan accepts writes back, as its
// Document, to a document it reads as the same plan
// (docs/decisions/0038-MADR-pinned-plans-lifecycle-conformance-and-bubble-tests.md
// D1-A).
func FuzzParsePlan(f *testing.F) {
	valid, err := json.Marshal(validPlanJSON())
	if err != nil {
		f.Fatal(err)
	}
	f.Add(valid)
	f.Add([]byte(`{"schema_version":1}`))
	f.Add([]byte(`{"schema_version":1,"product":"demo","PRODUCT":"x"}`))
	f.Add([]byte("null"))
	f.Fuzz(func(t *testing.T, data []byte) {
		p, err := ParsePlan(data)
		if err != nil {
			return
		}
		again, err := json.Marshal(p.Document())
		if err != nil {
			t.Fatal(err)
		}
		back, err := ParsePlan(again)
		if err != nil || back != p {
			t.Fatalf("round trip: %+v, %v; want %+v", back, err, p)
		}
	})
}

// FuzzVersionPolicy: Compare is antisymmetric, its errors are symmetric, and
// Validate accepts nothing outside the strict vMAJOR.MINOR.PATCH grammar.
func FuzzVersionPolicy(f *testing.F) {
	f.Add("v1.2.3", "v1.10.0")
	f.Add("v01.2.3", "v1.2.3-rc1")
	f.Add("v99999999999999999999999.0.0", "v1.0.0")
	f.Add("v1.2.3+meta", "v1.2.3\n")
	p := NewStrictVersionPolicy()
	f.Fuzz(func(t *testing.T, a, b string) {
		ab, errA := p.Compare(a, b)
		ba, errB := p.Compare(b, a)
		if (errA == nil) != (errB == nil) {
			t.Fatalf("asymmetric errors: %v / %v", errA, errB)
		}
		if errA == nil && ab != -ba {
			t.Fatalf("Compare(%q,%q)=%d but Compare(%q,%q)=%d", a, b, ab, b, a, ba)
		}
		if p.Validate(a) == nil && strings.ContainsAny(a, "-+\n ") {
			t.Fatalf("Validate accepted %q", a)
		}
	})
}
