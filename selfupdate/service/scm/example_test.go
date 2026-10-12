package scm_test

import (
	"fmt"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service/scm"
)

// A program run as a Windows service hands the service to the managed
// installer, as both its Lifecycle and its Reconciler: Install stops the
// service, replaces the binary, points the service at it if it moved,
// starts it and waits for it to be healthy, and rolls back if any step
// fails. New returns service.ErrUnsupported off Windows.
func ExampleNew() {
	svc, err := scm.New(scm.Options{Name: "relay", RewritePath: true})
	if err != nil {
		fmt.Println(err)
		return
	}
	inner, err := selfupdate.NewStandaloneInstaller(selfupdate.InstallOptions{})
	if err != nil {
		fmt.Println(err)
		return
	}
	managed, err := selfupdate.NewManagedInstaller(inner, svc, svc)
	if err != nil {
		fmt.Println(err)
		return
	}
	_ = managed // Config.Installer
}
