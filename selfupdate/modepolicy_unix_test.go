//go:build unix

package selfupdate

import (
	"context"
	"errors"
	"os"
	"syscall"
	"testing"
)

// Tests for docs/decisions/0010-PLAN-v1-6-0-owner-contracts.md S6 (Q6): the
// special-bit policy and ownership on Unix.

// unixInstall installs "new-bytes" over exe under policy.
func unixInstall(t *testing.T, exe string, policy TargetPolicy) (InstallResult, error) {
	t.Helper()
	inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: policy})
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
	return sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: stageNew(t, sess)}})
}

// TestSpecialBitsAllowed: with the policy, the target is replaced and the
// new binary keeps the bit.
func TestSpecialBitsAllowed(t *testing.T) {
	for _, bit := range []os.FileMode{os.ModeSetuid, os.ModeSetgid} {
		t.Run(bit.String(), func(t *testing.T) {
			_, exe := withTempHome(t)
			chmodOrSkip(t, exe, bit|0o755)
			if _, err := unixInstall(t, exe, TargetPolicy{ExecutablePath: exe, AllowSpecialModeBits: true}); err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(exe)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode()&bit == 0 || readString(t, exe) != "new-bytes" {
				t.Fatalf("new binary mode %v", info.Mode())
			}
		})
	}
}

// TestStagingOwnership: staging is given the target's owner and group.
// EPERM, an unprivileged updater's answer, is not an error; any other
// failure is.
func TestStagingOwnership(t *testing.T) {
	for _, c := range []struct {
		name    string
		err     error
		wantErr bool
	}{
		{"accepted", nil, false},
		{"EPERM", syscall.EPERM, false},
		{"EIO", syscall.EIO, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, exe := withTempHome(t)
			info, err := os.Stat(exe)
			if err != nil {
				t.Fatal(err)
			}
			st := info.Sys().(*syscall.Stat_t)
			var gotUID, gotGID int
			setSeam(t, &osLchown, func(_ string, uid, gid int) error {
				gotUID, gotGID = uid, gid
				return c.err
			})
			res, err := unixInstall(t, exe, TargetPolicy{ExecutablePath: exe})
			if gotUID != int(st.Uid) || gotGID != int(st.Gid) {
				t.Fatalf("chown to %d:%d, want %d:%d", gotUID, gotGID, st.Uid, st.Gid)
			}
			if c.wantErr != (err != nil) || (c.wantErr && !errors.Is(err, c.err)) {
				t.Fatalf("Install = %+v, %v", res, err)
			}
			if want := map[bool]string{false: "new-bytes", true: "old-bytes"}[c.wantErr]; readString(t, exe) != want {
				t.Fatalf("target = %q, want %q", readString(t, exe), want)
			}
		})
	}
}

// TestStagingTakesOwnerAsRoot: as root, the new binary has the old one's
// owner and group. It skips unless it runs as root;
// SELFUPDATE_REQUIRE_ROOT=1, which CI's Linux leg sets under sudo, makes
// that a failure.
func TestStagingTakesOwnerAsRoot(t *testing.T) {
	if os.Geteuid() != 0 {
		if os.Getenv("SELFUPDATE_REQUIRE_ROOT") == "1" {
			t.Fatal("SELFUPDATE_REQUIRE_ROOT=1, and this is not root")
		}
		t.Skip("needs root to give a file to another owner")
	}
	_, exe := withTempHome(t)
	const uid, gid = 4242, 4243
	if err := os.Lchown(exe, uid, gid); err != nil {
		t.Fatal(err)
	}
	if _, err := unixInstall(t, exe, TargetPolicy{ExecutablePath: exe}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(exe)
	if err != nil {
		t.Fatal(err)
	}
	st := info.Sys().(*syscall.Stat_t)
	if st.Uid != uid || st.Gid != gid || readString(t, exe) != "new-bytes" {
		t.Fatalf("new binary owned by %d:%d, want %d:%d", st.Uid, st.Gid, uid, gid)
	}
}
