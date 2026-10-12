---
status: in-progress
date: 2026-10-12
associated-madr: "0022-MADR-documentation-pass-after-v1-14-0.md"
---
# Implement the measured and pinned `SYNCHRONIZE` open from an unelevated caller

Associated MADR: [0022-MADR-documentation-pass-after-v1-14-0.md](0022-MADR-documentation-pass-after-v1-14-0.md),
Amendment A3 (D7-B). The record's other plans are
[0022-PLAN-documentation-pass-after-v1-14-0.md](0022-PLAN-documentation-pass-after-v1-14-0.md)
and
[0022-PLAN-error-chain-and-process-liveness.md](0022-PLAN-error-chain-and-process-liveness.md),
both complete.

## Goal

* **Measured:** whether an unelevated caller's `SYNCHRONIZE |
  PROCESS_QUERY_LIMITED_INFORMATION` open of a task's program succeeds,
  in the configuration programs ship: the detached update at medium
  integrity, the task's program at medium integrity.
* **Pinned:** `TestLiveTaskUpdate` fails if that open is refused or the
  wait does not read the program as running.
* **Recorded:** what the same open does when the task's program runs at
  high integrity and the caller does not.

## Scope

### In scope

| Phase | What | Commit |
| :--- | :--- | :--- |
| R0 | this plan, Amendment A3 and the index row | records only |
| R1 | `winproc.AliveAccess`, and the pin in `TestLiveTaskUpdate` | code and tests |
| R2 | the high-integrity pairing, measured with scratch probes | records only |
| R3 | closing | records only |

### Out of scope

* **Any change to `Alive`'s behaviour,** whatever R1 or R2 find. A
  refusal in R1 stops the plan (Rules); a refusal in R2 is recorded, and a
  change it calls for needs its own amendment.
* **Whether an unelevated caller can stop a task that runs with highest
  privileges.** R2 starts and ends its tasks from the elevated shell, and
  measures only the open.
* **The service backends' callers of `Alive`** (`service.processRunning`,
  the hop wait): the caller there is the service's own handoff, not an
  unelevated user's process.
* **A release:** `AliveAccess` is in an internal package, so nothing
  exported changes.
* **Push,** which happens only on the owner's ask in the same turn.

## Rules

* **`main`, no branch.** The owner commits and pushes on `main`. An agent
  commits only on an explicit "commit to main" in that turn, with
  `git commit --no-edit`.
* **Each phase stops at a deviation** (AGENTS.md, Plan deviations), and
  R1 at the stop condition in its step 6.
* **Checks before each commit:** `make pre-add-check FILES="…"` on every
  changed Go file; `scripts/check-docs.sh --links` and `--ids`, and
  markdownlint, on every changed Markdown file; `make gate` ending
  `overall=0`.
* **Seen failing first:** the new assertion fails on a
  `scripts/plant-copy.sh` copy with one planted break, never by dirtying
  the tree. The record quotes the failing line.
* **The Windows test host:** binaries cross-compiled here with
  `CGO_ENABLED=0 GOOS=windows GOARCH=amd64`, copied to a new directory
  under its temporary directory, run from its ssh shell, which is
  elevated (0021-PLAN Deviation D1), and cleaned up afterwards. Every
  task a probe or test registers is named `selfupdate-probe-<random>` or
  `selfupdate-live-<random>`, and is deleted when the run ends, pass or
  fail. No probe or test reads, changes or stops any other task.
* **An interactive-token task runs only while the account is signed in
  at the console** (0021-PLAN rule 6). Before R1's host runs and before
  R2, the agent asks the owner to confirm the sign-in. A run that finds
  no console session stops as a deviation; it is not recorded as a skip.
* **Identifiers:** nothing committed names a host, an account or a real
  absolute path.

## Implementation Steps

### Phase R0: records

1. `HEAD` is `5df1e3d` or a records-only descendant, on `main`.
2. `docs/README.md` gains this plan's row after
   `0022-PLAN-error-chain-and-process-liveness.md`'s.
3. On approval: this plan becomes `in-progress`, with `### Approval`
   quoting the owner; Amendment A3's heading loses "proposed", and it
   records the owner's decision and words.
