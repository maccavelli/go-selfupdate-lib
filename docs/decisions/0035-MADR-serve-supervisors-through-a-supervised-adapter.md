---
status: proposed
date: 2026-10-24
decision-makers: go-selfupdate-lib maintainers
consulted: "GitHub issue #2, \"service: add runit, s6 and OpenRC backends\" (2026-10-22), and its comment, from the owner for magic-cli-remote; 0035-REPORT-library-backends-or-consumer-api-for-runit-s6-and-openrc.md; a read of `selfupdate/service` and its four backends at 009838f (v1.21.0); magic-cli-remote at be41ef9, its code and its 0054, 0207 and 0210 MADRs; runit's sv(8), runsv(8), runsvdir(8) and FAQ; s6 2.15.1.1's s6-svc, s6-svstat, s6-supervise, s6-svscan, s6-svwait and servicedir pages; OpenRC's user-guide, service-script-guide, NEWS, rc.conf and man pages; all read 2026-10-24"
informed: the fleet's programs that run under a supervisor this module has no backend for, magic-cli-remote first
---
# Serve runit, s6, OpenRC and any other supervisor through one hardened adapter in `service`, which the program feeds with its supervisor's state and commands

## Context and Problem Statement

Issue #2 asks for reference backends for runit, s6 and OpenRC. A program
on these supervisors writes its own backend today, and that backend
misses parts of the contract the library's backends keep. The consumer
is magic-cli-remote, whose `scriptBackend` serves as this record's
example (F26 to F35).

This record was first drafted as three full backends.
[0035-REPORT](../reports/0035-REPORT-library-backends-or-consumer-api-for-runit-s6-and-openrc.md)
weighed that draft against a seam the program fills, and against a
hybrid. On 2026-10-24 the owner chose the hybrid, option C:
* **Now:** the library ships one supervisor-neutral adapter. It owns the
  contract. The program supplies its supervisor's state and commands.
* **Later:** first-party backends are built on the adapter, only where a
  supervisor documents a stable interface and the demand is there.

The full-backend draft is D0-A below, so its reasoning stays on record.
Its per-supervisor facts (F12 to F25) stay, because they tell a program,
and the adapter's tests, what each supervisor does.

This record decides:
* the adapter's shape and place;
* how it identifies an instance, stops, starts, waits for health, tells
  inside, detaches and reconciles;
* its sentinel and its state call;
* the platform and the tests.

It also states what magic-cli-remote must change to use the adapter.

### Evidence tags

As in 0032-MADR and 0034-MADR:
* **[read]**: this repository's code, or the consumer's at `be41ef9`;
* **[docs]**: a supervisor's manual, quoted through a fetch tool's
  rendering on 2026-10-24, so the PLAN re-reads each one it relies on;
* **[reported]**: the issue, or the consumer's records;
* **[derived]**: a conclusion drawn from tagged facts;
* **[unverified]**: not yet measured, until the PLAN's probes settle it.

### Facts [observed 2026-10-24, `main` at `009838f`]

**The contract and what the library already has [read]**

* **F1. The four backends.** Package `service`'s documentation names
  four backends. Each implements `selfupdate.Lifecycle`,
  `EnabledLifecycle`, `Reconciler` and `Detacher`, and returns
  `ErrUnsupported` on the wrong OS (`selfupdate/service/doc.go:6-9`).
  `service` also holds generic pieces a program combines:
  `ProcessDetacher`, `ExecReconciler`, `PollHealthy` and `Runner`.
* **F2. Tools by absolute path.** A `Command`'s path "must be absolute: a
  runner never looks a tool up on PATH" (`runner.go:15-16`).
  `ExecRunner` captures each output stream, at most 1 MiB, and kills the
  tool when `ctx` ends (`runner.go:56-60`).
* **F3. The sentinels.** They are `ErrUnsupported`, `ErrNotInstalled`,
  `ErrPermission`, `ErrInsideService`, `ErrTimeout` and `ErrUnhealthy`
  (`errors.go:11-31`). None means "the service exists, but nothing
  supervises it".
* **F4. The health wait.** `PollHealthy` waits for one `Ready` instance,
  other than `Previous`, for the whole `Settle` window, default 10 s.
  * A change of `Instance` starts the window again. It does not fail
    the wait.
  * It fails at once only on a `Failed` observation.
  * With `Previous` set, an empty `Instance` never counts
    (`poll.go:94-151`).

  So a restart during the wait is caught only by the probe: systemd's
  reports `Failed` when the unit's restart count rises.
