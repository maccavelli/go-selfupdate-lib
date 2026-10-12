---
status: in-progress
date: 2026-10-12
associated-madr: "0021-MADR-task-scheduler-lifecycle-and-windowless-children.md"
---
# Implement the measured instance race and the CI probe of the Task Scheduler live tests

Associated MADR: [0021-MADR-task-scheduler-lifecycle-and-windowless-children.md](0021-MADR-task-scheduler-lifecycle-and-windowless-children.md),
Amendment A5 (E1-A, E2-A). The record's first plan is
[0021-PLAN-task-scheduler-lifecycle-and-windowless-children.md](0021-PLAN-task-scheduler-lifecycle-and-windowless-children.md),
complete.

## Goal

* **E1-A, measured:** what an instance that has ended returns from
  `get_EnginePID`, and from the calls around it, on the Windows test
  host.
* **E1-A, fixed if the measurement calls for it:** `instances` skips an
  instance whose `get_EnginePID` returns `SCHED_E_TASK_NOT_RUNNING`
  (`0x8004130B`), with a test, released as `v1.15.2`.
* **E2-A:** CI runs the Task Scheduler live tests once with failures
  allowed, then either requires them or skips them with a message that
  states what was found.

## Scope

### In scope

| Phase | What | Commit |
| :--- | :--- | :--- |
| U0 | Amendment A5's decision, this plan and the index row | records only |
| U1 | the instance probes on the Windows test host | records only |
| U2 | `instances` skips an ended instance, if U1 calls for it | code and tests |
| U3 | the CI probe: session facts, and the live tests with failures allowed | CI |
| U4 | CI requires the live tests, or skips them with a true message | CI, a test message, docs |
| U5 | the migration guide's `v1.15.2` part, if U2 landed | docs |
| U6 | `v1.15.2`, if U2 landed | per 0017's release procedure |
| U7 | closing | records only |

### Out of scope

* **`IRunningTask` methods this module does not call,** such as
  `Refresh` and `get_Path`: U1 records them for context only.
* **The `GetInstances` mapping of `0x8004130B`** stays, as A5 says; U2
  changes only its comment to say what is measured.
* **The stale listing** (Amendment A4) beyond what U1's runs show.
* **The runner image:** this plan changes only this repository's
  workflow.
* **Push and tags,** which happen only on the owner's ask in the same
  turn.

## Rules

* **`main`, no branch.** The owner commits and pushes on `main`. An agent
  commits only on an explicit "commit to main" in that turn, with
  `git commit --no-edit`.
* **Each phase stops at a deviation** (AGENTS.md, Plan deviations). U1's
  and U4's branches are decided by Amendment A5 and are not deviations; a
  result outside them is.
* **Checks before each commit:** `make pre-add-check FILES="…"` on every
  changed Go file; `scripts/check-docs.sh --links` and `--ids`, and
  markdownlint, on every changed Markdown file;
  `scripts/check-workflows.sh` and `actionlint` on a changed workflow;
  `make gate` ending `overall=0`. A new Go file changes
  `docs/architecture.md`'s counts in the same commit.
* **Seen failing first:** every new test or assertion fails on a
  `scripts/plant-copy.sh` copy with one planted break, never by dirtying
  the tree. The record quotes the failing line.
* **The Windows test host:** probes and test binaries cross-compiled here
  with `CGO_ENABLED=0 GOOS=windows GOARCH=amd64`, copied to a new
  directory under its temporary directory, run from its ssh shell, which
  is elevated (0021-PLAN Deviation D1), and cleaned up afterwards. Every
  task is named `selfupdate-probe-<random>` or `selfupdate-live-<random>`
  and deleted when the run ends, pass or fail.
* **An interactive-token task runs only while the account is signed in
  at the console** (0021-PLAN rule 6). Before U1 and U2's host runs, the
  agent asks the owner to confirm the sign-in.
* **CI output in records:** quoted verbatim, except that an account or
  host name a runner prints is replaced with a placeholder.
* **Identifiers:** nothing committed names a host, an account or a real
  absolute path.

## Implementation Steps

### Phase U0: records

1. `HEAD` is `a64e257` or a records-only descendant, on `main`.
2. Amendment A5 records the owner's decision ("for the open options,
   follow recommendations"): E1-A and E2-A.
3. `docs/README.md` gains this plan's row after
   `0021-PLAN-task-scheduler-lifecycle-and-windowless-children.md`'s.
4. On approval of this plan: `in-progress`, with `### Approval` quoting
   the owner.
