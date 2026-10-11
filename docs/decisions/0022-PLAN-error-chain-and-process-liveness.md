---
status: in-progress
date: 2026-10-11
associated-madr: "0022-MADR-documentation-pass-after-v1-14-0.md"
---
# Implement the error-chain tests past interfaces and a process check that sees an exit

Associated MADR: [0022-MADR-documentation-pass-after-v1-14-0.md](0022-MADR-documentation-pass-after-v1-14-0.md),
Amendment A2. The first plan for that record is
[0022-PLAN-documentation-pass-after-v1-14-0.md](0022-PLAN-documentation-pass-after-v1-14-0.md),
complete.

## Goal

* **`errors.Is(err, selfupdate.ErrBatchFile)` is held where a program
  reads the error,** at every exported entry point, on every OS (MADR
  D5-A).
* **A process that has exited reads as gone,** whoever still holds a
  handle to it: one check, `winproc.Alive`, in the Task Scheduler
  backend, the handoff's hop wait and the live tests (D6-A).
* **`v1.15.1`** carries the backend fix.

## Scope

### In scope

| Phase | What | Commit |
| :--- | :--- | :--- |
| Q0 | this plan, Amendment A2 and the index row | records only |
| Q1 | measurements on the Windows test host | records only |
| Q2 | `winproc.Alive`, its callers, its tests | code and tests |
| Q3 | the behavioural chain tests, and `TestBatchErrorChain`'s doc | tests |
| Q4 | the architecture page and the migration guide's `v1.15.1` part | docs |
| Q5 | `v1.15.1`: CI, the tag, the pins | per 0017-PLAN's procedure |
| Q6 | closing | records only |

### Out of scope

* **A type-aware or call-graph walk** (D5-B, D5-C).
* **`scm`'s `created`,** which feeds the ancestor walk only
  (`winproc.Descends`), never a liveness decision.
* **Exported API.** `winproc` is internal; no exported name changes, so
  `apicheck` stays compatible with nothing added.
* **Push and tags,** which happen only on the owner's ask in the same turn.

## Rules

* **`main`, no branch.** The owner commits and pushes on `main`. An agent
  commits only on an explicit "commit to main" in that turn, with
  `git commit --no-edit`.
* **Each phase stops at a deviation** (AGENTS.md, Plan deviations), and
  Q1 at the stop condition below.
* **Checks before each commit:** `make pre-add-check FILES="…"` on every
  changed Go file; `scripts/check-docs.sh --links` and `--ids`, and
  markdownlint, on every changed Markdown file; `make gate` ending
  `overall=0`. A new test file changes `docs/architecture.md`'s count in
  the same commit, or the `arch` step fails.
* **Seen failing first:** every new test or assertion fails on a
  `scripts/plant-copy.sh` copy with one planted break, never by dirtying
  the tree. The record quotes the failing line.
* **The Windows test host:** binaries cross-compiled here with
  `CGO_ENABLED=0 GOOS=windows GOARCH=amd64`, copied to a new directory
  under its temporary directory, run from its ssh shell, which is
  elevated (0021-PLAN Deviation D1), with the console sign-in measured for
  Task Scheduler runs, and cleaned up afterwards.
* **Identifiers:** nothing committed names a host, an account or a real
  absolute path.

## Implementation Steps

### Phase Q0: records

1. `HEAD` is `78dde39` or a records-only descendant, on `main`.
2. `docs/README.md` gains this plan's row after
   `0022-PLAN-documentation-pass-after-v1-14-0.md`'s.
3. On approval: this plan becomes `in-progress` with `### Approval`
   quoting the owner; Amendment A2's heading loses "proposed", and it
   records the owner's words.
4. Checks; commit.

### Phase Q1: measurements on the Windows test host

Nothing in the tree changes; the probes live in the scratchpad.

