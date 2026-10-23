//go:build windows

package taskscheduler

import (
	"context"
	"errors"
	"testing"

	"golang.org/x/sys/windows"

	"github.com/maccavelli/go-selfupdate-lib/internal/winproc"
)

// TestComposeArgsMatchesEscapeArg pins escapeArg to the function it
// follows, golang.org/x/sys/windows.EscapeArg.
func TestComposeArgsMatchesEscapeArg(t *testing.T) {
	for _, s := range []string{
		"", "plain", "a b", `"q"`, `back\slash\`, `a\ b\`, `say "hi" now`, `x\"y`, "tab\there",
		`C:\Program Files\Demo\demo.exe`, `\\server\share\x`, `trailing\\`, `"`, `\`, `\"`, "ü 任务", `--selfupdate-handoff-env=C:\a b\c.env`,
	} {
		if got, want := escapeArg(s), windows.EscapeArg(s); got != want {
			t.Errorf("escapeArg(%q) = %s, windows.EscapeArg = %s", s, got, want)
		}
	}
}

// TestDetachOneShotRefusesBatchFile: a .cmd program is refused before
// anything is written or registered (0021-MADR D8).
func TestDetachOneShotRefusesBatchFile(t *testing.T) {
	f := newFake()
	task, _ := oneShotTask(t, f, Options{})
	spec := oneShotSpec
	spec.Executable = `C:\Program Files\Demo\demo.cmd`
	if _, err := task.Detach(context.Background(), spec); !errors.Is(err, winproc.ErrBatchFile) {
		t.Fatalf("Detach of a .cmd: %v, want ErrBatchFile", err)
	}
	for _, v := range f.verbs() {
		if v != "" && v[0] == 'r' {
			t.Fatalf("Task Scheduler was changed: %q", f.verbs())
		}
	}
}
