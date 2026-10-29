package selfupdate

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestInjectedRenameFailure(t *testing.T) {
	t.Parallel()
	_, exe := tempTarget(t)

	inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: targetPolicy(exe)})
	if err != nil {
		t.Fatal(err)
	}
	fsFor(inst).replacePath = func(ctx context.Context, root *os.Root, oldname, newname string) error {
		return errors.New("injected rename failure")
	}
	target, err := inst.ResolveTarget(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	orig, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := inst.Begin(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sess.Close() }()
	f, path, err := sess.CreateStaging(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("new-bytes")); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = sess.Install(context.Background(), InstallRequest{
		Product:  "demo",
		Artifact: StagedArtifact{Path: path},
	})
	if err == nil {
		t.Fatal("expected injected rename failure")
	}
	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(orig) {
		t.Fatalf("target mutated after failed rename: %q", got)
	}
}

func TestLockSymlinkRejected(t *testing.T) {
	t.Parallel()
	home, exe := tempTarget(t)
	inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: targetPolicy(exe)})
	if err != nil {
		t.Fatal(err)
	}
	target, err := inst.ResolveTarget(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	lock := home + string(os.PathSeparator) + lockBasename(target.Base)
	if err := os.Symlink(exe, lock); err != nil {
		t.Fatal(err)
	}
	_, err = inst.Begin(context.Background(), target)
	if err == nil {
		t.Fatal("accepted lock symlink")
	}
}
