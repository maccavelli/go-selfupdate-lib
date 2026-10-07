# Architecture

How `go-selfupdate-lib` is put together, as it is now. This file carries no
history and no rationale: the records under [decisions/](decisions/) hold the
argument, and [README.md](README.md) indexes them.

## What it is

A Git repository for the Go module `github.com/maccavelli/go-selfupdate-lib`,
the fleet's self-update library: `buildinfo` and `selfupdate` at the top,
each `selfupdate` subpackage in a directory under `selfupdate/`, no root
package, and no released binary. `internal/cmd/selfupdate-release` is a
tool the workflows build from source. It was `go-core-lib` up to `v1.4.1`.
It also hosts the two reusable GitHub Actions workflows that programs using
`selfupdate` build and publish their releases through.

The module requires Go 1.27.1 and three modules: `golang.org/x/mod v0.40.0`,
`golang.org/x/sys v0.47.0` and `golang.org/x/term v0.43.0`. Its current
release is `v1.10.0`, an annotated tag on commit
`a0a26b6ecf66f51c19e9fea0f665c76ca5e99e4c`. `v1.5.0`, the first under this
path, is the annotated tag on commit
`6deaa524cfb28aad90bea97a6d9162e5b4257204`.

## Tree

```text
README.md                   repository entry; links here
LICENSE                     Apache License 2.0
AGENTS.md                   rules for agents: dependencies, records, checks, commits
go.mod, go.sum              the module and its three requirements
Makefile                    development targets (below)
.golangci.yml               golangci-lint configuration
.markdownlint-cli2.jsonc    Markdown lint configuration
.gitattributes              LF line endings, except the byte-exact parity fixtures
.github/workflows/
  ci.yml                    CI
  build-selfupdate-release.yml     reusable build workflow (workflow_call):
                            builds, checks, packs and stages from a spec
  publish-selfupdate-release.yml   reusable release workflow (workflow_call)
scripts/
  gate.sh                   the full pre-commit gate, one line per step
                            (make gate)
  check-docs.sh             link and anchor check, and the identifier
                            check against the pre-push guard's deny list
  plant-copy.sh             copies the tree with one planted break, so a
                            new test is seen to fail outside the tree
  go-precheck.sh            the pre-add check
  verify-selfupdate-release.sh     validates a staged release set
  selfupdate_manifest.py    the verifier's SHA256SUMS parser, as a module;
                            the differential test calls it too
  refuse-existing-release.sh       refuses a tag that already has a release
  release-latest-flag.sh    the latest-flag rule: a stable tag below the
                            current latest is published --latest=false
  check-release-tag.sh      the tag rule: strict, or a listed prerelease
                            channel; the workflow and the verifier call it
  check-installers.sh       lints the installer templates, or a staged
                            release's installers: shellcheck, dash -n and
                            PSScriptAnalyzer
  check-workflows.sh        parses workflows as YAML: no ${{ }} in a run script,
                            every repository-scoped gh step sets GH_REPO,
                            a top-level permissions block, and every action
                            pinned to a commit SHA
  workflow-shape_test.sh    holds the two reusable workflows' step order,
                            guards and permissions in place
  check-api-compat.sh       fails on an incompatible exported API change
                            against the newest v1.* tag (apidiff)
  go-fuzz.sh                fuzzes each fuzz target of a package in turn
  requirements-workflow-check.txt  hash-pinned PyYAML for check-workflows.sh
  *_test.sh                 offline tests for each of those scripts
.claude/ .grok/ .opencode/  per-agent pointers to AGENTS.md
opencode.json
buildinfo/                  the library-owned build stamps
selfupdate/                 the self-update package
  cli/                      the canonical update command
  selfupdatetest/           its exported test doubles
  archive/                  tar.gz, zip and gz release assets: selection
                            and extraction
  codesign/                 opt-in macOS re-signing and signature checks
  releasespec/              the release spec: products, platforms, packaging,
                            extras, channels and installers, for the program
                            and CI
  service/                  what the reference service lifecycles share,
                            and the handoff
    systemd/                the systemd lifecycle, and sd_notify
    launchd/                the launchd lifecycle
    scm/                    the Windows SCM lifecycle
internal/cmd/selfupdate-release/  the workflows' logic: plan, build, stage,
                            check, identity, installer; never released
  installer/                the install.sh and install.ps1 templates, each a
                            working script with a values block to fill
  testdata/fixture/         a module of its own: the relay program the
                            tests and CI's rehearsal build
docs/
  README.md                 record index and the "I want to…" table
  architecture.md           this file
  decisions/                MADR and PLAN records
  reports/                  REPORT records
  guides/                   how-to guides
```

