---
status: in-progress
date: 2026-10-11
associated-madr: "0022-MADR-documentation-pass-after-v1-14-0.md"
---
# Implement the documentation pass after v1.14.0, the architecture-table check, a hardened `selfupdate.ErrBatchFile`, and the disabled-task measurement

Associated MADR: [0022-MADR-documentation-pass-after-v1-14-0.md](0022-MADR-documentation-pass-after-v1-14-0.md)

## Goal

* **Every finding below is closed:** fixed as its row says, or recorded
  as not done with the reason (MADR D1-A).
* **The architecture table's file counts are checked** by `make gate` and
  CI (D2-A).
* **A program can match the batch-file refusal** with
  `errors.Is(err, selfupdate.ErrBatchFile)` (D3-A). A static test holds
  the `%w` chain at every site on every OS (H1), and the guard refuses
  every spelling Windows runs as a batch file, by measurement (H2).
* **`Task.Start` on a task the user disabled is measured and held** by a
  live test (D4-A).
* **`v1.15.0`** carries the code and the comments.

## Scope

### In scope

| Phase | What | Commit |
| :--- | :--- | :--- |
| P0 | this pair and its two `docs/README.md` rows | records only |
| P1 | the measurements on the Windows test host: batch spellings (H2) and a disabled task's run (D4) | records only |
| P2 | `selfupdate.ErrBatchFile`, H1's tests, H2's guard and its tests | code and tests |
| P3 | `TestLiveTaskStartDisabled` (D4) | tests |
| P4 | `check-docs.sh --arch`, its test, the gate step and CI's, and findings A1 to A4 (D2) | scripts, CI, docs |
| P5 | the doc comments, findings C1 to C12 | Go comments only |
| P6 | the Markdown, every other finding | docs and records |
| P7 | `v1.15.0`: CI, the tag, the pins, §15 | per 0017-PLAN's procedure |
| P8 | closing | records only |

### Out of scope

* **New Example functions** for `selfupdate/service` or a backend. The
  audit found none, and a compile-only Example adds little the guide's
  examples do not. It is a decision of its own if wanted.
* **The rest of the architecture table:** the imports column and the
  `testdata/` notes are checked by reading, as the audit did, not by
  `--arch`.
* **H3 to H5** (MADR): refusing at construction, a new exit code or error
  kind, refusing by content.
* **A change to `Start`** if P1 shows `Run` refusing a disabled task: that
  stops P3 and goes to the owner (MADR decision 6).
* **Push and tags,** which happen only on the owner's ask in the same turn.

## Rules

* **`main`, no branch.** The owner commits and pushes on `main`. An agent
  commits only on an explicit "commit to main" in that turn, with
  `git commit --no-edit`.
* **Each phase stops at a deviation** (AGENTS.md, Plan deviations), and at
  the stop conditions MADR decisions 5 and 6 name.
* **Checks before each commit:**
  * `make pre-add-check FILES="…"` on every changed Go file;
  * `scripts/check-docs.sh --links` and `--ids`, and `markdownlint-cli2`,
    on every changed Markdown file;
  * `make gate` ending `overall=0`.

  Long output goes to a scratch file, with `$?` captured before any
  filter.
* **Seen failing first:** every new check and assertion is run on a
  `scripts/plant-copy.sh` copy with one planted break, never by dirtying
  the tree. The record quotes the failing line.
* **The Windows test host:** binaries are cross-compiled here with
  `CGO_ENABLED=0 GOOS=windows GOARCH=amd64`, copied to a new directory
  under the host's temporary directory, and run from its ssh shell, which
  is elevated (0021-PLAN Deviation D1). Live Task Scheduler runs need the
  console sign-in, measured as 0021 did. Each run's directory, tasks and
  processes are removed afterwards and the cleanup is recorded.
* **Identifiers:** nothing committed names a host, an account or a real
  absolute path. Use "the Windows test host", `<tmp>`, `<user>`.

## Findings

Confirmed at `cf0664c` against the source cited. "Fix" is the change to
make.

### `docs/architecture.md`

| ID | Line | Now | Fix |
| :--- | :--- | :--- | :--- |
| A1 | 122 | `systemd` 8 and 12 files | 9 and 17 (`git ls-files`) |
| A2 | 123 | `launchd` 6 and 11 | 7 and 13 |
| A3 | 124 | `scm` 9 test files | 12 |
| A4 | 127 | `selfupdate-release` 11 and 10 | 12 and 11 |
| A5 | 429-430 | the `make` targets omit `gate` | add `gate`, which the Makefile declares |
| A6 | 509-519 | CI's live tests list SCM for Windows, not the Task Scheduler step | add it: it runs on `windows-2025`, without `SELFUPDATE_REQUIRE_TASKSCHED`, so it skips with its reason |

