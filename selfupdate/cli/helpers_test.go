package cli

import (
	"context"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/selfupdatetest"
)

var update = flag.Bool("update", false, "rewrite the golden files under testdata/golden")

// here is the running platform, the only one the fixtures publish.
var here = selfupdate.Platform{OS: runtime.GOOS, Arch: runtime.GOARCH}

// golden compares got with testdata/golden/name, or rewrites it under
// -update.
func golden(t *testing.T, name, got string) {
	t.Helper()
	goldenIn(t, "golden", name, got)
}

// goldenIn is golden for testdata/dir.
func goldenIn(t *testing.T, dir, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", dir, name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path) //nolint:gosec // a fixed testdata path
	if err != nil {
		t.Fatalf("read golden %s: %v (run with -update to create it)", path, err)
	}
	if got != string(want) {
		t.Fatalf("%s differs\n--- got ---\n%s--- want ---\n%s", name, got, want)
	}
}

// target is a temporary executable the installer may replace, and its
// original bytes.
type target struct {
	path string
	body []byte
}

func newTarget(t *testing.T) target {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	name := "demo"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	tg := target{path: filepath.Join(dir, name), body: []byte("old binary\n")}
	if err := os.WriteFile(tg.path, tg.body, 0o700); err != nil { //nolint:gosec // an executable fixture
		t.Fatal(err)
	}
	return tg
}

// unchanged fails the test if the target's bytes changed.
func (tg target) unchanged(t *testing.T) {
	t.Helper()
	got, err := os.ReadFile(tg.path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(tg.body) {
		t.Fatalf("target changed: %q", got)
	}
}

// release is a fixture release of product at tag, for the running platform.
func release(product, tag string) selfupdatetest.ReleaseSpec {
	return selfupdatetest.NewRelease(product, tag, []selfupdate.Platform{here},
		func(selfupdate.Platform) []byte { return []byte(product + " " + tag + "\n") })
}

// newUpdater builds an Updater over src that may replace tg.
func newUpdater(t *testing.T, src selfupdate.ReleaseSource, tg target) *selfupdate.Updater {
	t.Helper()
	u, err := buildUpdater(src, tg)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// buildUpdater is newUpdater for code with no *testing.T, such as a helper
// process.
func buildUpdater(src selfupdate.ReleaseSource, tg target) (*selfupdate.Updater, error) {
	return buildUpdaterClosing(src, tg, nil)
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

// buildUpdaterClosing is buildUpdater whose sessions fail Close with
// closeErr, when it is not nil.
func buildUpdaterClosing(src selfupdate.ReleaseSource, tg target, closeErr error) (*selfupdate.Updater, error) {
	assets, err := selfupdate.NewExactAssetSelector([]selfupdate.Platform{here})
	if err != nil {
		return nil, err
	}
	inst, err := selfupdate.NewStandaloneInstaller(selfupdate.InstallOptions{
		TargetPolicy: selfupdate.TargetPolicy{ExecutablePath: tg.path, AllowedRoots: []string{filepath.Dir(tg.path)}},
	})
	if err != nil {
		return nil, err
	}
	var installer selfupdate.Installer = inst
	if closeErr != nil {
		installer = closeFailInstaller{inst, closeErr}
	}
	return selfupdate.New(selfupdate.Config{
		Source:    src,
		Versions:  selfupdate.NewStrictVersionPolicy(),
		Assets:    assets,
		Installer: installer,
		Reporter:  selfupdate.DiscardReporter(),
		Confirmer: selfupdate.NonInteractiveConfirmer(),
		Limits:    selfupdate.DefaultLimits(),
	})
}
