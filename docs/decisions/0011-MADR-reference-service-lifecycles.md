---
status: accepted
date: 2026-10-04
decision-makers: go-selfupdate-lib maintainers (the owner)
consulted: systemd, launchd and Windows Service Control Manager primary documentation; the source of kardianos/service, coreos/go-systemd, golang.org/x/sys/windows/svc/mgr, Tailscale, Teleport, Elastic Agent and Syncthing; the service code of magic-cli-remote and mcp-server-magictools
informed: magic-cli-remote, mcp-server-magictools, go-tui-lib, pi-go
---
# Build `selfupdate/service`: reference systemd, launchd and Windows SCM lifecycles on the platform tools and `x/sys`, with `PollHealthy`, `ExecReconciler`, `sd_notify`, and a handoff that keeps an updater from stopping its own service

## Context and Problem Statement

`ManagedInstaller` updates a program that runs as a service. Around the
binary swap it calls two seams the consumer supplies (`selfupdate/types.go`):

* `Lifecycle`: `Installed`, `Running`, `Stop`, `Start`, `WaitHealthy`, and
  since `v1.6.0` the optional `EnabledLifecycle.Enabled`
  ([0010-MADR-remediate-second-debugging-pass-findings.md](0010-MADR-remediate-second-debugging-pass-findings.md),
  Q2);
* `Reconciler`: `Reconcile` rewrites the service definition for the new
  binary and returns a `ReconcileResult` receipt; `Restore` undoes it.

The flow is fixed by `managed.go`: probe `Installed` and `Running`; ask
`Enabled` only when stopped; stop a running service; replace the binary by
rename; `Reconcile`; start only a service that was running or is enabled;
`WaitHealthy`; commit. Any failure stops what this update started,
`Restore`s the definition, rolls the binary back, and restarts what was
running.

[0004-MADR-evolve-selfupdate-api-and-tui-support.md](0004-MADR-evolve-selfupdate-api-and-tui-support.md)
§7 schedules, in Phase 4, a `selfupdate/service` package: "`PollHealthy` (the
loop copied verbatim in two programs) and `ExecReconciler` with one
documented JSON receipt", and "reference systemd, launchd and Windows SCM
lifecycles", using only the standard library and `x/sys` (its package
table). It records the decision to build them, not how. This record decides
how, because the details carry safety and portability risks that the 0004
table does not settle.

### What the two programs that implement the seams do today

A read-only survey of both programs (they pin `mcplib v1.4.1`, the
predecessor of this module) found:

* **mcp-server-magictools never manages its service on macOS or Windows.**
  Its updater looks for `com.mcp-server-magictools.plist`, but its
  installer writes `com.magictools.mcp-server-magictools`
  (`cmd/mcp-server-magictools/service_refresh.go:50` against
  `service.go:457`). Its Windows definition path is always absent
  (`service_refresh.go:51-54`). In both cases `Installed` is always false,
  so an update swaps the binary under a running service.
* **Its `Running` reads a PID file,** which passes on an old process still
  draining, or on a reused PID (`update_service.go:156-162`).
* **Its Windows stop does not wait for `SERVICE_STOPPED`**
  (`service_windows.go:207-234`).
* **magic-cli-remote** asks the service manager directly. Its launchd stop
  carries the bootout-wait fix that 0004 names: `launchctl bootout`, then
  poll `launchctl print` every 50 ms until it fails, for up to 15 s
  (`internal/cli/service/launchd_wait.go`). Its Windows backend is a Task
  Scheduler task, not a service; 0004 keeps it there.
* **The health loop** is the same in both: probe at once, then every
  250 ms, for 30 s, with success being `Running() == true`
  (magic-cli-remote `internal/updateclient/lifecycle.go:93-128`,
  mcp-server-magictools `update_service.go:102-135`). Neither proves that
  a *new* instance is up.
* **Their reconcile receipts are incompatible.** magic-cli-remote runs
  `<binary> setup-service --refresh --json` and reads
  `{"verdict","path","backup","changed","reloaded","reason","warnings"}`,
  documented as "add fields; never rename one" (`internal/cli/service/refresh.go:42-53`).
  mcp-server-magictools runs `<binary> service refresh --json` and reads
  `{"changed","path","backup_path","detail"}`.
* **magic-cli-remote loses its receipt on a partial failure.** When the
  child fails after writing (a failed `daemon-reload`, a failed lint), it
  prints no JSON, so the parent never learns of the backup and cannot
  restore (`internal/cli/setup_service.go:107`).
* **Neither is designed to update from inside its own service,** but one
  does. In both, `update` is a separate process a person runs, and
  magic-cli-remote's own record rejects updating from inside the daemon.
  Yet an agent that magic-cli-remote's relay spawned, and that a remote
  user controls, runs in the relay service's process tree. When it runs
  `mcremote update -y` on its host, the stop kills the agent's session and
  the updater with it, and the service never starts again (the owner's
  report, 2026-10-04).

### What the platforms require

Research into the primary documentation found constraints that any
implementation must meet. The sources are listed in More Information.

