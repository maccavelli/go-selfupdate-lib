package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"unicode"

	"golang.org/x/term"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
)

// DefaultTimeout bounds a run when Options.Timeout is zero (0004-MADR §5).
const DefaultTimeout = 15 * time.Minute

// Options are a run's streams and limits (0004-MADR §5, amendment F6).
type Options struct {
	// Stdout receives protocol output, and is written only when JSON is
	// set. It may be nil otherwise.
	Stdout io.Writer
	// Stderr receives everything else: events, prompts, the summary and
	// errors. It is required.
	Stderr io.Writer
	// Stdin is read for a confirmation.
	Stdin io.Reader
	// Interactive says Stdin is a terminal a person can answer on.
	Interactive bool
	// JSON selects JSON Lines on Stdout.
	JSON bool
	// Confirmer answers confirmations. Nil means
	// selfupdate.NewPromptConfirmer(Stdin, Stderr, Interactive).
	Confirmer selfupdate.Confirmer
	// Timeout bounds the run. Zero means DefaultTimeout; negative is
	// refused.
	Timeout time.Duration
	// Signals cancel the run. Nil means os.Interrupt and syscall.SIGTERM;
	// an empty, non-nil slice means none.
	Signals []os.Signal
}

// notifyContext is signal.NotifyContext, replaced in tests.
var notifyContext = signal.NotifyContext

// defaultSignals cancel a run when Options.Signals is nil.
var defaultSignals = []os.Signal{os.Interrupt, syscall.SIGTERM}

// StdioOptions returns the process's own streams, with Interactive set when
// standard input is a terminal.
func StdioOptions() Options {
	return Options{
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		Stdin:       os.Stdin,
		Interactive: term.IsTerminal(int(os.Stdin.Fd())), //nolint:gosec // a file descriptor fits in an int
	}
}

// check refuses options Run cannot honour.
func (o Options) check(u *selfupdate.Updater) error {
	switch {
	case u == nil:
		return errors.New("cli: updater is nil")
	case o.Stderr == nil:
		return errors.New("cli: Options.Stderr is nil")
	case o.JSON && o.Stdout == nil:
		return errors.New("cli: Options.Stdout is nil with JSON set")
	case o.Timeout < 0:
		return errors.New("cli: Options.Timeout is negative")
	}
	return nil
}

// Run runs req on u with the canonical streams: events and prompts on
// Stderr, or, under JSON, events on Stdout followed by exactly one result
// object. Without JSON it ends with Summary's line on Stderr. It cancels on
// the signals and the timeout, and never calls os.Exit (0004-MADR §5).
func Run(ctx context.Context, u *selfupdate.Updater, req selfupdate.Request, o Options) (selfupdate.Result, error) {
	if err := o.check(u); err != nil {
		return selfupdate.Result{}, err
	}
	sigs := o.Signals
	if sigs == nil {
		sigs = defaultSignals
	}
	if len(sigs) > 0 {
		var stop context.CancelFunc
		ctx, stop = notifyContext(ctx, sigs...)
		defer stop()
	}
	timeout := o.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	rep := selfupdate.NewTextReporter(o.Stderr)
	if o.JSON {
		rep = selfupdate.NewJSONReporter(o.Stdout)
	}
	conf := o.Confirmer
	if conf == nil {
		conf = selfupdate.NewPromptConfirmer(o.Stdin, o.Stderr, o.Interactive)
	}
	res, err := u.RunWith(ctx, req, selfupdate.WithReporter(rep), selfupdate.WithConfirmer(conf))
	if werr := errors.Join(o.warn(res), o.finish(res, err)); werr != nil {
		// An update nobody was told about is not "update available": the
		// write error alone decides, so the exit code is 1 and Exit
		// reports it (0010-MADR C3).
		if errors.Is(err, selfupdate.ErrUpdateAvailable) {
			return res, werr
		}
		return res, errors.Join(err, werr)
	}
	return res, err
}

// warn writes "warning: <text>" to Stderr for each of the run's warnings,
// in either mode: errors after the run did its work, which did not fail it
// (0010-MADR Q3).
func (o Options) warn(res selfupdate.Result) error {
	for _, w := range res.Warnings.List() {
		if _, err := io.WriteString(o.Stderr, "warning: "+oneLine(w)+"\n"); err != nil {
			return err
		}
	}
	return nil
}

// finish writes the run's last line: the result object under JSON, else
// the summary.
func (o Options) finish(res selfupdate.Result, err error) error {
	if o.JSON {
		return writeResult(o.Stdout, res, err)
	}
	if s := Summary(res, err); s != "" {
		_, werr := io.WriteString(o.Stderr, s+"\n")
		return werr
	}
	return nil
}

// resultLine is the final JSON Lines object (amendment F8).
type resultLine struct {
	Kind     string                    `json:"kind"`
	ExitCode int                       `json:"exit_code"`
	Error    string                    `json:"error,omitempty"`
	Result   selfupdate.ResultDocument `json:"result"`
}

// writeResult writes the result object as one line, in one Write.
func writeResult(w io.Writer, res selfupdate.Result, err error) error {
	line := resultLine{Kind: "result", ExitCode: selfupdate.ExitCode(res, err), Result: res.Document()}
	if err != nil {
		line.Error = oneLine(err.Error())
	}
	b, merr := json.Marshal(line)
	if merr != nil {
		return merr
	}
	_, werr := w.Write(append(b, '\n'))
	return werr
}

// Summary is the one-line outcome of a run, for people. It is empty for an
// error other than selfupdate.ErrUpdateAvailable: Exit reports those.
func Summary(res selfupdate.Result, err error) string {
	p, cur, tgt := oneLine(res.Product), oneLine(res.CurrentVersion), oneLine(res.TargetVersion)
	switch {
	case errors.Is(err, selfupdate.ErrUpdateAvailable):
		return p + ": update available: " + cur + " -> " + tgt
	case err != nil:
		return ""
	case res.Operation == selfupdate.OperationNone:
		return p + ": up to date (" + cur + ")"
	case res.Declined:
		return p + ": update declined"
	case res.DryRun:
		return p + ": dry run: " + tgt + " verified; nothing installed"
	case res.Applied:
		return p + ": updated " + cur + " -> " + tgt
	default:
		return ""
	}
}

// Exit writes "update failed: <message>" to stderr for any error but
// selfupdate.ErrUpdateAvailable, and returns selfupdate.ExitCode
// (amendment F7). It writes nothing else, and never to stdout.
func Exit(stderr io.Writer, res selfupdate.Result, err error) int {
	code := selfupdate.ExitCode(res, err)
	if err != nil && !errors.Is(err, selfupdate.ErrUpdateAvailable) {
		if _, werr := io.WriteString(stderr, "update failed: "+oneLine(err.Error())+"\n"); werr != nil {
			code = max(code, 1)
		}
	}
	return code
}

// oneLine makes s one line: control characters become spaces, and runs of
// white space become one space.
func oneLine(s string) string {
	return strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)), " ")
}
