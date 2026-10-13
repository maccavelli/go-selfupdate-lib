---
status: complete
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

### Deviation D3 (2026-10-12): the empty input wrote a byte-order mark on CI

The owner committed W3 as `05ff0f3` and pushed it, with `make gate` not
yet run (Deviation D2), and said "committed, pushed, proceed".

* **Found:** CI run `38306877278`, on `05ff0f3`, failed
  `validate (windows-2025)` at `go test ./...`, in Windows PowerShell 5.1
  only, all three modes:
  `installer_ps_test.go:750: standard input reads ["old stdin 3" "new
  stdin 3" "new stdin 3"]`. `main` was red. 16 other jobs passed.
* **Measured** on the Windows test host with a scratch probe (a program
  that reports what it reads, run as `@() | & prog`), Windows PowerShell
  5.1.26100 and PowerShell 7.6:

  ```text
  powershell 5.1: default_OutputEncoding=us-ascii preamble=0
    default -> read=0; utf-8 preamble=3 -> read=3 hex=efbbbf;
    utf-8 preamble=0 -> read=0; us-ascii -> read=0
  pwsh 7.6: default_OutputEncoding=utf-8 preamble=0
    the same four results
  ```

  An empty pipeline still writes `$OutputEncoding`'s byte-order mark. The
  Windows test host's defaults have none, so W3 passed there; CI's
  Windows PowerShell 5.1 defaults to UTF-8 with one [derived from the
  failure; the runner's setting was not read].
* **Offered:** 1, a BOM-less `$OutputEncoding` local to the two functions,
  with a test that reproduces CI's setting (recommended); 2, revert W3;
  3, start hooks through .NET's `Process`. Doing nothing would leave
  `main` red.
* **Decision (the owner, "follow recommendations"): 1.**
* **As built:** `Invoke-InstallHook` and `Test-Identity` set `$OutputEncoding
  = New-Object System.Text.UTF8Encoding $false` first; it is local to
  each function, and the session checks still find no new variable. The
  test driver for the iex and scriptblock forms sets `$OutputEncoding =
  [Text.Encoding]::UTF8` when `SELFUPDATE_INSTALL_TEST_BOM` is set; the
  case's body moves into `expectNoStdin`, and a second case, "…under a
  BOM encoding", runs it with that set, in those two modes.
* **Seen failing,** the Windows test host's scratch clone without the two
  `$OutputEncoding` lines: the BOM case failed in both PowerShells and
  both modes with `installer_ps_test.go:766: standard input reads ["old
  stdin 3" "new stdin 3" "new stdin 3"]`, as CI's did. With them: all 10
  standard-input cases `PASS`.
* **Checks:** the whole package `ok` on the Windows test host (315 `---
  PASS` lines), this Mac and the Linux test host. `make pre-add-check
  FILES="internal/cmd/selfupdate-release/installer_ps_test.go"`: clean.
  `make gate` on this Mac (the tree also held W4's uncommitted changes),
  load average 10.05 before and 47.81 after: every step `rc=0`,
  `apicheck` `compatible with v1.15.1`, `overall=0`.

### W4 (2026-10-12)

The owner committed Deviation D3's fix and W4 together as `8e3d830` (43
files), pushed, and said "i committed and pushed all, ci is running".

**Files.** 36 Go files, `.golangci.yml`, `scripts/check-docs.sh`,
`scripts/check-workflows.sh` and `AGENTS.md`. No file is added.

**Closed, as the table says:**

* **E1 to E4:** `ReleaseBuild` admits a prerelease tag under
  `NewSemverPolicy`; `OperationUpgrade`, `OperationRollback`,
  `OperationReplaceLocal`, `ErrLatestOlder` and
  `CheckRequest.TargetVersion` no longer say "stable" only, the last
  copying `Request.TargetVersion`'s wording; `CheckerConfig.Assets` and
  `Availability.AssetName` name archives; `EventWarning` and `Warnings`
  name `ReconcileResult.Warnings`.
* **E5:** `winproc`'s package doc names `Descends`, `Alive` and
  `AliveAccess`, and `Alive`'s doc names
  `0022-PLAN-error-chain-and-process-liveness.md` Q1.
* **E6 to E8:** launchd's `Stop` states its bound (the longer of
  `Poll.Timeout`, 60 s by default, and `ExitTimeOut`, 5 s by default, plus
  30 s); `UnpackOptions.Member` states the portable-name rules;
  `ListReleases` says 30 at a time.
