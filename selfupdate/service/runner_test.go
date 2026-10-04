package service

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestExecRunnerExitCodeIsNotAnError(t *testing.T) {
	c := fakeCommand(t, "receipt", "FAKE_OUT=out", "FAKE_ERR=err", "FAKE_EXIT=7")
	out, err := ExecRunner().Run(context.Background(), c)
	if err != nil || out.ExitCode != 7 || string(out.Stdout) != "out" || string(out.Stderr) != "err" {
		t.Fatalf("out = %+v, err = %v", out, err)
	}
}

func TestExecRunnerRefusesRelativePath(t *testing.T) {
	if _, err := ExecRunner().Run(context.Background(), Command{Path: "systemctl"}); err == nil ||
		!strings.Contains(err.Error(), "not an absolute path") {
		t.Fatalf("a PATH lookup was allowed: %v", err)
	}
}

func TestExecRunnerCapsOutput(t *testing.T) {
	// The cap is 1 MiB (0011-PLAN V1 step 1), written as a literal so a
	// change to the constant shows.
	out, err := ExecRunner().Run(context.Background(), fakeCommand(t, "big"))
	if err != nil || len(out.Stdout) != 1<<20 {
		t.Fatalf("%d bytes kept, err = %v; want 1 MiB", len(out.Stdout), err)
	}
}

func TestExecRunnerContextKills(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := ExecRunner().Run(ctx, fakeCommand(t, "sleep"))
	if err == nil || time.Since(start) > 30*time.Second {
		t.Fatalf("err = %v after %s; want the context's error, promptly", err, time.Since(start))
	}
}

func TestExecRunnerStdinAndEnv(t *testing.T) {
	c := fakeCommand(t, "env")
	c.Env = []string{fakeEnv + "=env", "ONLY=this"}
	out, err := ExecRunner().Run(context.Background(), c)
	if err != nil || !strings.Contains(string(out.Stdout), "ONLY=this") || strings.Contains(string(out.Stdout), "HOME=") {
		t.Fatalf("the environment was not the one given: %q, %v", out.Stdout, err)
	}
}
