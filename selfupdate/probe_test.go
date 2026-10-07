package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// Tests for docs/decisions/0004-PLAN-v1-1-0-core-api.md Step 8. The "new
// release" is this test binary: TestMain prints SELFUPDATE_TEST_PRINT_VERSION
// when it is set.

func probeRelease(t *testing.T) (Release, map[int64][]byte, []Platform) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	bin, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	plat := Platform{OS: runtime.GOOS, Arch: runtime.GOARCH}
	sum := sha256.Sum256(bin)
	hexsum := hex.EncodeToString(sum[:])
	name := ExactAssetName("demo", plat)
	manifest := []byte(hexsum + "  " + name + "\n")
	rel := Release{ID: 1, Tag: "v1.1.0", URL: "https://example.invalid/v1.1.0", Immutable: true, Assets: []Asset{
		{ID: 2, Name: name, State: AssetStateUploaded, Size: int64(len(bin)), Digest: "sha256:" + hexsum},
		{ID: 3, Name: manifestAssetName, State: AssetStateUploaded, Size: int64(len(manifest))},
	}}
	return rel, map[int64][]byte{2: bin, 3: manifest}, []Platform{plat}
}

func testVersionProber(t *testing.T) Prober {
	t.Helper()
	p, err := NewVersionProber([]string{"--version"}, nil, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// probeRun applies the test binary over a temporary target with the given
// staged probes.
func probeRun(t *testing.T, probes ...Prober) (Result, string, error) {
	t.Helper()
	_, exe := withTempHome(t)
	inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}})
	if err != nil {
		t.Fatal(err)
	}
	rel, bodies, plats := probeRelease(t)
	sel, _ := NewExactAssetSelector(plats)
	limits := DefaultLimits()
	u, err := New(Config{Source: &scriptSource{rel: rel, bodies: bodies}, Versions: NewStrictVersionPolicy(), Assets: sel,
		Installer: inst, Reporter: &recReporter{}, Confirmer: &recConfirmer{}, Limits: limits, Probes: probes})
	if err != nil {
		t.Fatal(err)
	}
	req := applyReq()
	req.Yes = true
	res, err := u.Run(context.Background(), req)
	return res, exe, err
}

// TestStagedProbeRunsRealBinary: the staging file is runnable, on Windows
// too, and a version mismatch stops the install.
func TestStagedProbeRunsRealBinary(t *testing.T) {
	t.Setenv("SELFUPDATE_TEST_PRINT_VERSION", "demo v1.1.0")
	res, _, err := probeRun(t, testVersionProber(t))
	if err != nil || !res.Applied {
		t.Fatalf("res=%+v err=%v", res, err)
	}

	t.Setenv("SELFUPDATE_TEST_PRINT_VERSION", "demo v1.0.0")
	res, exe, err := probeRun(t, testVersionProber(t))
	if err == nil || res.Applied || !strings.Contains(err.Error(), "failed a probe") {
		t.Fatalf("a mismatched version was installed: res=%+v err=%v", res, err)
	}
	if got := readString(t, exe); got != "old-bytes" {
		t.Fatalf("target holds %d bytes after a failed probe", len(got))
	}
}

// TestProbesRunBeforeInstall: staged probes run before Install, and a
// failure means Install never runs.
func TestProbesRunBeforeInstall(t *testing.T) {
	env := newContractEnv(t)
	probe := ProberFunc(func(context.Context, ProbeRequest) error {
		*env.log = append(*env.log, "Probe")
		return nil
	})
	u := env.build(t)
	u.probes = []Prober{probe}
	req := applyReq()
	req.Yes = true
	if _, err := u.Run(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	got := calls(*env.log)
	if p, i := slices.Index(got, "Probe"), slices.Index(got, "Install"); p < 0 || i < 0 || p > i {
		t.Fatalf("calls = %v, want Probe before Install", got)
	}

	failing := newContractEnv(t)
	fu := failing.build(t)
	fu.probes = []Prober{ProberFunc(func(context.Context, ProbeRequest) error { return errors.New("does not start") })}
	if _, err := fu.Run(context.Background(), req); err == nil {
		t.Fatal("a failed probe did not fail the run")
	}
	if slices.Contains(calls(*failing.log), "Install") {
		t.Fatalf("Install ran after a failed probe: %v", calls(*failing.log))
	}
}

func postInstallSession(t *testing.T, probe Prober) (InstallSession, string) {
	t.Helper()
	_, exe := withTempHome(t)
	inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}, PostInstall: probe})
	if err != nil {
		t.Fatal(err)
	}
	target, err := inst.ResolveTarget(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := inst.Begin(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	return sess, exe
}

func TestPostInstallRollback(t *testing.T) {
	sess, exe := postInstallSession(t, ProberFunc(func(context.Context, ProbeRequest) error { return errors.New("crashes") }))
	res, err := sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: stageNew(t, sess)}})
	if err == nil || res.Applied || !res.RolledBack || !strings.Contains(err.Error(), "failed its probe") {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if got := readString(t, exe); got != "old-bytes" {
		t.Fatalf("target holds %q after the rollback", got)
	}
	if left := backupsIn(t, filepath.Dir(exe)); len(left) != 0 {
		t.Fatalf("backups left: %v", left)
	}
}