### `docs/README.md`, `AGENTS.md`, `docs/reports/`

| ID | Where | Now | Fix |
| :--- | :--- | :--- | :--- |
| X1 | `docs/README.md:11-16` | six titles (0001 to 0003 MADR and PLAN) shortened | each record's H1, as the other 44 rows are |
| X2 | `docs/README.md:68` | "§6 to §11" | "§6 to §15" after P7 |
| X3 | `docs/README.md:89` | "a systemd, launchd or Windows service" | add "or a per-user Task Scheduler task" |
| X4 | `docs/README.md:95` | import rules → 0001-MADR §3, which has no depguard rule, no Charm ban and says `go.mod` has no `toolchain` line | AGENTS.md "Dependencies" and 0008-MADR |
| G1 | `AGENTS.md:10-29`, `:57-58` | the scope paragraph omits `internal/winproc`; `other-packages` covers "an `internal/` helper" | name `internal/winproc` (0021-MADR D7); `other-packages` covers an `internal/` helper with no rule of its own, `internal/cmd/selfupdate-release` today |
| G2 | `AGENTS.md:162-164` | `SELFUPDATE_REQUIRE_PYTHON` makes `TestManifestDifferential` mandatory | also `selfupdate/releasespec`'s parity test, which needs `python3` and `sh` (`parity_test.go:18-31`) |
| G3 | `AGENTS.md:180` | the `ids` step runs on "the changed files" | "the changed and untracked files" (`gate.sh:21`) |
| G4 | `AGENTS.md:156-161` | `GO_PRECHECK_SKIP_VULN` only | also `GOLANGCI_LINT=<path>` (`go-precheck.sh:50`, `Makefile:7`) |
| R1 | `docs/reports/0016-REPORT-precheck-gofmt-errors.md:3` | "Associated record: none in this repository." | a dated addendum: 0020-MADR and 0020-PLAN decide it |

### Go doc comments

| ID | Where | Now | Fix |
| :--- | :--- | :--- | :--- |
| C1 | `selfupdate/service/doc.go:6-7` | "subpackages systemd, launchd and scm" | add `taskscheduler` |
| C2 | `selfupdate/service/detach.go:10-11` | `DetachProcess`: "on Windows with no console" | "with a console of its own and no window" (0021-MADR D7) |
| C3 | `selfupdate/service/handoff.go:274-276` | `ProcessDetacher`: "such as a Task Scheduler task" | drop the example, and say a Task Scheduler task has `taskscheduler.Task` |
| C4 | `selfupdate/service/handoffhop.go:15-17` | "scm.Service.Detach sets it" | and `taskscheduler.Task.Detach` (0021-MADR D6) |
| C5 | `selfupdate/service/poll.go:14-15` | `Health.Instance`: "a launchd or SCM process ID" | add Task Scheduler |
| C6 | `selfupdate/service/runner.go:58-60`, `selfupdate/probe.go:76-80`, `selfupdate/service/detach.go:10-14`, `selfupdate/service/execreconciler.go:104` | silent on D7 and D8 | one sentence each: on Windows a `.bat` or `.cmd` program is refused (`ErrBatchFile`), and a child of a process with no console gets no window |
| C7 | `selfupdate/types.go:562-566` | `EnabledLifecycle`: "systemd is-enabled", no Task Scheduler | systemd's `UnitFileState`; add a Task Scheduler task with an enabled logon, boot or repeating trigger |
| C8 | `selfupdate/service/taskscheduler/task.go:36-37`, `selfupdate/service/scm/service.go:29-31`, each `Options.Name` | "Empty means the product", but `Inside` and so the handoff need it set (`taskscheduler/detach.go:16-17`, `scm/detach.go:17-19`) | "The handoff needs it set", as systemd's `Options.Unit` says |
| C9 | `taskscheduler/lifecycle.go:105-106` (exported), `internal/winproc/winproc_windows.go:24`, `selfupdate/github.go:222` | other repositories' records cited by number | repository and filename (AGENTS.md, Records), or this repository's 0021-MADR where it restates the fact |
| C10 | `internal/winproc/winproc.go:1` | "every process this module starts" | every process the library's packages under `selfupdate/` start; the release tool is outside (0021-MADR D7) |
| C11 | `selfupdate/doc.go` | never names `selfupdate/service` | one sentence naming it and its four backends, beside the `cli` and `ghattest` pointers |
| C12 | `selfupdate/example_test.go:72-73` | "binds its systemd, launchd or Windows service adapter" | "binds a `selfupdate/service` backend" |

### `docs/guides/extending-selfupdate.md`

