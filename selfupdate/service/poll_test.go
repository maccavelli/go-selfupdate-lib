package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeClock advances only when a wait is asked for, so the tests never
// sleep.
type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { return c.now }

func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	c.now = c.now.Add(d)
	ch := make(chan time.Time, 1)
	ch <- c.now
	return ch
}

func useFakeClock(t *testing.T) *fakeClock {
	t.Helper()
	c := &fakeClock{now: time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)}
	prev := pollClock
	pollClock = c
	t.Cleanup(func() { pollClock = prev })
	return c
}

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
	c := useFakeClock(t)
	start := c.now
	probe, calls := script(Health{}, Health{Ready: true, Instance: "b"})
	err := PollHealthy(context.Background(), probe, PollOptions{Interval: time.Second, Settle: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	// Ready at 1 s; healthy once 3 s have passed with the same instance.
	if got := c.now.Sub(start); got != 4*time.Second || *calls != 5 {
		t.Fatalf("healthy after %s and %d probes, want 4s and 5", got, *calls)
	}
}

func TestPollHealthyInstanceChangeRestartsSettle(t *testing.T) {
	c := useFakeClock(t)
	start := c.now
	probe, _ := script(Health{Ready: true, Instance: "b"}, Health{Ready: true, Instance: "b"},
		Health{Ready: true, Instance: "c"}, Health{Ready: true, Instance: "c"})
	if err := PollHealthy(context.Background(), probe, PollOptions{Interval: time.Second, Settle: 2 * time.Second}); err != nil {
		t.Fatal(err)
	}
	// "c" first seen at 2 s: healthy at 4 s, not at 2 s.
	if got := c.now.Sub(start); got != 4*time.Second {
		t.Fatalf("healthy after %s, want 4s", got)
	}
}

func TestPollHealthyPreviousNeverCounts(t *testing.T) {
	useFakeClock(t)
	for _, h := range []Health{{Ready: true, Instance: "old"}, {Ready: true}} {
		probe, _ := script(h)
		// No settle window: a counting probe would succeed at once.
		err := PollHealthy(context.Background(), probe, PollOptions{Interval: time.Second, Timeout: 5 * time.Second, Settle: -1, Previous: "old"})
		if !errors.Is(err, ErrTimeout) {
			t.Fatalf("instance %q: err = %v, want a timeout", h.Instance, err)
		}
	}
}

func TestPollHealthyFailedStopsAtOnce(t *testing.T) {
	c := useFakeClock(t)
	start := c.now
	probe, calls := script(Health{Ready: true, Instance: "b"}, Health{Failed: true, Detail: "ActiveState=failed"})
	err := PollHealthy(context.Background(), probe, PollOptions{Interval: time.Second})
	if !errors.Is(err, ErrUnhealthy) || !strings.Contains(err.Error(), "ActiveState=failed") {
		t.Fatalf("err = %v", err)
	}
	if *calls != 2 || c.now.Sub(start) != time.Second {
		t.Fatalf("%d probes over %s; want 2 over 1s", *calls, c.now.Sub(start))
	}
}

func TestPollHealthyTimeoutReportsLastError(t *testing.T) {
	useFakeClock(t)
	probeErr := errors.New("fixture: show failed")
	probe := func(context.Context) (Health, error) { return Health{}, probeErr }
	err := PollHealthy(context.Background(), probe, PollOptions{Interval: time.Second, Timeout: 3 * time.Second})
	if !errors.Is(err, ErrTimeout) || !errors.Is(err, probeErr) {
		t.Fatalf("err = %v, want the timeout with the probe error", err)
	}
}

func TestPollHealthyContextEnds(t *testing.T) {
	useFakeClock(t)
	ctx, cancel := context.WithCancel(context.Background())
	probe := func(context.Context) (Health, error) {
		cancel()
		return Health{}, nil
	}
	if err := PollHealthy(ctx, probe, PollOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want the context's", err)
	}
}

func TestPollHealthyNoSettle(t *testing.T) {
	c := useFakeClock(t)
	start := c.now
	probe, calls := script(Health{Ready: true})
	if err := PollHealthy(context.Background(), probe, PollOptions{Settle: -1}); err != nil {
		t.Fatal(err)
	}
	if *calls != 1 || c.now != start {
		t.Fatalf("%d probes over %s; want 1 at once", *calls, c.now.Sub(start))
	}
}

func TestPollHealthyDefaults(t *testing.T) {
	o := PollOptions{}.withDefaults()
	if o.Interval != DefaultPollInterval || o.Timeout != DefaultPollTimeout || o.Settle != DefaultPollSettle {
		t.Fatalf("defaults %+v", o)
	}
	if err := PollHealthy(context.Background(), nil, PollOptions{}); err == nil {
		t.Fatal("a nil probe was accepted")
	}
}
