package selfupdate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Tests for docs/decisions/0012-PLAN-archive-assets-and-macos-codesign.md
// U1: the extract stage (0012-MADR §2, §7).

// Selection stays comparable (0012-MADR §2).
var _ = Selection{} == Selection{Packed: true}

// packHeader marks the stand-in archive format these tests use: the
// program's bytes after a fixed prefix.
const packHeader = "PACK:"

// packedRelease is fixtureRelease with the binary asset replaced by an
// "archive" holding program.
func packedRelease(t *testing.T, product string, program []byte) (Release, map[int64][]byte, string) {
	t.Helper()
	plat := runningPlatform()
	archive := append([]byte(packHeader), program...)
	sum := sha256.Sum256(archive)
	hexsum := hex.EncodeToString(sum[:])
	name := fmt.Sprintf("%s-%s-%s.pack", product, plat.OS, plat.Arch)
	manifest := []byte(hexsum + "  " + name + "\n")
	rel := Release{
		ID: 1, Tag: "v1.1.0", URL: "https://example.invalid/v1.1.0", Immutable: true,
		Assets: []Asset{
			{ID: 2, Name: name, State: "uploaded", Size: int64(len(archive)), Digest: "sha256:" + hexsum},
			{ID: 3, Name: manifestAssetName, State: "uploaded", Size: int64(len(manifest))},
		},
	}
	return rel, map[int64][]byte{2: archive, 3: manifest}, name
}

// packSelector selects the named asset and SHA256SUMS, and marks the
// selection Packed as packed says.
type packSelector struct {
	name   string
	packed bool
}

func (s packSelector) Select(rel Release, _ string, _ Platform) (Selection, error) {
	sel := Selection{ManifestName: s.name, Packed: s.packed}
	for _, a := range rel.Assets {
		switch a.Name {
		case s.name:
			sel.Binary = a
		case manifestAssetName:
			sel.Manifest = a
		}
	}
	if sel.Binary.Name == "" || sel.Manifest.Name == "" {
		return Selection{}, errors.New("packSelector: asset missing")
	}
	return sel, nil
}

// recUnpacker strips packHeader from the archive into the program file,
// and records each request and what was on disk when it ran.
type recUnpacker struct {
	reqs       []UnpackRequest
	archiveWas []byte
	leftovers  []string // staging names in the target directory during Unpack
	write      func(req UnpackRequest, program []byte) error
	err        error
}

func (u *recUnpacker) Unpack(_ context.Context, req UnpackRequest) error {
	u.reqs = append(u.reqs, req)
	if u.err != nil {
		return u.err
	}
	b, err := os.ReadFile(req.Archive)
	if err != nil {
		return err
	}
	u.archiveWas = b
	dir := filepath.Dir(req.Program)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".selfupdate-") {
			u.leftovers = append(u.leftovers, e.Name())
		}
	}
	program, ok := bytes.CutPrefix(b, []byte(packHeader))
	if !ok {
		return errors.New("recUnpacker: not a pack")
	}
	if u.write != nil {
		return u.write(req, program)
	}
	return os.WriteFile(req.Program, program, 0o600)
}

type packEnv struct {
	src  *scriptSource
	exe  string
	name string
	rep  *recReporter
	unp  *recUnpacker
	cfg  Config
}

func newPackEnv(t *testing.T, program []byte) *packEnv {
	t.Helper()
	rel, bodies, name := packedRelease(t, "demo", program)
	_, exe := withTempHome(t)
	inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}})
	if err != nil {
		t.Fatal(err)
	}
	env := &packEnv{
		src: &scriptSource{rel: rel, bodies: bodies}, exe: exe, name: name,
		rep: &recReporter{}, unp: &recUnpacker{},
	}
	env.cfg = Config{
		Source: env.src, Versions: NewStrictVersionPolicy(), Assets: packSelector{name: name, packed: true},
		Unpacker: env.unp, Installer: inst, Reporter: env.rep, Confirmer: &recConfirmer{ok: true},
		Limits: DefaultLimits(),
	}
	return env
}

func (e *packEnv) run(t *testing.T, req Request) (Result, error) {
	t.Helper()
	u, err := New(e.cfg)
	if err != nil {
		t.Fatal(err)
	}
	return u.Run(context.Background(), req)
}

