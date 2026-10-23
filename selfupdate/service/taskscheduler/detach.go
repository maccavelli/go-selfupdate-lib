package taskscheduler

import (
	"context"
	"errors"
	"fmt"

	"github.com/maccavelli/go-selfupdate-lib/internal/winproc"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// Inside implements service.Detacher for the task Options.Name names:
// whether this process is a running instance's, or descends from one, and
// would be ended by Stop (0021-MADR D6).
func (t *Task) Inside(context.Context) (bool, error) {
	if t.o.Name == "" {
		return false, errors.New("selfupdate: taskscheduler: the handoff needs Options.Name")
	}
	name, path, err := t.task("")
	if err != nil {
		return false, err
	}
	return t.insideTask(name, path)
}

// insideTask walks from this process up its recorded parents, looking for a
// running instance's process (winproc.Descends). The walk ends at a parent
// that has exited, which is why a handoff detaches through a hop
// (0011-MADR amendment A4).
func (t *Task) insideTask(name, path string) (bool, error) {
	r, err := t.open(name, path)
	if errors.Is(err, service.ErrNotInstalled) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	pids, err := r.instances()
	if err = errors.Join(err, r.close()); err != nil {
		return false, mapError("list the instances of", path, err)
	}
	if len(pids) == 0 {
		return false, nil
	}
	procs, err := t.s.processes()
	if err != nil {
		return false, fmt.Errorf("selfupdate: taskscheduler: list processes: %w", err)
	}
	for _, pid := range pids {
		in, err := winproc.Descends(t.self(), pid, procs, t.s.created)
		if err != nil {
			return false, fmt.Errorf("selfupdate: taskscheduler: %w", err)
		}
		if in {
			return true, nil
		}
	}
	return false, nil
}

// Detach implements service.Detacher. By default it registers and runs a
// one-shot task, <Options.Name>.selfupdate.<id>, in the service task's
// folder, with the service task's user, logon type and run level. Task
// Scheduler starts that task's process, so the run is in none of this
// process's jobs, and survives both Stop and a job of this program's that
// kills its processes when it closes
// (docs/decisions/0032-MADR-task-scheduler-registration-and-one-shot-handoff.md
// D5-A; Amendment A1, P2-1 to P2-3). The run's environment travels in a
// private file in Options.HandOffDir, named by the run's first argument,
// service.ArgHandOffEnv; the run starts through a hop, so its console has
// no window, and only the hop's shows, for about 50 ms (P2-4). Detach
// returns once Task Scheduler reports the run started, bounded by ctx and
// Options.Poll's timeout, and first calls CleanupHandOffs. Only a service
// task that logs on with an interactive token is supported.
//
// With Options.HopDetach, it starts spec as a hop instead, which starts
// the real run detached and exits (service.HandOffHop), so the run does
// not descend from this process or the task. Stop ends only the task's
// own process, so the run survives it, even inside the task's job, which
// refuses breakaway (0021-MADR D6; Amendment A1, probe 2); a job of this
// program's that refuses breakaway and kills on close ends it.
//
// Either way, the program must call service.ReportFunc,
// service.LoadHandOffEnv or service.LoadHandOffEnvContext at start-up,
// before it reads its arguments, as a detached run does anyway.
func (t *Task) Detach(ctx context.Context, spec service.HandOff) (service.Detached, error) {
	if !t.o.HopDetach {
		return t.detachTask(ctx, spec)
	}
	spec.Env = append(append([]string(nil), spec.Env...), service.EnvHandOffHop+"=1")
	det, err := service.DetachProcess(ctx, spec)
	if err != nil {
		return service.Detached{}, err
	}
	det.Where = "a detached run, started through the hop " + det.Where
	return det, nil
}
