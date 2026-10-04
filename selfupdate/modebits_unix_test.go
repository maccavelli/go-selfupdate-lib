//go:build unix

package selfupdate

import (
	"context"
	"os"
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