5. Checks; commit.

### Phase U1: the instance probes (E1-A)

Nothing in the tree changes; the probes live in the scratchpad. They use
the `Schedule.Service` COM object from PowerShell, through `IDispatch`;
the module calls the same methods through their vtables. The record says
so.

1. **A held instance, three runs.** Register `selfupdate-probe-<random>`
   (`Interactive`, `Limited`, no trigger), whose action is a Go sleeper
   that exits after 3 s. Run it; poll `GetInstances(0)` until it lists
   one instance; keep that `IRunningTask`, and record its `EnginePID`.
   Wait until that process has exited, then 2 s more. Then record, for
   each call, its value or its HRESULT: the held instance's `EnginePID`,
   `State` and `Refresh()`; a fresh `GetInstances(0)`'s `Count`; and, if
   that lists an instance, its `EnginePID`.
2. **The race.** A second task whose sleeper exits at once. Run it 50
   times; after each `Run`, for 2 s, call `GetInstances(0)` and each
   listed instance's `EnginePID` in a loop. Record the counts of each
   outcome: `GetInstances` `S_OK` or its HRESULT; `EnginePID` a PID, `0`,
   or its HRESULT.
3. **Branches,** decided by A5:
   * an ended instance's `EnginePID` returns `0x8004130B`, in step 1 or
     step 2: U2 as written;
   * it returns a value with no error, and step 2 shows no `0x8004130B`
     from `EnginePID`: U2, U5 and U6 are not done, and the record says
     why;
   * any other failure: stop, and put it to the owner.
4. Host cleanup: tasks deleted, the run directory removed, and
   `schtasks /query` showing no `selfupdate-` task.

### Phase U2: an ended instance is skipped (E1-A, if U1 calls for it)

1. **`scheduler.go`** (no build constraint):

   ```go
   // livePIDs collects the engine PIDs of n listed instances. An instance
   // that ended after it was listed returns SCHED_E_TASK_NOT_RUNNING and
   // is skipped; a PID of 0 is not a process (0021-MADR Amendment A5).
   func livePIDs(n int32, enginePID func(i int32) (uint32, error)) ([]uint32, error)
   ```

   Items are 1-based, as `IRunningTaskCollection::get_Item` is. Any other
   error is returned.
2. **`scheduler_windows.go`:** `sysReg.instances` keeps its
   `GetInstances` call and mapping, and passes `livePIDs` a function that
   gets item `i` and its `EnginePID` and releases the item. An error from
   either call is that function's error.
3. **`api.go`:** `hrTaskNotRunning`'s comment says where it was measured
   (U1), and that `GetInstances`'s mapping is kept unmeasured.
4. **Tests,** in a new `scheduler_test.go` (no build constraint):
   `TestLivePIDs`, over a fake `enginePID`: every instance alive; the
   second ended (`hrTaskNotRunning`), skipped, no error; another HRESULT,
   returned; a PID of 0, dropped; `n` of 0. `docs/architecture.md`'s
   `selfupdate/service/taskscheduler` row counts the new file.
5. **Seen failing:** a copy whose `livePIDs` returns every error, as
   `instances` does today: `TestLivePIDs` fails on the ended case. The
   record quotes the line.
