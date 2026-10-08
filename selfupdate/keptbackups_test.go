package selfupdate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Tests for docs/decisions/0017-PLAN-verify-build-provenance-and-close-0015-open-items.md
// Q1 step 6: KeptBackups lists the kept backups beside the target, and
// nothing else (0017-PLAN N10).

// TestKeptBackups: only regular files named .<base>.selfupdate-kept-<digits>,
// sorted by name; none is nil; and no lock is taken.
func TestKeptBackups(t *testing.T) {
	_, exe := withTempHome(t)
	dir := filepath.Dir(exe)
	inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := inst.KeptBackups(context.Background()); err != nil || got != nil {
		t.Fatalf("with none: %+v, %v; want nil, nil", got, err)
	}
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write(".demo.selfupdate-kept-2", "two")
	write(".demo.selfupdate-kept-10", "ten!")
	// None of these is one.
	write(".demo.selfupdate-kept-x", "x")
	write(".demo.selfupdate-kept-", "x")
	write(".demo.selfupdate-kept-5.exe", "x")
	write(".demo.previous", "x")
	write(".demo.selfupdate.pending", "{}")
	write(".other.selfupdate-kept-3", "x")
	if err := os.Mkdir(filepath.Join(dir, ".demo.selfupdate-kept-4"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(exe, filepath.Join(dir, ".demo.selfupdate-kept-6")); err != nil {
		t.Logf("no symlink here: %v", err)
	}
	// Another session holds the lock: listing still works.
	target, err := inst.ResolveTarget(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := inst.Begin(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sess.Close() }()
	got, err := inst.KeptBackups(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, k := range got {
		if !filepath.IsAbs(k.Path) || k.ModTime.IsZero() {
			t.Fatalf("%+v: want an absolute path and a time", k)
		}
		names = append(names, filepath.Base(k.Path)+":"+string(rune('0'+k.Size)))
	}
	if strings.Join(names, " ") != ".demo.selfupdate-kept-10:4 .demo.selfupdate-kept-2:3" {
		t.Fatalf("KeptBackups = %q", names)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := inst.KeptBackups(cancelled); err == nil {
		t.Fatal("a cancelled context listed")
	}
}

// plainInstaller is an Installer without KeptBackups.
type plainInstaller struct{ inner *StandaloneInstaller }

func (p plainInstaller) ResolveTarget(ctx context.Context) (Target, error) {
	return p.inner.ResolveTarget(ctx)
}

func (p plainInstaller) Begin(ctx context.Context, target Target) (InstallSession, error) {
	return p.inner.Begin(ctx, target)
}

// TestManagedKeptBackups: a managed installer lists through its inner
// installer, and says so when that cannot list.
func TestManagedKeptBackups(t *testing.T) {
	m, _, sess, exe := managedEnv(t, &fakeLife{}, &fakeRec{})
	if err := sess.Close(); err != nil {
		t.Fatal(err)
	}
	kept := filepath.Join(filepath.Dir(exe), ".demo.selfupdate-kept-9")
	if err := os.WriteFile(kept, []byte("old-bytes"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := m.KeptBackups(context.Background())
	if err != nil || len(got) != 1 || !samePath(t, got[0].Path, kept) {
		t.Fatalf("KeptBackups = %+v, %v", got, err)
	}
	plain, err := NewManagedInstallerFor(plainInstaller{m.inner.(*StandaloneInstaller)}, &fakeLife{}, &fakeRec{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plain.KeptBackups(context.Background()); err == nil || !strings.Contains(err.Error(), "does not list kept backups") {
		t.Fatalf("err = %v", err)
	}
}
