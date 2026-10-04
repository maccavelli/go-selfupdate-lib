package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Tests for docs/decisions/0004-PLAN-v1-1-0-core-api.md Step 7 (manifest
// verifiers and OpenAsset).

// withSibling adds a detached-signature-like asset to a fixture release.
func withSibling(env *contractEnv, name string, body []byte) {
	env.src.rel.Assets = append(env.src.rel.Assets, Asset{ID: 4, Name: name, State: AssetStateUploaded, Size: int64(len(body))})
	env.src.bodies[4] = body
}

func manifestUpdater(t *testing.T, env *contractEnv, mv ...ManifestVerifier) *Updater {
	t.Helper()
	_, _, plats := fixtureRelease(t, "demo")
	sel, err := NewExactAssetSelector(plats)
	if err != nil {
		t.Fatal(err)
	}
	u, err := New(Config{Source: env.src, Versions: NewStrictVersionPolicy(), Assets: sel, Installer: env.inst,
		Reporter: env.rep, Confirmer: env.conf, Limits: env.lim, ManifestVerifiers: mv})
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// TestManifestVerifierOrder: manifest verifiers run after the manifest is
// fetched and before any binary byte is.
func TestManifestVerifierOrder(t *testing.T) {
	env := newContractEnv(t)
	var seen []string
	mv := ManifestVerifierFunc(func(context.Context, ManifestVerification) error {
		seen = append([]string(nil), env.src.calls...)
		return nil
	})
	req := applyReq()
	req.Yes = true
	if _, err := manifestUpdater(t, env, mv).Run(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(seen, "OpenAsset:3") || slices.Contains(seen, "OpenAsset:2") {
		t.Fatalf("calls before the manifest verifier = %v; want the manifest, not the binary", seen)
	}
	if !slices.Contains(env.src.calls, "OpenAsset:2") {
		t.Fatalf("the binary was never fetched: %v", env.src.calls)
	}

	failing := newContractEnv(t)
	boom := errors.New("bad signature")
	req2 := applyReq()
	req2.Yes = true
	_, err := manifestUpdater(t, failing, ManifestVerifierFunc(func(context.Context, ManifestVerification) error { return boom })).
		Run(context.Background(), req2)
	if !errors.Is(err, boom) || slices.Contains(failing.src.calls, "OpenAsset:2") {
		t.Fatalf("err=%v calls=%v; a failed manifest check must stop before the binary", err, failing.src.calls)
	}
}

// TestManifestVerifierFailureIntegrity: a failure matches ErrIntegrity and
// leaves the target and its directory as they were.
func TestManifestVerifierFailureIntegrity(t *testing.T) {
	_, exe := withTempHome(t)
	inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}})
	if err != nil {
		t.Fatal(err)
	}
	rel, bodies, plats := fixtureRelease(t, "demo")
	sel, _ := NewExactAssetSelector(plats)
	u, err := New(Config{Source: &scriptSource{rel: rel, bodies: bodies}, Versions: NewStrictVersionPolicy(), Assets: sel,
		Installer: inst, Reporter: &recReporter{}, Confirmer: &recConfirmer{}, Limits: DefaultLimits(),
		ManifestVerifiers: []ManifestVerifier{ManifestVerifierFunc(func(context.Context, ManifestVerification) error {
			return errors.New("untrusted manifest")
		})}})
	if err != nil {
		t.Fatal(err)
	}
	req := applyReq()
	req.Yes = true
	if _, err := u.Run(context.Background(), req); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("err = %v, want ErrIntegrity", err)
	}
	if got := readString(t, exe); got != "old-bytes" {
		t.Fatalf("target holds %q", got)
	}
	ents, err := os.ReadDir(filepath.Dir(exe))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".demo.selfupdate-") {
			t.Fatalf("staging left behind: %s", e.Name())
		}
	}
}

