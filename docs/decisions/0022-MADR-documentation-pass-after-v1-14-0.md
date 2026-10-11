---
status: accepted
date: 2026-10-11
decision-makers: go-selfupdate-lib maintainers
consulted: "a read-only audit of README.md, AGENTS.md, docs/, the three guides and every package's doc comments and Examples, against the code at cf0664c (v1.14.0 plus records), 2026-10-11; Go 1.27.2's os/exec and syscall on Windows; Microsoft's Task Scheduler 2.0 COM reference"
informed: the fleet's programs that read this module's guides and go doc, magic-cli-remote first
---
# Correct the documentation against v1.14.0, check the architecture table in the gate, export and harden the batch-file refusal, and measure Task Scheduler's run of a disabled task

## Context and Problem Statement

The owner asked, on 2026-10-11, to fix the four wrong rows of
`docs/architecture.md`'s package table that
[0021-PLAN-task-scheduler-lifecycle-and-windowless-children.md](0021-PLAN-task-scheduler-lifecycle-and-windowless-children.md)
T7 found and left alone, and to go over the documentation for items that
need updating and for gaps. magic-cli-remote is assessing a move onto
`v1.14.0`, so its authors are about to read the guides and `go doc` closely.

The audit raised two questions beyond the documents: the Windows
batch-file refusal has no sentinel a program can match, and the guide's
claim about a disabled task rests on a fake alone. This record decides
four things:

* D1: what the pass corrects;
* D2: how the architecture table is kept from drifting again;
* D3: how the batch-file refusal is exported and hardened;
* D4: how Task Scheduler's run of a disabled task is measured and held.

**Facts [observed 2026-10-11, `main` at `cf0664c`]:**

* **The audit.** Every document a reader of the module meets was checked
  against the code: README.md, AGENTS.md, `docs/README.md`,
  `docs/architecture.md`, the three guides, `docs/reports/`, every
  package's doc comment, exported doc comments and Example functions. Each
  finding was confirmed against the source before it was listed. The
  PLAN's "Findings" table lists all 50, with file, line and evidence:

  | Where | Findings | Kind |
  | :--- | :--- | :--- |
  | `docs/architecture.md` | 6 | 4 wrong count rows; `make gate` missing from the target list; CI's Task Scheduler step missing from the live-test list |
  | `docs/README.md` | 4 | 6 titles (0001 to 0003) that are not their record's H1; "§6 to §11"; the service row without Task Scheduler; the import-rules row pointing at 0001-MADR §3, which predates depguard |
  | `AGENTS.md` | 4 | `internal/winproc` missing from the scope paragraph, and `other-packages`' wording; `SELFUPDATE_REQUIRE_PYTHON`'s second test; `GOLANGCI_LINT`; the `ids` step's untracked files |
  | `docs/reports/0016-REPORT-…` | 1 | "Associated record: none", though 0020-MADR now decides it |
  | Go doc comments | 12 | `selfupdate/service`'s docs written before `taskscheduler` (backend list, `ProcessDetacher`, `EnvHandOffHop`, `Health.Instance`); `DetachProcess` "with no console"; the process starters silent on D7 and D8; `EnabledLifecycle` "systemd is-enabled"; `Options.Name` and the handoff; cross-repository records cited by number; `winproc`'s package doc; `selfupdate`'s package doc and Example silent on `selfupdate/service` |
  | `extending-selfupdate.md` | 11 | 4 wrong (systemd `is-enabled`, "on Windows" for an SCM-only rule, the stop bound, `schtasks /tn` without the folder); 7 gaps (`Folder` and `Restore`, `TriggerStartEnabled`, the replace-before-stop restart caveat, the handoff without `cli`, the systemd helpers, `AllowSelfHostedRunners`, the batch error) |
  | `building-releases.md` | 7 | `-Product` repeated in `install.ps1`; the module floor; the Go version source; the rehearsal stamp in extras' names; the identity run's exit status and environment; `retention-days`; what the installers download |
  | `migrating-from-mcplib-selfupdate.md` | 4 | the archive check's tightening under an unchanged publish workflow; patch-release `go get` lines that downgrade; "§7 to §10"; `service.ExecRunner` missing from §14's batch list |
  | `0021-PLAN` release notes | 1 | the same `service.ExecRunner` omission |

