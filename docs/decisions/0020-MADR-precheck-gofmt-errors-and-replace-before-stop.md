---
status: accepted
date: 2026-10-09
decision-makers: go-selfupdate-lib maintainers
consulted: "docs/reports/0016-REPORT-precheck-gofmt-errors.md; go-tui-lib's docs/decisions/0015-MADR-precheck-gofmt-errors.md and commit 47c91bf; 0011-MADR's Related note; dh_installsystemd(1); Teleport's lib/autoupdate/agent at 1283425; probes on this Mac, the Linux test host and the Windows test host, 2026-10-09"
informed: the fleet's programs, which run the pre-add check's copies and the managed installer
---
# Fail the pre-add check when gofmt fails, and offer an opt-in replace-before-stop order for managed updates

## Context and Problem Statement

Two items from the open-work list are assessed here, each on its own
evidence. They share a record, as
[0017-MADR-verify-build-provenance-and-close-0015-open-items.md](0017-MADR-verify-build-provenance-and-close-0015-open-items.md)
did, because each is small and the owner asked for both together. Each
part can be decided alone.

* **Item 1.** `scripts/go-precheck.sh` reads gofmt's list of files and
  ignores gofmt's exit status. It reports success when gofmt itself
  failed. This finding is recorded in
  [0016-REPORT-precheck-gofmt-errors.md](../reports/0016-REPORT-precheck-gofmt-errors.md),
  which decides nothing.
* **Item 4.** `ManagedInstaller` stops a running service, replaces its
  binary, reconciles the definition and starts it again.
  [0011-MADR-reference-service-lifecycles.md](0011-MADR-reference-service-lifecycles.md)
  (More Information, Related) names replacing the binary *before* the stop
  as "a candidate for a later record", because it "shortens downtime, as
  Debian's `dh_installsystemd --restart-after-upgrade` and Teleport do".

Every claim below is marked with how it was established:
* **[read]**: read in the tree or a cited source;
* **[probed]**: run on 2026-10-09, with its output;
* **[derived]**: reasoned from the two kinds above, and not run.

The probes ran on scratch clones or in scratch directories, never in the
tree. Host paths are shown as `<dir>`.

### Item 1: the pre-add check and gofmt

**The script [read].** `scripts/go-precheck.sh:95-105`:

```bash
unformatted="$(gofmt -l "${files[@]}")"
if [ -n "$unformatted" ]; then
```

It fails only when gofmt *prints* a file. The no-list path adds every
`git ls-files '*.go'` entry with no existence check (`:66-69`). The
file-list path adds only arguments that exist (`[ -f "$f" ]`, `:61`).

**What gofmt does [probed],** Go 1.27.2's gofmt, on a scratch clone at
`0b24ad7`:

| `gofmt -l` on | Exit | Standard output | Standard error |
| :--- | ---: | :--- | :--- |
| `buildinfo/buildinfo.go` | 0 | nothing | nothing |
| a file that does not parse | 2 | nothing | `buildinfo/zz_broken.go:3:9: expected ')', found '{'` |
| a missing file | 2 | nothing | `lstat buildinfo/missing.go: no such file or directory` |

**What the check does with them [probed],** `GO_PRECHECK_SKIP_VULN=1`, on
scratch clones at `0b24ad7`:

| Case | Exit | Result |
| :--- | ---: | :--- |
| P3: `buildinfo/stamp_test.go` deleted from the work tree, no file list | 0 | gofmt printed `lstat buildinfo/stamp_test.go: no such file or directory`. The run then ended `go-precheck: 294 file(s) clean (gofmt, golangci-lint, go vet, go test).` |
| P4: a file in `buildinfo` that does not parse, in a file list | 1 | gofmt's message printed, but did not fail the step. golangci-lint (`typecheck`, for linux, darwin and windows), `go vet` and `go test` failed it. |

So the check reports a *deleted tracked file* as clean. A file that does
not parse still fails, but through the later steps, not gofmt. This
matches the report's G1 and G3 at `6dcdd8a`, which counted 280 files.

