---
status: in-progress
date: 2026-10-23
associated-madr: "0033-MADR-scrub-service-manager-environment-from-handoffs.md"
---
# Implement leaving the service manager's per-unit variables out of passed-on environments, and release `v1.20.0`

Associated MADR: [0033-MADR-scrub-service-manager-environment-from-handoffs.md](0033-MADR-scrub-service-manager-environment-from-handoffs.md)
(D1-C, D2-C, D3-A, D4-A, D5-A and D6-A, as the owner chose on
2026-10-23).

## Goal

* No environment the library passes on carries the service manager's
  per-unit variables (MADR D1-C, D2-C): not a handoff's run, the hop, nor
  the reconcile command.
* A caller who names one in `HandOff.Env` or `ExecOptions.Env` still
  passes it on.
* `service.ManagerEnvNames` and `service.ScrubManagerEnv` are exported.
* The unsettled facts are measured first (K1), every new test is seen
  failing first, and it ships as `v1.20.0`. Issue #7 is closed on the
  owner's ask.

## Scope

### In scope

| Phase | What | Commit |
| :--- | :--- | :--- |
| K0 | this pair, its index rows | records only |
| K1 | probes on the Linux test host, this Mac and the Windows test host; the MADR amended with the results | records only |
| K2 | `service`: the list, the filter, and its use in `DetachProcess`, the hop and `ExecReconciler` | code and tests |
| K3 | the systemd, launchd and Task Scheduler handoffs | code and tests |
| K4 | the live tests | tests |
| K5 | the documents | docs |
| K6 | `v1.20.0` | per the architecture page's "Releasing this module" |
| K7 | closing | records only |

### Out of scope

* Unsetting the variables in the caller's own process: that is the
  program's choice (sd_notify(3)'s `unset_environment`).
* `internal/testproc`'s and `verify/ghattest`'s environments: test
  support, and a `gh` command the caller runs in its own unit.
* The user and session variables (`PATH`, `HOME`, `XDG_RUNTIME_DIR` and
  the rest), and the persistent directory variables (MADR D2-C).

## Rules

As 0032-PLAN's: commits on `main` only on the owner's "commit to main"
in that turn, with `git commit --no-edit`; a push or a tag needs its own
word; each phase stops at a deviation, recorded here and, when a decision
or a fact changes, in the MADR; `make pre-add-check` on changed Go files,
cross-target `go vet`, the docs checks on changed Markdown, the
architecture table when a `.go` file is added, and `make apicheck`
compatible with `v1.19.0`; every new test seen failing on a
`scripts/plant-copy.sh` copy; `make gate` ends `overall=0` before each
push.

* **The Linux test host.** systemd 259, a user manager running. Probes
  run as the login user, in user scope; every unit the probes start is a
  transient one named `selfupdate-probe-<random>`, stopped and gone
  before the probe ends. The host is not named in the tree.
* **This Mac.** launchd's live tests run here with
  `SELFUPDATE_REQUIRE_LAUNCHD=1`, as they do today.
* **The Windows test host.** As 0032-PLAN's: a driver task in the
  console session for anything measured at medium integrity.

## Implementation Steps

### Phase K0: records

1. `docs/README.md` gains this pair's rows after 0032's.
2. Checks; commit on the owner's word. On approval, the MADR records the
   owner's choices and becomes `accepted`, and this plan `in-progress`,
   in K1's commit.

### Phase K1: probes (MADR Confirmation)

Each probe is a short shell or Go program in the scratchpad, run on its
host; its output is redacted (`<user>`, `<host>`, IDs shortened) before
anything enters the tree.

1. **K1-1, Linux.** `systemd-run --user --wait -p Type=notify
   -p NotifyAccess=all` running a program that sends `READY=1`, dumps
   its environment's names and values to a file, and exits. Record the
   names, and which of F10's are present.
