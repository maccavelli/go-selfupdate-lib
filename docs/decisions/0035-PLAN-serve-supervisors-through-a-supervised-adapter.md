---
status: in-progress
date: 2026-10-24
associated-madr: "0035-MADR-serve-supervisors-through-a-supervised-adapter.md"
---
# Implement `service.Supervised`, `internal/procfs` and `ErrNoSupervisor`, and release `v1.22.0`

Associated MADR: [0035-MADR-serve-supervisors-through-a-supervised-adapter.md](0035-MADR-serve-supervisors-through-a-supervised-adapter.md)
(D0-C; D1-A, D2-A, D3-A, D4-C, D5-A to D12-A, D13-C and D14-A, as the
owner chose on 2026-10-24).

## Goal

* A program on any supervisor gets the backend contract from one adapter:
  * `service.NewSupervised`, fed with its own `State`, `Up` and `Down`;
  * a `Lifecycle`, `EnabledLifecycle`, `Reconciler` and `Detacher`, with
    `Status`;
  * hardened as the MADR's "Hardening, together" says.
* `internal/procfs` reads a Linux process's parent, start time and
  binary for it.
* `service.ErrNoSupervisor` reports a service that nothing supervises.
* The facts the code relies on are re-read and probed first (M1). Every
  new test is seen failing first. A live test drives the adapter over a
  real s6 tree. It ships as `v1.22.0`.
* Issue #2 is closed, and magic-cli-remote#27 is told the release, on
  the owner's ask.

## Scope

### In scope

| Phase | What | Commit |
| :--- | :--- | :--- |
| M0 | this plan, its index row | records only |
| M1 | the manuals re-read; probes on the WSL and in an Alpine container; the MADR amended with the results | records only |
| M2 | `internal/procfs`, its rule, AGENTS.md and the architecture table | code, tests, config |
| M3 | `ErrNoSupervisor`; `SupervisedState`, `SupervisedOptions`, `NewSupervised` and its checks; `Status`, `Installed`, `Running`, `Enabled` and `Inside` | code and tests |
| M4 | `Stop`, with `Kill`, and `Start` | code and tests |
| M5 | `WaitHealthy`, `Reconcile`, `Restore` and `Detach` | code and tests |
| M6 | the live s6 test, and its CI step | tests, CI |
| M7 | the documents | docs |
| M8 | `v1.22.0` | per the architecture page's "Releasing this module" |
| M9 | closing | records, issue comments |

### Out of scope

* Any first-party supervisor backend (MADR D0-C), and any parsing of
  runit's or OpenRC's output in the library.
* Other Unix than Linux (D12-A).
* magic-cli-remote's adoption, which is magic-cli-remote#27 and its own
  records.
* Moving launchd's ancestor walk onto `internal/procfs`. It runs on
  macOS, where `/proc` does not exist.

## Rules

As 0034-PLAN's:
* **Commits** go on `main` only on the owner's "commit to main" in that
  turn, with `git commit --no-edit`. A push or a tag needs its own word.
* **Deviations.** Each phase stops at a deviation. It is recorded here
  and, when a decision or a fact changes, in the MADR.
* **Checks:** `make pre-add-check` on changed Go files; cross-target
  `go vet`; the docs checks on changed Markdown; the architecture table
  when a `.go` file is added; `make apicheck` compatible with
  `v1.21.0`; and `make gate` ending `overall=0` before each push.
* **Seen failing.** Every new test is seen failing on a
  `scripts/plant-copy.sh` copy. A Linux-only test's plant runs on a
  Linux host.

Hosts:
* **The WSL on the Windows test host** is Ubuntu 24.04, its default
  distribution. It has `apt` and passwordless `sudo`.
  * M1 installs `runit` and `s6` there with `apt-get`. They stay
    installed for M6's runs, and are removed at M9 unless the owner
    keeps them.
  * The probes and the live test run as the WSL user, over scan
    directories under `/var/tmp`, each made and removed by its run.
  * Test binaries are cross-compiled on this Mac for the WSL's
    architecture, and copied over.
  * The host is not named in the tree.
* **OpenRC** is probed in a throwaway Alpine container, started with the
  Docker CLI on the same host, and removed after. If Docker Desktop
  cannot start there, this Mac's Podman machine is the fallback, on the
  owner's word.
* **The Linux test host** runs the `internal/procfs` and real-process
  tests. It has no supervisor installed.
* **Identifiers.** Probe output carries user and host names, which are
  replaced by `<user>` and `<host>` before anything enters the tree.

## Implementation Steps

### Phase M0: records

1. `docs/README.md` gains this plan's row after the 0035 MADR's.
2. Links and identifiers checked. Commit on the owner's word.

### Phase M1: re-read and probes (MADR Confirmation)

1. **Re-read.** Each [docs] fact the code or tests rely on is re-read
   from its page. Each re-read quote is recorded, and corrected in the
   MADR if it differs:
   * F12, F16, F17, F18 and F19;
   * F24's `rc_cgroup_cleanup` text.
2. **P1, `/proc` (the WSL, and the Linux test host).** For a running
   process:
   * field 22 of `/proc/<pid>/stat`, and its stability across reads;
   * that a command name holding spaces and parentheses still parses
     after the last `)`;
   * `/proc/<pid>/exe` while the binary is replaced by rename, which
     must still name the old file, shown as `(deleted)`, and the new
     path after a restart;
   * the error for a PID that has exited.