| ID | Line | Now | Fix |
| :--- | :--- | :--- | :--- |
| E1 | 449-451 | "systemd `is-enabled`", no Task Scheduler | `UnitFileState` `enabled` or `enabled-runtime` (`systemd/lifecycle.go:47-59`); Task Scheduler's rule |
| E2 | 516-518 | "on Windows, reads an unquoted command line" | "on the SCM": `taskscheduler` sets the `Exec` path and parses no command line |
| E3 | 490-496 | the stop bound adds 30 s for every manager | systemd and launchd add 30 s; the SCM and Task Scheduler wait `Poll.Timeout`, Task Scheduler after up to `StopGrace` when `RequestStop` is set; Task Scheduler's "stopped" is the recorded process gone, by PID and creation time |
| E4 | 584-586 | `schtasks /change /tn <name> /enable` | `/tn <folder>\<name>`, with `Options.Folder` |
| E5 | 541-598 | `Options.Folder` only in a comment; `Restore` on rollback unstated | a bullet each (`taskscheduler/reconcile.go:93-99`) |
| E6 | 451, 519-523 | `scm.Options.TriggerStartEnabled` not named | name it where the stopped-service rule is (`scm/lifecycle.go:48-66`) |
| E7 | 617-627 | the replace-before-stop restart caveat omits Task Scheduler | add a repeating trigger: the task is disabled only inside `Stop` |
| E8 | 637-708 | the handoff only through `cli.Command` | a pointer for a program without `cli`: `service.HandOffIfInside` and `service.HandOff` |
| E9 | 529-533 | systemd's `Ready`, `Watchdog`, `WatchdogInterval` only | also `Available`, `Status`, `Stopping`, `Reloading`, `TemplateInstance`, `Escape`, `ValidUnitName` |
| E10 | 309-313 | "made on a GitHub-hosted runner", always | "unless `Policy.AllowSelfHostedRunners`" |
| E11 | 536-539 | "an error that wraps it" | `selfupdate.ErrBatchFile`, and the spellings H2 refuses |

### `docs/guides/building-releases.md`

| ID | Line | Now | Fix |
| :--- | :--- | :--- | :--- |
| B1 | 436 | `-Product NAME` … "repeat it for more" | `install.ps1` takes a list, `-Product a,b` (`install.ps1:36`, `installer_ps_test.go:875`); only `--product` repeats |
| B2 | 363-366 | "the plan step refuses an older one" than the pin | the plan step refuses a requirement older than a field's floor, `v1.10.0` for `installer` (`floor.go:28-31`); keep the advice to move both |
| B3 | 188 | "The Go version is that module's `go` line." | its `toolchain` line, or else its `go` line (`setup-go` `go-version-file`, 0018-MADR) |
| B4 | 225-257 | extras named after `{tag}` | off a tag, `{tag}` is `rehearsal-<first 12 of the commit>` (`plan.go:95`, `:113`; `stage.go:254`); the extras job computes it |
| B5 | 139-146 | the identity run requires `<tag> (release)` | it must also exit 0, and runs with only `PATH`, and `SystemRoot` on Windows (`identity.go:92`, `:120-126`) |
| B6 | 181-203 | `retention-days`, the `tag` output and the default artifact names are absent | add them (`build-selfupdate-release.yml:34-45`, `plan.go:104-107`) |
| B7 | 405 | "Download every file of the release" | `SHA256SUMS` and this platform's asset of each selected product (`install.sh:436-444`, `install.ps1:389-411`) |

### `docs/guides/migrating-from-mcplib-selfupdate.md` and 0021's notes

| ID | Line | Now | Fix |
| :--- | :--- | :--- | :--- |
| M1 | 108-114 | "`v1.11.0` to `v1.14.0` change nothing in it" | true of the file; its archive check runs the pinned commit's unpacker, which `v1.11.0` and `v1.11.1` tighten (`selfupdate/archive/unpack.go`, +254 lines); point to §11 and §11.1 |
| M2 | 658, 700-701; 782, 813-814; 887, 904-905 | patch subsections `go get …@v1.10.1` / `@v1.11.1` / `@v1.12.1` and their checks, inside sections that take the current release | mark them "only if you stop at this patch", or drop them; the reader following the steps must not downgrade |
| M3 | 385-390 | "what `v1.7.0` … `v1.10.0` add is in §7–§10" | §7 to §15 after P7 |
| M4 | 983-990 | §14's batch list omits `service.ExecRunner` | add it, and so `ghattest`'s and `codesign`'s default runner: a `gh.cmd` is refused; name `ErrBatchFile` |
| N1 | 0021-PLAN, "Release notes for v1.14.0" | the same omission | a dated annotation pointing here; the notes stay as written |

