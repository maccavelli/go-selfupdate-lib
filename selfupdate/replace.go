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
	syncRootFn  = syncRoot
	replacePath func(context.Context, string, string) error
)

// replacePath's default reaches realFS on Windows, and writeJournalFn's
// does everywhere, and realFS's fields read both: an initializer would be
// an initialization cycle, so they are set here, until Q6 of
// docs/decisions/0038-PLAN-pinned-plans-lifecycle-conformance-and-bubble-tests.md
// moves them onto fsOps.
func init() {
	replacePath = replacePathOS
	writeJournalFn = func(root *os.Root, target Target, j pendingJournal) error {
		return realFS().writeJournal(root, target, j)
	}
}

// fsOps are the install path's effects on the file system, as function
// fields a test replaces on a value of its own: StandaloneInstaller holds
// one, and each of its sessions shares it
// (docs/decisions/0038-MADR-pinned-plans-lifecycle-conformance-and-bubble-tests.md
// D3-A). The fields are named for the package variables they replace.
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
	// lockOpenHook runs at each stage of openLockFile, and afterLockHook
	// right after beginSession takes the lock; tests use them to change
	// the lock or the directory at that moment.
	lockOpenHook  func(stage string)
	afterLockHook func()
}

// realFS returns the real effects, a new value on each call. Each field
// calls the package variable it is named for when it runs, so a test that
// sets that variable reaches every session, whenever it set it
// (docs/decisions/0038-PLAN-pinned-plans-lifecycle-conformance-and-bubble-tests.md
// Q5).
func realFS() *fsOps {
	return &fsOps{
		osFS:           realOSFS(),
		fileChmod:      func(f *os.File, mode os.FileMode) error { return fileChmod(f, mode) },
		fileSync:       func(f *os.File) error { return fileSync(f) },
		fileClose:      func(f *os.File) error { return fileClose(f) },
		osChmod:        func(name string, mode os.FileMode) error { return osChmod(name, mode) },
		osRemove:       func(name string) error { return osRemove(name) },
		osLink:         func(oldname, newname string) error { return osLink(oldname, newname) },
		syncDirFn:      func(dir string) error { return syncDirFn(dir) },
		syncRootFn:     func(root *os.Root) error { return syncRootFn(root) },
		replacePath:    func(ctx context.Context, oldpath, newpath string) error { return replacePath(ctx, oldpath, newpath) },
		writeJournalFn: func(root *os.Root, target Target, j pendingJournal) error { return writeJournalFn(root, target, j) },
		keptRenameFn:   func(root *os.Root, oldname, newname string) error { return keptRenameFn(root, oldname, newname) },
		leftoverRemove: func(root *os.Root, name string) error { return leftoverRemove(root, name) },
		lockOpenHook:   func(stage string) { lockOpenHook(stage) },
		afterLockHook: func() {
			if afterLockHook != nil {
				afterLockHook()
			}
		},
	}
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