* **The four count rows** were already wrong at `31cce88`, before 0021.
  Every PLAN that adds a file has to remember the table, and four did not.
  `scripts/check-docs.sh` checks links and identifiers; nothing checks the
  table.
* **The batch-file refusal has no exported sentinel.**
  `internal/winproc.ErrBatchFile` is returned, wrapped with `%w`, by
  `selfupdate`'s version probe (`probe.go:102-104`),
  `service.ExecRunner` (`runner.go:72-74`) and `service.DetachProcess`
  (`detach_windows.go:28`). A caller outside the module cannot import an
  `internal/` package, so it cannot test for the refusal with `errors.Is`.
  `service.ExecRunner` is the default runner of `ghattest` and `codesign`,
  so a `gh.cmd` given as `ghattest.Options.GH` now fails the update, and
  the program can only match the message.
* **Nothing holds the chain.** `errors.Is` reaches the sentinel only while
  every site wraps with `%w`. `TestNoDirectProcessStart`
  (`selfupdate/spawn_test.go`) holds that every start goes through
  `internal/winproc`. Nothing holds how its error is wrapped, and the
  Windows tests that would notice run only on Windows.
* **The guard reads the name as written.** `checkProgram`
  (`internal/winproc/winproc_windows.go:36-49`) resolves an absolute path
  with `exec.LookPath`, then compares `strings.ToLower(filepath.Ext(path))`
  with `.bat` and `.cmd`. Win32 path normalization drops a final
  component's trailing dots and spaces, so `C:\d\run.bat.` and
  `C:\d\run.bat ` name `run.bat` [docs: Win32 file naming]. For both,
  `filepath.Ext` gives `.` or `.bat `, and the guard passes them. Whether
  `CreateProcess` then runs the file through `cmd.exe`, and what an
  alternate data stream suffix (`run.bat::$DATA`) does, is [unverified].
  *(Measured in PLAN P1, 2026-10-11: `run.bat.`, `run.bat..` and
  `run.cmd.` put an injected argument under `cmd.exe`'s parser without
  running the script; `run.bat ` and `run.cmd ` run the script. The
  `v1.14.0` guard allows all five. `GetFullPathName` gives `run.bat` or
  `run.cmd` for each. `os/exec` refuses the stream spellings before any
  start [measured].)*
* **A disabled task's run is not measured.** The extending guide says "A
  task you had disabled is run, and stays disabled". `Task.Start` enables
  a task only when `Stop` disabled it (`lifecycle.go:243-253`), then calls
  `IRegisteredTask::Run` (`com_windows.go:71`). The fake shows the call
  (`lifecycle_test.go:215-235`). Whether Task Scheduler runs a disabled
  task on `Run`, refuses it, or runs it only through `RunEx` with
  `TASK_RUN_IGNORE_CONSTRAINTS`, is [unverified]. No live test covers it.
  If `Run` refuses it, a managed update of a program whose task the user
  disabled, but which was running, fails at `Start` and rolls back.
  *(Measured in PLAN P1, 2026-10-11: `Run` and `RunEx` with
  `TASK_RUN_IGNORE_CONSTRAINTS` both return `0x80041326`
  (`SCHED_E_TASK_DISABLED`) for a disabled task. The action does not run,
  and the task stays disabled. The guide's claim is wrong, and decision 6's
  stop applies [measured].)*

## Decision Drivers

* **The documents describe the code as it is.** A reader porting a program
  follows the guide literally; a wrong command or a downgrade costs them
  more than a missing paragraph.
