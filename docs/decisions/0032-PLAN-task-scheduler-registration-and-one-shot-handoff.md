---
status: proposed
date: 2026-10-23
associated-madr: "0032-MADR-task-scheduler-registration-and-one-shot-handoff.md"
---
# Implement Task Scheduler registration, comparison, state and the one-shot handoff, and release `v1.19.0`

Associated MADR: [0032-MADR-task-scheduler-registration-and-one-shot-handoff.md](0032-MADR-task-scheduler-registration-and-one-shot-handoff.md)
(D1-A to D8-A, as the owner chose on 2026-10-23).

## Goal

* `taskscheduler.Task` registers, reads and deletes a definition the
  program renders (`Register`, `Definition`, `Delete`), compares a stored
  definition with a wanted one (`DiffDefinitions`, `SameDefinition`), and
  reports its state in one call (`Status`). Issue #3.
* `Task.Detach` hands an update off as a one-shot task of its own, whose
  run survives the service's stop and the caller's kill-on-close jobs;
  `Options.HopDetach` keeps the hop. `Task.CleanupHandOffs` removes
  finished one-shot tasks. Issue #6.
* Every Windows fact the code relies on is measured first (J1), every new
  test is seen failing first, and the work ships as `v1.19.0`.

## Scope

### In scope

| Phase | What | Commit |
| :--- | :--- | :--- |
| J0 | this pair, its index rows | records only |
| J1 | probes P1 and P2 on the Windows test host; the MADR amended with the results; fixtures captured | records, and `testdata` fixtures |
| J2 | the COM transport's new calls, pinned by `TestCOMLayout` | code and tests |
| J3 | `Register`, `Definition`, `Delete` | code and tests |
| J4 | `DiffDefinitions`, `SameDefinition` | code, tests, fixtures |
| J5 | `Status` | code and tests |
| J6 | `service`: the handoff argument; `winproc.CheckProgram` | code and tests |
| J7 | the one-shot `Detach`, `CleanupHandOffs`, `Options.HopDetach` and `HandOffDir` | code and tests |
| J8 | the documents | docs |
| J9 | `v1.19.0` | per the architecture page's "Releasing this module" |
| J10 | closing | records only |

### Out of scope

* `scm`'s `Detach` (MADR, Out of scope).
* Rendering the program's definition.
* Any change to `Stop`, `Start`, `WaitHealthy`, `Reconcile` or `Restore`.

## Rules

* **`main`.** Each phase commits on `main` with `git commit --no-edit`,
  only on the owner's "commit to main" in that turn. A push or a tag
  needs its own word.
* **Each phase stops at a deviation**, recorded here as a dated entry,
  and in the MADR when it changes a decision or a fact.
* **Decision points stop the plan.** J1 names results that change a
  decision; on any of them the plan stops, the MADR is amended and
  re-approved, and only then does J2 start.
* **Checks before each commit:**
  * `make pre-add-check FILES="…"` on the changed Go files;
  * `CGO_ENABLED=0 GOOS=windows go vet ./selfupdate/... ./internal/...`
    for Windows-only files, which the host's own `go vet` does not build;
  * `scripts/check-docs.sh --links` and `--ids`, and
    `npx --yes markdownlint-cli2@0.23.2`, on changed Markdown;
  * `scripts/check-docs.sh --arch docs/architecture.md` when a `.go` file
    is added or removed, with the table updated in the same commit;
  * `make apicheck`, compatible with `v1.18.0`.

  `make gate` ends `overall=0` before each push.
* **Seen failing first:** every new test fails on a
  `scripts/plant-copy.sh` copy with one planted break, never a dirtied
  tree. A Windows-only test's plant runs on the Windows test host. The
  execution record quotes each failure.
