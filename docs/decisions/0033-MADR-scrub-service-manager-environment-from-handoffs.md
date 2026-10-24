---
status: accepted
date: 2026-10-23
decision-makers: go-selfupdate-lib maintainers
consulted: "GitHub issue #7, \"service: Detach copies the caller's per-unit service-manager variables into the detached run\" (2026-10-23), from the owner for magic-cli-remote; a read of `selfupdate/service` and its backends at a6af9dc (v1.19.0); systemd.exec(5) and systemd-run(1) of systemd 259 on the Linux test host, read 2026-10-23; sd_notify(3) and sd_listen_fds(3), read 2026-10-23; launchd.plist(5) and launchd(8) on this Mac"
informed: the fleet's programs that run as a systemd unit, a launchd job, a Windows service or a scheduled task and update from inside it, magic-cli-remote first
---
# Leave the service manager's per-unit variables out of every environment the library passes on, with an exported list and filter, and the caller's explicit variables still winning

## Context and Problem Statement

Issue #7 reports that a handoff copies the caller's whole environment into
its detached run. That includes variables a service manager set for the
caller's own unit. A run in another unit then holds the caller's
`NOTIFY_SOCKET`, `WATCHDOG_*`, `LISTEN_*`, `INVOCATION_ID` and others.

The issue's evidence: on a Linux test host, a consumer's `Type=notify`
user service handed off an update, and systemd logged "Got notification
message from PID <pid>, but reception is disabled" for the transient
unit. Some process in that unit had inherited the caller's
`NOTIFY_SOCKET`.

This record decides which names to leave out, in which of the library's
process starts, on which systems, whether the caller can still pass one
on, and whether the list is exported. It answers the issue's three open
questions.

### Evidence tags

As in 0032-MADR: **[read]** this repository's code; **[docs]** a
manual, quoted; **[measured upstream]** in the consumer's records;
**[derived]**; **[unverified]** until the PLAN's probes settle it.

### Facts [observed 2026-10-23, `main` at `a6af9dc`]

**Where the library passes an environment on [read]**

* **F1.** `service.DetachProcess` starts its run with
  `append(os.Environ(), spec.Env...)` (`selfupdate/service/detach.go:30`).
  The `scm` backend and `taskscheduler`'s `HopDetach` use it.
* **F2.** The hop starts the real run with its own `os.Environ()`, less
  the hop variables (`selfupdate/service/handoffhop.go:55`).
* **F3.** `ExecReconciler` runs the new binary with
  `append(os.Environ(), r.o.Env...)` (`selfupdate/service/execreconciler.go:176`).
* **F4.** `systemd.Unit.Detach` writes `append(os.Environ(), spec.Env...)`
  as an environment file (`selfupdate/service/systemd/detach.go:209`),
  and passes it to `systemd-run` as `-p EnvironmentFile=<file>`
  (`:108`).
* **F5.** `launchd.Job.Detach` writes `append(os.Environ(), spec.Env...)`
  to the private file the run loads (`selfupdate/service/launchd/detach.go:100`).
* **F6.** `taskscheduler.Task.Detach` writes
  `append(append(os.Environ(), spec.Env...), EnvHandOffHop=1)` to the same
  kind of file (`selfupdate/service/taskscheduler/handoff.go:78`),
  added in 0032.
* **F7.** `service.LoadHandOffEnv` applies a file with `os.Setenv`, after
  the run has started. So in a launchd or Task Scheduler run, a file's
  value replaces whatever the manager set for that run [read, derived].
* **F8.** `systemd.Notify` reads `NOTIFY_SOCKET` on every call
  (`selfupdate/service/systemd/notify.go:22`), so a run that calls it with
  the caller's value sends to the caller's socket [read].

**What systemd sets, and which source wins [docs]** (systemd.exec(5),
systemd 259, "Environment Variables in Spawned Processes")

