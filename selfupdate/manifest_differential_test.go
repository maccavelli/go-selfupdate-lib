package selfupdate

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestManifestDifferential compares ParseSHA256SUMS with the release
// verifier's own parser, scripts/selfupdate_manifest.py, on generated
// manifests (0004-MADR §8 H2, amendment C1). They must agree on accepting
// or rejecting each one, and on the entries of every accepted one: a
// manifest the verifier accepts but the client rejects becomes an immutable
// release nobody can install (0003-MADR D1).
//
// It skips when python3 is absent, unless SELFUPDATE_REQUIRE_PYTHON=1.
// SELFUPDATE_DIFFERENTIAL_N and SELFUPDATE_DIFFERENTIAL_SEED override the
// case count and the seed, for a longer hunt.
func TestManifestDifferential(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		if os.Getenv("SELFUPDATE_REQUIRE_PYTHON") == "1" {
			t.Fatalf("python3 is required (SELFUPDATE_REQUIRE_PYTHON=1): %v", err)
		}
		t.Skip("python3 not found; the differential needs the verifier's parser")
	}
	n := envInt(t, "SELFUPDATE_DIFFERENTIAL_N", 5000)
	seed := uint64(envInt(t, "SELFUPDATE_DIFFERENTIAL_SEED", 20261001))
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))

	dir := t.TempDir()
	cases := make([][]byte, n)
	for i := range cases {
		cases[i] = genManifest(rng)
		if err := os.WriteFile(filepath.Join(dir, caseName(i)), cases[i], 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command(python, "-B", filepath.Join("..", "scripts", "selfupdate_manifest.py"), "parse", dir)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("the verifier's parser failed: %v\n%s", err, stderr.String())
	}
	type pyResult struct {
		Case    string            `json:"case"`
		OK      bool              `json:"ok"`
		Entries map[string]string `json:"entries"`
		Error   string            `json:"error"`
	}
	results := map[string]pyResult{}
	for _, line := range bytes.Split(bytes.TrimSpace(out), []byte("\n")) {
		var r pyResult
		if err := json.Unmarshal(line, &r); err != nil {
			t.Fatalf("unreadable result %q: %v", line, err)
		}
		results[r.Case] = r
	}
	if len(results) != n {
		t.Fatalf("the verifier's parser returned %d results for %d cases", len(results), n)
	}

	accepted, disagreements := 0, 0
	for i, data := range cases {
		py, ok := results[caseName(i)]
		if !ok {
			t.Fatalf("no result for case %d", i)
		}
		entries, goErr := ParseSHA256SUMS(data)
		if goErr == nil {
			accepted++
		}
		var why string
		switch {
		case (goErr == nil) != py.OK:
			why = fmt.Sprintf("go: %v; python: ok=%v %s", goErr, py.OK, py.Error)
		case goErr == nil && !sameEntries(entries, py.Entries):
			why = fmt.Sprintf("entries differ: go %v; python %v", hexEntries(entries), py.Entries)
		default:
			continue
		}
		disagreements++
		if disagreements <= 10 {
			t.Errorf("case %d (seed %d) disagrees: %s\ninput: %q", i, seed, why, data)
		}
	}
	if disagreements > 10 {
		t.Errorf("%d disagreements in all; the first ten are above", disagreements)
	}
	t.Logf("N=%d seed=%d accepted=%d rejected=%d", n, seed, accepted, n-accepted)
	// Both outcomes must be common, or the comparison proves little.
	if floor := n * 15 / 100; accepted < floor || n-accepted < floor {
		t.Fatalf("accepted %d, rejected %d of %d: each must be at least %d", accepted, n-accepted, n, floor)
	}
}

func envInt(t *testing.T, name string, def int) int {
	t.Helper()
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		t.Fatalf("%s=%q is not a positive integer", name, v)
	}
	return n
}

func caseName(i int) string { return fmt.Sprintf("%05d", i) }

func hexEntries(entries map[string]string) map[string]string {
	out := make(map[string]string, len(entries))
	for name, digest := range entries {
		out[hex.EncodeToString([]byte(name))] = digest
	}
	return out
}

func sameEntries(entries, py map[string]string) bool {
	h := hexEntries(entries)
	if len(h) != len(py) {
		return false
	}
	for k, v := range h {
		if py[k] != v {
			return false
		}
	}
	return true
}

func pick[T any](r *rand.Rand, xs []T) T { return xs[r.IntN(len(xs))] }