2. **K1-2, Linux.** The same unit's program calls `systemd.Unit.Detach`
   (the library's code, from the working tree) for a one-off program that
   dumps its environment. Record, for the detached program:
   `INVOCATION_ID`, against the caller's and against its own unit's
   (`systemctl --user show -p InvocationID <transient>`);
   `JOURNAL_STREAM`; `NOTIFY_SOCKET`; `SYSTEMD_EXEC_PID`; and whether
   its process is in its own unit's cgroup. This settles F9's
   [unverified].
3. **K1-3, Linux.** A transient unit with `-p LoadCredential=probe:<file>`
   records `$CREDENTIALS_DIRECTORY` and exits; after the unit is gone,
   record whether the directory exists. Settles F11's [unverified].
4. **K1-4, macOS.** A throwaway LaunchAgent, loaded into `gui/<uid>`,
   dumps its environment and exits; diff its names against the loading
   shell's. Record every name launchd adds and whether its value names
   the job (such as `XPC_SERVICE_NAME`). Settles F13's [unverified].
5. **K1-5, Windows.** A probe task, as in 0032-PLAN J1, dumps its
   process's environment; diff against the driver's own. If the ssh
   shell can create a throwaway service, the same for a service process.
   Settles F14's [unverified].
6. **Decision points.** The plan stops for the owner if:
   * K1-2 shows the transient unit's `INVOCATION_ID` is its own despite
     the file (F9 wrong): the MADR's reasoning changes, though D1 may
     stand;
   * K1-3 shows the credentials directory outlives its unit: D2-C's
     reason for `CREDENTIALS_DIRECTORY` changes;
   * K1-4 or K1-5 find per-job or per-service variables: they join the
     list under D1-C. That is a recorded fact, not a stop, unless a value
     is something a run needs.
7. **The amendment.** The MADR gains `## Amendment A1 (<date>): probe
   results`, a table as 0032-MADR's: probe, question, result, effect;
   each [unverified] is annotated in place.

### Phase K2: `service`

1. **`selfupdate/service/managerenv.go`** (new):
   * `ManagerEnvNames() []string`, a fresh slice of MADR D1-C's and
     D2-C's names, in the order systemd.exec(5) lists them, each group
     with a comment citing it;
   * `ScrubManagerEnv(env []string) []string`: env without any entry
     whose name, compared exactly (Unix) or case-insensitively (Windows,
     where names are), is in the list; malformed entries are kept as
     they are;
   * an unexported `passOn(extra []string) []string`, returning
     `append(ScrubManagerEnv(os.Environ()), extra...)` (D5-A).
2. **Callers:** `DetachProcess` (`detach.go:30`) and `ExecReconciler`'s
   environment (`execreconciler.go:176`) use `passOn`; the hop
   (`handoffhop.go:55`) scrubs the environment it passes, keeping the hop
   variables' handling as it is. *(Deviation D1: the hop passes its
   environment on unchanged; it is already scrubbed.)*
3. **`managerenv_test.go`** (new):
   * `ManagerEnvNames` holds each name the MADR lists, and returns a
     copy;
   * `ScrubManagerEnv` drops each, keeps `PATH`, `HOME`,
     `XDG_RUNTIME_DIR`, `STATE_DIRECTORY` and a malformed entry, and on
     Windows matches case-insensitively;
   * `passOn` lets an explicit `NOTIFY_SOCKET` through;
   * `DetachProcess`, through a fake start function (the test seam
     `startDetached` already gives), passes no `NOTIFY_SOCKET` set in the
     test's environment, and passes one given in `HandOff.Env`;
   * `ExecReconciler`'s child, a fixture that prints its environment,
     has no `INVOCATION_ID` set in the test's, and has one given in
     `ExecOptions.Env`;
   * the hop's real run likewise. *(Deviation D1: the hop keeps an
     explicit variable.)*
4. **Seen failing:** plants of (a) one name dropped from the list, (b)
   `passOn` not scrubbing, (c) the explicit list scrubbed too (D5-B), (d)
   the hop not scrubbing.
5. `docs/architecture.md`'s `service` row. Checks; commit.

### Phase K3: the handoffs

1. **systemd** (`systemd/detach.go:209`), **launchd**
   (`launchd/detach.go:100`) and **Task Scheduler**
   (`taskscheduler/handoff.go:78`) build their file's environment with
   `service.ScrubManagerEnv(os.Environ())` and then `spec.Env`, and
   Task Scheduler's hop variable last.
2. **Tests** in each backend's fake tests: with `NOTIFY_SOCKET`,
   `INVOCATION_ID` and `RUNTIME_DIRECTORY` set by `t.Setenv`, the
   written file lacks them and keeps `STATE_DIRECTORY`; with
   `NOTIFY_SOCKET` in `spec.Env`, the file has it.
3. **Seen failing:** a plant of each backend passing `os.Environ()`
   unscrubbed.
