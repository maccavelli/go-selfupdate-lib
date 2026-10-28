package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/internal/testproc"
)

// The PollHealthy tests run in a synctest bubble, where time.Now, its
// timers and the probe context's deadline share one fake clock, so they
// never sleep and measure waits exactly
// (docs/decisions/0038-MADR-pinned-plans-lifecycle-conformance-and-bubble-tests.md D3-A).

// script returns a probe that gives each observation in turn, then the
// last one for ever, and counts its calls.
func script(steps ...Health) (HealthProbe, *int) {
	n := 0
	return func(context.Context) (Health, error) {
		h := steps[min(n, len(steps)-1)]
		n++
		return h, nil
	}, &n
}

func TestPollHealthySettles(t *testing.T) {
	testproc.CheckNoLeak(t)
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		probe, calls := script(Health{}, Health{Ready: true, Instance: "b"})
		err := PollHealthy(context.Background(), probe, PollOptions{Interval: time.Second, Settle: 3 * time.Second})
		if err != nil {
			t.Fatal(err)
		}
		// Ready at 1 s; healthy once 3 s have passed with the same instance.
		if got := time.Since(start); got != 4*time.Second || *calls != 5 {
			t.Fatalf("healthy after %s and %d probes, want 4s and 5", got, *calls)
		}
	})
}

func TestPollHealthyInstanceChangeRestartsSettle(t *testing.T) {
	testproc.CheckNoLeak(t)
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		probe, _ := script(Health{Ready: true, Instance: "b"}, Health{Ready: true, Instance: "b"},
			Health{Ready: true, Instance: "c"}, Health{Ready: true, Instance: "c"})
		if err := PollHealthy(context.Background(), probe, PollOptions{Interval: time.Second, Settle: 2 * time.Second}); err != nil {
			t.Fatal(err)
		}
		// "c" first seen at 2 s: healthy at 4 s, not at 2 s.
		if got := time.Since(start); got != 4*time.Second {
			t.Fatalf("healthy after %s, want 4s", got)
		}
	})
}

func TestPollHealthyPreviousNeverCounts(t *testing.T) {
	testproc.CheckNoLeak(t)
	for _, h := range []Health{{Ready: true, Instance: "old"}, {Ready: true}} {
		synctest.Test(t, func(t *testing.T) {
			start := time.Now()
			probe, _ := script(h)
			// No settle window: a counting probe would succeed at once.
			err := PollHealthy(context.Background(), probe, PollOptions{Interval: time.Second, Timeout: 5 * time.Second, Settle: -1, Previous: "old"})
			if !errors.Is(err, ErrTimeout) || time.Since(start) != 5*time.Second {
				t.Fatalf("instance %q: err = %v after %s, want a timeout at 5s", h.Instance, err, time.Since(start))
			}
		})
	}
}

func TestPollHealthyFailedStopsAtOnce(t *testing.T) {
	testproc.CheckNoLeak(t)
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		probe, calls := script(Health{Ready: true, Instance: "b"}, Health{Failed: true, Detail: "ActiveState=failed"})
		err := PollHealthy(context.Background(), probe, PollOptions{Interval: time.Second})
		if !errors.Is(err, ErrUnhealthy) || !strings.Contains(err.Error(), "ActiveState=failed") {
			t.Fatalf("err = %v", err)
		}
		if took := time.Since(start); *calls != 2 || took != time.Second {
			t.Fatalf("%d probes over %s; want 2 over 1s", *calls, took)
		}
	})
}

func TestPollHealthyTimeoutReportsLastError(t *testing.T) {
	testproc.CheckNoLeak(t)
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		probeErr := errors.New("fixture: show failed")
		probe := func(context.Context) (Health, error) { return Health{}, probeErr }
		err := PollHealthy(context.Background(), probe, PollOptions{Interval: time.Second, Timeout: 3 * time.Second})
		if !errors.Is(err, ErrTimeout) || !errors.Is(err, probeErr) {
			t.Fatalf("err = %v, want the timeout with the probe error", err)
		}
		if took := time.Since(start); took != 3*time.Second {
			t.Fatalf("timed out after %s, want 3s", took)
		}
	})
}

// TestPollHealthyProbeContextEndsAtDeadline: the probe's own context ends
// at the wait's deadline, on the clock the wait measures, so a probe that
// blocks cannot hold the wait past its timeout.
func TestPollHealthyProbeContextEndsAtDeadline(t *testing.T) {
	testproc.CheckNoLeak(t)
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		probe := func(ctx context.Context) (Health, error) {
			<-ctx.Done()
			return Health{}, ctx.Err()
		}
		err := PollHealthy(context.Background(), probe, PollOptions{Interval: time.Second, Timeout: 3 * time.Second})
		if !errors.Is(err, ErrTimeout) || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err = %v, want the timeout with the probe's deadline", err)
		}
		if took := time.Since(start); took != 3*time.Second {
			t.Fatalf("timed out after %s, want 3s", took)
		}
	})
}

func TestPollHealthyContextEnds(t *testing.T) {
	testproc.CheckNoLeak(t)
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		probe := func(context.Context) (Health, error) {
			cancel()
			return Health{}, nil
		}
		if err := PollHealthy(ctx, probe, PollOptions{}); !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want the context's", err)
		}
	})
}

func TestPollHealthyNoSettle(t *testing.T) {
	testproc.CheckNoLeak(t)
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		probe, calls := script(Health{Ready: true})
		if err := PollHealthy(context.Background(), probe, PollOptions{Settle: -1}); err != nil {
			t.Fatal(err)
		}
		if took := time.Since(start); *calls != 1 || took != 0 {
			t.Fatalf("%d probes over %s; want 1 at once", *calls, took)
		}
	})
}

func TestPollHealthyDefaults(t *testing.T) {
	testproc.CheckNoLeak(t)
	o := PollOptions{}.withDefaults()
	if o.Interval != DefaultPollInterval || o.Timeout != DefaultPollTimeout || o.Settle != DefaultPollSettle {
		t.Fatalf("defaults %+v", o)
	}
	if err := PollHealthy(context.Background(), nil, PollOptions{}); err == nil {
		t.Fatal("a nil probe was accepted")
	}
}

// TestPollOptionsValidate: a settle window as long as the timeout is
// refused, since every wait would time out; zero takes the defaults, and a
// negative settle is no window
// (docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md D10).
func TestPollOptionsValidate(t *testing.T) {
	testproc.CheckNoLeak(t)
	for _, c := range []struct {
		o  PollOptions
		ok bool
	}{
		{PollOptions{}, true},
		{PollOptions{Settle: 10 * time.Second, Timeout: 5 * time.Second}, false},
		{PollOptions{Settle: 10 * time.Second, Timeout: 10 * time.Second}, false},
		{PollOptions{Settle: -1, Timeout: time.Second}, true},
	} {
		err := c.o.Validate()
		if (err == nil) != c.ok || err != nil && !strings.Contains(err.Error(), "is not less than Poll.Timeout") {
			t.Errorf("%+v: %v, want ok %t", c.o, err, c.ok)
		}
	}
}
