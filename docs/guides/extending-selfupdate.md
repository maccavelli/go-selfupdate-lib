# Extending `selfupdate`

For a program built on `github.com/maccavelli/go-selfupdate-lib/selfupdate` that
needs more than the standalone default: a banner instead of an update, a
front end that reads JSON, its own credential store, an extra check before
install, releases shipped as archives, a macOS signature, its own installer,
or a service to update from inside. Every section names one seam, what the
package does around it, and a runnable pointer: an `Example` you can read
with `go doc`, or a test in `selfupdate/`.

Why each seam exists is in
[0004-MADR](../decisions/0004-MADR-evolve-selfupdate-api-and-tui-support.md);
what `v1.1.0` added, and the one behaviour change that can need code, are in
the release notes of
[0004-PLAN-v1-1-0-core-api.md](../decisions/0004-PLAN-v1-1-0-core-api.md#release-notes-for-v110).

## Show an update banner

`NewChecker` (or `Updater.Checker`, which shares the Updater's parts) answers
"is there an update?" as an `Availability`. It resolves no target, takes no
lock, prompts nobody and downloads no asset body, so it is safe on every
start.

For a check on every start, use `Checker.CheckCached` with
`NewFileCheckStore` under `os.UserCacheDir`. It answers from the file while
the record is younger than `maxAge`. After a rate limit it returns
`ErrCheckDeferred` and the saved answer until the limit has passed, without
touching the network.

Since `v1.6.0` the cache keeps the errors the same question would get again:
`ErrLatestOlder`, `ErrUnsupportedPlatform` and `ErrMutableRelease`. The
record's `Outcome` names which one, and for `maxAge` a cached one returns an
error that matches its sentinel with `errors.Is`, with no network call. A
program that ran on a newer build than any release, or on a platform with
no asset, no longer asks GitHub on every start. Any other error is not
saved. A record written before `v1.6.0` loads as a miss, which costs one
check.

- `ExampleNewChecker`, `ExampleChecker_CheckCached`
- `selfupdate/checker_test.go`, `selfupdate/checkcache_test.go`,
  `selfupdate/checkcache_outcome_test.go`

## Offer a beta channel

A channel lets users who opt in get prereleases, while everyone else
stays on stable builds. It takes a version policy that knows the channels,
a per-run choice, and a workflow input.

- **The policy.**
  `NewSemverPolicy(SemverOptions{AllowPrerelease: true, Channels: []string{"rc", "beta"}})`
  replaces `NewStrictVersionPolicy`.
  - It accepts `vX.Y.Z` and `vX.Y.Z-NAME.N` for each listed name.
  - The names are listed most stable first, in descending ASCII order,
    so version precedence agrees with stability; the constructor
    refuses any other order. A name's place is fixed by its spelling:
    `nightly` sorts above `beta`, so it can never be the less stable of
    the two.
  - Build metadata (`+…`) is never accepted.
- **The choice.** Set `Request.Channel` or `CheckRequest.Channel`, for
  example from a `--channel` flag or a setting.
  - `""` is the stable channel. It uses `Latest` and never offers a
    prerelease.
  - A channel admits stable releases, its own prereleases and those of
    every more stable channel: `beta` also gets `rc` builds.
  - A channel lists releases through `ReleaseLister`, which
    `GitHubSource` implements; a source that does not is refused.
  - `CheckCached` keys its answer by channel, so switching channel never
    reuses the other channel's answer.
  - A pinned `TargetVersion` that is a prerelease needs a channel that
    admits it.
- **Moving between channels** follows version order. On `v1.3.0-rc.2`,
  a stable `v1.3.0` is an upgrade on every channel. Back on the stable
  channel while it is still at `v1.2.0`, `Run` reports `ErrLatestOlder`;
  `--version v1.2.0` is the explicit way back.
- **Publishing.** Pass the channels to the release workflow, and tag the
  prerelease `vX.Y.Z-NAME.N`:

  ```yaml
  with:
    prerelease-channels-json: '["rc","beta"]'
  ```

  The workflow publishes such a tag as a GitHub prerelease that never
  becomes the repository's latest release, so stable clients never see
  it. Without the input, it refuses every prerelease tag.

- `ExampleNewSemverPolicy`
- `selfupdate/channel_test.go`, and `TestE2EUpdateRunningCopyOnChannel` in
  `selfupdate/e2e_running_test.go`
- Why, and the rules: [0005-MADR](../decisions/0005-MADR-opt-in-prerelease-channels.md)

## Read JSON output

`NewJSONReporter` writes one JSON object per event (JSON Lines), with the keys
`kind`, `product`, `current`, `target`, `asset`, `bytes`, `total` and
`detail`, in that order. `Result.Document` is the stable JSON form of the
final `Result`. Its `schema_version` is 2 since `v1.6.0`, which added
`service_started`, and `warnings` when there are any.

A run that goes on past selection has one terminal event: `complete`,
`failed` or `declined`. A check that succeeds, and a run that finds the
program up to date, install nothing: they end at `selected`, the result's
`operation` is the outcome, and no `complete` follows. Since
`v1.6.0`, an error that arrives after `complete`, such as a failed unlock
once the binary is replaced, does not fail the run. Each one is a `warning`
event, `Result.Warnings` lists them, and `Run` returns no error. A dry run's
`complete` has the `detail` `dry-run`. `Warnings` is a string type whose JSON
form is an array: read it with `List`, or build one with `NewWarnings`.

Keep stdout for the JSON, and write human text to stderr. `MultiReporter`
sends each event to several reporters in order: `NewTextReporter(os.Stderr)`
plus `NewJSONReporter(os.Stdout)` is the usual pair.

Byte progress is opt-in. Set `Config.ProgressInterval` to the minimum gap
between `EventProgress` events; zero reports none. The text reporter skips
progress events either way.

- `ExampleNewJSONReporter`, `ExampleMultiReporter`, `ExampleResult_Document`
- `selfupdate/testdata/golden/`: the exact text and JSON Lines output of
  seven whole runs

## Plug in a credential

`GitHubOptions.Credentials` takes a `CredentialProvider`. It is asked lazily,
on the first API request, after `GitHubOptions.Token` and before `GH_TOKEN`
and `GITHUB_TOKEN`. `ChainCredentials` asks providers in order, and
`ErrNoCredential` passes to the next. `EnvCredential` reads named variables.

- An empty `Credential.Header` sends `Authorization: Bearer <Value>`;
  otherwise `Header: Value` is sent as given.
- The credential goes only to the API origin. It is removed from a redirect
  to any other origin, such as GitHub's download host.
- On a 401 the provider is asked once more, with `CredentialRequest.Cause`
  set.
- `GitHubOptions.Observer` is told once when a credential was accepted. That
  is the moment to save a credential the user just typed.

Pointers:

- `ExampleChainCredentials`
- `selfupdate/credentials_test.go`
- `selfupdate/e2e_github_test.go`: no `Authorization` reaches the download
  origin

## Drive an update from a TUI or event loop

A TUI cannot sit blocked inside `Run`: it has to keep rendering. `Start`
runs the request in its own goroutine and returns a `Stream`, which the
event loop pulls from with `Next(ctx)` or `All(ctx)`.

- **`Progressed`** carries every event, in order. Byte progress
  (`EventProgress`) is coalesced, so a slow UI sees the newest count and
  never slows the download. Turn it on for the run with
  `WithProgressInterval`.
- **`*ConfirmNeeded`** carries the `Prompt`. Call `Answer(ok)`, or
  `Cancel(err)`. `Request.Yes`, or `Start(…, WithConfirmer(c))`, means it
  never appears.
- **`*CredentialNeeded`** appears when `PromptCredential()` is in the
  source's credential chain, or is passed as `WithCredentials`. Call
  `Supply(cred)` or `Cancel(err)`. A refused credential prompts once more,
  with `Request.Cause` set.
- **`Finished`** comes last, once the run has returned: recovery has run
  and the session is closed. `Next` then returns `io.EOF`.

`Stream.Cancel` cancels the run and still delivers `Finished`, so ctrl+c
can wait for the real outcome. The host must answer every request or call
`Cancel`; an unanswered request keeps the run waiting. Since `v1.6.0` a
zero `ConfirmNeeded` or `CredentialNeeded`, such as one a UI test builds,
can be answered safely: no run waits on it, so the call returns at once
and does nothing.

In Bubble Tea, `Next` is the "wait for activity" command: a `tea.Cmd`
that calls `s.Next(ctx)` and returns the interaction as a message, then is
issued again from `Update`. The planned `go-tui-lib/updatetea` adapter
packages that pattern, with a progress bar, a masked input and a confirm
prompt (0004-MADR §4).

`Updater.RunWith` is the same per-run override without a `Stream`:
`WithReporter`, `WithConfirmer`, `WithCredentials` and
`WithProgressInterval` apply to that run only, so one `Updater` serves a
`--json` CLI, a terminal prompt and a TUI.

- `ExampleStart`, `ExampleStream_All`, `ExampleUpdater_RunWith`,
  `ExamplePromptCredential`
- `selfupdate/stream_run_test.go`: cancellation mid-download, prompts,
  and the delivery order

## Ship an archive

Since `v1.8.0`, `selfupdate/archive` updates from a release whose asset is
a `.tar.gz`, `.zip` or `.gz` holding the program, instead of the bare
binary. Why it works as it does is in
[0012-MADR](../decisions/0012-MADR-archive-assets-and-macos-codesign.md).

- **Configure both halves:** `archive.NewSelector` as `Config.Assets`, and
  `archive.NewUnpacker` as `Config.Unpacker`.
  - The run refuses one without the other before it downloads anything,
    on `--check` too.
  - `New` refuses an `Unpacker` beside `NewExactAssetSelector`, or beside a
    `NewImageVerifier`, which would check the archive instead of the
    program.
- **Names:** by default `<product>-<os>-<arch>.tar.gz`, with `.zip` on
  Windows, beside `SHA256SUMS`.
  - **GoReleaser's defaults:** set `Name: archive.GoReleaserName` and
    `Manifest: archive.GoReleaserChecksums`, and a `Format` that returns
    `archive.TarGz` on every platform, since GoReleaser's default is
    tar.gz on Windows too.
  - **Any other scheme,** such as the uname style
    (`relay_Darwin_arm64.tar.gz`), is a `Name` function you write.
- **The program** is the one regular file named for the product (`.exe`
  on Windows), at the top level or one directory down.
  `UnpackOptions.Member` names another path.
- **What is checked:**
  - `SHA256SUMS`, the GitHub digest and the `Verifiers` check the archive
    as published.
  - The unpacker refuses an archive that two tools could read
    differently, or that exceeds the limits: unsafe or duplicate names
    (case-insensitively), links, devices and sparse files, too many
    entries, more than `Limits.Executable` bytes, overlapping zip entries,
    and more than one program.
  - It then checks that the program is an executable for the platform.
  - Every refusal is an `ErrIntegrity`.
- **Publishing:** since `v1.9.0`, this repository's workflows build,
  pack and publish archives from a release spec with
  `"packaging": "archive"`, and `spec.AssetSelector()` and
  `spec.Unpacker()` configure both halves; see
  [Building releases](building-releases.md#7-archives). Archives
  published with other tooling, such as GoReleaser, work too, as
  immutable GitHub releases: the updater refuses a mutable one.

Pointers:

- `archive.ExampleNewSelector`, `archive.ExampleNewUnpacker`
- `selfupdate/archive/e2e_test.go`: each format, in both namings, through
  `Updater` and `cli.Command`
- `selfupdate/archive/unpack_test.go`: every refusal

## Verify a signature later

No publisher signature is verified by default. Two hooks run inside the
update, and both make the run fail with `ErrIntegrity`:

- **`Config.ManifestVerifiers`** run on the downloaded `SHA256SUMS`, before
  any binary byte is fetched. A signature over the manifest belongs here. The
  verifier can read a sibling asset, such as `SHA256SUMS.sig`, through
  `ManifestVerification.OpenAsset`.
- **`Config.Verifiers`** run on the staged release asset after the built-in
  checks: the archive itself, when the selector picks one. `NewImageVerifier`
  is one: the binary must be an executable for the selected platform (ELF,
  Mach-O thin or fat, or PE). With an archive, the unpacker makes that check
  on the program instead.

How signing would be added, and why it is not yet, is in
[0004-REPORT](../reports/0004-REPORT-release-signing-research.md).

- `selfupdate/manifestverify_test.go`, `selfupdate/imageverify_test.go`

## Probe the new binary

`Config.Probes` run the staged binary before anything is replaced. On
Windows the staging file is named `.exe`, and elsewhere it is made executable
first. `InstallOptions.PostInstall` runs the installed binary before the
replacement is committed, and rolls it back on failure.
`NewVersionProber(args, want, timeout)` runs the binary with `args` and
requires its stdout to contain the release's version.

An `Unpacker`, then a `Transformer` such as a re-signing step, run before
the probes, so the probes see the bytes that will be installed.

- `selfupdate/probe_test.go`

## Sign on macOS

`selfupdate/codesign` is for a publisher who signs its macOS binaries. It
is opt-in: nothing runs it unless you configure it, and a binary that the
Go linker signed ad hoc, or that you do not sign, updates without it. Both
constructors return `service.ErrUnsupported` off macOS, so build them only
when `runtime.GOOS` is `"darwin"`.

- **Re-sign the staged binary** with your identity, before it is
  installed:

  ```go
  signer, err := codesign.NewSigner(codesign.SignOptions{
      Identity:   "Developer ID Application: Example (TEAMID)",
      Identifier: "com.example.relay",
  })
  cfg.Transformer = signer
  ```

  - **`Identifier` is required.** Without it, `codesign` names the
    signature after the staging file, `.relay`, not after your program.
  - After signing, the signer verifies the signature and requires that
    identifier, and `Requirement` too when you set it.
  - Signing with a certificate needs the identity in your keychains, and
    is expected to need Xcode or the Command Line Tools.
  - `Runtime` adds the hardened runtime. `Timestamp` asks Apple's timestamp
    server, which notarization needs; without it, an update never contacts
    that server.
- **Require a signature:** add `codesign.NewChecker` to `Config.Probes`,
  with a `Requirement` such as
  `anchor apple generic and certificate leaf[subject.OU] = "TEAMID"`.
  - Start the requirement from your release's own, which `codesign -d -r-`
    prints, as Apple's TN3127 advises.
  - A binary with no signature, an invalid one, and one that does not meet
    the requirement each fail the update with `ErrIntegrity`.
- **Order:** unpack, then sign, then the checker, which therefore checks
  the bytes that will be installed.

Pointers:

- `codesign.ExampleNewSigner`, `codesign.ExampleNewChecker`
- `selfupdate/codesign/live_darwin_test.go`: the real `codesign`, ad hoc,
  and with your own identity when `SELFUPDATE_CODESIGN_IDENTITY` names it

## Rehearse without installing

`Request.DryRun` does everything short of the install: download, verify,
unpack, transform and probe. It asks nobody, then discards the staging file. The
`Result` has `DryRun` set and both digests.

- `selfupdate/lifecycle_test.go`, `selfupdate/testdata/golden/text-dry-run.golden`

## Keep the previous binary

`InstallOptions.KeepPrevious` renames the replaced binary to `.<base>.previous`
beside the target at commit, over any older one, and reports it in
`Result.Previous`. On Windows this works while the old image is still
running. `StandaloneInstaller.CleanupPending`, called at startup, finishes
what an earlier update left behind. Since `v1.6.0` that includes the staging
files and backups of an update that crashed: under the lock they belong to
no one. An `ErrConcurrentUpdate` from it means another update holds the
lock, which is benign.

## Replace a setuid or setgid binary

A target with the setuid or setgid bit is refused, so an update never
silently grants or drops privileges. Set
`TargetPolicy.AllowSpecialModeBits` to replace it: the new binary keeps the
bits. The sticky bit is always kept. On Unix the new binary also gets the
old one's owner and group, when the updater may give them; an updater that
may not, such as one run by the file's owner without root, keeps its own.

- `selfupdate/lifecycle_test.go`, `selfupdate/lifecycle_windows_test.go`

## Write your own installer

`Installer` and `InstallSession` are the whole contract for a single-step
install.

- **Under a service manager:** to run under `NewManagedInstallerFor`, the
  session must also implement `TwoPhaseSession`:
  - `Apply` makes the replacement live and keeps a backup;
  - `Commit` and `Rollback` finish it once the service has been reconciled,
    restarted and checked;
  - `AppliedReplacement.State` carries your session's private state from
    `Apply` to `Commit` or `Rollback`.
- **A stopped service:** since `v1.6.0` the managed installer starts a
  service after the update only when it was running, or when your
  `Lifecycle` also implements `EnabledLifecycle` and `Enabled` reports it
  configured to start (systemd `is-enabled`, launchd `RunAtLoad` or
  `KeepAlive`, a Windows automatic start type). Otherwise the binary is
  replaced and the service stays stopped. An `Enabled` error fails the
  install before anything changes. `InstallResult.ServiceStarted`,
  `Result.ServiceStarted` and the document's `service_started` say whether
  it was started. Before `v1.6.0`, a stopped service was always started.
- **With a `Transformer`:** the session must implement `StagingOwner`, so
  the transformed staging file can be proven to be yours. A session without
  it owns nothing, and the run stops before install.

Pointers:

- `ExampleNewManagedInstallerFor`, `ExampleNewManagedInstaller`
- `selfupdate/twophase_test.go`: a custom session driven by the managed
  installer, in order
- `selfupdate/managed_stopped_test.go`, `selfupdate/managed_started_test.go`:
  the start rule, and recovery

## Run as a service

Since `v1.7.0`, `selfupdate/service` and its three backends are the
`Lifecycle` a managed install needs, written once for each service manager.
Why each behaves as it does is in
[0011-MADR](../decisions/0011-MADR-reference-service-lifecycles.md).

- **Pick the backend:**
  - `systemd.New(systemd.Options{Unit: "relay.service"})`, with
    `Scope: systemd.User` for a user unit;
  - `launchd.New(launchd.Options{Label: …, Domain: launchd.System(),
    Plist: …})`, or `launchd.GUI(uid)` or `launchd.User(uid)` for an agent;
  - `scm.New(scm.Options{Name: "relay"})`.
- **Use it twice.** Each is a `selfupdate.Lifecycle`, an
  `EnabledLifecycle` and a `Reconciler`:
  `selfupdate.NewManagedInstaller(inner, b, b)`.
- **What it does around the replace:**
  - `Stop` returns once the service has stopped, not when the stop was
    asked for: systemd `inactive` or `failed`; launchd out of the domain
    with its process gone; the SCM's `STOPPED`, by Microsoft's wait-hint
    loop.
  - `WaitHealthy` requires a new instance (a new systemd `InvocationID`,
    a new process ID) that stays up for `Options.Poll.Settle`, 10 s by
    default, then runs `Options.Probe` if you set one.
  - `Reconcile` checks that the definition still runs the binary, and
    changes nothing. `Options.RewritePath` lets it point the definition at
    a binary that moved. A program that owns its definition through its own
    install command uses `service.NewExecReconciler` instead.
- **systemd units: `Type=notify` or `Type=exec`.** With either, a failed
  start fails `systemctl start`. With `Type=simple`, it succeeds before the
  program has run. Under `Type=notify`, call `systemd.Ready()` once the
  program serves; `systemd.WatchdogInterval` and `systemd.Watchdog` serve
  `WatchdogSec=`.

### Updating from inside the service

A process the service started, such as an agent running a remote session,
is inside the service. When it runs `<prog> update --yes`, stopping the
service kills the update halfway, and the service stays stopped. The
handoff runs that update outside the service instead, as a detached copy
of the same command:

```go
func main() {
    report := service.ReportFunc() // first: see below
    unit, err := systemd.New(systemd.Options{Unit: "relay.service"})
    // …
    o := cli.StdioOptions()
    o.HandOff = cli.HandOff{
        Detach: service.HandOffFunc(unit, service.HandOff{Args: os.Args[1:]}),
        Report: report,
    }
    os.Exit(cli.Command(ctx, os.Args[1:], "relay", buildinfo.Identity(), newUpdater, o))
}
```

- **The handoff needs the name** in `Options.Unit`, `Options.Label` or
  `Options.Name`.
- **The agent's session sees** one line on stderr, and exit 0:

  ```text
  update handed off: 3f2a… (transient unit relay-selfupdate-3f2a….service); result in /opt/relay/.relay.selfupdate.handoff
  ```

  Under `--json`, the result object has `"handed_off"` with the same
  detail. The session then usually ends, because the service is
  restarting.
- **After reconnecting,** read the result file, `.<base>.selfupdate.handoff`
  beside the binary, with `service.ReadHandOffResult`, or as JSON:

  ```json
  {"schema_version":1,"id":"3f2a…","started_at":"…","finished_at":"…","exit_code":0,"result":{"schema_version":2,"applied":true,"service_started":true}}
  ```

  `exit_code` is the update's, `error` its message when it failed, and
  `result` the `--json` result document. The service never hands off the
  check: `--check` and `--dry-run` run in place.
- **Call `service.ReportFunc` first in `main`,** or `LoadHandOffEnv` if the
  program reads its environment before that. In a detached run it applies
  the private environment file the launchd handoff writes, and on Windows
  it completes the start, which goes through a short-lived hop.
- **The detached run cannot prompt,** so the update must have `--yes`, as
  an agent's does.
- **Per platform:**
  - **systemd:** a transient unit, `<unit>-selfupdate-<id>.service`,
    started by `systemd-run`, needs systemd 236 or later. System scope
    needs root.
  - **launchd:** a one-shot job, `<label>.selfupdate.<id>`. Call
    `Job.CleanupHandOffs` at start-up to remove finished ones; the next
    handoff does it too.
  - **Windows:** a detached process, out of the service's job where the job
    allows it.
  - **A service manager with no backend here,** such as a Task Scheduler
    task: `service.ProcessDetacher(inside)`, with your own check of
    whether this process is inside. On Unix it is no defence against a
    cgroup kill or a launchd job's reap.

Pointers:

- `ExampleHandOff` in `selfupdate/cli`
- `selfupdate/service/systemd/live_linux_test.go`,
  `selfupdate/service/launchd/live_darwin_test.go`,
  `selfupdate/service/scm/live_windows_test.go`: each runs an update from a
  process the service started, end to end, against the real service
  manager

## Test a program that self-updates

The `selfupdate/selfupdatetest` package provides:

- `NewRelease`, which builds a release with matching `SHA256SUMS`;
- `FakeSource`;
- `RecordingReporter`;
- `ScriptedConfirmer`;
- `GitHubServer`, a fake GitHub API on one TLS origin whose asset requests
  redirect to a second. It can rate-limit and truncate. `RequireToken`
  requires a bearer token; since `v1.6.0`, `RequireCredential(header,
  value)` requires a custom-header credential exactly as the library sends
  it. It logs every request with whether it carried `Authorization`, and,
  in `CredentialHeaders`, the names of the credential headers it carried,
  never their values. So a test can prove a credential stays off the
  download origin.

Pointers:

- `selfupdate/selfupdatetest/selfupdatetest_test.go`
- `selfupdate/e2e_github_test.go`, `selfupdate/golden_test.go`,
  `selfupdate/credential_header_e2e_test.go`
