package service

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The test binary doubles as the fake tool the tests run
// (SELFUPDATE_SERVICE_FAKE names the mode), so no test depends on a tool
// on the host (0011-PLAN V1).
const fakeEnv = "SELFUPDATE_SERVICE_FAKE"

func TestMain(m *testing.M) {
	if mode := os.Getenv(fakeEnv); mode != "" {
		os.Exit(fakeTool(mode))
	}
	os.Exit(m.Run())
}

// fakeTool runs one helper mode and returns its exit code.
func fakeTool(mode string) int {
	switch mode {
	case "receipt":
		// Print FAKE_OUT and FAKE_ERR, save stdin to FAKE_STDIN, exit with
		// FAKE_EXIT.
		if p := os.Getenv("FAKE_STDIN"); p != "" {
			in, _ := io.ReadAll(os.Stdin)
			_ = os.WriteFile(p, in, 0o600)
		}
		fmt.Print(os.Getenv("FAKE_OUT"))
		fmt.Fprint(os.Stderr, os.Getenv("FAKE_ERR"))
		code, _ := strconv.Atoi(os.Getenv("FAKE_EXIT"))
		return code
	case "big":
		// 3 MiB, a fixed size, so a change to the cap shows.
		_, _ = os.Stdout.Write(make([]byte, 3<<20))
		return 0
	case "sleep":
		time.Sleep(time.Minute)
		return 0
	case "env":
		fmt.Print(strings.Join(os.Environ(), "\n"))
		return 0
	case "parent":
		return fakeParent()
	case "child":
		return fakeChild()
	}
	return 2
}

// fakeParent waits for FAKE_PARENT_WAIT to exist when it is set, detaches a
// "child", prints its PID, then waits to be killed.
func fakeParent() int {
	if wait := os.Getenv("FAKE_PARENT_WAIT"); wait != "" && !waitForFile(wait) {
		return 6
	}
	exe, err := os.Executable()
	if err != nil {
		return 3
	}
	det, err := DetachProcess(context.Background(), HandOff{
		ID: "t", Executable: exe, Env: []string{fakeEnv + "=child"},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 4
	}
	fmt.Println(strings.TrimPrefix(det.Where, "process "))
	time.Sleep(time.Minute)
	return 0
}

// fakeChild waits for FAKE_GO to exist, then writes FAKE_ALIVE: proof it
// outlived its parent.
func fakeChild() int {
	if !waitForFile(os.Getenv("FAKE_GO")) {
		return 5
	}
	_ = os.WriteFile(os.Getenv("FAKE_ALIVE"), []byte(strconv.Itoa(os.Getpid())), 0o600)
	return 0
}

// waitForFile polls for path for up to 30 s.
func waitForFile(path string) bool {
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

// startParent runs the test binary as fakeParent, with the child's signal
// files, and returns it and a reader of the child's PID line.
func startParent(t *testing.T, cmd *exec.Cmd, env ...string) (childPID func() int) {
	t.Helper()
	cmd.Env = append(append(os.Environ(), fakeEnv+"=parent"), env...)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	lines := bufio.NewScanner(out)
	return func() int {
		t.Helper()
		if !lines.Scan() {
			t.Fatalf("the parent printed no child PID: %v", lines.Err())
		}
		pid, err := strconv.Atoi(strings.TrimSpace(lines.Text()))
		if err != nil {
			t.Fatal(err)
		}
		return pid
	}
}

// childOutlived reports whether the child wrote its alive file within 10 s
// of being told to.
func childOutlived(t *testing.T, goFile, alive string) bool {
	t.Helper()
	if err := os.WriteFile(goFile, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(alive); err == nil {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

// selfExe is the test binary's path.
func selfExe(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		t.Fatal(err)
	}
	return exe
}

// fakeCommand runs the test binary in mode with extra environment.
func fakeCommand(t *testing.T, mode string, env ...string) Command {
	t.Helper()
	return Command{Path: selfExe(t), Env: append(append(os.Environ(), fakeEnv+"="+mode), env...)}
}
