package selfupdate

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func TestLockConcurrentAndStaleUnlocked(t *testing.T) {
	t.Parallel()
	home, exe := tempTarget(t)
	inst, err := NewStandaloneInstaller(InstallOptions{
		TargetPolicy: targetPolicy(exe),
		LockTimeout:  50 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	target, err := inst.ResolveTarget(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	s1, err := inst.Begin(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	_, err = inst.Begin(context.Background(), target)
	if !errors.Is(err, ErrConcurrentUpdate) {
		t.Fatalf("second begin err = %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatal(err)
	}
	lockPath := home + string(os.PathSeparator) + lockBasename(target.Base)
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lock file should remain after unlock: %v", err)
	}
	s2, err := inst.Begin(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	if err := s2.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestBeginRejectsChangedTarget(t *testing.T) {
	t.Parallel()
	_, exe := tempTarget(t)
	inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: targetPolicy(exe)})
	if err != nil {
		t.Fatal(err)
	}
	target, err := inst.ResolveTarget(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("changed-bytes"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err = inst.Begin(context.Background(), target)
	if !errors.Is(err, ErrConcurrentUpdate) {
		t.Fatalf("err = %v", err)
	}
}