* **F9. An environment file wins over the manager's own variables.**
  "For each invoked process the list of environment variables set is
  compiled from the following sources": the manager's configured
  defaults, "Variables defined by the service manager itself", the
  manager's own environment block, `Environment=`, and "Variables read
  from files specified via EnvironmentFile=". "If the same environment
  variable is set by multiple of these sources, the later source —
  according to the order of the list above — wins."
  So through F4, the caller's `INVOCATION_ID`, `JOURNAL_STREAM`,
  `SYSTEMD_EXEC_PID` and the rest don't just leak into the transient
  unit: they replace the values systemd sets for it. A name systemd sets
  only for the caller's unit, such as `NOTIFY_SOCKET`, arrives in a unit
  that has no such socket [derived]. Whether the replacement happens as
  derived is [unverified until probe K1-2]. *(Measured, Amendment A1,
  K1-2: the detached run's `INVOCATION_ID` was the caller's, not its own
  unit's, and it held the caller's `NOTIFY_SOCKET`, `JOURNAL_STREAM`,
  `SYSTEMD_EXEC_PID` and `MEMORY_PRESSURE_*`.)*
* **F10. The variables systemd sets for a unit's processes**, with what
  each describes:
  * the user and session: `PATH`, `LANG`, `USER`, `LOGNAME`, `HOME`,
    `SHELL`, `XDG_RUNTIME_DIR`, `TERM`;
  * the unit's run: `INVOCATION_ID` ("identifying each runtime cycle of
    the unit"), `SYSTEMD_EXEC_PID` ("The PID of the unit process"),
    `MAINPID`, `MAINPIDFDID`, `PIDFILE`, `LOG_NAMESPACE`,
    `DEBUG_INVOCATION`;
  * the unit's channels to its manager: `NOTIFY_SOCKET`,
    `WATCHDOG_PID`, `WATCHDOG_USEC`, `LISTEN_FDS`, `LISTEN_PID`,
    `LISTEN_PIDFDID`, `LISTEN_FDNAMES`, `FDSTORE`,
    `MEMORY_PRESSURE_WATCH`, `MEMORY_PRESSURE_WRITE`, `JOURNAL_STREAM`;
  * the manager: `MANAGERPID`, `MANAGERPIDFDID`;
  * why the unit ran: `SERVICE_RESULT`, `EXIT_CODE`, `EXIT_STATUS`,
    `MONITOR_SERVICE_RESULT`, `MONITOR_EXIT_CODE`, `MONITOR_EXIT_STATUS`,
    `MONITOR_INVOCATION_ID`, `MONITOR_UNIT`, `TRIGGER_UNIT`,
    `TRIGGER_PATH`, `TRIGGER_TIMER_REALTIME_USEC`,
    `TRIGGER_TIMER_MONOTONIC_USEC`, `REMOTE_ADDR`, `REMOTE_PORT`,
    `SO_COOKIE`;
  * the unit's directories: `RUNTIME_DIRECTORY`, `STATE_DIRECTORY`,
    `CACHE_DIRECTORY`, `LOGS_DIRECTORY`, `CONFIGURATION_DIRECTORY`,
    `CREDENTIALS_DIRECTORY`, and `TMPDIR` (set to `/tmp` in one
    `PrivateTmp=disconnected` case).
* **F11. Two of those directories go away with the unit.** "If set to no
  (the default), the directories specified in RuntimeDirectory= are
  always removed when the service stops" (`RuntimeDirectoryPreserve=`).
  The credentials directory is "per-unit" and "only accessible to the UID
  associated with the unit"; whether it outlives the unit is not stated
  [unverified until probe K1-3]. *(Measured, Amendment A1, K1-3: it is
  gone once the unit has stopped.)* The state, cache, logs and configuration
  directories are persistent paths, valid for another process of the same
  program [docs, derived].
* **F12. sd_notify(3) and sd_listen_fds(3) name the hazard.** With
  `unset_environment`, `sd_notify()` unsets `$NOTIFY_SOCKET`: "Further
  calls to sd_notify() will then silently do nothing, and the variable is
  no longer inherited by child processes." `sd_listen_fds()` unsets
  `$LISTEN_FDS`, `$LISTEN_PID`, `$LISTEN_PIDFDID` and `$LISTEN_FDNAMES`
  the same way. A program that does not set the parameter passes them to
  every child [docs].

**Other managers**

* **F13. launchd** documents only `EnvironmentVariables`, "additional
  environmental variables to be set before running the job"
  (launchd.plist(5)) [docs]. Whether launchd sets per-job variables of its
  own, such as `XPC_SERVICE_NAME`, is not documented
  [unverified until probe K1-4]. If it does, F5 and F7 replace the
  one-shot job's value with the caller's [derived]. *(Measured, Amendment
  A1, K1-4: it sets `XPC_SERVICE_NAME` to the job's label, and
  `XPC_FLAGS`.)*
