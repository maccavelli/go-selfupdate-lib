//go:build unix

package selfupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Tests for docs/decisions/0010-PLAN-v1-6-0-owner-contracts.md S6 (Q6,
// B12): special mode bits. These use only the v1.5.1 API, so they run
// against the unfixed code too.

// chmodOrSkip sets mode on path, and skips the test when the OS will not
// let this user set it: BSD refuses setgid on a file whose group the user
// is not in, and some refuse sticky on a regular file.
func chmodOrSkip(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.Chmod(path, mode); err != nil {
		t.Skipf("chmod %v: %v", mode, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky|os.ModePerm) != mode {
		t.Skipf("chmod %v did not stick: %v", mode, info.Mode())
	}
}

// TestSpecialBitTargetRefused: a target with setuid or setgid is not
// replaced unless the policy allows it.
func TestSpecialBitTargetRefused(t *testing.T) {
	for _, bit := range []os.FileMode{os.ModeSetuid, os.ModeSetgid} {
		t.Run(bit.String(), func(t *testing.T) {
			_, exe := withTempHome(t)
			chmodOrSkip(t, exe, bit|0o755)
			inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}})
			if err != nil {
				t.Fatal(err)
			}
			_, err = inst.ResolveTarget(context.Background())
			if err == nil || !strings.Contains(err.Error(), "setuid or setgid") {
				t.Fatalf("ResolveTarget = %v, want the special bit refused", err)
			}
		})
	}
}

// TestStickyBitCarriedOver: the sticky bit is kept on the new binary.
func TestStickyBitCarriedOver(t *testing.T) {
	_, exe := withTempHome(t)
	chmodOrSkip(t, exe, os.ModeSticky|0o755)
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
	defer func() { _ = sess.Close() }()
	if _, err := sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: stageNew(t, sess)}}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(exe)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSticky == 0 || info.Mode().Perm() != 0o755 || readString(t, exe) != "new-bytes" {
		t.Fatalf("new binary mode %v", info.Mode())
	}
}

// TestKeepPreviousClearsSpecialBits: the previous binary kept beside the
// target loses setuid and setgid, so the copy an update replaced is not
// left privileged at a predictable path; the new binary keeps its bit
// (docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md B5).
func TestKeepPreviousClearsSpecialBits(t *testing.T) {
	for _, bit := range []os.FileMode{os.ModeSetuid, os.ModeSetgid} {
		t.Run(bit.String(), func(t *testing.T) {
			_, exe := withTempHome(t)
			chmodOrSkip(t, exe, bit|0o755)
			inst, err := NewStandaloneInstaller(InstallOptions{
				TargetPolicy: TargetPolicy{ExecutablePath: exe, AllowSpecialModeBits: true},
				KeepPrevious: true,
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
			defer func() { _ = sess.Close() }()
			res, err := sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: stageNew(t, sess)}})
			if err != nil || res.Previous == "" {
				t.Fatalf("Install = %+v, %v", res, err)
			}
			prev, err := os.Stat(res.Previous)
			if err != nil {
				t.Fatal(err)
			}
			if prev.Mode()&(os.ModeSetuid|os.ModeSetgid) != 0 || prev.Mode().Perm() != 0o755 {
				t.Fatalf("previous=%s mode=%v; want no setuid or setgid", res.Previous, prev.Mode())
			}
			cur, err := os.Stat(exe)
			if err != nil {
				t.Fatal(err)
			}
			if cur.Mode()&bit == 0 {
				t.Fatalf("the new binary lost %v: %v", bit, cur.Mode())
			}
		})
	}
}

// TestKeptBackupClearsSpecialBits: a backup kept because restoring it
// failed, the only copy of the previous binary, loses setuid too
// (docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md B5).
func TestKeptBackupClearsSpecialBits(t *testing.T) {
	_, exe := withTempHome(t)
	chmodOrSkip(t, exe, os.ModeSetuid|0o755)
	inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe, AllowSpecialModeBits: true}})
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
	defer func() { _ = sess.Close() }()
	staged := stageNew(t, sess)
	setSeam(t, &syncDirFn, func(string) error { return errors.New("injected directory sync failure") })
	failRestore(t)
	res, err := sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: staged}})
	if err == nil || res.Applied || res.Backup == "" {
		t.Fatalf("Install = %+v, %v; want a kept backup", res, err)
	}
	info, err := os.Lstat(res.Backup)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&(os.ModeSetuid|os.ModeSetgid) != 0 || info.Mode().Perm() != 0o755 {
		t.Fatalf("kept backup %s mode %v; want no setuid", filepath.Base(res.Backup), info.Mode())
	}
}
