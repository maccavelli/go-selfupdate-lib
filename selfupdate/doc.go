// Package selfupdate is the canonical CLI self-update implementation for
// fleet programs.
//
// It discovers GitHub Releases, selects exact raw-binary assets, verifies
// SHA-256 integrity, and replaces a running executable through an injected
// installer. The package does not import a CLI framework, UI toolkit, or
// service manager. Consumers bind flags, streams, and lifecycle adapters.
//
// # Updating
//
// New composes an Updater from explicit parts, and Updater.Run does one
// request: resolve the target, discover and select the release, confirm,
// download to session-owned staging, verify, optionally transform and probe,
// and install. Request.CheckOnly stops after selection and reports
// ErrUpdateAvailable; Request.DryRun runs everything short of the install,
// without prompting, and leaves the target untouched.
//
// # Driving an update from an event loop
//
// Updater.RunWith runs one request with per-run options, WithReporter,
// WithConfirmer, WithCredentials and WithProgressInterval, so one Updater
// can serve a --json CLI, a terminal prompt and a TUI without being
// rebuilt.
//
// Start runs a request in its own goroutine and returns a Stream that an
// event loop pulls from, with Next or All. The Stream delivers:
//
//   - Progressed, for every event in order, with byte progress coalesced
//     so a slow UI never slows the download;
//   - *ConfirmNeeded, which the host answers;
//   - *CredentialNeeded, which PromptCredential raises when a credential is
//     needed;
//   - Finished, last, once the run has returned.
//
// Stream.Cancel cancels the run and still delivers Finished, so a UI can
// wait for the real outcome rather than report one the run has not
// reached. The host must answer every request or call Cancel.
//
// # Asking without installing
//
// NewChecker, or Updater.Checker, answers "is there an update?" as an
// Availability value. It needs no Installer, Reporter or Confirmer, and
// downloads no asset body. Checker.CheckCached keeps the answer in a
// CheckStore, such as NewFileCheckStore, so a program that starts often asks
// the network at most once per interval and backs off after a rate limit.
// It keeps ErrLatestOlder, ErrUnsupportedPlatform and ErrMutableRelease too,
// as the record's CheckOutcome.
//
// # Channels
//
// NewStrictVersionPolicy accepts only vMAJOR.MINOR.PATCH. NewSemverPolicy,
// with AllowPrerelease, also accepts vMAJOR.MINOR.PATCH-NAME.N for each name
// in Channels, listed most stable first, such as {"rc", "beta"}. A user
// opts in per run with Request.Channel or CheckRequest.Channel. A channel
// admits stable releases and the prereleases of itself and of every more
// stable channel, so "beta" admits rc builds too. Discovery then picks the
// highest admitted release from a ReleaseLister, which GitHubSource is.
// With no channel, discovery uses Latest and never offers a prerelease.
//
// A release whose prerelease flag disagrees with its tag is never offered,
// because the tag of an immutable release cannot change and its flag can.
// Leaving a channel never downgrades: until the stable channel passes the
// running prerelease, Run reports ErrLatestOlder, and an exact
// Request.TargetVersion is the explicit way back. A pinned prerelease needs
// a channel that admits it.
//
// # Events and output
//
// A Reporter receives one Event per stage. NewTextReporter writes plain
// lines, NewJSONReporter writes JSON Lines, and MultiReporter fans out to
// several. EventProgress is opt-in: Config.ProgressInterval is zero by
// default, which reports none, and the text reporter skips it. The outcome
// events EventDeclined, EventFailed and EventRolledBack are advisory: a
// reporter error there never changes the run's result. Result.Document is the
// stable JSON form of a Result. A program keeps its stdout for structured
// output and writes human text to stderr.
//
// # Credentials
//
// GitHubOptions.Token, then GitHubOptions.Credentials, then GH_TOKEN and
// GITHUB_TOKEN supply the API credential. ChainCredentials and EnvCredential
// compose providers; a credential is asked for lazily, sent only to the API
// origin, and stripped from any redirect to another origin. A
// CredentialObserver learns when one was accepted.
//
// # Integrity
//
// Baseline verification proves release-asset integrity: HTTPS GitHub
// metadata, the GitHub asset digest when present, and the exact SHA256SUMS
// entry. It does not prove publisher signature authenticity, and no
// publisher signature is verified by default. Config.ManifestVerifiers run
// on the downloaded SHA256SUMS before any binary byte is fetched; that is the
// hook for a signature over the manifest. Config.Verifiers run on the staged
// binary after the integrity check, and NewImageVerifier checks that it is an
// executable for the selected platform.
//
// # Probes
//
// Config.Probes run the staged binary, made executable for the purpose,
// before anything is replaced. InstallOptions.PostInstall runs the installed
// binary before the replacement is committed, and a failure rolls it back.
// NewVersionProber checks that the binary prints the release's version.
//
// # Installers
//
// NewStandaloneInstaller replaces the binary under a per-target lock.
// InstallOptions.KeepPrevious keeps the replaced binary at .<base>.previous,
// and StandaloneInstaller.CleanupPending, called at startup, processes what
// an earlier update left behind. NewManagedInstaller adds service lifecycle
// and definition reconciliation; NewManagedInstallerFor does the same for any
// Installer whose sessions implement TwoPhaseSession. A custom session used
// with a Transformer must implement StagingOwner.
//
// # The canonical update command
//
// Package selfupdate/cli is the update subcommand every program shares: the
// flags, stdout for protocol output only, the exit codes 0, 10 and 1, signal
// cancellation and a timeout, in one call to cli.Command. Package buildinfo
// owns the build stamps that decide Request.CurrentBuild, and UserAgent
// builds the GitHub User-Agent
// (docs/decisions/0004-MADR-evolve-selfupdate-api-and-tui-support.md §5).
//
// The selfupdatetest package provides test doubles: release fixtures, a fake
// source, reporter and confirmer, and a fake GitHub API.
//
// Consumers publish through the reusable workflow
// .github/workflows/publish-selfupdate-release.yml at the exact go-selfupdate-lib
// module-tag commit. That workflow is the only supported publication path
// for the canonical asset contract. Its prerelease-channels-json input
// names the channels it may publish prereleases for; the default publishes
// stable tags only.
package selfupdate
