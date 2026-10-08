package selfupdate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// StandaloneInstaller owns target resolution, session construction, and
// binary replacement without service lifecycle.
type StandaloneInstaller struct {
	policy       TargetPolicy
	lockTimeout  time.Duration
	postInstall  Prober
	keepPrevious bool
}

// NewStandaloneInstaller returns a binary-only installer. LockTimeout zero
// selects DefaultLockTimeout. A negative duration is invalid.
func NewStandaloneInstaller(opts InstallOptions) (*StandaloneInstaller, error) {
	timeout := opts.LockTimeout
	if timeout == 0 {
		timeout = DefaultLockTimeout
	}
	if timeout < 0 {
		return nil, fmt.Errorf("selfupdate: lock timeout must not be negative")
	}
	s := &StandaloneInstaller{policy: opts.TargetPolicy, lockTimeout: timeout, keepPrevious: opts.KeepPrevious}
	if !isNil(opts.PostInstall) {
		s.postInstall = opts.PostInstall
	}
	return s, nil
}

// ResolveTarget implements Installer.
func (s *StandaloneInstaller) ResolveTarget(context.Context) (Target, error) {
	return resolveTarget(s.policy)
}

// Begin implements Installer.
func (s *StandaloneInstaller) Begin(ctx context.Context, target Target) (InstallSession, error) {
	sess, err := beginSession(ctx, s.policy, target, s.lockTimeout)
	if err != nil {
		return nil, err
	}
	sess.postInstall = s.postInstall
	sess.keepPrevious = s.keepPrevious
	return sess, nil
}

// CleanupPending finishes what an earlier update left behind: it takes
// the target's lock, which processes a pending cleanup receipt (on
// Windows, the backup the running image kept open; elsewhere, a stale
// receipt), and releases it. It installs nothing. The backup of an update
// that was interrupted between replacing the target and committing is
// kept, as .<base>.selfupdate-kept-<n>, not removed; KeptBackups lists it
// (docs/decisions/0017-MADR-verify-build-provenance-and-close-0015-open-items.md
// 3B).
//
// When another update holds the lock, the error matches
// ErrConcurrentUpdate. That is benign: retry later.
func (s *StandaloneInstaller) CleanupPending(ctx context.Context) error {
	target, err := resolveTarget(s.policy)
	if err != nil {
		return err
	}
	sess, err := beginSession(ctx, s.policy, target, s.lockTimeout)
	if err != nil {
		return err
	}
	return sess.Close()
}

// KeptBackup is a copy of a previous binary that an update left beside the
// target and that no session removes: one whose restore failed, or one an
// interrupted update left (0017-MADR 3B). Restore it, or remove it.
type KeptBackup struct {
	// Path is the copy's absolute path, .<base>.selfupdate-kept-<n> beside
	// the target.
	Path string `json:"path"`
	// Size is its length in bytes.
	Size int64 `json:"size"`
	// ModTime is its modification time, the replaced binary's own on Unix
	// when the backup is a hard link.
	ModTime time.Time `json:"mod_time"`
}

// KeptBackups lists the kept backups beside the target, sorted by name, or
// nil when there are none. It takes no lock, so it never fails with
// ErrConcurrentUpdate; call it after CleanupPending, which keeps an
// interrupted update's backup first (0017-MADR 3B).
func (s *StandaloneInstaller) KeptBackups(ctx context.Context) ([]KeptBackup, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	target, err := resolveTarget(s.policy)
	if err != nil {
		return nil, err
	}
	return keptBackups(target)
}

// keptBackups is KeptBackups for target: every regular file named
// .<base>.selfupdate-kept-<digits>, never through a symlink or reparse
// point.
func keptBackups(target Target) ([]KeptBackup, error) {
	root, err := os.OpenRoot(target.Dir)
	if err != nil {
		return nil, fmt.Errorf("selfupdate: open target directory: %w", err)
	}
	defer func() { advisory(root.Close()) }()
	dir, err := root.Open(".")
	if err != nil {
		return nil, fmt.Errorf("selfupdate: open target directory: %w", err)
	}
	names, err := dir.Readdirnames(-1)
	advisory(dir.Close())
	if err != nil {
		return nil, fmt.Errorf("selfupdate: list target directory: %w", err)
	}
	slices.Sort(names)
	prefix := "." + target.Base + ".selfupdate-kept-"
	var out []KeptBackup
	for _, name := range names {
		digits, ok := strings.CutPrefix(name, prefix)
		if !ok || !allDigits(digits) {
			continue
		}
		info, err := root.Lstat(name)
		path := filepath.Join(target.Dir, name)
		if err != nil || !info.Mode().IsRegular() || journalReparse(path) {
			continue
		}
		out = append(out, KeptBackup{Path: path, Size: info.Size(), ModTime: info.ModTime()})
	}
	return out, nil
}
