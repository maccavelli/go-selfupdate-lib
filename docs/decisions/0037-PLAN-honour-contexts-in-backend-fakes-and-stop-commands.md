---
status: in-progress
date: 2026-10-27
associated-madr: "0037-MADR-honour-contexts-in-backend-fakes-and-stop-commands.md"
---
<!-- markdownlint-disable MD004 MD013 MD024 MD060 -->

# Implement context-honest fakes and bounded stop commands for systemd, launchd and Task Scheduler

Associated MADR: [0037-MADR-honour-contexts-in-backend-fakes-and-stop-commands.md](0037-MADR-honour-contexts-in-backend-fakes-and-stop-commands.md)
(D1-A to D6-A, as the owner accepted on 2026-10-27).

## Goal

Every fake runner fails a call on an ended context, as `ExecRunner` does. Every `Stop` returns `ErrTimeout` at its documented bound, and no caller cancellation ends a stop that has been issued. Each change is first seen failing.

## Scope

### In scope

* `selfupdate/service/systemd`: `fake_test.go`; `lifecycle.go`'s `Stop` (D2-A); its tests; a live test.
* `selfupdate/service/launchd`: `fake_test.go`; `lifecycle.go`'s `Stop` and `bootstrap` (D3-A, D4-A); its tests.
* `selfupdate/service/taskscheduler`: `lifecycle.go`'s `waitGone` (D5-A); its tests.
* `selfupdate/service/errors.go`: `CommandError.Backend`'s comment (D6-A).
* `docs/guides/extending-selfupdate.md`, where it states a stop's bound; the records.
* `v1.22.2`, on the owner's word, if N3 or N4 changes behaviour.

### Out of scope

* scm (MADR F6). `WaitHealthy`. Any exported API.

## Implementation Steps

### Rules for every phase

* The phase's files are named in its step. A file outside them is a deviation: stop and prompt.
* Each new test or check is seen failing first, on the current code or a plant, in a scratch copy of the tree (`scripts/plant-copy.sh`, or a scratchpad script). The tree is never dirtied for it.
* `make pre-add-check` on the phase's Go files, and `make gate` ending `overall=0`, before the owner's commit.

### N0: the records

This MADR and PLAN, `proposed`, and the `docs/README.md` index rows. The owner's approval moves the plan to `in-progress`.

### N1: probes

1. **P1, on the Linux test host,** in a scratch directory, removed after:
   * a user unit, `TimeoutStopSec=infinity`, whose program traps TERM and keeps running;
   * time `systemctl --user stop <unit>`, killed after 20 s if still running, then `--no-block`;
   * record `ActiveState` and `SubState` during each;
   * clean up with `systemctl --user kill -s KILL`, then remove the unit.
2. **P2, on this macOS host:**
   * a job, `ExitTimeOut` 15, whose program traps TERM;
   * time `launchctl bootout`;
   * start a second `bootout` against a fresh copy of the job, kill `launchctl` 50 ms in, and record whether the job still goes;
   * clean up the job and its plist.
