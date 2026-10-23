---
status: accepted
date: 2026-10-23
decision-makers: go-selfupdate-lib maintainers
consulted: "GitHub issues #3, \"service/taskscheduler: register, read and delete a task definition\" (2026-10-22), and #6, \"service/taskscheduler: detach a handoff as a one-shot task, without job breakaway\" (2026-10-23), from the owner for magic-cli-remote; a read of `selfupdate/service/taskscheduler`, `selfupdate/service` and `internal/winproc` at a365dd1 (v1.18.0 plus its closing record); 0021-MADR and its PLANs; magic-cli-remote at 1bfab31b; Microsoft's Task Scheduler 2.0 reference, its schema, the [MS-TSCH] protocol specification and the job-object pages, read 2026-10-23; MinGW-w64's taskschd.h, read 2026-10-23"
informed: the fleet's Windows programs that run as a per-user task, magic-cli-remote first
---
# Let `taskscheduler` register, read, compare and delete a definition the program renders, report a task's state in one call, and hand an update off as a one-shot task of its own, so a run inside a kill-on-close job survives

## Context and Problem Statement

Two issues from the same consumer ask `selfupdate/service/taskscheduler`
for more of what it already does privately.

**Issue #3** asks the package to register, read and delete a task
definition through the COM API it already uses, and to compare a stored
definition with a wanted one. Today a program registers its task with
`schtasks.exe` from a UTF-16 temporary file, reads it back through a pipe
as 8-bit text, compares the two field by field, and reads the task's
state through PowerShell at about 1.3 s a call. That is the tool-output
parsing the package was built to avoid (0021-MADR D2).

**Issue #6** asks for a handoff that does not depend on job breakaway.
`taskscheduler.Task.Detach` starts a hop, which starts the real run and
exits. When the caller sits in a job with `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`
and no breakaway, the hop and the real run stay in that job. When the
service stops, the program closes the job, and Windows ends every process
in it: the update dies after the service was stopped. magic-cli-remote
has turned the Windows handoff off until this is fixed.

This record decides the scope change both issues need, the API for #3,
the comparison #3 asks for, a state snapshot, and the one-shot handoff
for #6: how the run gets its environment, which a task cannot carry, how
it avoids a visible window, and how its tasks are cleaned up. It answers
each issue's open questions, and names the facts that a probe on the
Windows test host must settle before the code is written.

### Evidence tags