* **The service manager kills the updater if the updater stops its own
  service.** systemd's default `KillMode=control-group` kills everything
  in the unit's cgroup on stop (systemd.kill(5)). launchd kills the job's
  whole process group when the job dies (launchd.plist(5),
  `AbandonProcessGroup`). On Windows, once a service reports
  `SERVICE_STOPPED`, "the service process can be terminated at any time"
  (`SetServiceStatus`). An updater that runs inside its service and calls
  `Stop` on itself therefore dies between the stop and the start.
* **Tool output is not all an interface.**
  * systemd promises stability for `systemctl show`, not for `status`
    (PORTABILITY_AND_STABILITY.md). Its man page says not to rely on
    `is-active` and `is-enabled` exit codes: "it is better to not rely on
    those return values but to look for specific unit states". `is-enabled`
    exits 0 for `static`, `indirect`, `generated`, `transient` and `alias`,
    none of which means "starts at boot".
  * launchctl(1): "This output is NOT API in any sense at all" for
    `print`. Only the legacy `list` output "should match" earlier
    releases.
* **launchd has no stop for a `KeepAlive` job other than `bootout`.**
  `stop` and `kill` let launchd relaunch it at once. `bootout` returns
  before the job has left the domain, so an immediate `bootstrap` fails
  with 5 (I/O error) or 37 (already in progress). Error 5 also covers a
  disabled label, which `enable` clears.
* **Windows needs least privilege and correct quoting.**
  `mgr.Connect` asks for `SC_MANAGER_ALL_ACCESS` and `Mgr.OpenService` for
  `SERVICE_ALL_ACCESS`; both fail for a non-administrator. The default
  service DACL gives authenticated users `SERVICE_QUERY_STATUS` and
  `SERVICE_QUERY_CONFIG`. `mgr.Service.UpdateConfig` passes
  `BinaryPathName` unquoted, and rewrites `SidType`, `DelayedAutoStart`
  and `Description` as a side effect. An unquoted service path with a
  space is a known privilege-escalation technique (MITRE ATT&CK
  T1574.009).
* **"Running" is not "ready".** systemd's `Type=simple` reports success
  right after `fork()`, even when the binary cannot run. Windows services
  are told to report `SERVICE_RUNNING` before initialisation finishes.
  launchd has no readiness protocol at all.
* **Replacing a running binary by rename is correct on every platform.**
  Apple's "Updating Mac Software" warns that modifying signed code in place
  crashes with `Code Signature Invalid`, and says to rename a new file
  over the old. This module already replaces by rename.

## Decision Drivers

* **Correct by default.** A reference lifecycle that leaves a service
  stopped, or reports a draining process as healthy, is worse than none.
  Both in-house implementations have such defects today.
* **No new module.** AGENTS.md admits a module only with a record naming
  it; this module sits below every consumer in the dependency graph.
* **Stable interfaces only.** Parse what each platform promises to keep:
  `systemctl show` properties, `launchctl` exit codes and `list`, Win32
  status and configuration structures.
* **The `Lifecycle` and `Reconciler` seams stay as they are.** The new
  packages implement them. Changing them is a separate decision.
* **Least privilege.** Read-only probes work without elevation where the
  platform allows it. Nothing escalates on the caller's behalf.
* **Testable without a real service manager,** and proven against a real
  one in CI.
* **Fail loudly where the platform makes success impossible,** such as an
  updater about to stop its own service.

## Considered Options

* **A. Run the platform tools, and use `x/sys/windows/svc/mgr` on Windows.**
  `systemctl` and `launchctl`/`plutil` run at absolute paths through an
  injectable runner.
* **B. Use D-Bus for systemd** (`github.com/coreos/go-systemd/v22/dbus`),
  with the tools for launchd and `x/sys` for Windows.
* **C. Wrap `github.com/kardianos/service`.**
* **D. Build no reference lifecycles.** Publish `PollHealthy` and
  `ExecReconciler` only, and keep each program's own lifecycle.

## Decision Outcome

Chosen option: "A. Run the platform tools, and use `x/sys/windows/svc/mgr`
on Windows", because it adds no module, parses only the outputs each
platform declares stable, and matches what production updaters do (Tailscale
and Teleport run `systemctl`; kardianos runs both tools). Option B adds two
modules, and still needs polkit or a bus for a non-root caller. Option C
inherits defects documented in its own tracker, and its templates. Option D
leaves the defects above in place.

### 1. Packages

| Package | Contents | Imports |
| :--- | :--- | :--- |
| `selfupdate/service` | `PollHealthy`, `ExecReconciler` and its receipt, the handoff (§9), the shared typed errors, the command runner | stdlib; `golang.org/x/sys/windows` (the detached process on Windows); `selfupdate` |
| `selfupdate/service/systemd` | `Lifecycle`, `EnabledLifecycle`, `Reconciler` for a systemd unit; unit-name validation and escaping | stdlib; `selfupdate`; `selfupdate/service` |
| `selfupdate/service/launchd` | the same for a launchd job | stdlib; `selfupdate`; `selfupdate/service` |
| `selfupdate/service/scm` | the same for a Windows service | stdlib; `golang.org/x/sys/windows`, `golang.org/x/sys/windows/svc/mgr`; `selfupdate`; `selfupdate/service` |

