// Command selfupdate-release is the logic of the build-and-stage release
// workflow (docs/decisions/0013-MADR-build-and-stage-release-workflow.md
// §1, §4–§8), in Go, on the library's own code. It is never released:
// build-selfupdate-release.yml and publish-selfupdate-release.yml build it
// from the called workflow's commit.
//
// Usage:
//
//	selfupdate-release plan     -spec FILE -module-dir DIR -ref-type TYPE -ref-name NAME -sha SHA -run-attempt N [-artifact-name NAME] [-github-output FILE] [-summary FILE]
//	selfupdate-release build    -spec FILE -module-dir DIR -stamp-version V -stamp-kind K -out DIR
//	selfupdate-release stage    -spec FILE -module-dir DIR -src DIR -bin DIR -out DIR -sha SHA -stamp-version V [-tag TAG] [-extras-dir DIR] [-repository OWNER/NAME] [-summary FILE]
//	selfupdate-release check    -dir DIR -products-json JSON -platforms-json JSON
//	selfupdate-release identity -staging DIR -asset NAME -product P -os OS -arch ARCH -args-json JSON -want-version V -want-kind K -sha SHA
//	selfupdate-release installer -spec FILE -repository OWNER/NAME -tag TAG -out DIR
//
// It exits 0 on success, 1 when a check fails, and 2 on a usage error.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strings"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

// usageError is a mistake in the command line, reported with exit 2.
type usageError struct{ msg string }

func (e usageError) Error() string { return e.msg }

func usagef(format string, args ...any) error {
	return usageError{fmt.Sprintf(format, args...)}
}

type subcommand func(ctx context.Context, args []string, stdout io.Writer) error

var subcommands = map[string]subcommand{
	"plan":      runPlan,
	"build":     runBuild,
	"stage":     runStage,
	"check":     runCheck,
	"identity":  runIdentity,
	"installer": runInstaller,
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	code, msg := dispatch(ctx, args, stdout)
	if msg != "" {
		if _, err := io.WriteString(stderr, msg+"\n"); err != nil && code == 0 {
			code = 1
		}
	}
	return code
}

// dispatch runs the subcommand args name, and returns the exit code and
// the line for stderr.
func dispatch(ctx context.Context, args []string, stdout io.Writer) (int, string) {
	if len(args) == 0 {
		return 2, "usage: selfupdate-release " + strings.Join(subcommandNames(), "|") + " [flags]"
	}
	sub, ok := subcommands[args[0]]
	if !ok {
		return 2, fmt.Sprintf("selfupdate-release: unknown subcommand %q; want %s", args[0], strings.Join(subcommandNames(), ", "))
	}
	err := sub(ctx, args[1:], stdout)
	if err == nil {
		return 0, ""
	}
	msg := fmt.Sprintf("selfupdate-release %s: %v", args[0], err)
	var ue usageError
	if errors.As(err, &ue) {
		return 2, msg
	}
	return 1, msg
}

// printf writes to w, and returns the write's error.
func printf(w io.Writer, format string, args ...any) error {
	_, err := fmt.Fprintf(w, format, args...)
	return err
}

// The stamp kinds.
const (
	kindRelease = "release"
	kindLocal   = "local"
)

func subcommandNames() []string {
	names := make([]string, 0, len(subcommands))
	for name := range subcommands {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// flags is a subcommand's flag set: string flags, some required.
type flags struct {
	fs       *flag.FlagSet
	values   map[string]*string
	required []string
}

func newFlags(name string) *flags {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return &flags{fs: fs, values: map[string]*string{}}
}

// str declares a string flag; required ones must be given and non-empty.
func (f *flags) str(name string, required bool) *string {
	v := f.fs.String(name, "", "")
	f.values[name] = v
	if required {
		f.required = append(f.required, name)
	}
	return v
}

func (f *flags) parse(args []string) error {
	if err := f.fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if f.fs.NArg() != 0 {
		return usagef("unexpected argument %q", f.fs.Arg(0))
	}
	for _, name := range f.required {
		if *f.values[name] == "" {
			return usagef("-%s is required", name)
		}
	}
	return nil
}
