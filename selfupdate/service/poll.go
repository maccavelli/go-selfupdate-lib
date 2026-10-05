package service

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Health is one observation of a service.
type Health struct {
	// Ready reports that the instance counts as up.
	Ready bool `json:"ready"`
	// Instance identifies the running instance, and changes with each
	// start: a systemd InvocationID, a launchd or SCM process ID. Empty
	// means the probe cannot tell instances apart.
	Instance string `json:"instance,omitempty"`
	// Failed reports that the instance failed: the wait ends at once.
	Failed bool `json:"failed"`
	// Detail describes the observation, for an error message.
	Detail string `json:"detail,omitempty"`
}

// A HealthProbe observes a service once. An error is not fatal to
// PollHealthy: it is kept, and reported if the wait times out.
type HealthProbe func(ctx context.Context) (Health, error)

// PollOptions bound PollHealthy. A zero field takes its default.
type PollOptions struct {
	// Interval is the time between probes. Default 250 ms.
	Interval time.Duration
	// Timeout bounds the whole wait. Default 60 s.
	Timeout time.Duration
	// Settle is how long the service must stay Ready, as one Instance,
	// before it counts as healthy. Default 10 s, launchd's default
	// ThrottleInterval; a negative value means no settle window.
	Settle time.Duration
	// Previous is an Instance that never counts, such as the one that ran
	// before the update. When it is set, an empty Instance never counts
	// either, since it cannot prove a new start.
	Previous string
}

// The PollOptions defaults (0011-MADR §4).
const (
	DefaultPollInterval = 250 * time.Millisecond
	DefaultPollTimeout  = 60 * time.Second
	DefaultPollSettle   = 10 * time.Second
)

func (o PollOptions) withDefaults() PollOptions {
	if o.Interval <= 0 {
		o.Interval = DefaultPollInterval
	}
	if o.Timeout <= 0 {
		o.Timeout = DefaultPollTimeout
	}
	switch {
	case o.Settle == 0:
		o.Settle = DefaultPollSettle
	case o.Settle < 0:
		o.Settle = 0
	}
	return o
}

// clock is time, replaced in tests.
type clock interface {
	Now() time.Time
	After(time.Duration) <-chan time.Time
}

type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

var pollClock clock = realClock{}

// PollHealthy probes a service until it is healthy: Ready, with one
// Instance other than o.Previous, for o.Settle. A change of Instance during
// the settle window starts it again. It returns nil when healthy;
// ErrUnhealthy as soon as a probe reports Failed; ErrTimeout, with the last
// observation and probe error, once o.Timeout has passed; and the context's
// error when ctx ends first (0011-MADR §4).
func PollHealthy(ctx context.Context, probe HealthProbe, o PollOptions) error {
	if probe == nil {
		return errors.New("selfupdate: service: health probe is required")
	}
	o = o.withDefaults()
	deadline := pollClock.Now().Add(o.Timeout)
	var (
		last      Health
		lastErr   error
		stableAt  time.Time
		stableFor string
		stable    bool
	)
	for {
		probeCtx, cancel := context.WithTimeout(ctx, max(deadline.Sub(pollClock.Now()), time.Millisecond))
		h, err := probe(probeCtx)
		cancel()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("selfupdate: service: health wait: %w", ctxErr)
		}
		now := pollClock.Now()
		switch {
		case err != nil:
			lastErr, stable = err, false
		case h.Failed:
			return fmt.Errorf("%w: %s", ErrUnhealthy, describe(h))
		case counts(h, o.Previous):
			last = h
			if !stable || h.Instance != stableFor {
				stable, stableAt, stableFor = true, now, h.Instance
			}
			if now.Sub(stableAt) >= o.Settle {
				return nil
			}
		default:
			last, stable = h, false
		}
		if !now.Before(deadline) {
			return timeoutError(o.Timeout, last, lastErr)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("selfupdate: service: health wait: %w", ctx.Err())
		case <-pollClock.After(min(o.Interval, deadline.Sub(now))):
		}
	}
}

// counts reports whether h is a ready instance that is not previous.
func counts(h Health, previous string) bool {
	if !h.Ready {
		return false
	}
	return previous == "" || (h.Instance != "" && h.Instance != previous)
}

func describe(h Health) string {
	if h.Detail != "" {
		return h.Detail
	}
	return fmt.Sprintf("ready=%t instance=%q", h.Ready, h.Instance)
}

func timeoutError(timeout time.Duration, last Health, lastErr error) error {
	err := fmt.Errorf("%w: not healthy within %s (last: %s)", ErrTimeout, timeout, describe(last))
	if lastErr != nil {
		err = errors.Join(err, lastErr)
	}
	return err
}
