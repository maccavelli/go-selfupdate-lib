package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// EnvHandOffEnv names a private file holding the rest of a detached run's
// environment, for a mechanism that would otherwise publish it, such as a
// launchd job's plist (0011-MADR amendment A3).
const EnvHandOffEnv = "SELFUPDATE_HANDOFF_ENV"

// ArgHandOffEnv is the first argument of a run that a Task Scheduler
// handoff starts, followed by the private environment file's path, in
// place of EnvHandOffEnv: a task's action cannot carry an environment
// variable
// (docs/decisions/0032-MADR-task-scheduler-registration-and-one-shot-handoff.md
// F11, D6-A). LoadHandOffEnvContext reads it only at os.Args[1].
const ArgHandOffEnv = "--selfupdate-handoff-env="

// maxHandOffEnv bounds an environment file.
const maxHandOffEnv = 1 << 20

// WriteHandOffEnv writes vars ("NAME=value") to a new file, handoff-<id>.env,
// of mode 0600 in dir, which it makes with mode 0700. On Windows, where
// the mode is not enforced, it gives dir a protected access list of the
// current user, SYSTEM and Administrators, which the file inherits:
// an inherited one can grant others access
// (docs/decisions/0032-MADR-task-scheduler-registration-and-one-shot-handoff.md
// Amendment A1, P1-8). It returns the file's path. The detached run
// applies it with LoadHandOffEnv. It takes no context: it writes one small
// local file (0025-MADR D3).
func WriteHandOffEnv(dir, id string, vars []string) (string, error) {
	if !filepath.IsAbs(dir) || strings.ContainsAny(id, `/\`) || id == "" {
		return "", fmt.Errorf("selfupdate: service: environment file %q in %q is not a plain name in an absolute directory", id, dir)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("selfupdate: service: %s is not a directory", dir)
	}
	if info.Mode().Perm() != 0o700 {
		if err := os.Chmod(dir, 0o700); err != nil { //nolint:gosec // a directory: 0700 is the private mode
			return "", err
		}
	}
	if err := protectDir(dir); err != nil {
		return "", fmt.Errorf("selfupdate: service: make %s private: %w", dir, err)
	}
	body, err := json.Marshal(vars)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "handoff-"+id+".env")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // a fixed name in a private directory
	if err != nil {
		return "", err
	}
	if err := writeAndClose(f, body); err != nil {
		return "", errors.Join(err, os.Remove(path))
	}
	return path, nil
}

// LoadHandOffEnv applies a run's private environment file, then deletes
// it, in this order:
//
//  1. when os.Args[1] is ArgHandOffEnv and a path, as a Task Scheduler
//     handoff starts its run, it removes that argument from os.Args and
//     applies the file it names;
//  2. it calls HandOffHop: in a hop it starts the real run and exits, so
//     a hop variable step 1 applied takes effect;
//  3. it applies the file EnvHandOffEnv names, then unsets the variable.
//
// With neither, it does nothing. The file must be a regular file of mode
// 0600 (not checked on Windows), at an absolute, clean path. ReportFunc
// calls it; a program that reads its arguments or its environment before
// it calls ReportFunc calls it first. It is LoadHandOffEnvContext with
// context.Background().
func LoadHandOffEnv() error {
	return LoadHandOffEnvContext(context.Background())
}

// LoadHandOffEnvContext is LoadHandOffEnv with ctx bounding
// HandOffHopContext's wait for the hop
// (docs/decisions/0025-MADR-align-with-go-sdk-design-practices.md D3).
func LoadHandOffEnvContext(ctx context.Context) error {
	if len(os.Args) > 1 && strings.HasPrefix(os.Args[1], ArgHandOffEnv) {
		path := strings.TrimPrefix(os.Args[1], ArgHandOffEnv)
		os.Args = append(os.Args[:1:1], os.Args[2:]...)
		if err := applyHandOffEnv(path); err != nil {
			return err
		}
	}
	if err := HandOffHopContext(ctx); err != nil {
		return err
	}
	path := os.Getenv(EnvHandOffEnv)
	if path == "" {
		return nil
	}
	return applyHandOffEnv(path)
}

// applyHandOffEnv sets the variables of the environment file at path, then
// deletes it and unsets EnvHandOffEnv.
func applyHandOffEnv(path string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return fmt.Errorf("selfupdate: service: environment file path %q is not absolute and clean", path)
	}
	info, err := os.Lstat(path) //nolint:gosec // checked above: absolute and clean
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || (runtime.GOOS != goosWindows && info.Mode().Perm() != 0o600) {
		return fmt.Errorf("selfupdate: service: environment file %s is not a private regular file", path)
	}
	f, err := os.Open(path) //nolint:gosec // checked above: absolute, clean, private, regular
	if err != nil {
		return err
	}
	body, rerr := io.ReadAll(io.LimitReader(f, maxHandOffEnv+1))
	if err := errors.Join(rerr, f.Close()); err != nil {
		return err
	}
	if len(body) > maxHandOffEnv {
		return errors.New("selfupdate: service: environment file is too large")
	}
	var vars []string
	if err := json.Unmarshal(body, &vars); err != nil {
		return fmt.Errorf("selfupdate: service: malformed environment file: %w", err)
	}
	for _, kv := range vars {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k == "" || k == EnvHandOffEnv {
			continue
		}
		if err := os.Setenv(k, v); err != nil {
			return err
		}
	}
	return errors.Join(os.Remove(path), os.Unsetenv(EnvHandOffEnv)) //nolint:gosec // checked above: absolute, clean, private, regular
}
