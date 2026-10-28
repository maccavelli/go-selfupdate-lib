//go:build windows

package selfupdate

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// TestMoveFileReplaceHonoursContext: a busy replacement stops retrying when
// the caller's context ends, well before DefaultLockTimeout (0003-MADR B10).
func TestMoveFileReplaceHonoursContext(t *testing.T) {
	fx := realFS()
	fx.moveFileExFn = func(*uint16, *uint16, uint32) error { return windows.ERROR_SHARING_VIOLATION }
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := fx.moveFileReplace(ctx, `C:\nonexistent\from`, `C:\nonexistent\to`)
	elapsed := time.Since(start)
	if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
		t.Fatalf("err = %v", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("kept retrying for %v after the context ended (DefaultLockTimeout %v)", elapsed, DefaultLockTimeout)
	}
}

// TestMoveFileReplaceRetriesAccessDenied: a transient ACCESS_DENIED on a
// running image is retried until it clears (0003-MADR B11).
func TestMoveFileReplaceRetriesAccessDenied(t *testing.T) {
	calls := 0
	fx := realFS()
	fx.moveFileExFn = func(*uint16, *uint16, uint32) error {
		calls++
		if calls < 3 {
			return windows.ERROR_ACCESS_DENIED
		}
		return nil
	}
	if err := fx.moveFileReplace(context.Background(), `C:\x\from`, `C:\x\to`); err != nil || calls != 3 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestIsUnsupportedDirSyncAccessDenied(t *testing.T) {
	err := &os.PathError{Op: "sync", Path: `.`, Err: windows.ERROR_ACCESS_DENIED}
	if !isUnsupportedSync(err) {
		t.Fatal("Windows directory ACCESS_DENIED must be treated as unsupported sync")
	}
	if isUnsupportedDirSync(windows.ERROR_INVALID_FUNCTION) {
		t.Fatal("unrelated Windows errors must remain fatal")
	}
}

func TestBusyRunningImageIncludesAccessDenied(t *testing.T) {
	err := &os.PathError{Op: "remove", Path: `old.exe`, Err: windows.ERROR_ACCESS_DENIED}
	if !isBusyRunningImage(err) {
		t.Fatal("deleting a running image that returns ACCESS_DENIED must be PendingBackup")
	}
	if !isBusyRunningImage(windows.ERROR_SHARING_VIOLATION) {
		t.Fatal("SHARING_VIOLATION must remain PendingBackup")
	}
}
