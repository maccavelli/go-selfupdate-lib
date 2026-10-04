# Extending `selfupdate`

For a program built on `github.com/maccavelli/go-selfupdate-lib/selfupdate` that
needs more than the standalone default: a banner instead of an update, a
front end that reads JSON, its own credential store, an extra check before
install, or its own installer. Every section names one seam, what the package
does around it, and a runnable pointer: an `Example` you can read with
`go doc`, or a test in `selfupdate/`.

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
final `Result`, with `schema_version` 1.

Keep stdout for the JSON, and write human text to stderr. `MultiReporter`
sends each event to several reporters in order: `NewTextReporter(os.Stderr)`
plus `NewJSONReporter(os.Stdout)` is the usual pair.

Byte progress is opt-in. Set `Config.ProgressInterval` to the minimum gap
between `EventProgress` events; zero reports none. The text reporter skips
progress events either way.

- `ExampleNewJSONReporter`, `ExampleMultiReporter`, `ExampleResult_Document`
- `selfupdate/testdata/golden/`: the exact text and JSON Lines output of six
  whole runs

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
`Cancel`; an unanswered request keeps the run waiting.

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

## Verify a signature later

No publisher signature is verified by default. Two hooks run inside the
update, and both make the run fail with `ErrIntegrity`:

- **`Config.ManifestVerifiers`** run on the downloaded `SHA256SUMS`, before
  any binary byte is fetched. A signature over the manifest belongs here. The
  verifier can read a sibling asset, such as `SHA256SUMS.sig`, through
  `ManifestVerification.OpenAsset`.
- **`Config.Verifiers`** run on the staged binary after the built-in checks.
  `NewImageVerifier` is one: the binary must be an executable for the selected
  platform (ELF, Mach-O thin or fat, or PE).

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

A `Transformer`, such as a re-signing step, runs before the probes, so the
probes see the bytes that will be installed.

- `selfupdate/probe_test.go`

## Rehearse without installing

`Request.DryRun` does everything short of the install: download, verify,
transform and probe. It asks nobody, then discards the staging file. The
`Result` has `DryRun` set and both digests.

- `selfupdate/lifecycle_test.go`, `selfupdate/testdata/golden/text-dry-run.golden`

## Keep the previous binary

`InstallOptions.KeepPrevious` renames the replaced binary to `.<base>.previous`
beside the target at commit, over any older one, and reports it in
`Result.Previous`. On Windows this works while the old image is still
running. `StandaloneInstaller.CleanupPending`, called at startup, finishes
what an earlier update left behind. An `ErrConcurrentUpdate` from it means
another update holds the lock, which is benign.

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
- **With a `Transformer`:** the session must implement `StagingOwner`, so
  the transformed staging file can be proven to be yours. A session without
  it owns nothing, and the run stops before install.

Pointers:

- `ExampleNewManagedInstallerFor`, `ExampleNewManagedInstaller`
- `selfupdate/twophase_test.go`: a custom session driven by the managed
  installer, in order

## Test a program that self-updates

The `selfupdate/selfupdatetest` package provides:

- `NewRelease`, which builds a release with matching `SHA256SUMS`;
- `FakeSource`;
- `RecordingReporter`;
- `ScriptedConfirmer`;
- `GitHubServer`, a fake GitHub API on one TLS origin whose asset requests
  redirect to a second. It can rate-limit and truncate, and logs every request
  with whether it carried `Authorization`.

Pointers:

- `selfupdate/selfupdatetest/selfupdatetest_test.go`
- `selfupdate/e2e_github_test.go`, `selfupdate/golden_test.go`
