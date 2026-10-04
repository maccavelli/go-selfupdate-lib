package selfupdate

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"iter"
	"sync"
)

// Interaction is one item a Stream delivers: a Progressed event, a
// *ConfirmNeeded or *CredentialNeeded request, or the run's Finished
// outcome (0004-MADR §4).
type Interaction interface {
	interaction()
}

// Progressed carries one reported Event. Every event is delivered, in
// order; only EventProgress is coalesced, so a slow consumer sees the
// newest progress rather than a backlog (0004-MADR amendment B4).
type Progressed struct {
	Event Event
}

func (Progressed) interaction() {}

// Finished is the run's outcome. It is always the last interaction, and
// it arrives only after the run has returned: the session is closed and
// any recovery has run.
type Finished struct {
	Result Result
	Err    error
}

func (Finished) interaction() {}

type confirmReply struct {
	ok  bool
	err error
}

// ConfirmNeeded asks the host to approve the selected operation. The run
// waits until Answer or Cancel is called, or the run is cancelled. The
// first reply wins; later ones do nothing (0004-MADR amendment B5).
type ConfirmNeeded struct {
	// Prompt describes the operation, as a Confirmer would receive it.
	Prompt Prompt

	once  sync.Once
	reply chan confirmReply
}

func (*ConfirmNeeded) interaction() {}

// Answer approves (true) or declines (false) the operation. On a zero
// ConfirmNeeded, which no run is waiting on, it returns at once and does
// nothing.
func (c *ConfirmNeeded) Answer(ok bool) {
	c.once.Do(func() { trySend(c.reply, confirmReply{ok: ok}) })
}

// Cancel fails the confirmation with err, or with context.Canceled when err
// is nil. On a zero ConfirmNeeded it returns at once and does nothing.
func (c *ConfirmNeeded) Cancel(err error) {
	if err == nil {
		err = context.Canceled
	}
	c.once.Do(func() { trySend(c.reply, confirmReply{err: err}) })
}

// trySend sends v unless ch cannot take it. A reply channel a run made has
// one free slot for the one reply sync.Once allows, so the send always
// happens; a zero value's nil channel takes nothing (0010-MADR Q5).
func trySend[T any](ch chan T, v T) {
	select {
	case ch <- v:
	default:
	}
}

type credentialReply struct {
	cred Credential
	err  error
}

// CredentialNeeded asks the host for a credential, as PromptCredential
// raises it inside a Stream. Request.Cause is non-nil when a credential
// already sent was refused. The source waits until Supply or Cancel is
// called, or the run is cancelled. The first reply wins; later ones do
// nothing (0004-MADR amendment B5).
type CredentialNeeded struct {
	// Request describes the request that needs the credential.
	Request CredentialRequest

	once  sync.Once
	reply chan credentialReply
}

func (*CredentialNeeded) interaction() {}

// Supply answers with cred. Its Value is copied, so the host may clear its
// own buffer afterwards. The source still validates the credential. On a
// zero CredentialNeeded, which no run is waiting on, it returns at once and
// does nothing.
func (c *CredentialNeeded) Supply(cred Credential) {
	cred.Value = bytes.Clone(cred.Value)
	c.once.Do(func() { trySend(c.reply, credentialReply{cred: cred}) })
}

// Cancel answers with err, or with ErrNoCredential when err is nil, so a
// chain moves on to its next provider or the request goes anonymous. On a
// zero CredentialNeeded it returns at once and does nothing.
func (c *CredentialNeeded) Cancel(err error) {
	if err == nil {
		err = ErrNoCredential
	}
	c.once.Do(func() { trySend(c.reply, credentialReply{err: err}) })
}

// Stream drives one run from an event loop. Start begins the run; the host
// pulls interactions with Next or All, answers each request, and may Cancel
// at any time. Delivery never blocks the run.
//
// The host must answer every ConfirmNeeded and CredentialNeeded, or call
// Cancel: a request left unanswered keeps the run waiting.
type Stream struct {
	cancel context.CancelFunc
	wake   chan struct{} // one slot: something was queued
	closed chan struct{} // closed once Next has returned Finished

	mu       sync.Mutex
	queue    []Interaction
	finished bool
}

// streamKey finds a run's Stream in the run's context (0004-MADR amendment
// B1).
type streamKey struct{}

