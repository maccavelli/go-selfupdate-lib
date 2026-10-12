---
status: in-progress
date: 2026-10-12
associated-madr: "0024-MADR-documentation-pass-after-v1-15-1.md"
---
# Implement the documentation pass after v1.15.1, the Examples, and the four defect fixes

Associated MADR: [0024-MADR-documentation-pass-after-v1-15-1.md](0024-MADR-documentation-pass-after-v1-15-1.md)
(D1-A, D2-A, D3-A, D4-A).

## Goal

* Every finding below is closed: the line changed, or recorded as not
  changed with the reason (D1-A).
* Go comments cite records by D2-A's rule, and AGENTS.md states it.
* The Examples D3-A names exist and pass.
* C1, C2, D2 and E14 are fixed, each with a test seen failing first,
  and released as `v1.15.2` (D4-A).

## Scope

### In scope

| Phase | What | Commit |
| :--- | :--- | :--- |
| W0 | this pair and its index rows | records only |
| W1 | Task Scheduler: C1 and C2 | code and tests |
| W2 | `FakeSource` and `ErrNoRelease`: E14 | code and tests |
| W3 | `install.ps1`'s standard input: D2 | code and tests |
| W4 | Go doc comments, citations and script headers | comments, AGENTS.md |
| W5 | the Examples | tests, `docs/architecture.md` counts |
| W6 | README.md, AGENTS.md, `docs/README.md`, reports | docs |
| W7 | `docs/architecture.md` | docs |
| W8 | `extending-selfupdate.md` | docs |
| W9 | the migration and building guides, and `v1.15.2`'s section | docs |
| W10 | `v1.15.2` | per 0017's release procedure |
| W11 | closing | records only |

### Out of scope

* **Exported API changes.** `apicheck` stays compatible with `v1.15.1`.
* **Adding markdownlint or actionlint to `make gate`** (A4): the pass
  documents what the gate leaves out; changing the gate is its own
  decision.
* **Citations in Go comments that already resolve** (D2-A).
* **Push and tags,** which happen only on the owner's ask in the same
  turn.

## Rules

* **`main`, no branch.** The owner commits and pushes on `main`. An agent
  commits only on an explicit "commit to main" in that turn, with
  `git commit --no-edit`.
* **Each phase stops at a deviation** (AGENTS.md, Plan deviations). A
  finding the phase cannot close as written is a deviation.
* **Checks before each commit:** `make pre-add-check FILES="…"` on every
  changed Go file; `scripts/check-docs.sh --links` and `--ids`, and
  markdownlint, on every changed Markdown file; `--arch` when a Go file
  is added; `make gate` ending `overall=0`.
  *(Deviation D2, 2026-10-12: `make gate` runs once before each push,
  not before each commit; the other checks stay per commit.)*
* **Seen failing first:** every new test or assertion fails on a
  `scripts/plant-copy.sh` copy with one planted break, never by dirtying
  the tree. The record quotes the failing line.
* **The Windows test host:** binaries cross-compiled here, copied to a
  new directory under its temporary directory, run from its ssh shell,
  and cleaned up. Before a live Task Scheduler run, the agent asks the
  owner to confirm the console sign-in.
* **Snippets:** every code snippet a phase adds or changes is built in a
  scratch module outside the repository, for linux, darwin and windows.
* **Identifiers:** nothing committed names a host, an account or a real
  absolute path.

## Findings

Found by the audit at `c2eded4` (MADR, Context). "Kind": wrong
(contradicts the code), stale (was true), gap (missing), broken (fails
as written), inconsistent (disagrees with another document or rule).

### README.md, AGENTS.md, `docs/README.md`, reports (W6)