3. **P2, s6 (the WSL).** A scan directory under `/var/tmp`, and an
   `s6-svscan` run as the user. A service whose `run` execs a test
   program. Then:
   * `s6-svstat -o up,wantedup,normallyup,ready,pid,exitcode,updownfor`
     for up, down, a down file, an exiting service, and no supervisor
     (exit 1);
   * `s6-svc -d` and `-u`, and their exit codes;
   * whether a process the service started in a new session survives
     `s6-svc -d`;
   * `s6-svc -k`'s effect on a program that ignores TERM.
4. **P3, runit (the WSL).** The same with `runsvdir`, `sv status`,
   `sv down`, `sv up` and `sv kill`, including the exit code and text
   for a directory with no `runsv`. This settles F14's [unverified] for
   the consumer, and for runit's fixtures in magic-cli-remote#27.
5. **P4, OpenRC (an Alpine container).** OpenRC is installed and booted
   as the container's init.
   * A system service, and a user service run with `openrc --user`,
     each started with `start-stop-daemon`.
   * For each: the service process's `/proc/self/cgroup`.
   * With `rc_cgroup_cleanup=YES`: does a process the service started in
     a new session survive `rc-service stop`?
   * The `status` exit codes for started, stopped and crashed.
   * The OpenRC version, from `openrc --version`.
6. **Decision points.** The plan stops for the owner if:
   * P1 shows field 22 can change for a live process, which breaks D3;
   * P2 or P3 shows a detached process killed by the stop, which
     breaks D8's premise for that supervisor;
   * P4 shows OpenRC user services get no cgroup and cleanup kills
     nothing, which leaves `DetachCheck` without an OpenRC use. That is
     recorded, and is not a stop: D8-A stands for any supervisor.
7. **The amendment.** The MADR gains `## Amendment A1 (<date>): probe
   results`, a table of probe, question, result and effect. Each
   [unverified] is annotated in place. This plan becomes `in-progress`.
   Commit on the owner's word.

### Phase M2: `internal/procfs`

1. **`internal/procfs/procfs.go`:** the package documentation, citing
   the MADR's D13-C, and `ErrUnsupported` for other OSes.
2. **`procfs_linux.go`.** Each reads `/proc/<pid>/stat` or `exe`:
   * `Parent(pid int) (int, error)`, from the fields after the last `)`;
   * `StartTime(pid int) (uint64, error)`, field 22 in clock ticks;
   * `Exe(pid int) (string, error)`.

   *(Deviation D1: `Parent` and `StartTime` are one read,
   `ReadStat(pid int) (Stat, error)`, which returns `Parent`,
   `StartTime` and `State`, with `Stat.Zombie()`.)*

   A missing process is an error wrapping `fs.ErrNotExist`.
3. **`procfs_other.go`:** each function returns `ErrUnsupported`.
4. **Tests:**
   * `procfs_test.go`: the `stat` parser over fixed lines, with a
     command name holding spaces and parentheses, and with short and
     malformed lines;
   * `procfs_linux_test.go`: this process's parent equals
     `os.Getppid()`; its start time is stable and non-zero; `Exe` equals
     `os.Executable()`; a child's start time differs from its parent's;
     an exited PID is `fs.ErrNotExist`;
   * `procfs_other_test.go`: `ErrUnsupported`.
5. **`.golangci.yml`:**
   * a rule `internal-procfs` that allows the standard library only;
   * `internal/procfs` added to `other-packages`' exclusions;
   * `service`'s rule allows `internal/procfs`.
6. **AGENTS.md:** its internal helpers name `internal/procfs`.
   **`docs/architecture.md`:** a row for it, and the CI section unchanged
   until M6.
7. **Seen failing:** plants of (a) the parser splitting at the first
   `)`, (b) the wrong field for the start time, (c) `Exe` returning the
   link's target unresolved, and (d) a foreign import that `depguard`
   must refuse.
8. Checks, with the Linux tests on the Linux test host. Commit.

### Phase M3: the type, its checks and its reads

1. **`selfupdate/service/errors.go`:** `ErrNoSupervisor`, "the service
   exists, but nothing supervises it" (D10-A).
2. **`selfupdate/service/supervised.go`:**
   * `SupervisedState`, JSON-tagged as the MADR's D2-A lists, with
     `Instance` filled by the adapter;
   * `SupervisedOptions`: `Name`, `State`, `Up`, `Down`, `Kill`,
     `KillAfter`, `Poll`, `Probe` and `DetachCheck`;
   * `Supervised`, and `NewSupervised`.

   `NewSupervised` refuses off Linux with `ErrUnsupported`, through a
   `goos` seam for tests. It also refuses:
   * a missing `State`, `Up` or `Down`;
   * a `Poll` that `PollOptions.Validate` refuses;
   * a `KillAfter` set without `Kill`, or not shorter than `Poll.Timeout`.

   The default `KillAfter` is half of `Poll.Timeout` (D4-C).
3. **The reads:**
   * `Status(ctx)` is `State` with `Instance` set to `"<pid>@<ticks>"`
     (D3-A, D11-A);
   * `Installed` is `State.Installed`;
   * `Enabled` is `State.NormallyUp`;
   * `Running` is `State.Up`. Under `ErrNoSupervisor` it is false, unless
     the instance `Stop` recorded is alive (D10-A).

   Each wraps errors with its step and `Name`.
4. **`Inside`** (D7-A): it walks `procfs.Parent` from this process,
   bounded, to `State.PID`. It counts only when that PID's start time
   matches the instance read with it. An unreadable step is an error.
5. **`supervised_test.go`**, with fakes, on every OS through the `goos`
   seam, and with the `/proc` reads behind a seam:
   * each refusal of `NewSupervised`;
   * `Status`'s instance;
   * `Installed`, `Enabled` and `Running`, including `ErrNoSupervisor`;
   * `Inside` true by descent, false off the chain, false on a
     start-time mismatch, and an error on an unreadable step.
