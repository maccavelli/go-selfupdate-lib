---
status: in-progress
date: 2026-10-24
associated-madr: "0034-MADR-export-task-scheduler-enable-and-disable.md"
---
# Implement `Task.Enable`, `Task.Disable`, `Status.Disabled` and the switch left out of `DiffDefinitions`, and release `v1.21.0`

Associated MADR: [0034-MADR-export-task-scheduler-enable-and-disable.md](0034-MADR-export-task-scheduler-enable-and-disable.md)
(D1-A, D2-A, D3-A, D4-A, D5-B and D6-A, as the owner chose on
2026-10-24).

## Goal

* A program can enable and disable its task through `taskscheduler.Task`,
  so its setup needs no `schtasks.exe` (issue #8).
* `Start`, `Stop` and the update's sequence are unchanged.
* A program can read the task's switch, and a stopped task is not
  reported as definition drift (MADR D5-B, D6-A).
* The unsettled facts are measured first (L1), every new test is seen
  failing first, and it ships as `v1.21.0`. Issue #8 is closed on the
  owner's ask.

## Scope

### In scope

| Phase | What | Commit |
| :--- | :--- | :--- |
| L0 | this pair, its index rows | records only |
| L1 | probes P1 to P5 on the Windows test host; the MADR amended with the results | records only |
| L2 | `Enable`, `Disable` and `Status.Disabled`, with fake tests | code and tests |
| L3 | `DiffDefinitions` skips `Settings/Enabled`, if P1 confirms F16 | code and tests |
| L4 | the live tests | tests |
| L5 | the documents | docs |
| L6 | `v1.21.0` | per the architecture page's "Releasing this module" |
| L7 | closing | records only |

### Out of scope

* Changing `Start` or `Stop` (MADR D2-A).
* Keeping `Stop`'s record across processes (MADR D2-C).
* An enable or a disable on the `systemd`, `launchd` or `scm` backends,
  or a service-wide interface (MADR D1-D).
* `Register` keeping the task's switch across a re-registration. Its
  documentation says what P2 measures.

## Rules

As 0033-PLAN's:

* **Commits.** Commits on `main` only on the owner's "commit to main" in
  that turn, with `git commit --no-edit`. A push or a tag needs its own
  word.
* **Deviations.** Each phase stops at a deviation. It is recorded here
  and, when a decision or a fact changes, in the MADR.
* **Checks:**
  * `make pre-add-check` on changed Go files;
  * cross-target `go vet`;
  * the docs checks on changed Markdown;
  * the architecture table when a `.go` file is added;
  * `make apicheck` compatible with `v1.20.0`;
  * `make gate` ends `overall=0` before each push.
* **Seen failing first.** Every new test fails on a
  `scripts/plant-copy.sh` copy, never a dirtied tree. A Windows-only
  test's plant runs on the Windows test host.
* **The Windows test host.** Its ssh shell is elevated. A probe that
  measures medium integrity (P4) runs inside a driver task at least
  privilege in the console session, as 0032-PLAN J1 did. The owner
  confirms the console sign-in before L1. Unit and live tests run from
  the ssh shell, as CI's runner does. The tree goes to the host as a
  bundle, and the host builds with Go 1.27.3. The host is not named in
  the tree.
* **Identifiers.** Probe output carries SIDs, account names and the
  host name. They become `<sid>`, `<user>` and `<host>` before anything
  enters the tree.

## Implementation Steps

### Phase L0: records

1. The MADR is `accepted` with the owner's choices. `docs/README.md`
   gains this pair's rows after 0033's.
2. `scripts/check-docs.sh --links` and `--ids` on the three files.
   Commit on the owner's word. This plan becomes `in-progress` in L1's
   commit.

### Phase L1: probes (MADR Confirmation)

Each probe is a PowerShell script in the scratchpad. It drives the COM
API through `New-Object -ComObject Schedule.Service`:

* `IRegisteredTask.Enabled = $false` is `put_Enabled`;
* `.Xml` is `get_Xml`;
* `.State` is `get_State`;
* `RegisterTaskDefinition` re-registers.

Each probe uses a throwaway task in a folder of its own,
`\selfupdate-probe-<random>`, whose action runs `ping -n 120 127.0.0.1`
as a long-running program. Each deletes its task and folder before it
ends, and a cleanup script removes any left over.

1. **P1.** Register a task with no `Settings/Enabled` element, and read
   `Xml`. Set `Enabled = $false` and read `Xml`, then set
   `Enabled = $true` and read `Xml` again. Record `Settings/Enabled`
   each time: absent, `true` or `false`.
2. **P2.** On the disabled task, call `RegisterTaskDefinition` with
   `TASK_CREATE_OR_UPDATE` (6) and the first definition, which has no
   `Settings/Enabled`. Record `Enabled` after.
3. **P3.** On an enabled task, set `Enabled = $true`, and on a disabled
   one, `$false`. Record whether each call succeeds, and `Xml` and
   `LastRunTime` before and after.
4. **P4.** From the elevated ssh shell, register a task for the
   console user, with an `InteractiveToken` principal naming that user.
   From the driver task at least privilege, set `Enabled = $false` on
   it. Record the HRESULT. If the host cannot register for another
   principal from that shell, record why, and leave F12's
   [unverified].
5. **P5.** Run the task, wait until its program runs, set
   `Enabled = $false`, and read `State` and the instances. Then stop the
   task, and read `State` again.
6. **Decision points.** The plan stops for the owner if:
   * P1 shows `Settings/Enabled` unchanged by `Enabled = $false`. D5 is
     then moot, and L3 is dropped with a recorded deviation;
   * P3 shows a same-value write fails. Then L2's "write only on a
     change" is a requirement, not a precaution, which is recorded and
     is not a stop;
   * P5 shows `State` is `disabled` while the program runs. D6-A stands
     as chosen, D6-C's con is corrected in the amendment, and this is
     not a stop.
7. **The amendment.** The MADR gains `## Amendment A1 (<date>): probe
   results`, a table as 0033-MADR's: probe, question, result, effect.
   Each [unverified] is annotated in place. The plan becomes
   `in-progress`. Checks; commit on the owner's word.

### Phase L2: `Enable`, `Disable` and `Status.Disabled`

1. **`selfupdate/service/taskscheduler/lifecycle.go`**, after `Start`:
   * `Enable(ctx context.Context, product string) error` and
     `Disable(ctx context.Context, product string) error`, through one
     unexported `setSwitch(ctx, product, on)`. It:
     * returns `ctx.Err()` first;
     * resolves the task (`t.task`) and opens it (`t.open`), which gives
       `ErrNotInstalled`;
     * reads `r.enabled()` and calls `r.setEnabled(on)` only when the
       value differs, wrapping errors with `mapError("enable" | "disable",
       path, err)`;
     * deletes `t.disabled[path]` under `t.mu` (MADR D3-A);
     * closes the task, joining its error.
   * Their documentation states:
     * what each does;
     * that `Disable` leaves a running program running, and that a
       later update's `Stop` refuses a disabled task whose program runs
       (D4-A);
     * that both clear `Stop`'s record;
     * that `ctx` is checked before and ignored after;
     * the errors.
   * `Start`'s documentation names `Enable` as the way to undo a
     disable made in another process (D2-A).
2. **`status.go`:** `Status` gains `Disabled bool` with JSON tag
   `disabled` (D6-A), set in `readStatus` from the switch that
   `switches` already reads. The `Status` and `Task.Status`
   documentation names it.
3. **`task.go`:**
   * the `Task` type's note on contexts names the two methods as
     checking `ctx` first;
   * the package documentation names `Enable` and `Disable` beside
     `Register`, `Definition` and `Delete`.
4. **`fake_test.go`:** `fakeTask` gains `setErr error`, which
   `fakeReg.setEnabled` returns before it changes anything. Its call is
   recorded either way.
5. **`enable_test.go`** (new), on every OS through the fake:
   * `Enable` on a disabled task, and `Disable` on an enabled one, each
     write once (`enable true`, `enable false` in the fake's record);
   * on a task already in that state, neither writes;
   * `Stop`, `Disable` and `Start`: `Start` returns `ErrTaskDisabled`,
     and records no `enable true` after the `Disable`;
   * `Stop` on one `Task`, then `Start` on a second `Task` value:
     `ErrTaskDisabled`. Then `Enable` on the second, and `Start`
     runs the task (the issue's case);
   * `Stop`, then `Enable`, then `Disable`, then `Start` on one `Task`:
     the record was cleared, so `ErrTaskDisabled`;
   * a missing task: `ErrNotInstalled`;
   * `setErr` set to `hrAccessDenied`: `ErrPermission`;
   * a cancelled `ctx`: its error, and no call recorded;
   * `Disable` on a running task: the fake's instance is still
     listed (D4-A).
6. **`status_test.go`:** `Status.Disabled` is false for an enabled task,
   true for a disabled one with no instance and for a disabled one with
   an instance, and false in a zero `Status`. The JSON golden, if one
   lists the fields, gains `disabled`.
7. **Seen failing** (plants):
   * (a) `setSwitch` not deleting the record;
   * (b) the write unconditional;
   * (c) `Disabled` never set;
   * (d) `Enable` calling `setEnabled(false)`;
   * (e) the `ctx` check removed.
8. **`docs/architecture.md`:** the `taskscheduler` row's test files go
   from 17 to 18. The package paragraph names the two methods.
9. Checks, including `make apicheck`, and `go doc` showing `Enable`,
   `Disable` and `Status.Disabled`. Commit on the owner's word.

### Phase L3: the switch left out of `DiffDefinitions` (if P1 confirms F16)

1. **`definition.go`:**
   * `compared` returns false for `Settings/Enabled` as well;
   * `DiffDefinitions`'s documented list of what is not compared gains
     it, with the reason: the switch is state that `Stop`, `Enable` and
     `Disable` set, read with `Status.Disabled` (D5-B);
   * `settingsDefaults` keeps its `Enabled` entry, now unused by the
     comparison, so the table still records Task Scheduler's default.
2. **`register.go`:** `Register`'s documentation says it sets the switch
   from the definition, as P2 measured, so a setup that keeps a task
   disabled calls `Disable` after `Register`.
3. **`definition_test.go`:**
   * a stored definition with `<Enabled>false</Enabled>` and a wanted
     one without it compare equal;
   * the reverse also compares equal;
   * the existing single-element cases still report every other
     `Settings` element;
   * the single-element case at `definition_test.go:61`, which expects
     a change of `<Enabled>` to report `Settings/Enabled`, moves to a
     list of changes that report nothing, beside the new cases. This is
     the one existing expectation L3 changes, by MADR D5-B.
4. **Seen failing:** (f) `compared` still comparing `Settings/Enabled`.
5. Checks; commit on the owner's word. If L1 dropped this phase, the
   deviation entry says so, and L5's documents do not mention it.

### Phase L4: the live tests

1. **`live_windows_test.go`:**
   * `TestLiveTaskStopRefusesDisabled` disables through `s.Disable` in
     place of `schtasks /change /disable` (MADR F11). Its other
     assertions stay.
   * New `TestLiveTaskEnableDisable`:
     * while the program runs, `Disable` leaves it running, `Status`
       reads `Disabled` true and `Running` true, and no new start
       appears;
     * a second `Task` value's `Start` after the first's `Stop` fails
       with `ErrTaskDisabled`;
     * after the second's `Enable`, its `Start` runs the task,
       `waitReady` sees a new instance, and `Status.Disabled` reads
       false.
   * Under L3, `TestLiveTaskRegister` or the new test also checks that
     after a `Disable`, `SameDefinition(Definition, the registered
     definition)` is true.
2. **Runs:** on the Windows test host, from the ssh shell, with
   `SELFUPDATE_REQUIRE_TASKSCHED=1`, as CI's `windows-2025` job runs the
   live tests.
3. **Seen failing:** on the Windows test host, with plants (a) and (d)
   in the live test's scratch copy, and (f) under L3.
4. Checks; commit on the owner's word.

### Phase L5: the documents

1. **`docs/guides/extending-selfupdate.md`, "Task Scheduler":**
   * a setup's start is `Enable`, then `Start`;
   * its stop is `Stop`, which leaves the task disabled;
   * `Disable` leaves the program running;
   * `Status.Disabled` reads the switch;
   * under L3, `DiffDefinitions` skips the switch, and `Register` sets
     it.
2. **The migration guide:** `## 21. From v1.20 to v1.21`, in §20's
   shape. "Behaviour you may notice" names L3's change to
   `DiffDefinitions`.
3. **`CHANGELOG.md`:** `## Unreleased`:
   * Added: `Enable`, `Disable`, `Status.Disabled`;
   * Changed: `DiffDefinitions`, under L3.
4. **`docs/architecture.md`:** the `taskscheduler` paragraph, if L2 left
   it short.
5. Checks; commit on the owner's word.

### Phase L6: `v1.21.0`

As 0033-PLAN K6:

1. The pre-tag change and `make gate`.
2. The push and CI on `main`.
3. The tag check, the disclosure guard, and the tag on the owner's
   word.
4. The tag's CI, its identity legs printing
   `v1.21.0 (release) <12hex>`.
5. The pin commit.
6. The proxy.
7. Issue #8 is closed with a comment naming `v1.21.0`, on the owner's
   ask.

### Phase L7: closing

`### Verification, at closing`, `status: complete`, the index row. The
MADR stays `accepted`.

## Verification

* **V1 (probes):** P1 to P5 ran, and the MADR's Amendment A1 records
  them.
* **V2 (methods):** `enable_test.go` passes on every OS, and plants (a),
  (b), (d) and (e) failed.
* **V3 (status):** `Status.Disabled` follows the switch in the fake and
  live tests, and plant (c) failed.
* **V4 (comparison):** under L3, a disabled task's stored definition
  compares equal to the registered one, every other `Settings` element
  is still compared, and plant (f) failed.
* **V5 (live):** `TestLiveTaskEnableDisable` and
  `TestLiveTaskStopRefusesDisabled` pass on the Windows test host and on
  CI's `windows-2025`, on `main` and on the tag.
* **V6 (API):** `make apicheck` against `v1.20.0` reports no
  incompatibility, and `go doc` shows the additions.
* **V7 (release):** `v1.21.0` is tagged, its CI is green, and the proxy
  serves it as `@latest`. #8 is closed.

## Rollout and Rollback

* **Rollout:** one commit per phase on `main`, pushed on the owner's
  word, and `v1.21.0` at L6. A program adopts the change by requiring
  `v1.21.0` and replacing its `schtasks /change /enable` and `/disable`
  with `Enable` and `Disable`.
* **Rollback:** before the tag, `git revert` of a phase. After it, fix
  forward. L3 alone can be reverted in a later patch release if a
  consumer depended on `Settings/Enabled` being reported. The migration
  guide names the change.

## Execution record

### Approval

The owner committed L0 as `3d7a149` and said "proceed" on 2026-10-24,
which approves this plan.

### L1: probes (2026-10-24)

* The console user was signed in on the Windows test host
  (`Win32_ComputerSystem.UserName` set), so P4's driver task could run
  in the console session.
* One PowerShell script, run from the ssh shell, and P4's driver script
  were copied to the host's temporary directory, run, and removed. The
  scripts print values only, never a definition, so no SID or account
  name left the host.
* Output, verbatim:

  ```text
  P1 registered without Settings/Enabled: Settings/Enabled=absent Enabled=True
  P1 after Enabled=false: Settings/Enabled=false Enabled=False State=1
  P1 after Enabled=true: Settings/Enabled=absent Enabled=True State=3
  P2 re-registered (TASK_CREATE_OR_UPDATE) while disabled: Enabled=True Settings/Enabled=absent
  P2 re-registered with <Enabled>true</Enabled> while disabled: Enabled=True
  P3 Enabled=true on an enabled task: ok; Xml unchanged=True; LastRunTime unchanged=True
  P3 Enabled=false on a disabled task: ok; Xml unchanged=True; LastRunTime unchanged=True
  P5 running before: State=4 instances=1
  P5 disabled while running: State=4 Enabled=False instances=1
  P5 after Stop: State=1 instances=0
  P4 driver integrity: Medium
  P4 p4least Enabled=false from medium: ok
  P4 p4highest Enabled=false from medium: ok
  P4 p4least after the driver: Enabled=False
  P4 p4highest after the driver: Enabled=False
  cleanup: probe folders left=0
  ```

* After the run: no `selfupdate-probe-*` task folder, no probe
  directory under the host's temporary directory, and no `PING` process
  left.
* **Decision points:** none tripped. P1 confirms F16, so L3 runs. P3's
  same-value writes succeed, so writing only on a change stays a
  precaution. P5 reads `running` for a disabled running task, which
  D6-A expected.
* The MADR gains Amendment A1, and its five [unverified] items are
  annotated in place.

### L2: `Enable`, `Disable` and `Status.Disabled` (2026-10-24)

* L1 was committed by the owner as `4c6416c`.
* **`lifecycle.go`:**
  * `Enable` and `Disable`, after `Start`, through `setSwitch`. It checks
    `ctx`, opens the task, reads the switch, writes it only when it
    differs, and deletes `Stop`'s record under `t.mu`.
  * `Start`'s documentation names `Enable` for a `Stop` made by another
    `Task`.
  * `Stop`'s unexported helper `disable` is renamed `stopDisable`.
    revive's `confusing-naming` refused `Disable` beside `disable` in one
    file, as it refused `refuseBatchFile`'s old name in 0032. This is an
    unexported rename in a file the phase names, and no behaviour
    changes.
* **`status.go`:** `Status.Disabled` (JSON `disabled`), after `Enabled`,
  set from the switch `readStatus` already reads.
* **`task.go`:** the package documentation names `Enable` and `Disable`.
  The `Task` type's note on contexts names the methods that check `ctx`
  first.
* **`fake_test.go`:** `fakeTask.setErr`, returned by `setEnabled` after
  it records the call.
* **`enable_test.go`** (new): `TestEnableDisableWrites`,
  `TestStartAfterStopElsewhere` (the issue's case),
  `TestSwitchClearsStopRecord`, `TestDisableLeavesProgramRunning` and
  `TestEnableDisableErrors`.
* **`status_test.go`:** the `disabled` case expects `Disabled`, and a new
  case, "disabled while running", expects `Disabled` with `State`
  `running` (MADR Amendment A1, P5). The JSON goldens gain `disabled`.
* **Here and on the Windows test host:** the `taskscheduler` package's
  tests pass.
* **Seen failing** (`scripts/plant-copy.sh`):

  | Plant | Failure |
  | :--- | :--- |
  | (a) the record not cleared | `enable_test.go:102: Stop, Disable, Start: Start = <nil>, want ErrTaskDisabled`; `enable_test.go:105: ... Start enabled the task: ["enable true" "run"]`, both cases |
  | (b) the write unconditional | `enable_test.go:42: enable an enabled task: calls ["enable true"], want []`, and the disable case |
  | (c) `Disabled` never set | `enable_test.go:126: Status {... Disabled:false State:running ...}; want running and disabled`; `status_test.go:49: disabled: ...` and `disabled while running: ...` |
  | (d) `Enable` disabling | `enable_test.go:42: enable a disabled task: calls [], want ["enable true"]`; `enable_test.go:45: ... switch false, want true`; `TestStartAfterStopElsewhere` failing |
  | (e) no `ctx` check | `enable_test.go:164: Disable, cancelled: <nil>`; `enable_test.go:167: a cancelled Disable changed the task: calls ["enable false"], switch false` |

* **Checks:**
  * `make pre-add-check` on the six Go files: "6 file(s) clean (gofmt,
    golangci-lint, go vet, go test, govulncheck)", after the rename. The
    first run reported the revive finding above.
  * `make apicheck`: "compatible with v1.20.0".
  * `go doc` shows `func (t *Task) Enable(ctx context.Context, product
    string) error`, `func (t *Task) Disable(...)` and `Disabled bool
    \`json:"disabled"\``.
  * `docs/architecture.md`: the `taskscheduler` row is 15 source and 18
    test files, and its paragraph names the two methods and the field.
    `--arch`: "18 rows, 0 differ".