| ID | Where | Kind | Finding, and the fix |
| :--- | :--- | :--- | :--- |
| A1 | README.md:78-80 | stale | "Redirects must stay on HTTPS unless they go to a loopback host": a plain-http loopback hop is allowed only from a loopback API (`selfupdate/github.go:222-230`, 0010-MADR A10). Say so. |
| A2 | `docs/reports/0004-REPORT-release-signing-research.md`:57, 69-70, 102-105 | stale | Provenance "not checked at run time" and "an opt-in nested module": `ghattest` does it since `v1.12.0` (0017-MADR), in this module. A dated addendum; the text stays. |
| A3 | the same report:296-299 | stale | Open question 1 (an unreleased `internal/cmd` tool) is settled by AGENTS.md and 0013-MADR. The same addendum. |
| A4 | AGENTS.md:181-184; `docs/README.md`:87 | inconsistent | "`make gate` runs every check a commit needs": it has no markdownlint or actionlint, which CI runs, and the stray-`.git` run is CI's. Name what it leaves out. |
| A5 | AGENTS.md:194-197 | gap | `plant-copy.sh`'s OLD and NEW are Python string literals, and the copy has no `.git`. Say so. |
| A6 | AGENTS.md:168-171 | gap | Live tests skip unless their `SELFUPDATE_REQUIRE_*` switch is set; only `SELFUPDATE_REQUIRE_PYTHON` is named. One sentence and a pointer to `docs/architecture.md`'s CI section. |
| A7 | AGENTS.md:29-33 | gap | The scope paragraph omits `internal/testproc`, and describes `winproc` only as building processes; it also holds `Alive` and `Descends`. |
| A8 | `docs/README.md`:97 | gap | "test a program that self-updates" links only the installer section. Link `#test-a-program-that-self-updates`. |
| A9 | `docs/README.md`:69-108 | gap | No rows for "Rehearse without installing", "Keep the previous binary", "Replace a setuid or setgid binary", "Replace before the stop". Add them. |
| A10 | README.md:78-80 | gap | The Windows batch-file refusal and `selfupdate.ErrBatchFile` are absent. One sentence. |

### `docs/architecture.md` (W7)

| ID | Where | Kind | Finding, and the fix |
| :--- | :--- | :--- | :--- |
| B1 | :94-95, :259-260 | wrong | `winproc` starts "every process the module starts": the release tool starts its own (`gotool.go:47`, `identity.go:81`). Narrow to `selfupdate/`. |
| B2 | :263-267 | wrong, gap | `Alive` is Windows-only (Unix uses `kill(pid, 0)`); `AliveAccess` and 0022-MADR A3 are unmentioned. |
| B3 | :216-222, :259-262 | gap | `selfupdate.ErrBatchFile` and where it is wrapped (0022-MADR D3). |
| B4 | :242-251 | gap | `ErrTaskDisabled` in `Stop` and `Start`; the rewrite limited to `InteractiveToken` principals. |
| B5 | :216-222 | gap | The hop wait (`hopwait_*.go`) and `SameExecutable` (`sameexec.go`). |
| B6 | :269-275 | gap | `testproc.Build`, `Budget`, and `TestNoDirectFixtureBuild`. |
| B7 | :534-537 | gap | CI's "Task Scheduler session probe" step. |
| B8 | :527, :532-533 | wrong | "each required, so a skip fails" includes codesign, but `TestLiveSignIdentity` skips without `SELFUPDATE_CODESIGN_IDENTITY`. List it under "Not in CI". |
| B9 | :492-493 | wrong | `other-packages` "`internal/` included": `winproc` and `testproc` have rules of their own. Match AGENTS.md. |
| B10 | :278-279, :291-294 | inconsistent | `ghattest` sits under 0012-MADR; it is 0017-MADR's. Its own bullet. |
| B11 | :112-354 | gap | No `releasespec` bullet; 0013-MADR and 0014-MADR never cited. |
| B12 | :161, :260 | inconsistent | "0020-MADR" and "0021-MADR D7 and D8" without filenames. Link them. |
| B13 | :570-571 | wrong | "On a branch or pull request it rehearses": CI runs on pushes to `main` and `release/**`, `v*` tags and pull requests. State the triggers. |
| B14 | :499-506 | wrong | The gate runs every script test but its own, and the identifier check on changed and untracked files. |
| B15 | :349-350 | stale | `journal_other.go` is part of the platform split. |
| B16 | :397-428 | wrong, gap | The publish workflow's first step checks `gh`'s capabilities; its permissions are not stated. |
| B17 | :142-143 | inconsistent | `apicheck` compares with the newest `v1.*` tag, not `v1.0.1`. |
| B18 | :230-235 | gap | launchd's `CleanupHandOffs`. |
| B19 | :223-229 | gap | systemd's `Notify` wrappers and unit-name helpers. |