4. Checks; commit.

### Phase R1: the pin (D7-B)

1. **`internal/winproc/winproc_windows.go`:**

   ```go
   // AliveAccess is the access Alive opens a process with: SYNCHRONIZE
   // for its wait, and the query right for its creation time.
   const AliveAccess = windows.SYNCHRONIZE | windows.PROCESS_QUERY_LIMITED_INFORMATION
   ```

   `Alive` opens with `AliveAccess`. Nothing else in `Alive` changes.
2. **`selfupdate/service/taskscheduler/live_windows_test.go`, the update
   side:**
   * `liveMode` passes `cfg` to `liveUpdate`;
   * before `runLiveUpdate`, while the task's program still runs, a new
     `writeOpen(cfg, result+".open")` reads the program's PID from the
     last line of `cfg.Dir/starts` (`liveServe` appends it), opens it
     with `winproc.AliveAccess`, and writes one line: `pid=<n> open=ok
     wait=<timeout|signaled|err>`, or `pid=<n> open=<the error>`. It
     closes the handle. A failure to read the PID or write the file is
     the update's error, as for `writeIntegrity`.
3. **The test side:** `TestLiveTaskUpdate` reads `result+".open"` after
   the integrity check, requires `open=ok` and `wait=timeout`, and fails
   with the line otherwise; it logs the line with the integrity RID.
4. **Seen failing,** each a `plant-copy.sh` copy of
   `live_windows_test.go`, cross-compiled and run on the Windows test
   host; the record quotes each failing line:
   * `writeOpen` opens PID 0, which `OpenProcess` always refuses:
     `TestLiveTaskUpdate` fails at the new check, quoting the open's
     error;
   * `writeOpen` writes `signaled` where the wait read `timeout`: it
     fails, quoting `wait=signaled`.

   A refusal by access rights cannot be planted reliably: which process
   an unelevated caller may not open is what R2 measures.
5. **On the Windows test host,** the console sign-in confirmed, the
   tree's test binary with `SELFUPDATE_REQUIRE_TASKSCHED=1`:
   * `-run '^TestLiveTaskUpdate$' -count=3`: `PASS`, each run logging
     `open=ok wait=timeout` and RID `0x2000`;
   * `-run '^TestLiveTask'`: the whole live suite `PASS`, ending with
     `TestLiveTaskzCleanup`;
   * `go test ./internal/winproc/` cross-compiled: `PASS`.
6. **Stop condition.** If the tree's run records a refused open, or
   `wait=` anything but `timeout`, A2's [derived] fact does not hold for
   the supported case: stop, and put the line to the owner with
   resolutions. The test is not loosened to pass.
7. Checks: vet for darwin, linux and windows (`CGO_ENABLED=0`); `make
   pre-add-check FILES="<the 2 Go files>"`; `make gate`. No file is added,
   so `docs/architecture.md`'s counts do not change.
8. Commit.

### Phase R2: the high-integrity pairing (scratch probes)

Nothing in the tree changes; the probes live in the scratchpad.

1. **The target:** from the elevated shell, register
   `selfupdate-probe-<random>` (`InteractiveToken`, `HighestAvailable`, no
   trigger) whose action is `pairprobe.exe target <dir>`. It writes its
   PID and its integrity RID, then sleeps up to 120 s or until a stop
   file appears. Run it; read the RID (`0x3000` expected, recorded as
   read).
2. **The caller:** register a second `selfupdate-probe-<random>`
   (`InteractiveToken`, `LeastPrivilege`, no trigger) whose action is
   `pairprobe.exe caller <dir>`. It records its own RID (`0x2000`
   required, or the run is void), opens the target's PID with
   `SYNCHRONIZE | PROCESS_QUERY_LIMITED_INFORMATION`, and records the
   result, the wait's reading if it opened, and whether the snapshot
   lists the PID.
