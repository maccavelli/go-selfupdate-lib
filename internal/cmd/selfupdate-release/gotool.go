package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// inherited are the only variables a go command inherits from the caller's
// environment: what it needs to find itself, its caches and the module
// proxy. GOFLAGS, GOAMD64, GOARM64, CGO_ENABLED and the rest are never
// inherited, and GOENV=off ignores the go env file, so nothing outside the
// recipe reaches the build (0013-MADR §4).
var inherited = []string{
	"PATH", "HOME", "USERPROFILE", "LOCALAPPDATA", "APPDATA", "SystemRoot", "XDG_CACHE_HOME",
	"TMPDIR", "TEMP", "TMP",
	"GOPATH", "GOCACHE", "GOMODCACHE",
	"GOPROXY", "GOSUMDB", "GOPRIVATE", "GONOPROXY", "GONOSUMDB", "GOINSECURE",
}

// goTool runs the go command with the recipe's environment.
type goTool struct {
	path string
	env  []string
}

func newGoTool() (goTool, error) {
	path, err := exec.LookPath("go")
	if err != nil {
		return goTool{}, fmt.Errorf("the go command is required: %w", err)
	}
	env := []string{"GOENV=off", "GOTOOLCHAIN=local", "GOFLAGS=-mod=readonly", "CGO_ENABLED=0", "GOWORK=off"}
	for _, name := range inherited {
		if v, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+v)
		}
	}
	return goTool{path: path, env: env}, nil
}

// run runs go with args in dir, with extra variables after the recipe's,
// and returns its stdout. A failure carries its stderr.
func (g goTool) run(ctx context.Context, dir string, extra []string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, g.path, args...) //nolint:gosec // G204: the go command found on PATH, with arguments built from a validated spec
	cmd.Dir = dir
	cmd.Env = append(append([]string{}, g.env...), extra...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("go %s: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// version is the toolchain's version, such as "go1.27.1".
func (g goTool) version(ctx context.Context, dir string) (string, error) {
	out, err := g.run(ctx, dir, nil, "env", "GOVERSION")
	return strings.TrimSpace(out), err
}

// modulePath is the main module's path in dir.
func (g goTool) modulePath(ctx context.Context, dir string) (string, error) {
	out, err := g.run(ctx, dir, nil, "list", "-m")
	return strings.TrimSpace(out), err
}