As in 0021-MADR: **[measured]** by this module's tests or probes;
**[measured upstream]** by magic-cli-remote and recorded in its records;
**[docs]** quoted from Microsoft's documentation; **[MS-TSCH]** quoted
from Microsoft's Task Scheduler Service Remoting Protocol specification,
which specifies the service behind the COM calls but does not say that
local COM calls are served by it ("No higher-layer protocols make use of
the Task Scheduler Remoting Protocol"), so it is the next-best source,
not a guarantee for local calls; **[read]** from this repository's code;
**[derived]** reasoned from the above; **[unverified]** open until a
probe settles it.

### Facts [observed 2026-10-23, `main` at `a365dd1`]

**What the package does today**

* **F1. The package never creates, renders or deletes a task.**
  `taskscheduler/task.go:9-11`: "It manages a task the program's setup
  registered: it never creates, renders or deletes one." 0021-MADR D1
  (line 265) decided it, and D9 (lines 486-494) lists "Rendering and
  creating a task ... and semantic comparison for setup's idempotency" as
  not ported: "The program's setup owns the definition (D-d)" [read].
* **F2. The COM calls #3 needs are half built.**
  * `scheduler_windows.go:190`, `xml()`, reads `IRegisteredTask::get_Xml`;
    `reconcile.go:75` uses it to keep a backup [read].
  * `scheduler_windows.go:406`, `restore(xml)`, calls
    `ITaskFolder::RegisterTask` with `taskUpdateFlags`
    (`TASK_UPDATE | TASK_IGNORE_REGISTRATION_TRIGGERS`,
    `com_windows.go:29-31`) and a fixed `logonInteractiveToken`
    (`:426`). It can update a task, not create one [read].
  * `com_windows.go:65` defines `folderDeleteTask` (slot 15) and `:68`
    `taskCreateOrUpdate` (6). Only the tests use them: `comRegister`
    (`com_windows_test.go:244-264`) registers with
    `TASK_CREATE_OR_UPDATE`, no user and `InteractiveToken` in `\`, and
    `comDelete` (`:266-275`) deletes [read].
* **F3. That registration is measured on a GitHub runner, not yet at
  medium integrity.** `newLiveTaskIn` (`live_windows_test.go:357-397`)
  registers every live test's task with `comRegister`. The `v1.18.0` tag's
  CI run (39969753986) passed `TestLiveTaskStopHoldsDisabled`,
  `TestLiveTaskRollback`, `TestLiveTaskRequestStop`,
  `TestLiveTaskStopRefusesDisabled` and `TestLiveTaskzCleanup` on
  `windows-2025` [measured]. That runner runs tasks at high integrity
  (0021-MADR Amendment A5). 0021-PLAN T1 registered its probe tasks at
  medium integrity, but with `schtasks /create /xml` from a driver task,
  not through COM (0021-PLAN Deviation D1) [read]. COM registration at
  medium integrity is [unverified]. *(Measured, Amendment A1, P1-1: it
  succeeds, and the stored `UserId` is the caller's SID.)*
* **F4. The handoff today is a hop.** `Task.Detach`
  (`taskscheduler/detach.go:61-77`) adds `EnvHandOffHop=1` and calls
  `service.DetachProcess`, whose Windows start (`detach_windows.go:19-25`)
  asks for `CREATE_BREAKAWAY_FROM_JOB` and, on `ERROR_ACCESS_DENIED`,
  starts the process again inside the job [read]. The hop exists so the
  real run's recorded parent has exited, and the ancestor walk in
  `Inside` stops there (0011-MADR Amendment A4; 0021-MADR D6) [read].
* **F5. Why the hop was enough.** 0021-MADR Amendment A1, probe 2: the
  task's job has limit flags `0x00000000`, "no `BREAKAWAY_OK`, and no
  `KILL_ON_JOB_CLOSE`"; breakaway is refused, "and the retry inside the
  job succeeds"; `Stop` ends the action's process only, and its children
  survive [measured]. Nothing there measured a caller in a job of its own.
* **F6. The library's own handoff mechanisms.**
  * `launchd.Job.Detach` (`launchd/detach.go:84-125`) loads a one-shot job
    `<label>.selfupdate.<id>`; its plist carries only `EnvHandOff`,
    `EnvHandOffResult` and `EnvHandOffEnv`, and the rest of the
    environment goes in a private file, `handoff-<id>.env`, that the run
    loads. `CleanupHandOffs` (`:127-165`) boots out finished one-shot jobs
    and removes their files [read].
  * `service.LoadHandOffEnvContext` (`handoffenv.go:76-80`) runs
    `HandOffHopContext` first, then reads the file `EnvHandOffEnv` names.
    The file's mode is "not checked on Windows" (`:63`) [read].
  * The guide tells a program to "Call `service.ReportFunc` first in
    `main`" (`docs/guides/extending-selfupdate.md:932`) [read].
* **F7. A batch file is refused.** `winproc.Detached` and
  `winproc.Command` refuse a `.bat` or `.cmd` program through the
  unexported `checkProgram` (`internal/winproc/winproc_windows.go:43`),
  because Windows runs one through `cmd.exe` (0021-MADR D8, BatBadBut)
  [read].

**What Windows does**

* **F8. Job membership is inherited, and kill-on-close reaches the whole
  chain [docs].**
  * "After a process is associated with a job, by default any child
    processes it creates using CreateProcess are also associated with the
    job." ([Job Objects])
  * "When processes are implicitly associated with a job during process
    creation, a child process is associated with every job in the job
    chain of its parent process." ([Nested Jobs])
  * "If the immediate job object does not allow breakaway, the child
    process does not break away even if jobs in its parent job chain
    allow it." ([Nested Jobs])
  * "If a nested job has the JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE flag
    specified, closing the last job object handle terminates all
    processes associated with the job and its child jobs in the
    hierarchy." ([Job Objects])
  * Membership otherwise comes only from `AssignProcessToJobObject`: "To
    associate a process with a job, use the AssignProcessToJobObject
    function." ([Job Objects])
* **F9. So the hop dies in a kill-on-close job [derived from F4 and
  F8].** A hop started from a process in a job with
  `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` and no breakaway is refused
  breakaway, is started inside the job, and starts the real run inside
  it. Closing the job's last handle ends both. Issue #6 reports the
  consequence; no test here has reproduced it [unverified as a
  measurement]. *(Measured, Amendment A1, P2-1: the hop's real run was in
  the kill-on-close job and died when its handle was closed.)*
* **F10. A process Task Scheduler starts is not the caller's child.**
  "When a non-COM task action starts up, the scheduling engine logs on
  the task principal account, gets the process token ... The executable
  action will then be started with the resulting token handle by using
  the CreateProcessAsUser API." ([Task Security Hardening]) [docs]. Its
  parent is `svchost.exe` (0021-MADR Amendment A1, probe 1) [measured].
  By F8, it is therefore in none of the caller's jobs [derived]; that it
  is outside a caller's kill-on-close job is [unverified as a
  measurement]. *(Measured, Amendment A1, P2-2 and P2-3: it is outside
  the caller's job, in Task Scheduler's own, and survives the caller's
  job closing.)*
* **F11. A task cannot carry environment variables.** An `Exec` action
  has only `Command`, `Arguments` and `WorkingDirectory`, and the task XML
  refuses an `<Environment>` element (magic-cli-remote 0159 F9, probe 9,
  recorded in 0021-MADR line 73) [measured upstream]. Environment
  variables in those three fields are expanded from a cache when the
  task engine starts ([IExecAction]) [docs]. So `launchd`'s mechanism,
  an environment variable naming the private file (F6), cannot be set on
  a task [derived].
* **F12. A task's console program shows a window.** 0021-MADR Amendment
  A1: "the task's console program had a console with a visible window",
  consistent with 0159 F10 [measured]. `DetachProcess`'s start gives the
  child a console with no window (0021-MADR D7) [measured].
* **F13. Task Scheduler stores a definition differently from how it was
  registered.** It "drops defaults, stores the SID, and adds `URI`,
  `IdleSettings` and `UseUnifiedSchedulingEngine`. A text comparison
  never matches" (0159 F19, recorded in 0021-MADR line 78) [measured
  upstream]. Microsoft documents only part of it: the service updates
  "the principal node of the XML task definition ... to reflect the
  principal just determined" [MS-TSCH], and sets `RegistrationInfo`'s
  author and date ([Task Registration Information]) [docs].
  Re-registering the XML Task Scheduler returned gives the same XML back,
  byte for byte (0021-MADR Amendment A1, probe 4) [measured].
* **F14. Registering, reading and deleting [docs, MS-TSCH].**
  * `RegisterTask(path, xmlText, flags, userId, password, logonType,
    sddl, ppTask)`; `TASK_CREATE` is 0x2, `TASK_UPDATE` 0x4,
    `TASK_CREATE_OR_UPDATE` 0x6 ([RegisterTask], [TASK_CREATION]).
  * "When you register a task with the logon type equal to
    TASK_LOGON_INTERACTIVE_TOKEN and the task does not have credentials
    specified ... then the task will be registered with the credentials
    of the user that called the Connect method." ([Security Contexts])
  * A user who is not an administrator need not give a password "if you
    register the task to run under the security context of your account
    and you use the S4U or interactive logon type"; such a user cannot
    register with `TASK_RUNLEVEL_HIGHEST` from a low-privilege process,
    nor for another account or a group ([Security Contexts]).
  * "Members of the Users group ... can only read, update, delete, and
    run the tasks that they have created." ([Security Contexts])
  * A missing task: `TASK_UPDATE` alone returns `0x80070002`, and
    `TASK_CREATE` alone on an existing task `0x800700B7`; `DeleteTask`
    returns `ERROR_FILE_NOT_FOUND` for a missing task and `0x80070003`
    (`ERROR_PATH_NOT_FOUND`) for a missing folder [MS-TSCH]. The COM
    pages give no codes for these. `GetTask` of a missing name returns
    `0x80070002` [measured] (0021-MADR Amendment A1, probe 5).
  * `CreateFolder` creates every missing level, and returns `0x800700B7`
    when the folder exists ([CreateFolder]). Whether a user who is not an
    administrator may create a folder under `\` is not documented
    [unverified]. *(Measured, Amendment A1, P1-3: at medium integrity it
    may, on the test host.)*
  * `GetTasks(TASK_ENUM_HIDDEN)` lists hidden tasks too
    ([GetTasks]); the service skips tasks the caller cannot read
    [MS-TSCH].
  * Whether `DeleteTask` ends a running instance, or processes in its
    job, is not documented [unverified]. *(Measured, Amendment A1, P1-5:
    it ends neither the action's process nor its child.)*
* **F15. Running a task with no trigger [docs].**
  * `Run` "will return without error, but the task will not run if the
    AllowDemandStart property of ITaskSettings is set to false"
    ([Run]); `AllowStartOnDemand` defaults to true (schema).
  * The schema allows `Triggers` to be absent (`minOccurs="0"`), and
    [MS-TSCH] requires only `Actions`; the Tasks overview says a task
    "must contain a trigger". The docs conflict [docs].
  * `DisallowStartIfOnBatteries`: "The default setting for this element
    is True." `StopIfGoingOnBatteries`: the schema says `default="true"`,
    its remark says "The default setting for this element is False"
    ([schema]). The docs conflict, so a definition must set both [derived].
  * "If a task is started on demand, the ExecutionTimeLimit setting is
    bypassed." ([ExecutionTimeLimit])
  * `DeleteExpiredTaskAfter` deletes a task only after it "expires", which
    is defined by the triggers' end boundaries ([DeleteExpiredTaskAfter]),
    so a task with no trigger cannot delete itself [derived].
  * A never-run task's `LastTaskResult` is `0x00041303`,
    `SCHED_S_TASK_HAS_NOT_RUN`, and `LastRunTime` advances past a `Run`
    call (0021-MADR, Not verified list and Amendment A1, probe 3)
    [measured].
* **F16. The vtable slots the new calls need.** MinGW-w64's
  `taskschd.h`, read 2026-10-23, gives `ITaskFolder`: `CreateFolder` 11,
  `GetTasks` 14; `IRegisteredTaskCollection`: `get_Count` 7, `get_Item`
  8; `IRegisteredTask`: `get_Name` 7, `get_Path` 8; `IPrincipal`:
  `get_UserId` 11, `get_RunLevel` 17. The same header gives every slot
  `com_windows.go:55-111` already uses, which `TestCOMLayout` measured,
  so the method is sound; the two `IPrincipal` slots were read from a
  summary of the header, not its text [derived]. `TestCOMLayout` pins
  each new slot before it is used. *(Amendment A1: the probes called
  `CreateFolder`, `DeleteFolder`, `GetTasks`, `get_Name`, `get_UserId`
  and `get_RunLevel` at these slots, and each returned what its method
  returns; `TestCOMLayout` still pins them in J2.)*
* **F17. 0011-MADR rejected a one-shot task for `scm`.** Amendment A4:
  "A one-shot scheduled task depends on `schtasks.exe`, needs cleanup,
  and cannot be created by a non-administrator for a LocalSystem
  service's account." [read]. For this backend, the first is answered by
  COM (F2), the third does not arise because the service task is the
  user's own (F14), and the second is a cost this record takes on.

**What the consumer does** (magic-cli-remote at `1bfab31b`, which the
issues cite at `6db2792` and `b946cf2d`; every cited file is unchanged
since, except where noted) [read]

* **F18. Registration.** `internal/cli/service/setup_schtasks.go:19-79`
  queries with `schtasks /query /tn <name> /xml ONE` (`:26`), stages the
  XML in a temporary file encoded as UTF-16LE with a BOM
  (`schtasks.go:195-203`), and runs `schtasks /create /tn <name> /xml
  <file> /f` (`:61`). `removeSchtasks` (`:83-96`) deletes with `schtasks
  /delete /tn <name> /f` (`:91`). `registerTaskXML`
  (`refresh_schtasks.go:145-162`) repeats the create. The task is named
  after the product, in `\` (`taskname.go:5`).
* **F19. The definition it renders.** `renderTaskXML`
  (`schtasks.go:107-172`) gives a logon trigger for the user, a repeating
  time trigger, a principal with the token's SID, `InteractiveToken` and
  `LeastPrivilege`, and settings including `IgnoreNew`,
  `DisallowStartIfOnBatteries` false, `StopIfGoingOnBatteries` false,
  `ExecutionTimeLimit` `PT0S`, and one `Exec` action.
* **F20. Its comparison.** `sameTaskDefinition`
  (`setup_schtasks.go:134-144`) and its helpers through `:318` (185 lines)
  compare the logon and time triggers, the principal's user, logon type
  and run level, ten settings with schema defaults applied to absent
  elements, and the `Exec` command, arguments and working directory. It
  ignores `RegistrationInfo`, `URI`, `IdleSettings` and
  `UseUnifiedSchedulingEngine`.
* **F21. Its state read.** `taskState` (`control_schtasks.go:25-44`) runs
  `powershell.exe -NoProfile -NonInteractive -Command` with
  `Get-ScheduledTask -TaskPath '\' -TaskName '<name>'` and prints the
  numeric state; 0159 probe 11 measured 1261 to 2375 ms a call
  [measured upstream].
* **F22. Its jobs.** `SuperviseStarted`
  (`internal/procutil/procutil_windows.go:261-300`) sets
  `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` alone (`:269-273`); no
  `BREAKAWAY_OK` appears anywhere in the repository. Its callers are the
  terminals and providers that run agents
  (`acpagent/terminal.go:111`, `codex/provider.go:625`,
  `httpagent/provider.go:554`, `providerauth/cli.go:129`).
* **F23. Its Windows handoff is off.** `detacherFor`
  (`internal/updateclient/client.go:205-211`) returns no detacher on
  Windows: "the library's Task Scheduler detach relies on job breakaway,
  which Magic's own kill-on-close jobs refuse, so the handoff waits for a
  detach that needs none (go-selfupdate-lib#6; MADR 0210 D3)". Its
  `0210-MADR-hand-off-agent-run-updates-outside-the-service.md` D3 waits
  on #6, and keeps `KILL_ON_JOB_CLOSE` alone.

## Decision Drivers

* **Additive API within v1**, `make apicheck` compatible with `v1.18.0`
  (0025-MADR D6).
* **No tool output parsed, no PowerShell**, as 0021-MADR D2 decided for
  this package.
* **An update handed off from inside the service survives the service's
  stop and the caller's own jobs,** whatever limits the caller sets.
* **No silent fallback** to a mechanism the caller did not choose, and
  no unmeasured logon type, as 0021-MADR's Q6 decided for the rewrite.
* **The program owns its definition's content;** the library owns the
  mechanics of storing it, and the tasks it creates for itself.
* **Every Windows claim the code relies on is measured** on the Windows
  test host at medium integrity, as 0021-PLAN T1 did, before the code
  that relies on it is written.
* **Exported results are fields with JSON tags,** and coherent
  speculative API is built (the owner's standing preference).

## Considered Options

### D1: the package's scope (both issues)

* **D1-A.** Amend 0021-MADR D1 and D9: the package registers, reads,
  compares and deletes a definition the program renders, and renders,
  registers and deletes its own one-shot handoff tasks (D7). It never
  renders the program's definition. `task.go`'s "never creates, renders
  or deletes one" becomes "never renders the program's definition".
* **D1-B.** Keep the scope: the program keeps `schtasks.exe` for its
  definition, and only #6 is done.

### D2: the registration API (issue #3, its first open question)

* **D2-A.** Methods on `*Task`, which already resolves the task's name
  and folder from `Options` and a product:
  * `Register(ctx, product, xml string) error` registers or updates the
    definition with `RegisterTask(name, xml, TASK_CREATE_OR_UPDATE,
    VT_EMPTY, VT_EMPTY, logonType, VT_EMPTY)`, in `Options.Folder`,
    creating that folder first when it does not exist. `logonType` is the
    definition's own `Principal/LogonType`, read from the XML, or
    `InteractiveToken` when it has none (the schema's default
    [MS-TSCH]). A definition whose logon type needs a password or names
    a group or service account is refused before Task Scheduler is
    called, with an error naming it: the backend has no credentials, and
    F14 allows a user only `InteractiveToken` and S4U for themselves.
  * `Definition(ctx, product) (string, error)` returns `get_Xml`; a
    missing task wraps `service.ErrNotInstalled`.
  * `Delete(ctx, product) error` calls `DeleteTask`; a missing task or
    folder is not an error, so removal is idempotent. It does not stop a
    running instance; its doc says to call `Stop` first.
  * Errors use `mapError`: `0x80070005` wraps `service.ErrPermission`.
* **D2-B.** A separate `taskscheduler.Registrar` type with its own
  options, so `Task` stays a lifecycle only.
* **D2-C.** Package functions taking a folder and a name.

### D3: the comparison (issue #3, its second open question)

* **D3-A.** `DiffDefinitions(stored, wanted string) ([]DefinitionDiff,
  error)` and `SameDefinition(stored, wanted string) (bool, error)`, pure
  Go over `encoding/xml`, on every OS:
  * **What is compared:** the elements a program sets, the union of
    F20's set and the rest of the schema's elements of the same kinds:
    * every trigger, in order, by type, with `Enabled`, `StartBoundary`,
      `EndBoundary`, `Delay`, `ExecutionTimeLimit`, `UserId`, and
      `Repetition`'s `Interval`, `Duration` and `StopAtDurationEnd`;
      *(Amended by Amendment A2: every element a trigger holds, by type.)*
    * the principal's `UserId`, `GroupId`, `LogonType` and `RunLevel`;
    * every `Settings` element the schema defines, except
      `IdleSettings` and `UseUnifiedSchedulingEngine`, which F13 says
      the service adds;
    * every action, in order: an `Exec`'s `Command`, `Arguments` and
      `WorkingDirectory`, and a `ComHandler`'s `ClassId` and `Data`.
  * **Defaults:** an element absent on one side compares as its schema
    default, from a table in the code that cites the schema page for
    each value. Where the docs conflict (F15), the table takes the
    schema's `default=` attribute, and a test pins the stored value the
    probe sees.
  * **Not compared:** `RegistrationInfo`, `URI`, `Data`, and anything
    the list does not name. A difference there is not reported; the doc
    says so. *(Amendment A2: an action is compared with every element it
    holds, too.)*
  * **Accounts:** a `UserId` is compared case-insensitively. A SID and an
    account name compare equal when one resolves to the other, through
    `LookupAccountSid` on Windows; off Windows they compare as text.
  * **The result:** `DefinitionDiff{Path, Stored, Wanted string}` with
    JSON tags, one per difference, `Path` an element path such as
    `Settings/DisallowStartIfOnBatteries`. `SameDefinition` is
    `len(diffs) == 0`.
* **D3-B.** Let Task Scheduler normalise: parse `wanted` into a new
  definition (`ITaskService::NewTask`, `put_XmlText`), read it back with
  `get_XmlText`, and compare texts after removing what registration adds.
  Whether `put_XmlText` normalises as registration does is not
  documented [unverified].
* **D3-C.** No comparison; the program keeps its own (F20).

### D4: a state snapshot (issue #3's PowerShell read, F21)

* **D4-A.** `(*Task).Status(ctx, product) (Status, error)`, as
  `launchd.Job.Status` (0031-MADR D4): `Installed`, `Enabled`, `State`
  (`TASK_STATE` as its name: `unknown`, `disabled`, `queued`, `ready`,
  `running`), `Running`, `PIDs` (each instance's `get_EnginePID`),
  `LastRunTime` and `LastResult`, with JSON tags; one task open. A
  missing task is `Installed: false` and no error.
* **D4-B.** No snapshot; a program composes `Installed`, `Enabled` and
  `Running`.

### D5: how `Detach` starts the run (issue #6, its first open question)

* **D5-A.** `Detach` registers and runs a one-shot task (D6 to D8), and
  the hop stays available as `Options.HopDetach bool`, for a caller that
  wants today's behaviour. The zero value is the one-shot task.
* **D5-B.** The one-shot task only behind an option; the hop stays the
  default.
* **D5-C.** The one-shot task replaces the hop; no option.
* **D5-D.** Keep the hop and document that a caller's job must allow
  breakaway (`JOB_OBJECT_LIMIT_BREAKAWAY_OK`). magic-cli-remote's 0210
  rejected this: it loosens containment for every process to serve the
  update (F23).

### D6: how the run gets its environment (F11, not in the issue)

* **D6-A.** A leading argument. The action's `Arguments` start with
  `--selfupdate-handoff-env=<path>`, the private file's absolute path,
  then the update's own arguments. `service.LoadHandOffEnvContext`
  looks for that argument at `os.Args[1]` before anything else, removes
  it from `os.Args`, and applies the file as it applies the file
  `EnvHandOffEnv` names today, then runs the hop check (D7). The file
  holds `os.Environ()`, `EnvHandOff`, `EnvHandOffResult`, the caller's
  `spec.Env` and, under D7-A, `EnvHandOffHop=1`. The definition holds
  the path, never a value.
* **D6-B.** A leading handoff ID. The action's `Arguments` start with
  `--selfupdate-handoff=<id>`, and the file is always
  `handoff-<id>.env` in one fixed per-user directory,
  `%LOCALAPPDATA%\selfupdate\handoff`. `LoadHandOffEnvContext` finds the
  file from the ID. The definition holds only a random ID. A custom
  `Options.HandOffDir` is not possible, because the run cannot learn it.
* **D6-C.** The working directory. The action's `WorkingDirectory` is a
  private per-handoff directory under the handoff root, and
  `LoadHandOffEnvContext` loads a file of a fixed name from the current
  directory when that directory is under the root. `os.Args` is not
  touched.

Rejected without a full weighing:

* `cmd.exe /c set NAME=value && program`. 0021-MADR D8 forbids starting
  `cmd.exe`, and the values would be stored in the definition.
* The user's registry environment (`HKCU\Environment`), set before `Run`
  and cleared after. Any process Task Scheduler starts in between, the
  service's own task included, would read it.
* No transfer: the run gets the fresh environment Task Scheduler builds
  for the user, with the handoff variables as arguments. It loses
  `spec.Env` and whatever the caller's environment set (a proxy, a
  config path), so the run would not be the caller's update.

### D7: the window (F12, not in the issue)

* **D7-A.** The one-shot task runs a hop: the file carries
  `EnvHandOffHop=1`, so the task's process starts the real run detached,
  with a console and no window, and exits. The task's own console
  window lasts only as long as the hop.
* **D7-B.** The one-shot task runs the real run directly; its window
  stays open for the update.
* **D7-C.** An S4U principal, whose task runs "in a non-interactive
  desktop" ([TASK_LOGON_TYPE]), so it has no visible window. S4U has "no
  access to either the network or to encrypted files", and an update
  downloads its release [docs].

### D8: the one-shot task itself (issue #6's other open questions)

* **D8-A.**
  * **Name and place:** `<task>.selfupdate.<id>` in the service task's
    folder, as `launchd`'s `<label>.selfupdate.<id>`. Not hidden, so a
    user who opens Task Scheduler sees what runs, with a `Description`
    that says so.
  * **Principal:** the service task's own `UserId`, `LogonType` and
    `RunLevel`, read through `IPrincipal`. The run then has the token
    the hop's run has today, because the caller is inside the service
    task. Only `InteractiveToken` is supported, the logon type probe P2
    measures; any other is refused with an error naming it and
    `Options.HopDetach`, as Q6 refused the rewrite.
  * **Definition:** no trigger; one `Exec` action with the executable,
    D6-A's arguments composed with `windows.ComposeCommandLine`
    *(0032-PLAN Deviation D6: each quoted by `windows.EscapeArg`'s rules,
    since none is a program name)*, and the
    caller's temporary directory as `WorkingDirectory`, as
    `DetachProcess` uses; settings `AllowStartOnDemand` true,
    `MultipleInstancesPolicy` `IgnoreNew`, `DisallowStartIfOnBatteries`
    false, `StopIfGoingOnBatteries` false, `ExecutionTimeLimit` `PT0S`,
    `Enabled` true, `Hidden` false, each written explicitly (F15).
  * **Registration:** `RegisterTask` with `TASK_CREATE`, so a name
    collision fails rather than replaces, then `Run`. `Detach` returns
    once an instance is listed or the task's `LastRunTime` is at or after
    the `Run` call (F15), bounded by `ctx` and `Options.Poll`'s timeout.
    On any failure after registration it deletes the task and the file.
  * **Refusals:** a `.bat` or `.cmd` executable, by the check `winproc`
    already makes (F7), exported for this; an executable holding a
    double quote, as `Reconcile` refuses one.
  * **The private file:** in `Options.HandOffDir`, by default
    `%LOCALAPPDATA%\selfupdate\handoff` (`os.UserCacheDir`), written by
    `service.WriteHandOffEnv`. Its directory's access is what protects
    it on Windows (F6); probe P1 reads its ACL. *(Amended by Amendment
    A1: the inherited ACL is not private on the test host, so
    `WriteHandOffEnv` gives the directory a protected DACL on Windows.)*
  * **Cleanup:** `(*Task).CleanupHandOffs(ctx) error` lists the folder
    with `GetTasks(TASK_ENUM_HIDDEN)` and deletes each
    `<task>.selfupdate.*` task that is not running and has run
    (`LastTaskResult` is not `0x00041303`), with any file left for its
    ID. `Detach` calls it first, best-effort, as `launchd` does; a
    program may call it at start-up.
  * `Detached.Where` is `task <folder>\<name>`.
* **D8-B.** As D8-A, in a subfolder, `<folder>\selfupdate`, hidden.
  Creating a folder needs a right that is not documented for a user who
  is not an administrator (F14).

## Decision Outcome

Chosen options (the owner, 2026-10-23): "D1-A", "D2-A", "D3-A", "D4-A",
"D5-A", "D6-A", "D7-A" and "D8-A", released as `v1.19.0`, a minor
release because each adds API.

The owner decided D5-A (the one-shot task as the default) and D4-A
(`Status` included) first, asked for D6 to be explained with three
options and a recommendation, then chose D6-A, a leading path argument,
and "follow recommendations for the other decisions": D1-A, D2-A, D3-A,
D7-A and D8-A.

**D5 to D8 depend on probe P2** (Confirmation);
if it contradicts F10's derivation, the PLAN stops and this record is
amended before any handoff code is written.

* **D1-A** gives #3 what it asks, and the one-shot task needs it anyway.
  The rendering stays the program's: the content of a definition is a
  product decision (0021-MADR D9's reason), but storing it is the
  mechanics this package exists to get right (F18's UTF-16 file, F21's
  PowerShell). D1-B leaves both in every consumer.
* **D2-A** keeps one type per task, with the name rules `Task` already
  applies, and adds methods to a struct, not to an interface any caller
  implements, so nothing breaks. A `Registrar` (D2-B) would duplicate
  `Options` for the same task. Reading the logon type from the XML
  passes Task Scheduler the value the definition already states, and the
  refusal names the cases a user cannot register without credentials
  (F14) instead of returning an HRESULT. Creating the folder serves a
  program that uses `Options.Folder`; whether a user may create one is
  P1's to measure, and a refusal maps to `service.ErrPermission`
  either way.
* **D3-A** answers "which fields count" with a set the library can name
  and test, against stored XML captured from Task Scheduler (P1), and
  says what it does not compare. A diff list serves a setup command that
  must say what changed before it replaces a task (F18's `--force`).
  D3-B rests on an undocumented normalisation, and the probe that could
  show it (P1) cannot show it holds for every element. D3-C leaves 185
  lines in the consumer (F20).
* **D4-A** removes the 1.3 s PowerShell read (F21) with the calls the
  package already makes, in the shape `launchd.Status` set.
* **D5-A** makes the handoff that survives every caller the default, so
  a program gets it without knowing what jobs it runs in, and keeps the
  hop for a program that has a reason to want it. D5-B would leave the
  default broken for the case #6 describes. D5-C removes a measured
  mechanism with no way back.
* **D6-A** carries one path in the definition and nothing secret, and
  needs only what the guide already requires: `ReportFunc` or
  `LoadHandOffEnv` first in `main` (F6). Anyone who can start the
  program with an argument can already set its environment, so the
  argument opens nothing new, and `Options.HandOffDir` works because the
  path travels with the run. D6-B stores less, a random ID, but fixes
  the directory for every program and user, so `HandOffDir` cannot
  exist, and a run started without the ID has nothing to find. D6-C
  makes the current directory an input to every start of the program,
  which a program run from an untrusted directory must not have.
* **D7-A** reuses two measured mechanisms, the hop (0011-MADR A4) and the
  windowless start (0021-MADR D7), and limits the window to the hop's
  life. D7-B shows a console for the whole update. D7-C cuts the update
  off from the network.
* **D8-A** follows `launchd`'s shape, which the issue asks for. The
  principal is the service's, so the update runs as the hop's run would
  have. `TASK_CREATE` and the cleanup rule use measured values (F15).
  The folder of the service task needs no new right (F14); a subfolder
  (D8-B) does, and gains only tidiness.

### Consequences

* Good, because a program registers, reads, compares and removes its task
  without `schtasks.exe`, a temporary UTF-16 file or PowerShell.
* Good, because an update handed off from inside the service, at any
  depth of the caller's jobs, runs outside them, and magic-cli-remote can
  turn its Windows handoff on (F23).
* Neutral, because the default `Detach` changes from a hop to a one-shot
  task; `Options.HopDetach` restores the old behaviour.
* Neutral, because `LoadHandOffEnvContext` now reads `os.Args[1]`, and
  applies the environment file before the hop check.
* Neutral, because a one-shot task appears in Task Scheduler while it
  runs and until the next cleanup, and a console window shows for the
  hop's life.
* Bad, because the package grows: four new COM calls, a definition
  renderer for the one-shot task, a comparison with a defaults table to
  maintain, and a cleanup.
* Bad, because `DiffDefinitions` ignores elements it does not list, and
  a difference there passes unseen.

### Confirmation

* **Probe P1 (registration), on the Windows test host, from a driver
  task at medium integrity in the console session (0021-PLAN Deviation
  D1's method):**
  1. COM `RegisterTask` with `TASK_CREATE_OR_UPDATE`, no user,
     `InteractiveToken`, in `\`, succeeds; the stored principal's
     `UserId` is the caller's SID.
  2. The same with S4U.
  3. `CreateFolder` of a new folder under `\`: success or `0x80070005`.
  4. `TASK_CREATE` on an existing task: `0x800700B7`. `DeleteTask` of a
     missing task, and in a missing folder: the HRESULTs.
  5. `DeleteTask` of a task whose instance runs: whether the process,
     and a child of it, survive.
  6. Registration from a BSTR whose XML declares `UTF-16`, `UTF-8`, and
     no encoding.
  7. The stored XML of F19's definition and of a set of definitions
     covering D3-A's elements, captured as test fixtures, with every
     SID, account and host name replaced by a placeholder.
  8. The ACL of `%LOCALAPPDATA%\selfupdate\handoff` after
     `WriteHandOffEnv` makes it.
* **Probe P2 (handoff), same method,** from a probe task whose program
  starts a child in a job with `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` and
  no breakaway, as F22's:
  1. Today's hop from that child: the real run dies when the job's handle
     is closed (F9, reproduced).
  2. A one-shot task registered and run by that child: its process's
     parent is `svchost.exe`, and `IsProcessInJob` on the child's job
     handle says it is not a member.
  3. With D7-A: the real run survives the hop's exit, the one-shot
     instance's end, the closing of the child's job, `Stop` of the probe
     task, and `DeleteTask` of the one-shot task.
  4. The real run's console has no window; the hop's window lasts how
     long.
  5. Arguments with spaces, quotes, backslashes and non-ASCII characters
     arrive in `os.Args` unchanged.
  6. The one-shot task's principal reads back as the probe task's
     `UserId`, `InteractiveToken` and `RunLevel`; its settings read back
     as D8-A writes them.
  7. `Run`, then `LastRunTime` or a listed instance, as D8-A's wait
     reads them.
* **Tests:** fakes for every new path; `TestCOMLayout` pins the new
  slots (F16); fixture tests for `DiffDefinitions` from P1 step 7, each
  seen failing on a planted single-field change; live tests for
  `Register`, `Definition`, `Delete`, `Status` and the one-shot handoff
  from a kill-on-close job, run in CI by its `^TestLiveTask` step.
* `make apicheck` is compatible with `v1.18.0`; `make gate` ends
  `overall=0`; the `v1.19.0` tag's CI passes.

## Pros and Cons of the Options

### D1-A. Register, read, compare, delete

* Good, because #3's three tool round trips go.
* Bad, because the package owns task creation, which 0021 chose not to.

### D1-B. Keep the scope

* Good, because nothing in the package changes for #3.
* Bad, because each Windows program keeps F18 to F21.

### D2-A. Methods on `*Task`

* Good, because `Options.Name` and `Options.Folder` already name the
  task.
* Neutral, because `Task` gains methods that are not part of the
  lifecycle.

### D2-B. A `Registrar`

* Good, because the lifecycle type stays as it is.
* Bad, because a second type repeats the same name and folder options.

### D2-C. Package functions

* Bad, because each call repeats the folder and name, which `Task`
  validates once.

### D3-A. A named field set, defaults applied, a diff list

* Good, because what it compares is written down and tested against
  stored XML.
* Bad, because the defaults table must follow Microsoft's schema.

### D3-B. Task Scheduler's own normalisation

* Good, because it would follow Task Scheduler wherever it changes.
* Bad, because it rests on an undocumented behaviour.

### D3-C. No comparison

* Bad, because every program writes F20 again.

### D4-A. `Status`

* Good, because a status command needs no PowerShell.
* Bad, because `Status` overlaps `Installed`, `Enabled` and `Running`.

### D4-B. No snapshot

* Bad, because a status command makes three calls.

### D5-A. One-shot by default, hop by option

* Good, because the default survives any caller's jobs.
* Neutral, because existing callers' runs start through Task Scheduler.

### D5-B. One-shot by option

* Bad, because the default stays broken for #6's caller.

### D5-C. One-shot only

* Bad, because a measured mechanism goes with no way back.

### D5-D. Document breakaway

* Bad, because the caller loosens containment for every process.

### D6-A. A leading path argument

* Good, because the definition holds only a path, and the directory can
  be configured.
* Good, because it has `EnvHandOffEnv`'s trust model: the path names a
  file the loader checks as it checks that one.
* Neutral, because a program that parses `os.Args` before `ReportFunc`
  would see the argument; the guide already says to call it first.

### D6-B. A leading ID argument and a fixed directory

* Good, because the definition holds only a random ID.
* Bad, because the directory is fixed for every program, so
  `Options.HandOffDir` cannot exist.
* Neutral, because it also edits `os.Args`, as D6-A does.

### D6-C. The working directory

* Good, because `os.Args` is untouched.
* Bad, because every start of the program reads its current directory
  for a file, which a program run from an untrusted directory must not
  do.

### D7-A. A hop inside the task

* Good, because both mechanisms are measured.
* Bad, because the hop's window still shows briefly.

### D7-B. No hop

* Bad, because a console shows for the whole update.

### D7-C. S4U

* Bad, because the update loses the network.

### D8-A. The service's folder and principal

* Good, because it needs no right the service task did not.

### D8-B. A hidden subfolder

* Bad, because folder creation may need an administrator.

## Amendment A1 (2026-10-23): probe results

Measured by
[0032-PLAN-task-scheduler-registration-and-one-shot-handoff.md](0032-PLAN-task-scheduler-registration-and-one-shot-handoff.md)
J1 on the Windows test host (Windows 11, build 26200), from a driver task
at medium integrity (`0x2000`) in the console session (session 1), with
the probe program the PLAN describes. Two complete runs agreed on every
value both measured; run 2 added the last three rows' measurements. The
owner chose the resolution of the one decision point the probes tripped
on 2026-10-23.

| Probe | Question | Result | Effect |
| :--- | :--- | :--- | :--- |
| P1-1 | COM registration at medium integrity | `RegisterTask(TASK_CREATE_OR_UPDATE, no user, InteractiveToken)` in `\`: `0x00000000`; the stored `Principal/UserId` is the caller's SID | D2-A holds |
| P1-2 | S4U | `0x80070005` | D2-A: `Register` refuses S4U with the logon types that need credentials |
| P1-3 | A folder under `\` | `CreateFolder`: `0x00000000`; again: `0x800700B7`; a task registered in it, then both deleted: `0x00000000` | D2-A's folder creation works for the user; D8-B's premise does not hold on this host, and D8-A stands for its other reasons |
| P1-4 | HRESULTs | `TASK_CREATE` on an existing task: `0x800700B7`; `TASK_UPDATE` alone on a missing task: `0x80070002`; `DeleteTask` of a missing task: `0x80070002`, and in a missing folder: `0x80070002`, not [MS-TSCH]'s `0x80070003`; `GetFolder` of a missing folder: `0x80070002` | D2-A: `Delete` treats `0x80070002` as done; `Register` creates the folder on `0x80070002` |
| P1-5 | `DeleteTask` with a running instance | `0x00000000`; the action's process and its child were both alive 5 s later | D8-A's cleanup cannot end a run; `Delete` does not stop the task |
| P1-6 | The XML declaration | `UTF-16`: accepted; none: accepted; `UTF-8`: `0x8004131A`. Task Scheduler stores `UTF-16` in both accepted cases | D2-A: `Register` removes a leading declaration, since the BSTR is already UTF-16 |
| P1-7 | Stored definitions | Captured for six cases; two runs identical. Absent and default-valued settings store alike; `DisallowStartIfOnBatteries`, `StopIfGoingOnBatteries` (both `true` by default), `MultipleInstancesPolicy`, `IdleSettings` and `UseUnifiedSchedulingEngine` are always written; a definition with no principal gets the caller's SID and `InteractiveToken`; `UseUnifiedSchedulingEngine` stored `true` where the definition set `false`; an absent `Triggers` is stored as `<Triggers />` | D3-A: the defaults table follows the stored values, settling F15's `StopIfGoingOnBatteries` conflict as `true`; `UseUnifiedSchedulingEngine` stays uncompared |
| P1-8 | The handoff directory's ACL | Inherited from `%LOCALAPPDATA%`: Full control for the user, SYSTEM and Administrators, and Modify for a local group and a second SID that are not the user | **Decision point; see below** |
| P1-9 | A protected DACL set by the user | `SetNamedSecurityInfo` with a protected DACL of the user, SYSTEM and Administrators: `0x00000000`; `icacls` then lists exactly those three, the file inherits them, and the user reads it | The resolution below is possible at medium integrity |
| P2-1 | Today's hop from a kill-on-close job | The hop and its real run had job flags `0x00002000` (kill on close); the real run was in the job (`IsProcessInJob`: true), alive before the close, dead 3 s after | F9 reproduced: #6's failure is real |
| P2-2 | The one-shot task's process | Parent `svchost.exe`; its job's flags `0x00000000`; the real run is not in the caller's job (`IsProcessInJob`: false) | D5-A holds |
| P2-3 | Survival | The real run was alive after the hop exited, after the one-shot instance ended, 3 s after the caller's job closed, after `Stop` of the service task, and after `DeleteTask` of the one-shot task, its heartbeat still fresh | D5-A and D8-A's cleanup hold |
| P2-4 | The window | The hop had a visible console window and lived about 50 ms; the real run had no console window (`GetConsoleWindow`: 0) | D7-A holds |
| P2-5 | Arguments | `--selfupdate-handoff-env=<path>`, then `a b`, `"q"`, `back\slash\` and `ü 任务`, composed by `windows.ComposeCommandLine`: the real run's `os.Args` matched; the file was applied, `PROBE_ENV` set from it, and the file deleted | D6-A holds |
| P2-6 | Principal and settings | The one-shot task read back with the service task's user, `InteractiveToken` and `LeastPrivilege`. `get_UserId` returns the account name without its domain; the stored XML holds the SID either way. Defaults written explicitly were dropped on store, as P1-7 | D8-A holds; copying `get_UserId` into `UserId` registers the same principal |
| P2-7 | The start signal | `Run` returned in 2 ms; an instance was listed 26 ms after it in both runs. `LastRunTime` has one-second resolution and read `…:46.9999998` for a run at `…:47`, so a comparison with the `Run` call can miss | D8-A: `Detach` waits for a listed instance, or a `LastTaskResult` other than `0x00041303`, not `LastRunTime` |

### The decision point: the environment file's directory

* **Found (P1-8).** D8-A said the handoff directory's access protects the
  file on Windows. On the test host, `%LOCALAPPDATA%`'s inherited ACL
  grants Modify to a local group and to a second SID besides the user,
  SYSTEM and Administrators, and the directory and file inherit it.
  `WriteHandOffEnv` sets mode 0700 on Unix and nothing on Windows
  (`handoffenv.go:63`). The file carries the caller's whole environment.
* **Decided (the owner, 2026-10-23).** `service.WriteHandOffEnv` gives the
  directory it makes or uses, on Windows, a protected DACL granting full
  control to the current user, SYSTEM and Administrators only, with no
  inherited entries; the file inherits it. This is the Windows
  counterpart of the Unix 0700, and P1-9 measured that a medium-integrity
  process can set it.
* **Not chosen.** Encrypting the file with DPAPI for the current user:
  more code, a profile dependency and a format of its own, and other ACL
  holders could still delete or replace the file.
* **Effect on D8-A.** "Its directory's access is what protects it on
  Windows" holds once `WriteHandOffEnv` sets that access itself. The
  PLAN's J6 gains the DACL and its tests.

## Amendment A2 (2026-10-23): the comparison covers every element a trigger or action holds

* **Found.** In 0032-PLAN J4, D3-A's list of trigger elements left out
  those a trigger of one type holds: a calendar trigger's schedule
  (`DaysInterval`, `WeeksInterval`, `DaysOfWeek`, and the monthly forms),
  a session-state trigger's `StateChange`, an event trigger's
  `Subscription` and `ValueQueries`, and a time or calendar trigger's
  `RandomDelay`. Each is in the captured definitions (Amendment A1,
  P1-7). With the list as written, a weekly task moved from Monday to
  Tuesday compares equal.
* **Decided (the owner, 2026-10-23).** `DiffDefinitions` compares every
  element a trigger holds, by type and position, and every element an
  action holds, by type and position. The principal's four elements,
  the `Settings` rule, the defaults and the "not compared" set are
  unchanged.
* **Not chosen.** Keeping the list as written, which leaves those
  changes unreported.
* **Consequence.** A trigger or action element a later Windows release
  writes with a value would be reported as a difference until the
  defaults table names it.

## More Information

### What each issue's open questions get

* **#3, API shape:** methods on `*Task` (D2-A).
* **#3, the comparison:** a named set with schema defaults, as a diff
  (D3-A).
* **#3, encoding:** `RegisterTask` takes a BSTR, so no file and no BOM;
  P1 step 6 measures which declarations Task Scheduler accepts. No case
  needs the file form.
* **#6, one-shot or hop:** one-shot by default, hop by option (D5-A).
* **#6, principal:** the service task's own, `InteractiveToken` only
  until measured otherwise (D8-A).
* **#6, visibility:** the service task's folder, not hidden (D8-A).

### Out of scope

* `scm`'s `Detach`, which also starts a hop. A service run by the Service
  Control Manager whose program uses kill-on-close jobs has the same
  exposure (F9); 0011-MADR A4's reasons for not using a task there (F17)
  still hold for a LocalSystem service. A record of its own may revisit
  it.
* Rendering the program's definition.
* `RegisterTaskDefinition` with an edited definition object, beyond the
  existing `Reconcile`.

### Related

* [0021-MADR-task-scheduler-lifecycle-and-windowless-children.md](0021-MADR-task-scheduler-lifecycle-and-windowless-children.md):
  D1 and D9 are amended by D1-A here, and its `## Amendment` list gains
  a pointer when this record is accepted.
* [0011-MADR-reference-service-lifecycles.md](0011-MADR-reference-service-lifecycles.md)
  §9 and Amendments A3 and A4: the handoff, the environment file and the
  hop.
* [0031-MADR-export-default-http-client-and-launchd-status.md](0031-MADR-export-default-http-client-and-launchd-status.md)
  D4: `launchd.Status`, the shape D4-A follows.
* The consumer's records are in another repository: magic-cli-remote,
  `docs/decisions/0210-MADR-hand-off-agent-run-updates-outside-the-service.md`
  D3, `0159-MADR-the-windows-service-path-is-half-wired.md` F9, F19, D14
  and probe 11, and `0116-MADR-windows-and-linux-arm64-build-targets.md`
  D12.

### Sources

Microsoft pages were read on 2026-10-23.

* [Job Objects]: <https://learn.microsoft.com/en-us/windows/win32/procthread/job-objects>
* [Nested Jobs]: <https://learn.microsoft.com/en-us/windows/win32/procthread/nested-jobs>
* [Task Security Hardening]: <https://learn.microsoft.com/en-us/windows/win32/taskschd/task-security-hardening>
* [IExecAction]: <https://learn.microsoft.com/en-us/windows/win32/api/taskschd/nn-taskschd-iexecaction>
* [Task Registration Information]: <https://learn.microsoft.com/en-us/windows/win32/taskschd/task-registration-information>
* [RegisterTask]: <https://learn.microsoft.com/en-us/windows/win32/api/taskschd/nf-taskschd-itaskfolder-registertask>
* [TASK_CREATION]: <https://learn.microsoft.com/en-us/windows/win32/api/taskschd/ne-taskschd-task_creation>
* [TASK_LOGON_TYPE]: <https://learn.microsoft.com/en-us/windows/win32/api/taskschd/ne-taskschd-task_logon_type>
* [Security Contexts]: <https://learn.microsoft.com/en-us/windows/win32/taskschd/security-contexts-for-running-tasks>
* [CreateFolder]: <https://learn.microsoft.com/en-us/windows/win32/api/taskschd/nf-taskschd-itaskfolder-createfolder>
* [GetTasks]: <https://learn.microsoft.com/en-us/windows/win32/api/taskschd/nf-taskschd-itaskfolder-gettasks>
* [Run]: <https://learn.microsoft.com/en-us/windows/win32/api/taskschd/nf-taskschd-iregisteredtask-run>
* [ExecutionTimeLimit]: <https://learn.microsoft.com/en-us/windows/win32/api/taskschd/nf-taskschd-itasksettings-get_executiontimelimit>
* [DeleteExpiredTaskAfter]: <https://learn.microsoft.com/en-us/windows/win32/api/taskschd/nf-taskschd-itasksettings-get_deleteexpiredtaskafter>
* [schema]: the settings elements under <https://learn.microsoft.com/en-us/windows/win32/taskschd/task-scheduler-schema-elements>,
  `DisallowStartIfOnBatteries` and `StopIfGoingOnBatteries` in particular.
* [MS-TSCH]: <https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-tsch/>,
  `SchRpcRegisterTask`, `SchRpcDelete`, `SchRpcRetrieveTask` and
  `SchRpcEnumTasks`.
* MinGW-w64 `taskschd.h`:
  <https://github.com/mingw-w64/mingw-w64/blob/master/mingw-w64-headers/include/taskschd.h>
