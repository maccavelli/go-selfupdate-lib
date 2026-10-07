// Command relay is the release fixture of
// docs/decisions/0013-PLAN-build-and-stage-release-workflow.md B2: a program
// built, staged and run by selfupdate-release and by the CI rehearsal.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/buildinfo"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version":
			fmt.Println(buildinfo.Identity())
			return
		case "silent":
			return
		case "hang":
			time.Sleep(time.Hour)
			return
		case "mark-installed":
			// The release's after_install hook
			// (docs/decisions/0014-PLAN-shared-installer-templates.md I5):
			// relay.installed beside the program says it ran.
			if err := markInstalled(); err != nil {
				fmt.Fprintln(os.Stderr, "relay:", err)
				os.Exit(1)
			}
			return
		}
	}
	fmt.Println("relay")
}

// markInstalled writes the program's identity to relay.installed in its
// own directory.
func markInstalled() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(filepath.Dir(exe), "relay.installed"), []byte(buildinfo.Identity().String()+"\n"), 0o644)
}
