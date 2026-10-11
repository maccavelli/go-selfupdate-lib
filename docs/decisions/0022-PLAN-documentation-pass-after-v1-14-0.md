---
status: complete
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

*(Superseded 2026-10-11: P1 showed `Run` refusing a disabled task. The
steps below are struck; P3 as executed is in the execution record,
"Owner decisions on D1, Q1 and the source guard", and MADR Amendment
A1.)*

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

### P2 (2026-10-11)

The owner committed P0 and P1 as `dfa481b` and said "proceed". Deviation
D1 and Q1 are still open; P2 depends on neither.

**Files.** Changed: `selfupdate/errors.go`, `selfupdate/spawn_test.go`,
`selfupdate/probe_batch_windows_test.go`,
`selfupdate/service/batch_windows_test.go`,
`internal/winproc/winproc_windows.go`,
`internal/winproc/winproc_windows_test.go`. No file is added, so the
architecture table's counts do not change.

**As built, against the steps:**

* **D3.** `selfupdate.ErrBatchFile = winproc.ErrBatchFile`, with the doc
  comment of step 1, which also names the spellings H2 refuses.
  `make apicheck`'s full report against `v1.14.0`, without
  `-incompatible`: one line under "Compatible changes",
  `./selfupdate.ErrBatchFile: added`; the incompatible report is empty.
* **The Windows tests** assert the exported name too. The service test
  checks it through `ExecRunner().Run` and `DetachProcess`, the two
  exported entry points, besides the unexported `runExec`.
* **H1 follows the chain further than step 3 says.** Step 3 asked for the
  error to reach a `return` in the function that calls `winproc`. The
  test also follows it out: a plain function that returns the error
  becomes a source in turn, and its callers are judged the same way,
  across packages. So the walk covers `startWith` → `startDetached` →
  `DetachProcess` → `scm` and `taskscheduler`'s `Detach`, and `runExec`.
  A method, or a call inside a function literal, ends the walk, since its
  callers cannot be named statically. `chainWant` lists the four
  functions the walk must reach, so a walk that follows nothing fails.
  The test is split into `chainLoad`, `chainWalk.pass` and
  `chainWalk.function` (`gocognit` refused one function at 54).
