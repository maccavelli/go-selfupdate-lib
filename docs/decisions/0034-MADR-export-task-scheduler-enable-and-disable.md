---
status: accepted
date: 2026-10-24
decision-makers: go-selfupdate-lib maintainers
consulted: "GitHub issue #8, \"taskscheduler: export Enable and Disable for a task\" (2026-10-24), from the owner for magic-cli-remote; a read of `selfupdate/service/taskscheduler` at a2233f8 (v1.20.0 and its closing records); 0021-MADR, 0022-MADR Amendment A1, 0024-MADR D4-A and 0032-MADR, and the live tests that hold them"
informed: the fleet's programs that run as a Windows scheduled task and set it up with this package, magic-cli-remote first
---
# Export `Task.Enable` and `Task.Disable`, keep `Start`'s rule, and stop counting the task's switch as definition drift

## Context and Problem Statement

Issue #8 asks for an exported way to enable and disable a scheduled task
on `taskscheduler.Task`. With `Register`, `Definition`, `Delete`,
`SameDefinition` and `Status` (`v1.19.0`), a program's setup needs
`schtasks.exe` for one thing only: `schtasks /change /enable` and
`/disable`.

The issue's case is a setup command that runs in a later process than
the one that stopped the task. `Stop` disables the task, so that a
repeating trigger cannot restart a program stopped on purpose, and leaves
it disabled. A later "start the service" command, in another process,
builds a new `Task`. Its `Start` does not know that a `Stop` disabled the
task, does not enable it, and fails with `ErrTaskDisabled`. The program
has to enable the task some other way first.

This record decides the shape of the API (the issue's first open
question), whether `Start`'s rule changes (its second), and how the new
methods meet the state `Stop` and `Start` keep. It also decides two
things the issue does not raise, which the code shows matter once a
program can disable a task: how `DiffDefinitions` treats the task's
switch, and how a program reads the switch.

### Evidence tags

As in 0032-MADR: **[read]** this repository's code; **[docs]** a
manual, quoted; **[measured]** by this repository's probes or live
tests; **[measured upstream]** in the consumer's records; **[derived]**;
**[unverified]** until the PLAN's probes settle it.

### Facts [observed 2026-10-24, `main` at `a2233f8`]

**The operation exists, unexported [read]**

* **F1.** The transport has the switch: `registered.enabled()` and
  `registered.setEnabled(on bool)` (`scheduler.go:66-68`). On Windows,
  `setEnabled` calls `IRegisteredTask::put_Enabled`, vtable slot 11, with
  a `VARIANT_BOOL` (`scheduler_windows.go:206-213`,
  `com_windows.go:77-78`). The test fake records each call as
  `enable <bool>` and moves the task's state between `disabled` and
  `ready` (`fake_test.go:273-284`).
* **F2.** `Stop` reads the switch, then refuses a disabled task whose
  program runs (`ErrTaskDisabled`, `lifecycle.go:166-172`). Otherwise it
  disables an enabled task and records that it did, in
  `t.disabled[path]` (`disable`, `lifecycle.go:213-226`). When `Stop`
  fails after that, with the program still running, it enables the task
  again and clears the record (`enableAgain`, `lifecycle.go:228-239`;
  0024-MADR D4-A).
* **F3.** `Start` enables the task only when the record says this
  `Task`'s `Stop` disabled it, then clears the record
  (`lifecycle.go:291-301`). A task disabled any other way stays
  disabled. Task Scheduler refuses to run it, and `Start` returns
  `ErrTaskDisabled` (`mapError`, `task.go:236-237`).
* **F4.** The record lives in memory, in the `Task` value:
  `disabled map[string]bool` (`task.go:119-121`), made empty by `New`
  (`task.go:178`) and guarded by `t.mu`. A second process, or a second
  `Task` in the same process, starts with no record [read]. So a `Stop`
  in one process and a `Start` in another leave the task disabled, and
  the `Start` fails with `ErrTaskDisabled` [derived from F3 and F4].
  This is the issue's case.
* **F5.** No exported method of `Task` sets the switch alone. Its
  methods are `Installed`, `Running`, `Enabled`, `Status`, `Stop`,
  `Start`, `WaitHealthy`, `Reconcile`, `Restore`, `Inside`, `Detach`,
  `CleanupHandOffs`, `Register`, `Definition` and `Delete` [read]. No
  other backend (`systemd`, `launchd`, `scm`) exports an enable or a
  disable either [read].