* **A wrong count is a defect the gate can catch.** The table has drifted
  once already. A rule that each PLAN must remember has failed.
* **The owner prefers an extensible API now** over a later breaking
  addition, where the addition is sensible.
* **A security guard is only as good as its weakest spelling.** BatBadBut
  guards that compare the name as typed have been bypassed by the forms
  Windows normalizes; the guard must refuse what Windows runs as a batch
  file, measured, not assumed.
* **A claim in the guide is measured or removed.** 0021 held every
  Task Scheduler behaviour it relied on to a live test; this one was
  missed.
* **No behaviour change** in the documentation phases. Code changes stay
  in their own phases, with their own checks.
* **Records stay historical.** Records are annotated, not rewritten.

## Considered Options

### D1: what the pass corrects

* **D1-A. Every confirmed finding, in the documents and the doc comments.**
* **D1-B. Only the four rows the owner named.**
* **D1-C. The Markdown only, leaving the doc comments for the next code
  change.**

### D2: keeping the table's counts right

* **D2-A. A third `check-docs.sh` mode, `--arch`, that recounts each row's
  non-test and test `.go` files from `git ls-files`, run by `make gate`
  and CI.**
* **D2-B. Fix the rows by hand, and nothing more.**
* **D2-C. Drop the two count columns from the table.**

### D3: the batch-file refusal

* **D3-A. Export `selfupdate.ErrBatchFile`, the same value as
  `winproc.ErrBatchFile`, and release `v1.15.0`.**
* **D3-B. Document the message only ("an error that names BatBadBut"),
  and export nothing.**
* **D3-C. Export it from `selfupdate/service` instead.**

With D3-A, the owner asked for two ways to harden it. Considered:

* **H1. A static test that every error from `winproc.Command` and
  `winproc.Detached` is returned bare or wrapped with `%w`,** beside an
  all-OS test that `selfupdate.ErrBatchFile` is `winproc.ErrBatchFile`.
* **H2. Check the name Windows will run, not the name as written:**
  normalize the path as Win32 does (`GetFullPathName`), drop an alternate
  data stream suffix, then compare the extension; the variants refused are
  those a measurement on the Windows test host shows `CreateProcess`
  running through `cmd.exe`.
* **H3. Refuse at construction:** `ghattest.NewVerifier`,
  `codesign.NewSigner` and `NewExecReconciler` check their program when
  built.
* **H4. A distinct exit code or JSON error kind** for the refusal in
  `selfupdate/cli`.
* **H5. Refuse by content:** read the file's first bytes and refuse
  anything that is not a PE image.

### D4: a disabled task's run

* **D4-A. Measure `Run` and `RunEx(TASK_RUN_IGNORE_CONSTRAINTS)` on a
  disabled task on the Windows test host, record the result, and add a
  live test that holds `Start`'s behaviour on a disabled task.**
* **D4-B. Remove the guide's claim, and measure nothing.**
* **D4-C. Leave it to a later amendment of 0021.**

## Decision Outcome

Chosen on 2026-10-11 by the owner: **D1-A, D2-A, D3-A hardened by H1 and
H2, and D4-A.** The owner's words: "D2, follow recommendation, D3, follow
recommendation, but find 2 ways to better harden the solution. once you
have the solution proposed for D3, write the madr and include the task
scheduler measurement and test. proceed." The owner asked the agent to
find the two hardenings; H1 and H2 are the agent's choice under that ask,
for the reasons below.

* **D1-A,** because every finding is checked against the code and most
  are one-line fixes. Leaving the doc comments for later would leave
  `go doc service` saying there are three backends while magic-cli-remote
  reads it.
* **D2-A,** because it turns a rule that has failed into a check that
  fails. It counts only what the table states as a number of `.go` files.
  It does not judge the imports column or the `testdata/` notes, which
  change rarely and need reading, not counting.