### `docs/guides/extending-selfupdate.md` (W8)

| ID | Where | Kind | Finding, and the fix |
| :--- | :--- | :--- | :--- |
| C1 | :506-509, :600-604 | wrong; a defect (W1) | `StopGrace` is spent inside `Poll.Timeout`, and a grace not shorter than it ends `Stop` without ending the task. After W1, describe the refusal in `New`. |
| C2 | :621-623 | gap; a defect (W1) | A failed `Stop` with the program running leaves the task disabled. After W1, describe that it is enabled again. |
| C3 | :559-561 | wrong | The trailing-dot and trailing-space spellings are refused since `v1.15.0`, as `::$DATA` is. |
| C4 | :680-684 | wrong | Without the handoff, every backend's `Stop` refuses with `service.ErrInsideService` and the run ends; name the error. |
| C5 | :502-503, :608 | gap | `v1.15.1`'s liveness check, and `Start`'s filter of the instance `Stop` recorded. |
| C6 | :761-766 | gap | Each backend's live-test switch, the integrity and administrator conditions, and `TestLiveTaskUpdate`'s absence from CI. |
| C7 | :498-537 | gap | `service`'s typed errors, `Poll`'s defaults (60 s, 250 ms), `HealthProbe` and `Health`. |
| C8 | :519-521 | gap | The exec reconciler: `Reconcile`, `Restore` or `RestoreFunc`, the `Receipt` JSON and its verdicts, `ExecState`. |
| C9 | :554-556 | inconsistent | The batch refusal's sources: the version probe, `ExecRunner` (so `ExecReconciler`, `ghattest`), `DetachProcess`; not codesign or the backends. |
| C10 | :529-532 | inconsistent | Task Scheduler's rewrite sits in the "Since `v1.10.1`" list; it is `v1.14.0`'s. |
| C11 | :535-537 | inconsistent | "on Windows" for the SCM-only dependents rule: "on the SCM". |
| C12 | :7-9 | inconsistent | Four sections lack the promised runnable pointer. Add them. |
| C13 | :422-423 | gap | `ManagedInstaller.KeptBackups`, and `CleanupPending` on the inner installer. |
| C14 | :193, :637 | inconsistent | "0004-MADR §4" unlinked; "0159, probe 7" by number. Full filenames. |
| C15 | :722 | inconsistent | The result example shows four keys; mark it abridged. |
| C16 | :687-697 | broken | The `main` snippet fails: `declared and not used: err`. |
| C17 | :735-738 | gap | "Without `cli`": `HandOffIfInside` checks neither `--yes` nor whether the run would install. |
| C18 | :343-344, :385-386 | gap | `selfupdate.ChainTransformers`. |
| C19 | :772-777 | gap | `selfupdatetest`'s constructors and methods. |

### The migration and building guides (W9)

| ID | Where | Kind | Finding, and the fix |
| :--- | :--- | :--- | :--- |
| D1 | migrating:14-18 | inconsistent | §1 names Go 1.27.1 only; build and test with 1.27.2. |
| D2 | building:147-148, :478; migrating:700-701 | wrong; a defect (W3) | `install.ps1` does not give hooks or the identity command empty standard input. After W3, the claim holds. |
| D3 | migrating:172-174, 415-419, 467-468, 742-746, 775 | stale | At `v1.15.1` the result is `schema_version` 4; §5 should point at §6 to §15. |
| D4 | migrating:321, :338 | inconsistent | "0004-MADR, owner decision 1" unlinked; C3 and C4 without their record. |
| D5 | migrating:388 | gap | No "From v1.5.0 to v1.5.1" section; no note for a go-core-lib reader. |
| D6 | building:141, :225 | gap | The arm64 identity runners may not exist for a private repository. |
| D7 | building:190-196, 322-341 | gap | The module must require the library; an extra with no `path` and no extras artifact fails. Both messages in §9. |
| D8 | building:47-48, 64-66 | gap | An extra's `path` is relative to the repository root and must be tracked; the example looks generated. |
| D9 | building:197-212 | gap | The `-tools` artifact, its one-day retention, and the re-run windows. |
| D10 | building:238-240 | inconsistent | The rehearsal stamp is the commit's first 12 hex. |
| D11 | building:313-314 | wrong | setup-go's cache is off, not Go's. |
| D12 | building:436-437, :459 | gap | `.old-<guid>` and `.bad-<guid>` files, which uninstall leaves. |
| D13 | building:55-71, §4 | gap | The spec's limits and the jobs' timeouts. |

