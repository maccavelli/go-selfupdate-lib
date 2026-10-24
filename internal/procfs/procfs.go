// Package procfs reads a Linux process's parent, start time, state and
// binary from /proc, for service.Supervised: its inside check, the instance
// it records at a stop, and the binary it verifies at a reconcile
// (docs/decisions/0035-MADR-serve-supervisors-through-a-supervised-adapter.md
// D3, D7, D9 and D13-C; 0035-PLAN-serve-supervisors-through-a-supervised-adapter.md
// Deviation D1). Off Linux every function returns ErrUnsupported.
//
// It imports only the standard library, and nothing in this module.
package procfs

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrUnsupported is returned off Linux, which is the only OS with this
// /proc.
var ErrUnsupported = errors.New("selfupdate: procfs: /proc is read on Linux only")

// Stat is what one read of /proc/<pid>/stat gives: the fields come from one
// process, even if its PID is reused right after.
type Stat struct {
	// Parent is the parent's PID, field 4.
	Parent int
	// StartTime is when the process started, in clock ticks after boot,
	// field 22. With the PID it tells a process from a later one that
	// reused the PID (0035-MADR D3; Amendment A1, P1).
	StartTime uint64
	// State is the state letter, field 3, such as 'R', 'S' or 'Z'.
	State byte
}

// Zombie reports a process that has exited and waits to be reaped. It
// keeps its entry and its start time until then, but runs nothing, so it
// counts as gone (0035-MADR Amendment A1, P4).
func (s Stat) Zombie() bool { return s.State == 'Z' }

// parseStat reads a /proc/<pid>/stat line. The command name, field 2, is in
// parentheses and may itself hold spaces and parentheses, so the fields are
// read after its last ')' (proc(5); 0035-MADR Amendment A1, P1).
func parseStat(line string) (Stat, error) {
	i := strings.LastIndexByte(line, ')')
	if i < 0 {
		return Stat{}, errors.New("selfupdate: procfs: stat has no command name")
	}
	// f[0] is field 3, f[1] field 4, and f[19] field 22.
	f := strings.Fields(line[i+1:])
	if len(f) < 20 {
		return Stat{}, fmt.Errorf("selfupdate: procfs: stat has %d fields after the command name, want at least 20", len(f))
	}
	if len(f[0]) != 1 {
		return Stat{}, fmt.Errorf("selfupdate: procfs: stat state %q is not one letter", f[0])
	}
	parent, err := strconv.Atoi(f[1])
	if err != nil {
		return Stat{}, fmt.Errorf("selfupdate: procfs: stat parent: %w", err)
	}
	start, err := strconv.ParseUint(f[19], 10, 64)
	if err != nil {
		return Stat{}, fmt.Errorf("selfupdate: procfs: stat start time: %w", err)
	}
	return Stat{Parent: parent, StartTime: start, State: f[0][0]}, nil
}