6. **On the Windows test host,** the console sign-in confirmed: the
   package's tests cross-compiled, then `SELFUPDATE_REQUIRE_TASKSCHED=1
   -test.run '^TestLiveTask'`: `PASS`.
7. Checks; commit.

### Phase U3: the CI probe (E2-A)

1. **`.github/workflows/ci.yml`,** in the `validate` job, Windows only:
   * a step before "Task Scheduler live test", "Task Scheduler session
     probe", under `pwsh`, that prints `[Environment]::UserInteractive`,
     the job process's session ID, the `Mandatory Label` line of
     `whoami /groups`, and how many `explorer` processes run in that
     session;
   * "Task Scheduler live test" gains `SELFUPDATE_REQUIRE_TASKSCHED: "1"`
     and `continue-on-error: true`, and its comment cites A5 E2-A and
     says the step is a probe.
2. `scripts/check-workflows.sh` (expressions, permissions, pins) and
   `actionlint`: clean.
3. Checks; commit. The owner pushes.
4. **The run:** the probe step's output and each live test's result,
   verbatim, per Rules; the job's conclusion.

### Phase U4: CI requires the tests, or skips them truthfully (E2-A)

Decided by A5, by U3's run:

* **Every live test passed:** the step keeps the variable, loses
  `continue-on-error`, and its comment says CI requires the tests. The
  session probe step stays, so a later failure shows the session it ran
  in. `requireLive`'s skip message no longer says GitHub runners have no
  console session; it says CI sets the variable.
* **Any live test failed:** the step loses the variable and
  `continue-on-error`, and its comment and `requireLive`'s skip message
  name what U3 found. The probe step stays.

Then: `docs/architecture.md`'s CI bullets say which. Checks; commit; the
owner pushes; CI green with the step as decided.

### Phase U5: the migration guide (if U2 landed)

The guide's versions move to `v1.15.2`, and a "From v1.15.1 to v1.15.2"
section says what changed for a program: `Running`, `Stop`, `Start` and
`HandOffIfInside` no longer fail when a task's instance ends while they
list it. No API changes. Checks; commit.

### Phase U6: `v1.15.2` (if U2 landed)

[0017-PLAN-verify-build-provenance-and-close-0015-open-items.md](0017-PLAN-verify-build-provenance-and-close-0015-open-items.md)'s
"Release procedure", as
[0022-PLAN-error-chain-and-process-liveness.md](0022-PLAN-error-chain-and-process-liveness.md)
Q5 ran it:

1. CI on `main` at U5's commit is green; the commits are pushed first,
   on the owner's ask.
2. The tag, on the owner's ask in that turn.
3. The tag's CI and its identity legs.
4. The pin commit: `git grep -n 'v1\.15\.1\|fa74db4' -- README.md
   docs/architecture.md docs/guides`; each changed line named; lines that
   state what `v1.15.1` itself changed are kept.
5. The proxy serves `v1.15.2`, and `@latest` resolves to it.
6. Release notes in this plan.

### Phase U7: closing

1. `### Verification, at closing`, V1 to V5 with their evidence.
2. Amendment A4's `0x8004130B` paragraph, A5's [derived] and
   [unverified] findings, and "Not verified"'s runner line gain italic
   annotations with what was measured. Their text is otherwise kept.
3. `### Closing`, `status: complete`, the index row. Commit, then push on
   the owner's ask.

## Verification

* **V1 (measured).** U1's three held-instance runs and the race counts
  are recorded verbatim, and the branch taken follows from them.
* **V2 (fixed).** If U2 ran: `TestLivePIDs` failed on its plant and
  passes on every OS; the live suite passed on the Windows test host. If
  not, the record says why, from V1.
* **V3 (CI probed).** U3's run is recorded: the session facts and each
  live test's result.
* **V4 (CI settled).** CI requires the live tests and passes, or skips
  them with a message that names U3's finding. No message or comment
  states what was not observed.
* **V5 (released).** If U2 ran: `v1.15.2` is tagged, its CI passed, and
  the proxy serves it.

## Rollout and Rollback

* **Rollout:** one commit per phase, on `main`, pushed on the owner's
  ask; `v1.15.2` only if U2 changed the transport.
* **Rollback:** `git revert` of U2 restores today's `instances`; of U3 or
  U4, today's skipping step. A released `v1.15.2` is not withdrawn; a
  fault in it is fixed forward.
* **A deviation in any phase** stops it.

## Execution record

### Approval (2026-10-12)

The owner committed Amendment A5's decision, this plan and the index row
as `b3b12f8`, pushed, and said "committed and pushed, proceed. i am
logged into the windows host." This plan is `in-progress`, here and in
the index.

### U1 (2026-10-12)

**The probe.** `instprobe.ps1` in the scratchpad, run once from the
elevated ssh shell, with the scratch Go sleeper from
[0022-PLAN-error-chain-and-process-liveness.md](0022-PLAN-error-chain-and-process-liveness.md)
Q1 (`sleep <n>`) as each task's action; the files' SHA-256 checked on
the host. It reached Task Scheduler through the `Schedule.Service`
object's `IDispatch`; the module calls the same methods through their
vtables. Each task was `selfupdate-probe-<random>`, `Interactive`,
`Limited`, no trigger. The script recorded `console_user_present=True
explorer_sessions=1`. For each call it records `ok:<value>`, or
`hr:<HRESULT>` of the innermost exception.

**Step 1, a held instance** (a sleeper of 3 s; the instance kept from
`GetInstances(0)`; the PID it gave shown as `<same pid>`):

