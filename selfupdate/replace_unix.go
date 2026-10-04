//go:build unix

package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
)

var (
	osRename = os.Rename
	osLchown = os.Lchown
)

// chownStaging gives staging the replaced binary's owner and group. An
// unprivileged updater owns what it writes, and may not give it away: EPERM
// is not an error. Any other failure is (0010-MADR Q6).
func chownStaging(staging string, old os.FileInfo) error {
	st, ok := old.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	if err := osLchown(staging, int(st.Uid), int(st.Gid)); err != nil && !errors.Is(err, syscall.EPERM) {
		return err
	}
	return nil
}

type applyResult struct {
	backup string
	// renamed reports that the staging file was consumed by the rename, so
	// the session must no longer remove it (0003-MADR B4).
	renamed bool
}

func isUnsupportedDirSync(error) bool {
	return false
}

func replacePathOS(_ context.Context, oldpath, newpath string) error {
	return osRename(oldpath, newpath)
}

func replaceTarget(ctx context.Context, target Target, staging string) (applyResult, error) {
	info, err := os.Lstat(target.Path)
	if err != nil {
		return applyResult{}, err
	}
	// Owner first: a chown clears setuid and setgid, which the chmod then
	// sets.
	if err := chownStaging(staging, info); err != nil {
		return applyResult{}, fmt.Errorf("selfupdate: chown staging: %w", err)
	}
	if err := chmodStaging(staging, target, info); err != nil {
		return applyResult{}, fmt.Errorf("selfupdate: chmod staging: %w", err)
	}
	backup, err := randomSibling(target.Dir, "."+target.Base+".selfupdate-bak-")
	if err != nil {
		return applyResult{}, fmt.Errorf("selfupdate: allocate backup: %w", err)
	}
	if err := backupFile(target.Path, backup); err != nil {
		return applyResult{}, fmt.Errorf("selfupdate: backup target: %w", err)
	}
	if err := replacePath(ctx, staging, target.Path); err != nil {
		return applyResult{}, joinRemove(fmt.Errorf("selfupdate: rename staging over target: %w", err), backup)
	}
	if err := syncDirFn(target.Dir); err != nil {
		syncErr := fmt.Errorf("selfupdate: sync directory: %w", err)
		// The restore is recovery: the caller's cancellation must not
		// abandon it with the new binary live (0004-MADR R4).
		if rerr := replacePath(context.WithoutCancel(ctx), backup, target.Path); rerr != nil {
			// The new binary is live and the backup is kept: report both, so
			// neither the failed restore nor the backup is lost (0003-MADR B1).
			return applyResult{backup: backup, renamed: true},
				errors.Join(syncErr, fmt.Errorf("selfupdate: restore backup: %w", rerr))
		}
		return applyResult{renamed: true}, syncErr
	}
	return applyResult{backup: backup, renamed: true}, nil
}

// keepAsPending applies on Windows only: a Unix rename replaces a running
// binary's name (0010-MADR B6).
func keepAsPending(Target, applyResult, error) (pending string, ok bool, kerr error) {
	return "", false, nil
}

func commitReplacement(target Target, result applyResult) (pending string, err error) {
	if result.backup == "" {
		return "", nil
	}
	if err := osRemove(result.backup); err != nil {
		return "", fmt.Errorf("selfupdate: remove backup: %w", err)
	}
	if err := syncDirFn(target.Dir); err != nil {
		return "", err
	}
	return "", nil
}

func rollbackReplacement(ctx context.Context, target Target, result applyResult) error {
	if result.backup == "" {
		return fmt.Errorf("selfupdate: no backup to restore")
	}
	if err := replacePath(ctx, result.backup, target.Path); err != nil {
		return fmt.Errorf("selfupdate: restore backup: %w", err)
	}
	if err := syncDirFn(target.Dir); err != nil {
		return errors.Join(errRestoredUnsynced, err)
	}
	return nil
}
