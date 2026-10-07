//go:build unix

package systemd

import (
	"context"
	"os"
	"strings"
	"testing"
)

// TestDetachKeepsOtherUnitsEnvFile: before systemd 240 an environment file
// stays until the next handoff removes it. Another unit's handoff, sharing
// the runtime directory, must not remove one that may still be starting
// (docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md D8).
func TestDetachKeepsOtherUnitsEnvFile(t *testing.T) {
	f := newFake()
	f.version = "systemd 239 (239-68.el8)"
	first := testUnit(t, f, Options{Scope: User, Unit: "demo.service"})
	second, err := newUnit(Options{
		Scope: User, Unit: "other.service", Systemctl: "/usr/bin/systemctl", SystemdRun: "/usr/bin/systemd-run",
		Runner: f, Poll: first.o.Poll,
	}, "linux", first.getenv)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Detach(context.Background(), handOffSpec(t)); err != nil {
		t.Fatal(err)
	}
	mine := envFileOf(t, f)
	if _, err := os.Stat(mine); err != nil {
		t.Fatalf("demo.service's env file: %v", err)
	}
	spec := handOffSpec(t)
	spec.ID = "def456"
	if _, err := second.Detach(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(mine); err != nil {
		t.Fatalf("other.service's handoff removed demo.service's env file: %v", err)
	}
	if want := "/selfupdate/demo.service/handoff-abc123.env"; !strings.HasSuffix(mine, want) {
		t.Fatalf("env file %s, want it under the unit's own directory (…%s)", mine, want)
	}
}

// envFileOf is the EnvironmentFile= of the last systemd-run call.
func envFileOf(t *testing.T, f *fakeSystemd) string {
	t.Helper()
	run := f.calls[len(f.calls)-1]
	for i, a := range run {
		if a == "-p" && i+1 < len(run) && strings.HasPrefix(run[i+1], "EnvironmentFile=") {
			return strings.TrimPrefix(run[i+1], "EnvironmentFile=")
		}
	}
	t.Fatalf("no EnvironmentFile= in %q", run)
	return ""
}