* **The identity test** compares `any(ErrBatchFile) != any(winproc.ErrBatchFile)`.
  A two-way `errors.Is` was refused by staticcheck (SA1032, "arguments
  have the wrong order"), and `errorlint` refuses `!=` on two errors. It
  also checks that `errors.Is` finds `ErrBatchFile` in a `%w`-wrapped
  winproc error.
* **H2.** `checkProgram` resolves with `exec.LookPath` as before, then
  takes `runName(path)`: `windows.GetFullPathName` (the path as written
  when it fails), its final component, cut at the first `:`. The error
  names that normalized name. `fullPathName` grows its buffer to the
  length the call asks for.
* **`TestBatchFileRefused`** gains the rows `run.bat.`, `run.bat..`,
  `run.bat `, `run.bat. .`, `run.cmd.`, `run.cmd `, `RUN.BAT.`,
  `run.bat::$DATA` and `run.bat:s` (refused), and `x.exe.`, `x.exe `,
  `x.bat.exe` and `x.batch` (not refused). Its paths are joined by hand,
  so each spelling reaches the guard as written.

**Fail first, on this Mac,** each a `scripts/plant-copy.sh` copy, checked
to have landed:

| Plant | Failing line |
| :--- | :--- |
| `runExec`'s wrap `%w` → `%v` | `spawn_test.go:199: service/runner.go:72:14: fmt.Errorf formats err without %w, so errors.Is cannot reach it` |
| `DetachProcess`'s wrap `%w` → `%v`, the second link | `spawn_test.go:199: service/detach.go:26:14: fmt.Errorf formats err without %w, …` |
| `taskscheduler.Task.Detach` returns `errors.New(err.Error())`, another package | `spawn_test.go:199: service/taskscheduler/detach.go:70:14: err is returned through a call the walk does not follow` |
| `startWith` discards `winproc.Detached`'s error | `spawn_test.go:199: service/detach_windows.go:28:12: the call's error is discarded` |
| `ErrBatchFile = fmt.Errorf("plant: %w", winproc.ErrBatchFile)` | `spawn_test.go:141: selfupdate.ErrBatchFile "plant: …" is not winproc.ErrBatchFile "…"` |
| the walk's `Command` source renamed, so it follows less | `spawn_test.go:203: the walk did not reach …/selfupdate/service.runExec: it follows less than it should` |

The sentinel plant wraps winproc's value rather than making a new one
(step 5 said `errors.New("plant")`): `errors.New` would leave the
`winproc` import unused, and the copy would not compile. A wrapper is
the harder case, since `errors.Is` still finds winproc's value through
it.

**The Windows test host,** cross-compiled test binaries, run from the
ssh shell:

* `internal/winproc`: `TestBatchFileRefused`, `TestNoConsoleWindowThreeDeep`
  (both subtests), `TestDescends`, `TestCommandPassesProgramAndArgs`,
  `TestDetachedPassesProgramAndArgs`: `PASS`.
* `selfupdate`, `-test.run 'Batch|ErrBatchFile|Prob|NoDirectProcessStart'`:
  `TestVersionProberRefusesBatchFile` and the 14 prober tests pass, as
  do `TestNoDirectProcessStart` and `TestErrBatchFileIsWinproc`.
  `TestBatchErrorChain` fails there: the binary runs away from its
  source tree and finds no file to read, as the tests that read
  `testdata` do (0021-PLAN T2). The test now says so
  (`no source file under selfupdate/: … run it from its package
  directory`). CI's Windows job runs it from source.
* `selfupdate/service`, `-test.run 'Batch|Detach'`:
  `TestBatchFileRefusedAtStartSites`, `TestDetachProcessLeavesJob` (both
  subtests), `TestDetachedRunNeverHandsOffAgain`,
  `TestProcessDetacherNeedsInside`: `PASS`.
* **Plant, today's guard** (`name := filepath.Base(path)` in place of
  `runName`): `TestBatchFileRefused` fails on each of the nine new
  refused rows, for `Command` and `Detached`, for example
  `winproc_windows_test.go:173: Command(run.bat.): refused false (<nil>), want true`.
  The four rows that must not be refused pass.
* **Plant, the wrapped sentinel:**
  `probe_batch_windows_test.go:32: Probe: … want the exported selfupdate.ErrBatchFile (0022-MADR D3)`;
  `batch_windows_test.go:33: ExecRunner: … want selfupdate.ErrBatchFile`;
  `batch_windows_test.go:36: DetachProcess: … want selfupdate.ErrBatchFile`.

**Found, and not changed:** `TestNoDirectProcessStart` passes when it
finds no file, as on the host above. That is how it was written in 0021,
and outside P2's steps. The same "no source file" guard would make it fail
there instead; it is listed for the owner rather than added here.

**Host cleanup.** The run directory was removed; 0 `selfupdate-p2-`
directories and 0 `.test.exe` processes remain.

**Checks.** `make pre-add-check FILES="<the 6 Go files>"`: `go-precheck: 6
file(s) clean (gofmt, golangci-lint, go vet, go test, govulncheck)`. Its
first run failed on the two lint findings above, which the as-built notes
answer. `make gate`: every step `rc=0`, `apicheck` `compatible with
v1.14.0`, `overall=0`.

### Owner decisions on D1, Q1 and the source guard (2026-10-11)

The owner committed P2 as `c787cf2`, and said: "open questions: 1. follow
recommendations. 2. as recommended. i committed, proceed, and add the
guard."

* **Deviation D1:** `Stop` refuses a disabled task whose program runs,
  with an exported `taskscheduler.ErrTaskDisabled`, and `Start` maps
  `0x80041326` to it. MADR Amendment A1 records it. P3 is rewritten
  below; its original step 1 (`TestLiveTaskStartDisabled`, `Start`
  succeeding on a disabled task) is struck, since Windows refuses it.
* **Q1:** `v1.15.0` is tagged once P2 to P5 land. P6's Markdown and the
  pins follow the tag. The phase order becomes P3, P4, P5, P7 (the tag),
  then P6 with P7's pin commit, then P8.
* **The guard:** `TestNoDirectProcessStart` fails, as
  `TestBatchErrorChain` does, when it finds no source file, instead of
  passing. Added to P3's scope, with its own fail-first run.

**P3, as now planned:**

1. `selfupdate/service/taskscheduler`:
   * `api.go`: `hrTaskDisabled = hresult(0x80041326)`, measured in P1;
   * `task.go`: `ErrTaskDisabled`, and `mapError` wraps it for
     `0x80041326`;
   * `lifecycle.go`: `Stop` reads `enabled()` after it lists the
     instances and before it records or disables anything; a disabled
     task with an instance returns `ErrTaskDisabled` with the remedy.
     `disable` takes the state already read. The doc comments of `Stop`
     and `Start` say so.
2. The fake's `run` returns `0x80041326` for a disabled task, as P1
   measured. `TestStopWithoutRequest`, which asserted a disabled running
   task is ended and run again, now uses an enabled task.
   `TestStopRefusesDisabled` (refused, no call made, the process alive),
   `TestStopDisabledIdle` (nil, no call) and `TestStartDisabled` (`Start`
   on a disabled task gives `ErrTaskDisabled`) are added.
3. `live_windows_test.go` gains `TestLiveTaskStopRefusesDisabled`: the
   task's program runs; the test disables the task with
   `schtasks /change /disable`, as a user would; `Stop` returns
   `ErrTaskDisabled`, the program still runs, and the task is still
   disabled; the test ends the program with `schtasks /end`; `Start`
   returns `ErrTaskDisabled`, and no new start is recorded.
4. `selfupdate/spawn_test.go`: `TestNoDirectProcessStart` fails when it
   reads no file.
5. **Seen failing,** on `plant-copy.sh` copies: `Stop` without the
   refusal (the fake test on this Mac, the live test on the Windows test
   host); `mapError` without the `0x80041326` case (`TestStartDisabled`,
   and the live test's `Start`); the guard run from an empty directory.
6. The live tests pass `-count=3` on the Windows test host, the console
   sign-in measured.
7. `make apicheck`'s full report against `v1.14.0` lists `ErrBatchFile`
   and `ErrTaskDisabled`, as added, and nothing else. Checks; commit.

### P3 (2026-10-11)

**Files.** Changed, in `selfupdate/service/taskscheduler/`: `api.go`,
`task.go`, `lifecycle.go`, `fake_test.go`, `lifecycle_test.go`,
`live_windows_test.go`; and `selfupdate/spawn_test.go`. No file is added.

**As built:**

* `hrTaskDisabled = hresult(0x80041326)`. `ErrTaskDisabled` is exported
  from `task.go`; `mapError` wraps it for that HRESULT, beside
  `ErrNotInstalled` and `ErrPermission`.
* `Stop` lists the instances, then reads `enabled()` once. A disabled task
  with an instance returns `ErrTaskDisabled: \<task> runs while disabled:
  enable the task, or stop its program, then update`, before anything is
  recorded, disabled or ended. `disable` takes the state `Stop` read, so
  the task is queried once. The doc comments of `ErrTaskDisabled`, `Stop`
  (its step 1) and `Start` say so.
* The fake's `run` returns `0x80041326` for a disabled task.
  `TestStopWithoutRequest` now uses an enabled task: `["enable false"
  "stop"]`, then `["enable false" "stop" "enable true" "run"]`.
  `TestStopRefusesDisabled`, `TestStopDisabledIdle` and `TestStartDisabled`
  are added.
* `TestLiveTaskStopRefusesDisabled` disables and ends the task with
  `schtasks`, and checks the program by the process snapshot
  (`running(pid, 0)`), not `pidRunning`, whose `OpenProcess` can succeed
  on a process that has exited while a handle to it stays open.
* `TestNoDirectProcessStart` counts the files it reads and fails on none.

**Fail first:**

| Plant | Where | Failing line |
| :--- | :--- | :--- |
| `Stop` without the refusal (`if false && !on && …`) | this Mac | `lifecycle_test.go:246: Stop: <nil>, want ErrTaskDisabled with the remedy` |
| the same | the Windows test host | `live_windows_test.go:702: Stop: <nil>, want ErrTaskDisabled` |
| `mapError` without the `0x80041326` case | this Mac | `lifecycle_test.go:278: Start: selfupdate: taskscheduler: run \demo: HRESULT 0x80041326, want ErrTaskDisabled` |
| the same | the Windows test host | `live_windows_test.go:726: Start: selfupdate: taskscheduler: run \selfupdate-live-<random>: HRESULT 0x80041326, want ErrTaskDisabled` |
| the source guard, run from an empty directory | this Mac | `spawn_test.go:64: no source file under selfupdate/: the test reads the source, so run it from its package directory` |
| the guard planted away (`read < 0`), same directory | this Mac | `--- PASS: TestNoDirectProcessStart (0.00s)`: the vacuous pass the guard removes |

**The Windows test host.** Cross-compiled on this Mac, run from the ssh
shell with `SELFUPDATE_REQUIRE_TASKSCHED=1 ./tree-ts.test.exe -test.run
'^TestCOMLayout$|^TestLiveTask' -test.count=3 -test.v`, exit 0. The
console check, run from a script a few minutes after (the inline run
printed nothing, a quoting fault): `console_user_present=True`,
`explorer_sessions=1`, `logonui=False`. The first pass, verbatim:

```text
    com_windows_test.go:30: CoInitializeEx HRESULT 0x00000000
    com_windows_test.go:35: never-run DATE 1999-11-30 00:00:00 -0600 CST LastTaskResult 0x00041303
    com_windows_test.go:38: never-run GetInstances HRESULT 0x00000000
    com_windows_test.go:49: idle Stop HRESULT 0x00000001
--- PASS: TestCOMLayout (0.34s)
    live_windows_test.go:521: the update's integrity RID 0x2000
--- PASS: TestLiveTaskUpdate (7.53s)
    live_windows_test.go:557: instance pids before Stop: [37048]
    live_windows_test.go:561: start pid 37048 running after Stop: false; pids []
--- PASS: TestLiveTaskStopHoldsDisabled (72.31s)
--- PASS: TestLiveTaskRollback (5.41s)
--- PASS: TestLiveTaskRequestStop (1.12s)
    live_windows_test.go:728: Start on the disabled task: selfupdate: taskscheduler: the task is disabled, and Task Scheduler does not run a disabled task: selfupdate: taskscheduler: run \selfupdate-live-a8d2f7e8013694ae: HRESULT 0x80041326
--- PASS: TestLiveTaskStopRefusesDisabled (4.06s)
--- PASS: TestLiveTaskzCleanup (0.61s)
```

The later passes match, each test passing, with their own PIDs, task
names and times: `TestLiveTaskStopRefusesDisabled` 4.04s and 4.07s.

**Host cleanup.** The run directory was removed; 0 `selfupdate-p3-`
directories, 0 `selfupdate-` tasks, 0 `.test.exe` or `demo.exe`
processes.

**Checks.** `make apicheck`'s full report against `v1.14.0`:
`./selfupdate.ErrBatchFile: added` and
`./selfupdate/service/taskscheduler.ErrTaskDisabled: added`, nothing
incompatible. `make pre-add-check FILES="<the 7 Go files>"`: `go-precheck:
7 file(s) clean`. `make gate`: every step `rc=0`, `overall=0`.

### P4 (2026-10-11)

The owner committed P3 as `687fce6`, and said "proceed".

**Files.** Changed: `scripts/check-docs.sh`, `scripts/check-docs_test.sh`,
`scripts/gate.sh`, `scripts/gate_test.sh`, `.github/workflows/ci.yml`,
`docs/architecture.md`, `AGENTS.md`.

**As built:**

* `check-docs.sh --arch FILE`, in the script's Python:
  * the table is the first one under `## Go code`, up to the next
    heading. Its header names the "Non-test files" and "Test files"
    columns, and a row is one whose first cell is a backquoted directory;
  * the counts are `git ls-files -z -- '*.go'`, from the repository's
    root, split by directory and on `_test.go`;
  * a row that differs prints `<file>:<line>: <dir>: table N/M, tree N/M`
    and the mode exits 1. A missing heading or table, a row with no
    count, or a directory with no tracked `.go` file exits 2. `--arch`
    takes one file.
* `gate.sh`: `step arch doc_arch` after `links`, which runs the mode on
  `docs/architecture.md`; its header says so.
* `ci.yml`, "Verify the gate and document tools": the mode after the
  `--links` line.
* `AGENTS.md`'s `make gate` paragraph names the step, and says that a
  change adding or removing a `.go` file updates the table in the same
  commit. `docs/architecture.md` names it in the tree's `check-docs.sh`
  line, the `make gate` bullet and the CI bullet. G3 in the same AGENTS.md
  paragraph is P6's.

**Seen failing on the real table first (step 2),** the mode on
`cf0664c`'s rows, unchanged until step 3:

```text
docs/architecture.md:122: selfupdate/service/systemd: table 8/12, tree 9/17
docs/architecture.md:123: selfupdate/service/launchd: table 6/11, tree 7/13
docs/architecture.md:124: selfupdate/service/scm: table 7/9, tree 7/12
docs/architecture.md:127: internal/cmd/selfupdate-release: table 11/10, tree 12/11
check-docs: 15 rows in docs/architecture.md, 4 differ
```

Exactly A1 to A4, exit 1. P2 and P3 added no file, so no other row
moved. After step 3: `check-docs: 15 rows in docs/architecture.md, 0
differ`, exit 0.

**Tests.** `check-docs_test.sh` gains five cases, in a throwaway
repository: a matching table passes, with an untracked `.go` file not
counted; a wrong count fails with its line and both counts; no table is
exit 2; a row for a directory with no tracked file is exit 2; two files
are a usage error. `16 passed, 0 failed`. `gate_test.sh` lists the
fifteen steps, and its link case now also requires `arch rc=1` and the
`check-docs --arch docs/architecture.md` call: `10 passed, 0 failed`.

**Seen failing,** `plant-copy.sh` copies run through each test's
`SCRIPT`:

* `check-docs.sh` with `if False and got != want:`:
  `FAIL arch drift: rc=0 out=[check-docs: 2 rows in docs/drift.md, 0 differ]`,
  `15 passed, 1 failed`;
* `gate.sh` without `step arch doc_arch`: `FAIL steps not reported: arch`,
  and the link case fails; `8 passed, 2 failed`.

**Checks.**

* `shellcheck` on the four scripts: clean.
* `actionlint` v1.7.12 on `ci.yml`: clean.
* `check-workflows.sh` with `expressions`, `permissions` and `pins` on
  `ci.yml`, and with every rule on the reusable workflows: each exit 0.
  The hash-pinned PyYAML in `requirements-workflow-check.txt` lists only
  CI's Linux wheels, so this Mac's venv refused the macOS wheel; the runs
  used the host's PyYAML, the same 6.0.3.
* `check-docs.sh --links` on `AGENTS.md` and `docs/architecture.md`, 0
  broken; markdownlint, 0 issues.
* `make gate`: every step `rc=0`, among them `arch rc=0 check-docs: 15
  rows in docs/architecture.md, 0 differ`; `overall=0`.

### P5 (2026-10-11)

The owner committed P4 as `cea264e`, and said "proceed".

**Files,** 17, comments only:

| ID | Changed |
| :--- | :--- |
| C1 | `selfupdate/service/doc.go`: the backends are systemd, launchd, scm and taskscheduler |
| C2 | `selfupdate/service/detach.go`: `DetachProcess` gives the run "a console of its own and no window" |
| C3 | `selfupdate/service/handoff.go`: `ProcessDetacher` no longer names Task Scheduler as having no backend, and points to `taskscheduler.Task` |
| C4 | `selfupdate/service/handoffhop.go`: `taskscheduler.Task.Detach` sets `EnvHandOffHop` too (0021-MADR D6) |
| C5 | `selfupdate/service/poll.go`: `Health.Instance` names a Task Scheduler process ID |
| C6 | `selfupdate/service/runner.go` (`ExecRunner`), `selfupdate/probe.go` (`NewVersionProber`), `selfupdate/service/detach.go` (`DetachProcess`), `selfupdate/service/execreconciler.go` (`ExecReconciler.Reconcile`): the batch refusal, with `ErrBatchFile`, and the window rule |
| C7 | `selfupdate/types.go`: `EnabledLifecycle` reads systemd's `UnitFileState` and names the Task Scheduler rule |
| C8 | `selfupdate/service/taskscheduler/task.go` and `selfupdate/service/scm/service.go`: `Options.Name` is needed by the handoff |
| C9 | `taskscheduler/lifecycle.go`, `internal/winproc/winproc_windows.go`: magic-cli-remote's `docs/decisions/0159-MADR-the-windows-service-path-is-half-wired.md`; `selfupdate/github.go`: mcplib's `docs/0005-PLAN-canonicalize-cli-self-update-in-mcplib.md` §4.2. Each was checked to exist there, with the cited probe, decision or section |
| C10 | `internal/winproc/winproc.go`: the library's packages under `selfupdate/`, with the release tool outside |
| C11 | `selfupdate/doc.go`: a paragraph naming `selfupdate/service` and its four backends |
| C12 | `selfupdate/example_test.go`: "a `selfupdate/service` backend" |

**As built, beyond the rows:** C2 also corrected `DetachProcess`'s second
paragraph, "It is for a lifecycle with no backend in this module": the
`scm` and `taskscheduler` backends detach through it. That sentence was
as stale as the first, in the same comment.

**Checks.**

* `git diff -U0 -- '*.go'`: every added and removed line is a comment.
* `go doc` read for `ExecRunner`, `EnabledLifecycle`,
  `taskscheduler.Options`, and the `selfupdate`, `service` and `winproc`
  package docs.
* `make pre-add-check FILES="<the 17 Go files>"`: `go-precheck: 17
  file(s) clean`, the Example tests included.
* `make gate`: every step `rc=0`, `apicheck` `compatible with v1.14.0`,
  `arch` `15 rows …, 0 differ`, `overall=0`.

No test was added, so there was no fail-first run.

### P7, `v1.15.0` (2026-10-11)

* **The release commit** is `ac4ef550916f9c18af0947441f4ab55f7cf3c49a`,
  P5's, which the owner committed and pushed.
* **Step 1:** CI run 38149442253 on `main` at `ac4ef55`: `success`, 17 of
  17 jobs. Its Linux job ran the new step, `check-docs: 15 rows in
  docs/architecture.md, 0 differ`. Its `windows-2025` job's `go test`
  passed `internal/winproc`, `selfupdate`, `selfupdate/service` and
  `taskscheduler` from source. The `TestLiveTask*`, the new one included,
  skip with their stated reason.
* **Step 2, the tag,** made and pushed by the owner. Verified:
  * `v1.15.0` is an annotated tag (`git cat-file -t` gives `tag`), object
    `bc98ed5aca1e0965710f404395891e88298eeb67`, peeled to `ac4ef55`;
  * `git ls-remote origin` gives `ac4ef55…` for `refs/tags/v1.15.0^{}`
    and for `refs/heads/main`;
  * `scripts/check-release-tag.sh v1.15.0` exits 0.
* **Step 3:** the tag's CI run, 38150539554, passed all 17 jobs; all ten
  identity legs print `v1.15.0 (release) ac4ef550916f`.
* **Step 5:**
  * `GOPROXY=https://proxy.golang.org GOFLAGS=-mod=mod go list -m -json
    github.com/maccavelli/go-selfupdate-lib@v1.15.0` gives `v1.15.0`,
    `Time` `2026-10-11T14:59:52Z`, `Origin.Hash` the release commit and
    `Ref` `refs/tags/v1.15.0`;
  * `@latest` resolves to `v1.15.0`.
* **Step 4, the pins,** land with P6, as Q1 decided; see P6 below.

### P6 (2026-10-11)

P6 lands after the tag, with P7's pin commit (Q1).

**Files.** `README.md`, `AGENTS.md`, `docs/README.md`,
`docs/architecture.md`, the three guides,
`docs/reports/0016-REPORT-precheck-gofmt-errors.md`, and
`docs/decisions/0021-PLAN-task-scheduler-lifecycle-and-windowless-children.md`.
No Go file changes.

**The findings, closed:**

* **A5:** the `make` targets list `gate`, and `help`, which `make` alone
  runs (`Makefile:14`).
* **A6:** a "Run, but not required" line for the Task Scheduler live
  tests.
* **X1:** the six titles are their H1s; the index's titles now all match
  (`same=52 differ=0`).
* **X2:** "§6 to §15". **X3:** "or a per-user Task Scheduler task".
  **X4:** AGENTS.md's Dependencies and 0008-MADR.
* **G1:** the scope paragraph names `internal/winproc`; `other-packages`
  covers an `internal/` helper with no rule of its own,
  `internal/cmd/selfupdate-release` today. **G2:** the parity test, with
  `python3` and `sh`. **G3:** "changed and untracked". **G4:**
  `GOLANGCI_LINT=<path>`.
* **R1:** a dated addendum naming 0020-MADR and its PLAN; the report's
  text is unchanged.
* **E1, E6:** the start rule per manager, as a list: systemd's
  `UnitFileState`, launchd's keys, the SCM's start type with
  `TriggerStartEnabled`, Task Scheduler's triggers.
* **E2:** "on the SCM", and Task Scheduler's rewrite sets the `Exec` path.
* **E3:** the stop condition and bound per manager.
* **E4:** `schtasks /change /tn <folder>\<name> /enable`.
* **E5:** an `Options.Folder` bullet, and one for `Restore` on a rollback.
* **E7:** a Task Scheduler repeating trigger in the restart caveat.
* **E8:** a "Without `cli`" bullet: `HandOffIfInside`, `Detached`,
  `ReportFunc` or `WriteHandOffResult`, and `HandOffFunc`.
* **E9:** `Status`, `Stopping`, `Reloading`, `Available`,
  `TemplateInstance`, `Escape` and `ValidUnitName`.
* **E10:** "unless `Policy.AllowSelfHostedRunners` is set".
* **E11:** `service.ExecRunner` and the tools it serves, `ErrBatchFile`,
  and the spellings H2 refuses.
* **The disabled task (MADR Amendment A1):** the guide's "A task you had
  disabled is run, and stays disabled" is replaced by what P1 measured
  and P3 built.
* **B1:** `-Product a,b` for `install.ps1`. **B2:** the floor, and that
  the plan step does not check the requirement against the pin.
  **B3:** the `toolchain` line, or else the `go` line. **B4:** a
  rehearsal's `{tag}` and how the extras job names its files. **B5:** exit
  0, and an environment of `PATH` and `SystemRoot`. **B6:** the default
  artifact names, `retention-days`, the outputs. **B7:** `SHA256SUMS` and
  this platform's asset of each selected product.
* **M1:** the archive check runs the pinned commit's unpacker, tightened
  in `v1.11.0` and `v1.11.1`. **M2:** each patch subsection's `go get`
  and check are marked "only if you stop at" that patch. **M3:** "§7 to
  §15". **M4:** §14's batch list names `service.ExecRunner`, the tools
  that use it and `gh.cmd`, and points to §15.
* **N1:** a dated annotation on 0021-PLAN's release notes.
* **§15** "From v1.14 to v1.15", in §14's form, and a pointer to it in
  §2.

**The pin commit (P7 step 4).**

* **Candidates:** `git grep -n 'v1\.14\.0\|81dae60' -- README.md
  docs/architecture.md docs/guides`, 44 lines with P6's text in place.
* **Preconditions:** `git diff v1.14.0 v1.15.0` of `.github/workflows/`,
  `internal/cmd/` and `scripts/` changes `ci.yml`, `check-docs.sh`,
  `gate.sh` and their tests; neither release workflow, the release tool
  nor the installer templates. So "`v1.11.0` to `v1.15.0` change nothing
  in it" holds.
* **Changed,** 34 lines, to `ac4ef550… # v1.15.0` or `v1.15.0`, by a
  script that changed only the listed lines and asserted each one
  changed: `README.md:24`, `:28`, `:126`; `docs/architecture.md:23-24`;
  the building guide `:26`, `:164`, `:175`, `:186`, `:261`; the migration
  guide `:23`, `:26`, `:97`, `:115`, `:117`, `:125`, `:171`, nine
  `go get` lines (`:390`, `:474`, `:518`, `:570`, `:615`, `:722`, `:836`,
  `:929`, `:974`) and eight `go list` checks (`:513`, `:565`, `:609`,
  `:714`, `:830`, `:924`, `:969`, `:1026`).
* **Kept,** 10 lines that state what `v1.14.0` itself did, or that §15
  compares with: the extending guide `:482`, `:553`, `:566`, `:747`; the
  migration guide `:42`, `:977`, `:1006`, `:1037`, `:1050`, `:1054`.

**Checks.** On the nine files: `scripts/check-docs.sh --links` `201 links
in 9 files, 0 broken`; `--ids` `9 files, 16 deny-list rules, 0
findings`; markdownlint `0 issues`; `--arch` `0 differ`. `make gate`:
every step `rc=0`, `apicheck` `compatible with v1.15.0`, the newest tag
now, `overall=0`.

### Release notes for v1.15.0 (2026-10-11)

`v1.15.0` hardens the Windows batch-file refusal and the Task Scheduler
backend, and corrects the documentation. `make apicheck` reports it
compatible with `v1.14.0`; the exported changes are two new errors.

* **`selfupdate.ErrBatchFile`:** the error a refused `.bat` or `.cmd`
  program wraps, from the version probe, `service.ExecRunner`,
  `service.DetachProcess` and the handoff, so `errors.Is` can match it.
  A static test holds the `%w` chain from `internal/winproc` to the
  exported entry points, on every OS.
* **Batch-file spellings closed:** on Windows, `v1.14.0`'s check read the
  extension as written. A trailing dot (`x.bat.`) put an argument under
  `cmd.exe`'s parser, and a trailing space ran the script; both were
  measured. The check now reads the name as Windows normalizes it, and
  cuts an alternate data stream suffix.
* **`taskscheduler.ErrTaskDisabled`:** Task Scheduler refuses to run a
  disabled task (`SCHED_E_TASK_DISABLED`, measured). `Stop` now refuses a
  disabled task whose program runs, before it changes anything, instead
  of ending the program and failing at `Start`; `Start` maps the refusal
  to the same error.
* **Documentation:** 50 corrections across the guides, `go doc`,
  `AGENTS.md` and the records index. `make gate` and CI check the
  architecture page's file counts (`check-docs.sh --arch`).
* **Tests:** the source-reading tests fail when they find no source,
  instead of passing on nothing.
* **Records:**
  [0022-MADR-documentation-pass-after-v1-14-0.md](0022-MADR-documentation-pass-after-v1-14-0.md)
  and this PLAN, with Amendment A1.

### P8: Verification, at closing (2026-10-11)

The owner committed P6 and the pin commit as `eb36b56`. It is not pushed
yet, so CI has not run on it; P6 changed no Go file, and `make gate`
passed on its tree.

* **V1 (findings): met.** All 50 are closed, each with its change named
  in a phase record: A1 to A4 in P4; C1 to C12 in P5; A5, A6, X1 to X4,
  G1 to G4, R1, E1 to E11, B1 to B7, M1 to M4 and N1 in P6. None was left
  undone.
* **V2 (table): met.** `--arch` on `cf0664c`'s table reported exactly A1
  to A4, exit 1, and reports `15 rows …, 0 differ` on the tree. Its test
  failed on the `if False and …` plant, and the gate's test on a gate
  without the step. `make gate`'s `arch` step and CI's (run 38149442253,
  Linux job) both ran it.
* **V3 (sentinel and chain): met, with one name more than planned.** The
  probe, `ExecRunner` and `DetachProcess` tests failed on the wrapped
  sentinel on the Windows test host, and pass there and in CI's Windows
  job. `TestBatchErrorChain` failed on each of five plants, naming the
  line, and passes on every OS from source. `apicheck` lists
  `ErrBatchFile` and, by Amendment A1, `ErrTaskDisabled`; the plan's "only
  `ErrBatchFile`" predates A1.
* **V4 (spellings): met.** P1's table is recorded verbatim. Each spelling
  that ran a script or put an argument under `cmd.exe` is refused, and
  `TestBatchFileRefused`'s nine new rows failed on `v1.14.0`'s guard on
  the Windows test host.
* **V5 (disabled task): met, by the plan's second branch.** P1's task
  probe is recorded; P3 stopped, and the owner's decision is recorded as
  Deviation D1 and MADR Amendment A1. `TestLiveTaskStopRefusesDisabled`,
  which replaced the planned `TestLiveTaskStartDisabled`, passed
  `-count=3` on the Windows test host, and failed on both of its plants
  there.
* **V6 (no unplanned change): met.** P5's diff is comments only, by
  `git diff -U0`. P6 changed no Go file. `apicheck` shows two additions,
  as in V3.
* **V7 (release): met.** `v1.15.0` is an annotated tag on `ac4ef55`; its
  CI (38150539554) passed 17 of 17 jobs, with ten identity legs; the
  proxy serves it, and `@latest` resolves to it. The pins name it in
  `eb36b56`.

### Closing (2026-10-11)

Every phase, P0 to P8, is recorded above, and V1 to V7 are met as stated.
This PLAN is `complete`, here and in the index. The MADR stays
`accepted`, with Amendment A1.

**Open at closing:**

* **From 0021, unchanged:** where `0x8004130B` arises is [unverified],
  so no test reaches its mapping; and whether a GitHub Windows runner can
  run an interactive-token task is [unverified], so CI skips the live
  Task Scheduler tests and the Windows test host is their evidence.
* **`TestBatchErrorChain`'s reach:** it follows the error through plain
  functions. A method, or a function literal, ends the walk, so a future
  site that returns the error through one is judged only up to there.
  Today's exported entry points are reached.
* **`pidRunning`** in `live_windows_test.go` opens a process handle,
  which can succeed on a process that has exited. Only a log line in
  `TestLiveTaskStopHoldsDisabled` uses it now; the new test uses the
  process snapshot.
* **CI on `eb36b56`** runs once it is pushed.

**Checks.** On this PLAN and `docs/README.md`: `scripts/check-docs.sh
--links` `93 links in 2 files, 0 broken`; `--ids` `2 files, 16 deny-list
rules, 0 findings`; markdownlint `0 issues` on `docs/README.md`, the
only one of the two its configuration lints. `make gate`: every step
`rc=0`, `apicheck` `compatible with v1.15.0`, `arch` `0 differ`,
`overall=0`.
