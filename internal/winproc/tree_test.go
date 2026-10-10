package winproc

import (
	"errors"
	"testing"
)

// TestDescends covers each way the walk ends.
func TestDescends(t *testing.T) {
	// 7000 is this process; its parent is 6000, whose parent is 4, whose
	// parent is 0, which is its own parent, as the System Idle Process is.
	tree := []Process{{PID: 7000, ParentPID: 6000}, {PID: 6000, ParentPID: 4}, {PID: 4, ParentPID: 0}, {PID: 0, ParentPID: 0}}
	none := func(uint32) uint64 { return 0 }
	born := func(m map[uint32]uint64) func(uint32) uint64 { return func(pid uint32) uint64 { return m[pid] } }

	// A chain of 100 processes: 1 is self, 100 the eldest.
	var chain []Process
	for pid := uint32(1); pid <= 100; pid++ {
		chain = append(chain, Process{PID: pid, ParentPID: pid + 1})
	}

	for _, c := range []struct {
		name    string
		procs   []Process
		self    uint32
		target  uint32
		created func(uint32) uint64
		want    bool
	}{
		{"self", tree, 7000, 7000, none, true},
		{"parent", tree, 7000, 6000, none, true},
		{"grandparent", tree, 7000, 4, none, true},
		{"unrelated", tree, 7000, 5555, none, false},
		{"own parent ends the walk", tree, 7000, 9999, none, false},
		// The hop has exited: the walk stops at the missing parent.
		{"parent gone", []Process{{PID: 7000, ParentPID: 6100}, {PID: 6000, ParentPID: 4}}, 7000, 6000, none, false},
		// 6000 was created after this process: a reused PID.
		{"reused pid", tree, 7000, 6000, born(map[uint32]uint64{7000: 100, 6000: 200}), false},
		{"older parent", tree, 7000, 6000, born(map[uint32]uint64{7000: 200, 6000: 100}), true},
		// A time that cannot be read counts the link.
		{"unreadable time", tree, 7000, 6000, born(map[uint32]uint64{7000: 200}), true},
		// The walk stops after maxAncestors steps.
		{"within the bound", chain, 1, 64, none, true},
		{"beyond the bound", chain, 1, 100, none, false},
	} {
		got, err := Descends(c.self, c.target, c.procs, c.created)
		if err != nil || got != c.want {
			t.Errorf("%s: Descends %t, %v; want %t", c.name, got, err, c.want)
		}
	}

	if _, err := Descends(1234, 4, tree, none); !errors.Is(err, ErrNotInSnapshot) {
		t.Errorf("self missing: %v, want ErrNotInSnapshot", err)
	}
}
