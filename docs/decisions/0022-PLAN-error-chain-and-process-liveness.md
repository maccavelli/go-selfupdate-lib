---
status: complete
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

### Q3 (2026-10-11)

The owner committed Q2 as `9895966` and said "proceed".

**Files.** Changed, tests only: `selfupdate/spawn_test.go`,
`selfupdate/service/handoff_test.go`,
`selfupdate/verify/ghattest/ghattest_test.go`,
`selfupdate/codesign/codesign_test.go`, `selfupdate/cli/handoff_test.go`.
No file is added, so the architecture table's counts do not change.

**As built, against step 1's table:**

| Entry point | Test | Fake |
| :--- | :--- | :--- |
| `Updater.Run`, staged probe | `TestBatchErrorReachesCallers/staged_probe` | a `ProberFunc` in the updater's probes |
| `Updater.Run`, manifest verifier | `…/manifest_verifier` | a `ManifestVerifierFunc` in `Config.ManifestVerifiers` |
| an install session's post-install probe | `…/post-install_probe` | `InstallOptions.PostInstall` |
| a managed install's reconcile | `…/managed_reconcile` | a `Reconciler` (`ranRec`) under `NewManagedInstaller` |
| `ExecReconciler.Reconcile` and `Restore` | `TestBatchErrorReachesServiceCallers` | `ExecOptions.Runner` (`refusingRunner`) |
| `HandOffIfInside` and `HandOffFunc` | the same | a `Detacher` (`refusingDetacher`) |
| `ghattest`'s manifest verifier and verifier | `TestBatchErrorReachesCaller` (ghattest) | `Options.Runner` |
| `codesign`'s checker and signer | `TestBatchErrorReachesCaller` (codesign) | `CheckOptions.Runner`, `SignOptions.Runner` |
| `cli.Run`, the handoff failing | `TestBatchErrorReachesCaller` (cli) | `HandOff.Detach` |

* The managed install is held at two layers: in `selfupdate`, through a
  fake `Reconciler`, and in `service`, through `ExecReconciler` itself.
  Step 1 named only the second, as one case.
* Each fake returns `fmt.Errorf("fake: %w", selfupdate.ErrBatchFile)`,
  the exported name, where step 1 said `winproc.ErrBatchFile`: they are
  one value (`TestErrBatchFileIsWinproc`), and the exported name keeps
  `internal/winproc` out of four more packages' tests.
* Each case asserts its fake ran, so a case that never reaches the fake
  fails instead of passing.
* `TestBatchErrorChain`'s doc says the walk stops at a method or a
  function literal, that every method on the chain is reached through an
  interface, and names these tests as the hold past it.

**Fail first, on this Mac,** each a `scripts/plant-copy.sh` copy with one
layer's `%w` broken, then every chain test of that package run:

| Plant | Fails |
| :--- | :--- |
| `runProbes` `%w` → `%v` | `TestBatchErrorReachesCallers/staged_probe` alone: `spawn_test.go:554` |
| `runManifestVerifiers` joins `errors.New(err.Error())` | `…/manifest_verifier` alone: `spawn_test.go:575: … manifest verification failed: selfupdate: integrity check failed` |
| `probeInstalled` `%w` → `%v` | `…/post-install_probe` alone: `spawn_test.go:581` |
| managed `recover` joins `errors.New(origin.Error())` | `…/managed_reconcile` alone: `spawn_test.go:604` |
| `ExecReconciler.Reconcile` `%w` → `%v` | its line alone: `handoff_test.go:227` |
| `ExecReconciler.Restore` `%w` → `%v` | its line alone: `handoff_test.go:230` |
| `HandOffIfInside` `%w` → `%v` | `handoff_test.go:234` and `:237`: `HandOffFunc` calls `HandOffIfInside`, the same layer |
| `ghattest`'s `running gh` `%w` → `%v` | both verifiers, `ghattest_test.go:433` and `:448`: they share that wrap |
| `codesign`'s `run` `%w` → `%v` | the checker and the signer, `codesign_test.go:323` and `:329`: they share `tool.run` |
| `cli.Run` joins `errors.New(err.Error())` for a failed handoff | `handoff_test.go:172: Detach ran true, exit 1, err fake: …; want … ErrBatchFile` |

No plant failed another layer's case.

**Checks.** `make pre-add-check FILES="<the 5 test files>"`: its first run
failed gofmt on a trailing blank line in `service/handoff_test.go`,
fixed with `gofmt -w`; then `go-precheck: 5 file(s) clean`. `make gate`:
every step `rc=0`, `apicheck` `compatible with v1.15.0`, `overall=0`.
The cases need no Windows host: they run on every OS, CI's three
included.

### Q4 (2026-10-11)

The owner committed Q3 as `d40b429`, and said "proceed".

* `docs/architecture.md`: the `internal/winproc` bullet names `Alive`, the
  one check of whether a process still runs, with Amendment A2.
