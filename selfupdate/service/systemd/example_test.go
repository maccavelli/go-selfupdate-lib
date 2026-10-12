package systemd_test

import (
	"fmt"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service/systemd"
)

// A program run as a systemd unit hands the unit to the managed installer,
// as both its Lifecycle and its Reconciler: Install stops the unit,
// replaces the binary, points the unit at it if it moved, starts the unit
// and waits for it to be healthy, and rolls back if any step fails. New
// returns service.ErrUnsupported off Linux.
func ExampleNew() {
	unit, err := systemd.New(systemd.Options{Unit: "relay.service", Scope: systemd.System, RewritePath: true})
	if err != nil {
		fmt.Println(err)
		return
	}
	inner, err := selfupdate.NewStandaloneInstaller(selfupdate.InstallOptions{})
	if err != nil {
		fmt.Println(err)
		return
	}
	managed, err := selfupdate.NewManagedInstaller(inner, unit, unit)
	if err != nil {
		fmt.Println(err)
		return
	}
	_ = managed // Config.Installer
}
