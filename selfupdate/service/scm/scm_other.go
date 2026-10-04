//go:build !windows

package scm

// systemManager has no SCM off Windows; New refuses before using it.
func systemManager() manager { return nil }
