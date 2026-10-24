package taskscheduler

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
)

// Register registers definition, task XML the program rendered, as the task
// Options.Name and Options.Folder name for product, creating the task or
// replacing its definition (TASK_CREATE_OR_UPDATE), and creating the
// folder, with every missing level, when it does not exist
// (0032-MADR D2-A).
//
// The definition registers with no user, so Task Scheduler gives an
// interactive-token principal this process's account. Its principal's
// logon type, InteractiveToken when it names none, is passed to Task
// Scheduler. A logon type a user cannot register for themselves without
// credentials is refused before Task Scheduler is called: Password,
// InteractiveTokenOrPassword, S4U, a GroupId, or a service account
// (0032-MADR Amendment A1, P1-2). A leading XML declaration is removed,
// because Task Scheduler refuses one that says UTF-8 and the definition
// travels as UTF-16 whatever it says (Amendment A1, P1-6).
//
// The definition sets the task's switch: one that leaves Settings/Enabled
// out, or sets it true, enables a disabled task, including one Stop or
// Disable disabled
// (docs/decisions/0034-MADR-export-task-scheduler-enable-and-disable.md
// Amendment A1, P2). A setup that keeps a task disabled calls Disable
// after Register.
//
// It checks ctx before it starts and ignores it after: each Task
// Scheduler call takes no context. Errors, matched with errors.Is:
// service.ErrPermission when Task Scheduler denies the registration.
func (t *Task) Register(ctx context.Context, product, definition string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	name, path, err := t.task(product)
	if err != nil {
		return err
	}
	body := stripDeclaration(definition)
	logon, err := registrationLogon(body)
	if err != nil {
		return fmt.Errorf("selfupdate: taskscheduler: register %s: %w", path, err)
	}
	err = t.s.register(t.o.Folder, name, body, taskCreateOrUpdate, logon)
	if errors.Is(err, hrFileNotFound) && t.o.Folder != `\` {
		if ferr := t.s.createFolder(t.o.Folder); ferr != nil && !errors.Is(ferr, hrAlreadyExists) {
			return mapError("create the folder of", path, ferr)
		}
		err = t.s.register(t.o.Folder, name, body, taskCreateOrUpdate, logon)
	}
	return mapErrorOrNil("register", path, err)
}

// Definition returns the task's registered definition, as Task Scheduler
// stores it (IRegisteredTask::get_Xml), which differs in form from the
// registered text: compare them with DiffDefinitions (0032-MADR D3-A).
// It checks ctx before it starts and ignores it after.
//
// Errors, matched with errors.Is: service.ErrNotInstalled when the task
// does not exist; service.ErrPermission.
func (t *Task) Definition(ctx context.Context, product string) (_ string, err error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	name, path, err := t.task(product)
	if err != nil {
		return "", err
	}
	r, err := t.open(name, path)
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, r.close()) }()
	x, err := r.xml()
	if err != nil {
		return "", mapError("read the definition of", path, err)
	}
	return x, nil
}

// Delete deletes the task. A task that does not exist, or whose folder
// does not exist, is not an error, so removal is idempotent. It does not
// stop a running instance, which outlives the deletion (0032-MADR
// Amendment A1, P1-5): call Stop first. It checks ctx before it starts
// and ignores it after.
//
// Errors, matched with errors.Is: service.ErrPermission.
func (t *Task) Delete(ctx context.Context, product string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	name, path, err := t.task(product)
	if err != nil {
		return err
	}
	if err := t.s.deleteTask(t.o.Folder, name); err != nil && !errors.Is(err, hrFileNotFound) {
		return mapError("delete", path, err)
	}
	return nil
}

// stripDeclaration removes a leading XML declaration, and any space
// before it.
func stripDeclaration(s string) string {
	t := strings.TrimLeft(s, " \t\r\n\ufeff")
	if !strings.HasPrefix(t, "<?xml") {
		return s
	}
	if i := strings.Index(t, "?>"); i >= 0 {
		return strings.TrimLeft(t[i+2:], " \t\r\n")
	}
	return s
}

// definitionPrincipal is the part of a definition registrationLogon reads.
type definitionPrincipal struct {
	Principals struct {
		Principal []struct {
			UserID    string `xml:"UserId"`
			GroupID   string `xml:"GroupId"`
			LogonType string `xml:"LogonType"`
		} `xml:"Principal"`
	} `xml:"Principals"`
}

// serviceAccounts are the accounts a user cannot register a task for:
// LocalSystem, LocalService and NetworkService, by SID and by name.
var serviceAccounts = []string{
	"S-1-5-18", "S-1-5-19", "S-1-5-20",
	"SYSTEM", `NT AUTHORITY\SYSTEM`, "LocalSystem",
	"LOCAL SERVICE", `NT AUTHORITY\LOCAL SERVICE`, "LocalService",
	"NETWORK SERVICE", `NT AUTHORITY\NETWORK SERVICE`, "NetworkService",
}

// registrationLogon reads the definition's logon type, refusing those a
// user cannot register for themselves without credentials.
func registrationLogon(body string) (int32, error) {
	var d definitionPrincipal
	if err := xml.Unmarshal([]byte(body), &d); err != nil {
		return 0, fmt.Errorf("the definition is not readable XML: %w", err)
	}
	if len(d.Principals.Principal) == 0 {
		return logonInteractiveToken, nil
	}
	p := d.Principals.Principal[0]
	if strings.TrimSpace(p.GroupID) != "" {
		return 0, errors.New("the principal names a group, which needs an administrator")
	}
	for _, a := range serviceAccounts {
		if strings.EqualFold(strings.TrimSpace(p.UserID), a) {
			return 0, fmt.Errorf("the principal is the service account %s, which needs an administrator", p.UserID)
		}
	}
	switch strings.TrimSpace(p.LogonType) {
	case "", "InteractiveToken":
		return logonInteractiveToken, nil
	case "S4U":
		return 0, errors.New("logon type S4U is refused to a user who is not an administrator (0032-MADR Amendment A1, P1-2)")
	}
	return 0, fmt.Errorf("logon type %s needs a password, which this package does not hold", p.LogonType)
}
