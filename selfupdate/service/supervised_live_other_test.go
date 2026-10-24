//go:build !linux

package service

// supervisedLiveMode has no live modes off Linux, where the supervised
// adapter does not run
// (docs/decisions/0035-PLAN-serve-supervisors-through-a-supervised-adapter.md
// M6).
func supervisedLiveMode(string) (int, bool) { return 0, false }
