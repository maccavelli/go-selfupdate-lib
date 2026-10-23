//go:build unix

package winproc

import "syscall"

// refuseBatchFile refuses nothing: a Unix kernel runs a script through its
// own interpreter line, not a shell that reparses the arguments.
func refuseBatchFile(string) error { return nil }

// attachedAttr leaves the child as os/exec starts it.
func attachedAttr() *syscall.SysProcAttr { return nil }

// detachedAttr starts the handoff's run in a new session.
func detachedAttr(bool) *syscall.SysProcAttr { return &syscall.SysProcAttr{Setsid: true} }
