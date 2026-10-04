---
status: proposed
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
     RestoreWithOld bool; Runner Runner})`.
   * It runs the new binary's reconcile arguments, and decodes the version
     1 receipt (MADR §6) from standard output. A missing `schema_version`
     reads as 1, and unknown fields are ignored.
   * A non-zero exit with a receipt is an error that keeps the receipt in
     `ReconcileResult.State`, so `Restore` can run. A non-zero exit with no
     receipt is an error with nothing to restore.
   * `Restore` passes the receipt on standard input to the restore
     arguments, run by the new or the old binary as configured.
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
     * restore by the new binary and by the old.
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

Not started.