var (
	// diffSeparators are field separators. The first group is whitespace to
	// Go's unicode.IsSpace, so both parsers split on it; U+001C..U+001F are
	// whitespace to Python's str.isspace but not to Go, which the verifier's
	// go_isspace must reproduce.
	diffSeparators = []string{
		" ", " ", " ", "  ", "\t", " \t ", "\v", "\f", "\r",
		"\u0085", "\u00a0", "\u1680", "\u2028", "\u2029", "\u3000",
	}
	diffNonSeparators = []string{"\x1c", "\x1d", "\x1e", "\x1f", "", "\x85", "\u200b"}
	diffGoodNames     = []string{"demo-linux-amd64", "demo-windows-amd64.exe", "a", "n\u00e4me", "x.tar.gz", "\xff\xfename", "a\x00b"}
	diffBadNames      = []string{"*", "**x", "a*b", "a/b", "a\\b", ".", "..", "/abs", "x/", "*/x", "a:b", "C:x"}
)

// genManifest returns one SHA256SUMS candidate. Most lines are close to
// valid, so both outcomes are common.
func genManifest(r *rand.Rand) []byte {
	if r.IntN(10) == 0 {
		return genRandomBytes(r)
	}
	var b bytes.Buffer
	if r.IntN(20) == 0 {
		b.WriteString("\xef\xbb\xbf")
	}
	var names []string
	lines := 1 + r.IntN(4)
	for i := range lines {
		switch r.IntN(14) {
		case 0:
			b.WriteString(pick(r, []string{"", " ", "\t", "\u00a0", "\u3000"}))
		case 1:
			b.WriteString(pick(r, []string{"", " ", "\t"}) + "# comment " + strconv.Itoa(r.IntN(100)))
		case 2:
			// A line around the scanner's limit: maxChecksumLine is 4096, and
			// the line and its newline must fit.
			b.WriteString(genLongLine(r))
		default:
			b.WriteString(genEntry(r, &names))
		}
		last := i == lines-1
		switch {
		case last && r.IntN(4) == 0:
			// no final newline
		case r.IntN(10) == 0:
			b.WriteString("\r\n")
		case r.IntN(20) == 0:
			b.WriteString("\r\r\n")
		default:
			b.WriteString("\n")
		}
	}
	return b.Bytes()
}

func genDigest(r *rand.Rand) string {
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(r.UintN(256))
	}
	d := hex.EncodeToString(raw)
	switch r.IntN(16) {
	case 0:
		return strings.ToUpper(d)
	case 1:
		return strings.ToUpper(d[:32]) + d[32:]
	case 2:
		return d[:63]
	case 3:
		return d + "0"
	case 4:
		return d[:10] + "g" + d[11:]
	case 5:
		return "\uff10" + d[1:] // a full-width digit
	default:
		return d
	}
}

func genEntry(r *rand.Rand, names *[]string) string {
	var name string
	switch n := r.IntN(14); {
	case n == 0:
		name = pick(r, diffBadNames)
	case n == 1 && len(*names) > 0:
		name = pick(r, *names) // a duplicate
	case n == 2:
		name = "*" + pick(r, diffGoodNames)
	default:
		name = pick(r, diffGoodNames) + strconv.Itoa(len(*names))
	}
	*names = append(*names, name)
	sep := pick(r, diffSeparators)
	if r.IntN(12) == 0 {
		sep = pick(r, diffNonSeparators)
	}
	line := pick(r, []string{"", "", "", " ", "\t", "\u2028"}) + genDigest(r) + sep + name
	switch r.IntN(14) {
	case 0:
		line += " extra"
	case 1:
		line += pick(r, []string{" ", "\t", "\u3000", "\u0085"})
	}
	return line
}

func genLongLine(r *rand.Rand) string {
	total := 4094 + r.IntN(4)
	if r.IntN(2) == 0 {
		return "#" + strings.Repeat("c", total-1)
	}
	prefix := strings.Repeat("ab", 32) + "  "
	return prefix + strings.Repeat("n", total-len(prefix))
}

func genRandomBytes(r *rand.Rand) []byte {
	tokens := []string{
		strings.Repeat("ab", 32), strings.Repeat("CD", 32), "demo", " ", "  ", "\t", "\n", "\r\n", "\r",
		"#", "*", "/", "\\", ".", "\u0085", "\u2028", "\xff", "\x1c", "x",
	}
	var b bytes.Buffer
	for range r.IntN(12) {
		b.WriteString(pick(r, tokens))
	}
	return b.Bytes()
}
