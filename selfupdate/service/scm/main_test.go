package scm

import (
	"os"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// liveArg makes the test binary act as the live test's service, its
// agent, or its detached update, instead of running tests
// (live_windows_test.go). The SCM starts a service with its command line,
// not this process's environment, so the mode is an argument.
const liveArg = "scm-live"

func TestMain(m *testing.M) {
	// First, as a program does: in a hop it starts the real run and exits
	// (LoadHandOffEnv calls service.HandOffHop).
	if err := service.LoadHandOffEnv(); err != nil {
		os.Exit(5)
	}
	if len(os.Args) > 2 && os.Args[1] == liveArg {
		os.Exit(liveMode(os.Args[2], os.Args[3:]))
	}
	if os.Getenv(fakeEnv) != "" {
		os.Exit(fakeRun())
	}
	os.Exit(m.Run())
}
