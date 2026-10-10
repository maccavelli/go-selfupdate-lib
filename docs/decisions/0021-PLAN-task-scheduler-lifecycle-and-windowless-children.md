---
status: in-progress
date: 2026-10-10
associated-madr: "0021-MADR-task-scheduler-lifecycle-and-windowless-children.md"
---
# Implement a Task Scheduler reference lifecycle and windowless Windows children

Associated MADR: [0021-MADR-task-scheduler-lifecycle-and-windowless-children.md](0021-MADR-task-scheduler-lifecycle-and-windowless-children.md)

## Goal

* **The facts first.** The five items in the MADR's "Not verified" list
  that a probe can settle are measured on the Windows test host, and the
  MADR is amended with the answers, before any code depends on them.
* **D7 and D8.** Every process this module starts goes through one
  constructor in a new package, `internal/winproc`:
  * a child of a process with no console gets a windowless console;
  * the detached handoff run gets one too, in place of `DETACHED_PROCESS`;
  * no `.bat` or `.cmd` file is started.

  A static test refuses any other process start under `selfupdate/`.
* **D6's walk** moves from `scm` to `internal/winproc`, with no change in
  `scm`'s behaviour.
* **D1 to D6.** A new backend, `selfupdate/service/taskscheduler`, manages
  one per-user Task Scheduler task through the COM API. It implements
  `selfupdate.Lifecycle`, `selfupdate.EnabledLifecycle`,
  `selfupdate.Reconciler` and `service.Detacher`. It is proven by tests over
  a fake, by a COM layout test and by a live test on the Windows test host.
* **`v1.14.0`** is released with all of it, in one minor release (MADR Q5),
  by the release procedure of
  [0017-PLAN-verify-build-provenance-and-close-0015-open-items.md](0017-PLAN-verify-build-provenance-and-close-0015-open-items.md).

## Scope

### In scope

| Phase | What | Commit |
| :--- | :--- | :--- |
| T0 | this PLAN, the MADR's acceptance, the 0011 and 0004 pointers, the index rows | records only |
| T1 | probes on the Windows test host; the MADR amended with the results | records only |
| T2 | `internal/winproc`: the constructor, D7 and D8; the four start sites moved onto it; the static ban; depguard | one commit |
| T3 | the ancestor walk moved from `scm` to `internal/winproc` | one commit |
| T4 | `taskscheduler`: options, probes, stop, start, health, reconcile, restore, inside, detach, all over a fake | one commit |
| T5 | `taskscheduler`: the COM transport, and its layout test on the Windows test host | one commit |
| T6 | `taskscheduler`: the live test, and CI's step | one commit |
| T7 | guides, architecture, README, migration guide | docs only |
| T8 | `v1.14.0`: the release procedure, and the pin commit | the tag, one commit |
| T9 | the closing record | records only |

### Out of scope

* **Creating, rendering or deleting a task in the backend** (MADR D1, D9).
  Tests create and delete their own throwaway tasks; the package never
  does.
* **Everything MADR D9 lists:** environment delivery, `FreeConsole` in a
  daemon, polite stops of a program's children, logging, signing, the
  installer's `Path` change.
* **`scm`, `systemd` and `launchd` contracts.** T3 moves code out of `scm`
  without changing what it does. T2 changes how `startDetached` and
  `runExec` start a process, not what they return.
* **magic-cli-remote.** Moving its 0054 from "keep the adapter" to "adopt
  `taskscheduler`" is that repository's record (MADR, Consequences). This
  PLAN does not write in that repository.
* **A Task Scheduler leg on a GitHub runner that needs a change of runner
  image or account.** T6 adds a step that runs the live test where the
  runner can, and skips with a stated reason where it cannot.
* **Push and tags.** Each happens only on the owner's ask in the same
  turn.

## Rules for every phase

1. **Commits.** Each phase commits on `main`, with `git commit --no-edit`.
   The global `prepare-commit-msg` hook writes the message.
2. **Before each commit:**
   * `make pre-add-check FILES="<the phase's Go files>"` exits 0 when the
     phase has Go files;
   * `make gate` ends `overall=0`;
   * for a phase that changes Markdown, `scripts/check-docs.sh --links`
     and `--ids` are clean on its files, and markdownlint
     (`npx --yes markdownlint-cli2@0.23.2`) is clean on any guide it
     changes.

   Long output goes to a scratch file, with `$?` captured before any
   filter.
3. **Every new test case fails first,** on a scratch copy, never in the
   tree. `scripts/plant-copy.sh FILE OLD NEW` plants one break, and the
   case runs in the copy. Each phase's record quotes the failing line. A
   Windows-only case is seen failing on the Windows test host.
4. **A deviation stops the phase.** The agent reports the evidence, a
   resolution (recommended first, with its cost) and what doing nothing
   costs. It records the owner's choice as a dated `Deviation Dn` here,
   and as an amendment in the MADR when a decision or fact changes. No
   workaround: no loosened assertion, no skipped test, no swallowed error.
5. **Identifiers.** Host results name the host by role ("this Mac", "the
   Windows test host") and paths as `<dir>`. Account names and SIDs are
   never quoted; a record says "the token's SID" or "the account".
6. **The Windows test host.**
   * Test and probe binaries are cross-compiled on this Mac with
     `CGO_ENABLED=0 GOOS=windows GOARCH=amd64`, copied over ssh into a new
     directory under the host's temporary directory, and run there. Go 1.27
     on the host is used only for `go test` runs a phase names.
   * **Every task a probe or test registers is named
     `selfupdate-probe-<random>` or `selfupdate-live-<random>`**, in the
     root folder, and is deleted when the run ends, pass or fail. No probe
     or test reads, changes or stops any other task.
   * **An interactive-token task runs only while the account is signed in
     at the console.** Before T1, T5 and T6, the agent asks the owner to
     confirm that it is. A run that finds no console session stops, as a
     deviation; it is not recorded as a skip.
7. **Each phase's record** goes under `## Execution record`: commands, exit
   statuses and the output lines that decide.

## Implementation Steps

### Phase T0: records

**Files:**

* `docs/decisions/0021-PLAN-task-scheduler-lifecycle-and-windowless-children.md`
  (this file)
* `docs/decisions/0021-MADR-task-scheduler-lifecycle-and-windowless-children.md`
* `docs/decisions/0011-MADR-reference-service-lifecycles.md`
* `docs/decisions/0004-MADR-evolve-selfupdate-api-and-tui-support.md`
* `docs/README.md`

1. The MADR is `accepted` with the owner's answers (done before T0 was
   approved, and committed in T0).