4. Checks; commit.

### Phase K4: the live tests

1. **systemd** (`systemd/live_linux_test.go`, whose unit is
   `Type=notify`): the agent passes its own `INVOCATION_ID` as
   `CALLER_INVOCATION_ID` in `HandOff.Env`; the detached update's
   `runLiveUpdate` fails when its `INVOCATION_ID` equals it, or when
   `NOTIFY_SOCKET` is set. Run on the Linux test host with
   `SELFUPDATE_REQUIRE_SYSTEMD=1` and `SELFUPDATE_SYSTEMD_SCOPE=user`;
   CI's Linux job runs it as today.
2. **launchd** (`launchd/live_darwin_test.go`): for each per-job variable
   K1-4 found, the agent passes its value as `CALLER_<NAME>`, and the run
   fails when its own equals it. Run on this Mac.
3. **Task Scheduler:** for each per-task variable K1-5 found, the same in
   `TestLiveTaskHandOffOneShot`. None found means no change.
4. **Seen failing:** a plant of K3's systemd change reverted fails the
   systemd live test on the Linux test host.
5. Checks; commit.

### Phase K5: the documents

1. **`docs/guides/extending-selfupdate.md`:** the handoff section says
   which names the run does not inherit and why, how to pass one on, and
   `ScrubManagerEnv` for a program's own children; `ExecReconciler`'s
   section says the same for the reconcile command.
2. **The migration guide:** `## 20. From v1.19 to v1.20`.
3. **`CHANGELOG.md`:** `## Unreleased`.
4. **`docs/architecture.md`:** `service`'s paragraph.
5. Checks; commit.

### Phase K6: `v1.20.0`

As 0032-PLAN J9: the pre-tag change and `make gate`; the push and CI on
`main`; the tag check, the disclosure guard and the tag on the owner's
word; the tag's CI; the pin commit; the proxy. Issue #7 is closed with a
comment naming `v1.20.0`, on the owner's ask.

### Phase K7: closing

`### Verification, at closing`, `status: complete`, the index row.

## Verification

* **V1 (probes):** K1's five probes ran and the MADR's Amendment A1
  records them.
* **V2 (filter):** `managerenv_test.go` passes on every OS, and each plant
  failed.
* **V3 (process starts):** `DetachProcess`, the hop, `ExecReconciler` and
  the three handoffs pass on nothing on the list, and pass an explicit
  one on.
* **V4 (live):** the systemd handoff's run has its own unit's
  `INVOCATION_ID` and no `NOTIFY_SOCKET`, on the Linux test host and in
  CI; launchd's and Task Scheduler's, per K1.
* **V5 (API):** `make apicheck` against `v1.19.0` reports no
  incompatibility.
* **V6 (release):** `v1.20.0` tagged, its CI green, the proxy serving it
  as `@latest`; #7 closed.

## Rollout and Rollback

* **Rollout:** one commit per phase on `main`, pushed on the owner's
  word; `v1.20.0` at K6. A program gets the change by requiring
  `v1.20.0`; one that needs a dropped name in its run passes it in
  `HandOff.Env`.
* **Rollback:** before the tag, `git revert` of a phase; after it, fix
  forward.

## Execution record

### Approval

* 2026-10-23: the owner committed K0 (`73818da`) and said "committed
  proceed" after the pair was presented with D1-C to D6-A proposed. The
  MADR becomes `accepted` and this plan `in-progress`, in K1's commit.

### K1: probes (2026-10-23)

**Built, outside the tree.** `envprobe`, a Go program built for
linux/amd64 from the scratchpad, with this module `replace`d by the
working tree, so its handoff is the library's own `systemd.Unit.Detach`;
`run.sh`, which starts it as a transient `Type=notify` user unit with
`NotifyAccess=all`, waits for the handoff, stops the unit, and runs K1-3;
`k1mac.sh`, two throwaway LaunchAgents running `/usr/bin/env`;
`k1win.ps1`, two throwaway tasks and two throwaway services running
`cmd /c set`.

**Results.**

* **K1-1** (Linux): the unit's own invocation ID, from `systemd-run`, was
  the process's `INVOCATION_ID`. Of MADR F10's per-unit names it had
  `INVOCATION_ID`, `SYSTEMD_EXEC_PID`, `NOTIFY_SOCKET`,
  `MEMORY_PRESSURE_WATCH`, `MEMORY_PRESSURE_WRITE`, `JOURNAL_STREAM`,
  `MANAGERPID` and `MANAGERPIDFDID`.