* **F6.** `Task.Enabled` is not the switch. It implements
  `selfupdate.EnabledLifecycle`, and is true only for an enabled task
  that also has an enabled logon, boot or repeating time trigger
  (`enabledBy`, `lifecycle.go:80-91`; 0021-MADR D3). `Status.Enabled` is
  the same answer (`status.go:21-23`, `:86`). `Status.State` is
  `TASK_STATE` by name (`stateName`, `task.go:259-273`). The test fake
  keeps a running task's state `running` when it is disabled
  (`fake_test.go:277-279`); whether Task Scheduler does the same is
  [unverified until probe P5]. No exported field or method gives the
  switch itself [read].
* **F7.** `Register`, `Definition`, `Delete` and `Status` check `ctx`
  before they start and ignore it after, because no Task Scheduler call
  takes one (`register.go:27-33`, `:56`, `:83-90`; `status.go:44-51`).
  `Installed`, `Running`, `Enabled`, `Start`, `Reconcile` and `Restore`
  ignore it entirely (`task.go:105-107`).
* **F8.** Errors are mapped once: `0x80070002` is
  `service.ErrNotInstalled`, `0x80070005` is `service.ErrPermission`,
  and `0x80041326` is `ErrTaskDisabled` (`mapError`, `task.go:227-240`).
  `New` returns `service.ErrUnsupported` off Windows, so no method of a
  `Task` runs there (`task.go:139-141`; `scheduler_other.go:7`).

**What Task Scheduler does with the switch**

* **F9.** A disabled task is not run by its triggers. A repeating
  `TimeTrigger` relaunched an ended task within a minute, and stopped
  relaunching once the task was disabled [measured upstream, 0021-MADR
  F7, probe 8]. `TestLiveTaskStopHoldsDisabled` holds this for `Stop`:
  no start while disabled, then `Start` enables and runs [measured].
* **F10.** A disabled task is not run on demand either: `Run`, and
  `RunEx` with `TASK_RUN_IGNORE_CONSTRAINTS`, fail with `0x80041326`
  [measured, 0022-MADR Amendment A1].
* **F11.** Disabling does not end a running instance. After
  `schtasks /change /disable` on a running task, its program still runs,
  and `Stop` refuses with `ErrTaskDisabled` and leaves it running
  (`TestLiveTaskStopRefusesDisabled`, `live_windows_test.go:865-905`)
  [measured]. That test disables the task through `schtasks.exe`,
  because no exported method does.
* **F12.** A user can set the switch of their own task at medium
  integrity: `Stop` and `Start` did so in 0021-PLAN T6a's live update,
  run from a least-privilege `InteractiveToken` task, whose token read
  as integrity `0x2000` [measured, 0021-MADR Amendment A4]. `/change /disable` needs
  no elevation for a user's own task [measured upstream, 0021-MADR D4].
  A task an administrator registered for the user may deny the change
  [unverified]. That surfaces as `ErrPermission` (F8) [derived].
* **F13.** `put_Enabled(1)` enables the task as `VARIANT_TRUE` does
  [measured, 0021-MADR Amendment A4]. Whether `put_Enabled` with the
  value the task already has succeeds without a change is
  [unverified until probe P3].

**The switch and the definition**

* **F14.** A definition carries the switch as `Settings/Enabled`. A
  definition registered with `<Enabled>false</Enabled>` is stored with
  it (`testdata/definitions/settings.wanted.xml:14`,
  `settings.stored.xml:18`; 0032-MADR Amendment A1, P1-7) [measured].
* **F15.** `DiffDefinitions` compares every `Settings` element except
  `IdleSettings` and `UseUnifiedSchedulingEngine`, so it compares
  `Settings/Enabled`, with `true` as its default
  (`definition.go:40-44`, `:190-207`) [read]. `SameDefinition` is its
  empty-diff test (`definition.go:26-30`).
* **F16.** Whether `put_Enabled(false)` changes the stored definition's
  `Settings/Enabled`, as `Definition` (`get_Xml`) returns it, is
  [unverified until probe P1]. Both `IRegisteredTask` and the
  definition's `ITaskSettings` have an `Enabled` property [docs]; whether
  they are one stored value is what P1 measures. If it does, then after
  any `Stop`,
  and after a `Disable`, `DiffDefinitions` between the stored definition
  and the program's rendered one reports `Settings/Enabled`:
  `false` against `true` [derived].
