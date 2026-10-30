---
status: in-progress
date: 2026-10-29
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
2. New helper modes where no live test arranges the state today (MADR Decision Outcome): systemd `SlowStop` (a TERM delay) and `Crashing`; launchd `Stubborn` (`FAKE_IGNORE_TERM`, with an `ExitTimeOut` launchd honours as no kill, or `ErrUnsupported` if ~~P6~~ P7 shows none: it does, Deviation D9) and `Crashing`; scm a handler that ignores stop and one that stops slowly with checkpoints, with cleanup that ends the process before `removeService`; Task Scheduler a definition without the minute watchdog trigger, and `Stubborn` and `SlowStop` returning `ErrUnsupported`. s6 `Stubborn` through a run script that ignores TERM.
3. Tests: `TestLiveConformance` (systemd, launchd, scm) and `TestLiveTaskConformance`, with ~~`RequireAll`~~ the exact list of skipped checks, and `RequireAll` where that list is empty (Deviation D7), and a short `Poll.Timeout` whose bound is passed as `StopBound`; for s6, a `conformance` subtest of `TestLiveSupervisedS6`, run through a hook (Deviation D8). The names match CI's selectors, so `ci.yml` does not change.
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

### Q4: systemd and launchd in bubbles (2026-10-27)

Files: `selfupdate/service/systemd/systemd_test.go`, `stopwait_test.go`, `fake_test.go`; `selfupdate/service/launchd/launchd_test.go`, `fake_test.go`, as listed, and `selfupdate/service/launchd/stopwait_test.go` (Deviation D2). No production file changed.

