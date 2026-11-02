package redact

import (
	"log/slog"
	"slices"
	"testing"
)

func TestEnv(t *testing.T) {
	t.Parallel()
	in := []string{"GH_TOKEN=secret", "EMPTY=", "secret", "=C:=C:\\x", "A=b=c"}
	want := []string{"GH_TOKEN=<redacted>", "EMPTY=<empty>", "<redacted>", "=<redacted>", "A=<redacted>"}
	if got := Env(in); !slices.Equal(got, want) {
		t.Errorf("Env = %q, want %q", got, want)
	}
	if Env(nil) != nil {
		t.Error("Env(nil) is not nil")
	}
	if got := Env([]string{}); got == nil || len(got) != 0 {
		t.Errorf("Env of an empty env = %#v, want empty and non-nil", got)
	}
}

func TestText(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ got, want string }{
		{EnvText(nil), "<nil>"},
		{EnvText([]string{}), "[]"},
		{EnvText([]string{"A=b"}), `["A=<redacted>"]`},
		{Quoted(nil), "[]"},
		{Quoted([]string{"a b"}), `["a b"]`},
		{Length(nil), "<nil>"},
		{Length([]byte{}), "<0 bytes>"},
		{Length([]byte("abc")), "<3 bytes>"},
		{SetOrNil(true), "<set>"},
		{SetOrNil(false), "<nil>"},
	} {
		if c.got != c.want {
			t.Errorf("got %q, want %q", c.got, c.want)
		}
	}
	if a := EnvAttr(nil); a.Key != "env" || a.Value.String() != "<nil>" {
		t.Errorf("EnvAttr(nil) = %v", a)
	}
	if a := EnvAttr([]string{"A=b"}); a.Key != "env" || a.Value.Kind() != slog.KindAny {
		t.Errorf("EnvAttr = %v", a)
	}
}
