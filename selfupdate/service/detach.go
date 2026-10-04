package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// DetachProcess starts spec as a process that outlives this one: on Windows
// with no console, in a new process group, and out of this process's job
// when the job allows it; on Unix in a new session. Its standard streams
// are the null device. spec.Executable must be absolute, as HandOffIfInside
// leaves it (0011-MADR §9).
//
// It is for a lifecycle with no backend in this module. On Unix it is no
// defence against a cgroup kill (systemd) or a launchd job reap: the
// systemd and launchd backends detach through the service manager instead.
func DetachProcess(ctx context.Context, spec HandOff) (Detached, error) {
	if err := ctx.Err(); err != nil {
		return Detached{}, err
	}
	if !filepath.IsAbs(spec.Executable) {
		return Detached{}, fmt.Errorf("selfupdate: service: %q is not an absolute path", spec.Executable)
	}
	pid, err := startDetached(spec.Executable, spec.Args, append(os.Environ(), spec.Env...))
	if err != nil {
		return Detached{}, fmt.Errorf("selfupdate: service: start the detached run: %w", err)
	}
	return Detached{ID: spec.ID, Where: fmt.Sprintf("process %d", pid), ResultPath: spec.ResultPath}, nil
}