### Found correct

The links and anchors of every document. The records index: 50 rows, one
per record file, statuses as in their front matter. Every `v1.14.0` pin
(`81dae60…`). Every Example compiles and passes. The six extending-guide
snippets and the migration guide's examples, built against `cf0664c` for
linux, windows and darwin. The publish and build workflows' inputs,
outputs, step names and runner list. The release spec's fields and
limits. Both installers' options and exit codes, except B1 and B7. Every
`docs/decisions/` path cited in a Go comment. The table's imports, per
`go list` for linux, darwin and windows.

## Implementation Steps

### Phase P0: records

1. `git rev-parse HEAD` is `cf0664c`, on `main`; the tree holds only this
   pair and the index rows.
2. `docs/README.md` gains the two 0022 rows after 0021's.
3. Checks as in Rules. P0 and P1 are records only and may land as one
   commit, on the owner's word.

### Phase P1: measurements on the Windows test host

Nothing in the tree changes. The probes live in the scratchpad and on
the host.

1. **Batch spellings (H2).** A probe program, `batchprobe.exe`, built from
   the standard library and `golang.org/x/sys/windows` only, makes a new
   directory holding `run.bat` and `run.cmd`. Each script writes a
   marker file and exits; neither reads an argument. For each spelling
   below, the probe:
   * removes the marker;
   * starts the spelling with `exec.Command(path).Run()`, which is what
     `winproc` hands to `os/exec`, with a 10 s bound;
   * records the error, whether the marker appeared, `exec.LookPath`'s
     result, `windows.GetFullPathName`'s result, and today's guard
     verdict: `checkProgram`'s logic, copied verbatim.

   The spellings: `run.bat`, `RUN.BAT`, `run.bat.`, `run.bat..`,
   `run.bat ` (a trailing space), `run.bat. .`, `run.bat::$DATA`,
   `run.bat:s` (a stream that does not exist), `run` (no extension, as
   0021 D2), `run.`, the 8.3 short name of `longbatchname.bat`, and
   `run.cmd.` and `run.cmd ` for `.cmd`.
2. **Stop condition (MADR decision 5).** If a spelling ran and
   `GetFullPathName` plus dropping a stream suffix would not make its
   extension `.bat` or `.cmd`, P2's guard cannot be H2 as decided: stop.
3. **A disabled task's run (D4).** A PowerShell probe, from the ssh shell,
   with the console sign-in measured:
   * registers `selfupdate-probe-<random>`: `InteractiveToken`,
     `LeastPrivilege`, no trigger, one `Exec` action that writes a
     marker file;
   * disables it, and checks `Enabled` is false;
   * calls `IRegisteredTask::Run($null)` through the `Schedule.Service`
     COM object, records the HRESULT, waits 5 s, and records whether the
     marker appeared and the task's `Enabled` and `State`;
   * removes the marker, then calls `RunEx($null, 2, 0, "")`
     (`TASK_RUN_IGNORE_CONSTRAINTS`), and records the same;
   * deletes the task.
4. The record holds both probes' output verbatim, redacted per Rules. The
   MADR's two [unverified] facts gain their measured answers as
   annotations.
5. Host cleanup is checked: no probe directory, no `selfupdate-probe-`
   task, no probe process.

### Phase P2: `selfupdate.ErrBatchFile`, H1 and H2

1. **D3.** `selfupdate/errors.go` gains, in the `var` block:

   ```go
   // ErrBatchFile is returned, wrapped, on Windows when the program to
   // start is a .bat or .cmd file, in any spelling Windows resolves to
   // one: the version probe, service.ExecRunner and
   // service.DetachProcess, and so the handoff, refuse it, because
   // cmd.exe would parse its arguments (BatBadBut, CVE-2024-24576;
   // 0021-MADR D8).
   ErrBatchFile = winproc.ErrBatchFile
   ```

2. The Windows tests that expect the refusal also assert
   `errors.Is(err, selfupdate.ErrBatchFile)`:
   `selfupdate/probe_batch_windows_test.go` and, through the exported
   name, `selfupdate/service/batch_windows_test.go`.
3. **H1.** `selfupdate/spawn_test.go` gains `TestBatchErrorChain`. It
   walks the same files as `TestNoDirectProcessStart`. For each call of
   `winproc.Command` or `winproc.Detached` (the import's local name
   resolved, as `directCalls` does), it finds the error variable the call
   assigns. In the same block, that variable must reach either:
   * a `return` that returns it as it is; or
   * an `fmt.Errorf` call whose format string's verb for that argument
     is `%w`.

   Otherwise the test fails with `file:line`. A call whose error is
   discarded fails. `TestErrBatchFileIsWinproc` asserts
   `ErrBatchFile == winproc.ErrBatchFile` on every OS.
