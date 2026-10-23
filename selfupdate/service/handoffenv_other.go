//go:build !windows

package service

// protectDir does nothing: WriteHandOffEnv's mode 0700 protects the
// directory.
func protectDir(string) error { return nil }
