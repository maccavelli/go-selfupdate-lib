---
status: accepted
date: 2026-10-27
decision-makers: go-selfupdate-lib maintainers
consulted: "a read of `selfupdate`, `selfupdate/cli`, `selfupdate/service` and its backends on `main` at 08e41cb (v1.22.2 and its pin commit); 0025-MADR-align-with-go-sdk-design-practices.md's scorecard; 0035 and 0037, which found the bugs F3 and F4 cite; the Go 1.27.3 toolchain's `go doc` and `api/go1.*.txt`; the pkg.go.dev pages of three other Go self-update libraries; probe P1, in a scratch clone, on 2026-10-27; after the owner's answers, three read-only surveys of the code against each option, with probes P2 to P5 in scratch clones, on 2026-10-27"
informed: the fleet's programs that call `selfupdate`, run `cli.Command` with a handoff, or implement `selfupdate.Lifecycle`
---
<!-- markdownlint-disable MD004 MD013 MD024 MD060 -->

# Improve the API and its implementation three ways: a pinned update plan, a lifecycle conformance check, and tests whose clock is the one production uses

## Context and Problem Statement

On 2026-10-27 the owner asked: "find me 3 ways we can improve this library, focusing on its API design and implementation. present options as an madr for review. be creative. do research. evaluate go API best practices and standards."

[0025-MADR-align-with-go-sdk-design-practices.md](0025-MADR-align-with-go-sdk-design-practices.md) already scored the module against a list of Go SDK practices, and its decisions shipped. This record does not repeat that list. It looks for places where the design still costs a caller or a maintainer, using evidence from this repository's own history, and Go standard library features that the module's floor, `go 1.26.10`, now allows.

It proposes three improvements, each with alternatives. Each can be accepted or rejected on its own.

The owner answered this record's four questions the same day (Decision Outcome), then asked: "assess the codebase and evaluate madr 0038 and determine the best, most idiomatic, flexible, and compatible way to implement these proposed changes, update the madr if needed, write the actionable, detailed, and comprehensive plan." That assessment corrected several facts and sharpened each option. Its results are folded into the facts and options below, and More Information lists what changed from the draft the owner reviewed.

### Method

* Read the code at `08e41cb`, and the records the findings cite.
* Read the Go standard library's documentation from the Go 1.27.3 toolchain that `go.mod` names (`go doc`), and checked which release added each API in that toolchain's `api/go1.*.txt` files.
* Read the Go blog and release notes, and the pkg.go.dev pages of three other Go self-update libraries (More Information).
* Ran probe P1 in a scratch clone.
* After the owner's answers, surveyed the code once per option, and ran probes P2 to P5 in scratch clones (More Information). The repository was not modified by any probe: `git status --short` showed only this record and its index row before and after each.

Evidence tags: **[read]** a code or record read at `08e41cb`; **[docs]** documentation, cited; **[probe]** measured in a probe; **[derived]** reasoning from the cited facts.

### Facts

* **F1. A check and the apply after it each resolve the release separately.**
  * `cli.Command` decides whether to hand off by running a full `Checker.Check` (`selfupdate/cli/run.go:140`, `:175-184`), and then either runs the update itself or hands it off.
  * The detached run gets the same arguments, `os.Args[1:]` (`service.HandOff.Args`). Unless the user passed `--version`, it resolves the latest release again, in another process, seconds or minutes later [read].
  * `Availability` carries the target's tag, release URL and asset name, but no release ID and no digest (`selfupdate/checker.go:79-98`). So a host that shows "v1.4.0 is available", and later calls `Run`, cannot tell `Run` to install that release and no other. `Request.TargetVersion` pins the tag, but nothing in the module passes the checked tag on: not `cli`, and not `Stream` [read].
  * So the release that justified the handoff, or that the user saw in a banner, need not be the one installed. Each resolution is also another API request against the rate limit [derived].
  * Within one `Run`, the release is resolved once, and the `Confirmer` is asked about that release. The order is fixed: discovery and selection (`updater.go:178-200`), the `Confirmer` (`:212-227`), then the session begins (`:238`), and only inside it the `SHA256SUMS` download, its entry and the `ManifestVerifiers` (`:257-276`), then the binary (`:279-291`). `TestRunCallOrderFailureBoundaries` (`updater_contract_test.go:178`) and the declined goldens (`testdata/golden/*-declined.golden`) pin that order: a declined run downloads no manifest [read].
  * `Release.ID` is documented as an identifier "the Updater passes back to OpenAsset and reads no further" (`types.go:244-246`). Releases are always required to be immutable (`checker.go:212`) [read].
* **F2. Other Go self-update libraries pass a value from the find step to the apply step** [docs]:
  * `creativeprojects/go-selfupdate`: `DetectLatest(ctx, repository) (*Release, bool, error)`, then `(*Updater).UpdateTo(ctx, rel *Release, cmdPath string) error`.
  * `minio/selfupdate`: `PrepareAndCheckBinary(update, opts)`, then `CommitBinary(opts)`, with the same `Options`.
  * `fynelabs/selfupdate`: `Source.LatestVersion() (*Version, error)`, then `Source.Get(*Version)`.

  This module's `Release` and `Selection` are exported, but they reach a caller only inside a `Stream` interaction or a `Result`, never as something the caller can hand back to apply.