* Every package compiles on every OS, so a consumer needs no build tags of
  its own. On the wrong OS, `New` returns `service.ErrUnsupported`.
* Each package gets its own depguard rule naming exactly these imports,
  and is excluded from `other-packages` (AGENTS.md).
* `schtasks` stays in magic-cli-remote behind `Lifecycle`, as 0004 says.
* `selfupdate` and `selfupdate/cli` do not import the new packages.
  `selfupdate/cli` gains one additive field, `Options.HandOff` (§9), a
  struct of two plain functions, so it imports nothing new.

### 2. What every backend does

* **Configuration is explicit.** A backend is built from an options struct
  naming the unit, label or service; its scope (systemd system or user;
  launchd `system`, `gui/<uid>` or `user/<uid>`); and, for tests, the tool
  path and runner. A name is never derived from the product silently. A
  default `<product>.service` or label is used only when it passes the
  platform's name grammar. A scope is never switched automatically.
* **Tools run at absolute paths** (`/usr/bin/systemctl`, falling back to
  `/bin/systemctl`; `/bin/launchctl`; `/usr/bin/plutil`), never through
  `PATH`. The environment is built, not inherited: `LC_ALL=C`,
  `SYSTEMD_PAGER=cat`, `SYSTEMD_COLORS=0`, and, in systemd user scope
  only, `XDG_RUNTIME_DIR` and `DBUS_SESSION_BUS_ADDRESS`. A unit or label
  is validated before use, and `--` precedes it.
* **Every call honours its context.** The runner kills the tool when the
  context ends.
* **Errors are typed,** in `selfupdate/service`, and wrap
  `selfupdate.ErrManagedInstall` where the managed installer reports them:
  * `ErrUnsupported`: the platform or OS is not this backend's;
  * `ErrNotInstalled`: the definition does not exist;
  * `ErrPermission`: the platform refused for lack of rights;
  * `ErrInsideService`: see §3;
  * `ErrTimeout`: a stop, start or health wait outlived its deadline;
  * `ErrUnhealthy`: the service failed, crashed or restarted during the
    health wait.
* **`Running` means a process holds the image,** so that `Stop` covers a
  service that is starting, reloading or paused. `WaitHealthy` alone
  demands the fully running state.
* **`WaitHealthy` proves a new instance.** It uses `PollHealthy` (§4) and
  requires an identity that changed since the update began: systemd's
  `InvocationID`, the launchd PID, the SCM process ID.

### 3. An updater never stops its own service; it hands the update off

Before anything changes, the update asks the backend whether the calling
process is inside the service's kill scope:

* **systemd:** the caller's cgroup, from `/proc/self/cgroup`, is the
  unit's `ControlGroup` or below it. `$INVOCATION_ID` alone is not enough:
  systemd.exec(5) passes it to the unit's processes, but an agent's
  session can clear its environment.
* **launchd:** the caller's process group is the job's, which launchd
  reaps with the job (launchd.plist(5), `AbandonProcessGroup`), or the
  job's PID is one of the caller's ancestors.
* **SCM:** the service's process ID is the caller's or one of its
  ancestors', from a process snapshot.

When it is, the update is handed off (§9) instead of run in place. As a
backstop, `Stop` makes the same check and returns `ErrInsideService`, so a
consumer that skips the handoff gets an error rather than a service that
never starts again.

### 4. `PollHealthy`

```go
type HealthProbe func(ctx context.Context) (Health, error)

type Health struct {
	Ready    bool   // the instance counts as up
	Instance string // an identity that changes with each start
	Failed   bool   // the instance failed; stop waiting
	Detail   string
}

type PollOptions struct {
	Interval time.Duration // default 250 ms
	Timeout  time.Duration // default 60 s
	Settle   time.Duration // default 10 s: Ready, with the same Instance, for this long
	Previous string        // an Instance that must not count, such as the one before the update
}

func PollHealthy(ctx context.Context, probe HealthProbe, o PollOptions) error
```

* It probes at once, then every `Interval`.
* It succeeds when `Ready` has held, with one `Instance` other than
  `Previous`, for `Settle`. A change of `Instance` during the settle window
  restarts it.
* `Failed` ends the wait with `ErrUnhealthy`. A probe error is kept and
  reported, not fatal, as in both copies today. The deadline is the
  sooner of `Timeout` and the context's.
* The defaults follow the platforms: launchd throttles a job to one start
  per 10 s by default (launchd.plist(5) `ThrottleInterval`), and Teleport
  requires the same PID over six consecutive 2 s polls before it calls an
  update healthy.
* Each backend accepts an optional application probe, run after its own
  check, for readiness the service manager cannot see (a socket, an HTTP
  endpoint, a version handshake).

### 5. Reconcile

* **The default is a verified no-op.** The binary is replaced by rename at
  the same path, so the definition already names it. Each backend's
  `Reconcile` checks that the definition's program path equals the target
  path, after cleaning both (case-insensitively on Windows), and returns
  `ReconcileResult{Changed: false}` without writing. A mismatch is an
  error, not a silent rewrite.