### Go doc comments and Examples (W4, W5)

| ID | Where | Kind | Finding, and the fix |
| :--- | :--- | :--- | :--- |
| E1 | `selfupdate/types.go:22` | stale | `ReleaseBuild` "a strict vMAJOR.MINOR.PATCH tag": prerelease tags too. |
| E2 | `types.go:94`, `:98`, `:100`; `errors.go:48`; `checker.go:48` | stale | "stable" release, from before channels. |
| E3 | `checker.go:69`, `:18` | stale | `Availability.AssetName` and `CheckerConfig.Assets` predate archives. |
| E4 | `types.go:670-675`; `warnings.go:8-10` | inconsistent | `EventWarning` and `Warnings` omit `ReconcileResult.Warnings`. |
| E5 | `internal/winproc/winproc.go:1-10` | stale | The package doc omits `Descends` and the Windows-only `Alive`; "its plan's Q1" is ambiguous. |
| E6 | `service/launchd/lifecycle.go:87` | broken | Names the unexported `stopBound`. State the bound. |
| E7 | `archive/unpack.go:53` | broken | Names the unexported `checkPortable`. State the rules. |
| E8 | `selfupdate/github.go:372` | broken | Names the unexported `listPageSize`. Say 30. |
| E9 | `service/execreconciler.go:35-48` | gap | `Receipt`'s eight fields and decode rules are undocumented. |
| E10 | `execreconciler.go:36`, `:68`; launchd `lifecycle.go:86`; taskscheduler `lifecycle.go:109-110` | inconsistent | Facts cited to another repository. Restate them, cite this repository's records. |
| E11 | `service/handoff.go:150`; `archive/select.go:70-72`; the backup and state types; `selfupdatetest`'s package doc | inconsistent | Another package's identifiers written unqualified. |
| E12 | `releasespec/validate.go:44` | stale | `Validate` checks 0014-MADR's rules too. |
| E13 | `selfupdate/doc.go:158-163` | gap | Publication names only the publish workflow; `archive`, `codesign`, `releasespec` unmentioned. |
| E14 | `types.go:274`; `selfupdatetest.go:131`, `:145` | gap; a defect (W2) | `FakeSource` does not wrap `ErrNoRelease`; `ReleaseSource` does not state the contract. |
| E15 | `types.go:213-214` | stale | `GitHubOptions.Token`'s fallback omits `Credentials`. |
| E16 | `types.go:809` | gap | `Config.Verifiers`: a refusal matches `ErrIntegrity`. |
| E17 | taskscheduler `lifecycle.go:107-113` | wrong | `Stop` step 5 runs only when the program still runs after the grace. |
| E18 | systemd `detach.go:80-84` | gap | `Detach` needs `Options.Unit` and systemd 236. |
| E19 | `checkcache.go:143-147` | gap | The back-off is capped at one hour. |
| E20 | `probe.go:75-82` | gap | `NewVersionProber`'s refusals and its 64 KiB limit. |
| E21 | 98 non-test files | inconsistent | Records cited by number; D2-A decides which change. |
| E22 | `buildinfo`, `service`, four backends, `selfupdatetest` | gap | No Example (D3-A). |
| E23 | `selfupdate/example_test.go:18-59` | stale | `ExampleNew_standalone` hard-codes what `UserAgent` and `buildinfo` supply. |
| E24 | `internal/cmd/selfupdate-release/main.go:1-5` | gap | The package doc omits 0014-MADR. |
| E25 | `internal/testproc/testproc.go:63-67` | gap | `Start` kills the helper when its budget runs out. |

