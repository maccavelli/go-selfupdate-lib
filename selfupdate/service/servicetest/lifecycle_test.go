package servicetest

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// The tests run in synctest bubbles: the fake's 60-second bounds cost no
// wall time, and VerifyLifecycle's timers and the fake's share one clock.

func fakeCase(f *FakeService, l Lab) LifecycleCase {
	return LifecycleCase{
		Product: "demo", Lab: l, StopBound: f.StopBound(), StartBound: f.StartBound(),
		RestoresAfterFailedStop: true,
	}
}

func TestVerifyLifecyclePasses(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := NewFakeService(FakeOptions{})
		var skipped []string
		c := fakeCase(f, f)
		c.Logf = func(format string, args ...any) { skipped = append(skipped, fmt.Sprintf(format, args...)) }
		if err := VerifyLifecycle(context.Background(), f, c); err != nil {
			t.Fatal(err)
		}
		if len(skipped) != 0 {
			t.Fatalf("skipped %q; the fake arranges every state", skipped)
		}
		if o, err := f.Observe(context.Background()); err != nil || o.Installed {
			t.Fatalf("after the run the lab sees %+v, %v; want the service absent", o, err)
		}
	})
}

// broken is the fake with one rule of the contract broken.
type broken struct {
	*FakeService
	stop        func(f *FakeService, ctx context.Context, product string) error
	start       func(f *FakeService, ctx context.Context, product string) error
	running     func(f *FakeService, ctx context.Context, product string) (bool, error)
	enabled     func(f *FakeService, ctx context.Context, product string) (bool, error)
	installed   func(f *FakeService, ctx context.Context, product string) (bool, error)
	waitHealthy func(f *FakeService, ctx context.Context, product string) error
}

func (b broken) Stop(ctx context.Context, p string) error {
	if b.stop != nil {
		return b.stop(b.FakeService, ctx, p)
	}
	return b.FakeService.Stop(ctx, p)
}

func (b broken) Start(ctx context.Context, p string) error {
	if b.start != nil {
		return b.start(b.FakeService, ctx, p)
	}
	return b.FakeService.Start(ctx, p)
}

func (b broken) Running(ctx context.Context, p string) (bool, error) {
	if b.running != nil {
		return b.running(b.FakeService, ctx, p)
	}
	return b.FakeService.Running(ctx, p)
}

func (b broken) Enabled(ctx context.Context, p string) (bool, error) {
	if b.enabled != nil {
		return b.enabled(b.FakeService, ctx, p)
	}
	return b.FakeService.Enabled(ctx, p)
}

func (b broken) Installed(ctx context.Context, p string) (bool, error) {
	if b.installed != nil {
		return b.installed(b.FakeService, ctx, p)
	}
	return b.FakeService.Installed(ctx, p)
}

func (b broken) WaitHealthy(ctx context.Context, p string) error {
	if b.waitHealthy != nil {
		return b.waitHealthy(b.FakeService, ctx, p)
	}
	return b.FakeService.WaitHealthy(ctx, p)
}

var _ selfupdate.EnabledLifecycle = broken{}

