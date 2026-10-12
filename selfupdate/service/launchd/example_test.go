package launchd_test

import (
	"fmt"
	"os"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service/launchd"
)

// A program run as a per-user launchd agent hands the job to the managed
// installer, as both its Lifecycle and its Reconciler: Install boots the
// job out, replaces the binary, points the plist at it if it moved, boots
// it in and waits for it to be healthy, and rolls back if any step fails.
// New returns service.ErrUnsupported off macOS.
func ExampleNew() {
	job, err := launchd.New(launchd.Options{Label: "com.example.relay", Domain: launchd.GUI(os.Getuid()), RewritePath: true})
	if err != nil {
		fmt.Println(err)
		return
	}
	inner, err := selfupdate.NewStandaloneInstaller(selfupdate.InstallOptions{})
	if err != nil {
		fmt.Println(err)
		return
	}
	managed, err := selfupdate.NewManagedInstaller(inner, job, job)
	if err != nil {
		fmt.Println(err)
		return
	}
	_ = managed // Config.Installer
}
