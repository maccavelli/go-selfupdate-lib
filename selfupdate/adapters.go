package selfupdate

import (
	"context"
	"errors"
	"fmt"
)

// Function adapters and small defaults for the seams
// (docs/decisions/0004-MADR-evolve-selfupdate-api-and-tui-support.md G6).

// ReporterFunc adapts a function to Reporter.
type ReporterFunc func(context.Context, Event) error

// Report implements Reporter.
func (f ReporterFunc) Report(ctx context.Context, ev Event) error {
	return f(ctx, ev)
}

// ConfirmerFunc adapts a function to Confirmer.
type ConfirmerFunc func(context.Context, Prompt) (bool, error)

// Confirm implements Confirmer.
func (f ConfirmerFunc) Confirm(ctx context.Context, p Prompt) (bool, error) {
	return f(ctx, p)
}

// VerifierFunc adapts a function to Verifier.
type VerifierFunc func(context.Context, Verification) error

// Verify implements Verifier.
func (f VerifierFunc) Verify(ctx context.Context, v Verification) error {
	return f(ctx, v)
}

// TransformerFunc adapts a function to Transformer.
type TransformerFunc func(context.Context, TransformRequest) error

// Transform implements Transformer.
func (f TransformerFunc) Transform(ctx context.Context, r TransformRequest) error {
	return f(ctx, r)
}

// UnpackerFunc adapts a function to Unpacker.
type UnpackerFunc func(context.Context, UnpackRequest) error

// Unpack implements Unpacker.
func (f UnpackerFunc) Unpack(ctx context.Context, r UnpackRequest) error {
	return f(ctx, r)
}

type discardReporter struct{}

func (discardReporter) Report(context.Context, Event) error { return nil }

// DiscardReporter returns a Reporter that drops every event.
func DiscardReporter() Reporter {
	return discardReporter{}
}

type multiReporter []Reporter

// MultiReporter returns a Reporter that reports each event to every given
// reporter, in order. Nil and typed-nil reporters are dropped. Every
// reporter is called even after one fails, and their errors are joined.
// With no reporters left it behaves like DiscardReporter.
func MultiReporter(reporters ...Reporter) Reporter {
	kept := make(multiReporter, 0, len(reporters))
	for _, r := range reporters {
		if !isNil(r) {
			kept = append(kept, r)
		}
	}
	return kept
}

func (m multiReporter) Report(ctx context.Context, ev Event) error {
	var errs []error
	for _, r := range m {
		if err := r.Report(ctx, ev); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

type nonInteractiveConfirmer struct{}

func (nonInteractiveConfirmer) Confirm(context.Context, Prompt) (bool, error) {
	return false, fmt.Errorf("selfupdate: pass --yes to apply without prompting: %w", ErrConfirmationRequired)
}

// NonInteractiveConfirmer returns a Confirmer that never prompts: every
// Confirm declines with ErrConfirmationRequired, so an apply needs
// Request.Yes.
func NonInteractiveConfirmer() Confirmer {
	return nonInteractiveConfirmer{}
}
