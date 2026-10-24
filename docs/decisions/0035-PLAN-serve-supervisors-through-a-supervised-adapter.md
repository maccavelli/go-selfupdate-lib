---
status: proposed
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

Pending: the owner approves this plan to execute it.