* **D3-A,** because every refusal site already wraps the error with `%w`,
  so one exported variable makes `errors.Is` work at all of them. It is a
  compatible addition. `selfupdate` is the package every program imports,
  and `service` already imports `selfupdate`, so one name serves both.
* **H1,** because the export is a promise that `errors.Is` works, and
  only the Windows tests would see a site that broke it. A static test
  runs on every OS, in every `go test`, and names the file and line.
* **H2,** because a sentinel for a guard that a trailing dot defeats
  documents a hole. Normalizing as Win32 does closes the known class of
  spellings in one place, for every caller, and the measurement decides
  which spellings matter rather than a guess.
* **D4-A,** because the outcome decides whether `Start` is correct, not
  only whether the guide is. A live test holds whichever behaviour is
  measured.

**The decisions in detail:**

1. **D1.** Each finding in the PLAN's table is fixed as its row says.
   Records are annotated, never rewritten: the 0016 REPORT gains an
   addendum, and 0021-PLAN's release notes an annotation. The index's six
   titles become their records' H1s, as the other 44 rows already are.
2. **D2.** `scripts/check-docs.sh --arch docs/architecture.md`:
   * reads the table under `## Go code`;
   * takes the first integer of the "Non-test files" and "Test files"
     cells;
   * counts `git ls-files '<dir>/*.go'` in that directory alone, split on
     `_test.go`;
   * reports each row that differs, with both numbers, and exits 1.

   `scripts/gate.sh` gains an `arch` step, and CI runs the mode where it
   runs `--links`. `check-docs_test.sh` gains a case, and the check is
   seen failing on today's table before the rows are fixed.
3. **D3.** `selfupdate/errors.go` gains `ErrBatchFile =
   winproc.ErrBatchFile`, documented with its call sites. The Windows
   tests of the probe, the runner and the handoff assert
   `errors.Is(err, selfupdate.ErrBatchFile)`. The guides name it.
   `v1.15.0` carries it, with its pins and the migration guide's §15.
4. **H1.** A test in `selfupdate`, beside `TestNoDirectProcessStart` and
   walking the same files, finds each call of `winproc.Command` and
   `winproc.Detached` and follows its error result. The error must be
   returned as it is, or passed to `fmt.Errorf` with a `%w` verb for that
   argument. Anything else fails with the file and line. A second test,
   on every OS, asserts `selfupdate.ErrBatchFile == winproc.ErrBatchFile`.
5. **H2.** On Windows, `checkProgram`:
   * resolves the path as today;
   * normalizes it with `windows.GetFullPathName`, which drops a final
     component's trailing dots and spaces;
   * drops an alternate data stream suffix (`:` after the volume);
   * then compares the extension, case-insensitively.

   The measurement comes first. It records, for each spelling, whether
   `CreateProcess` through `os/exec` runs a batch file, and what the guard
   says today. The fix must refuse each spelling the measurement shows
   running, and the test table holds each one. If the measurement shows a
   spelling the fix cannot refuse, the phase stops.
6. **D4.** On the Windows test host, a disabled task is run by
   `IRegisteredTask::Run`, and again by `RunEx` with
   `TASK_RUN_IGNORE_CONSTRAINTS`. For each, the record notes the HRESULT,
   whether the action ran, and the task's enabled state afterwards. A live
   test, `TestLiveTaskStartDisabled`, then holds `Start` on a task the user
   disabled:
   * **if `Run` runs it,** `Start` succeeds, a new instance runs, and the
     task stays disabled; the guide's claim stands, now measured;
   * **if `Run` refuses it,** the phase stops. The choices then (keep the
     refusal as an error that names the disabled task, use `RunEx`, or
     enable, run and disable again) change `Start`'s behaviour. They are
     put to the owner with the measurement, and recorded as an amendment
     here.

### Consequences

