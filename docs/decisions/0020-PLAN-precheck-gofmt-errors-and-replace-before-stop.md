---
status: complete
date: 2026-10-09
associated-madr: "0020-MADR-precheck-gofmt-errors-and-replace-before-stop.md"
---
# Implement a pre-add check that fails when gofmt fails, and an opt-in replace-before-stop order

Associated MADR: [0020-MADR-precheck-gofmt-errors-and-replace-before-stop.md](0020-MADR-precheck-gofmt-errors-and-replace-before-stop.md)

## Goal

* **Item 1 (1A).** Every gofmt check in the tree fails when gofmt fails:
  * `scripts/go-precheck.sh`;
  * CI's line;
  * `make gate`, which already does.

  The no-list path stops handing gofmt a tracked file the work tree no
  longer has, and still checks that file's package. The script's test can
  see both.
* **Item 4 (4B).** `ManagedOptions.ReplaceBeforeStop` and
  `NewManagedInstallerWith` run `Apply` while a running service still
  runs, then stop, reconcile, start, check and commit. `ReplacedBeforeStop`
  reports the order in `InstallResult`, `Result` and `ResultDocument`
  (schema 4). The default flow is unchanged.
* **`v1.13.0`** is released with both, by the release procedure of
  [0017-PLAN-verify-build-provenance-and-close-0015-open-items.md](0017-PLAN-verify-build-provenance-and-close-0015-open-items.md).

## Scope

### In scope

| Phase | What | Commit |
| :--- | :--- | :--- |
| S0 | this PLAN, its index row | records only |
| S1 | item 1: the script, its test, CI's line | one commit |
| S2 | item 4: the API, the flow, recovery, the result field, unit tests | one commit |
| S3 | item 4: a live test per backend | one commit |
| S4 | item 4: guides, architecture, migration guide, record notes | docs only |
| S5 | `v1.13.0`: the release procedure, and the pin commit | the tag, one commit |
| S6 | the closing record | records only |

### Out of scope

* **Changing the default order** (MADR 4C), and **splitting `Apply`**
  (4D).
* **Detecting the hazards** the MADR lists. The guide documents them.
* **`Lifecycle`, `Reconciler`, `TwoPhaseSession`, the journal and the
  handoff.** None of them changes.
* **The check record's schema** (`checkcache.go:303`,
  `checkRecordSchema = 3`). It is not the result document's, and stays 3.
* **go-tui-lib's and other repositories' copies** of the script.
* **Push and tags.** Each happens only on the owner's ask in the same
  turn.

## Rules for every phase

1. **Commits.** Each phase commits on `main`, with `git commit --no-edit`.
   The global `prepare-commit-msg` hook writes the message.
2. **Before each commit:**
   * `make pre-add-check FILES="<the phase's Go files>"` exits 0 when the
     phase has Go files;
   * `make gate` ends `overall=0`;
   * for a phase that changes Markdown, `scripts/check-docs.sh --links`
     and `--ids` are clean on its files.

   Long output goes to a scratch file, with `$?` captured before any
   filter.
3. **Every new test case fails first,** on a scratch copy, never in the
   tree:
   * a Go test: `scripts/plant-copy.sh FILE OLD NEW` plants one break, and
     the case runs in the copy;
   * a script case: it runs against today's script, through the test's
     `SCRIPT` variable.

   Each phase's record quotes the failing line.
4. **A deviation stops the phase.** The agent reports evidence, a
   resolution (recommended first, with its cost) and what doing nothing
   costs. It records the owner's choice as a dated `Deviation Dn` here,
   and as an amendment in the MADR when a decision or fact changes. No
   workaround: no loosened assertion, no skipped test, no swallowed error.
