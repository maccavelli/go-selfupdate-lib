package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
)

// ReceiptSchema is the receipt version ExecReconciler reads and documents
// (0011-MADR §6). Fields are only ever added, never renamed, so a later
// version still decodes.
const ReceiptSchema = 1

// The Receipt verdicts.
const (
	// VerdictNone: the product has no service definition.
	VerdictNone = "none"
	// VerdictUnchanged: the definition already names the binary.
	VerdictUnchanged = "unchanged"
	// VerdictRefreshed: the definition was rewritten.
	VerdictRefreshed = "refreshed"
	// VerdictKept: the definition differs but was edited by hand, so it was
	// left alone.
	VerdictKept = "kept"
)

// Receipt is what a product's reconcile command prints on its standard
// output: one JSON object, version 1 of magic-cli-remote's refresh result
// (0011-MADR §6, owner answer Q2). A receipt with no schema_version is
// version 1.
type Receipt struct {
	SchemaVersion int      `json:"schema_version"`
	Verdict       string   `json:"verdict"`
	Path          string   `json:"path,omitempty"`
	Backup        string   `json:"backup,omitempty"`
	Changed       bool     `json:"changed"`
	Reloaded      bool     `json:"reloaded"`
	Reason        string   `json:"reason,omitempty"`
	Warnings      []string `json:"warnings,omitempty"`
}

// ExecState is the ReconcileResult.State of an ExecReconciler: the receipt
// and the binary that wrote it, which Restore runs.
type ExecState struct {
	Receipt    Receipt `json:"receipt"`
	Executable string  `json:"executable"`
}

// ExecOptions configure an ExecReconciler.
type ExecOptions struct {
	// Reconcile are the arguments that make the new binary reconcile its
	// definition and print a Receipt, such as
	// []string{"setup-service", "--refresh", "--json"}. Required.
	Reconcile []string
	// Restore are the arguments that make the new binary restore the
	// definition from the Receipt on its standard input. Used when
	// RestoreFunc is nil.
	Restore []string
	// RestoreFunc, when set, restores in this process instead: the updater
	// is the old version, which is where magic-cli-remote restores
	// (0011-MADR amendment A1).
	RestoreFunc func(ctx context.Context, product string, r Receipt) error
	// Env is added to this process's environment for the child.
	Env []string
	// Runner runs the child. Nil means ExecRunner.
	Runner Runner
}

// ExecReconciler is a selfupdate.Reconciler that asks the product's own
// binary to reconcile its service definition, for a product whose setup
// command owns that definition (0011-MADR §6).
type ExecReconciler struct {
	o ExecOptions
}

var _ selfupdate.Reconciler = (*ExecReconciler)(nil)

// NewExecReconciler validates o. It needs Reconcile, and Restore or
// RestoreFunc.
func NewExecReconciler(o ExecOptions) (*ExecReconciler, error) {
	if len(o.Reconcile) == 0 {
		return nil, errors.New("selfupdate: service: exec reconciler needs reconcile arguments")
	}
	if len(o.Restore) == 0 && o.RestoreFunc == nil {
		return nil, errors.New("selfupdate: service: exec reconciler needs restore arguments or a restore function")
	}
	if o.Runner == nil {
		o.Runner = ExecRunner()
	}
	o.Reconcile = append([]string(nil), o.Reconcile...)
	o.Restore = append([]string(nil), o.Restore...)
	o.Env = append([]string(nil), o.Env...)
	return &ExecReconciler{o: o}, nil
}