func TestVerifyLifecycleFails(t *testing.T) {
	cases := []struct {
		name string
		// edit breaks one rule; free is the case's ContextFree.
		edit func(b *broken)
		free Method
		want string
	}{
		{"Stop ignores its context", func(b *broken) {
			b.stop = func(f *FakeService, ctx context.Context, p string) error {
				return f.issueStop(ctx, context.WithoutCancel(ctx), p)
			}
		}, 0, "ended context: after Stop on an ended context the lab sees running=false"},
		{"Stop returns nil for a service that ignores it", func(b *broken) {
			b.stop = func(f *FakeService, ctx context.Context, p string) error {
				if err := f.Stop(ctx, p); !errors.Is(err, service.ErrTimeout) {
					return err
				}
				return nil
			}
		}, 0, "stubborn: stop of a service that ignores it = no error"},
		{"Stop returns on its cancellation", func(b *broken) {
			b.stop = func(f *FakeService, ctx context.Context, p string) error {
				if err := ctx.Err(); err != nil {
					return err
				}
				return f.issueStop(ctx, ctx, p)
			}
		}, 0, "cancel during stop: stop returned on its cancellation while the service still ran"},
		{"Stop returns before the service has gone", func(b *broken) {
			b.stop = func(f *FakeService, ctx context.Context, _ string) error {
				if err := ctx.Err(); err != nil {
					return err
				}
				f.mu.Lock()
				early := f.st.mode == Running
				f.mu.Unlock()
				if early {
					return nil
				}
				return f.issueStop(ctx, context.WithoutCancel(ctx), "demo")
			}
		}, 0, "cycle: after Stop the lab sees running=true"},
		{"Running lies after a timeout", func(b *broken) {
			b.running = func(f *FakeService, ctx context.Context, p string) (bool, error) {
				f.mu.Lock()
				stopping := f.st.stopping
				f.mu.Unlock()
				if stopping {
					return false, nil
				}
				return f.Running(ctx, p)
			}
		}, 0, "stubborn: after the timed-out Stop, Running = false"},
		{"the restore is lost", func(b *broken) {
			b.stop = func(f *FakeService, ctx context.Context, p string) error {
				err := f.Stop(ctx, p)
				f.mu.Lock()
				f.st.wanted = false
				f.mu.Unlock()
				return err
			}
		}, 0, "stubborn: after the timed-out Stop the service runs, but its manager no longer wants it up"},
		{"Start installs an absent service", func(b *broken) {
			b.start = func(f *FakeService, ctx context.Context, p string) error {
				f.mu.Lock()
				f.st.installed = true
				f.mu.Unlock()
				return f.Start(ctx, p)
			}
		}, 0, "absent: after Start of an absent service the lab sees installed=true running=true"},
		{"Installed reports an absent service", func(b *broken) {
			b.installed = func(*FakeService, context.Context, string) (bool, error) { return true, nil }
		}, MethodInstalled, "absent: installed = true"},
		{"WaitHealthy passes a crash", func(b *broken) {
			b.waitHealthy = func(f *FakeService, ctx context.Context, p string) error {
				f.mu.Lock()
				crashed := f.st.crashed
				f.mu.Unlock()
				if crashed {
					return nil
				}
				return f.WaitHealthy(ctx, p)
			}
		}, 0, "crash: WaitHealthy of a service that exits at once = no error"},
		{"a hanging start is not ErrTimeout", func(b *broken) {
			b.start = func(f *FakeService, ctx context.Context, p string) error {
				err := f.Start(ctx, p)
				if errors.Is(err, service.ErrTimeout) {
					return errors.New("the start never finished")
				}
				return err
			}
		}, 0, "start hangs: start the manager never completes = the start never finished"},
		{"Enabled disagrees with the lab", func(b *broken) {
			b.enabled = func(*FakeService, context.Context, string) (bool, error) { return true, nil }
		}, MethodEnabled, "enabled: arranged enabled=false: Enabled = true"},
		{"Installed ignores its context, undocumented", func(b *broken) {
			b.installed = func(f *FakeService, _ context.Context, p string) (bool, error) {
				return f.Installed(context.Background(), p)
			}
		}, 0, "ended context: Installed on an ended context returned no error"},
		{"Start ignores its context, undocumented", func(b *broken) {
			b.start = func(f *FakeService, _ context.Context, p string) error {
				return f.Start(context.Background(), p)
			}
		}, 0, "ended context: Start on an ended context returned no error"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := NewFakeService(FakeOptions{})
				b := broken{FakeService: f}
				c.edit(&b)
				lc := fakeCase(f, f)
				lc.ContextFree = c.free
				err := VerifyLifecycle(context.Background(), b, lc)
				if err == nil || !strings.Contains(err.Error(), c.want) {
					t.Fatalf("err = %v\nwant one containing %q", err, c.want)
				}
			})
		})
	}
}

