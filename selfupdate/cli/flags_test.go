package cli

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"slices"
	"strings"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/buildinfo"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/selfupdatetest"
)

func TestBindStdlib(t *testing.T) {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	var f Flags
	f.Bind(fs)
	var names []string
	fs.VisitAll(func(fl *flag.Flag) { names = append(names, fl.Name) })
	want := []string{"channel", "check", "dry-run", "force", "json", "version", "y", "yes"}
	if !slices.Equal(names, want) {
		t.Fatalf("flags %v, want %v", names, want)
	}
}

// pflagDouble records what Bind registers on a pflag-shaped flag set.
type pflagDouble struct {
	names     []string
	shorthand map[string]string
}

func (d *pflagDouble) BoolVar(_ *bool, name string, _ bool, _ string) {
	d.names = append(d.names, name)
}

func (d *pflagDouble) StringVar(_ *string, name string, _ string, _ string) {
	d.names = append(d.names, name)
}

func (d *pflagDouble) BoolVarP(_ *bool, name, shorthand string, _ bool, _ string) {
	d.names = append(d.names, name)
	if d.shorthand == nil {
		d.shorthand = map[string]string{}
	}
	d.shorthand[name] = shorthand
}

func TestBindShorthand(t *testing.T) {
	var d pflagDouble
	var f Flags
	f.Bind(&d)
	want := []string{"check", "yes", "force", "dry-run", "json", "version", "channel"}
	if !slices.Equal(d.names, want) {
		t.Fatalf("names %v, want %v", d.names, want)
	}
	if d.shorthand["yes"] != "y" || len(d.shorthand) != 1 {
		t.Fatalf("shorthands %v, want only yes=y", d.shorthand)
	}
}

func TestParse(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    []string
		want    Flags
		err     error
		errText string
		help    bool // HelpText written to stderr
	}{
		{name: "none", args: nil},
		{name: "all", args: []string{"--check", "--force", "--dry-run", "--json", "--version", "v1.2.3", "--channel", "rc"},
			want: Flags{Check: true, Force: true, DryRun: true, JSON: true, Version: "v1.2.3", Channel: "rc"}},
		{name: "yes", args: []string{"--yes"}, want: Flags{Yes: true}},
		{name: "y", args: []string{"-y"}, want: Flags{Yes: true}},
		{name: "-h", args: []string{"-h"}, err: flag.ErrHelp},
		{name: "--help", args: []string{"--help"}, err: flag.ErrHelp},
		{name: "unknown", args: []string{"--bogus"}, errText: "flag provided but not defined: -bogus", help: true},
		{name: "positional", args: []string{"now"}, err: errPositional, help: true},
		{name: "json then unknown", args: []string{"--json", "--bogus"}, want: Flags{JSON: true},
			errText: "flag provided but not defined: -bogus", help: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var f Flags
			var stderr bytes.Buffer
			err := f.Parse(tc.args, &stderr)
			switch {
			case tc.err != nil:
				if !errors.Is(err, tc.err) {
					t.Fatalf("err %v, want %v", err, tc.err)
				}
			case tc.errText != "":
				if err == nil || err.Error() != tc.errText {
					t.Fatalf("err %v, want %q", err, tc.errText)
				}
			case err != nil:
				t.Fatalf("err %v", err)
			}
			if f != tc.want {
				t.Fatalf("flags %+v, want %+v", f, tc.want)
			}
			wantErr := ""
			if tc.help {
				wantErr = HelpText
			}
			if stderr.String() != wantErr {
				t.Fatalf("stderr %q, want %q", stderr.String(), wantErr)
			}
		})
	}
}

func TestRequestContradictions(t *testing.T) {
	tg := newTarget(t)
	u := newUpdater(t, selfupdatetest.NewFakeSource("v1.0.0", release("demo", "v1.0.0")), tg)
	id := buildinfo.Info{Version: "v1.0.0", Kind: buildinfo.KindRelease}
	for _, f := range []Flags{{Check: true, Yes: true}, {Check: true, Force: true}, {Check: true, DryRun: true}} {
		_, reqErr := f.Request("demo", id)
		if reqErr == nil {
			t.Fatalf("%+v: Request accepted a contradiction", f)
		}
		// The same request, built by hand, through the updater's own check.
		req := selfupdate.Request{Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: selfupdate.ReleaseBuild,
			CheckOnly: f.Check, Yes: f.Yes, Force: f.Force, DryRun: f.DryRun}
		_, runErr := u.Run(context.Background(), req)
		// The run names the package once (0043-MADR D4-A).
		if runErr == nil || runErr.Error() != "selfupdate: demo: "+strings.TrimPrefix(reqErr.Error(), "selfupdate: ") {
			t.Fatalf("%+v: Request says %q, Run says %v", f, reqErr, runErr)
		}
	}
	tg.unchanged(t)
}

func TestRequestMapping(t *testing.T) {
	f := Flags{Yes: true, Force: true, DryRun: true, Version: "v1.2.3", Channel: "rc"}
	for _, tc := range []struct {
		id      buildinfo.Info
		current string
		build   selfupdate.BuildKind
	}{
		{buildinfo.Info{Version: "v1.0.0", Kind: buildinfo.KindRelease}, "v1.0.0", selfupdate.ReleaseBuild},
		{buildinfo.Info{Kind: buildinfo.KindLocal}, "dev", selfupdate.LocalBuild},
		{buildinfo.Info{}, "dev", selfupdate.LocalBuild}, // KindUnknown is never a release
		{buildinfo.Info{Version: "v1.0", Kind: buildinfo.KindLocal}, "v1.0", selfupdate.LocalBuild},
	} {
		got, err := f.Request("demo", tc.id)
		if err != nil {
			t.Fatal(err)
		}
		want := selfupdate.Request{Product: "demo", CurrentVersion: tc.current, CurrentBuild: tc.build,
			TargetVersion: "v1.2.3", Channel: "rc", Yes: true, Force: true, DryRun: true}
		if got != want {
			t.Fatalf("Request = %+v, want %+v", got, want)
		}
	}
	got, err := Flags{Check: true}.Request("demo", buildinfo.Info{Kind: buildinfo.KindLocal})
	if err != nil || !got.CheckOnly {
		t.Fatalf("Check: %+v, %v", got, err)
	}
}

func TestHelpText(t *testing.T) {
	golden(t, "help.txt", Help("demo"))
	if !strings.HasPrefix(Help("x"), "Usage: x update [flags]\n\n") {
		t.Fatalf("Help(x) = %q", Help("x"))
	}
}
