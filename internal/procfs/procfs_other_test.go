//go:build !linux

package procfs

import (
	"errors"
	"testing"
)

// TestUnsupported: off Linux every read is ErrUnsupported
// (docs/decisions/0035-PLAN-serve-supervisors-through-a-supervised-adapter.md M2).
func TestUnsupported(t *testing.T) {
	if _, err := ReadStat(1); !errors.Is(err, ErrUnsupported) {
		t.Errorf("ReadStat: %v", err)
	}
	if _, err := Exe(1); !errors.Is(err, ErrUnsupported) {
		t.Errorf("Exe: %v", err)
	}
}
