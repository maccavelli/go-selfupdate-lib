---
status: accepted
date: 2026-10-27
decision-makers: go-selfupdate-lib maintainers
consulted: "0035-MADR-serve-supervisors-through-a-supervised-adapter.md Amendment A3 and 0035-PLAN-serve-supervisors-through-a-supervised-adapter.md M11, which found the bug class; a read of `selfupdate/service`'s four backends, their fakes and `selfupdate/managed.go` at 20e7cc7 (v1.22.1 and its pin commit), on 2026-10-27"
informed: the fleet's programs that run under systemd, launchd, the Service Control Manager or Task Scheduler
---
<!-- markdownlint-disable MD004 MD013 MD024 MD060 -->

# Make the backend fakes honour their context, and bound every stop command the way its wait is bounded

## Context and Problem Statement

[0035-MADR-serve-supervisors-through-a-supervised-adapter.md](0035-MADR-serve-supervisors-through-a-supervised-adapter.md), Amendment A3, found a bug that tests could not see. `Supervised.Stop` ran its restoring `Up` on a context that had already ended. The fake `Up` ignored its context, and the tests ran on a virtual clock that never reached the context's real deadline. `v1.22.1` fixed it. The owner chose, on 2026-10-27, to audit the other four backends for the same class of bug: a fake that ignores its context, and a command or a wait whose bound is not the one the documentation states.

### Facts [read, at 20e7cc7]

* **F1. Two fakes ignore their context.**
  * `systemd`'s `fakeSystemd.Run` takes `_ context.Context` (`systemd/fake_test.go:46`).
  * `launchd`'s `fakeLaunchd.Run` takes `ctx` but never reads it (`launchd/fake_test.go:78`).
  * `ExecRunner` fails a command whose context has ended, and kills one that is running when it ends. So these fakes answer calls the real runner would refuse.
* **F2. systemd's stop command has no bound.**
  * `Unit.Stop` runs `systemctl stop --no-ask-password --quiet -- <unit>` on `sctx`, `context.WithoutCancel(ctx)`, which has no deadline either (`systemd/lifecycle.go:91-92`).
  * Only the `waitState` after it is bounded, by `stopBound`: the longer of `Poll.Timeout` and `TimeoutStopUSec` plus 30 s, or `Poll.Timeout` when `TimeoutStopUSec` is `infinity` (`:110-126`).
  * Without `--no-block`, `systemctl stop` waits for the stop job to finish, per systemctl(1) [docs; to probe, P1].
  * So under `TimeoutStopUSec=infinity`, with a program that ignores TERM, `Stop` would block for as long as the program runs. That is past its documented `ErrTimeout`, and the caller cannot cancel it [derived].
* **F3. launchd's `bootout` runs on the caller's context.**
  * `Job.Stop` runs `launchctl bootout` on `ctx` (`launchd/lifecycle.go:169`). Only the wait after it, `waitGone`, runs on `context.WithoutCancel(ctx)` (`:231`).
  * A caller that ends its context during `bootout` makes `ExecRunner` kill `launchctl`, and `Stop` returns at once, with no wait. 0015-MADR B3 says the stop's wait outlives the caller once the stop is issued.
  * `bootout` returns while a TERM-ignoring job is still loaded (`TestLiveStopWaitsForSlowExit`, `launchd/live_darwin_test.go:449-452`), so the window is short.
  * The updater's `recoverStop` then reads `Running` and restarts the job only when it is down (`selfupdate/managed.go:287-300`). A job still going down at that read is left down once it goes [derived].
* **F4. launchd's bootstrap timeout is not always `ErrTimeout`.** `bootstrap` bounds its retries by `Poll.Timeout`. It returns `ErrTimeout` when its own `select` sees the deadline, but returns `launchctlRun`'s error unwrapped when the deadline ends a `launchctl` run (`launchd/lifecycle.go:288-300`). `Start`'s documentation promises `ErrTimeout` at the bootstrap's bound.
* **F5. Task Scheduler's stop wait can lose its message.**
  * `Task.Stop` bounds its context by `Poll.Timeout` before the `RequestStop` grace (`taskscheduler/lifecycle.go:154`).
  * `waitGone` then measures its own `Poll.Timeout` from after the grace (`:254-269`).
  * As in 0035's Amendment A3, the context ends first. The error is `timed out: <path>: context deadline exceeded`, where the clock's would be `<path> still runs after it was stopped`. The class, `ErrTimeout`, is right.
  * The restore after a failed stop, `enableAgain`, is a COM call with no context, so it is not affected.
* **F6. scm is not affected.** It has no restore after a failed stop: it records the dependents it stopped, for `Start`. Its Windows API calls take no context, and its virtual clock already returns the context's error (`scm/fake_test.go:245-247`). The same holds for `taskscheduler`'s virtual clock (`taskscheduler/fake_test.go:393-395`).
* **F7. `CommandError.Backend`'s documentation** names `"systemd"`, `"launchd"` or `"service"` (`errors.go:48-49`). magic-cli-remote's 0212 P2 fills it with `runit`, `s6` and `openrc`, for its own commands under `service.Supervised`.