5. **Identifiers.** Host results name the host by role ("this Mac", "the
   Linux test host", "the Windows test host") and paths as `<dir>`.
6. **Each phase's record** goes under `## Execution record`: commands, exit
   statuses and the output lines that decide.

## Implementation Steps

### Phase S0: records

**Files:**
* `docs/decisions/0020-MADR-precheck-gofmt-errors-and-replace-before-stop.md`
  (accepted; written before this PLAN)
* `docs/decisions/0020-PLAN-precheck-gofmt-errors-and-replace-before-stop.md`
* `docs/README.md`

1. `docs/README.md` gains this PLAN's row after the MADR's, `proposed`.
2. On approval, this PLAN becomes `status: in-progress`, with an
   `### Approval` entry quoting the owner.
3. Links, identifiers and `make gate`, then commit.

### Phase S1: item 1, the gofmt step

**Files:**
* `scripts/go-precheck.sh`
* `scripts/go-precheck_test.sh`
* `.github/workflows/ci.yml`

**Facts this phase rests on (MADR, Item 1):**
* gofmt exits 2 with nothing on standard output for a file it cannot read
  or parse.
* The script reads only standard output (`:98-99`).
* The no-list path has no existence check (`:66-69`).
* The test's gofmt stub always exits 0 (`:34-37`).
* CI's `test -z "$(gofmt -l .)"` (`ci.yml:144`) passes under `bash -e`.

1. **`scripts/go-precheck.sh`, the header.** After the 0013 D4 sentence,
   add a sentence naming this record: gofmt's own failure fails the check,
   and the no-list path skips a tracked file the work tree no longer has.
   The usage paragraph (`:30-34`) says so too.
2. **The no-list loop** (`:67-69`) becomes:

   ```bash
     while IFS= read -r f; do
       [ -n "$f" ] || continue
       # A tracked file deleted but not yet staged is not a file to format;
       # its package is still vetted and tested (0010-MADR D3).
       [ -f "$f" ] && files+=("$f")
       dirs+=("$(dirname "$f")")
     done < <(git ls-files '*.go')
   ```

3. **The gofmt step** (`:95-105`) becomes:

   ```bash
   # 1. gofmt, over the files that exist. Its exit status counts as well as
   # its list: a file it cannot read or parse makes it fail with nothing on
   # its output
   # (docs/decisions/0020-MADR-precheck-gofmt-errors-and-replace-before-stop.md).
   if [ "${#files[@]}" -gt 0 ]; then
     need gofmt || exit 2
     gofmt_err="$(mktemp)"
     unformatted="$(gofmt -l "${files[@]}" 2>"$gofmt_err")"
     gofmt_rc=$?
     if [ "$gofmt_rc" -ne 0 ]; then
       echo "gofmt: failed (exit $gofmt_rc):" >&2
       sed 's/^/  /' "$gofmt_err" >&2
       fail 1
     fi
     rm -f "$gofmt_err"
     if [ -n "$unformatted" ]; then
       echo "gofmt: these files are not formatted (run 'gofmt -w <file>'):" >&2
       printf '%s\n' "$unformatted" | sed 's/^/  /' >&2
       fail 1
     fi
     ran+=(gofmt)
   fi
   ```

   The script runs with `set -uo pipefail`, without `-e`, so `$?` after
   the assignment is gofmt's status.
4. **`scripts/go-precheck_test.sh`:**
   * **The stub** (`:34-37`) becomes:

     ```sh
     #!/bin/sh
     echo "gofmt $*" >>"$CALLS"
     [ -n "${STUB_GOFMT_ERR:-}" ] && echo "$STUB_GOFMT_ERR" >&2
     exit "${STUB_GOFMT_RC:-0}"
     ```

     The comment above the stubs (`:20-22`) names both variables.
   * **Case G1, gofmt's own failure fails the check.** `r=$(repo
     gofmtfail)`, then `run "$r" STUB_GOFMT_RC=2
     STUB_GOFMT_ERR=pkg/a.go:1:1:broken -- pkg/a.go`.
     * It passes when `rc` is 1 and `$out` holds both
       `gofmt: failed (exit 2)` and `pkg/a.go:1:1:broken`.
     * The message has no space, because `run` splits its `VAR=value`
       words on spaces (`:66-76`).
   * **Case G3, a deleted tracked file with no list.** `r=$(repo
     nolist)`, `rm "$r/pkg/a.go"`, then `run "$r" --`.
     * It passes when `rc` is 0, `$WORK/calls` has `gofmt -l pkg/b.go`
       and no `gofmt` line naming `pkg/a.go`, and it has
       `go vet ./...`.
     * The file stays tracked: `repo` ran `git add`, and the deletion is
       not staged.
5. **The new cases fail first.**
   * `git show HEAD:scripts/go-precheck.sh > <scratch>/old-precheck.sh`.
   * `SCRIPT=<scratch>/old-precheck.sh sh scripts/go-precheck_test.sh`
     must report `FAIL` for G1 (`rc=0`) and for G3 (a `gofmt` call naming
     `pkg/a.go`), and `ok` for every earlier case.
   * The new test then passes against the new script: `0 failed`.
6. **`.github/workflows/ci.yml:144`** becomes:

   ```bash
             unformatted="$(gofmt -l .)"
             test -z "$unformatted"
   ```

   with a comment that the assignment form fails when gofmt fails, under
   the step's `bash -e` (this record).
   * **Seen failing:** on a scratch clone holding an unparseable file,
     `bash -e -c 'unformatted="$(gofmt -l .)"; test -z "$unformatted"'`
     exits 2. The old line exits 0 there (MADR P1).
7. **Seen working on the real tool,** on scratch clones of the S1 commit's
   tree, with `GO_PRECHECK_SKIP_VULN=1`:
   * the MADR's P3 (deleted `buildinfo/stamp_test.go`, no list) no longer
     prints `lstat`. It exits 0 when every package still passes, which is
     correct, because that file is not formatted;
   * a scratch case with a tracked file made unreadable (`chmod 000`)
     makes gofmt fail. With no list it exits 1, with
     `gofmt: failed (exit 2)` naming the file;
   * P4 exits 1, now with `gofmt: failed (exit 2)` first.
8. **Checks:** `shellcheck scripts/go-precheck.sh
   scripts/go-precheck_test.sh` is clean; `make gate` (its `scripts` and
   `shellcheck` steps run both) ends `overall=0`. Commit the three files.

### Phase S2: item 4, the API, the flow and the result

**Files:**
* `selfupdate/managed.go`
* `selfupdate/types.go`
* `selfupdate/updater.go`
* `selfupdate/document.go`
* `selfupdate/doc.go`
* `selfupdate/cli/doc.go`
* `selfupdate/managed_replacefirst_test.go` (new)
* `selfupdate/managed_started_test.go`
* `selfupdate/example_test.go`
* `selfupdate/events_test.go`
* `selfupdate/warnings_test.go`
* `selfupdate/lifecycle_windows_test.go`

**Facts this phase rests on:**
* The flow and its recovery: `managed.go:96-165`, `recoverStop` at
  `:206-215`, `recover` at `:226-273`.
* `Apply` keeps the backup and runs the post-install probe, and rolls back
  itself when the probe fails (`session.go:239-281`, `errRolledBack`).
* `updater.go:363-370` copies `InstallResult` into `Result`.
* `document.go:5` has `resultDocumentSchema = 3`.

1. **`types.go`:**
   * after `InstallResult.Previous`, add `ReplacedBeforeStop bool`, with the
     MADR's comment;
   * after `Result.ProbesSkipped`, add `ReplacedBeforeStop bool`: "reports
     what InstallResult.ReplacedBeforeStop reports".
2. **`managed.go`:**
   * `ManagedOptions`, with `ReplaceBeforeStop` and the MADR's comment.
   * `ManagedInstaller` and `managedSession` gain `opts ManagedOptions`.
   * `NewManagedInstallerWith(inner Installer, life Lifecycle, rec
     Reconciler, opts ManagedOptions)` holds today's checks.
   * `NewManagedInstallerFor` returns
     `NewManagedInstallerWith(inner, life, rec, ManagedOptions{})`.
     `NewManagedInstaller` is unchanged: it calls `...For`.
   * `Begin` passes `opts` to the session.
3. **`managedSession.Install`,** after `start` is known and before the
   `if running { Stop }` block:

   ```go
   early := running && s.opts.ReplaceBeforeStop
   var applied AppliedReplacement
   if early {
       // The binary is replaced while the old instance still runs
       // (0020-MADR 4B): a failed Apply or probe leaves it running, and
       // nothing restarts.
       a, err := s.inner.Apply(ctx, req)
       if err != nil {
           res, rerr := s.recover(ctx, product, a, ReconcileResult{}, false, false, err)
           res.ReplacedBeforeStop = a.Backup != "" || errors.Is(err, errRolledBack)
           return res, rerr
       }
       applied = a
   }
   ```

   Then:
   * **The stop:** `if running { if err := s.life.Stop(...); err != nil {
     return s.recoverStop(ctx, product, applied, err) } }`.
   * **The late apply:** `if !early { applied, err = s.inner.Apply(ctx,
     req); ... }`, with today's code and comments.
   * **Every later `recover(...)` call and the success path** set
     `ReplacedBeforeStop = early` on the result they return. One helper,
     `func mark(r InstallResult, early bool) InstallResult`, keeps it to a
     line each.

   `ReplacedBeforeStop` is true when a replacement went live before
   `Stop`. A failed early `Apply` that never renamed (no backup, and no
   `errRolledBack`) reports false.
4. **`recoverStop`** gains `applied AppliedReplacement`, which is empty
   when the binary was not replaced first:
   * **The service still runs, or cannot be asked:** with a live backup,
     `s.inner.Rollback(ctx, applied)` runs under the recovery context, and
     its error and its `RolledBack` or kept backup are reported as
     `recover` reports them. There is no restart.
   * **The service went down:**
     `s.recover(parent, product, applied, ReconcileResult{}, true, false, ...)`,
     as today, with `applied` instead of `AppliedReplacement{}`. The binary
     is rolled back, then the service is started on it.
   * **Today's callers** pass `AppliedReplacement{}`. Their behaviour is
     unchanged, which `TestManagedStopFailsAfterStoppingRestarts` and
     `TestManagedStopFailsStillRunning` keep proving.
5. **`updater.go`,** after `resultOut.RolledBack = installed.RolledBack`:
   `resultOut.ReplacedBeforeStop = installed.ReplacedBeforeStop`.
6. **`document.go`:**
   * `resultDocumentSchema = 4`. The comment gains "version 4 adds
     replaced_before_stop (0020-MADR 4B)".
   * `ReplacedBeforeStop bool \`json:"replaced_before_stop"\`` goes after
     `ProbesSkipped`, and `Document()` maps it.
