package service

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
)

// EnvHandOffHop marks a hop: a run of the update command that only starts
// the real detached run and exits, so the real run's recorded parent is a
// process that has exited. A walk up its ancestors then stops there, and a
// tree kill that follows parent links misses it, as after a Unix double
// fork. scm.Service.Detach sets it (0011-MADR amendment A4).
const EnvHandOffHop = "SELFUPDATE_HANDOFF_HOP"

// HandOffHop does nothing unless EnvHandOffHop is set. In a hop it starts
// the real run, this executable with this process's arguments and its
// environment less EnvHandOffHop, detached as DetachProcess does, and exits
// 0. If the start fails it writes a HandOffResult with the error to
// EnvHandOffResult's path, when set, and exits 1; 2 when that write fails
// too.
//
// LoadHandOffEnv and ReportFunc call it first, so a program that calls
// either at start-up needs nothing more.
func HandOffHop() {
	if os.Getenv(EnvHandOffHop) == "" {
		return
	}
	os.Exit(hop(os.Executable, os.Args[1:], os.Environ(), startDetached))
}

// hop is HandOffHop's work, with its process inputs passed in; it returns
// the hop's exit code.
func hop(executable func() (string, error), args, environ []string, start func(path string, args, env []string) (int, error)) int {
	started := time.Now().UTC()
	env := make([]string, 0, len(environ))
	for _, kv := range environ {
		if !strings.HasPrefix(kv, EnvHandOffHop+"=") {
			env = append(env, kv)
		}
	}
	exe, err := executable()
	if err == nil {
		_, err = start(exe, args, env)
	}
	if err == nil {
		return 0
	}
	err = fmt.Errorf("selfupdate: service: the handoff hop could not start the run: %w", err)
	if path := lookupEnv(environ, EnvHandOffResult); path != "" {
		if werr := WriteHandOffResult(path, HandOffResult{
			SchemaVersion: HandOffResultSchema,
			ID:            lookupEnv(environ, EnvHandOff),
			StartedAt:     started,
			FinishedAt:    time.Now().UTC(),
			ExitCode:      selfupdate.ExitCode(selfupdate.Result{}, err),
			Error:         err.Error(),
		}); werr != nil {
			return 2
		}
	}
	return 1
}

// lookupEnv returns key's value in environ, or "".
func lookupEnv(environ []string, key string) string {
	for _, kv := range environ {
		if v, ok := strings.CutPrefix(kv, key+"="); ok {
			return v
		}
	}
	return ""
}
