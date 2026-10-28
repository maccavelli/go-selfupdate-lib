package selfupdate

// Test-only exports for the package's external tests (package
// selfupdate_test). This file is compiled only by go test.

// CheckNoLeak is checkNoLeak.
var CheckNoLeak = checkNoLeak

// SetRunningPlatform makes p u's running platform, so a test can apply a
// fixture built for another platform (0010-PLAN-v1-6-0 S4). It changes u
// alone
// (docs/decisions/0038-MADR-pinned-plans-lifecycle-conformance-and-bubble-tests.md
// D3-A).
func SetRunningPlatform(u *Updater, p Platform) {
	u.platform = p
}