7. **Package docs:**
   * `selfupdate/doc.go`'s managed-installer paragraph names the option and
     says the default order is unchanged;
   * `selfupdate/cli/doc.go:18` says schema 4, and what
     `replaced_before_stop` reports.
8. **Existing expectations of schema 3 move to 4,** and gain
   `"replaced_before_stop":false`:
   * `example_test.go:319`;
   * `events_test.go:370` and `:384`;
   * `warnings_test.go:102`.

   `checkcache_test.go`'s `"schema_version":3` lines are the check
   record's and do not change; `git diff --stat` must not list that file.
9. **New tests,** in `managed_replacefirst_test.go`. They use a recording
   lifecycle, `orderLife`:
   * its `Stop` records the target's bytes and `Running` at that moment;
   * its `Running` follows `Stop` and `Start`;
   * its failures are injectable.

   Each test builds the installer with `NewManagedInstallerWith(...,
   ManagedOptions{ReplaceBeforeStop: true})`.

   | Test | Setup | Asserts |
   | :--- | :--- | :--- |
   | `TestReplaceBeforeStopOrder` | running | at `Stop` the target held `new-bytes` and the service ran; log `stop, start, health`; `Applied`, `ReplacedBeforeStop` true |
   | `TestReplaceBeforeStopStoppedService` | stopped, enabled | log `start, health`; `ReplacedBeforeStop` false |
   | `TestReplaceBeforeStopNotInstalled` | not installed | standalone result; `ReplacedBeforeStop` false |
   | `TestReplaceBeforeStopProbeFails` | running, `PostInstall` fails | empty log: never stopped or started; target `old-bytes`; `RolledBack` and `ReplacedBeforeStop` true; error wraps `ErrManagedInstall` |
   | `TestReplaceBeforeStopApplyRefused` | running, a pending journal planted beside the target (`errJournalPending`) | empty log; target `old-bytes`; `ReplacedBeforeStop` false |
   | `TestReplaceBeforeStopStopFailsStillRunning` | running, `Stop` errors and the service stays up | log `stop`; target `old-bytes`; `RolledBack` true; no `start` |
   | `TestReplaceBeforeStopStopFailsWentDown` | running, `Stop` errors after stopping | log `stop, start, health`; target `old-bytes`; `RolledBack` true |
   | `TestReplaceBeforeStopReconcileFails` | running, `Reconcile` errors | log `stop, start, health`; target `old-bytes`; `RolledBack`, `ReplacedBeforeStop` true |
   | `TestReplaceBeforeStopHealthFails` | running, `WaitHealthy` errors | log `stop, start, health, stop, start, health`; target `old-bytes`; `RolledBack` true |
   | `TestReplaceBeforeStopCommitRefused` | running, the directory swapped in `Reconcile` (as `swapRec`) | `ErrConcurrentUpdate`; the new binary rolled back; `ReplacedBeforeStop` true |
   | `TestNewManagedInstallerWithRefusesNil` | nil and typed-nil installer, lifecycle, reconciler | each refused, as `TestManagedRefusesTypedNil` |
   | `TestManagedDefaultOrderUnchanged` | `NewManagedInstallerFor`, running | at `Stop` the target held `old-bytes`; `ReplacedBeforeStop` false |

   * **In `managed_started_test.go`:** `TestRunReportsReplacedBeforeStop`
     drives `Updater.Run` with the option and a running fake service, as
     `TestRunReportsRolledBack` does. It asserts
     `Result.ReplacedBeforeStop` and the document's
     `"replaced_before_stop":true`.
   * **`ExampleNewManagedInstallerWith`** goes in `example_test.go`, with
     no output, as `ExampleNewManagedInstallerFor`.
10. **On Windows,** `lifecycle_windows_test.go` gains
    `TestReplaceBeforeStopRunningImage`:
    * the target runs as a helper process (`SELFUPDATE_NATIVE_HELPER`, as
      `TestKeepPreviousRunningImage`);
    * the lifecycle's `Stop` ends the helper and waits for it;
    * the install with the option is `Applied`, the target holds the new
      bytes, and `PendingBackup` is empty, because the old image had
      stopped by `Commit`.
11. **Each new test fails first.** Each plant is made with
    `scripts/plant-copy.sh`, and the named tests run in the copy:

    | Plant in the copy | Must fail |
    | :--- | :--- |
    | `early := running && s.opts.ReplaceBeforeStop` → `early := false` | `TestReplaceBeforeStopOrder`, `...ProbeFails`, `TestRunReportsReplacedBeforeStop` |
    | in `recoverStop`, the still-running branch's `Rollback` call removed | `TestReplaceBeforeStopStopFailsStillRunning` |
    | the early-`Apply` failure calls `recover` with `restart` true | `TestReplaceBeforeStopProbeFails` (a `start` in the log) |
    | `document.go`: the `ReplacedBeforeStop` mapping removed | `TestRunReportsReplacedBeforeStop` |
    | `updater.go`: the copy of `ReplacedBeforeStop` removed | `TestRunReportsReplacedBeforeStop` |
    | `NewManagedInstallerFor` passes `ManagedOptions{ReplaceBeforeStop: true}` | `TestManagedDefaultOrderUnchanged` |

    The Windows test runs on the Windows test host (it builds only there).
    Its plant, `early := false`, makes it fail on "the target held
    old-bytes at Stop", which it asserts too.