* **F14. Windows.** Neither the Service Control Manager nor Task
  Scheduler documents per-service variables [unverified until probe
  K1-5]. *(Measured, Amendment A1, K1-5: neither sets one.)*
* **F15. The consumer** (magic-cli-remote,
  `0210-MADR-hand-off-agent-run-updates-outside-the-service.md`,
  amendment A, F5) recorded the journal line in the issue. Its daemon is
  a `Type=notify` user unit with no `NotifyAccess=`. It will unset
  `NOTIFY_SOCKET` in its daemon at start-up, and waits on nothing here
  [measured upstream].

## Decision Drivers

* A detached run, and anything it starts, acts for its own unit, never
  the caller's.
* A program that needs a variable passed on can still pass it on.
* No behaviour change on a system that sets none of these names.
* Additive API only; `make apicheck` is compatible with `v1.19.0`.
* The list cites the manuals and is pinned by tests; the claims the
  manuals don't settle are measured first.

## Considered Options

### D1: which names are left out

* **D1-A.** The issue's list: `NOTIFY_SOCKET`, `WATCHDOG_PID`,
  `WATCHDOG_USEC`, `LISTEN_PID`, `LISTEN_FDS`, `LISTEN_FDNAMES`,
  `MAINPID`, `MANAGERPID`, `INVOCATION_ID`, `JOURNAL_STREAM`,
  `SYSTEMD_EXEC_PID`.
* **D1-B.** Every variable F10 lists as describing the unit's run, its
  channels to its manager, its manager, or why it ran. That is D1-A
  plus `LISTEN_PIDFDID`, `MAINPIDFDID`, `MANAGERPIDFDID`, `PIDFILE`,
  `LOG_NAMESPACE`, `DEBUG_INVOCATION`, `FDSTORE`,
  `MEMORY_PRESSURE_WATCH`, `MEMORY_PRESSURE_WRITE`, `SERVICE_RESULT`,
  `EXIT_CODE`, `EXIT_STATUS`, the five `MONITOR_*`, the four `TRIGGER_*`,
  `REMOTE_ADDR`, `REMOTE_PORT` and `SO_COOKIE`. The user and session
  variables stay.
* **D1-C.** D1-B, plus any per-job variable probe K1-4 measures launchd
  setting, and any per-service variable probe K1-5 measures on Windows.
  *(Amendment A1: `XPC_SERVICE_NAME` and `XPC_FLAGS`; none on Windows.)*

### D2: the unit's directory variables

* **D2-A.** Leave all of F10's directory variables out.
* **D2-B.** Keep them all.
* **D2-C.** Leave out `RUNTIME_DIRECTORY`, which F11 says is removed when
  the unit stops, and `CREDENTIALS_DIRECTORY`, which is per-unit and
  private to the unit's user. Keep `STATE_DIRECTORY`, `CACHE_DIRECTORY`,
  `LOGS_DIRECTORY`, `CONFIGURATION_DIRECTORY` and `TMPDIR`: persistent or
  general paths the same program's run may use to find its data.

### D3: which process starts (the issue's first question)

* **D3-A.** Every one where the library passes an environment on:
  `DetachProcess` and the hop (F1, F2), `ExecReconciler` (F3), and the
  systemd, launchd and Task Scheduler handoffs (F4–F6).
* **D3-B.** The handoffs only, not `ExecReconciler`.

### D4: on which systems (the issue's second question)

* **D4-A.** On every system. A name that is not set is not in the
  environment, so nothing changes where no manager set it. A launchd job
  or a Windows task can still be a child of a process that has them.
* **D4-B.** On Linux only.

### D5: the caller's explicit variables

* **D5-A.** `HandOff.Env` and `ExecOptions.Env` are appended after the
  filter, so a caller who names one of these variables there still
  passes it on, as the issue proposes.
* **D5-B.** The filter applies to them too.

### D6: an export (the issue's third question)

* **D6-A.** `service.ManagerEnvNames() []string`, the list, and
  `service.ScrubManagerEnv(env []string) []string`, which returns env
  without them, for a program to apply to the other processes it starts.
* **D6-B.** Unexported.

## Decision Outcome

Chosen options (the owner, 2026-10-23, who committed this pair and said
"committed proceed" after it was presented with these options proposed):
"D1-C", "D2-C", "D3-A", "D4-A", "D5-A" and "D6-A", released as
`v1.20.0`, a minor release because D6-A adds API. Amendment A1 fills
D1-C's launchd and Windows parts from the probes.