4. **H2.** `internal/winproc/winproc_windows.go`'s `checkProgram`:
   after today's `exec.LookPath`, `windows.GetFullPathName` of the path
   (on an error, the path as it was), then the part before an
   alternate-data-stream `:` in the final component, then the
   case-insensitive extension. The doc comment names the measurement.
   `TestBatchFileRefused` gains a row for every spelling P1 showed
   running, and keeps today's rows.
5. **Seen failing,** each on a `plant-copy.sh` copy:
   * H1: one site's `%w` changed to `%v` (`TestBatchErrorChain` names its
     line), on this Mac;
   * H1: `ErrBatchFile = errors.New("plant")` (`TestErrBatchFileIsWinproc`
     fails on this Mac; the Windows batch tests fail on the host);
   * H2: today's `checkProgram` restored (the new spelling rows fail on
     the host).
6. The tree: `go test ./...` on this Mac; the `internal/winproc`,
   `selfupdate` batch and `service` test binaries on the Windows test
   host, `PASS`.
7. `make apicheck`'s full report against `v1.14.0` lists only
   `ErrBatchFile` under "Compatible changes".
8. Checks; commit.

### Phase P3: `TestLiveTaskStartDisabled` (D4)

Runs only if P1 showed `Run` running a disabled task. Otherwise P3 stops
before any change, and the owner decides (MADR decision 6).

1. `selfupdate/service/taskscheduler/live_windows_test.go` gains
   `TestLiveTaskStartDisabled`, before `TestLiveTaskzCleanup`, with the
   file's helpers:
   * registers the live task, as the other live tests do;
   * disables it from the test, as a user would, outside the backend;
   * `New(...).Start`, with no `Stop` first, so the backend did not
     disable it;
   * asserts `Start` returns nil, a new instance's process runs within
     `Poll.Timeout`, and the task reads disabled afterwards;
   * cleans up as the file's other tests do.
2. **Seen failing:** a `plant-copy.sh` copy whose `Start` returns nil
   before `r.run()`; the test fails on "no new instance" on the host.
3. The four existing live tests and the new one pass `-count=3` on the
   Windows test host, with `SELFUPDATE_REQUIRE_TASKSCHED=1`.
4. Checks; commit.

### Phase P4: the table check (D2)

1. `scripts/check-docs.sh` gains `--arch FILE`:
   * the table under `## Go code`, rows whose first cell is a backquoted
     directory;
   * the first integer in the "Non-test files" and "Test files" cells;
   * `git ls-files -- '<dir>/*.go'`, in that directory alone, split on
     `_test.go`;
   * one line per row that differs, `<file>:<line>: <dir>: table N/M,
     tree N/M`, and exit 1; a missing table, or a row whose directory has
     no tracked file, exits 2.

   The usage line and the header comment name the mode.
2. **Seen failing on the real table first:** run on `cf0664c`'s
   `docs/architecture.md`; it reports the rows that differ, A1 to A4 and
   any row P2 or P3 changed, and exits 1.
3. `docs/architecture.md`: A1 to A4 fixed, and any row P2 or P3 changed.
   The mode exits 0.
4. `scripts/check-docs_test.sh` gains cases: a clean table; a planted
   wrong count, named in the output; a missing table, exit 2.
   **Seen failing:** a copy of `check-docs.sh` whose comparison never
   reports fails the planted-count case.
5. `scripts/gate.sh` gains `step arch doc_arch` after `links`; its header
   names the step. `gate_test.sh` is extended if it lists the steps.
6. `.github/workflows/ci.yml`, "Verify the gate and document tools":
   `./scripts/check-docs.sh --arch docs/architecture.md` after the
   `--links` line. `scripts/check-workflows.sh` passes on it.
7. AGENTS.md's `make gate` list and `docs/architecture.md`'s tree, gate
   and CI lines name the step.
8. Checks; commit.

### Phase P5: doc comments

1. C1 to C12, comments only. C6 names `ErrBatchFile`.
2. `git diff` shows changes inside comments only. `go doc` of each
   changed identifier reads as intended.
3. `make pre-add-check FILES="<the changed Go files>"`; `make gate`;
   commit.

There is no new test, so no fail-first experiment applies.

### Phase P6: Markdown

1. A5, A6, X1 to X4, G1 to G4, R1, E1 to E11, B1 to B7, M1 to M4 and N1,
   as their rows say. Each changed line is named in the record.
2. The extending guide's Task Scheduler section states P1's measured
   behaviour for a disabled task, and cites `TestLiveTaskStartDisabled`.
   E11 and M4 name `ErrBatchFile` and the spellings H2 refuses.
