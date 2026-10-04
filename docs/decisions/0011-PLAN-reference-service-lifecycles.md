---
status: in-progress
date: 2026-10-04
associated-madr: "0011-MADR-reference-service-lifecycles.md"
---
# Implement `selfupdate/service`: reference systemd, launchd and Windows SCM lifecycles, the handoff and `sd_notify` (`v1.7.0`)

Associated MADR: [0011-MADR-reference-service-lifecycles.md](0011-MADR-reference-service-lifecycles.md)

## Goal

* `selfupdate/service`, `selfupdate/service/systemd`,
  `selfupdate/service/launchd` and `selfupdate/service/scm` exist as the
  MADR's Decision Outcome describes. Each backend implements
  `selfupdate.Lifecycle`, `selfupdate.EnabledLifecycle`,
  `selfupdate.Reconciler` and `service.Detacher`.
* An update started from inside the service it manages, such as by an
  agent the service spawned, hands off and completes: the service restarts
  on the new binary, and the handoff result says so (MADR §3, §9).
* `systemd.Notify` and `cli.Options.HandOff` exist (MADR §7, §9).
* Each backend and its handoff are proven against the real service
  manager in CI, and their logic against fakes on every OS.
* `v1.7.0` is tagged. `make apicheck` reports it compatible with `v1.6.0`.

## Scope

### In scope

| Phase | Work |
| :--- | :--- |
| V0 | Records: this pair, the owner's answers, the `docs/README.md` rows. No code. |
| V1 | `selfupdate/service`: the runner, typed errors, `PollHealthy`, `ExecReconciler`, the handoff core and `DetachProcess`; the four depguard rules |
| V2 | `selfupdate/service/systemd`, with its `Detacher` and `Notify` |
| V3 | `selfupdate/service/launchd`, with its `Detacher` |
| V4 | `selfupdate/service/scm`, with its `Detacher` |
| V5 | `cli.Options.HandOff`; docs; release notes; the release |

### Out of scope

* **Changing `Lifecycle`, `Reconciler` or `ManagedInstaller`,** including
  the order "replace before stop" (MADR, Related).
* **A D-Bus backend, Task Scheduler, `SMAppService`, Windows failure
  actions** (MADR, Handoff alternatives not chosen).
* **Installing or removing services.** The backends manage an existing
  definition. A product's `setup` command still writes it.
* **Moving magic-cli-remote and mcp-server-magictools** onto these
  packages, which fixes the owner's reported failure in practice. That is
  their own repositories' work. This PLAN's live tests reproduce the
  failure's shape (an update run from a process the service started) on
  each platform.
* **Push and tags,** which need the owner's ask.

## Rules for every phase

The v1.6.0 PLAN's rules apply
([0010-PLAN-v1-6-0-owner-contracts.md](0010-PLAN-v1-6-0-owner-contracts.md)):
tests first, seen to fail on a deliberately broken input; the full gate
before each commit (gofmt, `make lint` on three GOOS, vet, race, shuffle,
`make fuzz`, `make vuln`, `go mod tidy -diff`, the script tests, cross
vet); one commit per phase with `git commit --no-edit`; a dated deviation
entry, and a MADR amendment where a decision changes, before continuing
past any surprise.

In addition:

* `make apicheck` reports `compatible with v1.6.0`. The new packages and
  the `cli` field add API; nothing else changes.
* `go.mod` does not change: no module is added.
* Every live test is gated by an environment variable. It skips with its
  reason when the variable is unset, and fails when the variable is set
  and the test cannot run, as `SELFUPDATE_REQUIRE_PYTHON` and
  `SELFUPDATE_REQUIRE_ROOT` do.
* Every parse of tool output has a test with real output captured from a
  live run, kept under `testdata/`.
* The Windows test host runs V1, V4 and V5.

## Implementation Steps

### Phase V0: records

1. This MADR and PLAN, and their rows in `docs/README.md`.
2. The owner's answers to Q1–Q3 (2026-10-04) are in the MADR's §8, and the
   MADR is `accepted`.
3. The owner approves this PLAN before V1.

### Phase V1: `selfupdate/service`

1. **The runner.**
   * `type Runner interface { Run(ctx, Command) (Output, error) }`, with
     `Command{Path string; Args []string; Env []string; Stdin []byte}` and
     `Output{Stdout, Stderr []byte; ExitCode int}`.
   * The default runner uses `exec.CommandContext`, with no `PATH`
     lookup and a built environment. It caps stdout and stderr at 1 MiB
     each.
   * A non-zero exit is an `Output`, not an error. The backends decide
     what each exit code means.
2. **Typed errors:** `ErrUnsupported`, `ErrNotInstalled`, `ErrPermission`,
   `ErrInsideService`, `ErrTimeout`, `ErrUnhealthy`.
3. **`PollHealthy`,** as the MADR's §4 defines it, with a clock seam so
   tests do not sleep.
4. **`ExecReconciler`:**
   * `NewExecReconciler(ExecOptions{Reconcile, Restore []string;
     RestoreFunc func(ctx, product, Receipt) error; Runner Runner})`.
     *(Deviation D1, 2026-10-04: `RestoreFunc` replaces `RestoreWithOld
     bool`; MADR amendment A1.)*
   * It runs the new binary's reconcile arguments, and decodes the version
     1 receipt (MADR §6) from standard output. A missing `schema_version`
     reads as 1, and unknown fields are ignored.
   * A non-zero exit with a receipt is an error that keeps the receipt in
     `ReconcileResult.State`, so `Restore` can run. A non-zero exit with no
     receipt is an error with nothing to restore.
   * `Restore` calls `RestoreFunc` in-process when it is set, and
     otherwise passes the receipt on standard input to the new binary's
     restore arguments.
