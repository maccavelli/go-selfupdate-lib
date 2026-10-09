package selfupdate_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/selfupdatetest"
)

func ExampleNew_standalone() {
	src, err := selfupdate.NewGitHubSource(selfupdate.GitHubOptions{
		Repository: selfupdate.Repository{Owner: "maccavelli", Name: "prepare-commit-msg"},
		Client:     &http.Client{Timeout: 15 * time.Minute},
		UserAgent:  "prepare-commit-msg/v1.2.0",
		Limits:     selfupdate.DefaultLimits(),
	})
	if err != nil {
		fmt.Println("source:", err)
		return
	}
	selector, err := selfupdate.NewExactAssetSelector([]selfupdate.Platform{
		{OS: "linux", Arch: "amd64"},
		{OS: "darwin", Arch: "arm64"},
		{OS: "windows", Arch: "amd64"},
	})
	if err != nil {
		fmt.Println("selector:", err)
		return
	}
	installer, err := selfupdate.NewStandaloneInstaller(selfupdate.InstallOptions{})
	if err != nil {
		fmt.Println("installer:", err)
		return
	}
	updater, err := selfupdate.New(selfupdate.Config{
		Source:    src,
		Versions:  selfupdate.NewStrictVersionPolicy(),
		Assets:    selector,
		Installer: installer,
		Reporter:  selfupdate.NewTextReporter(os.Stderr),
		Confirmer: selfupdate.NewTerminalConfirmer(os.Stdin, os.Stderr),
		Limits:    selfupdate.DefaultLimits(),
	})
	if err != nil {
		fmt.Println("new:", err)
		return
	}
	_ = updater
	fmt.Println("standalone updater ready")
	// Output: standalone updater ready
}

func ExampleNewTextReporter() {
	reporter := selfupdate.NewTextReporter(os.Stdout)
	_ = reporter.Report(context.Background(), selfupdate.Event{
		Kind:    selfupdate.EventSelected,
		Product: "demo",
		Target:  "v1.1.0",
		Asset:   "demo-linux-amd64",
	})
	// Output: selfupdate: selected product=demo target=v1.1.0 asset=demo-linux-amd64
}

// exampleService is a stand-in service manager. A real program binds its
// systemd, launchd or Windows service adapter here.
type exampleService struct{}

func (exampleService) Installed(context.Context, string) (bool, error) { return true, nil }
func (exampleService) Running(context.Context, string) (bool, error)   { return true, nil }
func (exampleService) Stop(context.Context, string) error              { return nil }
func (exampleService) Start(context.Context, string) error             { return nil }
func (exampleService) WaitHealthy(context.Context, string) error       { return nil }

// exampleDefinition rewrites nothing; a real Reconciler updates the service
// definition to point at the new executable and can restore it.
type exampleDefinition struct{}

func (exampleDefinition) Reconcile(context.Context, string, string) (selfupdate.ReconcileResult, error) {
	return selfupdate.ReconcileResult{}, nil
}

func (exampleDefinition) Restore(context.Context, string, selfupdate.ReconcileResult) error {
	return nil
}

func ExampleNewManagedInstaller() {
	inner, err := selfupdate.NewStandaloneInstaller(selfupdate.InstallOptions{})
	if err != nil {
		fmt.Println(err)
		return
	}
	managed, err := selfupdate.NewManagedInstaller(inner, exampleService{}, exampleDefinition{})
	if err != nil {
		fmt.Println(err)
		return
	}
	// managed is passed as Config.Installer: Install stops the running
	// service, replaces the binary, reconciles, starts, and waits healthy,
	// rolling everything back if any step fails.
	fmt.Printf("%T\n", managed)
	// Output: *selfupdate.ManagedInstaller
}

func ExampleExitCode() {
	checkFound := fmt.Errorf("wrapped: %w", selfupdate.ErrUpdateAvailable)
	fmt.Println(selfupdate.ExitCode(selfupdate.Result{}, nil))
	fmt.Println(selfupdate.ExitCode(selfupdate.Result{Checked: true}, checkFound))
	fmt.Println(selfupdate.ExitCode(selfupdate.Result{}, fmt.Errorf("network down")))
	// Output:
	// 0
	// 10
	// 1
}

