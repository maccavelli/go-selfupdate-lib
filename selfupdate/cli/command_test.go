package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/buildinfo"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
)

// args is the scenario's command line.
func (sc scenario) args() []string {
	if sc.req != nil { // contradiction: the flags Request must refuse
		return []string{"--check", "--yes"}
	}
	var a []string
	for _, f := range []struct {
		on   bool
		flag string
	}{{sc.flags.Check, "--check"}, {sc.flags.Yes, "--yes"}, {sc.flags.Force, "--force"}, {sc.flags.DryRun, "--dry-run"}} {
		if f.on {
			a = append(a, f.flag)
		}
	}
	return a
}

// command runs the scenario through Command, with product as its name.
func (sc scenario) command(t *testing.T, product string, asJSON bool) outcome {
	t.Helper()
	src, tg := sc.fixture(t, product)
	args := sc.args()
	if asJSON {
		args = append(args, "--json")
	}
	var stdout, stderr bytes.Buffer
	code := Command(context.Background(), args, product, sc.id,
		func() (*selfupdate.Updater, error) { return buildUpdaterClosing(src, tg, sc.closeErr) },
		Options{Stdout: &stdout, Stderr: &stderr, Stdin: strings.NewReader(sc.stdin),
			Interactive: sc.interactive, Signals: []os.Signal{}})
	return outcome{stdout: normalize(stdout.String(), product), stderr: normalize(stderr.String(), product), code: code, tg: tg}
}

func TestCommandGolden(t *testing.T) {
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			name := sc.name
			if sc.req != nil {
				name += ".command" // Request's text has no wrapRun prefix (Step 5)
			}
			text, js := sc.command(t, "demo", false), sc.command(t, "demo", true)
			if text.code != js.code {
				t.Fatalf("exit %d in text mode, %d in JSON mode", text.code, js.code)
			}
			if text.stdout != "" {
				t.Fatalf("stdout %q without --json", text.stdout)
			}
			golden(t, name+".text.stderr", text.stderr)
			golden(t, name+".json.stdout", js.stdout)
			golden(t, name+".json.stderr", js.stderr)
			golden(t, name+".code", strconv.Itoa(text.code)+"\n")
		})
	}
}

func TestCommandLazyUpdater(t *testing.T) {
	for _, args := range [][]string{{"--bogus"}, {"now"}, {"--check", "--yes"}, {"--check", "--force"}, {"-h"}} {
		calls := 0
		var stderr bytes.Buffer
		Command(context.Background(), args, "demo", releaseID, func() (*selfupdate.Updater, error) {
			calls++
			return nil, errors.New("must not be built")
		}, Options{Stderr: &stderr})
		if calls != 0 {
			t.Errorf("%v: newUpdater called %d times", args, calls)
		}
	}
}

func TestCommandExitCodes(t *testing.T) {
	codes := map[string]int{}
	for _, sc := range scenarios {
		codes[sc.name] = sc.command(t, "demo", false).code
	}
	for name, want := range map[string]int{"up-to-date": 0, "available": 10, "failed": 1, "applied": 0, "declined": 0, "no-confirm": 1, "warning": 0} {
		if codes[name] != want {
			t.Errorf("%s: exit %d, want %d", name, codes[name], want)
		}
	}
	never := func() (*selfupdate.Updater, error) { return nil, errors.New("not built") }
	for _, tc := range []struct {
		args []string
		want int
		out  string // stderr
	}{
		{[]string{"--bogus"}, 1, HelpText + "update failed: flag provided but not defined: -bogus\n"},
		{[]string{"now"}, 1, HelpText + "update failed: cli: positional arguments are not accepted\n"},
		{[]string{"-h"}, 0, Help("demo")},
		{[]string{"--help"}, 0, Help("demo")},
		{nil, 1, "update failed: not built\n"},
	} {
		var stdout, stderr bytes.Buffer
		if code := Command(context.Background(), tc.args, "demo", releaseID, never, Options{Stdout: &stdout, Stderr: &stderr}); code != tc.want {
			t.Errorf("%v: exit %d, want %d", tc.args, code, tc.want)
		}
		if stderr.String() != tc.out || stdout.Len() != 0 {
			t.Errorf("%v: stderr %q stdout %q; want stderr %q", tc.args, stderr.String(), stdout.String(), tc.out)
		}
	}
	if code := Command(context.Background(), []string{"--check"}, "demo", releaseID, never, Options{}); code != 1 {
		t.Errorf("nil Stderr: exit %d, want 1", code)
	}
	if code := Command(context.Background(), []string{"--check"}, "demo", releaseID, nil, Options{Stderr: &bytes.Buffer{}}); code != 1 {
		t.Errorf("nil newUpdater: exit %d, want 1", code)
	}
}

func TestCommandJSONEarlyError(t *testing.T) {
	for _, args := range [][]string{{"--json", "--check", "--yes"}, {"--json", "--bogus"}, {"--json"}} {
		var stdout, stderr bytes.Buffer
		code := Command(context.Background(), args, "demo", buildinfo.Info{Kind: buildinfo.KindLocal},
			func() (*selfupdate.Updater, error) { return nil, errors.New("not built") },
			Options{Stdout: &stdout, Stderr: &stderr})
		lines := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n")
		if code != 1 || len(lines) != 1 || !strings.HasPrefix(lines[0], `{"kind":"result","exit_code":1,"error":`) ||
			!strings.Contains(lines[0], `"product":"demo","current_version":"dev"`) {
			t.Errorf("%v: exit %d, stdout %q", args, code, stdout.String())
		}
	}
}

// TestCommandJSONRefusedOptions: a nil updater and a negative timeout fail
// after flag parsing, so under --json they write the one result object
// (0010-MADR C4).
func TestCommandJSONRefusedOptions(t *testing.T) {
	src, tg := scenario{latest: "v1.1.0"}.fixture(t, "demo")
	for _, tc := range []struct {
		name       string
		newUpdater func() (*selfupdate.Updater, error)
		timeout    time.Duration
		want       string
	}{
		{"nil updater", func() (*selfupdate.Updater, error) { return nil, nil }, 0, "cli: updater is nil"},
		{"negative timeout", func() (*selfupdate.Updater, error) { return buildUpdater(src, tg) }, -time.Second, "cli: Options.Timeout is negative"},
	} {
		var stdout, stderr bytes.Buffer
		code := Command(context.Background(), []string{"--check", "--json"}, "demo", releaseID, tc.newUpdater,
			Options{Stdout: &stdout, Stderr: &stderr, Timeout: tc.timeout, Signals: []os.Signal{}})
		lines := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n")
		if code != 1 || len(lines) != 1 ||
			!strings.HasPrefix(lines[0], `{"kind":"result","exit_code":1,"error":"`+tc.want+`"`) {
			t.Errorf("%s: exit %d, stdout %q", tc.name, code, stdout.String())
		}
		if stderr.String() != "update failed: "+tc.want+"\n" {
			t.Errorf("%s: stderr %q", tc.name, stderr.String())
		}
	}
	tg.unchanged(t)
}

func TestCommandOverridesJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	Command(context.Background(), []string{"--check", "--yes"}, "demo", releaseID, nil,
		Options{Stdout: &stdout, Stderr: &stderr, JSON: true})
	if stdout.Len() != 0 {
		t.Fatalf("the caller's JSON was kept: stdout %q", stdout.String())
	}
}
