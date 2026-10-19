package signednote_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/mod/sumdb/note"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/selfupdatetest"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/verify/signednote"
)

// Tests for the release statement verifier
// (docs/decisions/0025-MADR-align-with-go-sdk-design-practices.md D10).

const repo = "github.com/owner/demo"

// keypair is a key pair from a fixed seed, so the test is repeatable.
func keypair(t *testing.T, name string, seed byte) (skey, vkey string) {
	t.Helper()
	skey, vkey, err := note.GenerateKey(bytes.NewReader(bytes.Repeat([]byte{seed}, 64)), name)
	if err != nil {
		t.Fatal(err)
	}
	return skey, vkey
}

func statementFor(tag string, manifest []byte) signednote.Statement {
	sum := sha256.Sum256(manifest)
	return signednote.Statement{Repository: repo, Tag: tag, Products: []string{"demo"}, Manifest: "SHA256SUMS", ManifestSHA256: hex.EncodeToString(sum[:])}
}

func signNote(t *testing.T, st signednote.Statement, skeys ...string) []byte {
	t.Helper()
	text, err := st.Text()
	if err != nil {
		t.Fatal(err)
	}
	var signers []note.Signer
	for _, k := range skeys {
		s, err := note.NewSigner(k)
		if err != nil {
			t.Fatal(err)
		}
		signers = append(signers, s)
	}
	msg, err := note.Sign(&note.Note{Text: text}, signers...)
	if err != nil {
		t.Fatal(err)
	}
	return msg
}

// verification is the input the Updater gives a ManifestVerifier, with the
// note served when msg is not nil.
func verification(tag string, manifest, msg []byte) selfupdate.ManifestVerification {
	assets := []selfupdate.Asset{{ID: 1, Name: "SHA256SUMS"}}
	if msg != nil {
		assets = append(assets, selfupdate.Asset{ID: 2, Name: "SHA256SUMS.note"})
	}
	return selfupdate.ManifestVerification{
		Product:   "demo",
		Release:   selfupdate.Release{Tag: tag, Assets: assets},
		Selection: selfupdate.Selection{Manifest: selfupdate.Asset{ID: 1, Name: "SHA256SUMS"}, ManifestName: "demo-linux-amd64"},
		Manifest:  manifest,
		OpenAsset: func(_ context.Context, name string, limit int64) (io.ReadCloser, error) {
			if name != "SHA256SUMS.note" || msg == nil {
				return nil, fmt.Errorf("no asset %q", name)
			}
			if int64(len(msg)) > limit {
				return nil, fmt.Errorf("asset %q exceeds limit %d", name, limit)
			}
			return io.NopCloser(bytes.NewReader(msg)), nil
		},
	}
}

func TestVerifyManifest(t *testing.T) {
	ciKey, ciVKey := keypair(t, "github.com/owner/demo/selfupdate", 1)
	otherKey, otherVKey := keypair(t, "github.com/owner/demo/selfupdate", 2)
	manifest := []byte(strings.Repeat("a", 64) + "  demo-linux-amd64\n")
	good := signNote(t, statementFor("v1.2.3", manifest), ciKey)
	edited := func(edit func(*signednote.Statement)) []byte {
		st := statementFor("v1.2.3", manifest)
		edit(&st)
		return signNote(t, st, ciKey)
	}
	for _, c := range []struct {
		name     string
		cfg      signednote.Config
		tag      string
		manifest []byte
		msg      []byte
		ok       bool
	}{
		{"valid", signednote.Config{Repository: repo, Keys: []string{ciVKey}}, "v1.2.3", manifest, good, true},
		{"rotation: old and new keys signed, new key trusted", signednote.Config{Repository: repo, Keys: []string{otherVKey}}, "v1.2.3", manifest,
			signNote(t, statementFor("v1.2.3", manifest), ciKey, otherKey), true},
		{"wrong key", signednote.Config{Repository: repo, Keys: []string{otherVKey}}, "v1.2.3", manifest, good, false},
		{"another tag", signednote.Config{Repository: repo, Keys: []string{ciVKey}}, "v1.2.4", manifest, good, false},
		{"another repository", signednote.Config{Repository: "github.com/owner/other", Keys: []string{ciVKey}}, "v1.2.3", manifest, good, false},
		{"another product", signednote.Config{Repository: repo, Keys: []string{ciVKey}}, "v1.2.3", manifest,
			edited(func(s *signednote.Statement) { s.Products = []string{"other"} }), false},
		{"another manifest name", signednote.Config{Repository: repo, Keys: []string{ciVKey}}, "v1.2.3", manifest,
			edited(func(s *signednote.Statement) { s.Manifest = "checksums.txt" }), false},
		{"a changed manifest byte", signednote.Config{Repository: repo, Keys: []string{ciVKey}}, "v1.2.3", append([]byte("b"), manifest[1:]...), good, false},
		{"missing at RequiredFrom", signednote.Config{Repository: repo, Keys: []string{ciVKey}, RequiredFrom: "v1.2.3"}, "v1.2.3", manifest, nil, false},
		{"missing with no RequiredFrom", signednote.Config{Repository: repo, Keys: []string{ciVKey}}, "v1.0.0", manifest, nil, false},
		{"missing below RequiredFrom", signednote.Config{Repository: repo, Keys: []string{ciVKey}, RequiredFrom: "v1.2.3"}, "v1.2.2", manifest, nil, true},
		{"missing on a prerelease of RequiredFrom", signednote.Config{Repository: repo, Keys: []string{ciVKey}, RequiredFrom: "v1.2.3"}, "v1.2.3-rc.1", manifest, nil, true},
		{"present below RequiredFrom, and wrong", signednote.Config{Repository: repo, Keys: []string{otherVKey}, RequiredFrom: "v9.0.0"}, "v1.2.3", manifest, good, false},
		{"over 16 KiB", signednote.Config{Repository: repo, Keys: []string{ciVKey}}, "v1.2.3", manifest, append(bytes.Repeat([]byte("x"), signednote.MaxNoteSize), good...), false},
		{"CRLF", signednote.Config{Repository: repo, Keys: []string{ciVKey}}, "v1.2.3", manifest, bytes.ReplaceAll(good, []byte("\n"), []byte("\r\n")), false},
	} {
		v, err := signednote.New(c.cfg)
		if err != nil {
			t.Fatalf("%s: New: %v", c.name, err)
		}
		err = v.VerifyManifest(context.Background(), verification(c.tag, c.manifest, c.msg))
		if (err == nil) != c.ok {
			t.Errorf("%s: err = %v, want ok %t", c.name, err, c.ok)
		}
	}
}

