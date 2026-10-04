package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
)

// ManagedInstaller composes standalone replacement with consumer lifecycle
// and definition reconciliation.
type ManagedInstaller struct {
	inner Installer
	life  Lifecycle
	rec   Reconciler
}

// NewManagedInstaller wraps a standalone installer. life and rec are required.
func NewManagedInstaller(inner *StandaloneInstaller, life Lifecycle, rec Reconciler) (*ManagedInstaller, error) {
	if inner == nil {
		return nil, fmt.Errorf("selfupdate: managed installer requires a standalone installer")
	}
	return NewManagedInstallerFor(inner, life, rec)
}

// NewManagedInstallerFor wraps any Installer whose sessions implement
// TwoPhaseSession; Begin refuses a session that does not. life and rec are
// required (0004-MADR G7).
func NewManagedInstallerFor(inner Installer, life Lifecycle, rec Reconciler) (*ManagedInstaller, error) {
	if isNil(inner) {
		return nil, fmt.Errorf("selfupdate: managed installer requires an installer")
	}
	// isNil refuses a typed nil too, which would otherwise panic in Install
	// after the download, with the lock held (0010-MADR C5).
	if isNil(life) {
		return nil, fmt.Errorf("selfupdate: managed installer requires a lifecycle")
	}
	if isNil(rec) {
		return nil, fmt.Errorf("selfupdate: managed installer requires a reconciler")
	}
	return &ManagedInstaller{inner: inner, life: life, rec: rec}, nil
}

// ResolveTarget implements Installer.
func (m *ManagedInstaller) ResolveTarget(ctx context.Context) (Target, error) {
	return m.inner.ResolveTarget(ctx)
}

// Begin implements Installer.
func (m *ManagedInstaller) Begin(ctx context.Context, target Target) (InstallSession, error) {
	inner, err := m.inner.Begin(ctx, target)
	if err != nil {
		return nil, err
	}
	sess, ok := inner.(TwoPhaseSession)
	if !ok {
		return nil, joinClose(fmt.Errorf("selfupdate: managed installer requires a two-phase session"), inner)
	}
	return &managedSession{inner: sess, life: m.life, rec: m.rec}, nil
}

type managedSession struct {
	inner TwoPhaseSession
	life  Lifecycle
	rec   Reconciler
}

func (s *managedSession) Target() Target { return s.inner.Target() }

func (s *managedSession) CreateStaging(ctx context.Context) (*os.File, string, error) {
	return s.inner.CreateStaging(ctx)
}

func (s *managedSession) Close() error { return s.inner.Close() }

// Owns implements StagingOwner.
func (s *managedSession) Owns(path string) bool { return s.inner.Owns(path) }

func (s *managedSession) Install(ctx context.Context, req InstallRequest) (InstallResult, error) {
	product := req.Product
	installed, err := s.life.Installed(ctx, product)
	if err != nil {
		return InstallResult{}, fmt.Errorf("selfupdate: probe installed: %w", errors.Join(ErrManagedInstall, err))
	}
	if !installed {
		return s.inner.Install(ctx, req)
	}
	running, err := s.life.Running(ctx, product)
	if err != nil {
		return InstallResult{}, fmt.Errorf("selfupdate: probe running: %w", errors.Join(ErrManagedInstall, err))
	}
	// A running service is started again. A stopped one is started only
	// when it is configured to start, which only an EnabledLifecycle can
	// say; otherwise it stays stopped (0010-MADR Q2).
	start := running
	if el, ok := s.life.(EnabledLifecycle); ok && !running {
		enabled, err := el.Enabled(ctx, product)
		if err != nil {
			return InstallResult{}, fmt.Errorf("selfupdate: probe enabled: %w", errors.Join(ErrManagedInstall, err))
		}
		start = enabled
	}
	if running {
		if err := s.life.Stop(ctx, product); err != nil {
			return InstallResult{}, fmt.Errorf("selfupdate: stop service: %w", errors.Join(ErrManagedInstall, err))
		}
	}
	// Until Start succeeds, recovery restarts only what was running.
	applied, err := s.inner.Apply(ctx, req)
	if err != nil {
		// applied carries a backup when the new binary is live and the
		// restore inside replaceTarget failed; recovery retries it
		// (0003-MADR B1).
		return s.recover(ctx, product, applied, ReconcileResult{}, running, false, err)
	}
	receipt, recErr := s.rec.Reconcile(ctx, product, s.inner.Target().Path)
	if recErr != nil {
		return s.recover(ctx, product, applied, receipt, running, false, recErr)
	}
	if start {
		if err := s.life.Start(ctx, product); err != nil {
			return s.recover(ctx, product, applied, receipt, running, false, err)
		}
		if err := s.life.WaitHealthy(ctx, product); err != nil {
			// The new binary was started: it is stopped before the old one
			// is restored under it, and the old one is started in its place
			// (0010-MADR B4).
			return s.recover(ctx, product, applied, receipt, true, true, err)
		}
	}
	result, err := s.inner.Commit(ctx, applied)
	if errors.Is(err, ErrConcurrentUpdate) {
		// The directory changed after the replacement: undo it in the locked
		// directory, as the standalone path does, rather than leave the new
		// binary live and the backup behind (0010-MADR B7). Any other commit
		// error is a cleanup failure after a healthy update, and is
		// returned as it is.
		return s.recover(ctx, product, applied, receipt, start, start, err)
	}
	result.ServiceInstalled = true
	result.ServiceWasRunning = running
	result.ServiceStarted = start
	return result, err
}

// recoveryTimeout bounds recovery. Recovery does not inherit the caller's
// cancellation: when the failure being recovered from was the caller's
// deadline, a cancelled context would leave a stopped service down
// (0003-MADR B2).
const recoveryTimeout = 2 * time.Minute

// recover undoes a failed managed install. Its result names the backup
// when the rollback failed and the backup still exists, so the caller
// learns where the previous binary is (0004-MADR R1).
func (s *managedSession) recover(parent context.Context, product string, applied AppliedReplacement, receipt ReconcileResult, restart, stopFirst bool, origin error) (InstallResult, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), recoveryTimeout)
	defer cancel()
	var recov error
	if stopFirst {
		if err := s.life.Stop(ctx, product); err != nil {
			recov = errors.Join(recov, err)
		}
	}
	if receipt.Changed || receipt.State != nil {
		if err := s.rec.Restore(ctx, product, receipt); err != nil {
			recov = errors.Join(recov, err)
		}
	}
	result := InstallResult{}
	target := s.inner.Target().Path
	if applied.Backup != "" {
		if err := s.inner.Rollback(ctx, applied); err != nil {
			recov = errors.Join(recov, err)
			if _, serr := os.Lstat(applied.Backup); serr == nil {
				result = InstallResult{Target: target, Backup: applied.Backup}
			}
		} else {
			result = InstallResult{Target: target, RolledBack: true}
		}
	}
	if errors.Is(origin, errProbeRolledBack) {
		// The session rolled back a replacement that failed its
		// post-install probe before recovery began.
		result = InstallResult{Target: target, RolledBack: true}
	}
	if restart {
		if err := s.life.Start(ctx, product); err != nil {
			recov = errors.Join(recov, err)
		} else if err := s.life.WaitHealthy(ctx, product); err != nil {
			recov = errors.Join(recov, err)
		}
	}
	return result, fmt.Errorf("selfupdate: managed install failed: %w", errors.Join(ErrManagedInstall, origin, recov))
}
