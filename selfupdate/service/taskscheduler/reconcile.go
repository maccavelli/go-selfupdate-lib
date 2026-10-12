package taskscheduler

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// DefinitionBackup is the selfupdate.ReconcileResult.State of a rewrite: the task's
// definition as Task Scheduler returned it before the rewrite, which
// Restore registers again.
type DefinitionBackup struct {
	// Folder and Name are the task's.
	Folder string `json:"folder"`
	Name   string `json:"name"`
	// XML is the definition (IRegisteredTask::get_Xml), kept as a string:
	// it never passes through an 8-bit pipe, so a non-ASCII path survives.
	XML string `json:"xml"`
}

// Reconcile implements selfupdate.Reconciler. The binary is replaced by
// rename at the same path, so the task already names it: Reconcile
// compares the one Exec action's Path with executable, cleaned and
// case-insensitively, then by file identity, and changes nothing. A task
// with any other action, or more than one, is refused. A task that runs
// another binary is an error, unless Options.RewritePath allows pointing it
// at executable. The rewrite sets the Path alone on the definition and
// registers it again, so every other element stays as Task Scheduler
// returned it; it is refused for a task whose principal does not log on
// with an interactive token, the only kind measured (0021-MADR D5;
// Amendment A1, Q6).
func (t *Task) Reconcile(_ context.Context, product, executable string) (_ selfupdate.ReconcileResult, err error) {
	name, path, err := t.task(product)
	if err != nil {
		return selfupdate.ReconcileResult{}, err
	}
	r, err := t.open(name, path)
	if err != nil {
		return selfupdate.ReconcileResult{}, err
	}
	defer func() { err = errors.Join(err, r.close()) }()
	actions, err := r.actions()
	if err != nil {
		return selfupdate.ReconcileResult{}, mapError("query the actions of", path, err)
	}
	if len(actions) != 1 || actions[0].Type != actionExec {
		return selfupdate.ReconcileResult{}, fmt.Errorf("selfupdate: taskscheduler: %s has %s; the backend manages a task with exactly one Exec action", path, describeActions(actions))
	}
	current := actions[0].Path
	quoted := strings.HasPrefix(current, `"`) && strings.HasSuffix(current, `"`) && len(current) > 1
	if quoted {
		current = current[1 : len(current)-1]
	}
	if samePath(current, executable) {
		return selfupdate.ReconcileResult{Detail: path + " already runs " + executable}, nil
	}
	if !t.o.RewritePath {
		return selfupdate.ReconcileResult{}, fmt.Errorf("selfupdate: taskscheduler: %s runs %s, not %s; Options.RewritePath allows rewriting it", path, current, executable)
	}
	logon, err := r.logonType()
	if err != nil {
		return selfupdate.ReconcileResult{}, mapError("query the principal of", path, err)
	}
	if logon != logonInteractiveToken {
		return selfupdate.ReconcileResult{}, fmt.Errorf("selfupdate: taskscheduler: %s logs on with %s; a rewrite is measured only for an interactive token (0021-MADR Amendment A1, Q6)", path, logonName(logon))
	}
	if strings.Contains(executable, `"`) {
		return selfupdate.ReconcileResult{}, fmt.Errorf("selfupdate: taskscheduler: %q holds a quote, which an Exec action's path cannot carry", executable)
	}
	before, err := r.xml()
	if err != nil {
		return selfupdate.ReconcileResult{}, mapError("read the definition of", path, err)
	}
	next := executable
	if quoted {
		next = `"` + executable + `"`
	}
	if err := r.setPath(next); err != nil {
		return selfupdate.ReconcileResult{}, mapError("change the action of", path, err)
	}
	return selfupdate.ReconcileResult{
		Changed: true,
		Detail:  path + " now runs " + executable,
		State:   DefinitionBackup{Folder: t.o.Folder, Name: name, XML: before},
	}, nil
}

// Restore registers again the definition a rewrite replaced. It runs in
// the process that ran the update, so the code that undoes the rewrite is
// the code that made it (0021-MADR D5).
func (t *Task) Restore(_ context.Context, _ string, receipt selfupdate.ReconcileResult) error {
	if !receipt.Changed {
		return nil
	}
	b, ok := receipt.State.(DefinitionBackup)
	if !ok {
		return errors.New("selfupdate: taskscheduler: the receipt is not a definition backup")
	}
	path := `\` + b.Name
	if b.Folder != `\` {
		path = b.Folder + `\` + b.Name
	}
	r, err := t.s.open(b.Folder, b.Name)
	if err != nil {
		return mapError("open", path, err)
	}
	return errors.Join(mapErrorOrNil("restore the definition of", path, r.restore(b.XML)), r.close())
}

func describeActions(actions []action) string {
	switch {
	case len(actions) == 0:
		return "no action"
	case len(actions) > 1:
		return fmt.Sprintf("%d actions", len(actions))
	}
	return fmt.Sprintf("an action of type %d, not Exec", actions[0].Type)
}

// samePath compares two Windows paths, cleaned and case-insensitively, then
// by file identity, which matches an 8.3 short name: scm's rule
// (service.SameExecutable, 0011-MADR amendment A5).
func samePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b)) || service.SameExecutable(a, b)
}

func mapErrorOrNil(step, path string, err error) error {
	if err == nil {
		return nil
	}
	return mapError(step, path, err)
}