// TestVerifyLifecycleContextFree: a method the implementation documents as
// ignoring its context is not held to it.
func TestVerifyLifecycleContextFree(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := NewFakeService(FakeOptions{})
		b := broken{FakeService: f,
			installed: func(f *FakeService, _ context.Context, p string) (bool, error) {
				return f.Installed(context.Background(), p)
			},
			start: func(f *FakeService, _ context.Context, p string) error {
				return f.Start(context.Background(), p)
			},
		}
		c := fakeCase(f, f)
		c.ContextFree = MethodInstalled | MethodStart
		if err := VerifyLifecycle(context.Background(), b, c); err != nil {
			t.Fatal(err)
		}
	})
}

// unsupported is a lab that cannot arrange one state.
type unsupported struct {
	Lab
	state State
}

func (u unsupported) Arrange(ctx context.Context, a Arrangement) error {
	if a.State == u.state {
		return fmt.Errorf("lab: %s: %w", a.State, errors.ErrUnsupported)
	}
	return u.Lab.Arrange(ctx, a)
}

// TestVerifyLifecycleSkips: a state the lab cannot arrange skips the checks
// that need it, through Logf; with RequireAll it is a violation. A zero
// StartBound skips the start-hangs check the same way.
func TestVerifyLifecycleSkips(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := NewFakeService(FakeOptions{})
		var logged []string
		c := fakeCase(f, unsupported{Lab: f, state: Stubborn})
		c.StartBound = 0
		c.Logf = func(format string, args ...any) { logged = append(logged, fmt.Sprintf(format, args...)) }
		if err := VerifyLifecycle(context.Background(), f, c); err != nil {
			t.Fatal(err)
		}
		got := strings.Join(logged, "\n")
		if len(logged) != 2 || !strings.Contains(got, "stubborn: skipped") || !strings.Contains(got, "start hangs: skipped") {
			t.Fatalf("logged %q; want the stubborn and start-hangs checks skipped", logged)
		}
		c.RequireAll = true
		err := VerifyLifecycle(context.Background(), f, c)
		if err == nil || !strings.Contains(err.Error(), "stubborn: arrange Stubborn") || !strings.Contains(err.Error(), "start hangs: no LifecycleCase.StartBound") {
			t.Fatalf("RequireAll: err = %v; want both skips as violations", err)
		}
	})
}

func TestVerifyLifecycleRefusesItsCase(t *testing.T) {
	f := NewFakeService(FakeOptions{})
	good := fakeCase(f, f)
	for name, c := range map[string]struct {
		l    selfupdate.Lifecycle
		edit func(*LifecycleCase)
	}{
		"nil lifecycle": {nil, func(*LifecycleCase) {}},
		"nil lab":       {f, func(c *LifecycleCase) { c.Lab = nil }},
		"no product":    {f, func(c *LifecycleCase) { c.Product = "" }},
		"no StopBound":  {f, func(c *LifecycleCase) { c.StopBound = 0 }},
	} {
		lc := good
		c.edit(&lc)
		if err := VerifyLifecycle(context.Background(), c.l, lc); err == nil || !strings.HasPrefix(err.Error(), "servicetest: ") {
			t.Errorf("%s: err = %v, want a refusal", name, err)
		}
	}
	if o, err := f.Observe(context.Background()); err != nil || o.Installed {
		t.Fatalf("a refused case touched the lab: %+v, %v", o, err)
	}
}

// TestVerifyLifecycleTakesNoWallTime: the fake's default bounds, a minute
// each, pass in a bubble without the test waiting for them.
func TestVerifyLifecycleTakesNoWallTime(t *testing.T) {
	var fake time.Duration
	start := time.Now()
	synctest.Test(t, func(t *testing.T) {
		f := NewFakeService(FakeOptions{})
		began := time.Now()
		if err := VerifyLifecycle(context.Background(), f, fakeCase(f, f)); err != nil {
			t.Fatal(err)
		}
		fake = time.Since(began)
	})
	if fake < 2*service.DefaultPollTimeout {
		t.Fatalf("the run took %s of bubble time; want the stubborn stop's and the hanging start's minutes", fake)
	}
	if wall := time.Since(start); wall > 10*time.Second {
		t.Fatalf("the run took %s of wall time", wall)
	}
}