* Good, because the guides, `go doc` and the index agree with the code at
  the release magic-cli-remote will adopt.
* Good, because the next PLAN that adds a file fails the gate until the
  table says so.
* Good, because a program can tell the batch refusal from every other
  failure, and a later change cannot quietly break that on any OS.
* Good, because the batch guard refuses what Windows runs, by measurement.
* Good, because every Task Scheduler claim in the guide is then held by a
  live test.
* Neutral, because `--arch` checks two numbers per row and nothing else
  in the table.
* Bad, because every PLAN that adds or removes a `.go` file must now edit
  `docs/architecture.md` in the same commit, or the gate fails.
* Bad, because D3-A needs a release, its pins and a migration section.
* Bad, because H2 refuses paths that `v1.14.0` started. A program that
  meant to run `run.bat.` was already running a batch file, which D8
  forbids; the migration guide says so.

### Confirmation

* Each finding in the PLAN's table is closed in the execution record, with
  the line that changed, or recorded as not done with the reason.
* `scripts/check-docs.sh --arch docs/architecture.md` exits 1 on `cf0664c`'s
  table, naming exactly the four rows, and 0 after the fix.
  `check-docs_test.sh` fails when its `--arch` case is planted away.
* `make apicheck`'s full report against `v1.14.0` lists only
  `ErrBatchFile`, as added.
* H1's test fails on a copy where one site wraps with `%v`, naming that
  line, and on a copy where `ErrBatchFile` is a new error.
* H2: the measurement is in the PLAN's record. `TestBatchFileRefused`
  holds each spelling it shows running, and fails on a copy of today's
  guard. It runs on the Windows test host and in CI's Windows job.
* D4: the measurement is in the PLAN's record, and
  `TestLiveTaskStartDisabled` passes `-count=3` on the Windows test host.
  It fails on a copy whose `Start` returns before the run.
* `make gate` ends `overall=0` at every phase; CI passes on each pushed
  commit.

## Pros and Cons of the Options

### D1-A. Every finding

* Good, because the pass leaves no known error behind.
* Bad, because it touches about twenty files.

### D1-B. The four rows only

* Good, because it is the smallest change.
* Bad, because 46 confirmed errors stay. Among them are a
  `go get` sequence that downgrades the module and a recovery command that
  fails for a task in a folder.

### D1-C. Markdown only

* Good, because no Go file changes.
* Bad, because `go doc` is where an adopting program reads the backend
  list and the handoff, and it stays wrong.

### D2-A. A check

* Good, because drift fails the gate.
* Neutral, because it reuses `check-docs.sh`'s Python and its test file.
* Bad, because it adds a gate step that every file-adding change meets.

### D2-B. By hand

* Good, because it costs nothing beyond the fix.
* Bad, because it is how the table drifted.

### D2-C. Drop the counts

* Good, because nothing can drift.
* Bad, because the table loses the one figure that shows each package's
  size and test weight, and the owner asked for the rows to be fixed, not
  removed.

### D3-A. `selfupdate.ErrBatchFile`

* Good, because `errors.Is` works at every site, with no change to how
  the error is made.
* Good, because it is additive; `apicheck` stays compatible.
* Bad, because it needs `v1.15.0`.

### D3-B. Document the message

* Good, because no release is needed.
* Bad, because matching an error's text is the practice the module's
  typed errors exist to avoid.

### D3-C. `service.ErrBatchFile`

* Good, because two of the three sites are in `service`.
* Bad, because the version probe is in `selfupdate`, which cannot import
  `service`, so a program that probes would import `service` only for the
  name.

### H1. The chain held statically

* Good, because it runs on every OS and fails at the line that breaks
  `errors.Is`.
* Neutral, because it follows the error through one assignment or a
  direct `return`, the forms the sites use; a site written another way
  fails it and must be written plainly.
* Bad, because it is a second AST walk to maintain.

### H2. The name Windows runs