* **E9, E10:** `Receipt` documents its fields, the decode rules and when
  a warning or a restore follows; `ExecOptions.RestoreFunc`, launchd's
  `Stop` and taskscheduler's `Stop` cite this repository's records for
  the facts they had cited to another.
* **E11:** `selfupdate.` qualifies `ErrConfirmationRequired`,
  `AssetSelector`, `NewExactAssetSelector`, `ReconcileResult.State` in the
  five backup and state types, and `ReleaseSource` in `selfupdatetest`.
* **E12, E13, E15, E16:** `Validate` cites 0014-MADR too; the package doc
  names `archive`, `codesign`, `releasespec` and both workflows with the
  building guide; `GitHubOptions.Token` names `Credentials` first;
  `Config.Verifiers` says a refusal matches `ErrIntegrity`.
* **E17 to E20:** taskscheduler's `Stop` step 5 runs only when the
  program still runs; systemd's `Detach` names `Options.Unit` and
  systemd 236; `CheckCached` states the one-hour cap; `NewVersionProber`
  states its refusals and the 64 KiB limit.
* **E24, E25:** the release tool's package doc cites 0014-MADR;
  `testproc.Start` says it kills the helper when the budget runs out.
* **O1, O3 to O5:** "its four backends"; the depguard `desc` strings cite
  0004-MADR by full filename; "Three modes"; "Four rules".

**E21, by D2-A.** The search found, and W4 changed:

* PLAN numbers with several PLANs (0004, 0010, 0021, 0022), cited short:
  `scheduler_windows.go` (0021-PLAN T6), `live_windows_test.go` (0021-PLAN
  Deviation D3; 0022-PLAN P1), `api.go`, `fake_test.go`,
  `winproc_windows.go` and `winproc_windows_test.go` (0022-PLAN P1); each
  now names the full filename;
* anchors that named no record, outside test files: `cli/run.go` (F7,
  F8) and `cli/flags.go` (F4), now `0004-MADR amendment …`;
  `service/doc.go` (A3, A4), now 0011-MADR's; `archive/doc.go` (E8, 4C),
  now 0015-MADR's and 0017-MADR's; `ghattest/doc.go`, now
  `docs/guides/building-releases.md`, step 4;
* another repository's record by number: `winproc_windows_test.go` and
  `taskscheduler/main_test.go` ("0159 …"), now 0021-MADR's F7 and F9
  rows.

Left, as D2-A allows: anchors in test files whose header comment names
the record (twelve files, each checked); citations of another
repository by repository and filename beside this repository's record
(`github.go`, `winproc_windows.go`); test comments that mention
magic-cli-remote as history, not as a citation. No report is cited in
Go. After the change the search finds none of the kinds D2-A changes.
AGENTS.md's "Records" gains D2-A's rule.