```text
held run=1 pid_while_running=ok state_while_running=ok:4 program_exited=True | after_exit: EnginePID=ok:<same pid> State=ok:4 Refresh=hr:0x8004130B EnginePID_after_refresh=ok:0 fresh_GetInstances_Count=ok:0 fresh_item_EnginePID=none
held run=2 pid_while_running=ok state_while_running=ok:4 program_exited=True | after_exit: EnginePID=ok:<same pid> State=ok:4 Refresh=hr:0x8004130B EnginePID_after_refresh=ok:0 fresh_GetInstances_Count=ok:0 fresh_item_EnginePID=none
held run=3 pid_while_running=ok state_while_running=ok:4 program_exited=True | after_exit: EnginePID=ok:<same pid> State=ok:4 Refresh=hr:0x8004130B EnginePID_after_refresh=ok:0 fresh_GetInstances_Count=ok:0 fresh_item_EnginePID=none
```

(`State` 4 is `TASK_STATE_RUNNING`.)

**Step 2, the race** (a sleeper of 0 s, 50 runs, 2 s of polling after
each):

```text
race runs=50, 2 s of polling each:
  EnginePID=ok:<pid> x105
  GetInstances=ok listed=0 x106861
  GetInstances=ok listed=1 x105
```

**Reading:**

* **`0x8004130B` arises from `IRunningTask::Refresh` on an instance whose
  program has exited** [measured], as the wrapper's issues reported.
* **An `IRunningTask`'s properties are a snapshot.** After the program
  exited, the held instance still gave its PID and `State` 4 with no
  error; only `Refresh` asked Task Scheduler again. After the failed
  `Refresh`, `EnginePID` read 0 [measured].
* **`get_EnginePID` did not return `0x8004130B`** in any read: three per
  held run, and 105 in the race. `GetInstances` did not return it in any
  call: 106,966 in the race, idle, during a run, or just after one, and
  every call in step 1. The module calls `GetInstances` and
  `get_EnginePID`, and never `Refresh`.
* **A5's [derived] gap does not arise:** an ended instance does not fail
  `get_EnginePID`; it gives the PID it had, which `Stop`'s wait and
  `Start`'s `current` already check by process and creation time
  (Amendment A4; 0022-MADR Amendment A2).
* **A5's hypothesis** that `026faaa` saw the code from the per-instance
  call is not supported. Where `026faaa` saw it stays [unverified].

**Branch (step 3):** the second. An ended instance's `EnginePID` returns
a value with no error, and step 2 shows no `0x8004130B` from it. U2, U5
and U6 are not done: the transport's behaviour is right for every call it
makes, and no release follows. The `GetInstances` mapping and its comment
stay as they are; U7 records the measurement in the MADR.

**Host cleanup.** The script printed `cleanup.task=removed` twice and
`dir-removed`; afterwards `schtasks /query` lists 0 `selfupdate-` tasks.

**Checks.** Records only: `scripts/check-docs.sh --links` and `--ids` on
this plan and `docs/README.md`, and markdownlint on `docs/README.md`.

### U3 (2026-10-12)

The owner committed U1 as `68b8fa3`, and said "committed and pushed,
proceed". A fetch showed `origin/main` at `b3b12f8`, so `68b8fa3` goes
out with this phase's push.

**Files.** Changed: `.github/workflows/ci.yml`.

**As built:**

* **"Task Scheduler session probe",** a new step before the live test,
  Windows only, under `pwsh`. It prints `user_interactive=`,
  `session_id=`, `explorer_in_session=` (Explorer processes in the job's
  session) and `integrity=`, the `Mandatory Label` line of `whoami
  /groups`. It calls `whoami.exe` by its System32 path: on the Windows
  test host, Git's `whoami` came first on the path and refused
  `/groups`, so the line was empty.
* **"Task Scheduler live test"** gains `continue-on-error: true` and
  `SELFUPDATE_REQUIRE_TASKSCHED: "1"`. Its comment says it is a probe,
  and cites Amendment A5, E2-A.

**The probe step, run on the Windows test host** from the elevated ssh
shell, its script taken from the workflow file: exit 0, and

```text
user_interactive=False
session_id=0
explorer_in_session=0
integrity=Mandatory Label\High Mandatory Level Label S-1-16-12288
```

An ssh shell runs in session 0 with no desktop, so this shows the step
works, not what a runner will print.

**Checks.** `scripts/check-workflows.sh --rule all` on `ci.yml`:
`check-workflows: ok (all) — 1 workflow(s)`. `actionlint`: clean. `make gate` on this Mac,
load average 3.76 before and 60.03 after: every step `rc=0`, `overall=0`.
