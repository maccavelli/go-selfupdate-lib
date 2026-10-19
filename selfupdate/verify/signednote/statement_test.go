package signednote

import (
	"reflect"
	"strings"
	"testing"
)

func fixtureStatement() Statement {
	return Statement{
		Repository:     "github.com/owner/demo",
		Tag:            "v1.2.3",
		Products:       []string{"demo", "democtl"},
		Manifest:       "SHA256SUMS",
		ManifestSHA256: strings.Repeat("ab", 32),
	}
}

const fixtureText = "selfupdate-release/v1\n" +
	"repository github.com/owner/demo\n" +
	"tag v1.2.3\n" +
	"products demo democtl\n" +
	"manifest SHA256SUMS sha256:" + "abababababababababababababababababababababababababababababababab" + "\n"

func TestStatementRoundTrip(t *testing.T) {
	text, err := fixtureStatement().Text()
	if err != nil || text != fixtureText {
		t.Fatalf("Text = %q, %v; want %q", text, err, fixtureText)
	}
	again, _ := fixtureStatement().Text()
	if again != text {
		t.Fatal("Text is not deterministic")
	}
	s, err := ParseStatement(text)
	if err != nil || !reflect.DeepEqual(s, fixtureStatement()) {
		t.Fatalf("ParseStatement = %+v, %v", s, err)
	}
}

func TestParseStatementRefuses(t *testing.T) {
	line := func(i int, with string) string {
		lines := strings.Split(strings.TrimSuffix(fixtureText, "\n"), "\n")
		lines[i] = with
		return strings.Join(lines, "\n") + "\n"
	}
	for name, text := range map[string]string{
		"CRLF":                 strings.ReplaceAll(fixtureText, "\n", "\r\n"),
		"no final LF":          strings.TrimSuffix(fixtureText, "\n"),
		"blank line":           strings.Replace(fixtureText, "\ntag", "\n\ntag", 1),
		"trailing line":        fixtureText + "extra\n",
		"another header":       line(0, "selfupdate-release/v2"),
		"lines out of order":   line(1, "tag v1.2.3"),
		"unsorted products":    line(3, "products democtl demo"),
		"duplicate products":   line(3, "products demo demo"),
		"double space":         line(3, "products demo  democtl"),
		"no products":          line(3, "products "),
		"uppercase digest":     line(4, "manifest SHA256SUMS sha256:"+strings.Repeat("AB", 32)),
		"short digest":         line(4, "manifest SHA256SUMS sha256:abab"),
		"no digest":            line(4, "manifest SHA256SUMS"),
		"short tag":            line(2, "tag v1.2"),
		"build metadata":       line(2, "tag v1.2.3+build"),
		"repository no host":   line(1, "repository owner/demo"),
		"repository uppercase": line(1, "repository GitHub.com/owner/demo"),
		"bad product":          line(3, "products -demo"),
	} {
		if _, err := ParseStatement(text); err == nil {
			t.Errorf("%s: accepted %q", name, text)
		}
	}
}

func TestTextRefusesInvalid(t *testing.T) {
	for name, edit := range map[string]func(*Statement){
		"no products": func(s *Statement) { s.Products = nil },
		"unsorted":    func(s *Statement) { s.Products = []string{"b", "a"} },
		"bad tag":     func(s *Statement) { s.Tag = "1.2.3" },
		"bad digest":  func(s *Statement) { s.ManifestSHA256 = "xyz" },
		"bad name":    func(s *Statement) { s.Manifest = "a b" },
		"bad repo":    func(s *Statement) { s.Repository = "github.com/owner" },
	} {
		s := fixtureStatement()
		edit(&s)
		if _, err := s.Text(); err == nil {
			t.Errorf("%s: Text accepted %+v", name, s)
		}
	}
}