* **F5. Where `Reconcile` runs.** `NewManagedInstallerWith` refuses a
  nil `Reconciler` (`selfupdate/managed.go:70-72`). It calls `Running`,
  `Stop`, `Reconcile`, `Start` and `WaitHealthy`, in that order
  (`managed.go:157-216`). So `Reconcile` runs while the service is
  stopped.
* **F6. Stop's bounds and recovery in Task Scheduler.** Task Scheduler's
  `Stop` takes a few steps:
  * it records the instance by PID and creation time, so a reused PID
    is not mistaken for it;
  * once the stop is issued, it waits to `Poll.Timeout` whatever the
    caller's context does, and returns the context's error after
    (0015-MADR B3);
  * when it fails with the program still running, it restores the
    switch it changed (0024-MADR D4-A).

  (`taskscheduler/lifecycle.go:102-211`)
* **F7. The handoff.** `HandOffIfInside` asks a `Detacher` whether this
  process is inside, and runs `Detach` when it is (`handoff.go:96`).
  `DetachProcess` starts the run in a new session, with the caller's
  environment minus the service manager's per-unit variables
  (0033-MADR). It "is no defence against a cgroup kill (systemd) or a
  launchd job reap" (`detach.go:9-21`).
* **F8. Inside checks.** launchd's `Inside` walks this process's
  ancestors to the job's PID (`launchd/detach.go:25-55`).
  `internal/winproc.Descends` walks a snapshot, and is fed only on
  Windows (`internal/winproc/tree.go:17-26`).
* **F9. Recognising the binary.** `service.SameExecutable` matches two
  paths to one binary, by cleaned path or by file identity
  (`sameexec.go:10-17`).
* **F10. Growth rules.** A new exported surface in `service` needs no new
  `depguard` rule, since `service`'s rule already allows what it uses
  (`.golangci.yml`). `make apicheck` holds v1 to additions only.
