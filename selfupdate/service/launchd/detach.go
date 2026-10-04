package launchd

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// maxAncestors bounds the walk up this process's parents.
const maxAncestors = 64

// Inside implements service.Detacher: the job's process is in this
// process's group, which launchd reaps with the job (launchd.plist(5),
// AbandonProcessGroup), or is one of this process's ancestors
// (0011-MADR §3).
func (j *Job) Inside(ctx context.Context) (bool, error) {
	s, err := j.probe(ctx)
	if err != nil {
		return false, err
	}
	if s.pid <= 0 {
		return false, nil
	}
	if mine, theirs, ok := processGroups(s.pid); ok && mine == theirs {
		return true, nil
	}
	cur := os.Getpid()
	for range maxAncestors {
		if cur == s.pid {
			return true, nil
		}
		parent, err := j.parent(ctx, cur)
		if err != nil {
			return false, err
		}
		if parent <= 1 {
			return false, nil
		}
		cur = parent
	}
	return false, nil
}

// parent is pid's parent, from `ps -o ppid= -p <pid>`.
func (j *Job) parent(ctx context.Context, pid int) (int, error) {
	out, err := j.o.Runner.Run(ctx, service.Command{Path: j.ps, Args: []string{"-o", "ppid=", "-p", strconv.Itoa(pid)}, Env: env})
	if err != nil {
		return 0, err
	}
	if out.ExitCode != 0 {
		return 0, nil // the process has gone
	}
	return strconv.Atoi(strings.TrimSpace(string(out.Stdout)))
}

// handOffLabel is <label>.selfupdate.<id>.
func (j *Job) handOffLabel(id string) string { return j.o.Label + ".selfupdate." + id }

// jobDir is where the one-shot jobs' property lists and environment files
// go: Options.JobDir, or a directory launchd accepts for the domain.
func (j *Job) jobDir() (string, error) {
	if j.o.JobDir != "" {
		return j.o.JobDir, nil
	}
	if j.o.Domain.kind == kindSystem {
		return "/Library/Application Support/selfupdate/handoff", nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "Application Support", "selfupdate", "handoff"), nil
}

// Detach implements service.Detacher. It loads spec as a one-shot job of
// its own, <label>.selfupdate.<id>: a separate job is a separate process
// group, so booting the service out does not reap it (0011-MADR §9). The
// job's plist carries only the handoff variables; the rest of the
// environment goes in a private file the run loads (0011-MADR amendment
// A3).
func (j *Job) Detach(ctx context.Context, spec service.HandOff) (service.Detached, error) {
	dir, err := j.jobDir()
	if err != nil {
		return service.Detached{}, err
	}
	// Best-effort: a finished job left loaded is harmless, and must not
	// stop a new handoff; only a cancelled context does.
	if err := j.CleanupHandOffs(ctx); err != nil && ctx.Err() != nil {
		return service.Detached{}, ctx.Err()
	}
	envFile, err := service.WriteHandOffEnv(dir, spec.ID, append(os.Environ(), spec.Env...))
	if err != nil {
		return service.Detached{}, err
	}
	label := j.handOffLabel(spec.ID)
	jobEnv := map[string]string{service.EnvHandOffEnv: envFile}
	for _, kv := range spec.Env {
		k, v, _ := strings.Cut(kv, "=")
		if k == service.EnvHandOff || k == service.EnvHandOffResult {
			jobEnv[k] = v
		}
	}
	plist := filepath.Join(dir, label+".plist")
	body := oneShotPlist(label, append([]string{spec.Executable}, spec.Args...), jobEnv)
	if err := writeNew(plist, body); err != nil {
		return service.Detached{}, errors.Join(err, os.Remove(envFile))
	}
	out, err := j.launchctlRun(ctx, "bootstrap", j.o.Domain.String(), plist)
	if err == nil && out.ExitCode != 0 {
		err = launchctlError("bootstrap", plist, out)
	}
	if err != nil {
		return service.Detached{}, errors.Join(err, os.Remove(plist), os.Remove(envFile))
	}
	return service.Detached{ID: spec.ID, Where: "launchd job " + label, ResultPath: spec.ResultPath}, nil
}

// CleanupHandOffs boots out the job's finished one-shot handoff jobs and
// removes their property lists and any environment file left. A job that
// has not finished, running or still being spawned, is left alone. Detach
// calls it; a program may call it at
// start-up (0011-MADR amendment A3).
func (j *Job) CleanupHandOffs(ctx context.Context) error {
	dir, err := j.jobDir()
	if err != nil {
		return err
	}
	plists, err := filepath.Glob(filepath.Join(dir, j.o.Label+".selfupdate.*.plist"))
	if err != nil {
		return err
	}
	var errs []error
	for _, p := range plists {
		label := strings.TrimSuffix(filepath.Base(p), ".plist")
		id := strings.TrimPrefix(label, j.o.Label+".selfupdate.")
		target := j.o.Domain.String() + "/" + label
		out, err := j.launchctlRun(ctx, "print", target)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if out.ExitCode == 0 {
			// Only "not running" is finished. A job launchd is still
			// spawning shows "xpcproxy", and an unknown state is left alone.
			if _, st := printState(out.Stdout); st != "not running" {
				continue
			}
			if out, err := j.launchctlRun(ctx, "bootout", target); err != nil || out.ExitCode != 0 && out.ExitCode != exitNotFound {
				errs = append(errs, errors.Join(err, fmt.Errorf("bootout %s: exit %d", target, out.ExitCode)))
				continue
			}
		}
		errs = append(errs, removeIfExists(p), removeIfExists(filepath.Join(dir, "handoff-"+id+".env")))
	}
	return errors.Join(errs...)
}

// oneShotPlist is a job that runs argv once, at load, with env, outside
// any other job's process group, its output discarded.
func oneShotPlist(label string, argv []string, env map[string]string) []byte {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
`)
	str := func(s string) string { return "<string>" + escape(s) + "</string>" }
	fmt.Fprintf(&b, "\t<key>Label</key>\n\t%s\n", str(label))
	b.WriteString("\t<key>ProgramArguments</key>\n\t<array>\n")
	for _, a := range argv {
		fmt.Fprintf(&b, "\t\t%s\n", str(a))
	}
	b.WriteString("\t</array>\n\t<key>EnvironmentVariables</key>\n\t<dict>\n")
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, "\t\t<key>%s</key>\n\t\t%s\n", escape(k), str(env[k]))
	}
	b.WriteString("\t</dict>\n")
	for _, kv := range [][2]string{{"WorkingDirectory", "/"}, {"StandardOutPath", "/dev/null"}, {"StandardErrorPath", "/dev/null"}} {
		fmt.Fprintf(&b, "\t<key>%s</key>\n\t%s\n", kv[0], str(kv[1]))
	}
	b.WriteString("\t<key>RunAtLoad</key>\n\t<true/>\n\t<key>AbandonProcessGroup</key>\n\t<true/>\n</dict>\n</plist>\n")
	return b.Bytes()
}

// escape is s as XML character data.
func escape(s string) string {
	var e bytes.Buffer
	if err := xml.EscapeText(&e, []byte(s)); err != nil {
		return "" // a bytes.Buffer does not fail
	}
	return e.String()
}

// writeNew writes body to a new file, mode 0600: launchd refuses a plist
// others may write.
func writeNew(path string, body []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // a fixed name in the job directory
	if err != nil {
		return err
	}
	if _, err := f.Write(body); err != nil {
		return errors.Join(err, f.Close(), os.Remove(path))
	}
	return f.Close()
}

func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
