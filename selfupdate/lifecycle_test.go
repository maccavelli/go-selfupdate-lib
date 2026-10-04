package selfupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Tests for docs/decisions/0004-PLAN-v1-1-0-core-api.md Step 10.

// lifecycleRun runs the test binary's release over a temporary target.
func lifecycleRun(t *testing.T, opts InstallOptions, req Request, conf Confirmer, probes ...Prober) (Result, *recReporter, string, error) {
	t.Helper()
	_, exe := withTempHome(t)
	opts.TargetPolicy = TargetPolicy{ExecutablePath: exe}
	inst, err := NewStandaloneInstaller(opts)
	if err != nil {
		t.Fatal(err)
	}
	rel, bodies, plats := probeRelease(t)
	sel, _ := NewExactAssetSelector(plats)
	rep := &recReporter{}
	u, err := New(Config{Source: &scriptSource{rel: rel, bodies: bodies}, Versions: NewStrictVersionPolicy(), Assets: sel,
		Installer: inst, Reporter: rep, Confirmer: conf, Limits: DefaultLimits(), Probes: probes})
	if err != nil {
		t.Fatal(err)
	}
	res, err := u.Run(context.Background(), req)
	return res, rep, exe, err
}

// updateFiles lists the staging and backup files beside a target.
func updateFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".demo.selfupdate-") {
			out = append(out, e.Name())
		}
	}
	return out
}

func TestDryRunLeavesTargetUntouched(t *testing.T) {
	var probed []ProbePhase
	probe := ProberFunc(func(_ context.Context, r ProbeRequest) error {
		probed = append(probed, r.Phase)
		return nil
	})
	req := applyReq()
	req.Yes = true
	req.DryRun = true
	res, rep, exe, err := lifecycleRun(t, InstallOptions{}, req, &recConfirmer{}, probe)
	if err != nil {
		t.Fatal(err)
	}
	if got := readString(t, exe); got != "old-bytes" {
		t.Fatalf("a dry run changed the target: %d bytes", len(got))
	}
	if left := updateFiles(t, filepath.Dir(exe)); len(left) != 0 {
		t.Fatalf("a dry run left %v", left)
	}
	if containsKind(rep.kinds, EventInstalling) {
		t.Fatalf("a dry run reported installing: %v", rep.kinds)
	}
	last := rep.events[len(rep.events)-1]
	if last.Kind != EventComplete || last.Detail != "dry-run" {
		t.Fatalf("last event = %+v", last)
	}
	if len(probed) != 1 || probed[0] != ProbeStaged {
		t.Fatalf("probes ran for phases %v, want one staged probe", probed)
	}
	if !res.DryRun || res.Applied || res.ReleaseDigest == "" || res.InstalledDigest == "" {
		t.Fatalf("res = %+v", res)
	}
}

func TestDryRunNeedsNoConfirmation(t *testing.T) {
	conf := &recConfirmer{ok: false}
	req := applyReq()
	req.DryRun = true
	res, _, _, err := lifecycleRun(t, InstallOptions{}, req, conf)
	if err != nil {
		t.Fatal(err)
	}
	if conf.calls != 0 || res.Declined || !res.DryRun {
		t.Fatalf("confirmer calls = %d, res = %+v", conf.calls, res)
	}
}

func TestDryRunEchoedWhenUpToDate(t *testing.T) {
	req := applyReq()
	req.CurrentVersion = "v1.1.0"
	req.DryRun = true
	res, _, _, err := lifecycleRun(t, InstallOptions{}, req, &recConfirmer{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Operation != OperationNone || !res.DryRun {
		t.Fatalf("res = %+v, want OperationNone with DryRun echoed", res)
	}
}

func TestCheckAndDryRunRejected(t *testing.T) {
	req := applyReq()
	req.CheckOnly = true
	req.DryRun = true
	err := validateRequest(req, NewStrictVersionPolicy())
	if err == nil || err.Error() != "selfupdate: --check and --dry-run are contradictory" {
		t.Fatalf("err = %v", err)
	}
}

func keepPreviousSession(t *testing.T) (InstallSession, string) {
	t.Helper()
	_, exe := withTempHome(t)
	inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}, KeepPrevious: true})
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