* **A path rewrite is opt-in** (`Options.RewritePath`), for a product that
  moves its binary between versions:
  * **systemd:** a library-owned drop-in,
    `/etc/systemd/system/<unit>.d/90-selfupdate.conf` (or the user
    equivalent), holding `ExecStart=` and then the new `ExecStart=`, as
    systemd.service(5) requires to replace the list. It is written by temp
    file and rename, then `daemon-reload`, then `NeedDaemonReload=no` is
    checked. Vendor units under `/usr/lib` are never edited, and
    `systemctl revert` is never used.
  * **launchd:** the plist's `Program` or `ProgramArguments.0` is
    replaced with `plutil -replace`, then `plutil -lint`; the file is
    written by temp file and rename with the original owner and mode. A
    changed plist needs `bootout`, the wait, then `bootstrap`.
  * **SCM:** `windows.ChangeServiceConfig` with `SERVICE_NO_CHANGE` for
    everything but the path, recomposed with `windows.ComposeCommandLine`
    so it is always quoted; a path holding `"` is refused. Never
    `mgr.Service.UpdateConfig`.
* **The receipt holds what `Restore` needs:** the previous bytes, owner
  and mode (systemd, launchd), or the previous `BinaryPathName` (SCM).

### 6. `ExecReconciler`

For a product whose own `setup` command owns its service definition, as
both in-house programs do:

* It runs the **new** binary with arguments the consumer gives, such as
  `setup-service --refresh --json`, with the context's deadline, and reads
  one JSON receipt from its standard output.
* **The receipt is version 1 of magic-cli-remote's shape** (owner answer
  Q2):

  ```json
  {"schema_version":1,"verdict":"refreshed","path":"…","backup":"…","changed":true,"reloaded":true,"reason":"…","warnings":["…"]}
  ```

  * `verdict` is `none` (no definition), `unchanged` (already right),
    `refreshed` (rewritten) or `kept` (different, but hand-edited, so left
    alone).
  * Fields are added, never renamed. A receipt with no `schema_version`
    reads as version 1, so magic-cli-remote's current output is valid.
* **The child prints its receipt even when it fails.** A receipt with
  `"changed": true` and a non-zero exit is a failure the parent can still
  `Restore`. magic-cli-remote loses this case today.
* `Restore` runs the restore arguments with the receipt on standard
  input, by the new binary or, by option, by the old one, which is where
  magic-cli-remote restores today.

### 7. The three backends

The rules below come from the platform documentation; More Information
cites each one.

* **systemd**
  * One `systemctl show -p` call per probe, parsed by key:
    `LoadState`, `ActiveState`, `SubState`, `UnitFileState`,
    `InvocationID`, `MainPID`, `NRestarts`, `Result`,
    `NeedDaemonReload`, `FragmentPath`, `DropInPaths`,
    `TimeoutStartUSec`.
  * `Installed` is `LoadState` other than `not-found`. `Running` is
    `ActiveState` in `active`, `reloading`, `refreshing`, `activating` or
    `deactivating`. `Enabled` is `UnitFileState` in `enabled` or
    `enabled-runtime`.
  * `Stop` and `Start` block on the job, with `--no-ask-password
    --no-pager --quiet`.
  * `WaitHealthy` requires `ActiveState=active` with a new `InvocationID`
    and an unchanged `NRestarts` for the settle window. It fails at once on
    `failed`, on a return to `inactive`, or on an `NRestarts` increase.
  * The documentation recommends `Type=notify` or `Type=exec` for the
    unit, because `Type=simple` reports success before the binary runs.
  * **`Notify`** (owner answer Q3) lets the service itself report
    readiness, so that under `Type=notify` "active" means ready:
    * `Notify(state ...string) (sent bool, err error)` sends the lines,
      joined by newlines, as one datagram to `$NOTIFY_SOCKET`: a path, or
      an abstract socket when it begins with `@` (sd_notify(3)). With the
      variable unset it returns `false` and no error, as sd_notify(3)
      returns 0.
    * `Ready`, `Reloading` (with `MONOTONIC_USEC`), `Stopping`,
      `Watchdog` and `Status(text)` send the standard states.
    * `WatchdogInterval() (time.Duration, bool)` reads `WATCHDOG_USEC`
      and `WATCHDOG_PID`.
    * It uses the standard library only, which sd_notify(3) endorses for
      reimplementing the protocol, and it never unsets the variables.
* **launchd**
  * Only `bootstrap`, `bootout`, `enable`, `kickstart`, `list`,
    `print-disabled` and `print` (for its exit code) are used, never
    `load`, `unload`, `stop` or `kill`.
  * Exit codes are mapped: 113 is not loaded; 119 is disabled; 5 and 37
    are retried within the deadline.
  * `Installed` is the plist's existence at its configured path. `Running`
    is a PID in `launchctl list <label>` (the documented-stable output), or
    in `print` for a domain `list` cannot see, parsed for `pid =` and
    `state =` only and pinned by a live test.
  * `Enabled` is (`RunAtLoad` or `KeepAlive`) and not disabled in
    `print-disabled`.
  * `Stop` is `bootout`, then polling `print` until it exits 113 (every
    50 ms, within `ExitTimeOut` plus 30 s), then confirming that the old
    PID is gone: magic-cli-remote's fix. `Start` is `enable`, then
    `bootstrap`, then `kickstart`.
  * `WaitHealthy` requires a PID other than the old one, unchanged for the
    settle window.
