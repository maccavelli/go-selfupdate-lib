package cli_test

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/maccavelli/go-selfupdate-lib/buildinfo"
	"github.com/maccavelli/go-selfupdate-lib/internal/testproc"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/cli"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/selfupdatetest"
)

// exampleUpdater builds an offline Updater for product, whose latest release
// is latest, over a throwaway target.
func exampleUpdater(product, latest string) (*selfupdate.Updater, func(), error) {
	here := selfupdate.Platform{OS: runtime.GOOS, Arch: runtime.GOARCH}
	dir, err := os.MkdirTemp("", "cli-example-")
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) } //nolint:errcheck // best-effort cleanup of a temp dir
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	exe := filepath.Join(dir, product)
	if err := testproc.WriteExecutable(exe, []byte("old\n"), 0o700); err != nil {
		cleanup()
		return nil, nil, err
	}
	body := func(selfupdate.Platform) []byte { return []byte("new\n") }
	src := selfupdatetest.NewFakeSource(latest,
		selfupdatetest.NewRelease(product, "v1.0.0", []selfupdate.Platform{here}, body),
		selfupdatetest.NewRelease(product, "v1.1.0", []selfupdate.Platform{here}, body))
	assets, err := selfupdate.NewExactAssetSelector([]selfupdate.Platform{here})
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	inst, err := selfupdate.NewStandaloneInstaller(selfupdate.InstallOptions{
		TargetPolicy: selfupdate.TargetPolicy{ExecutablePath: exe, AllowedRoots: []string{dir}},
	})
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	u, err := selfupdate.New(selfupdate.Config{
		Source: src, Versions: selfupdate.NewStrictVersionPolicy(), Assets: assets, Installer: inst,
		Reporter: selfupdate.DiscardReporter(), Confirmer: selfupdate.NonInteractiveConfirmer(),
		Limits: selfupdate.DefaultLimits(),
	})
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	return u, cleanup, nil
}

// lastLine is the last line s holds.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSuffix(s, "\n"), "\n")
	return lines[len(lines)-1]
}

// A program's update subcommand is one call. A real program passes
// buildinfo.Identity() and cli.StdioOptions().
func ExampleCommand() {
	u, cleanup, err := exampleUpdater("demo", "v1.1.0")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer cleanup()
	id := buildinfo.Info{Version: "v1.0.0", Kind: buildinfo.KindRelease}
	var stdout, stderr bytes.Buffer
	code := cli.Command(context.Background(), []string{"--check"}, "demo", id,
		func() (*selfupdate.Updater, error) { return u, nil },
		cli.Options{Stdout: &stdout, Stderr: &stderr})
	fmt.Println(code)
	fmt.Println(lastLine(stderr.String()))
	fmt.Printf("stdout: %q\n", stdout.String())
	// Output:
	// 10
	// demo: update available: v1.0.0 -> v1.1.0
	// stdout: ""
}

// Run is the part of Command a cobra program calls from RunE, after binding
// the flags itself.
func ExampleRun() {
	u, cleanup, err := exampleUpdater("demo", "v1.0.0")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer cleanup()
	req, err := cli.Flags{Check: true}.Request("demo", buildinfo.Info{Version: "v1.0.0", Kind: buildinfo.KindRelease})
	if err != nil {
		fmt.Println(err)
		return
	}
	var stderr bytes.Buffer
	res, err := cli.Run(context.Background(), u, req, cli.Options{Stderr: &stderr})
	fmt.Println(cli.Exit(&stderr, res, err))
	fmt.Println(lastLine(stderr.String()))
	// Output:
	// 0
	// demo: up to date (v1.0.0)
}

// Exit prints one line for a failure, and nothing for an available update.
func ExampleExit() {
	var stderr bytes.Buffer
	fmt.Println(cli.Exit(&stderr, selfupdate.Result{}, errors.New("network down\nretry later")))
	fmt.Println(cli.Exit(&stderr, selfupdate.Result{}, selfupdate.ErrUpdateAvailable))
	fmt.Print(stderr.String())
	// Output:
	// 1
	// 10
	// update failed: network down retry later
}

// Bind adds the canonical flags to a program's own flag set.
func ExampleFlags_Bind() {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	var f cli.Flags
	f.Bind(fs)
	if err := fs.Parse([]string{"-y", "--channel", "rc"}); err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("%+v\n", f)
	// Output:
	// {Check:false Yes:true Force:false DryRun:false JSON:false Version: Channel:rc}
}
