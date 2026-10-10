---
status: accepted
date: 2026-10-10
decision-makers: go-selfupdate-lib maintainers
consulted: "magic-cli-remote's docs/decisions/0159-MADR-the-windows-service-path-is-half-wired.md and its PLAN (measured on a Windows 11 host, 2026-09-19 to 2026-09-20); magic-cli-remote's docs/decisions/0054-MADR-standardize-self-update-on-go-selfupdate-lib.md (proposed, 2026-10-10); Microsoft's Task Scheduler 2.0 schema and COM reference; golang.org/x/sys v0.47.0 source; this module at 31cce88"
informed: the fleet's programs that run on Windows, magic-cli-remote first
---
# Add a Task Scheduler reference lifecycle built on magic-cli-remote's measured Windows behaviour, and start the library's own Windows children without a window

## Context and Problem Statement

This module has three reference lifecycles: `systemd`, `launchd` and `scm`
([0011-MADR-reference-service-lifecycles.md](0011-MADR-reference-service-lifecycles.md)).
On Windows it supports only a program installed as a Win32 service under the
Service Control Manager. That needs an administrator to install, and the
service runs in session 0 with no interactive desktop.

The fleet's one Windows background program is not an SCM service.
magic-cli-remote runs its daemon as a per-user, at-logon Task Scheduler task,
which needs no elevation. Its decision record explains why it chose a task
over a service (no elevation; a provider CLI's browser sign-in needs the
user's desktop). Record 0011 left that lifecycle in the consumer:

* 0011 §1: "`schtasks` stays in magic-cli-remote behind `Lifecycle`, as 0004
  says" [read];
* [0004-MADR-evolve-selfupdate-api-and-tui-support.md](0004-MADR-evolve-selfupdate-api-and-tui-support.md):
  "schtasks stays in magic-cli-remote behind `Lifecycle`" [read].

Two magic-cli-remote records now bear on that choice.

* **Its 0159 records found the task lifecycle was half-wired, measured why,
  and fixed it across three releases.** That work is
  `0159-MADR-the-windows-service-path-is-half-wired.md` and its PLAN, status
  `in-progress`; only P12 is left. Most of what it measured is a property of
  Windows and Task Scheduler, not of that product.
* **Its 0054 record proposes adopting this module's whole stack,** but keeps
  the Windows adapter as product code. Its words: "the repo's adapter stays
  as the Windows `Lifecycle`, grows an `Enabled` probe (the task's at-logon
  presence) … The library's `scm` backend is for Windows *services* and is
  not adopted." It names the cost: "the Windows adapter is still this
  repository's to finish." 0054 is `proposed`, dated 2026-10-10 [read].

This record asks which of 0159's findings belong in this module, and in what
form. It decides two things:

* whether a Task Scheduler lifecycle becomes a fourth reference backend
  here;
* whether this module's own Windows process starts change, because 0159
  measured something that applies to them.

Each claim below is marked with how it was established:

* **[read]**: read in this tree at `31cce88`, or in a cited record or source;
* **[measured upstream]**: measured by magic-cli-remote on its Windows 11
  host and recorded verbatim in its 0159 records; not re-run for this
  record;
* **[docs]**: Microsoft's documentation;
* **[derived]**: reasoned from the kinds above, and not run;
* **[unverified]**: open, and named for the PLAN to measure first;
* **[measured]**: measured on the Windows test host by the PLAN's T1,
  after acceptance (Amendment A1).

No probe was run for this record. The tree was not written, apart from this
file and its index row.

### What magic-cli-remote 0159 measured, and whether it is about Windows or about that product

| 0159 finding | What was measured | Windows property or product defect | Bearing here |
| :--- | :--- | :--- | :--- |
| F1 | On Windows, `update` rolled back every time: reconcile ran `setup-service --refresh`, which refused off Linux and macOS | product | none directly. It is the gap 0054 leaves open while the adapter stays product code |
| F7, probe 8 | `RestartOnFailure` `PT1M`×3 did not restart a program that exited 1, exited 0, or was ended: one run in 4 min 42 s. A repeating `TimeTrigger` with `IgnoreNew` relaunched within a minute, never started a second instance, and stopped relaunching once the task was disabled [measured upstream] | Windows | **yes**. Microsoft's page says the element restarts "if the task fails for any reason" [docs], which the measurement contradicts. A task that relies on a watchdog trigger must be **disabled** before it is stopped, or Task Scheduler relaunches it while its binary is being swapped |
| F8 | `schtasks /end` is a `TerminateProcess`, with no drain [measured upstream] | Windows | **yes**. A task has no graceful stop of its own |
| F9, probe 9 | The task XML refuses an `<Environment>` element ("unexpected node") [measured upstream]; the `Exec` schema has only `Command`, `Arguments` and `WorkingDirectory` [docs] | Windows | context only. This module does not render definitions |
| F10, F22–F27, probes 6–10 | A console program started by a task gets a console, handed to Windows Terminal. A child of a console-less parent started with only `CREATE_NEW_PROCESS_GROUP` gets a **visible** window. `DETACHED_PROCESS` moves that window down to the grandchild. `CREATE_NO_WINDOW` gives a windowless console, which grandchildren started with no flags inherit [measured upstream] | Windows | **yes**: see "This module's own Windows process starts" below |
| F11, probe 10 | The task's principal came from `USERDOMAIN\USERNAME`, which any shell can override. Task Scheduler stores the principal as a SID [measured upstream] | product defect, built on a Windows fact | **yes**, wherever this module registers a definition |
| F12, probe 11 | `schtasks /query` status parsing matched English text. `Get-ScheduledTask` gives a numeric `State` (4 is running) but takes 1261–2375 ms per call, against 53 ms for `schtasks` [measured upstream] | Windows | **yes**. The Task Scheduler API's `TASK_STATE` is numeric: 0 unknown, 1 disabled, 2 queued, 3 ready, 4 running [docs] |
| F18 | Rolling back a refreshed definition runs the **old** binary's restore code, which knew nothing of Task Scheduler [derived upstream] | product, of a general shape | **yes**. A restore owned by this module runs code that knows the backend |
| F19 | Task Scheduler rewrites a registered definition: it drops defaults, stores the SID, and adds `URI`, `IdleSettings` and `UseUnifiedSchedulingEngine`. A text comparison never matches [measured upstream] | Windows | **yes**, for any comparison this module makes |
| PLAN execution record, item 11 | A `schtasks /xml` export read through a pipe is 8-bit text, so a non-ASCII path is mis-decoded [measured upstream, consequence unverified there] | Windows tool | **yes**: prefer an interface that carries UTF-16 |
| F24, F32, F35 | A console-less parent cannot send `CTRL_BREAK` directly. `AttachConsole`, then `GenerateConsoleCtrlEvent`, then `FreeConsole` can, without signalling itself. `CTRL_BREAK` arrives in Go as `SIGINT` [measured upstream] | Windows | context. This module does not stop a program's children |
| F30, F31, probes 12–13 | An argument `a&calc` passed to an npm `.cmd` shim **ran Calculator**: `CreateProcess` supplies `cmd.exe` for a batch file. Quoting through `cmd.exe /d /s /v:off /c` made everything inert except `%` [measured upstream]. This is the BatBadBut class (CVE-2024-24576) | Windows and Go | **yes, as a guard**. This module never means to run a batch file |
| F4, F15, F16, F13, F20 | socket identity, atomic-write ACLs, the acceptance script, the signing claim, the installer's PATH notice | product | none, except F20's measured hazard, noted under Not chosen |

### What this module does on Windows today

* **`scm` covers SCM services only** [read]. Through `x/sys/windows`
  (`selfupdate/service/scm/scm_windows.go`) it:
  * opens each handle with only the rights its call needs;
  * reads numeric states;
  * waits by Microsoft's checkpoint loop;
  * rewrites `BinaryPathName` with `ChangeServiceConfig`, under
    `Options.RewritePath`;
  * walks this process's ancestors for `Inside`, comparing creation times;
  * detaches through a hop (0011 amendment A4).

  None of 0159's task findings applies to it: SCM states are numeric, and
  `SERVICE_CONTROL_STOP` is a real stop.
* **A program whose lifecycle has no backend here** is pointed at
  `service.ProcessDetacher(inside)` "with your own check of whether this
  process is inside", by `docs/guides/extending-selfupdate.md:630-633`
  [read]. That is magic-cli-remote's position today.
* **This module's own Windows process starts** [read]:

  | Site | What it starts | Creation flags |
  | :--- | :--- | :--- |
  | `selfupdate/probe.go:99` (`versionProber.Probe`) | the staged or installed binary, to read its version | none |
  | `selfupdate/service/runner.go:67` (`runExec`, used by `ExecReconciler`) | the new binary's reconcile and restore commands | none |
  | `selfupdate/service/detach_windows.go:15,30` (`startDetached`) | the handoff's hop, and the hop's real run | `DETACHED_PROCESS \| CREATE_NEW_PROCESS_GROUP`, and `CREATE_BREAKAWAY_FROM_JOB` when the job allows it |

  The detached handoff run therefore has no console. When it probes the new
  binary or runs `ExecReconciler`, each child is a console program started
  with no flags by a console-less parent. 0159 measured that this case gives
  a visible window (probe 8, row 3) [measured upstream]. In session 0 nobody
  sees it. **For a program that runs in the user's session, such as a
  per-user task's handoff, each child of the handoff flashes a console
  window** [derived; not run against this module].
* **`x/sys/windows` v0.47.0 exports** `CREATE_NO_WINDOW`,
  `GenerateConsoleCtrlEvent`, `CoInitializeEx`, `CoUninitialize`,
  `LookupSID`, `SID.LookupAccount`, `GetCurrentProcessToken` and
  `JOB_OBJECT_LIMIT_BREAKAWAY_OK`. It **does not** export `FreeConsole`,
  `AttachConsole`, `AllocConsole`, `GetConsoleProcessList`,
  `GetConsoleWindow`, `CoCreateInstance`, `SysAllocString` or
  `VariantInit` [read, by search of the module cache's source]. No Task
  Scheduler binding exists in `x/sys`.
* **The rules on adding packages and modules** [read]:
  * `AGENTS.md`: no module without a record that names it;
  * `AGENTS.md`: a new package gets its own depguard rule, whose name sorts
    after `banned`;
  * `.golangci.yml`: `service-scm` allows the standard library,
    `x/sys/windows`, `svc`, `svc/mgr`, `selfupdate` and
    `selfupdate/service`.

### What a Task Scheduler backend could rest on

* **The COM API** (`ITaskService` → `ITaskFolder::GetTask` →
  `IRegisteredTask`) [docs]:
  * `get_State` gives `TASK_STATE`;
  * `get_Enabled` and `put_Enabled` read and set the task's switch;
  * `Run` starts it;
  * `Stop` "stops all instances of the task". A user can stop instances that
    run under the user's own credentials;
  * `GetInstances` gives `IRunningTask`, whose `get_EnginePID` is "the
    process ID for the engine (process) which is running the task";
  * `get_Xml` returns the definition as a UTF-16 string, and
    `ITaskFolder::RegisterTask` takes one;
  * `get_Definition` returns the definition as an object, whose actions can
    be edited and registered again with
    `ITaskFolder::RegisterTaskDefinition`;
  * `get_LastTaskResult` and `get_LastRunTime` describe the last run.

  Reaching it from Go without cgo means `ole32!CoCreateInstance` through a
  lazy procedure, and calling the interfaces through their vtables with
  `syscall.SyscallN`, because `x/sys` has neither [derived].
* **The tools.** `schtasks.exe` for create, change, run and end, and
  PowerShell's `Get-ScheduledTask` for a locale-free state, at 1.3 to 2.4 s
  a call [measured upstream]. The export comes through a pipe as 8-bit text
  [measured upstream].
* **What is not known.**
  * Whether `get_EnginePID` is the action's own process for an `Exec`
    action, or a host process [unverified].
    *(Settled by Amendment A1: it is the action's own process [measured].)*
  * Whether Task Scheduler's job object allows breakaway, already in 0011's
    "Not verified" [read] and still open [unverified].
    *(Settled by Amendment A1: it does not; the job has no limit flags at all
    [measured].)*
  * Whether `Stop` kills the task's descendants as well as its action
    process [unverified].
    *(Settled by Amendment A1: it does not; it ends the action's process only
    [measured].)*

## Decision Drivers

* **D-a: one maintained implementation.** 0054 adopts this module's stack
  everywhere except the Windows lifecycle, and calls that adapter "still
  this repository's to finish". A lifecycle written once, here, ends the
  only private lifecycle left in the fleet.
* **D-b: rest on measurement, not on documentation.** 0159 found one
  documented behaviour (`RestartOnFailure`) that does not happen, and one
  obvious API (`GetConsoleWindow`) that gives the wrong answer under ConPTY.
  The backend's contracts must cite the measurement, and the PLAN must
  re-measure each one live before relying on it.
* **D-c: no elevation and no new module.** A per-user task needs no
  administrator (0159 D-g). AGENTS.md forbids a module no record names, and
  `x/sys` is already required.
* **D-d: keep 0011's model.** A backend manages a definition the program
  installed. It does not create one, and a definition change stays the
  program's, through `ExecReconciler`. The exception is the narrow path
  rewrite that `scm.Options.RewritePath` already sets the precedent for.
* **D-e: make no visible windows, and run no batch file.** This module's own
  process starts should not flash windows in a user's session, and should
  never put a `.cmd` or `.bat` file under `cmd.exe`'s parser.
* **D-f: additive API.** `make apicheck` must report the release compatible.
  Nothing in `scm`, `systemd` or `launchd` changes its contract.

## Considered Options

* **A. A `taskscheduler` reference backend here, on the COM API, plus
  windowless children.** It carries 0159's Windows findings into the backend
  and into this module's own process starts. magic-cli-remote's adapter
  retires when it adopts the backend.
* **B. Keep the lifecycle in the consumer; fix only this module's own
  process starts.** That is 0011's line plus the console fix, with 0159's
  findings written into `docs/guides/extending-selfupdate.md` for a consumer
  to follow.
* **C. Export building blocks, not a backend.** Exported helpers: a numeric
  task-state probe, a semantic definition comparison, a token-derived
  principal, and a console-aware command constructor. Each consumer
  assembles its own `Lifecycle`.
* **D. Steer Windows consumers to `scm`.** Document that a background program
  on Windows should be a service, and offer nothing for a task.
* **A′. As A, but on `schtasks.exe` and PowerShell instead of COM.** This is
  a variant of A's transport only, weighed under D2.

## Decision Outcome

Chosen option: "A. A `taskscheduler` reference backend here, on the COM API,
plus windowless children". Four reasons:

* It ends the last private lifecycle in the fleet (D-a).
* Every contract it needs was measured by 0159 on real hardware (D-b).
* It needs no elevation and no module (D-c).
* It fits 0011's backend model with no change to the other three backends
  (D-d, D-f).

B leaves 0054's named gap in place, and leaves a guide describing contracts
that nothing enforces. C ships the parts without the assembly, and the
assembly is where 0159's defects were: a stop that does not disable, a
restore that runs old code. D contradicts the measured reasons a task was
chosen.

This amends 0011 §1 and 0004's line: a Task Scheduler lifecycle now belongs
here. Both get an additive pointer in the PLAN's records phase.

The owner accepted this record on 2026-10-10, answering Q1 to Q4 as
recommended and Q5 against the recommendation: everything ships in one
minor release (More Information, "Owner decisions").

### D1. Package and shape

* New package **`selfupdate/service/taskscheduler`**. `New(Options)`
  returns a `*Task` that implements:
  * `selfupdate.Lifecycle`;
  * `selfupdate.EnabledLifecycle`;
  * `selfupdate.Reconciler`;
  * `service.Detacher`.

  It compiles on every OS and returns `service.ErrUnsupported` off Windows,
  as `scm` does.
* The name is `taskscheduler`, not `schtasks` (Q4): it names the service,
  as `scm` does, not a tool the backend does not run.
* `Options`:
  * `Name`: the task's name. Empty means the product.
  * `Folder`: the task folder. Empty means `\`.
  * `Probe`, `Poll`, `RewritePath`: with `scm`'s meaning.
  * `RequestStop` and `StopGrace` (D4).

  A name holding `\` or `/` is refused, as `scm.validName` refuses one.
* The Task Scheduler calls sit behind a small unexported `scheduler` interface,
  faked in tests, as `scm` does with `manager`. The real one lives in
  `scheduler_windows.go`.
* depguard gains `service-taskscheduler`, allowing the standard library,
  `golang.org/x/sys/windows`, `selfupdate`, `selfupdate/service` and
  `internal/winproc` (D6, D7). It is excluded from `other-packages`. AGENTS.md's scope paragraph names the
  package.
* The backend **never creates, renders or deletes a task.** The program's
  setup does that. A definition change is the program's, through
  `service.NewExecReconciler`, as 0054 already plans (D-d).

### D2. Transport: the Task Scheduler COM API through `x/sys/windows`

* `CoInitializeEx` on a locked OS thread. `CoCreateInstance` of
  `CLSID_TaskScheduler` through a lazy `ole32` procedure, then
  `ITaskService::Connect`, then vtable calls through `syscall.SyscallN`.
  BSTRs come from lazy `oleaut32` procedures. No cgo and no module.
* Why COM, against A′ (`schtasks.exe` and PowerShell):
  * the state is the numeric `TASK_STATE`, with no text to localise (0159
    F12) and none of PowerShell's 1.3 to 2.4 s a call (probe 11);
  * the definition is UTF-16 both ways, so a non-ASCII path survives a
    rewrite and a restore (PLAN item 11);
  * errors are HRESULTs, which map to the typed errors:
    * `0x80070002` (file not found) to `ErrNotInstalled`;
    * `0x80070005` (access denied) to `ErrPermission`.

    Neither needs parsing a tool's localised message;
  * there is no tool path to resolve and no tool environment to build.
* Cost, stated: hand-written vtable code, with interface layouts copied from
  `taskschd.h`. The layout is pinned by a Windows test that calls every
  method the backend uses against a throwaway task, so a wrong slot fails a
  test, not a user. The owner chose COM (Q1).

### D3. Probes

* **`Installed`**: `GetTask` succeeds. Not found gives false, with no error.
* **`Running`**: `TASK_STATE_RUNNING`, or `GetInstances` reports an instance.
  This follows 0011 §2: Running means a process may hold the image.
* **`Enabled`**: the task's `Enabled` is true **and** it has at least one
  enabled trigger that starts it with nobody acting: logon, boot, or a time
  trigger with a repetition. A task enabled with only a one-off time trigger
  does not count. This is 0054's "at-logon presence", generalised. The
  owner chose this rule (Q2).
* **`WaitHealthy`** waits, as `scm`'s does, until the task runs as an
  instance other than the one `Stop` recorded, and stays that instance for
  the settle window. It then runs `Options.Probe`. It fails at once with
  `service.ErrUnhealthy` when the task is no longer running and its last
  run began at or after the `Start`. The detail carries
  `IRegisteredTask::get_LastTaskResult`, the result Task Scheduler
  recorded for that run [docs]. Whether that is the program's exit code
  for an `Exec` action is [unverified].
  *(Settled by Amendment A1: it is, 7 for an exit of 7 and 0 for 0 [measured].)*
* **The instance identity** for `WaitHealthy` is the running instance's
  process ID.
  * It is `IRunningTask::get_EnginePID` if that is the action's process.
  * Otherwise it is the process from a snapshot whose image is the task's
    `Command`, cleaned, compared case-insensitively and then by
    `service.SameExecutable`, and whose parent is the instance's engine.

  Which of the two holds is [unverified], and is the PLAN's first probe. The
  record is amended with the answer before any code depends on it.
  *(Settled by Amendment A1: `get_EnginePID` is the action's process, so the
  first rule holds and the snapshot rule is not built [measured].)*

### D4. Stop and Start

* **`Stop`** does this, in order:
  1. Refuses with `ErrInsideService` when this process descends from the
     task's instance (D6).
  2. Records the instance's process ID.
  3. **When the task is enabled, disables it and records that it did.**
     0159 measured a repeating trigger relaunching an ended task within a
     minute, and that disabling stops it (probe 8); `/change /disable`
     needs no elevation for a user's own task [measured upstream].
  4. Calls `Options.RequestStop(ctx, pid)` when it is set. This is the
     program's own polite stop, such as an admin-socket shutdown, because a
     task has none (0159 F8). It waits for the process to exit, up to
     `Options.StopGrace` (default 10 s). An error from `RequestStop`
     ends the grace at once and goes on to step 5. It is part of
     `Stop`'s error only when step 5 also fails, because the task is
     stopped either way and the update can go on.
  5. Calls `IRegisteredTask::Stop`, then waits until no instance runs and
     the recorded process has exited. The wait is bounded by `Poll`'s
     timeout, and once begun it runs whatever the caller's context does,
     as `scm` does (0015-MADR B3).
* **`Start`** re-enables the task when this `Task` disabled it, then calls
  `Run`. A task that is already running is left alone.
* **Residual, documented.** A process killed between the disable and the
  re-enable, for example by power loss, leaves the task disabled. The
  managed update's rollback calls `Start`, so only a hard crash leaves it.
  The guide names the recovery: the program's setup, or
  `schtasks /change /tn <name> /enable`. 0159 D5 carries the same residual.
* **`RequestStop` and `StopGrace` are new API with no counterpart in `scm`.**
  The owner chose to ship them in the first release (Q3).

### D5. Reconcile and Restore

* **`Reconcile`** reads the definition and refuses one that has anything but
  exactly one `Exec` action. It compares `Command` with the new executable
  by `scm`'s `samePath` rule: cleaned, case-insensitive, then by file
  identity through `service.SameExecutable` (0011 A5). Equal gives
  `Changed: false`. `samePath` is one unexported line in `scm`, so the
  backend carries its own copy; `internal/winproc` cannot hold it, because
  it would import `service`, which imports it (D7).
* **Under `RewritePath`** it changes `Command` alone and keeps every other
  element as Task Scheduler returned it. It reads the definition object
  (`IRegisteredTask::get_Definition`), sets the one `IExecAction`'s `Path`,
  and registers the object with `ITaskFolder::RegisterTaskDefinition`
  (`TASK_UPDATE`) under the task's existing logon type [docs]. This
  sidesteps F19's normalisation, because nothing is compared, parsed or
  rendered as text.
  * When registration needs a user, the user is the **process token's SID**.
    `USERNAME` and `USERDOMAIN` are never read (0159 F11). Whether an
    interactive-token task's update needs a user at all is [unverified].
    *(Settled by Amendment A1: it needs none. With no user, the principal and every
    other element are unchanged, byte for byte, and the token's SID is
    not read [measured]. Other logon types were not measured: owner
    question Q6.)*
  * The receipt's `State` is the definition as it was: the XML `get_Xml`
    returned, kept as a Go string. It never passes through an 8-bit pipe.
* **`Restore`** re-registers that saved XML with `ITaskFolder::RegisterTask`
  (`TASK_UPDATE`). Because the restore is
  this module's code in the process running the update, the old binary
  knows how to undo it. This closes F18's class for a library-owned
  rewrite. A program's own definition change, run through
  `ExecReconciler`, keeps the restore the program supplies.

### D6. Inside and Detach

* **`Inside`** walks from this process up its recorded parents to the
  instance's process ID, with `scm`'s creation-time check. The walk moves
  from `scm/detach.go` to `internal/winproc`, as a function over a process
  snapshot and a creation-time lookup. Each backend supplies both through
  its own faked interface, as `scm` does today. An unexported function in
  `selfupdate/service` could not be called from either backend, and
  AGENTS.md puts helpers shared between packages under `internal/`.
  `scm`'s behaviour does not change.
* **`Detach`** starts the hop, as `scm.Detach` does (0011 A4).
* **Two facts decide whether this is enough**, and both are [unverified]:
  * whether `IRegisteredTask::Stop` kills the instance's descendants;
  * whether a process inside a task's job may break away.

  The PLAN measures both before it implements D6.
  * If `Stop` kills only the action's process, and the hop breaks away,
    D6 stands.
  * If breakaway is refused, the record is amended. The rejected
    alternatives in 0011 A4 are the starting point.

  *(Settled by Amendment A1: breakaway is refused, but D6 stands. `Stop` ends
  only the action's process, and the job has no kill-on-close limit, so
  the hop's run survives inside the job, as a plain child does
  [measured]. 0011 A4's alternatives are not needed.)*

### D7. This module's Windows children start without a window

* **One constructor, in a new package `internal/winproc`, for every
  process this module starts.** It applies `CREATE_NO_WINDOW` **when this process has no console**, so a
  child gets a windowless console that its own children inherit (0159 F25,
  probe 8 row 2). When this process has a console, the child inherits it,
  exactly as today.
  * The constructor covers `selfupdate`'s `versionProber`, and
    `selfupdate/service`'s `runExec` and `startDetached`. The Unix
    `startDetached` goes through it too, where it is plain `exec.Command`.
  * `internal/winproc` imports only the standard library and
    `golang.org/x/sys/windows`. It gets its own depguard rule,
    `internal-winproc`, and leaves `other-packages`. The `selfupdate`,
    `service`, `service-scm` and `service-taskscheduler` rules allow it.
  * "Has a console" is `GetConsoleProcessList` returning non-zero, through
    a lazy `kernel32` procedure. It is **not** `GetConsoleWindow`, which
    returns 0 under ConPTY for a process that has a console (0159 D17,
    measured upstream). The answer is read once and kept.
* **`startDetached` swaps `DETACHED_PROCESS` for `CREATE_NO_WINDOW`.** It
  keeps `CREATE_NEW_PROCESS_GROUP` and the breakaway retry. Under
  `DETACHED_PROCESS` the run has no console, and every console child it
  starts creates a visible one (probe 8 row 3). This amends 0011 §9's flag
  list. Microsoft documents that `CREATE_NO_WINDOW` is ignored with
  `DETACHED_PROCESS`, so the two cannot be combined [docs, process-creation
  flags].
* **A static test bans any other process start.** Over the non-test files of
  `selfupdate/...`, it refuses an `exec.Command`, `exec.CommandContext` or
  `os.StartProcess` outside `internal/winproc`. Each allowlist entry carries a
  reason, and the test fails on an allowlist entry that has gone stale
  (0159 D19, and its PLAN's lesson that `os.StartProcess` is the bypass one
  layer down). `internal/cmd/` is release tooling that never runs on a
  user's host, and is out of the walk.
* **Pinned on Windows by a three-deep helper-process test.** A parent calls
  `FreeConsole`, then starts a child through the real constructor, and the
  child reports whether it got a console window. The test asserts it did
  not. Before the fix it must be seen to fail.

### D8. No batch file reaches `cmd.exe`

* On Windows, the D7 constructor refuses a command whose resolved path has
  the extension `.bat` or `.cmd`, case-insensitively, so `runExec`,
  `versionProber` and `DetachProcess` all refuse one. The error names the
  BatBadBut class.
* "Resolved" means `exec.Cmd.Path` after `os/exec` has built the command,
  so an extension it adds from `PATHEXT` is caught as well [derived from
  `os/exec`; the PLAN tests an extensionless path with a `.cmd` beside
  it].
* Every path these functions receive is this module's own staging file or a
  target binary, so nothing legitimate is refused [derived]. The guard stops
  a misconfigured target from putting an argument under `cmd.exe`'s parser
  (0159 F30) [measured upstream].
* This module does not take on 0159 D23's quoting scheme, because it has no
  reason to run a batch file at all.

### D9. Not ported, and why

Each item is a product concern here, or already decided elsewhere:

* **Rendering and creating a task:** its triggers, its watchdog repetition,
  its principal element, and semantic comparison for setup's idempotency
  (0159 D5's trigger shape, D8, D14). The program's setup owns the
  definition (D-d). D5 here never compares text, so F19 does not arise.
* **Delivering environment to a task** (0159 D6). The schema cannot carry it
  [docs].
* **`FreeConsole` in a daemon, and polite stops of a program's children**
  (0159 D7, D22). That is runtime behaviour of the program, not of an
  update. D4's `RequestStop` is the update's only point of contact.
* **Logging, signing and the acceptance scripts** (0159 D3, D10, D11).

### Consequences

* Good, because magic-cli-remote can drop its adapter, and 0054's "still
  this repository's to finish" goes away. Its Windows update then runs:
  * a `Stop` that disables before it ends;
  * a numeric state probe;
  * a health wait that demands a new instance;
  * a restore that knows Task Scheduler;
  * a handoff.

  Each is pinned by tests here.
* Good, because a handoff run in a user's session no longer flashes a window
  for each probe or reconcile child, and no library process start can run a
  batch file.
* Good, because the API is additive: a new package, two new depguard rules,
  an internal package, and a moved unexported walk. `scm`, `systemd` and `launchd` keep their
  contracts, and `apicheck` reports a minor release.
* Bad, because the COM transport is hand-written vtable code. That is new
  ground for this module, and only a Windows test can check it.
* Bad, because five facts the design rests on are unmeasured. Three of
  them decide its shape: `get_EnginePID`, `Stop`'s reach, and breakaway.
  The other two are `get_LastTaskResult`'s meaning and the user an update
  registration needs. The PLAN measures all five first, and the record
  may need amending before the backend is built.
* Bad, because a task has no graceful stop of its own. `RequestStop` only
  helps a program that supplies it. Otherwise `Stop` terminates, as 0159 F8
  documents.
* Neutral, because restart latency, and the disable left behind after a hard
  crash, stay as 0159 D5 records them. The backend documents them and does
  not hide them.
* Neutral, because everything ships in one minor release, `v1.14.0` (Q5).
  D7 and D8 wait for the backend, so a Windows program gets the console
  fix later than a patch release would have given it.
* Neutral, because 0054's adoption plan changes from "keep the adapter" to
  "adopt `taskscheduler`". That change is magic-cli-remote's record to
  make, not this one's.

### Confirmation

* `make gate` ends `overall=0`, and `make apicheck` reports the release
  compatible with `v1.13.0`.
* depguard's `service-taskscheduler` and `internal-winproc` rules each fail
  on a planted foreign import.
* **Table tests over the fake scheduler** cover:
  * not installed;
  * each `TASK_STATE`;
  * the `Enabled` trigger rule;
  * Stop's call order (disable, then `RequestStop`, then `Stop`, then wait);
  * Start re-enabling only what Stop disabled;
  * Reconcile on a matching and on a moved binary;
  * the rewrite keeping every element but `Command`;
  * Restore re-registering the saved definition;
  * a definition with two actions refused;
  * the HRESULT-to-typed-error map.

  Each new test is seen to fail on a planted break in a scratch copy
  (`scripts/plant-copy.sh`).
* **A live test, gated by environment, on the Windows test host in an
  interactive session.** It registers a throwaway task named
  `selfupdate-live-<random>` with a repeating trigger, and removes it in
  `t.Cleanup`. It never touches another task. It shows:
  * a managed update from the task's process, handed off and completed;
  * the task disabled during the swap, with no relaunch, and re-enabled
    after;
  * a new instance identity;
  * a failing health check rolling back and restoring the definition;
  * no console window created by the handoff's children.

  Whether a GitHub Windows runner can run an interactive-token task is
  [unverified]. Until it is shown, CI skips the live test with a stated
  reason, and `SELFUPDATE_REQUIRE_TASKSCHED=1` makes it mandatory.
* **The D7 three-deep test** fails against today's `startDetached` and
  passes after. The D7 static ban names the file and line of a planted bare
  `exec.Command`.
* **The D8 guard** refuses `x.CMD` and `x.bat`, and accepts `x.exe`.

## Pros and Cons of the Options

### A. A `taskscheduler` reference backend here, on the COM API, plus windowless children (chosen)

* Good, because it ends the only private lifecycle in the fleet, which 0054
  names as unfinished.
* Good, because each contract rests on a measurement 0159 recorded verbatim,
  and the PLAN measures the five that are open.
* Good, because the COM transport removes three of 0159's defect classes at
  once: localised parsing, the cost of PowerShell, and 8-bit exports.
* Good, because it keeps 0011's model: no definition authoring, and a narrow
  rewrite that matches `scm`'s.
* Bad, because the vtable code is new and Windows-only to check.
* Bad, because the backend inherits Task Scheduler's limits: no graceful
  stop, a relaunch latency of about a minute for a watchdog trigger, and
  start at logon rather than at boot.

### B. Keep the lifecycle in the consumer; fix only this module's own process starts

* Good, because it is the smallest change, and 0011 §1 already says so.
* Good, because D7 and D8 still land.
* Bad, because the fleet keeps one hand-written Windows lifecycle, whose
  history (0159 F1, F12, F18, F19) is exactly the class of defect a shared
  backend exists to prevent.
* Bad, because guide text describing measured contracts enforces nothing.

### C. Export building blocks, not a backend

* Good, because each helper is small and useful alone.
* Bad, because 0159's worst defects were in how the parts were assembled: a
  stop that did not disable, a restore running old code, a health wait on
  "running" rather than on a new instance. Building blocks leave that
  assembly to every consumer.
* Bad, because it creates exported API with no backend in this module to
  prove it.

### D. Steer Windows consumers to `scm`

* Good, because it needs no code.
* Bad, because a service needs an administrator to install, and runs in
  session 0, where a provider CLI's browser sign-in cannot reach the user's
  desktop. Those are the reasons magic-cli-remote recorded for choosing a
  task.
* Bad, because it would leave the one Windows program in the fleet without a
  reference lifecycle.

### A′. As A, on `schtasks.exe` and PowerShell

* Good, because tool calls are easier to read than vtable calls, and 0159
  shipped this way.
* Bad, because the state probe costs 1.3 to 2.4 s a call, which makes the
  health wait coarse.
* Bad, because the export is 8-bit through a pipe, so a non-ASCII path can
  be corrupted by a rewrite or a restore.
* Bad, because tool errors are localised text, which cannot map reliably to
  the typed errors.
* Neutral, because 0011 chose the platform tools for systemd and launchd,
  where their output is declared stable. Task Scheduler's tools declare no
  such thing, and its API is the stable interface, as the SCM's is.

## More Information

### Owner decisions

The owner answered on 2026-10-10: "follow recommendations except 5, ship
everything together." The questions as asked, with the answers:

* **Q1. Transport.** COM through `x/sys/windows` (recommended, D2), or
  `schtasks.exe` and PowerShell (A′)? **COM.**
* **Q2. What `Enabled` counts.** An enabled task with a logon, boot or
  repeating time trigger (recommended, D3), or only a logon trigger, as
  0054 words it? **Logon, boot or repeating.**
* **Q3. `RequestStop`.** Ship it in the first release (recommended, D4), or
  leave stops as termination only and add it when a program asks?
  **Ship it.**
* **Q4. The package's name.** `taskscheduler` (recommended: it names the
  service, as `scm` does, not a tool this backend does not run), or
  `schtasks`? **`taskscheduler`.**
* **Q5. Release split.** D7 and D8 first, as a patch release, then the
  backend as a minor release (recommended: they stand alone, and every
  Windows program benefits). Or everything in one minor release?
  **Everything in one minor release, `v1.14.0`.**

### Not verified

* Whether a GitHub Windows runner can run an interactive-token task
  (Confirmation).
* That this module's handoff children flash a window in a user's session
  (Context). This is derived from 0159 probe 8 and this module's flags, and
  has not been run against this module. D7's three-deep test measures it
  before the fix.
* Whether golang/go#68313 was fixed in `syscall`. The issue is closed; the
  search for this record did not show how it was resolved. D8 does not
  depend on the answer.
* Whether a rewrite of a task whose logon type is not `InteractiveToken`
  keeps its principal (D5). Until a probe shows it, the rewrite refuses
  such a task (owner question Q6, decided).

The five items this list held for the PLAN's probes are settled by
Amendment A1.

### Amendment A1 (2026-10-10): probe results

Measured by 0021-PLAN T1 on the Windows test host (Windows 11, build
26200), from a driver task at `Medium Mandatory Level` in the console
session (session 1), as the PLAN's Deviation D1 records. Each probe task
was a per-user task with an `InteractiveToken` principal at least
privilege, a logon trigger, `IgnoreNew` and one `Exec` action running a Go
probe program. Three complete runs agreed on every measured value; the
PLAN's T1 record quotes the third, and says why the first two misreported
one driver-side comparison.

| Probe | Question | Result | Effect |
| :--- | :--- | :--- | :--- |
| 1 | What `get_EnginePID` names | The action's own process: `enginepid_is_probe=True`, `engine_image=taskprobe`. Its parent is `svchost.exe`. `State` is 4 while it runs and 3 after `Stop` | D3: the instance identity is `get_EnginePID`. The snapshot rule is not built |
| 2 | `Stop`'s reach | `Stop` ended the action's process. Its plain child and its detached child were both still alive 5 s later, checked by PID and creation time | D6 stands. `Stop` leaves a program's own children running: the guide says so, and a program that needs them stopped does it in `RequestStop` |
| 2 | Breakaway | The job's limit flags are `0x00000000`: no `BREAKAWAY_OK`, and no `KILL_ON_JOB_CLOSE`. `CREATE_BREAKAWAY_FROM_JOB` is refused with `Access is denied`, and the retry inside the job succeeds | D6 stands: the hop's run stays in the job and survives `Stop` |
| 3 | `get_LastTaskResult` | 7 after an exit of 7, and 0 after an exit of 0. `LastRunTime` advanced past the `Run` call each time | D3's failure detail is the program's exit code |
| 4 | An update's user | `RegisterTaskDefinition(TASK_UPDATE)` with no user succeeded. The principal stayed the token's SID with `InteractiveToken`; the trigger's user was unchanged; the XML read back equal, byte for byte, to the old XML with only the `Command` replaced. The task then ran from a directory whose name holds `é`, `ü` and a CJK character. `RegisterTask(TASK_UPDATE)` of the saved XML restored it byte for byte | D5 passes no user and never reads the token's SID. The rewrite and the restore keep a non-ASCII path. Measured for `InteractiveToken` only (Q6) |
| 5 | Error codes | `GetTask` of a missing name: `0x80070002`. An unelevated registration into `\Microsoft\Windows\`: `0x80070005` | D2's map holds as written |

Also seen, and consistent with 0159 F10: the task's console program had a
console with a visible window, which its plain child shared
(`console_procs=2`). The child started with `DETACHED_PROCESS` had no
console at all (`console_procs=0`, `console_hwnd=0x0`), the state D7 says
makes each of its own console children open a window. T2 measures that
last step.

#### Owner question Q6

* **Q6. A rewrite of a task with another logon type.** Probe 4 measured
  `InteractiveToken`, which is how magic-cli-remote registers its task.
  For a task that uses `S4U` or a stored password, an update with no user
  was not measured, and with a password it cannot work, since the backend
  has none. Recommended: `Reconcile` under `RewritePath` refuses a task
  whose logon type is not `InteractiveToken`, with an error naming the
  logon type, until a probe shows otherwise. The probes, `Stop`, `Start`
  and `Restore` of the saved XML are unaffected. The alternative is to
  attempt the update for every logon type and report the HRESULT if Task
  Scheduler refuses it.

  **The owner chose the recommendation on 2026-10-10.** Under
  `RewritePath`, `Reconcile` refuses a task whose logon type is not
  `InteractiveToken` (`TASK_LOGON_INTERACTIVE_TOKEN`, 3), with an error
  naming the logon type. `Reconcile` without `RewritePath`, the probes,
  `Stop`, `Start` and `Restore` are unaffected.

### Not chosen, recorded so it is not re-proposed blind

* **Adding the installer folder to the User `Path`** (0159 D16). It belongs
  to [0014-MADR-shared-installer-templates.md](0014-MADR-shared-installer-templates.md)
  if the owner wants it. 0159 measured the hazard any such change must
  avoid: a default registry read expands `%VAR%`, so writing the value back
  turns `REG_EXPAND_SZ` entries into literals.
* **`-H windowsgui`** for a daemon. 0159 probe 7 measured that it breaks a
  CLI in PowerShell: redirected output is empty, and the exit code is lost.
* **`RestartOnFailure` as a restart policy.** Measured not to restart a
  program that exits (0159 probe 8), whatever Microsoft's page says.

### Sources

* magic-cli-remote: `docs/decisions/0159-MADR-the-windows-service-path-is-half-wired.md`
  (findings F1–F37, probes 1–13, amendments of 2026-09-19);
  `docs/decisions/0159-PLAN-the-windows-service-path-is-half-wired.md`
  (execution records for P1–P20, "What the plan predicted incorrectly");
  `docs/decisions/0054-MADR-standardize-self-update-on-go-selfupdate-lib.md`.
* Microsoft, Task Scheduler:
  * `TASK_STATE`:
    <https://learn.microsoft.com/en-us/windows/win32/api/taskschd/ne-taskschd-task_state>
  * `IRegisteredTask::Stop`:
    <https://learn.microsoft.com/en-us/windows/win32/api/taskschd/nf-taskschd-iregisteredtask-stop>
  * `IRunningTask::get_EnginePID`:
    <https://learn.microsoft.com/en-us/windows/win32/api/taskschd/nf-taskschd-irunningtask-get_enginepid>
  * `RestartOnFailure`:
    <https://learn.microsoft.com/en-us/windows/win32/taskschd/taskschedulerschema-restartonfailure-settingstype-element>
  * `Exec`:
    <https://learn.microsoft.com/en-us/windows/win32/taskschd/taskschedulerschema-exec-actiongroup-element>
* Microsoft, process-creation flags:
  <https://learn.microsoft.com/en-us/windows/win32/procthread/process-creation-flags>
* golang/go#68313, "syscall/exec_windows.go: appendEscapeArg does not escape
  all necessary characters": <https://golang.org/issue/68313>. BatBadBut,
  CVE-2024-24576: <https://v2.cvefeed.io/vuln/detail/CVE-2024-24576>.
* `golang.org/x/sys` v0.47.0, `windows/types_windows.go`,
  `windows/zsyscall_windows.go` and `windows/security_windows.go`, searched
  in the module cache for the exports listed in Context.
* This module at `31cce88`:
  * `selfupdate/probe.go:99`;
  * `selfupdate/service/runner.go:67`;
  * `selfupdate/service/detach_windows.go:15,30`;
  * `selfupdate/service/scm/`;
  * `selfupdate/service/handoff.go:277`;
  * `docs/guides/extending-selfupdate.md:630-633`;
  * `.golangci.yml`'s `service-scm` rule;
  * 0011 §1 and "Not verified";
  * 0004's line 849.

### Related

* [0011-MADR-reference-service-lifecycles.md](0011-MADR-reference-service-lifecycles.md):
  §1 and §9 are amended by D1 and D7 once this record is accepted, and A4's
  hop is reused by D6.
* [0015-MADR-remediate-third-debugging-pass-findings.md](0015-MADR-remediate-third-debugging-pass-findings.md):
  B3's stop-wait rule is reused by D4.
* [0020-MADR-precheck-gofmt-errors-and-replace-before-stop.md](0020-MADR-precheck-gofmt-errors-and-replace-before-stop.md):
  `ManagedOptions.ReplaceBeforeStop` applies to the new backend unchanged.
  Renaming over a running image works on Windows, so a task's binary can be
  replaced before the stop.
* [0021-PLAN-task-scheduler-lifecycle-and-windowless-children.md](0021-PLAN-task-scheduler-lifecycle-and-windowless-children.md): the implementation. Its first
  phase probes the items in "Not verified" on the Windows test host,
  before any code.
