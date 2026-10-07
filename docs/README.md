# go-selfupdate-lib documentation

The module was `go-core-lib` up to `v1.4.1`. Records 0001–0008 use that
name, and the files they cite moved with the rename
([0009-MADR](decisions/0009-MADR-rename-to-go-selfupdate-lib.md)).

## Records

| Number | Kind | Record | Status |
| :--- | :--- | :--- | :--- |
| 0001 | MADR | [Scaffold go-core-lib as a Go 1.27.1 shared library](decisions/0001-MADR-scaffold-shared-go-library.md) | accepted |
| 0001 | PLAN | [Implement the library scaffold](decisions/0001-PLAN-scaffold-shared-go-library.md) | complete |
| 0002 | MADR | [Re-home `selfupdate` and its release tooling from mcplib as v1.0.0](decisions/0002-MADR-rehome-selfupdate-from-mcplib.md) | accepted |
| 0002 | PLAN | [Implement the `selfupdate` re-home](decisions/0002-PLAN-rehome-selfupdate-from-mcplib.md) | complete |
| 0003 | MADR | [Fix every debugging-pass finding before v1.0.0](decisions/0003-MADR-remediate-debugging-pass-findings.md) | accepted |
| 0003 | PLAN | [Implement the debugging-pass fixes](decisions/0003-PLAN-remediate-debugging-pass-findings.md) | complete |
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
| 0015 | PLAN | [Implement the third debugging pass's remediation: records and the gate on main, v1.10.1 of contract-preserving fixes, and v1.11.0 for the owner's contracts](decisions/0015-PLAN-remediate-third-debugging-pass-findings.md) | in-progress |

## I want to…

| I want to… | Start here |
| :--- | :--- |
| see what is in this repository today | [architecture.md](architecture.md) |
| move a program from `mcplib/selfupdate` to this module | [guides/migrating-from-mcplib-selfupdate.md](guides/migrating-from-mcplib-selfupdate.md) |
| move to the current release from an earlier one of this module | [guides/migrating-from-mcplib-selfupdate.md, §6 to §10](guides/migrating-from-mcplib-selfupdate.md#6-from-v15-to-v16) |
| add the update command to my program | [guides/migrating-from-mcplib-selfupdate.md, §5](guides/migrating-from-mcplib-selfupdate.md#5-adopt-the-canonical-update-command) |
| stamp a release build so `update` knows it is one | `buildinfo.LDFlags`; [0004-PLAN-v1-4-0, Step 2](decisions/0004-PLAN-v1-4-0-command-surface.md#step-2-buildinfo-buildinfobuildinfogo-new) |
| show an update banner | [guides/extending-selfupdate.md](guides/extending-selfupdate.md#show-an-update-banner) |
| offer a beta or rc channel | [guides/extending-selfupdate.md](guides/extending-selfupdate.md#offer-a-beta-channel) |
| read JSON output | [guides/extending-selfupdate.md](guides/extending-selfupdate.md#read-json-output) |
| plug in a credential | [guides/extending-selfupdate.md](guides/extending-selfupdate.md#plug-in-a-credential) |
| verify a signature later | [guides/extending-selfupdate.md](guides/extending-selfupdate.md#verify-a-signature-later) |
| probe the new binary | [guides/extending-selfupdate.md](guides/extending-selfupdate.md#probe-the-new-binary) |
| drive an update from a TUI or event loop | [guides/extending-selfupdate.md](guides/extending-selfupdate.md#drive-an-update-from-a-tui-or-event-loop) |
| see the update path proven end to end on each OS | `selfupdate/e2e_running_test.go`; [0004-PLAN, H4](decisions/0004-PLAN-h4-running-copy-end-to-end.md) |
| run every check before a commit | `make gate`, one line per step; see [AGENTS.md, Pre-add checks](../AGENTS.md#pre-add-checks) and [architecture.md, Tooling](architecture.md#tooling) |
| fuzz locally | `make fuzz` (each target for 20 s), or `make fuzz FUZZTIME=5m`; see [architecture.md, Tooling](architecture.md#tooling) |
| know what to do when CI finds a crasher | download the `fuzz-corpus` artifact, copy its file into `selfupdate/testdata/fuzz/<Name>/`, fix the defect, and commit the file as a regression seed ([0004-PLAN, H2](decisions/0004-PLAN-h2-fuzzing-and-manifest-differential.md)) |
| update from releases shipped as tar.gz, zip or gz archives, such as GoReleaser's | [guides/extending-selfupdate.md](guides/extending-selfupdate.md#ship-an-archive) |
| describe my program's release once (products, platforms, packaging, extras) and read it in Go | [guides/building-releases.md](guides/building-releases.md#1-write-the-spec) |
| build, check and publish my release in CI, with rehearsals on every pull request | [guides/building-releases.md](guides/building-releases.md#4-call-the-workflows) |
| know why a release build failed a check | [guides/building-releases.md, §9](guides/building-releases.md#9-when-a-check-fails) |
| give my users `curl … \| sh` and `irm … \| iex` installers without writing them | [guides/building-releases.md, §12](guides/building-releases.md#12-installers) |
| re-sign my macOS binary on update, or require its signature | [guides/extending-selfupdate.md](guides/extending-selfupdate.md#sign-on-macos) |
| run my program as a systemd, launchd or Windows service, and update it from inside | [guides/extending-selfupdate.md](guides/extending-selfupdate.md#run-as-a-service) |
| write my own installer, or test a program that self-updates | [guides/extending-selfupdate.md](guides/extending-selfupdate.md#write-your-own-installer) |
| know why `selfupdate` moved here, and what changed on the way | [0002-MADR](decisions/0002-MADR-rehome-selfupdate-from-mcplib.md) |
| know why `bridge-release` is gone | [0002-MADR, §3](decisions/0002-MADR-rehome-selfupdate-from-mcplib.md#3-what-changes-in-transit-and-nothing-else) |
| know why lint runs three times | [0002-MADR, §5](decisions/0002-MADR-rehome-selfupdate-from-mcplib.md#5-lint-covers-every-target-the-code-builds-for) |
| contribute: checks, records and commit rules | [AGENTS.md](../AGENTS.md) |
| know what may be imported here, and what never may | [0001-MADR, §3](decisions/0001-MADR-scaffold-shared-go-library.md#3-toolchain-and-dependencies) |
| know why the gates were never taught to pass on an empty module | [0001-MADR, §6](decisions/0001-MADR-scaffold-shared-go-library.md#6-gates-are-not-taught-to-pass-on-nothing) |
| see what the debugging pass found, and how each finding is fixed | [0003-MADR](decisions/0003-MADR-remediate-debugging-pass-findings.md) |
| see where `selfupdate` is going: API growth, TUI support, the canonical CLI | [0004-MADR](decisions/0004-MADR-evolve-selfupdate-api-and-tui-support.md) |
| know why releases are not signed, and how signing would be done | [0004-REPORT](reports/0004-REPORT-release-signing-research.md) |
| know how this repository's tooling differs from `go-llmprovider-sdk`'s | [0001-MADR, §5](decisions/0001-MADR-scaffold-shared-go-library.md#5-deliberate-differences-from-go-llmprovider-sdk) |
| see how the scaffold's checks were proven | [0001-PLAN, execution record](decisions/0001-PLAN-scaffold-shared-go-library.md#execution-record) |
