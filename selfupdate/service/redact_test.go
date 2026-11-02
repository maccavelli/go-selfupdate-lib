package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/internal/testproc"
)

// Redaction tests (docs/decisions/0040-MADR-close-four-open-items-after-v1-23-1.md
// D2-A): no fmt verb and no log/slog handler prints an Env value, as
// selfupdate's redact_test.go checks for its secrets.

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

// fixtureEnv holds a secret value, an empty value, and an entry with no
// "=", which is itself the secret.
func fixtureEnv() []string {
	return []string{"GH_TOKEN=" + fixtureSecret, "EMPTY=", fixtureSecret}
}

func TestEnvNeverFormats(t *testing.T) {
	testproc.CheckNoLeak(t)
	cmd := Command{Path: "/usr/bin/gh", Args: []string{"attestation"}, Env: fixtureEnv(), Stdin: []byte(fixtureSecret)}
	ho := HandOff{Executable: "/opt/demo", Args: []string{"update"}, Env: fixtureEnv()}
	eo := ExecOptions{
		Reconcile:   []string{"setup"},
		RestoreFunc: func(context.Context, string, Receipt) error { return nil },
		Env:         fixtureEnv(),
		Runner:      ExecRunner(),
	}
	values := map[string]any{
		"Command": cmd, "*Command": &cmd,
		"HandOff": ho, "*HandOff": &ho,
		"ExecOptions": eo, "*ExecOptions": &eo,
		"slice":        []any{cmd, ho},
		"struct field": struct{ C Command }{cmd},
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
}

func TestEnvFormatText(t *testing.T) {
	testproc.CheckNoLeak(t)
	for _, c := range []struct {
		name string
		v    fmt.Stringer
		want string
	}{
		{"Command", Command{Path: "/usr/bin/gh", Args: []string{"attestation"}, Env: fixtureEnv(), Stdin: []byte("{}")},
			`Command{Path:"/usr/bin/gh" Args:["attestation"] Env:["GH_TOKEN=<redacted>" "EMPTY=<empty>" "<redacted>"] Stdin:<2 bytes>}`},
		{"zero Command", Command{}, `Command{Path:"" Args:[] Env:<nil> Stdin:<nil>}`},
		{"empty Env", Command{Env: []string{}}, `Command{Path:"" Args:[] Env:[] Stdin:<nil>}`},
		{"HandOff", HandOff{ID: "h1", Executable: "/opt/demo", Args: []string{"update"}, Env: []string{"A=b"}, ResultPath: "/opt/.demo.selfupdate.handoff"},
			`HandOff{ID:"h1" Executable:"/opt/demo" Args:["update"] Env:["A=<redacted>"] ResultPath:"/opt/.demo.selfupdate.handoff"}`},
		{"ExecOptions", ExecOptions{Reconcile: []string{"setup"}, RestoreFunc: func(context.Context, string, Receipt) error { return nil }},
			`ExecOptions{Reconcile:["setup"] Restore:[] RestoreFunc:<set> Env:<nil> Runner:<nil>}`},
	} {
		if got := c.v.String(); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
	}
	var nilCmd *Command
	if s := fmt.Sprint(nilCmd); s != "<nil>" {
		t.Errorf("a nil *Command prints %q", s)
	}
}