2. **0011-MADR**, each an annotation, not a rewrite:
   * §1's "`schtasks` stays in magic-cli-remote behind `Lifecycle`" gets
     "*(Amended 2026-10-10 by 0021-MADR D1: a Task Scheduler lifecycle is
     a reference backend here, `selfupdate/service/taskscheduler`.)*";
   * §9's flag list gets "*(Amended 2026-10-10 by 0021-MADR D7: the
     detached run starts with `CREATE_NO_WINDOW` in place of
     `DETACHED_PROCESS`.)*".
3. **0004-MADR**, line 849's "schtasks stays in magic-cli-remote behind
   `Lifecycle`" gets the same D1 annotation.
4. **`docs/README.md`** has this PLAN's row, and the MADR's row reads
   `accepted` (written with this PLAN, committed in T0).
5. **Checks:** links and identifiers on the five files. Commit.

### Phase T1: probes on the Windows test host

Nothing in the tree changes until step 8. The probe programs and scripts
live in the scratchpad on this Mac, and their binaries in a temporary
directory on the Windows test host.

**The probe binary**, `taskprobe.exe`, one Go program with modes chosen by
its first argument, because a task cannot carry environment (0159 F9).
Each mode writes one JSON line to a file in a directory named by its second
argument:

* `report`: its PID, its parent's PID and image name, its session ID,
  whether it is in a job (`IsProcessInJob`), and that job's limit flags
  (`QueryInformationJobObject`, `JobObjectExtendedLimitInformation`).
  With a third argument N, it then sleeps N seconds;
* `spawn`: `report`, then starts `report` twice more as children:
  * once with no flags;
  * once with today's `startDetached` flags, `DETACHED_PROCESS |
    CREATE_NEW_PROCESS_GROUP | CREATE_BREAKAWAY_FROM_JOB`, recording whether
    `CreateProcess` returned `ERROR_ACCESS_DENIED`, and retrying without
    `CREATE_BREAKAWAY_FROM_JOB` if it did, as `startDetached` does.

  Then it sleeps 120 s;
* `exit N`: `report`, then exits with code N.

The COM side is read and driven from PowerShell, through
`New-Object -ComObject Schedule.Service`. That is the same COM API the
backend will call, with no hand-written vtable code yet. Tasks are
registered by `schtasks /create /xml` from an XML file with a
`LogonTrigger`, an `InteractiveToken` principal, `MultipleInstancesPolicy`
`IgnoreNew` and one `Exec` action.

**Steps:**

1. **Preconditions.** The owner confirms the console sign-in (rule 6).
   `whoami /groups` shows the shell is not elevated (`Mandatory
   Label\Medium Mandatory Level`), and the record says so without quoting
   the account.
   *(Deviation D1, 2026-10-10: the ssh shell is elevated. Steps 2 to 6
   run inside a driver task at least privilege, and the driver records
   its own integrity level in place of this check.)*
2. **Probe 1, the instance's process (MADR D3).** Register a task whose
   action is `taskprobe.exe report <dir> 120`, and `Run` it. Read `GetInstances(0)`, and each
   instance's `EnginePID`, `CurrentAction` and `InstanceGuid`, and the
   task's `State`. Compare `EnginePID` with the PID the probe reported.
   * Equal: D3's first rule holds.
   * Not equal: record what `EnginePID` names (image, and whether it is the
     probe's parent), and whether D3's snapshot rule finds the probe.
3. **Probe 2, `Stop`'s reach and breakaway (MADR D6).** Register a task
   whose action is `taskprobe.exe spawn <dir>`, and `Run` it. Once all
   three reports exist, call `IRegisteredTask.Stop(0)`. After 5 s, record
   which of the three PIDs is still alive, with the creation time checked
   so a reused PID is not counted. The record states:
   * whether breakaway was allowed, and the job's limit flags;
   * whether `Stop` killed the plain child;
   * whether `Stop` killed the detached child.

   This is also the first run of D6's design. D6 stands if the detached
   child survives `Stop`.
4. **Probe 3, the last result (MADR D3).** Register a task whose action is
   `taskprobe.exe exit 7`, `Run` it, wait for `State` 3 (ready), and read
   `LastTaskResult` and `LastRunTime`. Repeat with `exit 0`. D3's failure
   detail stands as written if the results are 7 and 0.
5. **Probe 4, an update's user (MADR D5).** On a probe task:
   * read `Definition`, set `Actions.Item(1).Path` to a second copy of
     `taskprobe.exe` in another directory whose name holds a non-ASCII
     character, and call `RegisterTaskDefinition(name, def, 4, $null,
     $null, 3)` (`TASK_UPDATE`, no user, `TASK_LOGON_INTERACTIVE_TOKEN`);
   * read the task's XML back. Record whether the call succeeded, whether
     `Principal/UserId` is still the SID it was (compared, not quoted), and
     whether the path survived intact;
   * if the call failed, repeat it with the token's SID as the user, and
     record that.
6. **Probe 5, the error codes (MADR D2).** Record the HRESULT of:
   * `GetTask` for a name that does not exist;
   * an unelevated `RegisterTaskDefinition` of a new
     `selfupdate-probe-<random>` task into `\Microsoft\Windows\`, which is
     expected to be refused. If it is not refused, the task is deleted at
     once, and the record says so.
7. **Cleanup.** `schtasks /query /fo csv` lists no task whose name starts
   `selfupdate-probe-`, and the host's temporary directory for the run is
   removed.
8. **The record.** Each probe's output lines go into T1's execution record.
   The MADR's "Not verified" list moves each settled item to a dated
   amendment, `### Amendment A1 (<date>): probe results`, under More
   Information, which states each result and its effect on D2, D3, D5 or
   D6. The `[unverified]` marks it settles become `[measured]`.
