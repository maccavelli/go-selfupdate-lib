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
| T6a | Deviations D3 to D5: the live tests that did not fail first, the wait and `S_FALSE` changes made in T6, the medium-integrity evidence | one commit (approved, 2026-10-10) |
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
     *(Deviation D2, 2026-10-10: it does not; `checkProgram` resolves the
     path with `exec.LookPath`. MADR Amendment A2.)*
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

Associated MADR: Amendment A3 is normative for this phase. Implement the
T4 `scheduler` and `registered` interfaces on the COM API. The exported
package still only manages a registered task.

**Files:**

* `selfupdate/service/taskscheduler/com_windows.go` (new, `//go:build windows`)
* `selfupdate/service/taskscheduler/scheduler_windows.go` (new, `//go:build windows`)
* `selfupdate/service/taskscheduler/com_windows_test.go` (new, `//go:build windows`)
* `selfupdate/service/taskscheduler/scheduler_other.go` (add `//go:build !windows`)

**Preconditions.** Rule 6: the owner confirms the console sign-in. Elevation
is the owner decision already recorded: iterate `TestCOMLayout` from the
elevated ssh shell, then run the final test binary once as the action of a
throwaway least-privilege InteractiveToken task in the console session.

#### 1. `com_windows.go`: plumbing

Lazy system DLLs, `windows.NewLazySystemDLL`, same form as
`internal/winproc`:

* `ole32.dll`: `CoCreateInstance`, `CoInitializeEx` (Amendment A3: the
  lazy procedure, because `windows.CoInitializeEx` treats `S_FALSE` as
  an error). `CoUninitialize` may use `windows.CoUninitialize`.
* `oleaut32.dll`: `SysAllocString`, `SysFreeString`, `VariantClear`.

GUIDs as `windows.GUID` literals, each commented with the IDL uuid. Values:

* `CLSID_TaskScheduler` `{0f87369f-a4e5-4cfc-bd3e-73e6154572dd}`
* `IID_ITaskService` `{2faba4c7-4da9-4013-9697-20cc3fd40f85}`
* `IID_ITaskFolder` `{8cfac062-a080-4c15-9a88-aa7c2af80dfc}`
* `IID_IRegisteredTask` `{9c86f320-dee3-4dd1-b972-a303f26b061e}`
* `IID_IRunningTask` `{653758fb-7b9a-4f1e-a471-beeb8e9b834e}`
* `IID_IRunningTaskCollection` `{6a67614b-6828-4fec-aa54-6d52e8f1f2db}`
* `IID_ITaskDefinition` `{f5bc8fc5-536d-4f77-b852-fbc1356fdeb6}`
* `IID_IActionCollection` `{02820e19-7b98-4ed2-b2e8-fdccceff619b}`
* `IID_IAction` `{bae54997-48b1-4cbe-9965-d6be263ebea4}`
* `IID_IExecAction` `{4c3d624d-fd6b-49a3-b9b7-09cb3cd3f047}`
* `IID_ITriggerCollection` `{85df5081-1b24-4f32-878a-d9d14df4cb77}`
* `IID_ITrigger` `{09941815-ea89-4b5b-89e0-2a773801fac3}`
* `IID_IRepetitionPattern` `{7fb9acf1-26be-400e-85b5-294b9c75dfd6}`
* `IID_IPrincipal` `{d98d51e5-c9b4-496a-a9c1-18980261cf0f}`

HRESULT helper: `uint32(r0)`; `failed` if bit 31 is set; `S_OK` 0,
`S_FALSE` 1, `RPC_E_CHANGED_MODE` `0x80010106`. Map `0x80070002` /
`0x80070005` onto the existing `hresult` sentinels in `api.go`.

`VARIANT`:

```text
type variant struct {
    vt       uint16
    reserved [3]uint16
    data     [16]byte // 8 on 386: use build-tagged files if sizeof fails
}
```

`TestVARIANTSize` asserts 24 on amd64/arm64 and 16 on 386. Zero value is
`VT_EMPTY`. Integer index: `vt = 3` (`VT_I4`), `lVal` in `data`.
`VARIANT_BOOL` true is `int16(-1)`.

BSTR helpers: `sysString(s string) (bstr, error)` via UTF-16 and
`SysAllocString`; `freeBSTR`; `bstrToString` copies then frees. NULL BSTR
is `""`. `SysAllocString` returning NULL on a non-empty input is an error.

One Go type per interface, first field `vtbl *struct{ … uintptr }`. Slots
0–6 are IUnknown + IDispatch on every type. Interface slots, 0-based after
those seven, commented `// ITaskService::GetFolder, slot 7`:

**`ITaskService`:** 7 `GetFolder`, 8 `GetRunningTasks` (unused, keep the
slot), 9 `NewTask` (unused), 10 `Connect`.

**`ITaskFolder`:** 7 `get_Name` (unused), 8 `get_Path` (unused), 9
`GetFolder` (unused), 10 `GetFolders` (unused), 11 `CreateFolder`
(unused), 12 `DeleteFolder` (unused), 13 `GetTask`, 14 `GetTasks`
(unused), 15 `DeleteTask` (unused), 16 `RegisterTask`, 17
`RegisterTaskDefinition`.

**`IRegisteredTask`:** 7 `get_Name`, 8 `get_Path`, 9 `get_State`, 10
`get_Enabled`, 11 `put_Enabled`, 12 `Run`, 13 `RunEx` (unused), 14
`GetInstances`, 15 `get_LastRunTime`, 16 `get_LastTaskResult`, 17
`get_NumberOfMissedRuns` (unused), 18 `get_NextRunTime` (unused), 19
`get_Definition`, 20 `get_Xml`, 21 `GetSecurityDescriptor` (unused), 22
`SetSecurityDescriptor` (unused), 23 `Stop`.

**`IRunningTaskCollection`:** 7 `get_Count`, 8 `get_Item` (`VARIANT`), 9
`get__NewEnum` (unused).

**`IRunningTask`:** 7 `get_Name` (unused), 8 `get_InstanceGuid` (unused),
9 `get_Path` (unused), 10 `get_State` (unused), 11 `get_CurrentAction`
(unused), 12 `Stop` (unused), 13 `Refresh` (unused), 14 `get_EnginePID`.

**`ITaskDefinition`:** 7 `get_RegistrationInfo` (unused), 8 put unused, 9
`get_Triggers`, 10 put unused, 11 `get_Settings` (unused), 12 put unused,
13 `get_Data` (unused), 14 put unused, 15 `get_Principal`, 16 put unused,
17 `get_Actions`.

**`IActionCollection`:** 7 `get_Count`, 8 `get_Item` (`long`), 1-based.

**`IAction`:** 7 `get_Id` (unused), 8 `put_Id` (unused), 9 `get_Type`.

**`IExecAction`:** IAction plus 10 `get_Path`, 11 `put_Path`, 12
`get_Arguments`, 13 `put_Arguments` (unused), 14 `get_WorkingDirectory`
(unused), 15 put unused.

**`ITriggerCollection`:** 7 `get_Count`, 8 `get_Item` (`long`), 1-based.

**`ITrigger`** (IDL order, slots 7–19): 7 `get_Type`, 8 `get_Id`
(unused), 9 `put_Id` (unused), 10 `get_Repetition`, 11 `put_Repetition`
(unused), 12 `get_ExecutionTimeLimit` (unused), 13 put unused, 14
`get_StartBoundary` (unused), 15 put unused, 16 `get_EndBoundary`
(unused), 17 put unused, 18 `get_Enabled`, 19 `put_Enabled` (unused).
`TestCOMLayout` is the check if a slot is off.

**`IRepetitionPattern`:** 7 `get_Interval`. Empty or NULL interval is
`Repeats: false`.

**`IPrincipal`:** 7 `get_Id` (unused) through 13 `get_LogonType` (IDL:
Id, put Id, DisplayName, put DisplayName, UserId, put UserId, LogonType).
`get_LogonType` is slot 13.

Every vtable call: `this` first, then arguments, `syscall.SyscallN`,
`runtime.KeepAlive` on `this` and any `VARIANT` / UTF-16 buffer.

Release helper: `IUnknown::Release` (slot 2). `close` on `registered` is
that Release.

