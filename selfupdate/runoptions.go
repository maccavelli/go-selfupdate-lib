package selfupdate

import (
	"context"
	"fmt"
	"time"
)

// RunOption changes one RunWith or Start call without changing the
// Updater, so one Updater can serve a --json CLI, a terminal prompt and a
// TUI (0004-MADR §4). When an option is given twice, the last one wins.
type RunOption interface {
	applyRun(*runScope) error
}

// runScope collects a call's options. A zero field means "the Updater's
// own".
type runScope struct {
	reporter    Reporter
	confirmer   Confirmer
	progress    time.Duration
	hasProgress bool
	credentials CredentialProvider
}

type runOptionFunc func(*runScope) error

func (f runOptionFunc) applyRun(s *runScope) error { return f(s) }

// WithReporter reports this run's events to r instead of Config.Reporter.
// Under Start, r receives every event after the Stream does (0004-MADR
// amendment B2).
func WithReporter(r Reporter) RunOption {
	return runOptionFunc(func(s *runScope) error {
		if isNil(r) {
			return fmt.Errorf("selfupdate: WithReporter: reporter is nil")
		}
		s.reporter = r
		return nil
	})
}

// WithConfirmer asks c instead of Config.Confirmer for this run. Under
// Start, c answers in place of ConfirmNeeded (0004-MADR amendment B2).
func WithConfirmer(c Confirmer) RunOption {
	return runOptionFunc(func(s *runScope) error {
		if isNil(c) {
			return fmt.Errorf("selfupdate: WithConfirmer: confirmer is nil")
		}
		s.confirmer = c
		return nil
	})
}

// WithProgressInterval sets this run's minimum time between EventProgress
// reports, in place of Config.ProgressInterval. Zero reports no progress; a
// negative value is refused (0004-MADR amendment B3).
func WithProgressInterval(d time.Duration) RunOption {
	return runOptionFunc(func(s *runScope) error {
		if d < 0 {
			return fmt.Errorf("selfupdate: WithProgressInterval: interval must not be negative")
		}
		s.progress, s.hasProgress = d, true
		return nil
	})
}

// WithCredentials makes p this run's credential provider, in the place of
// GitHubOptions.Credentials: after an explicit token and before the
// environment. The source must be a CredentialedSource, which gives the
// run its own copy with fresh credential state, so one run's credential
// never reaches the next (0004-MADR amendment B1).
func WithCredentials(p CredentialProvider) RunOption {
	return runOptionFunc(func(s *runScope) error {
		if isNil(p) {
			return fmt.Errorf("selfupdate: WithCredentials: provider is nil")
		}
		s.credentials = p
		return nil
	})
}

// run is one run's collaborators. Its fields shadow the embedded Updater's,
// so the run's methods read the run's reporter, confirmer, source and
// progress interval through the same names.
type run struct {
	*Updater
	source    ReleaseSource
	reporter  Reporter
	confirmer Confirmer
	progress  time.Duration
}

// scope applies opts in order.
func scope(opts []RunOption) (runScope, error) {
	var s runScope
	for _, o := range opts {
		if isNil(o) {
			return runScope{}, fmt.Errorf("selfupdate: run option is nil")
		}
		if err := o.applyRun(&s); err != nil {
			return runScope{}, err
		}
	}
	return s, nil
}

// newRun returns the Updater's collaborators with s applied.
func (u *Updater) newRun(s runScope) (*run, error) {
	r := &run{Updater: u, source: u.source, reporter: u.reporter, confirmer: u.confirmer, progress: u.progress}
	if s.reporter != nil {
		r.reporter = s.reporter
	}
	if s.confirmer != nil {
		r.confirmer = s.confirmer
	}
	if s.hasProgress {
		r.progress = s.progress
	}
	if s.credentials != nil {
		cs, ok := u.source.(CredentialedSource)
		if !ok {
			return nil, fmt.Errorf("selfupdate: WithCredentials: the source does not accept per-run credentials")
		}
		r.source = cs.WithCredentials(s.credentials)
	}
	return r, nil
}

// RunWith executes one self-update request with opts applied to this run
// only. It shares Run's guard: one run at a time per Updater. An invalid
// option fails before anything else, and nothing is reported for it.
func (u *Updater) RunWith(ctx context.Context, req Request, opts ...RunOption) (Result, error) {
	s, err := scope(opts)
	if err != nil {
		return Result{}, err
	}
	r, err := u.newRun(s)
	if err != nil {
		return Result{}, err
	}
	return u.execRun(ctx, req, r)
}

// execRun runs r under the Updater's one-run-at-a-time guard. RunWith and
// Start both use it.
func (u *Updater) execRun(ctx context.Context, req Request, r *run) (Result, error) {
	if !u.running.CompareAndSwap(false, true) {
		return Result{}, ErrConcurrentUpdate
	}
	defer u.running.Store(false)
	return r.execute(ctx, req)
}

// checker is a Checker over the run's source, which checks the selection
// against the run's Unpacker (0015-MADR C4).
func (u *run) checker() *Checker {
	return &Checker{source: u.source, versions: u.versions, assets: u.assets, limits: u.limits,
		unpacker: u.unpacker, fromUpdater: true}
}