* **F17.** `Register` uses `TASK_CREATE_OR_UPDATE` with the program's
  rendered definition (`register.go:43`). Whether re-registering a
  disabled task with a definition that leaves `Settings/Enabled` out
  enables it is [unverified until probe P2]. If it does, then a setup
  that runs "compare, and register when different" against a task
  `Stop` or `Disable` left disabled re-enables it, and a repeating
  trigger restarts the program the user stopped (F9) [derived from F15,
  F16 and F17].

**The consumer [reported in the issue]**

* **F18.** magic-cli-remote's `setup-service` enables, then runs, its
  per-user task (`startWindows`), and disables, then ends, it
  (`stopWindows`), with `schtasks /change /enable` and `/disable`
  (`internal/cli/service/control_schtasks.go`). Its
  `0207-MADR-move-to-current-library-releases-and-close-the-adoption-gaps.md`,
  amendment B (D17, D18), moves the rest of its Task Scheduler code onto
  `Register`, `Definition`, `Delete`, `SameDefinition` and `Status`.
  These two calls are what would be left.

## Decision Drivers

* **No `schtasks.exe` in a setup that uses this package (F18).** The
  package parses no tool's output and needs no tool on the path (package
  documentation, `task.go:8-10`).
* **An administrator's disable stays a decision** (0022-MADR Amendment
  A1). Nothing the library does on its own enables a task that someone
  disabled on purpose. A program enables it only by asking.
* **The update's sequence is unchanged.** `Stop`, `Start`,
  `WaitHealthy` and the refusal of a disabled running task (F2, F3)
  behave as today, and their live tests stand.
* **Readable call sites.** A setup's start and stop read as what they
  do.
* **A stopped task is not drift.** Once a program can disable a task,
  the switch is state its setup sets, not a definition it renders. A
  comparison that calls it drift invites a re-registration that undoes
  the stop (F17).
* **Facts before the release.** The three [unverified] items (F13, F16,
  F17) are measured before code depends on them, as 0032 and 0033 did.
* **Compatible within `v1`.** `make apicheck` reports additions only.

## Considered Options

### D1: the shape of the API (the issue's first question)

* **D1-A.** Two methods:
  `Task.Enable(ctx context.Context, product string) error` and
  `Task.Disable(ctx context.Context, product string) error`.
* **D1-B.** One method:
  `Task.SetEnabled(ctx context.Context, product string, on bool) error`.
* **D1-C.** Both: `SetEnabled`, and `Enable` and `Disable` calling it.
* **D1-D.** An interface in `selfupdate/service`, such as
  `Switcher{Enable, Disable}`, that every backend implements.

### D2: `Start`'s rule (the issue's second question)

* **D2-A.** `Start` stays as it is. `Enable` is the explicit way to undo
  a stop made by another process.
* **D2-B.** `Start` enables any disabled task.
* **D2-C.** `Stop`'s record survives the process, in the task (its
  `RegistrationInfo` or `Data`) or in a file, so that a later `Start`
  enables a task that any `Task`'s `Stop` disabled, and no other.

### D3: the new methods and `Stop`'s record

* **D3-A.** Both clear the record. After `Enable` or `Disable`, the
  switch is what the caller set, and `Start` does not change it.
* **D3-B.** `Enable` clears the record, and `Disable` leaves it.
* **D3-C.** Neither touches the record.

### D4: a running instance under `Disable`

* **D4-A.** `Disable` sets the switch and leaves a running instance
  running, as Task Scheduler does (F11). `Stop` ends it.
* **D4-B.** `Disable` also ends a running instance.

### D5: the switch in `DiffDefinitions` (raised by F16 and F17)

* **D5-A.** No change. The documentation says a disabled task reports
  `Settings/Enabled`, and a caller drops it by `DefinitionDiff.Path`.
* **D5-B.** `DiffDefinitions` stops comparing `Settings/Enabled`, as it
  already skips `IdleSettings` and `UseUnifiedSchedulingEngine`. The
  switch is read with D6's field and set with `Enable` and `Disable`.
  `Register`'s documentation says it sets the switch from the
  definition.
* **D5-C.** No change to `DiffDefinitions`. Add a variant that skips
  named paths, such as
  `DiffDefinitionsExcept(stored, wanted string, paths ...string)`.

### D6: reading the switch (raised by F6)

* **D6-A.** `Status` gains `Disabled bool` (JSON `disabled`): the task's
  switch is off. It is false when the task is not installed, as every
  other field of a zero `Status` is.