5. **The handoff core** (MADR §9):
   * `Detacher`, `HandOff`, `Detached`, `HandOffResult`;
   * `HandOffIfInside`;
   * `HandOffFunc` and `ReportFunc`, the plain functions `cli` takes;
   * `ReadHandOffResult`;
   * `WriteHandOffResult`, by temporary file and rename;
   * the environment contract: `SELFUPDATE_HANDOFF` and
     `SELFUPDATE_HANDOFF_RESULT`. `Inside` is false whenever
     `SELFUPDATE_HANDOFF` is set, so a detached run never hands off again.
6. **`DetachProcess`:**
   * On Windows, `CreateProcess` with `DETACHED_PROCESS |
     CREATE_NEW_PROCESS_GROUP | CREATE_BREAKAWAY_FROM_JOB`, retried without
     breakaway on `ERROR_ACCESS_DENIED`.
   * On Unix, a `setsid` child with its standard streams on the null
     device.
   * The child's PID is the `Detached.ID`.
7. **depguard:** the rules `service`, `service-systemd`,
   `service-launchd` and `service-scm`, each allowing exactly the imports
   in the MADR's §1 table, and each excluded from `other-packages`. Their
   names sort after `banned`.
8. **Tests:**
   * `PollHealthy`:
     * success after the settle window;
     * a changed instance restarts the settle window;
     * `Previous` never counts;
     * `Failed` stops the wait at once;
     * the timeout reports the last probe error;
     * the context ends the wait.
   * `ExecReconciler`, through a fake tool that re-runs the test binary
     (`SELFUPDATE_SERVICE_FAKE`):
     * each verdict;
     * the receipt kept on a failed child;
     * no receipt on a crash;
     * magic-cli-remote's receipt with no `schema_version`;
     * restore by the new binary, and in-process by `RestoreFunc`.
   * The handoff core:
     * `HandOffIfInside` with a fake `Detacher`, inside and outside;
     * no second handoff under `SELFUPDATE_HANDOFF`;
     * the result file round trip;
     * `ReportFunc` writes nothing outside a detached run.
   * `DetachProcess`, live on each OS:
     * Unix: the child survives its parent's process group being killed.
     * Windows: the child survives its parent's job being terminated,
       when the job allows breakaway and when it does not. The second
       case is pinned as found, and recorded.
   * depguard: a planted forbidden import in each package fails `make
     lint`, in a scratch copy.

### Phase V2: `selfupdate/service/systemd`

1. **`New(Options) (*Unit, error)`**, with `Options{Unit string; Scope
   Scope (System|User); Systemctl, SystemdRun string; Runner
   service.Runner; Probe service.HealthProbe; Poll service.PollOptions;
   RewritePath bool}`.
   * `Unit` is validated against systemd.unit(5)'s grammar: at most 255
     bytes, the allowed characters, a `.service` suffix, and a template
     instance only as `name@instance.service`.
   * `Escape` and `TemplateInstance` implement the documented escaping in
     Go, with no `systemd-escape` subprocess.
   * `Available()` reports whether `/run/systemd/system` exists
     (sd_booted(3)), and whether the tools exist.
   * In user scope, `New` fails if `XDG_RUNTIME_DIR` is unset.
2. **Probes:** one `show -p … -- <unit>` per call, parsed by key, and the
   MADR's §7 rules for `Installed`, `Running` and `Enabled`.
3. **`Stop` and `Start`:**
   * the `ErrInsideService` backstop first;
   * then a blocking `stop` or `start` with `--no-ask-password --no-pager
     --quiet`;
   * then a check of `ActiveState`, because the client's exit is not the
     job's result.
4. **`WaitHealthy`:**
   * `PollHealthy` over `show`, with the `InvocationID` read before the
     stop as `Previous`;
   * fail on `failed`, on `inactive`, or on an `NRestarts` increase;
   * then the optional application probe.
5. **`Reconcile` and `Restore`:**
   * The no-op path compares `ExecStart`'s program with the target. That
     is the first `path=` in `show -p ExecStart`, a format pinned by the
     live test.
   * The opt-in drop-in path follows the MADR's §5.
6. **`Inside` and `Detach`:**
   * `Inside`: the caller's cgroup in `/proc/self/cgroup` is the unit's
     `ControlGroup` or below it.
   * `Detach`: `systemd-run [--user] --unit=<unit>-selfupdate-<id>
     --collect --no-block --quiet --setenv=… -- <exe> <args>`.
     *(Deviation D2, 2026-10-04: the environment goes in a private file,
     `-p EnvironmentFile=`, not `--setenv`; MADR amendment A2.)*
   * `systemctl --version` is read first. Below 236 the result is
     `ErrUnsupported`.
7. **`Notify`:**
   * `Notify`, `Ready`, `Reloading`, `Stopping`, `Watchdog`, `Status`
     and `WatchdogInterval`, as the MADR's §7 specifies;
   * a `unixgram` socket, with the abstract `@` namespace.