// TestManifestVerifierReadsSibling: manifest and binary verifiers can both
// open another release asset, such as a detached signature.
func TestManifestVerifierReadsSibling(t *testing.T) {
	env := newContractEnv(t)
	withSibling(env, "SHA256SUMS.note", []byte("note-bytes"))
	var fromManifest, fromBinary string
	read := func(ctx context.Context, open func(context.Context, string, int64) (io.ReadCloser, error)) (string, error) {
		rc, err := open(ctx, "SHA256SUMS.note", 1024)
		if err != nil {
			return "", err
		}
		defer rc.Close()
		b, err := io.ReadAll(rc)
		return string(b), err
	}
	mv := ManifestVerifierFunc(func(ctx context.Context, v ManifestVerification) (err error) {
		if len(v.Manifest) == 0 {
			return errors.New("no manifest bytes")
		}
		fromManifest, err = read(ctx, v.OpenAsset)
		return err
	})
	u := manifestUpdater(t, env, mv)
	u.verifiers = []Verifier{VerifierFunc(func(ctx context.Context, v Verification) (err error) {
		fromBinary, err = read(ctx, v.OpenAsset)
		return err
	})}
	req := applyReq()
	req.Yes = true
	if _, err := u.Run(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if fromManifest != "note-bytes" || fromBinary != "note-bytes" {
		t.Fatalf("manifest verifier read %q, binary verifier read %q", fromManifest, fromBinary)
	}
}

// TestOpenAssetEnforcesSize: bodies must match their advertised size and
// digest; names must exist exactly once; the limit applies.
func TestOpenAssetEnforcesSize(t *testing.T) {
	env := newContractEnv(t)
	u := env.build(t)
	body := []byte("0123456789")
	rel := Release{ID: 1, Tag: "v1.1.0", Assets: []Asset{
		{ID: 10, Name: "ok", State: AssetStateUploaded, Size: 10},
		{ID: 11, Name: "short", State: AssetStateUploaded, Size: 11},
		{ID: 12, Name: "long", State: AssetStateUploaded, Size: 9},
		{ID: 13, Name: "digest", State: AssetStateUploaded, Size: 10, Digest: "sha256:" + strings.Repeat("0", 64)},
		{ID: 14, Name: "dup", State: AssetStateUploaded, Size: 10},
		{ID: 15, Name: "dup", State: AssetStateUploaded, Size: 10},
	}}
	for id := int64(10); id <= 15; id++ {
		env.src.bodies[id] = body
	}
	r, err := u.newRun(runScope{})
	if err != nil {
		t.Fatal(err)
	}
	open := r.openAsset(rel)
	readAll := func(name string, limit int64) error {
		rc, err := open(context.Background(), name, limit)
		if err != nil {
			return err
		}
		defer rc.Close()
		_, err = io.ReadAll(rc)
		return err
	}
	if err := readAll("ok", 1024); err != nil {
		t.Fatalf("ok: %v", err)
	}
	for name, why := range map[string]string{
		"short": "shorter than advertised", "long": "longer than advertised", "digest": "github digest",
	} {
		err := readAll(name, 1024)
		if !errors.Is(err, ErrIntegrity) || !strings.Contains(err.Error(), why) {
			t.Errorf("%s: %v, want ErrIntegrity saying %q", name, err, why)
		}
	}
	for name, limit := range map[string]int64{"ok": 5, "missing": 1024, "dup": 1024} {
		if err := readAll(name, limit); err == nil {
			t.Errorf("%s with limit %d was opened", name, limit)
		}
	}
	if err := readAll("ok", 0); err == nil {
		t.Error("a zero limit was accepted")
	}
}

func TestNewNilManifestVerifier(t *testing.T) {
	env := newContractEnv(t)
	_, _, plats := fixtureRelease(t, "demo")
	sel, _ := NewExactAssetSelector(plats)
	var typedNil ManifestVerifierFunc
	for _, mv := range []ManifestVerifier{nil, typedNil} {
		if _, err := New(Config{Source: env.src, Versions: NewStrictVersionPolicy(), Assets: sel, Installer: env.inst,
			Reporter: env.rep, Confirmer: env.conf, Limits: env.lim, ManifestVerifiers: []ManifestVerifier{mv}}); err == nil {
			t.Errorf("manifest verifier %#v accepted", mv)
		}
	}
}

// TestOpenAssetChecksDigestAtSize: a verifier that reads exactly Size bytes,
// and so never sees io.EOF, still gets ErrIntegrity for a body that does not
// match its digest, and a matching body still reads clean
// (0010-MADR A1).
func TestOpenAssetChecksDigestAtSize(t *testing.T) {
	env := newContractEnv(t)
	u := env.build(t)
	body := []byte("0123456789")
	good := sha256.Sum256(body)
	rel := Release{ID: 1, Tag: "v1.1.0", Assets: []Asset{
		{ID: 20, Name: "bad", State: AssetStateUploaded, Size: 10, Digest: "sha256:" + strings.Repeat("0", 64)},
		{ID: 21, Name: "good", State: AssetStateUploaded, Size: 10, Digest: "sha256:" + hex.EncodeToString(good[:])},
	}}
	env.src.bodies[20] = body
	env.src.bodies[21] = body
	r, err := u.newRun(runScope{})
	if err != nil {
		t.Fatal(err)
	}
	open := r.openAsset(rel)
	reads := map[string]func(io.Reader) error{
		"ReadFull": func(rc io.Reader) error {
			_, err := io.ReadFull(rc, make([]byte, 10))
			return err
		},
		"CopyN": func(rc io.Reader) error {
			_, err := io.CopyN(io.Discard, rc, 10)
			return err
		},
		"a byte at a time": func(rc io.Reader) error {
			b := make([]byte, 1)
			for range 10 {
				if _, err := io.ReadFull(rc, b); err != nil {
					return err
				}
			}
			return nil
		},
		"ReadAll": func(rc io.Reader) error {
			_, err := io.ReadAll(rc)
			return err
		},
	}
	for how, read := range reads {
		for name, wantBad := range map[string]bool{"bad": true, "good": false} {
			rc, err := open(context.Background(), name, 1024)
			if err != nil {
				t.Fatal(err)
			}
			err = read(rc)
			if cerr := rc.Close(); cerr != nil {
				t.Fatalf("%s %s: close: %v", how, name, cerr)
			}
			if got := errors.Is(err, ErrIntegrity); got != wantBad {
				t.Errorf("%s of %s: err = %v, want ErrIntegrity %v", how, name, err, wantBad)
			}
		}
	}
}

// TestVerifierFailureIsIntegrity: a binary verifier's failure is an
// ErrIntegrity failure, as a manifest verifier's is, and EventFailed says so
// (0010-MADR A8).
func TestVerifierFailureIsIntegrity(t *testing.T) {
	env := newContractEnv(t)
	u := env.build(t)
	u.verifiers = []Verifier{VerifierFunc(func(context.Context, Verification) error {
		return errors.New("signature does not verify")
	})}
	req := applyReq()
	req.Yes = true
	_, err := u.Run(context.Background(), req)
	if !errors.Is(err, ErrIntegrity) || !strings.Contains(err.Error(), "signature does not verify") {
		t.Fatalf("err = %v, want ErrIntegrity carrying the verifier's error", err)
	}
	last := env.rep.events[len(env.rep.events)-1]
	if last.Kind != EventFailed || last.Detail != "integrity" {
		t.Fatalf("last event %v %q, want failed integrity", last.Kind, last.Detail)
	}
}
