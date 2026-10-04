package launchd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// TestCapturedOutput replays launchctl output captured on the development
// host (macOS 26) through probe (0011-PLAN V3 step 8): list for a domain it
// sees, print for one it does not. print's nested "state = active" lines,
// in the coalition blocks, are not the job's state. A job launchd is still
// spawning shows state xpcproxy with its PID: print does not count it as
// running yet, list does. list quotes strings without escaping them.
func TestCapturedOutput(t *testing.T) {
	for _, c := range []struct {
		file            string
		domain          Domain
		pid             int
		loaded, running bool
	}{
		{"list-running.txt", GUI(503), 71739, true, true},
		{"list-idle.txt", GUI(503), 0, true, false},
		{"print-running.txt", User(503), 71739, true, true},
		{"print-xpcproxy.txt", User(503), 71179, true, false},
		{"print-idle.txt", User(503), 0, true, false},
		// Booted out, ignoring SIGTERM until ExitTimeOut: print knows it is
		// going; list shows only its PID. Stop waits on print (waitGone).
		{"print-sigtermed.txt", User(503), 80390, true, false},
		{"list-sigtermed.txt", GUI(503), 80390, true, true},
	} {
		t.Run(c.file, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("testdata", c.file))
			if err != nil {
				t.Fatal(err)
			}
			j := testJob(t, newFake(), Options{Domain: c.domain})
			var verbs []string
			j.o.Runner = service.RunnerFunc(func(_ context.Context, cmd service.Command) (service.Output, error) {
				verbs = append(verbs, cmd.Args[0])
				if cmd.Args[0] == "managername" {
					return service.Output{Stdout: []byte("Aqua\n")}, nil
				}
				return service.Output{Stdout: raw}, nil
			})
			s, err := j.probe(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if s.pid != c.pid || s.loaded != c.loaded || s.running != c.running {
				t.Fatalf("probe %+v", s)
			}
			want := "print"
			if strings.HasPrefix(c.file, "list-") {
				want = "list"
			}
			if verbs[len(verbs)-1] != want {
				t.Fatalf("probe ran %q; want %s last", verbs, want)
			}
		})
	}
}
