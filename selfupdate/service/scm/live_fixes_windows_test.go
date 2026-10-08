//go:build windows

package scm

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/windows/svc/mgr"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// The live test for docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md
// D5, against this host's SCM, as the other live tests run.

// binaryPath reads, or with set, replaces, the live service's command line.
func binaryPath(t *testing.T, set string) string {
	t.Helper()
	m, err := mgr.Connect()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Disconnect() }()
	s, err := m.OpenService(liveName)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	c, err := s.Config()
	if err != nil {
		t.Fatal(err)
	}
	if set != "" {
		c.BinaryPathName = set
		if err := s.UpdateConfig(c); err != nil {
			t.Fatal(err)
		}
	}
	return c.BinaryPathName
}

// TestLiveUnquotedPathWithSpace: the service is registered by an unquoted
// command line whose program path has a space, which the SCM runs. A
// rewrite to a binary in another such directory keeps every argument, and
// the service starts from it (D5).
func TestLiveUnquotedPathWithSpace(t *testing.T) {
	cfg := newLiveService(t)
	ctx := context.Background()
	plain := liveSvc(t, cfg)
	if err := plain.Stop(ctx, "demo"); err != nil {
		t.Fatal(err)
	}
	binaryPath(t, cfg.Target+" "+liveArg+" service "+syscall.EscapeArg(cfg.Path))
	pid := waitReady(t, cfg, 0)
	if err := plain.Start(ctx, "demo"); err != nil {
		t.Fatalf("the SCM did not start the unquoted command line: %v", err)
	}
	pid = waitReady(t, cfg, pid)
	if res, err := plain.Reconcile(ctx, "demo", cfg.Target); err != nil || res.Changed {
		t.Fatalf("Reconcile of the service's own binary: %+v, %v", res, err)
	}
	if err := plain.Stop(ctx, "demo"); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(filepath.Dir(cfg.Dir), "scm moved", "demo.exe")
	if err := os.Mkdir(filepath.Dir(moved), 0o755); err != nil { //nolint:gosec // the service, LocalSystem, runs from here
		t.Fatal(err)
	}
	build(t, moved, "\nmoved build\n")
	s, err := New(Options{Name: cfg.Name, Poll: plain.o.Poll, RewritePath: true})
	if err != nil {
		t.Fatal(err)
	}
	res, err := s.Reconcile(ctx, "demo", moved)
	if err != nil || !res.Changed {
		t.Fatalf("Reconcile = %+v, %v", res, err)
	}
	line := binaryPath(t, "")
	args, err := sysManager{}.decompose(line)
	if want := []string{moved, liveArg, "service", cfg.Path}; err != nil || !slices.Equal(args, want) {
		t.Fatalf("rewrote %s, which splits as %q, %v; want %q", line, args, err, want)
	}
	if err := s.Start(ctx, "demo"); err != nil {
		t.Fatalf("Start from the rewritten command line: %v", err)
	}
	waitReady(t, cfg, pid)
}

// TestLiveHandOffHealthFailureReportsRollback: the handed-off build is not
// a service program, so the detached run's start fails. Its result reports
// the rollback, and the service runs the previous build afterwards
// (docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md G3).
func TestLiveHandOffHealthFailureReportsRollback(t *testing.T) {
	cfg := newLiveService(t)
	before := waitReady(t, cfg, 0)
	whoami, err := os.ReadFile(filepath.Join(os.Getenv("SystemRoot"), "System32", "whoami.exe"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.New, whoami, 0o755); err != nil { //nolint:gosec // an executable fixture
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg.Dir, "trigger"), nil, 0o644); err != nil { //nolint:gosec // read by the service
		t.Fatal(err)
	}
	result := service.DefaultResultPath(cfg.Target)
	for deadline := time.Now().Add(150 * time.Second); ; time.Sleep(200 * time.Millisecond) {
		for _, name := range []string{"inside-error", "service-error"} {
			if msg, err := os.ReadFile(filepath.Join(cfg.Dir, name)); err == nil { //nolint:gosec // the live test's own file
				t.Fatalf("%s: %s", name, msg)
			}
		}
		if r, err := service.ReadHandOffResult(result); err == nil {
			if r.ExitCode != 1 || r.Result.Applied || !r.Result.RolledBack {
				t.Fatalf("handoff result %+v; want exit 1, not applied, rolled back", r)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no handoff result within 150 s")
		}
	}
	waitReady(t, cfg, before)
	if got, err := os.ReadFile(cfg.Target); err != nil || !strings.HasSuffix(string(got), "\nold build\n") { //nolint:gosec // the live test's own file
		t.Fatalf("the service's binary is not the previous build: %v", err)
	}
}
