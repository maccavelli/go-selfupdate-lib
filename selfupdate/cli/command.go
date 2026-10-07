package cli

import (
	"context"
	"errors"
	"flag"
	"io"

	"github.com/maccavelli/go-selfupdate-lib/buildinfo"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
)

// Command runs the whole update command and returns its exit status
// (0004-MADR §5, amendment F4). It parses args, builds the request for
// product and the running identity id, builds the updater with newUpdater
// only once both succeed, and runs it with Run and Exit.
//
//   - -h or --help writes Help(product) to o.Stderr and returns 0.
//   - A usage error, a refused request or a failed newUpdater is reported
//     like a failed run: o.HandOff.Report, when set; under --json (as parsed
//     so far) one result object on o.Stdout; then Exit's line on o.Stderr. It
//     returns 1.
//
// o.JSON is set from the parsed flags; the caller's value is ignored. A nil
// o.Stderr leaves nothing to report on, so Command returns 1 at once.
func Command(ctx context.Context, args []string, product string, id buildinfo.Info,
	newUpdater func() (*selfupdate.Updater, error), o Options) int {
	if isNilWriter(o.Stderr) {
		return 1
	}
	var f Flags
	err := f.Parse(args, o.Stderr)
	o.JSON = f.JSON
	if errors.Is(err, flag.ErrHelp) {
		if _, werr := io.WriteString(o.Stderr, Help(product)); werr != nil {
			return 1
		}
		return 0
	}
	if err != nil {
		return o.report(product, id, err)
	}
	req, err := f.Request(product, id)
	if err != nil {
		return o.report(product, id, err)
	}
	if newUpdater == nil {
		return o.report(product, id, errors.New("cli: newUpdater is nil"))
	}
	u, err := newUpdater()
	if err != nil {
		return o.report(product, id, err)
	}
	// Run refuses these too, but before it could write a result object
	// (0010-MADR C4).
	if err := o.check(u); err != nil {
		return o.report(product, id, err)
	}
	res, err := Run(ctx, u, req, o)
	return Exit(o.Stderr, res, err)
}

// report ends an invocation that failed before Run, as a failed run ends:
// HandOff.Report first, which in a detached run writes the result file the
// agent waits for (0015-MADR C2), then the result object or the summary.
func (o Options) report(product string, id buildinfo.Info, err error) int {
	res := selfupdate.Result{Product: product, CurrentVersion: id.Current()}
	if o.HandOff.Report != nil {
		if rerr := o.HandOff.Report(res, err); rerr != nil {
			err = errors.Join(err, rerr)
		}
	}
	if o.JSON && !isNilWriter(o.Stdout) {
		if werr := writeResult(o.Stdout, res, err); werr != nil {
			err = errors.Join(err, werr)
		}
	}
	return Exit(o.Stderr, res, err)
}