## Go code

| Directory | Package | Non-test files | Test files | Non-standard imports |
| :--- | :--- | :--- | :--- | :--- |
| `buildinfo/` | `buildinfo` | 1 | 2 | none |
| `selfupdate/` | `selfupdate` | 41 | 76, including five fuzz targets, plus `testdata/SHA256SUMS.{valid,invalid}`, 23 `testdata/manifest-parity/` cases and 14 `testdata/golden/` files | `x/mod/semver`, `x/sys/unix`, `x/sys/windows`, `x/term` |
| `selfupdate/cli/` | `cli` | 4 | 9, plus 53 `testdata/golden/` and 9 `testdata/migration/` files | `x/term` (and `selfupdate`, `buildinfo`) |
| `selfupdate/selfupdatetest/` | `selfupdatetest` | 2 | 1 | none (`selfupdate` itself) |
| `selfupdate/archive/` | `archive` | 3 | 6, including three fuzz targets, plus a real GoReleaser `testdata/` checksum file | none (`selfupdate`) |
| `selfupdate/codesign/` | `codesign` | 2 | 4 | none (`selfupdate`, `service`) |
| `selfupdate/releasespec/` | `releasespec` | 4 | 5, including a fuzz target, plus 4 `testdata/` specs | none (`selfupdate`, `archive`) |
| `selfupdate/service/` | `service` | 16 | 13 | `x/sys/windows` (and `selfupdate`) |
| `selfupdate/service/systemd/` | `systemd` | 8 | 12, plus 3 `testdata/` captures of `systemctl show` | none (`selfupdate`, `service`) |
| `selfupdate/service/launchd/` | `launchd` | 6 | 11, plus 7 `testdata/` captures of `launchctl print` and `list` | none (`selfupdate`, `service`) |
| `selfupdate/service/scm/` | `scm` | 7 | 9 | `x/sys/windows`, `x/sys/windows/svc`, `x/sys/windows/svc/mgr` (and `selfupdate`, `service`) |
| `internal/cmd/selfupdate-release/` | `main` | 11, plus the 2 `installer/` templates | 10, plus the `testdata/fixture/` module (2 programs, 2 specs, an extra) | none (`buildinfo`, `selfupdate`, `archive`, `releasespec`) |

- `selfupdate` began as `mcplib` `v1.6.0`'s `selfupdate` (commit
  `4e1f9a53e265`), and its `v1.0.x` API is that package's. It differs from
  that source in:
  - its import path and package comment;
  - four Windows-only lint fixes;
  - the fixes of
    [0003-MADR](decisions/0003-MADR-remediate-debugging-pass-findings.md)
    and of [0004-MADR](decisions/0004-MADR-evolve-selfupdate-api-and-tui-support.md)
    Phase 0;
  - the additive Phase 1 API of that MADR, which `make apicheck` keeps
    compatible with `v1.0.1`.
- The Phase 1 API, by file:
  - **asking without installing:** `checker.go` (`Checker`, shared with
    `Run`'s discovery) and `checkcache.go` (`CheckCached`, `CheckStore`,
    `NewFileCheckStore`, and since `v1.6.0` `CheckOutcome`, which caches
    the deterministic errors);
  - **output:** `jsonreporter.go`, `document.go` (`Result.Document`),
    `adapters.go` (the `…Func` adapters, `DiscardReporter`,
    `MultiReporter`, `NonInteractiveConfirmer`), and the progress and
    outcome events in `updater.go`;
  - **credentials:** `credentials.go`, with the lazy credential chain and
    its cross-origin stripping in `github.go`;
  - **integrity and probes:** `manifestverify.go`, `imageverify.go`
    (ELF, Mach-O, PE), and `probe.go` (staged and post-install probes);
  - **installers:** `TwoPhaseSession`, `StagingOwner` and
    `NewManagedInstallerFor` (`types.go`, `session.go`, `managed.go`), and
    `DryRun`, `KeepPrevious` and `CleanupPending` (`updater.go`,
    `session.go`, `standalone.go`).