func TestPostInstallRollbackFailureReportsBackup(t *testing.T) {
	sess, _ := postInstallSession(t, ProberFunc(func(context.Context, ProbeRequest) error { return errors.New("crashes") }))
	path := stageNew(t, sess)
	failRestore(t)
	res, err := sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: path}})
	if err == nil || res.Applied || res.RolledBack || res.Backup == "" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if got := readString(t, res.Backup); got != "old-bytes" {
		t.Fatalf("backup holds %q", got)
	}
}

func TestPostInstallManaged(t *testing.T) {
	_, exe := withTempHome(t)
	inner, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe},
		PostInstall: ProberFunc(func(context.Context, ProbeRequest) error { return errors.New("crashes") })})
	if err != nil {
		t.Fatal(err)
	}
	life := &fakeLife{installed: true, running: true}
	m, err := NewManagedInstaller(inner, life, &fakeRec{})
	if err != nil {
		t.Fatal(err)
	}
	target, err := m.ResolveTarget(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := m.Begin(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	res, err := sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: stageNew(t, sess)}})
	if !errors.Is(err, ErrManagedInstall) || !res.RolledBack || life.starts == 0 {
		t.Fatalf("res=%+v err=%v starts=%d", res, err, life.starts)
	}
	if got := readString(t, exe); got != "old-bytes" {
		t.Fatalf("target holds %q", got)
	}
}

func selfExe(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return exe
}

func TestVersionProberMatch(t *testing.T) {
	t.Setenv("SELFUPDATE_TEST_PRINT_VERSION", "demo v1.1.0 (linux/amd64)")
	if err := testVersionProber(t).Probe(context.Background(), ProbeRequest{Product: "demo", TargetVersion: "v1.1.0", Path: selfExe(t), Phase: ProbeStaged}); err != nil {
		t.Fatal(err)
	}
}

func TestVersionProberMismatch(t *testing.T) {
	t.Setenv("SELFUPDATE_TEST_PRINT_VERSION", "demo v1.0.0")
	err := testVersionProber(t).Probe(context.Background(), ProbeRequest{Product: "demo", TargetVersion: "v1.1.0", Path: selfExe(t), Phase: ProbeStaged})
	if err == nil || !strings.Contains(err.Error(), `printed "demo v1.0.0"`) {
		t.Fatalf("err = %v", err)
	}
}

func TestVersionProberNonZeroExit(t *testing.T) {
	t.Setenv("SELFUPDATE_TEST_PRINT_VERSION", "demo v1.1.0")
	t.Setenv("SELFUPDATE_TEST_EXIT", "3")
	if err := testVersionProber(t).Probe(context.Background(), ProbeRequest{TargetVersion: "v1.1.0", Path: selfExe(t)}); err == nil {
		t.Fatal("a non-zero exit passed")
	}
}

