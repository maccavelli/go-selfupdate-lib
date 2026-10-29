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
// only. It shares Run's guard: one run at a time per Updater, from any
// goroutine, and an overlapping run returns ErrConcurrentUpdate. An invalid
// option fails before anything else, and nothing is reported for it.
//
// Errors: those of Run, and an invalid option's, which comes first.
func (u *Updater) RunWith(ctx context.Context, req Request, opts ...RunOption) (Result, error) {
	if !u.constructed() {
		return Result{}, ErrNotConstructed
	}
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

// constructed reports whether New made u: a nil or zero Updater has no
// collaborators (0015-MADR C8).
func (u *Updater) constructed() bool {
	return u != nil && u.source != nil && u.versions != nil && u.assets != nil && u.installer != nil
}

// Apply installs the release p pins, for req, with opts applied to this
// run only, as RunWith does. req must be the request p was made for, as
// p.Request returns it, with Yes, DryRun or CheckOnly set as the caller
// wants. Apply fetches the release by p's tag, and installs it only when
// it still matches p: the same release ID, assets, digests and operation,
// and, once SHA256SUMS is downloaded, the same entry for the binary. A
// newer release published since p was made changes nothing. Apply asks the
// Confirmer as Run does, unless req says Yes or DryRun: a plan carries no
// approval. It reports Run's events, and shares Run's guard
// (docs/decisions/0038-MADR-pinned-plans-lifecycle-conformance-and-bubble-tests.md
// D1-A).
//
// Errors: those of RunWith; ErrPlanStale when req is not p's request or
// the release no longer matches p, before any source call for the first,
// and also matching ErrIntegrity for a changed SHA256SUMS entry; and an
// invalid plan's, before anything else but ErrNotConstructed.
func (u *Updater) Apply(ctx context.Context, req Request, p Plan, opts ...RunOption) (Result, error) {
	if !u.constructed() {
		return Result{}, ErrNotConstructed
	}
	if err := p.validate(); err != nil {
		return Result{}, err
	}
	s, err := scope(opts)
	if err != nil {
		return Result{}, err
	}
	r, err := u.newRun(s)
	if err != nil {
		return Result{}, err
	}
	return u.execPinned(ctx, req, r, &p)
}

// execRun runs r under the Updater's one-run-at-a-time guard. RunWith and
// Start both use it.
func (u *Updater) execRun(ctx context.Context, req Request, r *run) (Result, error) {
	return u.execPinned(ctx, req, r, nil)
}

// execPinned is execRun with a plan's pins, for Apply.
func (u *Updater) execPinned(ctx context.Context, req Request, r *run, pin *Plan) (Result, error) {
	if !u.running.CompareAndSwap(false, true) {
		return Result{}, ErrConcurrentUpdate
	}
	defer u.running.Store(false)
	return r.execute(ctx, req, pin)
}

// checker is a Checker over the run's source, which checks the selection
// against the run's Unpacker (0015-MADR C4).
func (u *run) checker() *Checker {
	return &Checker{source: u.source, versions: u.versions, assets: u.assets, limits: u.limits,
		unpacker: u.unpacker, fromUpdater: true, clock: u.clock, platform: u.runningPlatform()}
}