6. **`supervised_linux_test.go`:** `Inside` over real processes. The
   test binary starts a child, which starts a grandchild. The
   grandchild is inside for the child's PID and outside for an
   unrelated one.
7. **Seen failing:** plants of (e) `Inside` ignoring the start time,
   (f) an unreadable step read as outside, (g) `Running` ignoring
   `ErrNoSupervisor`, and (h) no `KillAfter` check.
8. Checks, `make apicheck` (additions only) and `go doc`. The
   architecture table gains `service`'s new files. Commit.

### Phase M4: `Stop` and `Start`

1. **`Stop`** (D4-C), in order:
   1. `State`: not installed is `ErrNotInstalled`.
   2. Inside: `ErrInsideService`. Nothing has been called before this
      point but `State`.
   3. Record the instance and `procfs.Exe`.
   4. `Down`.
   5. Wait on `State`, and on the recorded process being gone by PID
      and start time. The wait runs under `context.WithoutCancel` and
      `Poll.Timeout`, with `Kill` called once at `KillAfter`, and the
      caller's context error joined after (F6).
   6. On failure with the recorded process alive, call `Up`, and join
      its error.

   A service that is not up is recorded as nothing, and returns nil.
2. **`Start`** (D5-A): a current instance other than the recorded one is
   left alone. Otherwise `Up` runs. Errors are wrapped with the step.
3. **Tests**, with fakes on a virtual clock:
   * the order of calls;
   * the inside refusal, with no `Down`;
   * not installed;
   * a stop that completes;
   * a timeout that calls `Up` and returns `ErrTimeout`;
   * a caller's context ended mid-wait: the wait goes on, and the
     context's error is returned;
   * `Kill` at `KillAfter`, once;
   * `Kill`'s error reported only on failure;
   * a reused PID during the wait, which counts as gone;
   * `Start` leaving a current instance;
   * `Start` calling `Up`;
   * `Up`'s `ErrNoSupervisor` passed on.
4. **Seen failing:** plants of (i) no inside check, (j) the wait bound by
   the caller's context, (k) no `Up` after a failed stop, (l) no `Kill`,
   and (m) the PID alone compared during the wait.
5. Checks. Commit.

### Phase M5: `WaitHealthy`, `Reconcile`, `Restore` and `Detach`

1. **`WaitHealthy`** (D6-A): `PollHealthy`, with `Previous` set to the
   recorded instance, over a probe built from `State`:
   * an observation is `Ready` when it is `Up` and, where `Ready` is
     not nil, ready;
   * it is `Failed` on `State.Failed`;
   * it is `Failed` on an instance change after one has counted;
   * it is `Failed` on `Restarting` seen after up, or twice in a row.

   Then `Probe` runs, with `Settle` -1, as in the other backends.
2. **`Reconcile`** (D9-A) compares the recorded binary with the
   executable by `SameExecutable`. With nothing recorded, it returns
   `Changed: false` and a warning. A mismatch is an error naming
   `NewExecReconciler`. `Restore` returns nil.
3. **`Detach`** (D8-A): `DetachCheck`'s error is wrapped with
   `ErrUnsupported`. Then `DetachProcess`.
4. **Tests:**
   * fakes for:
     * a settle;
     * each failure;
     * readiness, both nil and false;
     * the application probe;
     * reconcile's three outcomes;
     * `DetachCheck`'s refusal, with no process started;
   * `supervised_linux_test.go`:
     * `HandOffIfInside` over a real child that the test marks as the
       supervised process;
     * the detached run outliving its parent, through the existing
       `envfile` fake mode (0033-PLAN K2).
5. **Seen failing:** plants of (n) no restart detection, (o) `Restarting`
   ignored, (p) `Ready` ignored, (q) no `DetachCheck` call, and (r)
   reconcile ignoring a mismatch.
6. Checks. Commit.

### Phase M6: the live s6 test

1. **`selfupdate/service/supervised_live_linux_test.go`**, behind
   `SELFUPDATE_REQUIRE_SUPERVISED=1`:
   * the test makes a scan directory under its temporary directory,
     starts `s6-svscan` there as the current user, and writes a service
     whose `run` execs a copy of the test binary in a "service" mode;
   * a test-only `State` reads `s6-svstat -o` through `ExecRunner`;
   * `Up` and `Down` are `s6-svc -u` and `-d`, and `Kill` is `-k`, at
     the absolute paths of the s6 tools.
   * The test then:
     1. stops, replaces the binary, starts and waits healthy;
     2. checks that a crash loop fails the health wait with
        `ErrUnhealthy`;
     3. runs an update from a process the service started, which must
        hand off, finish, and leave the service running the new build;
     4. tears down the tree and the directory.
2. **Runs:** on the WSL, with the binary cross-compiled.
3. **CI:** the Ubuntu job installs `s6` with `apt-get` and runs the test
   with the switch. `docs/architecture.md`'s CI section names the
   switch.
4. **Seen failing:** on the WSL, plants (j) and (n) fail the live test.
5. Checks, `actionlint` on the workflow. Commit. CI runs it on the push.

### Phase M7: the documents

1. **`selfupdate/service/doc.go`:** the adapter beside the four backends.
2. **`docs/guides/extending-selfupdate.md`:**
   * the line "A service manager with no backend here" becomes a section
     on `NewSupervised`, with a worked s6 example;
   * the section says what the program supplies and what the library
     guarantees;
   * it covers `Kill`, `DetachCheck` with the OpenRC case, and
     `ErrNoSupervisor`.
3. **The migration guide:** `## 22. From v1.21 to v1.22`.
4. **`CHANGELOG.md`:** `## Unreleased`.
5. **`docs/architecture.md`:** `service`'s paragraph, and the
   `internal/procfs` row from M2.
