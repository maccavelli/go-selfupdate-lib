//go:build !unix && !windows

package service

import (
	"context"
	"time"
)

// waitHopExit has no hop to wait for here: DetachProcess is unsupported.
func waitHopExit(context.Context, int, time.Duration) error { return nil }

func processRunning(int) bool { return false }