12. **Checks:**
    * `go test -race ./selfupdate/...`;
    * `CGO_ENABLED=0 GOOS=windows go vet ./selfupdate/...`;
    * the Windows test, 30 runs on the Windows test host (`-count=30`),
      as 0017 Q4 ran its own;
    * `make apicheck` reports "compatible with v1.12.1";
    * `make pre-add-check`, then `make gate`.

    Commit.

### Phase S3: item 4, live tests per backend

**Files:**
* `selfupdate/service/systemd/live_linux_test.go`
* `selfupdate/service/launchd/live_darwin_test.go`
* `selfupdate/service/scm/live_windows_test.go`

**Facts:**
* Each backend has `managedInstall(<backend>, target, newPath)`
  (`systemd :151`, `launchd :136`, `scm :225`), which builds
  `NewManagedInstaller(inner, b, b)`.
* CI runs `^TestLive` in each package, under `SELFUPDATE_REQUIRE_SYSTEMD`
  (system and user scope), `SELFUPDATE_REQUIRE_LAUNCHD` and
  `SELFUPDATE_REQUIRE_SCM` (`ci.yml:61-95`). New `TestLive…` tests run
  there with no workflow change.

1. **In each file,** `managedInstall` becomes a call to
   `managedInstallWith(life selfupdate.Lifecycle, rec selfupdate.Reconciler,
   target, newPath string, opts selfupdate.ManagedOptions)`, so existing
   callers are unchanged.
2. **A wrapper, `stopCheck`,** embeds the backend (`*Unit`, `*Job` or
   `*Service`), so `Enabled` and the `Reconciler` methods are promoted. Its
   `Stop` records two things before calling the backend's `Stop`:
   * whether the target already holds the new build's marker;
   * the backend's `Running`.
3. **`TestLiveReplaceBeforeStop`,** per backend, on the throwaway unit,
   job or service that `TestLiveManagedUpdate` uses:
   * the option is set;
   * at `Stop`, the target held the new build and the service was running;
   * `Applied`, `ServiceStarted` and `ReplacedBeforeStop` are true;
   * the service's identity changed: systemd's `InvocationID`, the
     launchd PID, the SCM process ID.
4. **`TestLiveReplaceBeforeStopHealthFailure`,** per backend, as
   `TestLiveHealthFailureRollsBack` but with the option:
   * the install fails;
   * `RolledBack` and `ReplacedBeforeStop` are true;
   * the service runs the previous binary afterwards.
5. **Seen failing first,** each on its host, in a scratch copy with
   `early := false` planted in `managed.go`: `TestLiveReplaceBeforeStop`
   fails on "target held the new build at Stop".
6. **Runs, recorded:**
   * **the Linux test host:** system scope under `sudo`, and user scope,
     as `ci.yml`'s systemd step runs them;
   * **this Mac:** `SELFUPDATE_REQUIRE_LAUNCHD=1`, in this user's GUI
     domain;
   * **the Windows test host:** `SELFUPDATE_REQUIRE_SCM=1`, from an
     elevated shell. If the host's shell is not elevated, CI's run is the
     record, and this says so;
   * **CI:** every `TestLive` step, on the pushed S3 commit.
7. **Checks:** `make gate`, then commit.

### Phase S4: item 4, documentation and record notes

**Files:**
* `docs/guides/extending-selfupdate.md`
* `docs/guides/migrating-from-mcplib-selfupdate.md`
* `docs/architecture.md`
* `docs/decisions/0004-MADR-evolve-selfupdate-api-and-tui-support.md`
* `docs/decisions/0011-MADR-reference-service-lifecycles.md`
* `docs/reports/0016-REPORT-precheck-gofmt-errors.md`

1. **`extending-selfupdate.md`:**
   * **"Run as a service"** gains a subsection, `### Replace before the
     stop`. It covers `NewManagedInstallerWith` with the option, and the
     order. It covers what the option buys: a failed `Apply` or probe
     costs no downtime, and the swap is a few tens of milliseconds
     (MADR). And it gives the three hazards as conditions for using it:
     * the service never starts its own executable while it runs;
     * no `ExecStop=`-like hook runs it;
     * the consumer accepts that an unplanned restart runs the new
       binary before its health check.
   * **"Read JSON output"** (`:100`) says schema 4 since `v1.13.0`, and
     names `replaced_before_stop`.
   * **The handoff result example** (`:564`) shows `"schema_version":4`.
2. **The migration guide** gains `## 13. From v1.12 to v1.13`, with
   `### What is new`, `### Adopting it` and `### Check`, in §12's form:
   * the option, and that the default is unchanged;
   * schema 4, which only adds a key;
   * the pre-add check's gofmt step, for a program that copied the
     script.

   §2's "current release" and the `go get` lines move in S5's pin commit,
   not here.
3. **`docs/architecture.md`:**
   * the managed-installer bullet (`:149`) names `NewManagedInstallerWith`
     and `ManagedOptions`;
   * the `selfupdate/` row's file counts (`:111`) are recounted with
     `ls selfupdate/*.go | grep -v _test | wc -l` and the `_test` count,
     and the command output is recorded.
4. **`README.md` is not changed here.** `247a2b6`'s one README line was a
   package-table row for a new package, and this release adds no package.
   Its pins move in S5.
5. **Record notes,** each an annotation, not a rewrite:
   * **0004-MADR P3's open-work table:** the row "Replacing the binary
     before stopping the service" moves to the done list, "opt-in, in
     `v1.13.0`, under 0020".
   * **0011-MADR, Related:** the candidate gets "*(Decided 2026-10-09 by
     0020-MADR: opt-in, `ManagedOptions.ReplaceBeforeStop`.)*".
   * **0016-REPORT:** a dated closing line says it is decided by
     0020-MADR and fixed in S1. Its claim that `make release-check` also
     reaches the gap is corrected there: this repository has no such
     target. The finding above it stays as written.