func TestKeepPrevious(t *testing.T) {
	sess, exe := keepPreviousSession(t)
	previous := filepath.Join(filepath.Dir(exe), ".demo.previous")
	if err := os.WriteFile(previous, []byte("older"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: stageNew(t, sess)}})
	if err != nil {
		t.Fatal(err)
	}
	// Previous is the target's canonical sibling: on macOS the temporary
	// directory resolves through the /var symlink.
	if !res.Applied || filepath.Base(res.Previous) != ".demo.previous" || res.PendingBackup != "" {
		t.Fatalf("res = %+v", res)
	}
	if got := readString(t, exe); got != "new-bytes" {
		t.Fatalf("target = %q", got)
	}
	if got := readString(t, previous); got != "old-bytes" {
		t.Fatalf("previous = %q, want the replaced binary over the older one", got)
	}
	if left := backupsIn(t, filepath.Dir(exe)); len(left) != 0 {
		t.Fatalf("backups left: %v", left)
	}

	// The coordinator reports it in Result.
	req := applyReq()
	req.Yes = true
	run, _, runExe, err := lifecycleRun(t, InstallOptions{KeepPrevious: true}, req, &recConfirmer{})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(run.Previous) != ".demo.previous" {
		t.Fatalf("Result.Previous = %q", run.Previous)
	}
	if got := readString(t, filepath.Join(filepath.Dir(runExe), ".demo.previous")); got != "old-bytes" {
		t.Fatalf("previous after Run = %d bytes", len(got))
	}
}

func TestKeepPreviousOffRemovesBackup(t *testing.T) {
	sess, exe := postInstallSession(t, nil)
	res, err := sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: stageNew(t, sess)}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Previous != "" {
		t.Fatalf("Previous = %q without KeepPrevious", res.Previous)
	}
	if _, err := os.Lstat(filepath.Join(filepath.Dir(exe), ".demo.previous")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("previous kept without KeepPrevious: %v", err)
	}
}

func TestKeepPreviousManaged(t *testing.T) {
	_, exe := withTempHome(t)
	inner, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}, KeepPrevious: true})
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
	if err != nil {
		t.Fatal(err)
	}
	if !res.Applied || !res.ServiceInstalled || filepath.Base(res.Previous) != ".demo.previous" || life.starts != 1 {
		t.Fatalf("res = %+v, starts = %d", res, life.starts)
	}
	if got := readString(t, filepath.Join(filepath.Dir(exe), ".demo.previous")); got != "old-bytes" {
		t.Fatalf("previous = %q", got)
	}
}

func cleanupInstaller(t *testing.T, exe string) *StandaloneInstaller {
	t.Helper()
	inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}, LockTimeout: 200 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	return inst
}

func TestCleanupPendingProcessesReceipt(t *testing.T) {
	_, exe := withTempHome(t)
	target, err := resolveTarget(TargetPolicy{ExecutablePath: exe})
	if err != nil {
		t.Fatal(err)
	}
	gone := plantPendingCleanup(t, target)
	if err := cleanupInstaller(t, exe).CleanupPending(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, p := range gone {
		if _, err := os.Lstat(p); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s remains after CleanupPending: %v", filepath.Base(p), err)
		}
	}
	if got := readString(t, exe); got != "old-bytes" {
		t.Fatalf("CleanupPending changed the target: %q", got)
	}
}

func TestCleanupPendingLocked(t *testing.T) {
	_, exe := withTempHome(t)
	inst := cleanupInstaller(t, exe)
	target, err := inst.ResolveTarget(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	holder, err := inst.Begin(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	if err := inst.CleanupPending(context.Background()); !errors.Is(err, ErrConcurrentUpdate) {
		t.Fatalf("CleanupPending under a held lock = %v, want ErrConcurrentUpdate", err)
	}
	if err := holder.Close(); err != nil {
		t.Fatal(err)
	}
	// Twice in a row: each call releases the lock it took.
	for i := range 2 {
		if err := inst.CleanupPending(context.Background()); err != nil {
			t.Fatalf("CleanupPending call %d = %v", i+1, err)
		}
	}
}
