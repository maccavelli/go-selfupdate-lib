package taskscheduler

import (
	"context"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/internal/winproc"
)

// TestInside: this process is inside when it is a running instance's
// process or descends from one, by the shared walk (0021-MADR D6).
func TestInside(t *testing.T) {
	for _, c := range []struct {
		name  string
		pids  []uint32
		procs []winproc.Process
		want  bool
	}{
		{"self", []uint32{selfPID}, nil, true},
		{"parent", []uint32{6000}, nil, true},
		{"grandparent", []uint32{4}, nil, true},
		{"second instance", []uint32{5555, 6000}, nil, true},
		{"unrelated", []uint32{5555}, nil, false},
		{"not running", nil, nil, false},
		// The hop has exited: the walk stops at the missing parent.
		{"parent gone", []uint32{6000}, []winproc.Process{{PID: selfPID, ParentPID: 6100}, {PID: 6000, ParentPID: 4}}, false},
	} {
		f := newFake()
		f.procs = c.procs
		f.add(`\demo`, 0).pids = c.pids
		task, _ := testTask(t, f, Options{})
		if got, err := task.Inside(context.Background()); err != nil || got != c.want {
			t.Errorf("%s: inside %t, %v; want %t", c.name, got, err, c.want)
		}
	}
	task, _ := testTask(t, newFake(), Options{})
	task.o.Name = ""
	if _, err := task.Inside(context.Background()); err == nil {
		t.Fatal("Inside without Options.Name")
	}
}