#### 2. `scheduler_windows.go`: T4 interfaces

`systemScheduler()` returns a `sysSched{}`.

Each COM method:

1. `runtime.LockOSThread()`; `defer runtime.UnlockOSThread()`.
2. `CoInitializeEx(0, windows.COINIT_MULTITHREADED)`.
   * `S_OK` or `S_FALSE`: `defer CoUninitialize()`.
   * `RPC_E_CHANGED_MODE`: proceed, no uninit. Record this HRESULT in
     `TestCOMLayout`.
   * other `failed`: return it.
3. `CoCreateInstance(CLSID_TaskScheduler, nil, CLSCTX_INPROC_SERVER,
   IID_ITaskService, &svc)`. Out pointer starts nil.
4. `Connect` with four zero `VARIANT`s.
5. Work. `defer` Release on `svc` and every other interface acquired.
6. Return.

`open(folder, name)`: `GetFolder(folder)` then `GetTask(name)`. Missing
task is `hrFileNotFound`. Keep the folder pointer on the `registered`
value so `setPath` / `restore` can `RegisterTaskDefinition` /
`RegisterTask` without a second Connect if that is simpler; a second
Connect per call is also allowed, as the PLAN originally said. Prefer one
Connect per exported method, releasing everything before return, so no
COM object outlives the apartment on that thread.

`state` / `enabled` / `setEnabled` / `run` / `stop` / `xml` / `lastRun`:
the `IRegisteredTask` slots above. `setEnabled` passes `int16(-1)` or 0.
`run` passes `VT_EMPTY` params and a NULL `IRunningTask**`; Release the
running task if one is returned. `stop` flags 0; `S_FALSE` is an error
whose text names that the user cannot stop the instances.

`instances`: `GetInstances(0)`; `Count`; `Item` 1..Count with `VARIANT`
`VT_I4`; `EnginePID`. Skip a PID of 0.

`actions`: `get_Definition` → `get_Actions` → Count / Item (`long`) →
`get_Type`; if Exec, `QueryInterface(IID_IExecAction)` → `get_Path`,
`get_Arguments`.

`triggers`: `get_Definition` → `get_Triggers` → Count / Item (`long`) →
`get_Type`, `get_Enabled`, `get_Repetition` → `get_Interval`.

`logonType`: `get_Definition` → `get_Principal` → `get_LogonType`.

`setPath(path)`: `get_Definition` → one Exec `put_Path` →
`RegisterTaskDefinition(name, def, TASK_UPDATE|TASK_IGNORE_REGISTRATION_TRIGGERS
/* 0x24 */, empty, empty, logonType, empty, &out)`. Release `out`.

`restore(xml)`: `RegisterTask(name, xml, 0x24, empty, empty,
logonInteractiveToken, empty, &out)`. Release `out`. The saved XML
already carries the principal; passing `logonInteractiveToken` matches
Q6's only measured logon. If `TestCOMLayout` shows restore needs the
XML's own logon type instead, that is a deviation (rule 4).

`processes` / `created`: copy `scm/scm_windows.go` (Toolhelp32 snapshot,
`GetProcessTimes`). `running(pid, born)`: `created(pid) == born` when
`born != 0`, else the pid is still in the snapshot.

Constants in `api.go` if T4 does not already have them: `taskUpdate = 4`,
`taskIgnoreRegistrationTriggers = 0x20`. Add them in this commit if
missing.

#### 3. `TestCOMLayout`