// exampleChecker answers from an in-memory release, so the examples below
// run offline. A program passes its GitHubSource instead.
func exampleChecker() (*selfupdate.Checker, *selfupdatetest.FakeSource) {
	plats := []selfupdate.Platform{{OS: "linux", Arch: "amd64"}}
	src := selfupdatetest.NewFakeSource("v1.1.0", selfupdatetest.NewRelease("demo", "v1.1.0", plats,
		func(selfupdate.Platform) []byte { return []byte("demo v1.1.0\n") }))
	selector, err := selfupdate.NewExactAssetSelector(plats)
	if err != nil {
		panic(err)
	}
	checker, err := selfupdate.NewChecker(selfupdate.CheckerConfig{
		Source: src, Versions: selfupdate.NewStrictVersionPolicy(), Assets: selector,
		Limits: selfupdate.DefaultLimits(),
	})
	if err != nil {
		panic(err)
	}
	return checker, src
}

// A startup banner: is there an update, and to what? Check resolves no
// target, takes no lock and downloads no asset body.
func ExampleNewChecker() {
	checker, _ := exampleChecker()
	avail, err := checker.Check(context.Background(), selfupdate.CheckRequest{
		Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: selfupdate.ReleaseBuild,
		Platform: selfupdate.Platform{OS: "linux", Arch: "amd64"},
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	if avail.Available {
		fmt.Printf("%s %s is available (you have %s): run 'demo update'\n",
			avail.Product, avail.TargetVersion, avail.CurrentVersion)
	}
	// Output: demo v1.1.0 is available (you have v1.0.0): run 'demo update'
}

// CheckCached keeps the last answer on disk, so a program that starts often
// asks the network at most once per maxAge.
func ExampleChecker_CheckCached() {
	dir, err := os.MkdirTemp("", "selfupdate-example-")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() { _ = os.RemoveAll(dir) }()
	store, err := selfupdate.NewFileCheckStore(filepath.Join(dir, "update-check.json"))
	if err != nil {
		fmt.Println(err)
		return
	}
	checker, src := exampleChecker()
	req := selfupdate.CheckRequest{
		Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: selfupdate.ReleaseBuild,
		Platform: selfupdate.Platform{OS: "linux", Arch: "amd64"},
	}
	for range 2 {
		rec, err := checker.CheckCached(context.Background(), req, store, 24*time.Hour)
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println(rec.Availability.TargetVersion, rec.Availability.Available)
	}
	fmt.Println("network calls:", len(src.Calls()))
	// Output:
	// v1.1.0 true
	// v1.1.0 true
	// network calls: 1
}

// A beta channel. Stable users keep getting v1.1.0. A user who opts in to
// "beta" gets the newest stable, rc or beta release; "rc" admits no beta.
func ExampleNewSemverPolicy() {
	policy, err := selfupdate.NewSemverPolicy(selfupdate.SemverOptions{
		AllowPrerelease: true, Channels: []string{"rc", "beta"},
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	plats := []selfupdate.Platform{{OS: "linux", Arch: "amd64"}}
	body := func(selfupdate.Platform) []byte { return []byte("demo\n") }
	beta := selfupdatetest.NewRelease("demo", "v1.2.0-beta.1", plats, body)
	beta.Prerelease = true
	src := selfupdatetest.NewFakeSource("v1.1.0", selfupdatetest.NewRelease("demo", "v1.1.0", plats, body), beta)
	selector, err := selfupdate.NewExactAssetSelector(plats)
	if err != nil {
		fmt.Println(err)
		return
	}
	checker, err := selfupdate.NewChecker(selfupdate.CheckerConfig{
		Source: src, Versions: policy, Assets: selector, Limits: selfupdate.DefaultLimits(),
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, channel := range []string{"", "rc", "beta"} {
		avail, err := checker.Check(context.Background(), selfupdate.CheckRequest{
			Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: selfupdate.ReleaseBuild,
			Platform: plats[0], Channel: channel,
		})
		if err != nil {
			fmt.Println(err)
			return
		}
		name := channel
		if name == "" {
			name = "stable"
		}
		fmt.Printf("%s: %s\n", name, avail.TargetVersion)
	}
	// Output:
	// stable: v1.1.0
	// rc: v1.1.0
	// beta: v1.2.0-beta.1
}

// JSON Lines on stdout for a front end or a script; human text goes to
// stderr.
func ExampleNewJSONReporter() {
	reporter := selfupdate.NewJSONReporter(os.Stdout)
	_ = reporter.Report(context.Background(), selfupdate.Event{
		Kind: selfupdate.EventProgress, Product: "demo", Target: "v1.1.0",
		Asset: "demo-linux-amd64", Bytes: 512, Total: 2048,
	})
	// Output: {"kind":"progress","product":"demo","target":"v1.1.0","asset":"demo-linux-amd64","bytes":512,"total":2048}
}

// One run, two audiences: every event goes to each reporter in order.
func ExampleMultiReporter() {
	reporter := selfupdate.MultiReporter(selfupdate.NewTextReporter(os.Stdout), selfupdate.NewJSONReporter(os.Stdout))
	_ = reporter.Report(context.Background(), selfupdate.Event{
		Kind: selfupdate.EventVerified, Product: "demo", Target: "v1.1.0", Asset: "demo-linux-amd64",
	})
	// Output:
	// selfupdate: verified product=demo target=v1.1.0 asset=demo-linux-amd64
	// {"kind":"verified","product":"demo","target":"v1.1.0","asset":"demo-linux-amd64"}
}

// keychainCredential stands in for a program's own store, such as an OS
// keychain.
type keychainCredential struct{}

func (keychainCredential) Credential(context.Context, selfupdate.CredentialRequest) (selfupdate.Credential, error) {
	return selfupdate.Credential{Value: []byte("example-token"), Source: "keychain"}, nil
}

// The first provider that has a credential wins; ErrNoCredential passes to
// the next. Pass the chain as GitHubOptions.Credentials.
func ExampleChainCredentials() {
	chain := selfupdate.ChainCredentials(
		selfupdate.EnvCredential("", "DEMO_EXAMPLE_UNSET_TOKEN"),
		keychainCredential{},
	)
	c, err := chain.Credential(context.Background(), selfupdate.CredentialRequest{})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("from", c.Source)
	// Output: from keychain
}

// NewManagedInstallerFor drives any Installer whose sessions implement
// TwoPhaseSession; the standalone installer's do.
func ExampleNewManagedInstallerFor() {
	inner, err := selfupdate.NewStandaloneInstaller(selfupdate.InstallOptions{KeepPrevious: true})
	if err != nil {
		fmt.Println(err)
		return
	}
	managed, err := selfupdate.NewManagedInstallerFor(inner, exampleService{}, exampleDefinition{})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("%T\n", managed)
	// Output: *selfupdate.ManagedInstaller
}

// ReplaceBeforeStop replaces a running service's binary before the stop, for
// a service that never starts its own executable while it runs
// (docs/decisions/0020-MADR-precheck-gofmt-errors-and-replace-before-stop.md).
func ExampleNewManagedInstallerWith() {
	inner, err := selfupdate.NewStandaloneInstaller(selfupdate.InstallOptions{})
	if err != nil {
		fmt.Println(err)
		return
	}
	managed, err := selfupdate.NewManagedInstallerWith(inner, exampleService{}, exampleDefinition{},
		selfupdate.ManagedOptions{ReplaceBeforeStop: true})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("%T\n", managed)
	// Output: *selfupdate.ManagedInstaller
}

// Document is the stable JSON form of a Result, for a --json flag.
func ExampleResult_Document() {
	res := selfupdate.Result{
		Product: "demo", CurrentVersion: "v1.0.0", TargetVersion: "v1.1.0",
		AssetName: "demo-linux-amd64", Operation: selfupdate.OperationUpgrade, DryRun: true,
	}
	out, err := json.Marshal(res.Document())
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(string(out))
	// Output: {"schema_version":4,"product":"demo","current_version":"v1.0.0","target_version":"v1.1.0","asset_name":"demo-linux-amd64","operation":"upgrade","checked":false,"applied":false,"declined":false,"dry_run":true,"service_installed":false,"service_was_running":false,"service_started":false,"rolled_back":false,"probes_skipped":false,"replaced_before_stop":false}
}

// exampleUpdater updates a stand-in binary in a temporary directory from
// an in-memory release, so the examples below run offline. A program
// passes its GitHubSource and its own installer instead.
func exampleUpdater() (*selfupdate.Updater, func()) {
	dir, err := os.MkdirTemp("", "selfupdate-example-")
	if err != nil {
		panic(err)
	}
	if dir, err = filepath.EvalSymlinks(dir); err != nil {
		panic(err)
	}
	exe := filepath.Join(dir, "demo")
	if err := os.WriteFile(exe, []byte("old\n"), 0o755); err != nil {
		panic(err)
	}
	// An apply must be for the running platform (Request.Platform).
	plats := []selfupdate.Platform{{OS: runtime.GOOS, Arch: runtime.GOARCH}}
	src := selfupdatetest.NewFakeSource("v1.1.0", selfupdatetest.NewRelease("demo", "v1.1.0", plats,
		func(selfupdate.Platform) []byte { return []byte("demo v1.1.0\n") }))
	selector, err := selfupdate.NewExactAssetSelector(plats)
	if err != nil {
		panic(err)
	}
	installer, err := selfupdate.NewStandaloneInstaller(selfupdate.InstallOptions{
		TargetPolicy: selfupdate.TargetPolicy{ExecutablePath: exe, AllowedRoots: []string{dir}},
	})
	if err != nil {
		panic(err)
	}
	u, err := selfupdate.New(selfupdate.Config{
		Source: src, Versions: selfupdate.NewStrictVersionPolicy(), Assets: selector, Installer: installer,
		Reporter: selfupdate.DiscardReporter(), Confirmer: selfupdate.NonInteractiveConfirmer(),
		Limits: selfupdate.DefaultLimits(),
	})
	if err != nil {
		panic(err)
	}
	return u, func() { _ = os.RemoveAll(dir) }
}

func exampleRequest() selfupdate.Request {
	return selfupdate.Request{Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: selfupdate.ReleaseBuild}
}

// One Updater, configured once, reports this run's events to a function
// and approves it without the configured confirmer.
func ExampleUpdater_RunWith() {
	u, cleanup := exampleUpdater()
	defer cleanup()
	res, err := u.RunWith(context.Background(), exampleRequest(),
		selfupdate.WithReporter(selfupdate.ReporterFunc(func(_ context.Context, ev selfupdate.Event) error {
			fmt.Println(ev.Kind)
			return nil
		})),
		selfupdate.WithConfirmer(selfupdate.ConfirmerFunc(func(context.Context, selfupdate.Prompt) (bool, error) {
			return true, nil
		})))
	fmt.Println(res.Applied, err)
	// Output:
	// resolving-target
	// fetching-release
	// selected
	// downloading-manifest
	// downloading-binary
	// verified
	// installing
	// complete
	// true <nil>
}

// An event loop pulls each interaction, answers the confirmation, and stops
// at Finished. A Bubble Tea program does the same from a command that calls
// Next and returns the interaction as a message.
func ExampleStart() {
	u, cleanup := exampleUpdater()
	defer cleanup()
	s := selfupdate.Start(context.Background(), u, exampleRequest())
	for {
		it, err := s.Next(context.Background())
		if err != nil {
			fmt.Println(err)
			return
		}
		switch v := it.(type) {
		case selfupdate.Progressed:
			fmt.Println("event:", v.Event.Kind)
		case *selfupdate.ConfirmNeeded:
			fmt.Printf("confirm: %s %s to %s\n", v.Prompt.Operation, v.Prompt.Product, v.Prompt.Target)
			v.Answer(true)
		case selfupdate.Finished:
			fmt.Println("applied:", v.Result.Applied, v.Err)
			return
		}
	}
	// Output:
	// event: resolving-target
	// event: fetching-release
	// event: selected
	// confirm: upgrade demo to v1.1.0
	// event: downloading-manifest
	// event: downloading-binary
	// event: verified
	// event: installing
	// event: complete
	// applied: true <nil>
}

// All ranges over the run. Request.Yes approves up front, so no
// ConfirmNeeded is delivered.
func ExampleStream_All() {
	u, cleanup := exampleUpdater()
	defer cleanup()
	req := exampleRequest()
	req.Yes = true
	events := 0
	for it := range selfupdate.Start(context.Background(), u, req).All(context.Background()) {
		switch v := it.(type) {
		case selfupdate.Progressed:
			events++
		case selfupdate.Finished:
			fmt.Println(events, "events; applied:", v.Result.Applied)
		}
	}
	// Output: 8 events; applied: true
}

// PromptCredential goes last in a chain. Outside a Stream it has nothing to
// offer, so the same chain serves Run, which never prompts, and Start, which
// does.
func ExamplePromptCredential() {
	chain := selfupdate.ChainCredentials(
		selfupdate.EnvCredential("", "DEMO_EXAMPLE_UNSET_TOKEN"),
		selfupdate.PromptCredential(),
	)
	_, err := chain.Credential(context.Background(), selfupdate.CredentialRequest{})
	fmt.Println(errors.Is(err, selfupdate.ErrNoCredential))
	// Output: true
}

// ChainTransformers runs transforms in order on the same staging file, such
// as a program's own step before a re-signing one; the coordinator rehashes
// once, after the last.
func ExampleChainTransformers() {
	step := func(name string) selfupdate.Transformer {
		return selfupdate.TransformerFunc(func(context.Context, selfupdate.TransformRequest) error {
			fmt.Println(name)
			return nil
		})
	}
	chain, err := selfupdate.ChainTransformers(step("stamp"), step("sign"))
	if err != nil {
		fmt.Println(err)
		return
	}
	_ = chain.Transform(context.Background(), selfupdate.TransformRequest{})
	// Output:
	// stamp
	// sign
}