* **D6-B.** A method, `Task.Switch(ctx, product) (bool, error)`.
* **D6-C.** Nothing: a caller compares `Status.State` with `disabled`.

## Decision Outcome

Chosen by the owner on 2026-10-24 ("d1-d6 follow recommendations"):

* **D1: "D1-A", `Enable` and `Disable`,** because the call sites read as
  the operation, mirror `Stop` and `Start`, and do not sit beside
  `Enabled`, whose meaning differs (F6). `SetEnabled(on)` next to an
  `Enabled()` that does not read what it sets would mislead. D1-C adds a
  second spelling of each call. D1-D asks three other service managers
  for a switch whose meaning differs on each. systemd's `enable` decides
  boot start and leaves `systemctl start` working, where a disabled task
  refuses even a manual run (F10) [docs; F10]. No consumer asks for it.
* **D2: "D2-A", `Start` unchanged,** because 0022-MADR Amendment A1
  guards exactly this: a `Start` that enables any disabled task undoes
  an administrator's disable during an update. With D2-A, a setup's
  "start the service" calls `Enable` and then `Start`. That is the
  user's explicit request to start. D2-C would make `Stop` write
  hidden state into the user's task or file system, and a record that
  outlives a crash can be stale.
* **D3: "D3-A", both clear the record,** because after `Enable` or
  `Disable` the switch is the caller's decision. Under D3-B, a `Stop`
  followed by an explicit `Disable` would have `Start` re-enable a task
  the caller had just disabled. Under D3-C, an `Enable` after `Stop`
  leaves a stale record that a later `Start` acts on.
* **D4: "D4-A", `Disable` leaves the program running,** because that is
  what Task Scheduler does (F11), what the issue proposes, and what
  `Stop` exists for. Its documentation says that a later update's
  `Stop` refuses a disabled task whose program runs (F2): stop the
  program, or enable the task, first.
* **D5: "D5-B", stop comparing `Settings/Enabled`, if probe P1 confirms
  F16,** because the switch becomes state that `Stop`, `Enable` and
  `Disable` set. A comparison that reports it after every `Stop` turns
  "compare, and register when different" into a way to re-enable a
  stopped task (F17). `IdleSettings` and `UseUnifiedSchedulingEngine`
  are skipped for a like reason: Task Scheduler sets them, not the
  program. If P1 shows `put_Enabled` leaves the stored element
  unchanged, D5 is moot and nothing changes. This is a change in what an
  exported function of `v1.19.0` returns, not in its signature. The
  migration guide names it.
* **D6: "D6-A", `Status.Disabled`,** because a setup's status command
  must tell "stopped on purpose" from "no trigger" and from "running",
  and today no field gives the switch (F6). Under D5-B, the field is also where the
  comparison's dropped value can be read. A field in the one-read
  `Status` costs nothing, and follows its exported, JSON-tagged shape
  (0032-MADR D4-A). D6-B adds a call that `Status` already makes. D6-C
  fails while the program runs.

Together:

* **`Enable` and `Disable`** each check `ctx` before they start and
  ignore it after, as `Register` does (F7). They open the task, read
  the switch, and write it only when it differs (F13: a write of the
  same value is not relied on), then clear the record under `t.mu`
  (D3-A).
* **Errors, matched with `errors.Is`:** `service.ErrNotInstalled` for a
  task that does not exist, and `service.ErrPermission` when Task
  Scheduler denies the change (F8, F12). A `Task` exists only on Windows
  (F8).
* **Neither refuses inside the task.** Neither ends a process, so
  `Inside` does not apply, unlike `Stop`.
* **Documentation:**
  * `Start`'s documentation names `Enable` as the way to undo a disable
    made in another process.
  * `Disable`'s says it does not stop the program, and that a later
    update refuses a disabled task whose program runs.
  * `Register`'s says it sets the switch from the definition.
  * The extending guide's Task Scheduler section shows a setup's start
    (`Enable`, `Start`) and stop (`Stop`, which leaves the task
    disabled).
* **Release:** `v1.21.0`. `make apicheck` against `v1.20.0` is to
  report additions only.

### Consequences

* Good, because a setup that uses this package needs no `schtasks.exe`
  (F18). The live test that disables a task through it (F11) can use
  `Disable`.
* Good, because the update's sequence and its two refusals are
  unchanged (D2-A), and an administrator's disable still holds unless a
  program asks to enable it.
* Good, because a setup's "compare, and register when different" no
  longer re-enables a stopped task (D5-B), and a setup can read the
  switch (D6-A).