1. **An exited process with a handle held.** A probe, `aliveprobe.exe`
   (standard library and `golang.org/x/sys/windows`), starts a child
   that sleeps 2 s and exits, and keeps a handle to it. Before and after
   the exit, it records, for the child's PID:
   * `OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION)`: opens or not;
   * `GetProcessTimes`: the creation time, equal to the first reading or
     not (today's `running(pid, born)`);
   * whether the Toolhelp snapshot lists the PID (today's
     `running(pid, 0)`);
   * `OpenProcess(SYNCHRONIZE | PROCESS_QUERY_LIMITED_INFORMATION)` and
     `WaitForSingleObject(h, 0)`: `WAIT_TIMEOUT` or `WAIT_OBJECT_0`;
   * `GetExitCodeProcess`.

   Then it closes its handle and records the same once more.
2. **Task Scheduler's engine.** From the ssh shell: register a throwaway
   `selfupdate-probe-<random>` task (`InteractiveToken`,
   `LeastPrivilege`, one `Exec` action that sleeps), run it, record its
   instance PID and creation time, end it with `schtasks /end`, and
   sample every 50 ms for 10 s: the creation-time check, the snapshot,
   and the wait. The record gives, for each check, how long after the end
   it still read "running".
3. **Stop condition.** If a process that exited signals no
   `WAIT_OBJECT_0`, or the `SYNCHRONIZE` open of the task's program is
   refused, D6-A as decided does not hold: stop, and put the measurement
   to the owner.
4. The MADR's two [unverified] facts gain their answers; the outputs go
   in this plan's record verbatim, redacted per Rules. Host cleanup is
   recorded.

### Phase Q2: `winproc.Alive` (D6-A)

1. `internal/winproc/alive_windows.go`, or `winproc_windows.go` if it
   fits the file's purpose, gains:

   ```go
   // Alive reports whether process pid runs, and, when born is not 0,
   // is the one created at born, not a later process with a reused ID.
   // A process that has exited is not alive, though another process
   // still holds a handle to it (0022-MADR Amendment A2).
   func Alive(pid uint32, born uint64) bool
   ```

   It opens with `SYNCHRONIZE | PROCESS_QUERY_LIMITED_INFORMATION`. No
   such process: false. `ERROR_ACCESS_DENIED`: the Toolhelp snapshot
   decides, as `running(pid, 0)` does today. `born` compared by
   `GetProcessTimes`. Alive only while `WaitForSingleObject(h, 0)` returns
   `WAIT_TIMEOUT`.
2. Callers:
   * `taskscheduler`'s `sysSched.running(pid, born)` returns
     `winproc.Alive(pid, born)`; its snapshot branch moves into `Alive`'s
     fallback;
   * `service.processRunning` calls `winproc.Alive(uint32(pid), 0)`;
   * the live tests' `pidRunning` is removed, and its log line uses
     `s.s.running`.
3. **Tests,** in `internal/winproc`'s Windows test file:
   * a child that runs: alive;
   * the same child after it exits, while the test holds a handle to it:
     not alive;
   * a running child checked with a `born` one tick off: not alive;
   * a PID with no process: not alive.
4. **Seen failing,** on the Windows test host:
   * `Alive` without the wait (today's open-and-compare): the
     exited-with-handle case fails;
   * `Alive` ignoring `born`: the reused-ID case fails.
5. The `taskscheduler` live tests, and `TestCOMLayout`, pass `-count=3`;
   `service`'s hop tests pass; `internal/winproc` passes.
6. Checks; commit.

### Phase Q3: the chain past interfaces (D5-A)

1. A case for each exported entry point, each with a fake at the
   interface that returns `fmt.Errorf("fake: %w", winproc.ErrBatchFile)`
   and records that it was called. Each asserts the fake ran and
   `errors.Is(err, selfupdate.ErrBatchFile)` on what the program
   receives:

   | Entry point | Fake | Package of the test |
   | :--- | :--- | :--- |
   | `Updater.Run`, staged probe | `Config.Probes` | `selfupdate` |
   | `Updater.Run`, manifest verifier | `Config.ManifestVerifiers` | `selfupdate` |
   | `Install`, post-install probe | `InstallOptions.PostInstall` | `selfupdate` |
   | managed install, `ExecReconciler` | `service.ExecOptions.Runner` | `selfupdate/service` |
   | `ghattest` verifier | `Options.Runner` | `selfupdate/verify/ghattest` |
   | `codesign` checker and signer | `CheckOptions.Runner`, `SignOptions.Runner` | `selfupdate/codesign` |
   | `HandOffIfInside` | a `service.Detacher` | `selfupdate/service` |
   | `cli.Run`'s error, the handoff failing | `HandOff.Detach` | `selfupdate/cli` |

   Each package's existing test helpers and fixtures are reused; where a
   case needs a release to reach the step, the fixtures that package's
   tests already use serve.
2. `TestBatchErrorChain`'s doc says where it stops, and names the test
   that holds the chain past it.
3. **Seen failing,** on this Mac, each a `%w` planted to `%v` at one layer
   above the fake: `Updater.runProbes`, `probeInstalled`, `ghattest`'s
   `running gh`, `codesign`'s `run`, `ExecReconciler.Reconcile`, and
   `cli.Run`'s join of the handoff's error (`run.go:143-144`). Each fails its own case and no other.
4. Checks; commit.

### Phase Q4: documentation

1. `docs/architecture.md`: the `internal/winproc` bullet names `Alive`.
2. The migration guide's §15 gains `### From v1.15.0 to v1.15.1`, in the
   form of the earlier patch parts: what `Stop` and `Start` read now, and
   that no API changes.
3. Checks; commit.

### Phase Q5: `v1.15.1`

0017-PLAN's "Release procedure" and 0022's first plan's P7:

1. CI on `main` at Q4's commit is green; the commits are pushed first, on
   the owner's ask.
2. The tag, on the owner's ask in that turn.
3. The tag's CI and its identity legs, `v1.15.1 (release) <12-hex>`.
4. The pin commit: `git grep -n 'v1\.15\.0\|ac4ef55' -- README.md
   docs/architecture.md docs/guides`; each changed line named; lines that
   state what `v1.15.0` itself changed are kept.
5. The proxy serves `v1.15.1`, and `@latest` resolves to it.
6. Release notes in this plan.

### Phase Q6: closing

1. `### Verification, at closing`, V1 to V5 with their evidence.
2. `### Closing`, `status: complete`, the index row. Commit, then push on
   the owner's ask.

## Verification

* **V1 (measured).** Q1's outputs are recorded, and the MADR's two
  [unverified] facts are answered.
* **V2 (liveness).** `Alive`'s tests failed on both plants and pass on
  the Windows test host and in CI's Windows job. The live tests pass
  `-count=3`.
* **V3 (chain).** Every case in Q3's table passes on every OS, and each
  failed on its planted `%v`, alone.
* **V4 (no API change).** `make apicheck` against `v1.15.0` reports
  nothing added or changed.
* **V5 (release).** `v1.15.1` is tagged, its CI is green, the pins name
  it, and the proxy serves it.

## Rollout and Rollback

* **Rollout:** each phase is one commit on `main`, pushed on the owner's
  ask. The backend fix reaches programs with `v1.15.1`.
* **Rollback, before the tag:** `git revert` of Q2 restores today's
  check; Q3 is tests only.
* **Rollback, after the tag:** a release cannot be withdrawn; a defect is
  fixed forward in `v1.15.2`.
* **A deviation in any phase** stops it.

## Execution record

### Approval (2026-10-11)

The owner committed Amendment A2, this plan and its index row, and said
"committed, approved, proceed". Amendment A2 records the decision.

### Q0 (2026-10-11)

The owner committed the records as `ea08716`. This plan is `in-progress`,
here and in the index; Amendment A2 records the owner's words.

### Q1 (2026-10-11)

**The probes.** `aliveprobe.exe`, the standard library and
`golang.org/x/sys/windows` v0.47.0, `go vet` clean, cross-compiled on
this Mac; `taskend.ps1`. Both ran from the Windows test host's elevated
ssh shell, in a new `selfupdate-q1-<random>` directory. The
task probe's own lines give the console sign-in.

**Step 1, a child that exits while a handle is held** (`aliveprobe
zombie`), verbatim:

```text
running open_query=ok created_equal=true snapshot=true wait=timeout exitcode=259
exited_handle_held open_query=ok created_equal=true snapshot=false wait=signaled exitcode=0
exited_execs_handle_released_ours_held open_query=ok created_equal=true snapshot=false wait=signaled exitcode=0
exited_all_released open_query=errno87 created_equal=false snapshot=false wait=open:errno87 exitcode=open:errno87
```

**Step 2, Task Scheduler ending a task** (`taskend.ps1`: a throwaway
`selfupdate-probe-<random>` task, `InteractiveToken`, `LeastPrivilege`,
whose action is `aliveprobe sleep 120`; `aliveprobe sample` read the
instance's `EnginePID` every 50 ms for 10 s after `schtasks /end`),
verbatim:

```text
console_user_present=True
explorer_sessions=1
logonui=False
engine_pid_found=True
before_end open_query=ok created_equal=true snapshot=true wait=timeout exitcode=259
schtasks_end_err=<nil> out_bytes=91
last_read_running_after_end created_equal=966.0887ms snapshot=966.0887ms wait_timeout=966.0887ms
after_10s open_query=errno87 created_equal=false snapshot=false wait=open:errno87 exitcode=open:errno87
cleanup.task=removed
```

**Reading it:**

* **Today's `running(pid, born)` is wrong for an exited process with a
  handle held:** the open succeeds and the creation time matches, so it
  reads "running" until the last handle closes. Then the open fails
  with `ERROR_INVALID_PARAMETER` (`errno87`).
* **The snapshot** stops listing a process once it exits, handle or not,
  so `running(pid, 0)` was right.
* **The wait** reads `timeout` while the process runs and `signaled` once
  it has exited, handle or not. `GetExitCodeProcess` reads 259 while it
  runs.
* **Task Scheduler's engine** held no handle past the exit in this run:
  the three checks stopped reading "running" in the same sample, 966 ms
  after `schtasks /end`. The defect is real, and reached when another
  process holds a handle; the engine was not seen to.
* **Stop condition (step 3):** not met. The exited process signals, and
  the `SYNCHRONIZE` open of the task's program succeeded. That open was
  made from the elevated shell (0021-PLAN Deviation D1); from a
  medium-integrity caller of the same user it is [derived] to succeed,
  and `Alive` falls back to the snapshot if it is refused. Q2's live
  tests run the backend from the same shell as before.

**Host cleanup.** Each probe removed its own task. The run directory was
removed; 0 `selfupdate-q1-` directories, 0 `selfupdate-` tasks, 0
`aliveprobe` processes.

The MADR's two [unverified] facts carry these answers.

### Q2 (2026-10-11)

The owner committed Q0 and Q1 as `9e59f0e`, and said "proceed".

**Files.** Changed: `internal/winproc/winproc_windows.go`,
`internal/winproc/winproc_windows_test.go`,
`selfupdate/service/taskscheduler/scheduler_windows.go`,
`selfupdate/service/taskscheduler/live_windows_test.go`,
`selfupdate/service/hopwait_windows.go`. No file is added, so the
architecture table's counts do not change.

**As built:**

* `winproc.Alive(pid uint32, born uint64) bool`, in
  `winproc_windows.go`, as step 1 says: an open with `SYNCHRONIZE |
  PROCESS_QUERY_LIMITED_INFORMATION`; `ERROR_ACCESS_DENIED` falls back to
  the snapshot (`inSnapshot`) without checking `born`; any other open
  error is not alive; a `born` that differs from `GetProcessTimes`'s
  creation time is not alive; alive only on `WAIT_TIMEOUT`. It is
  Windows-only, as every caller is.
* `sysSched.running` is `winproc.Alive(pid, born)`; with `born` 0 it now
  waits on the process rather than reading the snapshot, which Q1 showed
  right for an exited process too. `sysSched.processes` stays, for the
  ancestor walk; `created` stays, for the `born` that `Stop` records.
* `service.processRunning` is `winproc.Alive(uint32(pid), 0)`; its own
  `SYNCHRONIZE` open and wait are gone.
* `pidRunning` is removed; `TestLiveTaskStopHoldsDisabled`'s log line uses
  `s.s.running`.
* `TestAlive` uses a new helper mode, `until-eof`, so the test chooses
  when the child exits, and holds its own handle across the exit.

**Fail first, on the Windows test host,** each a `scripts/plant-copy.sh`
copy, checked to have landed:

| Plant | Failing line |
| :--- | :--- |
| `Alive` without the wait (`return true` after the `born` check) | `winproc_windows_test.go:229: exited child, its handle held: Alive(pid, 0) true, Alive(pid, born) true; want both false` |
| `Alive` ignoring `born` | `winproc_windows_test.go:222: running child with another creation time: alive, want a reused ID to read as gone` |

**The Windows test host,** cross-compiled here, run from the elevated ssh
shell; the console sign-in `console_user_present=True`,
`explorer_sessions=1`, `logonui=False`:

* `internal/winproc`: every test `PASS`, `TestAlive` among them.
* `selfupdate/service`, `-test.run 'Hop|Detach|Batch'`: nine tests
  `PASS`, among them `TestWaitHopExit` and `TestHandOffHopProcess`, which
  read `processRunning`.
* `taskscheduler`, `SELFUPDATE_REQUIRE_TASKSCHED=1 -test.run
  '^TestCOMLayout$|^TestLiveTask' -test.count=3`, exit 0. The first pass,
  verbatim, with the `TestCOMLayout` HRESULT lines left out (unchanged
  from 0021-PLAN T6a) and the PID log lines left out:

```text
--- PASS: TestCOMLayout (0.21s)
    live_windows_test.go:521: the update's integrity RID 0x2000
--- PASS: TestLiveTaskUpdate (5.22s)
--- PASS: TestLiveTaskStopHoldsDisabled (71.93s)
--- PASS: TestLiveTaskRollback (5.45s)
--- PASS: TestLiveTaskRequestStop (1.12s)
    live_windows_test.go:719: Start on the disabled task: selfupdate: taskscheduler: the task is disabled, and Task Scheduler does not run a disabled task: selfupdate: taskscheduler: run \selfupdate-live-d570709f0edf5099: HRESULT 0x80041326
--- PASS: TestLiveTaskStopRefusesDisabled (4.08s)
--- PASS: TestLiveTaskzCleanup (0.90s)
```

  The later passes match.

**Host cleanup.** 0 `selfupdate-q2-` directories, 0 `selfupdate-`
tasks, 0 `.test.exe` or `demo.exe` processes.

**Checks.** `make pre-add-check FILES="<the 5 Go files>"`: `go-precheck: 5
file(s) clean`. `make gate`: every step `rc=0`, `apicheck` `compatible
with v1.15.0`, `overall=0`. The full `apidiff` report against `v1.15.0`
is empty: `winproc` is internal.
