package systemd

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// Tests for docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md
// B3: once the stop is issued, Stop waits for the unit to be inactive
// whatever the caller's context does, bounded by systemd's own stop
// timeout.

// TestStopFinishesAfterCancel: a caller's context that ends once the stop
// is issued does not cut the wait short. Stop waits until the unit is
// inactive, then reports the cancellation.
func TestStopFinishesAfterCancel(t *testing.T) {
	f := newFake()
	u := testUnit(t, f, Options{Poll: service.PollOptions{Interval: time.Millisecond, Timeout: 5 * time.Second}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.onStop = func(p map[string]string) {
		cancel()
		p["ActiveState"] = "deactivating"
		f.showQueue = []map[string]string{
			{"ActiveState": "deactivating", "SubState": "stop-sigterm"},
			{"ActiveState": "deactivating", "SubState": "stop-sigkill"},
			{"ActiveState": "inactive", "SubState": "dead"},
		}
	}
	err := u.Stop(ctx, "demo")
	if !errors.Is(err, context.Canceled) || errors.Is(err, service.ErrTimeout) {
		t.Fatalf("Stop = %v, want the caller's cancellation alone", err)
	}
	v := f.verbs()
	stop := slices.Index(v, "stop")
	if stop < 0 || len(v)-stop-1 != 3 || len(f.showQueue) != 0 {
		t.Fatalf("verbs %q; want three probes after the stop, until inactive", v)
	}
}

// TestParseTimespan: systemctl show's time spans, and what is not one.
func TestParseTimespan(t *testing.T) {
	for in, want := range map[string]time.Duration{
		"1min 30s": 90 * time.Second,
		"5s":       5 * time.Second,
		"100ms":    100 * time.Millisecond,
		"2h":       2 * time.Hour,
		"1d 1us":   24*time.Hour + time.Microsecond,
	} {
		if got, ok := parseTimespan(in); !ok || got != want {
			t.Errorf("parseTimespan(%q) = %v, %t; want %v", in, got, ok, want)
		}
	}
	for _, in := range []string{"infinity", "", "1x", "s", "1.5s", "-1s", "99999999999999999999s"} {
		if got, ok := parseTimespan(in); ok {
			t.Errorf("parseTimespan(%q) = %v, want not a span", in, got)
		}
	}
}

// TestStopBound: the stop wait is the longer of the poll timeout and
// TimeoutStopUSec plus stopGrace; "infinity" or an unreadable value leaves
// the poll timeout.
func TestStopBound(t *testing.T) {
	for _, c := range []struct {
		value string
		poll  time.Duration
		want  time.Duration
	}{
		{"1min 30s", 0, 90*time.Second + stopGrace},
		{"5s", 100 * time.Millisecond, 5*time.Second + stopGrace},
		{"5s", 2 * time.Minute, 2 * time.Minute},
		{"infinity", time.Second, time.Second},
		{"", time.Second, time.Second},
		{"garbage", 0, service.DefaultPollTimeout},
	} {
		f := newFake()
		f.props["TimeoutStopUSec"] = c.value
		u := testUnit(t, f, Options{})
		u.o.Poll.Timeout = c.poll
		if got := u.stopBound(context.Background(), "demo.service"); got != c.want {
			t.Errorf("%q, poll %v: bound %v, want %v", c.value, c.poll, got, c.want)
		}
	}
}