8. **Tests:**
   * Table tests over a scripted runner:
     * every `ActiveState` and `UnitFileState` value;
     * `not-found`, and exit code 4;
     * the backstop;
     * a stalled stop;
     * the drop-in round trip, with `NeedDaemonReload`;
     * `Inside` on captured `/proc/self/cgroup` contents, for cgroup v1
       and v2;
     * the `systemd-run` arguments, and the version refusal.
   * `Notify` against a test `unixgram` listener, for a path socket and an
     abstract one, and with the variable unset.
   * Captured `show` output from the live run, under
     `testdata/systemd/`.
   * **Live, on Linux CI** (`SELFUPDATE_REQUIRE_SYSTEMD=1`, under
     `sudo`), with a throwaway `Type=notify` unit at a unique name, built
     from a test helper that calls `Ready`:
     * the full managed update through `ManagedInstaller`;
     * a failed health check, and its rollback;
     * **the handoff:** the helper, inside the unit, runs the update. It
       hands off; the unit restarts on the new binary; the result file
       reports success. Without the handoff, the backstop returns
       `ErrInsideService` and the unit is still running;
     * the unit becomes active only after `READY=1`;
     * removal of the unit afterwards.

     A user-scope variant runs under `systemctl --user` on the same
     runner. It is the shape of magic-cli-remote's relay.
9. **CI:** a Linux step that builds the live test binary as the runner,
   and runs it under `sudo env SELFUPDATE_REQUIRE_SYSTEMD=1`, following
   the ownership step's pattern.

### Phase V3: `selfupdate/service/launchd`

1. **`New(Options) (*Job, error)`**, with `Options{Label string; Domain
   Domain; Plist string; Launchctl, Plutil string; Runner; Probe; Poll;
   RewritePath bool}`.
   * `Domain` is `System`, `GUI(uid)` or `User(uid)`. There is no default
     from the plist's location.
   * `Label` is validated as a reverse-DNS label, with no `/` and no
     leading `-`.
2. **Probes:**
   * `Installed`: the plist exists.
   * `Running`: `list <label>` for the caller's own domain. Otherwise
     `print <domain>/<label>`, parsing only `pid =` and `state =`.
   * `Enabled`: `plutil -extract RunAtLoad raw` and `KeepAlive`, with a
     dictionary `KeepAlive` counting as true; and the label absent or
     enabled in `print-disabled <domain>`.
3. **`Stop`:**
   * the backstop;
   * `bootout <domain>/<label>`;
   * poll `print` every 50 ms until it exits 113, within the job's exit
     timeout plus 30 s;
   * confirm the old PID is gone, with `kill(pid, 0)`.
4. **`Start`:**
   * `enable`;
   * `bootstrap <domain> <plist>`, retrying 5 and 37 with backoff
     within the deadline;
   * `kickstart <domain>/<label>`.
5. **`WaitHealthy`:** `PollHealthy`, with the PID read before the stop as
   `Previous`, and a settle window of at least `ThrottleInterval`.
6. **`Reconcile` and `Restore`:** the no-op path compares `Program`, or
   `ProgramArguments.0`, with the target. The opt-in plist rewrite follows
   the MADR's §5, keeping the owner and mode.
7. **`Inside` and `Detach`:**
   * `Inside`: the caller's process group is the job's, or the job's PID
     is an ancestor of the caller.
   * `Detach`: the one-shot job of the MADR's §9. Its plist goes in a
     per-domain directory: for `system`, one owned by root and not group-
     or world-writable; for `gui` and `user`, one under the user's
     `Library`. It is bootstrapped there.
   * The next handoff, and `CleanupPending`, boot out finished jobs with
     the `.selfupdate.` label prefix and remove their plists.
     *(Deviation D3, 2026-10-04: `Job.CleanupHandOffs` does this, not
     `CleanupPending`; and the job's environment goes in a private file;
     MADR amendment A3.)*
8. **Tests:**
   * Scripted-runner tables:
     * exit codes 113, 119, 5 and 37;
     * a bootout still in progress, then gone;
     * a `KeepAlive` job;
     * the backstop;
     * the one-shot plist's keys;
     * the cleanup of finished jobs.
   * `print` and `list` output captured from the live run, under
     `testdata/launchd/`.
   * **Live, on macOS CI** (`SELFUPDATE_REQUIRE_LAUNCHD=1`), with a
     throwaway `KeepAlive` LaunchAgent in `gui/$(id -u)`:
     * the full managed update;
     * a failed health check, and its rollback;
     * **the handoff:** a process the job started runs the update. It
       hands off; the job restarts on the new binary; the result file
       reports success;
     * removal afterwards.

     It skips with its reason when `launchctl managername` is not `Aqua`.
     *(Deviation D4, 2026-10-04: with `SELFUPDATE_REQUIRE_LAUNCHD=1` it
     fails instead; without it, it skips with its reason.)*
   * The exit-code probe evidence in the MADR is asserted by this test.
9. **CI:** a macOS step running the live test.

### Phase V4: `selfupdate/service/scm`

1. **`New(Options) (*Service, error)`**, with `Options{Name string;
   Probe; Poll; RewritePath bool; StopDependents bool; TriggerStartEnabled
   bool}`.