**Who reaches the no-list path [read].**
* Only `make pre-add-check` with no `FILES` (`Makefile:83-84`).
* The machine-wide agent gate passes a list:
  `git diff --cached --name-only --diff-filter=ACM -- '*.go'` (its
  `precommit-checks.sh:39-43`). Every file on that list exists.
* The report's "and `make release-check`" does not hold here: this
  `Makefile` has no such target. That target is go-tui-lib's.

**The other two gofmt checks:**
* **`make gate` is already correct [read, probed].** `scripts/gate.sh:82`
  is `files=$(gofmt -l .) || return`. On a file that does not parse, the
  same logic failed with exit 2.
* **CI's line has the same defect, but `go vet` covers it [read,
  probed].** `.github/workflows/ci.yml:144` is
  `test -z "$(gofmt -l .)"`, in a step with no `shell:` key. On Linux that
  means GitHub's default `bash -e {0}`. Under `bash -e`, that line passed
  on a file that does not parse, with exit 0 and gofmt's message on
  standard error. The step's first line, `go vet ./...`, fails the same
  file with exit 1, so CI is not open today. The assignment form,
  `out="$(gofmt -l .)"; test -z "$out"`, failed under `bash -e` with exit
  2.

**The script's test cannot see the gap [read].**
`scripts/go-precheck_test.sh:34-37` stubs gofmt with a script that records
its arguments and exits 0.

**Prior art [read].** go-tui-lib took this script from this repository,
decided the same fix in its `0015-MADR-precheck-gofmt-errors.md`
(accepted), and built it in commit `47c91bf`:
* the gofmt step keeps gofmt's standard error and exit status
  (`2>"$WORK/gofmt.err"`, `gofmt_rc=$?`), and a non-zero status fails the
  step and shows the message;
* the no-list path keeps a file only if the work tree has it;
* new tests.

### Item 4: replacing the binary before stopping the service

**Today's flow [read],** `selfupdate/managed.go:96-165`:
1. probe `Installed` and `Running`, and `Enabled` when stopped;
2. `Stop` a running service;
3. `Apply`: the backup, the journal, the rename, the directory check and
   `InstallOptions.PostInstall`'s probe of the installed binary
   (`selfupdate/session.go:239-281`, `:286-307`);
4. `Reconcile`, then `Start`, then `WaitHealthy`;
5. `Commit`.

Any failure after step 2 restarts what was running (`recover`, `:226-273`).

**What the swap costs while the service is down [probed].** A scratch
test drove today's managed flow:
* a 16 MiB old binary and a 16 MiB new one;
* a lifecycle that timestamps `Stop` returning and `Start` being called;
* a no-op reconciler, and no post-install probe.

Fifteen runs on each host:

| Host | Minimum | Median | Maximum |
| :--- | ---: | ---: | ---: |
| this Mac (APFS) | 30.8 ms | 32.5 ms | 51.5 ms |
| the Linux test host | 145.6 ms | 165.3 ms | 195.3 ms |
| the Windows test host (NTFS) | 23.4 ms | 24.4 ms | 27.1 ms |

The time goes mostly to hashing the old target (`replace_*.go`,
`fileSHA256(target.Path)`) and to the syncs [derived]. A consumer's
`PostInstall` probe and its `Reconcile` add to it. Neither was measured:
* the probe runs the new binary;
* the reference backends' `Reconcile` is one `systemctl show`, `plutil`
  or SCM query when the path is unchanged (0011 §5);
* `ExecReconciler` runs the new binary.

Stopping and starting the service is not part of this window, and moving
the swap does not shorten them.

**Replacing a running binary by rename works on all three OSes [probed].**
A scratch program, built twice with its version stamped as `v1` and `v2`:
1. `v1` ran from `<dir>/app`;
2. a helper hard-linked `app` to `app.bak` and renamed the `v2` build over
   `app`, which is the library's order (`backupFile`, then
   `replacePath`);
3. the running `v1` then spawned a child from `os.Executable()`;
4. it was told to exit;
5. `app` was run once more.

