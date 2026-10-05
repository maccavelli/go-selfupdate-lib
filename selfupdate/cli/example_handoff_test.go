package cli_test

import (
	"context"
	"os"

	"github.com/maccavelli/go-selfupdate-lib/buildinfo"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/cli"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service/systemd"
)

// A program that runs as relay.service hands an update started inside the
// service, such as by an agent the service spawned, to a transient unit,
// and that detached run reports its outcome to a file the agent reads
// after reconnecting (0011-MADR §9). The backend serves as the managed
// update's Lifecycle in newUpdater as well.
func ExampleHandOff() {
	// First: in a detached run, apply the handoff's environment. Report's
	// result starts now.
	report := service.ReportFunc()

	unit, err := systemd.New(systemd.Options{Unit: "relay.service"})
	if err != nil {
		os.Exit(1) // off Linux: service.ErrUnsupported
	}
	o := cli.StdioOptions()
	o.HandOff = cli.HandOff{
		Detach: service.HandOffFunc(unit, service.HandOff{Args: os.Args[1:]}),
		Report: report,
	}
	os.Exit(cli.Command(context.Background(), os.Args[1:], "relay", relayIdentity, newRelayUpdater, o))
}

// relayIdentity and newRelayUpdater stand for the program's own build
// identity and updater, whose managed installer uses the unit as its
// Lifecycle and Reconciler.
var (
	relayIdentity   buildinfo.Info
	newRelayUpdater func() (*selfupdate.Updater, error)
)
