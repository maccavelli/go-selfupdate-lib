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
// allows rewriting it. launchd keeps the definition it loaded: a job Stop
// booted out reads the rewritten plist at Start's bootstrap, a job loaded
// but not running is reloaded here, and a running one is refused
// (0015-MADR D3).
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
// launchd keeps a loaded job's definition, so a job loaded but not running,
// which Stop left loaded, is reloaded: bootout, the wait, bootstrap. A job
// that is running is refused before anything changes (0015-MADR D3).
func (j *Job) rewrite(ctx context.Context, key, executable string) (selfupdate.ReconcileResult, error) {
	s, err := j.probe(ctx)
	if err != nil {
		return selfupdate.ReconcileResult{}, err
	}
	if s.loaded && s.running {
		return selfupdate.ReconcileResult{}, fmt.Errorf(
			"selfupdate: launchd: %s is running; stop the job first, so its rewritten plist can be loaded", j.target())
	}
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
	result := selfupdate.ReconcileResult{Changed: true, Detail: "rewrote " + key + " in " + j.o.Plist, State: backup}
	if s.loaded {
		// The receipt comes back with a failed reload, so recovery puts
		// the plist back.
		if err := j.reload(ctx, s.pid); err != nil {
			return result, err
		}
		result.Detail += ", and reloaded the job"
	}
	return result, nil
}

// Restore implements selfupdate.Reconciler: it puts the plist back as it
// was, owner and mode included, and reloads a job that is loaded but not
// running, as rewrite does (0015-MADR D3).
func (j *Job) Restore(ctx context.Context, _ string, receipt selfupdate.ReconcileResult) error {
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
	s, err := j.probe(ctx)
	if err != nil {
		return err
	}
	switch {
	case s.loaded && s.running:
		return fmt.Errorf("selfupdate: launchd: %s is running; the restored plist is read once it is booted out", j.target())
	case s.loaded:
		return j.reload(ctx, s.pid)
	}
	return nil
}

// reload makes launchd read the plist again: bootout, the stop wait, then
// bootstrap, without kickstart. A RunAtLoad job starts at the bootstrap;
// Start does not take that process for the one before the update.
func (j *Job) reload(ctx context.Context, pid int) error {
	bound := j.stopBound(ctx)
	out, err := j.launchctlRun(ctx, "bootout", j.target())
	if err != nil {
		return err
	}
	switch out.ExitCode {
	case 0, exitNotFound, exitNoProcess, exitInProgress:
	default:
		return launchctlError("bootout", j.target(), out)
	}
	if err := j.waitGone(ctx, pid, bound); err != nil {
		return err
	}
	if err := j.bootstrap(ctx); err != nil {
		return err
	}
	j.mu.Lock()
	j.reloaded = true
	j.mu.Unlock()
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