9. **A result that contradicts a decision stops the phase** (rule 4).
   These are the expected cases:
   * `EnginePID` is neither the probe nor its parent, and the snapshot rule
     cannot find the probe: D3 has no instance identity;
   * the detached child dies with `Stop`: D6's hop is not enough, and the
     MADR's D6 names 0011 A4's rejected alternatives as the start;
   * `RegisterTaskDefinition` keeps the action but changes the principal:
     D5's rewrite is not safe as designed.
10. **Checks:** links and identifiers on the MADR and this PLAN. Commit.

**What T1 cannot settle:** whether a GitHub Windows runner can run an
interactive-token task. T6 finds out, by running there.

### Phase T2: `internal/winproc`, D7 and D8

**Files:**

* `internal/winproc/winproc.go`, `winproc_windows.go`, `winproc_unix.go`,
  `winproc_other.go`, and their tests
* `selfupdate/probe.go`
* `selfupdate/service/runner.go`, `detach_windows.go`, `detach_unix.go`
* `selfupdate/spawn_test.go` (the static ban)
* `.golangci.yml`, `AGENTS.md`

1. **The package.** `internal/winproc` imports only the standard library
   and `golang.org/x/sys/windows`. Its exported names are module-internal,
   because the package is under `internal/`. Planned names:
   * `Command(ctx, path string, args ...string) (*exec.Cmd, error)`, for a
     child this process waits for. On Windows it refuses a batch file
     (step 3) and sets `CREATE_NO_WINDOW` when `HasConsole` is false. Off
     Windows it is `exec.CommandContext`.
   * `Detached(path string, args []string, breakaway bool) (*exec.Cmd,
     error)`, for the handoff. On Windows its flags are `CREATE_NO_WINDOW |
     CREATE_NEW_PROCESS_GROUP`, plus `CREATE_BREAKAWAY_FROM_JOB` when
     `breakaway` is set. On Unix it sets `Setsid`, as `detach_unix.go` does
     today.
   * `HasConsole() bool`, on Windows only: `GetConsoleProcessList` through
     `windows.NewLazySystemDLL("kernel32.dll")`, non-zero meaning a
     console. It is read once, with `sync.OnceValue`.
2. **The four start sites** call the package. `probe.go:99` and
   `runner.go:67` call `Command`. Both `startWith`s call `Detached`, and
   `startDetached`'s retry without breakaway stays in `service`. Each
   site's `//nolint:gosec` reason moves with it.
3. **D8.** On Windows, `Command` and `Detached` build the `exec.Cmd` first,
   then refuse it when `strings.ToLower(filepath.Ext(cmd.Path))` is `.bat`
   or `.cmd`. The error wraps a sentinel, `winproc.ErrBatchFile`, and its
   text names BatBadBut. `runExec`, `versionProber.Probe` and
   `DetachProcess` return it wrapped in their own messages, which the
   tests check with `errors.Is`.
4. **depguard:**
   * a new rule, `internal-winproc`, over `**/internal/winproc/*.go` and
     not its tests, allowing `$gostd` and `golang.org/x/sys/windows$`;
   * `other-packages` excludes `**/internal/winproc/*.go`;
   * the `selfupdate` and `service` rules allow
     `github.com/maccavelli/go-selfupdate-lib/internal/winproc$`.
   `AGENTS.md`'s depguard list names `internal-winproc`.
5. **The static ban,** `TestNoDirectProcessStart` in
   `selfupdate/spawn_test.go`. It parses every non-test `.go` file under
   `selfupdate/` with `go/parser` and refuses a call to `exec.Command`,
   `exec.CommandContext`, `os.StartProcess` or `syscall.StartProcess`. The
   failure names the file and line. It resolves the import's local name,
   so an aliased `os/exec` is caught. It has an allowlist, empty today,
   whose entries each need a reason. A stale entry fails the test.
6. **Unit tests, on every OS:**
   * `Command` and `Detached` pass the path and arguments through;
   * off Windows, neither sets Windows flags.
7. **Windows tests**, in `internal/winproc` and run on the Windows test
   host and in CI's Windows job:
   * **D8:** `x.CMD`, `x.bat` and `x.Bat` are refused with
     `ErrBatchFile`. `x.exe` is accepted. An extensionless `x`, with only
     `x.cmd` beside it, is refused, which shows the check runs on the
     resolved path (MADR D8, derived). If `os/exec` does not resolve it
     that way, the case records what it does instead, and the phase stops
     (rule 4).
   * **D7, three deep:** `TestNoConsoleWindowThreeDeep`. The test binary
     re-runs itself as a parent that calls `FreeConsole` (a lazy
     `kernel32` procedure in the test) and starts a child through
     `Command`. The child starts a grandchild with no flags. Child and
     grandchild each report `GetConsoleWindow`, `IsWindowVisible` and
     `GetConsoleProcessList`. The test asserts neither has a visible
     window, and that the child has a console. A second case does the same
     through `Detached`.
8. **Fail first** (rule 3), each in a scratch copy:
   * the three-deep `Command` case, against a copy where `Command` sets no
     flags: the child reports a visible window. This is also the first run
     of the MADR's derived claim that today's children flash a window,
     and the record says so;
   * the three-deep `Detached` case, against a copy with today's
     `DETACHED_PROCESS`: the grandchild reports a visible window;
   * the D8 case, against a copy with the guard removed;
   * the static ban, against a copy of `probe.go` that calls
     `exec.CommandContext` directly: the test names the file and line;
   * depguard, against a copy that imports `golang.org/x/mod/semver` into
     `internal/winproc`: `make lint` fails, naming `internal-winproc`.
9. **Behaviour that must not change:** `TestDetachProcessLeavesJob`
   (`detach_windows_test.go`) passes unchanged, and so do the `scm` live
   tests on the Windows test host (`SELFUPDATE_REQUIRE_SCM=1`, from an
   administrator shell).