* **K1-2** (Linux): `Detach` reported `transient unit
  <unit>-selfupdate-probe.service`. The detached run: `INVOCATION_ID ==
  caller's: True`, `== its own unit's: False`; `NOTIFY_SOCKET`,
  `JOURNAL_STREAM`, `SYSTEMD_EXEC_PID`, `MANAGERPID`, `MANAGERPIDFDID` and
  `MEMORY_PRESSURE_WATCH` each set and equal to the caller's; its unit's
  `NotifyAccess` `none`.
* **K1-3** (Linux): `CREDENTIALS_DIRECTORY` was
  `/run/user/<uid>/credentials/<unit>`, holding `probe`;
  `cred.after=gone`.
* **K1-4** (macOS): the two jobs' environments were `HOME LOGNAME
  OSLogRateLimit PATH SHELL SSH_AUTH_SOCK TMPDIR USER XPC_FLAGS
  XPC_SERVICE_NAME`, differing only in `XPC_SERVICE_NAME=<label-a>`
  against `<label-b>`.
* **K1-5** (Windows): tasks, 68 names, none differing, none naming the
  task; services, 39 names, none differing, none naming the service.

**Decision points.** None tripped: K1-2 confirmed F9, and K1-3 showed the
credentials directory gone. K1-4's two variables join the list (MADR
D1-C); K1-5 adds none.

**Left behind.** `systemctl --user list-units 'selfupdate-probe-*'`: 0;
`launchctl list` matching `selfupdate-probe`: 0; tasks and services named
`selfupdate-probe-*`: 0. The scratch directories on all three hosts were
removed.

**Recorded.** The MADR's Amendment A1, and F9, F11, F13 and F14 annotated
in place.

### K2: `service` (2026-10-23)

* K1 was committed by the owner as `04419f3`.
* **`selfupdate/service/managerenv.go`** (new): `managerEnvNames`, 39
  names in MADR D1-C's and D2-C's groups, each group citing its source;
  `ManagerEnvNames`, a copy; `ScrubManagerEnv`, case-insensitive on
  Windows, keeping malformed entries; `passOn`, the scrubbed environment
  then the explicit variables; and `goosWindows`, which `handoffenv.go`
  and `sameexec.go` now use too, since a third `"windows"` literal
  tripped `goconst`.
* **`detach.go`** (`DetachProcess`) and **`execreconciler.go`**
  (`ExecReconciler`'s environment) call `passOn`; `detach.go` loses its
  `os` import.
* **`main_test.go`:** an `envfile` fake mode, which writes a detached
  child's environment to `FAKE_OUT`.
* **`managerenv_test.go`** (new): `TestManagerEnvNames`,
  `TestScrubManagerEnv`, `TestPassOnExplicitWins`,
  `TestDetachProcessScrubs` (a real detached child),
  `TestExecReconcilerScrubs` (a recording runner) and
  `TestHopKeepsEnvironment`.
* **Here and on the Windows test host:** the six tests and the
  `selfupdate/service/...` packages pass, the last after the
  `goosWindows` change as before it.
* **Seen failing** (`scripts/plant-copy.sh`):

  | Plant | Failure |
  | :--- | :--- |
  | (a) `NOTIFY_SOCKET` dropped from the list | `managerenv_test.go:40: ManagerEnvNames() = [... no NOTIFY_SOCKET ...]`; `managerenv_test.go:55: ScrubManagerEnv = ["NOTIFY_SOCKET=caller" ...]` |
  | (b) `passOn` not scrubbing | `managerenv_test.go:90: NOTIFY_SOCKET = "/run/caller/notify", want the explicit value`; `managerenv_test.go:126: the detached run has NOTIFY_SOCKET="/run/caller/notify"`, and `INVOCATION_ID` |
  | (c) the explicit variables scrubbed too (MADR D5-B) | `managerenv_test.go:90: NOTIFY_SOCKET = "", want the explicit value`; `managerenv_test.go:130: the explicit LISTEN_FDS = "", want 3`; `TestExecReconcilerScrubs` failing |
  | (d) the hop scrubbing | `managerenv_test.go:187: the hop dropped an explicit NOTIFY_SOCKET: ["PATH=/usr/bin" "SELFUPDATE_HANDOFF_HOP_PARENT=42"]` |

  Plant (d)'s first anchor occurs twice in `handoffhop.go` and was
  refused by `plant-copy.sh`; the plant used the loop's preceding line
  as well.
* `docs/architecture.md`: the `service` row is 19 source and 18 test
  files. `make pre-add-check` on the seven Go files: "7 file(s) clean
  (gofmt, golangci-lint, go vet, go test, govulncheck)". `make apicheck`:
  "compatible with v1.19.0".

### Deviation D1 (2026-10-23): the hop does not scrub

* **Found.** K2 step 2 had the hop scrub the environment it passes. The
  hop's environment is already the product of a scrubbed pass: it comes
  from `DetachProcess` (`passOn`), or from a handoff file, which K3
  scrubs before the caller's explicit variables. Scrubbing again in the
  hop would drop a variable the caller put in `HandOff.Env`, which MADR
  D5-A passes on.
* **Done.** The hop passes its environment on as it is;
  `TestHopKeepsEnvironment` pins that, and failed on a plant of the
  scrub. No decision changes: MADR D3-A's "the hop" is covered by what
  starts it, and the MADR is annotated in place.

### K3: the handoffs (2026-10-24)

* K2 was committed by the owner as `7f82c67`.
* **Sources.** `systemd/detach.go` (the environment file's body),
  `launchd/detach.go` (`WriteHandOffEnv`'s list) and
  `taskscheduler/handoff.go` (the one-shot's list) now start from
  `service.ScrubManagerEnv(os.Environ())`, then `spec.Env`; Task
  Scheduler's hop variable stays last.
* **Tests**, one per backend, each with `NOTIFY_SOCKET`, `INVOCATION_ID`,
  `RUNTIME_DIRECTORY` and `STATE_DIRECTORY` set by `t.Setenv` and an
  explicit `NOTIFY_SOCKET=/run/explicit` in `spec.Env`:
  * `systemd/detach_unix_test.go` `TestDetachScrubsManagerEnv`, at
    systemd 239 so that the file stays to be read;
  * `launchd/detach_test.go` `TestDetachScrubsManagerEnv`, with
    `XPC_SERVICE_NAME` set as well;
  * `taskscheduler/handoff_test.go` `TestDetachOneShotScrubsManagerEnv`,
    which also checks the hop variable is the last entry.

  Each checks that the file keeps `STATE_DIRECTORY` and the explicit
  `NOTIFY_SOCKET`, and holds none of the caller's.
* **Here and on the Windows test host:** the three tests pass, and so do
  the five `selfupdate/service/...` packages on the Windows test host.
* **Seen failing** (`scripts/plant-copy.sh`, each backend passing
  `append(os.Environ(), spec.Env...)`):

  | Plant | Failure |
  | :--- | :--- |
  | (a) systemd | `detach_unix_test.go:154: environment file holds the caller's INVOCATION_ID=`, and `RUNTIME_DIRECTORY=` |
  | (b) launchd | `detach_test.go:166: the environment file holds the caller's XPC_SERVICE_NAME=com.example.demo`, and `NOTIFY_SOCKET`, `INVOCATION_ID`, `RUNTIME_DIRECTORY` |
  | (c) Task Scheduler | `handoff_test.go:292: the environment file holds the caller's NOTIFY_SOCKET=/run/caller/notify`, and `INVOCATION_ID`, `RUNTIME_DIRECTORY` |

  systemd's plant does not show the caller's `NOTIFY_SOCKET`: the
  environment file keeps a name's last value, and the explicit one comes
  last. The other two names catch the plant.