* Good, because it closes the trailing-dot and trailing-space spellings
  in the one function every start passes through.
* Good, because it is measured, so the test table is evidence, not a
  list of guesses.
* Bad, because it calls `GetFullPathName` on each start, a cheap Win32
  call, and refuses some odd spellings that `v1.14.0` allowed.

### H3. Refuse at construction

* Good, because a bad `gh.cmd` fails when the program starts, not during
  an update.
* Bad, because the start-time check stays necessary (paths change after
  construction), so it adds a second check rather than hardening the
  first, and it changes three constructors' errors.

### H4. A distinct exit code or error kind

* Good, because a script can react without parsing.
* Bad, because `cli`'s exit codes 0, 10 and 1 are a fleet contract, and
  the JSON document's error field is text; a new code is a contract
  change for one failure.

### H5. Refuse by content

* Good, because no spelling can disguise a batch file.
* Bad, because `CreateProcess` picks `cmd.exe` by the name's extension,
  which is what BatBadBut exploits, so the name check stays needed
  [derived]. Reading the file also races the start.

### D4-A. Measure and test

* Good, because `Start`'s behaviour on a disabled task is known and held.
* Bad, because it needs the Windows test host and a console sign-in.

### D4-B. Remove the claim

* Good, because no host time is needed.
* Bad, because `Start`'s behaviour stays unknown, and it decides whether
  an update rolls back.

### D4-C. Later

* Good, because 0022 stays about documents.
* Bad, because magic-cli-remote adopts the backend now, and the owner
  asked for it here.

## Amendments

### A1 (2026-10-11): `Stop` refuses a disabled task whose program runs

* **Found in PLAN P1:** Task Scheduler refuses `Run`, and `RunEx` with
  `TASK_RUN_IGNORE_CONSTRAINTS`, on a disabled task with `0x80041326`
  (`SCHED_E_TASK_DISABLED`) [measured]. In `v1.14.0`, an update of a
  program whose task the user disabled while it ran ends the program,
  replaces the binary, fails at `Start`, fails again at recovery's
  `Start`, and leaves the program stopped (PLAN Deviation D1).
* **Decision (the owner, 2026-10-11, "follow recommendations"):**
  * `Stop` reads the task's enabled state before it changes anything. A
    disabled task with a running instance is refused with
    `taskscheduler.ErrTaskDisabled`, whose message names the remedy:
    enable the task, or stop the program, then update. Nothing is
    stopped, disabled or replaced, and the program keeps running. A
    disabled task with no instance is left as it is, as before.
  * `Start` maps `0x80041326` to `ErrTaskDisabled`, for a task found
    disabled when `Stop` did not disable it.
  * `ErrTaskDisabled` is exported, so a program can match the refusal as
    it matches `selfupdate.ErrBatchFile`; it is a compatible addition in
    `v1.15.0`.
* **What this changes:** decision 6's first branch ("if `Run` runs it")
  did not arise. P3's live test becomes `TestLiveTaskStopRefusesDisabled`,
  which holds the refusal, the running program, and `Start`'s mapping.
  The extending guide's line 583 is replaced, not kept.
* **Not chosen:** `Start` enabling, running and disabling again (the
  triggers are live in between, and a failed re-disable loses the user's
  setting during an update that worked); a clearer error at `Start`
  alone (the program is still left stopped).

### A2 (2026-10-11, proposed): the error chain past interfaces, and a process check that sees an exit

The owner asked, after `v1.15.0`, to remedy two items
[0022-PLAN-documentation-pass-after-v1-14-0.md](0022-PLAN-documentation-pass-after-v1-14-0.md)
left open at closing, and to ground the remedy in the code. Its plan is
[0022-PLAN-error-chain-and-process-liveness.md](0022-PLAN-error-chain-and-process-liveness.md).

**Facts [observed 2026-10-11, `main` at `78dde39`]:**