- The Phase 2 core API:
  - **per-run options:** `runoptions.go` (`RunOption`, `RunWith`,
    `WithReporter`, `WithConfirmer`, `WithCredentials`,
    `WithProgressInterval`). An unexported `run` embeds the `Updater` and
    shadows its source, reporter, confirmer and progress interval, and the
    run's methods take it as their receiver. `GitHubSource.WithCredentials`
    gives a run its own copy of the source, client included;
  - **the interaction stream:** `stream.go` (`Start`, `Stream`,
    `Progressed`, `ConfirmNeeded`, `CredentialNeeded`, `Finished`). It is
    a mutex-guarded queue that coalesces only trailing progress, with a
    one-slot wake channel and a channel closed after `Finished`.
    `PromptCredential` finds the run's `Stream` through the run's context.
- Prerelease channels
  ([0005-MADR](decisions/0005-MADR-opt-in-prerelease-channels.md)):
  - **the grammar:** `version.go` (`NewSemverPolicy`, `SemverOptions`),
    whose policy is a `ChannelPolicy` (`types.go`, with `ValidChannel`
    and `Admits`);
  - **the opt-in:** `Request.Channel` and `CheckRequest.Channel`, checked
    by `validateRequest`; the cache file is schema 3, which keys the
    channel and holds the cached outcome; a record of an older schema
    reads as a miss (`checkcache.go`);
  - **listing:** `ReleaseLister` and `ListOptions` (`types.go`), and
    `GitHubSource.ListReleases`, which pages `releases?per_page=30` up to
    90 releases by default and 300 at most (`github.go`);
  - **discovery:** `checker.go`. With a channel it takes the highest
    admitted release on its tag and flags alone, then checks that one in
    full; a failure is an error, never a fallback. Under a
    `ChannelPolicy`, a release whose prerelease flag disagrees with its
    tag is refused on every request, the stable channel included.
- The Phase 3 command surface
  ([0004-PLAN-v1-4-0-command-surface.md](decisions/0004-PLAN-v1-4-0-command-surface.md)):
  - **`buildinfo`** (standard library only) owns two linker variables,
    named by `VersionVar` and `KindVar`. `Identity` reports a release
    only for the stamped kind `release` with a `vX.Y.Z` or
    `vX.Y.Z-NAME.N` tag, and reads `debug.ReadBuildInfo` for display
    only;
  - **`selfupdate/cli`**: `Flags` (`Bind`, `Parse`, `Request`), `Run`,
    `Exit`, `Summary`, `StdioOptions`, and `Command`, which is all of
    them in order. `Run` wraps the context with the signals and the
    timeout, picks the text or JSON reporter, and writes the summary
    line or the one result object. Nothing in the package names
    `os.Stdout` outside `StdioOptions`, which a test checks;
  - **`selfupdate.UserAgent`** (`useragent.go`) builds
    `product/version (goos/goarch)` for `GitHubOptions.UserAgent`.
- `selfupdatetest` provides `NewRelease`, `FakeSource`,
  `RecordingReporter`, `ScriptedConfirmer`, and `GitHubServer`, a fake
  GitHub API on one TLS origin whose asset requests redirect to a second,
  which can require a bearer token or a custom-header credential, and
  records the credential headers' names on each request.
- The reference service lifecycles
  ([0011-MADR](decisions/0011-MADR-reference-service-lifecycles.md)):
  - **`service`:** the command runner (`runner.go`), the six typed errors
    (`errors.go`), `PollHealthy` (`poll.go`), `ExecReconciler` and its
    version 1 receipt (`execreconciler.go`), and the handoff: `Detacher`,
    `HandOffIfInside`, `HandOffFunc`, `ReportFunc` and the result file
    (`handoff.go`), `DetachProcess` (`detach*.go`), the private
    environment file (`handoffenv.go`) and the Windows hop
    (`handoffhop.go`);
  - **`systemd`:** probes over one `systemctl show` per call, read by key;
    `Stop`, `Start` and `WaitHealthy` on the unit's state and
    `InvocationID`, with the restart count read after the start; a
    drop-in for a moved binary, checked against the effective `ExecStart`
    after the reload; `Inside` by cgroup; the handoff by `systemd-run`,
    its environment file under a directory per unit; and `Notify`
    (`notify.go`);
  - **`launchd`:** `launchctl` exit codes, `list`, and `print`'s top-level
    `pid` and `state` lines only; `Stop` waits until the job has left the
    domain and its process has exited; `plutil` for the plist, its values
    read with their types; a rewritten plist reloaded when the job is
    loaded but not running; `Inside` by process group or ancestry; the
    handoff by a one-shot job;
  - **`scm`:** an unexported interface over the SCM and the process table,
    which the tests fake; handles opened with only each call's rights;
    Microsoft's wait-hint and checkpoint loop; an unquoted command line
    read as `CreateProcess` reads it; `Inside` by an ancestor
    walk that checks creation times; the handoff by `DetachProcess`
    through a hop.

  Each backend compiles on every OS and returns `ErrUnsupported` on the
  wrong one. Each has live tests against its real service manager, which
  CI runs on its OS.
