---
status: in-progress
date: 2026-10-27
associated-madr: "0038-MADR-pinned-plans-lifecycle-conformance-and-bubble-tests.md"
---
<!-- markdownlint-disable MD004 MD013 MD024 MD060 -->

# Implement a pinned update plan, a lifecycle conformance check, and tests on the production clock

Associated MADR: [0038-MADR-pinned-plans-lifecycle-conformance-and-bubble-tests.md](0038-MADR-pinned-plans-lifecycle-conformance-and-bubble-tests.md)
(D1-A, D2-A and D3-A, with the owner's answers 1 to 5 of 2026-10-27).

## Goal

* **v1.22.3** (owner answer 4): every fake-driven test that measures time runs in a `testing/synctest` bubble on the production clock; the test clocks and the production `now`, `sleep` and `pollClock` seams are gone; `selfupdate`'s seams belong to the values that use them, and its tests may run in parallel. No exported identifier changes, and no behaviour a program can see.
* **v1.23.0:** `servicetest` checks any `Lifecycle` and `Runner`, and every backend passes it over its fake and its real manager; the backends do what they document (MADR F11); the install path resolves names through the session's root; a host, `cli` and a detached run can plan an update once and apply exactly that plan.
* Each new test or check is first seen failing.

## Scope

### In scope

* `selfupdate/service` and its four backends: tests, fakes, the clock seams, `servicetest`, the F11 fixes, live labs, the handoff plan carrier.
* `selfupdate`: the seams, the replace path, `Plan`, `Apply`, `StartApply` and the document types.
* `selfupdate/cli`: `HandOff.DetachPlan` and `HandOff.Planned`.
* `selfupdate/selfupdatetest`: `Publish` on both fakes; the ID check in `VerifyReleaseSource`.
* The codesign and ghattest test fakes (MADR F11).
* `.golangci.yml` (one depguard rule), `Makefile` (fuzz target count), AGENTS.md, `docs/architecture.md`, `docs/guides/extending-selfupdate.md`, the migration guide, `CHANGELOG.md`, package docs, `docs/README.md`.
* The tags `v1.22.3` and `v1.23.0`, each on the owner's word.

### Out of scope

* `Availability.Plan` and a plan-returning `Checker`; a `StartPlan` stream; `Root.Chmod`; scoping `CheckNoLeak` per bubble (MADR, set aside).
* `.github/workflows/ci.yml`: the new live tests are named to match today's selectors (`^TestLive`, `^TestLiveTask`, `^TestLiveSupervisedS6$`). A phase that finds it must change the workflow stops as a deviation.
* `internal/cmd/selfupdate-release`, `buildinfo`, `archive`, `releasespec`, `signednote`, the installers.
* Any API removal or signature change.

## Rules

As 0037-PLAN's:

* **Commits** go on `main` only on the owner's "commit to main" in that turn, with `git commit --no-edit`. A push or a tag needs its own word.
* **Files.** Each phase names its files. A file outside them is a deviation: stop and prompt. `docs/architecture.md`'s package table is in every phase's list, for a phase that adds or removes a `.go` file.
* **Deviations.** Each phase stops at a deviation. It is recorded here, dated, and, when a decision or a fact changes, in the MADR as an amendment.
* **Seen failing.** Every new test, check or bubble move is seen failing on a planted break in a scratch copy (`scripts/plant-copy.sh FILE OLD NEW`, or a scratchpad script), never in the tree. The plant and the failure line go into the execution record. A Linux-only test's plant runs on a Linux host, a Windows-only test's on the Windows test host.
* **Checks per phase:** `make pre-add-check FILES=…` on the phase's Go files; cross-target `go vet` (`CGO_ENABLED=0 GOOS=linux|darwin|windows`); `go test -race` on the touched packages; `make docs-check`, `make markdownlint` and `make check-records` when Markdown changes; the architecture table when a `.go` file is added or removed; `make apicheck` compatible with the newest `v1.*` tag; and `make gate` ending `overall=0` before the owner's commit.
* **Bubbles.** A test moves into `synctest.Test` in the same step that it drops its virtual clock (MADR F9). No `t.Run`, `t.Parallel` or `t.Deadline` inside a bubble: a table makes one bubble per subtest. No `httptest`, real process or real file wait inside a bubble. Intervals are realistic (the documented default, or at least 1 ms of bubble time per step), so a bounded wait is not millions of steps. A check never depends on which of two timers due at the same instant fires first (MADR F5).
* **Floor.** No API from Go 1.27: no `synctest.Sleep`. The `go126` job passes on every push.

Hosts:

* **This Mac** runs launchd's live tests and probes, in throwaway jobs in the user's GUI domain.
* **The Linux test host** and **the WSL on the Windows test host** run systemd's live tests (system and user scope) and the s6 test.
* **The Windows test host** runs scm's and Task Scheduler's live tests, and every Windows-only plant.
* CI runs every live job on `main` and on each tag.
* **Identifiers.** Probe and host output carries user and host names, which are replaced by `<user>` and `<host>` before anything enters the tree.

## Implementation Steps

### Phase Q0: records

1. This plan and the amended MADR, `proposed`, and the plan's row in `docs/README.md` after the 0038 MADR's.
2. `make check-records`, `make docs-check`, `make markdownlint`, `make disclosure-check`, `scripts/check-docs.sh --ids` on the three files. Commit on the owner's word (bootstrap exception: records only).
3. The owner's approval, with question 5 answered, moves this plan to `in-progress` and the MADR to `accepted`.

---

**Release 1: `v1.22.3`** (D3-A bubbles and seams; no exported change)

### Phase Q1: `service` on the production clock

Files: `selfupdate/service/poll.go`, `supervised.go`, `supervised_lifecycle.go` (the clock only), `poll_test.go`, `supervised_lifecycle_test.go`, `supervised_health_test.go`, `supervised_test.go`, `supervised_linux_test.go` if it sets the fields.

1. **P1 as a committed test.** `TestSupervisedStopRestoresInBubble`: P1's body (MADR More Information), `Poll{Timeout: DefaultPollTimeout, Interval: time.Second, Settle: -1}`, with `Down` delayed 0 and half the timeout. Seen failing with `supervised_lifecycle.go` replaced by `git show v1.22.0:selfupdate/service/supervised_lifecycle.go` in a scratch copy: `start again after the failed stop of demo: context deadline exceeded`.
2. **`PollHealthy`.** Delete `pollClock` (`poll.go:78-89`); `PollHealthy` calls `time.Now` and its context-aware sleep. The seven `fakeClock` tests in `poll_test.go` run in bubbles, assert elapsed bubble time with `time.Since`, and `fakeClock` and `useFakeClock` are deleted. Plants: the settle restart (`TestPollHealthyInstanceChangeRestartsSettle`), the timeout's last error, and the context end each fail their test.
3. **`fakeSup` steps on observation.** Its state moves once per `State` read made at a later bubble time than the last, in place of `vclock`'s tick; `Down` and `Up` may take bubble time (`time.Sleep` inside the bubble).
4. **`Supervised`'s clock.** The `vclock` tests (`TestSupervisedStop`, `…RefusesFirst`, `…Timeout`, `…BoundIncludesDown`, `…OutlivesContext`, `…Kill`, `…KillError`, `…ReusedPID`, `…DownFails`, `…NoSupervisor`, `TestSupervisedStart`) run in bubbles at the documented defaults; `vclock` and `supervisedOver`'s clock are deleted; `Supervised.now` and `sleep` (`supervised.go:130-136`, `:185`) are deleted, and the code calls `time.Now` and `sleepCtx`. Plants: the kill time, the bound that includes `Down`, and B3's outliving wait each fail their test.
5. **`TestSupervisedStopRestoresOnRealClock`** is deleted: step 1's test covers it at the default bound. Its comment's record citations move to step 1's test.
6. **`healthOver`'s clock.** `TestSupervisedWaitHealthy` and `…Probe` run in bubbles, and `healthOver` drops its stepping clock. This supersedes 0035-PLAN Deviation D4's "Virtual clock, in M11" (MADR D3-A); the test's comment says so. Plant: the settle window's restart.
7. `go test -race -count=3 ./selfupdate/service/` on this Mac, the Linux test host and the Windows test host.

### Phase Q2: scm on the production clock

Files: `selfupdate/service/scm/service.go`, `fake_test.go`, and every scm test file that calls `testService` (`scm_test.go`, `stopwait_test.go`, `detach_test.go`, `unquoted_test.go`, the Windows-only `alias_*` and `compose_*` tests).

1. `fakeSvc`'s queued states move on observation (as Q1 step 3), not on `virtualClock.onSleep`.
2. `testService` returns the `*Service` alone. Every caller runs in a bubble and measures with `time.Since`. `virtualClock` is deleted, and `Service.now` and `sleep` (`service.go:63-64`, `:100`) are deleted.
3. `TestStopWaits` gets a `Poll.Timeout` that admits its 5 s wait (the default), as P3 showed (MADR F4). The fix is named in its comment.
4. `TestDetach` and every test that starts a process stay on the real clock.
5. Plants: the stall floor (`stallFloor`), the clamp of a wait hint to 1 s, and the stop's `Poll.Timeout` each fail their test.
6. Windows-only tests run on the Windows test host, with `-race`.

### Phase Q3: Task Scheduler on the production clock

Files: `selfupdate/service/taskscheduler/task.go`, `fake_test.go`, and every test file that calls `testTask` (`lifecycle_test.go`, `enable_test.go`, `handoff_test.go`, `reconcile_test.go`, `register_test.go`, `status_test.go`, `detach_test.go`).

1. `fakeTask`'s `exitIn` and `stopKeeps` count observations at a later bubble time, and the fake's own `now` (`fake_test.go:25`, `:75`, `:426`) reads `time.Now`.
2. `testTask` returns the `*Task` alone; callers run in bubbles; `virtualClock` and `Task.now`/`sleep` (`task.go:115-116`, `:181`) are deleted. `TestDetachOneShotFailures` moves in the same step (MADR F9).
3. `TestStopTimeoutOnRealClock` becomes a bubble test at the default `Poll.Timeout` (P3's probe 3) and is renamed `TestStopTimeoutAtDefaultBound`.
4. Plants: `StopGrace` inside the bound, the still-running report at timeout (0037-MADR D5-A), and the handoff's one-shot deadline each fail their test. Windows-only files on the Windows test host.

### Phase Q4: systemd and launchd in bubbles

Files: `selfupdate/service/systemd/systemd_test.go`, `stopwait_test.go`, `fake_test.go`; `selfupdate/service/launchd/launchd_test.go`, `fake_test.go`, `lifecycle_test.go` if present. No production file.

1. systemd: `TestStopStalls` and `TestWaitHealthyNeedsNewInvocation` move from `Interval: 1` to a realistic interval (`systemd_test.go:185`, `:255`); `TestStopBoundsAJobThatNeverEnds` and `TestStopFinishesAfterCancel` run in bubbles at the default bound, and the first asserts `time.Since(start)` equals the bound exactly; the health and stop tests that wait run in bubbles. `TestCapturedShowOutput` and `TestWaitHealthyFailsFast` make one bubble per subtest.
2. launchd: the stop, bootstrap and health tests that wait run in bubbles; `TestCapturedOutput`, `TestProbeUsesListWhenItSees`, `TestLoaded` and `TestStatus` make one bubble per subtest if they wait, and otherwise stay as they are.
3. Plants: systemd's `--no-block` (0037-MADR F2), launchd's `bootout` on `bctx` (F3) and its bootstrap `ErrTimeout` (F4), as 0037-PLAN N3 and N4 planted them.

### Phase Q5: `selfupdate` seams, the scaffold

Files: `selfupdate/replace.go`, `replace_unix.go`, `replace_windows.go`, `journal.go`, `journal_other.go`, `journal_windows.go`, `leftovers.go`, `lock.go`, `lock_unix.go`, `lock_windows.go`, `session.go`, `standalone.go`, `cleanup.go`, `cleanup_other.go`, `cleanup_windows.go`, `close.go`. No test file.

1. `type fsOps struct` in `replace.go`, with one function field per filesystem seam (`fileChmod`, `fileSync`, `fileClose`, `osChmod`, `osRemove`, `osLink`, `syncDir`, `syncRoot`, `replacePath`, `writeJournal`, `keptRename`, `leftoverRemove`, `lockOpen`, `afterLock`), and an embedded build-tagged `osFS` (`lchown` on Unix in `replace_unix.go`; `moveFileEx` on Windows in `replace_windows.go`). Nil `lockOpen` and `afterLock` do nothing.
2. `realFS() *fsOps` returns a new value whose fields, in this phase, are closures that call the package variables at call time, so every `setSeam` keeps working, including those set after a session begins.
3. `StandaloneInstaller` gains an unexported `fs *fsOps` and `ops()`, which returns `realFS()` when `fs` is nil, so a zero `StandaloneInstaller{}` still works (0015-MADR C8). `beginSession` stores it in the session.
4. The install path's free functions become `*fsOps` methods: `replaceTarget`, `commitReplacement`, `rollbackReplacement`, `keepAsPending`, `backupFile`, `copyFile`, `chmodStaging`, `chownStaging`, `clearSpecialBits`, `writeJournal`, `removeJournal`, `recoverJournal`, `keepInterrupted`, `removeLeftovers`, `openLockFile`, `acquireLock`, `processCleanupReceipt`, and on Windows `writeCleanupReceipt`, `writeCleanupEntries` and `moveFileReplace`. The tests that call them directly call `realFS().name(…)` in this phase; that is the phase's one test change, and its files are added to its list when it is made.
5. No behaviour change: the full suite passes on the three hosts before and after, with the same test count.

### Phase Q6: `selfupdate` seams, the filesystem

Files: Q5's, and the test files that set each seam (from `setSeam` and the helpers `failRestore` and `swapAfterFirstReplace`).

For each seam, one step, smallest first: `osRename` and `evalSymlinks` (deleted: no test sets them), `afterLockHook`, `lockOpenHook`, `leftoverRemove`, `keptRenameFn`, `writeJournalFn`, `fileChmod`/`fileSync`/`fileClose`, `osLink`, `osLchown`, `osChmod`, `osRemove`, `syncRootFn`, `syncDirFn`, `replacePath`, `moveFileExFn` (Windows; run on the Windows test host). Each step:

1. its tests set the field on an `fsOps` they pass in (`inst.fs = fx`, or the session's `fs`), not the package variable;
2. `realFS()`'s field becomes the real function, and the package variable is deleted;
3. one of its tests is seen failing on a plant of the line it covers.

### Phase Q7: `selfupdate` seams, the rest, and parallel tests

Files: `selfupdate/target.go`, `github.go`, `credentials.go`, `confirmer.go`, `updater.go`, `version.go`, `checker.go`, `githubupdater.go`, `export_test.go`, `install_hardening_test.go` (`setSeam`), and the test files that use `withTempHome`, `noEnv`, `cacheEnv`, `progressRun`, `ttyPipe` and `SetRunningPlatform`.

1. **Target resolution.** `resolveTarget` takes its home-directory and executable lookups from the installer (unexported fields with real defaults). `withTempHome`'s callers that only need an allowed root pass `TargetPolicy.AllowedRoots`; `TestHomeUnavailableRoot` and `TestResolveTargetDefaultExecutable` set the fields.
2. **`timeNow`** is deleted. `cacheEnv` and the check-cache tests pass `CheckerConfig.Clock`; `progressRun` passes `Config.Clock`; the rate-limit test sets `GitHubSource.now` as `github_test.go:284` does.
3. **`lookupEnv`.** An unexported `newGitHubSource(opts, getenv)` behind `NewGitHubSource`, and a `getenv` field on `envCredential` with an `os.Getenv` default; `noEnv` returns options, not state.
4. **`isTerminal`.** An unexported `newTerminalConfirmer(in, out, isTerminal)` behind `NewTerminalConfirmer`; `ttyPipe` uses it.
5. **`runningPlatform`.** An unexported `platform` field on `Updater` and `Checker`, filled by `New` and `NewChecker` and read through a method that falls back to `runtime.GOOS/GOARCH`; `validateRequest` and `normalizePlatform` take it; `SetRunningPlatform` in `export_test.go` becomes a setter on the value.
6. `setSeam` and its comment are deleted. `grep` finds no package-level `var … = os.` or function-variable seam in `selfupdate`'s non-test files.
7. **Parallel.** Every test in `selfupdate` that does not call `CheckNoLeak`, `t.Setenv` or `t.Chdir` calls `t.Parallel()` first; the three kinds stay sequential (MADR F7). `go test -race -count=3 -shuffle=on ./selfupdate/` passes on the three hosts, and the stray-`.git` run (AGENTS.md) passes.
8. Each step's plant: one test per moved seam.

### Phase Q8: `v1.22.3`

1. The pre-tag change: a `## v1.22.3` entry in `CHANGELOG.md` ("Changed: tests only"; no program-visible change), and a "From v1.22.2 to v1.22.3" entry in the migration guide's §22 saying nothing changes for a program. `make gate`, `make apicheck` (no change against `v1.22.2`).
2. The push and CI on `main`, on the owner's word.
3. The tag check, the disclosure guard, and the tag on the owner's word.
4. The tag's CI, its identity legs printing `v1.22.3 (release) <12hex>`.
5. The pin commit, and the proxy serving `v1.22.3` as `@latest`.

---

**Release 2: `v1.23.0`** (D2-A, D3-A root-relative replacement, D1-A)

### Phase Q9: `servicetest`

Files (new): `selfupdate/service/servicetest/doc.go`, `lifecycle.go`, `runner.go`, `fake.go`, `lifecycle_test.go`, `runner_test.go`, `fake_test.go`, `example_test.go`. Changed: `.golangci.yml`, AGENTS.md, `docs/architecture.md`, `selfupdate/service/doc.go`.

1. **The API** (MADR D2-A), every exported identifier with a doc comment, and the package doc naming the bubble and the real-clock uses:

   ```go
   type State int // Absent, Stopped, Running, SlowStop, Stubborn, Crashing, StartHangs
   type Arrangement struct { State State `json:"state"`; Enabled bool `json:"enabled"` }
   type Observation struct {
       Installed, Running, Stopping, Enabled, Wanted bool // json snake_case tags
       Detail string `json:"detail,omitempty"`
   }
   type Lab interface {
       Arrange(ctx context.Context, a Arrangement) error
       Observe(ctx context.Context) (Observation, error)
   }
   type Method uint8 // MethodInstalled | MethodRunning | MethodEnabled | MethodStart
   type LifecycleCase struct {
       Product string; Lab Lab
       StopBound, StartBound, Slack time.Duration
       ContextFree Method; RestoresAfterFailedStop, RequireAll bool
       Logf func(format string, args ...any)
   }
   func VerifyLifecycle(ctx context.Context, l selfupdate.Lifecycle, c LifecycleCase) error
   type RunnerCase struct {
       Quick, Blocking service.Command; Calls func() int; KillBound time.Duration
   }
   func VerifyRunner(ctx context.Context, r service.Runner, c RunnerCase) error
   type FakeService struct{ /* unexported */ }
   func NewFakeService(o FakeOptions) *FakeService // a Lifecycle, an EnabledLifecycle and a Lab
   ```

   `FakeOptions` sets the stop and start latencies and the bound the fake documents. `VerifyLifecycle` refuses a nil lifecycle, a nil lab, an empty product and a zero `StopBound` before any check, and leaves the service `Absent`. Violations are joined, each prefixed `servicetest:` and naming its check, with `%w` for the error it saw (errorlint).
2. **The checks**, in MADR D2-A's order. A lab error wrapping `errors.ErrUnsupported` skips the checks that need its state, through `Logf`; with `RequireAll` it is a violation.
3. **Tests**, after `selfupdatetest/conformance_test.go`: `TestVerifyLifecyclePasses` (`FakeService`, in a bubble); `TestVerifyLifecycleFails`, a table of `broken` wrappers, one rule each: `Stop` ignores its context; `Stop` returns nil for a stubborn service; `Stop` returns on cancel before the service has gone; `Running` lies after a timeout; `Start` on an absent service starts it; `WaitHealthy` is nil on a crash; a hanging start is not `ErrTimeout`; the restore is lost; `Enabled` disagrees with the lab; the bad-case refusals; the skip and `RequireAll`. `VerifyRunner`: passes on a correct fake; fails on one that ignores its context, one that records a call on an ended context, and one whose blocking command ignores cancellation. Each table row is itself the planted break; additionally, each check is seen failing by disabling it in a scratch copy and watching its row's `broken` pass.
4. **depguard:** the `service-servicetest` rule (standard library, `selfupdate`, `service`) and `!**/selfupdate/service/servicetest/*.go` in `other-packages`. Seen failing: a planted `internal/winproc` import in a scratch copy reports `not allowed from list 'service-servicetest'`.
5. AGENTS.md's scope paragraph and depguard list; `docs/architecture.md`'s tree, package table row and rule count (20 to 21); `service/doc.go`.

### Phase Q10: the backends do what they document (MADR F11)

Files: `selfupdate/service/scm/lifecycle.go`, `scm/service.go` (doc), `launchd/lifecycle.go`, `supervised_lifecycle.go`, their tests; `selfupdate/codesign/codesign_test.go`, `selfupdate/verify/ghattest/ghattest_test.go`; `docs/guides/extending-selfupdate.md` where it states a bound or an error.

1. **Probe P6, on this Mac:** `launchctl bootstrap gui/<uid> <missing plist>`, its exit code and output, and `launchctl print` of a never-loaded label. The result goes into the MADR as Amendment A1. A result that voids a fact stops the phase.
2. **scm `Start`** checks `ctx.Err()` before it opens the service and before `h.start()`. Test: a cancelled context issues no start (`fakeSvc` records no `start`). Seen failing on today's code: the fake records `["open" "start"]`.
3. **launchd `Installed`** is documented as ignoring its context, in `Job`'s doc beside its siblings', and the guide's context table says so.
4. **launchd `Stop`:** `bootout` and the wait share one deadline, `bound` from the moment the bootout is issued; the documented bound (`lifecycle.go:134-139`) is unchanged. Test, in a bubble: a stubborn job whose bootout takes half the bound returns `ErrTimeout` at the bound, not 1.5 times it. Seen failing on today's code.
5. **launchd `Start`** returns `ErrNotInstalled`, wrapped with the plist path, when the plist does not exist, before `enable`. Test with the fake; seen failing on today's code, which retries the bootstrap to `ErrTimeout`.
6. **`Supervised.Start`**, as question 5 decides. If yes: `ErrNotInstalled` when `State` succeeds and reports `Installed` false; `Up` otherwise. Test with `fakeSup`; seen failing today (`calls ["up"]`). If no: its doc states that it runs `Up` for a service the supervisor does not know, and Q11's lab for `Supervised` declares it.
7. **The codesign and ghattest fakes** fail a call on an ended context and record nothing, as 0037-MADR D1-A; each package gets a `TestFakeHonoursContext`, seen failing on today's fake.

### Phase Q11: `servicetest` over the fakes

Files (new): `selfupdate/service/systemd/conformance_test.go`, `launchd/conformance_test.go`, `scm/conformance_test.go`, `taskscheduler/conformance_test.go`, `selfupdate/service/export_test.go` (package `service`), `selfupdate/service/conformance_test.go` (package `service_test`), `selfupdate/service/runner_conformance_test.go` (package `service_test`). Changed: the four backends' `fake_test.go`.

1. **Fake labs**, each implementing `Lab` over its package's fake, in a bubble:
   * systemd: a fresh `InvocationID` per start; `Stubborn` through `stopBlocks`; `SlowStop` and `StartHangs` through the fake's hooks.
   * launchd: the pid kept while a bootout is pending (today the fake clears it at bootout, `fake_test.go:154`); a slow bootout; `ContextFree: MethodInstalled`.
   * scm: `Stubborn` as STOP_PENDING forever; `ContextFree: MethodInstalled|MethodRunning|MethodEnabled`.
   * Task Scheduler: `ContextFree: MethodInstalled|MethodRunning|MethodEnabled|MethodStart`; `RestoresAfterFailedStop`.
   * `Supervised`: `export_test.go` exposes a constructor over `fakeSup` and fake `readStat`, `self` and `exe`; the external test runs `VerifyLifecycle` with `RestoresAfterFailedStop` and a `Down` that takes one second of bubble time.
2. **`VerifyRunner`** over the systemd and launchd fakes, and over `ExecRunner` on the real clock with the test binary's sleep mode (`SELFUPDATE_SERVICE_FAKE=sleep`, `main_test.go:20`). The two `TestFakeHonoursContext` tests stay: they name the rule where it lives.
3. **Seen failing** (MADR Confirmation): 0037-MADR F1 to F4 planted into scratch copies of their backends (the OLD/NEW strings in the execution record), and 0035-MADR Amendment A3 as the `v1.22.0` `supervised_lifecycle.go`, each fails its backend's conformance test. Each backend passes on `main`, 20 runs each with `-race`.

### Phase Q12: `servicetest` over the real managers

Files: each backend's `live_*_test.go` and `live_fixes_*_test.go`, `selfupdate/service/supervised_live_linux_test.go`, and each package's `main_test.go` for new helper modes.

1. Each live test file gains a `liveLab` whose `Arrange` and `Observe` drive the real manager, built from the helpers it has: systemd's `newLiveUnitWith`, launchd's `newLiveJob` and `newSlowLiveJob`, scm's `newLiveService` and `removeService`, Task Scheduler's `newLiveTaskWith`, and the s6 scan directory. The helpers return errors, with the `t`-taking wrappers kept for the existing tests.
2. New helper modes where no live test arranges the state today (MADR Decision Outcome): systemd `SlowStop` (a TERM delay) and `Crashing`; launchd `Stubborn` (`FAKE_IGNORE_TERM`, with an `ExitTimeOut` launchd honours as no kill, or `ErrUnsupported` if P6 shows none) and `Crashing`; scm a handler that ignores stop and one that stops slowly with checkpoints, with cleanup that ends the process before `removeService`; Task Scheduler a definition without the minute watchdog trigger, and `Stubborn` and `SlowStop` returning `ErrUnsupported`. s6 `Stubborn` through a run script that ignores TERM.
3. Tests: `TestLiveConformance` (systemd, launchd, scm) and `TestLiveTaskConformance`, with `RequireAll` and a short `Poll.Timeout` whose bound is passed as `StopBound`; for s6, a `conformance` subtest of `TestLiveSupervisedS6`. The names match CI's selectors, so `ci.yml` does not change.
4. The existing live tests whose setup matches a state move it to `lab.Arrange`; bespoke setups (override drop-ins, a missing dependency, the handoff) stay helpers.
5. Runs: systemd on the Linux test host and the WSL, in both scopes; launchd on this Mac; scm and Task Scheduler on the Windows test host; s6 on the WSL; then CI. Seen failing: one F11 or 0037 plant per backend, on its host.

### Phase Q13: root-relative replacement

Files: `selfupdate/replace.go`, `replace_unix.go`, `replace_windows.go`, `session.go`, `journal.go`, `filedigest.go`, `leftovers.go`, `cleanup_windows.go`, and their tests; a new `selfupdate/rootswap_test.go`.

1. **P2 as a committed test**, `TestInstallRefusesDirectorySwappedBeforeRename`: the `writeJournal` field of `fsOps` swaps the directory between the check and the rename (MADR P2). Seen failing on today's code with P2's output.
2. **Names in the root** (Unix and Windows): `rootCreateTemp(root, prefix)`, an `O_CREATE|O_EXCL` loop with the all-digit suffix `isLeftover` and `keptName` parse, for the staging file and the reserved names; the backup through `root.Link` to a fresh name, retrying on `EEXIST`, or a copy through `root.OpenFile`; `root.Lstat` for the target and staging; `rootFileSHA256` for the target's digest; `root.Remove` for the backup, staging and `.previous`; `syncRootFn(s.root)` for every directory flush.
3. **Owner and mode through a descriptor:** `root.OpenFile(base, O_RDONLY|O_NOFOLLOW)`, `os.SameFile` against the `Lstat`, then `(*os.File).Chown` and `Chmod`, for `chownStaging`, `chmodStaging` and `clearSpecialBits`. A mismatch is `ErrConcurrentUpdate`.
4. **Rename:** on Unix `root.Rename` (the `fsOps.replacePath` field takes the root and two names); on Windows `moveFileReplace` on paths, unchanged (MADR F8).
5. **Rollback:** after the rollback rename, a backup that is still present and the same file as the target is removed through the root (MADR P2).
6. Tests: step 1's test passes, the decoy untouched; a test per moved call that its descriptor or root use is reached (plant: the old path call restored); the existing 0015-MADR B2, 0004 R3/R5 and 0010 tests (`TestInstallRefusesTargetReplacedAfterBegin`, `TestInstallRollsBackWhenDirectoryMovesAfterRename`, `TestBeginChecksDirectoryBeforeReceipt`) pass unchanged; the Windows busy and budget tests pass on the Windows test host; the live root tests (`SELFUPDATE_REQUIRE_ROOT`) on the Linux test host.

### Phase Q14: `Plan` and `Apply`

Files: `selfupdate/checker.go`, `updater.go`, `runoptions.go`, `errors.go`, `types.go` (`Release.ID`'s doc), new `plan.go`, new `plan_test.go`, `plan_document_test.go`, `testdata/golden/plan.golden`, `fuzz_test.go`, `example_test.go`, `Makefile`.

1. **Discovery split, no behaviour change.** `discover` becomes `fetchRelease` plus `vet(ctx, req, rel, fromLatest)`, which holds the immutable, state, `Validate`, channel, selection, metadata, classification and unpacker checks, and returns `fromLatest`. The suite, the call-order test and the goldens pass unchanged.
2. **One pipeline.** `run.execute(ctx, req, pin *Plan)`; `run.apply` is renamed `run.install`. With a pin: after `normalizePlatform`, the request must match the plan (product, current version and build, platform, channel, requested tag, `Force`), else `ErrPlanStale`, before `EventResolvingTarget`; the release comes from `ByTag(pin.TargetVersion)`; `vet` runs with `pin.Latest`; the release ID, the selection (IDs, names, sizes, `Packed`, `ManifestName`, digests when the plan has one) and the operation must equal the plan's; after `checksumFor`, the entry must equal `pin.ManifestSHA256`, else `ErrPlanStale` joined with `ErrIntegrity`. Events are `Run`'s, with `EventFetchingRelease`'s target the request's, as today.
3. **`ErrPlanStale`** in `errors.go`, and `{ErrPlanStale, "plan-stale"}` in `failureClasses` after `latest-older` and before `mutable-release` and `integrity`, so a stale plan that also fails integrity is classed as stale.
4. **`plan.go`:** `Plan` (comparable; asserted by a compile-time `var _ = Plan{} == Plan{}`), `PlanDocument`, `AssetDocument`, `PlanSchema = 1`, `Plan.Document()`, `Plan.Request()`, `ParsePlan(data []byte)` (64 KiB cap, `DisallowUnknownFields`, trailing data refused, schema refused in `releasespec`'s words, and the plan's own validation: product, enums, platform, positive IDs and sizes, asset names, digest syntax), and `(*Updater).Plan(ctx, req, opts...)`: `ErrNotConstructed`, the options, `validateRequest`, `CheckOnly` refused, `normalizePlatform`, `fetchRelease`, `vet`, and for an operation other than `OperationNone` the manifest download within `Limits.Manifest`, `checksumFor` and the `ManifestVerifiers`. No events, no lock, no target.
5. **`(*Updater).Apply(ctx, req, p, opts...)`** in `runoptions.go`: an invalid plan is refused before anything else; then `RunWith`'s scope, the one-run guard (`ErrConcurrentUpdate`) and `execute` with the pin. The confirmation is `Run`'s block unchanged (owner answer 1).
6. **`Release.ID`'s doc**: the Updater compares it between `Plan` and `Apply`, so a source returns the same ID for a release each time.
7. **Tests:** `Plan` then `Apply` installs the planned release; a newer release appearing between them changes nothing (`scriptSource`); a changed release ID, asset ID, digest, `SHA256SUMS` entry or operation is `ErrPlanStale`, each in its own row; a request that does not match is `ErrPlanStale` with no source call; `Apply` asks the `Confirmer` unless `Yes` or `DryRun`, and a decline installs nothing; `Apply`'s events equal `Run`'s for the same release; `Plan` reports no event and downloads no binary; `CheckOnly` is refused; `OperationNone` plans carry no manifest digest and apply to an up-to-date `Result`; `ErrConcurrentUpdate` between `Run` and `Apply`; the document's golden; `ParsePlan` refusals; `FuzzParsePlan` (round trip of `Document`), and the `Makefile`'s fuzz count `-m 5` to `-m 6`; `ExampleUpdater_Plan`. Plants: drop the release-ID compare, the manifest compare, the request match, the schema check, and `pin.Latest`; each fails its row.
8. `TestRunCallOrderFailureBoundaries` and the goldens are unchanged, byte for byte.

### Phase Q15: streams and test support

Files: `selfupdate/stream.go`, `stream_run_test.go`; `selfupdate/selfupdatetest/selfupdatetest.go`, `githubserver.go`, `conformance.go`, their tests.

1. **`StartApply(ctx, u, req, p, opts...) *Stream`**, sharing `Start`'s preparation; a test drives a `ConfirmNeeded` from an `Apply` stream and sees the planned release installed.
2. **`(*GitHubServer).Publish(ReleaseSpec)` and `(*FakeSource).Publish(ReleaseSpec)`**: append the release and make it the latest; a re-published tag gets new IDs. Tests for both.
3. **`VerifyReleaseSource`** also checks that `ByTag` of the latest tag returns `Latest`'s release ID and asset IDs. Its failing table gains a source whose IDs change per call. The migration guide names the check (MADR Consequences).
4. **The end-to-end test** of MADR Confirmation: `NewGitHubServer`, `Plan`, `Publish` of a newer release, then `Apply` installs the planned release and `Run` the newer one.

### Phase Q16: the handoff carries the plan

Files: `selfupdate/service/handoff.go`, new `handoffplan.go` and `handoffplan_test.go`; `selfupdate/cli/run.go`, `doc.go`, `handoff_test.go`, `example_handoff_test.go`.

1. **`service`:** `const EnvHandOffPlan = "SELFUPDATE_HANDOFF_PLAN"`; `HandOffPlanFunc(d Detacher, spec HandOff) func(context.Context, selfupdate.Request, selfupdate.Plan) (bool, string, error)`, `HandOffFunc`'s rules (including `ErrConfirmationRequired` inside the service without `Yes`) with `EnvHandOffPlan=<compact PlanDocument JSON>` added to a copy of `spec.Env`; `LoadHandOffPlan() (selfupdate.Plan, bool, error)`, which reads the variable, unsets it and parses it, and is called after `ReportFunc` or `LoadHandOffEnv`.
2. **`cli.HandOff`:** `DetachPlan`, which replaces `Detach` when set: `cli.Run` calls `u.Plan`, hands off when the plan's operation is not `OperationNone`, and otherwise calls `u.Apply`; a `Plan` error falls back to today's path, so the failure goldens do not change. `Planned`: when it returns a plan, `cli.Run` calls `u.Apply` with it and resolves nothing; a load error fails the run through `Report`.
3. **Tests:** the env entry reaches each backend's carrier (systemd's environment file, launchd's and Task Scheduler's private file, scm's process environment) with the fakes those backends' handoff tests use; `LoadHandOffPlan` unsets the variable; a tampered plan is refused by `ParsePlan` or by `Apply`'s checks; `cli`'s handoff test sees the child apply the parent's plan while a newer release exists; with neither field set, every existing `cli` test and golden passes unchanged. `ExampleHandOff_plan` in `cli`. Plants: drop the env entry; ignore `Planned`; each fails its test.

### Phase Q17: documents

Files: `docs/architecture.md`, `docs/guides/extending-selfupdate.md`, `docs/guides/migrating-from-mcplib-selfupdate.md`, `CHANGELOG.md`, `selfupdate/doc.go`, `selfupdate/cli/doc.go`, `selfupdate/service/doc.go`, `README.md` if it lists the API.

1. `architecture.md`: the Go code table (`plan.go`, `servicetest`, `handoffplan.go`, test counts, the golden count), the API-by-file bullets (plans; `StartApply`; `servicetest`), the handoff bullet, the trust model (a plan pins IDs and digests, and carries no approval), the failure classes (`plan-stale`), the test-clock rules (bubbles), and the depguard count.
2. `extending-selfupdate.md`: "Show an update banner" (plan from the banner's tag), "Read JSON output" (`PlanDocument`), "Drive an update from a TUI" (`Plan`, show, `StartApply`), "Updating from inside the service" (`HandOffPlanFunc`, `LoadHandOffPlan`, `DetachPlan`, `Planned`), "Use another release host" (stable IDs), "Test a program" (`Publish`, `servicetest`), and a new "Check your Lifecycle" under "Run as a service".
3. The migration guide's `## 23. From v1.22 to v1.23`: what is new, behaviour you may notice (F11's fixes; the root-relative install; `VerifyReleaseSource`'s ID check), adopting it, and the check; the guide's version pins move after the tag.
4. `CHANGELOG.md`'s `v1.23.0` entry, Added / Changed / Fixed, linking the guide section and this pair.

### Phase Q18: `v1.23.0`

As Q8, for `v1.23.0`, with `make apicheck` against `v1.22.3` reporting additions only, the `go126` job green, and every live job green on the tag.

### Phase Q19: closing

1. The execution record complete, the Verification below answered with output, and this plan `complete`.
2. Scratch clones, probe jobs and units removed from the hosts; `git status --short` clean but for the owner's commits.

## Verification

* **V1 (records):** the MADR's facts, amended by A1 (P6) and any deviation, match the code at each tag.
* **V2 (bubbles):** no test clock, `pollClock` or production `now`/`sleep` seam remains (`git grep`); the moved tests pass on the three hosts and in `go126`, under `-race`, and each was seen failing on a plant; P1's test fails on the `v1.22.0` file.
* **V3 (seams):** no package-level seam remains in `selfupdate`; `setSeam` is gone; `go test -race -shuffle=on -count=3 ./selfupdate/` passes on the three hosts; each moved seam's test was seen failing.
* **V4 (servicetest):** its tests pass and fail as Q9 lists; the depguard rule refuses a planted import; every backend passes over its fake and its real manager; the five defects of MADR F3, planted, each fail.
* **V5 (F11):** each fix was seen failing first, and its documentation matches the code.
* **V6 (root):** P2's test passes after Q13 and failed before; no path-based call remains on the Unix install path but `checkDir`'s and `revalidateTarget`'s deliberate ones (`git grep` of `os.` in the replace files, reviewed).
* **V7 (plans):** Q14 to Q16's tests pass; `Run`'s call order and goldens are byte-identical; the end-to-end test installs the planned release over a newer one; `FuzzParsePlan` runs in `make gate`.
* **V8 (API):** `make apicheck` against `v1.22.2` for `v1.22.3` reports no change, and against `v1.22.3` for `v1.23.0` additions only; `go doc` shows each new identifier with its doc.
* **V9 (release):** both tags are cut, their CI is green, and the proxy serves each as `@latest` in turn.

## Rollout and Rollback

* **Rollout:** one commit per phase on `main`, pushed on the owner's word; `v1.22.3` after Q8, `v1.23.0` after Q18. A program adopts plans, `servicetest` or the handoff plan by calling them; nothing changes for a program that does not, except the F11 fixes and the root-relative install, which the migration guide lists.
* **Rollback:** before a tag, `git revert` of a phase. After it, fix forward in a patch release. Release 1 is internal, so a revert of any of its phases changes nothing a program sees. In release 2, `Plan`, `Apply`, `servicetest` and the handoff fields are additive and unused until called; the F11 fixes and Q13 are behaviour changes, each in its own commit, so each can be reverted alone.

## Execution record

### Approval (2026-10-27)

The owner, on this plan and the amended MADR: "q5: go with recommendation, proceed". Question 5 is yes (Q10 step 6). The MADR is `accepted` and this plan `in-progress`. A commit still needs the owner's "commit to main" in its turn.

### Q0: records (2026-10-27)

* The proposed pair and its index rows were committed by the owner as `8202473`.
* On approval: the MADR's status `accepted`, its Decision Outcome names the chosen option and records question 5's answer; this plan `in-progress`; both index rows updated. `make check-records` (88 records, 0 findings), `make docs-check`, `make markdownlint` and `--ids` on the three files exit 0.

### Q1: `service` on the production clock (2026-10-27)

Files: `selfupdate/service/poll.go`, `poll_test.go`, `supervised.go`, `supervised_lifecycle.go`, `supervised_lifecycle_test.go`, `supervised_health_test.go`, as listed. `supervised_test.go` and `supervised_linux_test.go` needed no change: neither sets a clock field.

1. **P1 committed** as `TestSupervisedStopRestoresInBubble`, at `DefaultPollTimeout`, `Down` delayed 0 and 30 s. Seen failing on the `v1.22.0` `supervised_lifecycle.go` in a scratch copy, with its three `s.now()` and one `s.sleep(ctx,` calls rewritten to `time.Now()` and `sleepCtx(ctx,` so that it compiles against the clockless struct:

   ```text
   --- FAIL: TestSupervisedStopRestoresInBubble (0.00s)
       supervised_lifecycle_test.go:282: Down for 0s: calls ["down"], want up false; want down, then up with time left
   ```

2. **`PollHealthy`:** `clock`, `realClock` and `pollClock` deleted; `PollHealthy` reads `time.Now`, `time.Until` and `time.After`. The seven `fakeClock` tests run in bubbles and assert elapsed bubble time; `fakeClock` and `useFakeClock` deleted. One test added, `TestPollHealthyProbeContextEndsAtDeadline`: the probe's context ends at the wait's deadline on the clock the wait measures. Plants, each in a scratch copy:

   | Plant (`poll.go`) | Failure |
   | :--- | :--- |
   | settle does not restart on a new instance | `poll_test.go:57: healthy after 2s, want 4s` |
   | the timeout drops the last probe error | `poll_test.go:100: err = … timed out: not ready within 3s …, want the timeout with the probe error` |
   | an ended context returns nil | `poll_test.go:138: err = <nil>, want the context's` |
   | the probe's deadline twice the wait's | `poll_test.go:124: timed out after 6s, want 3s` |
   | the deadline one interval late | `poll_test.go:71: … after 6s, want a timeout at 5s`; `poll_test.go:103: timed out after 4s, want 3s` |

   The first plant was first written so that it left `stableFor` unused, and the copy did not build; it was rewritten to `stableFor == "\x00"`, and the plant runner now counts a build failure as not caught.
3. **`fakeSup` moves on observation:** after `Down`, its process exits `downAfter` of bubble time later (a duration, negative for never), checked when `State`, `Up` or the process table is read.
4. **`Supervised`'s clock:** the `now` and `sleep` fields deleted from the struct and the constructor; `Stop` and `WaitHealthy` call `time.Now` and `sleepCtx`. The `vclock` tests run in bubbles at a 4 s bound and assert exact bubble times (`TestSupervisedStopTimeout` 4 s; `…Kill` 1 s; `…BoundIncludesDown` 3 s and 4 s; `…OutlivesContext` 500 ms); `vclock` deleted. Plants:

   | Plant (`supervised_lifecycle.go`) | Failure |
   | :--- | :--- |
   | `killAt` at twice `KillAfter` | `supervised_lifecycle_test.go:357: killed at 2s, want at KillAfter, 1 s` |
   | the bound counted from after `Down` | `supervised_lifecycle_test.go:306: killed at -2562047h47m16.854775808s, want at once after Down's 3 s …` (never killed) |
   | the wait on the caller's context (0015-MADR B3) | `supervised_lifecycle_test.go:333: Stop: … timed out: demo still runs 4s …` |
   | no restore after a timed-out stop | `:243: calls ["down"], want down then up`; `:282: Down for 0s: calls ["down"] …` |
   | the restart window twice the settle | `supervised_health_test.go:98: restarting longer than the settle window: took 6.1s, want 3.1s` |
   | a restart after up not a failure | `supervised_health_test.go:98: restarting after it was up: took 3.2s, want 100ms` |

5. **`TestSupervisedStopRestoresOnRealClock` deleted**; step 1's test carries its record citations.
6. **`healthOver`'s clock removed:** `TestSupervisedWaitHealthy` and `…Probe` run in bubbles, one per case, at a 100 ms interval, a 3 s settle and the default 60 s bound, and each case asserts how long its wait takes. The file's comment records that this replaces 0035-PLAN Deviation D4's stepping clock.
7. **Runs:** `go test -race -count=3 ./selfupdate/service/` passed on this Mac (59.2 s), the Linux test host (61.4 s) and the Windows test host (87.1 s); `-count=30 -race` of the moved tests passed on this Mac. Cross-target `go vet ./selfupdate/service/...` clean for linux, darwin and windows. `make pre-add-check` on the six files: `6 file(s) clean (gofmt, golangci-lint, go vet, go test, govulncheck)`.
8. **`make gate`**: every step rc=0, `apicheck` "compatible with v1.22.2", `overall=0`.

### Q2: scm on the production clock (2026-10-27)

Files: `selfupdate/service/scm/service.go`, `lifecycle.go`, `fake_test.go`, `scm_test.go`, `stopwait_test.go`, `detach_test.go`, `unquoted_test.go`, `alias_windows_test.go`, `compose_windows_test.go`, as listed.

1. **The fake moves on observation.** `fakeSCM.tick` is replaced by `observe`: a status read made at a later bubble time than the fake last saw moves every service one entry on, and a control or a start stamps the time, so the first read after it does not. That is one entry per `waitPending` sleep, as the virtual clock gave.
2. **`virtualClock` deleted; `Service.now` and `sleep` deleted.** `waitPending` calls `time.Now`, `time.Since` and `sleepCtx`. `testService` returns the `*Service` alone, and no longer sets `Poll.Interval` (1 ms) or `Poll.Timeout` (2 s): its services take the defaults. Every test that calls it runs in a bubble (a scratchpad script wrapped 24 bodies, `CheckNoLeak` left outside), except `TestDetach`.
3. **`TestStopWaits`** passes at the 60 s default bound, and its comment records that the 2 s fixture bound forbade the 5 s wait it asserted (MADR F4). `TestStopStalledCheckpoint` uses the default bound in place of an hour.
4. **`TestDetach`** stays on the real clock: it starts a real hop and waits for a file the run writes.
5. **One test added,** `TestStopTimesOutAtBound`: a stop whose checkpoint keeps advancing ends with `ErrTimeout` and `context.DeadlineExceeded` at exactly the 60 s default.
6. Plants, each in a scratch copy:

   | Plant | Failure |
   | :--- | :--- |
   | the wait hint clamped up to 2 s | `scm_test.go:201: waited 10s, want 5s`; `scm_test.go:226: hint 0 ms: slept 2s, want 1s` |
   | the default stop bound doubled | `stopwait_test.go:62: timed out after 2m0s, want 1m0s` |
   | the stop wait on the caller's context (0015-MADR B3) | `stopwait_test.go:36: Stop = … timed out: demo STOP_PENDING: context canceled` |
   | the old 2 s fixture bound (`fake_test.go`) | `scm_test.go:196: … timed out: demo STOP_PENDING: context deadline exceeded` |
   | the fake moving twice per observation | `scm_test.go:201: waited 3s, want 5s` |
   | a stall declared at twice its window | `scm_test.go:248: declared the hang after 21s` |

   A first stall plant doubled `stallFloor` itself, which the test reads, so the test moved with it and passed; the plant was moved to the stall comparison, which `waitPending` now writes with `time.Since`.
7. **Runs:** `go test -race -count=20 ./selfupdate/service/scm/` on this Mac; `go test -race -count=3 -v` on the Windows test host, the Windows-only tests included, 81 passes and the live tests skipped. Cross-target `go vet` clean. `make pre-add-check` on the nine files: `9 file(s) clean`.
8. **`make gate`**: every step rc=0, `apicheck` "compatible with v1.22.2", `overall=0`.

### Q3: Task Scheduler on the production clock (2026-10-27)

Files: `selfupdate/service/taskscheduler/task.go`, `lifecycle.go`, `fake_test.go`, `lifecycle_test.go`, `enable_test.go`, `handoff_test.go`, `reconcile_test.go`, `register_test.go`, `status_test.go`, `detach_test.go`, as listed, and `handoff.go` (Deviation D1).

1. **The fake moves on observation.** `fakeSched.tick` is replaced by `observe`, called by the reads a wait makes (`running`, `state`, `instances`, `lastRun`): a read later than the fake last saw counts down each task asked to exit. A run or a stop stamps the time. `exitIn` negative means never, which replaces `1<<30`. The fake's own `now` is gone; a run's time is the bubble's.
2. **`virtualClock` deleted; `Task.now` and `sleep` deleted.** `waitGone` and the one-shot handoff's wait call `time.Now` and `sleepCtx`. `testTask` returns the `*Task` alone and no longer shortens `Poll.Interval` (1 ms) or `Poll.Timeout` (2 s). Every test that calls `testTask` or `oneShotTask` runs in a bubble: 49 bodies, wrapped by a scratchpad script that leaves the lines of a multi-line raw string as they were (the raw strings of every edited file were compared before and after: identical). `TestDetachOneShotRefusesBatchFile`, in `handoff_windows_test.go`, was wrapped and then restored: it refuses before any wait, and the file is not in this phase's list.
3. **`TestStopTimeoutOnRealClock` is `TestStopTimeoutAtDefaultBound`:** the default `Poll.Timeout`, a 1 s grace, and an exact 60 s assertion.
4. **`TestDetachOneShotFailures`** uses the default bound, and its never-starts case asserts that the wait gives up at exactly 60 s.
5. Plants, each in a scratch copy:

   | Plant | Failure |
   | :--- | :--- |
   | the final wait not bounded by the stop's deadline (the grace outside the bound) | `lifecycle_test.go:232: timed out after 1m1s, want 1m0s` |
   | a timeout without the still-running error (0037-MADR D5-A) | `lifecycle_test.go:229: Stop = <nil>, want ErrTimeout saying the task still runs` |
   | the one-shot handoff waiting twice the bound | `handoff_test.go:171: never starts: gave up after 2m0s, want 1m0s` |
   | the grace skipped | `lifecycle_test.go:179: calls ["enable false" "stop"], want ["enable false"] …` |
   | the fake's countdown never running | `lifecycle_test.go:179: calls ["enable false" "stop"], want ["enable false"] …` |

6. **Runs:** `go test -race -count=20 ./selfupdate/service/taskscheduler/` on this Mac; no test of the package or of scm takes 0.1 s or more (`go test -json`). `go test -race -count=3 -v` on the Windows test host, the COM tests included: 216 passes, the live tests skipped. Cross-target `go vet` clean. `make pre-add-check` on the twelve files then changed, `handoff_windows_test.go` among them before its restore: `12 file(s) clean`.

### Deviation D1 (2026-10-27): `handoff.go` calls the deleted `Task.sleep`

* **Found:** Q3 step 2 deletes `Task.sleep`, and `handoff.go:140`, the one-shot handoff's start wait, called it. The file is not in Q3's list.
* **Decision:** the owner, 2026-10-27, chose "Add handoff.go to Q3": the change is the mechanical consequence of the step, and behaviour is unchanged, since `sleepCtx` was the field's only production value. `TestDetachOneShotFailures` covers the line (Q3 step 4 and its plant). No MADR change: no decision or fact moves.
* **Files added to Q3:** `selfupdate/service/taskscheduler/handoff.go`, one line: `t.sleep(ctx, t.interval())` becomes `sleepCtx(ctx, t.interval())`.

Q3, continued: `go test -race -count=3` of the package passed again after the restore, and `make gate`: every step rc=0, `apicheck` "compatible with v1.22.2", `overall=0`.
