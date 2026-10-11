---
status: proposed
date: 2026-10-11
associated-madr: "0022-MADR-documentation-pass-after-v1-14-0.md"
---
# Implement the error-chain tests past interfaces and a process check that sees an exit

Associated MADR: [0022-MADR-documentation-pass-after-v1-14-0.md](0022-MADR-documentation-pass-after-v1-14-0.md),
Amendment A2. The first plan for that record is
[0022-PLAN-documentation-pass-after-v1-14-0.md](0022-PLAN-documentation-pass-after-v1-14-0.md),
complete.

## Goal

* **`errors.Is(err, selfupdate.ErrBatchFile)` is held where a program
  reads the error,** at every exported entry point, on every OS (MADR
  D5-A).
* **A process that has exited reads as gone,** whoever still holds a
  handle to it: one check, `winproc.Alive`, in the Task Scheduler
  backend, the handoff's hop wait and the live tests (D6-A).
* **`v1.15.1`** carries the backend fix.

## Scope

### In scope

| Phase | What | Commit |
| :--- | :--- | :--- |
| Q0 | this plan, Amendment A2 and the index row | records only |
| Q1 | measurements on the Windows test host | records only |
| Q2 | `winproc.Alive`, its callers, its tests | code and tests |
| Q3 | the behavioural chain tests, and `TestBatchErrorChain`'s doc | tests |
| Q4 | the architecture page and the migration guide's `v1.15.1` part | docs |
| Q5 | `v1.15.1`: CI, the tag, the pins | per 0017-PLAN's procedure |
| Q6 | closing | records only |

### Out of scope

* **A type-aware or call-graph walk** (D5-B, D5-C).
* **`scm`'s `created`,** which feeds the ancestor walk only
  (`winproc.Descends`), never a liveness decision.
* **Exported API.** `winproc` is internal; no exported name changes, so
  `apicheck` stays compatible with nothing added.
* **Push and tags,** which happen only on the owner's ask in the same turn.

## Rules

* **`main`, no branch.** The owner commits and pushes on `main`. An agent
  commits only on an explicit "commit to main" in that turn, with
  `git commit --no-edit`.
* **Each phase stops at a deviation** (AGENTS.md, Plan deviations), and
  Q1 at the stop condition below.
* **Checks before each commit:** `make pre-add-check FILES="…"` on every
  changed Go file; `scripts/check-docs.sh --links` and `--ids`, and
  markdownlint, on every changed Markdown file; `make gate` ending
  `overall=0`. A new test file changes `docs/architecture.md`'s count in
  the same commit, or the `arch` step fails.
* **Seen failing first:** every new test or assertion fails on a
  `scripts/plant-copy.sh` copy with one planted break, never by dirtying
  the tree. The record quotes the failing line.
* **The Windows test host:** binaries cross-compiled here with
  `CGO_ENABLED=0 GOOS=windows GOARCH=amd64`, copied to a new directory
  under its temporary directory, run from its ssh shell, which is
  elevated (0021-PLAN Deviation D1), with the console sign-in measured for
  Task Scheduler runs, and cleaned up afterwards.
* **Identifiers:** nothing committed names a host, an account or a real
  absolute path.

## Implementation Steps

### Phase Q0: records

1. `HEAD` is `78dde39` or a records-only descendant, on `main`.
2. `docs/README.md` gains this plan's row after
   `0022-PLAN-documentation-pass-after-v1-14-0.md`'s.
3. On approval: this plan becomes `in-progress` with `### Approval`
   quoting the owner; Amendment A2's heading loses "proposed", and it
   records the owner's words.
4. Checks; commit.

### Phase Q1: measurements on the Windows test host

Nothing in the tree changes; the probes live in the scratchpad.