* The migration guide: `### From v1.15.0 to v1.15.1` in §15, before its
  `### Check`, in the earlier patch parts' form: what `Stop`, `Start` and
  the hop wait read now, the tests, and no API change. §2's list of
  releases gains `v1.15.1`. The guide's pins move with Q5's pin commit.
* **Checks.** `scripts/check-docs.sh --links` `60 links in 2 files, 0
  broken`; `--ids` `2 files, 16 deny-list rules, 0 findings`; markdownlint
  `0 issues`; `--arch` `0 differ`. `make gate`: every step `rc=0`,
  `overall=0`.

### Q5, `v1.15.1` (2026-10-11)

* **The release commit** is `fa74db4b292baaa14df6e057886102cc712bbeeb`,
  Q4's, which the owner committed and pushed.
* **Step 1:** CI run 38162672238 on `main` at `fa74db4`: `success`, 17 of
  17 jobs.
* **Step 2, the tag,** made and pushed by the owner. Verified:
  * `v1.15.1` is an annotated tag (`git cat-file -t` gives `tag`), object
    `82c83f7aefb5b7fae2522e4abdead245942f45a3`, peeled to `fa74db4`;
  * `git ls-remote origin` gives `fa74db4…` for `refs/tags/v1.15.1^{}`
    and for `refs/heads/main`;
  * `scripts/check-release-tag.sh v1.15.1` exits 0.
* **Step 3:** the tag's CI run, 38163532071, passed all 17 jobs; all ten
  identity legs print `v1.15.1 (release) fa74db4b292b`.
* **Step 4, the pin commit.**
  * **Candidates:** `git grep -n 'v1\.15\.0\|ac4ef55' -- README.md
    docs/architecture.md docs/guides`, 46 lines.
  * **Preconditions:** `git diff v1.15.0 v1.15.1` of
    `.github/workflows/`, `internal/cmd/`, `scripts/` and
    `selfupdate/archive/` is empty. So "`v1.11.0` to `v1.15.1` change
    nothing in it" holds, and the publish workflow's archive check is as
    at `v1.15.0`.
  * **Changed,** 36 lines, to `fa74db4b… # v1.15.1` or `v1.15.1`, by a
    script that changed only the listed lines and asserted each one
    changed: `README.md:24`, `:28`, `:126`; `docs/architecture.md:23-24`;
    the building guide `:26`, `:164`, `:175`, `:186`, `:261`; the migration
    guide `:23`, `:26`, `:98`, `:116`, `:118`, `:126`, `:172`, ten
    `go get` lines (`:391`, `:475`, `:519`, `:571`, `:616`, `:723`,
    `:837`, `:930`, `:975`, `:1032`) and nine `go list` checks (`:514`,
    `:566`, `:610`, `:715`, `:831`, `:925`, `:970`, `:1027`, `:1095`).
  * **Kept,** 10 lines that state what `v1.15.0` itself did, or name the
    patch part: the extending guide `:557`, `:561`, `:611`, `:617`; the
    migration guide `:44`, `:47`, `:1006`, `:1035`, `:1073`, `:1076`.
* **Step 5:** `GOPROXY=https://proxy.golang.org GOFLAGS=-mod=mod go list
  -m -json github.com/maccavelli/go-selfupdate-lib@v1.15.1` gives
  `v1.15.1`, `Time` `2026-10-11T18:09:09Z`, `Origin.Hash` the release
  commit and `Ref` `refs/tags/v1.15.1`; `@latest` resolves to `v1.15.1`.
* **Checks on the pin commit:** see "Checks on the pin commit" below the
  release notes.

### Release notes for v1.15.1 (2026-10-11)

`v1.15.1` fixes how the library reads whether a Windows process has
exited, and adds tests. `make apicheck` reports it compatible with
`v1.15.0`, with no exported change.

* **Task Scheduler reads an exited program as gone.** `Stop`'s wait and
  `Start`'s check of a stale instance compared the process's creation
  time, which Windows keeps while any process holds a handle to it, so a
  handle held by a monitor or an antivirus could make `Stop` time out
  with `ErrTimeout` or `Start` skip its run. Both now wait on the process
  object, which Windows signals at the exit (measured on a Windows host).
  The handoff's wait for its hop uses the same check,
  `internal/winproc.Alive`.
* **Tests:** a case at each exported entry point, on every OS, holds
  `errors.Is(err, selfupdate.ErrBatchFile)` past the interfaces that the
  static chain test cannot follow.
* **Records:**
  [0022-MADR-documentation-pass-after-v1-14-0.md](0022-MADR-documentation-pass-after-v1-14-0.md)
  Amendment A2, and this plan.

### Checks on the pin commit (2026-10-11)

* On the five changed files, on this Mac: `scripts/check-docs.sh --links`
  `96 links in 5 files, 0 broken`; `--ids` `5 files, 16 deny-list rules,
  0 findings`; markdownlint `0 issues`.