* **Checks.** `make pre-add-check` on the six Go files: "6 file(s) clean
  (gofmt, golangci-lint, go vet, go test, govulncheck)". `go vet` on
  `./selfupdate/service/...` for `linux`, `darwin` and `windows`: clean.
  `make apicheck`: "compatible with v1.19.0". No `.go` file was added, so
  the architecture table is unchanged.

### K4: the live tests (2026-10-24)

* K3 was committed by the owner as `e3744b3`.
* **systemd** (`systemd/live_linux_test.go`): `insideUpdate` passes the
  unit's `INVOCATION_ID` as `CALLER_INVOCATION_ID` in `HandOff.Env`;
  `runLiveUpdate` returns an error when `CALLER_INVOCATION_ID` is empty
  or equals the run's own `INVOCATION_ID`, or when `NOTIFY_SOCKET` is
  set, so `TestLiveHandOff`'s result check fails.
* **launchd** (`launchd/live_darwin_test.go`): the same with
  `CALLER_XPC_SERVICE_NAME`. K1-4 found one per-job variable:
  `XPC_SERVICE_NAME`. `XPC_FLAGS`, which joined the list because launchd
  sets it, had the same value in both probe jobs, so a run cannot tell
  its own from the caller's, and it gets no check.
* **Task Scheduler:** K1-5 found no per-task variable, so
  `TestLiveTaskHandOffOneShot` is unchanged.