3. Wording follows each file's present form. Records get annotations,
   never rewrites.
4. Checks; commit.

### Phase P7: `v1.15.0`

0017-PLAN's "Release procedure" and 0021-PLAN T8's form, with
`vX = v1.15.0` and the release commit P6's:

1. CI on `main` at P6's commit is green; the commits are pushed first, on
   the owner's ask.
2. The tag, on the owner's ask in that turn:
   `scripts/check-release-tag.sh v1.15.0`, an annotated tag, the
   disclosure guard, the push.
3. The tag's CI passes, and its identity legs print `v1.15.0 (release)
   <12-hex>`.
4. The pin commit: `git grep -n 'v1\.14\.0\|81dae60' -- README.md
   docs/architecture.md docs/guides`. Each changed line is named; lines
   that state what `v1.14.0` itself changed are kept. The migration guide
   gains `## 15. From v1.14 to v1.15`, in §14's form: `ErrBatchFile`, H2's
   refused spellings, and the disabled-task behaviour. X2 and M3 move to
   §15.
5. The proxy serves `v1.15.0`, and `@latest` resolves to it.
6. Release notes in this PLAN.

### Phase P8: closing

1. `### Verification, at closing`, V1 to V7 with their evidence.
2. `### Closing`, `status: complete`, the index row. Commit, then push on
   the owner's ask.

## Verification

* **V1 (findings).** Each of the 50 findings is closed in the execution
  record with the line that changed, or recorded as not done with the
  reason.
* **V2 (table).** `--arch` reported the wrong rows on `cf0664c`'s table
  and exits 0 on the tree. Its test failed on the plant. `make gate` and
  CI run it.
* **V3 (sentinel and chain).** The batch tests failed on a distinct
  `ErrBatchFile` and pass on the Windows test host and in CI's Windows
  job. `TestBatchErrorChain` failed on a `%v` plant, naming the line, and
  passes on every OS. `apicheck` lists only `ErrBatchFile`.
* **V4 (spellings).** P1's batch table is recorded. Every spelling that
  ran is refused, and `TestBatchFileRefused`'s new rows failed on
  today's guard.
* **V5 (disabled task).** P1's task probe is recorded, and
  `TestLiveTaskStartDisabled` passes `-count=3` on the Windows test host
  after failing on its plant; or P3's stop and the owner's decision are
  recorded.
* **V6 (no unplanned change).** P5's diff is comments only; P6 changes no
  Go file; `apicheck` shows one addition.
* **V7 (release).** `v1.15.0` is tagged, its CI is green, the pins name
  it, and the proxy serves it.

## Rollout and Rollback

* **Rollout:** each phase is one commit on `main`, pushed on the owner's
  ask. Readers see the Markdown on `main` at once; `go doc` and
  `pkg.go.dev` show the comments from `v1.15.0`. H2's wider refusal
  reaches programs with `v1.15.0`.
* **Rollback, before the tag:** `git revert` of any phase. P4's revert
  removes the gate step with the check.
* **Rollback, after the tag:** a release cannot be withdrawn.
  `ErrBatchFile` is additive, so removing it would be a breaking change;
  a defect is fixed forward in `v1.15.1`.
* **A deviation in any phase** stops it, per AGENTS.md.

## Execution record

### Approval (2026-10-11)

The owner, after the proposed pair: "D2, follow recommendation, D3,
follow recommendation, but find 2 ways to better harden the solution.
once you have the solution proposed for D3, write the madr and include the
task scheduler measurement and test. proceed." The pair was amended to
add H1, H2 and D4, and P1 to P3 were added. The MADR is `accepted` with
D1-A, D2-A, D3-A with H1 and H2, and D4-A.

### P0 (2026-10-11)

* The pair and its two index rows are written. The index titles are each
  record's H1 (`same=46 differ=6`: the six are X1's, untouched until P6).
* Checks on the three files: `scripts/check-docs.sh --links` `94 links in
  3 files, 0 broken`; `--ids` `3 files, 16 deny-list rules, 0 findings`;
  markdownlint `0 issues` on `docs/README.md` (its configuration excludes
  MADR and PLAN files).

### P1 (2026-10-11)

**Preconditions.** The probes were built on this Mac with
`CGO_ENABLED=0 GOOS=windows GOARCH=amd64` (Go 1.27.2), `go vet` clean,
copied to a new `selfupdate-p1-<random>` directory under the Windows test
host's temporary directory, and run from its elevated ssh shell. The
console sign-in was measured by the task probe.

**The batch probe** (`batchprobe.exe`, standard library only). Three
runs; only the third is evidence:

| Run | Outcome | Why |
| :--- | :--- | :--- |
| 1 | superseded | no argument was passed, so it showed only whether the script ran: `run.bat.` exited 1 with no marker, which did not say whether `cmd.exe` had started |
| 2 | superseded | an injected argument was added, but the payload quoted its redirect path, which `cmd.exe` then misparsed: even `run.bat` showed `cmd_parsed_arg=false` |
| 3 | complete | the payload `"&echo inj>` + `<dir>\injected.txt` + ` & rem `, with the probe refusing a directory that holds a space or a `cmd.exe` metacharacter. `run.bat` shows the injection, so the payload works |

Run 3, verbatim (`<dir>` is the probe's directory, `<home>` the account's
profile; `%q` doubles each backslash):

```text
short_name_base=LONGBA~1.BAT
case="run.bat" ran=true cmd_parsed_arg=true guard_today=true lookpath="run.bat" fullpath="run.bat" err="<nil>"
case="RUN.BAT" ran=true cmd_parsed_arg=true guard_today=true lookpath="RUN.BAT" fullpath="RUN.BAT" err="<nil>"
case="run.bat." ran=false cmd_parsed_arg=true guard_today=false lookpath="run.bat." fullpath="run.bat" err="<nil>"
case="run.bat.." ran=false cmd_parsed_arg=true guard_today=false lookpath="run.bat.." fullpath="run.bat" err="<nil>"
case="run.bat " ran=true cmd_parsed_arg=false guard_today=false lookpath="run.bat " fullpath="run.bat" err="<nil>"
case="run.bat. ." ran=false cmd_parsed_arg=false guard_today=false lookpath="run.bat. ." fullpath="run.bat" err="exit status 1"
case="run.bat::$DATA" ran=false cmd_parsed_arg=false guard_today=false lookpath="err:executable file not found in %PATH%" fullpath="run.bat::$DATA" err="exec: \"<dir>\\\\run.bat::$DATA\": executable file not found in %PATH%"
case="run.bat:s" ran=false cmd_parsed_arg=false guard_today=false lookpath="err:executable file not found in %PATH%" fullpath="run.bat:s" err="exec: \"<dir>\\\\run.bat:s\": executable file not found in %PATH%"
case="run" ran=true cmd_parsed_arg=true guard_today=true lookpath="run.bat" fullpath="run" err="<nil>"
case="run." ran=false cmd_parsed_arg=false guard_today=false lookpath="err:file does not exist" fullpath="run" err="exec: \"<dir>\\\\run.\": file does not exist"
case="run.cmd." ran=false cmd_parsed_arg=true guard_today=false lookpath="run.cmd." fullpath="run.cmd" err="<nil>"
case="run.cmd " ran=true cmd_parsed_arg=false guard_today=false lookpath="run.cmd " fullpath="run.cmd" err="<nil>"
case="<8.3 of longbatchname.bat>" ran=true cmd_parsed_arg=true guard_today=true lookpath="<home>\\AppData\\Local\\Temp\\SE32A5~1\\LONGBA~1.BAT" fullpath="<home>\\AppData\\Local\\Temp\\SE32A5~1\\LONGBA~1.BAT" err="<nil>"
```

**Reading it:**