// Start begins req on u in its own goroutine and returns its Stream at
// once; it never returns nil. opts apply to the run as in RunWith, except
// that WithReporter adds a reporter after the Stream's own, and
// WithConfirmer answers in place of ConfirmNeeded. The Updater's own
// Reporter and Confirmer are not used (0004-MADR amendment B2).
//
// A nil u, an invalid option, or a run already in progress gives a Stream
// whose only interaction is Finished with that error.
func Start(ctx context.Context, u *Updater, req Request, opts ...RunOption) *Stream {
	runCtx, cancel := context.WithCancel(ctx)
	s := &Stream{cancel: cancel, wake: make(chan struct{}, 1), closed: make(chan struct{})}
	r, err := s.prepare(u, opts)
	if err != nil {
		cancel()
		s.push(Finished{Err: err})
		return s
	}
	runCtx = context.WithValue(runCtx, streamKey{}, s)
	go func() {
		defer cancel()
		res, err := u.execRun(runCtx, req, r)
		s.push(Finished{Result: res, Err: err})
	}()
	return s
}

func (s *Stream) prepare(u *Updater, opts []RunOption) (*run, error) {
	if u == nil {
		return nil, fmt.Errorf("selfupdate: updater is nil")
	}
	sc, err := scope(opts)
	if err != nil {
		return nil, err
	}
	var reporter Reporter = streamReporter{s}
	if sc.reporter != nil {
		reporter = MultiReporter(reporter, sc.reporter)
	}
	sc.reporter = reporter
	if sc.confirmer == nil {
		sc.confirmer = streamConfirmer{s}
	}
	return u.newRun(sc)
}

// push queues it. A progress event replaces a progress event that is still
// the newest undelivered item; nothing else is replaced or dropped.
func (s *Stream) push(it Interaction) {
	s.mu.Lock()
	if n := len(s.queue); n > 0 && isProgress(it) && isProgress(s.queue[n-1]) {
		s.queue[n-1] = it
	} else {
		s.queue = append(s.queue, it)
	}
	s.mu.Unlock()
	s.signal()
}

func isProgress(it Interaction) bool {
	p, ok := it.(Progressed)
	return ok && p.Event.Kind == EventProgress
}

func (s *Stream) signal() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Next returns the next interaction, waiting for one. After it has
// returned Finished it returns io.EOF. When ctx ends first it returns
// ctx.Err() and consumes nothing (0004-MADR amendment B6). It is safe for
// concurrent use; each interaction is returned once.
func (s *Stream) Next(ctx context.Context) (Interaction, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		s.mu.Lock()
		if len(s.queue) > 0 {
			it := s.queue[0]
			s.queue[0] = nil
			s.queue = s.queue[1:]
			more := len(s.queue) > 0
			if _, ok := it.(Finished); ok {
				s.finished = true
				close(s.closed)
			}
			s.mu.Unlock()
			if more {
				s.signal()
			}
			return it, nil
		}
		finished := s.finished
		s.mu.Unlock()
		if finished {
			return nil, io.EOF
		}
		select {
		case <-s.wake:
		case <-s.closed:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// All yields each interaction up to and including Finished. It stops early
// when ctx ends or the loop breaks; breaking does not cancel the run.
func (s *Stream) All(ctx context.Context) iter.Seq[Interaction] {
	return func(yield func(Interaction) bool) {
		for {
			it, err := s.Next(ctx)
			if err != nil || !yield(it) {
				return
			}
		}
	}
}

// Cancel cancels the run. The run still finishes through its usual
// recovery, and Finished is still delivered. It may be called any number
// of times, from any goroutine.
func (s *Stream) Cancel() {
	s.cancel()
}

// streamReporter queues every event; it never blocks and never fails.
type streamReporter struct{ s *Stream }

func (r streamReporter) Report(_ context.Context, ev Event) error {
	r.s.push(Progressed{Event: ev})
	return nil
}

// streamConfirmer queues a ConfirmNeeded and waits for its reply or for
// the run to be cancelled.
type streamConfirmer struct{ s *Stream }

func (c streamConfirmer) Confirm(ctx context.Context, p Prompt) (bool, error) {
	req := &ConfirmNeeded{Prompt: p, reply: make(chan confirmReply, 1)}
	c.s.push(req)
	select {
	case r := <-req.reply:
		return r.ok, r.err
	case <-ctx.Done():
		return false, ctx.Err()
	}
}