* **SCM**
  * Handles are opened with only the rights each call needs:
    `SC_MANAGER_CONNECT`; `SERVICE_QUERY_STATUS` for the probes;
    `SERVICE_QUERY_CONFIG` for `Enabled`; `SERVICE_STOP` or
    `SERVICE_START` with `SERVICE_QUERY_STATUS`;
    `SERVICE_CHANGE_CONFIG` only for a path rewrite. They are wrapped in
    `mgr.Mgr` and `mgr.Service`.
  * `Installed` maps `ERROR_SERVICE_DOES_NOT_EXIST` to false. `Running`
    is any state but `SERVICE_STOPPED`. `Enabled` is `SERVICE_AUTO_START`,
    delayed or not; trigger-start services count only by option.
  * `Stop` and `Start` follow Microsoft's polling algorithm: sleep a tenth
    of `dwWaitHint`, clamped to 1–10 s, and declare a hang when
    `dwCheckPoint` stalls. `ERROR_SERVICE_NOT_ACTIVE` and
    `ERROR_SERVICE_ALREADY_RUNNING` are success. Running dependent services
    are an error by default; stopping them is an option.
  * `WaitHealthy` requires `SERVICE_RUNNING` with a new process ID, stable
    for the settle window, and reports `dwWin32ExitCode` or the
    service-specific code on a stop.

### 8. Owner answers (2026-10-04)

* **Q1. Updating from inside the service: build the helpers.** "I want
  robust service management including from remote controlled agents", for
  the failure in the Context. §3 and §9.
* **Q2. The `ExecReconciler` receipt: magic-cli-remote's shape as version
  1.** §6.
* **Q3. `sd_notify`: specify the helper and include it.** §7.

### 9. The handoff

```go
// In selfupdate/service.

// A Detacher is a backend that can run an update outside its service's
// kill scope.
type Detacher interface {
	// Inside reports whether this process would die with the service.
	Inside(ctx context.Context) (bool, error)
	// Detach starts spec outside the kill scope and returns at once.
	Detach(ctx context.Context, spec HandOff) (Detached, error)
}

type HandOff struct {
	Executable string   // default: os.Executable(), resolved
	Args       []string // the update command, such as os.Args[1:]
	Env        []string // added to a built environment
	ResultPath string   // default: beside the target
}

type Detached struct {
	ID         string // the transient unit, the job label or the process ID
	ResultPath string
}

// HandOffIfInside asks d, and when inside, detaches spec.
func HandOffIfInside(ctx context.Context, d Detacher, spec HandOff) (Detached, bool, error)

// ReadHandOffResult reads what a detached run wrote.
func ReadHandOffResult(path string) (HandOffResult, error)
```

* **The detached run is the same command.** It is the same program with
  the same arguments, plus `SELFUPDATE_HANDOFF=<id>` and
  `SELFUPDATE_HANDOFF_RESULT=<path>` in its environment, and runs the
  normal managed update under the target's lock: stop,
  replace, reconcile, start, health check, rollback. With that variable
  set it never hands off again. This is Tailscale's model, where
  `tailscaled` runs `tailscale update --yes` in a transient unit.
* **It reports to a file.** It writes `HandOffResult`, `{"schema_version":1,
  "id","started_at","finished_at","exit_code","error","result"}`, where
  `result` is the `ResultDocument`, by temporary file and rename. The
  default path is `.<base>.selfupdate.handoff` beside the target, a
  directory the run can already write. An agent whose session died with
  the service reads it after reconnecting.
* **The handing-off process returns at once.** `selfupdate/cli` gains
  `Options.HandOff`, of type `cli.HandOff`:

  ```go
  type HandOff struct {
  	// Detach runs after flag parsing, before an apply (never --check or
  	// --dry-run). handedOff true ends the command.
  	Detach func(ctx context.Context, req selfupdate.Request) (handedOff bool, detail string, err error)
  	// Report runs after the update with its outcome, in every run.
  	Report func(res selfupdate.Result, err error) error
  }
  ```

  * When `Detach` hands off, the command prints `update handed off:
    <detail>` on stderr and exits 0. Under `--json`, the result object
    gains a `handed_off` key with the same detail.
  * `service.HandOffFunc(d, spec)` returns a `Detach`, and
    `service.ReportFunc()` a `Report`. The latter writes the result file
    only in a detached run, when `SELFUPDATE_HANDOFF_RESULT` is set, and
    otherwise does nothing. A program that calls `cli.Run` itself calls
    both directly.
