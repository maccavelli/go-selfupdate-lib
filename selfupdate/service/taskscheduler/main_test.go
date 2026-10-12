package taskscheduler

import (
	"os"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// liveArg makes the test binary act as the live test's task, its agent, or
// its detached update (live_windows_test.go). A task cannot carry
// environment (0021-MADR, F9, probe 9), so the mode is an argument.
const liveArg = "tasksched-live"

func TestMain(m *testing.M) {
	if err := service.LoadHandOffEnv(); err != nil {
		os.Exit(5)
	}
	if len(os.Args) > 2 && os.Args[1] == liveArg {
		os.Exit(liveMode(os.Args[2], os.Args[3:]))
	}
	os.Exit(m.Run())
}
