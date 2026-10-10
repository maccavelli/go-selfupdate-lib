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

// Detach implements service.Detacher: it starts spec as a hop, which
// starts the real run detached and exits (service.HandOffHop), so the run
// does not descend from this process or the task. Stop ends only the
// task's own process, so the run survives it, even inside the task's job,
// which refuses breakaway (0021-MADR D6; Amendment A1, probe 2). The
// program must call service.ReportFunc or service.LoadHandOffEnv at
// start-up, as a detached run does anyway.
func (t *Task) Detach(ctx context.Context, spec service.HandOff) (service.Detached, error) {
	spec.Env = append(append([]string(nil), spec.Env...), service.EnvHandOffHop+"=1")
	det, err := service.DetachProcess(ctx, spec)
	if err != nil {
		return service.Detached{}, err
	}
	det.Where = "a detached run, started through the hop " + det.Where
	return det, nil
}
