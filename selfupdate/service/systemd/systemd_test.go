package systemd

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/internal/testproc"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// Tests for docs/decisions/0011-PLAN-reference-service-lifecycles.md V2
// steps 1 to 4, over the scripted fake.

func TestValidUnitName(t *testing.T) {
	testproc.CheckNoLeak(t)
	for name, ok := range map[string]bool{
		"relay.service":                       true,
		"relay@eu-1.service":                  true,
		`relay@a\x2db.service`:                true,
		"relay.socket":                        false,
		".service":                            false,
		"-relay.service":                      false,
		"relay@.service":                      false,
		"relay@a@b.service":                   false,
		"re lay.service":                      false,
		"relay;rm.service":                    false,
		strings.Repeat("a", 248) + ".service": false,
	} {
		if err := ValidUnitName(name); (err == nil) != ok {
			t.Errorf("ValidUnitName(%q) = %v, want ok %t", name, err, ok)
		}
	}
}

func TestEscapeAndTemplateInstance(t *testing.T) {
	testproc.CheckNoLeak(t)
	for in, want := range map[string]string{
		"eu/1":    "eu-1",
		".hidden": `\x2ehidden`,
		"a b":     `a\x20b`,
		"x.y_z:1": "x.y_z:1",
		"ümlaut":  `\xc3\xbcmlaut`,
	} {
		if got := Escape(in); got != want {
			t.Errorf("Escape(%q) = %q, want %q", in, got, want)
		}
	}
	if got, err := TemplateInstance("relay@.service", "eu/1"); err != nil || got != "relay@eu-1.service" {
		t.Fatalf("TemplateInstance = %q, %v", got, err)
	}
	for _, bad := range [][2]string{{"relay.service", "x"}, {"relay@.service", ""}} {
		if _, err := TemplateInstance(bad[0], bad[1]); err == nil {
			t.Errorf("TemplateInstance(%q, %q) accepted", bad[0], bad[1])
		}
	}
}

func TestNewRefuses(t *testing.T) {
	testproc.CheckNoLeak(t)
	env := func(string) string { return "" }
	if _, err := newUnit(Options{}, "darwin", env); !errors.Is(err, service.ErrUnsupported) {
		t.Fatalf("off Linux: %v", err)
	}
	if _, err := newUnit(Options{Scope: User, Systemctl: "/x/systemctl", SystemdRun: "/x/systemd-run"}, "linux", env); err == nil ||
		!strings.Contains(err.Error(), "XDG_RUNTIME_DIR") {
		t.Fatalf("user scope without XDG_RUNTIME_DIR: %v", err)
	}
	if _, err := newUnit(Options{Systemctl: "systemctl", SystemdRun: "/x/r"}, "linux", env); err == nil {
		t.Fatal("a relative tool path was accepted")
	}
	if _, err := newUnit(Options{Unit: "bad name.service"}, "linux", env); err == nil {
		t.Fatal("a bad unit name was accepted")
	}
	if _, err := newUnit(Options{Scope: 7, Systemctl: "/x/s", SystemdRun: "/x/r"}, "linux", env); err == nil {
		t.Fatal("an unknown scope was accepted")
	}
	// 0015-MADR D10.
	if _, err := newUnit(Options{Systemctl: "/x/s", SystemdRun: "/x/r", Poll: service.PollOptions{Timeout: 5 * time.Second}}, "linux", env); err == nil ||
		!strings.Contains(err.Error(), "Poll.Settle") {
		t.Fatalf("a settle as long as the timeout: %v", err)
	}
}

func TestProbes(t *testing.T) {
	testproc.CheckNoLeak(t)
	f := newFake()
	u := testUnit(t, f, Options{})
	ctx := context.Background()
	for state, want := range map[string]bool{"loaded": true, "masked": true, "not-found": false, "": false} {
		f.props["LoadState"] = state
		if got, err := u.Installed(ctx, "demo"); err != nil || got != want {
			t.Errorf("LoadState %q: Installed %t, %v", state, got, err)
		}
	}
	for state, want := range map[string]bool{
		"active": true, "reloading": true, "refreshing": true, "activating": true, "deactivating": true,
		"inactive": false, "failed": false, "maintenance": false,
	} {
		f.props["ActiveState"] = state
		if got, err := u.Running(ctx, "demo"); err != nil || got != want {
			t.Errorf("ActiveState %q: Running %t, %v", state, got, err)
		}
	}
	// is-enabled exits 0 for static, indirect, generated, alias and
	// transient; none means "starts at boot" (systemctl(1)).
	for state, want := range map[string]bool{
		"enabled": true, "enabled-runtime": true, "static": false, "indirect": false, "generated": false,
		"alias": false, "transient": false, "disabled": false, "masked": false, "linked": false,
	} {
		f.props["UnitFileState"] = state
		if got, err := u.Enabled(ctx, "demo"); err != nil || got != want {
			t.Errorf("UnitFileState %q: Enabled %t, %v", state, got, err)
		}
	}
}

