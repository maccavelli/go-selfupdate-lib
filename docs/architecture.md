# Architecture

How `go-selfupdate-lib` is put together, as it is now. This file carries no
history and no rationale: the records under [decisions/](decisions/) hold the
argument, and [README.md](README.md) indexes them.

## What it is

A Git repository for the Go module `github.com/maccavelli/go-selfupdate-lib`,
the fleet's self-update library: one package per top-level directory, with
no root package and no binary. It was `go-core-lib` up to `v1.4.1`. It also
hosts the reusable GitHub Actions workflow that programs using `selfupdate`
publish their releases through.

The module requires Go 1.27.1 and three modules: `golang.org/x/mod v0.40.0`,
`golang.org/x/sys v0.47.0` and `golang.org/x/term v0.43.0`. Its current
release is `v1.6.0`, an annotated tag. `v1.5.0`, the first under this path,
is the annotated tag on commit `6deaa524cfb28aad90bea97a6d9162e5b4257204`.

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
  publish-selfupdate-release.yml   reusable release workflow (workflow_call)
scripts/
  go-precheck.sh            the pre-add check
  verify-selfupdate-release.sh     validates a staged release set
  selfupdate_manifest.py    the verifier's SHA256SUMS parser, as a module;
                            the differential test calls it too
  refuse-existing-release.sh       refuses a tag that already has a release
  check-release-tag.sh      the tag rule: strict, or a listed prerelease
                            channel; the workflow and the verifier call it
  check-workflows.sh        parses workflows as YAML: no ${{ }} in a run script,
                            and every repository-scoped gh step sets GH_REPO
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
| `selfupdate/` | `selfupdate` | 39 | 57, including five fuzz targets, plus `testdata/SHA256SUMS.{valid,invalid}`, 23 `testdata/manifest-parity/` cases and 12 `testdata/golden/` files | `x/mod/semver`, `x/sys/unix`, `x/sys/windows`, `x/term` |
| `selfupdate/cli/` | `cli` | 4 | 7, plus 41 `testdata/golden/` and 9 `testdata/migration/` files | `x/term` (and `selfupdate`, `buildinfo`) |
| `selfupdate/selfupdatetest/` | `selfupdatetest` | 2 | 1 | none (`selfupdate` itself) |

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
    by `validateRequest`; the cache file is schema 2, which keys the
    channel, and still reads schema 1 (`checkcache.go`);
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
- The coordinator (`updater.go`) owns the order of every step. It validates
  the selected binary and manifest itself, and parses `SHA256SUMS` before any
  staging. It pins an exact `--version`, and closes the session before
  reporting `complete`, the run's one terminal event. An error after that
  point is a `warning` event and an entry in `Result.Warnings`
  (`warnings.go`), not a failure.
- The install path (`session.go`, `replace_*.go`, `lock*.go`, `cleanup*.go`,
  `managed.go`):
  - locks the target directory through `os.Root`, refusing a symlinked lock
    (`openLockFile`);
  - checks that the locked directory is the one at the path before it reads
    a cleanup receipt, and re-checks it, by the handle's identity, before
    the replace, after it and before commit;
  - undoes the rename through the directory handle when the directory
    changed after it;
  - refuses a staging path that is not a regular file;
  - reports a failed restore with the backup's path, in
    `Result.PendingBackup` with `Applied` false, and in the error;
  - runs the restore after a failed directory sync, and managed recovery,
    on contexts the caller's cancellation does not reach;
  - removes, under the lock, the staging files and backups a crashed
    update left beside the target (`leftovers.go`), but never a backup a
    cleanup receipt still lists or anything that is not a regular file;
  - refuses a setuid or setgid target unless
    `TargetPolicy.AllowSpecialModeBits` allows it, and gives the new binary
    the old one's mode, sticky bit included, and on Unix its owner and
    group where permitted;
  - starts a managed service after the update only when it was running, or
    an `EnabledLifecycle` reports it configured to start, and recovery
    restarts only what was running or what the update started.

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
   `selfupdate/testdata/manifest-parity/`;
6. creates a draft, uploads the files (one argument each), attests them,
   publishes, and waits for the release to be immutable and verified. A
   prerelease tag is created with `--prerelease --latest=false`, so it
   never becomes the release stable clients read.

Every `gh` step that acts on the calling repository sets `GH_REPO`. Every
`run:` block reads the ref from `env:` (`TAG`, `REF_TYPE`); no `${{ }}` is
interpolated into shell.

## Tooling

- **`make` targets:** `test`, `test-sum`, `fmt`, `vet`, `lint`, `tidy`,
  `vuln`, `apicheck`, `fuzz`, `pre-add-check`, `help`.
- **`make fuzz`** runs `scripts/go-fuzz.sh` on `selfupdate`. It finds
  every fuzz target, refuses fewer than five, and fuzzes each for
  `FUZZTIME` (default 20s), with minimization capped at 5 s. A failing
  input stays in `selfupdate/testdata/fuzz/<Name>/`, where it is a seed from
  then on.
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
- **Import rules** are six `depguard` rules in `.golangci.yml`
  ([0008-MADR](decisions/0008-MADR-enforce-import-rules-with-depguard.md)):
  - `banned`: mcplib, the MCP go-sdk, go-llmprovider-sdk and Charm, in
    every file;
  - `module`: only the standard library, this module, `x/mod`, `x/sys`
    and `x/term`, in every file;
  - `buildinfo`, `selfupdate`, `selfupdate-cli` and `selfupdatetest`: each
    package's own allowed imports, outside its tests.
- **`scripts/go-precheck.sh`** runs `gofmt` on the given Go files, the same
  three golangci-lint runs, `go vet` and `go test` on their packages, and
  `govulncheck ./...`. `make pre-add-check` runs it, and so does the
  machine-wide agent gate before an agent `git commit` that stages Go files.
- **`.golangci.yml`** enables `revive`'s `exported`, `package-comments` and
  `var-naming` rules in place of `golint`. Test files are exempt from
  `errcheck`, `gosec`, `unparam`, `revive`, `gocritic` and `goconst`.
- **CI** (`.github/workflows/ci.yml`) runs on Linux, macOS and Windows, with
  the Go version read from `go.mod`.
  - **Every OS:** `go test`, plus, under bash, the refuse-existing-release
    test.
  - **Linux and macOS:** `go test -race`, with
    `SELFUPDATE_REQUIRE_PYTHON=1`, so the differential runs rather than
    skips.
  - **Linux also:** a full-history checkout; `go test -shuffle=on -count=2`;
    the fuzz script's test, then `make fuzz`, with the corpus uploaded as
    an artifact when it fails;
    `go vet` for `freebsd/amd64`, `openbsd/amd64` and `linux/386`;
    `go vet`, `gofmt`, `go mod tidy -diff`, `make lint` (golangci-lint
    v2.14.0); `make apicheck` and the gate's own test; `govulncheck` v1.8.0;
    `shellcheck` v0.11.0 (the latest release, pinned by SHA-256 and first
    on `PATH`, so actionlint's embedded checks use it too),
    `markdownlint-cli2` 0.23.2 and `actionlint` v1.7.12; the verifier's
    fixture test; the release tag rule's test; and the workflow checker
    and its test.
  - One run per ref (`concurrency`, cancel in progress). Actions are pinned
    to commit SHAs.

## What is not here

- **Any consumer's migration,** and `mcplib`'s deprecation of its own copy.
  Each is recorded in that repository.
- **Release signing.** No publisher signature is verified; the
  `ManifestVerifier` hook is where one would be.
