package selfupdate

import (
	"context"
	"errors"
	"io"
	"os"
	"syscall"
	"time"
)

// fsOps are the install path's effects on the file system, as function
// fields a test replaces on a value of its own: StandaloneInstaller holds
// one, and each of its sessions shares it
// (docs/decisions/0038-MADR-pinned-plans-lifecycle-conformance-and-bubble-tests.md
// D3-A). The fields keep the names of the package variables they
// replaced, so a test reads as it did.
type fsOps struct {
	osFS
	fileChmod      func(*os.File, os.FileMode) error
	fileSync       func(*os.File) error
	fileClose      func(*os.File) error
	osChmod        func(string, os.FileMode) error
	osRemove       func(string) error
	osLink         func(string, string) error
	syncDirFn      func(string) error
	syncRootFn     func(*os.Root) error
	replacePath    func(context.Context, string, string) error
	writeJournalFn func(*os.Root, Target, pendingJournal) error
	keptRenameFn   func(*os.Root, string, string) error
	leftoverRemove func(*os.Root, string) error
	// osExecutable and userHomeDir locate the target and its default
	// allowed root.
	osExecutable func() (string, error)
	userHomeDir  func() (string, error)
	// lockOpenHook runs at each stage of openLockFile, and afterLockHook
	// right after beginSession takes the lock; tests use them to change
	// the lock or the directory at that moment.
	lockOpenHook  func(stage string)
	afterLockHook func()
}

// realFS returns the real effects, a new value on each call, so what one
// test changes reaches no other. replacePath and writeJournalFn are bound
// to the value's own methods, so they see its other fields.
func realFS() *fsOps {
	fx := &fsOps{
		osFS:           realOSFS(),
		fileChmod:      (*os.File).Chmod,
		fileSync:       (*os.File).Sync,
		fileClose:      (*os.File).Close,
		osChmod:        os.Chmod,
		osRemove:       os.Remove,
		osLink:         os.Link,
		syncDirFn:      syncDirectory,
		syncRootFn:     syncRoot,
		keptRenameFn:   (*os.Root).Rename,
		leftoverRemove: (*os.Root).Remove,
		osExecutable:   os.Executable,
		userHomeDir:    os.UserHomeDir,
		lockOpenHook:   func(string) {},
		afterLockHook:  func() {},
	}
	fx.replacePath = fx.replacePathOS
	fx.writeJournalFn = fx.writeJournal
	return fx
}

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

func (fx *fsOps) copyFile(src, dst string) (err error) {
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
	if chmodErr := fx.fileChmod(out, srcInfo.Mode().Perm()); chmodErr != nil {
		return joinRemove(joinClose(chmodErr, out), dst)
	}
	if syncErr := fx.fileSync(out); syncErr != nil {
		return joinRemove(joinClose(syncErr, out), dst)
	}
	if closeErr := fx.fileClose(out); closeErr != nil {
		return joinRemove(closeErr, dst)
	}
	return nil
}

func (fx *fsOps) backupFile(target, backup string) error {
	if err := fx.osLink(target, backup); err == nil {
		return nil
	}
	return fx.copyFile(target, backup)
}

// chmodStaging gives staging the replaced binary's mode: its permissions
// and sticky bit, and its setuid and setgid bits only when the target's
// policy allowed them. A bit set after the target was resolved is not
// carried (0010-MADR Q6).
func (fx *fsOps) chmodStaging(staging string, target Target, old os.FileInfo) error {
	return fx.osChmod(staging, stagingMode(old.Mode(), target.allowSpecial))
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
