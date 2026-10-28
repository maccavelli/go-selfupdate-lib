package taskscheduler

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/maccavelli/go-selfupdate-lib/internal/winproc"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// handOffName is a one-shot handoff task's name: <name>.selfupdate.<id>,
// in the service task's folder, as launchd's one-shot job is
// <label>.selfupdate.<id> (0032-MADR D8-A).
func handOffName(name, id string) string { return name + ".selfupdate." + id }

// fullPath is a task's path from its folder and name.
func fullPath(folder, name string) string {
	if folder == `\` {
		return `\` + name
	}
	return folder + `\` + name
}

// handOffDir is Options.HandOffDir, or %LOCALAPPDATA%\selfupdate\handoff.
func (t *Task) handOffDir() (string, error) {
	if t.o.HandOffDir != "" {
		return t.o.HandOffDir, nil
	}
	local, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("selfupdate: taskscheduler: the handoff directory: %w", err)
	}
	return filepath.Join(local, "selfupdate", "handoff"), nil
}

// detachTask is Detach by a one-shot task of the service task's own user,
// whose process Task Scheduler starts, so it is in none of this process's
// jobs (0032-MADR D5-A, D8-A; Amendment A1, P2-2 and P2-3). The task runs
// this program with service.ArgHandOffEnv and the private environment
// file first, because a task cannot carry an environment variable
// (D6-A), and the file asks for a hop, so the real run has no console
// window (D7-A; P2-4).
func (t *Task) detachTask(ctx context.Context, spec service.HandOff) (service.Detached, error) {
	if t.o.Name == "" {
		return service.Detached{}, errors.New("selfupdate: taskscheduler: the handoff needs Options.Name")
	}
	name, path, err := t.task("")
	if err != nil {
		return service.Detached{}, err
	}
	// Best-effort: a finished task left behind is harmless, and must not
	// stop a new handoff; only a cancelled context does.
	if err := t.CleanupHandOffs(ctx); err != nil && ctx.Err() != nil {
		return service.Detached{}, ctx.Err()
	}
	if err := winproc.CheckProgram(spec.Executable); err != nil {
		return service.Detached{}, err
	}
	if strings.Contains(spec.Executable, `"`) {
		return service.Detached{}, fmt.Errorf("selfupdate: taskscheduler: %q holds a quote, which an Exec action's path cannot carry", spec.Executable)
	}
	p, err := t.principalOf(name, path)
	if err != nil {
		return service.Detached{}, err
	}
	if p.LogonType != logonInteractiveToken {
		return service.Detached{}, fmt.Errorf("selfupdate: taskscheduler: %s logs on with %s; the one-shot handoff is measured only for an interactive token, and Options.HopDetach starts a hop instead (0032-MADR D8-A)", path, logonName(p.LogonType))
	}
	dir, err := t.handOffDir()
	if err != nil {
		return service.Detached{}, err
	}
	// The scrubbed environment, then spec.Env, then the hop variable
	// (0033-MADR D3-A, D5-A).
	env := append(append(service.ScrubManagerEnv(os.Environ()), spec.Env...), service.EnvHandOffHop+"=1")
	file, err := service.WriteHandOffEnv(dir, spec.ID, env)
	if err != nil {
		return service.Detached{}, err
	}
	oneShot := handOffName(name, spec.ID)
	oneShotPath := fullPath(t.o.Folder, oneShot)
	args := composeArgs(append([]string{service.ArgHandOffEnv + file}, spec.Args...))
	def := oneShotXML(p, "selfupdate handoff "+spec.ID+" for "+path, spec.Executable, args, os.TempDir())
	if err := t.s.register(t.o.Folder, oneShot, def, taskCreate, logonInteractiveToken); err != nil {
		return service.Detached{}, errors.Join(mapError("register the handoff task", oneShotPath, err), os.Remove(file))
	}
	if err := t.startOneShot(ctx, oneShot, oneShotPath); err != nil {
		return service.Detached{}, errors.Join(err, mapErrorOrNil("delete the handoff task", oneShotPath, t.s.deleteTask(t.o.Folder, oneShot)), removeIfExists(file))
	}
	return service.Detached{ID: spec.ID, Where: "task " + oneShotPath, ResultPath: spec.ResultPath}, nil
}

// principalOf reads the service task's principal.
func (t *Task) principalOf(name, path string) (_ principal, err error) {
	r, err := t.open(name, path)
	if err != nil {
		return principal{}, err
	}
	defer func() { err = errors.Join(err, r.close()) }()
	p, err := r.principal()
	if err != nil {
		return principal{}, mapError("query the principal of", path, err)
	}
	return p, nil
}