On the Windows test host and in CI's Windows job. Registers
`selfupdate-probe-<random>` in `\`, from a fixed UTF-16 XML: one Exec
(`taskprobe` or the test binary with a sleep argument), InteractiveToken,
LeastPrivilege, a logon trigger, `IgnoreNew`, `AllowDemandStart` left
default. Deletes in `t.Cleanup` with `ITaskFolder::DeleteTask` or
`schtasks /delete /f`. Never touches another task.

Then, in order, through the real `scheduler` / `registered` (not the
fake):

1. Record `CoInitializeEx`'s HRESULT (`S_OK`, `S_FALSE`, or
   `RPC_E_CHANGED_MODE`).
2. `open` of a missing name: `hrFileNotFound`.
3. `open` of the probe task.
4. Before `Run`: record `lastRun` (`DATE` bits and `LastTaskResult`). This
   is the never-run sentinel. Do not treat `0x00041303` as a failed
   `open`.
5. `state`, `enabled` (true), `triggers` (one logon, enabled), `actions`
   (one Exec, path matches, case-insensitive), `logonType`
   (`logonInteractiveToken`), `xml` contains the path.
6. `run`; `instances` length 1; `EnginePID` is a live process whose image
   is the Exec path.
7. `setEnabled(false)` then `enabled` false; `setEnabled(true)`.
8. `stop`; after a short wait, `instances` empty and `state` is ready or
   disabled, not running.
9. `setPath` to a copy of the binary under a directory whose name holds
   `é`, `ü` and a CJK character; `xml` contains that path; `actions` Path
   matches; principal / logon type unchanged.
10. `restore` of the XML saved before `setPath`; `xml` equals the saved
    string byte for byte.
11. `close`.

Fail first (rule 3), on a scratch copy with `get_State` and `get_Enabled`
slot constants swapped: the test names the method. Quote the line.

CI: if registration returns a permission HRESULT, skip with that HRESULT
in the reason. On the Windows test host it never skips.

#### 4. Medium-integrity confirmation

After the elevated-shell run is green, copy the same test binary into a
new host directory, register `selfupdate-probe-driver-<random>`
(InteractiveToken, LeastPrivilege, no trigger) whose action is that
binary's `TestCOMLayout` (or a tiny driver that execs it), wait, delete.
The driver's log must include `--- PASS: TestCOMLayout`. That run is the
medium-integrity evidence, as the owner decided before T5.

#### 5. Fail-first plants (this Mac or the host as noted)

| Plant | Where | What fails |
| :--- | :--- | :--- |
| `get_State` / `get_Enabled` slots swapped | Windows test host | `TestCOMLayout` names the method |
| `VARIANT` size asserted 16 on amd64 | this Mac with `GOARCH=amd64` | `TestVARIANTSize` |
| `CoInitializeEx` treated as failed on `S_FALSE` | Windows test host, if init returns 1 | `open` errors on a live task |
| `Stop` treats `S_FALSE` as success | only if a case can produce `S_FALSE`; otherwise record that it could not be planted | |
| `Item` 0-based | Windows test host | `instances` or `actions` empty / `E_INVALIDARG` |
| `put_Enabled(1)` instead of `-1` | Windows test host | `enabled` stays false after enable, or the plant is recorded as not distinguishing | |

#### 6. Checks

`GOOS=windows` lint clean on the new files. `make pre-add-check
FILES="<the phase's Go files>"`. `make gate` `overall=0`. Commit.

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
   trigger repeating every minute (`IgnoreNew`), `AllowDemandStart` left
   at its default (true; Amendment A3: `Run` is a no-op when it is
   false), runs it, and from inside
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

### Phase T6a: the checks T6 did not prove, and T6's code changes

Added 2026-10-10 by Deviations D3, D4 and D5 (Execution record), after
T6's commit `026faaa`. **Approved by the owner on 2026-10-10**, with Q7
(MADR Amendment A4) and Q8 (Deviation D5) answered as recommended.

**Files:** `selfupdate/service/taskscheduler/`: `lifecycle.go`,
`lifecycle_test.go`, `fake_test.go`, `com_windows_test.go`,
`live_windows_test.go`.

1. **The watchdog trigger fires** (D5). `liveXML`'s `TimeTrigger` has a
   `StartBoundary` of the registration time, in the host's local time, and
   no `Duration`, so it repeats every minute with no end. Today's
   `2020-01-01T00:00:00` with `P1D` stopped repeating on 2020-01-02, so the
   disable in `Stop` was never tested.
2. **The rollback restores a rewritten definition** (D5).
   `TestLiveTaskRollback` registers the task with its `Command` at a copy
   of the program under `<dir>\old\`, while the install target is
   `<dir>\demo.exe`. `Reconcile` under `RewritePath` then rewrites the
   path, and the failed health check's rollback must call `Restore`. The
   existing assertions stay: the XML equals the saved XML byte for byte,
   and the task is enabled and running. Step 8's `Restore` plant failing
   the XML assertion is the evidence that the rewrite happened. Today it
   passes, because the task already ran the target and nothing was
   rewritten.
3. **`RequestStop` ends the program without `stop`** (T6 step 5).
   `TestLiveTaskRequestStop` wraps the `Task`'s scheduler in a test
   decorator that counts `registered.stop` calls, and asserts the count is
   zero.
4. **The medium-integrity evidence** (D3). In `TestLiveTaskUpdate`, the
   detached update records its token's integrity RID
   (`GetTokenInformation`, `TokenIntegrityLevel`) in the handoff
   directory. The test asserts `SECURITY_MANDATORY_MEDIUM_RID` (`0x2000`).
   That run's `Stop`, `Reconcile`, `Start` and `WaitHealthy` are then the
   backend's calls at medium integrity, made from inside a least-privilege
   `InteractiveToken` task. They replace T5 step 4's driver run, which
   Deviation D3 found cannot register a task.
5. **`TestCOMLayout` measures T6's transport changes** (D4):
   * before the first `run`, the never-run task's raw
     `IRegisteredTask::GetInstances` HRESULT is logged, and `instances()`
     returns no PIDs and no error;
   * after `stop` and `waitInstances(t, r, 0)`, the raw
     `IRegisteredTask::Stop` HRESULT on the idle task is logged, and
     `stop()` returns nil;
   * `toggleEnabled` reads `enabled` back as true after
     `setEnabled(true)`.
6. **`waitGone` waits on the recorded process** (D4, MADR Amendment A4).
   `Stop` calls it only when it recorded an instance. It polls
   `running(pid, born)` and no longer lists instances, which removes the
   two branches `026faaa` left unreachable. The doc comments on `Stop`
   (step 5) and on `waitGone` say what it waits for.
   `TestStopStaleInstance` (fake): the fake keeps the PID listed after
   the process exits, and `Stop` returns nil, not `ErrTimeout`.
7. **`Start` after a stale listing** (Amendment A4 Q7, the owner's
   choice). `Start` does not count a listed instance whose
   PID is the one `Stop` recorded and whose process has exited.
   `TestStartStaleInstance` (fake): `Start` runs the task.
8. **Fail first** (rule 3), each in a `scripts/plant-copy.sh` copy:

   | Plant | Where | Must fail |
   | :--- | :--- | :--- |
   | `Stop` without the disable | the Windows test host | `TestLiveTaskStopHoldsDisabled`: a start while disabled |
   | `Restore` a no-op | the Windows test host | `TestLiveTaskRollback`: the XML differs |
   | `Stop` calls `stop` after the program exited in the grace period | the Windows test host | `TestLiveTaskRequestStop`: one `stop` call |
   | the task's `RunLevel` `HighestAvailable` | the Windows test host | `TestLiveTaskUpdate`: the integrity is not medium |
   | `instances` without the `0x8004130B` mapping | the Windows test host | `TestCOMLayout`, if the never-run task returns it; otherwise recorded as not reachable |
   | `stop` with `S_FALSE` an error again | the Windows test host | `TestCOMLayout`, if the idle `Stop` returns `S_FALSE`; otherwise recorded as not reachable |
   | `put_Enabled(1)` | the Windows test host | `TestCOMLayout`'s read-back, or recorded as not distinguishing if Windows reads any non-zero as true |
   | `waitGone` requires an empty listing | this Mac | `TestStopStaleInstance`: `ErrTimeout` |
   | `Start` counts the stale instance | this Mac | `TestStartStaleInstance`: no `run` |

   The D7 plant needs no repeat: T6's record shows it failing.
9. **The Windows test host** runs `TestCOMLayout` and the four live tests
   with `-count=3`, after the owner confirms the console sign-in
   (rule 6). The record quotes the `--- PASS` lines.
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

### Deviation D2 (2026-10-10): `os/exec` does not write the resolved path back

* **Found.** T2 step 7, the first run of `internal-winproc.test.exe` on the
  Windows test host. Every case passed except the extensionless one:

  ```text
  winproc_windows_test.go:154: Command(shim): refused false (<nil>), want true
  winproc_windows_test.go:158: Detached(shim): refused false (<nil>), want true
  --- FAIL: TestBatchFileRefused (0.00s)
  ```

  Go 1.27.2's `os/exec/exec.go:468-481` and `:690-707`: for an absolute
  path, `exec.Command` resolves the extension into `cachedLookExtensions`
  and `Start` passes it to `os.StartProcess`, leaving `cmd.Path` as given
  (go.dev/issue/62596). The MADR's D8 had assumed `cmd.Path` held the
  resolved path, marked `[derived]`. The gap is new code's, not
  pre-existing: nothing was guarded before T2.
* **Decision (the owner):** on Windows, `checkProgram` resolves an absolute
  path with `exec.LookPath`, the same `PATHEXT` search `Start` makes, and
  checks the result's extension. The test stays as written. MADR
  Amendment A2 records it.
* **Rejected:** refusing every extensionless path on Windows.
* **Files added to the phase:** none.

### T2 (2026-10-10)

**Files.** New: `internal/winproc/winproc.go`, `winproc_windows.go`,
`winproc_unix.go`, `winproc_other.go`, `winproc_test.go`,
`winproc_unix_test.go`, `winproc_windows_test.go`;
`selfupdate/spawn_test.go`, `selfupdate/probe_batch_windows_test.go`,
`selfupdate/service/batch_windows_test.go`. Changed: `selfupdate/probe.go`,
`selfupdate/service/runner.go`, `detach_unix.go`, `detach_windows.go`,
`.golangci.yml`, `AGENTS.md`.

**As built, against the plan's planned names:**

* `winproc.Command(ctx, path, args...)` and `winproc.Detached(path, args,
  breakaway)`, as planned. `winproc.ErrBatchFile` is the sentinel.
* The console check is unexported, `hasConsole`, because nothing outside
  the package needs it. It is `GetConsoleProcessList` from
  `windows.NewLazySystemDLL("kernel32.dll")`, under `sync.OnceValue`.
* `checkProgram` resolves an absolute path with `exec.LookPath` before
  checking the extension (Deviation D2).
* The four start sites call the package; `detachFlags` is gone, and the
  Windows `startWith` takes `breakaway bool`. The retry on
  `ERROR_ACCESS_DENIED` stays in `service`.
* The `getConsoleProcessList.Call` line carries
  `//nolint:errcheck,gosec`, with its reason, the repository's form for a
  checked-by-design call (`hopwait_windows.go:19`).
* depguard: `internal-winproc` (`$gostd`, `golang.org/x/sys/windows$`);
  `other-packages` excludes the package; `selfupdate` and `service` allow
  `internal/winproc$`. `AGENTS.md`'s list names the rule.
* The start-site test's shim takes no arguments, so that a broken guard,
  as in the fail-first runs below, runs nothing but an empty script on the
  host.

**Windows tests, on the Windows test host** (cross-compiled test binaries,
run from the ssh shell):

```text
--- PASS: TestCommandPassesProgramAndArgs (0.00s)
--- PASS: TestDetachedPassesProgramAndArgs (0.00s)
--- PASS: TestNoConsoleWindowThreeDeep (0.08s)
    --- PASS: TestNoConsoleWindowThreeDeep/parent-command (0.04s)
    --- PASS: TestNoConsoleWindowThreeDeep/parent-detached (0.04s)
--- PASS: TestBatchFileRefused (0.00s)
PASS
```

The first run, before Deviation D2's fix, failed only the `shim` case;
D2 quotes it.

**Fail first (step 8),** each in a `scripts/plant-copy.sh` copy:

| Plant | Where it ran | The failing line |
| :--- | :--- | :--- |
| `Command` drops its arguments | this Mac | `winproc_test.go:19: Command: Path "<tmp>/prog", Args ["<tmp>/prog"]; want … ["<tmp>/prog" "a" "b c"]` |
| `Detached` drops its arguments | this Mac | `winproc_test.go:32: Detached: … want … "a" "b c"]` |
| Unix `attachedAttr` returns a `SysProcAttr` | this Mac | `winproc_unix_test.go:18: Command set SysProcAttr &{…}, want none` |
| Unix `detachedAttr` with `Setsid: false` | this Mac | `winproc_unix_test.go:30: Detached SysProcAttr &{… Setsid:false …}, want Setsid` |
| a direct `exec.Command` in `service/runner.go` | this Mac | `spawn_test.go:65: service/runner.go:77:6: service/runner.go: os/exec.Command: start the process through internal/winproc (0021-MADR D7)` |
| `golang.org/x/mod/semver` imported into `internal/winproc` | this Mac, `GOOS=linux` lint | `internal/winproc/winproc.go:17:2: import 'golang.org/x/mod/semver' is not allowed from list 'internal-winproc' (depguard)` |
| `attachedAttr` without `CREATE_NO_WINDOW` | the Windows test host | `child has a console window: procs=1 hwnd=0x9b00ba visible=false`, and the grandchild shares it |
| `detachedAttr` with `DETACHED_PROCESS` | the Windows test host | `child has no console: procs=0 hwnd=0x0 visible=false`; `grandchild has a console window: procs=1 hwnd=0x9c00ba visible=false` |
| the `.bat`/`.cmd` case removed | the Windows test host | all eight refusal cases in `TestBatchFileRefused`; `runExec: <nil>, want ErrBatchFile` and `DetachProcess: <nil>, want ErrBatchFile`; `Probe: selfupdate: staged probe of x printed "", want "v1.0.0", want ErrBatchFile` |
| `exec.LookPath`'s result ignored | the Windows test host | `Command(shim): refused false (<nil>), want true`, and the same for `Detached` |

Notes on two of them:

* **The static ban's plant is in `service/runner.go`, not `probe.go`** as
  step 8 says. `plant-copy.sh` makes one replacement, and `runner.go`
  already imports `os/exec`; `probe.go` no longer does. The walk covers
  both files.
* **The depguard plant was made twice.** A blank import was reported by
  `revive` (`blank-imports`) instead, because golangci-lint keeps one
  finding per line. A used import (`var _ = semver.IsValid`) was then
  reported by depguard, as quoted.

**The MADR's derived window claim, first measured.** With today's flags,
the console-less parent's child got a console window (`hwnd` non-zero),
and so did the grandchild of the detached run. From the ssh session the
window reports `visible=false`, because that session has no interactive
desktop; magic-cli-remote's 0159 probe 8 measured `visible=true` in the
console session. The test therefore asserts that no window exists, which
holds in both.

**Behaviour that must not change (step 9),** on the Windows test host:

* `selfupdate-service.test.exe`, the whole suite: `PASS`;
  `TestDetachProcessLeavesJob` `--- PASS` (both subtests).
* `selfupdate-service-scm.test.exe`, the whole suite: `PASS`; and with
  `SELFUPDATE_REQUIRE_SCM=1` (the ssh shell is an administrator) all eight
  `TestLive*` `--- PASS`.
* `internal-winproc.test.exe`: `PASS`.
* `selfupdate.test.exe`, run away from its source tree, fails only tests
  that open `testdata\…` or `scripts\selfupdate_manifest.py` (`TestGolden`,
  `TestManifest*`, `TestParseSHA256SUMSExported`,
  `TestStreamDeliversRunInOrder`, `FuzzParseSHA256SUMS`), each with
  `The system cannot find the path specified` or `No such file`. The
  prober tests, run by name (`-test.run Prob`), all 14 `--- PASS`. CI's
  Windows job runs the whole suite from source.

**Checks.** `make pre-add-check FILES="<the 14 Go files>"`: `go-precheck: 14
file(s) clean (gofmt, golangci-lint, go vet, go test, govulncheck)`. Lint on
`GOOS=linux`, `darwin` and `windows`: `0 issues` each. `make gate`:
`overall=0`, `apicheck` `compatible with v1.13.0`. Links and identifiers on
the MADR and this PLAN: clean.

**Host cleanup.** The test directory was removed; no `selfupdate-t2-` or
`selfupdate-probe-` directory and no `.test.exe` process remains. The
host's temporary directory holds 510 older `selfupdate-*` entries, none
from today; they are not this PLAN's, and were left alone.

### T3 (2026-10-10)

**Files.** New: `internal/winproc/tree.go`, `tree_test.go`. Changed:
`selfupdate/service/scm/api.go`, `selfupdate/service/scm/detach.go`,
`.golangci.yml`.

* `winproc.Process` and `winproc.Descends(self, target, procs, created)
  (bool, error)` carry `scm.insideService`'s loop line for line:
  `maxAncestors` (64), the end at a missing parent or a self-parented
  entry, the end at a parent created after its child, and an unreadable
  time counted as a genuine link. `winproc.ErrNotInSnapshot` replaces the
  inline error; `scm` wraps it as `selfupdate: scm: %w`, so its message is
  the same text as before.
* `scm`'s `process` is now `type process = winproc.Process`, an alias, so
  `fake_test.go` and `scm_test.go` compile and run unchanged. Step 2 said
  the type "becomes `winproc.Process`"; the alias is how, without touching
  the tests that are the evidence.
* depguard: `service-scm` allows `internal/winproc$`. `fake_test.go`,
  `scm_test.go` and every other `scm` test file are unchanged in this
  commit (`git diff --stat`: only `api.go` and `detach.go` under `scm/`).

**Fail first,** each in a `scripts/plant-copy.sh` copy of `tree.go`, on
this Mac:

| Plant | The failing lines |
| :--- | :--- |
| `cur.PID == target+1` | `self`, `parent`, `grandparent`, `older parent`, `unreadable time`, `within the bound`: `Descends false, <nil>; want true` |
| a missing or self-parented parent returns true | `unrelated`, `own parent ends the walk`, `parent gone`: `Descends true, <nil>; want false` |
| the reused-PID check disabled | `reused pid: Descends true, <nil>; want false` |
| an unreadable time counted as reused | `parent`, `grandparent`, `unreadable time`, `within the bound`: `Descends false, <nil>; want true` |
| `maxAncestors * 2` steps | `beyond the bound: Descends true, <nil>; want false` |
| no error for a missing self | `tree_test.go:53: self missing: <nil>, want ErrNotInSnapshot` |

**Behaviour unchanged.** On this Mac, `go test ./selfupdate/service/scm`
`ok`. On the Windows test host: `scm.test.exe` `PASS`; with
`SELFUPDATE_REQUIRE_SCM=1`, all eight `TestLive*` `--- PASS`, among them
`TestLiveHandOff` and `TestLiveHandOffHealthFailureReportsRollback`, which
call `Inside` from inside a real service; `winproc.test.exe` `PASS`. The
host directory was removed.

**Checks.** `make pre-add-check FILES="<the 4 Go files>"`: `go-precheck: 4
file(s) clean`. `make gate`: `overall=0`, `apicheck` `compatible with
v1.13.0`.

### T4 (2026-10-10)

**Files.** New, in `selfupdate/service/taskscheduler/`: `task.go`,
`api.go`, `scheduler.go`, `scheduler_other.go`, `lifecycle.go`,
`reconcile.go`, `detach.go`; tests `fake_test.go`, `task_test.go`,
`lifecycle_test.go`, `reconcile_test.go`, `detach_test.go`,
`json_test.go`. Changed: `.golangci.yml`, `AGENTS.md`.

**Exported API:** `New`, `Options` (`Name`, `Folder`, `Probe`, `Poll`,
`RewritePath`, `RequestStop`, `StopGrace`), `Task` and its eight methods,
`DefaultStopGrace` (10 s), and `DefinitionBackup` (`folder`, `name`, `xml`).
The full apidiff report against `v1.13.0`, run once without
`-incompatible` as `scripts/check-api-compat.sh` builds it:

```text
Compatible changes:
- package github.com/maccavelli/go-selfupdate-lib/selfupdate/service/taskscheduler: added
```

**As built, against step 2's planned shape:**

* `scheduler` has `open`, `processes`, `created`, and `running(pid,
  born)`, which `Stop`'s waits need to tell an exited process from a
  reused ID. `images` and `tokenSID` are not there: T1 showed neither is
  needed (Amendment A1).
* `registered` has every method step 2 names. `actions` returns every
  action, with its type, so `Reconcile` can name what it refuses; an
  action's working directory is not read, since nothing uses it.