## Decision Drivers

* **A test's fake behaves like `ExecRunner` on an ended context,** or a test cannot see a context bug.
* **Each documented bound holds.** `Stop` returns `ErrTimeout` at its bound, and the caller's context never ends a stop that has been issued (0015-MADR B3).
* **No behaviour change without a probe** of the service manager it rests on.
* **Small and additive.** No exported API changes. `make apicheck` stays clean against `v1.22.1`.

## Considered Options

### D1: the fakes

* **D1-A.** Each fake runner returns `ctx.Err()` first, and records nothing, on an ended context, as `ExecRunner` and 0035's fake supervisor do.
* **D1-B.** A shared wrapper in `internal/testproc`. `testproc` cannot import `service`, since `service`'s own tests import `testproc`, so it would need generics over the runner's types for a two-line check.
* **D1-C.** Leave the fakes, and add tests that run the real `ExecRunner` against scripts.

### D2: systemd's stop command (F2)

* **D2-A.** `systemctl stop --no-block`, so the command only queues the stop job, and `waitState` bounds the wait by `stopBound`, as today. `systemctl` still exits non-zero for a refused job, such as for permission or an unknown unit. It does not report the job's result, which `waitState` reads from the unit's state.
* **D2-B.** Keep the blocking command, but run it on `sctx` bounded by `stopBound`, and report its kill at that bound as `ErrTimeout`.
* **D2-C.** Leave it. Document that `TimeoutStopUSec=infinity` can hold `Stop` indefinitely.

### D3: launchd's bootout (F3)

* **D3-A.** Run `bootout` on `context.WithoutCancel(ctx)`, bounded by `stopBound`, as systemd's stop runs. The caller's error is returned after the wait, as it is now.
* **D3-B.** Leave it: the window is short (F3).

### D4: launchd's bootstrap timeout (F4)

* **D4-A.** A `launchctl` run that the bootstrap's deadline ends is `ErrTimeout`, wrapping the command's error.
* **D4-B.** Leave it, and change `Start`'s documentation.

### D5: the timeout messages (F5)

* **D5-A.** `taskscheduler`'s `waitGone` reports `<path> still runs after it was stopped`, as `ErrTimeout`, whether its clock or the context's deadline ends the wait, as `Supervised`'s now does.
* **D5-B.** Leave it: the class is right.

### D6: `CommandError.Backend` (F7)

* **D6-A.** Its documentation adds that a program's own commands, such as under `service.Supervised`, name their own backend, such as `runit`.
* **D6-B.** Leave it.

## Decision Outcome

Chosen option: "D1-A, D2-A, D3-A, D4-A, D5-A and D6-A", accepted by the owner's "proceed" on 2026-10-27, because each is the smallest change that makes a fake or a bound behave as `ExecRunner` and the documentation already say.

* **D1-A** is the check that would have caught 0035's bug. It costs two lines per fake. D1-B's generics buy nothing for two fakes, and D1-C cannot drive a service manager's states.
* **D2-A** is what launchd's `bootout` already does, and what F2's bound assumes: the command queues, and the wait bounds. D2-B kills `systemctl` mid-job and leaves the job queued anyway, with a less certain error. D2-C keeps a `Stop` that can exceed its documented bound forever. D2-A rests on probe P1. If P1 shows `systemctl stop` returns before the job ends, F2 is void, and D2 becomes D2-C with a corrected fact.
* **D3-A** makes launchd follow 0015-MADR B3 as systemd does. The bound keeps a hung `launchctl` from holding `Stop`.
* **D4-A** keeps `Start`'s documented error. D4-B would document an accident.
* **D5-A** gives the timeout the same message whichever clock ends it, as 0035's Amendment A3 did for `Supervised`.
* **D6-A** documents a use the adapter invites.

A release, `v1.22.2`, follows on the owner's word if D2, D3 or D4 changes behaviour.

### Consequences

* Good, because systemd's and launchd's tests then fail a context bug as the real runner would.
* Good, because `Stop` returns `ErrTimeout` at its bound on every backend, whatever the service manager's own kill timeout, and no caller cancellation ends an issued stop.
* Neutral, because D2-A loses `systemctl`'s report of a failed stop job. `waitState` already treats `failed` as stopped, and a job that never ends is `ErrTimeout` at the bound.
* Bad, because a test that relied on a fake answering on an ended context needs changing. Phase N2 finds any such test before anything else changes.
* Bad, because a program that relied on `Stop` blocking past its bound under `TimeoutStopUSec=infinity` now gets `ErrTimeout`, while the unit is still stopping. That program was already outside the documented contract.

### Confirmation

* Each fake returns `ctx.Err()` on an ended context, and the existing tests pass. A plant that removes the check fails the new tests.
* **P1, on the Linux test host:** a user unit with `TimeoutStopSec=infinity`, whose program ignores TERM.
  * Time `systemctl --user stop`, with and without `--no-block`.
  * Then a live test: `Stop` returns `ErrTimeout` near `Poll.Timeout`, and the unit is still `deactivating`.
