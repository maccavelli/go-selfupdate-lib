package selfupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// Tests for docs/decisions/0010-PLAN-v1-6-0-owner-contracts.md S6 (Q6,
// B11): under the lock, a session removes what a crashed update left
// behind. These use only the v1.5.1 API, so they run against the unfixed
// code too.

// leftoverNames are what a crash can leave beside the target "demo": a
// staging file and a backup. The kept names match no leftover pattern.
func leftoverNames() (removed, kept []string) {
	return []string{
		".demo.selfupdate-1234567" + stagingSuffix(),
		".demo.selfupdate-bak-7654321",
	}, []string{
		".demo.previous",
		".demo.selfupdate-abc",
		".demo.selfupdate-bak-",
		".demo.selfupdate-12x",
		".other.selfupdate-123",
		"demo.selfupdate-123",
		"notes.txt",
	}
}

// plantLeftovers writes every name, and a directory that matches the
// staging pattern.
func plantLeftovers(t *testing.T, dir string) (removed, kept []string) {
	t.Helper()
	removed, kept = leftoverNames()
	for _, name := range append(append([]string{}, removed...), kept...) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("left"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	sub := ".demo.selfupdate-999"
	if err := os.Mkdir(filepath.Join(dir, sub), 0o700); err != nil {
		t.Fatal(err)
	}
	return removed, append(kept, sub)
}

func checkLeftovers(t *testing.T, dir string, removed, kept []string) {
	t.Helper()
	for _, name := range removed {
		if _, err := os.Lstat(filepath.Join(dir, name)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("leftover %s was not removed: %v", name, err)
		}
	}
	for _, name := range kept {
		if _, err := os.Lstat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s was removed: %v", name, err)
		}
	}
}

func TestBeginRemovesLeftovers(t *testing.T) {
	_, exe := withTempHome(t)
	removed, kept := plantLeftovers(t, filepath.Dir(exe))
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
	checkLeftovers(t, filepath.Dir(exe), removed, kept)
	if err := sess.Close(); err != nil {
		t.Fatal(err)
	}
	if got := readString(t, exe); got != "old-bytes" {
		t.Fatalf("target = %q", got)
	}
}

func TestCleanupPendingRemovesLeftovers(t *testing.T) {
	_, exe := withTempHome(t)
	removed, kept := plantLeftovers(t, filepath.Dir(exe))
	inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}})
	if err != nil {
		t.Fatal(err)
	}
	if err := inst.CleanupPending(context.Background()); err != nil {
		t.Fatal(err)
	}
	checkLeftovers(t, filepath.Dir(exe), removed, kept)
}

// TestLeftoverSymlinkNotFollowed: a symlink with a leftover's name is not a
// file this package wrote. It is kept, and so is what it points to.
func TestLeftoverSymlinkNotFollowed(t *testing.T) {
	home, exe := withTempHome(t)
	outside := filepath.Join(t.TempDir(), "precious")
	if err := os.WriteFile(outside, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(home, ".demo.selfupdate-bak-555")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}})
	if err != nil {
		t.Fatal(err)
	}
	if err := inst.CleanupPending(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatalf("the symlink was removed: %v", err)
	}
	if got := readString(t, outside); got != "keep" {
		t.Fatalf("the file it points to = %q", got)
	}
}
