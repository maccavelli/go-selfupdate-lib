package taskscheduler_test

import (
	"fmt"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service/taskscheduler"
)

// A program that Task Scheduler starts at logon hands its task to the
// managed installer, as both its Lifecycle and its Reconciler: Install
// disables and ends the task, replaces the binary, points the task at it
// if it moved, enables and runs it and waits for it to be healthy, and
// rolls back if any step fails. New returns service.ErrUnsupported off
// Windows.
func ExampleNew() {
	task, err := taskscheduler.New(taskscheduler.Options{Name: "relay", RewritePath: true})
	if err != nil {
		fmt.Println(err)
		return
	}
	inner, err := selfupdate.NewStandaloneInstaller(selfupdate.InstallOptions{})
	if err != nil {
		fmt.Println(err)
		return
	}
	managed, err := selfupdate.NewManagedInstaller(inner, task, task)
	if err != nil {
		fmt.Println(err)
		return
	}
	_ = managed // Config.Installer
}
