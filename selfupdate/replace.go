package selfupdate

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strconv"
	"syscall"
	"time"
)

// fsOps are the install path's effects on the file system, as function
// fields a test replaces on a value of its own: StandaloneInstaller holds
// one, and each of its sessions shares it
// (docs/decisions/0038-MADR-pinned-plans-lifecycle-conformance-and-bubble-tests.md
// D3-A). The fields keep the names of the package variables they
// replaced, so a test reads as it did. Every name they take is relative to
// the session's root, so a directory swapped in after the lock is never
// written to (0038-MADR F8).
type fsOps struct {
	osFS
	fileChmod      func(*os.File, os.FileMode) error
	fileSync       func(*os.File) error
	fileClose      func(*os.File) error
	rootRemove     func(*os.Root, string) error
	rootLink       func(*os.Root, string, string) error
	syncRootFn     func(*os.Root) error
	replacePath    func(ctx context.Context, root *os.Root, oldname, newname string) error
	writeJournalFn func(*os.Root, Target, pendingJournal) error
	keptRenameFn   func(*os.Root, string, string) error
	leftoverRemove func(*os.Root, string) error
	// osExecutable and userHomeDir locate the target and its default
	// allowed root.
	osExecutable func() (string, error)
	userHomeDir  func() (string, error)
	// lockOpenHook runs at each stage of openLockFile, afterLockHook
	// right after beginSession takes the lock, and afterCheckHook right
	// after a replacement checks the directory; tests use them to change
	// the lock or the directory at that moment.
	lockOpenHook   func(stage string)
	afterLockHook  func()
	afterCheckHook func()
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
		rootRemove:     (*os.Root).Remove,
		rootLink:       (*os.Root).Link,
		syncRootFn:     syncRoot,
		keptRenameFn:   (*os.Root).Rename,
		leftoverRemove: (*os.Root).Remove,
		osExecutable:   os.Executable,
		userHomeDir:    os.UserHomeDir,
		lockOpenHook:   func(string) {},
		afterLockHook:  func() {},
		afterCheckHook: func() {},
	}
	fx.replacePath = fx.replacePathOS
	fx.writeJournalFn = fx.writeJournal
	return fx
}

// maxTempTries bounds the search for a free name, as os.CreateTemp's does.
const maxTempTries = 10000

// randomDigits is a random decimal number: the suffix os.CreateTemp gives a
// name, which isLeftover and keptName parse.
func randomDigits() string {
	var b [4]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read never returns an error
	return strconv.FormatUint(uint64(binary.LittleEndian.Uint32(b[:])), 10)
}

// rootCreateTemp creates a new file in root, named prefix, a random decimal
// number and suffix, as os.CreateTemp does by path: with O_EXCL, so it
// never opens a file that was there, and mode 0o600. os.Root has no
// CreateTemp (0038-MADR F8).
func rootCreateTemp(root *os.Root, prefix, suffix string) (*os.File, string, error) {
	for range maxTempTries {
		name := prefix + randomDigits() + suffix
		f, err := root.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return nil, "", err
		}
		return f, name, nil
	}
	return nil, "", &fs.PathError{Op: "createtemp", Path: prefix + "*" + suffix, Err: fs.ErrExist}
}

// rootReserveName returns a name in root, prefix and a random decimal
// number, that was free: its file is created and removed again, so a
// rename can take the name.
func rootReserveName(root *os.Root, prefix string) (string, error) {
	f, name, err := rootCreateTemp(root, prefix, "")
	if err != nil {
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", joinRootRemove(err, root, name)
	}
	if err := root.Remove(name); err != nil {
		return "", err
	}
	return name, nil
}

// joinRootRemove is joinRemove for a name in root.
func joinRootRemove(err error, root *os.Root, name string) error {
	if name == "" {
		return err
	}
	rerr := root.Remove(name)
	if errors.Is(rerr, fs.ErrNotExist) {
		rerr = nil
	}
	return errors.Join(err, rerr)
}