6. **Checks:** links and identifiers on every changed file, markdownlint
   (`npx --yes markdownlint-cli2@0.23.2`) on the guides, then `make gate`.
   Commit.

### Phase S5: `v1.13.0`

0017-PLAN's "Release procedure" applies, with `vX = v1.13.0` and the
release commit S4's:

1. **CI** on `main` at S4's commit is green: every job, including S3's
   live tests. The commits must be pushed first, on the owner's ask.
2. **The live tests** are S2 step 12's and S3 step 6's records.
3. **The tag,** on the owner's ask in that turn:
   * `scripts/check-release-tag.sh v1.13.0`;
   * `git tag -a v1.13.0 -m v1.13.0 <commit>`;
   * the disclosure guard over the tag;
   * `git push origin v1.13.0`.
4. **The tag's state:** `ls-remote` gives the commit. The tag's CI passes
   all jobs, and the ten identity legs print `v1.13.0 (release) <12-hex>`.
5. **The pin commit,** whose candidates are
   `git grep -n 'v1\.12\.1\|e8116a2' -- README.md docs/architecture.md docs/guides`.
   Each changed line is named in the record. The lines that state what
   `v1.12.1` itself changed are kept.
6. **The proxy:** `go list -m -json …@v1.13.0` gives `Origin.Hash` the
   commit, and `@latest` resolves to `v1.13.0`.
7. **Release notes,** in this PLAN under `### Release notes for v1.13.0`.
8. **No installer rehearsal,** unless the owner asks for one.

### Phase S6: closing

1. `### Verification, at closing`: V1–V8, each with its evidence.
2. `### Closing`, `status: complete`, and the index row. Commit, then push
   on the owner's ask.

## Verification

* **V1 (item 1).** G1 and G3 failed against the old script and pass
  against the new one, and the test reports `0 failed`. On the real tool,
  P3 no longer prints `lstat`, and the unreadable-file case exits 1 with
  `gofmt: failed (exit 2)`.
* **V2 (item 1, CI).** The new CI line exits 2 on a scratch unparseable
  file under `bash -e`, and CI is green on S1's commit.
* **V3 (item 4, units).** Every S2 test failed against its plant and
  passes. The default-order test proves nothing changed without the
  option.
* **V4 (item 4, Windows).** `TestReplaceBeforeStopRunningImage` passed 30
  runs on the Windows test host and passes in CI's Windows job.
* **V5 (item 4, live).** `TestLiveReplaceBeforeStop` and its health-failure
  test pass on each backend, on the hosts S3 names and in CI.
* **V6 (API).** `make apicheck` is compatible with `v1.12.1`, and the
  apidiff report lists only the additions: `ManagedOptions`,
  `NewManagedInstallerWith`, `InstallResult.ReplacedBeforeStop`,
  `Result.ReplacedBeforeStop` and `ResultDocument.ReplacedBeforeStop`.
* **V7 (schema).** `resultDocumentSchema` is 4, `checkRecordSchema` is still
  3, and `checkcache_test.go` is unchanged since `v1.12.1`.
* **V8 (release).** `v1.13.0` is tagged at S4's commit (or a later
  release-shaped commit, recorded), its CI is green, the pins name it, and
  the proxy serves it.

## Rollout and Rollback

* **Rollout:** item 1 takes effect on its commit, for every agent commit
  and `make pre-add-check`. Item 4 reaches consumers in `v1.13.0`, and does
  nothing until a consumer sets the option.
* **Rollback, item 1:** `git revert` of S1's commit restores the old
  check.
* **Rollback, item 4, before the tag:** revert S2 to S4. After the tag, a
  release cannot be withdrawn, since the tag ruleset and the proxy keep
  it. A defect is fixed in `v1.13.1`. A consumer that hits one unsets the
  option, which restores today's flow without a downgrade.
* **A deviation in any phase** stops it, by rule 4.

## Execution record

### Approval (2026-10-09)

* The owner committed the MADR, this PLAN and the two index rows
  (`82b8463`), and approved execution: "i committed, proceed."

### S0 (2026-10-09)

* The records are `82b8463`, the owner's commit. This entry, the status
  `in-progress` and the index row follow in a records-only commit.

### S1 (2026-10-09)

1. **The script** (steps 1–3): the header names this record, and the usage
   paragraph says a deleted file is not formatted on either path. The
   no-list loop adds a file to gofmt's list only if it exists, and always
   adds its directory. The gofmt step keeps gofmt's standard error and
   exit status, and fails with `gofmt: failed (exit N):` and the message.
2. **The test** (step 4): the gofmt stub prints `$STUB_GOFMT_ERR` and exits
   `$STUB_GOFMT_RC`. Two cases are added, G1 and G3.
3. **Seen failing first** (step 5). `SCRIPT=<scratch>/old-precheck.sh`,
   which is `HEAD`'s script, made the test exit 1. Every earlier case
   reported `ok`, and the two new ones failed:

   ```text
     FAIL gofmt failure: rc=0 out=[pkg/a.go:1:1:broken
   go-precheck: 1 file(s) clean (gofmt, golangci-lint, go vet, go test, govulncheck).]
     FAIL deleted, no arguments: rc=0 out=[go-precheck: 2 file(s) clean (gofmt, golangci-lint, go vet, go test, govulncheck).] calls=[gofmt -l pkg/a.go pkg/b.go;go vet ./...;go test ./...;govulncheck ./...;]
   go-precheck_test: 11 passed, 2 failed
   ```

   Against the new script: `go-precheck_test: 13 passed, 0 failed`.
4. **CI's line** (step 6) is now `unformatted="$(gofmt -l .)"` and
   `test -z "$unformatted"`, with a comment. Under `bash -e`, on a scratch
   clone holding an unparseable file, the new form exited 2 and the old
   form exited 0.
5. **On the real tools** (step 7), on scratch clones with the new script
   and `GO_PRECHECK_SKIP_VULN=1`:

   | Case | Exit | Output that decides |
   | :--- | ---: | :--- |
   | P3: `buildinfo/stamp_test.go` deleted, no list | 0 | no `lstat` line (0 matches); `go-precheck: 294 file(s) clean (gofmt, golangci-lint, go vet, go test).` |
   | `buildinfo/buildinfo.go` made unreadable (`chmod 000`), no list | 1 | `gofmt: failed (exit 2):` / `open buildinfo/buildinfo.go: permission denied` |
   | P4: an unparseable file in a list | 1 | first lines: `gofmt: failed (exit 2):` / `buildinfo/zz_broken.go:3:9: expected ')', found '{'` |

