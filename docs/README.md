# go-selfupdate-lib documentation

The module was `go-core-lib` up to `v1.4.1`. Records 0001–0008 use that
name, and the files they cite moved with the rename
([0009-MADR](decisions/0009-MADR-rename-to-go-selfupdate-lib.md)).

## Records

| Number | Kind | Record | Status |
| :--- | :--- | :--- | :--- |
| 0001 | MADR | [Scaffold go-core-lib as a Go 1.27.1 shared library to the go-llmprovider-sdk standard, with honest gates until the first package lands](decisions/0001-MADR-scaffold-shared-go-library.md) | accepted |
| 0001 | PLAN | [Implement the go-core-lib Go 1.27.1 library scaffold](decisions/0001-PLAN-scaffold-shared-go-library.md) | complete |
| 0002 | MADR | [Re-home `selfupdate` and its release tooling from mcplib into go-core-lib as v1.0.0, without history](decisions/0002-MADR-rehome-selfupdate-from-mcplib.md) | accepted |
| 0002 | PLAN | [Implement the re-home of `selfupdate` and its release tooling into go-core-lib](decisions/0002-PLAN-rehome-selfupdate-from-mcplib.md) | complete |
| 0003 | MADR | [Fix every debugging-pass finding before tagging v1.0.0](decisions/0003-MADR-remediate-debugging-pass-findings.md) | accepted |
| 0003 | PLAN | [Implement the fixes for every debugging-pass finding](decisions/0003-PLAN-remediate-debugging-pass-findings.md) | complete |
| 0004 | MADR | [Evolve `selfupdate` into a layered, framework-neutral update toolkit](decisions/0004-MADR-evolve-selfupdate-api-and-tui-support.md) | accepted |
| 0004 | PLAN | [Implement Phase 0: the `v1.0.1` defect release](decisions/0004-PLAN-v1-0-1-defect-release.md) | complete |
| 0004 | PLAN | [Implement Phase 1: the `v1.1.0` core API](decisions/0004-PLAN-v1-1-0-core-api.md) | complete |
| 0004 | PLAN | [Implement Phase 2 core: per-run options and the interaction `Stream` (`v1.2.0`)](decisions/0004-PLAN-v1-2-0-interaction-stream.md) | complete |
| 0004 | PLAN | [Implement harness item H2: CI fuzzing and the manifest differential](decisions/0004-PLAN-h2-fuzzing-and-manifest-differential.md) | complete |
| 0004 | PLAN | [Implement harness item H4: an end-to-end update of a running copy](decisions/0004-PLAN-h4-running-copy-end-to-end.md) | complete |
| 0004 | PLAN | [Re-word ocp-login to patterns (`v1.3.1`)](decisions/0004-PLAN-v1-3-1-ocp-login-pattern-rewording.md) | complete |
| 0004 | PLAN | [Implement Phase 3: the canonical command surface (`v1.4.0`)](decisions/0004-PLAN-v1-4-0-command-surface.md) | complete |
| 0004 | REPORT | [Release signing for `selfupdate`: research and a design kept for later](reports/0004-REPORT-release-signing-research.md) | — |
| 0005 | MADR | [Offer opt-in prerelease channels without weakening the stable path](decisions/0005-MADR-opt-in-prerelease-channels.md) | accepted |
| 0005 | PLAN | [Implement opt-in prerelease channels (`v1.3.0`)](decisions/0005-PLAN-opt-in-prerelease-channels.md) | complete |
| 0006 | MADR | [Adopt golangci-lint v2.14.0, clearing its one finding at the source](decisions/0006-MADR-adopt-golangci-lint-v2-14.md) | accepted |
| 0006 | PLAN | [Implement the golangci-lint v2.14.0 adoption](decisions/0006-PLAN-adopt-golangci-lint-v2-14.md) | complete |
| 0007 | MADR | [Adopt govulncheck v1.8.0 in CI, in the install hints, and on every development host](decisions/0007-MADR-adopt-govulncheck-v1-8.md) | accepted |
| 0007 | PLAN | [Implement govulncheck v1.8.0 across CI, hints and hosts](decisions/0007-PLAN-adopt-govulncheck-v1-8.md) | complete |
| 0008 | MADR | [Enforce the module's import rules with depguard: forbidden modules, a module floor, and a floor per package](decisions/0008-MADR-enforce-import-rules-with-depguard.md) | accepted |
| 0008 | PLAN | [Implement depguard import rules](decisions/0008-PLAN-enforce-import-rules-with-depguard.md) | complete |
| 0009 | MADR | [Rename go-core-lib to go-selfupdate-lib: rename the repository in place, deprecate the old module path first, and publish the new path from v1.5.0](decisions/0009-MADR-rename-to-go-selfupdate-lib.md) | accepted |
| 0009 | PLAN | [Rename go-core-lib to go-selfupdate-lib (`v1.4.1`, `v1.5.0`)](decisions/0009-PLAN-rename-to-go-selfupdate-lib.md) | complete |
| 0010 | MADR | [Fix the second debugging pass's findings: a v1.5.1 of contract-preserving fixes, the tooling fixes with no release, and a v1.6.0 for the contracts the owner decides](decisions/0010-MADR-remediate-second-debugging-pass-findings.md) | accepted |
| 0010 | PLAN | [Implement the tooling fixes and the roadmap amendment (no release)](decisions/0010-PLAN-tooling-fixes.md) | complete |
| 0010 | PLAN | [Implement v1.5.1: the fixes that keep every documented contract](decisions/0010-PLAN-v1-5-1-contract-preserving-fixes.md) | complete |
| 0010 | PLAN | [Implement v1.6.0: the contracts the owner decided (Q1–Q6, A14)](decisions/0010-PLAN-v1-6-0-owner-contracts.md) | complete |
| 0011 | MADR | [Build `selfupdate/service`: reference systemd, launchd and Windows SCM lifecycles on the platform tools and `x/sys`, with `PollHealthy`, `ExecReconciler`, `sd_notify`, and a handoff that keeps an updater from stopping its own service](decisions/0011-MADR-reference-service-lifecycles.md) | accepted |
| 0011 | PLAN | [Implement `selfupdate/service`: reference systemd, launchd and Windows SCM lifecycles, the handoff and `sd_notify` (`v1.7.0`)](decisions/0011-PLAN-reference-service-lifecycles.md) | complete |
| 0012 | MADR | [Build `selfupdate/archive` and `selfupdate/codesign`: extract the program from a verified tar.gz or zip asset in a new stage before the transform, and re-sign or check it on macOS with `/usr/bin/codesign`](decisions/0012-MADR-archive-assets-and-macos-codesign.md) | accepted |
| 0012 | PLAN | [Implement `selfupdate/archive` and `selfupdate/codesign`: the extract stage, tar.gz, zip and gz assets, and opt-in macOS re-signing and signature checks (`v1.8.0`)](decisions/0012-PLAN-archive-assets-and-macos-codesign.md) | complete |
| 0013 | MADR | [Build, check and stage a self-update release in a reusable workflow, from one release spec that the program embeds](decisions/0013-MADR-build-and-stage-release-workflow.md) | accepted |
| 0013 | PLAN | [Implement the build-and-stage release workflow: `selfupdate/releasespec`, the internal release tool, `build-selfupdate-release.yml`, and archive releases through the publish workflow (`v1.9.0`)](decisions/0013-PLAN-build-and-stage-release-workflow.md) | complete |
| 0014 | MADR | [Generate each program's `install.sh` and `install.ps1` from its release spec in the build workflow, from one tested template each](decisions/0014-MADR-shared-installer-templates.md) | accepted |
| 0014 | PLAN | [Implement the shared installer templates: the spec's `installer` field, `install.sh` and `install.ps1` rendered and staged by the build workflow, and their tests (`v1.10.0`)](decisions/0014-PLAN-shared-installer-templates.md) | complete |
| 0015 | MADR | [Fix the third debugging pass's findings: a v1.10.1 of contract-preserving fixes, record and tooling fixes on main, and a v1.11.0 for the contracts the owner decides](decisions/0015-MADR-remediate-third-debugging-pass-findings.md) | accepted |
| 0015 | PLAN | [Implement the third debugging pass's remediation: records and the gate on main, v1.10.1 of contract-preserving fixes, and v1.11.0 for the owner's contracts](decisions/0015-PLAN-remediate-third-debugging-pass-findings.md) | complete |
| 0016 | REPORT | [The pre-add check passes a file gofmt cannot read](reports/0016-REPORT-precheck-gofmt-errors.md) | — |
| 0017 | MADR | [Check build provenance at update time with an opt-in `selfupdate/verify/ghattest`, and close the four items 0015 left open](decisions/0017-MADR-verify-build-provenance-and-close-0015-open-items.md) | accepted |
| 0017 | PLAN | [Implement 0017: the documentation on main, `v1.11.1` (the zip tiling and Windows-name refusals), and `v1.12.0` (the interrupted-update journal and `selfupdate/verify/ghattest`)](decisions/0017-PLAN-verify-build-provenance-and-close-0015-open-items.md) | complete |
| 0018 | MADR | [Build and test with Go 1.27.2 through a `toolchain` line, keep `go 1.27.1` as the floor, and release `v1.11.1` from a branch](decisions/0018-MADR-move-toolchain-to-go-1-27-2.md) | accepted |
| 0018 | PLAN | [Implement the Go 1.27.2 toolchain: the hosts, a `toolchain` line, and `v1.11.1` from a release branch](decisions/0018-PLAN-move-toolchain-to-go-1-27-2.md) | complete |
| 0019 | MADR | [Apply Go 1.27.2's `go fix` modernizers across every target OS, as one behaviour-preserving change](decisions/0019-MADR-apply-go-fix-modernizers.md) | accepted |
| 0019 | PLAN | [Implement Go 1.27.2's `go fix` modernizers across every target OS](decisions/0019-PLAN-apply-go-fix-modernizers.md) | complete |
| 0020 | MADR | [Fail the pre-add check when gofmt fails, and offer an opt-in replace-before-stop order for managed updates](decisions/0020-MADR-precheck-gofmt-errors-and-replace-before-stop.md) | accepted |
| 0020 | PLAN | [Implement a pre-add check that fails when gofmt fails, and an opt-in replace-before-stop order](decisions/0020-PLAN-precheck-gofmt-errors-and-replace-before-stop.md) | complete |
| 0021 | MADR | [Add a Task Scheduler reference lifecycle built on magic-cli-remote's measured Windows behaviour, and start the library's own Windows children without a window](decisions/0021-MADR-task-scheduler-lifecycle-and-windowless-children.md) | accepted |
| 0021 | PLAN | [Implement a Task Scheduler reference lifecycle and windowless Windows children](decisions/0021-PLAN-task-scheduler-lifecycle-and-windowless-children.md) | complete |
| 0021 | PLAN | [Implement the measured instance race and the CI probe of the Task Scheduler live tests](decisions/0021-PLAN-instance-race-and-ci-live-tests.md) | complete |
| 0022 | MADR | [Correct the documentation against v1.14.0, check the architecture table in the gate, export and harden the batch-file refusal, and measure Task Scheduler's run of a disabled task](decisions/0022-MADR-documentation-pass-after-v1-14-0.md) | accepted |
| 0022 | PLAN | [Implement the documentation pass after v1.14.0, the architecture-table check, a hardened `selfupdate.ErrBatchFile`, and the disabled-task measurement](decisions/0022-PLAN-documentation-pass-after-v1-14-0.md) | complete |
| 0022 | PLAN | [Implement the error-chain tests past interfaces and a process check that sees an exit](decisions/0022-PLAN-error-chain-and-process-liveness.md) | complete |
| 0022 | PLAN | [Implement the measured and pinned `SYNCHRONIZE` open from an unelevated caller](decisions/0022-PLAN-liveness-open-at-medium-integrity.md) | complete |
| 0023 | MADR | [Make the tests' fixture builds independent of the directory's git state, and their helper-process waits independent of machine load](decisions/0023-MADR-robust-test-fixtures-and-process-waits.md) | accepted |
| 0023 | PLAN | [Implement fixture builds independent of git state and helper waits independent of machine load](decisions/0023-PLAN-robust-test-fixtures-and-process-waits.md) | complete |
| 0024 | MADR | [Correct the documentation against v1.15.1, settle how Go comments cite records, add the missing Examples, and fix the four defects the audit found](decisions/0024-MADR-documentation-pass-after-v1-15-1.md) | accepted |
| 0024 | PLAN | [Implement the documentation pass after v1.15.1, the Examples, and the four defect fixes](decisions/0024-PLAN-documentation-pass-after-v1-15-1.md) | complete |
| 0025 | MADR | [Align go-selfupdate-lib with Go SDK design practices: redact secrets, type and document errors, write the concurrency and evolution contracts, add a one-call quickstart, retract the pre-rename tags, lower the Go floor to 1.26.9, and ship release signing as an opt-in verifier](decisions/0025-MADR-align-with-go-sdk-design-practices.md) | accepted |
| 0025 | PLAN | [Implement the Go SDK practices alignment as `v1.16.0`: redaction, typed errors, contexts, contracts, a quickstart, seams, a source conformance check, opt-in release signing, the Go 1.26.9 floor, `retract`, a changelog and a trust model](decisions/0025-PLAN-align-with-go-sdk-design-practices.md) | complete |
| 0026 | MADR | [Add a `before_uninstall` installer hook that runs the installed copy before `--uninstall` removes it, with the install hooks' rules, a `--force` to remove past a failed hook, and a library floor so an older program cannot embed it](decisions/0026-MADR-before-uninstall-installer-hook.md) | accepted |
| 0026 | PLAN | [Implement the `before_uninstall` installer hook as `v1.17.0`](decisions/0026-PLAN-before-uninstall-installer-hook.md) | complete |
| 0027 | MADR | [Correct the documentation against v1.17.0, fill the gaps v1.16.0 and v1.17.0 left, fix the three defects and two CI gaps the audit found, and release v1.17.1](decisions/0027-MADR-documentation-pass-after-v1-17-0.md) | accepted |
| 0027 | PLAN | [Implement the documentation pass after v1.17.0, the Examples, the three defect fixes and the two CI gaps, as v1.17.1](decisions/0027-PLAN-documentation-pass-after-v1-17-0.md) | complete |
| 0028 | MADR | [Skip the full CI for a push or pull request that changes only Markdown, and check it with a one-job docs workflow instead](decisions/0028-MADR-skip-full-ci-for-docs-only-pushes.md) | accepted |
| 0028 | PLAN | [Implement skipping the full CI for docs-only pushes, with a docs workflow](decisions/0028-PLAN-skip-full-ci-for-docs-only-pushes.md) | complete |
| 0029 | MADR | [Build and test with Go 1.27.3, raise the floor to `go 1.26.10`, and release `v1.17.2`](decisions/0029-MADR-move-toolchain-to-go-1-27-3-and-floor-to-1-26-10.md) | accepted |
| 0029 | PLAN | [Implement Go 1.27.3 and the `go 1.26.10` floor, and release `v1.17.2`](decisions/0029-PLAN-move-toolchain-to-go-1-27-3-and-floor-to-1-26-10.md) | complete |
| 0030 | MADR | [Publish this module's GitHub release from the tag's own CI run, after every job passes, with notes from the changelog](decisions/0030-MADR-publish-github-release-on-tag.md) | rejected |
| 0030 | PLAN | [Implement publishing this module's GitHub release from the tag's CI run](decisions/0030-PLAN-publish-github-release-on-tag.md) | rejected |
| 0031 | MADR | [Export `DefaultHTTPClient`, keep `GitHubOptions.Client` required, and give `launchd.Job` `Loaded` and a `Status` snapshot, as `v1.18.0`](decisions/0031-MADR-export-default-http-client-and-launchd-status.md) | accepted |
| 0031 | PLAN | [Implement `DefaultHTTPClient` and `launchd.Job`'s `Loaded` and `Status`, and release `v1.18.0`](decisions/0031-PLAN-export-default-http-client-and-launchd-status.md) | complete |
| 0032 | MADR | [Let `taskscheduler` register, read, compare and delete a definition, report its state, and hand an update off as a one-shot task](decisions/0032-MADR-task-scheduler-registration-and-one-shot-handoff.md) | accepted |
| 0032 | PLAN | [Implement Task Scheduler registration, comparison, state and the one-shot handoff, and release `v1.19.0`](decisions/0032-PLAN-task-scheduler-registration-and-one-shot-handoff.md) | complete |
| 0033 | MADR | [Leave the service manager's per-unit variables out of every environment the library passes on](decisions/0033-MADR-scrub-service-manager-environment-from-handoffs.md) | accepted |
| 0033 | PLAN | [Implement leaving the per-unit variables out of passed-on environments, and release `v1.20.0`](decisions/0033-PLAN-scrub-service-manager-environment-from-handoffs.md) | complete |
| 0034 | MADR | [Export `Task.Enable` and `Task.Disable`, keep `Start`'s rule, and stop counting the task's switch as definition drift](decisions/0034-MADR-export-task-scheduler-enable-and-disable.md) | accepted |
| 0034 | PLAN | [Implement `Task.Enable`, `Task.Disable`, `Status.Disabled` and the switch left out of `DiffDefinitions`, and release `v1.21.0`](decisions/0034-PLAN-export-task-scheduler-enable-and-disable.md) | in-progress |

## I want to…

| I want to… | Start here |
| :--- | :--- |
| see what is in this repository today | [architecture.md](architecture.md) |
| move a program from `mcplib/selfupdate` to this module | [guides/migrating-from-mcplib-selfupdate.md](guides/migrating-from-mcplib-selfupdate.md) |
| see what changed in each release | [CHANGELOG.md](../CHANGELOG.md) |
| report a vulnerability | [SECURITY.md](../SECURITY.md) |
| know what an update proves, and what it does not | [architecture.md, Trust model](architecture.md#trust-model) |
| know how the API may change within `v1` | [architecture.md, API evolution](architecture.md#api-evolution) |
| move to the current release from an earlier one of this module | [guides/migrating-from-mcplib-selfupdate.md, §6 to §17](guides/migrating-from-mcplib-selfupdate.md#6-from-v15-to-v16) |
| update a standalone program in one call | [README.md, Quickstart](../README.md#quickstart) |
| add the update command to my program | [guides/migrating-from-mcplib-selfupdate.md, §5](guides/migrating-from-mcplib-selfupdate.md#5-adopt-the-canonical-update-command) |
| stamp a release build so `update` knows it is one | `buildinfo.LDFlags`; [0004-PLAN-v1-4-0, Step 2](decisions/0004-PLAN-v1-4-0-command-surface.md#step-2-buildinfo-buildinfobuildinfogo-new) |
| show an update banner | [guides/extending-selfupdate.md](guides/extending-selfupdate.md#show-an-update-banner) |
| offer a beta or rc channel | [guides/extending-selfupdate.md](guides/extending-selfupdate.md#offer-a-beta-channel) |
| read JSON output | [guides/extending-selfupdate.md](guides/extending-selfupdate.md#read-json-output) |
| plug in a credential | [guides/extending-selfupdate.md](guides/extending-selfupdate.md#plug-in-a-credential) |
| verify a release signature | [guides/extending-selfupdate.md](guides/extending-selfupdate.md#verify-a-release-signature) |
| sign my releases | [guides/building-releases.md, step 13](guides/building-releases.md#13-sign-a-release-opt-in) |
| check that a release was built by my workflow | [guides/extending-selfupdate.md](guides/extending-selfupdate.md#check-build-provenance) |
| probe the new binary | [guides/extending-selfupdate.md](guides/extending-selfupdate.md#probe-the-new-binary) |
| drive an update from a TUI or event loop | [guides/extending-selfupdate.md](guides/extending-selfupdate.md#drive-an-update-from-a-tui-or-event-loop) |
| log update events with `log/slog` | [guides/extending-selfupdate.md](guides/extending-selfupdate.md#log-with-slog) |
| see the update path proven end to end on each OS | `selfupdate/e2e_running_test.go`; [0004-PLAN, H4](decisions/0004-PLAN-h4-running-copy-end-to-end.md) |
| run every check before a commit | `make gate`, one line per step, and markdownlint, actionlint and the stray-`.git` test run by hand where they apply; see [AGENTS.md, Pre-add checks](../AGENTS.md#pre-add-checks) and [architecture.md, Tooling](architecture.md#tooling) |
| fuzz locally | `make fuzz` (each target for 20 s), or `make fuzz FUZZTIME=5m`; see [architecture.md, Tooling](architecture.md#tooling) |
| know what to do when CI finds a crasher | download the `fuzz-corpus` artifact, copy its file into `<package>/testdata/fuzz/<Name>/` of the package that owns the target (`selfupdate`, `selfupdate/archive`, `selfupdate/releasespec` or `selfupdate/verify/signednote`), fix the defect, and commit the file as a regression seed ([0004-PLAN, H2](decisions/0004-PLAN-h2-fuzzing-and-manifest-differential.md)) |
| update from releases shipped as tar.gz, zip or gz archives, such as GoReleaser's | [guides/extending-selfupdate.md](guides/extending-selfupdate.md#ship-an-archive) |
| check my own release source for another host | [guides/extending-selfupdate.md](guides/extending-selfupdate.md#use-another-release-host) |
| describe my program's release once (products, platforms, packaging, extras) and read it in Go | [guides/building-releases.md](guides/building-releases.md#1-write-the-spec) |
| build, check and publish my release in CI, with rehearsals on every pull request | [guides/building-releases.md](guides/building-releases.md#4-call-the-workflows) |
| know why a release build failed a check | [guides/building-releases.md, §9](guides/building-releases.md#9-when-a-check-fails) |
| give my users `curl … \| sh` and `irm … \| iex` installers without writing them | [guides/building-releases.md, §12](guides/building-releases.md#12-installers) |
| run a command at install or before uninstall | [guides/building-releases.md, Hooks](guides/building-releases.md#hooks) |
| re-sign my macOS binary on update, or require its signature | [guides/extending-selfupdate.md](guides/extending-selfupdate.md#sign-on-macos) |
| run my program as a systemd, launchd or Windows service, or a per-user Task Scheduler task, and update it from inside | [guides/extending-selfupdate.md](guides/extending-selfupdate.md#run-as-a-service) |
| write my own installer | [guides/extending-selfupdate.md](guides/extending-selfupdate.md#write-your-own-installer) |
| test a program that self-updates | [guides/extending-selfupdate.md](guides/extending-selfupdate.md#test-a-program-that-self-updates) |
| rehearse an update without installing | [guides/extending-selfupdate.md](guides/extending-selfupdate.md#rehearse-without-installing) |
| keep the previous binary after an update | [guides/extending-selfupdate.md](guides/extending-selfupdate.md#keep-the-previous-binary) |
| replace a setuid or setgid binary | [guides/extending-selfupdate.md](guides/extending-selfupdate.md#replace-a-setuid-or-setgid-binary) |
| replace a service's binary before stopping it | [guides/extending-selfupdate.md](guides/extending-selfupdate.md#replace-before-the-stop) |
| know why `selfupdate` moved here, and what changed on the way | [0002-MADR](decisions/0002-MADR-rehome-selfupdate-from-mcplib.md) |
| know why `bridge-release` is gone | [0002-MADR, §3](decisions/0002-MADR-rehome-selfupdate-from-mcplib.md#3-what-changes-in-transit-and-nothing-else) |
| know why lint runs three times | [0002-MADR, §5](decisions/0002-MADR-rehome-selfupdate-from-mcplib.md#5-lint-covers-every-target-the-code-builds-for) |
| contribute: checks, records and commit rules | [AGENTS.md](../AGENTS.md) |
| know what may be imported here, and what never may | [AGENTS.md, Dependencies](../AGENTS.md#dependencies), enforced by depguard ([0008-MADR](decisions/0008-MADR-enforce-import-rules-with-depguard.md)) |
| know why the gates were never taught to pass on an empty module | [0001-MADR, §6](decisions/0001-MADR-scaffold-shared-go-library.md#6-gates-are-not-taught-to-pass-on-nothing) |
| see what the debugging pass found, and how each finding is fixed | [0003-MADR](decisions/0003-MADR-remediate-debugging-pass-findings.md) |
| see where `selfupdate` is going: API growth, TUI support, the canonical CLI, and what is still open | [0004-MADR, amendment P3](decisions/0004-MADR-evolve-selfupdate-api-and-tui-support.md#p3-2026-10-07-phase-4-is-built-the-open-work-in-one-place) |
| know how release signing was designed, and why it is opt-in | [0004-REPORT](reports/0004-REPORT-release-signing-research.md), and [0025-MADR](decisions/0025-MADR-align-with-go-sdk-design-practices.md) D10 |
| know how this repository's tooling differs from `go-llmprovider-sdk`'s | [0001-MADR, §5](decisions/0001-MADR-scaffold-shared-go-library.md#5-deliberate-differences-from-go-llmprovider-sdk) |
| see how the scaffold's checks were proven | [0001-PLAN, execution record](decisions/0001-PLAN-scaffold-shared-go-library.md#execution-record) |