func sha(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// TestRunEventOrderWithUnpacker: Verified, then Unpacking, then the
// transform when there is one; the program, not the archive, is installed,
// and the digests are the archive's and the program's.
func TestRunEventOrderWithUnpacker(t *testing.T) {
	program := []byte("program-bytes")
	for _, withTransform := range []bool{false, true} {
		t.Run(fmt.Sprintf("transform=%v", withTransform), func(t *testing.T) {
			env := newPackEnv(t, program)
			want := []EventKind{
				EventResolvingTarget, EventFetchingRelease, EventSelected, EventDownloadingManifest,
				EventDownloadingBinary, EventVerified, EventUnpacking, EventInstalling, EventComplete,
			}
			installed := program
			if withTransform {
				env.cfg.Transformer = growTransformer{extra: []byte("-signed")}
				want = slices.Insert(want, 7, EventTransforming)
				installed = append(slices.Clone(program), "-signed"...)
			}
			res, err := env.run(t, yesReq())
			if err != nil || !res.Applied {
				t.Fatalf("res=%+v err=%v", res, err)
			}
			if !slices.Equal(env.rep.kinds, want) {
				t.Fatalf("events = %v\nwant     %v", env.rep.kinds, want)
			}
			got, err := os.ReadFile(env.exe)
			if err != nil || !bytes.Equal(got, installed) {
				t.Fatalf("installed %q (%v), want %q", got, err, installed)
			}
			archive := env.src.bodies[2]
			if res.ReleaseDigest != sha(archive) || res.InstalledDigest != sha(installed) {
				t.Fatalf("release digest %s installed digest %s; want the archive's and the program's", res.ReleaseDigest, res.InstalledDigest)
			}
			if res.AssetName != env.name {
				t.Fatalf("asset %q, want the archive %q", res.AssetName, env.name)
			}
			for _, ev := range env.rep.events {
				if ev.Kind == EventUnpacking && (ev.Asset != env.name || ev.Product != "demo" || ev.Target != "v1.1.0") {
					t.Fatalf("unpacking event %+v", ev)
				}
			}
		})
	}
}

// TestUnpackRequest: the unpacker gets the verified archive and a second,
// empty staging path, both the session's, with the executable limit.
func TestUnpackRequest(t *testing.T) {
	env := newPackEnv(t, []byte("program-bytes"))
	if _, err := env.run(t, yesReq()); err != nil {
		t.Fatal(err)
	}
	if len(env.unp.reqs) != 1 {
		t.Fatalf("unpacked %d times", len(env.unp.reqs))
	}
	r := env.unp.reqs[0]
	dir, err := filepath.EvalSymlinks(filepath.Dir(env.exe))
	if err != nil {
		t.Fatal(err)
	}
	if r.Product != "demo" || r.Platform != runningPlatform() || r.AssetName != env.name ||
		r.Limit != DefaultLimits().Executable || r.Archive == r.Program ||
		filepath.Dir(r.Archive) != dir || filepath.Dir(r.Program) != dir {
		t.Fatalf("request %+v", r)
	}
	if !bytes.Equal(env.unp.archiveWas, env.src.bodies[2]) {
		t.Fatalf("the archive on disk was %q", env.unp.archiveWas)
	}
	// Both staging files are named so that a crash leaves nothing the next
	// session's sweep would miss (0010-MADR Q6).
	if len(env.unp.leftovers) != 2 {
		t.Fatalf("staging files during Unpack: %v", env.unp.leftovers)
	}
	for _, n := range env.unp.leftovers {
		if !isLeftover("demo", n) {
			t.Fatalf("%s is not swept after a crash", n)
		}
	}
	assertNoStaging(t, filepath.Dir(env.exe))
}

func assertNoStaging(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".selfupdate-") {
			t.Fatalf("staging left behind: %s", e.Name())
		}
	}
}