### Deviation D1 (2026-10-09): the CLI's golden files hold schema 3

* **Found in S2:** after steps 1–8, `go test ./selfupdate/...` failed in
  `selfupdate/cli` only: `TestGolden` and `TestCommandGolden`. The other
  packages passed. Thirteen files under `selfupdate/cli/testdata/golden/`
  (`applied.json.stdout`, `failed.json.stdout` and eleven more) expect the
  result document with `"schema_version":3` and no `replaced_before_stop`
  key. S2's schema change caused it; it is not pre-existing. Step 8's list
  missed them. It also missed a second schema-3 expectation in
  `warnings_test.go`, the suffix at `:107` beside the `:102` it names.
  That file was already in scope, and its line is updated.
* **Decision (the owner):** "Regenerate and verify".
  * The files are rewritten by the package's own flag:
    `go test ./selfupdate/cli -run 'TestGolden|TestCommandGolden' -update`
    (`selfupdate/cli/helpers_test.go:16`).
  * Every changed line is checked to differ only by `schema_version` 3 →
    4 and an added `"replaced_before_stop":false`, with no other byte
    changed.
* **Scope:** the thirteen golden files join S2's files. The MADR is
  unchanged: it decided schema 4, and asserts nothing about these files.

### S2 (2026-10-09)

1. **The API** (steps 1–2):
   * `ManagedOptions{ReplaceBeforeStop bool}`, with the MADR's comment and
     its hazards;
   * `NewManagedInstallerWith`, which holds the constructor checks.
     `NewManagedInstallerFor` returns it with `ManagedOptions{}`;
   * `InstallResult.ReplacedBeforeStop` and `Result.ReplacedBeforeStop`.
2. **The flow** (step 3) is as written:
   * `early := running && s.opts.ReplaceBeforeStop`;
   * a `marked` closure sets the field on every result after the early
     `Apply`;
   * a failed early `Apply` reports it only when a replacement went live.
3. **`recoverStop`** (step 4) takes the applied replacement. A still-running
   service gets the binary rolled back and no restart. A service that went
   down gets the binary rolled back, then a start.
4. **The result** (steps 5–6): `updater.go` copies the field.
   `resultDocumentSchema = 4`, and `replaced_before_stop` follows
   `probes_skipped`.
5. **Docs** (step 7): `selfupdate/doc.go` names the option and the result
   field; `selfupdate/cli/doc.go` says schema 4.
6. **Schema 3 → 4** (step 8):
   * `example_test.go:319`, `events_test.go:370` and `:384`, and
     `warnings_test.go:102`, plus `:107`;
   * the thirteen CLI golden files (Deviation D1).

   The D1 check printed
   `13 files, 13 changed lines, each only schema 3->4 and the new key`. It
   failed, as it must, on a scratch copy with `"applied":true` planted as
   `false` in one golden. `checkcache_test.go` is not in the diff.
7. **New tests** (steps 9–10):
   * `selfupdate/managed_replacefirst_test.go` holds the twelve named tests.
     The plan's `orderLife` is named `firstLife` (and `enabledFirstLife`):
     `twophase_test.go:15` already declares `orderLife`.
     `TestReplaceBeforeStopApplyRefused` also requires that the error
     names the journal.
   * `TestRunReportsReplacedBeforeStop` and `ExampleNewManagedInstallerWith`
     are added.
   * `TestReplaceBeforeStopRunningImage`, with a `helperService`, is in
     `lifecycle_windows_test.go`.
   * All pass: `go test ./selfupdate/ -run
     'ReplaceBeforeStop|ManagedDefaultOrder|NewManagedInstallerWith'` gave
     12 `PASS` lines and `ok`.
8. **Seen failing** (step 11). Each plant was made with
   `scripts/plant-copy.sh`, and each named test failed in the copy (`go
   test` rc=1):

   | Plant | Failing test, line |
   | :--- | :--- |
   | `early := false` | `TestReplaceBeforeStopOrder` (`:120`), `TestReplaceBeforeStopProbeFails` (`:192`), `TestRunReportsReplacedBeforeStop` (`managed_started_test.go:148`) |
   | still-running `recoverStop` without the rollback | `TestReplaceBeforeStopStopFailsStillRunning` (`:228`): `RolledBack:false` |
   | early-`Apply` failure with `restart` true | `TestReplaceBeforeStopProbeFails` (`:195`): `lifecycle [start health], running true; want the service untouched` |
   | `document.go` mapping removed | `TestRunReportsReplacedBeforeStop` (`managed_started_test.go:155`): the document |
   | `updater.go` copy removed | `TestRunReportsReplacedBeforeStop` (`:148`) |
   | `NewManagedInstallerFor` passing `ReplaceBeforeStop: true` | `TestManagedDefaultOrderUnchanged` (`:155`) |
   | `early := false`, on the Windows test host | `TestReplaceBeforeStopRunningImage` (`lifecycle_windows_test.go:140`): `ReplacedBeforeStop:false` |

   The Windows plant failed on the result check, which runs before the
   "target at Stop" check that the plan expected to fail.
9. **Checks** (step 12):
   * `CGO_ENABLED=0 GOOS=windows go vet ./selfupdate/` is clean;
   * `TestReplaceBeforeStopRunningImage` passed `-test.count=30` twice on
     the Windows test host: 30 `--- PASS` lines, then `PASS`;
   * `make pre-add-check` on the twelve Go files:
     `go-precheck: 12 file(s) clean (gofmt, golangci-lint, go vet, go test, govulncheck).`;
   * `make gate`: `overall=0`, with `race`, `shuffle` and `crossvet`
     rc=0, and `apicheck` `compatible with v1.12.1`.
   * The full apidiff report against `v1.12.1`, `Compatible changes:` only:
     * `InstallResult.ReplacedBeforeStop`: added;
     * `ManagedOptions`: added;
     * `NewManagedInstallerWith`: added;
     * `Result.ReplacedBeforeStop`: added;
     * `ResultDocument.ReplacedBeforeStop`: added.

### S3 (2026-10-09)

1. **Helpers** (steps 1–2). In each of the three live files, `managedInstall`
   now calls `managedInstallWith(life, rec, target, newPath, opts)`, with
   `ManagedOptions{}`, so its callers are unchanged. Each file also gains
   two helpers:
   * `stopCheck`, which embeds the backend (`*Unit`, `*Job`, `*Service`).
     At the first `Stop` it records whether the target held the new
     build's bytes, and the backend's `Running`;
   * `replaceFirst`, which installs with `ReplaceBeforeStop`.
