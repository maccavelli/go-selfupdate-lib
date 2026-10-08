package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

type installSession struct {
	target   Target
	root     *os.Root
	lock     lockHandle
	staging  map[string]struct{}
	closed   bool
	mu       sync.Mutex
	closeErr error
	// dirInfo identifies the locked target directory (0003-MADR B10). It
	// comes from the root handle, so its identity does not depend on what
	// the path names later (0004-MADR R3).
	dirInfo os.FileInfo
	// lockTimeout also bounds the Windows busy-image retry (0004-MADR R4).
	lockTimeout time.Duration
	// postInstall runs the installed binary before commit (0004-MADR G9).
	postInstall Prober
	// keepPrevious renames the backup to previousPath at commit instead of
	// removing it (0004-MADR G11).
	keepPrevious bool
}

// previousPath is where KeepPrevious keeps the previous binary. The name
// matches neither backupPrefix nor the lock or receipt names, so a
// cleanup receipt can never name it.
func previousPath(target Target) string {
	return filepath.Join(target.Dir, "."+target.Base+".previous")
}

// stagingSuffix gives staging the executable extension on Windows. exec
// does not need it (a staging name always contains a dot, and the
// Windows host ran an extensionless staging file); it keeps the file
// recognisable as an executable to tools that key on the extension
// (0004-PLAN-v1-1-0-core-api.md Step 8, deviation D3).
func stagingSuffix() string {
	if runtime.GOOS == goosWindows {
		return ".exe"
	}
	return ""
}

// errRolledBack marks a failure whose replacement was undone: the previous
// binary is back in place, after a failed post-install probe, a failed
// directory sync, or a directory swap (0015-MADR B4).
var errRolledBack = errors.New("selfupdate: the replacement was rolled back")

// errReplacementFinished refuses a second Commit or Rollback of one
// replacement (0015-MADR B7).
var errReplacementFinished = errors.New("selfupdate: the replacement was already committed or rolled back")

// dryRunKey marks a dry run's context. Its session sweeps nothing: a dry
// run leaves the target's directory as it found it (0015-MADR B1).
type dryRunKey struct{}

func withDryRun(ctx context.Context) context.Context {
	return context.WithValue(ctx, dryRunKey{}, true)
}

func isDryRun(ctx context.Context) bool {
	v, ok := ctx.Value(dryRunKey{}).(bool)
	return ok && v
}

// errRestoredUnsynced marks a rollback whose rename restored the backup and
// whose directory sync then failed: the backup no longer exists
// (0010-MADR B9).
var errRestoredUnsynced = errors.New("selfupdate: the backup was restored, but the directory sync failed")

func (s *installSession) Target() Target {
	return s.target
}

func (s *installSession) CreateStaging(ctx context.Context) (*os.File, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, "", fmt.Errorf("selfupdate: session is closed")
	}
	f, err := os.CreateTemp(s.target.Dir, "."+s.target.Base+".selfupdate-*"+stagingSuffix())
	if err != nil {
		return nil, "", fmt.Errorf("selfupdate: create staging: %w", err)
	}
	name := f.Name()
	base := filepath.Base(name)
	if _, err := s.root.Lstat(base); err != nil {
		err = fmt.Errorf("selfupdate: staging escaped target directory: %w", err)
		err = joinClose(err, f)
		return nil, "", joinRemove(err, name)
	}
	if s.staging == nil {
		s.staging = make(map[string]struct{})
	}
	s.staging[name] = struct{}{}
	return f, name, nil
}

// ownsLocked reports a staging path this session created. The caller
// holds s.mu.
func (s *installSession) ownsLocked(path string) bool {
	_, ok := s.staging[path]
	return ok
}

// Owns implements StagingOwner.
func (s *installSession) Owns(path string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ownsLocked(path)
}

// errForeignReplacement rejects an AppliedReplacement whose State this
// session did not produce.
var errForeignReplacement = errors.New("selfupdate: replacement was not applied by this session")