- `cli.Options.HandOff` (`run.go`) runs `Detach` before an apply and
  `Report` after an update that ran.
- Archive assets and macOS signing
  ([0012-MADR](decisions/0012-MADR-archive-assets-and-macos-codesign.md)):
  - **in `selfupdate`:** the extract stage, which runs after the
    `Verifiers` and before the transform (`Unpacker`, `UnpackRequest`,
    `Config.Unpacker`, `Selection.Packed` and `EventUnpacking` in
    `types.go`, the stage in `updater.go`); `CheckImage`
    (`imageverify.go`); `ChainTransformers` (`transform.go`);
  - **`archive`:** `NewSelector` and its naming helpers (`select.go`), and
    `NewUnpacker` (`unpack.go`), which refuses what two readers could
    read differently and checks the program's image;
  - **`codesign`:** `NewSigner`, a `Transformer`, and `NewChecker`, a
    `Prober`, over `/usr/bin/codesign` through `service.Runner`
    (`codesign.go`). It is opt-in: nothing in the module imports it.
- The coordinator (`updater.go`) owns the order of every step. It validates
  the selected binary and manifest itself, and parses `SHA256SUMS` before any
  staging. It pins an exact `--version`, and closes the session before
  reporting `complete`, the run's one terminal event. An error after that
  point is a `warning` event and an entry in `Result.Warnings`
  (`warnings.go`), not a failure. A check that succeeds, and a run that
  finds the program up to date, install nothing and end at `selected`: the
  result's `Operation` is the outcome, and no `complete` follows.
- The install path (`session.go`, `replace_*.go`, `lock*.go`, `cleanup*.go`,
  `managed.go`):
  - locks the target directory through `os.Root`, refusing a symlinked lock
    (`openLockFile`);
  - checks that the locked directory is the one at the path before it reads
    a cleanup receipt, and re-checks it, by the handle's identity, before
    the replace, after it and before commit;
  - undoes the rename through the directory handle when the directory
    changed after it;
  - refuses a staging path that is not a regular file, and a target that is
    no longer the file it resolved, replaced by another file or a symlink
    since `Begin` (`ErrConcurrentUpdate`);
  - reports a failed restore with the backup's path, in
    `Result.PendingBackup` with `Applied` false, and in the error, and
    keeps that backup as `.<base>.selfupdate-kept-<n>`, which no later
    session removes; reports `RolledBack` whenever the previous binary is
    back in place, its directory sync failed or not;
  - finishes a replacement with one `Commit` or one `Rollback`, refusing a
    second;
  - runs the restore after a failed directory sync, and managed recovery,
    on contexts the caller's cancellation does not reach;
  - removes, under the lock, the staging files and backups a crashed
    update left beside the target (`leftovers.go`), but never a backup a
    cleanup receipt still lists, a kept backup, or anything that is not a
    regular file; a dry run removes nothing;
  - refuses a setuid or setgid target unless
    `TargetPolicy.AllowSpecialModeBits` allows it, and gives the new binary
    the old one's mode, sticky bit included, and on Unix its owner and
    group where permitted;
  - starts a managed service after the update only when it was running, or
    an `EnabledLifecycle` reports it configured to start, and recovery
    restarts only what was running or what the update started; a stop
    that fails after the service stopped starts it again.

  On Windows, a busy running image is retried until the installer's lock
  timeout or the caller's context ends. An access-denied error on a
  read-only destination is not retried. A cleanup receipt may name only a
  backup of its own target, which is hashed and removed through the
  directory handle.
