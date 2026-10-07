package selfupdate

import (
	"errors"
	"strings"
	"testing"
)

const testDigest = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"

func TestParseSHA256SUMS(t *testing.T) {
	t.Run("gnu text and binary", func(t *testing.T) {
		body := testDigest + "  demo-linux-amd64\n" +
			strings.ToUpper(testDigest) + " *demo-windows-amd64.exe\n"
		got, err := ParseSHA256SUMS([]byte(body))
		if err != nil {
			t.Fatal(err)
		}
		if got["demo-linux-amd64"] != testDigest {
			t.Fatalf("text entry = %q", got["demo-linux-amd64"])
		}
		if got["demo-windows-amd64.exe"] != testDigest {
			t.Fatalf("binary entry = %q", got["demo-windows-amd64.exe"])
		}
	})

	t.Run("crlf blank and comment", func(t *testing.T) {
		body := "# generated\r\n\r\n" + testDigest + "  demo-linux-amd64\r\n"
		got, err := ParseSHA256SUMS([]byte(body))
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got["demo-linux-amd64"] != testDigest {
			t.Fatalf("got %#v", got)
		}
	})

	t.Run("duplicate entries", func(t *testing.T) {
		body := testDigest + "  demo-linux-amd64\n" + testDigest + "  demo-linux-amd64\n"
		if _, err := ParseSHA256SUMS([]byte(body)); err == nil {
			t.Fatal("accepted duplicate filename")
		}
	})

	t.Run("malformed hex", func(t *testing.T) {
		body := strings.Repeat("zz", 32) + "  demo-linux-amd64\n"
		if _, err := ParseSHA256SUMS([]byte(body)); !errors.Is(err, ErrIntegrity) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("extra fields", func(t *testing.T) {
		body := testDigest + "  demo-linux-amd64 extra\n"
		if _, err := ParseSHA256SUMS([]byte(body)); err == nil {
			t.Fatal("accepted extra fields")
		}
	})

	t.Run("absolute unix path", func(t *testing.T) {
		body := testDigest + "  /tmp/demo-linux-amd64\n"
		if _, err := ParseSHA256SUMS([]byte(body)); err == nil {
			t.Fatal("accepted absolute path")
		}
	})

	t.Run("traversal", func(t *testing.T) {
		body := testDigest + "  ../demo-linux-amd64\n"
		if _, err := ParseSHA256SUMS([]byte(body)); err == nil {
			t.Fatal("accepted traversal")
		}
	})

	t.Run("nested path", func(t *testing.T) {
		body := testDigest + "  dir/demo-linux-amd64\n"
		if _, err := ParseSHA256SUMS([]byte(body)); err == nil {
			t.Fatal("accepted nested path")
		}
	})

	t.Run("overlong line", func(t *testing.T) {
		body := testDigest + "  " + strings.Repeat("a", maxChecksumLine) + "\n"
		if _, err := ParseSHA256SUMS([]byte(body)); err == nil {
			t.Fatal("accepted overlong line")
		}
	})

	t.Run("lookup", func(t *testing.T) {
		body := testDigest + "  demo-linux-amd64\n" +
			strings.Repeat("cd", 32) + "  demo-darwin-arm64\n"
		entries, err := ParseSHA256SUMS([]byte(body))
		if err != nil {
			t.Fatal(err)
		}
		got, err := checksumFor(entries, "demo-linux-amd64")
		if err != nil {
			t.Fatal(err)
		}
		if got != testDigest {
			t.Fatalf("lookup = %q", got)
		}
		if _, err := checksumFor(entries, "missing"); err == nil {
			t.Fatal("lookup succeeded for missing name")
		}
	})
}

func TestParseGitHubDigest(t *testing.T) {
	got, err := parseGitHubDigest("sha256:" + strings.ToUpper(testDigest))
	if err != nil {
		t.Fatal(err)
	}
	if got != testDigest {
		t.Fatalf("normalized = %q", got)
	}
	if _, err := parseGitHubDigest("SHA256:" + testDigest); err == nil {
		t.Fatal("accepted uppercase prefix")
	}
	if _, err := parseGitHubDigest(testDigest); err == nil {
		t.Fatal("accepted bare hex")
	}
}

// TestChecksumNameRefusesColon: a ':' is refused on every OS. filepath.Base
// strips a drive volume on Windows only, so it used to refuse "a:b" there and
// accept it elsewhere (0010-MADR A12).
func TestChecksumNameRefusesColon(t *testing.T) {
	for _, name := range []string{"a:b", "C:x", "x:"} {
		if err := validateChecksumName(name); !errors.Is(err, ErrIntegrity) {
			t.Errorf("%q: err = %v, want ErrIntegrity", name, err)
		}
		line := strings.Repeat("a", 64) + "  " + name + "\n"
		if _, err := ParseSHA256SUMS([]byte(line)); !errors.Is(err, ErrIntegrity) {
			t.Errorf("ParseSHA256SUMS with %q: err = %v, want ErrIntegrity", name, err)
		}
	}
}

// TestChecksumNameFitsTheLine: a name is accepted only when its canonical
// line, "<digest>  <name>" and a newline, fits the line cap, so every
// manifest the parser accepts still parses once written back. A line with
// one space before its name is a byte shorter than the canonical one
// (0015-MADR amendment A2, A10).
func TestChecksumNameFitsTheLine(t *testing.T) {
	fits := strings.Repeat("n", 4029)
	for _, body := range []string{testDigest + " " + fits + "\n", testDigest + "  " + fits + "\n"} {
		got, err := ParseSHA256SUMS([]byte(body))
		if err != nil || got[fits] != testDigest {
			t.Fatalf("a %d-byte name in a %d-byte line: %v", len(fits), len(body), err)
		}
	}
	over := fits + "n"
	line := testDigest + " " + over + "\n"
	if _, err := ParseSHA256SUMS([]byte(line)); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("a %d-byte name in a %d-byte line: err = %v, want ErrIntegrity", len(over), len(line), err)
	}
	if err := validateChecksumName(over); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("validateChecksumName of %d bytes: err = %v, want ErrIntegrity", len(over), err)
	}
}
