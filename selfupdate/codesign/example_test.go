package codesign_test

import (
	"fmt"
	"runtime"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/codesign"
)

// ExampleNewSigner re-signs the staged binary with the publisher's
// identity before it is installed, on macOS only. A program that does not
// sign leaves Config.Transformer unset.
func ExampleNewSigner() {
	var cfg selfupdate.Config
	if runtime.GOOS == "darwin" {
		signer, err := codesign.NewSigner(codesign.SignOptions{
			Identity:   "Developer ID Application: Example (TEAMID)",
			Identifier: "com.example.relay",
		})
		if err != nil {
			fmt.Println(err)
			return
		}
		cfg.Transformer = signer
	}
	_ = cfg
}

// ExampleNewChecker refuses an update whose binary is not signed by the
// publisher's team. Start the requirement from the release's own, which
// codesign -d -r- prints.
func ExampleNewChecker() {
	var cfg selfupdate.Config
	if runtime.GOOS == "darwin" {
		checker, err := codesign.NewChecker(codesign.CheckOptions{
			Requirement: `anchor apple generic and certificate leaf[subject.OU] = "TEAMID"`,
		})
		if err != nil {
			fmt.Println(err)
			return
		}
		cfg.Probes = append(cfg.Probes, checker)
	}
	_ = cfg
}