3. **The control,** in the same run: the caller opens a third task's
   program at `LeastPrivilege` the same way. It must open, or R1's
   finding and R2's disagree, which is a deviation.
4. Both answers are recorded verbatim, redacted per Rules. A refused
   open means `Alive` takes its snapshot path for that pairing, which its
   doc comment already describes; no stop.
5. Host cleanup: the tasks deleted, the run directory removed, and
   `schtasks /query` showing no `selfupdate-` task.

### Phase R3: closing

1. `### Verification, at closing`, V1 to V4 with their evidence.
2. Amendment A2's "[unverified] until the measurement" consequence, and
   A3's two [derived] and [unverified] facts, gain italic annotations
   with the measured answers, as A2's Q1 annotations did. The earlier
   plans' records are not edited.
3. `### Closing`, `status: complete`, the index row. Commit, then push on
   the owner's ask.

## Verification

* **V1 (measured).** R1's tree runs record `open=ok wait=timeout` from
  integrity RID `0x2000`, three times.
* **V2 (pinned).** The new check failed on both of R1's plants, a
  refused open and a wait read as signalled, and passes on the tree; the
  live suite passes.
* **V3 (pairing).** R2 records the open's result for an unelevated
  caller and a high-integrity program, with its control.
* **V4 (gate).** `make gate` ends `overall=0` for R1; CI passes on the
  pushed commit. CI does not run the live suite, and the record says so.

## Rollout and Rollback

* **Rollout:** R0 to R3, one commit each, on `main`, pushed on the
  owner's ask. No release.
* **Rollback:** `git revert` of R1 removes the constant and the pin;
  `Alive`'s behaviour is the same either way.
* **A deviation in any phase** stops it.

## Execution record

### Approval (2026-10-12)

The owner committed this plan, Amendment A3 and the index row as
`7735803`, and said "committed, proceed". Amendment A3 records D7-B,
the recommended option, as decided; its heading no longer says
"proposed". This plan is `in-progress`, here and in the index.

### R1 (2026-10-12)

**Files.** Changed: `internal/winproc/winproc_windows.go`,
`selfupdate/service/taskscheduler/live_windows_test.go`. No file is
added.

**As built:**

* `winproc.AliveAccess`, `SYNCHRONIZE |
  PROCESS_QUERY_LIMITED_INFORMATION`, with a doc comment citing
  Amendment A3; `Alive` opens with it. Nothing else in `Alive` changed.
* `liveMode` passes `cfg` to `liveUpdate`. After `writeIntegrity`,
  `writeOpen` reads the task program's PID from the last line of
  `starts`, opens it with `winproc.AliveAccess`, and writes `pid=<n>
  open=ok wait=<timeout|signaled|err: …|0x…>` or `pid=<n> open=<the
  error>` to `<result>.open`.
* `TestLiveTaskUpdate` requires the line to end ` open=ok wait=timeout`,
  and logs it with the integrity RID.