* **D1-C** takes every variable systemd documents as being about the
  unit and not about the user (F10). Each of them describes something
  the run is not: the caller's invocation, its sockets and descriptors,
  its main process, or why it ran. D1-A leaves out variables of the same
  kind that are newer than the issue's list, such as the `*PIDFDID`
  forms, `FDSTORE` and `MEMORY_PRESSURE_*`. The launchd and Windows
  additions are held to what the probes measure, rather than guessed.
* **D2-C** drops what is wrong outside the unit: a runtime directory
  removed when the unit stops (F11), and credentials private to the unit.
  It keeps the persistent paths, which are still right for the same
  program. D2-A would leave a program that finds its state through
  `$STATE_DIRECTORY` unable to find it in the run. D2-B passes on paths
  that vanish mid-update.
* **D3-A** is one rule: the library never passes these names into a
  process of its own making. The reconcile command is the run's child,
  and a reconcile that runs `systemctl daemon-reload`, which the issue
  suspects of sending the notification, is exactly the process that
  should not have them.
* **D4-A** costs nothing where the names are absent. On macOS or Windows,
  the caller may itself have been started from a systemd unit, as under
  WSL.
* **D5-A** leaves the caller in control, which the issue asks for.
* **D6-A** lets a program apply the same rule to the processes it starts
  itself, the consumer's daemon's children included. The list is a
  function, so it cannot be changed by a caller.

### Consequences

* Good, because a handoff's transient unit, one-shot job or task runs
  with its own manager's values, and nothing it starts talks to the
  caller's sockets.
* Good, because the reconcile command no longer carries the caller's
  unit identity.
* Neutral, because a program that read one of these names in its
  detached run sees it unset there, or set to its own unit's value. It
  can pass one on through `HandOff.Env`.
* Neutral, because `ManagerEnvNames` will grow as systemd documents new
  variables. Each addition is a behaviour change, listed in the
  changelog.
* Bad, because the list must follow systemd.exec(5).

### Confirmation

* **Probes (0033-PLAN K1):**
  * K1-1: the variables a `Type=notify` user unit's process has on
    systemd 259, from `systemd-run --user`.
  * K1-2: the same unit hands off through `systemd.Unit.Detach`. The
    transient unit's process shows whether the caller's `INVOCATION_ID`
    and `JOURNAL_STREAM` replaced its own (F9), and whether it has the
    caller's `NOTIFY_SOCKET`.
  * K1-3: whether a unit's `CREDENTIALS_DIRECTORY` exists after the unit
    stops.
  * K1-4: the variables a launchd job has that the loading shell does
    not.
  * K1-5: the same for a Task Scheduler task's process, and for a Windows
    service, where the host allows it.
* **Tests:**
  * the filter, its list and the explicit-variable rule, on every OS;
  * each of F1–F6's process starts, with `NOTIFY_SOCKET` and the others
    set in the test's environment, showing them absent from what is
    passed on, and present when given in `HandOff.Env`;
  * every new test seen failing on a planted break.
* **Live:**
  * systemd's live handoff test, under a `Type=notify` unit on the Linux
    test host, shows the run's `INVOCATION_ID` is its own unit's and
    `NOTIFY_SOCKET` is unset;
  * launchd's live handoff test shows any per-job variable K1-4 finds is
    the one-shot job's own.
* `make apicheck` is compatible with `v1.19.0`; `make gate` ends
  `overall=0`; the tag's CI passes.

## Pros and Cons of the Options

### D1-A. The issue's list

* Good, because it is the measured case.
* Bad, because it leaves out newer variables of the same kind.

### D1-B. Every per-unit systemd variable

* Good, because it follows the manual's own list.
* Neutral, because systemd adds variables over time.

### D1-C. D1-B and what the probes find elsewhere

* Good, because each name has a source.
* Neutral, because the launchd and Windows parts wait on K1.

### D2-A. Drop every directory variable

* Bad, because a program's persistent paths disappear from its run.

### D2-B. Keep them all

* Bad, because the runtime directory is removed when the unit stops.

### D2-C. Drop runtime and credentials, keep the persistent ones

* Good, because what is kept stays valid.
* Neutral, because it rests on K1-3 for the credentials directory.

### D3-A. Every process start

* Good, because there is one rule, in one place.

