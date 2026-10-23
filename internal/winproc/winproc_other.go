//go:build !unix && !windows

package winproc

import "syscall"

// refuseBatchFile refuses nothing where there is no cmd.exe.
func refuseBatchFile(string) error { return nil }

// attachedAttr leaves the child as os/exec starts it.
func attachedAttr() *syscall.SysProcAttr { return nil }

// detachedAttr adds nothing: the service package does not detach here.
func detachedAttr(bool) *syscall.SysProcAttr { return nil }