* **F3. Only one of the contracts a caller implements has a conformance check.**
  * `selfupdatetest.VerifyReleaseSource` checks a `ReleaseSource` against the contract the `Updater` relies on (`selfupdate/selfupdatetest/conformance.go:30-45`; 0025-MADR D11). It takes no `*testing.T`, collects every violation, and returns them joined, as `testing/fstest.TestFS` does for `io/fs`.
  * `selfupdate.Lifecycle` has a longer and subtler contract (`types.go:589-603`). Its own godoc states what the caller does: `Running` is asked again after a failed `Stop`, and a recovery runs on its own 2-minute context. Each backend documents the rest: an issued stop outlives the caller's context (0015-MADR B3), a stop that does not finish is `service.ErrTimeout` at a documented bound, and a missing service is `ErrNotInstalled`. Context handling is per method: scm documents that `Installed`, `Running` and `Enabled` ignore their context (`scm/service.go:57-59`), and Task Scheduler the same plus `Start` (`taskscheduler/task.go:107-111`) [read].
  * Programs implement that contract themselves: magic-cli-remote did for runit, s6 and OpenRC before its 0212 record moved them onto `service.Supervised`, and `Supervised`'s `State`, `Up`, `Down` and `Kill` callbacks are the same contract in smaller pieces [read].
  * In the last two releases, five defects were divergences from that contract, and each was found by reading the code. No check failed for any of them:
    * 0035-MADR Amendment A3: `Supervised.Stop`'s restore ran on an ended context;
    * 0037-MADR F1: two fakes ignored their context;
    * F2: systemd's stop command had no bound;
    * F3: launchd's `bootout` ran on the caller's context;
    * F4: launchd's bootstrap timeout was not `ErrTimeout`.

    The fixes added tests for each case in each backend, but nothing a new implementation can run [read].
* **F4. The tests keep their own clocks, and those clocks have hidden real deadlines.**
  * Five test clocks: `fakeClock`, which swaps the package variable `pollClock` (`selfupdate/service/poll_test.go:15`, `:26-33`; `poll.go:78-89`); `vclock` (`supervised_lifecycle_test.go:156`); the clock `healthOver` steps by one interval per `State` read (`supervised_health_test.go:57-77`); and a `virtualClock` each in `scm/fake_test.go:233` and `taskscheduler/fake_test.go:381`.
  * Production structs carry `now` and `sleep` fields only so tests can replace them: `service.Supervised` (`supervised.go:130-136`, "tests replace them"), `scm.Service` (`scm/service.go:63-64`) and `taskscheduler.Task` (`taskscheduler/task.go:115-116`). Nothing outside a test sets them, or `pollClock`. Only `Supervised`'s fields are named in a record (0035-PLAN, M4); the scm and Task Scheduler fields came with their backends' first commits [read].
  * A virtual clock does not move a `context.WithTimeout` deadline, so the two clocks disagree:
    * 0035-MADR Amendment A3: "M4's tests did not see it. They run on a virtual clock, which never reaches the context's real deadline."
    * 0035-PLAN Deviation D4: a settle window measured on wall time failed on a loaded Windows CI runner. The owner chose "Virtual clock, in M11" then, which is `healthOver`'s clock.
    * 0037 added three tests on the real clock, at 300 to 400 ms each, to see deadlines at all.
    * P3 found a fourth instance, in a fixture: scm's `TestStopWaits` asserts a 5 s wait under a 2 s `Poll.Timeout` (`scm/fake_test.go:274-276`, `scm_test.go:178-181`). On the virtual clock it passes; on the production clock it times out [probe].
  * The test files call `time.Sleep` 57 times. Most are in live tests, helper programs and waits on files a real child writes; a bubble removes two (`supervised_lifecycle_test.go:280`, `stream_test.go:89`) [read].