2. **Tests** (steps 3–4): `TestLiveReplaceBeforeStop` and
   `TestLiveReplaceBeforeStopHealthFailure` per backend.
   * The health-failure tests also require the target to equal the
     previous binary's bytes afterwards.
   * The SCM one also requires the failure to be the start's, as
     `TestLiveHealthFailureRollsBack` does.
   * `go vet` is clean for linux, darwin and windows.
3. **Runs** (step 6), every `^TestLive` test in each package:

   | Host | Scope | Result |
   | :--- | :--- | :--- |
   | this Mac | launchd, this user's GUI domain | 10 `PASS`, among them `TestLiveReplaceBeforeStop` (10.59s) and `...HealthFailure` (25.71s); `ok` |
   | the Linux test host, systemd 259 (259.5-0ubuntu3.4) | system, under `sudo` | 9 `PASS`; `TestLiveReplaceBeforeStop` (2.85s), `...HealthFailure` (2.79s) |
   | the same host | user | 9 `PASS`; (2.15s), (1.98s) |
   | the Windows test host, an administrator shell (`net session` exit 0) | SCM | 8 `PASS`; (5.37s), (5.39s) |

   CI's run on the pushed commit is recorded with S5 step 1.
4. **Seen failing** (step 5), in scratch copies:
   * **`early := false`.** Both new tests failed on every host, first on
     the result check, `ReplacedBeforeStop` false:
     `live_darwin_test.go:631`, `:665`; `live_linux_test.go:496`, `:529`;
     `live_windows_test.go:575`, `:605`.
   * **The at-Stop check** runs after the result check, so a second plant
     was needed to see it fail:
     * the early branch was made `if false {` and the late one
       `if true {`. That is the stop-first order, with
       `ReplacedBeforeStop` still reported;
     * `TestLiveReplaceBeforeStop` then failed on that check on each host:
       * `live_darwin_test.go:634: at Stop: target held the new build false, job running true; want both`;
       * `live_linux_test.go:499: … unit running true; want both`;
       * `live_windows_test.go:578: … service running true; want both`.

### S3, a lint failure committed (2026-10-09)

* **What happened.** S3's commit, `1efe00b`, was made although its checks
  had failed.
  * `make pre-add-check` exited 2, and `make gate` ended `overall=1` on
    `lint`.
  * The agent's command chained the commit after the checks with `;`, not
    `&&`, so the failure did not stop it.
  * The machine-wide commit gate saw no staged Go file when it ran, before
    the command staged them.
  * This broke Rules item 2. The commit was not pushed.
* **The finding,** for linux, darwin and windows alike:
  `QF1008: could remove embedded field "Unit" from selector (staticcheck)`
  at `c.Unit.Running(ctx, product)`. The same appears for `Job` and
  `Service` in the other two files.
* **The fix,** a new commit with no amend and no rewrite:
  * the three lines call the promoted `c.Running(ctx, product)`, which is
    the same method, so the runs recorded above are unchanged in what they
    test;
  * `c.<Backend>.Stop` stays: `stopCheck` overrides `Stop`.

  `make pre-add-check` on the three files and `make gate` were run again,
  and the commit was chained on their success.

### S4 (2026-10-09)

1. **`extending-selfupdate.md`:**
   * "Read JSON output" says schema 4 since `v1.13.0`, names
     `replaced_before_stop`, and adds that each version only adds keys;
   * "Run as a service" gains `### Replace before the stop`: the call,
     what it buys, the three conditions, recovery, and the result field;
   * the handoff result example shows `"schema_version":4`.