10. **Checks** (rule 2). Commit.

### Phase T3: the ancestor walk moves to `internal/winproc`

**Files:**

* `internal/winproc/tree.go`, `tree_test.go`
* `selfupdate/service/scm/detach.go`, `api.go`, `scm_windows.go`,
  `fake_test.go`
* `.golangci.yml`

1. **`winproc.Process`** (`PID`, `ParentPID`) and
   **`winproc.Descends(self, target uint32, procs []Process, created
   func(uint32) uint64) (bool, error)`** carry the loop of
   `scm.insideService` as it is: `maxAncestors`, the end at an exited
   parent, the end at a parent created after its child, and an unreadable
   time counted as a genuine link.
2. **`scm`** keeps its `manager.processes()` and `manager.created()`, and
   its `process` type becomes `winproc.Process`. `insideService` keeps the
   service query and calls `Descends`.
3. **depguard:** `service-scm` allows `internal/winproc$`.
4. **Tests.** `tree_test.go` carries table cases for each of the four
   ends. The existing `scm` tests (`detach_test.go`, `scm_test.go`) pass
   unchanged, which is the evidence that behaviour did not move. Each new
   case is seen failing against a planted break in `Descends`.
5. **Checks** (rule 2). Commit.

### Phase T4: `taskscheduler` over a fake

**Files:** `selfupdate/service/taskscheduler/`:

* `task.go`: the package doc, `Options`, `Task`, `New`, name checks;
* `api.go`: Task Scheduler's values, declared here, as `scm/api.go` does,
  so the logic and its fakes compile on every OS: `TASK_STATE`, trigger
  types (`TASK_TRIGGER_TIME` 1, `TASK_TRIGGER_BOOT` 8,
  `TASK_TRIGGER_LOGON` 9), logon types, `TASK_UPDATE`, and the two
  HRESULTs;
* `scheduler.go`: the unexported `scheduler` and `registered` interfaces;
* `lifecycle.go`: `Installed`, `Running`, `Enabled`, `Stop`, `Start`,
  `WaitHealthy`;
* `reconcile.go`: `Reconcile`, `Restore`, `samePath`;
* `detach.go`: `Inside`, `Detach`;
* `scheduler_other.go`: off Windows, `New` returns
  `service.ErrUnsupported`;
* tests: `fake_test.go`, `task_test.go`, `lifecycle_test.go`,
  `reconcile_test.go`, `detach_test.go`, `json_test.go`;
* `.golangci.yml`, `AGENTS.md`.

