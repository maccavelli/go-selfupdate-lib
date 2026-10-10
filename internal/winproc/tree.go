package winproc

import "errors"

// maxAncestors bounds the walk up the process table.
const maxAncestors = 64

// Process is one entry of a process snapshot.
type Process struct {
	PID       uint32
	ParentPID uint32
}

// ErrNotInSnapshot is Descends's error when self is not in the snapshot.
var ErrNotInSnapshot = errors.New("this process is not in the process snapshot")

// Descends reports whether self is target, or descends from it, by walking
// from self up the recorded parents in procs. It is the walk the scm and
// taskscheduler backends' Inside share (0021-MADR D6).
//
// The walk ends at a parent that has exited, which is why a handoff
// detaches through a hop (0011-MADR amendment A4), and at a parent created
// after its child: a reused process ID. created gives a process's creation
// time, or 0 when it cannot be read; a time that cannot be read counts the
// link as genuine, erring towards a handoff.
func Descends(self, target uint32, procs []Process, created func(pid uint32) uint64) (bool, error) {
	byPID := make(map[uint32]Process, len(procs))
	for _, p := range procs {
		byPID[p.PID] = p
	}
	cur, ok := byPID[self]
	if !ok {
		return false, ErrNotInSnapshot
	}
	for range maxAncestors {
		if cur.PID == target {
			return true, nil
		}
		parent, ok := byPID[cur.ParentPID]
		if !ok || parent.PID == cur.PID {
			return false, nil
		}
		if pc, cc := created(parent.PID), created(cur.PID); pc != 0 && cc != 0 && pc > cc {
			return false, nil
		}
		cur = parent
	}
	return false, nil
}