// replacement is an AppliedReplacement's State: the session that applied
// it, what it applied, and whether Commit or Rollback has finished it. It
// is a pointer, so the copies a caller holds see a kept backup's new name
// and the finish (0015-MADR B1, B7).
type replacement struct {
	sess     *installSession
	applied  applyResult
	finished bool
}

// stateOf returns the replacement a's State holds, refusing one this
// session did not apply.
func (s *installSession) stateOf(a AppliedReplacement) (*replacement, error) {
	st, ok := a.State.(*replacement)
	if !ok || st == nil || st.sess != s {
		return nil, errForeignReplacement
	}
	return st, nil
}

func (s *installSession) Install(ctx context.Context, req InstallRequest) (InstallResult, error) {
	if err := ctx.Err(); err != nil {
		return InstallResult{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	applied, err := s.replaceLocked(ctx, req.Artifact.Path)
	if err != nil {
		if errors.Is(err, errRolledBack) {
			return InstallResult{Target: s.target.Path, RolledBack: true}, err
		}
		// Backup is non-empty only when the new binary is live and the
		// restore failed (0003-MADR B1). It is kept: it is the only copy of
		// the previous binary (0015-MADR B1).
		return InstallResult{Target: s.target.Path, Backup: s.retainLocked(applied.backup)}, err
	}
	if err := s.checkDir(); err != nil {
		// The rename went into the locked directory, wherever it is now:
		// undo it there, through the handle (0004-MADR R3).
		if rerr := s.rollbackInRoot(applied); rerr != nil {
			if errors.Is(rerr, errRestoredUnsynced) {
				// The backup is back in place; only its sync failed
				// (0015-MADR B4).
				return InstallResult{Target: s.target.Path, RolledBack: true}, errors.Join(err, rerr)
			}
			// Not applied: the backup is the only copy of the previous
			// binary, reported as the restore failure above is
			// (0010-MADR B8).
			return InstallResult{Target: s.target.Path, Backup: s.retainLocked(applied.backup)}, errors.Join(err, rerr)
		}
		return InstallResult{Target: s.target.Path, RolledBack: true}, err
	}
	if err := s.probeInstalled(ctx, req, applied); err != nil {
		if errors.Is(err, errRolledBack) {
			return InstallResult{Target: s.target.Path, RolledBack: true}, err
		}
		return InstallResult{Target: s.target.Path, Backup: s.retainLocked(applied.backup)}, err
	}
	pending, previous, err := s.commitLocked(ctx, applied)
	return InstallResult{
		Target:        s.target.Path,
		Backup:        applied.backup,
		Applied:       true,
		PendingBackup: pending,
		Previous:      previous,
	}, err
}

// commitLocked finishes a live replacement: it removes the backup or, with
// keepPrevious, renames it to previousPath over any older one. On Windows
// the backup is a hard link to the running image, and renaming it works
// while that image runs (0004-PLAN-v1-1-0-core-api.md Step 10). The
// caller holds s.mu.
func (s *installSession) commitLocked(ctx context.Context, applied applyResult) (pending, previous string, err error) {
	if !s.keepPrevious || applied.backup == "" {
		pending, err = commitReplacement(s.target, applied)
		return pending, "", err
	}
	previous = previousPath(s.target)
	if err := clearSpecialBits(applied.backup); err != nil {
		// A privileged copy must not stay at a predictable path: the
		// backup goes (0015-MADR B5).
		return "", "", errors.Join(fmt.Errorf("selfupdate: keep previous: %w", err), os.Remove(applied.backup))
	}
	if err := replacePath(withRetryBudget(ctx, s.lockTimeout), applied.backup, previous); err != nil {
		// A .previous a running image holds cannot be replaced: on Windows
		// the new backup goes on the cleanup receipt instead, and the
		// commit stands (0010-MADR B6).
		if pending, ok, kerr := keepAsPending(s.target, applied, err); ok {
			return pending, "", kerr
		}
		return "", "", fmt.Errorf("selfupdate: keep previous: %w", err)
	}
	return "", previous, syncDirFn(s.target.Dir)
}

// Apply implements TwoPhaseSession: it replaces the target, keeps the
// backup, and runs the post-install probe. A failed Apply returns a
// Backup only when the previous binary still needs restoring.
func (s *installSession) Apply(ctx context.Context, req InstallRequest) (AppliedReplacement, error) {
	if err := ctx.Err(); err != nil {
		return AppliedReplacement{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st := &replacement{sess: s}
	applied, err := s.replaceLocked(ctx, req.Artifact.Path)
	if err == nil {
		// The directory again, now that the rename has gone into it: Install
		// checks here too (0004-MADR R3; 0010-MADR B7).
		if derr := s.checkDir(); derr != nil {
			rerr := s.rollbackInRoot(applied)
			switch {
			case rerr == nil:
				err = errors.Join(derr, errRolledBack)
			case errors.Is(rerr, errRestoredUnsynced):
				// The backup is back in place; only its sync failed
				// (0015-MADR B4).
				err = errors.Join(derr, rerr, errRolledBack)
			default:
				err = errors.Join(derr, rerr)
			}
			if rerr == nil || errors.Is(rerr, errRestoredUnsynced) {
				applied = applyResult{}
			}
		} else if err = s.probeInstalled(ctx, req, applied); errors.Is(err, errRolledBack) {
			applied = applyResult{}
		}
	}
	if err != nil && applied.backup != "" {
		// The new binary is live and the restore failed: the backup is the
		// only copy of the previous binary, and is kept (0015-MADR B1).
		applied.backup = s.retainLocked(applied.backup)
	}
	st.applied = applied
	// A failure that left no live backup has nothing for Commit or
	// Rollback to finish (0015-MADR B7).
	st.finished = err != nil && applied.backup == ""
	return AppliedReplacement{Target: s.target.Path, Backup: applied.backup, State: st}, err
}

// probeInstalled runs the post-install probe on the replaced target. On
// failure it rolls the replacement back; the error wraps errRolledBack when
// that succeeded. The caller holds s.mu.
func (s *installSession) probeInstalled(ctx context.Context, req InstallRequest, applied applyResult) error {
	if s.postInstall == nil {
		return nil
	}
	perr := s.postInstall.Probe(ctx, ProbeRequest{
		Product: req.Product, TargetVersion: req.TargetVersion, Path: s.target.Path, Phase: ProbeInstalled,
	})
	if perr == nil {
		return nil
	}
	perr = fmt.Errorf("selfupdate: installed binary failed its probe: %w", perr)
	rctx := withRetryBudget(context.WithoutCancel(ctx), s.lockTimeout)
	if rerr := rollbackReplacement(rctx, s.target, applied); rerr != nil {
		if errors.Is(rerr, errRestoredUnsynced) {
			// The backup is back in place; only its sync failed. It is
			// rolled back, and no backup is left to report (0010-MADR B9).
			return errors.Join(perr, rerr, errRolledBack)
		}
		return errors.Join(perr, rerr)
	}
	return errors.Join(perr, errRolledBack)
}

// replaceLocked replaces the target with an owned staging file. The caller
// holds s.mu. Staging is deregistered only once the rename has consumed it,
// so a failure before that leaves it for Close to remove (0003-MADR B4).
func (s *installSession) replaceLocked(ctx context.Context, path string) (applyResult, error) {
	if s.closed {
		return applyResult{}, fmt.Errorf("selfupdate: session is closed")
	}
	if !s.ownsLocked(path) {
		return applyResult{}, fmt.Errorf("selfupdate: artifact is not owned by this session")
	}
	// The directory first: once it has been swapped, every path below it
	// names something other than what was locked.
	if err := s.checkDir(); err != nil {
		return applyResult{}, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return applyResult{}, fmt.Errorf("selfupdate: stat staging: %w", err)
	}
	if !info.Mode().IsRegular() {
		return applyResult{}, fmt.Errorf("selfupdate: staging is not a regular file")
	}
	applied, err := replaceTarget(withRetryBudget(ctx, s.lockTimeout), s.target, path)
	if applied.renamed {
		delete(s.staging, path)
	}
	return applied, err
}

// checkDir requires the target directory to be the one the session locked:
// a directory swapped in after Begin would put the replacement outside the
// lock (0003-MADR B10).
func (s *installSession) checkDir() error {
	if s.dirInfo == nil {
		return nil
	}
	cur, err := os.Stat(s.target.Dir)
	if err != nil || !os.SameFile(s.dirInfo, cur) {
		return fmt.Errorf("selfupdate: target directory changed during the update: %w", ErrConcurrentUpdate)
	}
	return nil
}

// Commit implements TwoPhaseSession: it removes the backup of a
// replacement this session applied.
func (s *installSession) Commit(ctx context.Context, a AppliedReplacement) (InstallResult, error) {
	st, err := s.stateOf(a)
	if err != nil {
		return InstallResult{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// After Close the lock is released, and another session may hold the
	// target (0010-MADR B10).
	if s.closed {
		return InstallResult{}, fmt.Errorf("selfupdate: session is closed")
	}
	if st.finished {
		return InstallResult{}, errReplacementFinished
	}
	// The replacement is live either way: a refused commit leaves the new
	// binary and its backup in place, for a Rollback.
	pending, previous := "", ""
	if err = s.checkDir(); err == nil {
		pending, previous, err = s.commitLocked(ctx, st.applied)
		st.finished = true
	}
	return InstallResult{
		Target:        s.target.Path,
		Backup:        st.applied.backup,
		Applied:       true,
		PendingBackup: pending,
		Previous:      previous,
	}, err
}

// Rollback implements TwoPhaseSession: it restores the backup of a
// replacement this session applied. When the restore fails, the backup is
// kept, under the name backupOf reports (0015-MADR B1).
func (s *installSession) Rollback(ctx context.Context, a AppliedReplacement) error {
	st, err := s.stateOf(a)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("selfupdate: session is closed")
	}
	if st.finished {
		return errReplacementFinished
	}
	// A directory swapped since Begin: the backup is in the locked
	// directory, wherever it is now, so the undo goes through its handle
	// (0004-MADR R3; 0010-MADR B7).
	if s.checkDir() != nil {
		err = s.rollbackInRoot(st.applied)
	} else {
		err = rollbackReplacement(withRetryBudget(ctx, s.lockTimeout), s.target, st.applied)
	}
	if err == nil || errors.Is(err, errRestoredUnsynced) {
		st.finished = true
		return err
	}
	st.applied.backup = s.retainLocked(st.applied.backup)
	return err
}

// retainLocked renames a backup that is the only copy of the previous
// binary to its kept name, which no later session sweeps (0015-MADR B1),
// and returns its path. It returns backup unchanged when there is none,
// when the name is not a backup's, when the kept name is taken, or when the
// rename fails. The caller holds s.mu.
func (s *installSession) retainLocked(backup string) string {
	if backup == "" {
		return ""
	}
	kept, ok := keptName(s.target.Base, filepath.Base(backup))
	if !ok {
		return backup
	}
	if _, err := s.root.Lstat(kept); err == nil {
		return backup
	}
	if err := s.root.Rename(filepath.Base(backup), kept); err != nil {
		return backup
	}
	advisory(syncRootFn(s.root))
	path := filepath.Join(filepath.Dir(backup), kept)
	// The only copy of the previous binary is kept, without setuid or
	// setgid; a restore must set them again (0015-MADR B5).
	advisory(clearSpecialBits(path))
	return path
}

// clearSpecialBits drops setuid and setgid from a copy of the previous
// binary, keeping its permissions and the sticky bit, so the copy an
// update replaced is not left privileged (0015-MADR B5).
func clearSpecialBits(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	mode := info.Mode()
	if mode&(os.ModeSetuid|os.ModeSetgid) == 0 {
		return nil
	}
	return osChmod(path, mode.Perm()|mode&os.ModeSticky)
}

// backupOf is a's backup as it is now: a session of this package renames a
// backup it keeps, and records the new name in the State (0015-MADR B1).
// Another TwoPhaseSession's is a.Backup.
func backupOf(a AppliedReplacement) string {
	st, ok := a.State.(*replacement)
	if !ok || st == nil || st.sess == nil {
		return a.Backup
	}
	st.sess.mu.Lock()
	defer st.sess.mu.Unlock()
	return st.applied.backup
}

// rollbackInRoot restores the backup over the target inside the locked
// directory through its handle. The caller holds s.mu.
func (s *installSession) rollbackInRoot(applied applyResult) error {
	if applied.backup == "" {
		return fmt.Errorf("selfupdate: no backup to restore")
	}
	if err := s.root.Rename(filepath.Base(applied.backup), s.target.Base); err != nil {
		return fmt.Errorf("selfupdate: restore backup: %w", err)
	}
	if err := syncRootFn(s.root); err != nil {
		// The rename restored the backup: no backup is left to report
		// (0015-MADR B4, as rollbackReplacement does since 0010-MADR B9).
		return errors.Join(errRestoredUnsynced, err)
	}
	return nil
}

func (s *installSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return s.closeErr
	}
	s.closed = true
	var errs []error
	for name := range s.staging {
		if err := os.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	s.staging = nil
	if s.root != nil {
		if err := s.root.Close(); err != nil {
			errs = append(errs, err)
		}
		s.root = nil
	}
	if err := s.lock.release(); err != nil {
		errs = append(errs, err)
	}
	s.closeErr = errors.Join(errs...)
	return s.closeErr
}

// afterLockHook runs, when set, right after beginSession takes the lock.
// Tests use it to change the directory at that moment.
var afterLockHook func()

func beginSession(ctx context.Context, policy TargetPolicy, original Target, timeout time.Duration) (*installSession, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(original.Dir)
	if err != nil {
		return nil, fmt.Errorf("selfupdate: open target directory: %w", err)
	}
	lock, err := acquireLock(ctx, root, original.Base, timeout)
	if err != nil {
		err = joinClose(err, root)
		if errors.Is(err, ErrConcurrentUpdate) {
			return nil, err
		}
		return nil, fmt.Errorf("selfupdate: acquire lock: %w", err)
	}
	if afterLockHook != nil {
		afterLockHook()
	}
	// The directory opened as root must be the one at the path before
	// anything is read or removed in it, receipt included (0004-MADR R5).
	// The handle's identity is what later steps re-check (0004-MADR R3).
	rootInfo, err := root.Stat(".")
	if err != nil {
		return nil, errors.Join(fmt.Errorf("selfupdate: stat target directory: %w", err), lock.release(), root.Close())
	}
	pathInfo, err := os.Stat(original.Dir)
	if err != nil || !os.SameFile(rootInfo, pathInfo) {
		return nil, errors.Join(fmt.Errorf("selfupdate: target directory changed while locking: %w", ErrConcurrentUpdate), lock.release(), root.Close())
	}
	// A dry run changes nothing beside the target, so it neither processes
	// the receipt nor sweeps (0015-MADR B1).
	if !isDryRun(ctx) {
		if err := processCleanupReceipt(original, root); err != nil {
			return nil, errors.Join(err, lock.release(), root.Close())
		}
		// Under the lock, what a crashed update left behind is no one's
		// (0010-MADR Q6).
		keep, sweepBackups := listedBackups(original)
		removeLeftovers(original, root, keep, sweepBackups)
	}
	if err := revalidateTarget(original, policy); err != nil {
		return nil, errors.Join(err, lock.release(), root.Close())
	}
	return &installSession{
		target:      original,
		root:        root,
		lock:        lock,
		staging:     make(map[string]struct{}),
		dirInfo:     rootInfo,
		lockTimeout: timeout,
	}, nil
}