1. **`testUnit` and `testJob` keep their real-clock defaults** (1 ns and 1 ms intervals, a 2 s timeout): eight other systemd test files and five other launchd ones use them for tests that do not wait, outside this phase's files. Each package's `fake_test.go` gains `bubblePoll`, the documented defaults with no settle window, which every test moved into a bubble passes.
2. **systemd, in bubbles:** `TestStopWaitsForInactive`, `TestStopStalls`, `TestStartThenWaitHealthy`, `TestWaitHealthyNeedsNewInvocation`, `TestStopFinishesAfterCancel`, `TestStopBoundsAJobThatNeverEnds`, and `TestWaitHealthyFailsFast` with one bubble per subtest. `TestStopStalls` and `TestWaitHealthyNeedsNewInvocation` move from a 1 ns interval to `bubblePoll` (MADR F9), and assert their timeouts exactly: the stop's bound, from `stopBound`, and 60 s. `TestStopBoundsAJobThatNeverEnds` runs at the 60 s default, its watchdog at twice the bound, and asserts the bound exactly. `TestCapturedShowOutput` does not wait, and stays as it is.
3. **launchd, in bubbles:** `TestStopWaitsUntilGone`, `TestStopTimesOut` (exactly 60 s), `TestStartBootstrapsAndRetries` (exactly `3*bootstrapFirst`, the two retries' backoff), `TestStartBootstrapHangs` (exactly 60 s), `TestWaitHealthy` (the same-PID timeout exactly 60 s), `TestWaitHealthySettlesAtLeastThrottle`, `TestStopFinishesAfterCancel`, `TestManagedStopTimeoutRestartsJob` and `TestStopWaitFollowsExitTimeOut`. `TestCapturedOutput`, `TestProbeUsesListWhenItSees`, `TestLoaded` and `TestStatus` do not wait, and stay as they are. The fake's `pidAlive` stays a real `kill(pid, 0)` on a PID no process has; it returns at once, so it does not hold the bubble.
4. Plants, each in a scratch copy:

   | Plant | Failure |
   | :--- | :--- |
   | systemd's stop without `--no-block` (0037-MADR F2) | `stopwait_test.go:72: Stop still blocked after twice its bound on a stop job that never ends …` |
   | systemd's stop wait at twice its bound | `stopwait_test.go:72: …`; `systemd_test.go:197: timed out after 2m0s, want the stop's bound, 1m0s` |
   | launchd's bootout on the caller's context (0037-MADR F3) | `stopwait_test.go:44: verbs ["managername" "list" "list" "bootout"], loaded true: Stop returned while the job was still loaded` |
   | a bootstrap the bound ends not `ErrTimeout` (0037-MADR F4) | `launchd_test.go:421: Start = context deadline exceeded, want ErrTimeout wrapping the deadline` |
   | the bootstrap backoff tripled | `launchd_test.go:393: started after 400ms, want 300ms` |

5. **Runs:** `go test -race -count=20` of both packages on this Mac, which runs launchd's darwin-only tests; `-race -count=3` of both on the Linux test host, which runs systemd's Linux-only tests. Cross-target `go vet` clean. `make pre-add-check` on the six files: `6 file(s) clean`.

### Deviation D2 (2026-10-27): launchd's stop tests live in `stopwait_test.go`

* **Found:** Q4 step 2 moves launchd's stop tests into bubbles, and step 3 plants 0037-MADR F3, whose test, `TestStopFinishesAfterCancel`, is in `selfupdate/service/launchd/stopwait_test.go`, with `TestManagedStopTimeoutRestartsJob` and `TestStopWaitFollowsExitTimeOut`. Q4's list names `launchd_test.go`, `fake_test.go` and "`lifecycle_test.go` if present", which is not; it omits `stopwait_test.go`.
* **Decision:** the owner, 2026-10-27, chose "Add stopwait_test.go to Q4": the file holds the stop tests the step moves and the test its F3 plant needs. Test-only; no MADR change.
* **Files added to Q4:** `selfupdate/service/launchd/stopwait_test.go`: the three tests move into bubbles with `bubblePoll`; `TestStopWaitFollowsExitTimeOut` keeps its 1 ms poll timeout, which is its subject, with the default interval.

Q4, continued: `make gate`: every step rc=0, `apicheck` "compatible with v1.22.2", `overall=0`.

### Q5: `selfupdate` seams, the scaffold (2026-10-27)

Files: `selfupdate/replace.go`, `replace_unix.go`, `replace_windows.go`, `journal.go`, `leftovers.go`, `lock.go`, `lock_unix.go`, `lock_windows.go`, `session.go`, `standalone.go`, `cleanup_other.go`, `cleanup_windows.go`, from the list; `journal_other.go`, `journal_windows.go`, `cleanup.go` and `close.go` needed no change. Step 4's test files: `branch_coverage_test.go`, `cleanup_busy_windows_test.go`, `cleanup_windows_test.go`, `journal_test.go`, `lifecycle_windows_test.go`, `modepolicy_test.go`, `replace_test.go`, `replace_windows_budget_test.go`, `replace_windows_test.go`.

1. **`fsOps`** in `replace.go`: one function field per filesystem seam, each named for the package variable it replaces (`fileChmod`, `fileSync`, `fileClose`, `osChmod`, `osRemove`, `osLink`, `syncDirFn`, `syncRootFn`, `replacePath`, `writeJournalFn`, `keptRenameFn`, `leftoverRemove`, `lockOpenHook`, `afterLockHook`), and an embedded build-tagged `osFS`: `osLchown` on Unix, `moveFileExFn` on Windows. A field and a method of one type cannot share a name, so the fields keep the variables' names and the methods keep the functions'.
2. **`realFS()`** returns a new value whose fields call the package variables at call time, so every `setSeam` still works, including one set after a session began. `replacePath` and `writeJournalFn` lose their initializers and are set in an `init`: their defaults reach `realFS` (`replacePathOS` on Windows calls `realFS().moveFileReplace`), whose fields read them, which Go refuses as an initialization cycle in a declaration. The `init` is commented as temporary, until Q6 moves both.
3. **`StandaloneInstaller.fs`** and `ops()`, which is `realFS()` when `fs` is nil, so a zero `StandaloneInstaller` still works. `beginSession` is an `*fsOps` method, and stores its receiver in the new `installSession.fs`.
4. **Methods:** `copyFile`, `backupFile`, `chmodStaging`, `chownStaging`, `replaceTarget`, `keepAsPending`, `commitReplacement`, `rollbackReplacement`, `writeJournal`, `removeJournal`, `recoverJournal`, `keepInterrupted`, `removeLeftovers`, `openLockFile`, `acquireLock`, `tryAcquireLock`, `clearSpecialBits`, `beginSession`, `processCleanupReceipt`, and on Windows `moveFileReplace`, `writeCleanupReceipt` and `writeCleanupEntries`. A scratchpad script converted them and rewrote every call inside a converted method or a session method to go through the receiver or `s.fs`, then asserted that no such body still calls a seam or a converted function unqualified. 31 direct calls in nine test files became `realFS().name(…)`.
5. **No behaviour change:** the test lists and counts match a copy of HEAD: `go test -list` on darwin, 449 tests both; and `-race -v` runs, HEAD and now, 833 passes and 1 skip on this Mac and on the Linux test host, 841 passes and 11 skips on the Windows test host. Cross-target `go build` and `go vet` clean.
6. **The wiring, seen failing:** each plant breaks one `realFS` field's call through its package variable, in a scratch copy, and the tests that set that seam fail:

   | Plant | Failing tests |
   | :--- | :--- |
   | `replacePath` calls `replacePathOS` | `TestRollbackReplacementFailures`, `TestInstallersReportRolledBack`, `TestInjectedRenameFailure` |
   | `lockOpenHook` does nothing | `TestOpenLockFileRefusesReplacedLock`, `TestOpenLockFileGivesUpWhenLockKeepsChanging` |
   | `afterLockHook` never runs | `TestBeginChecksDirectoryBeforeReceipt` |
   | `writeJournalFn` writes the real journal | `TestJournalWriteFailure` |
   | `syncDirFn` calls `syncDirectory` | `TestCommitReportsDirectorySyncFailure`, `TestRollbackReplacementFailures`, `TestInstallSyncAndRollbackFailureReported`, `TestManagedApplyFailureRetriesRollback` |

7. `make pre-add-check` on the 21 files: `21 file(s) clean`.
8. **`make gate`**: every step rc=0, `apicheck` "compatible with v1.22.2", `overall=0`.

### Q6: `selfupdate` seams, the filesystem (2026-10-28)

Files: Q5's `replace.go`, `replace_unix.go`, `replace_windows.go`, `journal.go`, `leftovers.go`, `lock.go`, `session.go`; and the test files that set a seam: `branch_coverage_test.go`, `events_test.go`, `fs_test.go`, `install_hardening_test.go`, `install_regression_test.go`, `journal_test.go`, `managed_regression_test.go`, `modebits_unix_test.go`, `modepolicy_test.go`, `modepolicy_unix_test.go`, `probe_test.go`, `recovery_test.go`, `replace_windows_budget_test.go`, `replace_windows_test.go`.

1. **Test helpers**, beside `setSeam` in `install_hardening_test.go`: `fsOf(t, sess)`, the effects a session uses (through a managed session to its standalone one), and `fsFor(inst)`, which gives an installer effects of its own. An installer with none hands each session a fresh `realFS()`, so a test that changes one session's effects changes nothing else, and restores nothing.
2. **One step per seam, in the plan's order**, each built for the three targets, vetted and tested before the next: `osRename` (deleted; `replacePathOS` calls `os.Rename`), `afterLockHook`, `lockOpenHook`, `leftoverRemove`, `keptRenameFn`, `writeJournalFn`, the `fileChmod`/`fileSync`/`fileClose` trio, `osLink`, `osLchown`, `osChmod`, `osRemove`, `syncRootFn`, `syncDirFn`, `replacePath`, `moveFileExFn`. In each, its tests set the field on `fsOf` or `fsFor`, or on a `realFS()` they call directly; `realFS()`'s field became the real function; the package variable was deleted. `evalSymlinks` moved to Q7 (Deviation D4).
3. **Helpers that set seams took the effects as a parameter:** `failRestore(t, fx)` and `swapAfterFirstReplace(t, fx, dir, readOnly)`; `unixInstall` takes optional edits to its installer's effects; `plantRecovery`'s `failRename` is set by its caller on the installer. `TestKeptBackupSurvivesLaterSessions` no longer puts seams back: its later sessions have their own effects.
4. **The scaffold's `init` is gone.** `replacePathOS` is an `*fsOps` method on both systems (on Windows it calls the value's own `moveFileReplace`), and `realFS` binds `replacePath` and `writeJournalFn` to the value's own methods, so they see its other fields. No `selfupdate` package variable holds a filesystem seam.
5. Plants, one per seam, each bypassing the field at the line that consults it, in a scratch copy (`moveFileExFn` on the Windows test host):

   | Seam | Plant | Failure |
   | :--- | :--- | :--- |
   | `afterLockHook` | not called | `recovery_test.go:264: Begin accepted a swapped directory` |
   | `lockOpenHook` | not called after the Lstat | `branch_coverage_test.go:96: err = <nil>, want one containing "lock changed while opening"` |
   | `leftoverRemove` | `root.Remove` | `modepolicy_test.go:124: CleanupPending = <nil> after 0 removal attempts` |
   | `keptRenameFn` | `root.Rename` | `journal_test.go:438: journal false (want true), backup false (want true), kept 1 (want 0)` |
   | `writeJournalFn` | the session's `writeJournal` | `journal_test.go:279: Applied=true err=<nil>; want the journal failure` |
   | `fileChmod`, `fileSync`, `fileClose` | the `*os.File` methods | `branch_coverage_test.go:274: err = <nil>, want the injected chmod failure` (and sync, close) |
   | `osLink` | the copy fallback's mode dropped (the copy runs only when the injected link failure is consulted) | `install_hardening_test.go:215: mode after rollback -rw-------, want -rwxr-xr-x` |
   | `osLchown` | `os.Lchown` | `modepolicy_unix_test.go:91: chown to 0:0, want 503:20` |
   | `osChmod` | `os.Chmod` | `install_hardening_test.go:372: err = <nil>` |
   | `osRemove` | `os.Remove` | `branch_coverage_test.go:198: err = <nil>, want one containing "remove backup"` |
   | `syncRootFn` | `syncRoot` in the rollback | none, then `install_hardening_test.go:557: err = … concurrent update, want the rollback's failed sync reported` after Deviation D3 |
   | `syncDirFn` | `syncDirectory` after the rename | `install_hardening_test.go:375: applied = true` |
   | `replacePath` | the session's `replacePathOS` | `fs_test.go:48: expected injected rename failure`; `install_hardening_test.go:372: err = <nil>` |
   | `moveFileExFn` | `windows.MoveFileEx` | all four `TestMoveFileReplace…` tests, e.g. `replace_windows_test.go:46: err=The system cannot find the path specified. calls=0` |

   The first `osLink` plant left a variable unused and did not build; it was rewritten as above. A scratchpad script stopped twice partway (a line-numbered match that had moved, and an installer line that occurs twice in `fs_test.go`); each was finished by a script matching text, and the step's checks ran after.
6. **Runs:** `go test -race -v ./selfupdate/`: 833 passes and 1 skip on the Linux test host, as before; 841 passes and 11 skips on the Windows test host, as before. `make pre-add-check` on the 21 files: `21 file(s) clean`.
7. **`make gate`**: every step rc=0, `apicheck` "compatible with v1.22.2", `overall=0`.

### Deviation D3 (2026-10-28): `syncRootFn`'s only test did not see it

* **Found:** the `syncRootFn` plant was not caught. `TestInstallReportsRestoreAfterSyncFailure`'s "rolled back in the locked directory, unsynced" case fails the rollback's directory sync, but asserted nothing that depends on it: with the rollback's sync bypassing the seam it still passed, on this tree and on HEAD with the package seam (`q6headcheck.py`). The gap predates this plan.
* **Decision:** the owner, 2026-10-28, chose "Strengthen the subtest": it now requires `errors.Is(err, errRestoredUnsynced)`. Seen passing three times on the code and failing on the plant.
* **Files:** `install_hardening_test.go`, already in Q6.

### Deviation D4 (2026-10-28): `evalSymlinks` is in a Q7 file

* **Found:** Q6's first step deletes the two seams no test sets, `osRename` and `evalSymlinks`; `evalSymlinks` is in `selfupdate/target.go`, which is Q7's.
* **Decision:** the owner, 2026-10-28, chose "Move it to Q7 step 1", with the rest of target resolution. Q6 stays within its files.

### Q7: `selfupdate` seams, the rest, and parallel tests (2026-10-28)

Files: from the list, `selfupdate/target.go`, `github.go`, `credentials.go`, `confirmer.go`, `updater.go`, `version.go`, `checker.go`, `githubupdater.go`, `export_test.go`, `install_hardening_test.go`; `replace.go`, `session.go`, `standalone.go`, `types.go` and `runoptions.go` (Deviation D5); and the test files that used `withTempHome`, `noEnv`, `cacheEnv`, `progressRun`, `ttyPipe`, `SetRunningPlatform` or the seams they set. In all, 80 of the package's 88 test files changed, most for the `tempTarget` rename and step 7's `t.Parallel()`; `export_test.go` and `install_hardening_test.go` are among them.

1. **Target resolution.** `osExecutable` and `userHomeDir` are `fsOps` fields, the installer's effects, and `resolveTarget`, `rawExecutablePath`, `allowedRoots` and `revalidateTarget` are `*fsOps` methods, reached through `StandaloneInstaller.ops()` and `beginSession`. `evalSymlinks` is deleted; its two callers call `filepath.EvalSymlinks` (Deviation D4). In the tests, `withTempHome` is `tempTarget`, which makes the directory and touches no state, and the 89 policies that named its target are `targetPolicy(exe)` or `specialPolicy(exe)`, which allow that directory as a root. The tests about the home root itself (`TestResolveTarget…`, `TestHomeUnavailableRoot`) set `userHomeDir` on their own `fsOps` (`homeFS`). On Windows a temporary directory is under the real home, so only those tests may rely on a home of their own; they do. `TestCheckerNeedsNoInstaller` loses its two seam lines: a Checker resolves no target.
2. **`timeNow` deleted.** `clockNow`'s default, `GitHubSource.now`'s and `parseRateLimit`'s are `time.Now`. The check-cache tests set the Checker's `clock` (or pass `CheckerConfig.Clock`); `progressRun` and `progressUpdater` pass `Config.Clock`. An `env.src.now` line first added to `TestGitHubSecondaryRateLimitWithoutHeaders` was removed again: that test's limit carries no time, and a plant showed the line had no effect.
3. **`lookupEnv` deleted.** `NewGitHubSource` calls `gitHubSourceFrom(opts, os.Getenv)`, and `EnvCredential` calls `envCredentialFrom(os.Getenv, …)`. The test helpers that build sources (`credServer.source`, `newGitHubEnv`, `hardeningSource`) pass `lookupIn(env)`, a map, so no test reads the real environment; the 20 `noEnv(t, nil)` calls went with `noEnv`, and the two tests that set variables pass a map.
4. **`isTerminal` deleted.** `NewTerminalConfirmer` calls `terminalConfirmerFrom(in, out, term.IsTerminal)`; the tests that stood a pipe in for a terminal pass `alwaysTTY`.
5. **`runningPlatform` deleted.** `Updater` and `Checker` hold `platform`, set by `New` and `NewChecker`, and copied by `Updater.Checker` and `run.checker`; `runningPlatform()` methods fall back to `runtimePlatform()` when it is zero. `validateRequest` and `normalizePlatform` take the platform. `SetRunningPlatform(u, p)` sets it on one Updater; `e2e_github_test.go`, `golden_test.go` and `stream_run_test.go` call it after `New`. A first name for the methods, `running`, collided with the Updater's one-run guard field and was renamed.
6. **`setSeam` deleted.** No package-level function variable remains in `selfupdate`'s non-test files (a scan of every unexported package variable).
7. **Parallel.** 402 top-level tests call `t.Parallel()` first. 45 stay sequential: those that call `CheckNoLeak`, `t.Setenv` or `t.Chdir`, directly or through `e2eUpdater`, `e2eUpdaterWithEnvToken` or `newE2E`. Go runs the sequential tests before it releases the parallel ones, so `CheckNoLeak`'s process-wide count sees no parallel test's goroutines.
8. **Names.** The plan names the unexported constructors `newGitHubSource` and `newTerminalConfirmer`; the repository's revive rule `confusing-naming` refuses a name that differs from an exported one only by case, so they are `gitHubSourceFrom` and `terminalConfirmerFrom`, as `envCredentialFrom`.
9. Plants, each in a scratch copy:

   | Seam | Plant | Failure |
   | :--- | :--- | :--- |
   | `userHomeDir` | `os.UserHomeDir` in `allowedRoots` | `target_test.go:17: … TestResolveTargetHomeRoot…/demo is outside allowed roots` |
   | `osExecutable` | `os.Executable` | `target_test.go:140: … selfupdate.test is outside allowed roots` |
   | the Checker's and Updater's `Clock` | `clockNow` ignores it | `checkcache_test.go:70`, `:120`, `:140`, `:376` |
   | the source's clock | `parseRateLimit(resp, time.Now)` | `github_test.go:307: … reset 0001-01-01T00:00:00Z` |
   | the token's environment | `os.Getenv` | `github_test.go:137: token = "", want GH_TOKEN`; `credentials_test.go:155` |
   | the credential's environment | `os.Getenv` | `credentials_test.go:374: … err=selfupdate: no credential` |
   | the terminal check | `term.IsTerminal` | `confirmer_read_test.go:44: … pass --yes to apply without a TTY …` |
   | the running platform | `runtimePlatform()` in `execute` | `golden_test.go:121: … cannot apply a linux/amd64 release on darwin/arm64 …`; `e2e_github_test.go:77` |

   The source-clock plant was first run against `TestGitHubSecondaryRateLimitWithoutHeaders`, which never reads that clock, and passed; it fails the two tests that set `src.now`.
10. **Runs:** after step 1, `go test -v ./selfupdate/` on the Windows test host, 841 passes and 11 skips as before. After step 7, `go test -race -count=3 -shuffle=on ./selfupdate/` on this Mac (66 s), the Linux test host (87 s) and the Windows test host (58 s); the stray-`.git` run of `./selfupdate/... ./buildinfo/ ./internal/...` clean. `make pre-add-check` on the 93 files: `93 file(s) clean`.
11. **After the renames**: `go test -race -shuffle=on ./selfupdate/` on the Windows test host passed; `make gate`: every step rc=0, `apicheck` "compatible with v1.22.2", `overall=0`.

### Deviation D5 (2026-10-28): Q7's list omits five production files

* **Found:** step 1's target resolution is reached only through the installer and the session, so its lookups touched `replace.go` (the `fsOps` fields), `session.go` (`beginSession`'s re-check) and `standalone.go` (three `resolveTarget` calls). Those changes were made, and each step built and passed, before the omission was seen; the owner was told. Step 5 needs `types.go` (the `Updater` struct) and `runoptions.go` (`run.checker`). Test files that set these seams directly, rather than through the named helpers (`checker_test.go`, `checkcache_outcome_test.go`, `github_test.go`), were taken as within the list's intent.
* **Decision:** the owner, 2026-10-28, chose "Add all five to Q7". No design change; no MADR change.

### Q8: `v1.22.3` (2026-10-28)