6. Checks. Commit.

### Phase M8: `v1.22.0`

As 0034-PLAN L6:
1. the pre-tag change and `make gate`;
2. the push and CI on `main`;
3. the tag check, the disclosure guard, and the tag on the owner's word;
4. the tag's CI, its identity legs printing `v1.22.0 (release) <12hex>`;
5. the pin commit;
6. the proxy.

### Phase M9: closing

1. `### Verification, at closing`, `status: complete`, the index row.
2. **On the owner's ask:**
   * close issue #2 with a comment naming `v1.22.0`;
   * comment on magic-cli-remote#27 that the adapter has shipped, naming
     the final API, `Kill`, and M1's probe results for its runit fixtures
     and its OpenRC `DetachCheck`.
3. Remove `runit` and `s6` from the WSL unless the owner keeps them.

## Verification

* **V1 (facts):** the re-reads and P1 to P4 are recorded in the MADR's
  Amendment A1.
* **V2 (procfs):** its tests pass on Linux and elsewhere, and plants (a)
  to (d) failed.
* **V3 (adapter):** the fake and real-process tests pass on every OS
  where they build, and plants (e) to (r) failed.
* **V4 (live):** the live s6 test passes on the WSL and in CI, on
  `main` and on the tag.
* **V5 (API):** `make apicheck` against `v1.21.0` reports additions only,
  and `go doc` shows them.
* **V6 (release):** `v1.22.0` is tagged, its CI is green, and the proxy
  serves it as `@latest`. #2 is closed and #27 is told, on the ask.

## Rollout and Rollback

* **Rollout:** one commit per phase on `main`, pushed on the owner's
  word, and `v1.22.0` at M8. A program adopts the adapter by requiring
  `v1.22.0`. Nothing changes for a program that does not call it.
* **Rollback:** before the tag, `git revert` of a phase. After it, fix
  forward. The adapter is additive, so no existing behaviour depends on
  it.

## Execution record

### Approval

The owner committed M0 and said "proceed" on 2026-10-24, which approves
this plan.

### M1: re-reads and probes (2026-10-24)

* **Re-reads:** by a fetch of each page; all matched (MADR Amendment A1).
* **P1:** run with `python3` on the Linux test host and in the WSL. The
  first version was a bash script. It hung on the Linux test host,
  whose `/bin/sleep` is a multi-call binary that refuses another name,
  and it left one directory and two `sleep` processes there, which were
  removed by PID. The owner then directed that probes be Python, as the
  global instructions say. Every probe since is Python.
* **P2 and P3:** `sudo apt-get install runit s6` in the WSL (exit 0).
  Ubuntu's `runit` package enabled `runit.service`, a root
  `runsvdir -P /etc/service` over an empty `/etc/service`, which the
  probes do not use.

  The first run stopped in its s6 teardown: `s6-svscan` did not exit on
  TERM while a TERM-ignoring service ran. It left six processes in the
  WSL, which were listed and ended by PID. The teardown was hardened:
  `-k` first, KILL after 5 s, every detached PID recorded, and a sweep
  of the user's processes under the probe directory. The second run
  completed and left nothing.
