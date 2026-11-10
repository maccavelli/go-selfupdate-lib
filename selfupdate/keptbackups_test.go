package selfupdate

import (
	"context"
	"errors"
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
	t.Parallel()
	_, exe := tempTarget(t)
	dir := filepath.Dir(exe)
	inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: targetPolicy(exe)})
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

// TestMaintenanceThroughSymlink: for a program started through a link,
// found by os.Executable or by os.Args[0], CleanupPending does nothing and
// returns nil, and KeptBackups lists the backups beside the link's target.
// An ExecutablePath that is the link is still refused by both, and a link
// that does not resolve leaves KeptBackups nothing to list
// (docs/decisions/0041-MADR-refuse-symlink-invocation-on-every-platform.md
// Amendment A1 D5-A).
func TestMaintenanceThroughSymlink(t *testing.T) {
	t.Parallel()
	_, exe := tempTarget(t)
	dir := filepath.Dir(exe)
	link := filepath.Join(t.TempDir(), "demo")
	symlinkOrSkip(t, exe, link)
	dangling := filepath.Join(t.TempDir(), "demo")
	symlinkOrSkip(t, filepath.Join(dir, "missing"), dangling)
	if err := os.WriteFile(filepath.Join(dir, ".demo.selfupdate-kept-1"), []byte("kept"), 0o755); err != nil {
		t.Fatal(err)
	}
	installer := func(policy TargetPolicy, executable, arg0 string) *StandaloneInstaller {
		t.Helper()
		inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: policy})
		if err != nil {
			t.Fatal(err)
		}
		fx := fsFor(inst)
		fx.osExecutable = func() (string, error) { return executable, nil }
		fx.osArgs0 = func() string { return arg0 }
		return inst
	}
	ctx := context.Background()
	roots := TargetPolicy{AllowedRoots: []string{dir}}
	for name, inst := range map[string]*StandaloneInstaller{
		"executable": installer(roots, link, ""),
		"invocation": installer(roots, exe, link),
	} {
		t.Run(name, func(t *testing.T) {
			before := dirNames(t, dir)
			if err := inst.CleanupPending(ctx); err != nil {
				t.Fatalf("CleanupPending = %v, want nil", err)
			}
			if after := dirNames(t, dir); after != before || strings.Contains(after, ".demo.selfupdate.lock") {
				t.Fatalf("CleanupPending changed the target's directory:\nbefore %s\nafter  %s", before, after)
			}
			got, err := inst.KeptBackups(ctx)
			if err != nil || len(got) != 1 || filepath.Base(got[0].Path) != ".demo.selfupdate-kept-1" {
				t.Fatalf("KeptBackups = %+v, %v; want the one kept backup", got, err)
			}
			if _, err := inst.ResolveTarget(ctx); !errors.Is(err, ErrSymlinkInvocation) {
				t.Fatalf("ResolveTarget = %v, want the refusal", err)
			}
		})
	}
	t.Run("policy", func(t *testing.T) {
		inst := installer(TargetPolicy{ExecutablePath: link, AllowedRoots: []string{dir}}, exe, "")
		var refusal *SymlinkInvocationError
		if err := inst.CleanupPending(ctx); !errors.As(err, &refusal) || refusal.Source != SymlinkSourcePolicy {
			t.Fatalf("CleanupPending = %v, want the policy's refusal", err)
		}
		if _, err := inst.KeptBackups(ctx); !errors.As(err, &refusal) || refusal.Source != SymlinkSourcePolicy {
			t.Fatalf("KeptBackups = %v, want the policy's refusal", err)
		}
	})
	t.Run("dangling", func(t *testing.T) {
		inst := installer(roots, dangling, "")
		if err := inst.CleanupPending(ctx); err != nil {
			t.Fatalf("CleanupPending = %v, want nil", err)
		}
		if got, err := inst.KeptBackups(ctx); !errors.Is(err, ErrSymlinkInvocation) {
			t.Fatalf("KeptBackups = %+v, %v; want the refusal", got, err)
		}
	})
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
	t.Parallel()
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