- The terminal confirmer reads one line per answer, a byte at a time, and
  leaves no read outstanding once a prompt is answered.
- Platform code is split by build tag: `*_unix.go` (`//go:build unix`),
  `*_windows.go`, and `cleanup_other.go` for non-Windows receipt handling.
- The package's GitHub `User-Agent` is supplied by the program;
  `UserAgent` builds one. It reads
  `GH_TOKEN`, then `GITHUB_TOKEN`, when set.

## Build workflow

`build-selfupdate-release.yml` is called with `spec-path`, and optionally
`module-dir` (default `.`), `extras-artifact-name`, `artifact-name` and
`retention-days` (default 7). Its token is `contents: read`. Its `build`
job, on `ubuntu-24.04`:

1. checks out its own commit at `tools/` and the caller's source, at the
   event's ref and SHA, at `src/`, neither with persisted credentials;
2. sets up the source module's Go, cache off, and builds the tool;
3. `plan`: reads the spec, picks a release (on a tag) or a rehearsal, and
   writes the publish inputs, the artifact name and the identity matrix;
   on a tag, `check-release-tag.sh` with the spec's channels;
4. `build`: per product and platform, requires `buildinfo` among the
   package's dependencies, then builds with the fixed recipe
   (`CGO_ENABLED=0`, `-trimpath`, `-buildvcs=true`, `-s -w` and the
   `buildinfo` stamp, `GOFLAGS=-mod=readonly`, `GOENV=off`,
   `GOTOOLCHAIN=local`, `GOWORK=off`);
5. `stage`: checks each binary's build information (platform, tags, cgo,
   `-trimpath`, commit, clean tree, toolchain, module and package, and the
   tag as the main module's version at the repository root) and its
   image; packs archives with fixed metadata and unpacks each with the
   client's unpacker; writes `SHA256SUMS` and parses it back; copies the
   extras; when the spec has `installer`, renders `install.sh` (for a
   platform other than Windows) and `install.ps1` (for a Windows one) from
   the embedded templates, for the calling repository (`-repository`) and
   the stamp, refusing any value outside `[A-Za-z0-9._:=/,+@%-]`;
6. runs the publish workflow's verifier on the staged set, and uploads it,
   with the tool cross-compiled for the identity runners.

Its `identity` job runs each staged program with the product's
`identity_args`, on its own platform's runner (`ubuntu-24.04`,
`ubuntu-24.04-arm`, `macos-15`, `windows-2025`, `windows-11-arm`), and
requires `<tag> (release)` (`rehearsal-<sha12> (local)` off a tag) as its
first line. Its outputs are `artifact-name`, `tag`, `rehearsal` and the
four publish inputs.

## Release workflow

`publish-selfupdate-release.yml` is called with `artifact-name`,
`products-json`, `platforms-json` and `extra-assets-json`, and optionally
`prerelease-channels-json` (default `[]`). On a tag ref, in order, it:

1. checks out its own commit at `.core-lib-release-tools`, to run the scripts
   above from the same commit as the workflow;
2. requires an admitted tag (`check-release-tag.sh`): a strict
   `vMAJOR.MINOR.PATCH`, or `vMAJOR.MINOR.PATCH-NAME.N` for a `NAME` in
   `prerelease-channels-json`, the rule `NewSemverPolicy` applies;
3. refuses an existing release (`refuse-existing-release.sh`), after
   proving the repository itself is readable;
4. downloads the caller's artifact to `staging/`;
5. validates the staged set (`verify-selfupdate-release.sh`): regular files
   only, safe extra names, and a `SHA256SUMS` parsed exactly as the client
   parses it. Both parsers run the fixtures in
   `selfupdate/testdata/manifest-parity/`. A `format` on every platform
   object makes the release one of archives,
   `<product>-<os>-<arch>.<format>`, and `SHA256SUMS` lists those;
6. for a release of archives only, sets up Go from its own `go.mod` and
   runs `selfupdate-release check`, which reads each tar.gz and gz to its
   end with gzip (one member, its checksum, nothing after it), then
   unpacks each archive with the client's own unpacker and checks the
   program's image;
7. creates a draft, uploads the files (one argument each), attests them,
   publishes, and waits for the release to be immutable and verified. A
   prerelease tag is created with `--prerelease --latest=false`, so it
   never becomes the release stable clients read. A stable tag below the
   current latest release, such as a backport, is created and published
   with `--latest=false` too (`release-latest-flag.sh`), so clients keep
   seeing the newest one.