* **`make gate` on this Mac failed in `race` and `shuffle`,** on timeouts
  in tests that build and start a fresh binary:
  `TestE2EUpdateRunningCopyOnChannel` and `TestE2ERunningCopyRefusals`
  (`the running copy never signalled ready`, 10 s), and `TestIdentity`
  (`version did not finish within 5s`). They failed again run alone.
  * The Mac was saturated: load average 54 to 77 on 10 cores, with
    another workload's `go test` binaries running (none of them this
    repository's packages) beside a management daemon and Spotlight
    indexing. A fresh hello-world binary took 0.86 s and 1.52 s on its
    first start, and 0.05 s after.
  * The Go code is `fa74db4`'s (`git diff --stat fa74db4 -- '*.go'` is
    empty), where CI passed these tests in runs 38162672238 and
    38163532071, 17 of 17 jobs each.
  * The other workload's processes were left alone.
* **`make gate` on a Linux test host,** on the owner's word ("the …
  host is idle. you can ssh to it"):
  * a clone of the public repository at `fa74db4`, with this commit's
    diff applied; the five files' SHA-256 matched this Mac's, and
    `git status --short` listed those five alone;
  * `GATE_SKIP=ids`: the host has no deny list. The step reads only
    text, and passed on this Mac on the same files (above);
  * the host's `TMPDIR` holds an empty `.git` directory, so every build
    in a temporary directory failed to read its VCS status (exit 128),
    and its `BASH_ENV` resets `TMPDIR` in every bash script, so setting
    it alone did not reach `go test`. The run that counts used `env -u
    BASH_ENV` and a `TMPDIR` beside the clone. Two earlier runs, without
    that, failed only in the `archive` tests and the fuzz step that lists
    them, on that VCS error; they are not evidence either way;
  * the result: every step `rc=0` but `ids skipped`; `race` 15 packages
    `ok`, 0 `FAIL`; `apicheck` `compatible with v1.15.1`; `arch` `0
    differ`; `overall=0`.
  * The run directory was removed afterwards; the host's `.git` and
    `BASH_ENV` were not touched.

### Q6: Verification, at closing (2026-10-11)

The owner committed the pin commit as `2cadf13`. It is not pushed yet, so
CI has not run on it; it changes Markdown only.

* **V1 (measured): met.** Q1's outputs are recorded verbatim. A process
  that has exited, with a handle held, still opens and keeps its creation
  time, and is signalled; the snapshot drops it. Task Scheduler's engine
  held no handle past the exit. Both [unverified] facts in Amendment A2
  carry these answers.
* **V2 (liveness): met.** `TestAlive` failed on both plants on the
  Windows test host (no wait: `want both false`; `born` ignored: `a reused
  ID`), and passes there. CI's Windows job ran `internal/winproc` at
  `fa74db4` (runs 38162672238 and 38163532071). The Task Scheduler live
  tests passed `-count=3` on the host at Q2.
* **V3 (chain): met.** Every case in Q3's table passes on every OS, in
  CI's three jobs and on this Mac; each of the ten planted `%v` layers
  failed its own case and no other.
* **V4 (no API change): met.** `apidiff`'s full report of the tree
  against `v1.15.0`, at Q2, was empty; `make apicheck` reported
  `compatible with v1.15.0` at every phase.
* **V5 (release): met.** `v1.15.1` is an annotated tag on `fa74db4`; its
  CI (38163532071) passed 17 of 17 jobs with ten identity legs; the proxy
  serves it, and `@latest` resolves to it. The pins name it in `2cadf13`,
  whose `make gate` passed on a Linux test host (Q5's checks).

### Closing (2026-10-11)

Every phase, Q0 to Q6, is recorded above, and V1 to V5 are met. This plan
is `complete`, here and in the index; Amendment A2 stands as decided.

**Open at closing:**

* **A medium-integrity caller's `SYNCHRONIZE` open** of a task's program
  is [derived], not measured: every run was from the elevated ssh shell
  (0021-PLAN Deviation D1). `Alive` falls back to the snapshot if it is
  refused.
* **`make gate` on this Mac** could not pass under the load recorded in
  Q5; the Linux test host's run stands for it. Nothing in the tree
  explains the timeouts.
* **From 0021, unchanged:** where `0x8004130B` arises, and whether a
  GitHub runner can run an interactive-token task.
* **CI on `2cadf13`** runs once it is pushed.

**Checks.** On this plan and `docs/README.md`: `scripts/check-docs.sh
--links` `95 links in 2 files, 0 broken`; `--ids` `2 files, 16 deny-list
rules, 0 findings`; markdownlint `0 issues` on `docs/README.md`. This
Mac's load average was still about 50, so `make gate` ran on the Linux
test host as in Q5: a clone at `fa74db4` with the diff from `fa74db4`
applied (the pin commit and this record; the six files' SHA-256
matched), `env -u BASH_ENV`, a clean `TMPDIR`, `GATE_SKIP=ids`. Every
other step `rc=0`, `overall=0`. The run directory was removed.
