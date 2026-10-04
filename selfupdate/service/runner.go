package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"time"
)

// Command is one program run. Path must be absolute: a runner never looks a
// tool up on PATH (0011-MADR §2).
type Command struct {
	// Path is the program's absolute path.
	Path string
	// Args are the arguments, without the program name.
	Args []string
	// Env is the complete environment. Nil inherits this process's, as
	// os/exec does; the backends always pass one they built.
	Env []string
	// Stdin, when non-nil, is written to the program's standard input.
	Stdin []byte
}

// Output is what a program produced. A non-zero ExitCode is not an error:
// the caller decides what each exit code means.
type Output struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// A Runner runs a Command. It returns an error only when the program could
// not be run, or the context ended.
type Runner interface {
	Run(ctx context.Context, c Command) (Output, error)
}

// RunnerFunc adapts a function to Runner.
type RunnerFunc func(ctx context.Context, c Command) (Output, error)

// Run implements Runner.
func (f RunnerFunc) Run(ctx context.Context, c Command) (Output, error) {
	return f(ctx, c)
}

// maxOutput bounds each captured stream; the rest is discarded.
const maxOutput = 1 << 20

// waitDelay bounds the wait for the program's streams once it has exited
// or been killed, so a grandchild holding them cannot hang the run.
const waitDelay = 5 * time.Second

// ExecRunner returns the Runner the backends use by default. It runs the
// program with os/exec, kills it when the context ends, and keeps at most
// 1 MiB of each output stream.
func ExecRunner() Runner {
	return RunnerFunc(runExec)
}

func runExec(ctx context.Context, c Command) (Output, error) {
	if !filepath.IsAbs(c.Path) {
		return Output{}, fmt.Errorf("selfupdate: service: %q is not an absolute path", c.Path)
	}
	cmd := exec.CommandContext(ctx, c.Path, c.Args...) //nolint:gosec // the backends pass a fixed tool path and validated arguments
	cmd.Env = c.Env
	if c.Stdin != nil {
		cmd.Stdin = bytes.NewReader(c.Stdin)
	}
	var stdout, stderr cappedBuffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.WaitDelay = waitDelay
	err := cmd.Run()
	out := Output{Stdout: stdout.buf.Bytes(), Stderr: stderr.buf.Bytes()}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return out, fmt.Errorf("selfupdate: service: %s: %w", filepath.Base(c.Path), ctxErr)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		out.ExitCode = exitErr.ExitCode()
		return out, nil
	}
	if err != nil {
		return out, fmt.Errorf("selfupdate: service: run %s: %w", filepath.Base(c.Path), err)
	}
	return out, nil
}

// cappedBuffer keeps the first maxOutput bytes written to it and discards
// the rest, reporting every write as complete so the program is never
// blocked or failed by the cap.
type cappedBuffer struct {
	buf bytes.Buffer
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if room := maxOutput - c.buf.Len(); room > 0 {
		if len(p) > room {
			c.buf.Write(p[:room])
		} else {
			c.buf.Write(p)
		}
	}
	return len(p), nil
}