1. **`Options`** (MADR D1): `Name`, `Folder`, `Probe`, `Poll`,
   `RewritePath`, `RequestStop func(ctx context.Context, pid uint32)
   error`, `StopGrace time.Duration`. Each field's comment states its
   default and cites the MADR. A `Name` or `Folder` element holding `/`,
   or a `Name` holding `\`, is refused. `Folder` must start with `\`.
   `StopGrace` below zero is refused; zero means 10 s.
2. **The `scheduler` interface,** planned shape (T5 implements it on COM;
   the fake implements it in memory):
   * `open(folder, name string) (registered, error)`, where a missing task
     is `service.ErrNotInstalled`;
   * `processes() ([]winproc.Process, error)` and `created(pid uint32)
     uint64`, as in `scm`;
   * `images(pids []uint32) map[uint32]string`, for D3's snapshot rule if
     T1 needs it;
   * `tokenSID() (string, error)`.

   *(After T1: `images` and `tokenSID` are not needed. `get_EnginePID` is
   the action's process, and an update needs no user; MADR Amendment
   A1.)*

   `registered` has `state`, `enabled`, `setEnabled`, `run`, `stop`,
   `instances` (each with its `EnginePID`), `triggers` (type, enabled,
   whether it repeats), `action` (the `Exec` actions' `Path`, `Arguments`
   and `WorkingDirectory`, and the count of all actions), `lastRun`
   (result and time), `xml`, `setPath(path, user string)` (D5's rewrite)
   and `restore(xml, user string)`, and `close`.
3. **Probes** (MADR D3), with T1's results applied:
   * `Installed`: `open` succeeds; `ErrNotInstalled` is false, nil.
   * `Running`: state 4, or any instance.
   * `Enabled`: the task is enabled, and has an enabled logon trigger, boot
     trigger, or time trigger that repeats.
4. **`Stop`** (MADR D4), in this order: `Inside` refuses with
   `ErrInsideService`; record the instance's PID; disable when enabled,
   and record that it did, per task; `RequestStop` and a wait up to
   `StopGrace` for the PID to exit, when `RequestStop` is set; `stop`;
   then wait until no instance runs and the PID has exited. The wait is
   bounded by `Poll`'s timeout and, once begun, ignores the caller's
   context, by 0015-MADR B3, as `scm`'s does. A `RequestStop` error ends
   the grace at once and goes on to `stop`. It is part of `Stop`'s error
   only when `stop` also fails, as MADR D4 says.
   A task that is not running is still disabled, so its trigger cannot
   start it mid-update.
5. **`Start`**: re-enable when this `Task` disabled it, then `run`, unless
   an instance already runs.
6. **`WaitHealthy`** (MADR D3, as amended by T1): ready when running as an
   instance other than the recorded one, through `service.PollHealthy`,
   then `Options.Probe` with `Settle` -1, as `scm` does. Failed when not
   running and the last run began at or after `Start`, with the last
   result in the detail.
7. **`Reconcile` and `Restore`** (MADR D5, and Q6 of Amendment A1: a
   rewrite refuses a task whose logon type is not `InteractiveToken`,
   with an error naming it, and a table case pins that): refuse anything
   but exactly
   one `Exec` action; compare `Path` by `samePath`; without `RewritePath`,
   a moved binary is an error naming the option. With it, save `xml()` as
   the receipt's `State` (a struct with a JSON tag, by the memory rule on
   exported fields), call `setPath`, and report `Changed: true`.
   `Restore` calls `restore` with the saved XML. A receipt whose `State`
   is not this package's is refused.
8. **`Inside`** (MADR D6) calls `winproc.Descends` with the instance's
   PID. **`Detach`** is `scm.Detach`'s body: the hop.
9. **depguard:** a new rule, `service-taskscheduler`, allowing `$gostd`,
   `golang.org/x/sys/windows$`, `selfupdate$`, `selfupdate/service$` and
   `internal/winproc$`; `other-packages` excludes the package. `AGENTS.md`
   names the package in its scope paragraph and the rule in its list.
10. **Table tests over the fake**, each seen failing on a plant (rule 3):
    * not installed, for each method;
    * each `TASK_STATE` through `Running`;
    * the `Enabled` rule: logon, boot, repeating time, one-off time,
      disabled trigger, disabled task;
    * `Stop`'s call order, recorded by the fake: disable, `RequestStop`,
      `stop`, wait. And: no disable for a task already disabled; no
      `RequestStop` call when unset; `stop` after `StopGrace` when the
      process does not exit; the wait running past a cancelled context;
    * `Stop` from inside refused with `ErrInsideService`;
    * `Start` re-enabling only what `Stop` disabled, and not running a
      second instance;
    * `WaitHealthy`: a new instance is ready; the old one is not; a failed
      run reports its result;
    * `Reconcile`: a match, a match by case, a moved binary with and
      without `RewritePath`, two actions refused, a non-`Exec` action
      refused;
    * `Restore` re-registers exactly the saved XML, and refuses a foreign
      receipt;
    * the HRESULT map: `0x80070002` to `ErrNotInstalled`, `0x80070005` to
      `ErrPermission`, anything else wrapped with its code in hex;
    * `json_test.go`: a receipt survives a JSON round trip, as the journal
      needs.
11. **depguard fails first**, on a scratch copy importing
    `golang.org/x/mod/semver` into the package.
12. **`make apicheck`** reports the change compatible, with only the new
    package's names added.
13. **Checks** (rule 2). Commit.

### Phase T5: the COM transport

**Files:** `selfupdate/service/taskscheduler/scheduler_windows.go`,
`com_windows.go`, `com_windows_test.go`.

1. **`com_windows.go`:** the COM plumbing, through `x/sys/windows` and
   `syscall.SyscallN`, with no cgo:
   * lazy `ole32` (`CoCreateInstance`) and `oleaut32` (`SysAllocString`,
     `SysFreeString`, `VariantClear`) procedures, from
     `windows.NewLazySystemDLL`;
   * the CLSID and IIDs from `taskschd.h`, each with its source line in a
     comment;
   * one Go type per interface used (`ITaskService`, `ITaskFolder`,
     `IRegisteredTask`, `IRunningTaskCollection`, `IRunningTask`,
     `ITaskDefinition`, `IActionCollection`, `IAction`, `IExecAction`,
     `ITriggerCollection`, `ITrigger`, `IRepetitionPattern`), whose vtable
     slot numbers are constants, each commented with the `taskschd.h`
     method name and its position after `IDispatch`'s seven;
   * `VARIANT` as a fixed 24-byte struct on amd64 and arm64 (16 on 386),
     with a test asserting its size;
   * every call checks its HRESULT, and every interface pointer and BSTR
     is released on every path.
2. **`scheduler_windows.go`:** each `scheduler` call runs on a goroutine
   locked to its OS thread, with `CoInitializeEx(COINIT_MULTITHREADED)`
   and `CoUninitialize` around it, and a fresh `ITaskService::Connect` with
   empty arguments, the current user on the local machine. `processes` and
   `created` are `scm`'s code. `tokenSID` is
   `windows.GetCurrentProcessToken().GetTokenUser()`'s SID, as a string.
3. **The layout test,** `TestCOMLayout`, on the Windows test host and in
   CI's Windows job. It registers a `selfupdate-probe-<random>` task from
   a fixed XML, then calls every `scheduler` and `registered` method once,
   checking each result against the XML: state, enabled, triggers,
   action, XML, run, instances, stop, disable and re-enable, `setPath` to
   a non-ASCII directory and back, `restore`, and `open` of a missing name.
   It deletes the task in `t.Cleanup`. A wrong slot fails this test.
4. **Fail first:** on a scratch copy with two adjacent slot constants
   swapped (`get_State` and `get_Enabled`), `TestCOMLayout` fails,
   naming the method. The record quotes the line.
5. **CI.** If `TestCOMLayout` cannot register a task on a GitHub runner,
   it skips there with the HRESULT in the reason, and that is recorded.
   On the Windows test host it never skips.
6. **Checks** (rule 2), with `GOOS=windows` lint clean on the new files.
   Commit.

### Phase T6: the live test, and CI

**Files:** `selfupdate/service/taskscheduler/main_test.go`,
`live_windows_test.go`, `live_other_test.go`; `.github/workflows/ci.yml`.

1. **The mode.** `main_test.go` follows `scm/main_test.go`: an argument,
   `tasksched-live`, makes the test binary act as the task's program, its
   detached update, or a child that reports its console. The mode travels
   in the action's `Arguments`, because a task cannot carry environment
   (0159 F9).
2. **`TestLiveTaskUpdate`**, with `SELFUPDATE_REQUIRE_TASKSCHED=1`, else
   skipped with that reason. It copies the test binary to `<dir>`,
   registers `selfupdate-live-<random>` with a logon trigger and a time
   trigger repeating every minute (`IgnoreNew`), runs it, and from inside
   the task runs a managed update to a second copy through
   `service.HandOffIfInside`, the handoff and this backend. It asserts:
   * the update completes, and the handoff's result file says so;
   * the task is enabled afterwards;
   * the new instance's PID differs from the old one;
   * the detached run and the version probe's child each report no
     visible console window (D7, in the user's session).
3. **`TestLiveTaskStopHoldsDisabled`**: `Stop` on the running task, a
   70 s wait, longer than the repeat interval, then `Start`. The program
   logs each start; the test asserts no start between `Stop` and
   `Start`, and exactly one after. This is the relaunch 0159 probe 8
   measured, held off by the disable.
4. **`TestLiveTaskRollback`**: as `TestLiveTaskUpdate`, with a health
   probe that fails.
   It asserts the rollback ran, the definition's XML equals the saved XML
   byte for byte, and the task is enabled and running the old binary.
5. **`TestLiveTaskRequestStop`**: `RequestStop` writes a file the program
   watches; the program exits 0 within `StopGrace`, and `stop` is never
   reached. The fake scheduler cannot show this against a real task.
6. **Cleanup.** Each test deletes its task in `t.Cleanup` with
   `schtasks /delete /f`, and a final check lists no `selfupdate-live-`
   task.
7. **Fail first,** each on a scratch copy:
   * `Stop` without the disable: `TestLiveTaskStopHoldsDisabled` sees a
     start;
   * `Restore` as a no-op: the XML assertion fails;
   * D7 reverted: the console assertion fails.
8. **CI.** A step after "SCM live test", `if: runner.os == 'Windows'`,
   runs `go test -count=1 -v -run '^TestLiveTask' ./selfupdate/service/taskscheduler`.
   * It sets no `SELFUPDATE_REQUIRE_TASKSCHED` until a run shows the
     runner can run an interactive-token task. The skip reason names the
     missing console session.
   * If CI runs it and it passes, the same phase sets
     `SELFUPDATE_REQUIRE_TASKSCHED: "1"` on the step, and the record says
     which run showed it.
9. **The Windows test host** runs all four, three times each
   (`-count=3`), and the record quotes the `--- PASS` lines.
10. **Checks** (rule 2). Commit.

### Phase T7: documentation

**Files:**

* `docs/guides/extending-selfupdate.md`
* `docs/guides/migrating-from-mcplib-selfupdate.md`
* `docs/architecture.md`
* `README.md`

1. **`extending-selfupdate.md`, "Run as a service":**
   * a subsection, `### Task Scheduler`, covering:
     * `taskscheduler.New` and its options;
     * what the program's setup must create: one `Exec` action; a logon
       trigger; a repeating time trigger with `IgnoreNew` as the watchdog;
     * that `RestartOnFailure` does not restart a program that exits
       (0159 probe 8), so it is not a watchdog;
     * `RequestStop`, and that without it `Stop` terminates the process;
     * that a crash between disable and enable leaves the task disabled,
       and the recovery: the program's setup, or `schtasks /change /tn
       <name> /enable`;
     * that `-H windowsgui` breaks a CLI's output in PowerShell (0159 probe
       7);
   * the handoff's per-platform list (`:630-633`): the Task Scheduler line
     names the backend in place of `service.ProcessDetacher(inside)`, and
     the Windows line says the run has a windowless console;
   * a note under "Run as a service" that no library process start runs a
     `.bat` or `.cmd` file, and why.
2. **The migration guide** gains `## 14. From v1.13 to v1.14`, in §13's
   form: what is new (the backend; windowless children; the batch-file
   refusal), adopting it, and a check. §2's "current release" and the
   `go get` lines move in T8's pin commit.