Every `gh` step that acts on the calling repository sets `GH_REPO`. Every
`run:` block reads the ref from `env:` (`TAG`, `REF_TYPE`); no `${{ }}` is
interpolated into shell.

## Tooling

- **`make` targets:** `test`, `test-sum`, `fmt`, `vet`, `lint`, `tidy`,
  `vuln`, `apicheck`, `fuzz`, `pre-add-check`, `help`.
- **`make fuzz`** runs `scripts/go-fuzz.sh` on `selfupdate`,
  `selfupdate/archive` and `selfupdate/releasespec`. In each it finds every
  fuzz target, refuses fewer than the package holds (five, three and one),
  and fuzzes each for `FUZZTIME` (default 20s), with minimization capped
  at 5 s. A failing input stays in the package's
  `testdata/fuzz/<Name>/`, where it is a seed from then on.
- **The running-copy end-to-end tests** (`selfupdate/e2e_running_test.go`).
  - The test builds a small helper twice, as `v1.0.0` and `v1.1.0`,
    with the version stamped by `-ldflags -X`, and starts the v1 build
    from the target.
  - `Updater.Run` then updates it through `GitHubServer` (a redirect and
    a token), the image verifier, both version probes and the standalone
    installer.
  - **Linux and macOS:** the commit is clean.
  - **Windows:** the running image keeps a pending backup and receipt.
    `CleanupPending` keeps both, without an error, while the old process
    runs, and clears them once it has exited.
  - Six refusals each leave the running v1 byte-identical.
  - A channel case serves a stable `v1.2.0` and a prerelease
    `v1.3.0-rc.1` under `NewSemverPolicy`: on `rc` the running copy
    becomes the rc build, and on the stable channel `v1.2.0`.
- **The manifest differential.** `TestManifestDifferential` generates 5,000
  manifests from a fixed seed. Each must be accepted or rejected alike, with
  the same entries, by `ParseSHA256SUMS` and by the verifier's own parser,
  `scripts/selfupdate_manifest.py`, run once through `python3`.
  - It skips without `python3`, unless `SELFUPDATE_REQUIRE_PYTHON=1`.
  - `SELFUPDATE_DIFFERENTIAL_N` and `SELFUPDATE_DIFFERENTIAL_SEED` widen a
    hunt.
- **`make apicheck`** runs `scripts/check-api-compat.sh`: the pinned
  `apidiff` compares the working tree with the newest `v1.*` tag (or
  `BASE=`), and any incompatible change fails it.
- **`make lint`** runs `golangci-lint run -c .golangci.yml ./...` three
  times: `GOOS=linux`, `darwin` and `windows`, each with `CGO_ENABLED=0`.
- **Import rules** are 14 `depguard` rules in `.golangci.yml`
  ([0008-MADR](decisions/0008-MADR-enforce-import-rules-with-depguard.md)):
  - `banned`: mcplib, the MCP go-sdk, go-llmprovider-sdk and Charm, in
    every file;
  - `module`: only the standard library, this module, `x/mod`, `x/sys`
    and `x/term`, in every file;
  - `buildinfo`, `selfupdate`, `selfupdate-cli`, `selfupdatetest`,
    `service`, `service-launchd`, `service-scm`, `service-systemd`,
    `selfupdate-archive`, `selfupdate-codesign` and
    `selfupdate-releasespec`: each package's own allowed imports, outside
    its tests;
  - `other-packages`: any other package, `internal/` included, only the
    standard library and this module.
- **`scripts/go-precheck.sh`** runs `gofmt` on the given Go files, the same
  three golangci-lint runs, `go vet` and `go test` on their packages, each
  in the module that owns it (a nested module, such as the release tool's
  fixture, with `go -C`), and `govulncheck ./...`. `make pre-add-check` runs it, and so does the
  machine-wide agent gate before an agent `git commit` that stages Go files.
