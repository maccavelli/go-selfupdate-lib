package launchd

import (
	"os"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// liveModeEnv makes the test binary act as the live test's job instead of
// running tests (live_darwin_test.go). The detached update is told by
// service.EnvHandOff, which the one-shot job's plist carries: anything else
// it needs is in the private environment file that only service.ReportFunc
// loads.
const liveModeEnv = "SELFUPDATE_LAUNCHD_FAKE"

func TestMain(m *testing.M) {
	if os.Getenv(service.EnvHandOff) != "" {
		os.Exit(liveMode("update"))
	}
	if mode := os.Getenv(liveModeEnv); mode != "" {
		os.Exit(liveMode(mode))
	}
	os.Exit(m.Run())
}