* **P2, on this macOS host:** a TERM-ignoring job with `ExitTimeOut` set. Time `launchctl bootout`, and check that the job still goes when `launchctl` is killed during it. This confirms F3's premise and D3-A's bound.
* Fake tests on the real clock show each of D2-A to D5-A failing before the change, and passing after.
* `make gate`, with `apicheck` reporting compatibility with `v1.22.1`.

## Pros and Cons of the Options

### D1-A

* Good, because it matches `ExecRunner`, and 0035's fake supervisor already does it.
* Bad, because each future fake must repeat it. Review is the only guard.

### D1-B

* Good, because the check lives in one place.
* Bad, because the import cycle forces generics for a two-line check.

### D1-C

* Good, because the real runner is exercised.
* Bad, because a script cannot model `systemctl show` or `launchctl print` states without becoming a second fake.

### D2-A

* Good, because the stop's bound is `stopBound` alone, as documented.
* Good, because launchd's `bootout` already behaves this way.
* Neutral, because the job's own result comes from the unit's state, not `systemctl`'s exit code.
* Bad, because it rests on probe P1.

### D2-B

* Good, because `systemctl` still reports a failed job within the bound.
* Bad, because killing `systemctl` at the bound leaves the job queued anyway, with an error the backend must map.

### D2-C

* Good, because nothing changes.
* Bad, because `Stop` can exceed its documented bound indefinitely.

### D3-A

* Good, because 0015-MADR B3 then holds for launchd as written.
* Neutral, because `bootout` returns quickly, so the bound rarely matters.

### D3-B

* Good, because nothing changes.
* Bad, because a cancelled caller can leave a job going down after recovery has read it as running.

### D4-A, D5-A and D6-A

* Good, because each makes the code match its documentation, or the documentation match the code's use.
* Neutral, because none changes the behaviour a program sees, other than an error's class or text.

## More Information

* Plan: [0037-PLAN-honour-contexts-in-backend-fakes-and-stop-commands.md](0037-PLAN-honour-contexts-in-backend-fakes-and-stop-commands.md).
* [0035-MADR-serve-supervisors-through-a-supervised-adapter.md](0035-MADR-serve-supervisors-through-a-supervised-adapter.md), Amendment A3: the bug that started this audit.
* [0015-MADR-remediate-third-debugging-pass-findings.md](0015-MADR-remediate-third-debugging-pass-findings.md), B3: an issued stop outlives the caller, and is bounded by the longer of `Poll.Timeout` and the service manager's kill bound.
* [0011-MADR-reference-service-lifecycles.md](0011-MADR-reference-service-lifecycles.md) §7: the stop bounds.
* Out of scope: `WaitHealthy`'s two `PollHealthy` runs, each with its own `Poll.Timeout`, which are documented. scm (F6).

## Amendment A1 (2026-10-27): probes P1 and P2

**P1, systemd 259 (259.9-0ubuntu0.1), on the Linux test host** (`p1_systemd.py`). The probe used transient user units from `systemd-run --user -p TimeoutStopSec=infinity`, each running a Python program that ignores TERM:

| Command | Result | Unit after |
| :--- | :--- | :--- |
| `systemctl --user stop --no-ask-password --quiet -- <unit>` | still running at 20 s, then killed | `deactivating`, `stop-sigterm` |
| the same with `--no-block` | exit 0 in 0.03 s | `deactivating`, `stop-sigterm` |

F2 holds: the blocking command waits as long as the program runs, and killing it leaves the stop job running. D2-A stands. Each unit was then killed with KILL and reset, ending `inactive`, `dead`.

**P2, macOS 26.6.2, on this host** (`p2_launchd.py`). The probe used jobs in `gui/<uid>`, from plists in a temporary directory, with `ExitTimeOut` 15 and a Python program that ignores TERM:

| Case | Result |
| :--- | :--- |
| `launchctl bootout` | exit 0 in 0.04 s. The job stayed loaded and alive, and launchd killed it 15.0 s later |
| `launchctl` killed at once (0 ms) | the bootout never reached launchd: the job stayed loaded and running |
| `launchctl` killed at 50 ms | `bootout` had already exited 0. The job went 15.1 s later |

F3 holds only within the few tens of milliseconds between launchd taking the request and `launchctl` exiting. A kill before that leaves the job running. `recoverStop` then reads it as running, which is right. D3-A stands for B3's sake, and its bound costs nothing. No job was left loaded.

## Amendment A2 (2026-10-27): D2-A supersedes one line of 0011

[0011-MADR-reference-service-lifecycles.md](0011-MADR-reference-service-lifecycles.md)'s systemd rules say "`Stop` and `Start` block on the job". Its PLAN's step 3 then checks `ActiveState`, "because the client's exit is not the job's result". This MADR's context did not cite that line.

D2-A supersedes it for `Stop` only:

* `Stop` queues its job with `--no-block`, and `waitState` reads the result, as 0011's step 3 already did.
* `Start` still blocks on its job, bounded by the caller's context.
* 0011 keeps its text, and this amendment records the change.

Found while executing N3. It changes no decision: it names one that D2-A replaces.