3. **`docs/architecture.md`:** the tree gains `taskscheduler/` and
   `internal/winproc/`; the package table gains both rows, with counts
   from `ls <dir>/*.go | grep -v _test | wc -l` and the `_test` count,
   recorded; the backend bullets (`:227`) gain `taskscheduler`; the
   depguard list (`:447`) gains both rules.
4. **`README.md`**'s package row (`:42`) names `taskscheduler`, "a
   per-user Task Scheduler task".
5. **Checks:** links, identifiers and markdownlint on the four files;
   `make gate`. Commit.

### Phase T8: `v1.14.0`

0017-PLAN's "Release procedure" applies, with `vX = v1.14.0` and the
release commit T7's:

1. **CI** on `main` at T7's commit is green: every job, including T5's and
   T6's steps. The commits are pushed first, on the owner's ask.
2. **The live tests** are T2 step 9's and T6 step 8's records.
3. **The tag,** on the owner's ask in that turn:
   * `scripts/check-release-tag.sh v1.14.0`;
   * `git tag -a v1.14.0 -m v1.14.0 <commit>`;
   * the disclosure guard over the tag;
   * `git push origin v1.14.0`.
4. **The tag's state:** `ls-remote` gives the commit. The tag's CI passes
   all jobs, and the identity legs print `v1.14.0 (release) <12-hex>`.
5. **The pin commit,** whose candidates are
   `git grep -n 'v1\.13\.0\|5e199c8' -- README.md docs/architecture.md docs/guides`.
   Each changed line is named in the record. Lines that state what
   `v1.13.0` itself changed are kept.
6. **The proxy:** `go list -m -json …@v1.14.0` gives `Origin.Hash` the
   commit, and `@latest` resolves to `v1.14.0`.
7. **Release notes,** in this PLAN under `### Release notes for v1.14.0`.

### Phase T9: closing

1. `### Verification, at closing`: V1 to V9, each with its evidence.
2. `### Closing`, `status: complete`, and the index row. Commit, then push
   on the owner's ask.

## Verification

* **V1 (facts).** Each of the five probes has its output in T1's record,
  and the MADR's Amendment A1 states each result. No `[unverified]` mark
  that T1 could settle is left.
* **V2 (D7).** The three-deep tests failed against today's flags, on the
  Windows test host, and pass after T2, there and in CI's Windows job.
* **V3 (D8).** The batch-file cases, including the extensionless one,
  failed with the guard removed and pass with it.
* **V4 (static ban).** `TestNoDirectProcessStart` named the file and line
  of a planted call, and passes on the tree.
* **V5 (T3).** `scm`'s existing tests and live tests pass unchanged after
  the move.
* **V6 (backend logic).** Every T4 table case failed on its plant and
  passes.
* **V7 (COM).** `TestCOMLayout` failed on swapped slots and passes on the
  Windows test host. CI's result is recorded, pass or skip with its
  reason.
* **V8 (live).** The four live tests pass `-count=3` on the Windows test
  host. CI's result is recorded; if CI runs them, the step requires them.
