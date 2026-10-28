package selfupdate

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/mod/semver"
)

// Tests for docs/decisions/0004-PLAN-v1-1-0-core-api.md Step 2.

// prereleasePolicy accepts any valid semver tag, prereleases included.
type prereleasePolicy struct{}

func (prereleasePolicy) Validate(tag string) error {
	if !semver.IsValid(tag) {
		return errors.New("not semver")
	}
	return nil
}

func (p prereleasePolicy) Compare(a, b string) (int, error) {
	return semver.Compare(a, b), nil
}

func runWith(t *testing.T, versions VersionPolicy, req Request) (Result, error) {
	t.Helper()
	env := newContractEnv(t)
	_, _, plats := fixtureRelease(t, "demo")
	sel, err := NewExactAssetSelector(plats)
	if err != nil {
		t.Fatal(err)
	}
	u, err := New(Config{
		Source: env.src, Versions: versions, Assets: sel,
		Installer: env.inst, Reporter: env.rep, Confirmer: env.conf, Limits: env.lim,
	})
	if err != nil {
		t.Fatal(err)
	}
	return u.Run(context.Background(), req)
}

// TestExportedSentinels: the two outcomes a CLI must explain are exported
// (0004-MADR G4).
func TestExportedSentinels(t *testing.T) {
	t.Parallel()
	_, err := runWith(t, NewStrictVersionPolicy(), Request{
		Product: "demo", CurrentVersion: "dev", CurrentBuild: LocalBuild, Yes: true,
	})
	if !errors.Is(err, ErrForceRequired) {
		t.Fatalf("local build without --force: %v", err)
	}
	_, err = runWith(t, NewStrictVersionPolicy(), Request{
		Product: "demo", CurrentVersion: "v2.0.0", CurrentBuild: ReleaseBuild, Yes: true,
	})
	if !errors.Is(err, ErrLatestOlder) {
		t.Fatalf("latest older than running: %v", err)
	}
}

// TestValidateRequestUsesConfiguredPolicy: request versions are validated
// with Config.Versions, not a hardcoded strict policy (0004-MADR G2).
func TestValidateRequestUsesConfiguredPolicy(t *testing.T) {
	t.Parallel()
	req := Request{Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: ReleaseBuild, TargetVersion: "v1.2.3-rc.1"}
	if err := validateRequest(req, prereleasePolicy{}, runtimePlatform()); err != nil {
		t.Fatalf("configured policy accepts the tag, yet: %v", err)
	}
	if err := validateRequest(req, NewStrictVersionPolicy(), runtimePlatform()); err == nil {
		t.Fatal("the strict policy accepted a prerelease tag")
	}
	_, err := runWith(t, prereleasePolicy{}, req)
	if err == nil || strings.Contains(err.Error(), "not a strict stable tag") {
		t.Fatalf("Run validated with the strict policy: %v", err)
	}
}

// TestParseSHA256SUMSExported: the exported parser gives each parity
// fixture's recorded verdict (its expect file).
func TestParseSHA256SUMSExported(t *testing.T) {
	t.Parallel()
	dirs, err := os.ReadDir(filepath.Join("testdata", "manifest-parity"))
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) < 20 {
		t.Fatalf("only %d parity fixtures", len(dirs))
	}
	for _, d := range dirs {
		data, err := os.ReadFile(filepath.Join("testdata", "manifest-parity", d.Name(), "SHA256SUMS"))
		if err != nil {
			t.Fatal(err)
		}
		expect, err := os.ReadFile(filepath.Join("testdata", "manifest-parity", d.Name(), "expect"))
		if err != nil {
			t.Fatal(err)
		}
		got, gotErr := ParseSHA256SUMS(data)
		accepted := gotErr == nil && len(got) > 0
		if want := strings.TrimSpace(string(expect)) == "accept"; accepted != want {
			t.Errorf("%s: accepted=%v (%v), expect %q", d.Name(), accepted, gotErr, strings.TrimSpace(string(expect)))
		}
	}
}

func TestExactAssetName(t *testing.T) {
	t.Parallel()
	for p, want := range map[Platform]string{
		{OS: "linux", Arch: "amd64"}:   "demo-linux-amd64",
		{OS: "windows", Arch: "arm64"}: "demo-windows-arm64.exe",
	} {
		if got := ExactAssetName("demo", p); got != want {
			t.Errorf("%v: %q, want %q", p, got, want)
		}
	}
	if AssetStateUploaded != "uploaded" {
		t.Fatalf("AssetStateUploaded = %q", AssetStateUploaded)
	}
}

