# Changelog

Every release of `github.com/maccavelli/go-selfupdate-lib`, newest first,
in the form of [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Within `v1` every exported change is an addition, which `make apicheck`
enforces against the newest `v1.*` tag; behaviour a program can see is
listed in the
[migration guide](docs/guides/migrating-from-mcplib-selfupdate.md), one
section per release, and each entry links its section and its records.

## v1.17.2 - 2026-10-21

See [the guide](docs/guides/migrating-from-mcplib-selfupdate.md#from-v1171-to-v1172),
[0029-MADR](docs/decisions/0029-MADR-move-toolchain-to-go-1-27-3-and-floor-to-1-26-10.md),
[0029-PLAN](docs/decisions/0029-PLAN-move-toolchain-to-go-1-27-3-and-floor-to-1-26-10.md),
[0028-MADR](docs/decisions/0028-MADR-skip-full-ci-for-docs-only-pushes.md) and
[0028-PLAN](docs/decisions/0028-PLAN-skip-full-ci-for-docs-only-pushes.md).

### Security

- The module requires Go 1.26.10, not 1.26.9, and is built and tested
  with Go 1.27.3, not 1.27.2. Both older versions carry GO-2026-6809, in
  `crypto/x509`, which `govulncheck` reaches through the manifest check.
  CI's `go126` job tests 1.26.10. A program on Go 1.26.9 moves to
  1.26.10 or later to take this release.
- For a caller that pins `v1.17.2`, the publish workflow builds its
  release tools with Go 1.27.3.

### Changed

- CI skips a push or pull request that changes only Markdown;
  `.github/workflows/docs.yml` checks it instead, with markdownlint, the
  link check and the architecture table check. Every tag still runs CI
  in full.

## v1.17.1 - 2026-10-20

See [the guide](docs/guides/migrating-from-mcplib-selfupdate.md#from-v1170-to-v1171),
[0027-MADR](docs/decisions/0027-MADR-documentation-pass-after-v1-17-0.md) and
[0027-PLAN](docs/decisions/0027-PLAN-documentation-pass-after-v1-17-0.md).

### Fixed

- `install.ps1`'s uninstall reported a program it could not remove, such
  as a running one, as removed, and exited 0. Both installers now report
  it, remove the others, and exit 3, which also means "removed only in
  part". The installers ship with the workflows.
- `(*Updater).Checker` on a nil `*Updater` panicked. It returns a
  `Checker` whose methods return `ErrNotConstructed`, as that error's
  doc says.
- A managed install's session did not serialize its methods, as
  `InstallSession`'s doc says: `Close` could run while `Install` was
  between its calls. It now waits.

### Changed

- CI runs `govulncheck` on the Go 1.26.9 floor too, as `SECURITY.md`
  says, and keeps `signednote`'s fuzz corpus when a fuzz run fails.
- The tests' leak check, `internal/testproc.CheckNoLeak`, compares
  goroutine IDs, not a count, so a goroutine that ends during a test can
  no longer hide a leak ([0025-MADR](docs/decisions/0025-MADR-align-with-go-sdk-design-practices.md), Amendment A6). Test support
  only.
- A documentation pass fixed the audit's findings in the guides, the
  architecture page, `go doc` and the script and workflow headers. Two
  Examples that failed when run are fixed, and `NewSlogReporter`,
  `NewVersionProberWith`, `StandaloneInstaller.CleanupPending` and
  `selfupdatetest.VerifyReleaseSource` gained one.

## v1.17.0 - 2026-10-19

See [the guide](docs/guides/migrating-from-mcplib-selfupdate.md#17-from-v116-to-v117),
[0026-MADR](docs/decisions/0026-MADR-before-uninstall-installer-hook.md) and
[0026-PLAN](docs/decisions/0026-PLAN-before-uninstall-installer-hook.md),
from [issue #1](https://github.com/maccavelli/go-selfupdate-lib/issues/1).

### Added

- `releasespec.HookBeforeUninstall`, a third installer hook time:
  `before_uninstall` runs the installed copy of a selected product before
  `--uninstall` / `-Uninstall` removes it. A failure removes nothing (exit
  1); the installers' new `--force` / `-Force` removes anyway (exit 3).
  With `--uninstall`, `--dry-run` / `-DryRun` lists the hooks it would
  run.

### Changed

- The build workflow's plan step refuses a spec with a `before_uninstall`
  hook in a module that requires a release below `v1.17.0`, which cannot
  parse it. When a module misses more than one floor, the message names
  the highest.
- The doc comments of the exported calls that take no context say why:
  `systemd.Notify` and `Available`, `service.SameExecutable`,
  `ReportFunc`, `WriteHandOffResult`, `ReadHandOffResult` and
  `WriteHandOffEnv`, and `InstallSession.Close`
  ([0025-PLAN](docs/decisions/0025-PLAN-align-with-go-sdk-design-practices.md),
  deviation D5).

## v1.16.0 - 2026-10-19

See [the guide](docs/guides/migrating-from-mcplib-selfupdate.md#16-from-v115-to-v116),
[0025-MADR](docs/decisions/0025-MADR-align-with-go-sdk-design-practices.md) and
[0025-PLAN](docs/decisions/0025-PLAN-align-with-go-sdk-design-practices.md).

### Added

- `NewGitHubUpdater` and `GitHubUpdaterOptions`: one call for a standalone
  program on GitHub, with safe defaults and a client bounded by
  transport timeouts, not a whole-request timeout.
- `HTTPError`, a non-2xx response no sentinel describes, and
  `service.CommandError`, a failed `systemctl`, `systemd-run`, `launchctl`
  or exec-reconciler command; both work with `errors.As`, with the
  messages unchanged.
- `service.HandOffHopContext` and `service.LoadHandOffEnvContext`, whose
  context bounds the wait for the hand-off hop.
- `Clock`, `ClockFunc`, `Config.Clock` and `CheckerConfig.Clock`;
  `NewVersionProberWith` and `VersionProbeOptions`; `NewSlogReporter`.
- `selfupdatetest.VerifyReleaseSource` and `SourceCase`, a conformance
  check for a `ReleaseSource` of another host.
- `selfupdate/verify/signednote`, an opt-in check of a signed release
  statement, `SHA256SUMS.note`; the publish workflow's opt-in `sign` job
  and the unreleased signer `internal/cmd/selfupdate-note`.
- Doc comments that state each type's concurrency contract, and an
  "Errors" paragraph on `Run`, `RunWith`, `Start`, `Check`, `CheckCached`
  and the service backends' `Stop`, `Start` and `WaitHealthy`.

### Changed

- The module requires Go 1.26.9, not 1.27.1; it is still built and tested
  with Go 1.27.2, and CI also tests it with 1.26.9.
- `fmt` and `log/slog` print `Credential`, `GitHubOptions` and
  `*GitHubSource` with the secret as `<redacted>`; `encoding/json` still
  carries a `Credential`'s value, so a `CredentialObserver` can save it.
- On Windows, the cleanup receipt's retries take the session's
  `LockTimeout`, not a fixed 5 s, and at session start they end with the
  caller's context.
- `go.mod` retracts `v1.0.0` to `v1.4.1`, which were published as
  `go-core-lib` and never resolve under this path.
- `SHA256SUMS.note` is reserved: a release spec and the publish workflow
  refuse it as an extra asset.

### Fixed

- A data race on the Task Scheduler backend's last `CoInitializeEx`
  result when two tasks were driven at once.

## v1.15.2 - 2026-10-12

### Fixed

- Task Scheduler's `StopGrace` is checked against the stop timeout, and a
  failed stop leaves the task enabled; `selfupdatetest.FakeSource` wraps
  `ErrNoRelease`; `install.ps1` gives hooks no standard input.

See [the guide](docs/guides/migrating-from-mcplib-selfupdate.md#from-v1151-to-v1152),
[0024-MADR](docs/decisions/0024-MADR-documentation-pass-after-v1-15-1.md) and
[0021-PLAN](docs/decisions/0021-PLAN-instance-race-and-ci-live-tests.md).

## v1.15.1 - 2026-10-11

### Fixed

- The Task Scheduler backend reads an exited program as gone. No API change.

See [the guide](docs/guides/migrating-from-mcplib-selfupdate.md#from-v1150-to-v1151)
and [0022-PLAN](docs/decisions/0022-PLAN-error-chain-and-process-liveness.md).

## v1.15.0 - 2026-10-11

### Added

- `ErrBatchFile`, exported; a disabled task that cannot be restarted is
  refused.

See [the guide](docs/guides/migrating-from-mcplib-selfupdate.md#15-from-v114-to-v115),
[0022-MADR](docs/decisions/0022-MADR-documentation-pass-after-v1-14-0.md) and
[0022-PLAN](docs/decisions/0022-PLAN-documentation-pass-after-v1-14-0.md).

## v1.14.0 - 2026-10-10

### Added

- The Task Scheduler backend; the library's Windows children start
  without a window, and batch files are refused.

See [the guide](docs/guides/migrating-from-mcplib-selfupdate.md#14-from-v113-to-v114)
and [0021-MADR](docs/decisions/0021-MADR-task-scheduler-lifecycle-and-windowless-children.md).

## v1.13.0 - 2026-10-09

### Added

- `ManagedOptions.ReplaceBeforeStop`, replacing a service's binary before
  the stop.

See [the guide](docs/guides/migrating-from-mcplib-selfupdate.md#13-from-v112-to-v113)
and [0020-MADR](docs/decisions/0020-MADR-precheck-gofmt-errors-and-replace-before-stop.md).

## v1.12.1 - 2026-10-08

### Fixed

- The Windows installer. No API change.

See [the guide](docs/guides/migrating-from-mcplib-selfupdate.md#from-v1120-to-v1121)
and [0017-PLAN](docs/decisions/0017-PLAN-verify-build-provenance-and-close-0015-open-items.md).

## v1.12.0 - 2026-10-08

### Added

- `selfupdate/verify/ghattest`, an opt-in build-provenance check; an
  interrupted update's backup is kept.

See [the guide](docs/guides/migrating-from-mcplib-selfupdate.md#12-from-v111-to-v112)
and [0017-MADR](docs/decisions/0017-MADR-verify-build-provenance-and-close-0015-open-items.md).

## v1.11.1 - 2026-10-08

### Fixed

- The last archive differentials are closed; the module builds with a
  `toolchain go1.27.2` line.

See [the guide](docs/guides/migrating-from-mcplib-selfupdate.md#from-v1110-to-v1111),
[0017-PLAN](docs/decisions/0017-PLAN-verify-build-provenance-and-close-0015-open-items.md) and
[0018-MADR](docs/decisions/0018-MADR-move-toolchain-to-go-1-27-2.md).

## v1.11.0 - 2026-10-08

### Changed

- `ErrNotConstructed` for a zero value; a missing release is
  `ErrNoRelease`, cached as `no-release`.

See [the guide](docs/guides/migrating-from-mcplib-selfupdate.md#11-from-v110-to-v111)
and [0015-MADR](docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md).

## v1.10.1 - 2026-10-07

### Fixed

- The third debugging pass's findings. No API change.

See [the guide](docs/guides/migrating-from-mcplib-selfupdate.md#from-v1100-to-v1101)
and [0015-MADR](docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md).

## v1.10.0 - 2026-10-07

### Added

- The release spec's `installer`: `install.sh` and `install.ps1`, rendered
  by the build workflow.

See [the guide](docs/guides/migrating-from-mcplib-selfupdate.md#10-from-v19-to-v110)
and [0014-MADR](docs/decisions/0014-MADR-shared-installer-templates.md).

## v1.9.0 - 2026-10-06

### Added

- `selfupdate/releasespec` and the build-and-stage workflow.

See [the guide](docs/guides/migrating-from-mcplib-selfupdate.md#9-from-v18-to-v19)
and [0013-MADR](docs/decisions/0013-MADR-build-and-stage-release-workflow.md).

## v1.8.0 - 2026-10-05

### Added

- `Config.Unpacker`, `selfupdate/archive` and `selfupdate/codesign`.

See [the guide](docs/guides/migrating-from-mcplib-selfupdate.md#8-from-v17-to-v18)
and [0012-MADR](docs/decisions/0012-MADR-archive-assets-and-macos-codesign.md).

## v1.7.0 - 2026-10-05

### Added

- `selfupdate/service`, with the systemd, launchd and Windows SCM
  backends and the hand-off.

See [the guide](docs/guides/migrating-from-mcplib-selfupdate.md#7-from-v16-to-v17)
and [0011-MADR](docs/decisions/0011-MADR-reference-service-lifecycles.md).

## v1.6.0 - 2026-10-04

### Changed

- An error after the update is a warning; the JSON result is schema 2; a
  stopped service stays stopped.

See [the guide](docs/guides/migrating-from-mcplib-selfupdate.md#6-from-v15-to-v16),
[0010-MADR](docs/decisions/0010-MADR-remediate-second-debugging-pass-findings.md) and
[0010-PLAN](docs/decisions/0010-PLAN-v1-6-0-owner-contracts.md).

## v1.5.1 - 2026-10-03

### Fixed

- Every failure writes the `--json` result. No API change.

See [the guide](docs/guides/migrating-from-mcplib-selfupdate.md#from-v150-to-v151)
and [0010-PLAN](docs/decisions/0010-PLAN-v1-5-1-contract-preserving-fixes.md).

## v1.5.0 - 2026-10-02

### Changed

- The first release under this module path, renamed from `go-core-lib`.

See [the guide](docs/guides/migrating-from-mcplib-selfupdate.md#2-the-go-import)
and [0009-MADR](docs/decisions/0009-MADR-rename-to-go-selfupdate-lib.md).

## v1.0.0 to v1.4.1

Published as `github.com/maccavelli/go-core-lib`. They never resolve under
this path, and `go.mod` retracts them; see the
[0009-MADR](docs/decisions/0009-MADR-rename-to-go-selfupdate-lib.md) for
the rename.