func TestNewRefuses(t *testing.T) {
	_, vkey := keypair(t, "github.com/owner/demo/selfupdate", 1)
	for name, c := range map[string]signednote.Config{
		"bad repository":   {Repository: "owner/demo", Keys: []string{vkey}},
		"no keys":          {Repository: repo},
		"unparsable key":   {Repository: repo, Keys: []string{"not-a-key"}},
		"key given twice":  {Repository: repo, Keys: []string{vkey, vkey}},
		"bad RequiredFrom": {Repository: repo, Keys: []string{vkey}, RequiredFrom: "1.0"},
	} {
		if _, err := signednote.New(c); err == nil {
			t.Errorf("%s: New accepted %+v", name, c)
		}
	}
}

// TestUpdaterRefusesWrongKey: through an Updater's dry run, a note the
// trusted key signed lets the run through, and one another key signed
// fails it with selfupdate.ErrIntegrity before the binary is fetched.
func TestUpdaterRefusesWrongKey(t *testing.T) {
	skey, vkey := keypair(t, "github.com/owner/demo/selfupdate", 1)
	_, otherVKey := keypair(t, "github.com/owner/demo/selfupdate", 2)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, "demo")
	if err := os.WriteFile(exe, []byte("demo v1.0.0\n"), 0o755); err != nil { //nolint:gosec // a test executable
		t.Fatal(err)
	}
	plats := []selfupdate.Platform{{OS: runtime.GOOS, Arch: runtime.GOARCH}}
	spec := selfupdatetest.NewRelease("demo", "v1.1.0", plats, func(selfupdate.Platform) []byte { return []byte("demo v1.1.0\n") })
	var manifest []byte
	for _, a := range spec.Assets {
		if a.Name == "SHA256SUMS" {
			manifest = a.Body
		}
	}
	spec.Assets = append(spec.Assets, selfupdatetest.AssetSpec{Name: "SHA256SUMS.note", Body: signNote(t, statementFor("v1.1.0", manifest), skey)})
	for _, c := range []struct {
		key string
		ok  bool
	}{{vkey, true}, {otherVKey, false}} {
		src := selfupdatetest.NewFakeSource("v1.1.0", spec)
		v, err := signednote.New(signednote.Config{Repository: repo, Keys: []string{c.key}})
		if err != nil {
			t.Fatal(err)
		}
		selector, err := selfupdate.NewExactAssetSelector(plats)
		if err != nil {
			t.Fatal(err)
		}
		installer, err := selfupdate.NewStandaloneInstaller(selfupdate.InstallOptions{
			TargetPolicy: selfupdate.TargetPolicy{ExecutablePath: exe, AllowedRoots: []string{dir}},
		})
		if err != nil {
			t.Fatal(err)
		}
		u, err := selfupdate.New(selfupdate.Config{
			Source: src, Versions: selfupdate.NewStrictVersionPolicy(), Assets: selector, Installer: installer,
			Reporter: selfupdate.DiscardReporter(), Confirmer: selfupdate.NonInteractiveConfirmer(),
			Limits: selfupdate.DefaultLimits(), ManifestVerifiers: []selfupdate.ManifestVerifier{v},
		})
		if err != nil {
			t.Fatal(err)
		}
		_, err = u.Run(context.Background(), selfupdate.Request{Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: selfupdate.ReleaseBuild, DryRun: true})
		if c.ok && err != nil {
			t.Errorf("the trusted key: %v", err)
		}
		if !c.ok && !errors.Is(err, selfupdate.ErrIntegrity) {
			t.Errorf("another key: err = %v, want ErrIntegrity", err)
		}
	}
}