3. The results go into the MADR as Amendment A1. A result that voids a fact, such as P1 showing that `systemctl stop` returns at once, stops the work for the owner (MADR, D2-A's outcome).

### N2: the fakes (D1-A)

1. `systemd/fake_test.go` and `launchd/fake_test.go`: `Run` returns `service.Output{}, ctx.Err()`, and records nothing, when `ctx` has ended.
2. Run both packages' tests, on this host and the Linux test host. A test that fails is a finding: stop and prompt, before any fix.
3. A test in each package: a call on a cancelled context returns its error, and records no call. Seen failing with the check planted out.

### N3: systemd's stop (D2-A)

1. `lifecycle.go`: `Stop` runs `systemctl stop --no-block --no-ask-password --quiet -- <unit>`.
2. A fake test: a stop job that never ends makes `Stop` return `ErrTimeout` at `stopBound`, measured on the real clock with a short `Poll.Timeout` and `TimeoutStopUSec` `infinity`.
   * The fake's `stop` blocks until its context ends unless `--no-block` is passed, as P1 shows `systemctl` does.
   * Seen failing on today's `Stop`, which never returns: the test bounds itself and reports the hang.
3. A live test, `TestLiveStopInfinityTimesOut`, under `SELFUPDATE_REQUIRE_SYSTEMD`: P1's unit, with `Poll.Timeout` 3 s. `Stop` returns `ErrTimeout` within 3 to 10 s, and the unit is `deactivating`. Cleanup kills it. Run on the Linux test host and the WSL.

### N4: launchd's bootout and bootstrap (D3-A, D4-A)

1. `lifecycle.go`:
   * `Stop` runs `bootout` on `context.WithoutCancel(ctx)`, bounded by `stopBound`;
   * a `launchctl` run that the deadline ends is `ErrTimeout` in `bootstrap`.
2. Fake tests:
   * a caller cancelled during `bootout` still waits for the job to go, then returns the caller's error;
   * a `bootstrap` whose `launchctl` blocks past the deadline is `ErrTimeout`.

   Each is seen failing on today's code.
3. `go test` on this host, and the live launchd tests with `SELFUPDATE_REQUIRE_LAUNCHD=1`.

### N5: messages and documentation (D5-A, D6-A)

1. `taskscheduler/lifecycle.go`: `waitGone` returns `<path> still runs after it was stopped`, as `ErrTimeout`, when the context's deadline ends the wait. A test with `RequestStop` and a grace sees it, seen failing on today's message.
2. `errors.go`: `CommandError.Backend`'s comment, D6-A.
3. `extending-selfupdate.md`: systemd's stop bound holds under `TimeoutStopUSec=infinity`, and launchd's issued stop outlives the caller.
4. `go test` for `taskscheduler` on the Windows test host.

### N6: `v1.22.2`, on the owner's word

As 0035-PLAN-serve-supervisors-through-a-supervised-adapter.md M12: the `CHANGELOG.md` entry and migration-guide section; `make gate`; the push and CI; the tag check, the disclosure guard and the tag; the tag's CI; the pin commit; the proxy.

## Verification

* **V1 (probes):** P1 and P2 are recorded in the MADR's Amendment A1.
* **V2 (fakes):** both fakes fail on an ended context, and every existing test passes, or a deviation records why not.
* **V3 (bounds):**
  * the fake tests for D2-A to D5-A fail on today's code and pass after;
  * the systemd live test passes on the Linux test host and the WSL;
  * the launchd live tests pass on this host.
* **V4 (API):** `make apicheck` reports compatibility with `v1.22.1`.
* **V5 (release):** `v1.22.2` tagged, its CI green, the proxy serving it, if the owner chooses N6.

## Rollout and Rollback

* **Rollout:** one commit per phase on `main`, pushed on the owner's word, then `v1.22.2`. A program takes it by bumping its requirement. Nothing in the API changes.
* **Rollback:** before the tag, `git revert` of a phase. After it, fix forward. D2-A's rollback is removing `--no-block`.

## Execution record

### Approval (2026-10-27)

The owner answered "proceed" to the proposed pair, which approves this plan and accepts the MADR's D1-A to D6-A. D2-A still rests on P1.

### N1: probes (2026-10-27)

* **P1** (`p1_systemd.py`, Linux test host, systemd 259): `systemctl --user stop` on a `TimeoutStopSec=infinity` unit that ignores TERM was still running at 20 s. `--no-block` exited 0 in 0.03 s. Both left the unit `deactivating`, `stop-sigterm`. Transient units only, so no unit file was written. Each was killed and reset, ending `inactive`, `dead`.
* **P2** (`p2_launchd.py`, this host, macOS 26.6.2): `bootout` exited 0 in 0.04 s, with the job gone 15.0 s later at `ExitTimeOut`. `launchctl` killed at 0 ms left the job running. At 50 ms, `bootout` had already exited and the job went at 15.1 s. Plists lived in a temporary directory, since removed, and no job was left loaded.
* Recorded in the MADR as Amendment A1. Both premises hold, F3's only for a few tens of milliseconds. No fact is voided, so N2 follows.

### N2: the fakes (2026-10-27)

* **`systemd/fake_test.go` and `launchd/fake_test.go`:** `Run` returns `service.Output{}, ctx.Err()`, and records nothing, on an ended context, with a comment citing D1-A. `TestFakeHonoursContext` in each package checks it.
* **Existing tests, with the stricter fakes:** `go test -count=1 ./selfupdate/service/systemd/ ./selfupdate/service/launchd/` passes, both here and on the Linux test host. No test relied on a fake answering on an ended context, so N2 found nothing to stop for.
* **Seen failing** (`plants_37.py N2`, scratch copies):

  | Plant | Result |
  | :--- | :--- |
  | (a) systemd's check removed | FAIL: `Run on a cancelled context: <nil>, calls [["/usr/bin/systemctl" "show"]]; want context.Canceled and none` |
  | (b) launchd's check removed | FAIL: `Run on a cancelled context: <nil>, calls [["/bin/launchctl" "list"]]; want context.Canceled and none` |

* **`make pre-add-check`** on the two files: "2 file(s) clean (gofmt, golangci-lint, go vet, go test, govulncheck)".

### N3: systemd's stop (2026-10-27)

* **`systemd/lifecycle.go`:** `Stop` runs `systemctl stop --no-block --no-ask-password --quiet -- <unit>`, with a comment citing D2-A. The wait after it is unchanged.
* **`systemd/fake_test.go`:** `stopBlocks`, when set, holds a stop without `--no-block` until it is closed or the call's context ends, as P1 showed `systemctl` does. `blocksOnStop` tells such a stop apart.
* **`systemd/stopwait_test.go`, `TestStopBoundsAJobThatNeverEnds`:** `TimeoutStopUSec` `infinity`, a job that stays `deactivating`, and a 300 ms `Poll.Timeout` on the real clock. `Stop` must return `ErrTimeout` between 300 ms and 3 s. The test bounds itself at 5 s, then releases the fake.
* **`systemd/live_linux_test.go`:** an `ignore-term` mode, which reports ready and ignores TERM, and `TestLiveStopInfinityTimesOut`.
  * The test runs a `TimeoutStopSec=infinity` unit with a 3 s `Poll.Timeout`. `Stop` must return `ErrTimeout` within 3 to 10 s, with the unit `deactivating`.
  * Its cleanup kills the unit first, since cleanups run last first, and the shared cleanup's blocking `systemctl stop` would otherwise never end.
  * Its first run failed on the test itself: `Poll.Settle 10s is not less than Poll.Timeout 3s`. It now sets `Settle: -1`.
* **Seen failing:**

  | Run | Result |
  | :--- | :--- |
  | (c) `plants_37.py N3`: the fake test against HEAD's `lifecycle.go` | FAIL: `Stop still blocked after 5 s on a stop job that never ends (then: … timed out: not ready within 300ms (last: demo.service ActiveState=deactivating SubState=stop-sigterm))` |
  | `n3live_old.py`: the live test against HEAD's `lifecycle.go`, on the Linux test host | FAIL: `Stop still blocked after 30 s (then: <nil>)`. The test's KILL ended the unit, and so the blocked `systemctl` |

* **Runs:**
  * `go test ./selfupdate/service/systemd/` here: `ok`.
  * `-run ^TestLive`, with `SELFUPDATE_REQUIRE_SYSTEMD=1` in user scope, all ten pass on the Linux test host and on the WSL. `TestLiveStopInfinityTimesOut` took 3.64 s and 3.20 s.
  * No `selfupdate-live-*` unit was left on the Linux test host after any run.
* **`make pre-add-check`** on the four files: "4 file(s) clean (gofmt, golangci-lint, go vet, go test, govulncheck)".
* **Found:** 0011-MADR's systemd rules say `Stop` blocks on its job. D2-A supersedes that line, as the MADR's Amendment A2 records.

### N4: launchd's bootout and bootstrap (2026-10-27)

* **`launchd/lifecycle.go`:**
  * `Stop` returns the caller's error if its context has ended before `bootout`, as systemd's `Stop` does.
  * It then runs `bootout` on `context.WithTimeout(context.WithoutCancel(ctx), bound)`, with `stopBound`'s bound. A run that the bound ends is `ErrTimeout`, and the caller's error is joined to any failure.
  * In `bootstrap`, a `launchctl` run that fails after the bound ended is `ErrTimeout`, wrapping the run's error, as the retry loop's own deadline already was.
  * Each change has a comment citing D3-A or D4-A.
* **`launchd/fake_test.go`:**
  * `bootout`, after its effect, returns the call's context error when that context ended during the call. That is P2's kill after launchd took the request: the job goes, and `ExecRunner` reports the error.
  * `bootstrapHangs` holds every `bootstrap` until its context ends.
* **Tests:**
  * `TestStopFinishesAfterCancel` (`stopwait_test.go`) already cancelled the caller during `bootout`. Its fake then answered as if `launchctl` had not been killed, so it passed on today's code. With the fake above it covers D3-A, and its comment now says so.
  * `TestStartBootstrapHangs` (`launchd_test.go`) checks a hung `bootstrap` under a 200 ms `Poll.Timeout`: `Start` must return `ErrTimeout` wrapping `context.DeadlineExceeded`.
* **Seen failing** (`plants_37.py N4`, scratch copies):

  | Plant | `TestStopFinishesAfterCancel` | `TestStartBootstrapHangs` |
  | :--- | :--- | :--- |
  | (d) HEAD's `lifecycle.go` | FAIL: `verbs ["managername" "list" "list" "bootout"], loaded true: Stop returned while the job was still loaded` | FAIL: `Start = context deadline exceeded, want ErrTimeout wrapping the deadline` |
  | (e) `bootout` on the caller's context | FAIL, as (d) | pass |
  | (f) `bootstrap`'s run error unwrapped | pass | FAIL, as (d) |

* **Runs:**
  * `go test ./selfupdate/service/launchd/`: `ok`, here and on the Linux test host.
  * `SELFUPDATE_REQUIRE_LAUNCHD=1 go test -run ^TestLive`, on this host: all 11 pass in 164 s, `TestLiveStopWaitsForSlowExit` included (8.91 s). No test job was left loaded.
* **`make pre-add-check`** on the four files: "4 file(s) clean (gofmt, golangci-lint, go vet, go test, govulncheck)".