* **F11. Two kinds of exported API.**
  * **Function fields** in an options struct grow compatibly: a new
    optional field is an addition. `ExecOptions`, `PollOptions` and
    taskscheduler's `Options.RequestStop` are of this kind.
  * **An interface** gains a method only by breaking its implementers.

  [read; Go's compatibility rules]

**The supervisors [docs]** (as gathered for the first draft)

* **F12. runit control.**
  * `sv down` sends "the TERM signal, and the CONT signal … After it
    stops, do not restart service".
  * `sv up` starts the service and restarts it if it stops.
* **F13. runit exit codes.** `sv` exits 0 on success, 1 to 99 by the
  count of failed services, including "the directory is not controlled
  by a runsv(8) process, or sv timed out", and 100 on error.
* **F14. What runit leaves undocumented.** The text `sv status`
  prints, beyond one FAQ example ("ok: run: cron: (pid 5869) 0s"). Also
  the contents of `supervise/stat` and `supervise/pid`, and any process
  group or session for `./run`.
* **F15. runit's down file.** "If the file service/down exists, runsv
  does not start ./run immediately." A user tree is a `runsvdir` run as
  the user.
* **F16. s6 control.** `s6-svc -d` sends SIGTERM, then SIGCONT, to "the
  supervised process". Only `-P`, `-C` and `-K` reach "the process
  group".
* **F17. s6 state.** `s6-svstat -o` prints "programmatically parsable"
  fields: `up`, `wantedup`, `normallyup`, `ready`, `pid`, `exitcode` and
  `updownfor`. It exits 1 when "s6-supervise not running on
  servicedir".
* **F18. s6 sessions.** "s6-supervise always spawns its child in a new
  session, as a session leader."
* **F19. s6 readiness and restarts.** Ready is `up,ready` reading
  `true true`. `wantedup` true with `up` false is a service being
  restarted, with "a minimum 1-second delay between two ./run spawns".
* **F20. s6 rescans.** `s6-svscan` does not rescan by default.
  `s6-svscanctl -a` asks it to.
* **F21. OpenRC user services.** They arrived in 0.60, marked
  experimental, and are stable from 0.62. User scripts live in
  `${XDG_CONFIG_HOME}/rc/init.d`, and "XDG_RUNTIME_DIR … must be set".
* **F22. OpenRC status.** `status`'s "return code matches the status,
  with the exception of 'started' returning 0", and the other codes are
  not given. No command prints a service's PID.
* **F23. OpenRC crashes.** A service whose daemon is gone is "marked as
  crashed". start-stop-daemon, the default, does not restart it.
* **F24. OpenRC's stop scope.** A stop signals the PID, and with
  `stopgroup` the process group. With `rc_cgroup_cleanup=YES`, "all of
  the processes in a service's cgroup [are] killed". The default
  is shown commented as `NO`. Whether user services get cgroups is
  [unverified].
* **F25. OpenRC runlevels.** A runlevel is a directory of symlinks, and
  `rc-update add <svc> default` enables a service.

**The consumer at `be41ef9` [read, reported]**

* **F26. Its backend.** `internal/cli/service/scriptbackend.go`, 247
  lines, is its runit, s6 and OpenRC user backend. `NewBackend` returns
  it for the script kinds (`backend.go:53-60`).
* **F27. Its inside check.** `Inside` walks `/proc/<pid>/stat` from this
  process to the supervised PID (`:124-155`;
  `scriptbackend_inside_linux.go`). The PID comes from the status
  output for runit and s6, and from `${XDG_RUNTIME_DIR:-/tmp}/<name>.pid`,
  which its OpenRC script writes, for OpenRC (`:91-122`).
* **F28. Its stop.** `Stop` refuses inside with `ErrInsideService`, runs
  `sv down`, `s6-svc -d` or `rc-service --user <name> stop`, then polls
  to its own 15 s with `time.Sleep`, ignoring `ctx`, and returns a plain
  error on timeout (`:162-192`).
* **F29. Its start.** When no `runsvdir` or `s6-svscan` watches the scan
  directory, or no OpenRC user session runs, `Start` starts one for
  this session, then runs `sv up`, `s6-svc -u` or `rc-service --user
  <name> start` (`:194-230`).
* **F30. Its health wait.** `WaitHealthy` runs `settle`. The service
  counts as up after two samples with the same PID, or two seconds of
  reported uptime. It fails on:
  * a reported failure, such as OpenRC's "status: crashed";
  * a PID that changes while watched;
  * an exit while the supervisor wants the service up, after it was up
    or seen twice.

  Its errors are plain (`:232-247`; `settle.go:82-135`). It does not
  run the product's application probe, which the other backends run
  (`backend.go:34-38`).
* **F31. Its state reads.** It reads state with regular expressions:
  `^run: .*\(pid (\d+)\) (\d+)s` for runit, and `^up \(pid (\d+)[^)]*\)
  (\d+) seconds` for `s6-svstat`'s human form. For OpenRC it reads the
  exit code and the text "status: crashed" (`settle.go:182-188`,
  `:198-219`).
* **F32. Its commands.** `runCmd` finds the tool by name on `PATH`
  (`procutil.Command`), and bounds it by its own 30 s rather than the
  caller's context. It streams the tool's standard output to this
  process's (`setup.go:42`, `:1369-1388`). So `sv down` and `sv up`
  print to standard output during an update [read]. With `--json`,
  that output shares the stream with the result document [derived].
* **F33. Its handoff wiring.** The update sets a handoff for any backend
  that is a `service.Detacher`, by type assertion
  (`internal/updateclient/client.go:187-217`, `:323-331`). Otherwise it
  refuses an inside run before it downloads (`refuseInside`, `:236-255`;
  its 0210-MADR D8).

  So a script backend that becomes a `Detacher` gets the handoff with
  no change to the update command [derived].
* **F34. Its reconciler.** For every backend, the update passes its own
  `service.NewExecReconciler`, which runs the new binary's
  `setup-service --refresh` (`internal/updateclient/managed.go:66-90`).
  So it does not use a backend's own `Reconcile` [derived].
* **F35. What it has planned.** Its 0207-MADR D20 (proposed) makes the
  script backend honour `ctx` and wrap the sentinels. Its 0210-MADR
  declined to hand off through `ProcessDetacher` itself, and waits for
  issue #2 ("Guard now, handoff via #2"). It reproduced an agent-run
  update leaving the service down on runit, s6 and OpenRC 0.62.6 (F2
  there).

## Decision Drivers

* **The library owns the contract.** The library owns stopping and
  starting under `ctx`, the typed errors, proving a new instance, the
  inside check and the handoff. Those are the parts the issue says
  programs get wrong (F26 to F30).
* **The program owns its supervisor.** The program owns its scripts,
  its session supervisors, its PID-file layout and the forms its tools
  print. That keeps the library's interfaces at 0011-MADR's bar,
  "Stable interfaces only" (0035-REPORT).
* **At least as strict as what it replaces.** The adapter's health wait
  catches every failure the consumer's `settle` catches (F30), and more.
* **Compatible growth.** The adapter's surface can gain fields within v1
  (F11).
* **Reuse what exists.** It uses `PollHealthy`, `DetachProcess` and its
  scrubbed environment, `HandOffIfInside`, `SameExecutable`, the
  sentinels, and the stop rules of F6.
* **Facts before code.** Each [unverified] item is probed first.

## Considered Options

### D0: who owns the supervisor-specific part

* **D0-A.** Three full backends, `service/runit`, `service/s6` and
  `service/openrc`, which parse each supervisor's state and run its
  commands. This was the first draft.
* **D0-B.** A generic adapter only.
* **D0-C.** The adapter now, and first-party backends built on it later,
  only where a supervisor documents a stable interface (s6's
  `s6-svstat -o`, F17) and a second consumer or the owner asks.

### D1: place and name

* **D1-A.** In `service`, beside `ProcessDetacher` and `ExecReconciler`:
  * `type Supervised struct`;
  * `func NewSupervised(o SupervisedOptions) (*Supervised, error)`.
* **D1-B.** A new package, `selfupdate/service/supervised`.

### D2: what the program supplies

* **D2-A.** Function fields in `SupervisedOptions`. Three are required:
  * `State func(ctx) (SupervisedState, error)`;
  * `Up func(ctx) error`;
  * `Down func(ctx) error`.

  The optional ones are `Name`, `Poll`, `Probe` and `DetachCheck`.
  `SupervisedState` is exported, JSON-tagged and documented field by
  field:
  * `Installed`, `Supervised`, `Up` and `PID`;
  * `WantUp`, `NormallyUp` and `Restarting`;
  * `Failed`, `Ready` (a `*bool`, nil when the supervisor has no
    readiness) and `Detail`.
* **D2-B.** An interface, `Supervisor{State; Up; Down}`, that the program
  implements.

### D3: the instance

* **D3-A.** `Instance` is the PID and the process's start time, from
  field 22 of `/proc/<pid>/stat`, as `"<pid>@<ticks>"`, so a reused PID
  is a new instance (F6's rule, on Linux).
* **D3-B.** The PID alone.

### D4: Stop

* **D4-A.** In order:
  1. Read `State`. A service that is not installed is `ErrNotInstalled`.
  2. Refuse with `ErrInsideService` when this process is inside (D7),
     before anything changes.
  3. Record the instance (D3) and its binary, from `/proc/<pid>/exe`
     (D9).
  4. Call `Down`.
  5. Wait until `State` reports it down and the recorded process has
     exited, bounded by `Poll.Timeout` whatever the caller's context
     does. Return the context's error after (F6). A timeout is
     `ErrTimeout`.
  6. When the stop fails with the recorded process still alive, call
     `Up`, so that the supervisor wants the service up again, as it did
     before (F6).

  There is no `RequestStop`. A supervisor that wants the service up
  restarts a program that exits on request, so the polite stop is the
  supervisor's own signal, which the program handles.
* **D4-B.** Call `Down`, and wait on `State` only, bounded by the
  caller's context.

### D5: Start

* **D5-A.**
  * A current instance, other than the one `Stop` recorded, is left
    alone.
  * Otherwise `Up` runs. The program's `Up` may start a session
    supervisor first (F29).
  * Errors from `State` or `Up` pass through, wrapped with the step.
* **D5-B.** `Up` always.

### D6: WaitHealthy

* **D6-A.** `PollHealthy`, with `Previous` set to the instance `Stop`
  recorded, over a probe that reports `Failed` in four cases:
  * when `State.Failed`;
  * when the instance changes after one has counted, which is a restart
    during the wait;
  * when `State.Restarting`, a "down, want up", is seen after the
    service was up;
  * when it is seen twice in a row.

  Then `Options.Probe` runs, the program's application check, as for
  the other backends. This keeps every failure of F30, and the settle
  window and timeout are the program's, through `Poll`.
* **D6-B.** `PollHealthy` over `State.Up` alone.

### D7: inside

* **D7-A.** This process is inside when it is the supervised process, or
  descends from it. That is read by walking `/proc/<pid>/stat`'s parent
  field to PID 1:
  * the walk is bounded;
  * an unreadable step is an error, never "outside";
  * the target's start time must match the instance, so a reused PID
    does not count.

  That is the consumer's rule (F27). It is also the reach of every
  supervisor's stop (F12, F16, F24).
* **D7-B.** Descent, or a shared process group.

### D8: Detach

* **D8-A.** `Options.DetachCheck`, when set, runs first. Its error
  refuses the handoff, wrapped with `ErrUnsupported`. For an OpenRC
  service under `rc_cgroup_cleanup=YES`, the program refuses there
  (F24). Then `DetachProcess` runs, with its new session and scrubbed
  environment (F7).
* **D8-B.** A built-in cgroup rule: refuse when this process shares the
  supervised process's cgroup.
* **D8-C.** `DetachProcess` with no check.

### D9: Reconcile

* **D9-A.** Verify only.
  * `Reconcile` compares the binary that `Stop` recorded with the
    executable, by `SameExecutable` (F9).
  * A match is `Changed: false`.
  * No record, because the service was not running, is `Changed: false`
    with a warning that it could not check.
  * A mismatch is an error that names `NewExecReconciler`.
  * `Restore` does nothing.

  A program that owns its definition passes its own reconciler, as the
  consumer does (F34).
* **D9-B.** `Supervised` implements no `Reconciler`.

### D10: no supervisor

* **D10-A.** A new sentinel, `service.ErrNoSupervisor`: "the service
  exists, but nothing supervises it". `State` returns it, wrapped, and
  the adapter passes it on.
  * `Running` reads it as not running, unless the recorded process is
    alive.
  * `Start`'s `Up` is where a program starts a supervisor, so the
    sentinel reaches the caller only when the program chose not to.
* **D10-B.** No sentinel: the program picks an existing one.

### D11: a state call

* **D11-A.** `Supervised.Status(ctx) (SupervisedState, error)`, the
  program's own `State`, with the adapter's `Instance` filled in.
* **D11-B.** None.

### D12: platform

* **D12-A.** Linux. `NewSupervised` returns `ErrUnsupported` elsewhere,
  as the adapter reads `/proc` for D3, D7 and D9.
* **D12-B.** Every Unix, with `ps` for the walk and no start time or
  binary.

### D13: where the `/proc` reads live

* **D13-A.** Unexported, in `service` (`supervised_linux.go`), which is
  its only user.
* **D13-B.** In `internal/winproc`, beside `Descends`.

### D14: tests

* **D14-A.**
  * Fake `State`, `Up` and `Down` functions for every path and error.
  * Real processes on Linux for descent, a reused PID, the recorded
    binary and the detached run.
  * A live test that drives the adapter over a real s6 tree on CI's
    Ubuntu runner, as the runner user, behind
    `SELFUPDATE_REQUIRE_SUPERVISED`. Its test-only `State` reads
    `s6-svstat -o`, the one documented form (F17).
  * Every new test is seen failing first.
* **D14-B.** Fakes only.

## Decision Outcome

* **D0: "D0-C", chosen by the owner on 2026-10-24** after 0035-REPORT.
  The library owns the contract, and the program owns its supervisor.
  A first-party s6 backend is left to a later record.

Proposed for the owner, D1 to D14:

* **D1: "D1-A",** because `ProcessDetacher`, `ExecReconciler` and
  `PollHealthy` already sit in `service` as generic pieces a program
  combines (F1), and a new package would need its own rule for no gain.
* **D2: "D2-A", function fields,** because they grow compatibly within
  v1 and an interface does not (F11). They also match `HealthProbe` and
  `RequestStop`, so a program wraps the code it has (F31, F29).
  `Ready` is a pointer, so "no readiness" is not "not ready".
* **D3: "D3-A",** because the stop and the health wait compare
  instances, and a PID alone can be reused in between (F6).
* **D4: "D4-A",** because the managed installer's recovery relies on a
  stop that either completes or leaves the service as it was (F6). A
  stop that gives up early leaves a process that the supervisor no
  longer wants up, and a crash there would not be restarted.
* **D5: "D5-A",** because an instance already running needs no `Up`, and
  the program's `Up` keeps its session logic (F29) without a wrapper.
  Under D0-A, a library `Start` that returned `ErrNoSupervisor` would
  have forced the program to wrap the backend to start one. That would
  bring back the consumer-side lifecycle the issue wanted gone.
* **D6: "D6-A",** because it keeps every failure the consumer's `settle`
  detects (F30), on the library's tested wait (F4). It also adds the
  application probe, which the consumer's script backend lacks.
* **D7: "D7-A",** because descent is what each supervisor's stop and
  the consumer's teardown reach (F12, F16, F24, F27). A shared group
  adds false positives, and s6's child is a session leader anyway
  (F18).
* **D8: "D8-A",** because only the program knows whether its OpenRC
  service runs under cgroup cleanup (F24). A built-in cgroup rule would
  refuse a runit or s6 tree that runs inside a desktop session's cgroup,
  which those supervisors never kill. With no check, the update is
  killed halfway, and the service is left down.
* **D9: "D9-A",** because the managed installer requires a
  `Reconciler` (F5), and verifying the recorded binary needs no script.
  The consumer passes its own (F34).
* **D10: "D10-A",** because the state is distinct, fixable, and common
  after a logout (F29). It joins `service`'s errors for any adapter
  user, and for a first-party backend later.
* **D11: "D11-A",** because a program's `doctor` and status commands
  then read the same state the update does, as with launchd's and Task
  Scheduler's `Status`.
* **D12: "D12-A",** because every fact and test host is Linux, and the
  hardening rests on `/proc`.
* **D13: "D13-A",** because the adapter is the only user, and
  `internal/winproc`'s role is starting processes and Windows liveness.
* **D14: "D14-A",** because fakes alone cannot show that the adapter
  meets a real supervisor. s6 is the supervisor with a documented form,
  which also prepares the later first-party backend.

### Hardening, together

* **Every wait** is bounded by `Poll`. A stop's wait outlives the
  caller's context, and returns that context's error after (D4).
* **Every refusal** comes before anything changes: inside, not
  installed, and a failed `DetachCheck`.
* **Instances** are `PID@start-time`, so a reused PID never passes for
  the old or the new instance (D3, D7).
* **A failed stop** puts the supervisor's want back (D4).
* **A restart, a crash loop or a failure** during the health wait fails
  it at once (D6).
* **Errors** wrap the sentinels:
  * `ErrInsideService`, `ErrNotInstalled`, `ErrNoSupervisor`,
    `ErrTimeout`, `ErrUnhealthy` and `ErrUnsupported`;
  * the program's own error inside, so `errors.Is` sees both.
* **The handoff** is `HandOffIfInside` over the adapter's `Inside` and
  `Detach`. That gives the scrubbed environment of 0033-MADR, the result
  file and `ReportFunc`, as on the other backends.
* **`Supervised`** is safe for concurrent use. The updater drives one
  product's calls in order, as 0025-MADR D4 says for every backend.

### What magic-cli-remote does to use it

Each change below follows from F26 to F35. The issue opened there
restates them.

1. **Build a `service.Supervised` per script kind** in `NewBackend`, in
   place of `scriptBackend`:
   * `State` from its samplers;
   * `Up` from its `Start`, with the session supervisor start kept;
   * `Down` from its stop commands;
   * `Probe` from `HealthOptions`.
2. **Make `State`, `Up` and `Down` honour `ctx`**, find tools by
   absolute path, and capture rather than stream output. Its own runner
   uses `PATH`, its own timeout, and the terminal (F32). A
   `service.Runner` does all three.
3. **Read s6 with `s6-svstat -o`**, the documented form (F17), in place
   of the human-form regular expression (F31). Keep runit's and
   OpenRC's parses, with their fixtures and live runs.
4. **Fill `Restarting` and `Failed`** as `settle` does today (F30), so
   the adapter's wait fails on the same cases.
5. **Return `service.ErrNoSupervisor`** from `State` when no supervisor
   serves the service, and keep starting one in `Up` (F29).
6. **Set `DetachCheck` for OpenRC**, refusing when cgroup cleanup is on
   for the user's services. The library's probes measure that first
   (F24).
7. **Delete what the adapter replaces:** the inside walk, the stop poll,
   the script `WaitHealthy`, and `settle` for script kinds. Keep
   `refuseInside` for any backend that is not a `Detacher`.
8. **Keep its own reconciler** (F34), and its scripts, its PID file and
   its session supervisors.
9. **Retire its 0207-MADR D20 and its 0210-MADR's wait on #2** into the
   adoption. Re-run its 0210 R1 reproduction, which must now end with
   the update handed off and the service restarted on runit, s6 and
   OpenRC.

### Consequences

* Good, because the consumer gets the handoff, `ctx`, the sentinels and
  the application probe on these supervisors in one library release,
  through a type assertion it already makes (F33).
* Good, because the library parses no undocumented output. Any
  supervisor, named here or not, uses the same adapter.
* Good, because the hardening is written and tested once. A future
  first-party backend is a `State`, an `Up` and a `Down`.
* Neutral, because the program keeps its parsing, and keeps
  responsibility for its correctness.
* Neutral, because `DetachCheck` puts OpenRC's cgroup judgement on the
  program, which is the only party that can make it.
* Bad, because `service` grows by one type, one options struct, one
  state struct and one sentinel, all permanent within v1.
* Bad, because Linux only: runit and s6 on a BSD wait for a later
  record.

### Confirmation

* **Re-read.** Each [docs] fact a test relies on is re-read from its
  page.
* **Probes,** on the Linux test host as the login user, and in an Alpine
  container for OpenRC:
  * field 22 of `/proc/<pid>/stat`, and `/proc/<pid>/exe`, for a
    supervised process;
  * whether a process started in a new session by the service survives
    `sv down` and `s6-svc -d`;
  * for OpenRC user and system services, the process's
    `/proc/self/cgroup`. With `rc_cgroup_cleanup=YES`, is a detached run
    killed? This tells the consumer what its `DetachCheck` must test.
* **Fake tests, one per path:**
  * the inside refusal, not installed, and no supervisor;
  * a stop that completes;
  * a stop that times out, which calls `Up` again;
  * a stop whose context ends, which waits on and returns the context's
    error;
  * a start that is left alone, and a start that runs `Up`;
  * a health wait that settles;
  * a health wait that fails each way: `Failed`, a restart, and
    `Restarting` once after up or twice;
  * the application probe;
  * `DetachCheck`'s refusal;
  * reconcile's match, warning and mismatch.
* **Real-process tests on Linux:**
  * descent;
  * a reused PID, read as a new instance through a mismatched start
    time;
  * the recorded binary;
  * `Detach`'s run surviving its parent.
* **The live s6 test** on CI's Ubuntu runner and the Linux test host:
  stop, start and health wait, and a handoff from inside the service
  that completes.
* **Seen failing.** Each test fails on a planted break. The breaks: no
  inside check; the stop bound by the caller's context; no `Up` after a
  failed stop; the PID alone as the instance; no restart detection; no
  `DetachCheck` call.
* **`make apicheck`:** additions only.
* **The consumer's adoption** passes its own reproduction (step 9 above).
  That is the consumer's record. It is linked from the PLAN's closing.

## Pros and Cons of the Options

### D0-A. Full backends

* Good, because a program writes only its scripts.
* Bad, because two of the three rest on undocumented forms (F14, F22),
  below 0011-MADR's bar.
* Bad, because the consumer would still keep its scripts, PID layout and
  session start. It would also need a wrapper around a library `Start`
  to start a supervisor (D5).
* Bad, because the handoff arrives one backend per release.

### D0-B. The adapter only

* Good, because it serves every supervisor in one release.
* Neutral, because it closes the door on first-party backends unless a
  later record reopens it.

### D0-C. The adapter, then first-party backends where documented

* Good, because it has every benefit of D0-B, and a path to a library
  backend for s6.

### D1-A. In `service`

* Good, because it sits with the other generic pieces, with no new rule.

### D1-B. A new package

* Bad, because there is a new rule and a new import for one type.

### D2-A. Function fields

* Good, because new optional fields are additions within v1.
* Good, because a program adapts the code it has.

### D2-B. An interface

* Good, because it suits a program with state.
* Bad, because any method added later breaks every implementer.

### D3-A. PID and start time

* Good, because a reused PID cannot pass for an instance.

### D3-B. PID only

* Bad, because a stop, then a quick reuse of the PID by another
  process, reads as the old instance still running, or as a new start.

### D4-A. Ordered stop, bound past the context, want restored

* Good, because it is the same discipline as Task Scheduler's (F6).

### D4-B. Down and a context-bound wait

* Bad, because a cancelled context mid-stop leaves the service in an
  unknown state, with the supervisor holding it down.

### D5-A. Start leaves a current instance alone

* Good, because it keeps idempotence and the program's session logic.

### D5-B. Always `Up`

* Neutral, because `Up` is usually idempotent. But it cannot see an
  instance that came up on its own.

### D6-A. Failure-aware probe, then the application probe

* Good, because it is at least as strict as the consumer's `settle`
  (F30).

### D6-B. `Up` only

* Bad, because a crash loop that is up at each sample passes.

### D7-A. Descent, start-time checked

* Good, because it matches the stops' reach and the consumer's rule.

### D7-B. Descent or group

* Bad, because it adds false positives.

### D8-A. A program's check, then `DetachProcess`

* Good, because the judgement sits where the knowledge is.

### D8-B. A built-in cgroup rule

* Bad, because it refuses safe handoffs under runit and s6 in a session
  cgroup.

### D8-C. No check

* Bad, because under cgroup cleanup the update dies after the stop.

### D9-A. Verify only

* Good, because it keeps the contract with no script parsing.

### D9-B. No Reconciler

* Bad, because `NewManagedInstaller` refuses that (F5).

### D10-A. `ErrNoSupervisor`

* Good, because it is precise, and is shared by every user of
  `service`.

### D10-B. No sentinel

* Bad, because `ErrNotInstalled` misreports a service that exists.

### D11-A. Status

* Good, because one call serves `doctor` and the update alike.

### D11-B. None

* Bad, because a program calls `State` and computes the instance
  itself.

### D12-A. Linux

* Good, because every fact and test host is Linux.

### D12-B. Every Unix

* Bad, because it would lose D3's start time and D9's binary there.

### D13-A. In `service`

* Good, because it is next to its one user.

### D13-B. In `internal/winproc`

* Bad, because it mixes Linux process reads into the package that starts
  processes.

### D14-A. Fakes, real processes and a live s6 tree

* Good, because it proves the adapter against a real supervisor.
* Bad, because it adds one CI step.

### D14-B. Fakes only

* Bad, because nothing meets a real supervisor.

## More Information

* Issue: <https://github.com/maccavelli/go-selfupdate-lib/issues/2>.
* Report:
  [0035-REPORT](../reports/0035-REPORT-library-backends-or-consumer-api-for-runit-s6-and-openrc.md).
* Records:
  * [0011-MADR](0011-MADR-reference-service-lifecycles.md), the
    contract;
  * [0015-MADR](0015-MADR-remediate-third-debugging-pass-findings.md)
    B3 and
    [0024-MADR](0024-MADR-documentation-pass-after-v1-15-1.md) D4-A,
    the stop's discipline;
  * [0032-MADR](0032-MADR-task-scheduler-registration-and-one-shot-handoff.md)
    D4-A, the `Status` shape;
  * [0033-MADR](0033-MADR-scrub-service-manager-environment-from-handoffs.md),
    the handoff's environment.
* Manuals:
  * runit: <http://smarden.org/runit/sv.8.html>,
    <http://smarden.org/runit/runsv.8.html>,
    <http://smarden.org/runit/faq.html>;
  * s6: <https://skarnet.org/software/s6/s6-svc.html>,
    <https://skarnet.org/software/s6/s6-svstat.html>,
    <https://skarnet.org/software/s6/s6-supervise.html>;
  * OpenRC: <https://github.com/OpenRC/openrc/blob/master/user-guide.md>,
    <https://github.com/OpenRC/openrc/blob/master/NEWS.md>,
    <https://github.com/OpenRC/openrc/blob/master/etc/rc.conf>.
* The consumer: magic-cli-remote at `be41ef9`:
  * `internal/cli/service/scriptbackend.go`, `settle.go`, `setup.go`
    and `backend.go`;
  * `internal/updateclient/client.go` and `managed.go`;
  * its 0054, 0207 and 0210 MADRs.

  Its adoption is tracked in
  <https://github.com/maccavelli/magic-cli-remote/issues/27>.
* The PLAN follows once the owner decides D1 to D14.
