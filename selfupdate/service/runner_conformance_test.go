package service_test

import (
	"context"
	"os"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/internal/testproc"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service/servicetest"
)

// TestExecRunnerConformance: ExecRunner meets servicetest.VerifyRunner, on
// the real clock, over the test binary as the program: its "receipt" mode
// answers at once, and its "sleep" mode runs for a minute unless killed
// (main_test.go;
// docs/decisions/0038-PLAN-pinned-plans-lifecycle-conformance-and-bubble-tests.md
// Q11).
func TestExecRunnerConformance(t *testing.T) {
	testproc.CheckNoLeak(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	tool := func(mode string) service.Command {
		return service.Command{Path: exe, Env: append(os.Environ(), "SELFUPDATE_SERVICE_FAKE="+mode)}
	}
	if err := servicetest.VerifyRunner(context.Background(), service.ExecRunner(), servicetest.RunnerCase{
		Quick: tool("receipt"), Blocking: tool("sleep"),
	}); err != nil {
		t.Fatal(err)
	}
}
