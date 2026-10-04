package service

import (
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

// maxHandOffEnv bounds an environment file.
const maxHandOffEnv = 1 << 20

// WriteHandOffEnv writes vars ("NAME=value") to a new file, handoff-<id>.env,
// of mode 0600 in dir, which it makes with mode 0700. It returns the file's
// path. The detached run applies it with LoadHandOffEnv.
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

// LoadHandOffEnv applies the environment file EnvHandOffEnv names, then
// deletes it and unsets the variable. Without the variable it does
// nothing. The file must be a regular file of mode 0600 (not checked on
// Windows), at an absolute, clean path. ReportFunc calls it; a program that
// reads its environment before it calls ReportFunc calls it first.
//
// It calls HandOffHop first: in a hop it starts the real run and exits.
func LoadHandOffEnv() error {
	HandOffHop()
	path := os.Getenv(EnvHandOffEnv)
	if path == "" {
		return nil
	}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return fmt.Errorf("selfupdate: service: environment file path %q is not absolute and clean", path)
	}
	info, err := os.Lstat(path) //nolint:gosec // checked above: absolute and clean
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o600) {
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