### Seen outside the five scopes (W4, W9)

| ID | Where | Kind | Finding, and the fix |
| :--- | :--- | :--- | :--- |
| O1 | `.golangci.yml:121` | stale | "its three backends": four since 0021. |
| O2 | migrating:82 | stale | The loopback-redirect wording of A1. |
| O3 | `.golangci.yml:51`, `:53` | inconsistent | depguard `desc` cites "0004-MADR" by number. |
| O4 | `scripts/check-docs.sh:5` | inconsistent | "Two modes"; it has three. |
| O5 | `scripts/check-workflows.sh:4` | inconsistent | "Three rules"; it has four. |

## Implementation Steps

### Phase W0: records

1. `HEAD` is `c2eded4` or a records-only descendant, on `main`.
2. `docs/README.md` gains the pair's rows after 0023's.
3. On approval: the MADR records the owner's decisions; this plan
   becomes `in-progress` with `### Approval`.
4. Checks; commit.

### Phase W1: Task Scheduler, C1 and C2 (D4-A)

1. **C1:** `New` refuses an `Options.StopGrace` not shorter than the
   effective `Options.Poll.Timeout` (its default when zero), with an
   error naming both. `StopGrace`'s doc says so.
   *(Deviation D1, 2026-10-12: only a grace the program set, with
   `RequestStop` set, is refused; a zero grace becomes the shorter of
   `DefaultStopGrace` and half the effective timeout.)*
2. **C2:** when `Stop` returns an error after it disabled the task, and
   the recorded program still runs (`running(pid, born)`), it enables the
   task again and clears its record that it disabled it, joining any
   error from that. `Stop`'s doc says a failed stop leaves the task as it
   found it while the program runs.
3. **Tests,** over the fake transport in `lifecycle_test.go`: `New` with
   `StopGrace` equal to `Poll.Timeout` fails, one shorter succeeds; a
   `Stop` whose end fails with the program still running leaves the task
   enabled; one whose program is gone leaves it disabled for `Start`.
4. **Seen failing:** a copy without C1's check, and a copy without C2's
   re-enable, each fails its test; the record quotes the lines.
5. **The Windows test host,** the console sign-in confirmed: the live
   suite, `SELFUPDATE_REQUIRE_TASKSCHED=1 -run '^TestLiveTask'`, `PASS`.
6. Checks; commit.

### Phase W2: `FakeSource` and `ErrNoRelease`, E14 (D4-A)

1. `FakeSource.Latest` and `ByTag` wrap `selfupdate.ErrNoRelease` for a
   missing release, keeping their messages' prefix. `ReleaseSource`'s doc
   says a missing release wraps `ErrNoRelease`.
2. A test: `errors.Is(err, selfupdate.ErrNoRelease)` for both, and a
   `Checker.CheckCached` over the fake reporting `CheckNoRelease`.
3. Seen failing on a copy without the wrap. Checks; commit.

### Phase W3: `install.ps1`'s standard input, D2 (D4-A)

1. **The test first:** the installer's Windows test
   (`internal/cmd/selfupdate-release/installer_ps_test.go`) gains a case
   whose hook and identity command report whether their standard input
   is at end of file, with the installer's own standard input holding
   data. On the Windows test host, at `c2eded4`'s `install.ps1`, it fails
   (the record quotes it): that is the measurement.
2. **The fix:** `install.ps1` runs each hook and the identity command
   with empty standard input, as `install.sh` does.
3. On the Windows test host the case passes; the installer's tests on
   this Mac and the Linux test host pass.
4. Checks; commit.

### Phase W4: Go doc comments, citations and headers (D1-A, D2-A)

1. E1 to E13 and E15 to E20, E24, E25 as the table says; E14's doc in
   W2; E17 after W1.
