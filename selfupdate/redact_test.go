package selfupdate

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// Redaction tests (docs/decisions/0025-MADR-align-with-go-sdk-design-practices.md
// D1): no fmt verb and no log/slog handler prints a secret.

const fixtureSecret = "fixture-secret-0123"

// secretForms are the ways a secret can show through a formatter: as
// text, as hex, as base64, or as the decimal bytes %v prints for a
// []byte.
func secretForms() []string {
	b := []byte(fixtureSecret)
	return []string{
		fixtureSecret,
		hex.EncodeToString(b),
		strings.ToUpper(hex.EncodeToString(b)),
		base64.StdEncoding.EncodeToString(b),
		strings.Trim(fmt.Sprint([]int{int(b[0]), int(b[1]), int(b[2])}), "[]"),
	}
}

func redactionFixtures(t *testing.T) map[string]any {
	t.Helper()
	cred := Credential{Header: "X-Token", Value: []byte(fixtureSecret), Source: "env:X_TOKEN"}
	base, err := url.Parse("https://api.example.invalid/api/v3")
	if err != nil {
		t.Fatal(err)
	}
	opts := GitHubOptions{
		Repository: Repository{Owner: "o", Name: "r"},
		Client:     &http.Client{},
		APIBaseURL: base,
		UserAgent:  "demo/v1.0.0",
		Token:      fixtureSecret,
		Limits:     DefaultLimits(),
	}
	src, err := NewGitHubSource(opts)
	if err != nil {
		t.Fatal(err)
	}
	return map[string]any{
		"Credential":     cred,
		"*Credential":    &cred,
		"GitHubOptions":  opts,
		"*GitHubOptions": &opts,
		"*GitHubSource":  src,
		"slice":          []any{cred, opts, src},
		"struct field":   struct{ C Credential }{cred},
	}
}

func TestSecretsNeverFormat(t *testing.T) {
	for name, v := range redactionFixtures(t) {
		for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d"} {
			out := fmt.Sprintf(verb, v)
			for _, form := range secretForms() {
				if strings.Contains(out, form) {
					t.Errorf("%s with %s prints the secret as %q: %s", name, verb, form, out)
				}
			}
		}
		for _, h := range []struct {
			name string
			new  func(*bytes.Buffer) slog.Handler
		}{
			{"text", func(b *bytes.Buffer) slog.Handler { return slog.NewTextHandler(b, nil) }},
			{"json", func(b *bytes.Buffer) slog.Handler { return slog.NewJSONHandler(b, nil) }},
		} {
			var buf bytes.Buffer
			slog.New(h.new(&buf)).Info("fixture", "v", v)
			if name == "struct field" && h.name == "json" {
				// slog encodes a value without LogValue with
				// encoding/json, which keeps Value (Amendment A3).
				if !strings.Contains(buf.String(), base64.StdEncoding.EncodeToString([]byte(fixtureSecret))) {
					t.Errorf("a struct holding a credential, through slog's json handler, = %s; want encoding/json's Value", buf.String())
				}
				continue
			}
			for _, form := range secretForms() {
				if strings.Contains(buf.String(), form) {
					t.Errorf("%s through slog's %s handler prints the secret as %q: %s", name, h.name, form, buf.String())
				}
			}
		}
	}
	for _, v := range []fmt.Stringer{Credential{Value: []byte("x")}, GitHubOptions{Token: "x"}} {
		if s := v.String(); !strings.Contains(s, "<redacted>") {
			t.Errorf("%T.String() = %q, want <redacted>", v, s)
		}
	}
	if s := (Credential{}).String(); !strings.Contains(s, "Value:<empty>") {
		t.Errorf("an empty credential prints %q, want Value:<empty>", s)
	}
	var nilSource *GitHubSource
	if s := fmt.Sprint(nilSource); s != "GitHubSource(nil)" {
		t.Errorf("a nil source prints %q", s)
	}
}

// TestCredentialJSONKeepsValue: JSON is how a CredentialObserver saves an
// accepted credential, so encoding/json still carries Value (D1-A).
func TestCredentialJSONKeepsValue(t *testing.T) {
	out, err := json.Marshal(Credential{Value: []byte("v")})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"Value":"dg=="`) {
		t.Fatalf("json = %s, want Value kept", out)
	}
}