1. **The pre-tag change:** a `## v1.22.3` entry in `CHANGELOG.md` (Changed: the module's own tests only; no exported identifier and no behaviour a program can see), and a "From v1.22.2 to v1.22.3" section in the migration guide's §22, before its Check. The version pins move in the pin commit after the tag, as `v1.22.2`'s did. `make gate`: every step rc=0, `apicheck` "compatible with v1.22.2", `overall=0`; `make markdownlint`, `make docs-check` (846 links, 0 broken), `make check-records` (88 records, 0 findings) and `--ids` on the two files exit 0. The pre-push disclosure guard over the eight unpushed commits, `8202473` to `0e8536b`, exits 0 with no finding.
2. **The push and CI on `main`:** the owner committed the pre-tag change as `0620e96` and pushed `main`. CI run 40725764533 completed `success`, all 18 jobs; the Docs run 40725764458 passed.
3. **The tag:** `scripts/check-release-tag.sh v1.22.3` exited 0, and the disclosure guard passed with `main` level with `origin`. The owner made the annotated tag and pushed it: tag object `d3410cc…`, on `0620e96`.
4. **The tag's CI:** run 40728965923 completed `success`, all 18 jobs; its identity legs print `v1.22.3 (release) 0620e96ba7eb`, 10 times.
5. **The pin commit, this change:** `git diff --stat v1.22.2 v1.22.3 -- .github internal/cmd scripts go.mod go.sum` is empty, so the pins move with nothing else. Moved to `v1.22.3` and `0620e96…`: `README.md`'s current release, `go get` line and publish pin; `docs/architecture.md`'s current release and commit; `building-releases.md`'s "examples below pin", three workflow pins and `git ls-remote` example; in the migration guide, the 18 `go get` and 16 `go list -m` lines, "is the current release", §3's workflow pin, "the example pins" and `git ls-remote`, §5's "Require go-selfupdate-lib", and §2's release list, which gains `v1.22.3` and links its section. The lines that name `v1.22.2` as history stay. `make markdownlint`, `make docs-check` (847 links, 0 broken) and `--ids` on the four files exit 0.
6. **The proxy:** `go list -m -json github.com/maccavelli/go-selfupdate-lib@v1.22.3` gives `Hash 0620e96ba7eb47d45cc94b474dc2db87d029ce40`, `Ref refs/tags/v1.22.3`, and `@latest` resolves to `v1.22.3`.

Release 1 is out. Q9 starts release 2 on the owner's word.

### Q9: `servicetest` (2026-10-28)

Files, as listed: new `selfupdate/service/servicetest/doc.go`, `lifecycle.go`, `runner.go`, `fake.go`, `lifecycle_test.go`, `runner_test.go`, `fake_test.go`, `example_test.go`; changed `.golangci.yml`, AGENTS.md, `docs/architecture.md`, `selfupdate/service/doc.go`.

1. **The API**, as the plan's step 1 names it: `State` (with `String`), `Arrangement` and `Observation` with JSON tags, `Lab`, `Method` and its four constants, `LifecycleCase`, `VerifyLifecycle`, `RunnerCase`, `VerifyRunner`, and `FakeService` with `FakeOptions`, `NewFakeService`, and `StopBound` and `StartBound` methods that a case reads its bounds from. `VerifyLifecycle` refuses a nil lifecycle, a nil lab, an empty product and a zero `StopBound` before any check, and leaves the service `Absent`.
2. **The checks**, in the MADR's order: absent, ended context (`Stop` and `WaitHealthy` always held to it; `Installed`, `Running`, `Enabled` and `Start` unless `ContextFree` names them), enabled (both arrangements, against the lab), cycle, stubborn, cancel during stop, crash, start hangs. A lab's error wrapping `errors.ErrUnsupported` skips the check through `Logf`, and `RequireAll` makes it a violation. Two choices beyond the plan's text:
   * a violation shows a lifecycle's or a lab's error as text, never wraps it, so a lifecycle error that wraps `errors.ErrUnsupported` cannot turn a violation into a skip; only an arrange error, the lab's own word, can skip;
   * an arrange that fails after a check has found violations reports the violations, so a skip never hides them.
3. **`FakeService`** keeps its whole stop in an unexported `issueStop(caller, wait, product)`, which `Stop` calls once it has checked its context; the tests build their broken lifecycles over it without a defect switch in the exported type.
4. **Tests**, in bubbles: `TestVerifyLifecyclePasses` (the fake, no check skipped); `TestVerifyLifecycleFails`, 13 broken lifecycles, one rule each, each failing with its rule named; `TestVerifyLifecycleContextFree`; `TestVerifyLifecycleSkips` (a lab that cannot arrange `Stubborn`, and no `StartBound`: two skips, and two violations with `RequireAll`); `TestVerifyLifecycleRefusesItsCase`; `TestVerifyLifecycleTakesNoWallTime` (over two minutes of bubble time); `TestVerifyRunnerPasses`, `TestVerifyRunnerFails` (ignores its context; records a call on an ended context; a blocking run that ignores its cancellation), `TestVerifyRunnerRefusesItsCase`; `TestFakeServiceStates`, `TestFakeServiceSlowStop`; and `ExampleVerifyLifecycle`, on the real clock with short bounds. Two test faults found and fixed on the way: the "Stop ignores its context" row's expected message (the check named a different, also correct, violation), and the runner fake's `exited` channel, first made outside the bubble, where a receive does not count as durably blocked, so bubble time stood still and the run timed out after ten minutes.
5. **Seen failing**, each in a scratch copy:

   | Plant | Failure |
   | :--- | :--- |
   | each of the eight checks disabled (`return nil` first) | `lifecycle_test.go:217: err = <nil>` in its rows |
   | `RequireAll` ignored | `lifecycle_test.go:278: RequireAll: err = <nil>; want both skips as violations` |
   | `ContextFree` ignored | `lifecycle_test.go:240: servicetest: ended context: Installed on an ended context returned no error` |
   | `VerifyRunner`'s cancelled-context check disabled | `runner_test.go:98: err = servicetest: Run past its deadline = no error; …` (the deadline check still caught the ignoring runner) |
   | its recorded-calls check disabled | `runner_test.go:98: err = <nil>` |
   | its blocking check disabled | `runner_test.go:98: err = <nil>` |
   | an `internal/winproc` import planted in `doc.go` | `import '…/internal/winproc' is not allowed from list 'service-servicetest' (depguard)` |

6. **depguard:** the `service-servicetest` rule (the standard library, `selfupdate`, `service`), and the package excluded from `other-packages`. **AGENTS.md:** the scope paragraph names the package, and the depguard list the rule. **architecture.md:** the tree, the package table's row (4 and 4 files), and the rule count, 22. The page said 20 and left `internal-procfs` out of its list before this change, though `.golangci.yml` had 21 rules; both are corrected here. **`service/doc.go`** names the subpackage.
7. **Runs:** `go test -race -count=20 ./selfupdate/service/servicetest/` on this Mac; golangci-lint for linux, darwin and windows clean; `go test -race -count=5` of the package on the Linux and Windows test hosts; `make pre-add-check` on the nine Go files: `9 file(s) clean`; `make markdownlint`, `make docs-check` (847 links, 0 broken; 20 architecture rows, 0 differ) and `--ids` on the changed files exit 0.
8. **`make gate`**: every step rc=0, `apicheck` "compatible with v1.22.3", `overall=0`.

### Q10: the backends do what they document (2026-10-28)

Files: from the list, `selfupdate/service/scm/lifecycle.go`, `launchd/lifecycle.go`, `supervised_lifecycle.go` and their tests (`scm/scm_test.go`, `launchd/launchd_test.go`, `launchd/stopwait_test.go`, `supervised_lifecycle_test.go`), `selfupdate/codesign/codesign_test.go`, `selfupdate/verify/ghattest/ghattest_test.go`, `docs/guides/extending-selfupdate.md`; the MADR (Amendment A1); and `launchd/job.go` (Deviation D6). `scm/service.go` needed no change: its documentation already exempts only `Installed`, `Running` and `Enabled` from their context, so `Start` honouring it now matches.

1. **Probe P6, on this Mac**, in the GUI domain: `launchctl bootstrap gui/<uid> <missing plist>` exits 5, `Bootstrap failed: 5: Input/output error`; `launchctl print` of a never-loaded label exits 113, `Could not find service`; `launchctl enable` of that label exits 0 and leaves an override entry, which `print-disabled` then lists as `enabled`. Recorded as MADR Amendment A1; no fact or decision changes. **The probe's own `enable` left that entry, for `com.example.p6-never-loaded`, on this Mac.** launchctl has no command that removes an override and the override file is root-owned, so it was reported to the owner, not edited.
2. **scm `Start`** checks its context before it opens the service and again before `h.start()`. `TestStartHonoursEndedContext`, in a bubble: no call, the service left stopped. Seen failing with the first check planted out (`scm_test.go:450: calls ["open"]; want none on an ended context`) and on HEAD's `lifecycle.go` (`calls ["open" "start"]`, probe P4's behaviour).
3. **launchd `Installed`** is documented as ignoring its context, on `Job` and on the method, as scm's and Task Scheduler's types document theirs. The guide has no table of context-free methods for any backend, so that part of the step had nothing to update; none was added.
4. **launchd `Stop`:** `bootout` and the wait share one deadline, `bound` from when the bootout is issued; the wait gets what remains, at least one look. `TestStopOneDeadline`, in a bubble: a job that never goes, whose bootout takes half the bound, is `ErrTimeout` at exactly 60 s; on HEAD's `lifecycle.go`, `stopwait_test.go:68: Stop took 1m30s, want its bound, 1m0s`.
5. **launchd `Start`** returns `ErrNotInstalled`, with the plist's path, when the plist does not exist, before any launchctl command. `TestStartMissingPlist`: no call, at once; on HEAD's `lifecycle.go`, `launchd_test.go:446: Start = <nil>, want ErrNotInstalled`.
6. **`Supervised.Start`**, by the owner's answer to question 5: `ErrNotInstalled` when `State` answers and reports the service not installed; `Up` otherwise, including with no supervisor. `TestSupervisedStart` gains the case; with the check planted out, `supervised_lifecycle_test.go:467: not installed: Start = <nil>, calls ["up"]; want ErrNotInstalled and no Up`.
7. **The codesign and ghattest fakes** refuse a call on an ended context and record nothing, and each package gains `TestFakeHonoursContext`; on the old fakes, `codesign_test.go:345: Run on a cancelled context: <nil>, calls [{/usr/bin/codesign …}]` and `ghattest_test.go:464: … calls [{/usr/bin/gh …}]`.
8. **The guide** says launchd's bootout and wait share one bound since `v1.23.0`, and that a `Start` on an uninstalled service is `ErrNotInstalled` at once for launchd and `Supervised`, and starts nothing on an ended context for the SCM.
9. **Runs:** `go test -race -count=3` of `./selfupdate/service/...`, `codesign` and `verify/...` on the Linux and Windows test hosts, nine packages each, all `ok`; `make pre-add-check` on the ten Go files: `10 file(s) clean`; `make markdownlint` and `make docs-check` exit 0; `make gate`: every step rc=0, `apicheck` "compatible with v1.22.3", `overall=0`.

### Deviation D6 (2026-10-28): `Job`'s doc is in `launchd/job.go`

* **Found:** Q10 step 3 documents launchd's `Installed` as ignoring its context "in `Job`'s doc beside its siblings'", and `Job` is declared in `selfupdate/service/launchd/job.go`, which the list does not name.
* **Decision:** the owner, 2026-10-28, chose "Add job.go to Q10": four comment lines on `Job`, as scm and Task Scheduler document theirs on the type. Comment-only; no MADR change.

### Q11: `servicetest` over the fakes (2026-10-28)

