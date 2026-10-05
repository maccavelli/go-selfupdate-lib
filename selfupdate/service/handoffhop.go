package service

import (
	"fmt"
	"os"
	"strconv"
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

// EnvHandOffHopParent carries the hop's process ID to the real run, which
// waits for the hop to exit before anything else: until it has, a walk up
// the run's ancestors still reaches the service.
const EnvHandOffHopParent = "SELFUPDATE_HANDOFF_HOP_PARENT"

// hopWait bounds the real run's wait for the hop to exit.
const hopWait = 10 * time.Second

// HandOffHop finishes the hop. In a hop, with EnvHandOffHop set, it starts
// the real run, this executable with this process's arguments and its
// environment less EnvHandOffHop, plus EnvHandOffHopParent, detached as
// DetachProcess does, and exits 0. If the start fails it writes a
// HandOffResult with the error to EnvHandOffResult's path, when set, and
// exits 1; 2 when that write fails too.
//
// In the real run, with EnvHandOffHopParent set, it waits up to 10 s for
// the hop to exit, then unsets the variable. In any other run it does
// nothing.
//
// LoadHandOffEnv and ReportFunc call it first, so a program that calls
// either at start-up needs nothing more.
func HandOffHop() error {
	if os.Getenv(EnvHandOffHop) != "" {
		os.Exit(hop(os.Executable, os.Args[1:], os.Environ(), os.Getpid(), startDetached))
	}
	v, ok := os.LookupEnv(EnvHandOffHopParent)
	if !ok {
		return nil
	}
	if pid, err := strconv.Atoi(v); err == nil && pid > 0 {
		waitHopExit(pid, hopWait)
	}
	return os.Unsetenv(EnvHandOffHopParent)
}

// hop is HandOffHop's work in a hop, with its process inputs passed in;
// it returns the hop's exit code.
func hop(executable func() (string, error), args, environ []string, self int,
	start func(path string, args, env []string) (int, error)) int {
	started := time.Now().UTC()
	env := make([]string, 0, len(environ)+1)
	for _, kv := range environ {
		if !strings.HasPrefix(kv, EnvHandOffHop+"=") && !strings.HasPrefix(kv, EnvHandOffHopParent+"=") {
			env = append(env, kv)
		}
	}
	env = append(env, EnvHandOffHopParent+"="+strconv.Itoa(self))
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
