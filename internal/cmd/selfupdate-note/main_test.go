package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/mod/sumdb/note"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate/verify/signednote"
)

// Tests for the release signer (docs/decisions/0025-MADR-align-with-go-sdk-design-practices.md
// D10).

func runCmd(t *testing.T, env map[string]string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), args, func(k string) string { return env[k] }, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// keys makes a key pair with keygen, and returns the signer key and the
// verifier key it printed.
func keys(t *testing.T, name string) (skey, vkey string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "key")
	code, out, errOut := runCmd(t, nil, "keygen", "-name", name, "-out", path)
	if code != 0 {
		t.Fatalf("keygen: %d %s", code, errOut)
	}
	b, err := os.ReadFile(path) //nolint:gosec // the test's own file
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(b)), strings.TrimSpace(out)
}

func TestKeygen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "key")
	code, out, errOut := runCmd(t, nil, "keygen", "-name", "github.com/owner/demo/selfupdate", "-out", path)
	if code != 0 {
		t.Fatalf("keygen: %d %s", code, errOut)
	}
	if _, err := note.NewVerifier(strings.TrimSpace(out)); err != nil {
		t.Fatalf("printed %q, not a verifier key: %v", out, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("the signer key file is mode %v, want 0600", info.Mode().Perm())
	}
	b, err := os.ReadFile(path) //nolint:gosec // the test's own file
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, strings.TrimSpace(string(b))) {
		t.Fatal("keygen printed the signer key")
	}
	if code, _, _ := runCmd(t, nil, "keygen", "-name", "x", "-out", path); code != 1 {
		t.Fatalf("keygen over an existing file: exit %d, want 1", code)
	}
	if code, _, _ := runCmd(t, nil, "keygen", "-name", "a+b", "-out", filepath.Join(dir, "k2")); code != 2 {
		t.Fatalf("a + in the name: exit %d, want 2", code)
	}
}

func TestSign(t *testing.T) {
	skey, vkey := keys(t, "github.com/owner/demo/selfupdate")
	_, otherVKey := keys(t, "github.com/owner/demo/selfupdate")
	dir := t.TempDir()
	manifest := []byte(strings.Repeat("a", 64) + "  demo-linux-amd64\n")
	if err := os.WriteFile(filepath.Join(dir, "SHA256SUMS"), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	args := func(out, verifier string) []string {
		return []string{"sign", "-dir", dir, "-manifest", "SHA256SUMS", "-repository", "github.com/owner/demo",
			"-tag", "v1.2.3", "-products-json", `["democtl","demo"]`, "-verifier-key", verifier, "-out", out}
	}
	env := map[string]string{keyEnv: skey}
	first, second := filepath.Join(dir, "out", "1.note"), filepath.Join(dir, "out", "2.note")
	for _, out := range []string{first, second} {
		code, stdout, stderr := runCmd(t, env, args(out, vkey)...)
		if code != 0 {
			t.Fatalf("sign: %d %s", code, stderr)
		}
		if strings.Contains(stdout+stderr, skey) {
			t.Fatal("sign printed the signer key")
		}
	}
	a, err := os.ReadFile(first) //nolint:gosec // the test's own file
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(second) //nolint:gosec // the test's own file
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Fatal("two signs of the same statement differ")
	}
	st, err := signednote.Open(a, []string{vkey})
	if err != nil || st.Tag != "v1.2.3" || strings.Join(st.Products, " ") != "demo democtl" {
		t.Fatalf("the note opens as %+v, %v", st, err)
	}
	for name, c := range map[string]struct {
		env  map[string]string
		args []string
		want int
	}{
		"empty secret":          {map[string]string{}, args(filepath.Join(dir, "e.note"), vkey), 1},
		"another verifier key":  {env, args(filepath.Join(dir, "v.note"), otherVKey), 1},
		"an existing output":    {env, args(first, vkey), 1},
		"an unparsable secret":  {map[string]string{keyEnv: "PRIVATE+KEY+bad"}, args(filepath.Join(dir, "u.note"), vkey), 1},
		"a missing flag":        {env, []string{"sign", "-dir", dir}, 2},
		"an unknown subcommand": {env, []string{"verify"}, 2},
	} {
		code, stdout, stderr := runCmd(t, c.env, c.args...)
		if code != c.want {
			t.Errorf("%s: exit %d, want %d (%s)", name, code, c.want, stderr)
		}
		if strings.Contains(stdout+stderr, skey) {
			t.Errorf("%s: printed the signer key", name)
		}
	}
}
