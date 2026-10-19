package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/internal/testproc"
)

// Typed command errors (docs/decisions/0025-MADR-align-with-go-sdk-design-practices.md
// D2). The systemd and launchd backends' cases are in their own packages'
// tests.

func TestCommandErrorText(t *testing.T) {
	testproc.CheckNoLeak(t)
	var nilErr *CommandError
	if got := nilErr.Error(); got != "selfupdate: service: command failed" {
		t.Errorf("nil CommandError = %q", got)
	}
	e := &CommandError{Backend: "systemd", Step: "stop", Name: "demo.service", ExitCode: 5, Message: "boom"}
	if got, want := e.Error(), "selfupdate: systemd: stop demo.service: exit 5: boom"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

// TestChildErrorIsCommandError: the exec reconciler's failed child is a
// *CommandError with the text it had before, its message still capped.
func TestChildErrorIsCommandError(t *testing.T) {
	testproc.CheckNoLeak(t)
	err := childError("demo", "reconcile", Output{Stderr: []byte(" broke \n"), ExitCode: 3})
	var ce *CommandError
	if !errors.As(err, &ce) || ce.Backend != "service" || ce.Step != "reconcile" || ce.Name != "demo" || ce.ExitCode != 3 {
		t.Fatalf("err = %#v, want the child as a *CommandError", err)
	}
	if got, want := err.Error(), "selfupdate: service: reconcile demo: exit 3: broke"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
	long := childError("demo", "restore", Output{Stdout: []byte(strings.Repeat("x", maxChildMessage+10)), ExitCode: 1})
	if !errors.As(long, &ce) || len(ce.Message) != maxChildMessage+len("…") {
		t.Fatalf("a long message is %d bytes, want the %d-byte cap and an ellipsis", len(ce.Message), maxChildMessage)
	}
}