// startOneShot runs the one-shot task and waits until it has started: an
// instance is listed, or its last result is no longer "has not run". Its
// last run time is not used: it has whole seconds and reads just below the
// second it names (0032-MADR Amendment A1, P2-7). The wait is bounded by
// ctx and Options.Poll's timeout.
func (t *Task) startOneShot(ctx context.Context, name, path string) (err error) {
	r, err := t.open(name, path)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, r.close()) }()
	if err := r.run(); err != nil {
		return mapError("run the handoff task", path, err)
	}
	ctx, cancel := t.deadline(ctx)
	defer cancel()
	for {
		pids, err := r.instances()
		if err != nil {
			return mapError("list the instances of", path, err)
		}
		last, err := r.lastRun()
		if err != nil {
			return mapError("query the last run of", path, err)
		}
		if len(pids) > 0 || last.Result != hasNotRun {
			return nil
		}
		if err := sleepCtx(ctx, t.interval()); err != nil {
			return fmt.Errorf("%w: selfupdate: taskscheduler: %s did not start: %w", service.ErrTimeout, path, err)
		}
	}
}

// CleanupHandOffs deletes the service task's finished one-shot handoff
// tasks, <Options.Name>.selfupdate.<id> in Options.Folder, and the
// environment file left for each in Options.HandOffDir. A task that is
// running, or has not run, is left alone. Deleting a task does not end a
// run it started (0032-MADR Amendment A1, P1-5). Detach calls it; a
// program may call it at start-up. It checks ctx before it starts and
// ignores it after.
//
// Errors, matched with errors.Is: service.ErrPermission.
func (t *Task) CleanupHandOffs(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if t.o.Name == "" {
		return errors.New("selfupdate: taskscheduler: the handoff needs Options.Name")
	}
	tasks, err := t.s.list(t.o.Folder)
	if errors.Is(err, hrFileNotFound) {
		return nil
	}
	if err != nil {
		return mapError("list the folder", t.o.Folder, err)
	}
	prefix := strings.ToLower(handOffName(t.o.Name, ""))
	dir, dirErr := t.handOffDir()
	var errs []error
	for _, task := range tasks {
		if !strings.HasPrefix(strings.ToLower(task.Name), prefix) || task.State == stateRunning || task.LastResult == hasNotRun {
			continue
		}
		path := fullPath(t.o.Folder, task.Name)
		if err := t.s.deleteTask(t.o.Folder, task.Name); err != nil && !errors.Is(err, hrFileNotFound) {
			errs = append(errs, mapError("delete", path, err))
			continue
		}
		if id := task.Name[len(prefix):]; dirErr == nil && id != "" && !strings.ContainsAny(id, `/\`) {
			errs = append(errs, removeIfExists(filepath.Join(dir, "handoff-"+id+".env")))
		}
	}
	return errors.Join(errs...)
}

func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// oneShotXML is 0032-MADR D8-A's definition: the service task's principal,
// no trigger, one Exec action, and every setting it depends on written
// out, since Task Scheduler's defaults would refuse to start on battery
// (0032-MADR F15).
func oneShotXML(p principal, description, executable, args, workDir string) string {
	esc := func(s string) string {
		var b strings.Builder
		_ = xml.EscapeText(&b, []byte(s)) //nolint:errcheck // a strings.Builder does not fail
		return b.String()
	}
	user := ""
	if p.UserID != "" {
		user = "<UserId>" + esc(p.UserID) + "</UserId>"
	}
	level := "LeastPrivilege"
	if p.RunLevel == 1 {
		level = "HighestAvailable"
	}
	return `<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo><Description>` + esc(description) + `</Description></RegistrationInfo>
  <Principals><Principal id="Author">` + user + `<LogonType>InteractiveToken</LogonType><RunLevel>` + level + `</RunLevel></Principal></Principals>
  <Settings>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <Enabled>true</Enabled>
    <Hidden>false</Hidden>
  </Settings>
  <Actions Context="Author"><Exec><Command>` + esc(executable) + `</Command><Arguments>` + esc(args) + `</Arguments><WorkingDirectory>` + esc(workDir) + `</WorkingDirectory></Exec></Actions>
</Task>
`
}

// composeArgs joins arguments into an Exec action's Arguments, each quoted
// as CommandLineToArgvW reads it back. Every element is an argument, not a
// program name, so each is quoted by golang.org/x/sys/windows.EscapeArg's
// rules, which this follows; TestComposeArgsMatchesEscapeArg pins them on
// Windows.
func composeArgs(args []string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = escapeArg(a)
	}
	return strings.Join(quoted, " ")
}

// escapeArg is golang.org/x/sys/windows.EscapeArg: an argument with a
// space or tab is quoted, and a quote, with the backslashes before it, is
// escaped by backslashes.
func escapeArg(s string) string {
	if s == "" {
		return `""`
	}
	if !strings.ContainsAny(s, "\"\\ \t") {
		return s
	}
	hasSpace := strings.ContainsAny(s, " \t")
	var b strings.Builder
	if hasSpace {
		b.WriteByte('"')
	}
	slashes := 0
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '\\':
			slashes++
			b.WriteByte(c)
		case '"':
			for ; slashes > 0; slashes-- {
				b.WriteByte('\\')
			}
			b.WriteString(`\"`)
		default:
			slashes = 0
			b.WriteByte(c)
		}
	}
	if hasSpace {
		for ; slashes > 0; slashes-- {
			b.WriteByte('\\')
		}
		b.WriteByte('"')
	}
	return b.String()
}