### D3-B. Handoffs only

* Bad, because the reconcile command still carries them.

### D4-A. Every system

* Good, because nothing changes where the names are unset.

### D4-B. Linux only

* Bad, because a child of a unit can run elsewhere, as under WSL.

### D5-A. Explicit variables win

* Good, because the caller can still pass one on deliberately.

### D5-B. Filter them too

* Bad, because a deliberate choice is overridden.

### D6-A. Export

* Good, because a program can apply the same rule to its own children.
* Neutral, because it is API to keep.

### D6-B. Unexported

* Bad, because every program writes the list again.

## Amendment A1 (2026-10-23): probe results

Measured by
[0033-PLAN-scrub-service-manager-environment-from-handoffs.md](0033-PLAN-scrub-service-manager-environment-from-handoffs.md)
K1: on the Linux test host (systemd 259, user scope), with the library's
own `systemd.Unit.Detach` from the working tree; on this Mac; and on the
Windows test host. No probe tripped a decision point.

| Probe | Question | Result | Effect |
| :--- | :--- | :--- | :--- |
| K1-1 | What a `Type=notify` user unit's process has | Of F10's per-unit names: `INVOCATION_ID`, `SYSTEMD_EXEC_PID`, `NOTIFY_SOCKET`, `MEMORY_PRESSURE_WATCH`, `MEMORY_PRESSURE_WRITE`, `JOURNAL_STREAM`, `MANAGERPID`, `MANAGERPIDFDID`; the rest of its environment is the user manager's | `MEMORY_PRESSURE_*`, outside the issue's list, are set by default: D1-B's wider list over D1-A |
| K1-2 | Does a handoff's environment file replace the transient unit's own values (F9) | The detached run's `INVOCATION_ID` equalled the caller's and not its own unit's (which `systemctl show` gave as different); it had the caller's `NOTIFY_SOCKET`, though its unit's `NotifyAccess` is `none`, and the caller's `JOURNAL_STREAM`, `SYSTEMD_EXEC_PID`, `MEMORY_PRESSURE_WATCH` and `MEMORY_PRESSURE_WRITE` | F9 holds: the file replaces systemd's own values. D1 stands |
| K1-3 | Does a unit's `CREDENTIALS_DIRECTORY` outlive it | Inside the unit it was `/run/user/<uid>/credentials/<unit>` and held the credential; once the unit had stopped, the path was gone | D2-C's reason holds |
| K1-4 | What launchd sets for a job | Two LaunchAgents' environments differed only in `XPC_SERVICE_NAME`, each its own label; both also had `XPC_FLAGS`, which launchd sets, and `OSLogRateLimit`, with the same value | `XPC_SERVICE_NAME` and `XPC_FLAGS` join the list under D1-C: the one-shot job's own would otherwise be replaced by the caller's (F7). `OSLogRateLimit` names no job, and stays |
| K1-5 | What Task Scheduler and the Service Control Manager set | Two tasks' environments were the same as each other, and so were two services'; no value named its task or service | Nothing joins the list for Windows |

After the probes, no `selfupdate-probe-*` unit, LaunchAgent, task or
service was left on any host, and their scratch directories were removed.

## More Information

* Issue: GitHub issue #7 of this repository. It is closed when the
  release ships, on the owner's ask.
* [0011-MADR-reference-service-lifecycles.md](0011-MADR-reference-service-lifecycles.md)
  §9 and Amendment A3: the handoff and its environment file.
* [0032-MADR-task-scheduler-registration-and-one-shot-handoff.md](0032-MADR-task-scheduler-registration-and-one-shot-handoff.md)
  D6-A: the Task Scheduler handoff's file, which F6 and F7 cover.
* The consumer's record is in another repository: magic-cli-remote,
  `docs/decisions/0210-MADR-hand-off-agent-run-updates-outside-the-service.md`,
  amendment A, F5.
* Sources:
  * systemd.exec(5) and systemd-run(1), systemd 259, as installed on the
    Linux test host; also
    <https://www.freedesktop.org/software/systemd/man/latest/systemd.exec.html>.
  * sd_notify(3): <https://www.freedesktop.org/software/systemd/man/latest/sd_notify.html>.
  * sd_listen_fds(3): <https://www.freedesktop.org/software/systemd/man/latest/sd_listen_fds.html>.
  * launchd.plist(5) and launchd(8), as installed on macOS.