* **`v1.14.0`'s guard is bypassed by five spellings.** `run.bat.`,
  `run.bat..` and `run.cmd.` start `cmd.exe`, which parses the injected
  argument, although the script itself does not run. `run.bat ` and
  `run.cmd `, with a trailing space, run the script (the injection does
  not fire there, but the arguments still reach `cmd.exe`'s parser). The
  guard says `false` for all five.
* `run.bat. .` exits 1 and shows neither; it is refused by H2 all the
  same, since `GetFullPathName` gives `run.bat`.
* `GetFullPathName` gives `run.bat` or `run.cmd` for every spelling above,
  so MADR decision 5's stop condition does not apply.
* The stream spellings (`::$DATA`, `:s`) and `run.` never start:
  `os/exec` fails them first (`executable file not found`, `file does not
  exist`). H2's stream step still refuses them with `ErrBatchFile`, so
  the error says why.
* Today's guard is right for `run.bat`, `RUN.BAT`, the extensionless `run`
  and the 8.3 short name.

**The disabled-task probe** (`taskprobe-disabled.ps1`). Three runs; only
the third is evidence:

| Run | Outcome | Why |
| :--- | :--- | :--- |
| 1 | registration failed | `New-ScheduledTaskPrincipal -UserId "$env:USERDOMAIN\$env:USERNAME"`: `No mapping between account names and security IDs was done` (`0x80070534`) from the ssh shell. The task was never created; the cleanup line confirms it |
| 2 | stopped after the first disabled `Run` | the principal now uses the token's SID. `Run` on the disabled task threw `-2147216602`, and the probe's HRESULT formatter failed on the negative value before `RunEx` |
| 3 | complete | the formatter fixed (`'0x{0:X8}' -f [int]$h`) |

Run 3, verbatim:

```text
console_user_present=True
explorer_sessions=1
logonui=False
baseline.run=ok
baseline.ran=True enabled=True state=3
disabled.enabled_before=False
disabled.run=0x80041326
disabled.run.ran=False enabled=False state=1
disabled.runex_ignore=0x80041326
disabled.runex_ignore.ran=False enabled=False state=1
cleanup.task=removed
cleanup.dir_exists=False
```

**Reading it:** the enabled task runs its action (`baseline.ran=True`).
Disabled, both `Run` and `RunEx` with `TASK_RUN_IGNORE_CONSTRAINTS`
return `0x80041326` (`SCHED_E_TASK_DISABLED`); the action does not run;
the task stays disabled (`state=1`, `TASK_STATE_DISABLED`).

**Host cleanup.** Each probe removed its own task and directory. Then the
run directory was removed, and a final check found 0 `selfupdate-p1-`,
`selfupdate-probe-` or `selfupdate-batchprobe-` directories, 0
`selfupdate-` tasks and 0 probe processes.

**Records.** The MADR's two [unverified] facts carry the measured
answers. 0021-MADR's D8 carries an annotation: its guard compared the
extension as written, and the five spellings above pass it in `v1.14.0`.

### Deviation D1 (2026-10-11): `Start` cannot run a disabled task

* **Found** in P1's task probe: `IRegisteredTask::Run` returns
  `0x80041326` for a disabled task, and so does `RunEx` with
  `TASK_RUN_IGNORE_CONSTRAINTS`. The extending guide's "A task you had
  disabled is run, and stays disabled" (line 583) is wrong. The behaviour
  is `v1.14.0`'s, not new code's.
* **What it does today.** `Enabled` reports false for a disabled task, so
  a stopped one is never started. The case is a task the user disabled
  while its program runs:
  * `Stop` does not disable it again, so it records nothing in
    `disabled`, and ends it;
  * the binary is replaced;
  * `Start` calls `Run`, which fails with `0x80041326`, mapped to a
    generic error;
  * the managed installer recovers (`managed.go:187-188`): it restores the
    old binary and calls `Start` again, which fails the same way.

  The update fails, and the program that was running is left stopped.
* **MADR decision 6** stops P3 here. The resolutions change `Start` or
  `Stop`, so they are the owner's:
  * **Recommended: `Stop` refuses first.** `Stop` checks `enabled()`
    before it changes anything. A disabled task with a running instance
    is refused with an error that names the state and the remedy: enable
    the task, or stop the program, then update. Nothing is stopped or
    replaced, and the program keeps running. `Start` also maps
    `0x80041326` to an error that names a disabled task, for a task
    someone disables between `Stop` and `Start`. P3's live test then
    asserts the refusal and that the program still runs. The guide and
    §15 say so. Cost: one check, one mapping, two fake cases, a live
    test.
  * **`Start` enables, runs, disables again** for a task the user
    disabled. The update succeeds, and the user's setting is put back.
    Cost: three COM calls, each with its own failure. Between the enable
    and the disable, the task's triggers are live. If the re-disable
    fails, the user's setting is lost, and `Start` can only report it as
    an error, which starts a rollback of an update that worked.
  * **A clearer error at `Start`,** nothing more. Cost: one mapping. The
    outcome stays an update that fails after the replace and leaves the
    program stopped.
* **Doing nothing:** `v1.15.0` ships a guide that promises a behaviour
  Windows refuses, and an update of such a program leaves it stopped.
* **Files added to the phase if the recommendation is chosen:**
  `selfupdate/service/taskscheduler/lifecycle.go`, `api.go` (the
  HRESULT), `lifecycle_test.go`, `fake_test.go`, `live_windows_test.go`.

### Owner question Q1 (2026-10-11): when `v1.15.0` is tagged

The P1 batch result is a bypass of a released security guard. Exploiting
it needs a caller to pass a path ending in a dot or a space: every
library call site passes a fixed tool path, the staged file or the
program's own target, so the exposure is low, but the guard's claim is
false in `v1.14.0`.

* **Recommended:** tag `v1.15.0` once P2 to P5 land (the code, the live
  test, the table check and the comments), and land P6's Markdown with
  the pin commit after the tag. The fix reaches programs without waiting
  for the guides.
* **As planned:** tag after P6. The release waits for the whole
  Markdown pass.
