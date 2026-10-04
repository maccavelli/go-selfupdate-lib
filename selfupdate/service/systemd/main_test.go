package systemd

import (
	"os"
	"testing"
)

// liveModeEnv makes the test binary act as the live test's service or its
// detached update instead of running tests (live_linux_test.go).
const liveModeEnv = "SELFUPDATE_SYSTEMD_FAKE"

func TestMain(m *testing.M) {
	if mode := os.Getenv(liveModeEnv); mode != "" {
		os.Exit(liveMode(mode))
	}
	os.Exit(m.Run())
}
