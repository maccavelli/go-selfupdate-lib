package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"reflect"
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
	// HandOff runs an update started from inside a service detached from
	// it (0011-MADR §9). The zero value never hands off.
	HandOff HandOff
}

// HandOff hooks a run into a service's handoff: an update started inside
// the service, such as by an agent the service spawned, runs as a detached
// copy of the same command, outside the service's kill scope
// (0011-MADR §9). service.HandOffFunc builds Detach for a backend, and
// service.ReportFunc builds Report. A program that sets DetachPlan, from
// service.HandOffPlanFunc, and Planned, service.LoadHandOffPlan, has the
// detached run install the release this run resolved, not the latest one
// when it starts
// (docs/decisions/0038-MADR-pinned-plans-lifecycle-conformance-and-bubble-tests.md
// D1-A).
type HandOff struct {
	// Detach runs before an apply that would install, never a check, a
	// dry run, or a run with nothing to install. When it hands off, the
	// run ends at once: "update handed off: <detail>" on
	// Stderr, or under JSON a result object with "handed_off", and exit
	// 0. Its error fails the run.
	Detach func(ctx context.Context, req selfupdate.Request) (handedOff bool, detail string, err error)
	// DetachPlan replaces Detach when it is set. Run makes a plan with
	// Updater.Plan first, and hands off only one that installs, with the
	// plan, as Detach would; when it does not hand off, Run applies that
	// plan here with Updater.Apply. When Plan fails, Run goes on without a
	// plan, and the run reports the error as it would have.
	DetachPlan func(ctx context.Context, req selfupdate.Request, p selfupdate.Plan) (handedOff bool, detail string, err error)
	// Planned, in a detached run, returns the plan the run that handed off
	// made. When ok, Run applies it with Updater.Apply and resolves
	// nothing itself; Apply refuses a plan the release no longer matches.
	// Its error fails the run, through Report. With ok false, Run runs as
	// it would without it.
	Planned func() (p selfupdate.Plan, ok bool, err error)
	// Report runs after every update that ran here, with its outcome, and
	// after an invocation Command ended before Run, with its error. It runs
	// before the result object, and its error fails the run.
	Report func(res selfupdate.Result, err error) error
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
	case isNilWriter(o.Stderr):
		return errors.New("cli: Options.Stderr is nil")
	case o.JSON && isNilWriter(o.Stdout):
		return errors.New("cli: Options.Stdout is nil with JSON set")
	case o.Timeout < 0:
		return errors.New("cli: Options.Timeout is negative")
	}
	return nil
}

// isNilWriter reports a nil writer, including a nil pointer behind the
// interface, which would panic on its first write (0015-MADR C7).
func isNilWriter(w io.Writer) bool {
	if w == nil {
		return true
	}
	v := reflect.ValueOf(w)
	switch v.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan, reflect.Interface:
		return v.IsNil()
	}
	return false
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

	base := selfupdate.Result{Product: req.Product, CurrentVersion: req.CurrentVersion}
	pin, loadErr := o.planned()
	if loadErr == nil && pin == nil {
		p, handed, detail, err := o.detach(ctx, u, req)
		if err != nil {
			return base, errors.Join(err, o.finish(base, err))
		}
		if handed {
			return base, o.handedOff(base, detail)
		}
		pin = p
	}

	rep := selfupdate.NewTextReporter(o.Stderr)
	if o.JSON {
		rep = selfupdate.NewJSONReporter(o.Stdout)
	}
	conf := o.Confirmer
	if conf == nil {
		conf = selfupdate.NewPromptConfirmer(o.Stdin, o.Stderr, o.Interactive)
	}
	opts := []selfupdate.RunOption{selfupdate.WithReporter(rep), selfupdate.WithConfirmer(conf)}
	var res selfupdate.Result
	var err error
	switch {
	case loadErr != nil:
		res, err = base, loadErr
	case pin != nil:
		res, err = u.Apply(ctx, req, *pin, opts...)
	default:
		res, err = u.RunWith(ctx, req, opts...)
	}
	// The warnings and HandOff.Report come before the last line, so the
	// result object carries every error that decides the exit code
	// (0015-MADR C1).
	late := o.warn(res)
	if o.HandOff.Report != nil {
		late = errors.Join(late, o.HandOff.Report(res, joinLate(err, late)))
	}
	err = joinLate(err, late)
	return res, joinLate(err, o.finish(res, err))
}

