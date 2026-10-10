//go:build unix

package winproc

import (
	"os/exec"
	"syscall"
)

// checkProgram refuses nothing: a Unix kernel runs a script through its
// own interpreter line, not a shell that reparses the arguments.
func checkProgram(*exec.Cmd) error { return nil }

// attachedAttr leaves the child as os/exec starts it.
func attachedAttr() *syscall.SysProcAttr { return nil }

// detachedAttr starts the handoff's run in a new session.
func detachedAttr(bool) *syscall.SysProcAttr { return &syscall.SysProcAttr{Setsid: true} }
