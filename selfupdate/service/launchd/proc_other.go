//go:build !unix

package launchd

import "io/fs"

// fileOwner is unknown off Unix: -1 leaves an owner unchanged.
func fileOwner(fs.FileInfo) (uid, gid int) { return -1, -1 }

// pidAlive is false off Unix, where New refuses anyway.
func pidAlive(int) bool { return false }

// processGroups cannot be read off Unix.
func processGroups(int) (mine, theirs int, ok bool) { return 0, 0, false }