Files: from the list, the new `systemd/conformance_test.go`, `launchd/conformance_test.go`, `scm/conformance_test.go`, `taskscheduler/conformance_test.go`, `service/export_test.go`, `service/conformance_test.go` and `service/runner_conformance_test.go`; `systemd/fake_test.go` and `launchd/fake_test.go`; and `docs/architecture.md`'s package table (`service` 24 to 27 test files, systemd 18 to 19, launchd 14 to 15, scm 13 to 14, Task Scheduler 18 to 19). `scm/fake_test.go` and `taskscheduler/fake_test.go` needed no change: their hooks (`onStop`, `onStart`, `stopKeeps`, `exitIn`, `runExits`) arrange every state, and their reads already move the fake as time passes.

1. **Fake labs**, each a `servicetest.Lab` over its package's fake, run inside a bubble, each with an assertion on the checks it skipped:

   | Backend | Test | Case | Skipped |
   | :--- | :--- | :--- | :--- |
   | systemd | `TestConformance` | `StopBound` `DefaultPollTimeout` (no `TimeoutStopUSec`) | start hangs: `Start` documents no bound of its own |
   | launchd | `TestConformance` | `StopBound` and `StartBound` `DefaultPollTimeout`; `ContextFree: MethodInstalled` | none |
   | scm | `TestConformance` | `StopBound` and `StartBound` `DefaultPollTimeout`; `ContextFree: MethodInstalled\|MethodRunning\|MethodEnabled` | none |
   | Task Scheduler | `TestTaskConformance` | `StopBound` `DefaultPollTimeout`; `ContextFree` the four methods; `RestoresAfterFailedStop` | start hangs: `Start` runs the task without waiting |
   | `Supervised` | `TestSupervisedConformance` (external) | `StopBound` twice `DefaultPollTimeout` (a failed stop's `Up` has a bound of its own); `RestoresAfterFailedStop` | start hangs: `Up` runs on the caller's context |

   * **systemd:** a new `InvocationID` per start; `SlowStop` goes 5 s after the stop job is queued, through a new `onRun` hook that runs before each call is answered; `Stubborn` stays `deactivating` and sets `stopBlocks`, so a stop that waits for its job (F2) blocks. `StartHangs` sets a new `startBlocks`, the same hold for a start; no check reaches it, since the case passes no `StartBound`. The fake now reads both channels under its lock, so a lab that replaces them races with no stray call.
   * **launchd:** the lab is also the `Runner` the `Job` uses, passing each command to the fake: `bootout` returns 2 s after launchd took it, and returns the context's error if the context ends first. A new `pidUntilGone` field keeps the job's process while a bootout leaves it loaded. It is opt-in, not the fake's default: `TestManagedStopTimeoutRestartsJob` (`stopwait_test.go:76`) relies on bootout clearing the pid, and failed with the default changed. `SlowStop` goes 5 s after its bootout; `Stubborn` sets `ExitTimeOut` 0. Process IDs start at 50,000,000, which no host runs, so `pidAlive` is false once a job has gone. `bootstrapHangs` is read under the fake's lock.
   * **scm:** `SlowStop` reports `STOP_PENDING` five times, its checkpoint advancing, then `STOPPED`; `Stubborn` stays `STOP_PENDING` with no progress, which `waitPending` declares hung after 10 s; `StartHangs` the same in `START_PENDING`; a new process ID per start.
   * **Task Scheduler:** `SlowStop` through `stopKeeps`, `Stubborn` through `stopKeeps` and `exitIn` -1; `Stopping` is a recorded `stop` since `Arrange` while the program runs; `Wanted` is the task's switch, which a failed stop turns back on.
   * **`Supervised`:** `export_test.go` exports `SupervisedFake` and `NewSupervisedFake` over `fakeSup`, its process table, and a fixed `self` and `exe`, with a `Down` that takes one second of bubble time. The lab is in the external test, because `servicetest` imports `service`: an internal test file cannot import it.
2. **`VerifyRunner`:** `TestRunnerConformance` over the systemd fake (`Blocking` a stop without `--no-block`, with `stopBlocks`) and the launchd fake (a hanging `bootstrap`), each also checking that only the quick call was recorded; `TestExecRunnerConformance` over `ExecRunner` on the real clock, the test binary in its `receipt` and `sleep` modes. The two `TestFakeHonoursContext` tests stay.
3. **Seen failing**, each in a scratch copy, the first lines of the failure:

   | Plant | Test | Failure |
   | :--- | :--- | :--- |
   | 0037-MADR F1: `fakeSystemd.Run`'s context check removed | systemd both | `ended context: Installed on an ended context returned no error`; `Run on a cancelled context = no error; want an error wrapping context.Canceled` |
   | F1: `fakeLaunchd.Run`'s context check removed | launchd both | `ended context: Running on an ended context returned no error`; `Run on a cancelled context = no error; …` |
   | F2: `"stop", "--no-block", "--no-ask-password"` to `"stop", "--no-ask-password"` | systemd | `stubborn: stop of a service that ignores it did not return within 1m1s` |
   | F3: `context.WithDeadline(context.WithoutCancel(ctx), deadline)` to `context.WithDeadline(ctx, deadline)` | launchd | `cancel during stop: stop returned on its cancellation while the service still ran (running=true, no error): the cancellation cut the wait short` |
   | F4: the bootstrap's `return fmt.Errorf("%w: bootstrap %s: %w", service.ErrTimeout, j.o.Plist, err)` to `return err` | launchd | `start hangs: start the manager never completes = context deadline exceeded after 1m0s; want service.ErrTimeout` |
   | 0035-MADR Amendment A3: `supervised_lifecycle.go` of `v1.22.0`, its three `s.now()` and one `s.sleep(ctx,` rewritten to `time.Now()` and `sleepCtx(ctx,`, as in Q1 | `TestSupervisedConformance` | `stubborn: after the timed-out Stop the service runs, but its manager no longer wants it up`; also `absent: after Start of an absent service the lab sees installed=false running=true`, the Q10 fix that file predates |
   | 0015-MADR B3: systemd's `sctx := context.WithoutCancel(ctx)` to `sctx := ctx` | systemd | `cancel during stop: stop returned on its cancellation while the service still ran …` |
   | B3: scm's `s.deadline(context.WithoutCancel(ctx))` to `s.deadline(ctx)` in `Stop` | scm | the same |
   | B3: Task Scheduler's `t.deadline(context.WithoutCancel(ctx))` to `t.deadline(ctx)` | Task Scheduler | the same |
   | 0024-MADR D4-A: Task Scheduler's restore after a failed stop disabled (`&& false`) | Task Scheduler | `stubborn: after the timed-out Stop the service runs, but its manager no longer wants it up` |
   | Q10: `supervised_lifecycle.go` at `eb525be` | `TestSupervisedConformance` | `absent: after Start of an absent service the lab sees installed=false running=true` |
   | Q10: `scm/lifecycle.go` at `eb525be` | scm | `ended context: after Start on an ended context the lab sees running=true (no error); want the service untouched` |
   | Q10: `launchd/lifecycle.go` at `eb525be` | launchd | `absent: after Start of an absent service …`; `stubborn: stop of a service that ignores it did not return within 1m1s` (the two bounds Q10 joined) |
   | `ExecRunner` runs on `context.WithoutCancel(ctx)` | `TestExecRunnerConformance` | `a run whose context was cancelled still ran after 10s` |

4. **Runs:** `go test -race -count=20 -run Conformance` of the five packages on this Mac, all `ok`; `go test -race -count=3` of `./selfupdate/service/` and its five subpackages on the Linux and Windows test hosts, six packages each, all `ok`; the conformance tests under Go 1.26.10, all `ok`; `make pre-add-check` on the nine Go files: `9 file(s) clean`; the architecture table: 20 rows, 0 differ; `make markdownlint`, `make check-records`, `make docs-check` and `--ids` on the changed files exit 0.
5. **`make gate`**: every step rc=0, `apicheck` "compatible with v1.22.3", `overall=0`.

### Deviation D7 (2026-10-29): `RequireAll` and the skips the plan expects

* **Found:** Q12 step 3 runs every live test with `RequireAll`, which makes a skipped check a violation (`servicetest/lifecycle.go:181`). Step 2 has Task Scheduler's lab return `ErrUnsupported` for `Stubborn` and `SlowStop`, as its `Stop` ends the program after its grace, which skips "stubborn" and "cancel during stop"; and systemd's `Start`, Task Scheduler's and `Supervised`'s `Up` document no bound, so they pass no `StartBound` and "start hangs" is always skipped (`lifecycle.go:482-483`). As written, the systemd, Task Scheduler and s6 live tests fail on every run. Not pre-existing: Q11's fake tests met the same skips with an assertion on the exact skip list, and no `RequireAll`.
* **Decision:** the owner, 2026-10-29, chose "Exact skip list": each live test records its skips through `Logf` and requires exactly the list it expects, as Q11's do; `RequireAll` is set where that list is empty (scm, and launchd unless its probe shows `Stubborn` cannot be arranged). An unexpected skip fails the test. MADR Amendment A2.

### Deviation D8 (2026-10-29): the s6 subtest cannot import `servicetest`

* **Found:** Q12 step 3 makes the s6 run a `conformance` subtest of `TestLiveSupervisedS6`, which is in package `service` (`supervised_live_linux_test.go:3`); `servicetest` imports `service`, so that file cannot import it, the cycle Q11 met. CI selects the test by `^TestLiveSupervisedS6$` (`ci.yml:77`).
* **Decision:** the owner, 2026-10-29, chose "Hook from external file": `supervised_live_linux_test.go` declares an exported hook variable, a new Linux-only external test file sets it in `init` and holds the s6 lab, and `TestLiveSupervisedS6` runs `t.Run("conformance", …)` through it, failing when it is unset. Both packages build into one test binary, so CI's selector runs it and `ci.yml` does not change. No MADR change.
* **Files added to Q12:** `selfupdate/service/supervised_live_conformance_linux_test.go` (package `service_test`); `docs/architecture.md`'s package table (`service` 27 to 28 test files).

### Deviation D9 (2026-10-29): launchd reads `ExitTimeOut` 0 as a kill at once

* **Found:** Q12 step 2 makes launchd's `Stubborn` depend on an `ExitTimeOut` launchd honours as no kill, and cites P6, which probed only a missing plist. Probe P7 on this Mac (MADR Amendment A3) shows a job that ignores TERM killed at once after `bootout` under `ExitTimeOut` 0, and at 3 s under 3. `launchd/lifecycle.go:214-215` says launchd reads 0 "as no kill", as do test comments at `stopwait_test.go:59,81`, `launchd_test.go:281` and Q11's `conformance_test.go:125`. Pre-existing in the comments; no behaviour depends on it, since a 0 leaves the stop's bound at the poll timeout, more than a kill at once needs.
* **Decision:** the owner, 2026-10-29, chose "Amend + fix comments": MADR Amendment A3 records P7; the five comments are corrected, comment only. launchd's live lab returns `ErrUnsupported` for `Stubborn`, as step 2 allows, so its exact skip list (Deviation D7) is "stubborn"; P7's malformed plist, which `bootstrap` refuses with 5 every time, arranges `StartHangs`.
* **Files added to Q12:** `selfupdate/service/launchd/lifecycle.go`, `stopwait_test.go`, `launchd_test.go` and `conformance_test.go`, comments only.

### Deviation D10 (2026-10-29): systemd's `Start` of a unit that crashes is not `ErrUnhealthy`

* **Found:** on the Linux test host, systemd 259, in both scopes, `TestLiveConformance` failed one check: `servicetest: crash: start of a service that exits at once = selfupdate: systemd: start selfupdate-live-74caca15.service: exit 1: `. The lab's `Crashing` unit is `Type=notify`, as every live unit is, and its program exits 3 before it is ready; `Unit.Start` waits for the start (`systemctl start --quiet`), which fails with exit 1 and no message, and `commandError` returns a bare `*service.CommandError` (`systemd/lifecycle.go:143-160`). The contract takes a failed `Start` only with `ErrUnhealthy` (`servicetest/lifecycle.go:468-472`). Pre-existing; the managed installer still rolls back (`TestLiveHealthFailureRollsBack`), but a caller cannot tell a crashed start from another failure on systemd alone. Every other check passed.
* **Decision:** the owner, 2026-10-29, chose "Wrap ErrUnhealthy": after a failed start, `Start` reads the unit's `InvocationID`, `ActiveState` and `Result`; a new invocation that is `failed` or `inactive` ran and failed, and the error wraps `service.ErrUnhealthy` as well as the `CommandError`. A start that fails before the unit runs, as on a missing dependency, keeps the invocation, and is not `ErrUnhealthy` even when an earlier failure left the unit failed. MADR Amendment A4.
* **Files added to Q12:** `selfupdate/service/systemd/lifecycle.go` (`Start`, its doc, and `startError`) and `systemd_test.go` (`TestStartOfCrashedUnitIsUnhealthy`, seen failing on today's code: `err = selfupdate: systemd: start demo.service: exit 1: ; want a CommandError, ErrUnhealthy true`, for the `failed` and `inactive` cases).

### Deviation D11 (2026-10-29): gosec reports `SameExecutable`'s first `os.Stat`

* **Found:** with Q12's Linux test changes, `GOOS=linux golangci-lint run ./...` reports `selfupdate/service/sameexec.go:22:20: G703: Path traversal via taint analysis (gosec)` on `ia, err := os.Stat(ca)`; darwin and windows are clean, and a copy of `HEAD` is clean. Bisected in scratch copies: no one change in `supervised_live_linux_test.go` alone brings it back over `HEAD`, nor does the external s6 file or passing the adapter as a function value; `HEAD`'s own s6 update mode already passes paths from the environment to `Reconcile`, and so to `SameExecutable`, unreported. gosec's taint analysis reaches the sink depending on the call graph's shape. The function's second `os.Stat` (line 25) already carries `//nolint:gosec // metadata of a path the caller names; nothing is opened, read or written`.
* **Decision:** the owner, 2026-10-29, chose "Mirror nolint": line 22 carries the same comment and reason. Comment only; no MADR change.
* **Files added to Q12:** `selfupdate/service/sameexec.go`.

### Q12: `servicetest` over the real managers (2026-10-29)

Files: from the list, `launchd/live_darwin_test.go`, `systemd/live_linux_test.go`, `scm/live_windows_test.go`, `taskscheduler/live_windows_test.go` and `service/supervised_live_linux_test.go`; the `live_fixes_*_test.go` files and the `main_test.go` files needed no change, since the new modes sit beside each file's existing ones and the fix tests keep the `t`-taking helpers. Added by the deviations: `service/supervised_live_conformance_linux_test.go` and the package table (`service` 27 to 28 test files) (D8); comments in `launchd/lifecycle.go`, `stopwait_test.go`, `launchd_test.go` and `conformance_test.go` (D9); `systemd/lifecycle.go` and `systemd_test.go` (D10); `service/sameexec.go` (D11).

1. **Live labs**, each a `servicetest.Lab` over the real manager on the real clock, each test requiring its exact skips (D7):

   | Backend | Test | Case | Skipped |
   | :--- | :--- | :--- | :--- |
   | launchd | `TestLiveConformance` | `Poll.Timeout` 20 s; `StopBound` 35 s, the longer of it and the default `ExitTimeOut` plus `stopGrace`; `StartBound` 20 s; `ContextFree: MethodInstalled` | stubborn (P7, MADR Amendment A3) |
   | systemd | `TestLiveConformance` | `Poll.Timeout` 10 s; the lab's units `TimeoutStopSec=5`, so `StopBound` 35 s | start hangs |
   | scm | `TestLiveConformance` | `Poll.Timeout` 20 s, `StopBound` and `StartBound` 20 s; `ContextFree` Installed, Running and Enabled; `RequireAll` | none |
   | Task Scheduler | `TestLiveTaskConformance` | `Poll.Timeout` 30 s, `StopGrace` 5 s; `StopBound` 30 s; `ContextFree` the four methods; `RestoresAfterFailedStop` | stubborn, cancel during stop, start hangs |
   | s6 | `TestLiveSupervisedS6`, subtest `conformance` | the adapter without `Kill`; `Poll.Timeout` 10 s, `StopBound` 20 s; `RestoresAfterFailedStop` | start hangs |

   * **launchd:** the lab writes the job's plist for each state under the live tests' fixed label. `SlowStop` ignores TERM under `ExitTimeOut` 3; the service mode now catches TERM and notes it in a `termed` file rather than ignoring it, so the lab sees the stop under way. `Crashing` runs an `exit 3` script; `StartHangs` is P7's malformed plist, which `Start` retries to its bound; `Stubborn` is `ErrUnsupported`. `newLiveJob` and `newSlowLiveJob` arrange through the lab (`Running`, and `SlowStop` with their own `ExitTimeOut`); `build` keeps its `t` form over an error-returning `buildTo`.
   * **systemd:** `newLiveEnv`, `writeUnit` and `remove` return errors. `remove` stops the unit without blocking, which also cancels a restart, kills it if it has not gone 3 s later, then clears its failure, disables it, deletes its file and reloads. Units gain `[Install] WantedBy=default.target`, enabled only when the arrangement asks. New modes: `slow-stop` (exits 3 s after TERM), `crash` (exits before it is ready), `never-ready`. `newLiveUnit` and `TestLiveStopInfinityTimesOut` arrange through the lab (`Running`, `Stubborn`); `newLiveUnitWith` stays for the fix tests' own lines. A first version also ran `systemctl disable` on a fresh unit, a second reload just before the start: one WSL system-scope run of `TestLiveStopInfinityTimesOut` then failed `Stop = … show …: exit 1: Failed to get properties: Transport endpoint is not connected after 21.70651ms`. With `enable` only when asked, three further runs of that test and `TestLiveConformance`, in both scopes, passed.
   * **scm:** the service's config file gains `Mode`; the handler's modes are `slow-stop` (`STOP_PENDING` for 3 s, its checkpoint advancing), `ignore-stop` (one `STOP_PENDING` with no progress, then it runs on), `crash` (exits 1 at once) and `never-ready` (`START_PENDING` with no progress). `endService` stops the service and, when it is still there 5 s later, ends its process with `taskkill /F`, before `deleteService`. `removeService`, `registeredPath` and `build` keep their `t` forms over error-returning ones; `newLiveService` arranges through the lab (`Running`, automatic).
   * **Task Scheduler:** `liveXMLWith` writes the definition without the watchdog trigger, so only a start runs the task; a `crash` mode exits 1. The lab reads the task through `schtasks /query /xml` and the program by the process it reported ready as, checked by its image, so a reused process ID is not taken for it. `newLiveTaskWith` is unchanged: its watchdog trigger is what `TestLiveTaskStopHoldsDisabled` tests.
   * **s6:** the hook of D8. The adapter is built without `Kill`: with it, `Stop` kills a script that ignores TERM at half its timeout, and `Stubborn` could not be arranged. The reference `State` reports a missing service directory as not installed, for `absent`. The lab links the service directory into the scan directory and removes it as s6 does, by unlinking it and `s6-svscanctl -an`: with the directory in the scan directory, `s6-svscan` started `s6-supervise` again, and the first run failed `servicetest: stubborn: arrange Stubborn: s6-supervise exits: not within 10s`.
2. **Runs on the managers**, the final code unless noted:

   | Host | Run | Result |
   | :--- | :--- | :--- |
   | this Mac, launchd | `^TestLive`, `SELFUPDATE_REQUIRE_LAUNCHD=1` | 12 tests `ok`, 221 s; `TestLiveConformance` 56 s |
   | the Linux test host, systemd 259 | the test binary as CI runs it, `^TestLive`, as root (system scope) and as the user (user scope) | 11 tests `ok` in each scope; `TestLiveConformance` 39 s and 28 s |
   | the WSL, systemd | the same | 11 tests `ok` in each scope; `TestLiveConformance` 21 s and 19 s |
   | the Windows test host, scm | `^TestLive`, `SELFUPDATE_REQUIRE_SCM=1` | 9 tests `ok`; `TestLiveConformance` 39 s |
   | the Windows test host, Task Scheduler | `^TestLiveTask`, `-skip '^TestLiveTaskUpdate$'` as CI | 11 tests `ok`, `TestLiveTaskzCleanup` last |
   | the WSL, s6 | `^TestLiveSupervisedS6$`, `SELFUPDATE_REQUIRE_SUPERVISED=1` | `ok`; subtest `conformance` 18 s |

3. **Seen failing**, one plant per backend in a scratch copy, run on its host, the first violation lines:

   | Plant | Host | Failure |
   | :--- | :--- | :--- |
   | 0037-MADR F2: `"stop", "--no-block", "--no-ask-password"` to `"stop", "--no-ask-password"` | the Linux test host, both scopes | `servicetest: stubborn: stop of a service that ignores it did not return within 36s` |
   | F11: `launchd/lifecycle.go` at `eb525be` | this Mac | `servicetest: absent: Start of an absent service = selfupdate: service: timed out: bootstrap …: exit 5: Bootstrap failed: 5: Input/output error` |
   | F11: `scm/lifecycle.go` at `eb525be` | the Windows test host | `servicetest: ended context: after Start on an ended context the lab sees running=true (no error); want the service untouched` |
   | `Task.Stop`'s ended-context check removed | the Windows test host | `servicetest: ended context: after Stop on an ended context the lab sees running=false stopping=false (no error); want the service untouched` |
   | 0035-MADR Amendment A3 and F11: `supervised_lifecycle.go` of `v1.22.0`, rewritten as in Q11 | the WSL | `servicetest: absent: Start of an absent service = … s6-svc -u exit 111 …`; `servicetest: stubborn: after the timed-out Stop the service runs, but its manager no longer wants it up` |
   | D10: systemd's `Start` before the fix (today's code) | the Linux test host, both scopes | `servicetest: crash: start of a service that exits at once = selfupdate: systemd: start …: exit 1: ` |

   0037-MADR F3, planted first for launchd, passed live: P7 shows `launchctl bootout` returning within 30 ms, before the lab can see the stop, so no cancellation reaches it on a real launchd. Q11's fake, whose bootout takes 2 s, catches it; the live run uses the F11 plant instead.
4. **Checks:** `make pre-add-check` on the 13 Go files: `13 file(s) clean`; `golangci-lint` for `GOOS=linux`, `darwin` and `windows`: 0 issues each, after D11; `go test -race -count=3 ./selfupdate/service/...` on this Mac, the Linux test host and the Windows test host: six packages `ok` on each; under Go 1.26.10, `go test ./selfupdate/service/...` `ok` and `go vet` for the three targets clean; the architecture table: 20 rows, 0 differ.
5. **Host state:** `s6` installed in the WSL with `apt-get install s6` (exit 0), as 0035's run did; it is left there until Q19 removes it, unless the owner keeps it. The P6 override entry 0038-MADR Amendment A1 records stays; the probes and runs here used the fixed label, and added none.
6. **`make gate`**: every step rc=0, `apicheck` "compatible with v1.22.3", `overall=0`.
7. **CI** on the pushed commit `2f3d4de`: the CI run `success`, all 18 jobs (`validate` on Ubuntu 24.04, macOS 15 and Windows 2025, `go126`, `installer-containers`, both release rehearsals and their identity jobs, `release-rehearsal-check`); the Docs run `success`.

### Deviation D12 (2026-10-29): `lockedTarget` reads the target by path, in `target.go`

* **Found:** Q13 step 2 has the replace path read the target with `root.Lstat`. Its read today is `lockedTarget` (`selfupdate/target.go:222-231`), `os.Lstat(target.Path)`, whose only callers are `replace_unix.go:56` and `replace_windows.go:35`. `target.go` is not in Q13's list; with both callers on the root, a path version left behind is unused, which the `unused` linter refuses. Pre-existing.
* **Decision:** the owner, 2026-10-29, chose "Root param in target.go": `lockedTarget(root *os.Root, target Target)` reads `root.Lstat(target.Base)`, with its doc and identity check kept. No MADR change.
* **Files added to Q13:** `selfupdate/target.go`.

### Deviation D13 (2026-10-29): `cleanup.go` names `randomSibling`

* **Found:** `selfupdate/cleanup.go:13` reads `// backupPrefix is the name prefix of every backup randomSibling allocates.` Q13 step 2 replaces `randomSibling` with `backupFile`, a hard-link-or-copy loop in the root, and deletes it. `cleanup.go` is not in Q13's list. Every other mention is in a listed file or a historical record.
* **Decision:** the owner, 2026-10-29, chose "Fix the comment": the comment names `backupFile`. Comment only; no MADR change.
* **Files added to Q13:** `selfupdate/cleanup.go`.

### Q13: root-relative replacement (2026-10-29)

Files: from the list, `selfupdate/replace.go`, `replace_unix.go`, `replace_windows.go`, `session.go`, `journal.go`, `filedigest.go`, `leftovers.go` and `cleanup_windows.go`; the new `selfupdate/rootswap_test.go`; the tests of those files that change the effects or call the changed functions: `branch_coverage_test.go`, `cleanup_busy_windows_test.go`, `cleanup_windows_test.go`, `events_test.go`, `fs_test.go`, `install_hardening_test.go`, `install_regression_test.go`, `lifecycle_windows_test.go`, `managed_regression_test.go`, `modebits_unix_test.go`, `modepolicy_test.go`, `modepolicy_unix_test.go`, `recovery_test.go` and `replace_test.go`; and the package table (`selfupdate` 88 to 89 test files). Added by the deviations: `selfupdate/target.go` (D12) and `selfupdate/cleanup.go` (D13).

1. **P2 as a committed test.** `TestInstallRefusesDirectorySwappedBeforeRename`: `writeJournalFn` writes the journal, then renames the locked directory aside and makes a decoy holding `demo` and a file at the staging file's name. Run on the tree before any production change, it failed as P2 did: `decoy .demo.selfupdate-<n>: "", want "-rwxr-xr-x decoy .demo.selfupdate-<n>"`; `decoy demo: "-rwxr-xr-x decoy .demo.selfupdate-<n>", want "-rwxr-xr-x decoy demo"`; `backups left in the locked directory: [.demo.selfupdate-bak-<n>]`. The rename went into the decoy, and the session's rollback and `ErrConcurrentUpdate` passed the test's first check, as in P2. After steps 2 to 5 it passes: the rename goes into the locked directory, the check after it rolls back there, and the decoy is untouched.
2. **Names in the root.** `rootCreateTemp(root, prefix, suffix)`: an `O_RDWR|O_CREATE|O_EXCL` loop, mode 0o600, up to 10 000 names of the form prefix, a random decimal number from `crypto/rand`, and suffix, the form `isLeftover` and `keptName` parse. It names staging (`CreateStaging`), the journal's temporary file, and the Windows receipt's temporary file. `rootReserveName` creates and removes one, for an interrupted backup's fresh kept name. `backupFile(root, target)` links the target to a fresh backup name with `Root.Link`, the next name on `EEXIST`, and copies it through the root (`copyFile`) when the link fails otherwise. `lockedTarget` reads `root.Lstat` (D12); the target's and staging's digests are `rootFileSHA256`, whose mismatch error now names the file; the backup goes with `Root.Remove` at commit; staging goes through the root at `Close`; and every directory flush is `syncRootFn(root)`. `randomSibling`, `syncDirectory` and `fileSHA256` are gone, and so is `CreateStaging`'s check that staging did not escape the directory, which a name made in the root cannot do. `fsOps` loses `osChmod`, `osRemove`, `osLink` and `syncDirFn`, gains `rootRemove`, `rootLink` and `afterCheckHook` (run right after `replaceLocked`'s directory check, for the tests), and on Unix `osLchown` becomes `fileChown`. No `.previous` was removed by path: it is only ever replaced, now by `replacePath` in the root.
3. **Owner and mode through a descriptor.** `openChecked(root, name, info)` opens with `modeOpenFlags` and refuses a file other than the one `info` describes with `ErrConcurrentUpdate`. On Unix the flags are `O_RDONLY|O_NOFOLLOW`; fchown and fchmod need no more, and a running binary's hard-linked backup allows no write. On Windows they are `O_RDWR`: `(*os.File).Chmod` sets the read-only attribute with `SetFileInformationByHandle`, which needs write access, and only staging is opened so. `setStagingMode` gives staging its owner (Unix) and mode through one descriptor, owner first; `clearSpecialBits(root, name)` reads `root.Lstat` and sets the mode through `openChecked`.
4. **Rename.** `replacePath(ctx, root, oldname, newname)`: on Unix `Root.Rename`; on Windows `moveFileReplace` on the root's path joined with each name, unchanged in effect (MADR F8).
5. **Rollback.** `dropSameBackup` runs after the rollback rename in `rollbackReplacement` and `rollbackInRoot`: a backup still there as the target's own file is removed through the root, advisorily.
6. **Tests.** New, in `rootswap_test.go`: the P2 test; `TestInstallSwapAtEachStep`, which swaps the directory for a read-only decoy holding only `demo` after `Begin`, after the check (with and without hard links), and during a healthy probe, a healthy probe with `KeepPrevious`, and a failed probe, and requires the decoy untouched, the locked directory as the install would leave it, and, after the check, the journal's digests to be those of the locked files and the restored target to be the original file; `TestClearSpecialBitsThroughRoot`; `TestKeepInterruptedThroughRoot`; `TestRollbackRemovesSameFileBackup` (`rollbackReplacement` and `rollbackInRoot`); and `TestRootNamesAreLeftovers`. Every flush is now a root sync, the journal's first among them, so the tests that injected a `syncDirFn` failure number the syncs after the journal's through a new helper, `syncsAfterJournal`, and fail the same ones as before. The 0015-MADR B2, 0004 R3/R5 and 0010 tests (`TestInstallRefusesTargetReplacedAfterBegin`, `TestInstallRollsBackWhenDirectoryMovesAfterRename`, `TestBeginChecksDirectoryBeforeReceipt`) pass unchanged. `TestInstallSwapAtEachStep` was split into a case type with `run` and `check` methods after `gocognit` reported a complexity of 68; the plants below ran on both versions with the same result.

   **Seen failing:** each moved call put back by path in a scratch copy (`scripts/plant-copy.sh`, one script), with `go test -count=1 ./selfupdate/` on this Mac. All 19 failed a test, and none failed to build:

   | Plant | Failed |
   | :--- | :--- |
   | staging made with `os.CreateTemp(target.Dir, …)` | `TestInstallSwapAtEachStep` (after `Begin`: `create staging: open <tmp>/…/.demo.selfupdate-<n>: permission denied`) |
   | staging read with `os.Lstat(path)` | `TestInstallSwapAtEachStep` |
   | staging hashed through a root reopened by path | `TestInstallSwapAtEachStep` |
   | `lockedTarget` with `os.Lstat(target.Path)` | `TestInstallSwapAtEachStep` |
   | `openChecked` with `os.OpenFile` on a path | `TestClearSpecialBitsThroughRoot`, `TestInstallSwapAtEachStep` |
   | the target hashed through a root reopened by path | `TestInstallSwapAtEachStep` |
   | the backup linked with `os.Link` on paths | `TestInstallSwapAtEachStep` (`the restored target is not the original file`) |
   | the copy read with `os.Open` on a path | `TestInstallSwapAtEachStep` (`locked directory's target holds "decoy demo", want "old-bytes"`) |
   | the journal's temporary name reserved with `os.CreateTemp` by path | `TestInstallSwapAtEachStep` |
   | `replacePathOS` with `os.Rename` on paths | `TestInstallRefusesDirectorySwappedBeforeRename`, `TestInstallSwapAtEachStep` |
   | the replacement's flush through a root reopened by path | `TestReplaceRestoreSurvivesCancellation`, `TestRunReportsKeptBackup`, `TestKeptBackupSurvivesLaterSessions`, `TestKeptBackupClearsSpecialBits` and two more |
   | the commit's `os.Remove(result.backup)` | `TestInstallInjectedFailures`, `TestCommitReportsBackupRemovalFailure`, `TestInstallSwapAtEachStep` |
   | `KeepPrevious`'s `os.Rename(backup, previous)` | `TestInstallSwapAtEachStep` |
   | `clearSpecialBits` with `os.Lstat` on a path | `TestClearSpecialBitsThroughRoot`, `TestInstallSwapAtEachStep` |
   | `Close` with `os.Remove(name)` | `TestInstallSwapAtEachStep` (`left in the locked directory: [.demo.selfupdate-<n>]`) |
   | `chownStaging` with `os.Lchown` on a name | `TestStagingOwnership`, `TestInstallSwapAtEachStep` |
   | the fresh kept name reserved with `os.CreateTemp` by path | `TestKeepInterruptedThroughRoot` (`the backup was not kept`) |
   | `rollbackReplacement` without `dropSameBackup` | `TestRollbackRemovesSameFileBackup` (`backups left: [.demo.selfupdate-bak-1]`) |
   | `rollbackInRoot` without `dropSameBackup` | `TestRollbackRemovesSameFileBackup` |

   Not planted: the Windows receipt's temporary file in the root. Windows refuses to rename a directory with open handles, so no swap can reach it there; the Windows receipt and busy-image tests cover the call itself.
7. **Runs.** This Mac: `go test -race -shuffle=on -count=3 ./selfupdate/` `ok`. The Linux test host: `go test -race -count=1 ./selfupdate/` `ok`, then the test binary as root with `SELFUPDATE_REQUIRE_ROOT=1`, as CI's "ownership as root" step, for `TestStagingTakesOwnerAsRoot` and the six new tests: all `PASS`. The Windows test host: `go test -race -count=1 -v ./selfupdate/` `ok`, with the busy-image, retry-budget and receipt tests (`TestMoveFileReplaceHonoursRetryBudget`, `TestWindowsCommitAddsToPendingReceipt`, `TestWindowsCommitReceiptNotCancelled`, `TestWindowsCommitReceiptBoundedByLockTimeout`, `TestWindowsKeepPreviousBusy`, `TestWindowsBusyPendingBackupKept`, `TestWindowsReceiptKeepsOnlyBusy`), `TestRollbackRemovesSameFileBackup`, `TestRootNamesAreLeftovers` and `TestChmodStagingCopiesPerm` passing; the swap tests skip there, as the existing ones do. Each host ran the final tree. A first run of the race step with `CGO_ENABLED=0` was refused by the toolchain (`-race requires cgo`), and was run again with cgo on.
8. **V6:** a grep of `os.Remove`, `Rename`, `Lstat`, `Stat`, `Chmod`, `Chown`, `Lchown`, `Link`, `CreateTemp`, `Open`, `OpenFile`, `ReadFile`, `WriteFile` and `OpenRoot`, `openAbsFile` and `joinRemove` over the replace files leaves `checkDir`'s `os.Stat` (`session.go:401`), `beginSession`'s `os.OpenRoot` and its identity check's `os.Stat` (`:588`, `:608`), `resolveTarget`'s and `revalidateTarget`'s reads in `target.go`, and the Windows receipt read (`cleanup_windows.go:186`, `:196`). None is on the Unix replacement path.
9. **Checks:** `make pre-add-check` on the 25 Go files: `25 file(s) clean`, after the split; under Go 1.26.10, `go test ./selfupdate/` `ok` and `go vet` for the three targets clean; the architecture table: 20 rows, 0 differ.
10. **`make gate`**: every step rc=0, `apicheck` "compatible with v1.22.3", `overall=0`; `make markdownlint`, `make check-records` and `make docs-check` exit 0; `scripts/check-docs.sh --ids` on the 27 changed files: 0 findings.

### Deviation D14 (2026-10-29): revive refuses `Plan{} == Plan{}`

* **Found:** Q14 step 4 asserts `Plan`'s comparability with `var _ = Plan{} == Plan{}`. `golangci-lint` refuses it for each target: `selfupdate/plan.go:65:9: constant-logical-expr: expression always evaluates to true (revive)`.
* **Decision:** the owner, 2026-10-29, chose "Map-key assertion": `var _ map[Plan]struct{}`, which a field that is not comparable fails to build, as `==` does. No MADR change.

### Q14: `Plan` and `Apply` (2026-10-29)

Files: from the list, `selfupdate/checker.go`, `updater.go`, `runoptions.go`, `errors.go`, `types.go` (`Release.ID`'s doc), the new `plan.go`, `plan_test.go` and `plan_document_test.go`, `testdata/golden/plan.golden`, `fuzz_test.go`, `example_test.go` and the `Makefile`; and the package table (`selfupdate` 47 to 48 files, 89 to 91 test files, five to six fuzz targets, 14 to 15 goldens). `stream.go` is untouched: `Start` keeps calling `execRun`, which `execPinned` now backs.

1. **Discovery split.** `discover` is `fetchRelease`, then `vet(req, rel, fromLatest)`, which holds the immutable, state, `Validate`, channel, selection, metadata, classification and unpacker checks. `vet` takes no context: it makes no call. `fetchRelease`'s tag branch is `fetchTag(ctx, tag)`, which the pinned path also uses. With only this step in place, `go test ./selfupdate/` passed, the call-order test and the goldens included.
2. **One pipeline.** `run.execute(ctx, req, pin *Plan)`, and `run.apply` is `run.install(…, pin)`. With a pin, the request must match the plan after `normalizePlatform`, before `EventResolvingTarget` (`Plan.matches`, naming each field that differs); `Checker.pinned` fetches `pin.TargetVersion` with `fetchTag`, vets it with `pin.Latest`, and compares the release ID, the selection (IDs, names, sizes and, where the plan has one, digests; `ManifestName`; `Packed`) and the operation (`Plan.sameRelease`). The manifest steps are one function, `run.manifestEntry`, which `install` and `Plan` share: it downloads `SHA256SUMS` within `Limits.Manifest`, finds the entry, compares it with the pin (`Plan.sameEntry`, `ErrPlanStale` joined with `ErrIntegrity`), and runs the `ManifestVerifiers`. It replaced `install`'s inline steps after `gocognit` reported `install` at 52, and is what the MADR's "Plan shares its discovery and manifest steps" names. `execRun` keeps its signature for `Start`, over a new `execPinned(ctx, req, r, pin)`.
3. **`ErrPlanStale`** in `errors.go`, and `{ErrPlanStale, "plan-stale"}` after `latest-older` and before `mutable-release`.
4. **`plan.go`:** `PlanSchema = 1`; `Plan`, comparable, asserted by `var _ map[Plan]struct{}` (D14); `PlanDocument` and `AssetDocument`; `Plan.Document()`, `Plan.Request()`; `ParsePlan` (64 KiB, `DisallowUnknownFields`, trailing data, `schema_version` refused as `releasespec` words it, then the build kind and operation parsed, then `Plan.validate`); and `(*Updater).Plan`. `validate` also holds two rules the plan did not list, which a plan `Updater.Plan` makes always meets: `requested_version`, when set, is `target_version`, and `latest` is set exactly when no version was requested, as `fetchRelease` decides it.
5. **`(*Updater).Apply`** in `runoptions.go`: `ErrNotConstructed`, then an invalid plan, then the options, then `execPinned`. The confirmation is `Run`'s, unchanged. A request with `CheckOnly` checks the planned release, as `Run` does.
6. **`Release.ID`'s doc** says Apply compares it, so a source returns the same ID each time, from `Latest` and from `ByTag`.
7. **Tests**, in `plan_test.go` over a `tagSource`, a `scriptSource` whose `ByTag` answers by tag so a newer release can be published between `Plan` and `Apply`: `TestPlanThenApplyInstalls`; `TestApplyIgnoresNewerRelease` (Apply's source calls are `ByTag`, `OpenAsset:3`, `OpenAsset:2`); `TestApplyRefusesChangedRelease`, a row each for the release ID, the binary's asset ID, its digest, the `SHA256SUMS` entry (also `ErrIntegrity`) and the operation, each with no binary download and `plan-stale` as the failed event's detail, and a row where a latest plan names a rollback, which is `ErrLatestOlder`; `TestApplyRefusesAnotherRequest`, a row per request field with no source call, and an invalid plan; `TestApplyConfirms` (`Yes`, `DryRun`, approved, declined); `TestApplyCheckOnly`; `TestApplyEventsMatchRun`; `TestPlanReportsNothing` (no event, no question, source calls `Latest` and `OpenAsset:3`, nothing beside the target); `TestPlanRefusesCheckOnly`; `TestPlanUpToDate` (no manifest, no entry, and Apply's up-to-date result); `TestApplySharesRunsGuard`; `TestPlanDocumentRoundTrip`; `TestParsePlanRefuses`, 25 rows. `TestPlanDocumentGolden` in `plan_document_test.go` writes `plan.golden` and parses it back; `FuzzParsePlan` round-trips `Document`, and the `Makefile` fuzzes `./selfupdate` with `-m 6`; `ExampleUpdater_Plan`. A 20 s `go test -fuzz '^FuzzParsePlan$'` ran 536 948 inputs clean.

   **Seen failing**, each in a scratch copy with `go test -count=1 ./selfupdate/`, all caught and none a build failure; the five of step 7 first:

   | Plant | Failed |
   | :--- | :--- |
   | the release-ID compare dropped | `TestApplyRefusesChangedRelease/release_ID` |
   | the manifest-entry compare dropped | `TestApplyRefusesChangedRelease/SHA256SUMS_entry` |
   | the request match dropped | every row of `TestApplyRefusesAnotherRequest` |
   | the schema check dropped | `TestParsePlanRefuses/schema_2`, `/no_schema` |
   | `pin.Latest` replaced by `false` | `TestApplyRefusesChangedRelease/a_latest_plan's_rollback` |
   | `pinned` fetching the latest release | `TestApplyIgnoresNewerRelease`, and rows of `TestApplyRefusesChangedRelease` |
   | no confirmation with a pin | `TestApplyConfirms/approved`, `/declined` |
   | `Plan` accepting `CheckOnly` | `TestPlanRefusesCheckOnly` |
   | `Plan` downloading the manifest for `OperationNone` | `TestPlanUpToDate` |
   | `Apply` without the guard | `TestApplySharesRunsGuard` |
   | `Document` without `release_url` | `TestPlanDocumentGolden`, `TestPlanDocumentRoundTrip`, `ExampleUpdater_Plan` |
   | `Document` without `latest` | `FuzzParsePlan/seed#0` (`round trip: … latest: a plan is the latest exactly when no version was requested`) |
   | `Plan` reporting an event | `TestPlanReportsNothing`, `TestApplyEventsMatchRun` |
   | `Apply`'s `EventSelected` with another detail | `TestApplyEventsMatchRun` |
   | the latest consistency rule dropped | `TestParsePlanRefuses/latest_with_a_requested_tag` |
   | the `plan-stale` class dropped | `TestApplyRefusesChangedRelease`'s stale rows |
   | `Apply` with `CheckOnly` going on to install | `TestApplyCheckOnly` (`Applied:true … want ErrUpdateAvailable`) |

   `TestPlanDocumentGolden` also failed before its golden existed (`no such file`), and `-update` wrote it.
8. **`Run` unchanged:** `TestRunCallOrderFailureBoundaries`, `updater_contract_test.go` and `golden_test.go` are not changed, `git diff` of `testdata/golden/` shows only the new `plan.golden`, and the suite passes.
9. **Runs:** `go test -race -shuffle=on -count=3 ./selfupdate/` on this Mac `ok`; `go test -race -count=1 ./selfupdate/` on the Linux test host and the Windows test host `ok`; under Go 1.26.10, `go test ./selfupdate/` `ok` and `go vet` for the three targets clean.
10. **Checks:** `golangci-lint` for the three targets, 0 issues each after `manifestEntry` and D14; `make pre-add-check` on the Go files: clean; the architecture table: 20 rows, 0 differ.
11. **`make gate`**: every step rc=0, `apicheck` "compatible with v1.22.3", the fuzz step "6 fuzz targets ran clean in ./selfupdate", `FuzzParsePlan` among them, `overall=0`; `make markdownlint`, `make check-records` and `make docs-check` exit 0; `scripts/check-docs.sh --ids` on the 14 changed files: 0 findings.

### Q15: streams and test support (2026-10-29)

Files: from the list, `selfupdate/stream.go` and `stream_run_test.go`; `selfupdate/selfupdatetest/selfupdatetest.go`, `githubserver.go`, `conformance.go`, `selfupdatetest_test.go` and `conformance_test.go`. No `.go` file added or removed.

1. **`StartApply(ctx, u, req, p, opts...) *Stream`** and `Start` share `startStream`; `Stream.prepare` checks the plan after the Updater and before the options, as `Apply` does. `Start` still calls `execRun`, and `StartApply` `execPinned`, so `execRun`'s doc ("RunWith and Start both use it") stays true and `runoptions.go` is untouched. The helper is `startStream`, not `start`: revive's `confusing-naming` refuses a function that differs from `Start` only in case. Tests: `TestStartApplyConfirms` plans, publishes v1.2.0 on the `FakeSource`, drives the one `ConfirmNeeded` of an Apply stream (its prompt names v1.1.0), and sees v1.1.0 installed; `TestStartApplyRefusesBeforeRunning`: a nil Updater and an invalid plan each give only `Finished`, with no source call.
2. **`Publish(ReleaseSpec)`** on `FakeSource` and `GitHubServer`: the release takes IDs from a counter each fake now keeps, lists after every other, and is the latest unless it is a draft or a prerelease, as on GitHub; a tag already published is replaced with new IDs, and its old asset IDs are no longer served. `FakeSource.Latest` now reads the latest tag under the lock. `GitHubServer`'s handlers read the release list through `snapshot()`, under the lock, and `Publish` replaces the slice rather than changing it, so a handler never reads one being changed; `serveRelease` went into the handler. Tests: `TestFakeSourcePublish` and `TestGitHubServerPublish`, the second ending with a loop of requests while 50 releases are published.
3. **`VerifyReleaseSource`** also requires `ByTag(LatestTag)` to return `Latest`'s release ID and asset IDs, and its doc says `Updater.Apply` compares them. `TestVerifyReleaseSourceFails` gains a source whose release ID changes on each call. The migration guide is not in this phase's list; Q17 step 3 names the check there.
4. **End to end:** `TestPlanSurvivesNewerReleaseOnGitHub` in `stream_run_test.go`, over `NewGitHubServer` and a `GitHubSource`: `Plan` for v1.1.0, `Publish` of v1.2.0, `Apply` installs v1.1.0, and a `Run` from v1.1.0 then installs v1.2.0.

   **Seen failing**, each in a scratch copy with `go test -count=1 ./selfupdate/ ./selfupdate/selfupdatetest/` (the lock plant with `-race` on `selfupdatetest`), all caught on the final tests:

   | Plant | Failed |
   | :--- | :--- |
   | `StartApply` running unpinned (`execRun`) | `TestStartApplyConfirms` |
   | `StartApply` without the plan check | `TestStartApplyRefusesBeforeRunning` |
   | `FakeSource.Publish` not moving the latest | `TestFakeSourcePublish` |
   | `FakeSource.Publish` numbering from zero | `TestFakeSourcePublish` |
   | `GitHubServer.snapshot` without the lock | `TestGitHubServerPublish`, `WARNING: DATA RACE` (three runs of three) |
   | `GitHubServer.Publish` keeping the old release of a tag | `TestGitHubServerPublish` |
   | `VerifyReleaseSource` without the ID check | `TestVerifyReleaseSourceFails` |
   | `Apply` fetching the latest release | `TestPlanSurvivesNewerReleaseOnGitHub`, `TestStartApplyConfirms`, and Q14's `TestApplyIgnoresNewerRelease` |

   The lock plant first passed: the test then published once while 20 requests ran, and each handler takes the lock in `record` before it reads, so the race needs a publish between the two. The test now publishes 50 releases while requests run, and the plant fails each time.
5. **Runs:** `go test -race -shuffle=on -count=3` of `./selfupdate/` and `./selfupdate/selfupdatetest/` on this Mac `ok`; `go test -race -count=1` of both on the Linux test host and the Windows test host `ok`; under Go 1.26.10, `go test` of both `ok` and `go vet` for the three targets clean.
6. **Checks:** `make pre-add-check` on the 7 Go files: clean, after the rename; the architecture table: 20 rows, 0 differ; `make gate`: every step rc=0, `apicheck` "compatible with v1.22.3", `overall=0`; `scripts/check-docs.sh --ids` on the changed files: 0 findings.

### Deviation D15 (2026-10-29): the backends' carriers are tested in their own packages

* **Found:** Q16 step 3 requires the plan's environment entry to reach each backend's carrier, "with the fakes those backends' handoff tests use". The carriers differ: systemd writes an `EnvironmentFile` with its own escaping (`systemd/detach.go:240`), launchd and Task Scheduler the private file of `service.WriteHandOffEnv` (`launchd/detach.go:102`, `taskscheduler/handoff.go:81`), and scm the process environment (`scm/detach.go:58`). Those fakes and tests are in `systemd/detach_unix_test.go`, `launchd/detach_test.go`, `taskscheduler/handoff_test.go` and `scm/detach_test.go`, none in Q16's list.
* **Decision:** the owner, 2026-10-29, chose "Add the 4 test files": each gains a test that a `HandOff` whose `Env` holds `EnvHandOffPlan` with a real plan document reaches its carrier, read back as the same plan where the test can read it. Tests only; no MADR change.
* **Files added to Q16:** `selfupdate/service/systemd/detach_unix_test.go`, `selfupdate/service/launchd/detach_test.go`, `selfupdate/service/taskscheduler/handoff_test.go`, `selfupdate/service/scm/detach_test.go`.

### Q16: the handoff carries the plan (2026-10-29)

Files: from the list, `selfupdate/service/handoff.go` (`HandOffFunc`'s doc names `HandOffPlanFunc`), the new `handoffplan.go` and `handoffplan_test.go`; `selfupdate/cli/run.go`, `doc.go`, `handoff_test.go` and `example_handoff_test.go`; the package table (`service` 21 to 22 files, 28 to 29 test files). Added by D15: the four backends' handoff test files.

1. **`service`:** `EnvHandOffPlan`; `HandOffPlanFunc(d, spec)`, which encodes the plan's `PlanDocument` compactly, adds `EnvHandOffPlan=<document>` to a copy of `spec.Env`, and returns `HandOffFunc`'s hook for that spec, so its rules hold, the refusal of a request without `Yes` inside the service included; `LoadHandOffPlan()`, which unsets the variable, treats an empty one as none, and parses the rest with `selfupdate.ParsePlan`. The empty case was found when the first `cli` run failed `selfupdate: plan: EOF` on a variable a test had set empty.
2. **`cli.HandOff`:** `DetachPlan` and `Planned`. `Run` first asks `Planned`; with a plan it applies it with `Updater.Apply`, and a load error goes through `Report` and fails the run. Otherwise `detach` runs `DetachPlan`, or else `Detach` as before: `DetachPlan` calls `Updater.Plan`, hands off only a plan whose operation is not `OperationNone`, and returns that plan, which `Run` applies when nothing was handed off; when `Plan` fails, `Run` goes on without a plan and reports what it reports today. The fallback is written as `if p, perr := u.Plan(ctx, req); perr == nil { … }`, as `nilerr` refuses `if perr != nil { return …, nil }`. With neither field set, every `cli` test and golden passes unchanged.
3. **Tests.** `service`: `TestHandOffPlanFuncAddsThePlan` (the entry after `spec.Env`'s, compact, parsing back to the plan, `spec.Env` unchanged); `TestHandOffPlanFuncRules`; `TestLoadHandOffPlan` (unset, empty, valid and unset after, a tampered `schema_version` refused and unset); `TestHandOffPlanThroughEnvFile` (`WriteHandOffEnv`, `LoadHandOffEnv`, then `LoadHandOffPlan`). The backends (D15), each with a plan whose URL holds `"`, `&`, `$` and `\`: systemd's `EnvironmentFile`, unquoted as systemd reads it; launchd's private file, and not its plist; Task Scheduler's private file, and not its definition; scm's real run, whose environment the test binary writes out. `cli`: `TestHandOffCarriesThePlan` (the parent inside the service hands off through `HandOffPlanFunc`; the detached run, with v1.2.0 published, installs the parent's v1.1.0 through `LoadHandOffPlan`); `TestDetachPlanAppliesHere`; `TestDetachPlanOnlyForAnInstall` (check, dry run, up to date); `TestDetachPlanFallsBack` (a failing source: the same exit and stderr as without `DetachPlan`); `TestPlannedRefusals` (a plan with another release ID is `ErrPlanStale`, and a load error fails the run, each through `Report`, nothing installed). `ExampleHandOff_plan`.

   **Seen failing**, each in a scratch copy with `go test -count=1 ./selfupdate/cli/ ./selfupdate/service/...`, all caught on the final code; the two of step 3 first:

   | Plant | Failed |
   | :--- | :--- |
   | `HandOffPlanFunc` without the env entry | `TestHandOffPlanFuncAddsThePlan`, `TestHandOffCarriesThePlan` |
   | `Run` ignoring `Planned` | `TestHandOffCarriesThePlan`, `TestPlannedRefusals` |
   | `DetachPlan`'s plan not applied here | `TestDetachPlanAppliesHere` |
   | a `Plan` error failing the run | `TestDetachPlanFallsBack` |
   | `DetachPlan` for an up-to-date plan | `TestDetachPlanOnlyForAnInstall` |
   | `LoadHandOffPlan` leaving the variable set | `TestLoadHandOffPlan` |
   | an empty variable read as a plan | `TestLoadHandOffPlan`, `TestHandOffCarriesThePlan` |
   | systemd's `EnvironmentFile` without the `\` and `"` escapes | systemd's `TestDetachCarriesPlan`, `TestEnvFileBody` |
   | launchd putting every variable in the plist | launchd's `TestDetachCarriesPlan`, `TestDetach` |
   | Task Scheduler's file without `spec.Env` | `TestDetachOneShotCarriesPlan`, and two existing tests |
   | scm dropping the plan's entry | scm's `TestDetachCarriesPlan` |

4. **Runs:** `go test -race -shuffle=on -count=3` of `./selfupdate/cli/` and `./selfupdate/service/...` on this Mac `ok`; `go test -race -count=1` of both on the Linux test host and the Windows test host `ok`, the Windows run of the carrier tests passing scm's through a real hop and real run; under Go 1.26.10, `go test` of both `ok` and `go vet` for the three targets clean.
5. **Checks:** `make pre-add-check` on the 11 Go files: clean, after the `nilerr` change; the architecture table: 20 rows, 0 differ; `make gate`: every step rc=0, `apicheck` "compatible with v1.22.3", `overall=0`.

### Q17: documents (2026-10-29)

Files: from the list, `docs/architecture.md`, `docs/guides/extending-selfupdate.md`, `docs/guides/migrating-from-mcplib-selfupdate.md`, `CHANGELOG.md`, `selfupdate/doc.go`, `selfupdate/service/doc.go` and `README.md`. `selfupdate/cli/doc.go` needed no change: Q16 gave it the plan's paragraph.

1. **`architecture.md`:** a "Plans" bullet by file (`plan.go`; `Apply` and `StartApply`; the shared `fetchRelease`, `vet` and `manifestEntry`; the handoff's `handoffplan.go` and `cli`'s `DetachPlan` and `Planned`); `servicetest` in the reference lifecycles, with how each backend runs it; systemd's crashed start as `ErrUnhealthy`; `selfupdatetest`'s `Publish` and the ID check; the handoff's plan in `service`'s bullet and `cli.Options.HandOff`'s; the failure classes, `plan-stale` among them in its place; the install path's root-relative names and the rollback's same-file backup; the trust model, what `Apply` proves and that a plan is unsigned and carries no approval; and "Tests on the production clock" under Tooling. The package table, its test counts and goldens, and the depguard count (22, `service-servicetest` among them) were already current from Q9 to Q16, and the architecture check confirms them.
2. **`extending-selfupdate.md`:** "Show an update banner" (install what the banner showed, from its tag or a plan); "Read JSON output" (`PlanDocument`'s keys, `ParsePlan`'s refusals, and the `plan-stale` class); "Drive an update from a TUI" (`Plan`, show, `StartApply`); "Run as a service" (systemd's crashed start); "Updating from inside the service" ("Hand off the plan", with `DetachPlan` and `Planned`, and `ExampleHandOff_plan`); a new "Check your Lifecycle" under "Run as a service", after the handoff; "Use another release host" (stable IDs, and the check); "Test a program" (`Publish`, `servicetest`, and the end-to-end test).
3. **The migration guide:** `## 23. From v1.22 to v1.23`, with "What is new", "Behaviour you may notice" (the root-relative install; each F11 fix and Amendment A4; `VerifyReleaseSource`'s ID check, which Q15 step 3 left here), "Adopting it" and "Check". Its `go get` names `v1.23.0`; the guide's other pins, and the README's, move after the tag (Q18).
4. **`CHANGELOG.md`:** `v1.23.0`, dated 2026-10-29, with Added, Changed and Fixed, linking §23, the MADR and Q9 to Q16; Q18 moves the date if the tag is cut on another day.
5. **Package docs:** `selfupdate/doc.go` gains "Plans", `servicetest` and `Publish`; `service/doc.go` the plan's handoff. **README:** two rows in "I want to…", for a banner's release and for checking a `Lifecycle`.
6. **Checks:** `make markdownlint` 0 issues, after one line of §23 was wrapped (MD013, 220 characters); `make docs-check`: 866 links in 101 files, 0 broken, the table 0 differ; `make check-records` 0 findings; `make pre-add-check` on the two doc files: clean; `scripts/check-docs.sh --ids` on the changed files: 0 findings.
7. **`make gate`**: every step rc=0, `apicheck` "compatible with v1.22.3", `overall=0`.
