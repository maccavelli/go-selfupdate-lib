//go:build !unix && !windows

package winproc

import (
	"os/exec"
	"syscall"
)

// checkProgram refuses nothing where there is no cmd.exe.
func checkProgram(*exec.Cmd) error { return nil }

// attachedAttr leaves the child as os/exec starts it.
func attachedAttr() *syscall.SysProcAttr { return nil }

// detachedAttr adds nothing: the service package does not detach here.
func detachedAttr(bool) *syscall.SysProcAttr { return nil }