2. **The migration guide** gains `## 13. From v1.12 to v1.13`, with its
   own `go get …@v1.13.0` line: `### What is new`, `### Adopting it` (the
   option's conditions, schema 4, the copied pre-add script) and
   `### Check`.
3. **`docs/architecture.md`:**
   * the installers bullet names `NewManagedInstallerWith` and
     `ManagedOptions`;
   * the `selfupdate/` row's test files are 81. That is
     `ls selfupdate/*_test.go | wc -l`, and the non-test count stays 45,
     from `ls selfupdate/*.go | grep -v _test | wc -l`. `selfupdate/cli`'s
     53 golden files are unchanged in number.
4. **`README.md`** is not changed (step 4).
5. **Record notes:**
   * **0004-MADR P3:** the row leaves the open-work table, and the done
     list names it under 0020, as `247a2b6` did for 0017's items;
   * **0011-MADR, Related:** an italic dated note follows the candidate;
   * **0016-REPORT:** a `## Decided (2026-10-09)` section, with the
     `make release-check` correction.
6. **Checks:**
   * links: "103 links in 6 files, 0 broken", the new
     `#replace-before-the-stop` anchor included;
   * identifiers: 0 findings;
   * markdownlint: "0 issues".

### S5, `v1.13.0` (2026-10-09)

* **The release commit** is `5e199c831b5691ea687943e3c3fd495d50c739ed`, S4's.
* **Step 1:** CI run 37976906986 on `main` at `5e199c8` passed all 17
  jobs. Its live steps ran the new tests:
  * launchd, `validate (macos-15)`: `TestLiveReplaceBeforeStop` (10.56s)
    and `TestLiveReplaceBeforeStopHealthFailure` (25.61s);
  * systemd, `validate (ubuntu-24.04)`: system scope (1.78s, 1.75s) and
    user scope (1.48s, 1.46s);
  * SCM, `validate (windows-2025)`: (4.34s, 4.27s).
* **Step 2:** the live records are S2 step 12's and S3's.
* **Step 3, the tag,** was made and pushed by the owner. Verified:
  * `v1.13.0` is an annotated tag (`git cat-file -t` gives `tag`), object
    `0a121fbc2001f7013cac58f65399ab72fef6f47d`, peeled to `5e199c8`;
  * `scripts/check-release-tag.sh v1.13.0` exits 0.
* **Step 4:**
  * `git ls-remote origin 'refs/tags/v1.13.0^{}'` gives
    `5e199c831b5691ea687943e3c3fd495d50c739ed`, and `refs/heads/main`
    gives the same commit;
  * the tag's CI run, 37979214344, passed all 17 jobs; all ten identity
    legs print `v1.13.0 (release) 5e199c831b56`.
* **Step 6:**
  * `GOPROXY=https://proxy.golang.org GOFLAGS=-mod=mod go list -m -json
    github.com/maccavelli/go-selfupdate-lib@v1.13.0` gives `v1.13.0`,
    `Time` `2026-10-09T18:41:40Z`, `Origin.Hash` the release commit and
    `Ref` `refs/tags/v1.13.0`;
  * `@latest` resolves to `v1.13.0`.
* **Step 5, the pin commit.**
  * **Candidates:** 39 lines, from
    `git grep -n 'v1\.12\.1\|e8116a2' -- README.md docs/architecture.md docs/guides`.
  * **Preconditions:** `git diff v1.12.1 v1.13.0` of both release
    workflows and the installer templates is empty; only `ci.yml`
    changed.
  * **Changed,** 31 lines, to `5e199c83… # v1.13.0` or `v1.13.0`:
    * `README.md:24` (the current release), `:28` (`go get`), `:126` (the
      publish pin);
    * `docs/architecture.md:23-24`, the current release and its commit;
    * the building guide: `:26` ("the examples below pin"), `:161`,
      `:172` and `:250` (the workflow pins), `:183` (the `ls-remote`
      example);
    * the migration guide:
      * `:23` (`go get`) and `:26` (current release);
      * `:39` gains "`v1.13.0` only adds to the API: see [13. …]";
      * `:90` (§3's pin), `:108` ("`v1.11.0` to `v1.13.0` change
        nothing in it"), `:110` ("the example pins") and `:113`
        (`ls-remote`);
      * `:159` (§5's `go.mod`);
      * seven `go get` lines (`:378`, `:465`, `:509`, `:561`, `:606`,
        `:710`, `:821`);
      * six `go list` checks (`:504`, `:556`, `:600`, `:702`, `:815`,
        `:906`).
  * **Kept,** 9 lines that state what `v1.12.1` itself is or changed:
    * the building guide `:426` ("since `v1.12.1`");
    * the migration guide `:38-39` (the summary of `v1.12.1`);
    * its section `### From v1.12.0 to v1.12.1`, with its own `go get`,
      its text, its pin advice and its check;
    * §13's "compatible with `v1.12.1`".
* **Step 8:** the live installer rehearsal was not asked for, and was not
  run.

### Release notes for v1.13.0 (2026-10-09)

`v1.13.0` adds an opt-in order for managed updates. `make apicheck` reports
it compatible with `v1.12.1`.

* **`ManagedOptions.ReplaceBeforeStop`,** through
  `NewManagedInstallerWith`: a running service's binary is replaced while
  the old instance still runs, then the service is stopped, reconciled,
  started and checked.
  * A replacement or post-install probe that fails then costs no
    downtime.
  * A failed stop rolls the binary back too.
  * The default order is unchanged.
  * Use it only for a service that never starts its own executable while
    it runs, and whose stop hooks do not run it. The extending guide's
    "Replace before the stop" has the conditions.
* **The JSON result is schema 4:** `replaced_before_stop`, with
  `Result.ReplacedBeforeStop` and `InstallResult.ReplacedBeforeStop`. The
  key is the only change.
* **Tooling, not in the API:**
  * the pre-add check fails when gofmt itself fails;
  * the no-list path no longer hands gofmt a tracked file the work tree
    lacks;
  * CI's gofmt line fails on its own.
* **Records:** this PLAN and its MADR. MADR 0011's replace-before-stop
  candidate, and report 0016, are decided.

### Verification, at closing (2026-10-09)

* **V1 (item 1).** G1 and G3 failed against the old script and pass
  against the new one: `13 passed, 0 failed`. On the real tool, P3 prints
  no `lstat`, and the unreadable file exits 1 with
  `gofmt: failed (exit 2)` (S1).
* **V2 (item 1, CI): holds with a note.** The new CI line exits 2 on a
  scratch unparseable file under `bash -e` (S1). No CI run exists for
  `2870b54` alone: it was pushed with the later commits in one push, and
  `gh run list --commit 2870b54` lists none. Run 37976906986 at `5e199c8`,
  which holds it, passed all 17 jobs, `vet, gofmt, tidy, lint` included.
* **V3 (item 4, units).** Every S2 test failed against its plant and
  passes. `TestManagedDefaultOrderUnchanged` failed when the default was
  planted on, and passes (S2).
* **V4 (item 4, Windows).** `TestReplaceBeforeStopRunningImage` passed
  `-test.count=30` twice on the Windows test host (S2). It has no skip,
  and CI's `validate (windows-2025)` ran it in `go test ./...`:
  `ok  github.com/maccavelli/go-selfupdate-lib/selfupdate  30.775s`.
* **V5 (item 4, live).** `TestLiveReplaceBeforeStop` and its
  health-failure test pass:
  * on this Mac (launchd);
  * on the Linux test host (systemd 259, system and user scope);
  * on the Windows test host (SCM);
  * in CI on all three runners (S3, S5).

  Each failed against `early := false`, and the at-Stop check failed
  against the second plant, on every host.
* **V6 (API).** The full apidiff report against `v1.12.1` lists only the
  five additions (S2), and the tag's CI `apicheck` passed.
* **V7 (schema).** `selfupdate/document.go:7` has
  `resultDocumentSchema = 4`, and `selfupdate/checkcache.go:303` has
  `checkRecordSchema = 3`. `git diff --stat v1.12.1 HEAD --
  selfupdate/checkcache_test.go` is empty. (`checkcache.go` itself changed
  since `v1.12.1` only in
  [0019-PLAN-apply-go-fix-modernizers.md](0019-PLAN-apply-go-fix-modernizers.md)'s
  `errors.AsType` rewrite.)
* **V8 (release).** `v1.13.0` is the annotated tag on `5e199c8`, S4's
  commit. Its CI passed with ten identity legs, the pins name it
  (`2db85b4`), and the proxy serves it as `@latest` (S5).

### Closing (2026-10-09)

* V1–V8 hold, V2 with its note. There was one deviation, D1, and one
  breach of Rules item 2: S3's lint failure was committed, then fixed in
  `6f56a04`. Both are recorded above.
* This PLAN is `complete`, and its row in `docs/README.md` follows. The
  MADR stays `accepted`.
* **Not done, as scoped:**
  * the default order is unchanged (MADR 4C);
  * `Apply` is not split (4D);
  * the hazards are documented, not detected;
  * go-tui-lib's and other repositories' copies of the pre-add script are
    theirs to change. The migration guide's §13 tells a program that
    copied it.