// TestMultiReporter: order, every reporter called after a failure, errors
// joined, nil and typed-nil entries dropped.
func TestMultiReporter(t *testing.T) {
	t.Parallel()
	var order []string
	first := errors.New("first")
	third := errors.New("third")
	rec := func(name string, err error) Reporter {
		return ReporterFunc(func(context.Context, Event) error {
			order = append(order, name)
			return err
		})
	}
	var typedNil *nilPtrReporter
	m := MultiReporter(rec("a", first), nil, typedNil, rec("b", nil), rec("c", third))
	err := m.Report(context.Background(), Event{Kind: EventSelected})
	if strings.Join(order, ",") != "a,b,c" {
		t.Fatalf("order = %v", order)
	}
	if !errors.Is(err, first) || !errors.Is(err, third) {
		t.Fatalf("err = %v, want both errors joined", err)
	}
	if err := MultiReporter().Report(context.Background(), Event{}); err != nil {
		t.Fatalf("empty MultiReporter: %v", err)
	}
	if err := DiscardReporter().Report(context.Background(), Event{}); err != nil {
		t.Fatalf("DiscardReporter: %v", err)
	}
}

// TestFuncAdapters: each adapter forwards its arguments and its result.
func TestFuncAdapters(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	sentinel := errors.New("sentinel")
	var gotEv Event
	if err := ReporterFunc(func(_ context.Context, ev Event) error { gotEv = ev; return sentinel }).
		Report(ctx, Event{Kind: EventVerified}); !errors.Is(err, sentinel) || gotEv.Kind != EventVerified {
		t.Fatalf("ReporterFunc: %v %v", err, gotEv)
	}
	ok, err := ConfirmerFunc(func(_ context.Context, p Prompt) (bool, error) { return p.Product == "demo", sentinel }).
		Confirm(ctx, Prompt{Product: "demo"})
	if !ok || !errors.Is(err, sentinel) {
		t.Fatalf("ConfirmerFunc: %v %v", ok, err)
	}
	if err := VerifierFunc(func(_ context.Context, v Verification) error {
		if v.Product != "demo" {
			return errors.New("lost product")
		}
		return sentinel
	}).Verify(ctx, Verification{Product: "demo"}); !errors.Is(err, sentinel) {
		t.Fatalf("VerifierFunc: %v", err)
	}
	if err := TransformerFunc(func(_ context.Context, r TransformRequest) error {
		if r.Path != "p" {
			return errors.New("lost path")
		}
		return sentinel
	}).Transform(ctx, TransformRequest{Path: "p"}); !errors.Is(err, sentinel) {
		t.Fatalf("TransformerFunc: %v", err)
	}
}

func TestNonInteractiveConfirmer(t *testing.T) {
	t.Parallel()
	ok, err := NonInteractiveConfirmer().Confirm(context.Background(), upgradePrompt)
	if ok || !errors.Is(err, ErrConfirmationRequired) {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

// TestPromptConfirmer: answers, refusals, and the host's next line left
// unread (0004-MADR G6, R2).
func TestPromptConfirmer(t *testing.T) {
	defer checkNoLeak(t)()
	ctx := context.Background()
	for input, want := range map[string]bool{"y\n": true, "yes\n": true, "n\n": false, "": false} {
		var out strings.Builder
		ok, err := NewPromptConfirmer(strings.NewReader(input), &out, true).Confirm(ctx, upgradePrompt)
		if err != nil || ok != want {
			t.Errorf("input %q: ok=%v err=%v", input, ok, err)
		}
		if !strings.Contains(out.String(), "upgrade demo from v1.0.0 to v1.1.0? [y/N]") {
			t.Errorf("input %q: prompt %q", input, out.String())
		}
	}
	if _, err := NewPromptConfirmer(strings.NewReader("y\n"), io.Discard, false).Confirm(ctx, upgradePrompt); !errors.Is(err, ErrConfirmationRequired) {
		t.Errorf("interactive=false: %v", err)
	}
	if _, err := NewPromptConfirmer(nil, io.Discard, true).Confirm(ctx, upgradePrompt); !errors.Is(err, ErrConfirmationRequired) {
		t.Errorf("nil in: %v", err)
	}
	var nilFile *os.File
	if _, err := NewPromptConfirmer(nilFile, io.Discard, true).Confirm(ctx, upgradePrompt); !errors.Is(err, ErrConfirmationRequired) {
		t.Errorf("typed-nil in: %v", err)
	}
	if _, err := NewPromptConfirmer(strings.NewReader("y\n"), nil, true).Confirm(ctx, upgradePrompt); err == nil {
		t.Error("nil out accepted")
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})
	if _, err := io.WriteString(w, "y\nhost-command\n"); err != nil {
		t.Fatal(err)
	}
	if ok, err := NewPromptConfirmer(r, io.Discard, true).Confirm(ctx, upgradePrompt); err != nil || !ok {
		t.Fatalf("pipe: ok=%v err=%v", ok, err)
	}
	got := make(chan string, 1)
	go func() {
		buf := make([]byte, 64)
		n, _ := r.Read(buf)
		got <- string(buf[:n])
	}()
	select {
	case s := <-got:
		if s != "host-command\n" {
			t.Fatalf("host read %q", s)
		}
	case <-time.After(2 * time.Second):
		_ = w.Close()
		t.Fatal("the host's input was consumed by the confirmer")
	}
}
