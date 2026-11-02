package ghattest

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// Redaction tests (docs/decisions/0040-MADR-close-four-open-items-after-v1-23-1.md
// D2-A): no fmt verb and no log/slog handler prints an Env value.

const fixtureSecret = "fixture-secret-0123"

// secretForms are the ways a secret can show through a formatter.
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

func TestOptionsEnvNeverFormats(t *testing.T) {
	t.Parallel()
	o := Options{
		Policy: Policy{Repository: selfupdate.Repository{Owner: "o", Name: "r"}},
		GH:     "/usr/bin/gh",
		Env:    []string{"GH_TOKEN=" + fixtureSecret, "EMPTY=", fixtureSecret},
		Runner: service.ExecRunner(),
	}
	nested := o
	nested.Runner = nil // encoding/json cannot encode a Runner, which is a function
	values := map[string]any{
		"Options": o, "*Options": &o,
		"slice":        []any{nested},
		"struct field": struct{ O Options }{nested},
	}
	for name, v := range values {
		for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d"} {
			out := fmt.Sprintf(verb, v)
			for _, form := range secretForms() {
				if strings.Contains(out, form) {
					t.Errorf("%s with %s prints the secret as %q: %s", name, verb, form, out)
				}
			}
			if !strings.Contains(out, "GH_TOKEN=<redacted>") {
				t.Errorf("%s with %s does not name GH_TOKEN: %s", name, verb, out)
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
			if (name == "struct field" || name == "slice") && h.name == "json" {
				// slog encodes a value without LogValue with
				// encoding/json, which keeps Env (0025-MADR Amendment A3).
				if !strings.Contains(buf.String(), fixtureSecret) {
					t.Errorf("%s through slog's json handler = %s; want encoding/json's Env", name, buf.String())
				}
				continue
			}
			for _, form := range secretForms() {
				if strings.Contains(buf.String(), form) {
					t.Errorf("%s through slog's %s handler prints the secret as %q: %s", name, h.name, form, buf.String())
				}
			}
			if !strings.Contains(buf.String(), "GH_TOKEN=<redacted>") {
				t.Errorf("%s through slog's %s handler does not name GH_TOKEN: %s", name, h.name, buf.String())
			}
		}
	}
	want := `Options{Policy:{Repository:"o/r" SignerWorkflow:"" SignerDigests:[] AllowSelfHostedRunners:false} GH:"/usr/bin/gh" ` +
		`Env:["GH_TOKEN=<redacted>" "EMPTY=<empty>" "<redacted>"] Runner:<set> Timeout:0s TempDir:""}`
	if got := o.String(); got != want {
		t.Errorf("String:\n got %s\nwant %s", got, want)
	}
	if got := (Options{Timeout: time.Minute}).String(); !strings.Contains(got, "Env:<nil> Runner:<nil> Timeout:1m0s") {
		t.Errorf("zero Options print %s", got)
	}
}