* **V9 (API and release).** `make apicheck` is compatible with `v1.13.0`,
  and its report lists only `taskscheduler`'s names. `v1.14.0` is tagged
  at T7's commit (or a later release-shaped commit, recorded), its CI is
  green, the pins name it, and the proxy serves it.

## Rollout and Rollback

* **Rollout:** D7 and D8 reach every consumer in `v1.14.0`, with no
  option. A child that had a window now has none. A program that started
  a batch file through `ExecReconciler`, or targeted one, now gets
  `ErrBatchFile`; the migration guide says so. The backend does nothing
  until a program constructs it.
* **Rollback, before the tag:** revert T2 to T7 in reverse order. T3 is a
  pure move, and T2's start-site change stands alone.
* **Rollback, after the tag:** a release cannot be withdrawn, since the tag
  ruleset and the proxy keep it. A defect is fixed in `v1.14.1`. A program
  that hits a backend defect keeps its own adapter until then, as
  magic-cli-remote does today.
* **A deviation in any phase** stops it, by rule 4.

## Execution record

### Approval (2026-10-10)

The owner committed the MADR, this PLAN and the index rows as `bb09133`,
and said "proceed".

### T0 (2026-10-10)

* The MADR's acceptance and this PLAN's index row were already in
  `bb09133`, which the owner committed. T0 step 1 and step 4 needed no
  further change.
* **0011-MADR:** §1's `schtasks` line carries the D1 annotation, and
  §9's Windows flag list carries the D7 annotation.
* **0004-MADR:** line 849's `schtasks` line carries the D1 annotation.
* This PLAN is `in-progress`, here and in the index.
* Checks on the four changed files, each exit 0:
  * `scripts/check-docs.sh --ids`: `check-docs: 4 files, 16 deny-list
    rules, 0 findings`;
  * `scripts/check-docs.sh --links`: `check-docs: 132 links in 4 files, 0
    broken`.
* T0 has no Go files, so `make pre-add-check` does not apply. Rule 2's
  `make gate` ran on the commit's tree and exited 0: every step `rc=0`,
  `apicheck` `compatible with v1.13.0`, `links` `437 links in 60 files, 0
  broken`, `overall=0`.

### Deviation D1 (2026-10-10): the ssh shell on the Windows test host is elevated

* **Found.** T1 step 1, read-only over ssh: `whoami /groups` lists
  `BUILTIN\Administrators` and `Mandatory Label\High Mandatory Level`. A
  key-based ssh login on that host gets the account's full administrator
  token. That was already true before T1; earlier PLANs ran the `scm` live
  tests from that shell because they need an administrator.
* **Why it matters.** The backend's real caller is the task's own program,
  which runs unelevated. From an elevated shell, probe 5's refused
  registration would likely succeed, and probe 4's update may keep or set
  a principal differently. Probes 1 to 3 measure the task's process, not
  the caller.
* **Decision (the owner, 2026-10-10): run the probes through a driver
  task.** The ssh shell only copies the files, registers
  `selfupdate-probe-driver-<random>` (`InteractiveToken`, `LeastPrivilege`,
  no trigger), runs it, waits for it to finish, deletes it and reads its
  results. The driver, a PowerShell script, runs steps 2 to 6 itself, in
  the console session at medium integrity, which is where the backend
  will run. It records its own `Mandatory Label` line and session ID as
  the evidence that replaces step 1's check. The owner confirmed the
  console sign-in.
* **Files added to the phase:** none in the tree. The driver,
  `common.ps1`, `start-driver.ps1` and `taskprobe.exe` live in the
  scratchpad and on the host, as T1 already said for its probes.
* **Still to settle:** T5's `TestCOMLayout` and T6's live tests are run with
  `go test` from the same elevated shell (rule 6). Before each of those
  phases, the agent says how the elevation affects it and asks before
  going on.

### T1 (2026-10-10)

**Preconditions.** The owner confirmed the console sign-in. The ssh shell
is elevated, which stopped step 1 (Deviation D1); the owner chose the
driver task. The driver's own lines prove its context:
`driver.integrity=Mandatory Label\Medium Mandatory Level` and
`driver.session=1`, the console session.

**What was built, outside the tree:**

* `taskprobe.exe`, a Go program with the modes step 1 names, built on this
  Mac with `CGO_ENABLED=0 GOOS=windows GOARCH=amd64` (Go 1.27.2) and
  `go vet` clean. It uses only the standard library's `syscall`. Each
  report carries its PID, parent, session, job membership and limit
  flags, creation time, console process count, console window and its
  visibility, and its executable path. The driver never prints the path;
  it compares it.