1. **An exited process with a handle held.** A probe, `aliveprobe.exe`
   (standard library and `golang.org/x/sys/windows`), starts a child
   that sleeps 2 s and exits, and keeps a handle to it. Before and after
   the exit, it records, for the child's PID:
   * `OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION)`: opens or not;
   * `GetProcessTimes`: the creation time, equal to the first reading or
     not (today's `running(pid, born)`);
   * whether the Toolhelp snapshot lists the PID (today's
     `running(pid, 0)`);
   * `OpenProcess(SYNCHRONIZE | PROCESS_QUERY_LIMITED_INFORMATION)` and
     `WaitForSingleObject(h, 0)`: `WAIT_TIMEOUT` or `WAIT_OBJECT_0`;
   * `GetExitCodeProcess`.

   Then it closes its handle and records the same once more.
2. **Task Scheduler's engine.** From the ssh shell: register a throwaway
   `selfupdate-probe-<random>` task (`InteractiveToken`,
   `LeastPrivilege`, one `Exec` action that sleeps), run it, record its
   instance PID and creation time, end it with `schtasks /end`, and
   sample every 50 ms for 10 s: the creation-time check, the snapshot,
   and the wait. The record gives, for each check, how long after the end
   it still read "running".
3. **Stop condition.** If a process that exited signals no
   `WAIT_OBJECT_0`, or the `SYNCHRONIZE` open of the task's program is
   refused, D6-A as decided does not hold: stop, and put the measurement
   to the owner.
4. The MADR's two [unverified] facts gain their answers; the outputs go
   in this plan's record verbatim, redacted per Rules. Host cleanup is
   recorded.

### Phase Q2: `winproc.Alive` (D6-A)

1. `internal/winproc/alive_windows.go`, or `winproc_windows.go` if it
   fits the file's purpose, gains:

   ```go
   // Alive reports whether process pid runs, and, when born is not 0,
   // is the one created at born, not a later process with a reused ID.
   // A process that has exited is not alive, though another process
   // still holds a handle to it (0022-MADR Amendment A2).
   func Alive(pid uint32, born uint64) bool
   ```

   It opens with `SYNCHRONIZE | PROCESS_QUERY_LIMITED_INFORMATION`. No
   such process: false. `ERROR_ACCESS_DENIED`: the Toolhelp snapshot
   decides, as `running(pid, 0)` does today. `born` compared by
   `GetProcessTimes`. Alive only while `WaitForSingleObject(h, 0)` returns
   `WAIT_TIMEOUT`.
2. Callers:
   * `taskscheduler`'s `sysSched.running(pid, born)` returns
     `winproc.Alive(pid, born)`; its snapshot branch moves into `Alive`'s
     fallback;
   * `service.processRunning` calls `winproc.Alive(uint32(pid), 0)`;
   * the live tests' `pidRunning` is removed, and its log line uses
     `s.s.running`.
3. **Tests,** in `internal/winproc`'s Windows test file:
   * a child that runs: alive;
   * the same child after it exits, while the test holds a handle to it:
     not alive;
   * a running child checked with a `born` one tick off: not alive;
   * a PID with no process: not alive.
4. **Seen failing,** on the Windows test host:
   * `Alive` without the wait (today's open-and-compare): the
     exited-with-handle case fails;
   * `Alive` ignoring `born`: the reused-ID case fails.
5. The `taskscheduler` live tests, and `TestCOMLayout`, pass `-count=3`;
   `service`'s hop tests pass; `internal/winproc` passes.
6. Checks; commit.

### Phase Q3: the chain past interfaces (D5-A)

1. A case for each exported entry point, each with a fake at the
   interface that returns `fmt.Errorf("fake: %w", winproc.ErrBatchFile)`
   and records that it was called. Each asserts the fake ran and
   `errors.Is(err, selfupdate.ErrBatchFile)` on what the program
   receives:

   | Entry point | Fake | Package of the test |
   | :--- | :--- | :--- |
   | `Updater.Run`, staged probe | `Config.Probes` | `selfupdate` |
   | `Updater.Run`, manifest verifier | `Config.ManifestVerifiers` | `selfupdate` |
   | `Install`, post-install probe | `InstallOptions.PostInstall` | `selfupdate` |
   | managed install, `ExecReconciler` | `service.ExecOptions.Runner` | `selfupdate/service` |
   | `ghattest` verifier | `Options.Runner` | `selfupdate/verify/ghattest` |
   | `codesign` checker and signer | `CheckOptions.Runner`, `SignOptions.Runner` | `selfupdate/codesign` |
   | `HandOffIfInside` | a `service.Detacher` | `selfupdate/service` |
   | `cli.Run`'s error, the handoff failing | `HandOff.Detach` | `selfupdate/cli` |

   Each package's existing test helpers and fixtures are reused; where a
   case needs a release to reach the step, the fixtures that package's
   tests already use serve.
2. `TestBatchErrorChain`'s doc says where it stops, and names the test
   that holds the chain past it.
3. **Seen failing,** on this Mac, each a `%w` planted to `%v` at one layer
   above the fake: `Updater.runProbes`, `probeInstalled`, `ghattest`'s
   `running gh`, `codesign`'s `run`, `ExecReconciler.Reconcile`, and
   `cli.Run`'s join of the handoff's error (`run.go:143-144`). Each fails its own case and no other.
4. Checks; commit.

### Phase Q4: documentation

1. `docs/architecture.md`: the `internal/winproc` bullet names `Alive`.
2. The migration guide's §15 gains `### From v1.15.0 to v1.15.1`, in the
   form of the earlier patch parts: what `Stop` and `Start` read now, and
   that no API changes.
3. Checks; commit.

### Phase Q5: `v1.15.1`

0017-PLAN's "Release procedure" and 0022's first plan's P7:

1. CI on `main` at Q4's commit is green; the commits are pushed first, on
   the owner's ask.
2. The tag, on the owner's ask in that turn.
3. The tag's CI and its identity legs, `v1.15.1 (release) <12-hex>`.
4. The pin commit: `git grep -n 'v1\.15\.0\|ac4ef55' -- README.md
   docs/architecture.md docs/guides`; each changed line named; lines that
   state what `v1.15.0` itself changed are kept.
5. The proxy serves `v1.15.1`, and `@latest` resolves to it.
6. Release notes in this plan.

### Phase Q6: closing

1. `### Verification, at closing`, V1 to V5 with their evidence.
2. `### Closing`, `status: complete`, the index row. Commit, then push on
   the owner's ask.

## Verification

* **V1 (measured).** Q1's outputs are recorded, and the MADR's two
  [unverified] facts are answered.
* **V2 (liveness).** `Alive`'s tests failed on both plants and pass on
  the Windows test host and in CI's Windows job. The live tests pass
  `-count=3`.
* **V3 (chain).** Every case in Q3's table passes on every OS, and each
  failed on its planted `%v`, alone.
* **V4 (no API change).** `make apicheck` against `v1.15.0` reports
  nothing added or changed.
* **V5 (release).** `v1.15.1` is tagged, its CI is green, the pins name
  it, and the proxy serves it.

## Rollout and Rollback

* **Rollout:** each phase is one commit on `main`, pushed on the owner's
  ask. The backend fix reaches programs with `v1.15.1`.
* **Rollback, before the tag:** `git revert` of Q2 restores today's
  check; Q3 is tests only.
* **Rollback, after the tag:** a release cannot be withdrawn; a defect is
  fixed forward in `v1.15.2`.
* **A deviation in any phase** stops it.

## Execution record

*(Empty until approval.)*