// planned is the plan HandOff.Planned returns, nil when there is none, and
// its error.
func (o Options) planned() (*selfupdate.Plan, error) {
	if o.HandOff.Planned == nil {
		return nil, nil
	}
	p, ok, err := o.HandOff.Planned()
	if err != nil {
		return nil, fmt.Errorf("cli: load the handed-off plan: %w", err)
	}
	if !ok {
		return nil, nil
	}
	return &p, nil
}

// detach hands req off, through DetachPlan or else Detach, before an
// apply that would install; never for a check or a dry run. pin is the
// plan DetachPlan's run made, which Run applies when nothing was handed
// off.
func (o Options) detach(ctx context.Context, u *selfupdate.Updater, req selfupdate.Request) (pin *selfupdate.Plan, handed bool, detail string, err error) {
	if req.CheckOnly || req.DryRun {
		return nil, false, "", nil
	}
	if dp := o.HandOff.DetachPlan; dp != nil {
		if p, perr := u.Plan(ctx, req); perr == nil {
			if p.Operation == selfupdate.OperationNone {
				return &p, false, "", nil
			}
			handed, detail, err = dp(ctx, req, p)
			return &p, handed, detail, err
		}
		// Plan failed: the run reports it, as a run without a plan would.
		return nil, false, "", nil
	}
	if d := o.HandOff.Detach; d != nil && wouldInstall(ctx, u, req) {
		handed, detail, err = d(ctx, req)
		return nil, handed, detail, err
	}
	return nil, false, "", nil
}

// wouldInstall reports whether req would install a release: there is
// nothing to hand off for a run that is up to date, or that needs --force
// it lacks. A check that fails gives false; the run that follows reports
// its error (0015-MADR C3).
func wouldInstall(ctx context.Context, u *selfupdate.Updater, req selfupdate.Request) bool {
	av, err := u.Checker().Check(ctx, selfupdate.CheckRequest{
		Product: req.Product, CurrentVersion: req.CurrentVersion, CurrentBuild: req.CurrentBuild,
		TargetVersion: req.TargetVersion, Platform: req.Platform, Channel: req.Channel,
	})
	if err != nil || av.ForceRequired && !req.Force {
		return false
	}
	return av.Available || req.Force
}

// joinLate adds an error that arrived after the run to the run's. An update
// nobody was told about is not "update available": the late error alone
// decides, so the exit code is 1 and Exit reports it (0010-MADR C3).
func joinLate(err, late error) error {
	switch {
	case late == nil:
		return err
	case errors.Is(err, selfupdate.ErrUpdateAvailable):
		return late
	}
	return errors.Join(err, late)
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

// handedOff ends a run that was handed off: the notice on Stderr, or the
// result object under JSON.
func (o Options) handedOff(res selfupdate.Result, detail string) error {
	if o.JSON {
		return writeLine(o.Stdout, resultLine{Kind: "result", Result: res.Document(), HandedOff: oneLine(detail)})
	}
	_, err := io.WriteString(o.Stderr, "update handed off: "+oneLine(detail)+"\n")
	return err
}

// resultLine is the final JSON Lines object (0004-MADR amendment F8). HandedOff is
// the handoff's detail when the update was handed off (0011-MADR §9).
type resultLine struct {
	Kind      string                    `json:"kind"`
	ExitCode  int                       `json:"exit_code"`
	Error     string                    `json:"error,omitempty"`
	HandedOff string                    `json:"handed_off,omitempty"`
	Result    selfupdate.ResultDocument `json:"result"`
}

// writeResult writes the result object as one line, in one Write.
func writeResult(w io.Writer, res selfupdate.Result, err error) error {
	line := resultLine{Kind: "result", ExitCode: selfupdate.ExitCode(res, err), Result: res.Document()}
	if err != nil {
		line.Error = oneLine(err.Error())
	}
	return writeLine(w, line)
}

// writeLine writes line as one JSON line, in one Write.
func writeLine(w io.Writer, line resultLine) error {
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
// (0004-MADR amendment F7). It writes nothing else, and never to stdout.
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
