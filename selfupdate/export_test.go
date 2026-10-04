package selfupdate

import "testing"

// Test-only exports for the package's external tests (package
// selfupdate_test). This file is compiled only by go test.

// CheckNoLeak is checkNoLeak.
var CheckNoLeak = checkNoLeak

// SetRunningPlatform makes p the running platform until t ends, so a test
// can apply a fixture built for another platform (0010-PLAN-v1-6-0 S4).
func SetRunningPlatform(t *testing.T, p Platform) {
	t.Helper()
	setSeam(t, &runningPlatform, func() Platform { return p })
}