* **The Windows test host.** Its ssh shell is elevated (0021-PLAN
  Deviation D1). Probes that measure medium integrity run inside a
  driver task at least privilege in the console session; the owner
  confirms the console sign-in before J1. Unit and live tests run from
  the ssh shell, at high integrity, as CI's runner does (0021-MADR
  Amendment A5); each phase says when that matters. The tree goes to the
  host as a bundle with no AppleDouble files, and the host builds with
  the `toolchain` line's Go (1.27.3).
* **Identifiers.** Probe output and captured XML carry SIDs, account
  names and the host name. They are replaced by placeholders (`<sid>`,
  `<user>`, `<host>`) before anything is written into the tree, and the
  identifier check runs on every fixture.

## Implementation Steps

### Phase J0: records

1. `docs/README.md` gains this pair's rows after 0031's.
2. Checks; commit on the owner's word.
3. On approval, the MADR records the owner's choices and becomes
   `accepted`, and this plan `in-progress` with an `### Approval` entry,
   in J1's commit.

### Phase J1: probes (MADR Confirmation, P1 and P2)

**Built outside the tree**, in the scratchpad and on the host, as
0021-PLAN T1 did:

* `tsprobe.exe`, a Go program built with `CGO_ENABLED=0 GOOS=windows
  GOARCH=amd64` from the scratchpad, `go vet` clean. It reaches COM with
  the same `x/sys/windows` vtable calls `com_windows.go` makes (it may
  copy that file). Modes:
  * `register`, `create`, `delete`, `mkfolder`, `xml`: P1 steps 1 to 6,
    each printing the HRESULT in hex;
  * `capture`: P1 step 7, registering each definition in a list,
    printing its stored XML, and deleting it;
  * `service`: the probe task's program for P2. It starts `tsprobe child`
    and assigns it to a job created with
    `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` only (F22's flags), then waits;
  * `child`: in the job, it runs P2's step named on its command line and
    reports;
  * `oneshot`, `hop`, `run`: the one-shot task's program, the hop and the
    real run. Each report carries its PID, parent PID, session, integrity
    label, `IsProcessInJob` against the job handle it was given (P2 step
    2) and against its own job, called through a lazy `kernel32`
    procedure, since `x/sys/windows` v0.47.0 has none, the job's limit
    flags, its console
    process count, console window and its visibility, `os.Args` as JSON,
    and a heartbeat file the driver watches.
* `driver.ps1` and `start-driver.ps1`, after 0021-PLAN Deviation D1:
  the ssh shell registers `selfupdate-probe-driver-<random>`
  (`InteractiveToken`, `LeastPrivilege`, no trigger), runs it, waits,
  deletes it and reads its log. The driver records its integrity label
  and session first.

**Steps:**

1. Owner confirms the console sign-in on the Windows test host.
2. **P1**, in the driver, each step on a new task `selfupdate-probe-<random>-p1-<n>`:
   1. `register`: `RegisterTask(TASK_CREATE_OR_UPDATE, no user,
      InteractiveToken)` in `\`; read back `get_Xml`; record whether
      `Principal/UserId` is the driver's SID (compared in the driver,
      printed as `match`/`differ`).
   2. The same with `<LogonType>S4U</LogonType>` and logon type 2.
   3. `mkfolder`: `CreateFolder("\selfupdate-probe-<random>")`; then
      register a task in it; then delete both.
   4. `create`: `TASK_CREATE` on an existing task; `DeleteTask` of a
      missing task; `DeleteTask` in a missing folder.
   5. `delete` of a task whose instance runs `tsprobe run` with a child:
      whether each survives 5 s, by PID and creation time.
   6. `xml`: register the step-1 definition declared `UTF-16`, `UTF-8`
      and with no declaration.
   7. `capture`: the stored XML of (a) F19's definition, rendered with
      the consumer's settings, (b) a definition per trigger type a user
      may register (logon for self, time, daily, weekly, registration,
      idle, session-state for self, event), each with every element
      D3-A compares set to a non-default value, (c) the same with every
      element absent, (d) a `ComHandler` action.
   8. `WriteHandOffEnv` into `%LOCALAPPDATA%\selfupdate\handoff`, then
      `icacls` on the directory and the file.
3. **P2**, in the driver: register a probe task (`InteractiveToken`,
   `LeastPrivilege`, a logon trigger, `IgnoreNew`) running `tsprobe
   service`; run it; its child runs each step:
   1. **Hop (today):** `service.DetachProcess` with `EnvHandOffHop`, as
      `Task.Detach` does today (F4). Then the service process closes the
      job handle. Record whether the hop's real run survives.
   2. **One-shot:** register `selfupdate-probe-<random>.selfupdate.<id>`
      from the child with D8-A's definition rendered by hand, and run it.
      Record the one-shot process's parent image, and `IsProcessInJob`
      against the child's job handle.
   3. **Survival:** with the one-shot running the hop (D7-A): after the
      hop exits, after the one-shot instance ends, after the service
      closes the job, after `Stop` of the probe task, and after
      `DeleteTask` of the one-shot task, record whether the real run is
      alive.
   4. **Window:** the real run's console window (none expected); the
      hop's window, and for how long it lived.
   5. **Arguments:** `--selfupdate-handoff-env=C:\p a\b.env`, `a b`,
      `"q"`, `back\slash\`, `ü 任务`, composed with
      `windows.ComposeCommandLine`, compared with the run's `os.Args`.
   6. **Principal and settings:** read back the one-shot task's
      `UserId` (compared with the probe task's), `LogonType`,
      `RunLevel`, and the settings D8-A writes.
   7. **Start signal:** time from `Run` to a listed instance, and to
      `LastRunTime` at or after the call; the instance's `LastTaskResult`
      after it ends.
4. **Repeat** P1 and P2 until two complete runs agree on every value, as
   0021-PLAN T1 did, and keep the last run's output.
5. **Decision points.** The plan stops for the owner, with evidence and
   resolutions (the rules above), if:
   * P2 step 1's run survives: #6's failure is not reproduced, and D5's
     premise needs re-reading;
   * P2 step 2's process is in the child's job, or step 3's run dies at
     any point: the one-shot task does not escape, or cleanup kills it
     (D5 to D8);
   * P2 step 4's real run has a visible window (D7);
   * P2 step 5 changes any argument (D6-A, D8-A);
   * P1 step 1 or P2 step 6's principal is not the caller's own (D8-A);
   * P1 step 8's ACL grants access to anyone but the user, SYSTEM and
     Administrators (D8-A's file).

   P1 steps 2, 3, 4 and 6 set facts, not decisions: their results go in
   the amendment, and S4U is accepted by `Register` only if step 2
   succeeds; otherwise `Register` refuses it with the other logon types
   (D2-A).
6. **The amendment.** The MADR gains `### Amendment A1 (<date>): probe
   results`, a table in 0021-MADR Amendment A1's form: probe, question,
   result, effect. Each [unverified] it names is annotated in place.
7. **Fixtures.** P1 step 7's XML, with `<sid>`, `<user>` and `<host>`
   placeholders, goes to
   `selfupdate/service/taskscheduler/testdata/definitions/` as
   `<case>.stored.xml`, beside the registered form as
   `<case>.wanted.xml`. A `README.md` there says where they came from.
   The identifier check runs on the directory.
8. Checks; commit (records and fixtures only).

### Phase J2: the COM transport (MADR F16)

1. **`com_windows.go`:** `folderCreateFolder = 11`, `folderGetTasks =
   14`, `regTaskGetName = 7`, `prinGetUserID = 11`, `prinGetRunLevel =
   17`, `taskCreate = 0x2`, `taskEnumHidden = 0x1`; each comment names
   the method, as the file's others do.
2. **`scheduler.go`:** the `scheduler` interface gains
   * `register(folder, name, xml string, flags, logon int32) error`,
   * `deleteTask(folder, name string) error`,
   * `createFolder(path string) error`,
   * `list(folder string) ([]listed, error)`, `listed` holding a task's
     `Name`, `State`, `LastResult` and `LastRun`;

   and `registered` gains `principal() (principal, error)`, holding
   `UserID`, `LogonType` and `RunLevel`.
3. **`scheduler_windows.go`:** the five, each through `withFolder` or
   `withTask`, BSTRs freed, out pointers starting nil and released, as
   the file's rules require (0021-MADR Amendment A3). `list` reads the
   collection with `get_Count` and 1-based `get_Item(VT_I4)`, as
   `instances` does.
4. **`fake_test.go`:** `fakeSched` implements them over its `tasks` map,
   records each call, and can fail any of them.
5. **`com_windows_test.go`:** `TestCOMLayout` registers its task through
   `register` instead of `comRegister`, lists the folder and finds it,
   reads `principal()` (`InteractiveToken`, the caller's SID by
   `windows.GetCurrentProcessToken`), creates and deletes a folder under
   `\` (skipped with the HRESULT on `0x80070005`, as J1 measured), and
   deletes the task through `deleteTask`, then deletes it again for
   P1's missing-task HRESULT. `comRegister` and `comDelete` go.
6. **Seen failing:** a plant of each new slot one lower fails
   `TestCOMLayout` on the host.
7. Run `go test -count=1 -run 'TestCOMLayout|TestVARIANTSize' ./selfupdate/service/taskscheduler/`
   on the host. Checks; commit.

### Phase J3: `Register`, `Definition`, `Delete` (MADR D2-A)

1. **`register.go`** (new):
   * `(*Task).Register(ctx, product, xml string) error`: `t.task(product)`;
     the logon type from `Principals/Principal/LogonType` by
     `encoding/xml`, `InteractiveToken` when absent; refuse `Password`,
     `InteractiveTokenOrPassword`, `Group` (a `GroupId`) and a service
     account (`UserId` of `S-1-5-18`, `S-1-5-19`, `S-1-5-20`, or their
     names), and `S4U` unless J1 accepted it; open `Options.Folder`,
     calling `createFolder` when it is missing (`0x80070002` or
     `0x80070003`); `register(folder, name, xml, taskCreateOrUpdate,
     logon)`; errors through `mapError("register", path, err)`.
   * `(*Task).Definition(ctx, product) (string, error)`: `open`, then
     `xml()`; `mapError` gives `ErrNotInstalled`.
   * `(*Task).Delete(ctx, product) error`: `deleteTask`; the missing-task
     and missing-folder HRESULTs J1 measured are `nil`.
   * Each checks `ctx.Err()` first; each doc says it ignores the context
     after that, as the package's other COM calls do (`task.go`'s `Task`
     doc).
2. **`register_test.go`** (new), over `fakeSched`: create, update, a
   missing folder created, a creation refused with `0x80070005`
   (`errors.Is(err, service.ErrPermission)`), every refused logon type
   with no `register` call, `Definition` of a missing task
   (`ErrNotInstalled`), `Delete` twice (nil both times), and an invalid
   product name.
3. **`live_windows_test.go`:** `TestLiveTaskRegister`: `Register` a
   `liveXML` task, `Definition` contains its command, `Register` again
   with another argument and read it back, `Status` (after J5; until
   then `Installed`), `Delete`, `Delete` again, `Definition` is
   `ErrNotInstalled`. `newLiveTaskIn` registers through `Register`.
4. **Seen failing:** plants of (a) `taskUpdate` for `taskCreateOrUpdate`
   (the create case fails on the host), (b) no folder creation, (c) a
   missing task returned as an error by `Delete`, (d) S4U accepted when
   J1 refused it, or refused when J1 accepted it.
5. `docs/architecture.md`'s `taskscheduler` row: the new files. Checks;
   `make apicheck` lists no incompatibility; commit.

### Phase J4: `DiffDefinitions`, `SameDefinition` (MADR D3-A)

1. **`definition.go`** (new):
   * `type DefinitionDiff struct { Path, Stored, Wanted string }` with
     JSON tags `path`, `stored`, `wanted`;
   * `DiffDefinitions(stored, wanted string) ([]DefinitionDiff, error)`
     and `SameDefinition(stored, wanted string) (bool, error)`;
   * a parse of the compared elements into a struct by `encoding/xml`,
     the encoding declaration ignored, since a BSTR is already decoded;
   * `defaults`, the table of schema defaults, one entry per compared
     `Settings` and trigger element, each with the schema page it comes
     from in a comment; where J1's stored XML shows another value for a
     conflicting default (MADR F15), the entry follows J1 and says so;
   * comparison of durations as values (`PT1M` equals `PT60S`), of
     booleans as `true`/`1` and `false`/`0`, and of text trimmed.
2. **`definition_windows.go`** and **`definition_other.go`** (new):
   `sameAccount(a, b string) bool`, case-insensitive, and on Windows
   resolving a SID to an account with `windows.StringToSid` and
   `LookupAccount`, and an account to a SID with `windows.LookupSID`.
3. **`definition_test.go`** (new):
   * for each J1 fixture pair, `DiffDefinitions(stored, wanted)` is empty;
   * for each element D3-A compares, a copy of `wanted` with that element
     changed gives exactly one diff naming its path;
   * an element absent on one side and set to its default on the other
     is no diff; set to another value is one;
   * `RegistrationInfo`, `URI`, `IdleSettings` and
     `UseUnifiedSchedulingEngine` differences are no diff;
   * JSON keys of `DefinitionDiff` pinned;
   * malformed XML on either side is an error.
4. **Seen failing:** plants of (a) a defaults entry changed, (b) one
   compared element dropped from the parse, (c) durations compared as
   text; each named test fails.
5. `docs/architecture.md`'s row. Checks; commit.

### Phase J5: `Status` (MADR D4-A)

1. **`status.go`** (new): `type Status struct` with `Installed`,
   `Enabled`, `State` (a string), `Running`, `PIDs []uint32`,
   `LastRunTime time.Time` and `LastResult int32`, JSON tags
   `installed`, `enabled`, `state`, `running`, `pids`,
   `last_run_time`, `last_result`; `(*Task).Status(ctx, product)`: one
   `open`, then `state`, `enabled`, `instances` and `lastRun`; a missing
   task is `Status{}` and no error. `PIDs` is a slice, so `Status` is not
   comparable; `make apicheck` decides nothing there, since the type is
   new.
2. **`status_test.go`** (new): running, ready, disabled, missing, an
   error from each call, and the JSON keys pinned.
3. **`live_windows_test.go`:** `TestLiveTaskRegister` and the existing
   rollback test check `Status` once each.
4. **Seen failing:** plants of (a) `stateName` not used (the number
   given), (b) a missing task returned as an error.
5. `docs/architecture.md`'s row. Checks; commit.

### Phase J6: `service` and `winproc` (MADR D6-A, D8-A's refusals)

1. **`internal/winproc/winproc.go`:** `CheckProgram(path string) error`,
   the exported form of `checkProgram`, on every OS (nil off Windows);
   `Command` and `Detached` call it. `winproc_windows_test.go` covers it
   with the cases `checkProgram`'s tests already use.
2. **`selfupdate/service/handoffenv.go`:**
   * `const ArgHandOffEnv = "--selfupdate-handoff-env="`, documented as
     the argument a Task Scheduler handoff uses in place of
     `EnvHandOffEnv`, which a task cannot carry (0032-MADR F11);
   * `LoadHandOffEnvContext`: first, when `os.Args[1]` has the prefix,
     remove it from `os.Args`, and apply that file as the
     `EnvHandOffEnv` file is applied; then `HandOffHopContext`; then the
     `EnvHandOffEnv` file as today. Its doc states the order, and that
     the argument is read only at `os.Args[1]`.
3. **`handoffenv_test.go`:** the argument applied and removed, with the
   remaining arguments intact; the argument at `os.Args[2]` left alone;
   a relative path refused as `EnvHandOffEnv`'s is; `EnvHandOffHop` from
   the file seen by the hop check that follows; no argument, no change.
   The tests run on every OS.
4. **Seen failing:** plants of (a) the hop check before the file, (b) the
   argument left in `os.Args`, (c) the argument read at any position.
5. `make apicheck` lists `ArgHandOffEnv` and `CheckProgram` is internal.
   Checks; commit.

### Phase J7: the one-shot handoff (MADR D5-A, D7-A, D8-A)

1. **`task.go`:** `Options.HopDetach bool` and `Options.HandOffDir
   string`, documented as D5-A and D8-A say; the package doc's scope
   sentence becomes "never renders the program's definition" (D1-A).
2. **`handoff.go`** (new):
   * `oneShotXML(principal, executable, args, workDir, description)`,
     rendering D8-A's definition with `encoding/xml` escaping;
   * `handOffName(name, id)`, `<name>.selfupdate.<id>`;
   * `(*Task).detachTask(ctx, spec)`: `CleanupHandOffs` best-effort (a
     cancelled `ctx` returns); open the service task and read its
     `principal()`; refuse a logon type other than `InteractiveToken`
     with an error naming it and `Options.HopDetach`;
     `winproc.CheckProgram(spec.Executable)`; refuse a `"` in it;
     `service.WriteHandOffEnv(dir, spec.ID, append(os.Environ(),
     append(spec.Env, service.EnvHandOffHop+"=1")...))`; arguments
     `ArgHandOffEnv+file` then `spec.Args`, composed by
     `windows.ComposeCommandLine` through a small OS-split helper;
     `register(folder, handOffName, xml, taskCreate, logonInteractiveToken)`;
     `run()`; wait for an instance or `LastRunTime` at or after the call,
     at `Options.Poll`'s interval, bounded by `ctx` and its timeout; on
     any failure after registration, `deleteTask` and remove the file;
     return `Detached{ID, Where: "task " + path, ResultPath}`.
   * `(*Task).CleanupHandOffs(ctx) error`: `list(folder)`; for each name
     with the prefix `<name>.selfupdate.`, skip it when `State` is
     running or `LastResult` is `0x00041303`; otherwise `deleteTask`, and
     remove `handoff-<id>.env` in the handoff directory if present;
     errors joined.
3. **`detach.go`:** `Detach` calls `detachTask` unless
   `Options.HopDetach`, which keeps today's body. Its doc describes both.
4. **`handoff_test.go`** (new), over `fakeSched`:
   * the rendered XML parses, and `DiffDefinitions` against a golden
     `testdata/oneshot.xml` is empty;
   * the principal copied; a non-interactive logon refused, naming
     `HopDetach`, with nothing registered;
   * a `.cmd` executable and a quoted path refused;
   * the environment file holds `EnvHandOffHop=1` and the handoff
     variables, and the definition holds only its path;
   * the arguments composed and quoted;
   * a register failure, a run failure and a timed-out start each leave
     no task and no file;
   * `Where` and `ResultPath`;
   * `HopDetach` starts the hop and registers nothing;
   * `CleanupHandOffs` deletes finished tasks and their files, keeps a
     running one, a never-run one and another task's, and joins errors.
5. **`live_windows_test.go`:** `TestLiveTaskHandOffOneShot`, from J1's
   P2 shape: the live task's program starts a child in a kill-on-close
   job; the child runs the update through `Task.Detach`; the service
   process then closes the job; the update completes, its
   `HandOffResult` says applied, a new instance runs, the one-shot task
   is gone after `CleanupHandOffs`, and no window was created
   (`assertNoWindow`, as the existing handoff test checks). The existing
   `TestLiveTaskUpdate` runs with `HopDetach` set, so the hop stays
   covered.
6. **Seen failing:** plants of (a) `DisallowStartIfOnBatteries` left at
   its default (the golden test), (b) the handoff argument omitted (the
   live test's run cannot load its environment), (c) the cleanup
   deleting a running task, (d) `taskCreateOrUpdate` for `taskCreate`
   (the collision test), (e) `HopDetach` ignored.
7. `docs/architecture.md`'s row. Checks; `make apicheck`; commit.

### Phase J8: the documents

1. **`docs/guides/extending-selfupdate.md`:** the per-platform handoff
   list's Task Scheduler entry describes the one-shot task, its name,
   its folder, `HandOffDir`, `CleanupHandOffs`, the brief console window
   and `HopDetach`; "Call `service.ReportFunc` first in `main`" adds that
   on Windows the handoff passes its file as the first argument.
2. **The guide's Task Scheduler section** (wherever `taskscheduler` is
   documented, found with `git grep -n taskscheduler docs/guides`):
   `Register`, `Definition`, `Delete`, `DiffDefinitions` and `Status`,
   with a setup example: `Definition`, `DiffDefinitions`, refuse or
   `Register`.
3. **The migration guide:** `## 19. From v1.18 to v1.19`, with what is
   new, the changed default `Detach`, `ArgHandOffEnv`, and adopting it.
4. **`CHANGELOG.md`:** `## Unreleased`, Added and Changed.
5. **`docs/architecture.md`:** the `taskscheduler` paragraph (scope,
   handoff), and `service`'s handoff paragraph.
6. **0021-MADR:** a dated pointer under its amendments to this record,
   for D1 and D9, made on acceptance.
7. Snippets compile in a scratch module outside the repository against
   the working tree, with `GOOS=windows`. Checks; commit.

### Phase J9: `v1.19.0`

The architecture page's "Releasing this module", steps 1 to 7, with
`vX = v1.19.0`, as 0031-PLAN H4 ran it: the pre-tag change and
`make gate`; the push and CI on `main`; the tag check, the disclosure
guard and the tag on the owner's word; the tag's CI, including the new
live tests on `windows-2025`; the pin commit; the proxy. Issues #3 and
#6 are closed with a comment naming `v1.19.0`, on the owner's ask.

### Phase J10: closing

`### Verification, at closing`, `status: complete`, the index row. The
MADR stays `accepted`.

## Verification

* **V1 (probes):** P1 and P2 ran twice with agreeing results, and the
  MADR's Amendment A1 records them (J1).
* **V2 (transport):** `TestCOMLayout` passes on the host with every new
  slot, and failed on each planted slot (J2).
* **V3 (registration):** `register_test.go` passes; `TestLiveTaskRegister`
  passes on the host and in CI (J3).
* **V4 (comparison):** every fixture pair compares equal, every
  single-element change gives one diff, and each plant failed (J4).
* **V5 (state):** `status_test.go` passes; `Status` agrees with the live
  task (J5).
* **V6 (environment):** the handoff argument tests pass on every OS (J6).
* **V7 (handoff):** `TestLiveTaskHandOffOneShot` completes an update
  from inside a kill-on-close job, on the host and in CI, and
  `TestLiveTaskUpdate` still passes through the hop (J7).
* **V8 (API):** `make apicheck` against `v1.18.0` reports no
  incompatibility; `go doc` shows the additions.
* **V9 (release):** `v1.19.0` tagged, its CI green, the proxy serving it
  as `@latest`; #3 and #6 closed (J9).

## Rollout and Rollback

* **Rollout:** one commit per phase on `main`, pushed on the owner's
  word; `v1.19.0` at J9. A program adopts registration by calling
  `Register` in its setup, and gets the one-shot handoff by requiring
  `v1.19.0`; `Options.HopDetach` keeps the old handoff.
  magic-cli-remote turns its Windows handoff on in its own repository
  (its 0210-MADR D3).
* **Rollback:** before the tag, `git revert` of a phase; after it, fix
  forward. A program that finds the one-shot handoff wrong for it sets
  `HopDetach` without waiting for a release.
