package systemd

import (
	"context"
	"testing"
)

// Tests for docs/decisions/0011-PLAN-reference-service-lifecycles.md V2
// step 6 and amendment A2: Inside, and the environment file's format.
// Detach itself is in detach_unix_test.go.

func TestParseCgroup(t *testing.T) {
	for raw, want := range map[string]string{
		"0::/system.slice/demo.service\n":                               "/system.slice/demo.service",
		"12:pids:/x\n1:name=systemd:/system.slice/demo.service\n0::/\n": "/",
		"12:pids:/x\n1:name=systemd:/system.slice/demo.service\n":       "/system.slice/demo.service",
	} {
		if got, err := parseCgroup([]byte(raw)); err != nil || got != want {
			t.Errorf("parseCgroup(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}
	if _, err := parseCgroup([]byte("12:pids:/x\n")); err == nil {
		t.Error("a file with no systemd cgroup was accepted")
	}
}

func TestInside(t *testing.T) {
	f := newFake()
	u := testUnit(t, f, Options{})
	for cg, want := range map[string]bool{
		"0::/system.slice/demo.service\n":            true,
		"0::/system.slice/demo.service/agent-3\n":    true,
		"0::/system.slice/demo.service2\n":           false,
		"0::/user.slice/user-1000.slice/session-3\n": false,
	} {
		setCgroup(t, cg)
		if got, err := u.Inside(context.Background()); err != nil || got != want {
			t.Errorf("cgroup %q: Inside %t, %v", cg, got, err)
		}
	}
	f.props["ControlGroup"] = ""
	setCgroup(t, "0::/system.slice/demo.service\n")
	if got, err := u.Inside(context.Background()); err != nil || got {
		t.Errorf("a stopped unit: Inside %t, %v", got, err)
	}
	u.o.Unit = ""
	if _, err := u.Inside(context.Background()); err == nil {
		t.Error("Inside without Options.Unit was accepted")
	}
}

func TestEnvFileBody(t *testing.T) {
	got := string(envFileBody([]string{
		"A=plain", `B=say "hi" $HOME \n` + "`x`", "A=last", "1BAD=x", "NOEQUALS", "C=two\nlines", "bad-name=x",
	}))
	want := "A=\"last\"\nB=\"say \\\"hi\\\" \\$HOME \\\\n\\`x\\`\"\nC=\"two\nlines\"\n"
	if got != want {
		t.Fatalf("env file:\n%s\nwant:\n%s", got, want)
	}
}