| Host | Rename | `v1` kept running | Child spawned by `v1` | `v1` exit | `app` afterwards |
| :--- | :--- | :--- | :--- | :--- | :--- |
| this Mac (macOS, ad-hoc linker-signed Go binary) | `<nil>`, 0.18 ms | yes (heartbeat 16 → 38) | **`child version=v2`** | 0 | `v2` |
| the Linux test host (kernel 7.0.0) | `<nil>`, 1.65 ms | yes (20 → 41); `/proc/<pid>/exe` read `<dir>/app (deleted)` | **`child version=v2`** | 0 | `v2` |
| the Windows test host (`MINGW64_NT-10.0-26200`) | `<nil>`, 2.01 ms | yes (19 → 41) | **`child version=v2`** | 0 | `v2` |

Three things follow:
* **macOS:** Apple's warning concerns modifying signed code in place, and
  a rename does not do that. The probe agrees (0011 Context).
* **A running old version that starts its own path runs the new
  version.** `os.Executable()` gave the target path on every host:
  * on Linux, Go strips the `" (deleted)"` suffix
    (`$GOROOT/src/os/executable_procfs.go:25-27`) [read];
  * macOS uses the saved exec path, and Windows uses
    `GetModuleFileName` [read].
* **Windows already does this in production [read].**
  `TestKeepPreviousRunningImage` and the 0004 v1.1.0 PLAN's Step 10
  experiment replace and rename a running image. 0011 §9 says the SCM
  handoff's detached run "runs the executable it is about to replace".

**Prior art [read]:**
* **Debian.** dh_installsystemd(1), `--restart-after-upgrade`: "Do not stop
  the unit file until after the package upgrade has been completed. This
  is the default behaviour in compat 10. In earlier compat levels the
  default was to stop the unit file in the prerm, and start it again in
  the postinst."
* **Teleport,** `lib/autoupdate/agent/updater.go` at `1283425`.
  * `update` runs `Install`, then `Link` (the new version into the path),
    then `Setup` with reload, which runs `Reload`.
  * The service is never stopped first. `Reload` (`process.go`) runs
    `systemctl reload`, falls back to `systemctl try-restart`, then
    requires a stable PID.
  * On a failed restart it logs "Reverting symlinks due to failed
    restart", relinks the old version and reloads again.

**What each backend needs from the order [read]:**
* **systemd:** the binary is not read at stop. A unit's `ExecStop=` or
  `ExecStopPost=` that runs the program itself would run the *new* binary
  against the old instance [derived].
* **launchd:** 0011 A7 (D3) refuses a `Reconcile` path rewrite of a job
  that is running, so a rewrite must stay after the stop.
* **SCM:** `ChangeServiceConfig` applies at the next start.

**What the fleet's service programs do with their own path [read]:**
* mcp-server-magictools' `watchBinary` shuts the program down when its
  binary changes, but returns at once when `MCP_SERVICE_MODE=true`
  (`cmd/mcp-server-magictools/infra.go:35-39`). As a service it does not
  react.
* Its `writeServiceState` records `os.Executable()` as `BinaryPath`
  (`main.go:952-958`). That is a name, not a version.
* magic-cli-remote uses `os.Executable()` for its service definition
  (`internal/cli/service/setup.go`, `refresh.go`) and in one Codex
  provider check (`internal/provider/codex/provider.go:1134-1142`).

No program found re-executes itself as a worker while it serves.

## Decision Drivers

* **A gate that says "clean" must mean it** (item 1). A gate weaker than
  it claims is not a gate.
* **Correct by default** (0011). A default that changes what the old
  instance runs, or what starts after a crash, needs a reason that
  outweighs it.
* **Downtime and failure exposure.** Item 4's case rests on two benefits:
  * a shorter stop-to-start window;
  * failures before any stop: a failed `Apply` or post-install probe
    would then cost no downtime at all.
* **The seams stay as they are.** `Lifecycle`, `Reconciler` and
  `TwoPhaseSession` are consumer contracts, so a change to them is
  additive or none.