- **`make gate`** runs `scripts/gate.sh`, every check a commit needs, in
  order: `gofmt`, `make lint`, `go vet`, `go test -race`, `go test
  -shuffle=on -count=2`, `go mod tidy -diff`, `make apicheck`, `make fuzz`,
  `make vuln`, every script test, `shellcheck`, the cross-target `go vet`,
  the link check over every tracked Markdown file, and the identifier check
  on the changed files. Each step's output goes to `GATE_OUT`, and it prints
  one `<step> rc=<N>` line; `GATE_SKIP` skips named steps. Records cite it
  in place of the session gate the 0010–0014 PLANs ran
  ([0015-PLAN](decisions/0015-PLAN-remediate-third-debugging-pass-findings.md)
  R1).
- **`scripts/plant-copy.sh FILE OLD NEW`** copies the tracked and untracked
  files with one planted break, for a new test to be seen failing outside
  the tree.
- **`.golangci.yml`** enables `revive`'s `exported`, `package-comments` and
  `var-naming` rules in place of `golint`. Test files are exempt from
  `errcheck`, `gosec`, `unparam`, `revive`, `gocritic` and `goconst`.
- **CI** (`.github/workflows/ci.yml`) runs on Linux, macOS and Windows, with
  the Go version read from `go.mod`.
  - **Every OS:** `go test`, plus, under bash, the refuse-existing-release
    test. The installer tests must reach the runner's shells
    (`SELFUPDATE_INSTALL_REQUIRE_SHELLS`): dash and bash on Linux, `sh`
    and bash on macOS, Windows PowerShell 5.1 and PowerShell 7 on
    Windows.
  - **Linux and macOS:** `go test -race`, with
    `SELFUPDATE_REQUIRE_PYTHON=1`, so the differential runs rather than
    skips.
  - **The live tests,** each required, so a skip fails:
    - Linux: "ownership as root", the staging-owner test under `sudo`
      (`SELFUPDATE_REQUIRE_ROOT`); the systemd live tests, in system scope
      under `sudo` and in the runner's user scope
      (`SELFUPDATE_REQUIRE_SYSTEMD`);
    - macOS: the launchd live tests (`SELFUPDATE_REQUIRE_LAUNCHD`) and the
      codesign live tests (`SELFUPDATE_REQUIRE_CODESIGN`);
    - Windows: the SCM live tests (`SELFUPDATE_REQUIRE_SCM`).
  - **Linux also:** a full-history checkout; `go test -shuffle=on -count=2`;
    the fuzz script's test, then `make fuzz`, with the corpus uploaded as
    an artifact when it fails;
    `go vet` for `freebsd/amd64`, `openbsd/amd64` and `linux/386`;
    `go vet`, `gofmt`, `go mod tidy -diff`, `make lint` (golangci-lint
    v2.14.0); `make apicheck` and the API gate's test; `govulncheck` v1.8.0;
    `shellcheck` v0.11.0 (the latest release, pinned by SHA-256 and first
    on `PATH`, so actionlint's embedded checks use it too),
    `markdownlint-cli2` 0.23.2 and `actionlint` v1.7.12; the installer
    lint script's test, then the script on both templates, with the
    runner's PSScriptAnalyzer; the verifier's
    fixture test; the release tag rule's test; the latest-release rule's
    test (`release-latest-flag_test.sh`); the pre-add gate's test
    (`go-precheck_test.sh`); the workflow checker, with
    every rule on both reusable workflows and `expressions`,
    `permissions` and `pins` on `ci.yml`, its test, and the workflow shape
    test; the full gate's test, the document checker's and the plant
    helper's, and the link check over every tracked Markdown file.
  - **The release rehearsal:** two calls of `build-selfupdate-release.yml`
    by its local path on the release tool's fixture, one of raw binaries
    and one of archives, each with its identity runs on the five runners;
    then a job that checks both staged sets as the publish workflow
    would, without publishing, and checks their installers as rendered
    for this repository and the stamp. On a branch or pull request it
    rehearses; on a `v*` tag it builds the fixture as that release.
  - **The installer containers:** `install.sh`'s tests, built and staged
    on the runner, then run by the static test binary in `alpine:3.24.2`
    (BusyBox ash and wget, musl) and `buildpack-deps:trixie-curl` (dash,
    curl), each pinned by digest, as the runner's user.
  - One run per ref (`concurrency`, cancel in progress). Actions are pinned
    to commit SHAs.

## What is not here

- **Any consumer's migration,** and `mcplib`'s deprecation of its own copy.
  Each is recorded in that repository.
- **Release signing.** No publisher signature is verified; the
  `ManifestVerifier` hook is where one would be.
