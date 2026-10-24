package procfs

import (
	"strings"
	"testing"
)

// Tests for parseStat
// (docs/decisions/0035-PLAN-serve-supervisors-through-a-supervised-adapter.md
// M2, Deviation D1). The lines are /proc/<pid>/stat as Linux writes it:
// pid, (command), then the state and 49 more fields.

// statLine is a stat line for comm with parent 41, state st, and start time
// 9876 in field 22.
func statLine(comm, st string) string {
	f := []string{st, "41", "40", "40", "0", "-1", "4194560", "100", "0", "0", "0", "1", "2", "0", "0", "20", "0", "1", "0", "9876", "8650752", "200"}
	return "4242 (" + comm + ") " + strings.Join(f, " ") + " 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0\n"
}

func TestParseStat(t *testing.T) {
	for _, comm := range []string{"sleep", "a b) (c", "x)", "(", ") ) )"} {
		s, err := parseStat(statLine(comm, "S"))
		if err != nil || s != (Stat{Parent: 41, StartTime: 9876, State: 'S'}) {
			t.Errorf("command %q: %+v, %v", comm, s, err)
		}
	}
	if s, err := parseStat(statLine("z", "Z")); err != nil || !s.Zombie() {
		t.Errorf("zombie: %+v, %v", s, err)
	}
	if s, _ := parseStat(statLine("r", "R")); s.Zombie() {
		t.Error("a running process reads as a zombie")
	}
}

func TestParseStatRefuses(t *testing.T) {
	short := "4242 (sleep) S 41 40 40 0 -1\n"
	for name, line := range map[string]string{
		"no command name":      "4242 sleep S 41\n",
		"too few fields":       short,
		"state not one letter": strings.Replace(statLine("x", "S"), ") S ", ") SS ", 1),
		"parent not a number":  strings.Replace(statLine("x", "S"), ") S 41 ", ") S p ", 1),
		"start not a number":   strings.Replace(statLine("x", "S"), " 9876 ", " t ", 1),
		"empty":                "",
	} {
		if s, err := parseStat(line); err == nil {
			t.Errorf("%s: parsed %+v", name, s)
		}
	}
}
