package selfupdatetest_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/selfupdatetest"
)

// A program's test runs its updater offline against an in-memory release: a
// dry run downloads and verifies it and installs nothing, and the fake
// records each call it answered. The target is a stand-in binary in a
// temporary directory, the only root the installer may write to.
func ExampleNewFakeSource() {
	dir, err := os.MkdirTemp("", "selfupdatetest-example-")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() { _ = os.RemoveAll(dir) }()
	if dir, err = filepath.EvalSymlinks(dir); err != nil {
		fmt.Println(err)
		return
	}
	exe := filepath.Join(dir, "demo")
	if err := os.WriteFile(exe, []byte("demo v1.0.0\n"), 0o755); err != nil {
		fmt.Println(err)
		return
	}
	plats := []selfupdate.Platform{{OS: runtime.GOOS, Arch: runtime.GOARCH}}
	src := selfupdatetest.NewFakeSource("v1.1.0", selfupdatetest.NewRelease("demo", "v1.1.0", plats,
		func(selfupdate.Platform) []byte { return []byte("demo v1.1.0\n") }))
	selector, err := selfupdate.NewExactAssetSelector(plats)
	if err != nil {
		fmt.Println(err)
		return
	}
	installer, err := selfupdate.NewStandaloneInstaller(selfupdate.InstallOptions{
		TargetPolicy: selfupdate.TargetPolicy{ExecutablePath: exe, AllowedRoots: []string{dir}},
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	updater, err := selfupdate.New(selfupdate.Config{
		Source: src, Versions: selfupdate.NewStrictVersionPolicy(), Assets: selector, Installer: installer,
		Reporter: selfupdate.DiscardReporter(), Confirmer: selfupdate.NonInteractiveConfirmer(),
		Limits: selfupdate.DefaultLimits(),
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	res, err := updater.Run(context.Background(), selfupdate.Request{
		Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: selfupdate.ReleaseBuild, DryRun: true,
	})
	fmt.Println(res.TargetVersion, res.DryRun, res.Applied, err)
	fmt.Println(len(src.Calls()) > 0)
	// Output:
	// v1.1.0 true false <nil>
	// true
}
