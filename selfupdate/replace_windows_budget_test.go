//go:build windows

package selfupdate

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// TestMoveFileReplaceReadOnlyFailsFast: access denied on a read-only
// destination never clears by waiting, so it is not retried (0004-MADR R4).
func TestMoveFileReplaceReadOnlyFailsFast(t *testing.T) {
	dir := t.TempDir()
	from := filepath.Join(dir, "from")
	to := filepath.Join(dir, "to")
	for _, p := range []string{from, to} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(to, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(to, 0o644) })
	calls := 0
	setSeam(t, &moveFileExFn, func(*uint16, *uint16, uint32) error {
		calls++
		return windows.ERROR_ACCESS_DENIED
	})
	start := time.Now()
	err := realFS().moveFileReplace(context.Background(), from, to)
	if err == nil {
		t.Fatal("want the access-denied error")
	}
	if calls != 1 {
		t.Fatalf("calls = %d after %v; a read-only destination must not be retried", calls, time.Since(start))
	}
}

// TestMoveFileReplaceHonoursRetryBudget: the busy-image retry stops at the
// session's lock timeout, not DefaultLockTimeout (0004-MADR R4).
func TestMoveFileReplaceHonoursRetryBudget(t *testing.T) {
	calls := 0
	setSeam(t, &moveFileExFn, func(*uint16, *uint16, uint32) error {
		calls++
		return windows.ERROR_ACCESS_DENIED
	})
	ctx := withRetryBudget(context.Background(), 50*time.Millisecond)
	start := time.Now()
	err := realFS().moveFileReplace(ctx, `C:\nonexistent\from`, `C:\nonexistent\to`)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("want the access-denied error")
	}
	if calls < 2 {
		t.Fatalf("calls = %d; a busy image must still be retried", calls)
	}
	if elapsed > time.Second {
		t.Fatalf("retried for %v with a 50ms budget (DefaultLockTimeout %v)", elapsed, DefaultLockTimeout)
	}
}
