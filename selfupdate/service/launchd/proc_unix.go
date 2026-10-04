//go:build unix

package launchd

import (
	"errors"
	"io/fs"
	"syscall"
)

// fileOwner is info's owner and group.
func fileOwner(info fs.FileInfo) (uid, gid int) {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return int(st.Uid), int(st.Gid)
	}
	return -1, -1
}

// pidAlive reports whether a process with pid exists. EPERM means it does,
// owned by someone else.
func pidAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// processGroups returns this process's group and pid's, or ok false when
// pid's cannot be read.
func processGroups(pid int) (mine, theirs int, ok bool) {
	g, err := syscall.Getpgid(pid)
	if err != nil {
		return 0, 0, false
	}
	return syscall.Getpgrp(), g, true
}