// TestDryRunUnpacks: a dry run unpacks, installs nothing, and leaves no
// staging.
func TestDryRunUnpacks(t *testing.T) {
	program := []byte("program-bytes")
	env := newPackEnv(t, program)
	req := yesReq()
	req.DryRun = true
	res, err := env.run(t, req)
	if err != nil || res.Applied {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if len(env.unp.reqs) != 1 || res.InstalledDigest != sha(program) {
		t.Fatalf("unpacked %d times, installed digest %s", len(env.unp.reqs), res.InstalledDigest)
	}
	if got, _ := os.ReadFile(env.exe); string(got) != "old-bytes" {
		t.Fatalf("a dry run replaced the target: %q", got)
	}
	last := env.rep.events[len(env.rep.events)-1]
	if last.Kind != EventComplete || last.Detail != "dry-run" {
		t.Fatalf("last event %+v", last)
	}
	assertNoStaging(t, filepath.Dir(env.exe))
}

// TestUnpackFailures: an unpacker error, and a program that is a symlink,
// too large, or not the session's, each fail the run before Install and
// leave no staging.
func TestUnpackFailures(t *testing.T) {
	cases := []struct {
		name  string
		setup func(e *packEnv)
		want  string
	}{
		{"unpacker error", func(e *packEnv) { e.unp.err = errors.New("bad archive") }, "bad archive"},
		{"symlink", func(e *packEnv) {
			e.unp.write = func(req UnpackRequest, _ []byte) error {
				if err := os.Remove(req.Program); err != nil {
					return err
				}
				return os.Symlink(e.exe, req.Program)
			}
		}, "is not a regular file"},
		{"over the limit", func(e *packEnv) {
			e.cfg.Limits.Executable = 64
			e.unp.write = func(req UnpackRequest, _ []byte) error {
				return os.WriteFile(req.Program, bytes.Repeat([]byte("x"), 65), 0o600)
			}
		}, "exceeds executable limit"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env := newPackEnv(t, []byte("program-bytes"))
			c.setup(env)
			_, err := env.run(t, yesReq())
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want %q", err, c.want)
			}
			if got, _ := os.ReadFile(env.exe); string(got) != "old-bytes" {
				t.Fatalf("the target changed: %q", got)
			}
			if slices.Contains(env.rep.kinds, EventInstalling) {
				t.Fatalf("installing after a failed unpack: %v", env.rep.kinds)
			}
			assertNoStaging(t, filepath.Dir(env.exe))
		})
	}
}

// notOwnerInstaller's sessions are logSessions with Owns hidden.
type notOwnerInstaller struct{ *logInstaller }

type notOwnerSession struct{ InstallSession }

func (i notOwnerInstaller) Begin(ctx context.Context, t Target) (InstallSession, error) {
	sess, err := i.logInstaller.Begin(ctx, t)
	if err != nil {
		return nil, err
	}
	return notOwnerSession{InstallSession: sess}, nil
}

// TestUnpackRequiresStagingOwner: a session that is not a StagingOwner
// cannot unpack, as it cannot transform (0004-MADR G7).
func TestUnpackRequiresStagingOwner(t *testing.T) {
	env := newContractEnv(t)
	rel, bodies, name := packedRelease(t, "demo", []byte("program-bytes"))
	env.src.rel, env.src.bodies = rel, bodies
	u, err := New(Config{
		Source: env.src, Versions: NewStrictVersionPolicy(), Assets: packSelector{name: name, packed: true},
		Unpacker:  &recUnpacker{},
		Installer: notOwnerInstaller{logInstaller: env.inst},
		Reporter:  env.rep, Confirmer: env.conf, Limits: env.lim,
	})
	if err != nil {
		t.Fatal(err)
	}
	req := applyReq()
	req.Yes = true
	_, err = u.Run(context.Background(), req)
	if err == nil || !strings.Contains(err.Error(), "is not owned by the session") {
		t.Fatalf("Run = %v, want the ownership refusal", err)
	}
	if slices.Contains(*env.log, "Install") {
		t.Fatalf("Install ran: %v", *env.log)
	}
}

// TestPackedSelectionNeedsUnpacker: a selector and an unpacker that
// disagree fail before any asset body is opened, in a check as in an
// apply.
func TestPackedSelectionNeedsUnpacker(t *testing.T) {
	for _, c := range []struct {
		name     string
		packed   bool
		unpacker Unpacker
		want     string
	}{
		{"archive without unpacker", true, nil, "no Unpacker is configured"},
		{"unpacker without archive", false, &recUnpacker{}, "is not marked as an archive"},
	} {
		for _, check := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/check=%v", c.name, check), func(t *testing.T) {
				env := newPackEnv(t, []byte("program-bytes"))
				env.cfg.Assets = packSelector{name: env.name, packed: c.packed}
				env.cfg.Unpacker = c.unpacker
				req := yesReq()
				if check {
					req.Yes, req.CheckOnly = false, true
				}
				_, err := env.run(t, req)
				if err == nil || !strings.Contains(err.Error(), c.want) {
					t.Fatalf("err = %v, want %q", err, c.want)
				}
				for _, call := range env.src.calls {
					if strings.HasPrefix(call, "OpenAsset") {
						t.Fatalf("opened an asset first: %v", env.src.calls)
					}
				}
			})
		}
	}
}