2. **Handles:**
   * An unexported interface over `OpenSCManager`, `OpenService` (with
     the per-call masks in the MADR's §7), `QueryServiceStatusEx`,
     `ControlService`, `StartService`, `QueryServiceConfig`,
     `ChangeServiceConfig`, and the dependents list.
   * The real implementation wraps its handles in `mgr.Mgr` and
     `mgr.Service`. `mgr.Connect` is never called.
3. **Probes:** `Installed`, `Running` and `Enabled` as the MADR's §7 says.
   A driver service is refused.
4. **`Stop` and `Start`:**
   * Microsoft's clamped wait-hint and checkpoint loop for both, within
     the context;
   * `STOP_PENDING` waited out before a start;
   * the backstop;
   * the dependents rule.
5. **`WaitHealthy`:** `PollHealthy`, with the process ID read before the
   stop as `Previous`. On `SERVICE_STOPPED` it returns `ErrUnhealthy`
   carrying the exit code.
6. **`Reconcile` and `Restore`:**
   * Compare the decomposed program path, case-insensitively.
   * On a rewrite, `ChangeServiceConfig` with `SERVICE_NO_CHANGE` and a
     recomposed, quoted path.
   * Refuse `"` in the path.
7. **`Inside` and `Detach`:**
   * `Inside`: the service's process ID is the caller's or an ancestor's,
     from a `CreateToolhelp32Snapshot` walk.
   * `Detach`: `DetachProcess`.
   *(Deviation D5, 2026-10-04: `Detach` is two hops, through
   `service.HandOffHop`, and the walk checks creation times; MADR
   amendment A4.)*
8. **Tests:**
   * Fake-handle tables on every OS:
     * each state;
     * each error code;
     * a stalled checkpoint;
     * dependents running;
     * quoting a path with spaces;
     * the ancestor walk.
   * **Live, on Windows CI** (`SELFUPDATE_REQUIRE_SCM=1`), with a
     throwaway service built from a test helper, installed at a path with
     a space under a unique name:
     * the full managed update;
     * a failed health check, and its rollback;
     * **the handoff:** a process the service started runs the update. It
       hands off; the service restarts on the new binary; the result file
       reports success;
     * deletion afterwards.
   * The Windows test host runs the live test too.

### Phase V5: the CLI hook, docs and release

1. **`cli.Options.HandOff`** (MADR §9):
   * `Detach` runs after flag parsing, for an apply only.
   * When it hands off, the command prints `update handed off: <detail>`
     on stderr, exits 0, and under `--json` adds `handed_off` to the result
     object.
   * `Report` runs after every update.
   * Tests:
     * the text and JSON goldens of a handoff;
     * `--check` and `--dry-run` never call `Detach`;
     * `Report` sees the outcome;
     * a `Detach` error fails the command with exit 1.
2. **Docs:**
   * a new guide section, "Run as a service", with the agent case:
     what the agent's session sees, and how to read the result after
     reconnecting;
   * `architecture.md`;
   * the package docs;
   * the recommendation of `Type=notify` or `Type=exec`.
3. **Release notes** in this PLAN's execution record.
4. **The owner** pushes, waits for CI, and tags `v1.7.0` (annotated).
5. **After the tag,** a commit moves the workflow pins to the tag's
   commit, as the v1.6.0 PLAN's S8 did.
6. **The agent checks:**
   * CI on the tag;
   * the proxy's `@latest`;
   * a scratch consumer that drives each backend against a fake, and
     uses `cli.Options.HandOff`.

## Verification

* **V1.** Every new test was seen to fail on a deliberately broken input,
  recorded per phase.
* **V2.** `make apicheck`: `compatible with v1.6.0`. `go.mod` is
  unchanged.
* **V3.** The live tests pass in CI on Linux (system and user scope),
  macOS and Windows, and none of them skipped.
* **V4.** The handoff live test passes on each platform: an update run
  from a process the service started leaves the service running on the
  new binary.
* **V5.** The MADR's probe evidence is asserted by a live test.
* **V6.** CI is green on `main` and on `v1.7.0`.

## Rollout and Rollback

* **Rollout.** The packages and the `cli` field are new. No existing
  consumer changes behaviour until it chooses to use them.
* **Rollback.** Before the tag, any phase reverts alone. After it, fix
  forward in `v1.7.x`. A defect in one backend does not affect the others,
  or `selfupdate`.

## Execution Record

### Phase V0: records (2026-10-04)

* The MADR and the PLAN were drafted from five research reports: systemd,
  launchd, Windows SCM, Go prior art, and the two in-house programs. The
  owner answered Q1 (build the helpers), Q2 (magic-cli-remote's receipt
  shape) and Q3 (include `sd_notify`). The MADR is `accepted`, and the
  owner approved this PLAN.

### Deviation D1 (2026-10-04): the old version restores in-process

* **Found,** before any V1 code. Step 4 specified `RestoreWithOld bool`.
  In the managed flow, `Restore` runs before the binary rollback, so no
  old binary exists at a path the reconciler knows, and the updater's own
  executable path has been renamed over.
* **Decision.** The owner chose "RestoreFunc in-process". MADR amendment
  A1 records it. Step 4 is amended in place.

### Phase V1: `selfupdate/service` (2026-10-04)

* **Built,** as steps 1 to 7 specify:
  * the runner (`Runner`, `RunnerFunc`, `ExecRunner`);
  * the six typed errors;
  * `PollHealthy`;
  * `ExecReconciler` with the version 1 `Receipt` and `ExecState`;
  * the handoff core: `Detacher`, `HandOff`, `Detached`,
    `HandOffResult`, `HandOffIfInside`, `HandOffFunc`, `ReportFunc`,
    `ReadHandOffResult`, `WriteHandOffResult`, `DefaultResultPath`;
  * `DetachProcess`;
  * the depguard rules `service`, `service-launchd`, `service-scm` and
    `service-systemd`, each excluded from `other-packages`.
* **Small additions within the MADR's §9,** none changing a decision:
  * `HandOff.ID`, so a caller can name its handoff;
  * `Detached.Where`, naming the unit, job or process;
  * `ProcessDetacher(inside)`, a `Detacher` over `DetachProcess` for a
    lifecycle with no backend here.
* **Hardening found while linting.** `gosec` flagged that the result path
  comes from the environment. `WriteHandOffResult` refuses a path that is
  not absolute and clean, and `HandOffIfInside` refuses a relative
  `ResultPath`.
* **Tests:**
  * `poll_test.go` runs on a fake clock. `TestMain` makes the test binary
    the fake tool for the runner, the reconciler and the detach tests.
  * **The live detach tests:**
    * **Unix:** the detached child outlives a `SIGKILL` of its parent's
      whole process group.
    * **Windows:** with a job that allows breakaway, the child outlives
      `TerminateJobObject`. With one that does not, breakaway is refused,
      the start is retried inside the job, and the child dies with it.
      That case is pinned as found.
* **Plants,** in scratch copies, each caught by its test:
  * no `Setsid`: `the detached child died with its parent's process
    group`;
  * `Previous` counted: `TestPollHealthyPreviousNeverCounts`;
  * the receipt dropped on a failed child:
    `TestExecReconcilerKeepsReceiptOnFailure`;
  * an `Inside` error read as outside: `TestHandOffIfInsideFailsClosed`;
  * the output cap raised: `TestExecRunnerCapsOutput`;
  * a detached run handing off again: `TestDetachedRunNeverHandsOffAgain`;
  * the clean-path check removed:
    `TestWriteHandOffResultNeedsCleanAbsolutePath`;
  * a `golang.org/x/term` import in each of the four packages: depguard
    refused it four times, each by its own rule (`import
    'golang.org/x/term' is not allowed from list 'service-systemd'`, and so
    on).
* **Mistakes in my own tests, caught by the plants or the hosts:**
  * `TestPollHealthyPreviousNeverCounts` first timed out before its
    default 10 s settle window could end, so the plant passed. It now sets
    no settle window.
  * The output-cap test derived its sizes from the constant under test, so
    the plant passed. It now uses fixed sizes.
  * The first depguard plant used a blank import. `revive` reported that
    line first, and golangci-lint keeps one finding per line, so depguard
    was hidden. The plant now uses a named import.
  * `filepath.Join` cleaned the unclean path in the clean-path test. It is
    now built by hand.
  * `TestHandOffFunc` used `/r/x`, which is not absolute on Windows. It
    now uses a temporary directory.
* **Windows test host:** `go test -race -count=1 ./...` rc 0 for all five
  packages; `selfupdate/service` took 25.2 s.
* **Checks** (`gate.sh`, every one rc 0):
  * `make lint`, 0 issues;
  * race and shuffle;
  * `make apicheck`: `compatible with v1.6.0`;
  * fuzz, vuln, tidy (`go.mod` unchanged);
  * every script test;
  * cross vet.

### Deviation D2 (2026-10-04): the systemd handoff's environment

* **Found,** before any V2 code. Step 6 passes the environment with
  `--setenv`. Any local user can read a unit's `Environment` with
  `systemctl show`, and the detached run needs the caller's environment,
  credentials included.
* **Decision.** The owner chose "Private env file". MADR amendment A2
  records it: a 0600 file in a 0700 directory on tmpfs, passed with
  `-p EnvironmentFile=`, and deleted once a `Type=exec` unit has started on
  systemd 240 or later.

### Phase V2: `selfupdate/service/systemd` (2026-10-04)

* **Built,** as steps 1 to 7 specify:
  * `New`, `Options`, `Scope`, `Available`;
  * `ValidUnitName`, `Escape`, `TemplateInstance`;
  * the probes over one `show` per call;
  * `Stop`, with the backstop, and `Start`;
  * `WaitHealthy`, which requires a new `InvocationID` and an unchanged
    `NRestarts`;
  * `Reconcile` and `Restore`, with the `DropIn` receipt;
  * `Inside`, by cgroup;
  * `Detach`, by `systemd-run`, with the private environment file of
    deviation D2;
  * `Notify`, `Ready`, `Reloading`, `Stopping`, `Watchdog`, `Status` and
    `WatchdogInterval`.
* **Within the MADR's §5,** the opt-in rewrite reads the effective
  `ExecStart=` line from the unit's files, the fragment then the drop-ins
  in `DropInPaths` order. `show -p ExecStart` joins `argv[]` with spaces
  and loses systemd's quoting. Only the program is replaced. A unit with
  more than one `ExecStart` command is refused.
* **`MONOTONIC_USEC`** is read with a raw `clock_gettime` call:
  `x/sys/unix` is not among the package's imports in the MADR's §1.
* **Live, on two Linux test hosts:** Ubuntu 26.04 with systemd 259, and
  WSL on the Windows test host, Ubuntu 24.04 with systemd 255. Each ran
  `TestLiveManagedUpdate`, `TestLiveHealthFailureRollsBack` and
  `TestLiveHandOff`, in system scope under `sudo` and in user scope. All
  twelve passed. The handoff test shows each step:
  * from inside the unit, `Stop` was refused with `ErrInsideService`;
  * the update handed off to a transient unit;
  * the unit restarted on the handed-off build, with a new
    `InvocationID`;
  * the result file reported exit 0, applied and started;
  * a value with `"`, `$`, `\`, a backquote and a newline came through
    the environment file unchanged.

  `systemctl start` returned only after the helper's delayed `READY=1`,
  more than 1 s later.
* **Captured output.** `testdata/` holds real `systemctl show` output
  from both hosts: an active unit on each, and a unit that does not exist.
  `TestCapturedShowOutput` replays it through the probes. The captures
  settle two points the MADR listed as inferred:
  * `show` on a unit that does not exist exits 0 with
    `LoadState=not-found`;
  * properties come in systemd's order, not the order `-p` asked for,
    which is why they are parsed by key.

  They also include a real `static` unit, systemd-journald, which
  `is-enabled` would call enabled.
* **Plants,** in scratch copies, each caught:
  * `static` counted as enabled: `TestProbes`;
  * the `Stop` backstop removed: `TestStopRefusesInsideUnit`;
  * no `Previous` invocation: `TestWaitHealthyNeedsNewInvocation`;
  * the restart check removed: `TestWaitHealthyFailsFast/restarted`;
  * the environment file kept after a `Type=exec` start: `TestDetach`;
  * the environment file's escaping removed: `TestEnvFileBody`;
  * an exact cgroup match only: `TestInside`;
  * **live,** on the systemd 259 host: `Detach` replaced by
    `service.DetachProcess`, a plain `setsid` child. `TestLiveHandOff`
    failed in both scopes, because the detached run was still in the unit's
    cgroup, so its own stop was refused. That is the case the transient
    unit exists for.
* **Mistakes, mine, caught by the hosts and the linter:**
  * My first WSL runner passed `$(id -u)` through Git Bash on the Windows
    side, which expanded it to the Windows user's ID. The service then got
    a runtime directory it could not use, and the script reported `rc=0`
    after a failure. The runner script is now copied into the target and
    run there. Git Bash also rewrote `/var/tmp` into a Windows path, which
    `MSYS_NO_PATHCONV=1` stops.
  * `TestDetach` and `TestDetachOldSystemd` assumed Linux paths and file
    modes, and failed on Windows. They are now in `detach_unix_test.go`.
    `New` refuses anywhere but Linux.
  * Lint: an unchecked `os.Remove`; a directory `chmod` that `gosec` read
    as a file's; De Morgan in `validEnvName`; and `inside` beside `Inside`,
    now `insideUnit`.
* **CI:** a Linux step runs the live tests in system scope under `sudo`,
  then in the runner user's own manager. actionlint and
  `check-workflows.sh` pass.
* **Windows test host:** `go test -race -count=1 ./...` rc 0 for all six
  packages. The systemd package's fake tests run there too.
* **Checks** (`gate.sh`, every one rc 0):
  * `make lint`, 0 issues;
  * race and shuffle;
  * `make apicheck`: `compatible with v1.6.0`;
  * fuzz, vuln, tidy (`go.mod` unchanged);
  * every script test;
  * cross vet.

### Deviation D3 (2026-10-04): the launchd handoff's environment, and its cleanup

* **Found,** before any V3 code.
  * Step 7's one-shot job would carry the caller's environment in its
    plist. A non-root user on the development host read a system daemon's
    environment with `launchctl print system/<label>`.
  * Step 7 has `CleanupPending` boot out finished jobs. `CleanupPending`
    belongs to `selfupdate`, which this PLAN does not change.
* **Decision.** The owner chose "Private env file" and
  "Job.CleanupHandOffs". MADR amendment A3 records both. V3 also adds, to
  `selfupdate/service`, `EnvHandOffEnv`, `WriteHandOffEnv` and
  `LoadHandOffEnv`, which `ReportFunc` calls.

### Deviation D4 (2026-10-04): the launchd live test fails when required

* **Found** while writing the live test. Step 8 has it skip when
  `launchctl managername` is not `Aqua`; Verification V3 has no live test
  skip in CI. With `SELFUPDATE_REQUIRE_LAUNCHD=1` on a host without a GUI
  login session, the two cannot both hold.
* **Decision.** The owner chose "Fail when required". Without the
  variable the test skips with its reason, as the systemd test does; with
  it, a host that is not `Aqua` fails the test, so CI cannot pass on a
  skip. Step 8 is annotated in place. No MADR decision changes.

### Phase V3: `selfupdate/service/launchd` (2026-10-04)

* **Built,** as steps 1 to 7 specify, with deviation D3:
  * `New`, `Options`, `Domain` (`System`, `GUI`, `User`);
  * the probes: `list` for a domain it sees, else `print`'s top-level
    `pid =` and `state =` lines only;
  * `Stop`, with the backstop, `bootout`, and the wait until `print`
    exits 113 and the old PID is gone; `Start`, as `enable`, `bootstrap`
    with retries, and `kickstart`;
  * `WaitHealthy`, with a settle window of at least `ThrottleInterval`;
  * `Reconcile` and `Restore`, with the `PlistBackup` receipt;
  * `Inside`, by process group or ancestry; `Detach`, the one-shot job;
    `CleanupHandOffs`;
  * in `selfupdate/service`: `EnvHandOffEnv`, `WriteHandOffEnv` and
    `LoadHandOffEnv`, which `ReportFunc` calls first.
* **Within the MADR's §5:**
  * Exit 119 is mapped: its error says the job is disabled and that
    `launchctl enable` clears it.
  * Exit 36 ("Operation now in progress") is treated as 37 is: success
    for `bootout`, which is then waited out, and retried for
    `bootstrap`. No probe produced it; the mapping is defensive.
  * A one-shot job counts as finished only in state `not running`. One
    launchd is still spawning shows `xpcproxy` with its PID, and must not
    be booted out by a concurrent cleanup.
  * Paths are checked with `filepath.IsAbs`, the same as a leading `/` on
    macOS, so the fake suite also runs on the Windows test host.
* **Live, on the development Mac** (`gui/<uid>`, `Aqua`), with
  `SELFUPDATE_REQUIRE_LAUNCHD=1`; all five pass, and nothing is left
  loaded:
  * `TestLiveManagedUpdate`: the job restarts on the new build, a new
    PID; `print`'s `pid` and `state` agree with `list`;
  * `TestLiveExitCodes`: the codes under "Probe evidence" in the MADR;
    then `Start` brings back a job someone disabled;
  * `TestLiveStopWaitsForSlowExit`: a job ignoring SIGTERM with
    `ExitTimeOut` 3; `bootout` returns at once, and `Stop` only once the
    PID is gone, after about 3.5 s;
  * `TestLiveHealthFailureRollsBack`;
  * `TestLiveHandOff`: from inside the job, `Stop` is refused with
    `ErrInsideService`; the update hands off to a one-shot job; the job
    restarts on the handed-off build; the result file reports exit 0,
    applied and started; a value with `"`, `$`, `\`, a backquote and a
    newline comes through the environment file unchanged, and the file is
    removed.

  The live test uses one fixed label. `Start` runs `launchctl enable`,
  which keeps an override for the label in launchd's database for good,
  and launchctl cannot delete one. The development Mac keeps six such
  entries from runs with a label per run, before that was found; they are
  inert.
* **Captured output.** `testdata/` holds real `print` and `list` output
  for a running job, an idle one, one in `xpcproxy` and one in
  `SIGTERMed`, with the scratch path and the per-boot socket path
  redacted. `TestCapturedOutput` replays it through the probes. It shows
  `print`'s nested `state = active` lines, in the coalition blocks, which
  the top-level-only reading ignores, and that `list` quotes strings
  without escaping them.
* **Plants,** in scratch copies, each caught:
  * `plutil -extract … json`: `TestEnabledRealPlutil`;
  * `print` read at every depth: `TestCapturedOutput`;
  * the `Stop` backstop removed: `TestStopRefusesInsideJob`;
  * the settle window below `ThrottleInterval`:
    `TestWaitHealthySettlesAtLeastThrottle`;
  * no `bootstrap` retry: `TestStartBootstrapsAndRetries`;
  * no wait after `bootout`: `TestStopWaitsUntilGone`;
  * no 119 mapping: `TestLaunchctlError`;
  * no ancestry in `Inside`: `TestInsideByAncestry`;
  * the whole environment in the one-shot plist: `TestDetach`;
  * a spawning job cleaned up: `TestCleanupHandOffs`;
  * the environment file kept, and its mode unchecked: the
    `handoffenv_test.go` tests;
  * **live:** `Detach` as `service.DetachProcess`, a plain `setsid`
    child: `TestLiveHandOff` failed, because the child is a descendant of
    the job and its own `Stop` was refused, the case the one-shot job
    exists for; `Start` without `enable`: `TestLiveExitCodes` timed out
    on `bootstrap`'s exit 5; `Stop` without the wait:
    `TestLiveStopWaitsForSlowExit` saw the PID alive and `print` exit 0.
* **Mistakes, mine, caught by the live run, the plants and the hosts:**
  * `Enabled` read the plist with `plutil -extract … json`, not step 2's
    `raw`. `json` refuses a boolean on its own ("Invalid object in plist
    for JSON format"), so `Enabled` was false for every ordinary plist.
    The fake `plutil` hid it; it now answers only `raw`, and
    `TestEnabledRealPlutil` runs the real tool on macOS.
  * The first live builds appended bytes to a signed binary, which fails
    `codesign`'s strict validation. Builds are now told apart by their
    signing identifier.
  * The detached run was dispatched on a variable carried in the private
    environment file, which only `ReportFunc` loads; it is dispatched on
    `SELFUPDATE_HANDOFF`, in the plist, as a program would.
  * The slow-exit test first stopped the job while it was still
    `xpcproxy`, before it ignored SIGTERM; it now waits for the job's own
    ready mark.
  * `Reconcile` trimmed all white space from `plutil`'s value; it now
    trims only the trailing newline.
  * On the Windows host the fake suite failed on Unix path literals; the
    owner-and-mode rewrite test is in `reconcile_unix_test.go`.
  * Lint: `errorlint`, `goconst` (the domain kinds), and `gosec` G703 on
    the environment file's checked path.
* **CI:** a macOS step runs the live tests with
  `SELFUPDATE_REQUIRE_LAUNCHD=1`. `check-workflows.sh` passes; actionlint
  is not installed on the development Mac and runs in CI.
* **Windows test host:** `go test -race -count=1 ./...` rc 0 for all seven
  packages.
* **Checks** (`gate.sh`, every one rc 0):
  * `make lint`, 0 issues;
  * race and shuffle;
  * `make apicheck`: `compatible with v1.6.0`;
  * fuzz, vuln, tidy (`go.mod` unchanged);
  * every script test;
  * cross vet.

### Deviation D5 (2026-10-04): the SCM handoff detaches in two hops

* **Found,** before any V4 code. Step 7 has `Inside` walk the caller's
  ancestors and `Detach` call `DetachProcess`. `CreateProcess` records the
  creator as the child's parent. A probe on the Windows test host, in a
  scratch copy: `creator pid 61816; detached process 61020; child sees
  parent: 61816 true`. In a handoff the creator is a process the service
  started, and the service is still running when the detached run calls
  `Stop`, so the run's own walk reaches the service and the backstop
  refuses with `ErrInsideService`: the update would never apply.
* **Decision.** The owner chose "Two-hop detach". MADR amendment A4
  records it. `scm.Detach` starts a hop, a run of the same command marked
  by `SELFUPDATE_HANDOFF_HOP`; `service.HandOffHop`, which `ReportFunc`
  and `LoadHandOffEnv` call first, starts the real run detached and exits.
  The real run's recorded parent has exited, so the walk stops there. The
  walk also compares creation times, so a reused process ID is never taken
  for an ancestor. `DetachProcess` is unchanged. V4 adds `EnvHandOffHop`
  and `HandOffHop` to `selfupdate/service`.

### Phase V4: `selfupdate/service/scm` (2026-10-04)

* **Built,** as steps 1 to 7 specify, with deviation D5:
  * `New`, `Options` (`Name`, `Probe`, `Poll`, `RewritePath`,
    `StopDependents`, `TriggerStartEnabled`) and `PathBackup`, the
    rewrite's receipt;
  * an unexported interface over the SCM and the process table; the real
    one connects with `SC_MANAGER_CONNECT` alone, opens each service with
    its call's rights, and wraps the handles in `mgr.Mgr` and
    `mgr.Service`;
  * the probes, `Stop`, `Start`, `WaitHealthy`, `Reconcile`, `Restore`,
    `Inside` and `Detach`;
  * in `selfupdate/service`: `EnvHandOffHop` and `HandOffHop`, which
    `LoadHandOffEnv`, and so `ReportFunc`, call first.
* **Within the MADR's §7 and amendment A4:**
  * Every open adds `SERVICE_QUERY_STATUS`, which the driver check reads,
    so `Enabled` opens with `SERVICE_QUERY_CONFIG | SERVICE_QUERY_STATUS`.
    The default service ACL grants both to authenticated users.
  * The status is read with `QueryServiceStatusEx`: `mgr.Service.Query`
    leaves out the checkpoint and wait hint the polling loop needs.
  * The stall rule uses the wait hint, at least 10 s. A service built on
    `x/sys/windows/svc` reports its pending states with a zero hint and
    checkpoint unless it sets them, and Microsoft's loop would call it hung
    after 1 s.
  * A service that cannot take the stop yet,
    `ERROR_SERVICE_CANNOT_ACCEPT_CTRL`, is waited out and asked once more;
    one that still refuses is an error.
  * `Reconcile` reads an unquoted command line that starts with the
    executable as the SCM does, not split at the path's first space.
  * `Inside` needs `Options.Name`, as the systemd backend's needs
    `Options.Unit`.
  * A hop that cannot write its failure exits 2, not 1.
  * `PathBackup` has exported fields and no JSON tags, like `DropIn` and
    `PlistBackup`.
* **Live, on the Windows test host** (elevated), with
  `SELFUPDATE_REQUIRE_SCM=1`: a throwaway service, `selfupdate-livetest`,
  running a copy of the test binary from a directory whose name has a
  space. It puts itself in a job object that kills its processes when it
  exits and allows breakaway, as an agent's host may. All three pass, and
  the service is deleted afterwards:
  * `TestLiveManagedUpdate`: the probes, a no-op `Reconcile`, the update,
    a new process ID;
  * `TestLiveHealthFailureRollsBack`: the new binary is not a service
    program; the SCM's start fails with `ERROR_SERVICE_REQUEST_TIMEOUT`
    (1053) within seconds, and the old binary comes back;
  * `TestLiveHandOff`: a process the service started runs the update.
    `Stop` is refused with `ErrInsideService`; the update hands off
    through the hop; the agent dies with the service's job; the service
    restarts on the handed-off build; the result file reports exit 0,
    applied and started; a value with `"`, `%`, `\`, a backquote and a
    newline comes through unchanged.
* **The hop,** on each OS: the real run's recorded parent is the exited
  hop on Windows, and its adopter on Unix (`TestHandOffHopProcess`).
* **Plants,** in scratch copies, each caught:
  * on the development Mac: no creation-time check (`TestInside`);
    `Detach` without the hop (`TestDetach`); the backstop removed; no
    stall floor; no wait-hint clamp; `Running` as `RUNNING` only; trigger
    starts always enabled; `Stop` opening with all access
    (`TestOpenRights`); a rewrite left unquoted; an unquoted path split at
    a space; running dependents ignored; the previous process ID not
    kept; `ERROR_SERVICE_CANNOT_ACCEPT_CTRL` as an error; a driver
    accepted; the hop keeping its marker; `LoadHandOffEnv` skipping the
    hop (`TestHandOffHopProcess`);
  * on the Windows test host: the real `ComposeCommandLine` dropped
    (`TestReconcileRealCommandLine`); and, **live,** `Detach` without the
    hop, where the run's own `Stop` was refused (deviation D5's finding),
    and `Inside` without the walk, where the agent stopped its own
    service: both fail `TestLiveHandOff`.
* **Mistakes, mine, caught by the tests, the hosts and the linter:**
  * The fake SCM first advanced its scripted states on every status read,
    so the driver check and `Inside` consumed them. They now advance with
    virtual time, as a service's do.
  * `stopHandle` first retried a refused stop by recursion, with no bound.
  * The first hop process test read the run's parent before Unix had
    reparented it; the race detector's slowdown showed it. The run now
    reads it once the test has reaped the hop.
  * One `Reconcile` case relied on Windows path cleaning; it is in
    `compose_windows_test.go`.
  * A stray `var _ = errors.New` in `handoffhop.go`, removed before any
    run.
  * Lint: `errcheck` with `check-blank` on deferred closes, now joined into
    named error returns as elsewhere in the module; audited `unsafe`; a
    signed length comparison.
* **Earlier backends,** after `LoadHandOffEnv` gained the hop check: the
  systemd live tests pass in both scopes on the systemd 259 host, and the
  launchd live tests on the development Mac.
* **CI:** a Windows step runs the live tests with
  `SELFUPDATE_REQUIRE_SCM=1`. The V3 push's CI run passed, its macOS
  launchd live step running all five tests. `check-workflows.sh` passes.
* **Windows test host:** `go test -race -count=1 ./...` rc 0 for all eight
  packages.
* **Checks** (`gate.sh`, every one rc 0):
  * `make lint`, 0 issues;
  * race and shuffle;
  * `make apicheck`: `compatible with v1.6.0`;
  * fuzz, vuln, tidy (`go.mod` unchanged);
  * every script test;
  * cross vet.