* **The platform mechanisms:**
  * **systemd:** `systemd-run [--user] --unit=<unit>-selfupdate-<id>
    --collect --no-block --quiet --setenv=SELFUPDATE_HANDOFF=<id> --
    <exe> <args>`. This creates a transient service "with the service
    manager as its parent process" (systemd-run(1)), in its own cgroup, so
    stopping the unit does not kill it.
    * `--collect` needs systemd 236 or later (Tailscale probes the same
      versions); an older systemd returns `ErrUnsupported`.
    * System scope needs polkit's `manage-units`, root in practice; user
      scope, magic-cli-remote's, needs nothing more.
  * **launchd:** a one-shot job of its own, labelled
    `<label>.selfupdate.<id>`:
    * Its plist holds `ProgramArguments`, `EnvironmentVariables`,
      `RunAtLoad`, and `AbandonProcessGroup`, and no `KeepAlive`. It is
      written by temporary file and rename to a directory the domain
      accepts (for `system`, owned by root and not group- or
      world-writable), then `launchctl bootstrap` loads it.
    * A separate job is a separate process group, so `bootout` of the
      service does not reap it. This avoids relying on a new session
      surviving `bootout`, which is unverified.
    * A finished job is booted out, and its plist removed, by the next
      handoff or `CleanupPending`.
  * **Windows:** `CreateProcess` of the executable with
    `DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP |
    CREATE_BREAKAWAY_FROM_JOB`. When the caller's job refuses breakaway,
    which needs `JOB_OBJECT_LIMIT_BREAKAWAY_OK`, it retries without that
    flag (process-creation flags).
    * The child keeps the caller's token, so a LocalSystem service's run
      can still control the SCM, as Elastic Agent's re-exec does.
    * The child runs the executable it is about to replace. The Windows
      installer already replaces a running image by rename.
* **A generic `service.DetachProcess(ctx, spec)`** is exported for a
  consumer whose lifecycle has no backend here, such as magic-cli-remote's
  Task Scheduler task. It uses the Windows mechanism above, or on Unix a
  `setsid` child. On Unix it is documented as no defence against a cgroup
  kill or a launchd job reap, which is why the systemd and launchd
  backends do not use it.

### Consequences

* Good, because a consumer gets correct stop, start, health and reconcile
  behaviour on three platforms without writing it, and the two in-house
  programs can drop their defective copies.
* Good, because no module is added, and every parsed output is one the
  platform documents as stable, or is pinned by a live test where it is
  not.
* Good, because an updater can no longer kill itself halfway through an
  update.
* Good, because an update started from inside the service, such as by
  an agent the service spawned, completes and restarts the service,
  instead of leaving it stopped.
* Bad, because the handoff adds a mechanism per platform, with its own
  failure modes: an old systemd, a domain that refuses a job, a job object
  that refuses breakaway. Each fails before anything changes.
* Neutral, because the handing-off session still dies when the service
  restarts. The update survives it; the session cannot.
* Bad, because running tools costs a process per call and loses typed job
  results, so stop and start are confirmed by polling. A D-Bus backend can
  be added later, under its own record, behind the same seams.
* Bad, because the settle window adds about 10 s to every managed update.
* Bad, because `launchctl print` remains a fallback for a domain that
  `list` cannot see, and Apple says it is not an interface. A live macOS
  test detects a change.

### Confirmation

* `make apicheck` reports the release compatible with `v1.6.0`: the new
  packages add API and change none.
* depguard rules for the four packages fail on a planted foreign import.
* Each backend has table tests over a faked runner or SCM, a fake-tool
  test that re-runs the test binary, and an environment-gated live test
  against the real service manager in CI: a throwaway systemd unit under
  `sudo` on Linux, a throwaway LaunchAgent in `gui/$(id -u)` on macOS, and
  a throwaway service whose path has a space on Windows.
* The live tests pin the probe evidence this record relies on: launchd
  exit codes 113, 119 and 5; the `systemctl show` property values; the SCM
  states and errors.
* The handoff is proven in each live test: a process inside the
  throwaway unit, job or service runs the update; the service restarts on
  the new binary; the handoff result reports success. A second run with a
  failing health check reports the rollback in its result.
* `Notify` is proven by a throwaway `Type=notify` unit that becomes
  active only once it sends `READY=1`.

## Pros and Cons of the Options

### A. Run the platform tools, and use `x/sys/windows/svc/mgr` on Windows

* Good, because it adds no module, and `x/sys` is already required.
* Good, because `systemctl show` and the `launchctl` exit codes are
  documented interfaces, and SCM is a stable Win32 API.
* Good, because Tailscale, Teleport and kardianos/service do the same,
  so the behaviour is known in production.
* Good, because the runner and the SCM calls sit behind small interfaces
  that tests can fake on any OS.
* Bad, because there are no typed job results: stop and start completion
  is polled.
* Bad, because a tool spawn per call is slower than a bus call, though
  irrelevant at update frequency.

### B. Use D-Bus for systemd

* Good, because `StartUnitContext` and `StopUnitContext` return typed job
  results (`done`, `failed`, `timeout`, …), with no output parsing.
