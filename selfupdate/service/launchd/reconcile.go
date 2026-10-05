package launchd

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// PlistBackup is the ReconcileResult.State of a rewrite: the plist as it
// was, which Restore puts back.
type PlistBackup struct {
	// Path is the plist.
	Path string `json:"path"`
	// Previous is its content before the rewrite.
	Previous []byte `json:"previous"`
	// Mode, UID and GID are its mode and owner, which launchd checks.
	Mode fs.FileMode `json:"mode"`
	UID  int         `json:"uid"`
	GID  int         `json:"gid"`
}

// Reconcile implements selfupdate.Reconciler. The binary is replaced by
// rename at the same path, so the plist already names it: Reconcile checks
// Program, or else ProgramArguments.0, and changes nothing (0011-MADR §5).
// A plist that runs another binary is an error, unless Options.RewritePath
// allows rewriting it. Stop booted the job out and Start bootstraps it, so
// launchd reads the rewritten plist with no extra reload.
func (j *Job) Reconcile(ctx context.Context, _ string, executable string) (selfupdate.ReconcileResult, error) {
	key, current, err := j.program(ctx)
	if err != nil {
		return selfupdate.ReconcileResult{}, err
	}
	if service.SameExecutable(current, executable) {
		return selfupdate.ReconcileResult{Detail: j.o.Plist + " runs " + current}, nil
	}
	if !j.o.RewritePath {
		return selfupdate.ReconcileResult{}, fmt.Errorf(
			"selfupdate: launchd: %s runs %s, not %s; Options.RewritePath allows rewriting it", j.o.Plist, current, executable)
	}
	return j.rewrite(ctx, key, executable)
}

// program reads the plist's program: Program, or else ProgramArguments.0,
// as launchd.plist(5) resolves it.
func (j *Job) program(ctx context.Context) (key, path string, err error) {
	for _, key := range []string{"Program", "ProgramArguments.0"} {
		out, err := j.o.Runner.Run(ctx, service.Command{
			Path: j.plutil, Args: []string{"-extract", key, "raw", "-o", "-", "--", j.o.Plist}, Env: env,
		})
		if err != nil {
			return "", "", err
		}
		if out.ExitCode == 0 {
			// raw ends the value with one newline; a path may hold spaces.
			if v := strings.TrimSuffix(string(out.Stdout), "\n"); v != "" {
				return key, v, nil
			}
		}
	}
	return "", "", fmt.Errorf("selfupdate: launchd: %s has neither Program nor ProgramArguments", j.o.Plist)
}

// rewrite replaces key with executable in a copy of the plist, lints it,
// gives it the original's owner and mode, and renames it over the plist.
func (j *Job) rewrite(ctx context.Context, key, executable string) (selfupdate.ReconcileResult, error) {
	info, err := os.Lstat(j.o.Plist)
	if err != nil {
		return selfupdate.ReconcileResult{}, err
	}
	prev, err := os.ReadFile(j.o.Plist)
	if err != nil {
		return selfupdate.ReconcileResult{}, err
	}
	backup := PlistBackup{Path: j.o.Plist, Previous: prev, Mode: info.Mode().Perm()}
	backup.UID, backup.GID = fileOwner(info)
	tmp, err := writeTemp(j.o.Plist, prev, backup)
	if err != nil {
		return selfupdate.ReconcileResult{}, err
	}
	for _, args := range [][]string{
		{"-replace", key, "-string", executable, "--", tmp},
		{"-lint", "--", tmp},
	} {
		out, err := j.o.Runner.Run(ctx, service.Command{Path: j.plutil, Args: args, Env: env})
		if err == nil && out.ExitCode != 0 {
			err = fmt.Errorf("selfupdate: launchd: plutil %s: exit %d: %s", args[0], out.ExitCode, strings.TrimSpace(string(out.Stderr)))
		}
		if err != nil {
			return selfupdate.ReconcileResult{}, errors.Join(err, os.Remove(tmp))
		}
	}
	if err := os.Rename(tmp, j.o.Plist); err != nil {
		return selfupdate.ReconcileResult{}, errors.Join(err, os.Remove(tmp))
	}
	return selfupdate.ReconcileResult{Changed: true, Detail: "rewrote " + key + " in " + j.o.Plist, State: backup}, nil
}

// Restore implements selfupdate.Reconciler: it puts the plist back as it
// was, owner and mode included.
func (j *Job) Restore(_ context.Context, _ string, receipt selfupdate.ReconcileResult) error {
	if !receipt.Changed {
		return nil
	}
	b, ok := receipt.State.(PlistBackup)
	if !ok {
		return errors.New("selfupdate: launchd: the receipt is not a plist backup")
	}
	tmp, err := writeTemp(b.Path, b.Previous, b)
	if err != nil {
		return err
	}
	if err := os.Rename(tmp, b.Path); err != nil {
		return errors.Join(err, os.Remove(tmp))
	}
	return nil
}

// writeTemp writes body beside path, with b's mode and, when this process
// may set it, owner.
func writeTemp(path string, body []byte, b PlistBackup) (string, error) {
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return "", err
	}
	name := f.Name()
	if _, err := f.Write(body); err != nil {
		return "", errors.Join(err, f.Close(), os.Remove(name))
	}
	if err := f.Close(); err != nil {
		return "", errors.Join(err, os.Remove(name))
	}
	if err := os.Chmod(name, b.Mode); err != nil {
		return "", errors.Join(err, os.Remove(name))
	}
	if err := os.Lchown(name, b.UID, b.GID); err != nil && !errors.Is(err, fs.ErrPermission) {
		return "", errors.Join(err, os.Remove(name))
	}
	return name, nil
}