* **`TestBatchErrorChain` stops at the first interface.** It follows
  `winproc`'s error through plain functions, by name, and stops at a
  method or a function literal (`selfupdate/spawn_test.go`,
  `chainWalk.function`). Every method on today's chain sits behind an
  interface, and the exported outcome is reached only through one:
  * `versionProber.Probe` is called as `Prober.Probe` by
    `Updater.runProbes` (`updater.go:488`) and the installed probe
    (`session.go:290`);
  * `ExecRunner` is called as `Runner.Run` by `ghattest`
    (`ghattest.go:239`), `codesign` (`codesign.go:98`),
    `ExecReconciler` (`execreconciler.go:111`, `:160`) and the systemd and
    launchd backends;
  * `DetachProcess` is reached as `Detacher.Detach`, from
    `HandOffIfInside`, and a failed handoff is `cli.Run`'s error,
    joined (`cli/run.go:143-144`).

  A syntax walk cannot follow an interface call. Each of those callers
  wraps with `%w` today (each line read); nothing holds that.
* **Every boundary has an exported injection point:** `Config.Probes` and
  `ManifestVerifiers`, `InstallOptions.PostInstall`,
  `ghattest.Options.Runner`, `codesign.CheckOptions.Runner` and
  `SignOptions`, `service.ExecOptions.Runner`, a `service.Detacher`, and
  `cli.HandOff.Detach`.
* **`pidRunning` is not only a test's weakness.** The live test's
  `pidRunning` (`live_windows_test.go:590`) opens the process with
  `PROCESS_QUERY_LIMITED_INFORMATION` and calls a successful open
  "running". The backend's own check, `sysSched.running(pid, born)`
  (`scheduler_windows.go:57-60`), calls `created`
  (`OpenProcess` and `GetProcessTimes`) and compares the creation time.
  Both succeed on a process that has exited, for as long as any process
  holds a handle to it: "the objects themselves exist until all open
  handles to them are closed" [docs: Terminating a Process]. `running`
  decides `Stop`'s wait (`waitGone`, `lifecycle.go:226`) and `Start`'s
  filter of a stale instance (`current`, `lifecycle.go:289`, 0021
  Amendment A4). A handle held after the exit, by Task Scheduler's engine
  or by any other process, would make `Stop` wait to its timeout and fail
  with `ErrTimeout`, or make `Start` skip the run. Whether Task
  Scheduler's engine holds one, and for how long, is [unverified]; the
  live tests have passed, so it is not long in the cases they reach.
* **The module already has the documented check:**
  `service.processRunning` (`hopwait_windows.go:35-43`) opens with
  `SYNCHRONIZE` and calls the process running only while
  `WaitForSingleObject(h, 0)` returns `WAIT_TIMEOUT`. "When a process
  terminates, the state of the process object becomes signaled" [docs].
  It is unexported in `service`, and only the hop's test helper uses it.
* **The other readings are unreliable** [docs]: `GetExitCodeProcess`
  returns `STILL_ACTIVE` (259) for a running process, which a process
  can also exit with; `GetProcessTimes`'s exit time "is undefined" while
  the process runs.
* **The Toolhelp snapshot** (`running(pid, 0)`, which
  `TestLiveTaskStopRefusesDisabled` uses) lists live processes; whether it
  still lists one that has exited while a handle is held is [unverified].

**Options for the chain (D5):**

* **D5-A. A behavioural test at each exported entry point, on every OS:**
  a fake at the interface boundary returns an error wrapping
  `winproc.ErrBatchFile`, and the test asserts
  `errors.Is(err, selfupdate.ErrBatchFile)` where a program sees the
  error: `Updater.Run`'s result for a staged probe and a manifest
  verifier, `Install`'s for a post-install probe and a managed install's
  `ExecReconciler`, `ghattest` and `codesign` through their `Runner`,
  `HandOffIfInside`, and `cli.Run`'s error when the handoff fails.
  `TestBatchErrorChain` keeps the concrete layer, and its doc names the
  boundary and the test past it. Cost: one table of cases per package,
  no production change.