* **P4:** Docker Desktop was stopped. The owner granted starting,
  running and stopping it ("you have the permissions to start/run/stop
  docker desktop on this host"), and `docker desktop start` started it.
  * A registry pull failed in the Windows credential helper from the
    ssh logon ("A specified logon session does not exist"), with or
    without a probe-only Docker config. The image was built instead by
    `docker import` of Alpine's minirootfs 3.24.3, downloaded on this
    Mac and checked against the SHA-256 in Alpine's
    `latest-releases.yaml`.
  * The first container's PID 1 did not reap, so a stopped daemon
    stayed a zombie, and `rc-service stop` reported "1 process refused
    to stop". It was rerun with `--init`, and the user's runlevel
    directories made.
  * A third run added an `rc_sys=""` pass.
  * Each run removed its container and image. Docker Desktop was
    stopped again at the end, and the host has no probe container,
    image or file.
* **Results:** the MADR's Amendment A1, with the facts annotated in
  place. No decision point tripped:
  * P1 holds D3;
  * P2 and P3 show a detached run surviving each supervisor's stop;
  * P4 shows no OpenRC user-service cgroup, which the plan records
    rather than stops on. Cleanup on a real OpenRC host stays
    unmeasured.
* **Left in place for M6:** `runit` and `s6` in the WSL, with
  `runit.service`.

### Deviation D1 (2026-10-24): one `Stat` read in `internal/procfs`

* **Found.** M2 step 2 names `Parent` and `StartTime` as separate reads.
  M1's P4 showed that a process which has exited, but which nothing has
  reaped, keeps its `/proc` entry and its start time. So the adapter's
  stop wait, and `Running`'s "the recorded process is alive", must also
  read the process's state, and count a zombie as gone. Two separate
  reads could also come from two processes, if the PID were reused
  between them.
* **Decision (the owner, 2026-10-24, "One `Stat` read").**
  * `procfs.ReadStat(pid int) (Stat, error)` returns `Parent`,
    `StartTime` and `State`, from one read of `/proc/<pid>/stat`. A Go
    function cannot share the type's name `Stat`, so the owner's
    "`Stat` read" is spelled `ReadStat`.
  * `Stat.Zombie()` reports state `Z`.
  * `Exe(pid int)` stays.
  * `Parent` and `StartTime` are not separate functions.
* **Effect.** M2's tests and plant (b) target `Stat`. M3's `Inside` and
  M4's wait use `Stat`, and count a zombie as gone. No decision changes;
  the MADR's D13-C is annotated in place.

### M2: `internal/procfs` (2026-10-24)

* M1 was committed by the owner as `e00a413`. Deviation D1, above, was
  decided and recorded before any code.
* **`internal/procfs`:**
  * `procfs.go` holds the package documentation, `ErrUnsupported`, and
    `Stat` with `Parent`, `StartTime`, `State` and `Zombie()`. It also
    holds `parseStat`, which reads after the command name's last `)`
    and refuses a missing name, fewer than 20 fields, a state that is not
    one letter, and a parent or start time that is not a number.
  * `procfs_linux.go`: `ReadStat(pid)` and `Exe(pid)`. A PID that is not
    positive is an error, and a missing process wraps `fs.ErrNotExist`.
    `ReadStat`'s path carries a justified `//nolint:gosec` for G703,
    since the path is a checked positive integer under `/proc`.
  * `procfs_other.go`: both return `ErrUnsupported`.
* **Tests:**
  * `procfs_test.go`: `TestParseStat`, over command names `sleep`,
    `a b) (c`, `x)`, `(` and `) ) )`, a zombie and a running state; and
    `TestParseStatRefuses`, over six malformed lines.
  * `procfs_linux_test.go`: `TestReadStatSelf`, `TestExeSelf`,
    `TestReadStatChild` (a helper through `testproc.Start`),
    `TestReadStatZombieAndGone` (a child read as a zombie before it is
    reaped, then `fs.ErrNotExist` for both reads) and `TestInvalidPID`.
  * `procfs_other_test.go`: `TestUnsupported`.
  * `main_test.go`: the helper modes.
* **Config and documents:**
  * `.golangci.yml` adds a rule `internal-procfs` (the standard library
    only), excludes the package from `other-packages`, and lets
    `service`'s rule import it.
  * AGENTS.md names the package and the rule.
  * `docs/architecture.md` has the tree line, the table row (3 source, 4
    test files) and a paragraph. `--arch`: "19 rows, 0 differ".
* **Runs:** the tests pass here (the parser and `TestUnsupported`) and on
  the Linux test host (all seven Linux tests).
* **Seen failing** (`scripts/plant-copy.sh`; (a) to (c) run on the Linux
  test host):

  | Plant | Failure |
  | :--- | :--- |
  | (a) the parser splits at the first `)` | `procfs_test.go:24: command "a b) (c": … stat state "(c)" is not one letter`, and the same for `x)` and `) ) )` |
  | (b) field 21 read as the start time | `procfs_linux_test.go:27: this process: {Parent:… StartTime:0 …}; want … a start time`; `TestReadStatChild` and `TestParseStat` too |
  | (c) `Exe` reads `/proc/self/exe`, not the PID's | `procfs_linux_test.go:109: Exe of a reaped process: <nil>, want fs.ErrNotExist` |
  | (z) a zombie not recognised | `procfs_test.go:28: zombie: {Parent:41 StartTime:9876 State:90}, <nil>` |
  | (d) a foreign import | `procfs.go:17:2: import 'github.com/maccavelli/go-selfupdate-lib/internal/winproc' is not allowed from list 'internal-procfs' (depguard)` |

  The plan named (c) as "`Exe` returning the link's target
  unresolved". A link's target is what `Exe` returns by design, so the
  plant broke the PID it reads instead. (z) is added for `Zombie()`.

  The first try at (d) planted a blank, unsorted import. Lint refused
  it for format and `revive` reasons, not `depguard`. So it was
  replanted as a sorted, used import, which only `depguard` refused.
* **Checks:** `make pre-add-check` on the seven files reports "7 file(s)
  clean (gofmt, golangci-lint, go vet, go test, govulncheck)". The first
  run found the gosec G703 above. Cross-target `go vet` for `linux` and
  `windows` is clean, and `make apicheck` reports "compatible with
  v1.21.0".
* **Tooling.** The remote test runner and the plant driver are now
  Python scripts (`remoterun.py`, `plants_m2.py`) in place of the earlier
  bash runners, per the owner's direction in M1.

### M3: the type, its checks and its reads (2026-10-24)

* M2 was committed by the owner as `dbd4492`.
* **`errors.go`:** `ErrNoSupervisor`, "nothing supervises the service".
* **`supervised.go`:**
  * `SupervisedState`, JSON-tagged, documented field by field. `Ready`
    is a `*bool`, and its comment records P2: s6 reads `false` without a
    `notification-fd`.
  * `SupervisedOptions`, whose `State` contract is: with no supervisor,
    return an error wrapping `ErrNoSupervisor` and a state that still
    reports `Installed` and `NormallyUp`.
  * `Supervised`, and `NewSupervised` through `supervisedOn(o, goos)`.
    It refuses off Linux with `ErrUnsupported`, and refuses:
    * a missing `State`, `Up` or `Down`;
    * a `Poll` that `Validate` refuses;
    * a negative `KillAfter`, a `KillAfter` without `Kill`, and one not
      shorter than the timeout.

    With `Kill` set and no `KillAfter`, it uses half of the timeout.
  * `Status`, `Installed`, `Enabled`, `Running` and `Inside`.
    * `Instance` is `"<pid>@<start time>"`, through `procfs.ReadStat`. A
      process that is gone, or a zombie, gets none.
    * `Running` under `ErrNoSupervisor` is whether the instance a `Stop`
      recorded still runs, by PID and start time.
    * `Inside` walks the parent links, bounded at 4096. It counts the
      service's process only with the start time first read, and returns
      an error, never "outside", for an unreadable step or a loop.
  * The binary fields are left to M4, where `Stop` first uses them.
* **Tests:**
  * `supervised_test.go`, on every OS through `supervisedOn` and a fake
    process table: `TestNewSupervisedRefuses` (eight refusals, the
    default `KillAfter`, and `ErrUnsupported` off Linux),
    `TestSupervisedStatus`, `TestSupervisedReads` and
    `TestSupervisedInside`. The last covers six cases, a PID reused
    between the observation and the walk, an unreadable parent, a
    parent loop, and `State`'s error.
  * `supervised_linux_test.go`: `TestSupervisedInsideReal` (this
    process and its parent are inside, a child of its own is not) and
    `TestSupervisedStatusReal`.

    Step 6 named a child and a grandchild. The test uses this process,
    its parent, and a child, which is the same descent with one helper
    fewer.
* **Runs:** the `service` package passes here, on the Linux test host
  and on the Windows test host.
* **Seen failing** (`scripts/plant-copy.sh`, run here over the fakes):

  | Plant | Failure |
  | :--- | :--- |
  | (e) `Inside` compares the PID, not the start time | `supervised_test.go:218: a reused PID: true, <nil>; want false` |
  | (f) an unreadable step read as outside | `supervised_test.go:226: an unreadable parent: false, <nil>; want an error` |
  | (g) `Running` ignores `ErrNoSupervisor` | `supervised_test.go:159: Running with no supervisor: false, … nothing supervises the service`, and lines 164 and 168 |
  | (h) no `KillAfter`-under-timeout check | `supervised_test.go:88: KillAfter not under timeout: accepted` |

  The first try at (e), `return true, nil`, left a variable unused and
  did not build. It was replanted to compare PIDs.

  The plants ran before the rename below, which changes only the
  unexported constructor's name.
* **Checks:**
  * `make pre-add-check` on the four files reports "4 file(s) clean
    (gofmt, golangci-lint, go vet, go test, govulncheck)". The first run
    found revive's `confusing-naming`, `newSupervised` beside
    `NewSupervised`, so the unexported constructor is `supervisedOn`.
  * `make apicheck` reports "compatible with v1.21.0".
  * `docs/architecture.md`'s `service` row is 20 source and 20 test
    files, and names `internal/procfs`. `--arch`: "19 rows, 0 differ".

### M4: `Stop` and `Start` (2026-10-24)

* M3 was committed by the owner as `9e0681e`.
* **`supervised_lifecycle.go`** (new):
  * `Stop` follows step 1's order. It records the instance and the binary
    (`procfs.Exe`, best-effort, empty when unreadable) under `mu`. It
    then calls `Down`, and waits in `waitDown` under
    `context.WithoutCancel` and `Poll.Timeout`, joining the caller's
    context error after. `Kill` runs once at `KillAfter`.
  * A failed `Down` or wait joins `Kill`'s error, and calls `Up` when
    the recorded process still runs.
  * A service that is down and not wanted up is left alone. With no
    supervisor, `Stop` returns nil when nothing runs, and an error
    wrapping `ErrNoSupervisor` when the recorded process does.
  * `Start` leaves a running instance other than the recorded one, and
    otherwise calls `Up`.
  * `sleepCtx` is the real sleep.
* **`supervised.go`:** the `exe`, `now` and `sleep` seams, and the
  `binary` field.
* **`supervised_lifecycle_test.go`** (new): a fake supervisor whose
  process exits a set number of clock ticks after `Down`, or never, on a
  virtual clock. The tests:
  * `TestSupervisedStop`;
  * `TestSupervisedStopRefusesFirst`: inside, not installed, and down
    and not wanted, each with no call;
  * `TestSupervisedStopTimeout`: `down`, then `up`, after the 4 s bound;
  * `TestSupervisedStopOutlivesContext`;
  * `TestSupervisedStopKill`: killed at 1 s;
  * `TestSupervisedStopKillError`;
  * `TestSupervisedStopReusedPID`;
  * `TestSupervisedStopDownFails`;
  * `TestSupervisedStopNoSupervisor`;
  * `TestSupervisedStart`: left alone, a new process after a stop, the
    recorded instance, and `Up`'s `ErrNoSupervisor`.
* **Runs:** the `service` package passes here, on the Linux test host
  and on the Windows test host.
* **Seen failing** (`scripts/plant-copy.sh`, run here):

  | Plant | Failure |
  | :--- | :--- |
  | (i) no inside check | `supervised_lifecycle_test.go:214: inside: <nil>, want ErrInsideService` |
  | (j) the wait bound by the caller's context | `supervised_lifecycle_test.go:274: Stop: selfupdate: service: timed out: demo: context canceled` |
  | (k) no `Up` after a failed stop | `supervised_lifecycle_test.go:248: calls ["down"], want down then up`; `:342` the same for a refused `Down` |
  | (l) no `Kill` at `KillAfter` | `supervised_lifecycle_test.go:289: … timed out: demo still runs 4s after it was stopped (last: up=true pid=100 )`; `:314` |
  | (m) the PID alone compared | `supervised_lifecycle_test.go:326: Stop with the PID reused: … timed out …`; `supervised_test.go:168: Running with no supervisor, PID reused: true, <nil>` |

  The plants were run again after the gocritic change below, and each
  failed the same tests.
* **Checks:**
  * `make pre-add-check` on the three files reports "3 file(s) clean
    (gofmt, golangci-lint, go vet, go test, govulncheck)". Its first run
    found gocritic's `ptrToRefParam` on `waitDown`'s `*error` parameter,
    so `waitDown` now returns `Kill`'s error as a result.
  * The timeout message drops its trailing space when the state has no
    `Detail`.
  * `make apicheck` reports "compatible with v1.21.0".
  * `docs/architecture.md`'s `service` row is 21 source and 21 test
    files.

### M5: `WaitHealthy`, `Reconcile`, `Restore` and `Detach` (2026-10-24)

* M4 was committed by the owner as `46b680a`.
* **`supervised_lifecycle.go`:**
  * `WaitHealthy` runs `PollHealthy` with `Previous` set to the recorded
    instance. Its probe counts an observation when the service is up as
    a new instance and, where `Ready` is set, ready. It reports `Failed`
    on four cases:
    * `State.Failed`;
    * an instance change after one counted;
    * `Restarting` after the service was up;
    * `Restarting` twice in a row.

    The application `Probe` then runs with `Settle` -1.
  * `describeState` builds the detail.
  * `Reconcile` compares the recorded binary, with any ` (deleted)` cut
    (Amendment A1, P1), by `SameExecutable`. With nothing recorded, it
    returns no change and a warning. A mismatch is an error naming
    `NewExecReconciler`.
  * `Restore` returns nil.
  * `Detach` runs `DetachCheck` first, wrapping its error with
    `ErrUnsupported`, then `DetachProcess`.
* **`supervised.go`:** compile-time assertions that `*Supervised` is a
  `selfupdate.Lifecycle`, `EnabledLifecycle`, `Reconciler` and
  `Detacher`.
* **Tests:**
  * `supervised_health_test.go` (new):
    * `TestSupervisedWaitHealthy`, over nine scripted cases;
    * `TestSupervisedWaitHealthyProbe`: the application probe passing,
      failing, and not run after a failed wait;
    * `TestSupervisedReconcile`: no record, `(deleted)`, a cleaned path,
      a mismatch, and `Restore`;
    * `TestSupervisedDetachCheck`: a refusing check comes before
      `DetachProcess`'s own refusal of a relative path, and with no
      check `DetachProcess`'s error comes through.

    The helper is named `observations`, since `poll_test.go` already
    has a `script`.
  * `supervised_linux_test.go`: `TestSupervisedHandOffReal`, in which
    `HandOffIfInside`, with this test process as the service's process,
    detaches a real run. That run writes its environment, with
    `SELFUPDATE_HANDOFF`, through the existing `envfile` mode.
* **Runs:** the `service` package passes here, on the Linux test host
  (with `TestSupervisedHandOffReal`) and on the Windows test host.
* **Seen failing** (`scripts/plant-copy.sh`, run here):

  | Plant | Failure |
  | :--- | :--- |
  | (n) no restart detection | `supervised_health_test.go:87: restarted during the wait: <nil>, want selfupdate: service: not healthy` |
  | (o) `Restarting` ignored | `:87: restarting after it was up: … timed out …`; `restarting twice in a row: <nil>`; `:109` the application probe case |
  | (p) `Ready` ignored | `:87: not ready, where the supervisor says: <nil>, want selfupdate: service: timed out` |
  | (q) no `DetachCheck` call | `:148: Detach with a refusing check: selfupdate: service: "demo" is not an absolute path, want ErrUnsupported and the check's error` |
  | (r) reconcile ignores a mismatch | `:129: another binary: <nil>, want an error naming NewExecReconciler` |

* **Checks:**
  * `make pre-add-check` on the four files reports "4 file(s) clean
    (gofmt, golangci-lint, go vet, go test, govulncheck)". The first run
    found errorlint's `==` on a sentinel in a test, which is now
    `errors.Is`.
  * `make apicheck` reports "compatible with v1.21.0". `go doc` shows
    `NewSupervised` and the twelve methods of `*Supervised`.
  * `docs/architecture.md`'s `service` row is 21 source and 22 test
    files.

### Deviation D2 (2026-10-24): `Detach` goes through the hop

* **Found before M6's live test.**
  * M5's `Supervised.Detach` called `DetachProcess` directly. On Unix,
    that starts the run in a new session, as a child of the calling
    process (`detach_unix.go`). In a handoff that is the service's
    process.
  * So the detached run descends from the service until the service
    exits, and the adapter's own `Stop`, called in that run, refuses it
    with `ErrInsideService` (D7-A). The handoff could never apply an
    update.
  * 0011-MADR Amendment A4 found the same flaw for the SCM handoff, and
    solved it with the two-hop detach.
  * M5's `TestSupervisedHandOffReal` did not catch it, because it checks
    only that the detached run starts.
* **Decision (the owner, 2026-10-24, "Two-hop detach").**
  * `Detach` adds `SELFUPDATE_HANDOFF_HOP=1` to the spec's environment
    and calls `DetachProcess`, as `scm.Service.Detach` does.
  * The hop starts the real run and exits, and the real run waits for
    the hop to exit (`HandOffHop`, `hopwait_unix.go`), after which its
    parent chain stops outside the service.
  * A program already calls `ReportFunc` or `LoadHandOffEnv` first in
    `main`, which completes the hop.
* **Files added to M6's scope:**
  * `supervised_lifecycle.go` (`Detach`, and its documentation);
  * `supervised_health_test.go` (a fake test that the hop is
    requested);
  * the MADR, whose D8-A is annotated in place.

  M6's live test proves a real handoff completes. Its plant, (s), drops
  the hop.

### Deviation D3 (2026-10-24): a crash loop is an exit after up, or a restart that lasts too long

* **Found in M6's first live run on the WSL.**
  * The update cycle passed, and the crash loop failed with
    `ErrUnhealthy`.
  * Then `WaitHealthy`, called just after the crash flag was removed,
    failed at once: "s6 live keeps exiting while its supervisor starts
    it again".
  * D6-A's "`Restarting` … twice in a row" counts observations, not
    exits. s6 waits a second before it starts a service that died again
    (F19), so one exit reads as about ten "down, want up" observations
    at a 100 ms poll.
  * One exit before the first up therefore failed the wait. That is
    stricter than the consumer's `settle`, which samples once a second
    and tolerates one exit (F30).
* **Decision (the owner, 2026-10-24, "Exit after up, or too long").**
  `WaitHealthy` fails with `ErrUnhealthy` on `Restarting` when:
  * it is seen after a new instance was seen up: an exit after up, as
    before;
  * or it has lasted, without a break, longer than the settle window
    (`DefaultPollSettle` when there is none), with no new instance ever
    up: a loop too fast to be seen up.

  One restart before the first up is tolerated.
* **Files in M6's scope:**
  * `supervised_lifecycle.go` (`WaitHealthy`, and its documentation);
  * `supervised_health_test.go`: the case "restarting twice in a row"
    becomes "restarting longer than the settle window", with a new case
    for several readings of one restart before up;
  * the MADR, whose D6-A is annotated in place.

  Plant (o) still applies.

### M6: the live s6 test (2026-10-24)

* M5 was committed by the owner as `99ceb2f`. Deviations D2 and D3,
  above, were decided at stops in this phase and recorded before their
  code.
* **`supervised_live_linux_test.go`** (new), behind
  `SELFUPDATE_REQUIRE_SUPERVISED=1`:
  * `s6Tool` finds the s6 tools in fixed directories, never on `PATH`.
  * `s6Supervised` is the adapter over a service directory, with a
    reference `State` over `s6-svstat -o up,wantedup,pid` (exit 1 is
    `ErrNoSupervisor`), and `Up`, `Down` and `Kill` over `s6-svc -u`,
    `-d` and `-k`, through `ExecRunner`.
  * The modes are `s6service` (the service, which exits 3 while a
    `crash` file exists, and hands off on a `trigger` file) and
    `s6update` (the detached run).
  * The test starts `s6-svscan` as the user over its own scan
    directory. It then runs:
    1. the update cycle: stop, replace, reconcile, start, health wait,
       with the new instance running the new build, read through
       `/proc/<pid>/exe`;
    2. a crash loop, which fails with `ErrUnhealthy`, then recovers;
    3. a handoff from inside the service. Its detached run is not
       inside, and it stops, replaces, reconciles, starts and checks the
       service, leaving the handed-off build running.

    The cleanup sends `-k` before `-dx` (Amendment A1, P2).
* **`supervised_live_other_test.go`** (new): the mode stub off Linux.
  **`main_test.go`:** `fakeTool` falls through to `supervisedLiveMode`.
* **`.github/workflows/ci.yml`:** the Linux job's "supervised adapter
  live test (s6)" step runs `apt-get update` and `install s6`, then the
  test with the switch. `actionlint`: clean.
  **`docs/architecture.md`:** the CI section names the switch, and the
  `service` row is 21 source and 24 test files.
* **Tooling correction.** I first wrote a cross-compiling driver,
  having checked the WSL for Go with a non-login `sh`, which does not
  see it. The owner corrected this: "the wsl absolutely has a go
  toolchain". Its Go 1.27.3 is on the login shell's `PATH`. The runs
  below are native `go test` in the WSL, through `remoterun.py wsl`
  (`bash -lc`).
* **Runs:**
  * The WSL's first live run passed the cycle and the crash loop, then
    failed "WaitHealthy once the crash ended: … keeps exiting …", which
    is Deviation D3.
  * After D3: `--- PASS: TestLiveSupervisedS6 (8.62s)`, and again with
    the whole package, `(8.68s)`.
  * The five `selfupdate/service/...` packages pass on the Linux test
    host, where the live test skips for want of s6, and `service` on the
    Windows test host.
  * After the runs the WSL had no `s6-svscan`, no `s6-supervise` and no
    test directory left.
* **Seen failing, live** (`scripts/plant-copy.sh`, the copy run in the
  WSL):

  | Plant | Failure |
  | :--- | :--- |
  | (s) `Detach` without the hop (D2) | `supervised_live_linux_test.go:356: handoff: result "error: the detached run is inside: true, <nil>", service "handed: true a detached run, started through the hop process 817"` |
  | (t) `Restarting` timed from its first observation (D3) | `supervised_live_linux_test.go:339: WaitHealthy once the crash ended: … keeps exiting …, for over 2s`; and in the fakes, `supervised_health_test.go:89: one restart before it comes up: …` and `several readings of one restart: …` |
  | (o) `Restarting` ignored | the live test passed, in 30.77 s: the crash loop's short-lived instances were caught by the restart rule instead, two different instances seen up. (o) is held by the fakes, where it failed in M5 and again after D3 |

  Plan step 4 named (j) and (n) for the live test. (j), the wait bound
  by the caller's context, cannot show there: the live test's context
  never ends. (n), restart detection, is not what catches the live crash
  loop, whose instances are rarely seen up. Both are held by the fakes.
  The live plants are the three that the live test can show: the hop,
  and the two crash-loop rules.
* **M5's plants after D3:** (n), (o), (p), (q) and (r) each still fail
  their fake tests.
* **Checks:**
  * `make pre-add-check` on eight files reports "8 file(s) clean
    (gofmt, golangci-lint, go vet, go test, govulncheck)".
  * The first run found gosec G703 at `sameexec.go:26`, a file outside
    this phase. The taint ran from the live test's `FAKE_DIR`, through
    `Reconcile`, into `SameExecutable`'s `os.Stat`. That `os.Stat` now
    carries a justified `//nolint:gosec`: it reads metadata only, and
    nothing is opened, read or written.
  * `make apicheck` reports "compatible with v1.21.0".
