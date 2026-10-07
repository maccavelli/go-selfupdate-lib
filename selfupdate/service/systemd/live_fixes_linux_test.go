//go:build linux

package systemd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// Live tests for docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md
// D2, D4 and D11, against a real systemd, as the other live tests run.

// TestLiveStartDuringAutoRestart: a start issued while the unit waits out
// RestartSec ends the wait, and systemd counts it as a restart. The count
// WaitHealthy compares against is read after the start, so the unit is
// healthy (D2, 0015-MADR A3).
func TestLiveStartDuringAutoRestart(t *testing.T) {
	scope := requireLive(t)
	e := newLiveUnitWith(t, scope, liveUnitOpts{service: "Restart=always\nRestartSec=6\n"})
	wait := func(what string, ok func() bool) {
		t.Helper()
		for deadline := time.Now().Add(30 * time.Second); !ok(); time.Sleep(100 * time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatalf("%s: NRestarts=%s ActiveState=%s SubState=%s",
					what, e.show(t, "NRestarts"), e.show(t, "ActiveState"), e.show(t, "SubState"))
			}
		}
	}
	if err := e.systemctl("kill", "--kill-whom=main", "--signal=KILL", e.unit); err != nil {
		t.Fatal(err)
	}
	wait("the first restart", func() bool { return e.show(t, "NRestarts") == "1" && e.show(t, "ActiveState") == "active" })
	if err := e.systemctl("kill", "--kill-whom=main", "--signal=KILL", e.unit); err != nil {
		t.Fatal(err)
	}
	wait("the restart wait", func() bool { return e.show(t, "SubState") == "auto-restart" })
	u, err := New(Options{Unit: e.unit, Scope: scope})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := u.Start(ctx, "demo"); err != nil {
		t.Fatal(err)
	}
	if err := u.WaitHealthy(ctx, "demo"); err != nil {
		t.Fatalf("WaitHealthy = %v; the start during the restart wait is counted, and the unit is healthy", err)
	}
}

// TestLiveRewriteWithOverride: an override.conf that resets ExecStart
// loads after this package's drop-in, so the unit still runs the old
// binary. Reconcile says so, and its receipt's Restore removes the
// drop-in (D4).
func TestLiveRewriteWithOverride(t *testing.T) {
	scope := requireLive(t)
	e := newLiveUnit(t, scope)
	ctx := context.Background()
	dropIns := e.unitFile + ".d"
	if err := os.MkdirAll(dropIns, 0o755); err != nil { //nolint:gosec // a unit directory
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(dropIns)
		_ = e.systemctl("daemon-reload")
	})
	override := "[Service]\nExecStart=\nExecStart=\"" + e.target + "\"\n"
	if err := os.WriteFile(filepath.Join(dropIns, "override.conf"), []byte(override), 0o644); err != nil { //nolint:gosec // systemd reads units as any user
		t.Fatal(err)
	}
	if err := e.systemctl("daemon-reload"); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(e.dir, "moved")
	self, _ := os.Executable()
	copyFile(t, self, moved, nil)
	u, err := New(Options{Unit: e.unit, Scope: scope, RewritePath: true})
	if err != nil {
		t.Fatal(err)
	}
	res, err := u.Reconcile(ctx, "demo", moved)
	if err == nil || !res.Changed {
		t.Fatalf("Reconcile = %+v, %v; want the override reported, with the receipt", res, err)
	}
	if err := u.Restore(ctx, "demo", res); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dropIns, dropInName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the drop-in was kept after Restore: %v", err)
	}
	if p := execStartPath(e.show(t, "ExecStart")); p != e.target {
		t.Fatalf("ExecStart path %q after Restore, want %q", p, e.target)
	}
}

// TestLiveMissingDependency: a start that fails because a unit this one
// requires does not exist is not ErrNotInstalled: this unit is installed
// (D11).
func TestLiveMissingDependency(t *testing.T) {
	scope := requireLive(t)
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	missing := "selfupdate-live-missing-" + hex.EncodeToString(b[:]) + ".service"
	e := newLiveUnitWith(t, scope, liveUnitOpts{unit: "Requires=" + missing + "\nAfter=" + missing + "\n", noStart: true})
	u, err := New(Options{Unit: e.unit, Scope: scope})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if ok, err := u.Installed(ctx, "demo"); err != nil || !ok {
		t.Fatalf("Installed = %t, %v", ok, err)
	}
	err = u.Start(ctx, "demo")
	if err == nil || errors.Is(err, service.ErrNotInstalled) {
		t.Fatalf("Start = %v; want a failure that is not ErrNotInstalled", err)
	}
}