* **F5. `testing/synctest` gives a test one clock for everything** [docs]:
  * `synctest.Test` and `synctest.Wait` were added in Go 1.25 (`api/go1.25.txt`, #67434), below the floor, and need no `GOEXPERIMENT`. `synctest.Sleep` was added in Go 1.27 (#77169), above it.
  * Inside a bubble, the `time` package uses a fake clock, and timers, `time.Now` and context deadlines all follow it. Time advances only when every goroutine in the bubble is durably blocked.
  * A goroutine blocked on a mutex, on I/O or in a system call is not durably blocked. A bubble therefore suits code driven by fakes, and does not suit tests that run real processes, real sockets (`httptest`) or a real service manager.
  * `T.Run`, `T.Parallel` and `T.Deadline` must not be called inside a bubble, so a table test makes one bubble per subtest. `synctest.Test` fails when a goroutine started in the bubble is still blocked at its end, so it is a leak check for the bubble.
  * Timers that fire at the same instant fire in no fixed order. P4 measured a check that depended on it failing 15 of 40 runs [probe].
* **F6. Probe P1: one bubble test catches the v1.22.0 restore bug, with the production clock** [probe]. The probe put `TestSupervisedStopRestoresOnRealClock`'s body inside `synctest.Test`. It kept `Supervised`'s production `now` and `sleep`, and raised `Poll.Timeout` from 400 ms to 60 s, the default.

  | Code under test | Result | Wall time |
  | :--- | :--- | :--- |
  | `08e41cb` (v1.22.2) | PASS. Fake time 1m0s; `timed out: demo still runs 1m0s after it was stopped (last: up=true pid=100)`; calls `["down" "up"]` | test 0.00 s; package 0.69 s |
  | `supervised_lifecycle.go` from `v1.22.0` | FAIL. `start again after the failed stop of demo: context deadline exceeded`; calls `["down"]`, want up false | test 0.00 s |

  The real-clock test needs a short timeout, and spends at least 2 × 400 ms of wall time. The bubble test uses the documented default and takes no wall time.
* **F7. `selfupdate`'s test seams are package state, and most belong to the installer.**
  * 24 package-level function variables in `selfupdate` exist to be replaced in tests. 22 are; `osRename` (`replace_unix.go:14`) and `evalSymlinks` (`target.go:14`) never are.
  * 68 `setSeam` calls in 24 test files replace them, and helpers replace them for their callers: `withTempHome` sets `userHomeDir` for 92 call sites in 33 files, `noEnv` sets `lookupEnv` for 23, `cacheEnv` sets `timeNow` for 13. `setSeam`'s comment says why the package has no parallel tests: "Seams are package state, so no test in this package calls t.Parallel" (`install_hardening_test.go:37-40`).
  * 17 of the 22 are filesystem effects of `StandaloneInstaller` and its session (`replace.go:13-21`, `replace_unix.go:14-15`, `replace_windows.go:88`, `journal.go:51,54`, `leftovers.go:18`, `lock.go:11`, `session.go:573`), reached through `Begin` and `CleanupPending` without an `Updater`. The rest are target resolution (`osExecutable`, `userHomeDir`), the clock (`timeNow`, which the public `Config.Clock` and `CheckerConfig.Clock` already cover), the environment (`lookupEnv`), the terminal (`isTerminal`) and the platform (`runningPlatform`).
  * Even without package seams, `testproc.CheckNoLeak` compares goroutines process-wide (`internal/testproc/testproc.go:225-251`), and `t.Setenv` and `t.Chdir` refuse a parallel test, so the tests that use them stay sequential.
  * The repository's 11 `t.Parallel` calls are all in `internal/cmd/selfupdate-release` [read].
* **F8. The replace path works by path name, beside a session that already holds the directory open.**
  * Each install session holds an `*os.Root` on the target's directory, opened under the lock, and checked once to be the directory that was locked (`session.go:597-620`). It already uses the root for `Lstat`, the journal, the kept rename and the rollback rename (`:104`, `:339`, `:484-490`, `:533-536`), and lock files (`lock.go`).
  * The replacement itself does not use the root:
    * `replacePath` is `os.Rename` on full paths (`replace_unix.go:43-44`, called at `:80`, `:87`, `:122`; `session.go:227`);
    * the backup is a hard link, `os.Link`, or a copy through `openAbsFile`, at a name `randomSibling` reserves with `os.CreateTemp` and `os.Remove` (`replace.go:18`, `:73`; `replace_unix.go:68-72`);
    * the staging file is made with `os.CreateTemp(target.Dir, …)` (`session.go:98`), owned with `os.Lchown` (`replace_unix.go:26`) and moded with `os.Chmod` (`replace.go:84`); setuid bits are cleared with `os.Chmod` (`session.go:511`);
    * the target is read by `os.Lstat` and `fileSHA256` on its path (`replace_unix.go:52`, `:64`);
    * the backup and staging are removed with `os.Remove` (`replace_unix.go:109`; `session.go:225`, `:553`);
    * the directory is flushed by `syncDirFn(target.Dir)`, which opens the directory again by name (`replace.go:95-101`; `replace_unix.go:83`, `:112`, `:125`), where `syncRootFn` already flushes through the root (`replace.go:105-118`).
  * Each of those calls looks the directory up again by name. **Probe P2** shows the consequence on `08e41cb`: a directory swapped after the target is checked and before the rename sends the rename into the swapped-in directory, whose target becomes the decoy's staging file; the session then sees the change, rolls back, and returns `ErrConcurrentUpdate` [probe]. That is the class of 0015-MADR B2, there for the target file.
  * P2 also showed that the rollback renames a hard-link backup over a target that is the same file; POSIX `rename` does nothing then, so the backup stays until the next session's leftover sweep [probe].
  * Go 1.25 added `Root.Rename`, `Link`, `Chmod`, `Lchown`, `Chown`, `Chtimes`, `Symlink`, `Readlink`, `MkdirAll` and `RemoveAll` (`api/go1.25.txt`, #67002). `os.Root` has no `CreateTemp`.
  * `os.Root`'s documentation states that on Unix, `Root.Chmod`, `Chown` and `Chtimes` can act on a link swapped in during the call. On Linux, `Root.Chmod` uses `fchmodat2` (kernel 6.6 and later) and otherwise goes through `/proc/self/fd`, which fails where `/proc` is not mounted (`internal/syscall/unix/fchmodat_linux.go` in the toolchain). `os.Chmod` on a path has neither limit, and `(*os.File).Chmod` on a descriptor has neither race.
  * The Linux `Root.Chmod` escape, CVE-2026-32282, was fixed in Go 1.25.9 and 1.26.2, both below the floor [docs].
  * On Windows, `Root.Rename` uses `FileRenameInformationEx` with replace-if-exists and POSIX semantics (`internal/syscall/windows/at_windows.go:458-460` in the toolchain). It has no write-through flag and no retry while the image is busy. `moveFileReplace` has both (`replace_windows.go:88-125`; 0003-MADR B10, B11) [read].
* **F9. Probe P3: most fake-driven backend tests already run in a bubble** [probe]. Every test of `service`, scm, Task Scheduler, systemd and launchd that builds on darwin, 247, was wrapped unchanged in `synctest.Test`. 231 passed, in 8.4 s of wall time against 16.1 s outside. The 16 that failed fell into four kinds: seven call `t.Run` inside the bubble; five start real processes or wait on files a child writes; two poll every nanosecond (`systemd_test.go:185`, `:255`), which is twenty million fake steps; and one, Task Scheduler's `TestDetachOneShotFailures`, keeps a virtual clock whose sleep never blocks, so bubble time never moves and it hangs. A test therefore moves into a bubble in the same step that it drops its virtual clock. With the clocks removed and the fakes stepping per observation, scm's and Task Scheduler's stop tests pass at the 60 s default in no wall time, and under `-race`.
* **F10. Probe P4: a prototype `servicetest` catches the contract defects, given two checks the draft did not list** [probe]. A prototype `VerifyLifecycle` and `VerifyRunner`, run over fake labs for systemd, launchd and `Supervised` inside bubbles, passed on `08e41cb` and failed on planted copies of 0037-MADR F1, F2, F3 and F4. F4 needs a check that a start the manager never completes is `ErrTimeout`. 0035-MADR Amendment A3 needs a check that a timed-out stop leaves the service still wanted up, with a fake `Down` that takes time; the faithful break is the `v1.22.0` file (40 of 40 runs fail, 0 of 40 on `main`), because a one-line plant failed only 25 of 40. `VerifyRunner` needs to be told which command answers at once and which blocks.
* **F11. The prototype found places where a backend does not do what its documentation says** [probe, read]:
  * scm's `Start` on an ended context issues the start, then returns `ErrTimeout` wrapping `context.Canceled` (`scm/lifecycle.go:242-262`). scm's documentation exempts `Installed`, `Running` and `Enabled` from their context, not `Start`.
  * launchd's `Installed` ignores its context (`launchd/lifecycle.go:21-31`), and its documentation does not say so, where scm's and Task Scheduler's do.
  * launchd's `Stop` bounds `bootout` by the stop's bound, then waits for the job by the same bound again (`launchd/lifecycle.go:178`, `:192`), so a stop can take twice what its documentation states (`:134-139`). The prototype's stop of a job that ignores it took 62 s against a 60 s bound.
  * launchd's `Start` documents `ErrNotInstalled` (`:258`), but on a missing plist it runs `enable`, then `bootstrap`, which retries a failing `launchctl` until `ErrTimeout` (`:324-329`). What `launchctl bootstrap` returns for a missing plist was not measured.
  * `Supervised.Start` runs the program's `Up` for a service its supervisor reports as not installed (`supervised_lifecycle.go:163-175`), where `Stop` returns `ErrNotInstalled` (`:50`). Its documentation promises nothing either way.
  * Two test fakes outside 0037's scope ignore their context: `codesign/codesign_test.go:25` and `verify/ghattest/ghattest_test.go:43`. 0037-MADR D1-A's rule is that every fake fails a call on an ended context.
* **F12. Probe P5: the D1 API is additive, with two constraints** [probe, read].
  * Stub `Plan`, `PlanDocument`, `Updater.Plan`, `Updater.Apply`, `StartApply` and new `cli.HandOff` fields compiled for linux, darwin and windows, and `apidiff` against `v1.22.2` reported compatible changes only. A method `Plan` beside a type `Plan` compiles and vets, as `time.Time.Month() Month` and `reflect.Value.Kind() Kind` do in the standard library.
  * A slice field added to `Availability` made `apidiff` report "old is comparable, new is not" for `Availability` and `CheckRecord`. A `Plan` that might one day ride in either must stay comparable: no slices or maps.
  * `selfupdatetest.NewGitHubServer` cannot publish a release, change a digest or flip `Immutable` after it is built (`githubserver.go:116-152`).
  * `selfupdate/cli` may import only the standard library, `x/term`, `selfupdate` and `buildinfo` (`.golangci.yml:114-123`), so it cannot call `service` itself. `ArgHandOffEnv` is Task Scheduler's carrier only (`service/handoffenv.go:20-26`); every backend carries `HandOff.Env`: systemd in a private `EnvironmentFile`, launchd and Task Scheduler in a private file `LoadHandOffEnv` reads and deletes, scm and `ProcessDetacher` in the process environment.
  * Adding `MarshalText` to `Operation` or `BuildKind` would change the JSON of `Result`, `Availability`, `Event` and `CheckRecord`, which encode them as numbers today. Documents pair a typed value with a tagged document type instead (`document.go`, `ResultDocument`).

## Decision Drivers

* **What the user approved is what gets installed.** A check, a confirmation and a handoff should name one release, not three resolutions of "latest".
* **A contract a program implements should be one the program can check**, as `VerifyReleaseSource` made the source contract.
* **Tests should see time as production does.** The bugs in F4 lived in the gap between a test clock and a context deadline.
* **Additive within v1** (0025-MADR D6): `make apicheck` stays compatible, no exported identifier is removed or changed, an interface a caller implements never gains a method, and a new capability is a new type, method or optional interface.
* **`Run`'s observable behaviour does not change**: its call order, its events and its goldens (F1).
* **No new module requirement** (depguard, AGENTS.md). Everything below is standard library.
* **The floor stays `go 1.26.10`.** Library and test code use only APIs from Go 1.26 or earlier, so `synctest.Sleep`, generic methods, `encoding/json/v2` and `crypto/mldsa` are out of reach. The `go126` CI job enforces it.
* **The owner prefers a complete, extensible surface,** with exported fields and JSON tags where a value is a document, to cuts made only for scope.

## Considered Options

### D1: a pinned update plan (F1, F2, F12)

* **D1-A. `Plan` and `Apply`.**
  * `selfupdate.Plan`, a comparable struct: the request it was made for (product, current version and build, normalized platform, channel, the requested tag, `Force`); the resolved release (`TargetVersion`, `ReleaseID`, `ReleaseURL`, and `Latest`, whether it was resolved as the latest or the channel's newest, without which a re-fetch by tag would classify an older "latest" as a rollback, `version.go:236-268`); `Operation`; the `Selection`; and `ManifestSHA256`, the binary's entry in `SHA256SUMS`.
  * `PlanDocument` and `AssetDocument`, its JSON form, in `ResultDocument`'s style: snake_case tags, `schema_version`, strings for the operation and build kind. `Plan.Document()`, `ParsePlan(data) (Plan, error)` (a size cap, unknown fields, trailing data and any schema other than `PlanSchema` refused, as `releasespec.Parse` does), and `const PlanSchema = 1`. The document carries no `Yes` and no `DryRun`, so approval does not travel with a plan.
  * `(*Updater).Plan(ctx, Request, ...RunOption) (Plan, error)` runs validation and discovery, then downloads `SHA256SUMS`, finds the binary's entry and runs the `ManifestVerifiers`. It takes no lock, resolves no target, reports no events and downloads no binary. `WithCredentials` reaches a private repository.
  * `(*Updater).Apply(ctx, Request, Plan, ...RunOption) (Result, error)` refuses, before any network call, a request that does not match the plan (`ErrPlanStale`). It then fetches the release by tag, and refuses any difference from the plan with `ErrPlanStale`: another release ID, other assets or digests, another operation, or another `SHA256SUMS` entry. A failure that is also `ErrMutableRelease` or `ErrIntegrity` matches both. It asks the `Confirmer` unless the request says `Yes` or `DryRun` (owner answer 1), and then downloads, verifies, installs and reports exactly as `Run` does, under the same one-run guard.
  * `Run` keeps its order and its events. `Run` and `Apply` share one internal pipeline, with the plan's pins as an option, and `Plan` shares its discovery and manifest steps, so the two paths cannot disagree (0004-MADR G3's rule, carried over) without `Run` downloading a manifest before its confirmation (F1).
  * `Plan.Request() Request` returns the request the plan was made for, without `Yes` or `DryRun`, so a host builds `Apply`'s request from the plan.
  * `StartApply(ctx, *Updater, Request, Plan, ...RunOption) *Stream` is `Start` for `Apply`, so a TUI can show a plan, confirm it and apply it. A plan is a required input, and `RunOption` only swaps collaborators (`runoptions.go:9-11`), so it is a function, not an option.
  * The handoff carries the plan as the environment variable `service.EnvHandOffPlan`, which every backend already delivers through `HandOff.Env` (F12). `service.HandOffPlanFunc` builds the detaching hook with the plan added; `service.LoadHandOffPlan` reads it, unsets it and parses it in the child. `cli.HandOff` gains `DetachPlan`, which replaces `Detach` when set and makes `cli.Run` plan once, and `Planned`, through which a detached run applies the parent's plan and never resolves again. With neither set, `cli` behaves as today.
  * `selfupdatetest` gains `(*GitHubServer).Publish` and `(*FakeSource).Publish`, so a test can publish a newer release between `Plan` and `Apply`. `VerifyReleaseSource` also checks that `ByTag` of the latest tag returns the same release and asset IDs as `Latest`, which `Apply` relies on, and `Release.ID`'s documentation says so.
* **D1-B. Pins, without a new flow.**
  * `Availability` gains `ReleaseID` and `AssetDigest`.
  * `Request` gains `ExpectReleaseID` and `ExpectDigest`. `Run` refuses a resolved release or binary that does not match them, with `ErrPlanStale`.
  * `cli.Command` passes the checked tag and pins into the run and the handoff.
  * This is smaller, and a host has to copy the fields across itself.
* **D1-C. Documentation only.** The guides tell a host to pass `Availability.TargetVersion` as `Request.TargetVersion`, and `cli` does so. The tag is pinned, and its digest is not.

### D2: a lifecycle conformance check (F3, F10, F11)

* **D2-A. An exported `selfupdate/service/servicetest` package**, the sibling of `selfupdatetest.VerifyReleaseSource` for the service contract:
  * It lives beside `service`, not in `selfupdatetest`, whose depguard rule allows only `selfupdate`: putting it there would pull `service`, `internal/winproc`, `internal/procfs` and `x/sys/windows` into every program that imports `selfupdatetest` for its fake source.
  * `VerifyLifecycle(ctx, selfupdate.Lifecycle, LifecycleCase) error` returns every violation, joined. It takes no `*testing.T`, so it runs from any harness, in a `synctest` bubble over a fake or on the real clock over a real manager.
  * `Lab` is the interface the implementer provides to set the service's state and to see it without the `Lifecycle`: `Arrange(ctx, Arrangement) error` and `Observe(ctx) (Observation, error)`. The states are values of `State` (`Absent`, `Stopped`, `Running`, `SlowStop`, `Stubborn`, `Crashing`, `StartHangs`), so a new state is a new constant and `Lab` never gains a method. A lab returns an error wrapping `errors.ErrUnsupported` for a state it cannot arrange; the checks that need it are skipped and logged, and `LifecycleCase.RequireAll` turns a skip into a violation, as a live CI run needs.
  * `LifecycleCase` names the product, the lab, `StopBound` (the longest `Stop` may take, as the implementation documents it; required), `StartBound` (zero skips the start-hangs check), `Slack`, `ContextFree` (the methods documented to ignore their context), `RestoresAfterFailedStop`, `RequireAll` and `Logf`.
  * The checks are what the `ManagedInstaller` relies on, and what the implementations document:
    * an absent service is `Installed` false; a `Stop` or `Start` on it either fails with an error wrapping `ErrNotInstalled` or leaves it absent;
    * on an ended context, a method not in `ContextFree` fails with an error wrapping `ctx.Err()`, and `Stop` changes nothing;
    * `Enabled`, when the type implements `EnabledLifecycle`, agrees with the lab; `Stop` then `Start` leaves the service running, and `WaitHealthy` succeeds;
    * a stop the service ignores is `ErrTimeout` within `StopBound` plus `Slack`, `Running` then agrees with the lab, and, when `RestoresAfterFailedStop`, the manager still wants it up;
    * a cancellation while a stop is under way does not end the wait (0015-MADR B3): `Stop` returns after the service has gone, with the cancellation's error;
    * a service that exits at once fails `WaitHealthy`; a start the manager never completes is `ErrTimeout` within `StartBound`.
  * `VerifyRunner(ctx, service.Runner, RunnerCase) error` checks a `Runner` against `ExecRunner`'s context behaviour (0037-MADR D1-A): a quick command fails on an ended context and is not recorded; a blocking command ends when its context does.
  * `FakeService`, an exported in-memory service that is a `Lifecycle`, an `EnabledLifecycle` and its own `Lab`. It is the reference the package's tests pass, and a program can test its own managed-update flow with it.
  * The library runs `VerifyLifecycle` itself: each backend over its fake, in a bubble, in unit tests; each backend over its real manager, behind a live `Lab`, in its live CI test (owner answer 3); and `Supervised` over its fake process table, from an external test package, because `service`'s own tests cannot import `servicetest`.
  * The divergences F11 lists are settled before the checks are adopted:
    * scm's `Start` checks its context before it issues the start;
    * launchd's `Installed` is documented as ignoring its context, as scm's and Task Scheduler's are;
    * launchd's `Stop` runs `bootout` and the wait under one deadline, the bound it documents;
    * launchd's `Start` returns `ErrNotInstalled` when the plist is missing, before `enable`;
    * `Supervised.Start` returns `ErrNotInstalled` when the supervisor answers and reports the service not installed, and runs `Up` otherwise, including when no supervisor answers (owner question 5);
    * the codesign and ghattest fakes fail a call on an ended context.
  * depguard gains a `service-servicetest` rule, and `other-packages` excludes the package.
* **D2-B. An internal helper,** `internal/lifecycletest`, with the same checks, used by the library's own backends only.
* **D2-C. No change.** Each backend keeps its own tests, as 0037 left them.

### D3: tests on the production clock, and effects bound to the session (F4–F9)

* **D3-A. All three parts:**
  * **Bubbles.** Tests that drive fakes and measure time run inside `synctest.Test`, with the production clock.
    * The five test clocks are deleted, with `pollClock` and the `now` and `sleep` fields of `Supervised`, `scm.Service` and `taskscheduler.Task`; production calls `time.Now` and a context-aware sleep. This supersedes the owner's "Virtual clock, in M11" (0035-PLAN Deviation D4), which chose between a virtual clock and wall time before bubbles were in reach.
    * The fakes that move only on a virtual sleep move on observation instead: a fake's state steps once per read made at a later bubble time than the last.
    * Tests use the documented default bounds and realistic intervals, one bubble per subtest. A fixture whose bounds contradict its assertion, as scm's `TestStopWaits` does, is corrected (F4).
    * The real-clock tests 0035 and 0037 added become bubble tests with the documented default bounds, as P1 shows.
    * Tests that run real processes, sockets, files a child writes or a service manager stay on the real clock (F5, F9).
  * **Seams per owner.** `selfupdate`'s package-level seams move onto the values that use them:
    * the filesystem effects onto an unexported `fsOps` of function fields, which `StandaloneInstaller` holds, each session shares, and a zero `StandaloneInstaller` fills with the real functions when it is used;
    * the free functions on the install path become methods of `fsOps`;
    * target resolution, the environment, the terminal and the platform onto unexported fields or constructors of the types that read them;
    * `timeNow` is deleted in favour of the public `Config.Clock` and `CheckerConfig.Clock`, and the two seams no test replaces are deleted;
    * `setSeam` is deleted, and `selfupdate`'s tests may call `t.Parallel`, except those that use `CheckNoLeak`, `t.Setenv` or `t.Chdir` (F7).
  * **Root-relative replacement.** The install path's names resolve through the session's `*os.Root`:
    * the rename, link, remove and `Lstat` through the `Root` methods; the staging and backup names from an `O_EXCL` loop in the root that keeps the all-digit suffix the leftover sweep parses; the directory flush through `syncRootFn`; the target's digest through `rootFileSHA256`;
    * the owner and mode through a descriptor opened from the root and checked to be the file the `Lstat` saw, then `(*os.File).Chown` and `Chmod`, not `Root.Chmod` (F8);
    * on Windows, the same calls go through the root, and the rename stays `moveFileReplace`, for its write-through and its busy-image retry (F8);
    * a rollback whose backup and target are the same file removes the backup (P2).
* **D3-B. Bubbles only.** The clocks and the real-clock tests change; the seams and the replace path stay.
* **D3-C. Root-relative replacement only.**
* **D3-D. No change.**

## Decision Outcome

Chosen option: **"D1-A, D2-A and D3-A"**, in the order D3, D2, D1, as the owner accepted on 2026-10-27 with the PLAN ("q5: go with recommendation, proceed"), because each closes a gap the facts show with standard library means, additively within v1:

* **D3-A first**, because it changes no exported API and makes the checks D2 runs against fakes fast and deterministic. P1 shows the same bug class caught at the documented default bound, in no wall time, and P3 shows most of the backend tests ready for it. Root-relative replacement closes F8's window, which P2 reproduces, with standard library calls the floor already has. Per-session seams are the larger part. They pay for themselves only if the owner wants `selfupdate`'s tests parallel, which is why D3-B exists.
* **D2-A second**, because the five defects in F3 were all divergences from a contract that no check could test, P4 shows a check that catches them, and the module's own precedent, `VerifyReleaseSource`, shows the shape. Exporting it lets programs that implement `Lifecycle`, or feed `Supervised`, check themselves. D2-B helps only the library.
* **D1-A last**, because it is the largest API addition. It is the one that changes what a caller can express: "install this release", not "install whatever is latest now". D1-B gets the safety for less API, but leaves every host to copy pins by hand. D1-C pins the tag only.

Owner questions, to settle before a PLAN:

1. **D1-A: is calling `Apply` the confirmation?** One view: the host already showed the plan, so `Apply` does not ask the `Confirmer`. The other: `Apply` asks as `Run` does, unless the request said `Yes`.
2. **D1-A: should a plan expire?** On an immutable release, a plan stays true. A newer release may appear in the meantime, and that is the point of a pin. The proposal is no expiry.
3. **D2-A: is `Lab` the right seam for the live backends?** The live tests already arrange these states for systemd, launchd, SCM and Task Scheduler. The PLAN would move that arranging behind `Lab`.
4. **D3-A: which parts, and in which release.** The parts are bubbles, per-session seams and root-relative replacement.

The owner answered all four on 2026-10-27:

1. `Apply` asks the `Confirmer` as `Run` does, unless the request says `Yes`. A host that has already shown the plan sets `Yes`. One rule covers `Run` and `Apply`, so a host that calls `Apply` directly cannot skip the prompt by accident.
2. No expiry. `Apply` checks the pins again and returns `ErrPlanStale` when the asset or its digest has changed. A newer release is what the pin is meant to ignore.
3. `Lab` is the seam. The live systemd, launchd, SCM and Task Scheduler tests move their arranging behind it, so the library and its callers run the same cases.
4. All three parts, in two steps. Bubbles and per-session seams come first, as a patch release with no API change. Root-relative replacement on Unix ships with D2-A and D1-A in `v1.23.0`, so the behaviour change lands in a minor release with its own notes.

The assessment after those answers settled how each answer is built, in the options above:

* Answer 1 needs a request to read `Yes` from, so `Apply` takes the `Request` as well as the `Plan`, and refuses one that does not match (F12; D1-A).
* `Run` is not rebuilt as `Plan`, then the confirmation, then `Apply`, which would download the manifest before the confirmation and change `Run`'s call order and goldens. The three share one pipeline instead (F1; D1-A).
* Answer 3 holds, but no live test today arranges a service that ignores a stop for scm, launchd or s6, or a slow stop for systemd or scm, and Task Scheduler cannot arrange one at all. The live labs add those modes, or return `ErrUnsupported` where the manager cannot (D2-A).
* Answer 4's patch release is `v1.22.3`: bubbles and per-session seams change tests and unexported code only. The F11 fixes change behaviour a program can see, so they ship in `v1.23.0` with D2-A.

One question remained, for the PLAN's approval:

5. **D2-A: should `Supervised.Start` return `ErrNotInstalled` for a service its supervisor reports as not installed?** The managed installer never starts a service that `Installed` reports missing, so updates are unaffected; a program that calls `Start` itself sees the error in place of its `Up`'s. The cost: a program whose `Up` installs the service before starting it would need `State` to report it installed first. The proposal is yes, which matches `Stop`; the alternative is to document today's behaviour and have `VerifyLifecycle` accept it.

The owner answered on 2026-10-27, accepting the PLAN: "q5: go with recommendation, proceed". `Supervised.Start` returns `ErrNotInstalled` when the supervisor answers and reports the service not installed, and runs `Up` otherwise.

### Consequences

* Good, because a host, the CLI and a detached run can name one release from check to install (D1-A).
* Good, because a program that implements `Lifecycle` can run the same check the library's backends pass (D2-A).
* Good, because tests measure deadlines on the clock production uses, so F4's class of bug fails a test (D3-A, P1, P3).
* Good, because the replace path resolves names against the directory the lock checked (D3-A, P2).
* Good, because the backends do what they document in the places F11 lists (D2-A).
* Neutral, because every addition is additive, and `make apicheck` stays compatible with `v1.22.2`.
* Bad, because D1-A adds to a large public package: in `selfupdate`, `Plan`, `PlanDocument`, `AssetDocument`, `PlanSchema`, `ParsePlan`, `ErrPlanStale`, three methods and `StartApply`; in `service`, `EnvHandOffPlan`, `HandOffPlanFunc` and `LoadHandOffPlan`; two `cli.HandOff` fields; and two `Publish` methods in `selfupdatetest`.
* Bad, because `VerifyReleaseSource`'s new ID check can fail a third-party source's test that passed before, so the migration guide names it.
* Bad, because `Lab` asks a live backend's test to express its arrangements through an interface, which is work for each backend, and new live modes for the states no live test arranges (D2-A).
* Bad, because per-session seams touch 68 `setSeam` calls and the helpers behind 128 more call sites (D3-A).

### Confirmation

* **D1:**
  * a test publishes a newer release between `Plan` and `Apply` (`GitHubServer.Publish`). `Apply` installs the planned release, and `Run` the newer one;
  * a changed asset ID, digest or `SHA256SUMS` entry under the same tag is `ErrPlanStale` (an in-package script source, since GitHub cannot change an immutable release);
  * a request that does not match the plan is `ErrPlanStale` before any network call;
  * `Apply` asks the `Confirmer` unless `Yes`, and its events equal `Run`'s for the same release;
  * `cli`'s handoff test sees the child apply the parent's plan while a newer release exists;
  * `Run`'s call-order test and goldens are unchanged, and `FuzzParsePlan` runs in `make gate`.
* **D2:**
  * `servicetest`'s own tests: `FakeService` passes; a table of broken lifecycles, one rule each, fails with the rule named; a lab that returns `ErrUnsupported` skips, and with `RequireAll` fails.
  * 0037-MADR F1–F4, planted into scratch copies of the backend that had each, make `VerifyLifecycle` or `VerifyRunner` fail. 0035-MADR Amendment A3 is planted as the `v1.22.0` file, as P1 and P4 did.
  * Each backend passes over its fake on `main`, and over its real manager in its live CI job.
  * Each F11 fix is seen failing first.
* **D3:**
  * P1 becomes a committed test, and the `v1.22.0` code is seen to fail it;
  * each test moved into a bubble is seen to fail on a planted break before it is trusted;
  * `go test -race ./...` passes on all three targets, and in the `go126` job;
  * P2 becomes a committed test that swaps the directory between the check and the rename: on `08e41cb` it fails as P2 did, and after D3-A the decoy is untouched and the result is `ErrConcurrentUpdate`.
* `make gate` ends `overall=0`; `make apicheck` reports compatibility with the newest `v1.*` tag.

## Pros and Cons of the Options

### D1-A

* Good, because it matches the find-then-apply shape the other Go libraries use (F2), with the integrity data pinned as well as the tag.
* Good, because a `Plan` is a document: a CLI can print it, a TUI can show it, and a handoff can carry it.
* Good, because `Run`, `Plan` and `Apply` share one pipeline, so they cannot drift apart, and `Run` does not change.
* Neutral, because `Plan` downloads `SHA256SUMS` and runs the manifest verifiers, and `Apply` does both again, where `Check` downloads no body.
* Bad, because it is the largest of the three additions.

### D1-B

* Good, because it pins the release and digest with four fields and one sentinel.
* Bad, because every host copies pins between two structs by hand, and a detached run has to rebuild them from arguments.

### D1-C

* Good, because it costs nothing.
* Bad, because it pins only the tag, and leaves the rate-limit cost of resolving twice.

### D2-A

* Good, because the contract becomes checkable by the people who implement it, as the source contract is.
* Good, because the library's four backends and `Supervised` share one statement of the contract, not five sets of tests.
* Good, because P4 shows it catching every defect F3 lists.
* Bad, because `Lab` is new API whose shape has to fit both fakes and real service managers, and a check is only as good as its lab: F3 needs a slow `bootout`, A3 a slow `Down`.

### D2-B

* Good, because it needs no API.
* Bad, because programs that implement `Lifecycle`, the case F3 describes, gain nothing.

### D2-C

* Bad, because the next divergence is again found by reading.

### D3-A

* Good, because P1 shows the bubble catching a bug the virtual clock missed, at the default bound, in no wall time, and P3 found a fixture the virtual clock hid.
* Good, because the test clocks and the production `now` and `sleep` fields go away.
* Good, because `selfupdate`'s tests can run in parallel.
* Good, because root-relative names close F8's window, which P2 reproduces, with standard library calls.
* Bad, because the per-session seams are a large mechanical change, with a risk of a missed seam until the tests that use it are seen to fail on a planted break.
* Bad, because bubbles do not cover real processes. The live tests stay on the real clock, so some timing remains wall-time-sensitive.

### D3-B

* Good, because it is the part with the measured payoff (P1, P3), at a fraction of D3-A's cost.
* Bad, because the package seams still rule out parallel tests, and F8's window stays.

### D3-C

* Good, because it is small and closes F8.
* Bad, because F4's class stays.

### D3-D

* Bad, because F4's class has already reached a release once (`v1.22.0`).

## More Information

The PLAN is [0038-PLAN-pinned-plans-lifecycle-conformance-and-bubble-tests.md](0038-PLAN-pinned-plans-lifecycle-conformance-and-bubble-tests.md).

**Probes.** Each ran on 2026-10-27, in a scratch clone of `08e41cb`, with Go 1.27.3 on macOS. The clones are not part of the repository; the PLAN commits the tests that P1, P2 and P4 prototyped.

* **P1** wrapped `TestSupervisedStopRestoresOnRealClock`'s body in `synctest.Test`, with `Poll{Timeout: 60s, Interval: 1s, Settle: -1}`. It ran first on that tree, then with `selfupdate/service/supervised_lifecycle.go` replaced by `git show v1.22.0:…`, which is the code before 0035-PLAN M11.
* **P2** ran `StandaloneInstaller` with a `writeJournalFn` hook, the last point between the target's check and the rename (`replace_unix.go:75-79`). The hook wrote the journal, renamed the locked directory aside, and made a decoy directory in its place, holding a target and a file at the staging file's name. Output: `Install: applied=false rolledBack=true … target directory changed during the update: selfupdate: concurrent update`; `decoy dir target: "decoy-staging"`; `locked dir target: "old-bytes"`; a backup left in the locked directory.
* **P3** generated a `synctest.Test` wrapper for every test of `service` and its four backends that builds on darwin, ran each alone with a 60 s timeout, then removed the scm and Task Scheduler clocks from a copy of their fakes and ran their stop tests at the default bounds, once under `-race`.
* **P4** built a prototype `servicetest` with fake labs for systemd, launchd and `Supervised`, ran it inside bubbles 40 times per case on `main` and on each planted defect, and added the proposed depguard rule, which refused a planted `internal/winproc` import.
* **P5** added stub declarations of D1-A's API and ran `apidiff` against `v1.22.2`, built from the module cache, and `go vet` for the three targets.

**Changes from the draft the owner reviewed** (2026-10-27):

* F1 states `Run`'s order, and D1-A no longer rebuilds `Run` from `Plan` and `Apply`; `Plan` takes options; `Apply` takes the `Request`; the plan records whether it was resolved as the latest; the handoff carries the plan in `HandOff.Env`, not beside `ArgHandOffEnv`, which is Task Scheduler's alone; `Plan.Request`, `StartApply`, the document types, the `selfupdatetest` additions and the `VerifyReleaseSource` check are named.
* F3 states where each part of the lifecycle contract is documented. D2-A's checks follow it: context-free methods are exempt, bounds are the implementation's, and a start-hangs check and a still-wanted-up check are added (F10). `VerifyRunner` takes a `RunnerCase`. `Lab` is two methods over a `State`. `FakeService` is added. The F11 divergences, and question 5, are new.
* F4 counts five clocks, with `pollClock` and `healthOver`'s, and says how few sleeps a bubble removes. F7 counts 24 seams, 22 replaced, and names their owners; D3-A puts the filesystem seams on the installer, not an `effects` value on the `Updater`. F8 lists every path-based call, P2's result and `Root.Chmod`'s limits, and D3-A changes owner and mode through descriptors. F9 to F12 are new.

**Considered and set aside:**

* **Generic methods, `encoding/json/v2`, `crypto/mldsa`** are in the Go 1.27 toolchain (`go doc`), above the floor. ML-DSA is a candidate for `verify/signednote` when the floor reaches 1.27.
* **`//go:fix inline`** lets `go fix` migrate callers off a deprecated function (Go 1.26). It becomes useful with the module's first `Deprecated:`, and belongs in 0025-MADR D6's evolution policy then.
* **A `service.Detect` that picks a backend for the running process** was set aside. A program knows how it is installed better than the environment does, and a process can sit under several managers at once.
* **`Availability.Plan`, or a `Checker` that returns a plan.** `Check` promises no asset body is downloaded (`checker.go:31-34`) and runs no manifest verifiers. A banner host passes `Availability.TargetVersion` to `Plan`. A comparable `Plan` keeps the door open (F12).
* **A `StartPlan` stream.** `Plan` asks nothing of the host but credentials, which `WithCredentials` supplies.
* **`Root.Chmod` and `Root.Chown`** for the staging file's mode and owner, for F8's race and `/proc` limits.
* **Scoping `CheckNoLeak` to one bubble or one goroutine tree,** which would let the tests that use it run in parallel. The tests that use it stay sequential instead.

**Sources** (read 2026-10-27):

* `testing/synctest`: <https://pkg.go.dev/testing/synctest>, and the Go 1.27.3 `go doc testing/synctest`.
* `os.Root`: the Go 1.27.3 `go doc os.Root`, and `api/go1.24.txt` and `api/go1.25.txt` in that toolchain.
* `testing/fstest.TestFS`: <https://pkg.go.dev/testing/fstest#TestFS>, the standard library's conformance check that takes no `*testing.T`.
* Go 1.27 release notes: <https://go.dev/doc/go1.27>.
* "//go:fix inline and the source-level inliner": <https://go.dev/blog/inliner>.
* CVE-2026-32282, `Root.Chmod` on Linux: <https://groups.google.com/g/golang-announce/c/0uYbvbPZRWU>.
* <https://pkg.go.dev/github.com/creativeprojects/go-selfupdate>, <https://pkg.go.dev/github.com/minio/selfupdate> and <https://pkg.go.dev/github.com/fynelabs/selfupdate>.
* "Keeping Your Modules Compatible": <https://go.dev/blog/module-compatibility>, for additive change through config structs, new methods and optional interfaces, as 0025-MADR D6 adopted.