// copyFile copies src in root to a new file named prefix and a random
// decimal number, and returns the copy's name.
func (fx *fsOps) copyFile(root *os.Root, src, prefix string) (name string, err error) {
	in, err := root.Open(src)
	if err != nil {
		return "", err
	}
	defer func() {
		err = joinClose(err, in)
	}()
	srcInfo, err := in.Stat()
	if err != nil {
		return "", err
	}
	out, name, err := rootCreateTemp(root, prefix, "")
	if err != nil {
		return "", err
	}
	if _, copyErr := io.Copy(out, in); copyErr != nil {
		return "", joinRootRemove(joinClose(copyErr, out), root, name)
	}
	// The backup is what a rollback restores: it keeps the executable's
	// mode, not the creation default (0003-MADR B3).
	if chmodErr := fx.fileChmod(out, srcInfo.Mode().Perm()); chmodErr != nil {
		return "", joinRootRemove(joinClose(chmodErr, out), root, name)
	}
	if syncErr := fx.fileSync(out); syncErr != nil {
		return "", joinRootRemove(joinClose(syncErr, out), root, name)
	}
	if closeErr := fx.fileClose(out); closeErr != nil {
		return "", joinRootRemove(closeErr, root, name)
	}
	return name, nil
}

// backupFile backs up target's file in root under a fresh backup name, and
// returns it: a hard link, retried on a name that is taken, or a copy when
// the link fails otherwise.
func (fx *fsOps) backupFile(root *os.Root, target Target) (string, error) {
	prefix := backupPrefix(target.Base)
	for range maxTempTries {
		name := prefix + randomDigits()
		err := fx.rootLink(root, target.Base, name)
		if err == nil {
			return name, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return fx.copyFile(root, target.Base, prefix)
		}
	}
	return "", &fs.PathError{Op: "link", Path: prefix + "*", Err: fs.ErrExist}
}

// openChecked opens name in root to set its owner or mode, refusing a file
// other than the one info describes: a name replaced since info was read,
// or a symlink, which Lstat reports as itself. The owner and mode are then
// set through the descriptor, which a swap cannot redirect, rather than by
// name (0038-MADR F8).
func openChecked(root *os.Root, name string, info os.FileInfo) (*os.File, error) {
	f, err := root.OpenFile(name, modeOpenFlags, 0)
	if err != nil {
		return nil, err
	}
	got, err := f.Stat()
	if err != nil {
		return nil, joinClose(err, f)
	}
	if !os.SameFile(info, got) {
		return nil, joinClose(fmt.Errorf("selfupdate: %s changed while its mode was set: %w", name, ErrConcurrentUpdate), f)
	}
	return f, nil
}

// setStagingMode gives staging in root the replaced binary's owner and
// mode, through one descriptor. Owner first: a chown clears setuid and
// setgid, which the chmod then sets.
func (fx *fsOps) setStagingMode(root *os.Root, staging string, target Target, old os.FileInfo) error {
	info, err := root.Lstat(staging)
	if err != nil {
		return fmt.Errorf("selfupdate: stat staging: %w", err)
	}
	f, err := openChecked(root, staging, info)
	if err != nil {
		return fmt.Errorf("selfupdate: open staging: %w", err)
	}
	if err := fx.chownStaging(f, old); err != nil {
		return joinClose(fmt.Errorf("selfupdate: chown staging: %w", err), f)
	}
	if err := fx.chmodStaging(f, target, old); err != nil {
		return joinClose(fmt.Errorf("selfupdate: chmod staging: %w", err), f)
	}
	return f.Close()
}

// chmodStaging gives staging the replaced binary's mode: its permissions
// and sticky bit, and its setuid and setgid bits only when the target's
// policy allowed them. A bit set after the target was resolved is not
// carried (0010-MADR Q6).
func (fx *fsOps) chmodStaging(staging *os.File, target Target, old os.FileInfo) error {
	return fx.fileChmod(staging, stagingMode(old.Mode(), target.allowSpecial))
}

func stagingMode(old os.FileMode, allowSpecial bool) os.FileMode {
	mode := old.Perm() | old&os.ModeSticky
	if allowSpecial {
		mode |= old & (os.ModeSetuid | os.ModeSetgid)
	}
	return mode
}

// dropSameBackup removes backup from root after a rollback's rename, when
// it is still there as the target's own file. Renaming one hard link over
// another of the same file does nothing (POSIX rename), so the backup would
// otherwise stay until the next session's sweep (0038-MADR P2). It is
// advisory: the rollback has succeeded.
func dropSameBackup(root *os.Root, backup, base string) {
	b, err := root.Lstat(backup)
	if err != nil {
		return
	}
	t, err := root.Lstat(base)
	if err != nil || !os.SameFile(b, t) {
		return
	}
	advisory(root.Remove(backup))
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
