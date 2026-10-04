package selfupdate_test

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/selfupdatetest"
)

var update = flag.Bool("update", false, "rewrite testdata/golden from the current reporter output")

type goldenCase struct {
	name     string
	req      selfupdate.Request
	answers  []bool
	release  func() selfupdatetest.ReleaseSpec
	wantErr  error
	wantExit int
	// closeErr, when set, fails the session's Close after the install.
	closeErr error
}

// closeFailInstaller's sessions fail Close, after the work is done.
type closeFailInstaller struct {
	selfupdate.Installer
	err error
}

func (i closeFailInstaller) Begin(ctx context.Context, t selfupdate.Target) (selfupdate.InstallSession, error) {
	sess, err := i.Installer.Begin(ctx, t)
	if err != nil {
		return nil, err
	}
	return closeFailSession{sess, i.err}, nil
}

type closeFailSession struct {
	selfupdate.InstallSession
	err error
}

func (s closeFailSession) Close() error {
	return errors.Join(s.InstallSession.Close(), s.err)
}

func goldenRelease() selfupdatetest.ReleaseSpec {
	return selfupdatetest.NewRelease("demo", "v1.1.0", []selfupdate.Platform{goldenPlatform}, releaseBody("v1.1.0"))
}

// tamperedRelease serves a binary whose bytes differ from its SHA256SUMS
// entry.
func tamperedRelease() selfupdatetest.ReleaseSpec {
	spec := goldenRelease()
	spec.Assets[0].Body = []byte("tampered\n")
	return spec
}

func goldenCases() []goldenCase {
	req := func(mod func(*selfupdate.Request)) selfupdate.Request {
		r := selfupdate.Request{Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: selfupdate.ReleaseBuild, Platform: goldenPlatform}
		mod(&r)
		return r
	}
	return []goldenCase{
		{name: "upgrade", req: req(func(r *selfupdate.Request) { r.Yes = true }), release: goldenRelease},
		{name: "check", req: req(func(r *selfupdate.Request) { r.CheckOnly = true }), release: goldenRelease,
			wantErr: selfupdate.ErrUpdateAvailable, wantExit: 10},
		{name: "local", req: req(func(r *selfupdate.Request) {
			r.CurrentBuild, r.CurrentVersion, r.CheckOnly = selfupdate.LocalBuild, "dev", true
		}), release: goldenRelease, wantErr: selfupdate.ErrUpdateAvailable, wantExit: 10},
		{name: "failed-integrity", req: req(func(r *selfupdate.Request) { r.Yes = true }), release: tamperedRelease,
			wantErr: selfupdate.ErrIntegrity, wantExit: 1},
		{name: "declined", req: req(func(*selfupdate.Request) {}), answers: []bool{false}, release: goldenRelease},
		{name: "dry-run", req: req(func(r *selfupdate.Request) { r.DryRun = true }), release: goldenRelease},
		// A late error is a warning after complete (0010-MADR Q3).
		{name: "warning", req: req(func(r *selfupdate.Request) { r.Yes = true }), release: goldenRelease,
			closeErr: errors.New("unlock failed")},
	}
}

// TestGolden runs each case through a full Run with FakeSource and compares
// the text and JSON Lines reporter output with testdata/golden byte for
// byte. go test -run TestGolden -update rewrites the files.
func TestGolden(t *testing.T) {
	for _, c := range goldenCases() {
		t.Run(c.name, func(t *testing.T) {
			exe, policy := tempTarget(t)
			standalone, err := selfupdate.NewStandaloneInstaller(selfupdate.InstallOptions{TargetPolicy: policy})
			if err != nil {
				t.Fatal(err)
			}
			var inst selfupdate.Installer = standalone
			if c.closeErr != nil {
				inst = closeFailInstaller{standalone, c.closeErr}
			}
			sel, err := selfupdate.NewExactAssetSelector([]selfupdate.Platform{goldenPlatform})
			if err != nil {
				t.Fatal(err)
			}
			var text, jsonl bytes.Buffer
			u, err := selfupdate.New(selfupdate.Config{
				Source:   selfupdatetest.NewFakeSource("v1.1.0", c.release()),
				Versions: selfupdate.NewStrictVersionPolicy(), Assets: sel, Installer: inst,
				Reporter:  selfupdate.MultiReporter(selfupdate.NewTextReporter(&text), selfupdate.NewJSONReporter(&jsonl)),
				Confirmer: &selfupdatetest.ScriptedConfirmer{Answers: c.answers},
				Limits:    selfupdate.DefaultLimits(),
			})
			if err != nil {
				t.Fatal(err)
			}
			res, err := u.Run(context.Background(), c.req)
			if (c.wantErr == nil) != (err == nil) || (c.wantErr != nil && !errors.Is(err, c.wantErr)) {
				t.Fatalf("err = %v, want %v", err, c.wantErr)
			}
			if code := selfupdate.ExitCode(res, err); code != c.wantExit {
				t.Fatalf("ExitCode = %d, want %d", code, c.wantExit)
			}
			if c.name != "upgrade" && c.name != "warning" {
				if got, rerr := os.ReadFile(exe); rerr != nil || string(got) != "old-bytes" {
					t.Fatalf("target changed: %q, %v", got, rerr)
				}
			}
			compareGolden(t, "text-"+c.name+".golden", text.Bytes())
			compareGolden(t, "jsonl-"+c.name+".golden", jsonl.Bytes())
		})
	}
}

func compareGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test -run TestGolden -update to create it)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs from the reporter output:\n--- got\n%s--- want\n%s", path, got, want)
	}
}