* `Options.RequestStop` receives `Stop`'s own context, bounded by
  `Poll.Timeout`, not the caller's.
* `Reconcile` compares an Exec `Path` without its surrounding quotes, and
  writes a rewrite in the same form. magic-cli-remote writes an unquoted
  absolute path (`internal/cli/service/schtasks.go:156` there).
* `Folder` must be `\` or a path such as `\Vendor\Product`: no `/`, no
  empty element, no trailing separator.
* The method that walks the process tree is `insideTask`, not `inside`:
  revive's `confusing-naming` refuses a method that differs from `Inside`
  only by case. `scm`'s is `insideService`.
* **Interim, until T5:** `scheduler_other.go` has no build tag, so
  `systemScheduler` returns nil on every OS, and `newTask` refuses a nil
  transport with `service.ErrUnsupported`. The plan did not say what
  Windows builds at T4; this keeps T4's commit building and linting for
  Windows. T5 adds `scheduler_windows.go` and the `!windows` tag.
* depguard: `service-taskscheduler` (`$gostd`, `golang.org/x/sys/windows$`,
  `selfupdate$`, `selfupdate/service$`, `internal/winproc$`);
  `other-packages` excludes the package. `AGENTS.md` names the package in
  its scope paragraph and the rule in its list.

**Tests.** 29 tests over the fake, covering every item in step 10, and Q6's
refusal of an `S4U` or stored-password task. On this Mac `ok`; on the
Windows test host the cross-compiled suite `PASS`.

**Fail first (steps 10 and 11).** A script made 28 planted copies with
`scripts/plant-copy.sh`, ran the package's tests in each, and checked that
every test failed under at least one plant: `tests: 29 never failed:
none`. Seven plants first failed to compile (`declared and not used`),
which proves nothing; they were rewritten to keep the variable in use, and
the whole set was run again after the `insideTask` rename. The second
run's results:

| Plant | Tests that failed | First failing line |
| :--- | :--- | :--- |
| `off-windows` | `TestNewOffWindows` | `task_test.go:15: linux: <nil>, want ErrUnsupported` |
| `folder-trailing` | `TestOptionsValidated` | `task_test.go:45: folder with trailing separator: <nil>, want ok false` |
| `path-join` | `TestTaskPath` | `task_test.go:74: {Name:demo Folder:\Vendor Probe:<nil> Poll:{Interval:0s Timeout:0s Settle:0s Previous:} Re...` |
| `map-denied` | `TestHRESULTMap`, `TestPermission` | `lifecycle_test.go:51: Installed: selfupdate: taskscheduler: open \demo: HRESULT 0x80070005` |
| `map-missing` | `TestHRESULTMap`, `TestNotInstalled` | `lifecycle_test.go:20: Installed: false, selfupdate: taskscheduler: open \demo: HRESULT 0x80070002` |
| `running-instances` | `TestRunningByState` | `lifecycle_test.go:74: ready, 1 instances: false, <nil>; want true` |
| `enabled-repeats` | `TestEnabledRule` | `lifecycle_test.go:103: one-off time: true, <nil>; want false` |
| `no-disable` | `TestStopGraceRunsOut`, `TestStopNotRunning`, `TestStopOrder`, `TestStopRequestHonoured` | `lifecycle_test.go:128: Stop calls ["request 4242" "stop"], want ["enable false" "request 4242" "stop"]` |
| `request-ignored` | `TestStopRequestHonoured` | `lifecycle_test.go:160: calls ["enable false" "stop"], want ["enable false"]: the program exited on its own` |
| `no-grace` | `TestStopGraceRunsOut`, `TestStopRequestHonoured` | `lifecycle_test.go:160: calls ["enable false" "stop"], want ["enable false"]: the program exited on its own` |
| `request-error-waits` | `TestStopRequestFails` | `lifecycle_test.go:202: waited the grace after a failed request: 1h0m0s` |
| `request-error-dropped` | `TestStopRequestFails` | `lifecycle_test.go:210: Stop: selfupdate: taskscheduler: stop \demo: HRESULT 0x80004005, want both errors` |
| `start-always-enables` | `TestStartRunning`, `TestStopWithoutRequest` | `lifecycle_test.go:230: Start: <nil>, calls ["stop" "enable true" "run"], want ["stop" "run"]` |
| `stop-wait-cancels` | `TestStopFinishesAfterCancel` | `lifecycle_test.go:266: Stop = selfupdate: service: timed out: \demo: context canceled` |
| `stop-no-timeout` | `TestStopTimesOut` | `lifecycle_test.go:281: Stop: <nil>, want ErrTimeout` |
| `stop-inside` | `TestStopInside` | `lifecycle_test.go:292: Stop: <nil>, want ErrInsideService` |
| `start-second-instance` | `TestStartRunning` | `lifecycle_test.go:305: Start: <nil>, calls ["run"]` |
| `probe-skipped` | `TestWaitHealthyNewInstance` | `lifecycle_test.go:327: WaitHealthy: <nil>, probed false` |
| `old-instance-counts` | `TestWaitHealthyOldInstance` | `lifecycle_test.go:340: WaitHealthy: <nil>, want ErrTimeout` |
| `failed-run-ignored` | `TestWaitHealthyFailedRun` | `lifecycle_test.go:361: WaitHealthy: selfupdate: service: timed out: not ready within 2s (last: \demo ran an...` |
| `old-run-fails` | `TestWaitHealthyBeforeRun` | `lifecycle_test.go:373: WaitHealthy: selfupdate: service: not healthy: \demo ran and exited with code 1, wan...` |
| `match-by-text` | `TestReconcileMatch` | `reconcile_test.go:21: c:\program files\demo\DEMO.EXE: {Changed:true Detail:\demo now runs C:\Program Files\...` |
| `receipt-empty` | `TestReconcileMoved` | `reconcile_test.go:48: receipt taskscheduler.DefinitionBackup{Folder:"\\", Name:"demo", XML:""}, want tasksc...` |
| `quote-dropped` | `TestReconcileQuoted` | `reconcile_test.go:65: calls ["setpath C:\\New Place\\demo.exe"], want ["setpath \"C:\\New Place\\demo.exe\""]` |
| `any-logon` | `TestReconcileRefused` | `reconcile_test.go:92: S4U: <nil>, calls ["setpath C:\\New\\demo.exe"]; want an error about "S4U" and no change` |
| `restore-empty` | `TestRestore` | `reconcile_test.go:110: definition "", calls ["restore"]` |
| `first-instance-only` | `TestInside` | `detach_test.go:33: second instance: inside false, <nil>; want true` |
| `json-key` | `TestDefinitionBackupJSON` | `json_test.go:15: {"folder":"\\Vendor","name":"demo","definition":"\u003cCommand\u003eC:\\café\\demo.exe\u00...` |

depguard, a used `golang.org/x/mod/semver` import planted into `api.go`:
`selfupdate/service/taskscheduler/api.go:6:2: import 'golang.org/x/mod/semver' is not allowed from list 'service-taskscheduler' (depguard)`.

**Checks.** Lint on `GOOS=linux`, `darwin` and `windows`: `0 issues` each.
`make pre-add-check FILES="<the 13 Go files>"`: `go-precheck: 13 file(s)
clean`. `make gate`: `overall=0`, `apicheck` `compatible with v1.13.0`.

### Records amendment A3 (2026-10-10): COM research for T5

Official Task Scheduler, COM and Automation pages were read and evaluated
in Amendment A3 of the MADR. T5 above was rewritten against those facts:
`ole32!CoInitializeEx` is called through a lazy procedure; `Stop` treats
`S_FALSE` as failure; rewrite/restore pass `TASK_UPDATE |
TASK_IGNORE_REGISTRATION_TRIGGERS`; collection indexes are 1-based;
`tokenSID` is struck. No Go files changed.

### Owner decision before T5 (2026-10-10)

Deviation D1 asked for a decision before T5. The owner chose to iterate
`TestCOMLayout` from the elevated ssh shell, then run the final test binary
once more as the action of a throwaway least-privilege task in the console
session, as T1's driver did, and record that run as the medium-integrity
evidence. The owner confirmed the console sign-in.

### T5 (2026-10-10)

**Files.** New: `selfupdate/service/taskscheduler/com_windows.go`,
`scheduler_windows.go`, `com_windows_test.go`. Changed:
`scheduler_other.go` (`//go:build !windows`), `api.go` (no new exported
API).

* Lazy `ole32` / `oleaut32`, vtable slots after `IDispatch`, `VARIANT` 24
  bytes on amd64, BSTRs, `CoInitializeEx` through a lazy procedure.
* `systemScheduler` implements the T4 interfaces. Rewrite/restore use
  `TASK_UPDATE | TASK_IGNORE_REGISTRATION_TRIGGERS`. `Stop` treats
  `S_FALSE` as failure.
* `TestCOMLayout` registers a throwaway `selfupdate-probe-<random>` task
  through `ITaskFolder::RegisterTask` (not `schtasks`), exercises every
  T4 method, rewrites the Exec path into a directory whose name holds
  `é`, `ü` and a CJK character, and restores.

**Windows test host** (elevated ssh, High Mandatory Level, as D1):
`TestVARIANTSize` `--- PASS`. `TestCOMLayout` `--- PASS`.
`CoInitializeEx HRESULT 0x00000000` (`S_OK`). Never-run
`DATE 1999-11-30 00:00:00` local, `LastTaskResult 0x00041303`.

**Fail first,** each in a `scripts/plant-copy.sh` copy, run on the
Windows test host:

| Plant | Test | Failing line |
| :--- | :--- | :--- |
| `get_State` slot 9→10 | `TestCOMLayout` | `get_State: 65535, <nil>` |
| `sizeof(variant)` want 16 | `TestVARIANTSize` | `sizeof(variant) = 24, want 24 on amd64/arm64` |
| `Item` index `i-1` (0-based) | `TestCOMLayout` | `HRESULT 0x80070057` (`E_INVALIDARG`) |

**Deviation D3 (2026-10-10).** *(First written as a second "Deviation
D2"; renumbered 2026-10-10, since D2 is T2's. Its resolution is under
"Deviation D3, resolution" below.)* The owner-chosen medium-integrity run
started `TestCOMLayout` as the action of a throwaway LeastPrivilege
InteractiveToken task. `ITaskFolder::RegisterTask` from that process
returned `0x80070005`. Nested create of a second per-user task is
refused while the test itself is already a running task. The elevated
ssh run covers create and every method. T6's live tests create their
tasks from the ssh shell, as T1 did for the driver, not from inside a
running task.

**Checks.** `make pre-add-check FILES="<the 5 Go files>"`: `go-precheck:
5 file(s) clean`. Lint on `GOOS=linux`, `darwin` and `windows`: `0
issues` each. `make gate`: `overall=0`, `apicheck` `compatible with
v1.13.0`.

**Host cleanup.** Throwaway `selfupdate-t5-*` directories removed; no
`selfupdate-probe-` task remains.

### T5's unrecorded plants (found 2026-10-10)

T5 step 5's plant table had six rows; T5's record shows three. The other
three:

* **`CoInitializeEx` treated as failed on `S_FALSE`:** not plantable on
  this host. Every run's init returned `S_OK` (`CoInitializeEx HRESULT
  0x00000000`), so the `S_FALSE` path is not taken.
* **`Stop` treats `S_FALSE` as success:** superseded by Deviation D4,
  which made `S_FALSE` with no instance left a success. T6a step 5
  measures what an idle `Stop` returns.
* **`put_Enabled(1)`:** planted on 2026-10-10 on the Windows test host.
  `TestCOMLayout` passed (`--- PASS: TestCOMLayout (0.21s)`): the test
  never reads `enabled` back after `setEnabled(true)`. Deviation D5.

### T6 (2026-10-10)

**Committed by the owner** as `026faaa`, before this record was written.
The agent wrote this record afterwards, from the diff and from a re-run on
the Windows test host.

**Files.** New: `selfupdate/service/taskscheduler/main_test.go`,
`live_windows_test.go`, `live_other_test.go`. Changed:
`.github/workflows/ci.yml`; and, outside T6's file list,
`selfupdate/service/taskscheduler/api.go`, `lifecycle.go` and
`scheduler_windows.go` (Deviation D4).

**As built, against the plan:**

* `main_test.go` takes the `tasksched-live` argument, with modes `serve`
  (the task's program), `agent` (a child of the program that runs the
  update from inside the task), `update` (the detached run) and `probe`
  (the version probe's child, which reports its console).
* `TestLiveTaskUpdate`: the program starts the agent when the test drops
  a trigger file. The agent asserts that `Stop` from inside returns
  `ErrInsideService`, then calls `service.HandOffIfInside`. The detached
  run does the managed install. It also checks that an environment value
  with quotes, `%PATH%` and a newline reaches it unchanged.
* `TestLiveTaskStopHoldsDisabled` is as planned, but its time trigger
  stopped repeating in 2020 (Deviation D5).
* `TestLiveTaskRollback` runs the managed install from the test process,
  not from inside the task. Its failing health is a copy of `whoami.exe`
  installed as the new build: it exits at once, and `WaitHealthy`'s
  failed-run rule reports it. It does not use a failing `Options.Probe`.
  The task already runs the install target, so nothing is rewritten and
  `Restore` is not called (Deviation D5, Q8).
* `TestLiveTaskRequestStop` asserts that the program exited on the
  request. It does not assert that `stop` was never called, as step 5
  says.
* Step 6's final check is `TestLiveTaskzCleanup`, placed last in the
  file. It runs last because `go test` runs tests in source order. Under
  `-shuffle` it can run first and prove nothing; it needs
  `SELFUPDATE_REQUIRE_TASKSCHED`, so `make gate`'s shuffle step skips it.
* CI's step is as planned: `go test -count=1 -v -run '^TestLiveTask'`,
  with no `SELFUPDATE_REQUIRE_TASKSCHED`. It has not run: the commits are
  not pushed.

**The Windows test host, re-run by the agent on 2026-10-10** on the
owner's word ("if you need to re-run it do so"). The console sign-in was
measured, not asked for as rule 6 says: `Win32_ComputerSystem.UserName`
was set, `explorer` ran in session 1, and no `LogonUI` process ran. The
test binary was cross-compiled at `026faaa` and run from the elevated
ssh shell with `SELFUPDATE_REQUIRE_TASKSCHED=1 ./ts.test.exe -test.run
'^TestLiveTask' -test.count=3 -test.v`, exit 0:

```text
--- PASS: TestLiveTaskUpdate (5.25s)
    live_windows_test.go:500: instance pids before Stop: [11932]
    live_windows_test.go:504: start pid 11932 running after Stop: false; pids []
--- PASS: TestLiveTaskStopHoldsDisabled (72.00s)
--- PASS: TestLiveTaskRollback (5.32s)
--- PASS: TestLiveTaskRequestStop (0.99s)
--- PASS: TestLiveTaskzCleanup (1.00s)
--- PASS: TestLiveTaskUpdate (5.32s)
    live_windows_test.go:500: instance pids before Stop: [38536]
    live_windows_test.go:504: start pid 38536 running after Stop: false; pids []
--- PASS: TestLiveTaskStopHoldsDisabled (71.86s)
--- PASS: TestLiveTaskRollback (5.15s)
--- PASS: TestLiveTaskRequestStop (1.12s)
--- PASS: TestLiveTaskzCleanup (0.63s)
--- PASS: TestLiveTaskUpdate (5.35s)
    live_windows_test.go:500: instance pids before Stop: [41180]
    live_windows_test.go:504: start pid 41180 running after Stop: false; pids []
--- PASS: TestLiveTaskStopHoldsDisabled (71.92s)
--- PASS: TestLiveTaskRollback (5.24s)
--- PASS: TestLiveTaskRequestStop (1.13s)
--- PASS: TestLiveTaskzCleanup (0.65s)
PASS
```

**Fail first (step 7),** each a `scripts/plant-copy.sh` copy, built on this
Mac and run on the Windows test host with `-test.count=1`. Each plant was
checked to have landed in its copy (old text 0 times, new text once):

| Plant | Test | Result |
| :--- | :--- | :--- |
| `Stop` without the disable (`t.disable(r, path)` replaced by `error(nil)`) | `TestLiveTaskStopHoldsDisabled` | **passed** (71.84s); no start while the task was enabled. Deviation D5 |
| `Restore` a no-op (`r.restore(b.XML)` replaced by `error(nil)`) | `TestLiveTaskRollback` | **passed** (5.15s). Deviation D5 |
| D7 reverted (`detachedAttr` with `DETACHED_PROCESS` in place of `CREATE_NO_WINDOW`) | `TestLiveTaskUpdate` | failed: `live_windows_test.go:463: <tmp>\TestLiveTaskUpdate…\001\.demo.exe.selfupdate.handoff.console has no console: procs=0 hwnd=0x0 visible=false` |

**Checks.** `make gate` on `026faaa`, with only this record's edits in
the tree: every step `rc=0`, `apicheck` `compatible with v1.13.0`,
`overall=0`. The owner's own pre-commit checks for `026faaa` are not
recorded.

**Host cleanup.** The run's directory was removed. Afterwards: 0
`selfupdate-` tasks, and no `.test.exe` process. One older
`selfupdate-t6-*` directory, from the owner's earlier T6 run, was left
alone.

**Not done in T6:** step 7's first two plants did not fail; step 5's
assertion on `stop` is missing; step 8's CI run waits for a push. T6a
holds the remedies.

### Deviation D3, resolution (2026-10-10)

* **Found.** T5's Deviation D3: the medium-integrity `TestCOMLayout` run,
  from inside a least-privilege task, was refused at registration
  (`0x80070005`). T5 step 4 is unmet, and no owner decision was recorded.
* **Recommended:** T6a step 4. The live update's detached run, inside a
  least-privilege `InteractiveToken` task, records and asserts its
  integrity level. Its `Stop`, `Reconcile`, `Start` and `WaitHealthy` are
  the backend's calls at medium integrity, which is what step 4 was for.
  It covers every backend method; it does not cover registration, which
  the backend never does (MADR D1). Cost: one helper, one assertion, one
  plant.
* **Doing nothing:** the backend's COM calls are shown only from an
  elevated process, and T5 step 4 stays open at T9's V7.

### Deviation D4 (2026-10-10): T6 changed the transport and `Stop`'s wait

* **Found** in `026faaa`'s diff and message. The live output that led to
  it was not kept.
  * `sysReg.instances` returns no PIDs, and no error, for
    `SCHED_E_TASK_NOT_RUNNING` (`0x8004130B`).
  * `sysReg.stop`, on `S_FALSE`, lists the instances. With none left, or
    `0x8004130B`, it returns nil; otherwise it returns an error. This
    contradicts MADR Amendment A3's normative rule 6 and T5's record:
    "`Stop` treats `S_FALSE` as failure".
  * `waitGone`, once `Stop` recorded a PID, waits for that process alone.
    The commit says `GetInstances` "can still list a PID after the
    program has exited". `waitGone` also maps `0x8004130B`, which cannot
    arrive, since the transport already maps it. Its branch for no
    recorded PID cannot run either, since `Stop` returns before
    `waitGone` when it lists no instance.
  * No test covers any of the three, and none was seen failing first.
* **Today's evidence:** three runs of `TestLiveTaskStopHoldsDisabled`
  logged `pids []` straight after `Stop` returned. The stale listing was
  not reproduced, and is marked [reported] in MADR Amendment A4.
* **Recommended:** keep the behaviour; record it as MADR Amendment A4;
  T6a steps 5 to 7 measure and test it. Cost: three fake cases, two
  `TestCOMLayout` checks, and a smaller `waitGone`.
* **Doing nothing:** the MADR's normative rule and the code disagree, and
  three behaviours ship in `v1.14.0` with no test.

### Deviation D5 (2026-10-10): three checks did not fail on their plants

* **Found** in the re-run above, and in T5's `put_Enabled(1)` plant:
  * `TestLiveTaskStopHoldsDisabled` passed with no disable. `liveXML`'s
    `TimeTrigger` has `StartBoundary` `2020-01-01T00:00:00` and
    `Duration` `P1D`, so it repeated for one day in 2020 and never fires
    now. The test cannot see a relaunch.
  * `TestLiveTaskRollback` passed with `Restore` a no-op. The task's
    `Command` is the install target, so `Reconcile` reports "already
    runs" and the rollback has nothing to restore.
  * `TestCOMLayout` passed with `put_Enabled(1)`: `toggleEnabled` does
    not read `enabled` back after enabling.

  All three are in new code (T5, T6), not pre-existing.
* **Recommended:** T6a steps 1, 2 and 5, with the plants of step 8. Cost:
  test changes only; the host run takes about 10 minutes.
* **Q8, for the owner: where the rollback runs.** T6 step 4 said "as
  `TestLiveTaskUpdate`", from inside the task, through the handoff.
  * Recommended: keep it from the test process (T6a step 2 fixes what it
    proves). The handoff's rollback path does not depend on the backend,
    and `scm`'s `TestLiveHandOffHealthFailureReportsRollback` runs it
    from inside a real service. Cost: none beyond step 2. **The owner
    chose this on 2026-10-10.**
  * Alternative: run it through the agent, as `TestLiveTaskUpdate` does.
    Cost: one more agent mode and a longer test; it adds a
    backend-specific check of the handoff's result on a rollback.
* **Doing nothing:** `Stop`'s disable and `Restore` ship with no live
  evidence, and V8 cannot be met.

### Owner decisions on T6a (2026-10-10)

Q7: the recommendation (MADR Amendment A4). Q8: keep the rollback test in
the test process (Deviation D5). T6a approved. The owner committed the
records above as `eb6c47e`, and commits T6a.

### T6a (2026-10-10)

**Files.** Changed, in `selfupdate/service/taskscheduler/`:
`lifecycle.go`, `lifecycle_test.go`, `fake_test.go`,
`com_windows_test.go`, `live_windows_test.go`. No exported API changed.

**As built:**

* `waitGone(ctx, path, inst, limit)` polls `running(pid, born)` alone; the
  `registered` argument, the instance listing and both unreachable
  branches are gone. `Stop`'s step 5 and `waitGone`'s comments say what
  it waits for.
* `Start` filters the listing through `current(pids, prev)`, which drops
  the instance `Stop` recorded once its process has exited (Q7).
  `hrTaskNotRunning` stays in `api.go`; only the transport uses it.
* The fake's `staleList` keeps an ended instance listed.
  `TestStopStaleInstance` and `TestStartStaleInstance` (with a second case:
  a recorded process that still runs is counted).
* `TestCOMLayout` logs the raw `GetInstances` HRESULT of the never-run
  task and the raw `Stop` HRESULT of the idle task, through
  `rawTaskCall`, and asserts `instances()` empty and `stop()` nil there.
  `toggleEnabled` reads `enabled` back after enabling.
* `liveXML`'s time trigger starts at registration and has no `Duration`.
  `newLiveTaskIn(t, "old")` puts the task's `Command` at
  `<dir>\old\demo.exe` for `TestLiveTaskRollback`.
* `TestLiveTaskRequestStop` wraps the scheduler in `stopCounter`, which
  counts `registered.stop`, and asserts zero.
* The detached update writes its integrity RID (`TokenIntegrityLevel`,
  the label's last subauthority) beside the handoff result;
  `TestLiveTaskUpdate` asserts `0x2000`.

**The Windows test host.** Console sign-in measured, as for T6:
`console_user_present=True`, `explorer_sessions=1`, `logonui=False`.
Binaries cross-compiled on this Mac from the T6a tree, run from the
elevated ssh shell. `TestCOMLayout` and the four live tests,
`-test.count=3`, exit 0. The first of the three passes, verbatim; the
other two match, with their own PIDs and times:

```text
    com_windows_test.go:30: CoInitializeEx HRESULT 0x00000000
    com_windows_test.go:35: never-run DATE 1999-11-30 00:00:00 -0600 CST LastTaskResult 0x00041303
    com_windows_test.go:38: never-run GetInstances HRESULT 0x00000000
    com_windows_test.go:49: idle Stop HRESULT 0x00000001
--- PASS: TestCOMLayout (0.21s)
    live_windows_test.go:521: the update's integrity RID 0x2000
--- PASS: TestLiveTaskUpdate (4.97s)
    live_windows_test.go:557: instance pids before Stop: [26960]
    live_windows_test.go:561: start pid 26960 running after Stop: false; pids []
--- PASS: TestLiveTaskStopHoldsDisabled (71.89s)
--- PASS: TestLiveTaskRollback (5.44s)
--- PASS: TestLiveTaskRequestStop (1.12s)
--- PASS: TestLiveTaskzCleanup (0.98s)
```

The later passes: `TestCOMLayout` 0.34s and 0.34s; `TestLiveTaskUpdate`
5.36s and 5.47s; `TestLiveTaskStopHoldsDisabled` 71.89s and 72.00s;
`TestLiveTaskRollback` 5.50s and 5.31s; `TestLiveTaskRequestStop` 1.11s
and 1.11s; `TestLiveTaskzCleanup` 0.88s and 0.99s. Every pass logged the
same four HRESULT lines and RID `0x2000`.

**Fail first (step 8),** each a `scripts/plant-copy.sh` copy, checked to
have landed:

| Plant | Where | Result |
| :--- | :--- | :--- |
| `Stop` without the disable | the Windows test host | failed: `live_windows_test.go:564: a start ran while disabled: [7504 41020]` |
| `Restore` a no-op | the Windows test host | failed: `live_windows_test.go:638: xml after rollback differs: len 1303 vs 1307, <nil>` |
| `Stop` calls `stop` after the program exited in the grace period | the Windows test host | failed: `live_windows_test.go:658: IRegisteredTask::Stop called 1 times after the program exited on RequestStop` |
| the task's `RunLevel` `HighestAvailable` | the Windows test host | failed: `live_windows_test.go:519: the update ran at integrity 0x3000, want 0x2000 (medium)` |
| `instances` without the `0x8004130B` mapping | the Windows test host | **passed**: not reachable. The never-run task's `GetInstances` returns `S_OK`, so no test reaches the mapping (MADR Amendment A4) |
| `stop` with `S_FALSE` an error again | the Windows test host | failed: `com_windows_test.go:51: stop of an idle task: plant: S_FALSE is an error` |
| `put_Enabled(1)` | the Windows test host | **passed**: not distinguishing. The read-back sees `true`; Windows reads any non-zero `VARIANT_BOOL` as true (MADR Amendment A4) |
| `Stop` requires an empty listing after the wait | this Mac | failed: `lifecycle_test.go:317: Stop: selfupdate: service: timed out: \demo still runs after it was stopped, want nil once the process has exited` |
| `current` keeps the stale instance | this Mac | failed: `lifecycle_test.go:334: Start after a stale listing: <nil>, calls ["enable false" "stop" "enable true"], want [… "run"]` |
| `current` drops a recorded instance that still runs | this Mac | failed: `lifecycle_test.go:346: Start with the recorded process alive: <nil>, calls [… "enable true" "run"], want [… "enable true"]` |

**What T6a settles:**

* Deviation D3: the backend's calls ran at medium integrity (`0x2000`),
  from inside a least-privilege task. T5 step 4 is met this way, not by a
  registration from inside a task.
* Deviation D4: the idle `Stop` returns `S_FALSE`, so `026faaa`'s change
  is needed, and is now tested. The stale listing was not seen in 9
  `Stop`s across T6 and T6a; `Start`'s filter is tested over the fake
  only.
* Deviation D5: the disable and `Restore` plants now fail.

**Not settled:** where `0x8004130B` arises, so its mapping is untested
(MADR Amendment A4). CI's step has still not run, and waits for a push.

**Checks.** `make pre-add-check FILES="<the 5 Go files>"`: `go-precheck: 5
file(s) clean (gofmt, golangci-lint, go vet, go test, govulncheck)`. `make
gate`: every step `rc=0`, `apicheck` `compatible with v1.13.0`,
`overall=0`.

**Host cleanup.** The run's directory was removed. Afterwards: 0
directories, 0 `selfupdate-` tasks, 0 `.test.exe` processes.

### CI for T6 and T6a (2026-10-10)

The owner committed T6a as `ac542d1` and pushed. CI run 38104218144 on
`main` at `ac542d1`: `completed success`, 17 jobs, each `success`.

* **T6 step 8, the "Task Scheduler live test" step** on
  `validate (windows-2025)` ran all five `TestLiveTask*` and each
  skipped, with the reason `live_windows_test.go:481: set
  SELFUPDATE_REQUIRE_TASKSCHED=1 to run against this host's Task
  Scheduler; GitHub runners have no interactive console session`. CI has
  not shown that the runner can run an interactive-token task, so the
  step keeps `SELFUPDATE_REQUIRE_TASKSCHED` unset, as step 8 says.
* **`TestCOMLayout` in CI** runs in the job's `go test ./...`, without
  `-v`. The package line is `ok …/selfupdate/service/taskscheduler
  0.448s`, which a pass and a skip both print, so the log does not say
  which it was. V7 records CI's result as "passed or skipped; not
  distinguishable from the log". The Windows test host is V7's evidence.

### T7 (2026-10-10)

The owner said "ci is green" after CI run 38104218144; T7 is the approved
PLAN's next phase.

**Files.** `docs/guides/extending-selfupdate.md`,
`docs/guides/migrating-from-mcplib-selfupdate.md`, `docs/architecture.md`,
`README.md`.

* **`extending-selfupdate.md`, "Run as a service":**
  * the opening names Task Scheduler since `v1.14.0` and cites
    0021-MADR; "Pick the backend" gains `taskscheduler.New`;
  * a bullet: no library process start runs a `.bat` or `.cmd` file, and
    why;
  * `### Task Scheduler`: `New` and its options; what the setup
    registers (one `Exec` action, a logon trigger, a repeating time
    trigger with `IgnoreNew` as the watchdog, `InteractiveToken` for
    `RewritePath`); that `RestartOnFailure` is no watchdog (0159 probe
    8); `Stop` and `RequestStop`, and that ending the task leaves the
    program's children running (Amendment A1, probe 2); `Start`; the
    recovery from a crash between them; `WaitHealthy`; the handoff; and
    `-H windowsgui` (0159 probe 7);
  * the handoff's per-platform list: the Windows line is "Windows (SCM)"
    and says the run has a console with no window; a Task Scheduler line
    names the backend; the line for a manager with no backend no longer
    names Task Scheduler;
  * the pointers name `taskscheduler/live_windows_test.go`.
* **Migration guide:** `## 14. From v1.13 to v1.14`, in §13's form.
  The header's "current release" lines and their pointer list move in
  T8's pin commit, as step 2 says.
* **`docs/architecture.md`:**
  * the tree gains `taskscheduler/` and `internal/winproc/`;
  * the package table gains both rows, counted with `ls <dir>/*.go`, split
    on `_test.go`: `taskscheduler` 9 and 10, `winproc` 5 and 4. Their
    imports were read with `go list` for the host and `GOOS=windows`;
  * three rows this PLAN changed are corrected: `selfupdate` tests 81 to
    83 and `service` tests 13 to 14 (T2's new test files), and
    `internal/winproc` added to the imports of `selfupdate`, `service`
    and `scm`;
  * the backend bullets gain `taskscheduler`, and the CI sentence says its
    live tests skip on a GitHub runner;
  * a bullet for `internal/winproc` and `TestNoDirectProcessStart`;
  * the depguard paragraph: 17 rules, counted in `.golangci.yml`, with
    `internal-winproc` and `service-taskscheduler`.
* **`README.md`:** the backends' row names `taskscheduler`, "a per-user
  Task Scheduler task".

**Found, and not changed (outside T7):** four rows of the package table
were already wrong at `31cce88`, before this PLAN: `systemd` says 8 and
12 (the tree has 9 and 17), `launchd` 6 and 11 (7 and 13), `scm`'s test
count 9 (12), and `internal/cmd/selfupdate-release` 11 and 10 (12 and
11). They wait for the owner's word.

**Checks.** On the four files: `scripts/check-docs.sh --links` `94 links
in 4 files, 0 broken`; `--ids` with this PLAN `5 files, 16 deny-list
rules, 0 findings`; markdownlint `0 issues`. `make gate`: every step
`rc=0`, `links` `443 links in 60 files, 0 broken`, `apicheck`
`compatible with v1.13.0`, `overall=0`.
