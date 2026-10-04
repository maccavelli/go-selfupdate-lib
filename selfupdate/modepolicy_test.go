package selfupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// Tests for docs/decisions/0010-PLAN-v1-6-0-owner-contracts.md S6 (Q6):
// the parts that need the v1.6.0 API or internal seams.

func TestStagingMode(t *testing.T) {
	for _, c := range []struct {
		old   os.FileMode
		allow bool
		want  os.FileMode
	}{
		{0o755, false, 0o755},
		{os.ModeSticky | 0o755, false, os.ModeSticky | 0o755},
		{os.ModeSetuid | os.ModeSetgid | 0o750, false, 0o750},
		{os.ModeSetuid | os.ModeSetgid | os.ModeSticky | 0o750, true, os.ModeSetuid | os.ModeSetgid | os.ModeSticky | 0o750},
		{os.ModeSetgid | 0o711, true, os.ModeSetgid | 0o711},
	} {
		if got := stagingMode(c.old, c.allow); got != c.want {
			t.Errorf("stagingMode(%v, %t) = %v, want %v", c.old, c.allow, got, c.want)
		}
	}
}

// TestIsLeftover: only the names CreateStaging and randomSibling make for
// this target match.
func TestIsLeftover(t *testing.T) {
	for name, want := range map[string]bool{
		".demo.selfupdate-1" + stagingSuffix():  true,
		".demo.selfupdate-bak-42":               true,
		".demo.selfupdate-":                     false,
		".demo.selfupdate-bak-":                 false,
		".demo.selfupdate-bak-4x":               false,
		".demo.selfupdate-x1" + stagingSuffix(): false,
		".demo.selfupdate.lock":                 false,
		".demo.selfupdate.cleanup":              false,
		".demo.previous":                        false,
		"demo":                                  false,
		// Another target whose name starts with this one's.
		".demo.selfupdate-x.selfupdate-1": false,
	} {
		if got := isLeftover("demo", name); got != want {
			t.Errorf("isLeftover(%q) = %t, want %t", name, got, want)
		}
	}
}

// TestRemoveLeftoversKeepsListed: a backup a receipt lists is kept, and an
// unreadable receipt keeps every backup.
func TestRemoveLeftoversKeepsListed(t *testing.T) {
	dir := t.TempDir()
	listed, other, staging := ".demo.selfupdate-bak-1", ".demo.selfupdate-bak-2", ".demo.selfupdate-3"+stagingSuffix()
	plant := func() {
		for _, n := range []string{listed, other, staging} {
			if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	exists := func(n string) bool { _, err := os.Lstat(filepath.Join(dir, n)); return err == nil }
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	target := Target{Path: filepath.Join(dir, "demo"), Dir: dir, Base: "demo"}

	plant()
	removeLeftovers(target, root, map[string]bool{listed: true}, true)
	if !exists(listed) || exists(other) || exists(staging) {
		t.Fatalf("listed kept %t, other kept %t, staging kept %t", exists(listed), exists(other), exists(staging))
	}
	plant()
	removeLeftovers(target, root, nil, false)
	if !exists(listed) || !exists(other) || exists(staging) {
		t.Fatalf("unreadable receipt: backups kept %t %t, staging kept %t", exists(listed), exists(other), exists(staging))
	}
}

// TestLeftoverRemovalFailureIgnored: a leftover that cannot be removed
// does not fail the session.
func TestLeftoverRemovalFailureIgnored(t *testing.T) {
	_, exe := withTempHome(t)
	left := filepath.Join(filepath.Dir(exe), ".demo.selfupdate-bak-9")
	if err := os.WriteFile(left, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	tried := 0
	setSeam(t, &leftoverRemove, func(*os.Root, string) error {
		tried++
		return errors.New("fixture: busy")
	})
	inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}})
	if err != nil {
		t.Fatal(err)
	}
	if err := inst.CleanupPending(context.Background()); err != nil || tried != 1 {
		t.Fatalf("CleanupPending = %v after %d removal attempts", err, tried)
	}
}
