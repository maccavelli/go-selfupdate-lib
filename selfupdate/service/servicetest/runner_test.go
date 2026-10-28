package servicetest

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

var (
	quick    = service.Command{Path: "/fake/quick"}
	blocking = service.Command{Path: "/fake/blocking"}
)

// fakeRunner answers quick at once and blocks on blocking until its
// context ends. Its flags each break one rule.
type fakeRunner struct {
	mu    sync.Mutex
	calls int
	// ignoreContext runs a command whatever its context.
	ignoreContext bool
	// recordEnded records a call made on an ended context before refusing
	// it.
	recordEnded bool
	// blockFor, when set, holds blocking that long whatever its context,
	// then closes exited.
	blockFor time.Duration
	exited   chan struct{}
}

func (r *fakeRunner) Run(ctx context.Context, c service.Command) (service.Output, error) {
	if r.recordEnded {
		r.count()
	}
	if err := ctx.Err(); err != nil && !r.ignoreContext {
		return service.Output{}, err
	}
	if !r.recordEnded {
		r.count()
	}
	if c.Path != blocking.Path {
		return service.Output{}, nil
	}
	if r.blockFor > 0 {
		time.Sleep(r.blockFor)
		close(r.exited)
		return service.Output{}, nil
	}
	<-ctx.Done()
	return service.Output{}, ctx.Err()
}

func (r *fakeRunner) count() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
}

func (r *fakeRunner) Calls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func TestVerifyRunnerPasses(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := &fakeRunner{}
		if err := VerifyRunner(context.Background(), r, RunnerCase{Quick: quick, Blocking: blocking, Calls: r.Calls}); err != nil {
			t.Fatal(err)
		}
	})
}

func TestVerifyRunnerFails(t *testing.T) {
	for _, c := range []struct {
		name string
		r    *fakeRunner
		want string
	}{
		{"ignores its context", &fakeRunner{ignoreContext: true}, "Run on a cancelled context = no error"},
		{"records a call on an ended context", &fakeRunner{recordEnded: true}, "a Run on an ended context was recorded: 1 calls, then 3"},
		{"a blocking run ignores its cancellation", &fakeRunner{blockFor: time.Minute}, "a run whose context was cancelled still ran after 10s"},
	} {
		t.Run(c.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				// Made in the bubble: a receive on a channel made outside one
				// is not durably blocking, and bubble time would never move.
				if c.r.blockFor > 0 {
					c.r.exited = make(chan struct{})
				}
				err := VerifyRunner(context.Background(), c.r, RunnerCase{Quick: quick, Blocking: blocking, Calls: c.r.Calls})
				if err == nil || !strings.Contains(err.Error(), c.want) {
					t.Fatalf("err = %v\nwant one containing %q", err, c.want)
				}
				// VerifyRunner does not wait for a run that ignores its
				// cancellation; the bubble must, before it ends.
				if c.r.exited != nil {
					<-c.r.exited
				}
			})
		})
	}
}

func TestVerifyRunnerRefusesItsCase(t *testing.T) {
	if err := VerifyRunner(context.Background(), nil, RunnerCase{Quick: quick}); err == nil {
		t.Fatal("a nil runner was accepted")
	}
	if err := VerifyRunner(context.Background(), &fakeRunner{}, RunnerCase{}); err == nil {
		t.Fatal("a case with no Quick command was accepted")
	}
	failing := service.RunnerFunc(func(context.Context, service.Command) (service.Output, error) {
		return service.Output{}, errors.New("cannot run")
	})
	if err := VerifyRunner(context.Background(), failing, RunnerCase{Quick: quick}); err == nil || !strings.Contains(err.Error(), "Quick on a live context = cannot run") {
		t.Fatalf("a runner that cannot run Quick: %v", err)
	}
}
