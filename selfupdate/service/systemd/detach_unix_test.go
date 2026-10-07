//go:build unix

package systemd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// Tests for docs/decisions/0011-PLAN-reference-service-lifecycles.md V2
// step 6 and amendment A2: Detach and its environment file. They exercise
// the Linux paths and file modes, so they run on Unix; New refuses
// elsewhere.

func handOffSpec(t *testing.T) service.HandOff {
	t.Helper()
	return service.HandOff{
		ID: "abc123", Executable: "/opt/demo/demo", Args: []string{"update", "-y"},
		Env: []string{service.EnvHandOff + "=abc123", "SECRET_TOKEN=s3cret"}, ResultPath: "/opt/demo/.demo.selfupdate.handoff",
	}
}

func TestDetach(t *testing.T) {
	f := newFake()
	u := testUnit(t, f, Options{Scope: User, Unit: "relay@eu.service"})
	var envFile string
	det, err := u.Detach(context.Background(), handOffSpec(t))
	if err != nil {
		t.Fatal(err)
	}
	run := f.calls[len(f.calls)-1]
	want := []string{"/usr/bin/systemd-run", "--user", "--unit=relay-eu-selfupdate-abc123.service", "--collect", "--quiet",
		"--description=selfupdate handoff for relay@eu.service"}
	if !slices.Equal(run[:len(want)], want) {
		t.Fatalf("systemd-run %q", run)
	}
	for i, a := range run {
		if a == "-p" && strings.HasPrefix(run[i+1], "EnvironmentFile=") {
			envFile = strings.TrimPrefix(run[i+1], "EnvironmentFile=")
		}
	}
	if !slices.Contains(run, "Type=exec") || slices.Contains(run, "--no-block") ||
		!slices.Equal(run[len(run)-4:], []string{"--", "/opt/demo/demo", "update", "-y"}) {
		t.Fatalf("systemd-run %q", run)
	}
	for _, a := range run {
		if strings.Contains(a, "s3cret") {
			t.Fatalf("a secret reached the command line: %q", run)
		}
	}
	// Each unit's handoffs keep to its own directory (0015-MADR D8).
	if !strings.HasPrefix(envFile, u.getenv("XDG_RUNTIME_DIR")+"/selfupdate/relay@eu.service/handoff-abc123.env") {
		t.Fatalf("environment file %q", envFile)
	}
	if _, err := os.Stat(envFile); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the environment file was kept after a Type=exec start: %v", err)
	}
	if det.Where != "transient unit relay-eu-selfupdate-abc123.service" || det.ResultPath != handOffSpec(t).ResultPath {
		t.Fatalf("detached %+v", det)
	}
}

// TestDetachOldSystemd: before 240 there is no Type=exec, so the run is
// started without blocking and the file stays; before 236, no handoff.
func TestDetachOldSystemd(t *testing.T) {
	f := newFake()
	f.version = "systemd 239 (239-68.el8)"
	u := testUnit(t, f, Options{Scope: User})
	if _, err := u.Detach(context.Background(), handOffSpec(t)); err != nil {
		t.Fatal(err)
	}
	run := f.calls[len(f.calls)-1]
	if !slices.Contains(run, "--no-block") || slices.Contains(run, "Type=exec") {
		t.Fatalf("systemd-run %q", run)
	}
	file := filepath.Join(u.getenv("XDG_RUNTIME_DIR"), "selfupdate", "demo.service", "handoff-abc123.env")
	body := readFile(t, file)
	if !strings.Contains(body, `SECRET_TOKEN="s3cret"`) || !strings.Contains(body, service.EnvHandOff+`="abc123"`) {
		t.Fatalf("environment file %q", body)
	}
	info, err := os.Stat(file)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("environment file mode %v, %v", info.Mode(), err)
	}
	dir, err := os.Stat(filepath.Dir(file))
	if err != nil || dir.Mode().Perm() != 0o700 {
		t.Fatalf("directory mode %v, %v", dir.Mode(), err)
	}
	// The next handoff removes the file the last one left.
	spec := handOffSpec(t)
	spec.ID = "def456"
	if _, err := u.Detach(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("an earlier handoff's file was kept: %v", err)
	}

	f.version = "systemd 235"
	if _, err := u.Detach(context.Background(), handOffSpec(t)); !errors.Is(err, service.ErrUnsupported) {
		t.Fatalf("systemd 235: %v", err)
	}
	f.version = "garbage"
	if _, err := u.Detach(context.Background(), handOffSpec(t)); err == nil {
		t.Fatal("an unreadable version was accepted")
	}
}

func TestDetachFailureRemovesFile(t *testing.T) {
	f := newFake()
	f.fail["systemd-run"] = service.Output{ExitCode: 1, Stderr: []byte("Failed to start transient service unit: Access denied")}
	u := testUnit(t, f, Options{Scope: User})
	if _, err := u.Detach(context.Background(), handOffSpec(t)); !errors.Is(err, service.ErrPermission) {
		t.Fatalf("err = %v", err)
	}
	entries, _ := os.ReadDir(filepath.Join(u.getenv("XDG_RUNTIME_DIR"), "selfupdate", "demo.service"))
	if len(entries) != 0 {
		t.Fatalf("a failed handoff left %v", entries)
	}
}