// Reconcile implements selfupdate.Reconciler. It runs executable, the new
// binary, with the reconcile arguments, and decodes its Receipt. A child
// that fails after printing a receipt returns that receipt with the error,
// so the managed installer can still Restore it.
func (r *ExecReconciler) Reconcile(ctx context.Context, product, executable string) (selfupdate.ReconcileResult, error) {
	out, err := r.o.Runner.Run(ctx, Command{Path: executable, Args: r.o.Reconcile, Env: r.env()})
	if err != nil {
		return selfupdate.ReconcileResult{}, fmt.Errorf("selfupdate: service: reconcile %s: %w", product, err)
	}
	rec, perr := decodeReceipt(out.Stdout)
	if perr != nil {
		if out.ExitCode != 0 {
			return selfupdate.ReconcileResult{}, childError(product, "reconcile", out)
		}
		return selfupdate.ReconcileResult{}, fmt.Errorf("selfupdate: service: reconcile %s: %w", product, perr)
	}
	result := selfupdate.ReconcileResult{
		Changed:  rec.Changed,
		Detail:   receiptDetail(rec),
		State:    ExecState{Receipt: rec, Executable: executable},
		Warnings: selfupdate.NewWarnings(rec.Warnings...),
	}
	if rec.Changed && !rec.Reloaded {
		// The update succeeds; the user is told (0015-MADR G4).
		result.Warnings = result.Warnings.Add(fmt.Sprintf(
			"%s: %s was rewritten and the service manager was not reloaded; the next start may run the previous definition",
			product, rec.Path))
	}
	if out.ExitCode != 0 {
		return result, childError(product, "reconcile", out)
	}
	return result, nil
}

// Restore implements selfupdate.Reconciler. A receipt that changed nothing
// needs nothing.
func (r *ExecReconciler) Restore(ctx context.Context, product string, receipt selfupdate.ReconcileResult) error {
	st, ok := receipt.State.(ExecState)
	if !ok {
		if !receipt.Changed && receipt.State == nil {
			return nil
		}
		return fmt.Errorf("selfupdate: service: restore %s: the receipt is not an exec reconciler's", product)
	}
	if !st.Receipt.Changed {
		return nil
	}
	if r.o.RestoreFunc != nil {
		return r.o.RestoreFunc(ctx, product, st.Receipt)
	}
	body, err := json.Marshal(st.Receipt)
	if err != nil {
		return err
	}
	out, err := r.o.Runner.Run(ctx, Command{Path: st.Executable, Args: r.o.Restore, Env: r.env(), Stdin: body})
	if err != nil {
		return fmt.Errorf("selfupdate: service: restore %s: %w", product, err)
	}
	if out.ExitCode != 0 {
		return childError(product, "restore", out)
	}
	return nil
}

func (r *ExecReconciler) env() []string {
	return append(os.Environ(), r.o.Env...)
}

// decodeReceipt reads the one JSON object a reconcile command prints.
func decodeReceipt(stdout []byte) (Receipt, error) {
	dec := json.NewDecoder(bytes.NewReader(stdout))
	var rec Receipt
	if err := dec.Decode(&rec); err != nil {
		return Receipt{}, fmt.Errorf("malformed receipt: %w", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return Receipt{}, errors.New("malformed receipt: more than one JSON value")
	}
	if rec.SchemaVersion == 0 {
		rec.SchemaVersion = ReceiptSchema
	}
	if rec.Verdict == "" {
		return Receipt{}, errors.New("malformed receipt: no verdict")
	}
	return rec, nil
}

func receiptDetail(rec Receipt) string {
	d := rec.Verdict
	if rec.Path != "" {
		d += " " + rec.Path
	}
	if rec.Reason != "" {
		d += ": " + rec.Reason
	}
	return d
}

// maxChildMessage bounds the child output quoted in an error.
const maxChildMessage = 4 << 10

func childError(product, step string, out Output) error {
	msg := strings.TrimSpace(string(out.Stderr))
	if msg == "" {
		msg = strings.TrimSpace(string(out.Stdout))
	}
	if len(msg) > maxChildMessage {
		msg = msg[:maxChildMessage] + "…"
	}
	return fmt.Errorf("selfupdate: service: %s %s: exit %d: %s", step, product, out.ExitCode, msg)
}

// absExecutable resolves path, or this process's executable when path is
// empty, to an absolute path with symlinks evaluated.
func absExecutable(path string) (string, error) {
	if path == "" {
		var err error
		if path, err = os.Executable(); err != nil {
			return "", err
		}
	}
	path, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	return filepath.Abs(path)
}