**Checks.** gofmt clean; `go vet ./...` for linux, darwin and windows
(`CGO_ENABLED=0`): clean. `make pre-add-check FILES="<the 35 Go
files>"`: `go-precheck: 35 file(s) clean`. The 36th,
`taskscheduler/main_test.go` (the "0159 F9" citation), was changed after
that run; gofmt and vet were clean on it, and `make gate`'s lint covered
it. `go doc` read back for the
changed packages. On AGENTS.md: links, identifiers and markdownlint
clean. `make gate` ran with W4 in the tree (Deviation D3's checks):
`overall=0`.

### Deviation D4 (2026-10-12): D3's fix failed on CI too

* **Found:** CI run `38310786338`, on `8e3d830` (D3's fix with W4),
  failed `validate (windows-2025)` at `go test ./...`, Windows PowerShell
  5.1 only: both standard-input cases, `["old stdin 3" "new stdin 3"
  "new stdin 3"]`. The fix was in the commit (`install.ps1:266`, `:292`).
  `main` is red; 16 other jobs passed.
* **Measured and researched:** MADR Amendment A2. The bytes come from
  .NET Framework's standard-input writer, which encodes with
  `[Console]::InputEncoding`, not from `$OutputEncoding`. The host's
  console input is IBM437; CI's is UTF-8. `Start-Process` with an empty
  file gives 0 bytes in both editions under either BOM.
* **The owner asked** to evaluate the recommendation against official
  documentation, public practice and both editions, and to improve it.
  The recommendation changes from setting `[Console]::InputEncoding`
  (A2-2) to `Start-Process` with an empty file (A2-1).
* **Decision (the owner, "follow the new recommendations. accept the
  madr, approved to execute the plan"): A2-1.** For W3 again (files:
  `installer/install.ps1`,
  `installer_ps_test.go`):
  1. one function in `install.ps1` starts a program with no input, as
     A2-1 says, and returns its exit code, or its first output line for
     the identity check; the hooks and `Test-Identity` call it; D3's
     `$OutputEncoding` lines go;
  2. the test driver's `SELFUPDATE_INSTALL_TEST_BOM` sets both
     `[Console]::InputEncoding` and `$OutputEncoding` to UTF-8 with a BOM,
     and puts the console's back in `finally`. The console is shared by
     the parallel cases while it is set; after the fix no case's result
     depends on it;
  3. **seen failing:** the BOM case against `8e3d830`'s `install.ps1` on
     the Windows test host, as CI failed, then passing with the fix;
  4. the hook cases still see each hook's output, its exit code, and a
     failed `before_install` stopping the install; the whole package on
     the three hosts; then CI on the push.
* **Doing nothing** leaves `main` red.

### Deviation D4, as built (2026-10-12)

**Files.** Changed: `internal/cmd/selfupdate-release/installer/install.ps1`,
`internal/cmd/selfupdate-release/installer_ps_test.go`.

* **`install.ps1`:** a new `Invoke-NoInput` starts a program with
  `Start-Process -NoNewWindow -PassThru -RedirectStandardInput` an empty
  file from `[IO.Path]::GetTempFileName()`, reads `.Handle` at once,
  polls `HasExited` every 50 ms, then `WaitForExit()`, and returns
  `ExitCode`, or -1 when `Start-Process` fails. With an output file, it
  redirects standard output there and standard error to a discarded
  temporary file. The temporary files are removed in `finally`. A
  comment gives each reason (A2). `Invoke-InstallHook` runs each hook
  through it, the hook writing straight to the installer's output;
  `Test-Identity` runs the identity command into a temporary file and
  reads its first line with `Get-Content -TotalCount 1`. D3's
  `$OutputEncoding` lines and the pipelines are gone.
* **The test driver** sets `$OutputEncoding` and `[Console]::InputEncoding`
  to UTF-8 with a BOM when `SELFUPDATE_INSTALL_TEST_BOM` is set, and puts
  the console's back before it reports; the case's comment says why.

**Seen failing,** the Windows test host's clone at `8e3d830` with only
the test changed (`install.ps1` as D3 left it): every Windows PowerShell
5.1 standard-input case failed with `standard input reads ["old stdin 3"
"new stdin 3" "new stdin 3"]`, as CI did; every PowerShell 7 case
passed. The plain 5.1 case failed too, because the BOM case's console
setting is shared by the cases running in parallel, as step 2 foresaw.

**After the fix:** the whole package on the Windows test host, `ok`,
315 `--- PASS` lines (the standard-input cases in both editions, and the
hook cases: each hook's output, a failed `before_install` stopping the
install, a failed `after_install` exiting 3); on this Mac `ok … 45.310s`;
on the Linux test host `ok … 16.822s`.

**Not tested:** a hook that starts a long-lived program. Without `-Wait`
the installer waits only for the hook's own process, as before; no case
starts one.

**Checks.** vet for windows: clean. `make pre-add-check
FILES="internal/cmd/selfupdate-release/installer_ps_test.go"`:
`go-precheck: 1 file(s) clean`. `make gate` on this Mac, load average
14.17 before and 49.81 after: every step `rc=0`, `apicheck` `compatible
with v1.15.1`, `overall=0`. CI runs on the push.

### W5 (2026-10-12)

The owner committed D4's fix as `f99f5e6`, pushed, and said "committed
and pushed, proceed".

**Files.** New: `buildinfo/example_test.go`,
`selfupdate/selfupdatetest/example_test.go`, and an `example_test.go` in
each of `selfupdate/service/systemd`, `launchd`, `scm` and
`taskscheduler`. Changed: `selfupdate/example_test.go`,
`docs/architecture.md`.

**As built (D3-A, E22, E23):**

* `buildinfo`: `ExampleLDFlags` (the exact `-X` string) and
  `ExampleIdentity` (a test binary is `local`, and `Current` is
  non-empty), each with `// Output:`.
* `selfupdatetest`: `ExampleNewFakeSource`, a dry run of an `Updater`
  over the fake with a stand-in target in a temporary directory:
  `v1.1.0 true false <nil>`, and the fake recorded its calls.
* Each backend: a compile-only `ExampleNew` that wires the backend into
  `selfupdate.NewManagedInstaller` as both its `Lifecycle` and its
  `Reconciler`, and says `New` returns `service.ErrUnsupported` off its
  OS.
* `selfupdate`'s `ExampleNew_standalone` builds its User-Agent with
  `selfupdate.UserAgent` and `buildinfo.Identity().Current()`, and its
  comment names `releasespec`'s `Spec.AssetSelector` and `Spec.Unpacker`
  and the building guide's step 2.
* `docs/architecture.md`: the test-file counts of the six packages
  (`buildinfo` 3, `selfupdatetest` 2, systemd 18, launchd 14, scm 13,
  taskscheduler 11); `--arch`: `16 rows … 0 differ`.

**Deviation from step 1's text:** the first `ExampleNewFakeSource`
ran its dry run against the test binary and failed with "target … is
outside allowed roots"; a dry run still checks the install target. The
Example now gives the installer a stand-in binary in a temporary
directory, as `selfupdate`'s own Examples do. It is still the dry run
step 1 names.

**Seen failing (step 3):** a `plant-copy.sh` copy with `ExampleIdentity`'s
output planted as `release`: `--- FAIL: ExampleIdentity`, `got: local`,
`want: release`.

**Checks.** `go test -run '^Example'` on `buildinfo`, `selfupdate`,
`selfupdatetest` and the four backends: `ok`. vet for linux, darwin and
windows: clean. `make pre-add-check FILES="<the 7 Go files>"`:
`go-precheck: 7 file(s) clean`. `make gate` runs before the push
(Deviation D2).

### Deviation D5 (2026-10-12): Windows PowerShell 5.1's `Start-Process` loses a fast hook

* **Found:** CI run `38315770402`, on `f99f5e6`, failed
  `validate (windows-2025)`: `TestInstallPs1/powershell/file/a_running_copy_is_replaced`,
  `installer_ps_test.go:1016: exit 1, want 0`, the output `running relay
  hook-before`, `old ran hook-before`, `install: before_install hook
  failed`. Every standard-input case passed; 16 other jobs passed. `main`
  is red.
* **Measured on the Windows test host:** 100 starts with `-PassThru` and
  the handle read at once, and 100 with `-Wait -PassThru`, lost no exit
  code in either edition; 300 more under busy loops on every core lost
  none and threw none. The race was not reproduced there.
* **The probe's own cleanup failed:** the under-load probe started one
  busy loop per core and did not end them; a check found 32 still
  running, and stopped them all (`left=0`).
* **Researched:** MADR Amendment A3: PowerShell 7 fixed the start race
  in 2023 (#20749); Windows PowerShell 5.1 did not.
* **Decision (the owner, "follow recommendations, proceed"): A3-1.**
* **Offered:** A3-1, `CreateProcess` from a C# helper (recommended);
  A3-2, split by edition; A3-3, revert W3. Doing nothing leaves `main`
  red.
* **W5** (the Examples) is done and uncommitted; it does not depend on
  this.

### Deviation D5, as built (2026-10-12)

**Files.** Changed: `internal/cmd/selfupdate-release/installer/install.ps1`.

* **`Invoke-NoInput`** keeps its signature and callers. It compiles,
  once per session, `SelfupdateInstall.NoInput` with `Add-Type`, as
  `Send-SettingChange` compiles its helper, and starts the program
  through it: `CreateProcess` with `STARTF_USESTDHANDLES`; standard input
  the `NUL` device opened inheritable; standard output and error
  inheritable copies of the installer's own, or the output file and
  `NUL` for the identity check; the same console; the command line
  quoted as `CommandLineToArgvW` reads it. The process handle comes from
  `CreateProcess`, so nothing looks the process up afterwards. The
  installer polls `Wait(handle, 100)`, then reads `GetExitCodeProcess`,
  and closes the handle in `finally`; a failure to start is -1. The
  comment gives the reasons (Amendments A2 and A3).

**Not reproducible as a failing test:** the start race did not occur on
the Windows test host in 800 tries across both probes, so no case fails
on the old code there; CI's run `38315770402` is the failing evidence.

**The helper on the Windows test host** (the function as `install.ps1`
holds it, dot-sourced under `Set-StrictMode -Version 3.0`, with
`[Console]::InputEncoding` and `$OutputEncoding` both UTF-8 with a BOM),
in Windows PowerShell 5.1.26100 and PowerShell 7.6.6, identical:

```text
one run: code=5 output=read=0 hex= err=<nil> args=["5" "a b"]
under load: wrong exit code 0 of 300
missing program: code=-1
```

A path with a space, and an argument with a space, arrived whole. The
probe's busy loops: its own stop by ID reported none left, yet a check by
command line found 32 and stopped them (`left=0`).

**The package:** `ok` on the Windows test host (315 `--- PASS` lines), on
this Mac (`ok … 43.713s`) and on the Linux test host (`ok … 16.572s`).

**Checks.** `make gate` on this Mac (the tree also held W5), load
average 4.11 before and 44.87 after: every step `rc=0`, `apicheck`
`compatible with v1.15.1`, `arch` `0 differ`, `overall=0`. CI run
`38320675457`, on `8024c9b` (this fix with W5): 17 of 17 jobs `success`;
`main` is green again.

### W6 (2026-10-12)

The owner committed D5's fix and W5 as `8024c9b`, pushed, and said "all
committed, pushed, proceed".

**Files.** Changed: README.md, AGENTS.md, `docs/README.md`,
`docs/reports/0004-REPORT-release-signing-research.md`.

**Closed:**

* **A1, A10 (README.md):** a plain-http redirect hop is allowed only to a
  loopback host from an API on loopback; on Windows no `.bat` or `.cmd`
  program is started, in any spelling, by the version probe, the default
  runner (and so `ghattest` and the exec reconciler) or the handoff, and
  the error wraps `selfupdate.ErrBatchFile`.
* **A4, A5, A6, A7 (AGENTS.md):** what `make gate` leaves out
  (markdownlint-cli2 0.23.2, `actionlint`, the stray-`.git` run);
  `plant-copy.sh`'s arguments are Python string literals, FILE holds OLD
  once, and the copy has no `.git`; the live tests skip unless their
  switch is set (`SELFUPDATE_REQUIRE_*`, and
  `SELFUPDATE_INSTALL_REQUIRE_SHELLS`), with a pointer to the
  architecture page's CI section; `winproc` walks the process tree and
  reads liveness on Windows, and `internal/testproc` is test support.
* **A4, A8, A9 (`docs/README.md`):** the gate row says markdownlint and
  actionlint run by hand; the installer and testing rows are split; four
  rows added: rehearse without installing, keep the previous binary,
  replace a setuid or setgid binary, replace before the stop.
* **A2, A3 (the 0004 report):** a "Since (2026-10-12)" section before
  "Sources", as the 0016 report's "Decided" section is: runtime
  provenance is checked opt-in by `ghattest` since `v1.12.0`, in this
  module, and question 1 is settled by AGENTS.md and 0013-MADR. The
  report's text is otherwise unchanged.

**A correction before the record:** the first draft of A10 named
`codesign` among the refusers; `codesign` returns `ErrUnsupported` off
macOS (finding C9), so it was taken out. The first draft of A6 named only
`SELFUPDATE_REQUIRE_*`; the installers' switch is
`SELFUPDATE_INSTALL_REQUIRE_SHELLS`.

**Checks.** On the four files: `scripts/check-docs.sh --links` `134
links in 4 files, 0 broken`; `--ids` `0 findings`; markdownlint `0
issues`. Every index title still matches its record's H1 (59 of 59).
`make gate` runs before the push (Deviation D2).

### W7 (2026-10-12)

**Files.** Changed: `docs/architecture.md`.

**Closed:**

* **B1, B2, B3:** the tree and the `winproc` bullet name the library's
  packages under `selfupdate/`, not the module, and say the release tool
  starts its own processes; the batch-file refusal wraps
  `selfupdate.ErrBatchFile` through the version probe, `ExecRunner`,
  `ExecReconciler` and `DetachProcess` (0022-MADR D3); `Alive` is
  Windows-only, `AliveAccess` is named with 0022-MADR Amendment A3, and
  Unix's liveness checks use `kill(pid, 0)` (`proc_unix.go:22`,
  `hopwait_unix.go:23`).
* **B4:** taskscheduler's `Stop` refuses a disabled running task with
  `ErrTaskDisabled`, ends the task only if the program still runs, and
  enables it again after a failure with the program running (W1's C2);
  `Start` returns `ErrTaskDisabled`; the rewrite is for an
  interactive-token principal.
* **B5, B18, B19:** the hop wait (`hopwait_*.go`) and `SameExecutable`;
  launchd's `CleanupHandOffs`; systemd's `Notify` wrappers and unit-name
  helpers.
* **B6:** `testproc.Build`, `Budget` and `TestNoDirectFixtureBuild`.
* **B7, B8:** the Windows leg names the session probe step (0021's second
  plan, U3); codesign's CI tests are the ad-hoc ones, and
  `TestLiveSignIdentity` is listed under "Not in CI".
* **B9:** `other-packages` matches AGENTS.md.
* **B10, B11:** `ghattest` is its own bullet under 0017-MADR; a new
  `releasespec` bullet cites 0013-MADR and 0014-MADR.
* **B12:** 0020-MADR and 0021-MADR are linked by full filename.
* **B13:** CI's triggers are stated, and the rehearsal runs on `main`, a
  `release/**` branch or a pull request.
* **B14:** the gate runs every script test but its own, and the
  identifier check on changed and untracked files.
* **B15:** the platform split names `*_other.go` (`cleanup_other.go`,
  `journal_other.go`).
* **B16:** the publish workflow's token, and its first step, the `gh`
  capability check (0010-MADR D5); its steps are numbered 1 to 8.
* **B17:** `apicheck` compares with the newest `v1.*` tag.

**Checks.** `scripts/check-docs.sh --links` `24 links in 1 files, 0
broken`; `--ids` `0 findings`; `--arch` `16 rows … 0 differ`;
markdownlint `0 issues`. `make gate` runs before the push (Deviation D2).

### W8 (2026-10-12)

The owner committed W6 and W7 as `667f41a`, and said "committed,
proceed".

**Files.** Changed: `docs/guides/extending-selfupdate.md`.

**Closed:**

* **C1, C2 (W1's fixes):** the stop bound says Task Scheduler spends
  `StopGrace` inside `Poll.Timeout`; since `v1.15.2` `New` refuses a set
  grace not shorter than it, and a default grace is the shorter of 10 s
  and half the timeout; `Stop` ends the task only if the program still
  runs, and a `Stop` that fails with the program running enables the
  task again (since `v1.15.2`).
* **C3, C9:** the trailing-dot and trailing-space spellings are dated
  `v1.15.0` with `::$DATA`; the refusers are the version probe,
  `ExecRunner` (so `NewExecReconciler` and `ghattest`) and the handoff,
  not `codesign` or the backends' tools.
* **C4, C17:** inside the service, every backend's `Stop` refuses with
  `service.ErrInsideService`, and the update's error matches it and
  `ErrManagedInstall`; `HandOffIfInside` checks neither `--yes` nor
  whether the run would install, which `HandOffFunc` and `cli` do.
* **C5:** the `v1.15.1` liveness check, and `Start`'s filter of the
  instance `Stop` recorded.
* **C6:** each backend's live-test switch, with the administrator,
  console and integrity conditions, and why CI skips
  `TestLiveTaskUpdate`.
* **C7, C8:** a paragraph on the typed errors, `Poll`'s defaults and
  `HealthProbe`; the exec reconciler's arguments, `Receipt` and its
  verdicts, `Restore` or `RestoreFunc`, and `ExecState`.
* **C10, C11:** Task Scheduler's rewrite sits outside the `v1.10.1` list,
  dated `v1.14.0`; the dependents rule says "on the SCM".
* **C12:** pointers added to "Keep the previous binary", "Run as a
  service" (each backend's `ExampleNew`, from W5), "Task Scheduler" and
  "Replace before the stop".
* **C13, C18, C19:** `ManagedInstaller.KeptBackups` and `CleanupPending`
  on the inner installer; `ChainTransformers`; `selfupdatetest`'s
  constructors, `Calls`, `RateLimit`, `TruncateAssets` and `Requests`,
  and `FakeSource`'s `ErrNoRelease` (W2).
* **C14:** 0004-MADR linked; "0159, probe 7" named by repository and
  filename, as the guide's other citation of it is.
* **C15, C16:** the handoff result's `result` is marked abridged; the
  `main` snippet checks `err`, declares `ctx`, and the text names
  `newUpdater`.

**The snippet built (Rules):** the handoff `main`, verbatim, in a
scratch module outside the repository with a `replace` of this module
and a stub `newUpdater`: `go build` for linux, darwin and windows
(`CGO_ENABLED=0`), each `rc=0`. The audit had seen the old one fail with
`declared and not used: err`.

**Checks.** `scripts/check-docs.sh --links` `15 links in 1 files, 0
broken`; `--ids` `0 findings`; markdownlint `0 issues`. `make gate` runs
before the push (Deviation D2).

### W9 (2026-10-12)

The owner committed W8 as `26f6ea3`, and said "committed, proceed".

**Files.** Changed: `docs/guides/migrating-from-mcplib-selfupdate.md`,
`docs/guides/building-releases.md`.

**The migration guide:**

* **The current version is `v1.15.2`:** its eleven `go get` lines, its
  nine checks, §2's "current release" and §5's `go.mod` line; §2 names
  `v1.15.2` and its section. The pinned workflow commits and §3's
  `refs/tags/v1.15.1^{}` stay until the tag exists, for W10's pin commit.
* **"From v1.15.1 to v1.15.2"**, after §15's `v1.15.1` part: the Task
  Scheduler grace and failed-stop fixes (W1), `FakeSource` and
  `ErrNoRelease` (W2), `install.ps1`'s `NUL` input and Windows PowerShell
  5.1's exit codes (W3, Deviations D3 to D5), the documentation pass and
  the Examples, and what to adopt. It cites this record.
* **D1:** build and test with Go 1.27.2, with the go-core-lib rename
  noted. **D3:** §5 points at §6 to §15 and schema 4; §6's and §11's
  schema lines say the current release writes 4. **D4:** 0004-MADR is
  linked for owner decision 1 and for C3 and C4. **D5:** a "From v1.5.0
  to v1.5.1" section from 0010-MADR §2 (C4, B4, A2, A10, A8). **O2:** the
  loopback rule. **D2:** §10's installer line dates `install.ps1`'s empty
  input to `v1.15.2`.

**The building guide:**

* **D2:** the identity check and the hooks: `install.ps1` gives them `NUL`
  since `v1.15.2`.
* **D6:** the arm64 runners are announced for public repositories; a
  private repository's arm64 identity jobs may find no runner.
* **D7:** the module must require this library; two §9 rows, with
  `floor.go`'s and `stage.go`'s exact messages.
* **D8:** an extra's `path` is relative to the repository root and must
  be committed; the example extra is a committed `NOTICE`, and a
  generated SBOM goes through the extras artifact.
* **D9:** the `-tools` artifact, kept 1 day, and the re-run windows.
* **D10, D11:** the rehearsal stamp's 12 hex; setup-go's cache.
* **D12:** `.old-<guid>` beside `.bad-<guid>`, and that uninstall leaves
  both.
* **D13:** the spec's limits (`validate.go`, `installer.go`,
  `releasespec.MaxSize`) and the jobs' timeouts (30 and 10 minutes).

**The spec example checked (Rules):** §1's JSON block, verbatim,
embedded in a scratch program outside the repository: `releasespec.Parse`
accepts it, and `ExtraNames("v1.2.3")` gives `relay-v1.2.3.notice.txt`,
`install.sh` and `install.ps1`.

**Checks.** `scripts/check-docs.sh --links` `56 links in 2 files, 0
broken`; `--ids` `0 findings`; markdownlint `0 issues`. `make gate` runs
before the push (Deviation D2).

After W9's record, `make gate` on this Mac, load average 3.24 before and
56.15 after: every step `rc=0`, `apicheck` `compatible with v1.15.1`,
`links` `531 links in 69 files, 0 broken`, `overall=0`. The disclosure
guard passed on `667f41a` and `26f6ea3`.

### W10, `v1.15.2` (2026-10-12)

The owner committed W9 as `436a7f5`, pushed it, created the tag and
pushed it, and said "tag pushed ci running".

1. **CI on `main`** at `436a7f5`: run `38328935226`, `success`.
2. **The tag:** `v1.15.2` is an annotated tag object (`b4d9884`) on
   `436a7f5976a6815491c7384527fef90869574a1b`; `git ls-remote …
   'refs/tags/v1.15.2^{}'` gives that commit.
3. **The tag's CI:** run `38329947405`, 17 of 17 jobs `success`, among
   them both release rehearsals' ten identity legs (relay on linux/amd64,
   linux/arm64, darwin/arm64, windows/amd64 and windows/arm64, raw and
   archive).
4. **The pin commit** (uncommitted, for the owner): README.md's current
   release, `go get` line and publish pin; `docs/architecture.md`'s
   current release and commit; `building-releases.md`'s "pin `v1.15.2`'s",
   three workflow pins and the peeled ref; the migration guide's publish
   pin, "`v1.11.0` to `v1.15.2` change nothing in it", "the example pins
   `v1.15.2`" and the peeled ref. Kept, since they say what `v1.15.1`
   itself changed: `extending-selfupdate.md:521`, and the migration
   guide's lines 50 to 53, 1099, 1101, 1118 and 1121. Links, identifiers
   and markdownlint on the four files: clean.
5. **The proxy:** `go list -m -json …@v1.15.2` from
   `proxy.golang.org` gives `v1.15.2`, origin hash `436a7f5…`; `@latest`
   resolves to `v1.15.2`.

### Release notes for v1.15.2 (2026-10-12)

`v1.15.2` changes no exported API: `make apicheck` reports it compatible
with `v1.15.1`.

* **Task Scheduler's stop grace:** `taskscheduler.New` refuses an
  `Options.StopGrace` the program set that is not shorter than the
  effective `Options.Poll.Timeout`, when `Options.RequestStop` is set; a
  default grace is the shorter of 10 s and half the timeout. A grace as
  long as the timeout used to end `Stop` without ending the task.
* **A failed Task Scheduler stop leaves the task enabled:** a `Stop` that
  fails after it disabled the task, with the program still running,
  enables the task again before it returns.
* **`selfupdatetest.FakeSource`** wraps `selfupdate.ErrNoRelease` for a
  missing release, as `GitHubSource` does.
* **`install.ps1`:** hooks and the identity check get the `NUL` device as
  their standard input, through `CreateProcess` from a small compiled
  helper, so they cannot read the user's console, whatever either
  PowerShell's encodings are; and Windows PowerShell 5.1 no longer loses a
  hook's exit code when the hook exits at once. Callers take it by moving
  their workflow pins to `v1.15.2`.
* **Documentation:** the guides, `go doc`, the records index and AGENTS.md
  were checked against the code and 91 findings fixed; Examples were
  added for `buildinfo`, `selfupdatetest` and each service backend; Go
  comments' record citations follow AGENTS.md's new rule.

### W11: Verification, at closing (2026-10-12)

The owner committed the pin commit as `f774c5c`, pushed it, and said
"tagged, ci green, committed, proceed". CI run `38331026979`, on `f774c5c`: 17 of 17 jobs `success`.

* **V1 (findings): met.** All 91 are closed: A1 to A10 in W6, B1 to B19
  in W7, C1 to C19 in W8, D1 to D13 and O2 in W9, E1 to E25 with O1 and
  O3 to O5 in W4 and W5; C1, C2, D2 and E14 also by code (W1 to W3). E21
  is closed as D2-A decides: the citations a reader cannot resolve were
  changed, and the rest are allowed by AGENTS.md's new rule.
* **V2 (defects): met.** C1's and C2's tests failed on their plants and
  pass; the Task Scheduler live suite passed on the Windows test host
  (W1). E14's test failed on both plants and passes (W2). D2's test failed
  on `install.ps1` as it was, and on each later fix's predecessor where
  the host could reproduce it (W3, Deviations D3 and D4), and passes in
  both PowerShell editions on the Windows test host and on CI; Deviation
  D5's start race could not be reproduced on the host, so CI is its
  evidence (below).
* **V3 (citations): met.** W4's search finds none of the kinds D2-A
  changes; AGENTS.md's "Records" states the rule.
* **V4 (Examples): met.** Every Example passes `go test -run '^Example'`,
  and `buildinfo`, `selfupdatetest` and the four backends each have one
  (W5).
* **V5 (checks): met.** Links, identifiers, the architecture table and
  markdownlint were clean at each phase; `make gate` ended `overall=0`
  before each push (Deviation D2), the last at W9; `apicheck` stayed
  compatible with `v1.15.1`.
* **V6 (release): met.** `v1.15.2` is an annotated tag on `436a7f5`; its
  CI passed 17 of 17 jobs with ten identity legs; the proxy serves it and
  `@latest` resolves to it (W10).

### Closing (2026-10-12)

W0 to W11 are recorded above, and V1 to V6 are met. This plan is
`complete`, here and in the index; the MADR stays `accepted`, with D1-A
to D4-A and Amendments A1 to A3.

**What the pass changed beyond documents:** two Task Scheduler fixes, a
test double that matches the real source, and an installer whose hooks
cannot read the console, each released in `v1.15.2`. The installer fix
took three attempts (Deviations D3 to D5): each failure was found by CI's
Windows PowerShell 5.1 and not by the Windows test host, whose console
encoding and speed differed.

**Open at closing:**

* **Deviation D5's start race** in Windows PowerShell 5.1's
  `Start-Process` is inferred from PowerShell's source and CI's one
  failure; 800 tries on the Windows test host did not reproduce it. Every
  CI run since `8024c9b` has passed.
* **Why CI's Windows console input is UTF-8** is [derived] from the
  failures; the runner's setting was not read.
* **Not tested:** a hook that starts a long-lived program, and Ctrl+C
  during a hook (`Invoke-NoInput` waits for the hook alone, and polls).
* **`make gate`** still leaves markdownlint and actionlint to be run by
  hand; AGENTS.md says so.
* **From 0021, unchanged:** where `026faaa` saw `0x8004130B`, and why a
  GitHub runner runs a least-privilege task at high integrity.
* **This pass's own footprint outside the repository:** one audit
  installed `apidiff` into the Go bin directory (MADR, Context); twice a
  load probe on the Windows test host left its busy loops running, and
  each time a check found and stopped all of them (Deviation D5).
