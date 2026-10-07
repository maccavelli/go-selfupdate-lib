package systemd

import (
	"context"
	"testing"
)

// TestStartBaselineIsAfterStart: the restart count WaitHealthy compares
// against is read once the start returned. A start issued while the unit
// waits out RestartSec is counted as a restart, so a count read before it
// would fail a healthy start
// (docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md D2,
// A3).
func TestStartBaselineIsAfterStart(t *testing.T) {
	f := newFake()
	f.props["ActiveState"], f.props["SubState"], f.props["NRestarts"] = "activating", "auto-restart", "1"
	f.onStart = func(p map[string]string) {
		p["ActiveState"], p["SubState"], p["InvocationID"], p["NRestarts"] = "active", "running", "bbb", "2"
	}
	u := testUnit(t, f, Options{})
	ctx := context.Background()
	if err := u.Start(ctx, "demo"); err != nil {
		t.Fatal(err)
	}
	if err := u.WaitHealthy(ctx, "demo"); err != nil {
		t.Fatalf("WaitHealthy = %v; a start during the restart wait is counted, and the unit is healthy", err)
	}
}
