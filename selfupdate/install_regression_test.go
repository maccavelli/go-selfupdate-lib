package selfupdate

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Regression tests for docs/decisions/0010-MADR-remediate-second-debugging-pass-findings.md
// B1, B2, B7–B10 (docs/decisions/0010-PLAN-v1-5-1-contract-preserving-fixes.md P3).

// swapAfterFirstReplace renames the target directory away right after the
// first replacePath (the replacement) and puts an empty one in its place.
// readOnly also makes the moved, locked directory read-only, so a rollback
// through the root fails while the backup survives.
func swapAfterFirstReplace(t *testing.T, dir string, readOnly bool) (moved string) {
	t.Helper()
	if runtime.GOOS == goosWindows {
		t.Skip("a directory swap is refused on Windows")
	}
	moved = dir + ".moved"
	real := replacePath
	calls := 0
	setSeam(t, &replacePath, func(ctx context.Context, oldpath, newpath string) error {
		calls++
		if err := real(ctx, oldpath, newpath); err != nil {
			return err
		}
		if calls == 1 {
			if err := os.Rename(dir, moved); err != nil {
				t.Fatalf("swap: %v", err)
			}
			if readOnly {
				if err := os.Chmod(moved, 0o555); err != nil {
					t.Fatal(err)
				}
			}
			return os.Mkdir(dir, 0o755)
		}
		return nil
	})
	t.Cleanup(func() {
		_ = os.Chmod(moved, 0o755)
		_ = os.RemoveAll(dir)
		_ = os.Rename(moved, dir)
	})
	return moved
}

// TestProbeOutputCapped: the probe keeps at most maxProbeOutput bytes, both
// when written to and when copied into, so a version printed past the cap
// is not found (B1).
func TestProbeOutputCapped(t *testing.T) {
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_, _ = pw.Write(bytes.Repeat([]byte("x"), 4<<20))
		_ = pw.Close()
	}()
	out := &cappedBuffer{limit: maxProbeOutput}
	if _, err := io.Copy(out, pr); err != nil {
		t.Fatal(err)
	}
	if err := pr.Close(); err != nil {
		t.Fatal(err)
	}
	if out.Len() != maxProbeOutput {
		t.Fatalf("buffered %d bytes through io.Copy, want the cap %d", out.Len(), maxProbeOutput)
	}

	if runtime.GOOS == goosWindows {
		return
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	script := filepath.Join(t.TempDir(), "bin")
	body := "#!/bin/sh\nhead -c 204800 /dev/zero | tr '\\0' 'x'\necho\necho v9.9.9\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil { //nolint:gosec // an executable fixture
		t.Fatal(err)
	}
	p, err := NewVersionProber([]string{"--version"}, nil, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	err = p.Probe(context.Background(), ProbeRequest{Product: "demo", TargetVersion: "v9.9.9", Path: script, Phase: ProbeStaged})
	if err == nil {
		t.Fatal("a version printed after 200 KiB was found; the 64 KiB cap was bypassed")
	}
}

// TestHomeUnavailableRoot: an unset, root or missing home directory is
// skipped, so an AllowedRoots entry that covers the target is enough; with
// no root left, resolution fails (B2).
func TestHomeUnavailableRoot(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "demo")
	if err := os.WriteFile(exe, []byte("old-bytes"), 0o755); err != nil { //nolint:gosec // an executable fixture
		t.Fatal(err)
	}
	for name, home := range map[string]func() (string, error){
		"unset":   func() (string, error) { return "", errors.New("$HOME is not defined") },
		"root":    func() (string, error) { return string(filepath.Separator), nil },
		"missing": func() (string, error) { return filepath.Join(root, "no-such-home"), nil },
	} {
		t.Run(name, func(t *testing.T) {
			setSeam(t, &userHomeDir, home)
			if _, err := resolveTarget(TargetPolicy{ExecutablePath: exe, AllowedRoots: []string{root}}); err != nil {
				t.Fatalf("refused although AllowedRoots covers the target: %v", err)
			}
			if _, err := resolveTarget(TargetPolicy{ExecutablePath: exe}); err == nil {
				t.Fatal("resolved with no usable root")
			}
		})
	}
}

