# Extending `selfupdate`

The standalone default is `NewGitHubUpdater`: one call for a program
released on GitHub with exact asset names. It fixes
`NewStrictVersionPolicy`, `NewExactAssetSelector` and a
`StandaloneInstaller` with `InstallOptions{}`, and has no `Unpacker`,
`Verifiers`, `Probes`, `GitHubOptions.Credentials` or `Clock`.

This guide is for a program built on
`github.com/maccavelli/go-selfupdate-lib/selfupdate` that needs more than
that: a banner instead of an update, a front end that reads JSON, its own
credential store, an extra check before install, releases shipped as
archives, a macOS signature, its own installer, a service to update from
inside, or another release host. A program that changes a part builds the
Updater with `New(Config)` from the same parts, with that one changed:
`ExampleNewGitHubUpdater` is the one call, and `ExampleNew_standalone` the
same default built with `New`. Every section
names one seam, what the package does around it, and a runnable pointer:
an `Example` you can read with `go doc`, or a test in `selfupdate/`.

Why each seam exists is in
[0004-MADR](../decisions/0004-MADR-evolve-selfupdate-api-and-tui-support.md);
what each release adds is in the [changelog](../../CHANGELOG.md), and the
code a release can need is in the
[migration guide](migrating-from-mcplib-selfupdate.md).

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
no asset, no longer asks GitHub on every start. Since `v1.11.0` it keeps
`ErrNoRelease` too: a repository with no stable release yet, or a tag never
published, which GitHub answers with a 404. Any other error is not saved.
A record written before `v1.6.0` loads as a miss, which costs one check.
An Updater or a Checker that its constructor did not make, such as a zero
value or a nil pointer, returns `ErrNotConstructed` instead of panicking;
so, since `v1.17.1`, does the `Checker` of a nil `*Updater`.

