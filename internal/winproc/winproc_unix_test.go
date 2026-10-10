//go:build unix

package winproc

import (
	"context"
	"testing"
)

// TestUnixCommandAddsNothing: off Windows, a waited-for child starts as
// os/exec starts it.
func TestUnixCommandAddsNothing(t *testing.T) {
	cmd, err := Command(context.Background(), "/bin/true")
	if err != nil {
		t.Fatal(err)
	}
	if cmd.SysProcAttr != nil {
		t.Fatalf("Command set SysProcAttr %+v, want none", cmd.SysProcAttr)
	}
}

// TestUnixDetachedStartsNewSession: the handoff's run starts in a new
// session, as detach_unix.go always started it.
func TestUnixDetachedStartsNewSession(t *testing.T) {
	cmd, err := Detached("/bin/true", nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setsid {
		t.Fatalf("Detached SysProcAttr %+v, want Setsid", cmd.SysProcAttr)
	}
}