// TestCommandShape: --user in user scope, -- before the unit, and the
// built environment, never this process's.
func TestCommandShape(t *testing.T) {
	testproc.CheckNoLeak(t)
	t.Setenv("HOME", "/home/<user>")
	f := newFake()
	u := testUnit(t, f, Options{Scope: User})
	if _, err := u.Running(context.Background(), "demo"); err != nil {
		t.Fatal(err)
	}
	call := f.calls[0]
	if call[0] != "/usr/bin/systemctl" || call[1] != "--user" || call[2] != "show" ||
		!slices.Contains(call, "--no-pager") || call[len(call)-2] != "--" || call[len(call)-1] != "demo.service" {
		t.Fatalf("call %q", call)
	}
	env := f.envs[0]
	if !slices.Contains(env, "LC_ALL=C") || !slices.Contains(env, "SYSTEMD_PAGER=cat") || slices.Contains(env, "HOME=/home/<user>") ||
		!slices.ContainsFunc(env, func(s string) bool { return strings.HasPrefix(s, "XDG_RUNTIME_DIR=") }) {
		t.Fatalf("env %q", env)
	}
}

func TestDefaultUnitFromProduct(t *testing.T) {
	testproc.CheckNoLeak(t)
	f := newFake()
	u := testUnit(t, f, Options{})
	u.o.Unit = ""
	if _, err := u.Running(context.Background(), "relay"); err != nil {
		t.Fatal(err)
	}
	if last := f.calls[len(f.calls)-1]; last[len(last)-1] != "relay.service" {
		t.Fatalf("unit %q", last[len(last)-1])
	}
	if _, err := u.Running(context.Background(), "bad name"); err == nil {
		t.Fatal("an invalid product unit was accepted")
	}
}

func TestStopWaitsForInactive(t *testing.T) {
	testproc.CheckNoLeak(t)
	synctest.Test(t, func(t *testing.T) {
		f := newFake()
		u := testUnit(t, f, Options{Poll: bubblePoll})
		// The first probe after the stop sees deactivating, the next inactive.
		f.onStop = func(p map[string]string) {
			p["ActiveState"] = "deactivating"
			f.showQueue = []map[string]string{
				{"ActiveState": "deactivating", "SubState": "stop-sigterm"},
				{"ActiveState": "inactive", "SubState": "dead"},
			}
		}
		if err := u.Stop(context.Background(), "demo"); err != nil {
			t.Fatal(err)
		}
		v := f.verbs()
		stop := slices.Index(v, "stop")
		if stop < 0 || len(v)-stop-1 != 2 {
			t.Fatalf("verbs %q; want two probes after the stop", v)
		}
	})
}

func TestStopStalls(t *testing.T) {
	testproc.CheckNoLeak(t)
	synctest.Test(t, func(t *testing.T) {
		f := newFake()
		u := testUnit(t, f, Options{Poll: bubblePoll})
		f.onStop = func(p map[string]string) { p["ActiveState"] = "deactivating" }
		bound := u.stopBound(context.Background(), "demo.service")
		start := time.Now()
		if err := u.Stop(context.Background(), "demo"); !errors.Is(err, service.ErrTimeout) {
			t.Fatalf("err = %v, want a timeout", err)
		}
		if took := time.Since(start); took != bound {
			t.Fatalf("timed out after %s, want the stop's bound, %s", took, bound)
		}
	})
}

func TestStopRefusesInsideUnit(t *testing.T) {
	testproc.CheckNoLeak(t)
	f := newFake()
	u := testUnit(t, f, Options{})
	setCgroup(t, "0::/system.slice/demo.service/agent\n")
	if err := u.Stop(context.Background(), "demo"); !errors.Is(err, service.ErrInsideService) {
		t.Fatalf("err = %v", err)
	}
	if slices.Contains(f.verbs(), "stop") {
		t.Fatal("the unit was stopped from inside it")
	}
}