* Neutral, because a program that wants Task Scheduler to start its
  stopped task again must call `Enable`. `Start` alone keeps failing
  with `ErrTaskDisabled` in another process (D2-A), as it does today.
* Bad, because under D5-B a program that renders `<Enabled>false` to
  ship a task disabled, and finds it enabled, is not told by
  `DiffDefinitions`. It reads `Status.Disabled` instead.
* Bad, because `DiffDefinitions` returns less for a disabled task than
  in `v1.19.0` and `v1.20.0` (D5-B). That is a change of behaviour
  within `v1`, and `apicheck` does not see it. The changelog and the
  migration guide must say it.

### Confirmation

* **Probes, on the Windows test host, before any code**, each on a
  throwaway task in a folder of the probe's own, deleted after:
  * **P1:** `put_Enabled(false)`, then `get_Xml`: is `Settings/Enabled`
    `false`? And after `put_Enabled(true)`, absent or `true`? This
    settles F16 and D5.
  * **P2:** on a disabled task, `RegisterTaskDefinition` with
    `TASK_CREATE_OR_UPDATE` and a definition that leaves
    `Settings/Enabled` out: is the task enabled after? This settles F17,
    for `Register`'s documentation.
  * **P3:** `put_Enabled` with the value the task has: does it succeed,
    and does it change `get_Xml` or `LastRunTime`? This settles F13.
  * **P4, if the host allows it:** a task registered by an elevated
    shell for the user, then `put_Enabled` from a medium-integrity
    process: `0x80070005`? This settles F12's [unverified].
  * **P5:** `get_State` of a task disabled while its program runs:
    `TASK_STATE_RUNNING` or `TASK_STATE_DISABLED`? This settles F6's
    [unverified], and with it D6-C's weakness.
