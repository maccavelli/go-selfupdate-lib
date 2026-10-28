package servicetest

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// TestFakeServiceStates: Arrange sets what Observe reports, and refuses a
// state it does not know.
func TestFakeServiceStates(t *testing.T) {
	ctx := context.Background()
	f := NewFakeService(FakeOptions{})
	for _, c := range []struct {
		a    Arrangement
		want Observation
	}{
		{Arrangement{State: Absent, Enabled: true}, Observation{}},
		{Arrangement{State: Stopped, Enabled: true}, Observation{Installed: true, Enabled: true}},
		{Arrangement{State: Running}, Observation{Installed: true, Running: true, Wanted: true}},
		{Arrangement{State: Stubborn, Enabled: true}, Observation{Installed: true, Running: true, Enabled: true, Wanted: true}},
		{Arrangement{State: Crashing}, Observation{Installed: true}},
	} {
		if err := f.Arrange(ctx, c.a); err != nil {
			t.Fatal(err)
		}
		got, err := f.Observe(ctx)
		got.Detail = ""
		if err != nil || got != c.want {
			t.Errorf("%+v: observed %+v, %v; want %+v", c.a, got, err, c.want)
		}
	}
	if err := f.Arrange(ctx, Arrangement{State: StartHangs + 1}); !errors.Is(err, errors.ErrUnsupported) {
		t.Fatalf("an unknown state: %v, want errors.ErrUnsupported", err)
	}
	if got := (StartHangs + 1).String(); got != "State(7)" {
		t.Fatalf("String of an unknown state = %q", got)
	}
}

// TestFakeServiceSlowStop: a SlowStop service goes StopLatency after the
// stop, and Stop waits for it.
func TestFakeServiceSlowStop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		f := NewFakeService(FakeOptions{StopLatency: 3 * time.Second})
		if err := f.Arrange(ctx, Arrangement{State: SlowStop, Enabled: true}); err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		if err := f.Stop(ctx, "demo"); err != nil {
			t.Fatal(err)
		}
		if took := time.Since(start); took < 3*time.Second || took > 3*time.Second+service.DefaultPollInterval {
			t.Fatalf("Stop took %s; want the 3 s latency, to the next look", took)
		}
	})
}