func TestCommandErrors(t *testing.T) {
	testproc.CheckNoLeak(t)
	f := newFake()
	u := testUnit(t, f, Options{})
	f.fail["stop"] = service.Output{ExitCode: 4, Stderr: []byte("Failed to stop demo.service: Access denied")}
	if err := u.Stop(context.Background(), "demo"); !errors.Is(err, service.ErrPermission) {
		t.Fatalf("err = %v, want ErrPermission", err)
	}
	f.fail["start"] = service.Output{ExitCode: 5, Stderr: []byte("Failed to start demo.service: Unit demo.service not found.")}
	if err := u.Start(context.Background(), "demo"); !errors.Is(err, service.ErrNotInstalled) {
		t.Fatalf("err = %v, want ErrNotInstalled", err)
	}
	// Another unit's "not found", such as a missing dependency, does not
	// make this one uninstalled (0015-MADR D11).
	f.fail["start"] = service.Output{ExitCode: 5, Stderr: []byte("Failed to start demo.service: Unit missing-dep.service not found.")}
	if err := u.Start(context.Background(), "demo"); err == nil || errors.Is(err, service.ErrNotInstalled) {
		t.Fatalf("err = %v, want an error that is not ErrNotInstalled", err)
	}
	f.fail["stop"] = service.Output{ExitCode: 5, Stderr: []byte("Failed to stop demo.service: Unit demo.service not loaded.")}
	if err := u.Stop(context.Background(), "demo"); !errors.Is(err, service.ErrNotInstalled) {
		t.Fatalf("not loaded: err = %v, want ErrNotInstalled", err)
	}
	f.fail["show"] = service.Output{ExitCode: 1, Stderr: []byte("Failed to connect to bus")}
	if _, err := u.Running(context.Background(), "demo"); err == nil || !strings.Contains(err.Error(), "Failed to connect to bus") {
		t.Fatalf("err = %v", err)
	}
}

// failOnStart is the fake, which then leaves the unit as a failed start
// did: its invocation, ActiveState and Result.
type failOnStart struct {
	f                   *fakeSystemd
	inv, active, result string
}

func (c failOnStart) Run(ctx context.Context, cmd service.Command) (service.Output, error) {
	out, err := c.f.Run(ctx, cmd)
	if slices.Contains(cmd.Args, "start") {
		c.f.mu.Lock()
		c.f.props["InvocationID"], c.f.props["ActiveState"], c.f.props["Result"] = c.inv, c.active, c.result
		c.f.mu.Unlock()
	}
	return out, err
}

// TestStartOfCrashedUnitIsUnhealthy: a start that fails because the unit
// ran and failed, as a Type=notify program that exits before it is ready
// does, wraps ErrUnhealthy as well as the CommandError; one that fails
// before the unit ran, as on a missing dependency, does not, though an
// earlier failure left it failed
// (docs/decisions/0038-MADR-pinned-plans-lifecycle-conformance-and-bubble-tests.md
// Amendment A4).
func TestStartOfCrashedUnitIsUnhealthy(t *testing.T) {
	testproc.CheckNoLeak(t)
	for _, c := range []struct {
		name      string
		after     failOnStart
		unhealthy bool
	}{
		{"failed", failOnStart{inv: "bbb", active: "failed", result: "exit-code"}, true},
		{"inactive", failOnStart{inv: "bbb", active: "inactive", result: "exit-code"}, true},
		{"not run", failOnStart{inv: "aaa", active: "failed", result: "exit-code"}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newFake()
			u := testUnit(t, f, Options{})
			c.after.f = f
			u.o.Runner = c.after
			f.fail["start"] = service.Output{ExitCode: 1}
			err := u.Start(context.Background(), "demo")
			var ce *service.CommandError
			if !errors.As(err, &ce) || errors.Is(err, service.ErrUnhealthy) != c.unhealthy {
				t.Fatalf("err = %v; want a CommandError, ErrUnhealthy %t", err, c.unhealthy)
			}
		})
	}
}

func TestStartThenWaitHealthy(t *testing.T) {
	testproc.CheckNoLeak(t)
	synctest.Test(t, func(t *testing.T) {
		f := newFake()
		appProbed := 0
		u := testUnit(t, f, Options{Poll: bubblePoll, Probe: func(context.Context) (service.Health, error) {
			appProbed++
			return service.Health{Ready: true}, nil
		}})
		ctx := context.Background()
		if err := u.Start(ctx, "demo"); err != nil {
			t.Fatal(err)
		}
		if err := u.WaitHealthy(ctx, "demo"); err != nil || appProbed != 1 {
			t.Fatalf("err = %v, application probe ran %d times", err, appProbed)
		}
	})
}

