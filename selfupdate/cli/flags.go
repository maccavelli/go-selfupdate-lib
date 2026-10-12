package cli

import (
	"errors"
	"flag"
	"io"

	"github.com/maccavelli/go-selfupdate-lib/buildinfo"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
)

// FlagSet is the part of a flag set Bind needs. *flag.FlagSet and pflag's
// *FlagSet both satisfy it (0004-MADR §5).
type FlagSet interface {
	BoolVar(p *bool, name string, value bool, usage string)
	StringVar(p *string, name string, value string, usage string)
}

// boolVarP is pflag's shorthand method. Bind uses it for -y when the flag
// set has it.
type boolVarP interface {
	BoolVarP(p *bool, name, shorthand string, value bool, usage string)
}

// Flags are the canonical update flags (0004-MADR §5, amendment F3).
type Flags struct {
	// Check reports whether an update is available and installs nothing.
	Check bool
	// Yes installs without asking.
	Yes bool
	// Force replaces a local build, or reinstalls the same version.
	Force bool
	// DryRun downloads and verifies the release, and installs nothing.
	DryRun bool
	// JSON writes JSON Lines to stdout.
	JSON bool
	// Version selects an exact release tag.
	Version string
	// Channel selects a prerelease channel, such as "rc".
	Channel string
}

// HelpText is the flag list and the exit-status paragraph. Help adds the
// usage line (0004-MADR §5, amendment F5).
const HelpText = `Flags:
  --check            report whether an update is available; install nothing
  --yes, -y          install without asking
  --force            replace a local build, or reinstall the same version
  --dry-run          download and verify the release; install nothing
  --json             write JSON Lines to stdout; everything else to stderr
  --version vX.Y.Z   install exactly this release
  --channel NAME     follow a prerelease channel, such as rc

Exit status: 0 up to date, declined or installed; 10 an update is
available (with --check); 1 any error.
`

// Help returns the usage line for prog, a blank line, and HelpText.
func Help(prog string) string {
	return "Usage: " + prog + " update [flags]\n\n" + HelpText
}

// errPositional is returned by Parse for any positional argument.
var errPositional = errors.New("cli: positional arguments are not accepted")

// The contradictions Request refuses, in validateRequest's words.
var (
	errCheckYes    = errors.New("selfupdate: --check and --yes are contradictory")
	errCheckForce  = errors.New("selfupdate: --check and --force are contradictory")
	errCheckDryRun = errors.New("selfupdate: --check and --dry-run are contradictory")
)

// Bind registers the flags on fs: check, yes (and -y), force, dry-run, json,
// version and channel.
func (f *Flags) Bind(fs FlagSet) {
	fs.BoolVar(&f.Check, "check", false, "report whether an update is available; install nothing")
	if p, ok := fs.(boolVarP); ok {
		p.BoolVarP(&f.Yes, "yes", "y", false, "install without asking")
	} else {
		fs.BoolVar(&f.Yes, "yes", false, "install without asking")
		fs.BoolVar(&f.Yes, "y", false, "install without asking")
	}
	fs.BoolVar(&f.Force, "force", false, "replace a local build, or reinstall the same version")
	fs.BoolVar(&f.DryRun, "dry-run", false, "download and verify the release; install nothing")
	fs.BoolVar(&f.JSON, "json", false, "write JSON Lines to stdout; everything else to stderr")
	fs.StringVar(&f.Version, "version", "", "install exactly this release")
	fs.StringVar(&f.Channel, "channel", "", "follow a prerelease channel, such as rc")
}

// Parse parses args with the standard flag package (0004-MADR amendment F4). It
// returns flag.ErrHelp for -h and --help, writing nothing, so the caller
// prints Help with its program name. Any other usage error, including a
// positional argument, writes HelpText to stderr and returns the error.
// The flag package's own message is not written: the caller reports the
// returned error once.
func (f *Flags) Parse(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	f.Bind(fs)
	err := fs.Parse(args)
	if err == nil && fs.NArg() > 0 {
		err = errPositional
	}
	if err != nil && !errors.Is(err, flag.ErrHelp) {
		if _, werr := io.WriteString(stderr, HelpText); werr != nil {
			err = errors.Join(err, werr)
		}
	}
	return err
}

// Request builds the request for product from the flags and the running
// identity. It refuses --check with --yes, --force or --dry-run, in
// selfupdate's own words, before any updater exists.
func (f Flags) Request(product string, id buildinfo.Info) (selfupdate.Request, error) {
	switch {
	case f.Check && f.Yes:
		return selfupdate.Request{}, errCheckYes
	case f.Check && f.Force:
		return selfupdate.Request{}, errCheckForce
	case f.Check && f.DryRun:
		return selfupdate.Request{}, errCheckDryRun
	}
	build := selfupdate.LocalBuild
	if id.Kind == buildinfo.KindRelease {
		build = selfupdate.ReleaseBuild
	}
	return selfupdate.Request{
		Product:        product,
		CurrentVersion: id.Current(),
		CurrentBuild:   build,
		TargetVersion:  f.Version,
		Channel:        f.Channel,
		CheckOnly:      f.Check,
		Yes:            f.Yes,
		Force:          f.Force,
		DryRun:         f.DryRun,
	}, nil
}