2. **E21, by D2-A:** a search for every number-only citation in Go
   comments; each that names a PLAN number with more than one PLAN, a
   report, an anchor with no record, or another repository is changed.
   The record lists them. AGENTS.md's "Records" gains D2-A's rule.
3. O1, O3, O4 and O5.
4. Checks: `go doc` for each changed package; vet for three targets;
   `make pre-add-check`; commit.

### Phase W5: the Examples (D3-A)

1. `buildinfo`: `ExampleIdentity`. `selfupdatetest`: an Example with
   `FakeSource` and an `Updater` dry run, with `// Output:`. systemd,
   launchd, scm, taskscheduler: a compile-only `ExampleNew` wiring the
   backend into `selfupdate.NewManagedInstaller`. `selfupdate`:
   `ExampleNew_standalone` uses `UserAgent` and `buildinfo`, and names
   `releasespec` (E23).
2. `docs/architecture.md`'s counts for every package that gains a file.
3. `go test -run '^Example' ./...` and vet for three targets; seen
   failing: one Example's `// Output:` planted wrong fails. Checks;
   commit.

### Phases W6 to W9: the documents

Each phase closes its table rows. Snippets are built as Rules say. W8's
C1 and C2 describe W1's behaviour. W9 adds the migration guide's "From
v1.15.1 to v1.15.2" section, moves its current version to `v1.15.2`, and
names W1's, W2's and W3's changes. Checks; one commit per phase.

### Phase W10: `v1.15.2`

[0017-PLAN-verify-build-provenance-and-close-0015-open-items.md](0017-PLAN-verify-build-provenance-and-close-0015-open-items.md)'s
"Release procedure", as
[0022-PLAN-error-chain-and-process-liveness.md](0022-PLAN-error-chain-and-process-liveness.md)
Q5 ran it: CI green and pushed; the tag on the owner's ask; the tag's
CI and identity legs; the pin commit; the proxy; release notes.

### Phase W11: closing

