//go:build windows

package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"time"

	"golang.org/x/sys/windows"
)

type applyResult struct {
	backup    string
	oldDigest string
	// renamed reports that the staging file was consumed by the replace, so
	// the session must no longer remove it (0003-MADR B4).
	renamed bool
}

func isUnsupportedDirSync(err error) bool {
	return errors.Is(err, windows.ERROR_ACCESS_DENIED)
}

func (fx *fsOps) replacePathOS(ctx context.Context, oldpath, newpath string) error {
	return fx.moveFileReplace(ctx, oldpath, newpath)
}

// replaceTarget replaces target with staging, keeping a backup. When
// beforeRename is set it runs once the backup exists and before the
// replace, with the backup's path and the previous binary's digest; its
// error leaves the target untouched (0017-PLAN N7).
func (fx *fsOps) replaceTarget(ctx context.Context, target Target, staging string, beforeRename func(backup, oldDigest string) error) (applyResult, error) {
	info, err := lockedTarget(target)
	if err != nil {
		return applyResult{}, err
	}
	if err := fx.chmodStaging(staging, target, info); err != nil {
		return applyResult{}, fmt.Errorf("selfupdate: chmod staging: %w", err)
	}
	oldDigest, err := fileSHA256(target.Path)
	if err != nil {
		return applyResult{}, err
	}
	backup, err := randomSibling(target.Dir, "."+target.Base+".selfupdate-bak-")
	if err != nil {
		return applyResult{}, fmt.Errorf("selfupdate: allocate backup: %w", err)
	}
	if err := fx.backupFile(target.Path, backup); err != nil {
		return applyResult{}, fmt.Errorf("selfupdate: backup target: %w", err)
	}
	if beforeRename != nil {
		if err := beforeRename(backup, oldDigest); err != nil {
			return applyResult{}, joinRemove(fmt.Errorf("selfupdate: write the journal: %w", err), backup)
		}
	}
	if err := fx.replacePath(ctx, staging, target.Path); err != nil {
		return applyResult{}, joinRemove(fmt.Errorf("selfupdate: replace target: %w", err), backup)
	}
	if err := fx.syncDirFn(target.Dir); err != nil && !isUnsupportedSync(err) {
		syncErr := fmt.Errorf("selfupdate: sync directory: %w", err)
		// The restore is recovery: the caller's cancellation must not
		// abandon it with the new binary live. moveFileReplace still bounds
		// it by the retry budget (0004-MADR R4).
		if rerr := fx.replacePath(context.WithoutCancel(ctx), backup, target.Path); rerr != nil {
			// The new binary is live and the backup is kept: report both
			// (0003-MADR B1).
			return applyResult{backup: backup, oldDigest: oldDigest, renamed: true},
				errors.Join(syncErr, fmt.Errorf("selfupdate: restore backup: %w", rerr))
		}
		// The previous binary is back (0015-MADR B4).
		return applyResult{renamed: true}, errors.Join(syncErr, errRolledBack)
	}
	return applyResult{backup: backup, oldDigest: oldDigest, renamed: true}, nil
}

// retryBudget is the bound on retrying a busy replacement: the session's
// lock timeout when one was set, otherwise DefaultLockTimeout.
func retryBudget(ctx context.Context) time.Duration {
	if d, ok := ctx.Value(retryBudgetKey{}).(time.Duration); ok && d > 0 {
		return d
	}
	return DefaultLockTimeout
}

// osFS are fsOps's Windows-only effects.
type osFS struct {
	moveFileExFn func(from, to *uint16, flags uint32) error
}

func realOSFS() osFS {
	return osFS{moveFileExFn: windows.MoveFileEx}
}

// moveFileReplace retries a busy replacement until the retry budget (the
// session's lock timeout) or the caller's context ends, whichever comes
// first (0003-MADR B10, B11; 0004-MADR R4).
func (fx *fsOps) moveFileReplace(ctx context.Context, from, to string) error {
	fromW, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	toW, err := windows.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(retryBudget(ctx))
	var last error
	for {
		last = fx.moveFileExFn(fromW, toW, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
		if last == nil {
			return nil
		}
		// A running image transiently refuses replacement with access
		// denied as well as a sharing violation (0003-MADR B11), so the
		// code alone cannot tell busy from denied. A read-only destination
		// is the one denial that can be told apart: it never clears by
		// waiting (0004-MADR R4).
		if !isBusyRunningImage(last) || time.Now().After(deadline) || isReadOnlyDenial(last, toW) {
			return last
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return errors.Join(last, ctx.Err())
		case <-timer.C:
		}
	}
}

func isSharingViolation(err error) bool {
	return errors.Is(err, windows.ERROR_SHARING_VIOLATION)
}

func isBusyRunningImage(err error) bool {
	return isSharingViolation(err) || errors.Is(err, windows.ERROR_ACCESS_DENIED)
}

// isReadOnlyDenial reports an access-denied refusal whose destination
// carries FILE_ATTRIBUTE_READONLY.
func isReadOnlyDenial(err error, to *uint16) bool {
	if !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		return false
	}
	attrs, aerr := windows.GetFileAttributes(to)
	return aerr == nil && attrs&windows.FILE_ATTRIBUTE_READONLY != 0
}

// keepAsPending puts result's backup on the cleanup receipt when err is a
// running image's refusal (0010-MADR B6). ok reports that it applied.
func (fx *fsOps) keepAsPending(ctx context.Context, target Target, result applyResult, err error) (pending string, ok bool, kerr error) {
	if !isBusyRunningImage(err) {
		return "", false, nil
	}
	if werr := fx.writeCleanupReceipt(ctx, target, result); werr != nil {
		return "", true, errors.Join(fmt.Errorf("selfupdate: keep previous: %w", err), werr)
	}
	return result.backup, true, fx.syncDirFn(target.Dir)
}

func (fx *fsOps) commitReplacement(ctx context.Context, target Target, result applyResult) (pending string, err error) {
	if result.backup == "" {
		return "", nil
	}
	if err := fx.osRemove(result.backup); err != nil {
		if isBusyRunningImage(err) {
			if werr := fx.writeCleanupReceipt(ctx, target, result); werr != nil {
				return "", errors.Join(fmt.Errorf("selfupdate: remove backup: %w", err), werr)
			}
			return result.backup, nil
		}
		return "", fmt.Errorf("selfupdate: remove backup: %w", err)
	}
	return "", fx.syncDirFn(target.Dir)
}

func (fx *fsOps) rollbackReplacement(ctx context.Context, target Target, result applyResult) error {
	if result.backup == "" {
		return fmt.Errorf("selfupdate: no backup to restore")
	}
	if err := fx.replacePath(ctx, result.backup, target.Path); err != nil {
		return fmt.Errorf("selfupdate: restore backup: %w", err)
	}
	if err := fx.syncDirFn(target.Dir); err != nil {
		return errors.Join(errRestoredUnsynced, err)
	}
	return nil
}