* **D5-B. A type-aware static walk** (`go/types` with the source
  importer): methods resolve, but an interface call still needs a rule
  that any implementation may return the error, and the importer reads
  one build context, so Windows-only files need a second pass. Cost: a
  larger test that judges forms, not outcomes.
* **D5-C. A call graph from `golang.org/x/tools`:** a new module
  requirement, which AGENTS.md admits only with a record, for a test.
* **D5-D. Keep the limit, documented.**

**Options for the process check (D6):**

* **D6-A. One check in `internal/winproc`, used by every caller:**
  `winproc.Alive(pid uint32, born uint64) bool`: open with `SYNCHRONIZE |
  PROCESS_QUERY_LIMITED_INFORMATION`; no such process is not alive; when
  `born` is set, a different creation time is a reused ID, not alive;
  alive only while `WaitForSingleObject(h, 0)` returns `WAIT_TIMEOUT`.
  An open refused with `ERROR_ACCESS_DENIED` means a process exists that
  the caller may not open; it falls back to the snapshot, as `running`
  does today without `born`. `sysSched.running`, `service.processRunning`
  and the live tests use it; `pidRunning` is removed. Cost: one function
  and its Windows test, three call sites, and a patch release.
* **D6-B. Fix `sysSched.running` alone,** copying `processRunning`'s
  pattern. Cost: lower, but two copies of one check, and the test helper
  stays wrong.
* **D6-C. `GetExitCodeProcess` against `STILL_ACTIVE`:** ambiguous for a
  program that exits with 259 [docs].
* **D6-D. `GetProcessTimes`'s exit time:** undefined while running
  [docs].
* **D6-E. Keep it:** the test helper only logs.

**Proposed: D5-A and D6-A,** because:

* D5-A checks the outcome a program sees, at the exported entry points,
  on every OS, where D5-B and D5-C would still be judging the shape of
  the code. A planted `%v` at any layer fails a named case.
* D6-A uses the check Windows documents for "has it exited", which the
  module already uses in one place, and puts it where both backends'
  process code already lives. It fixes the backend, not only the test.
* A measurement comes first, on the Windows test host: whether a
  process that exited while a handle is held still opens, keeps its
  creation time, appears in the snapshot, and signals; and whether Task
  Scheduler's engine holds a handle after the task's program ends, and
  for how long. The plan stops if the measurement contradicts the docs.

**Consequences:**

* Good, because `errors.Is(err, ErrBatchFile)` is held where programs
  read it, not only up to the first interface.
* Good, because `Stop` and `Start` read a process as gone when it is,
  whoever still holds a handle to it.
* Neutral, because D6-A asks for `SYNCHRONIZE` as well as the query
  right. That a task's program grants it to the updater is [unverified]
  until the measurement; an open refused falls back to the snapshot.
* Bad, because the backend fix needs `v1.15.1`.

## More Information

* The audit ran read-only at `cf0664c`. Its scratch checkers and example
  builds lived outside the repository.
* [0021-MADR-task-scheduler-lifecycle-and-windowless-children.md](0021-MADR-task-scheduler-lifecycle-and-windowless-children.md)
  D7 and D8 made the refusal and the windowless children; Amendment A1
  measured Task Scheduler's other behaviours; Amendment A4 holds the open
  items its closing listed.
* [0008-MADR-enforce-import-rules-with-depguard.md](0008-MADR-enforce-import-rules-with-depguard.md):
  `selfupdate` already allows `internal/winproc`, and `internal-winproc`
  already allows `golang.org/x/sys/windows`, so neither D3 nor H2 needs a
  rule change.
* Microsoft's "Naming Files, Paths, and Namespaces": the Win32 layer
  strips trailing periods and spaces from a path's final component.
