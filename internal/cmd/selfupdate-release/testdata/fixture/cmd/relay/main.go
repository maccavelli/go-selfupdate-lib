// Command relay is the release fixture of
// docs/decisions/0013-PLAN-build-and-stage-release-workflow.md B2: a program
// built, staged and run by selfupdate-release and by the CI rehearsal.
package main

import (
	"fmt"
	"os"
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
		}
	}
	fmt.Println("relay")
}