`### Verification, at closing`, each finding's outcome, `status:
complete`, the index rows; the MADR stays `accepted`.

## Verification

* **V1 (findings).** Every row above is closed, or recorded as left with
  the reason.
* **V2 (defects).** C1's, C2's, D2's and E14's tests failed on their
  plants and pass; the Task Scheduler live suite and the installer's
  Windows test pass on the Windows test host.
* **V3 (citations).** The W4 search finds no Go comment citation of the
  kinds D2-A changes; AGENTS.md states the rule.
* **V4 (Examples).** Every Example passes, and every package D3-A names
  has one.
* **V5 (checks).** Links, identifiers, the architecture table and
  markdownlint are clean; `make gate` ends `overall=0` at each phase;
  `apicheck` is compatible with `v1.15.1`.
* **V6 (release).** `v1.15.2` is tagged, its CI passed, and the proxy
  serves it.

## Rollout and Rollback

* **Rollout:** one commit per phase, on `main`, pushed on the owner's
  ask; `v1.15.2` at W10.
* **Rollback:** `git revert` of a phase. A released `v1.15.2` is fixed
  forward.
* **A deviation in any phase** stops it.

## Execution record

### Approval (2026-10-12)

The owner said "follow recommendations and proceed": D1-A, D2-A, D3-A
and D4-A. The MADR is `accepted` and this plan `in-progress`, here and
in the index. The pair and its rows were not yet committed; they go in
with W0's commit.

### Deviation D1 (2026-10-12): C1's refusal would break working configurations

* **Found** reading `New` and `Stop` before W1's first change. With the
  default `StopGrace` of 10 s (`task.go:32`), a program with
  `RequestStop` set and a `Poll.Timeout` of 10 s or less would be refused
  by `New` as step 1 was written, though it works today when its program
  exits on request in time (`lifecycle.go:170-176`). `StopGrace` is
  unused without `RequestStop`. Not pre-existing; found in the plan's
  design.
* **Offered:** 1, refuse only a grace the program set, with
  `RequestStop`, and shorten a defaulted one to the shorter of 10 s and
  half the timeout (recommended); 2, no refusal, and `Stop` caps the
  grace at half the timeout, silently shortening a long grace a program
  set; 3, no code change for C1, documented. Doing nothing would have
  broken those programs at `New`.
* **Decision (the owner, "go with recommendations. proceed."): 1.** The
  MADR carries Amendment A1. `TestStopRequestFails`' one-hour grace,
  which A1 refuses, becomes one shorter than the test's timeout; its
  assertion is unchanged.

### W1 (2026-10-12)

**Files.** Changed: `selfupdate/service/taskscheduler/task.go`,
`lifecycle.go`, `task_test.go`, `lifecycle_test.go`. No file is added.

**As built:**

* **C1, as Amendment A1 decided.** `New` validates `Poll` first, then:
  a zero `StopGrace` becomes `min(DefaultStopGrace, timeout/2)`, where
  `timeout` is `Poll.Timeout` or its default; with `RequestStop` set, a
  `StopGrace` not shorter than `timeout` is refused with `Options.StopGrace
  <d> is not shorter than Options.Poll.Timeout <d>, which it is spent
  inside`. `StopGrace`'s and `DefaultStopGrace`'s docs say so.
* **C2.** After `Stop` disables the task and has an instance to end, a
  deferred check runs on any error: when the task was enabled and the
  recorded process still runs (`running(pid, born)`), `enableAgain`
  enables it and clears the record that `Stop` disabled it, joining any
  error from that. `Stop`'s doc says a failed stop leaves the task as it
  found it while the program runs.
* **Tests.** `TestOptionsValidated` gains three grace cases and the
  shortened default (a 4 s timeout gives 2 s); `TestStopFailureEnablesAgain`
  is new, for both outcomes; `TestStopRequestFails`' grace is 1.5 s, under
  its 2 s timeout, as Deviation D1 says.

**Seen failing (step 4),** each a `scripts/plant-copy.sh` copy:

| Plant | Failing line |
| :--- | :--- |
| the refusal removed | `task_test.go:49: grace not under the timeout, with RequestStop: <nil>, want ok false` |
| the default not shortened | `task_test.go:62: a default grace with a 4 s timeout: 10s, <nil>; want 2s (0024-MADR Amendment A1)` |
| no re-enable | `lifecycle_test.go:227: calls ["enable false" "stop"], enabled false; want ["enable false" "stop" "enable true"] and the task enabled` |

**The Windows test host (step 5),** the owner having confirmed the
console sign-in ("signed in proceed"): a clone at `c2eded4` with the diff
applied (seven files, SHA-256 matched), Go 1.27.2,
`SELFUPDATE_REQUIRE_TASKSCHED=1 -run '^TestLiveTask'`: all six `PASS`
(`ok … 90.201s`), `TestLiveTaskUpdate` logging `the update's integrity
RID 0x2000; pid=<n> open=ok wait=timeout`. The package's unit tests
passed there too; afterwards `schtasks /query` lists 0 `selfupdate-`
tasks.

**Checks.** gofmt clean; vet for darwin, linux and windows: clean. `make
pre-add-check FILES="<the 4 Go files>"`: `go-precheck: 4 file(s) clean`.
`make gate` on this Mac, load average 8.75 before and 52.47 after: every
step `rc=0`, `apicheck` `compatible with v1.15.1`, `overall=0`.

### W2 (2026-10-12)

The owner committed W0 and W1 as `597d560`, and said "committed,
proceed".

**Files.** Changed: `selfupdate/selfupdatetest/selfupdatetest.go`,
`selfupdatetest_test.go`, `selfupdate/types.go`.

**As built:**

* `FakeSource.Latest` with no latest release, and a missing tag through
  `ByTag` and `Latest`, return `selfupdatetest: no latest release:
  selfupdate: no such release` and `selfupdatetest: no release "<tag>":
  selfupdate: no such release`, wrapping `ErrNoRelease`. The prefixes are
  unchanged, and nothing in the repository matched the old text.
  `OpenAsset`'s errors are unchanged. `NewFakeSource`'s doc says so.
* `ReleaseSource`'s doc: `Latest` and `ByTag` wrap `ErrNoRelease` for a
  release that does not exist, and `CheckCached` saves it as
  `CheckNoRelease` (E14).
* `TestFakeSourceNoRelease`: both methods match `ErrNoRelease`, and a
  `Checker.CheckCached` over the fake returns `CheckNoRelease` with an
  error matching it.

**Seen failing:**

| Plant | Failing line |
| :--- | :--- |
| `Latest` without the wrap | `selfupdatetest_test.go:181: Latest with no latest release = selfupdatetest: no latest release, want ErrNoRelease` |
| `lookup` without the wrap | `selfupdatetest_test.go:185: ByTag of an unknown tag = selfupdatetest: no release "v9.9.9", want ErrNoRelease` |

**Checks.** `go test ./selfupdate/selfupdatetest/ ./selfupdate/`: `ok`.
`make pre-add-check FILES="<the 3 Go files>"`: `go-precheck: 3 file(s)
clean`. `make gate` on this Mac, load average 18.41 before and 56.26
after: every step `rc=0`, `apicheck` `compatible with v1.15.1`,
`overall=0`.

### Deviation D2 (2026-10-12): the gate before each push

* **Found:** `make gate` and the pre-add check take several minutes each
  on this Mac under load (load average 56 during W2's gate), and the
  owner asked "what is taking so long".
* **Offered:** keep the gate per commit, or run it once before each push
  for the documentation phases.
* **Decision (the owner, "committed, proceed, gate before push only"):**
  `make gate` runs once before each push, for every phase. The pre-add
  check stays on every changed Go file, as AGENTS.md requires before
  staging, and the documentation checks stay per commit.

### W3 (2026-10-12)

The owner committed W2 as `820b7e2`, and said "committed, proceed, gate
before push only".

**Files.** Changed:
`internal/cmd/selfupdate-release/installer/install.ps1`,
`internal/cmd/selfupdate-release/installer_ps_test.go`.

**The test first (step 1).** The stand-in program the installer's tests
use for hooks and identity checks gains `STANDIN_STDIN`: with it set, it
reads its standard input to the end and logs `<role> stdin <bytes>`.
`psCase` gains `stdin`, the installer's standard input. The new case,
"hooks and the identity check get no standard input" (all three modes),
places the old stand-in, publishes the new one, gives the installer
`typed at the console\n` (21 bytes), and requires at least three reads,
each of 0 bytes: the `before_install` hook, the identity check and the
`after_install` hook.

**The measurement,** the case run on the Windows test host against
`install.ps1` as it was, Windows PowerShell 5.1 and PowerShell 7, three
modes each: every run failed with

```text
installer_ps_test.go:750: standard input reads ["old stdin 21" "new stdin 0" "new stdin 0"]; want the two hooks and the identity check, each reading nothing
```

The first program the installer ran, the `before_install` hook, read the
installer's standard input; the later ones read nothing only because it
was spent. D2 is [measured]: in a console, that input is the user's.

**The fix (step 2).** Each hook runs as `@() | & $exe @hookArgs | …`,
and the identity command as `$first = @() | & (Join-Path …) @idArgs
2>$null | …`: an empty pipeline gives the program a standard input that
is closed at once. A comment at the hook says why.

**After the fix (step 3):**

* the case on the Windows test host: all six `PASS`;
* **seen failing for the identity check alone:** the host's scratch clone
  with the hook fixed and the identity command not: `standard input reads
  ["old stdin 0" "new stdin 21" "new stdin 0"]`, `FAIL`. The case guards
  each call on its own;
* the whole package: `ok` on the Windows test host (311 `--- PASS`
  lines), on this Mac (`ok … 43.479s`), and on the Linux test host
  (`ok … 17.315s`).

**Checks.** gofmt clean; vet for windows: clean. `make pre-add-check
FILES="internal/cmd/selfupdate-release/installer_ps_test.go"`:
`go-precheck: 1 file(s) clean`. `make gate` runs before the push
(Deviation D2).
