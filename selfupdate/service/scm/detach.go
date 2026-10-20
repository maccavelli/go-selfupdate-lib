package scm

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/maccavelli/go-selfupdate-lib/internal/winproc"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// Inside implements service.Detacher for the service Options.Name names:
// whether this process is the service's, or descends from it, and would be
// caught by a stop that takes the service's process tree with it
// (0011-MADR §3).
func (s *Service) Inside(context.Context) (bool, error) {
	if s.o.Name == "" {
		return false, errors.New("selfupdate: scm: the handoff needs Options.Name")
	}
	return s.insideService(s.o.Name)
}

// insideService walks from this process up its recorded parents, looking
// for the service's process (winproc.Descends). The walk ends at a parent
// that has exited, which is why a handoff detaches through a hop
// (0011-MADR amendment A4), and at a parent created after its child: a
// reused process ID.
func (s *Service) insideService(name string) (bool, error) {
	st, err := s.query(name)
	if errors.Is(err, service.ErrNotInstalled) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if st.ProcessID == 0 {
		return false, nil
	}
	procs, err := s.m.processes()
	if err != nil {
		return false, fmt.Errorf("selfupdate: scm: list processes: %w", err)
	}
	inside, err := winproc.Descends(s.self(), st.ProcessID, procs, s.m.created)
	if err != nil {
		return false, fmt.Errorf("selfupdate: scm: %w", err)
	}
	return inside, nil
}

// Detach implements service.Detacher: it starts spec as a hop, which
// starts the real run detached and exits (service.HandOffHop), so the run
// does not descend from this process or the service (0011-MADR amendment
// A4). The program must call service.ReportFunc,
// service.LoadHandOffEnv or service.LoadHandOffEnvContext at start-up,
// as a detached run does anyway.
func (s *Service) Detach(ctx context.Context, spec service.HandOff) (service.Detached, error) {
	spec.Env = append(append([]string(nil), spec.Env...), service.EnvHandOffHop+"=1")
	det, err := service.DetachProcess(ctx, spec)
	if err != nil {
		return service.Detached{}, err
	}
	det.Where = "a detached run, started through the hop " + det.Where
	return det, nil
}

// currentPID is this process's ID.
func currentPID() uint32 {
	return uint32(os.Getpid()) //nolint:gosec // a process ID fits in a DWORD
}