* `common.ps1` (task XML, `schtasks /create /xml` from a UTF-16 file, the
  HRESULT and redaction helpers), `driver.ps1` (steps 2 to 6) and
  `start-driver.ps1` (Deviation D1's wrapper).
* Each run used a new directory under the host's temporary directory, and
  task names `selfupdate-probe-<random>-p1` to `-p5` and
  `selfupdate-probe-driver-<random>`.

**Runs, in order.** Six runs; runs 3, 5 and 6 are complete and agree on
every measured value.

| Run | Outcome | Why |
| :--- | :--- | :--- |
| 1 | driver exited 1, no results | the driver's result helper was named `R`, which is PowerShell's built-in alias for `Invoke-History`; an alias wins over a function |
| 2 | returned at once | run 1 had left `driver.done`, which ended the wrapper's wait. The driver task's last result was `0x00041301` (still running) when the wrapper deleted it; its log showed the same alias error |
| 3 | complete | the helper renamed `Write-Result`, and the wrapper clears `driver.done`, `results.txt` and `driver.log` first. `p4.ran_from_new_path=False`, see below |
| 4 | **invalid** | it reused run 3's directory, so every wait found run 3's report files: its PIDs are run 3's. Not used |
| 5 | complete, new directory | the same values as run 3, including `p4.ran_from_new_path=False` |
| 6 | complete, new directory | the record below; `p4.ran_from_new_path=True` |

`p4.ran_from_new_path` was a defect in the driver, not a measurement.
Windows PowerShell 5.1 reads a BOM-less file as the ANSI code page, so the
UTF-8 report's non-ASCII path was mis-decoded before the comparison. Read
directly with `-Encoding UTF8`, run 5's report compared equal
(`equal_ceq=True`, both 81 characters), and the task had run from the
non-ASCII directory. The fix (`Get-Content -Raw -Encoding UTF8`) was meant
to land before run 4, but its `sed` ran behind a `grep` guard that did not
match, so it was skipped and runs 4 and 5 ran without it. It was applied
with an exact-match edit before run 6, and that edit was checked
(`grep -c` = 1). A second `sed`, a guard refusing an existing probe
directory, also failed to land (`grep -c` = 0). Run 6's protection against
stale files is its new directory alone.

**Run 6, verbatim** (the random directory is `<tmp>\selfupdate-probe-<random>`
on the host; the account's profile path is `<home>`):

```text
driver.task_last_result=0x00000000
driver.integrity=Mandatory Label\Medium Mandatory Level
driver.session=1
p1.report={"tag":"main","pid":39516,"ppid":3464,"parent":"svchost.exe","session":1,"in_job":true,"job_flags":"0x00000000","created":134361354258596354,"console_procs":1,"console_hwnd":"0x1f0894","visible":true}
p1.state=4
p1.instances=1
p1.enginepid_is_probe=True
p1.enginepid_is_probe_parent=False
p1.engine_image=taskprobe
p1.current_action=<tmp>\selfupdate-probe-9a312b87\taskprobe.exe
p1.probe_alive_after_stop=False
p1.state_after_stop=3
p2.report.spawn={"tag":"spawn","pid":4796,"ppid":3464,"parent":"svchost.exe","session":1,"in_job":true,"job_flags":"0x00000000","created":134361354292894429,"console_procs":1,"console_hwnd":"0x2a03b6","visible":true,"plain_pid":8988,"detached_pid":38848,"breakaway_err":"fork/exec <tmp>\\selfupdate-probe-9a312b87\\taskprobe.exe: Access is denied.","detached_flags":"0x00000208"}
p2.report.plain={"tag":"plain","pid":8988,"ppid":4796,"parent":"taskprobe.exe","session":1,"in_job":true,"job_flags":"0x00000000","created":134361354293996095,"console_procs":2,"console_hwnd":"0x2a03b6","visible":true}
p2.report.detached={"tag":"detached","pid":38848,"ppid":4796,"parent":"taskprobe.exe","session":1,"in_job":true,"job_flags":"0x00000000","created":134361354294039366,"console_procs":0,"console_hwnd":"0x0","visible":false}
p2.enginepid_is_spawn=True
p2.alive_after_stop.spawn=False
p2.alive_after_stop.plain=True
p2.alive_after_stop.detached=True
p2.state_after_stop=3
p3.exit7.settled=True
p3.exit7.state=3
p3.exit7.last_task_result=7
p3.exit7.last_task_result_hex=0x00000007
p3.exit7.last_run_time_advanced=True
p3.exit7.last_run_time_at_or_after_run_call=True
p3.exit0.settled=True
p3.exit0.state=3
p3.exit0.last_task_result=0
p3.exit0.last_task_result_hex=0x00000000
p3.exit0.last_run_time_advanced=True
p3.exit0.last_run_time_at_or_after_run_call=True
p4.principal_before_is_token_sid=True
p4.update_no_user=ok
p4.principal_unchanged=True
p4.logon_type_after=InteractiveToken
p4.trigger_userid_unchanged=True
p4.path_intact=True
p4.xml_equal_but_command=True
p4.ran_from_new_path=True
p4.restore_xml_equal=True
p5.missing_hresult=0x80070002
p5.system_folder_hresult=0x80070005
cleanup.killed.plain=8988
cleanup.killed.detached=38848
```

**Reading it** (MADR Amendment A1 states each result and its effect):

* Probe 1: `get_EnginePID` is the action's process. `State` 4 running, 3
  after `Stop`.
* Probe 2: the job's limit flags are `0x00000000`; breakaway is refused
  and the retry succeeds (`detached_flags=0x00000208`). `Stop` ended only
  the action's process: `alive_after_stop.plain=True` and
  `.detached=True`. Both were then killed by the driver's cleanup
  (`cleanup.killed.*`).
* Probe 3: `LastTaskResult` is the exit code.
* Probe 4: an update with no user keeps the principal and every other
  byte, and a restore of the saved XML is byte-identical.
* Probe 5: `0x80070002` for a missing task, `0x80070005` for a refused
  registration.

**None of step 9's stop conditions arose.** `get_EnginePID` is the
probe; the detached child survived `Stop`; the update kept the principal.
Breakaway was refused, which the MADR's D6 said would amend the record:
Amendment A1 does, and finds D6 stands, because the hop's run survives
inside the job. Probe 4 measured `InteractiveToken` only, which raises
owner question Q6 in the MADR. Q6 blocks T4's rewrite path, not T2 or T3.

**Cleanup (step 7).** After run 6, from the ssh shell:

* `schtasks /query /fo csv`: 0 lines naming `selfupdate-probe`.
* `tasklist`: 2 `taskprobe` processes at the first sweep. 8 s later a
  `Win32_Process` query found none, so their command lines could not be
  read. They were most likely run 4's children: run 4's stale waits never
  reached its cleanup, and its children sleep 120 s. That is inferred,
  not shown.
* The three run directories were removed. A final check found 0 probe
  directories, 0 `selfupdate-` tasks and 0 `taskprobe` processes.

* Checks: `scripts/check-docs.sh --ids` and `--links` on the MADR and
  this PLAN, 0 findings and 0 broken; `make gate` `overall=0`.

### Owner decision Q6 (2026-10-10)

Asked after T1's commit: under `RewritePath`, `Reconcile` refuses a task
whose logon type is not `InteractiveToken`, the recommended option. Recorded
in MADR Amendment A1 under Q6, and in T4 step 7. It changes T4 only.