* Good, because root reaches PID 1 directly through `/run/systemd/private`.
* Bad, because it adds two modules (`coreos/go-systemd/v22`,
  `godbus/dbus/v5`), which this module's position below every consumer
  argues against.
* Bad, because a non-root caller still needs polkit
  (`org.freedesktop.systemd1.manage-units`) or a session bus, the same as
  the tool.
* Bad, because it covers systemd only; launchd and SCM still need A's
  approach.

### C. Wrap `github.com/kardianos/service`

* Good, because one API covers the three platforms, under the zlib
  licence.
* Bad, because it adds a module.
* Bad, because its launchd backend uses the legacy `load` and `unload`,
  whose exit code is always 0, and it parses `launchctl list` with a
  regular expression.
* Bad, because its systemd `Status` matches `is-active` text by prefix
  and does not know `deactivating` or `reloading`; its unit template hard
  codes `RestartSec=120`, about two minutes of downtime per restart in one
  downstream report.
* Bad, because k0s is replacing it with an in-tree package for these
  reasons.

### D. Build no reference lifecycles

* Good, because it costs nothing now.
* Bad, because the defects in the two programs stay, and every new
  service program re-implements the seams.
* Bad, because it abandons 0004 §7's scheduled scope.

## Amendments

### A1 (2026-10-04): the old version restores in-process

*Status: accepted (2026-10-04). The owner chose "RestoreFunc in-process"
at a PLAN stop, deviation D1 of
[0011-PLAN-reference-service-lifecycles.md](0011-PLAN-reference-service-lifecycles.md).*

* **Found.** §6 says `Restore` runs "by the new binary or, by option, by
  the old one". In the managed flow, `Restore` runs before the binary is
  rolled back. The target path then holds the new binary, the old one is
  at a backup path the reconciler is never given, and the updater's own
  executable path has been renamed over. No old binary can be run.
* **Decided.** The updater process is the old version. `ExecOptions` gains
  `RestoreFunc func(ctx context.Context, product string, r Receipt) error`.
  When it is set, `Restore` calls it in-process, which is what
  magic-cli-remote does today. Otherwise `Restore` runs the new binary's
  restore arguments, with the receipt on standard input.

### A2 (2026-10-04): the systemd handoff's environment goes in a private file

*Status: accepted (2026-10-04). The owner chose "Private env file" at a
PLAN stop, deviation D2 of
[0011-PLAN-reference-service-lifecycles.md](0011-PLAN-reference-service-lifecycles.md).*

* **Found.** §9 passes the detached run's environment with `systemd-run
  --setenv`. The detached run is the same command, so it needs the
  caller's environment, which can hold a credential such as `GH_TOKEN`.
  A unit's `Environment` property is readable by any local user through
  `systemctl show`, so `--setenv` would publish it.
* **Decided.**
  * The caller's environment, the handoff variables and `HandOff.Env` are
    written to a file of mode 0600, in a directory of mode 0700:
    `/run/selfupdate` in system scope, `$XDG_RUNTIME_DIR/selfupdate` in
    user scope, both on tmpfs.
  * `systemd-run` takes it as `-p EnvironmentFile=`, so only its path is
    visible.
  * On systemd 240 or later, the transient unit runs as `Type=exec`, so
    `systemd-run` returns only once the program has started, and the file
    is then deleted. On 236 to 239 it stays until the next handoff removes
    it.
  * Values are written in double quotes, with `\`, `"`, `` ` `` and `$`
    escaped; a name that is not a valid variable name is skipped. The
    live test pins the round trip.

## More Information

### Probe evidence

* **launchd,** on this repository's development host (macOS 26.6.2), read
  only: `launchctl print gui/$UID/<missing>`, `print system/<missing>` and
  `list <missing>` each exit 113; `launchctl error` decodes 5 as
  "Input/output error", 37 as "Operation already in progress", 113 as
  "Could not find specified service", 119 as "Service is disabled" and 125
  as "Domain does not support specified action"; `print` shows
  `exit timeout = 5` for a job with no `ExitTimeOut` key. The live tests in
  the PLAN pin these.
* **GitHub-hosted runners:** Ubuntu runners are virtual machines with
  systemd as PID 1 and passwordless `sudo`; macOS runners have a usable
  `gui/501` domain, as public workflows show; Windows runners run as
  administrators with UAC disabled.

### Sources

systemd:

* systemctl(1), systemd.unit(5), systemd.service(5), systemd.kill(5),
  systemd.exec(5), systemd-run(1), sd_notify(3), sd_booted(3),
  org.freedesktop.systemd1(5), daemon(7):
  <https://www.freedesktop.org/software/systemd/man/latest/> (read from
  the sources at <https://github.com/systemd/systemd/tree/main/man>)
* Interface stability:
  <https://github.com/systemd/systemd/blob/main/docs/PORTABILITY_AND_STABILITY.md>
* NEWS (exit code 4 for unknown units from v253; `NRestarts` from v235):
  <https://github.com/systemd/systemd/blob/main/NEWS>

launchd:

* launchctl(1), launchd.plist(5), plutil(1):
  <https://keith.github.io/xcode-man-pages/launchctl.1.html>,
  <https://keith.github.io/xcode-man-pages/launchd.plist.5.html>,
  <https://keith.github.io/xcode-man-pages/plutil.1.html>
* Apple, "Updating Mac Software":
  <https://developer.apple.com/documentation/security/updating-mac-software>
* Apple, "Creating Launch Daemons and Agents" (archived):
  <https://developer.apple.com/library/archive/documentation/MacOSX/Conceptual/BPSystemStartup/Chapters/CreatingLaunchdJobs.html>
* Apple DTS on status polling: <https://developer.apple.com/forums/thread/130269>

Windows:

* `OpenSCManagerW`, `OpenServiceW`, `ControlService`, `StartServiceW`,
  `ChangeServiceConfigW`, `SERVICE_STATUS_PROCESS`, `QUERY_SERVICE_CONFIGW`,
  "Service Security and Access Rights", "Stopping a Service", "Starting a
  Service", `SetServiceStatus`:
  <https://learn.microsoft.com/en-us/windows/win32/services/>
* `golang.org/x/sys/windows/svc/mgr` source:
  <https://github.com/golang/sys/tree/master/windows/svc/mgr>
* MITRE ATT&CK T1574.009, unquoted path:
  <https://attack.mitre.org/techniques/T1574/009/>

Prior art:

* kardianos/service:
  <https://github.com/kardianos/service/blob/99070899946d7ab341109f83b7c9fb941a118be0/service_systemd_linux.go>,
  <https://github.com/kardianos/service/blob/99070899946d7ab341109f83b7c9fb941a118be0/service_darwin.go>;
  k0s replacing it: <https://github.com/k0sproject/k0s/pull/7053>
* coreos/go-systemd:
  <https://github.com/coreos/go-systemd/blob/737492515b5789f80e3980c5755a544c6faf445f/go.mod>
* Tailscale's transient-unit updater:
  <https://github.com/tailscale/tailscale/blob/9128778b6515f32e13d92e7380044fe025f9b08e/feature/clientupdate/clientupdate.go>;
  Windows copy-and-exit:
  <https://github.com/tailscale/tailscale/blob/9128778b6515f32e13d92e7380044fe025f9b08e/clientupdate/clientupdate_windows.go>
* Teleport's stable-PID health check:
  <https://github.com/gravitational/teleport/blob/1283425b60ec5f60d509ba4c791183d452923ff7/lib/autoupdate/agent/process.go>
* Elastic Agent's watcher, and the cgroup kill it met:
  <https://github.com/elastic/elastic-agent/issues/3123>; its Windows
  re-exec through the SCM:
  <https://github.com/elastic/elastic-agent/blob/main/internal/pkg/agent/application/reexec/reexec_windows.go>
* Windows process-creation flags (`DETACHED_PROCESS`,
  `CREATE_BREAKAWAY_FROM_JOB`):
  <https://learn.microsoft.com/en-us/windows/win32/procthread/process-creation-flags>
* Syncthing's exit-and-restart codes:
  <https://docs.syncthing.net/users/syncthing.html>

### Not verified

* Whether a process in a new session survives `bootout` of its launchd
  job (launchd may also reap by resource coalition). The launchd handoff
  uses a separate job instead; `DetachProcess` on macOS is documented as
  no defence.
* Whether Task Scheduler's job object allows breakaway. It decides
  whether `DetachProcess` survives `schtasks /end` for magic-cli-remote,
  and is pinned by that program's adoption test.
* The systemd versions that added `systemd-run`'s `--wait` (232),
  `--pipe` (235) and `--collect` (236) come from Tailscale's source, not
  from NEWS; the live test runs on a current systemd.
* Whether killing a blocking `systemctl` client cancels its job in PID 1.
  The backend therefore waits on the unit's state, not on the client.
* Whether the macOS Login Items switch (macOS 13+) appears in
  `print-disabled`.
* The default ACL on `Program Files` from a primary Microsoft source.

### Handoff alternatives not chosen

* **`KillMode=process` in the unit,** Elastic Agent's fix for its
  watcher (<https://github.com/elastic/elastic-agent/pull/3220>). It
  changes the owner's unit for every stop, not just an update's, and
  systemd.kill(5) recommends against it.
* **Exit and let the service manager restart,** Syncthing's way
  (`RestartForceExitStatus=`), or `KeepAlive`, or Windows failure actions.
  Nothing runs the health check or the rollback; launchd throttles a
  restart to one per 10 s; Windows applies a change to the non-crash flag
  only at the next boot and cannot cancel a queued restart; and each
  needs the owner's definition to cooperate.
* **A separate updater service,** the Chromium updater's design. It is a
  whole second service for each product to install; the handoff needs
  none.

### Related

* A candidate for a later record: replacing the binary before stopping
  the service, rather than after, shortens downtime, as Debian's
  `dh_installsystemd --restart-after-upgrade` and Teleport do. It changes
  `ManagedInstaller`'s flow, so it is not decided here.
* Moving magic-cli-remote and mcp-server-magictools to these packages is
  their own repositories' records.