**The console sign-in.** The owner confirmed it ("windows is logged in,
screen is unlocked"). Before that, a read-only check found Explorer in
session 1 and a console user set.

**Seen failing (step 4),** each planted copy cross-compiled here and run
on the Windows test host with `SELFUPDATE_REQUIRE_TASKSCHED=1
-test.run '^TestLiveTaskUpdate$'`:

| Plant | Result |
| :--- | :--- |
| `writeOpen` opens PID 0 | `--- FAIL: TestLiveTaskUpdate (5.27s)`: `live_windows_test.go:567: the update's open of the task's program with winproc.AliveAccess: pid=44128 open=The parameter is incorrect.; want open=ok wait=timeout (0022-MADR A3)` |
| `writeOpen` writes `signaled` for `WAIT_TIMEOUT` | `--- FAIL: TestLiveTaskUpdate (5.24s)`: `live_windows_test.go:567: … pid=38092 open=ok wait=signaled; want open=ok wait=timeout (0022-MADR A3)` |

**The tree (step 5),** on the Windows test host, the binaries' SHA-256
checked there:

* `-test.run '^TestLiveTaskUpdate$' -test.count 3`: `PASS` three times
  (5.19 s, 5.34 s, 5.32 s), each logging `the update's integrity RID
  0x2000; pid=<n> open=ok wait=timeout`;
* `-test.run '^TestLiveTask'`: `TestLiveTaskUpdate` (`0x2000`,
  `open=ok wait=timeout`), `TestLiveTaskStopHoldsDisabled` (71.89 s),
  `TestLiveTaskRollback`, `TestLiveTaskRequestStop`,
  `TestLiveTaskStopRefusesDisabled` and `TestLiveTaskzCleanup` `PASS`;
* `internal/winproc`'s tests, `TestAlive` among them: `PASS`.

**Step 6, the stop condition:** not met. An unelevated update of the same
user opened the task's program with `AliveAccess` in all four runs, and
the wait read it as running. A2's [derived] fact holds for the supported
case [measured].

**Host cleanup.** The run directory was removed; `schtasks /query` lists
0 `selfupdate-` tasks.

**Checks.** gofmt clean; vet for darwin, linux and windows
(`CGO_ENABLED=0`): clean. `make pre-add-check FILES="<the 2 Go files>"`:
`go-precheck: 2 file(s) clean`. `make gate` on this Mac, load average
8.89 before and 45.86 after: every step `rc=0`, `apicheck` `compatible
with v1.15.1`, `arch` `16 rows … 0 differ`, `overall=0`.

### R2 (2026-10-12)

The owner committed R1, confirmed the console sign-in ("logged in"), and
said "proceed". Each run's script also recorded a console user present
and Explorer in one interactive session.

**The probes,** in the scratchpad, not the tree: `pairprobe.exe` (the
standard library and `golang.org/x/sys/windows`, cross-compiled here)
and `pair.ps1`, run from the elevated ssh shell. The script registers
three `selfupdate-probe-<random>` tasks for the signed-in user, each
`Interactive`, with no trigger:

* **high:** `RunLevel Highest`, `pairprobe target <dir> high`: writes its
  PID and integrity RID, then waits for a stop file;
* **low (the control):** `RunLevel Limited`, the same with `low`;
* **caller:** `RunLevel Limited`, `pairprobe caller <dir> high low`:
  records its own RID, then for each target opens it with `SYNCHRONIZE |
  PROCESS_QUERY_LIMITED_INFORMATION` (`winproc.AliveAccess`'s value) and
  reads `WaitForSingleObject(h, 0)`, opens it with each right alone, and
  looks for it in the snapshot.

The script starts the two targets, then the caller, prints the results,
then writes the stop file and removes the three tasks in `finally`. It
ran three times, each in a new directory, the binary's and script's
SHA-256 checked on the host.

**Output, identical in the three runs** (the targets' and caller's
lines):

```text
high rid=0x3000
low rid=0x2000
caller rid=0x2000
high alive_open=ok wait=timeout query_open=ok sync_open=ok snapshot=true
low alive_open=ok wait=timeout query_open=ok sync_open=ok snapshot=true
```

**Reading:**

* The caller ran at medium integrity (`0x2000`), so the runs are valid
  (step 2).
* **The control (step 3)** opened and read as running, as R1 found
  through the backend: R1 and R2 agree.
* **An unelevated caller opens a high-integrity program of the same user
  with `AliveAccess`** [measured]: the open succeeded, and each right
  alone succeeded, in every run. A3's [unverified] expectation of a
  possible refusal is answered: no. `Alive` takes its open-and-wait path
  for this pairing too, and checks `born`; the snapshot fallback was not
  reached in any pairing measured.
* Why the integrity label does not refuse it is not measured. Both rights
  are of a kind a process's default mandatory policy (no read up, no
  write up) does not withhold, which is consistent with the result
  [derived].

**Host cleanup.** Each run printed `cleanup.task=removed` three times and
`dir-removed`; afterwards `schtasks /query` lists 0 `selfupdate-` tasks.

**Checks.** Records only: `scripts/check-docs.sh --links` and `--ids` on
this plan, clean. The repository's markdownlint configuration excludes
MADR and PLAN files.
