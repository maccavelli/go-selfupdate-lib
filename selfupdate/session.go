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

// errProbeRolledBack marks a post-install probe failure whose replacement
// was rolled back.
var errProbeRolledBack = errors.New("selfupdate: the replacement was rolled back")

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

func appliedState(a AppliedReplacement) (applyResult, error) {
	applied, ok := a.State.(applyResult)
	if !ok {
		return applyResult{}, errForeignReplacement
	}
	return applied, nil
}

func (s *installSession) Install(ctx context.Context, req InstallRequest) (InstallResult, error) {
	if err := ctx.Err(); err != nil {
		return InstallResult{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	applied, err := s.replaceLocked(ctx, req.Artifact.Path)
	if err != nil {
		// Backup is non-empty only when the new binary is live and the
		// restore failed (0003-MADR B1).
		return InstallResult{Target: s.target.Path, Backup: applied.backup}, err
	}
	if err := s.checkDir(); err != nil {
		// The rename went into the locked directory, wherever it is now:
		// undo it there, through the handle (0004-MADR R3).
		if rerr := s.rollbackInRoot(applied); rerr != nil {
			// Not applied: the backup is the only copy of the previous
			// binary, reported as the restore failure above is
			// (0010-MADR B8).
			return InstallResult{Target: s.target.Path, Backup: applied.backup}, errors.Join(err, rerr)
		}
		return InstallResult{Target: s.target.Path, RolledBack: true}, err
	}
	if err := s.probeInstalled(ctx, req, applied); err != nil {
		if errors.Is(err, errProbeRolledBack) {
			return InstallResult{Target: s.target.Path, RolledBack: true}, err
		}
		return InstallResult{Target: s.target.Path, Backup: applied.backup}, err
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
	if err := replacePath(withRetryBudget(ctx, s.lockTimeout), applied.backup, previous); err != nil {
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
	applied, err := s.replaceLocked(ctx, req.Artifact.Path)
	if err == nil {
		// The directory again, now that the rename has gone into it: Install
		// checks here too (0004-MADR R3; 0010-MADR B7).
		if derr := s.checkDir(); derr != nil {
			if rerr := s.rollbackInRoot(applied); rerr != nil {
				return AppliedReplacement{Target: s.target.Path, Backup: applied.backup, State: applied}, errors.Join(derr, rerr)
			}
			return AppliedReplacement{Target: s.target.Path, State: applyResult{}}, derr
		}
		if err = s.probeInstalled(ctx, req, applied); errors.Is(err, errProbeRolledBack) {
			applied = applyResult{}
		}
	}
	return AppliedReplacement{Target: s.target.Path, Backup: applied.backup, State: applied}, err
}

// probeInstalled runs the post-install probe on the replaced target. On
// failure it rolls the replacement back; the error wraps
// errProbeRolledBack when that succeeded. The caller holds s.mu.
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
			return errors.Join(perr, rerr, errProbeRolledBack)
		}
		return errors.Join(perr, rerr)
	}
	return errors.Join(perr, errProbeRolledBack)
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
	applied, err := appliedState(a)
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
	// The replacement is live either way: a refused commit leaves the new
	// binary and its backup in place.
	pending, previous := "", ""
	if err = s.checkDir(); err == nil {
		pending, previous, err = s.commitLocked(ctx, applied)
	}
	return InstallResult{
		Target:        s.target.Path,
		Backup:        applied.backup,
		Applied:       true,
		PendingBackup: pending,
		Previous:      previous,
	}, err
}

// Rollback implements TwoPhaseSession: it restores the backup of a
// replacement this session applied.
func (s *installSession) Rollback(ctx context.Context, a AppliedReplacement) error {
	applied, err := appliedState(a)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("selfupdate: session is closed")
	}
	return rollbackReplacement(withRetryBudget(ctx, s.lockTimeout), s.target, applied)
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
	return syncRoot(s.root)
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
	if err := processCleanupReceipt(original, root); err != nil {
		return nil, errors.Join(err, lock.release(), root.Close())
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