* **Fake tests.** These run on every OS:
  * `Enable` and `Disable` write only on a change.
  * They clear `Stop`'s record, so a `Stop`, then `Disable`, then
    `Start` returns `ErrTaskDisabled` and writes nothing.
  * `Enable` after a `Stop` made by another `Task` lets `Start` run the
    task (the issue's case).
  * A missing task is `ErrNotInstalled`, a denied write
    `ErrPermission`, and a done `ctx` is returned before any call.
  * `Status.Disabled` follows the switch while the program runs.
  * Under D5-B, `DiffDefinitions` reports no `Settings/Enabled`, and
    still reports every other `Settings` element.
* **Live tests** on the Windows test host and CI's `windows-2025`:
  * `Disable` on a running task leaves its program running, and the
    task disabled.
  * A second `Task` value's `Start` fails with `ErrTaskDisabled` after
    the first's `Stop`, and succeeds after its `Enable`.
  * `Status.Disabled` reads true while the program runs, then false.
  * `TestLiveTaskStopRefusesDisabled` disables through `Disable`.
* **Seen failing:** each new test, on a planted break in a scratch copy
  (`scripts/plant-copy.sh`). The breaks: no record cleared, an
  unconditional write, the comparison not skipping the element, and the
  field not set.
* **`make apicheck`** against `v1.20.0`: compatible. `go doc` shows the
  additions.

## Pros and Cons of the Options

### D1-A. `Enable` and `Disable`

* Good, because each call site reads as its operation, like `Stop` and
  `Start`, and as the issue proposes.
* Good, because each documents its own effects: `Disable` leaves the
  program running, and `Enable` lets `Start` run the task.
* Neutral, because it is two methods where one would do.

### D1-B. `SetEnabled(on)`

* Good, because it is one method, and convenient where `on` comes from a
  flag.
* Bad, because it sits beside `Enabled()`, which reports something else
  (F6). A reader expects them to be a pair.
* Bad, because `SetEnabled(ctx, product, false)` reads worse than
  `Disable(ctx, product)`.

### D1-C. Both

* Good, because every caller gets the form it likes.
* Bad, because there are two spellings of one operation to document and
  test.

### D1-D. A service-wide interface

* Good, because a program could switch any backend's service the same
  way.
* Bad, because the switch means something different on each manager.
  systemd's `disable` decides boot start and leaves a manual start
  working. A disabled task refuses even a manual run (F10). A shared
  name would promise a sameness that is not there.
* Bad, because no consumer asks for it, and it would cost three
  backends' work and live tests.

### D2-A. `Start` unchanged

* Good, because 0022-MADR Amendment A1's guarantee holds.
* Good, because nothing in the update's sequence changes.
* Neutral, because a setup's "start" makes two calls, `Enable` and
  `Start`.

### D2-B. `Start` enables any disabled task

* Good, because one call starts a task in any state.
* Bad, because an update's `Start` would undo an administrator's
  disable, which 0022-MADR Amendment A1 rejected.
* Bad, because the triggers then run a task the user had turned off.

### D2-C. `Stop`'s record survives the process

* Good, because a later process's `Start` behaves as the same process's
  does.
* Bad, because `Stop` would write into the user's task definition or
  file system, and `DiffDefinitions` or a cleanup would have to know
  about it.
* Bad, because a record left by a crash can name a disable that someone
  has since made on purpose.

### D3-A. Both clear the record

* Good, because the switch is then always the caller's last decision.
* Good, because `Stop`, then `Disable`, then `Start` keeps the task
  disabled, as the caller asked.

### D3-B. Only `Enable` clears it

* Good, because it is the issue's proposal word for word.
* Bad, because `Start` would then re-enable a task the caller disabled
  after a `Stop`.

### D3-C. Neither

* Bad, because a stale record outlives the switch it described, and
  `Start` acts on it.

### D4-A. `Disable` leaves the program running

* Good, because it is what Task Scheduler does, and `Stop` already
  covers ending the program.
* Bad, because a program disabled while it runs makes a later update's
  `Stop` refuse (F2) until it is stopped or enabled. The documentation
  says so.

### D4-B. `Disable` ends the program

* Good, because the task and its program agree afterwards.
* Bad, because it duplicates `Stop` without `Stop`'s waits,
  `RequestStop` or the refusal from inside the task.

### D5-A. No change to `DiffDefinitions`

* Good, because `v1.19.0`'s behaviour holds.
* Bad, because every caller that compares after a `Stop` must know to
  drop `Settings/Enabled`, or it re-registers and re-enables (F17).

### D5-B. Stop comparing `Settings/Enabled`

* Good, because "same definition" then means the same rendered
  definition, whatever state `Stop`, `Enable` or `Disable` left.
* Good, because the skip has precedent in the skipped elements that Task
  Scheduler, not the program, sets.
* Bad, because a program that renders `Enabled false` is not told when
  the task was enabled. `Status.Disabled` tells it.
* Bad, because it changes what an exported function returns within
  `v1`.

### D5-C. A variant that skips named paths

* Good, because `DiffDefinitions` is unchanged, and a caller chooses what
  to skip.
* Bad, because the safe call is the new one, and a caller who uses the
  old one still re-enables a stopped task.
* Bad, because it adds API surface for one element.

### D6-A. `Status.Disabled`

* Good, because one read gives the switch with the rest of the state,
  and the field is exported and JSON-tagged like its neighbours.
* Neutral, because the field is named for the off state, so that a zero
  `Status` (not installed) reads naturally.

### D6-B. `Task.Switch`

* Good, because it is a direct answer.
* Bad, because it is a second open for a value `Status` reads anyway.

### D6-C. Nothing

* Good, because it adds nothing, if `Status.State` reads `disabled` for
  any disabled task.
* Bad, because the fake keeps a disabled running task `running`, and if
  P5 shows Task Scheduler does too, the answer is wrong exactly when it
  matters (F6).
* Bad, because a state name is a weaker contract than a field named
  for the switch.

## More Information

* Issue: <https://github.com/maccavelli/go-selfupdate-lib/issues/8>.
* Records:
  [0021-MADR](0021-MADR-task-scheduler-lifecycle-and-windowless-children.md)
  (D3, D4; F7, probe 8; Amendment A4),
  [0022-MADR](0022-MADR-documentation-pass-after-v1-14-0.md) Amendment
  A1, [0024-MADR](0024-MADR-documentation-pass-after-v1-15-1.md) D4-A,
  and
  [0032-MADR](0032-MADR-task-scheduler-registration-and-one-shot-handoff.md)
  (D3-A, D4-A; Amendment A1, P1-7).
* Microsoft: `IRegisteredTask::put_Enabled`, `ITaskSettings::Enabled`,
  `TASK_STATE`, and the Task Scheduler schema's `settingsType`.
* Plan:
  [0034-PLAN-export-task-scheduler-enable-and-disable.md](0034-PLAN-export-task-scheduler-enable-and-disable.md).
  It runs the probes first, and stops at a probe result that changes
  D5, D6 or F12.