* **The API may grow ahead of need, when it is sensible and opt-in** (the
  owner's stated preference).

## Considered Options

**Item 1:**
* **1A. Fix the check, its test and CI's line.** The gofmt step fails on
  gofmt's exit status and shows its standard error. The no-list path
  skips a missing file for gofmt and still checks its package. The test's
  gofmt stub can fail on demand. CI's line uses the assignment form.
* **1B. Fix the check and its test; leave CI's line.**
* **1C. Record only.** The agent gate passes a list, and CI's `go vet`
  catches a file that does not parse.

**Item 4:**
* **4A. Keep stop-then-replace,** and close 0011's candidate with this
  evidence.
* **4B. Opt-in replace-before-stop.** A `ManagedOptions` field runs
  `Apply`, with its probe, while the service runs, then stop, reconcile,
  start and check. The default stays as it is.
* **4C. Replace-before-stop as the new default.**
* **4D. Preflight before the stop.** Run `Apply`'s fallible checks
  before the stop (the directory check, a pending journal, the staging
  file, hashing the old target, the backup link), and keep only the
  rename and its sync in the window.

## Decision Outcome

Chosen options: **"1A"** and **"4B"**, the owner's answers of 2026-10-09
(Owner questions), with 4B's setting as a bool and a new result field
that reports the order.

### Item 1: 1A

1A closes a gate that reports success over an error. It follows the fix
go-tui-lib already ships, and costs one script, its test and one CI line.

1. **The gofmt step** captures standard error and the exit status:
   `unformatted="$(gofmt -l "${files[@]}" 2>"$err")"; rc=$?`. A non-zero
   status fails the step with `gofmt: failed (exit N)` and gofmt's
   message. The list check stays as it is.
2. **The no-list path** passes a tracked file to gofmt only if the work
   tree has it (`[ -f "$f" ]`). Its directory still goes to `dirs`, so the
   deletion's package is vetted and tested, as for a listed deletion
   ([0010-MADR-remediate-second-debugging-pass-findings.md](0010-MADR-remediate-second-debugging-pass-findings.md)
   D3). This differs from go-tui-lib, which drops the file.
3. **`scripts/go-precheck_test.sh`:**
   * the gofmt stub exits `${STUB_GOFMT_RC:-0}` and prints
     `${STUB_GOFMT_ERR:-}` on standard error;
   * one case expects exit 1 and gofmt's message when it fails;
   * one case deletes a tracked file and runs with no list: exit 0, no
     `gofmt` call naming it, and its package vetted.
4. **CI's line** becomes `out="$(gofmt -l .)"; test -z "$out"`, which
   fails under the step's `bash -e` when gofmt does.
5. No release: none of this is in the module's API.

### Item 4: 4B

The evidence does not support changing the default (4C):
* **The window it removes is small.** It is a median of 24–165 ms for a
  16 MiB binary, plus a consumer's probe and reconcile. Stopping and
  starting, which it does not touch, dominate.
* **It adds proven hazards for every consumer:**
  * the old instance's own spawns run the new version;
  * an unplanned restart (`Restart=`, `KeepAlive`, SCM failure actions)
    starts the new binary before its health check;
  * a unit's `ExecStop=` that runs the program runs the new binary against
    the old instance.

It is a real benefit for a consumer whose service has none of those, and
whose `Apply` or post-install probe can fail. Then a failure costs no
downtime at all, as with Debian's and Teleport's order. So it is offered,
opt-in:

1. **API, additive:**

   ```go
   // ManagedOptions configure a ManagedInstaller. The zero value is
   // today's behaviour.
   type ManagedOptions struct {
       // ReplaceBeforeStop runs Apply, and InstallOptions.PostInstall's
       // probe, while a running service still runs; then Stop,
       // Reconcile, Start, WaitHealthy and Commit, as before. Use it only
       // when the service never starts its own executable while it
       // runs, and no ExecStop-like hook runs it.
       ReplaceBeforeStop bool
   }

   func NewManagedInstallerWith(inner Installer, life Lifecycle, rec Reconciler, opts ManagedOptions) (*ManagedInstaller, error)
   ```

   `NewManagedInstaller` and `NewManagedInstallerFor` keep their
   signatures and pass the zero value.

   The order that ran is reported (owner answer 4):

   ```go
   // In InstallResult and Result.

   // ReplacedBeforeStop reports that the binary was replaced while the
   // service still ran, before Stop: ManagedOptions.ReplaceBeforeStop
   // was set and the service was running. It is reported on a failed
   // install too, so a rollback can be read against the order.
   ReplacedBeforeStop bool
   ```

   `ResultDocument` gains `replaced_before_stop`, and its
   `schema_version` becomes 4. Fields are only added: a reader of schema 3
   reads schema 4.
2. **The flow with the option, for a running service:**
   1. `Apply`;
   2. `Stop`;
   3. `Reconcile`;
   4. `Start`;
   5. `WaitHealthy`;
   6. `Commit`.

   `Reconcile` stays after `Stop`, because launchd refuses a path rewrite
   of a running job (0011 A7). A stopped service, or one not installed,
   runs as today: there is nothing to stop.
3. **Recovery, with the option:**
   * **A failed `Apply`** rolls back, if a backup is live, and leaves the
     service running. Nothing restarts, because nothing stopped.
   * **A failed `Stop`** rolls the binary back. `recoverStop` is given the
     applied replacement. When the service still runs, it is not
     restarted, as today. When it went down, it is started again on the
     restored binary.
   * Every later failure recovers as today.
4. **Not changed:** `Lifecycle`, `Reconciler`, `TwoPhaseSession`, the
   journal and the handoff. The handoff's `HandOffResult` carries the
   `ResultDocument`, so it reports the new field with no change of its
   own.
5. **Release:** a minor release, `v1.13.0`. The migration guide gains its
   section, and `extending-selfupdate.md` gets the option and its three
   hazards.

### Consequences

* Good, because the pre-add check no longer reports "clean" over a gofmt
  error, and its test can see a gofmt failure.
* Good, because CI's gofmt line fails on its own, not only because
  `go vet` runs first.
* Good, because a consumer can choose Debian's and Teleport's order, and
  then a failed `Apply` or probe costs no downtime.
* Neutral, because the default managed flow is unchanged. No consumer's
  behaviour changes without its choice.
* Bad, because the opt-in has hazards the library cannot detect. They are
  documented, not checked.
* Bad, because `recoverStop` and `Apply`'s failure path gain a branch,
  with tests on three OSes.
* Neutral, because the window removed is milliseconds for the swap
  itself. The benefit is mainly in failures, not in normal runs.
* Neutral, because the result document becomes schema 4. The change only
  adds a key, as schemas 2 and 3 did, but a reader that pins the
  version sees a new number.

### Confirmation

* **Item 1.** Each new test case is seen failing first against today's
  script, on a scratch copy (`scripts/plant-copy.sh`):
  * the gofmt-failure case passes wrongly against today's code;
  * the deleted-file case's assertions fail against today's code.

  P3 and P4 above then give exit 1 and exit 1 with gofmt named, on
  scratch clones. CI's new line fails on a scratch file that does not
  parse when run alone under `bash -e`.
* **Item 4.** With the option, table tests over a recording lifecycle
  assert the call order, and every recovery path:
  * a failed `Apply`;
  * a failed `Stop`, with the service down and with it up;
  * a failed `Reconcile`, `Start` and `WaitHealthy`;
  * a refused `Commit`.

  Each fails first against a planted break. A Windows case replaces the
  binary while a copy of it runs as the "service". `make apicheck`
  reports the release compatible with `v1.12.1`. depguard and the gate
  are clean. `ReplacedBeforeStop` is asserted true only with the option
  and a running service, on success and on each failure path, and
  `replaced_before_stop` with `schema_version` 4 in the document's
  tests.

## Pros and Cons of the Options

### 1A. Fix the check, its test and CI's line

* Good, because every gofmt check in the tree then fails on gofmt's
  failure.
* Good, because it matches the fix go-tui-lib ships.
* Bad, because it changes a CI line that is not open today.

### 1B. Fix the check and its test

* Good, because it closes the reported gap.
* Bad, because CI's line stays correct only as long as `go vet` runs
  before it.

### 1C. Record only

* Good, because nothing changes.
* Bad, because `make pre-add-check` keeps reporting "clean" over a gofmt
  error.

### 4A. Keep stop-then-replace

* Good, because it adds no code and no hazard, and the measured gain is
  small.
* Bad, because a consumer that wants Debian's order has no way to get it,
  and a failed `Apply` or probe always costs a restart.

### 4B. Opt-in replace-before-stop

* Good, because the consumer who knows its service is safe gets the order,
  and the default stays safe.
* Good, because `ManagedOptions` gives later managed-flow choices a
  place.
* Bad, because the hazards are documented, not detected.

### 4C. Replace-before-stop as the default

* Good, because every consumer gets the failure benefit.
* Bad, because every consumer gets the three hazards, with no change on
  its side. That fails "correct by default".

### 4D. Preflight before the stop

* Good, because most fallible checks run before the stop, with no hazard:
  nothing is replaced until the service is down.
* Bad, because the post-install probe, which runs the installed binary,
  still runs after the stop.
* Bad, because the window shrinks only by the hash and the link, and
  splitting `Apply` changes `TwoPhaseSession`, a consumer contract.

## More Information

### Owner questions

Asked and answered on 2026-10-09:

1. **Item 1:** 1A, 1B or 1C? The owner chose **"1A: fix check, test,
   CI"**, as recommended.
2. **Item 4:** 4B, 4A, 4C or 4D? The owner chose **"4B: opt-in
   setting"**, as recommended.
3. **4B's shape:** a bool, `ManagedOptions.ReplaceBeforeStop`, or an
   order enum (`ManagedOrder`: `StopThenReplace`, `ReplaceThenStop`)?
   The owner chose **"Bool field"**, as recommended.
4. **4B's reporting:** no new result field, or
   `InstallResult.ReplacedBeforeStop` and a `ResultDocument` field, which
   makes the result document schema 4? The recommendation was no new
   field. The owner chose **"Add ReplacedBeforeStop"** (Decision
   Outcome, item 4, point 1).

### Not verified

* A consumer's `PostInstall` probe and `ExecReconciler` time inside the
  window, which depend on the program.
* The live systemd, launchd and SCM order with the option. The probes
  above used a plain process, not a service manager. The PLAN's live
  tests would pin it.
* Whether any fleet program starts its own path while it serves, beyond
  the read-only search above.

### Sources

* [0016-REPORT-precheck-gofmt-errors.md](../reports/0016-REPORT-precheck-gofmt-errors.md)
* go-tui-lib, `docs/decisions/0015-MADR-precheck-gofmt-errors.md`, and
  its commit `47c91bf`, "fix(precheck): fail on gofmt errors and skip
  deleted files"
* [0011-MADR-reference-service-lifecycles.md](0011-MADR-reference-service-lifecycles.md),
  More Information, Related; A7 (D3)
* [0004-PLAN-v1-1-0-core-api.md](0004-PLAN-v1-1-0-core-api.md), Step 10's
  record: replacing a running image on Windows
* dh_installsystemd(1):
  <https://manpages.debian.org/unstable/debhelper/dh_installsystemd.1.en.html>
* Teleport, `lib/autoupdate/agent/updater.go` and `process.go` at
  `1283425b60ec5f60d509ba4c791183d452923ff7`:
  <https://github.com/gravitational/teleport/tree/1283425b60ec5f60d509ba4c791183d452923ff7/lib/autoupdate/agent>
* Go's `os.Executable`: `$GOROOT/src/os/executable_procfs.go`,
  `executable_darwin.go`, `executable_windows.go`
* GitHub Actions' default shell, `bash -e {0}` on Linux: "Workflow syntax
  for GitHub Actions", `jobs.<job_id>.steps[*].shell`
