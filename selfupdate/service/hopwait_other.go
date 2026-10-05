//go:build !unix && !windows

package service

import "time"

// waitHopExit has no hop to wait for here: DetachProcess is unsupported.
func waitHopExit(int, time.Duration) {}

func processRunning(int) bool { return false }