* **The Linux test host** (`SELFUPDATE_REQUIRE_SYSTEMD=1`,
  `SELFUPDATE_SYSTEMD_SCOPE=user`, the tree copied to `/var/tmp` and
  removed after): all nine `TestLive` tests in `systemd` pass, among them
  `TestLiveHandOff` and `TestLiveHandOffHealthFailureReportsRollback`.
  CI's Linux job runs them in both scopes on the push.
* **This Mac** (`SELFUPDATE_REQUIRE_LAUNCHD=1`): `TestLiveHandOff`
  passes.
* **Seen failing**, live (`scripts/plant-copy.sh`; the systemd plants
  run on the Linux test host):

  | Plant | Failure |
  | :--- | :--- |
  | (a) K3's systemd change reverted | `live_linux_test.go:429: handoff result {... ExitCode:1 Error:the run's INVOCATION_ID "43ac2906…", the caller's "43ac2906…" ...}` |
  | (b) `NOTIFY_SOCKET` dropped from the list, so the `INVOCATION_ID` check passes | `live_linux_test.go:429: handoff result {... ExitCode:1 Error:the run has NOTIFY_SOCKET="/run/user/<uid>/systemd/notify" ...}` |
  | (c) K3's launchd change reverted | `live_darwin_test.go:556: handoff result {... ExitCode:1 Error:the run's XPC_SERVICE_NAME "io.github.maccavelli.selfupdate.livetest", the caller's "io.github.maccavelli.selfupdate.livetest" ...}` |

  Plant (a) is the one the plan names; (b) shows the `NOTIFY_SOCKET`
  check fails on its own, and (c) shows the same for the launchd check.
  After each run, the Linux test host had no `selfupdate` user units
  and no copy left in `/var/tmp`, and this Mac had no `selfupdate`
  launchd job.
* **Checks.** `make pre-add-check` on the two files: "2 file(s) clean
  (gofmt, golangci-lint, go vet, go test, govulncheck)". No `.go` file
  was added.

### K5: the documents (2026-10-24)

* K4 was committed by the owner as `1c3e0f2`.
* **`docs/guides/extending-selfupdate.md`:**
  * "Updating from inside the service" gains a bullet, "The run is not
    your service". It names the groups `ManagerEnvNames` lists, what a
    systemd and a launchd run had before (K1-2, K1-4), what still passes
    on (MADR D2-C), that `HandOff.Env` passes any name, and
    `ScrubManagerEnv` for a program's own children.
  * The `ExecReconciler` text in "Run as a service" says the same for the
    reconcile command and `ExecOptions.Env`, linking that section.
* **The migration guide:** `## 20. From v1.19 to v1.20`, in §19's shape.
  It does not say the journal attributes the run differently: journald
  takes `_SYSTEMD_INVOCATION_ID` from the process's unit, not from its
  environment. It names `INVOCATION_ID` and `JOURNAL_STREAM` as the
  values a program reads.
* **`CHANGELOG.md`:** `## Unreleased`, with `### Added` (the two
  functions) and `### Fixed` (the handoffs, `DetachProcess` and
  `ExecReconciler`), linking §20, this pair and issue #7.
* **`docs/architecture.md`:** `service`'s paragraph names `managerenv.go`,
  `ManagerEnvNames` and `ScrubManagerEnv`, citing the MADR.
* **Checks** on the four files: `npx --yes markdownlint-cli2@0.23.2`:
  "0 issues in 0 files"; `scripts/check-docs.sh --links`: "226 links in 4
  files, 0 broken"; `--ids`: "0 findings"; `--arch docs/architecture.md`:
  "18 rows, 0 differ".
