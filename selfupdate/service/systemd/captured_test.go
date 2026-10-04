package systemd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// TestCapturedShowOutput replays `systemctl show` output captured from real
// hosts (testdata/, systemd 255 and 259) through the probes, pinning the
// format this package parses (0011-PLAN V2 step 8). The captures show that
// properties come in systemd's order, not -p's, so they are parsed by key;
// and that a unit that does not exist exits 0 with LoadState=not-found.
func TestCapturedShowOutput(t *testing.T) {
	for _, c := range []struct {
		file                        string
		installed, running, enabled bool
		program                     string
	}{
		// systemd-journald is static: is-enabled exits 0, yet nothing
		// enables it.
		{"show-active-255.txt", true, true, false, "/usr/lib/systemd/systemd-journald"},
		{"show-active-259.txt", true, true, false, "/usr/sbin/sshd"},
		{"show-not-found-259.txt", false, false, false, ""},
	} {
		t.Run(c.file, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("testdata", c.file))
			if err != nil {
				t.Fatal(err)
			}
			replay := service.RunnerFunc(func(context.Context, service.Command) (service.Output, error) {
				return service.Output{Stdout: raw}, nil
			})
			u := testUnit(t, newFake(), Options{})
			u.o.Runner = replay
			ctx := context.Background()
			installed, err1 := u.Installed(ctx, "demo")
			running, err2 := u.Running(ctx, "demo")
			enabled, err3 := u.Enabled(ctx, "demo")
			if err1 != nil || err2 != nil || err3 != nil {
				t.Fatal(err1, err2, err3)
			}
			if installed != c.installed || running != c.running || enabled != c.enabled {
				t.Fatalf("installed %t running %t enabled %t", installed, running, enabled)
			}
			p, err := u.show(ctx, "demo.service", "ExecStart")
			if err != nil {
				t.Fatal(err)
			}
			if got := execStartPath(p["ExecStart"]); got != c.program {
				t.Fatalf("ExecStart program %q, want %q", got, c.program)
			}
		})
	}
}