// TestApplyUndoesSwap: a directory swapped during Apply is detected after
// the rename, and the replacement is undone in the locked directory (B7).
func TestApplyUndoesSwap(t *testing.T) {
	_, exe := withTempHome(t)
	inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}})
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
	defer sess.Close()
	path := stageNew(t, sess)
	moved := swapAfterFirstReplace(t, filepath.Dir(exe), false)
	applied, err := sess.(TwoPhaseSession).Apply(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: path}})
	if !errors.Is(err, ErrConcurrentUpdate) {
		t.Fatalf("Apply err = %v, want ErrConcurrentUpdate", err)
	}
	if got := readString(t, filepath.Join(moved, "demo")); got != "old-bytes" {
		t.Fatalf("locked directory's target = %q, want the old binary back", got)
	}
	if applied.Backup != "" || len(backupsIn(t, moved)) != 0 {
		t.Fatalf("a backup is reported (%q) or left (%v) after the undo", applied.Backup, backupsIn(t, moved))
	}
}

// TestRunReportsUnrestoredBackup: when the undo after a swap fails, Run
// reports the backup, the only copy of the previous binary (B8).
func TestRunReportsUnrestoredBackup(t *testing.T) {
	_, exe := withTempHome(t)
	inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}})
	if err != nil {
		t.Fatal(err)
	}
	rel, bodies, plats := probeRelease(t)
	sel, err := NewExactAssetSelector(plats)
	if err != nil {
		t.Fatal(err)
	}
	u, err := New(Config{Source: &scriptSource{rel: rel, bodies: bodies}, Versions: NewStrictVersionPolicy(), Assets: sel,
		Installer: inst, Reporter: &recReporter{}, Confirmer: &recConfirmer{}, Limits: DefaultLimits()})
	if err != nil {
		t.Fatal(err)
	}
	moved := swapAfterFirstReplace(t, filepath.Dir(exe), true)
	req := applyReq()
	req.Yes = true
	res, err := u.Run(context.Background(), req)
	if err == nil {
		t.Fatal("Run succeeded across a directory swap")
	}
	left := backupsIn(t, moved)
	if len(left) != 1 || res.PendingBackup == "" || filepath.Base(res.PendingBackup) != left[0] {
		t.Fatalf("PendingBackup = %q, backups left = %v: the surviving backup must be reported", res.PendingBackup, left)
	}
}

// TestProbeRollbackSyncFailure: when the probe's rollback renames the
// backup back but the directory sync fails, the result says rolled back and
// names no backup that no longer exists (B9).
func TestProbeRollbackSyncFailure(t *testing.T) {
	_, exe := withTempHome(t)
	inst, err := NewStandaloneInstaller(InstallOptions{
		TargetPolicy: TargetPolicy{ExecutablePath: exe},
		PostInstall:  ProberFunc(func(context.Context, ProbeRequest) error { return errors.New("crashes on start") }),
	})
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
	defer sess.Close()
	calls := 0
	setSeam(t, &syncDirFn, func(dir string) error {
		calls++
		if calls == 2 {
			return errors.New("injected sync failure after the rollback rename")
		}
		return syncDirectory(dir)
	})
	path := stageNew(t, sess)
	res, err := sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: path}})
	if err == nil || !strings.Contains(err.Error(), "injected sync failure") {
		t.Fatalf("err = %v, want the sync failure reported", err)
	}
	if got := readString(t, exe); got != "old-bytes" {
		t.Fatalf("target = %q, want the old binary back", got)
	}
	if !res.RolledBack || res.Backup != "" {
		t.Fatalf("RolledBack=%v Backup=%q, want rolled back and no backup named", res.RolledBack, res.Backup)
	}
}

// TestClosedSessionRefusesCommitAndRollback: after Close has released the
// lock, Commit and Rollback refuse, so they never touch a target another
// session holds (B10).
func TestClosedSessionRefusesCommitAndRollback(t *testing.T) {
	_, exe := withTempHome(t)
	inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}})
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
	two := sess.(TwoPhaseSession)
	path := stageNew(t, sess)
	applied, err := two.Apply(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: path}})
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Close(); err != nil {
		t.Fatal(err)
	}
	if err := two.Rollback(context.Background(), applied); err == nil || !strings.Contains(err.Error(), "session is closed") {
		t.Fatalf("Rollback after Close = %v", err)
	}
	if _, err := two.Commit(context.Background(), applied); err == nil || !strings.Contains(err.Error(), "session is closed") {
		t.Fatalf("Commit after Close = %v", err)
	}
	if got := readString(t, exe); got != "new-bytes" {
		t.Fatalf("target = %q: a closed session changed it", got)
	}
}