From `GitHubSource`, a rate limit is a `*RateLimitError`, and any other
non-2xx response an `*HTTPError` (since `v1.16.0`), which `errors.As` reads;
a missing release also wraps `ErrNoRelease` around it. Since `v1.16.0`,
`CheckerConfig.Clock`, or `Config.Clock` for `Updater.Checker`, is the clock
`CheckCached` measures record ages by: a `ClockFunc` that returns a fixed
time makes them deterministic in a test.

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
- **Publishing.** List the channels in the release spec's
  `prerelease_channels`, in the same order
  ([Building releases, step 1](building-releases.md#1-write-the-spec)),
  and tag the prerelease `vX.Y.Z-NAME.N`. The build workflow's output
  `prerelease-channels-json` carries them to the publish workflow, which
  the caller passes on as `needs.build.outputs.prerelease-channels-json`
  ([step 4](building-releases.md#4-call-the-workflows)). The publish
  workflow publishes such a tag as a GitHub prerelease that never becomes
  the repository's latest release, so stable clients never see it.
  Without channels, it refuses every prerelease tag.

- `ExampleNewSemverPolicy`
- `selfupdate/channel_test.go`, and `TestE2EUpdateRunningCopyOnChannel` in
  `selfupdate/e2e_running_test.go`
- Why, and the rules: [0005-MADR](../decisions/0005-MADR-opt-in-prerelease-channels.md)

## Read JSON output

`NewJSONReporter` writes one JSON object per event (JSON Lines), with the keys
`kind`, `product`, `current`, `target`, `asset`, `bytes`, `total` and
`detail`, in that order; an empty string or a zero number is left out,
except `kind`. `Result.Document` is the stable JSON form of the
final `Result`. Its `schema_version` is 4 since `v1.13.0`, which added
`replaced_before_stop` (a managed install replaced the binary while the
service still ran: [Replace before the stop](#replace-before-the-stop));
3 since `v1.11.0`, which added
`rolled_back` (the previous binary was restored after the new one was
installed) and `probes_skipped` (a dry run for another platform did not run
the probes); 2 since `v1.6.0`, which added `service_started`, and `warnings`
when there are any. Each version only adds keys.

A run that goes on past selection has one terminal event: `complete`,
`failed` or `declined`. A check that succeeds, and a run that finds the
program up to date, install nothing: they end at `selected`, the result's
`operation` is the outcome, and no `complete` follows. Since
`v1.6.0`, an error that arrives after `complete`, such as a failed unlock
once the binary is replaced, does not fail the run. Each one is a `warning`
event, `Result.Warnings` lists them, and `Run` returns no error. Since
`v1.11.0`, so is each of a managed install's `ReconcileResult.Warnings`,
such as an exec reconciler's receipt warnings, or a definition rewritten
without the service manager being reloaded. A dry run's
`complete` has the `detail` `dry-run`. `Warnings` is a string type whose JSON
form is an array: read it with `List`, or build one with `NewWarnings`.

Keep stdout for the JSON, and write human text to stderr. `MultiReporter`
sends each event to several reporters in order: `NewTextReporter(os.Stderr)`
plus `NewJSONReporter(os.Stdout)` is the usual pair.

Under `<prog> update --json`, `selfupdate/cli` ends the stream with exactly
one result object: `{"kind":"result","exit_code":…,"result":{…}}`, where
`result` is the `Result.Document`, `exit_code` the process's exit status,
`error` the error's message when the run failed, and `handed_off` the
handoff's detail when the update was handed off. The package doc,
`go doc github.com/maccavelli/go-selfupdate-lib/selfupdate/cli`, states the
stream rule.

Byte progress is opt-in. Set `Config.ProgressInterval` to the minimum gap
between `EventProgress` events; zero reports none. The text reporter skips
progress events either way.

- `ExampleNewJSONReporter`, `ExampleMultiReporter`, `ExampleResult_Document`
- `selfupdate/testdata/golden/`: the exact text and JSON Lines output of
  seven whole runs

### Log with slog

Since `v1.16.0`, `NewSlogReporter(logger)` logs each event to a
`*slog.Logger`, with the event kind as the message and its fields as
attributes, each left out when empty or zero. `EventFailed` logs at Error,
`EventRolledBack` and `EventWarning` at Warn, `EventProgress` at Debug, and
every other kind at Info. Beside the text or JSON reporter, it is one more
`MultiReporter` entry.

- `ExampleNewSlogReporter`

## Plug in a credential

`GitHubOptions.Credentials` takes a `CredentialProvider`. It is asked lazily,
on the first API request, after `GitHubOptions.Token` and before `GH_TOKEN`
and `GITHUB_TOKEN`. `ChainCredentials` asks providers in order, and
`ErrNoCredential` passes to the next. `EnvCredential` reads named variables.

- An empty `Credential.Header` sends `Authorization: Bearer <Value>`;
  otherwise `Header: Value` is sent as given.
- The credential goes only to the API origin. A redirect to any other
  origin, such as GitHub's download host, keeps only `Accept`,
  `Accept-Encoding`, `User-Agent` and `X-GitHub-Api-Version`.
- On a 401 the provider is asked once more, with `CredentialRequest.Cause`
  set.
- When the provider declines (`ErrNoCredential`, as `PromptCredential` does
  outside a `Stream`) and `GH_TOKEN` or `GITHUB_TOKEN` is used instead, that
  token holds for that run only. A startup check therefore does not stop a
  later `Start` from asking the provider.
- `GitHubOptions.Observer` is told once when a credential was accepted. That
  is the moment to save a credential the user just typed.
- Since `v1.16.0`, `fmt` and `log/slog` print `Credential`, `GitHubOptions`
  and `*GitHubSource` with the secret as `<redacted>`. `encoding/json` still
  carries `Credential.Value`, so a `CredentialObserver` can save it, and
  slog's JSON handler encodes a struct that holds a `Credential` with
  `encoding/json`, secret included: log the `Credential` itself.

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
prompt ([0004-MADR](../decisions/0004-MADR-evolve-selfupdate-api-and-tui-support.md),
§4).

`Updater.RunWith` is the same per-run override without a `Stream`:
`WithReporter`, `WithConfirmer`, `WithCredentials` and
`WithProgressInterval` apply to that run only, so one `Updater` serves a
`--json` CLI, a terminal prompt and a TUI. It serves one run at a time: a
`Run`, `RunWith` or `Start` that overlaps another on the same `Updater`
fails with `ErrConcurrentUpdate`, which `Start` delivers in its `Finished`.

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
  - Since `v1.10.1` it also refuses: a name with a byte outside
    printable ASCII, or an element ending in a dot or a space, which
    APFS, NTFS or Win32 would make another name; a zip entry whose local
    header disagrees with its central-directory record, or that carries
    an Info-ZIP Unicode Path field; a zip directory attribute on a name
    without a trailing `/`, and a directory entry that holds data; a tar
    regular file whose name ends in `/`; and an encrypted zip entry.
  - Since `v1.11.0`, a tar.gz from `git archive` unpacks: its PAX global
    header is skipped, and counts as an entry. One that sets `path`,
    `linkpath`, `size` or a sparse map is refused.
  - Since `v1.11.1` it also refuses a zip whose records do not account
    for every byte of it (bytes before, between or after its entries,
    slack in its central directory, or a data descriptor that does not
    read one way only), and, on every host, a name Windows reserves: an
    element holding one of `< > : " | ? *`, or naming a device such as
    `aux` or `com1`.
  - It then checks that the program is an executable for the platform.
  - Every refusal is an `ErrIntegrity`.
- **Publishing:** since `v1.9.0`, this repository's workflows build,
  pack and publish archives from a release spec with
  `"packaging": "archive"`, and `spec.AssetSelector()` and
  `spec.Unpacker()` configure both halves; see
  [Building releases](building-releases.md#7-archives). Archives
  published with other tooling, such as GoReleaser, work too, as
  immutable GitHub releases: the updater refuses a mutable one.

### Universal, musl and arm variants

`Platform` is `{OS, Arch}`, so a variant a platform can run is chosen by
name, not by field
([0025-MADR](../decisions/0025-MADR-align-with-go-sdk-design-practices.md)
D13):

- **One universal macOS archive:** a `Name` function that returns the same
  name for `darwin/amd64` and `darwin/arm64`, such as
  `relay-darwin-universal.tar.gz`. The image check accepts a fat Mach-O
  that holds the running architecture.
- **musl, or an arm level:** the asset to install depends on the host, not
  on `GOOS` and `GOARCH`, so write an `AssetSelector` that looks at the host
  (for example, whether `/lib/ld-musl-*` exists) and picks the exact name;
  raw binaries have no name hook, so this is the route for them too.
- **GoReleaser's names** for `arm` and `mips*` carry a variant `Platform`
  does not hold, so `GoReleaserName` refuses them; name them with a `Name`
  function of your own.

Pointers:

- `archive.ExampleNewSelector`, `archive.ExampleNewUnpacker`
- `selfupdate/archive/e2e_test.go`: each format, in both namings, through
  `Updater` and `cli.Command`
- `selfupdate/archive/unpack_test.go`: every refusal

## Verify a release signature

No publisher signature is verified by default.
`selfupdate/verify/signednote`, since `v1.16.0`, is the opt-in check: a
`ManifestVerifier` that requires `SHA256SUMS.note`, a signed note over the
release's statement, to open with a key your program embeds, and to name
your repository, the tag, your product and the exact manifest. The publish
workflow signs when you ask; [the building guide, step
13](building-releases.md#13-sign-a-release-opt-in), says how.

```go
v, err := signednote.New(signednote.Config{
    Repository:   "github.com/example/relay",
    Keys:         []string{ciKey}, // a verifier key constant you embed
    RequiredFrom: "v1.4.0",
})
if err != nil {
    return err
}
cfg.ManifestVerifiers = append(cfg.ManifestVerifiers, v)
```

- **`Keys`** are note verifier keys, embedded as Go constants. A note
  signed by any of them is accepted, so listing an old and a new key
  rotates from one to the other.
- **`RequiredFrom`** is the first signed tag: a release below it may have
  no note, so an exact `--version` can still install an older, unsigned
  release. Empty means every tag needs one.
- **What it does not prove:** freshness. A stale release served as the
  latest still verifies.

Two hooks run inside the update, and both make the run fail with
`ErrIntegrity`:

- **`Config.ManifestVerifiers`** run on the downloaded `SHA256SUMS`, before
  any binary byte is fetched. A signature over the manifest belongs here. The
  verifier can read a sibling asset, such as `SHA256SUMS.note`, through
  `ManifestVerification.OpenAsset`.
- **`Config.Verifiers`** run on the staged release asset after the built-in
  checks: the archive itself, when the selector picks one. `NewImageVerifier`
  is one: the binary must be an executable for the selected platform (ELF,
  Mach-O thin or fat, or PE). With an archive, the unpacker makes that check
  on the program instead.

The design and its trade-offs are in
[0004-REPORT](../reports/0004-REPORT-release-signing-research.md); signing
was built, opt-in, by
[0025-MADR](../decisions/0025-MADR-align-with-go-sdk-design-practices.md)
D10. To check that a release was built by your workflow, see
[Check build provenance](#check-build-provenance).

- `signednote.ExampleNew`
- `selfupdate/verify/signednote/signednote_test.go`,
  `selfupdate/manifestverify_test.go`, `selfupdate/imageverify_test.go`

## Check build provenance

`selfupdate/verify/ghattest` checks, during an update, that a release was
built by your release workflow: since `v1.12.0`. This module's publish
workflow attests every file it publishes, `SHA256SUMS` included, and the
check verifies that attestation with `gh attestation verify`. It is opt-in:
nothing runs it unless you configure it.

```go
v, err := ghattest.NewManifestVerifier(ghattest.Options{
    Policy: ghattest.Policy{Repository: selfupdate.Repository{Owner: "example", Name: "relay"}},
    GH:     "/usr/local/bin/gh",
})
if err != nil {
    return err
}
cfg.ManifestVerifiers = append(cfg.ManifestVerifiers, v)
```

- **What it checks:** the attestation's source is your repository at the
  release's tag (`refs/tags/<tag>`), its signer is this module's publish
  workflow (`Policy.SignerWorkflow`, optionally pinned to commits by
  `SignerDigests`), it was made on a GitHub-hosted runner unless
  `Policy.AllowSelfHostedRunners` is set, and it names
  the downloaded `SHA256SUMS`, which pins every asset's digest. It runs
  before any binary is downloaded. `ghattest.NewVerifier` checks the asset
  itself instead.
- **What it needs:** `gh`, by an absolute path, logged in even for a public
  repository. `Options.Env` is `gh`'s environment (nil: this process's),
  and `Options.Timeout` bounds each run (default 2 minutes).
- **It fails closed:** a missing or logged-out `gh`, a verification that
  fails, a timeout, or an attestation that does not meet the whole policy
  each fail the update with `ErrIntegrity`.
- **What it does not stop:** a malicious commit and `v*` tag pushed through
  your workflow, which attests it like any other. Restrict who can create
  `v*` tags (the building guide's step 4).

Pointers:

- `ghattest.ExampleNewManifestVerifier`
- `selfupdate/verify/ghattest/live_test.go`: the real `gh`, when
  `SELFUPDATE_REQUIRE_GHATTEST=1`
- [0017-MADR](../decisions/0017-MADR-verify-build-provenance-and-close-0015-open-items.md), item 1

## Probe the new binary

`Config.Probes` run the staged binary before anything is replaced. On
Windows the staging file is named `.exe`, and elsewhere it is made executable
first. `InstallOptions.PostInstall` runs the installed binary before the
replacement is committed, and rolls it back on failure.
`NewVersionProber(args, want, timeout)` runs the binary with `args` and
requires its stdout to contain the release's version.
`NewVersionProberWith(VersionProbeOptions{Args, Want, Timeout, Run})`
(since `v1.16.0`) is the same with an optional `Run`, which replaces how
the binary is started, for a program that runs probes its own way or tests
them without a process; the timeout and the 64 KiB cap on stdout still
apply.

An `Unpacker`, then a `Transformer` such as a re-signing step, run before
the probes, so the probes see the bytes that will be installed.
`selfupdate.ChainTransformers` runs several transformers in order, such as
the codesign signer and one of your own.

- `ExampleNewVersionProberWith`, `ExampleChainTransformers`
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
  if err != nil {
      return err
  }
  cfg.Transformer = signer
  ```

  - **`Identifier` is required.** Without it, `codesign` names the
    signature after the staging file, `.relay`, not after your program.
  - After signing, the signer verifies the signature and requires that
    identifier, and `Requirement` too when you set it. Since `v1.10.1`
    each is checked in its own `codesign --verify` run, so a
    `Requirement` cannot undo the identifier check.
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
- `selfupdate/codesign/live_darwin_test.go`: the real `codesign`, ad hoc
  when `SELFUPDATE_REQUIRE_CODESIGN=1`, and with your own identity when
  `SELFUPDATE_CODESIGN_IDENTITY` names it

## Rehearse without installing

`Request.DryRun` does everything short of the install: download, verify,
unpack, transform and probe. It asks nobody, then discards the staging file. The
`Result` has `DryRun` set and both digests. A dry run may name another
`Request.Platform`; this host cannot run that platform's binary, so it
skips `Config.Probes` and sets `Result.ProbesSkipped`.

- `selfupdate/lifecycle_test.go`, `selfupdate/testdata/golden/text-dry-run.golden`

## Keep the previous binary

`InstallOptions.KeepPrevious` renames the replaced binary to `.<base>.previous`
beside the target at commit, over any older one, and reports it in
`Result.Previous`. It keeps no setuid or setgid bit
([below](#replace-a-setuid-or-setgid-binary)). On Windows this works while
the old image is still running. `StandaloneInstaller.CleanupPending`,
called at startup, finishes
what an earlier update left behind. Since `v1.6.0` that includes the staging
files and backups of an update that crashed: under the lock they belong to
no one. An `ErrConcurrentUpdate` from it means another update holds the
lock, which is benign.

When an update cannot restore the previous binary after the new one went
live, it reports the backup in `Result.PendingBackup` as the only copy of
the previous binary. Since `v1.10.1` that backup is
`.<base>.selfupdate-kept-<n>`, which no later session removes: restore or
remove it yourself. A dry run removes nothing.

Since `v1.12.0` an update interrupted between replacing the target and
committing, by a crash or a kill, keeps its backup too: the update's
journal, `.<base>.selfupdate.pending`, tells the next session, which keeps
the backup under the same kept name. `StandaloneInstaller.KeptBackups`
lists every kept backup, so call it after `CleanupPending`. A service
program holds a `*ManagedInstaller`, which has its own `KeptBackups`;
`CleanupPending` is the inner `StandaloneInstaller`'s, so keep that one to
call it.

- `ExampleStandaloneInstaller_CleanupPending`
- `selfupdate/journal_test.go`, `selfupdate/lifecycle_windows_test.go`,
  `selfupdate/cleanup_busy_windows_test.go`

## Replace a setuid or setgid binary

A target with the setuid or setgid bit is refused, so an update never
silently grants or drops privileges. Set
`TargetPolicy.AllowSpecialModeBits` to replace it: the new binary keeps the
bits. The sticky bit is always kept. On Unix the new binary also gets the
old one's owner and group, when the updater may give them; an updater that
may not, such as one run by the file's owner without root, keeps its own.
A previous binary kept beside the target, by `KeepPrevious` or as a kept
backup after a failed restore or an interrupted update, loses setuid and
setgid, so no old copy is left privileged: a restore from it must set them
again.

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
  configured to start:
  - systemd: a `UnitFileState` of `enabled` or `enabled-runtime`, not
    `static` or `indirect`, though `systemctl is-enabled` succeeds for
    them;
  - launchd: `RunAtLoad` or `KeepAlive`, with the label not disabled in
    launchd's override database (`launchctl disable`, which
    `launchctl enable` undoes);
  - the SCM: an automatic start type, or a demand start with start
    triggers when `scm.Options.TriggerStartEnabled` is set;
  - Task Scheduler: an enabled task with an enabled logon, boot or
    repeating time trigger.

  Otherwise the binary is replaced and the service stays stopped. An `Enabled` error fails the
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

Since `v1.7.0`, `selfupdate/service` and its backends are the
`Lifecycle` a managed install needs, written once for each service manager:
systemd, launchd and the SCM since `v1.7.0`, Task Scheduler since
`v1.14.0`. Why each behaves as it does is in
[0011-MADR](../decisions/0011-MADR-reference-service-lifecycles.md) and,
for Task Scheduler,
[0021-MADR](../decisions/0021-MADR-task-scheduler-lifecycle-and-windowless-children.md).

- **Pick the backend:**
  - `systemd.New(systemd.Options{Unit: "relay.service"})`, with
    `Scope: systemd.User` for a user unit;
  - `launchd.New(launchd.Options{Label: …, Domain: launchd.System(),
    Plist: …})`, or `launchd.GUI(uid)` or `launchd.User(uid)` for an agent;
  - `scm.New(scm.Options{Name: "relay"})`;
  - `taskscheduler.New(taskscheduler.Options{Name: "relay"})`, for a
    per-user task ([Task Scheduler](#task-scheduler));
  - `service.NewSupervised(service.SupervisedOptions{…})`, since
    `v1.22.0`, for runit, s6, OpenRC or any other supervisor on Linux
    ([Runit, s6, OpenRC and other supervisors](#runit-s6-openrc-and-other-supervisors))
- **Use it twice.** Each is a `selfupdate.Lifecycle`, an
  `EnabledLifecycle` and a `Reconciler`:
  `selfupdate.NewManagedInstaller(inner, b, b)`. Each backend's `ExampleNew`
  shows it.
- **Errors and waits.** A backend's error wraps one of `service`'s typed
  errors, which `errors.Is` matches: `ErrNotInstalled`, `ErrUnsupported`,
  `ErrInsideService`, `ErrTimeout`, `ErrUnhealthy` or `ErrPermission`.
  Since `v1.16.0`, a failed `systemctl`, `systemd-run`, `launchctl` or
  exec-reconciler command is also a `*service.CommandError`, which
  `errors.As` reads for the step, the exit code and the message; the scm
  and taskscheduler backends fail through Windows API errors, and return
  none. `Options.Poll` bounds the waits: `Timeout` 60 s, `Interval` 250 ms
  and `Settle` 10 s by default (`DefaultPollTimeout`, `DefaultPollInterval`,
  `DefaultPollSettle`). `Options.Probe` is a `service.HealthProbe`, a
  function that observes the service once and returns a `service.Health`;
  an error from it is kept, and reported if the wait times out.
  `service.ExamplePollHealthy` shows a probe and the wait.
- **What it does around the replace:**
  - `Stop` returns once the service has stopped, not when the stop was
    asked for: systemd `inactive` or `failed`; launchd out of the domain
    with its process gone; the SCM's `STOPPED`, by Microsoft's wait-hint
    loop; for Task Scheduler, the recorded process gone, by its process ID
    and creation time and, since `v1.15.1`, by its process object's
    signalled state, so a process that has exited reads as gone though
    another process still holds a handle to it. Once the stop is issued,
    the wait does not end with your context. On systemd and launchd it
    runs to the service manager's own kill bound, the longer of
    `Options.Poll.Timeout` and launchd's `ExitTimeOut` (5 s when unset) or
    systemd's `TimeoutStopUSec`, plus 30 s. On the SCM and on Task
    Scheduler it is `Options.Poll.Timeout`. Task Scheduler spends
    `Options.StopGrace`, the wait for a program asked to exit by
    `Options.RequestStop`, inside that timeout, so since `v1.15.2` `New`
    refuses a `StopGrace` you set that is not shorter than it, and a
    default grace is the shorter of 10 s and half the timeout.
  - A stop that fails after the service stopped, such as a wait that timed
    out, starts the service again; the update fails with
    `ErrManagedInstall`, and the binary is not replaced.
  - `WaitHealthy` requires a new instance (a new systemd `InvocationID`,
    a new process ID) that stays up for `Options.Poll.Settle`, 10 s by
    default, then runs `Options.Probe` if you set one. systemd's restart
    count is read after the start, so a start during a `RestartSec` wait,
    which systemd counts as a restart, is not a failure (since `v1.10.1`).
  - `Reconcile` checks that the definition still runs the binary, and
    changes nothing. `Options.RewritePath` lets it point the definition at
    a binary that moved. A program that owns its definition through its own
    install command uses `service.NewExecReconciler` instead:
    `ExecOptions.Reconcile` are the arguments that make the new binary
    reconcile its definition and print a `service.Receipt`, one JSON object
    whose `verdict` is `none`, `unchanged`, `refreshed` or `kept`; and
    `ExecOptions.Restore`, or `RestoreFunc` in the updater itself, restores
    a receipt that `changed`. One of the two is required. The state kept
    for the restore is an `ExecState`. Since `v1.20.0`, the command's
    environment leaves out your unit's or job's own variables, as a
    handoff's does
    ([Updating from inside the service](#updating-from-inside-the-service)),
    and `ExecOptions.Env` passes on whatever it names. Since `v1.10.1`, a
    rewrite:
    - on systemd, checks the unit's effective `ExecStart` after the
      reload; a drop-in that loads later and sets it again, such as an
      `override.conf`, is an error, and recovery removes this package's
      drop-in;
    - on launchd, reloads a job that is loaded but not running, so
      launchd reads the new plist, and refuses a job that is running;
    - on the SCM, reads an unquoted command line whose path holds a
      space the way the SCM runs it, and refuses one that could name two
      programs: quote it.

    Task Scheduler's rewrite (since `v1.14.0`) sets the `Exec` action's
    path field, and parses no command line.
  - Since `v1.11.0`, `New` refuses `Options.Poll` whose settle window is
    not shorter than its timeout, launchd's at least 10 s, since every
    wait would time out (`PollOptions.Validate`); and on the SCM the
    dependents `StopDependents` stopped are started again after the
    service.
  - `Enabled` on launchd counts `RunAtLoad` only as `<true/>`; launchd
    does not run a job at load for an integer, even 1. A plist `plutil`
    cannot read is an error (since `v1.10.1`).
  - On systemd, a start that fails because a unit this one requires is
    missing is not `service.ErrNotInstalled` (since `v1.10.1`).
- **systemd units: `Type=notify` or `Type=exec`.** With either, a failed
  start fails `systemctl start`. With `Type=simple`, it succeeds before the
  program has run. Under `Type=notify`, call `systemd.Ready()` once the
  program serves; `systemd.WatchdogInterval` and `systemd.Watchdog` serve
  `WatchdogSec=`, and `systemd.Status`, `Stopping` and `Reloading` send the
  other notifications. `systemd.Available` reports whether systemd runs
  here, for a program that picks its backend at run time;
  `TemplateInstance`, `Escape` and `ValidUnitName` build and check a
  template unit's instance name, such as `relay@main.service`.
- **No library process start runs a `.bat` or `.cmd` file** (since
  `v1.14.0`). On Windows, `cmd.exe` parses a batch file's arguments by
  rules no quoting can make safe (the BatBadBut class), so the version
  probe, `service.ExecRunner`, and so `service.NewExecReconciler` and
  `ghattest` through their default runner, and the handoff refuse one.
  The error wraps `selfupdate.ErrBatchFile` (since `v1.15.0`), so
  `errors.Is` tells it from any other failure. The check reads the name
  Windows will run: `x.cmd`, found for a path with no extension; and,
  since `v1.15.0`, `x.bat.` or `x.bat` with a trailing space, which
  Windows reads as `x.bat`, and `x.bat::$DATA`. Point them at an `.exe`: a
  `gh.cmd` shim as `ghattest.Options.GH` is refused too.

### Task Scheduler

Since `v1.14.0`, `selfupdate/service/taskscheduler` manages one task your
program's setup registered: typically a per-user task that starts the
program at logon and needs no administrator. It drives Task Scheduler
through its COM API, never `schtasks.exe` or PowerShell. Since `v1.19.0`
it also registers, reads, compares and deletes the definition your setup
renders, and reads a task's state in one call
([0032-MADR](../decisions/0032-MADR-task-scheduler-registration-and-one-shot-handoff.md)); it never
renders the definition itself. Since `v1.21.0` it switches the task on
and off ([0034-MADR](../decisions/0034-MADR-export-task-scheduler-enable-and-disable.md)), so
your setup needs no `schtasks.exe` at all.

```go
task, err := taskscheduler.New(taskscheduler.Options{
    Name:        "relay",        // in Folder; `\` when empty
    RewritePath: true,           // follow a binary that moved
    RequestStop: askRelayToExit, // optional: a polite stop
})
if err != nil {
    return err
}
m, err := selfupdate.NewManagedInstaller(inner, task, task)
if err != nil {
    return err
}
```

Off Windows, `New` returns `service.ErrUnsupported`.

- **`Options.Folder`** is the task's folder, from the root: `\` (the
  default) or a path such as `\Vendor\Product`. The task is
  `<Folder>\<Name>`.
- **What the setup registers:**
  - exactly one `Exec` action, the program; `Reconcile` refuses any other
    shape;
  - a logon trigger;
  - a time trigger that repeats, every minute or so, with
    `MultipleInstancesPolicy` `IgnoreNew`, as the watchdog that starts the
    program again after it exits. `RestartOnFailure` is not a watchdog: it
    was measured not to restart a program that exits, whatever its
    documentation says (magic-cli-remote's
    `docs/decisions/0159-MADR-the-windows-service-path-is-half-wired.md`,
    probe 8);
  - an `InteractiveToken` principal, if you set `RewritePath`: a rewrite of
    a task with any other logon type is refused, since only that one was
    measured to keep its principal.
- **`Stop`** disables the task first, so the watchdog cannot start the
  program while its binary is replaced. It then calls
  `Options.RequestStop`, when set, and waits up to `Options.StopGrace` (10 s
  by default, or half of `Options.Poll.Timeout` when that is shorter) for
  the program to exit on its own; then, if it still runs, it ends the
  task, and waits for the program's process to exit. The grace is spent
  inside `Options.Poll.Timeout`: since `v1.15.2`, `New` refuses a
  `StopGrace` you set that is not shorter than it, with `RequestStop`
  set. Without `RequestStop`, the
  program is ended at once, with no chance to clean up. Ending the task
  ends only the program's own process: children it started keep running,
  so stop them in your `RequestStop` handler.
- **`Start`** enables the task again when `Stop` disabled it, and runs it.
  Task Scheduler can still list a process that has exited; `Start` does
  not count the instance `Stop` recorded once that process has gone.
  `Start` knows only its own `Task`'s `Stop`: after a `Stop` in another
  process, such as your setup's `stop` command, the task stays disabled
  and `Start` returns `ErrTaskDisabled`. Call `Enable` first (since
  `v1.21.0`).
- **A task you disabled yourself** cannot be run: Task Scheduler refuses
  to run a disabled task, even with `TASK_RUN_IGNORE_CONSTRAINTS`
  (`SCHED_E_TASK_DISABLED`, measured). So since `v1.15.0`, `Stop` refuses
  a disabled task whose program runs, before it changes anything, with an
  error wrapping `taskscheduler.ErrTaskDisabled`: the program keeps
  running, and nothing is replaced. Enable the task (`Enable`, since
  `v1.21.0`), or stop the program, then update. A disabled task whose program does not run is left alone,
  and not started after the update. `Start` on a disabled task returns
  `ErrTaskDisabled` too. Before `v1.15.0`, such an update ended the
  program and left it stopped
  ([0022-MADR](../decisions/0022-MADR-documentation-pass-after-v1-14-0.md),
  Amendment A1).
- **If the update crashes between `Stop` and `Start`,** the task stays
  disabled and the watchdog does not run it. Your setup's repair registers
  it again, or `Enable` re-enables it (since `v1.21.0`; before, `schtasks
  /change /tn <folder>\<name> /enable`). A `Stop` that fails while the program still runs, such as a wait
  that timed out, enables the task again before it returns (since
  `v1.15.2`), so the task is left as it was; before, it stayed disabled
  too.
- **`WaitHealthy`** needs a new process running for `Options.Poll.Settle`,
  then runs `Options.Probe`. A program that exits first fails the check at
  once, with its exit code in the detail.
- **A rollback after a rewrite** calls `Restore`, which registers the
  definition `Reconcile` saved, its `DefinitionBackup`, again, byte for
  byte, from the process that ran the update.
- **Registering the task** (since `v1.19.0`). `Register` registers or
  replaces the XML your setup renders, in `<Folder>\<Name>`, creating the
  folder when it is missing, for your account with no password.
  `Definition` reads it back as Task Scheduler stored it, and `Delete`
  removes it; a task, or a folder, that is already gone is not an error.
  Task Scheduler stores a definition in another form than it was given,
  so compare with `DiffDefinitions`, never as text:

  ```go
  stored, err := task.Definition(ctx, "relay")
  switch {
  case errors.Is(err, service.ErrNotInstalled):
      err = task.Register(ctx, "relay", wanted)
  case err == nil:
      var diffs []taskscheduler.DefinitionDiff
      if diffs, err = taskscheduler.DiffDefinitions(stored, wanted); err == nil && len(diffs) > 0 {
          if !force {
              return fmt.Errorf("the task differs from this version's: %v", diffs)
          }
          err = task.Register(ctx, "relay", wanted)
      }
  }
  if err != nil {
      return err
  }
  ```

  - `Register` refuses, before Task Scheduler sees it, a principal you
    cannot register for yourself without a password: `S4U`,
    `Password`, `InteractiveTokenOrPassword`, a `GroupId`, or a service
    account. It removes the XML declaration, since Task Scheduler refuses
    one that says `UTF-8`; you need no UTF-16 file.
  - `Delete` does not stop a running program, which outlives the task:
    call `Stop` first.
  - `Register` sets the task's switch from the definition: one that
    leaves `Settings/Enabled` out, or sets it `true`, enables a task that
    `Stop` or `Disable` disabled. To keep the task disabled, call
    `Disable` after `Register`.
  - `DiffDefinitions` compares every trigger and action, with every
    element they hold, the principal's user, group, logon type and run
    level, and every setting but `IdleSettings` and
    `UseUnifiedSchedulingEngine`, which Task Scheduler adds, and, since
    `v1.21.0`, `Enabled`. That is the task's switch, which Task Scheduler
    writes into the stored definition when `Stop`, `Enable` or `Disable`
    sets it, so a stopped task is not reported as changed. An element
    left out compares as its default; a SID equals the account it names;
    durations and booleans compare as values. `RegistrationInfo` is not
    compared. Each difference is a `DefinitionDiff`: a path such as
    `Settings/DisallowStartIfOnBatteries`, and the two values.
- **`Status`** (since `v1.19.0`) reads the task's whole state in one call:
  whether it is installed, `Enabled`'s and `Running`'s answers, its state
  by name (`ready`, `running`, `disabled`, ...), its running processes,
  and its last run's time and result. A task that is not installed is a
  zero `Status`. Since `v1.21.0`, `Disabled` is the task's switch, off. A
  task disabled while its program runs has the state `running`, so only
  `Disabled` tells it.
- **Switching the task on and off** (since `v1.21.0`). `Enable` and
  `Disable` set the task's switch, and do nothing to a task already so.
  A setup's start and stop commands, each in a process of its own:

  ```go
  // start: undo a stop made by an earlier process, then run.
  if err := task.Enable(ctx, "relay"); err != nil {
      return err
  }
  return task.Start(ctx, "relay")
  ```

  ```go
  // stop: the task stays disabled, so the watchdog does not restart it.
  return task.Stop(ctx, "relay")
  ```

  - `Disable` does not end a running program; `Stop` does. Until the
    program exits, an update's `Stop` refuses the task with
    `ErrTaskDisabled`.
  - Both forget that this `Task`'s `Stop` disabled the task, so `Start`
    leaves the switch as you set it.
  - Errors wrap `service.ErrNotInstalled` for a task that does not exist,
    and `service.ErrPermission` when Task Scheduler denies the change.
    Each checks `ctx` before it starts.
- **Updating from inside the task** works as for the other backends:
  `service.HandOffFunc(task, service.HandOff{Args: os.Args[1:]})`. Since
  `v1.19.0`, `Detach` registers and runs a one-shot task,
  `<Name>.selfupdate.<id>`, in the task's folder, with the task's own
  user, logon type and run level. Task Scheduler starts its process, so
  the update is in none of your program's jobs: it survives the task's
  end, and a job of yours that kills its processes when it closes, which
  the old hop did not. The service task must log on with an interactive
  token. `CleanupHandOffs` removes finished one-shot tasks; `Detach` calls
  it first, and you may at start-up. `Options.HopDetach` restores the hop,
  a detached process in your program's jobs.
- **The console window.** A console program run by a task has a visible
  console window in your session. Building with `-H windowsgui` removes
  it, but breaks a CLI's output in PowerShell: redirected output is empty,
  and the exit code is lost (magic-cli-remote's
  `docs/decisions/0159-MADR-the-windows-service-path-is-half-wired.md`,
  probe 7). The library's own children
  never add a window of their own (see the handoff's Windows line below).

Pointers:

- `taskscheduler.ExampleNew`
- `selfupdate/service/taskscheduler/lifecycle_test.go`, over a fake Task
  Scheduler; `live_windows_test.go`, against the real one

### Runit, s6, OpenRC and other supervisors

Since `v1.22.0`, `service.NewSupervised` is the backend for a supervisor
this module has none for, on Linux
([0035-MADR](../decisions/0035-MADR-serve-supervisors-through-a-supervised-adapter.md)). You supply three
functions:

- `State` reads the service once;
- `Up` asks the supervisor to start it and keep it up;
- `Down` asks it to stop it and not restart it.

The adapter does the rest, as the other backends do. The result is a
`Lifecycle`, an `EnabledLifecycle`, a `Reconciler` and a `Detacher`. The
library parses none of your supervisor's output, so the parsing is yours.
An s6 service, whose `s6-svstat -o` is documented as machine-readable:

```go
svc := "/home/<user>/.local/share/s6/service/relay"
run := service.ExecRunner()
s6 := func(ctx context.Context, tool string, args ...string) (service.Output, error) {
    return run.Run(ctx, service.Command{Path: "/usr/bin/" + tool, Args: append(args, svc), Env: os.Environ()})
}
ctl := func(flag string) func(context.Context) error {
    return func(ctx context.Context) error {
        out, err := s6(ctx, "s6-svc", flag)
        if err == nil && out.ExitCode != 0 {
            err = fmt.Errorf("s6-svc %s: exit %d", flag, out.ExitCode)
        }
        return err
    }
}
sv, err := service.NewSupervised(service.SupervisedOptions{
    Name: "relay",
    State: func(ctx context.Context) (service.SupervisedState, error) {
        _, derr := os.Stat(filepath.Join(svc, "down"))
        st := service.SupervisedState{Installed: true, NormallyUp: errors.Is(derr, fs.ErrNotExist)}
        out, err := s6(ctx, "s6-svstat", "-o", "up,wantedup,pid")
        if err != nil {
            return st, err
        }
        if out.ExitCode == 1 { // s6-supervise not running on the directory
            return st, service.ErrNoSupervisor
        }
        f := strings.Fields(string(out.Stdout)) // e.g. "true true 4242"
        st.Supervised, st.Up, st.WantUp = true, f[0] == "true", f[1] == "true"
        st.Restarting = !st.Up && st.WantUp
        if st.Up {
            st.PID, _ = strconv.Atoi(f[2])
        }
        return st, nil
    },
    Up:    ctl("-u"),
    Down:  ctl("-d"),
    Kill:  ctl("-k"),
    Probe: relayReady, // your application check, as for the other backends
})
if err != nil {
    return err
}
m, err := selfupdate.NewManagedInstaller(inner, sv, sv)
```

`selfupdate/service/supervised_live_linux_test.go` drives this against a
real s6 tree.

- **What you supply in `State`:**
  - `Installed`, `Supervised`, `Up`, the service's own `PID` while it is
    up, and `WantUp`;
  - `NormallyUp`, which is `Enabled`'s answer: for runit and s6, no `down`
    file; for OpenRC, the service in the `default` runlevel;
  - `Restarting`: down while the supervisor wants it up;
  - `Failed`: failed for good, such as OpenRC's `crashed`;
  - `Ready`, only when the service reports readiness. s6 reads `false`
    for a service with no `notification-fd`, so leave `Ready` nil
    there.

  When nothing supervises the service, return an error wrapping
  `service.ErrNoSupervisor`, with a state that still says `Installed`
  and `NormallyUp`. Start a supervisor in `Up` when your program owns
  one for the session.
- **What each supervisor printed** (0035-MADR Amendment A1):
  - **runit 2.1.2:** `sv status <dir>` prints `run: <dir>: (pid N) Ns`,
    `down: <dir>: Ns, normally up, want up` while restarting, and exits
    1 with `unable to open supervise/ok` when no `runsv` serves the
    directory.
  - **s6 2.12:** `s6-svstat -o up,wantedup,pid` prints `true true N`. It
    exits 1 when no `s6-supervise` runs, and `s6-svc` then exits 100.
  - **OpenRC 0.63:** `rc-service <svc> status` exits 0 when started, 3
    when stopped and 32 when crashed. No command prints the PID, so read
    the PID file your script writes.
- **What the library guarantees:**
  - `Stop` refuses, before anything changes, when this process is the
    service's process or descends from it. It counts a process by PID
    and start time, so a reused PID never counts.
  - `Stop` calls `Down`, then waits to `Poll.Timeout` even if your
    context ends, and returns the context's error after. The timeout
    counts from before `Down`, so a `Down` that blocks, as
    `rc-service stop` does, spends part of it.
  - `Kill`, when set, runs once at `KillAfter`, half the timeout by
    default: `sv kill <dir>` or `s6-svc -k <dir>`, since runit's
    `sv down` sends TERM only.
  - A stop that fails while the process runs calls `Up` again, so the
    supervisor still wants the service up. `Up` gets a context of its
    own, `Poll.Timeout` long, so a failed `Stop` can take twice that.
    Before `v1.22.1`, this `Up` ran on the stop's spent context, and a
    command it ran never started.
  - `WaitHealthy` needs a new instance up, and ready where `Ready` is
    set, for `Poll.Settle`. It fails with `ErrUnhealthy` when the
    service:
    - fails;
    - restarts during the wait;
    - exits after it was up;
    - or keeps restarting for longer than the settle window.

    One restart before the first up is tolerated. Then `Probe` runs.
- **Updating from inside the service** works as for the other backends,
  with `service.HandOffFunc(sv, service.HandOff{Args: os.Args[1:]})`.
  The run starts through the hop, so it no longer descends from the
  service once the hop exits. Call `service.ReportFunc` or
  `LoadHandOffEnv` first in `main`.
  - `DetachCheck` refuses a handoff the supervisor would kill: an OpenRC
    system service with `rc_cgroup_cleanup=YES`, whose cgroup holds the
    detached run.
  - A runit or s6 stop signals only the service's process, and an
    OpenRC user service has no cgroup of its own, so neither needs a
    check (Amendment A1, P2 to P4).
- **`Reconcile`** checks that the service ran the executable when it was
  stopped, and rewrites nothing. A program whose setup renders the
  service's run script or init script passes `service.NewExecReconciler`
  as the reconciler instead.
- **Off Linux,** `NewSupervised` returns `service.ErrUnsupported`.

### Replace before the stop

By default a managed install stops a running service, replaces its binary,
then starts it. Since `v1.13.0`, it can replace the binary first, while the
old instance still runs, then stop, reconcile, start and check, as Debian's
`dh_installsystemd --restart-after-upgrade` and Teleport's updater do:

```go
m, err := selfupdate.NewManagedInstallerWith(inner, b, b,
    selfupdate.ManagedOptions{ReplaceBeforeStop: true})
```

- **What it buys:** a replacement or post-install probe that fails does so
  before the stop, so it costs no downtime at all: the binary is rolled
  back under the running service, which is not stopped or restarted. The
  replace itself is only a few tens of milliseconds of downtime either way
  ([0020-MADR](../decisions/0020-MADR-precheck-gofmt-errors-and-replace-before-stop.md)).
- **Use it only when all of these hold**, because the library cannot check
  them:
  - the service never starts its own executable while it runs, such as a
    worker it spawns from `os.Executable()`: after the replace, that path
    runs the new version under the old one;
  - no stop hook runs the program, such as systemd's `ExecStop=` or
    `ExecStopPost=`: it would run the new binary against the old instance;
  - an unplanned restart between the replace and the stop, by systemd's
    `Restart=`, launchd's `KeepAlive`, an SCM failure action or a Task
    Scheduler repeating trigger (the task is disabled only inside `Stop`),
    may start the new binary before its health check. The planned stop, start and
    health check then follow as usual.
- **Recovery:** a stop that fails rolls the binary back too: under a
  service that still runs, which is not restarted, or before a service
  that went down is started again on the previous binary. Later failures
  recover as without the option.
- **The result says which order ran:** `Result.ReplacedBeforeStop`, and
  `replaced_before_stop` in the JSON result, are true when the binary was
  replaced while the service ran. A service that was stopped, or not
  installed, has nothing to stop, so the option changes nothing for it.

Pointers:

- `ExampleNewManagedInstallerWith` in `selfupdate`
- `selfupdate/managed_replacefirst_test.go`

### Updating from inside the service

A process the service started, such as an agent running a remote session,
is inside the service. When it runs `<prog> update --yes`, every
backend's `Stop` refuses before it changes anything, with an error wrapping
`service.ErrInsideService`, and the update ends with an error that matches
both it and `selfupdate.ErrManagedInstall`: the service keeps running the
old binary, and with `ReplaceBeforeStop` the replaced binary is rolled
back. A `Lifecycle` of your own without that check would kill the update
halfway. The handoff runs that update outside the service instead, as a
detached copy of the same command:

```go
func main() {
    report := service.ReportFunc() // first: see below
    ctx := context.Background()
    unit, err := systemd.New(systemd.Options{Unit: "relay.service"})
    if err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
    o := cli.StdioOptions()
    o.HandOff = cli.HandOff{
        Detach: service.HandOffFunc(unit, service.HandOff{Args: os.Args[1:]}),
        Report: report,
    }
    os.Exit(cli.Command(ctx, os.Args[1:], "relay", buildinfo.Identity(), newUpdater, o))
}
```

`newUpdater` is your function that builds the `Updater`, as for any
`cli.Command`.

- **The handoff needs the name** in `Options.Unit`, `Options.Label` or
  `Options.Name`.
- **Without `cli`,** call `service.HandOffIfInside(ctx, backend,
  service.HandOff{Args: os.Args[1:]})` before the update: when it reports
  that it handed off, the returned `Detached` names the run and its
  `ResultPath`, and this process does not update. In the detached run,
  write the outcome with the function `service.ReportFunc` returns, or
  `service.WriteHandOffResult`. `service.HandOffFunc` is the same call,
  shaped for `cli.HandOff.Detach`. `HandOffIfInside` checks neither
  `--yes` nor whether the run would install anything: `HandOffFunc`
  refuses a run without `Yes` inside the service, and `cli` runs one with
  nothing to install in place. Without `cli`, make both checks yourself
  before you call it.
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
  {"schema_version":1,"id":"3f2a…","started_at":"…","finished_at":"…","exit_code":0,"result":{"schema_version":4,…,"applied":true,"service_started":true,"rolled_back":false,…}}
  ```

  (`result` is abridged here: it is the whole `--json` result document.)

  `exit_code` is the update's, `error` its message when it failed, and
  `result` the `--json` result document; since `v1.11.0` its
  `rolled_back` says the new binary failed its health check and the
  previous one runs. The service never hands off the check: `--check` and
  `--dry-run` run in place, and since `v1.11.0` neither does a run with
  nothing to install.
- **Call `service.ReportFunc` first in `main`,** or `LoadHandOffEnv` if the
  program reads its arguments or environment before that. In a detached
  run it applies the private environment file the launchd and Task
  Scheduler handoffs write, and on Windows it completes the start, which
  goes through a short-lived hop. A Task Scheduler run names its file in
  its first argument, `--selfupdate-handoff-env=<path>`
  (`service.ArgHandOffEnv`), because a task cannot carry an environment
  variable; `LoadHandOffEnv` removes that argument from `os.Args`, so
  your flag parsing never sees it. Neither
  takes a context; a program that needs its context to bound the wait for
  the hop calls `service.LoadHandOffEnvContext(ctx)`, or
  `service.HandOffHopContext(ctx)`, first (since `v1.16.0`).
- **The run is not your service** (since `v1.20.0`). Its environment is
  yours without the variables the service manager sets for your unit or
  job, which `service.ManagerEnvNames` lists: systemd's that describe
  the unit's run, its channels to the manager, the manager and why the
  unit ran (`INVOCATION_ID`, `NOTIFY_SOCKET`, `LISTEN_FDS`,
  `WATCHDOG_USEC`, `JOURNAL_STREAM`, `MAINPID`, `TRIGGER_UNIT` and the
  rest), `RUNTIME_DIRECTORY` and `CREDENTIALS_DIRECTORY`, which go with
  your unit, and launchd's `XPC_SERVICE_NAME` and `XPC_FLAGS`. Before,
  a systemd run had your unit's `INVOCATION_ID` and `JOURNAL_STREAM` in
  place of its own unit's, and your unit's `NOTIFY_SOCKET`; a launchd
  run had your job's `XPC_SERVICE_NAME`. Your user's and session's
  variables, and `STATE_DIRECTORY`, `CACHE_DIRECTORY`, `LOGS_DIRECTORY`,
  `CONFIGURATION_DIRECTORY` and `TMPDIR`, still pass on. A variable you name in `HandOff.Env` passes on
  whatever its name. For a process your program starts itself,
  `service.ScrubManagerEnv(os.Environ())` leaves out the same names
  ([0033-MADR](../decisions/0033-MADR-scrub-service-manager-environment-from-handoffs.md)).
- **The detached run cannot prompt,** so the update must have `--yes`, as
  an agent's does. Since `v1.11.0`, an update from inside the service
  without it is refused with `ErrConfirmationRequired` (exit 1), even when
  this run could ask.
- **Per platform:**
  - **systemd:** a transient unit, `<unit>-selfupdate-<id>.service`,
    started by `systemd-run`, needs systemd 236 or later. System scope
    needs root.
  - **launchd:** a one-shot job, `<label>.selfupdate.<id>`. Call
    `Job.CleanupHandOffs` at start-up to remove finished ones; the next
    handoff does it too.
  - **Windows (SCM):** a detached process, out of the service's job where
    the job allows it. Since `v1.14.0` it has a console with no window,
    in place of no console at all, so console programs it starts do not
    open a window either.
  - **Task Scheduler:** since `v1.19.0`, a one-shot task,
    `<Name>.selfupdate.<id>`, in the task's folder, whose environment is in
    a file in `Options.HandOffDir` (`%LOCALAPPDATA%\selfupdate\handoff`),
    private to your account. Its own console window shows for the tens of
    milliseconds its hop runs; the update has none. Call
    `Task.CleanupHandOffs` at start-up to remove finished ones; the next
    handoff does it too. With `Options.HopDetach`, the detached process of
    the SCM line, which a kill-on-close job of your program's would end.
  - **runit, s6, OpenRC and other supervisors** (since `v1.22.0`):
    `service.Supervised`, whose run starts through the hop; see
    [Runit, s6, OpenRC and other supervisors](#runit-s6-openrc-and-other-supervisors).
  - **A service manager with neither:**
    `service.ProcessDetacher(inside)`, with your own check of whether this
    process is inside. On Unix it is no defence against a cgroup kill or a
    launchd job's reap.

Pointers:

- `ExampleHandOff` in `selfupdate/cli`
- `selfupdate/service/systemd/live_linux_test.go`,
  `selfupdate/service/launchd/live_darwin_test.go`,
  `selfupdate/service/scm/live_windows_test.go`,
  `selfupdate/service/taskscheduler/live_windows_test.go`: each runs an
  update from a process the service started, end to end, against the real
  service manager. Each skips unless its switch is set:
  `SELFUPDATE_REQUIRE_SYSTEMD` (with `SELFUPDATE_SYSTEMD_SCOPE=user` for a
  user unit), `SELFUPDATE_REQUIRE_LAUNCHD`, `SELFUPDATE_REQUIRE_SCM`, which
  needs an administrator session, and `SELFUPDATE_REQUIRE_TASKSCHED`,
  which needs a signed-in console. `TestLiveTaskUpdate` also requires the
  update to run at medium integrity, which a GitHub runner does not give
  it, so CI skips that one test

## Use another release host

`GitHubSource` is one `ReleaseSource`. A program whose releases live
elsewhere implements the interface for that host and passes it as
`Config.Source`; every other seam in this guide works the same over it.

- **`Latest` and `ByTag`** return a `Release`, and an error that wraps
  `ErrNoRelease` when the release asked for does not exist, so
  `CheckCached` can save that answer. `OpenAsset` opens an asset's body.
- **The field rules** are on `Release` and `Asset`:
  - a release is `Immutable` only when the host keeps its files
    unchanged, since the Updater refuses a mutable one with
    `ErrMutableRelease`; one that is a `Draft` is never installed;
  - each asset has a positive `ID`; a `Name` that is the exact filename,
    a basename with no separator or control character; `State`
    `AssetStateUploaded`; a positive `Size`, exactly what `OpenAsset`
    returns; and a `Digest` that is empty or `sha256:<64 hex>`, which the
    downloaded bytes must then match.
- **Channels** need `ReleaseLister` too: `ListReleases` lists releases,
  prereleases included. A source without it is refused on a channel.
- **`WithCredentials`** needs a `CredentialedSource`: its
  `WithCredentials(p)` returns a copy whose provider is `p`, with fresh
  credential state, and leaves the receiver unchanged. A source without it
  makes such a run fail.

Since `v1.16.0`, `selfupdatetest.VerifyReleaseSource` checks a source
against those rules, in the source's own tests:

```go
func TestSourceConforms(t *testing.T) {
    src := newTestSource(t) // your source, against a test server
    err := selfupdatetest.VerifyReleaseSource(t.Context(), src,
        selfupdatetest.SourceCase{LatestTag: "v1.2.0", MissingTag: "v9.9.9"})
    if err != nil {
        t.Fatal(err)
    }
}
```

It requires `Latest` to return `LatestTag`, immutable and not a draft;
`ByTag` to return the same release for it, and an error that wraps
`ErrNoRelease` for `MissingTag`; every asset to meet the rules above; and
`OpenAsset` to return exactly `Size` bytes, whose SHA-256 is `Digest` when
it is set. It returns every violation, joined.

Pointers:

- `selfupdatetest.ExampleVerifyReleaseSource`
- `go doc github.com/maccavelli/go-selfupdate-lib/selfupdate Release`, and
  `Asset`: each field's rule
- [0025-MADR](../decisions/0025-MADR-align-with-go-sdk-design-practices.md)
  D11

## Evolve with the library

Within `v1` the library only adds, and an interface you implement, such as
`Lifecycle`, `InstallSession` or `ReleaseSource`, never gains a method. A
new capability arrives as an optional interface your type may also
implement: since `v1.6.0` a `Lifecycle` that also implements
`EnabledLifecycle` reports whether the service is configured to start, and
a managed update then starts a stopped one that is; a `Lifecycle` without
it keeps the earlier behaviour, starting only a service that was running. The rules are
in [architecture.md, API evolution](../architecture.md#api-evolution), and
what each release adds is in the [changelog](../../CHANGELOG.md).

## Test a program that self-updates

The `selfupdate/selfupdatetest` package provides:

- `NewRelease`, which builds a release with matching `SHA256SUMS`;
- `NewFakeSource(latest, releases…)`, an in-memory source whose `Calls`
  lists what it answered, and whose missing release wraps
  `selfupdate.ErrNoRelease` (since `v1.15.2`), as GitHub's does;
- `RecordingReporter`;
- `ScriptedConfirmer`;
- `NewGitHubServer(t, owner, repo, releases…)`, a fake GitHub API on one
  TLS origin whose asset requests redirect to a second. `RateLimit` and
  `TruncateAssets` make it rate-limit and truncate, and `Requests` lists
  what it received. `RequireToken`
  requires a bearer token; since `v1.6.0`, `RequireCredential(header,
  value)` requires a custom-header credential exactly as the library sends
  it. It logs every request with whether it carried `Authorization`, and,
  in `CredentialHeaders`, the names of the credential headers it carried,
  never their values. So a test can prove a credential stays off the
  download origin;
- `VerifyReleaseSource(ctx, src, SourceCase{LatestTag, MissingTag})`
  (since `v1.16.0`), which checks a `ReleaseSource` for another host
  against the contract the Updater relies on
  ([Use another release host](#use-another-release-host)).

Pointers:

- `selfupdatetest.ExampleNewFakeSource`,
  `selfupdatetest.ExampleVerifyReleaseSource`,
  `selfupdate/selfupdatetest/selfupdatetest_test.go`
- `selfupdate/e2e_github_test.go`, `selfupdate/golden_test.go`,
  `selfupdate/credential_header_e2e_test.go`
