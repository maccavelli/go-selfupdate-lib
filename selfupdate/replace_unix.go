//go:build unix

package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// osFS are fsOps's Unix-only effects.
type osFS struct {
	fileChown func(*os.File, int, int) error
}

func realOSFS() osFS {
	return osFS{fileChown: (*os.File).Chown}
}

// modeOpenFlags opens a file to set its owner and mode: read-only, which
// fchown and fchmod need no more than, and which a running binary's
// hard-linked backup allows; and without following a symlink.
const modeOpenFlags = os.O_RDONLY | syscall.O_NOFOLLOW

// chownStaging gives staging the replaced binary's owner and group. An
// unprivileged updater owns what it writes, and may not give it away: EPERM
// is not an error. Any other failure is (0010-MADR Q6).
func (fx *fsOps) chownStaging(staging *os.File, old os.FileInfo) error {
	st, ok := old.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	if err := fx.fileChown(staging, int(st.Uid), int(st.Gid)); err != nil && !errors.Is(err, syscall.EPERM) {
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

func (*fsOps) replacePathOS(_ context.Context, root *os.Root, oldname, newname string) error {
	return root.Rename(oldname, newname)
}

// replaceTarget replaces target with staging, both names in root, keeping
// a backup. When beforeRename is set it runs once the backup exists and
// before the rename, with the backup's path and the previous binary's
// digest; its error leaves the target untouched (0017-PLAN N7).
func (fx *fsOps) replaceTarget(ctx context.Context, root *os.Root, target Target, staging string, beforeRename func(backup, oldDigest string) error) (applyResult, error) {
	info, err := lockedTarget(root, target)
	if err != nil {
		return applyResult{}, err
	}
	if err := fx.setStagingMode(root, staging, target, info); err != nil {
		return applyResult{}, err
	}
	oldDigest, err := rootFileSHA256(root, target.Base, info)
	if err != nil {
		return applyResult{}, fmt.Errorf("selfupdate: hash target: %w", err)
	}
	name, err := fx.backupFile(root, target)
	if err != nil {
		return applyResult{}, fmt.Errorf("selfupdate: backup target: %w", err)
	}
	backup := filepath.Join(target.Dir, name)
	if beforeRename != nil {
		if err := beforeRename(backup, oldDigest); err != nil {
			return applyResult{}, joinRootRemove(fmt.Errorf("selfupdate: write the journal: %w", err), root, name)
		}
	}
	if err := fx.replacePath(ctx, root, staging, target.Base); err != nil {
		return applyResult{}, joinRootRemove(fmt.Errorf("selfupdate: rename staging over target: %w", err), root, name)
	}
	if err := fx.syncRootFn(root); err != nil {
		syncErr := fmt.Errorf("selfupdate: sync directory: %w", err)
		// The restore is recovery: the caller's cancellation must not
		// abandon it with the new binary live (0004-MADR R4).
		if rerr := fx.replacePath(context.WithoutCancel(ctx), root, name, target.Base); rerr != nil {
			// The new binary is live and the backup is kept: report both, so
			// neither the failed restore nor the backup is lost (0003-MADR B1).
			return applyResult{backup: backup, renamed: true},
				errors.Join(syncErr, fmt.Errorf("selfupdate: restore backup: %w", rerr))
		}
		// The previous binary is back (0015-MADR B4).
		return applyResult{renamed: true}, errors.Join(syncErr, errRolledBack)
	}
	return applyResult{backup: backup, renamed: true}, nil
}

// keepAsPending applies on Windows only: a Unix rename replaces a running
// binary's name (0010-MADR B6).
func (fx *fsOps) keepAsPending(context.Context, *os.Root, Target, applyResult, error) (pending string, ok bool, kerr error) {
	return "", false, nil
}

func (fx *fsOps) commitReplacement(_ context.Context, root *os.Root, _ Target, result applyResult) (pending string, err error) {
	if result.backup == "" {
		return "", nil
	}
	if err := fx.rootRemove(root, filepath.Base(result.backup)); err != nil {
		return "", fmt.Errorf("selfupdate: remove backup: %w", err)
	}
	if err := fx.syncRootFn(root); err != nil {
		return "", err
	}
	return "", nil
}

func (fx *fsOps) rollbackReplacement(ctx context.Context, root *os.Root, target Target, result applyResult) error {
	if result.backup == "" {
		return fmt.Errorf("selfupdate: no backup to restore")
	}
	backup := filepath.Base(result.backup)
	if err := fx.replacePath(ctx, root, backup, target.Base); err != nil {
		return fmt.Errorf("selfupdate: restore backup: %w", err)
	}
	dropSameBackup(root, backup, target.Base)
	if err := fx.syncRootFn(root); err != nil {
		return errors.Join(errRestoredUnsynced, err)
	}
	return nil
}