func TestVersionProberTimeout(t *testing.T) {
	t.Setenv("SELFUPDATE_TEST_PRINT_VERSION", "demo v1.1.0")
	t.Setenv("SELFUPDATE_TEST_SLEEP", "10s")
	p, err := NewVersionProber([]string{"--version"}, nil, 200*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	err = p.Probe(context.Background(), ProbeRequest{TargetVersion: "v1.1.0", Path: selfExe(t)})
	if err == nil || !strings.Contains(err.Error(), "timed out") || time.Since(start) > 5*time.Second {
		t.Fatalf("err=%v after %v", err, time.Since(start))
	}
}

// TestProbeRequestFields: both probes see the product, the tag, their phase
// and the path of the copy they run.
func TestProbeRequestFields(t *testing.T) {
	var staged, installed ProbeRequest
	_, exe := withTempHome(t)
	inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe},
		PostInstall: ProberFunc(func(_ context.Context, r ProbeRequest) error { installed = r; return nil })})
	if err != nil {
		t.Fatal(err)
	}
	rel, bodies, plats := fixtureRelease(t, "demo")
	sel, _ := NewExactAssetSelector(plats)
	u, err := New(Config{Source: &scriptSource{rel: rel, bodies: bodies}, Versions: NewStrictVersionPolicy(), Assets: sel,
		Installer: inst, Reporter: &recReporter{}, Confirmer: &recConfirmer{}, Limits: DefaultLimits(),
		Probes: []Prober{ProberFunc(func(_ context.Context, r ProbeRequest) error { staged = r; return nil })}})
	if err != nil {
		t.Fatal(err)
	}
	req := applyReq()
	req.Yes = true
	if _, err := u.Run(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	// The target is canonical: on macOS the temporary directory resolves
	// through the /var symlink.
	canon, err := filepath.EvalSymlinks(exe)
	if err != nil {
		t.Fatal(err)
	}
	if staged.Phase != ProbeStaged || staged.Product != "demo" || staged.TargetVersion != "v1.1.0" ||
		filepath.Dir(staged.Path) != filepath.Dir(canon) || !strings.HasPrefix(filepath.Base(staged.Path), ".demo.selfupdate-") {
		t.Fatalf("staged request %+v", staged)
	}
	if runtime.GOOS == goosWindows && !strings.HasSuffix(staged.Path, ".exe") {
		t.Fatalf("Windows staging %q does not end in .exe", staged.Path)
	}
	if installed.Phase != ProbeInstalled || installed.Path != canon || installed.TargetVersion != "v1.1.0" {
		t.Fatalf("installed request %+v", installed)
	}
	if ProbeStaged.String() != "staged" || ProbeInstalled.String() != "installed" {
		t.Fatal("phase names")
	}
}

func TestNewVersionProberArguments(t *testing.T) {
	if _, err := NewVersionProber(nil, nil, time.Second); err == nil {
		t.Error("no arguments accepted")
	}
	if _, err := NewVersionProber([]string{"--version"}, nil, 0); err == nil {
		t.Error("zero timeout accepted")
	}
	env := newContractEnv(t)
	_, _, plats := fixtureRelease(t, "demo")
	sel, _ := NewExactAssetSelector(plats)
	if _, err := New(Config{Source: env.src, Versions: NewStrictVersionPolicy(), Assets: sel, Installer: env.inst,
		Reporter: env.rep, Confirmer: env.conf, Limits: env.lim, Probes: []Prober{nil}}); err == nil {
		t.Error("a nil probe accepted")
	}
}

// TestVersionProberHonoursRunContext: a probe the run's deadline or
// cancellation ends says so: its error wraps the run's context error, not
// the prober's own timeout, so EventFailed's class is the run's
// (docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md B6).
func TestVersionProberHonoursRunContext(t *testing.T) {
	t.Setenv("SELFUPDATE_TEST_PRINT_VERSION", "demo v1.1.0")
	t.Setenv("SELFUPDATE_TEST_SLEEP", "10s")
	p, err := NewVersionProber([]string{"--version"}, nil, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	req := ProbeRequest{Product: "demo", TargetVersion: "v1.1.0", Path: selfExe(t), Phase: ProbeStaged}
	t.Run("deadline", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		err := p.Probe(ctx, req)
		if !errors.Is(err, context.DeadlineExceeded) || failureClass(err) != "deadline-exceeded" {
			t.Fatalf("err = %v, class %q; want the run's deadline", err, failureClass(err))
		}
	})
	t.Run("cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(100*time.Millisecond, cancel)
		err := p.Probe(ctx, req)
		if !errors.Is(err, context.Canceled) || failureClass(err) != "canceled" {
			t.Fatalf("err = %v, class %q; want the run's cancellation", err, failureClass(err))
		}
	})
}
