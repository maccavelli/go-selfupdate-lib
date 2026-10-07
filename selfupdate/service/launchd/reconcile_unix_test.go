//go:build unix

package launchd

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// The rewrite keeps the plist's owner and mode, which are Unix's; the
// package runs on macOS.

func TestReconcileRewriteAndRestore(t *testing.T) {
	f := newFake()
	// As after Stop, which booted the job out: Start bootstraps the
	// rewritten plist (0015-MADR D3).
	f.loaded, f.pid, f.state = false, 0, "not running"
	j := testJob(t, f, Options{RewritePath: true})
	if err := os.Chmod(j.o.Plist, 0o640); err != nil {
		t.Fatal(err)
	}
	res, err := j.Reconcile(context.Background(), "demo", "/opt/new/demo")
	if err != nil || !res.Changed {
		t.Fatalf("%+v, %v", res, err)
	}
	if got := readString(t, j.o.Plist); got != "replaced ProgramArguments.0 with /opt/new/demo" {
		t.Fatalf("plist %q", got)
	}
	if info, err := os.Stat(j.o.Plist); err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("mode %v, %v", info.Mode(), err)
	}
	if err := j.Restore(context.Background(), "demo", res); err != nil {
		t.Fatal(err)
	}
	if got := readString(t, j.o.Plist); got != "<plist/>" {
		t.Fatalf("restored %q", got)
	}
	if err := j.Restore(context.Background(), "demo", selfupdate.ReconcileResult{Changed: true, State: "x"}); err == nil {
		t.Fatal("a foreign receipt was restored")
	}
}

// TestReconcileReloadsLoadedJob: a job loaded but not running, such as a
// RunAtLoad job whose process exited, is not booted out by Stop. After the
// rewrite it is reloaded, bootout then bootstrap, so launchd reads the new
// plist; kickstart alone would run the cached definition. Start then
// starts it, and Restore reloads the old plist the same way
// (docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md D3).
func TestReconcileReloadsLoadedJob(t *testing.T) {
	f := newFake()
	f.pid, f.state = 0, "not running"
	j := testJob(t, f, Options{RewritePath: true})
	ctx := context.Background()
	res, err := j.Reconcile(ctx, "demo", "/opt/new/demo")
	if err != nil || !res.Changed {
		t.Fatalf("Reconcile = %+v, %v", res, err)
	}
	v := f.verbs()
	out, in := slices.Index(v, "bootout"), slices.Index(v, "bootstrap")
	if out < 0 || in < out || slices.Contains(v, "kickstart") {
		t.Fatalf("verbs %q; want bootout, then bootstrap, and no kickstart", v)
	}
	if err := j.Start(ctx, "demo"); err != nil {
		t.Fatal(err)
	}
	if v := f.verbs(); v[len(v)-1] != "kickstart" {
		t.Fatalf("verbs %q; want Start to kickstart the reloaded job", v)
	}
	if err := j.WaitHealthy(ctx, "demo"); err != nil {
		t.Fatal(err)
	}
	f.loaded, f.pid, f.state = true, 0, "not running"
	f.calls = nil
	if err := j.Restore(ctx, "demo", res); err != nil {
		t.Fatal(err)
	}
	if v := f.verbs(); !slices.Contains(v, "bootout") || !slices.Contains(v, "bootstrap") {
		t.Fatalf("Restore verbs %q; want a reload", v)
	}
}

// TestReconcileReloadedProcessIsNew: a RunAtLoad job starts at the
// reload's bootstrap, on the new plist, and kickstart leaves a running job
// alone. Start does not take that process for the one before the update,
// so WaitHealthy accepts it (0015-MADR D3).
func TestReconcileReloadedProcessIsNew(t *testing.T) {
	f := newFake()
	f.pid, f.state = 0, "not running"
	f.onBootstrap = func(f *fakeLaunchd) { f.pid, f.state = 300, "running" }
	f.onStart = nil
	j := testJob(t, f, Options{RewritePath: true, Poll: service.PollOptions{Settle: -1, Timeout: 200 * time.Millisecond}})
	ctx := context.Background()
	if res, err := j.Reconcile(ctx, "demo", "/opt/new/demo"); err != nil || !res.Changed {
		t.Fatalf("Reconcile = %+v, %v", res, err)
	}
	if err := j.Start(ctx, "demo"); err != nil {
		t.Fatal(err)
	}
	if err := j.WaitHealthy(ctx, "demo"); err != nil {
		t.Fatalf("WaitHealthy = %v; the reload's process runs the new plist", err)
	}
}

// TestReconcileRefusesRunningJob: a rewrite of a job that is running would
// need it stopped; the plist is not touched (0015-MADR D3).
func TestReconcileRefusesRunningJob(t *testing.T) {
	f := newFake()
	j := testJob(t, f, Options{RewritePath: true})
	if _, err := j.Reconcile(context.Background(), "demo", "/opt/new/demo"); err == nil || !strings.Contains(err.Error(), "stop the job first") {
		t.Fatalf("err = %v, want the running job refused", err)
	}
	if got := readString(t, j.o.Plist); got != "<plist/>" {
		t.Fatalf("plist %q, want it untouched", got)
	}
}
