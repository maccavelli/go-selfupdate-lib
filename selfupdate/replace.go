package selfupdate

import (
	"context"
	"errors"
	"io"
	"os"
	"syscall"
	"time"
)

var (
	fileChmod   = (*os.File).Chmod
	fileSync    = (*os.File).Sync
	fileClose   = (*os.File).Close
	osChmod     = os.Chmod
	osRemove    = os.Remove
	osLink      = os.Link
	syncDirFn   = syncDirectory
	replacePath = replacePathOS
)

func randomSibling(dir, prefix string) (string, error) {
	f, err := os.CreateTemp(dir, prefix)
	if err != nil {
		return "", err
	}
	name := f.Name()
	if err := f.Close(); err != nil {
		return "", joinRemove(err, name)
	}
	if err := os.Remove(name); err != nil {
		return "", err
	}
	return name, nil
}

func copyFile(src, dst string) (err error) {
	in, err := openAbsFile(src, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer func() {
		err = joinClose(err, in)
	}()
	srcInfo, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := openAbsFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, copyErr := io.Copy(out, in); copyErr != nil {
		return joinRemove(joinClose(copyErr, out), dst)
	}
	// The backup is what a rollback restores: it keeps the executable's
	// mode, not the creation default (0003-MADR B3).
	if chmodErr := fileChmod(out, srcInfo.Mode().Perm()); chmodErr != nil {
		return joinRemove(joinClose(chmodErr, out), dst)
	}
	if syncErr := fileSync(out); syncErr != nil {
		return joinRemove(joinClose(syncErr, out), dst)
	}
	if closeErr := fileClose(out); closeErr != nil {
		return joinRemove(closeErr, dst)
	}
	return nil
}

func backupFile(target, backup string) error {
	if err := osLink(target, backup); err == nil {
		return nil
	}
	return copyFile(target, backup)
}

// chmodStaging gives staging the replaced binary's mode: its permissions
// and sticky bit, and its setuid and setgid bits only when the target's
// policy allowed them. A bit set after the target was resolved is not
// carried (0010-MADR Q6).
func chmodStaging(staging string, target Target, old os.FileInfo) error {
	return osChmod(staging, stagingMode(old.Mode(), target.allowSpecial))
}

func stagingMode(old os.FileMode, allowSpecial bool) os.FileMode {
	mode := old.Perm() | old&os.ModeSticky
	if allowSpecial {
		mode |= old & (os.ModeSetuid | os.ModeSetgid)
	}
	return mode
}

func syncDirectory(dir string) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	return errors.Join(syncRoot(root), root.Close())
}

// syncRoot flushes the directory root is anchored to, whatever path now
// names it.
func syncRoot(root *os.Root) error {
	f, err := root.Open(".")
	if err != nil {
		if isUnsupportedSync(err) {
			return nil
		}
		return err
	}
	syncErr := joinClose(f.Sync(), f)
	if syncErr == nil || isUnsupportedSync(syncErr) {
		return nil
	}
	return syncErr
}

// retryBudgetKey carries the session's lock timeout to the Windows
// busy-image retry, which sits below the replacePath seam (0004-MADR R4).
type retryBudgetKey struct{}

func withRetryBudget(ctx context.Context, d time.Duration) context.Context {
	return context.WithValue(ctx, retryBudgetKey{}, d)
}

func isUnsupportedSync(err error) bool {
	return errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOENT) || isUnsupportedDirSync(err)
}