// TestNewRefusesUnpackerCombinations: New refuses an Unpacker with the
// exact raw-binary selector or with the image verifier, and a typed nil.
func TestNewRefusesUnpackerCombinations(t *testing.T) {
	env := newPackEnv(t, []byte("program-bytes"))
	exact, err := NewExactAssetSelector([]Platform{runningPlatform()})
	if err != nil {
		t.Fatal(err)
	}
	image, err := NewImageVerifier(Platform{})
	if err != nil {
		t.Fatal(err)
	}
	var typedNil *recUnpacker
	for _, c := range []struct {
		name string
		edit func(*Config)
		want string
	}{
		{"exact selector", func(c *Config) { c.Assets = exact }, "NewExactAssetSelector"},
		{"image verifier", func(c *Config) { c.Verifiers = []Verifier{image} }, "NewImageVerifier"},
		{"typed nil", func(c *Config) { c.Unpacker = typedNil }, "unpacker is a typed nil"},
	} {
		cfg := env.cfg
		c.edit(&cfg)
		if _, err := New(cfg); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: New = %v, want %q", c.name, err, c.want)
		}
	}
	// Without an Unpacker, both are as valid as ever.
	cfg := env.cfg
	cfg.Unpacker, cfg.Assets, cfg.Verifiers = nil, exact, []Verifier{image}
	if _, err := New(cfg); err != nil {
		t.Fatalf("New without an Unpacker: %v", err)
	}
}

// TestEventUnpacking: the new kind keeps every earlier number and has its
// name.
func TestEventUnpacking(t *testing.T) {
	if EventUnpacking != EventWarning+1 {
		t.Fatalf("EventUnpacking = %d, want %d", EventUnpacking, EventWarning+1)
	}
	if EventUnpacking.String() != "unpacking" {
		t.Fatalf("String = %q", EventUnpacking.String())
	}
}

// TestUnpackerFunc adapts a function.
func TestUnpackerFunc(t *testing.T) {
	var got UnpackRequest
	f := UnpackerFunc(func(_ context.Context, r UnpackRequest) error { got = r; return nil })
	if err := f.Unpack(context.Background(), UnpackRequest{Product: "demo"}); err != nil || got.Product != "demo" {
		t.Fatalf("got %+v err %v", got, err)
	}
}

// TestChainTransformers: in order, on the same request; the first error
// stops the chain; the coordinator rehashes once after it.
func TestChainTransformers(t *testing.T) {
	var order []string
	step := func(name string, err error) Transformer {
		return TransformerFunc(func(_ context.Context, r TransformRequest) error {
			order = append(order, name+":"+r.Path)
			return err
		})
	}
	chain, err := ChainTransformers(step("a", nil), step("b", nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := chain.Transform(context.Background(), TransformRequest{Path: "/p"}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(order, []string{"a:/p", "b:/p"}) {
		t.Fatalf("order %v", order)
	}
	order = nil
	boom := errors.New("boom")
	chain, _ = ChainTransformers(step("a", boom), step("b", nil))
	if err := chain.Transform(context.Background(), TransformRequest{Path: "/p"}); !errors.Is(err, boom) {
		t.Fatalf("err %v", err)
	}
	if !slices.Equal(order, []string{"a:/p"}) {
		t.Fatalf("ran past the error: %v", order)
	}
	var typedNil *nilPtrTransformer
	for name, ts := range map[string][]Transformer{
		"empty": nil, "nil": {step("a", nil), nil}, "typed nil": {typedNil},
	} {
		if _, err := ChainTransformers(ts...); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// TestRunWithChainedTransformers: two transforms in a chain, through the
// Updater, install both changes.
func TestRunWithChainedTransformers(t *testing.T) {
	env := newPackEnv(t, []byte("program-bytes"))
	chain, err := ChainTransformers(growTransformer{extra: []byte("-a")}, growTransformer{extra: []byte("-b")})
	if err != nil {
		t.Fatal(err)
	}
	env.cfg.Transformer = chain
	res, err := env.run(t, yesReq())
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("program-bytes-a-b")
	if got, _ := os.ReadFile(env.exe); !bytes.Equal(got, want) || res.InstalledDigest != sha(want) {
		t.Fatalf("installed %q digest %s", got, res.InstalledDigest)
	}
}