// TestWaitHealthyNeedsNewInvocation: the invocation from before the start
// never counts.
func TestWaitHealthyNeedsNewInvocation(t *testing.T) {
	testproc.CheckNoLeak(t)
	synctest.Test(t, func(t *testing.T) {
		f := newFake()
		u := testUnit(t, f, Options{Poll: bubblePoll})
		f.onStart = func(p map[string]string) { p["ActiveState"] = "active" } // same InvocationID
		if err := u.Start(context.Background(), "demo"); err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		if err := u.WaitHealthy(context.Background(), "demo"); !errors.Is(err, service.ErrTimeout) {
			t.Fatalf("err = %v, want a timeout", err)
		}
		if took := time.Since(start); took != service.DefaultPollTimeout {
			t.Fatalf("timed out after %s, want %s", took, service.DefaultPollTimeout)
		}
	})
}

func TestWaitHealthyFailsFast(t *testing.T) {
	testproc.CheckNoLeak(t)
	for name, after := range map[string]func(map[string]string){
		"failed":   func(p map[string]string) { p["ActiveState"], p["InvocationID"] = "failed", "bbb" },
		"inactive": func(p map[string]string) { p["ActiveState"], p["InvocationID"] = "inactive", "bbb" },
		"auto-restart": func(p map[string]string) {
			p["ActiveState"], p["SubState"], p["InvocationID"] = "activating", "auto-restart", "bbb"
		},
		"restarted": nil, // below: a restart after the start's baseline
	} {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := newFake()
				u := testUnit(t, f, Options{Poll: bubblePoll})
				f.onStart = after
				if after == nil {
					// The baseline is read once the start returned, so the
					// restart comes after it (0015-MADR D2).
					f.onStart = func(map[string]string) {
						f.showQueue = []map[string]string{
							{"NRestarts": "0"},
							{"ActiveState": "active", "SubState": "running", "InvocationID": "ccc", "NRestarts": "1"},
						}
					}
				}
				if err := u.Start(context.Background(), "demo"); err != nil {
					t.Fatal(err)
				}
				if err := u.WaitHealthy(context.Background(), "demo"); !errors.Is(err, service.ErrUnhealthy) {
					t.Fatalf("err = %v, want ErrUnhealthy", err)
				}
			})
		})
	}
}

// TestCommandErrorAs: a failed systemctl run is a *service.CommandError,
// still classified, with the text it had before
// (docs/decisions/0025-MADR-align-with-go-sdk-design-practices.md D2).
func TestCommandErrorAs(t *testing.T) {
	testproc.CheckNoLeak(t)
	for _, c := range []struct {
		name   string
		out    service.Output
		class  error
		prefix string
	}{
		{"permission", service.Output{Stderr: []byte("Failed to stop demo.service: Access denied\n"), ExitCode: 4}, service.ErrPermission, service.ErrPermission.Error() + ": "},
		{"missing", service.Output{Stderr: []byte("Unit demo.service not loaded.\n"), ExitCode: 5}, service.ErrNotInstalled, service.ErrNotInstalled.Error() + ": "},
		{"other", service.Output{Stdout: []byte("something else\n"), ExitCode: 1}, nil, ""},
	} {
		err := commandError("stop", "demo.service", c.out)
		var ce *service.CommandError
		if !errors.As(err, &ce) || ce.Backend != "systemd" || ce.Step != "stop" || ce.Name != "demo.service" || ce.ExitCode != c.out.ExitCode {
			t.Errorf("%s: err = %v, want a *service.CommandError", c.name, err)
			continue
		}
		if c.class != nil && !errors.Is(err, c.class) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.class)
		}
		msg := strings.TrimSpace(string(c.out.Stderr))
		if msg == "" {
			msg = strings.TrimSpace(string(c.out.Stdout))
		}
		if want := c.prefix + "selfupdate: systemd: stop demo.service: exit " + strconv.Itoa(c.out.ExitCode) + ": " + msg; err.Error() != want {
			t.Errorf("%s: Error() = %q, want %q", c.name, err.Error(), want)
		}
	}
}
