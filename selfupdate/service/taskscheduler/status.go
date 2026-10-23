package taskscheduler

import (
	"context"
	"errors"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// hasNotRun is SCHED_S_TASK_HAS_NOT_RUN, a never-run task's last result
// (0021-MADR, Not verified; measured by TestCOMLayout).
const hasNotRun = 0x00041303

// Status is a task's state at one moment (0032-MADR D4-A), as one
// Task Scheduler read gives it, in place of a PowerShell
// Get-ScheduledTask.
type Status struct {
	// Installed is Installed's answer: Task Scheduler knows the task.
	Installed bool `json:"installed"`
	// Enabled is Enabled's answer: the task is enabled and has an enabled
	// trigger that starts it with nobody acting.
	Enabled bool `json:"enabled"`
	// State is the task's TASK_STATE by name: unknown, disabled, queued,
	// ready or running; empty when the task is not installed.
	State string `json:"state"`
	// Running is Running's answer: Task Scheduler reports the task
	// running, or lists a running instance.
	Running bool `json:"running"`
	// PIDs are the running instances' processes.
	PIDs []uint32 `json:"pids"`
	// LastRunTime is when the last run began; zero when the task has
	// never run.
	LastRunTime time.Time `json:"last_run_time,omitzero"`
	// LastResult is the last run's result: the program's exit code for an
	// Exec action (0021-MADR Amendment A1, probe 3), or 0x41303,
	// SCHED_S_TASK_HAS_NOT_RUN, when it has never run.
	LastResult int32 `json:"last_result"`
}

// Status reads every state of the task with one open: whether it is
// installed, Enabled's answer, its TASK_STATE, Running's answer, its
// running instances' processes, and its last run. A task that is not
// installed is a zero Status and no error. It checks ctx before it starts
// and ignores it after.
//
// Errors, matched with errors.Is: service.ErrPermission.
func (t *Task) Status(ctx context.Context, product string) (_ Status, err error) {
	if err := ctx.Err(); err != nil {
		return Status{}, err
	}
	name, path, err := t.task(product)
	if err != nil {
		return Status{}, err
	}
	r, err := t.open(name, path)
	if errors.Is(err, service.ErrNotInstalled) {
		return Status{}, nil
	}
	if err != nil {
		return Status{}, err
	}
	defer func() { err = errors.Join(err, r.close()) }()
	s, err := readStatus(r)
	if err != nil {
		return Status{}, mapError("query", path, err)
	}
	return s, nil
}

func readStatus(r registered) (Status, error) {
	st, pids, err := status(r)
	if err != nil {
		return Status{}, err
	}
	on, triggers, err := switches(r)
	if err != nil {
		return Status{}, err
	}
	last, err := r.lastRun()
	if err != nil {
		return Status{}, err
	}
	s := Status{
		Installed:  true,
		Enabled:    enabledBy(on, triggers),
		State:      stateName(st),
		Running:    st == stateRunning || len(pids) > 0,
		PIDs:       pids,
		LastResult: last.Result,
	}
	if last.Result != hasNotRun {
		s.LastRunTime = last.Time
	}
	return s, nil
}
