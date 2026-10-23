// Package winproc builds every process the library's packages under
// selfupdate/ start, so that on Windows a child of a process with no
// console gets a console with no window, and no batch file is started. The
// release tool under internal/cmd starts its own, and is outside it
// (docs/decisions/0021-MADR-task-scheduler-lifecycle-and-windowless-children.md
// D7, D8). Off Windows it adds nothing but the Unix handoff's new session.
//
// It also walks the process tree, for the backends' check that the caller
// runs inside a service (Descends), and on Windows reads whether a process
// still runs by its object's signalled state (Alive, and the access it
// opens with, AliveAccess; docs/decisions/0022-MADR-documentation-pass-after-v1-14-0.md
// Amendments A2 and A3).
//
// It imports only the standard library and golang.org/x/sys/windows, and
// nothing in this module, so every package that starts a process can
// import it.
package winproc

import (
	"context"
	"errors"
	"os/exec"
)

// ErrBatchFile is returned for a command whose program is a .bat or .cmd
// file. Windows runs one through cmd.exe, whose parser can turn an
// argument into a second command (BatBadBut, CVE-2024-24576); this module
// never means to run one (0021-MADR D8).
var ErrBatchFile = errors.New("selfupdate: refusing to start a .bat or .cmd file, which Windows runs through cmd.exe (BatBadBut, CVE-2024-24576)")

// Command returns the command for a child this process waits for. On
// Windows it refuses a batch file, and, when this process has no console,
// starts the child with CREATE_NO_WINDOW, so the child and its own
// children share a console with no window (0021-MADR D7). Elsewhere it is
// exec.CommandContext.
func Command(ctx context.Context, path string, args ...string) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, path, args...) //nolint:gosec // the caller's program: a fixed tool, the release under test, or a reconcile command (0021-MADR D7)
	if err := CheckProgram(cmd.Path); err != nil {
		return nil, err
	}
	cmd.SysProcAttr = attachedAttr()
	return cmd, nil
}

// Detached returns the command for a process that outlives this one: on
// Windows in a new process group with a console of its own and no window,
// and out of this process's job when breakaway is set; on Unix in a new
// session (0011-MADR §9, as amended by 0021-MADR D7). On Windows it refuses
// a batch file.
func Detached(path string, args []string, breakaway bool) (*exec.Cmd, error) {
	cmd := exec.Command(path, args...) //nolint:gosec // HandOffIfInside resolved path to this program
	if err := CheckProgram(cmd.Path); err != nil {
		return nil, err
	}
	cmd.SysProcAttr = detachedAttr(breakaway)
	return cmd, nil
}

// CheckProgram refuses, on Windows, a program Windows would run through
// cmd.exe: a .bat or .cmd file, in any spelling the Win32 layer resolves
// to one, with an error wrapping ErrBatchFile (0021-MADR D8). Command and
// Detached make this check; a package that starts a program another way,
// such as through a Task Scheduler action, calls it itself
// (docs/decisions/0032-MADR-task-scheduler-registration-and-one-shot-handoff.md
// D8-A). Elsewhere it refuses nothing.
func CheckProgram(path string) error {
	return refuseBatchFile(path)
}
