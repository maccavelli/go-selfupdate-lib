---
status: complete
date: 2026-09-30
associated-madr: "0003-MADR-remediate-debugging-pass-findings.md"
---
# Implement the fixes for every debugging-pass finding

Associated MADR: [0003-MADR-remediate-debugging-pass-findings.md](0003-MADR-remediate-debugging-pass-findings.md)

## Goal

Every finding in the MADR's tables is closed:

* all 41 findings: A1, A3, A5–A8, A10, A11 (A2 is C1, A9 is C9); B1–B8
  and B10; C1–C12 and C14; D1–D9, D11 and D12. Also B11, added by
  deviation D1, for 42;
* the four test-gap groups A4, B9, C13 and D10.

Each fix has evidence in the execution record, and `v1.0.0` can then be
tagged under
[0002-PLAN-rehome-selfupdate-from-mcplib.md](0002-PLAN-rehome-selfupdate-from-mcplib.md)
Phase 6.

## Scope

### Finding → phase

| Phase | Area | Findings |
|---|---|---|
| 0 | records | D2, D12, and 0002's amendment |
| 0b | Windows running-image replace (deviation D1) | B11 |
| 1 | coordinator and confirmer | C1–C14 (C9 = A9), C13 |
| 2 | network and integrity | A1 with A5, A2 (= C1 at source), A3, A6, A7, A8, A10, A11, A4 |
| 3 | install path, portable | B1, B2, B3, B4, B5, B8, B10 (directory identity), B9 |
| 4 | install path, Windows | B6, B7, B10 (Windows retry and `ctx`) |
| 5 | release tooling | D1, D4, D5, D6, D7, D8, D10 |
| 6 | CI and tooling | D9, D11 |
| 7 | docs and close-out | D3, plus README and architecture for behaviour changes |

### Out of scope

* Any exported API change. No exported identifier is added, removed or
  re-typed (MADR "What B means"). New seams are unexported.
* `mcplib` and the consumers.
* The GitHub App check suite noted in D11. That is a GitHub installation
  setting for the owner, not code.
* `git push` and tags. Those stay under 0002-PLAN Phase 6 and need the
  owner's ask.

### Fixed inputs

* **The development host:** macOS, go1.27.1, `golangci-lint` 2.13.2,
  `govulncheck` 1.7.0, `shellcheck`, `markdownlint-cli2` 0.23.2, `python3`.
  Cross-OS commands set `CGO_ENABLED=0`.
* **The Windows test host,** reached over SSH and never named in a record:
  * Windows 10.0.26200 (Windows 11), amd64;
  * go1.27.1 windows/amd64 with `CGO_ENABLED=1`, and MinGW-w64 gcc 16.2,
    so `-race` works;
  * a Git Bash (MSYS2) login shell, PowerShell 7.6;
  * Developer Mode on, so unprivileged symlinks can be created.

  Measured on 2026-09-29, read-only.
* **Using the Windows host.**
  * A `git archive` of the commit under test is extracted into a new
    directory under the host's `%TEMP%`, named `go-core-lib-verify-<short
    sha>`. The directory is removed after the run.
  * Nothing else on the host is written, apart from Go's build and module
    caches.
  * Outputs are copied back into the development host's scratch space.
    Records quote them with the user-profile path replaced by `<user>`.

### Rules for every phase

* **Fail first.** For each behaviour fix, the new or changed test is copied
  into a scratch `git archive` of the phase's starting commit. It must fail
  there, and the failure line is recorded. It then passes in the tree.
* **Test-gap items** are proven by mutation on a scratch copy: remove the
  check the test covers, and the test must fail.
* **Gates before each commit:** `make pre-add-check` (three-target lint),
  `go test -race -count=1 ./...`, `go mod tidy -diff`, `shellcheck
  scripts/*.sh`, and `markdownlint-cli2` on touched Markdown. The last
  commit runs every gate.
* **The Windows gate,** for every phase that touches Go code or scripts
  (1–6): on the Windows host, `go vet ./...`,
  `go test -race -count=1 ./...`, and under its bash
  `./scripts/refuse-existing-release_test.sh` and
  `./scripts/check-workflow-gh-repo.sh`, plus from Phase 5 on the new
  script tests. This runs on the phase's working tree, before the commit.
  The tree is shipped as
  `git ls-files -co --exclude-standard | tar -cf - -T -`, which covers
  tracked and new, non-ignored files and never commits. A Windows fail-first run is also required for each
  test that exists only on Windows.
* **G-api** (normalised `go doc -all` against `mcplib` `v1.6.0`) is re-run
  each phase. It may differ only in doc comments.
* `git commit --no-edit`, one commit per phase. A phase too large to review
  may be split into lettered sub-phases (1a, 1b), each committed, and the
  split is recorded.

## Implementation Steps

### Phase 0: records

1. Owner approval. Set this PLAN `in-progress`, and index it in
   `docs/README.md`.
   * **Windows baseline:** run the Windows gate on `004d13b`, the code as
     pushed, and record which tests ran. That includes the two existing
     `//go:build windows` receipt tests, listed with `go test -v -run
     Windows`. This is the first Windows run outside CI, and every later
     Windows result is compared to it.
2. **D12.**
   * Replace the stale `docs/README.md` row "know why CI and `make lint`
     fail until the first package lands".
   * In 0002-MADR "Consequences", strike "the first commit names it", and
     annotate it with a pointer to the §2 amendment.
3. **D2.** In 0002-MADR, strike and annotate "one deleted line in one
   consumer", "`magic-cli-remote` also deletes its `bridge-release` line",
   and option A's "Bad" line. All six consumers delete `bridge-release`.
4. **0002 amendment.** Add a dated amendment to 0002-MADR: `v1.0.0` =
   `mcplib` `v1.6.0` + §3 + the 0003 fixes, and G-diff's baseline grows
   accordingly. Add a dated entry to 0002-PLAN Phase 6: steps 3–5 wait for
   this PLAN to be `complete`.
5. Commit the records.

### Phase 0b: Windows running-image replace (B11; added by deviation D1)

This phase runs first, because the Windows gate cannot pass until B11 is
fixed.

1. In `moveFileReplace`, retry while `isBusyRunningImage(last)` (a
   sharing violation **or** access denied), within the existing
   `DefaultLockTimeout` deadline. Taking the caller's `ctx` is B10's change
   and stays in Phase 4.
2. Tighten `TestNativeReplaceRunningCopy`: a nil `Install` error must leave
   exactly the new bytes at the target. The old-bytes allowance stays only
   for the pending-backup path.
3. **Proof on the Windows host.**
   * The tightened test fails at least once in 10 runs on the unchanged
     code.
   * With the fix, `-race` included, it passes **20 of 20** runs.
   * The full Windows gate passes.
4. On this host: `CGO_ENABLED=0 GOOS=windows go vet` and `go test -c`,
   three-target lint, and `go test -race`.
5. Commit.

### Phase 1: coordinator and confirmer (`updater.go`, `confirmer.go`, `types.go`, `example_test.go`)

Each item lists the change, then its test.

1. **C3.** `New` rejects a nil or typed-nil value in every required seam
   and in each `Verifiers` element, using an unexported `isNil` helper
   (`reflect` for typed nils).
   * `TestNewRejectsNilCollaborators`, rewritten as a table: each seam nil
     in turn, a typed-nil seam, a nil verifier, a typed-nil verifier, and
     zero `Limits`.
2. **C1, coordinator side.** In `fetchRelease`, when `TargetVersion` is set
   and `rel.Tag != TargetVersion`, return an error wrapping `ErrIntegrity`
   that names both tags with `%q`.
   * `TestRunRejectsTagMismatch`.
3. **C4.** Check order becomes PLAN §4.6 step 4's: immutable, then
   draft/prerelease, then `Validate`. Every error that carries a tag uses
   `%q`.
   * `TestRunEscapesUntrustedTag`: the error contains no ESC, BEL or
     newline.
4. **C9.** Parse `SHA256SUMS` and resolve the selected entry right after
   the manifest download, before `CreateStaging`.
   * `TestRunRejectsManifestBeforeStaging`: `CreateStaging` and the binary
     `OpenAsset` are never called.
5. **C8.** Emit `EventVerified` after the verifiers, then
   `EventTransforming`, then transform and revalidate.
   * `TestRunEventOrderWithTransformer`: the exact event sequence. It also
     covers `hashFile`, `sessOwns` and `hashAndValidateStaging`,
     including the size-limit branch.
6. **C2.** When `Install` returns `Applied:false` and a nil error, return
   an error saying the installer reported no commit.
   * `TestRunInstallerNoCommitIsError`: non-nil error, `ExitCode` 1.
7. **C14.** After a committed `Install`, call `sess.Close()` explicitly and
   join its error, *then* report `EventComplete`. The deferred `Close`
   stays as a safety net (it is idempotent, `session.go:126-128`).
   * `TestRunCompleteAfterClose`: ordering.
   * `TestRunCloseErrorJoinedAfterCommit`: `Applied` true, error joined.
8. **C10.** Every error return in `execute` and `apply` goes through
   `wrapRun`, including validation, reporter and post-commit errors.
   * `TestRunErrorsNameProduct`: a table over each return site; the message
     is prefixed `selfupdate: demo:` and `errors.Is` still holds.
9. **C5.** In check mode with `OperationReplaceLocal`, `EventSelected`
   carries `Detail` "local build: apply requires --force".
   * `TestRunLocalBuildCheckHintsForce`.
10. **C12.**
    * The pending-backup `Detail` names the full retained path
      (`filepath.Join(target.Dir, pending)`) and says it is validated
      before the next download.
    * `ExampleNewManagedInstaller` composes a real `NewManagedInstaller`
      with example `Lifecycle` and `Reconciler` types.
    * A new `ExampleExitCode`.
    * `TestRunPendingBackupDetail`; the examples' `// Output:` lines.
11. **C6.** EOF before any input is a decline, `(false, nil)`.
    * `TestTerminalConfirmerEOFDeclines`.
12. **C7.** `terminalConfirmer` starts a single reader goroutine, once, and
    delivers lines through a channel. A cancelled `Confirm` leaves the
    pending line for the next `Confirm` instead of losing it.
    * `TestTerminalConfirmerCancelKeepsLine`: cancel, write `y\n`, and a
      second `Confirm` returns true.
13. **C11.** The `Config.Limits` doc comment states that the Updater uses
    `Manifest` and `Executable`, and that `ReleaseJSON` and `ErrorBody`
    belong to the source's own `Limits`. This is a doc fix; there is no
    test.
14. **C13.**
    * `TestRunCallOrderFailureBoundaries`: a table injecting a failure at
      every collaborator call; no later call happens.
    * `TestRunOperationsMatrix`: rollback, reinstall with `--force`, local
      build with and without `--force`, latest-older-is-error, and
      `--version`, all through `Run`.
    * `TestOverlappingRun` rewritten to use a channel handshake instead of
      `time.Sleep`, and to assert that no collaborator is invoked by the
      rejected `Run`.
15. Gates, then commit.

### Phase 2: network and integrity (`github.go`, `download.go`, `reporter.go`, `updater.go`)

1. **A1 with A5**, in one commit so that validation is never absent.
   * `mapRelease` checks only structure for every asset: `ID > 0`, a
     non-empty basename, no control characters.
   * `execute` validates the *selected* binary against
     `Limits.Executable`, and the manifest against `Limits.Manifest`, with
     `validateAssetMetadata` (state, size, digest syntax). This happens
     before classification and check mode.
   * Tests: `TestGitHubExtraAssetsDoNotPoisonRelease` (zero-size, `open`,
     oversize and `sha512:` extras; `Latest` succeeds), and
     `TestRunValidatesSelectedAssets` (an `open` binary or an oversize
     manifest fails before any check result).
2. **A2.** `ByTag` rejects a response whose `tag_name` differs from the
   request.
   * `TestGitHubByTagRejectsMismatchedTag`.
3. **A3.** `checkRedirect` refuses a redirect whose scheme is not `https`
   unless the host is loopback.
   * `TestGitHubRedirectRequiresHTTPS`.
4. **A6.** After `Decode`, `dec.Token()` must return `io.EOF`.
   * `TestDecodeJSONRejectsTrailingDelimiters`.
5. **A7.** A `Retry-After` in seconds above
   `math.MaxInt64 / int64(time.Second)` is treated as malformed (zero).
   * `TestRateLimitRetryAfterOverflow`.
6. **A8.** In `getRelease`, and in `OpenAsset` where it reads an error
   body, check the status first. For a non-2xx response, read at most
   `ErrorBody` bytes, tolerate truncation, and map the status.
   * `TestGitHubLargeErrorBodyKeepsRateLimit`.
7. **A10.** `validateGitHubName` rejects `.` and `..`.
   * `TestNewGitHubSourceRejectsDotNames`.
8. **A11.** `sanitizeDiagnostic` and `sanitizeText` replace Unicode
   format controls (`unicode.Cf`) and U+2028/U+2029.
   * `TestSanitizeRemovesFormatControls`.
9. **A4.** New `httptest` cases:
   * 403 with `X-RateLimit-Remaining: 0` and no `Retry-After`;
   * `OpenAsset` 404 and 429;
   * an asset in `open` state; oversize and zero-size selected assets;
   * a foreign asset ID;
   * `RateLimitError.Error` text.

   Mutation proof: each of the five checks A4 names, removed on a scratch
   copy, makes a test fail.
10. Gates, then commit.

### Phase 3: install path, portable (`replace*.go`, `session.go`, `managed.go`, `lock_*.go`, `target.go`)

1. **B1.** When the rollback rename fails, `replaceTarget` (unix and
   windows) returns the backup and
   `errors.Join(syncErr, fmt.Errorf("selfupdate: restore backup: %w", rerr))`.
   * `installSession.Install` returns
     `InstallResult{Target, Backup}` with that error.
   * `managedSession.Install` passes the returned `applied` to `recover`,
     so recovery can retry the rollback.
   * Tests: `TestInstallSyncAndRollbackFailureReported`, and
     `TestManagedApplyFailureRetriesRollback` (through `replacePath` and
     `syncDirFn` seams).
2. **B2.** `recover` runs on
   `context.WithTimeout(context.WithoutCancel(ctx), recoveryTimeout)`, with
   an unexported `recoveryTimeout = 2 * time.Minute`.
   * `TestManagedRecoveryIgnoresCancelledContext`: `Start` and
     `WaitHealthy` see a live context.
3. **B3.** `copyFile` chmods the backup to the source's mode before `Sync`.
   * `TestBackupCopyFallbackPreservesMode`: `osLink` is forced to fail; the
     mode is kept after rollback.
4. **B4.** `applyResult` gains an unexported `renamed bool`.
   `Install` deregisters staging only when the rename happened, so `Close`
   removes staging after an earlier failure.
   * `TestInstallFailureBeforeRenameRemovesStagingOnClose`.
5. **B5.** An unexported `openLockFile(root, name)`, shared by
   `lock_unix.go` and `lock_windows.go`:
   * `root.Lstat` first, rejecting anything not regular (a symlink
     included);
   * when absent, create with `O_CREATE|O_EXCL` (which `os.Root` never
     follows);
   * after opening, require `os.SameFile` between the pre-open `Lstat` and
     `f.Stat()`.

   Tests: `TestLockRejectsRelativeSymlink` and
   `TestLockRejectsDanglingRelativeSymlink`. They skip only on Windows, and
   only when symlink creation is not permitted, with the reason logged.
6. **B10, directory identity.** `Begin` records `root.Stat(".")`. Before
   `replaceTarget` and before commit, `Install` requires `os.SameFile`
   with `os.Stat(target.Dir)`, else `ErrConcurrentUpdate`.
   * `TestInstallDetectsSwappedDirectory`.
   * This is the chosen fix for B10's first half. Re-routing every
     operation through `os.Root` is not done, because Windows `MoveFileEx`
     is path-based. The identity check closes the swap the finding
     describes on both platforms.
7. **B8.** `TestManagedRollbackErrorJoined` asserts `errors.Is(err,
   rec.restoreE)` unconditionally. Mutation proof: removing `recov` from
   the join now fails it.
8. **B9.** New tests, with unexported seams added only where a failure has
   none:
   * `TestStagingRejectsPlantedSymlink`;
   * `TestInstallInjectedFailures`: a table of chmod, backup, rename,
     dir-sync and commit failures; the target stays intact or the error is
     reported, as each case requires;
   * `TestInstallPermissionDenied` (unix, a read-only directory);
   * `TestInstallCancelledBeforeAndAfterStaging`;
   * a source body shorter than its declared size, or failing mid-stream,
     leaves no staging file after `Close`;
   * `TestManagedFailureMatrix`: `fakeLife` gains stop, start, installed
     and running error injection; stop, start, restore and restart
     failures are each joined, and recovery calls are counted.

   Windows receipt cases are in Phase 4.
9. Gates, then commit.

### Phase 4: install path, Windows (`cleanup.go`, `cleanup_windows.go`, `replace_windows.go`)

1. **B6.** A portable `validateReceiptBackup(target Target, name string)`
   in `cleanup.go` requires:
   * `filepath.IsLocal(name)`;
   * `filepath.Base(name) == name`;
   * the prefix `"." + target.Base + ".selfupdate-bak-"`;
   * `name != target.Base`.

   `processCleanupReceipt` uses it, with `root.Lstat` and `root.Remove`.
   * `TestValidateReceiptBackup` (portable): a table of the target, the
     lock, an absolute path, traversal, and a good name.
2. **B7.** When the named backup does not exist, remove the receipt, sync,
   and return nil. A digest mismatch, a reparse point or a failed delete
   still fail closed.
   * `TestWindowsCleanupReceiptMissingBackup` (Windows).
3. **B10, Windows.** `moveFileReplace(ctx, …)` stops retrying when `ctx` is
   done, through an unexported `moveFileExFn` seam.
   * `TestMoveFileReplaceHonoursContext` (Windows).
4. Also for Windows: `TestWindowsCleanupReceiptMalformed`,
   `TestWindowsCleanupReceiptReparseBackup` and
   `TestWindowsReceiptConsumedByBegin`.
5. **Checks on this host:** `CGO_ENABLED=0 GOOS=windows go vet` and
   `go test -c`, plus the three-target lint. ~~The Windows tests' first run
   is the next push (0002-PLAN Phase 6). Until it passes, this phase is
   recorded as "compiled and linted, Windows run pending", and `v1.0.0`
   waits.~~ *Amended 2026-09-29, before approval (Windows host provided):*
   * Every Windows-only test in this phase fails first on the Windows host
     against the phase's starting commit, then passes against the phase's
     working tree, with `-race`.
   * `TestLockRejectsRelativeSymlink` and
     `TestLockRejectsDanglingRelativeSymlink` (Phase 3) run there without
     skipping, because Developer Mode permits symlinks. A skip there is a
     failure of this step.
   * The Windows CI leg at the next push confirms, and `v1.0.0` waits for
     it.
6. Gates, then commit.

### Phase 5: release tooling (`scripts/`, `publish-selfupdate-release.yml`)

1. **D1 with D10: shared parity fixtures.**
   * Fixtures live in `selfupdate/testdata/manifest-parity/<case>/`, each
     with `SHA256SUMS` and `expect` (`accept` or `reject`). Cases:
     * LF, CRLF, `*` marker, uppercase hex, blank lines and comments;
     * lone CR, vertical tab, form feed, a 5000-byte line, a BOM;
     * a duplicate name, traversal, empty.
   * `TestManifestParityFixtures` (Go) asserts `expect` for each.
   * `verify-selfupdate-release_test.sh` builds a staging set from each
     fixture and asserts the same outcome.
   * The verifier's parser changes to: split on `\n` only, strip one
     trailing `\r`, reject any other `\r`, `\v`, `\f`, `\x1c`–`\x1f`,
     U+0085, U+2028 or U+2029, and reject a line over 4096 bytes.
   * Also added to the verifier test: a missing `.exe`, empty platforms, a
     symlink, a directory and unsafe extra names.
2. **D5.**
   * Extras must match `^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`. All six
     consumers' current extras match; this is asserted in the execution
     record.
   * The upload step builds an array from
     `find staging -type f -print0 | sort -z` and passes
     `"${files[@]}"`.
3. **D6.** The verifier rejects any staged entry that is a symlink or not
   a regular file (`os.lstat`, `stat.S_ISREG`).
4. **D7.**
   * `refuse-existing-release.sh` requires a non-empty `GH_REPO`, and
     first runs `"$GH" repo view "$GH_REPO" --json name`. A failure
     refuses.
   * `refuse-existing-release_test.sh`: the stub handles `repo view`, with
     new cases "`GH_REPO` unset" and "repository probe fails", both
     refusing.
5. **D8.**
   * Every `run:` block in the reusable workflow reads `TAG` and
     `REF_TYPE` from `env:`. No `${{ … }}` stays inside `run:`.
   * New `scripts/check-workflow-expressions.sh` fails on any `${{`
     inside a `run:` block, with `check-workflow-expressions_test.sh`
     (a planted expression is caught; the real workflow passes).
6. **D4.** `check-workflow-gh-repo.sh`:
   * skips comment lines before matching `GH_REPO:`;
   * resets its state on any `^      - ` step start;
   * treats a `.core-lib-release-tools/scripts/refuse-existing-release.sh`
     call as a `gh` call.

   New `check-workflow-gh-repo_test.sh` covers the three plants from the
   MADR (script-only step, commented `GH_REPO`, unnamed step), a control
   plant, and the real workflow.
7. Gates, then commit.

### Phase 6: CI and tooling (`ci.yml`, `scripts/go-precheck.sh`, `Makefile`)

1. **D9.**
   * Linux adds `go test -race -count=1 ./...`.
   * Linux adds `shellcheck scripts/*.sh`,
     `npx --yes markdownlint-cli2@0.23.2` (the local version), and
     actionlint `v1.7.12` via `go run`.
   * `golangci-lint` is pinned at `v2.13.2` in `ci.yml` and in
     `go-precheck.sh`'s install hint.
   * CI also runs the new script tests from Phase 5.
2. **D11.** `ci.yml` gains
   `concurrency: {group: ci-${{ github.ref }}, cancel-in-progress: true}`.
   The orphan GitHub App check suite is reported to the owner in the
   hand-off; it is not a repository change.
3. Local proof: YAML parses; `go run …actionlint@v1.7.12` is clean on
   both workflows; `-race` passes locally.
4. Gates, then commit.

### Phase 7: documentation and close-out

1. **D3.** The guide resolves the pin with
   `git ls-remote https://github.com/maccavelli/go-core-lib 'refs/tags/v1.0.0^{}'`,
   and says why: annotated tags.
2. `README.md` and `docs/architecture.md` describe:
   * the behaviour changes (tag pinning, the https-only redirect, EOF as a
     decline, extras not validated, the event order);
   * the new scripts and CI steps;
   * the concurrency group.
3. **Close-out evidence,** on a fresh `git archive` of the final commit:
   * re-run the B and C reviewer probes and the author's A and D1 probes;
     each must now show the fixed behaviour;
   * record G-diff (now `v1.6.0` + 0002 §3 + these fixes) and G-api (doc
     comments only);
   * run every gate.
4. Set this PLAN `complete`, update the index, and commit. 0002-PLAN
   Phase 6 then resumes, starting with a push that needs the owner's ask.

## Verification

* **V1.** Each ID in the MADR maps to a step above. The execution record
  lists, per ID, the commit, the test name and its fail-first line.
* **V2.** G-api differs from `mcplib` `v1.6.0` only in doc comments.
* **V3.** `go test -race -count=1 ./...` passes. Coverage is recorded
  against 75.5 %, and it must not fall.
* **V4.** Three-target lint, `shellcheck`, `markdownlint-cli2`,
  actionlint and `go mod tidy -diff` are clean.
* **V5.** The re-run probes show the fixed behaviour for A1–A3, A6–A8,
  A10, B1–B6, C1–C6, C8 and D1.
* **V6.** On the Windows host, the Windows gate passes on the final commit,
  every Windows-only test has a recorded fail-first run, and neither
  symlink test skips. Then the Windows CI leg passes before `v1.0.0` is
  tagged, under 0002-PLAN Phase 6.

## Rollout and Rollback

* **Rollout.** Local commits per phase; nothing is pushed by this PLAN.
  The next push, under 0002-PLAN Phase 6 on the owner's ask, carries the
  0002 record commit `004d13b` and every 0003 commit.
* **Rollback.** Each phase is one commit (or lettered sub-commits). Revert
  that commit here; nothing is published. After a push and before the tag,
  fix forward.

## Execution Record

2026-09-29, before approval: the owner said a Windows test host is
available over SSH, with PowerShell 7 and a full Go environment. It was
probed read-only, and its properties are in "Fixed inputs". Its login
shell turned out to be Git Bash, not PowerShell. The PLAN and MADR were
amended to use it (the Windows gate, Phase 0 baseline, Phase 4 step 5,
and V6).

### Phase 0: records (2026-09-29)

The owner approved: "Proceed". This PLAN is `in-progress`.

* **Windows baseline** on `004d13b`, the pushed code. It ran on the
  Windows test host from a `git archive`, with a scratch runner script that
  is not committed:
  * `go vet ./...`: exit 0.
  * The listed Windows tests pass: `TestExactAssetNameWindowsExtension`,
    `TestWindowsCleanupReceiptRoundTrip`,
    `TestWindowsCleanupReceiptDigestMismatch`,
    `TestIsUnsupportedDirSyncAccessDenied`,
    `TestBusyRunningImageIncludesAccessDenied`.
  * Under the host's bash, `refuse-existing-release_test.sh` gives
    `6 passed, 0 failed`, `check-workflow-gh-repo.sh` gives `ok`, and
    `verify-selfupdate-release_test.sh` gives `all fixtures passed` (the
    host has `python3`).
  * **`go test -race -count=1 ./...` failed:**
    `replace_native_test.go:115: selfupdate: replace target: Access is denied.`
    in `TestNativeReplaceRunningCopy`. That led to D1.
  * The remote directory was removed after every run (`cleanup ok`).
* **D12.** `docs/README.md`'s stale "fail until the first package lands"
  row is replaced by a row for 0001 §6 as history, and a row for this
  record is added. 0002-MADR's "the first commit names it" is struck and
  annotated.
* **D2.** 0002-MADR's three "one consumer" statements are struck and
  annotated: all six consumers delete `bridge-release`.
* **0002 amendment.** 0002-MADR "More Information" has a dated amendment:
  `v1.0.0` includes 0003's fixes, the API is unchanged, G-diff grows, and
  §8 waits. 0002-PLAN Phase 6 has a dated entry: `v1.0.0` goes on the
  commit that completes this PLAN, not on `3700381`.

#### Deviation D1 (2026-09-29): B11, running-image replace fails on Windows

* **Found** by the Phase 0 baseline, and pre-existing: the code is
  `mcplib` `v1.6.0`'s.
  * `TestNativeReplaceRunningCopy` failed on the Windows host in 3 of 6
    diagnostic runs, and then in **12 of 15** runs (10 without `-race`, 5
    with), always
    `selfupdate: replace target: Access is denied.`.
  * Host facts: Defender real-time protection `False`; Windows
    `10.0.26200.0`.
  * The same package passed the `windows-2025` CI leg once, on
    `3700381`. Whether that was chance or a build difference was not
    established.
* **Cause.** `moveFileReplace` (`replace_windows.go:74`) retries only on
  `ERROR_SHARING_VIOLATION`. A running image transiently returns
  `ERROR_ACCESS_DENIED`, which `isBusyRunningImage` (`:85`) already
  classifies as busy.
* **Experiment,** on a scratch copy only: the retry condition changed to
  `isBusyRunningImage(last)`, with the deadline unchanged. The result was
  **15 of 15** passes, against 3 of 15 unchanged, on the same host in the
  same session.
* **Owner's decision:** "Option 1 fix it".
  * B11 is added to the MADR (High).
  * Phase 0b is added before Phase 1, so the Windows gate can pass in
    every later phase.
  * Option 2 (move the running image aside first) was not chosen.
* **Scope added.** `replace_windows.go` and `replace_native_test.go` in
  Phase 0b. Phase 4's B10 step still adds the `ctx` parameter.

Commit: `11015fc`.

#### Note (2026-09-29): an outside commit and push during Phase 0

Commit `6b9fb99`, "docs(decisions): plan fixes for debugging-pass
findings", was made in this clone at 23:12:19 under the owner's identity,
not by this PLAN's executor. It holds the 0003 MADR and PLAN as they were
before Phase 0, and a `docs/README.md` row. It and `004d13b` were pushed to
`origin/main`.
* That push's CI run `36667778889` concluded `success` on all three
  operating systems.
* So `TestNativeReplaceRunningCopy` passed on `windows-2025` for a second
  time, while the Windows test host fails it most of the time. B11's
  trigger therefore depends on the host.
* Nothing was rewritten. `11015fc` builds on `6b9fb99`.

### Phase 0b: Windows running-image replace, B11 (2026-09-29)

* **Test tightened.** `TestNativeReplaceRunningCopy` keeps `Install`'s
  error in its own variable (`installErr`), and now fails if a clean
  `Install` (nil error, no pending backup) leaves anything but the new
  bytes.
  * The first draft reused `err`, which `os.ReadFile` then overwrote; it
    was caught on reading and corrected before any run.
* **Fail-first**, Windows host, working tree with only the test change, 10
  runs without `-race` and 5 with: `pass=6 fail=9`, each failure
  `replace_native_test.go:115: selfupdate: replace target: Access is denied.`.
* **Fix.** `replace_windows.go` `moveFileReplace` retries while
  `isBusyRunningImage(last)`. The `DefaultLockTimeout` deadline is
  unchanged, and a comment cites B11.
* **Proof,** Windows host, fixed tree:
  * `native x20 -race: pass=20 fail=0`;
  * then the full Windows gate: `go vet` 0, `go test -race` 0,
    `refuse-existing-release_test.sh` 0, `check-workflow-gh-repo.sh` 0,
    `verify-selfupdate-release_test.sh` 0; `overall=0`, `cleanup ok`.
* **On this host:** `gofmt -l` empty. `CGO_ENABLED=0 GOOS=<t>` `go vet` and
  `go test -c` pass for linux, darwin and windows. `make pre-add-check`
  gives `47 file(s) clean`, `go test -race` passes, and
  `go mod tidy -diff` exits 0.

Commit: `6f0b661`.

### Phase 1: coordinator and confirmer (2026-09-29)

**Changes.**

* **C3.** `New` uses an `isNil` helper (reflect) for every required seam. It
  rejects a nil or typed-nil verifier and a typed-nil transformer; an
  untyped nil transformer is still a no-op.
* **C1.** When `--version` is set, `fetchRelease` requires
  `rel.Tag == req.TargetVersion`, else an `ErrIntegrity` error that quotes
  both tags.
* **C4.** Order is now immutable, then draft/prerelease, then `Validate`.
  Every tag in an error is `%q`, in `updater.go` and in `github.go`'s four
  release and asset errors.
* **C9.** `SHA256SUMS` is parsed and the entry resolved before
  `EventDownloadingBinary` and `CreateStaging`.
* **C8.** `EventVerified` follows the verifiers; `EventTransforming`
  follows it.
* **C2.** `Applied:false` with a nil error becomes the unexported
  `errNotCommitted`.
* **C14.** A once-only `closeSession` runs explicitly before
  `EventComplete` and joins its error; the defer is the safety net.
* **C10.** Every error from `execute` and `apply` passes through `wrapRun`,
  with two exceptions. A validation error for an invalid product name is
  returned unprefixed, since that name is not safe to print. And
  `ErrUpdateAvailable` stays a bare status result, as before.
* **C5.** `EventSelected.Detail` is "local build: apply requires --force"
  in check mode.
* **C12.** The pending-backup detail names
  `filepath.Join(target.Dir, name)`. `ExampleNewManagedInstaller` builds a
  real `ManagedInstaller`, and `ExampleExitCode` is added.
* **C6.** EOF before an answer is a decline.
* **C7.** `terminalConfirmer` is a pointer with one reader goroutine and a
  line channel.
* **C11.** The `Config.Limits` doc comment is corrected.
* **Test helpers.** `scriptSource` gained `entered`, `openErr` and a shared
  `log`; `recReporter` gained `events` and `log`.
* **New tests** (`updater_contract_test.go` and `confirmer_test.go`):
  `TestRunCallOrderFailureBoundaries`, `TestRunOperationsMatrix`,
  `TestRunRejectsTagMismatch`, `TestRunEscapesUntrustedTag`,
  `TestRunRejectsManifestBeforeStaging`,
  `TestRunEventOrderWithTransformer`, `TestRunTransformerSizeLimit`,
  `TestRunInstallerNoCommitIsError`, `TestRunCompleteAfterClose`,
  `TestRunCloseErrorJoinedAfterCommit`, `TestRunErrorsNameProduct`,
  `TestRunLocalBuildCheckHintsForce`, `TestRunPendingBackupDetail`,
  `TestTerminalConfirmerEOFDeclines`,
  `TestTerminalConfirmerCancelKeepsLine`. `TestNewRejectsNilCollaborators`
  and `TestOverlappingRun` were rewritten.

**Fail-first.** The new test files ran against a `git archive` of `6f0b661`,
with a scratch-only shim declaring `errNotCommitted`.

* **Failed, as required** (14):
  * `TestNewRejectsNilCollaborators` (subtests nil verifier, typed-nil
    source, reporter, verifier and transformer);
  * `TestRunErrorsNameProduct` (all 9 subtests);
  * `TestRunRejectsTagMismatch`, `TestRunEscapesUntrustedTag`,
    `TestRunRejectsManifestBeforeStaging`,
    `TestRunEventOrderWithTransformer`,
    `TestRunInstallerNoCommitIsError`, `TestRunCompleteAfterClose`,
    `TestRunLocalBuildCheckHintsForce`, `TestRunPendingBackupDetail`,
    `TestTerminalConfirmerEOFDeclines`;
  * `TestTerminalConfirmerCancelKeepsLine`, after two corrections. As
    first written it **passed** on the old code, because `cancel()` could
    run before the old `Confirm` started its reader. The test now waits
    for the prompt to be written, gives the reader 100 ms to block,
    cancels, writes the answer, and confirms with a 3 s deadline. On the
    old code: `ok=false err=context deadline exceeded, want the line
    typed after cancellation` (3 of 3 runs). On the current code: 5 of 5
    passes with `-race`.
* **Passed on the old code, so proven by mutation on a scratch copy**
  (these tests close gaps, not bugs):
  * `ignore-confirm-error` fails `TestRunCallOrderFailureBoundaries`
    (`failure at Confirm`);
  * `rollback-as-upgrade` fails `TestRunOperationsMatrix`
    (`op=upgrade … want rollback`);
  * `no-transform-size-limit` fails `TestRunTransformerSizeLimit`
    (`err = <nil>`);
  * `drop-close-error` fails `TestRunCloseErrorJoinedAfterCommit`;
  * `no-overlap-guard` first failed only through the 10-minute `go test`
    timeout, a hang. The second `Run` is now bounded by a 5 s context, and
    the mutation fails at once: `overlap err = selfupdate: demo: context
    deadline exceeded`. The fake's `entered` handoff moved under its mutex
    to avoid a data race in that mutated run.

**Gates.**

* `make pre-add-check`: `48 file(s) clean`.
* `go test -race -count=1 ./...`: ok, coverage **81.4 %** (baseline
  75.5 %). `go mod tidy -diff`: 0.
* **G-api** against `v1.6.0`: 26 differing lines, all doc text: the import
  header, the package comment, and the `Config.Limits`,
  `NewTerminalConfirmer` and `New` comments. No `func`, `type`, `var` or
  `const` line differs.
* **Windows gate:** `go vet` 0; `go test -race` ok; the confirmer tests,
  `TestRunCallOrderFailureBoundaries`, `TestOverlappingRun` and
  `TestNativeReplaceRunningCopy` all PASS; the three script tests 0;
  `overall=0`; `cleanup ok`.

Commit: `f095505`.

### Phase 2: network and integrity (2026-09-29)

**Changes.**

* **A1 with A5.**
  * `mapRelease` now calls a new `validateAssetStructure` (ID, basename,
    no control characters) for every asset.
  * `validateAssetMetadata` builds on it for the full check.
  * `execute` runs `validateAssetMetadata` on `sel.Binary` against
    `Limits.Executable`, and on `sel.Manifest` against `Limits.Manifest`,
    right after `Select`. `OpenAsset` still checks the asset it opens.
* **A2.** `ByTag` rejects `rel.Tag != tag` with `ErrIntegrity`.
* **A3.** `checkRedirect` refuses any hop that is not `https` unless the
  host is loopback. The URL is not echoed.
* **A6.** `decodeJSON` requires `dec.Token()` to return `io.EOF`.
* **A7.** A `Retry-After` in seconds above `maxRetryAfterSeconds` is
  ignored. The first draft, `int64(math.MaxInt64 / int64(time.Second))`,
  failed the pre-add gate with `unnecessary conversion (unconvert)` on all
  three targets. It was corrected and re-gated.
* **A8.** `getRelease` checks the status before reading. Non-2xx bodies,
  there and in `OpenAsset`, go through the new `readTruncated`, which is
  capped at `ErrorBody` and tolerates truncation.
* **A10.** `validateGitHubName` rejects `.` and `..`.
* **A11.** The new `isInvisibleControl` (`unicode.Cf`, U+2028, U+2029) is
  applied in `sanitizeDiagnostic` and `sanitizeText`.
* **Tests.**
  * `github_hardening_test.go`: `TestGitHubExtraAssetsDoNotPoisonRelease`,
    `TestGitHubByTagRejectsMismatchedTag`,
    `TestGitHubRedirectRequiresHTTPS`,
    `TestDecodeJSONRejectsTrailingDelimiters`,
    `TestRateLimitRetryAfterOverflow`,
    `TestGitHubLargeErrorBodyKeepsRateLimit`,
    `TestNewGitHubSourceRejectsDotNames`,
    `TestSanitizeRemovesFormatControls`, and for A4
    `TestGitHubForbiddenRemainingZeroIsRateLimit`,
    `TestGitHubOpenAssetStatuses`, `TestGitHubOpenAssetValidatesAsset` and
    `TestRateLimitErrorText`.
  * `updater_contract_test.go`: `TestRunValidatesSelectedAssets`.
  * Authoring slip: the sanitizer test's `\u` escapes were written into
    the file as literal invisible runes, and `gofmt` refused them ("illegal
    byte order mark"). A scratch script rewrote the 12 runes as Go escapes.
    A scan found no literal control bytes in any Go or script file; `\x`
    escapes had been preserved.

**Fail-first** against a `git archive` of `f095505`.

* **Failed, as required** (9): `TestGitHubExtraAssetsDoNotPoisonRelease`,
  `TestGitHubByTagRejectsMismatchedTag`, `TestGitHubRedirectRequiresHTTPS`,
  `TestDecodeJSONRejectsTrailingDelimiters`,
  `TestRateLimitRetryAfterOverflow`,
  `TestGitHubLargeErrorBodyKeepsRateLimit`,
  `TestNewGitHubSourceRejectsDotNames`,
  `TestSanitizeRemovesFormatControls`, `TestRunValidatesSelectedAssets`.
* **The four A4 tests passed there** (they close gaps). Mutation proofs
  for the five checks A4 named:

  | Mutation | Test | Failure |
  |---|---|---|
  | `no-403-remaining` | `TestGitHubForbiddenRemainingZeroIsRateLimit` | `err = selfupdate: github http 403` |
  | `no-state-check` | `TestGitHubOpenAssetValidatesAsset` | `open state: accepted` |
  | `no-size-limit` | `TestGitHubOpenAssetValidatesAsset` | `oversize: accepted` |
  | `no-openasset-status` | `TestGitHubOpenAssetStatuses` | `status 429: no error` |
  | `no-belongs-check` | `TestGitHubOpenAssetValidatesAsset` | `foreign id: accepted` |

**Gates.**

* `make pre-add-check`: `49 file(s) clean`.
* `go test -race -count=1 -cover ./...`: coverage **83.3 %**.
* `go mod tidy -diff`: 0.
* G-api: byte-identical to Phase 1's (26 doc-text lines against
  `v1.6.0`; no signature line).
* Windows gate: `go vet` 0, `go test -race` 0, the named Phase 2 tests
  PASS, script tests 0, `overall=0`, `cleanup ok`.

Commit: `e5bb201`.

### Phase 3: install path, portable (2026-09-29)

**Changes.**

* **B1.** `replaceTarget` (unix and windows), when the directory sync
  fails and the restore also fails, returns the backup with
  `errors.Join(syncErr, "restore backup: …")`.
  * `installSession.Install` returns `InstallResult{Target, Backup}` with
    that error.
  * `managedSession.Install` passes the returned `applied` to `recover`,
    which retries the restore.
* **B2.** `recover` runs on
  `context.WithTimeout(context.WithoutCancel(parent), recoveryTimeout)`,
  with `recoveryTimeout = 2 * time.Minute` (unexported).
* **B3.** `copyFile` chmods the backup to the source's mode before `Sync`.
* **B4.** `applyResult.renamed`. The new `replaceLocked`, shared by
  `Install` and the managed `apply`, deregisters staging only when the
  rename consumed it.
* **B5.** New `lock.go` `openLockFile`: `root.Lstat` first (anything not
  regular, a symlink included, is refused); create only with `O_EXCL`;
  `os.SameFile` after opening; one retry if a concurrent creator wins.
  `lock_unix.go` (keeping `O_NOFOLLOW`) and `lock_windows.go` use it.
* **B10, directory identity.**
  * `beginSession` requires `os.SameFile(root.Stat("."), os.Stat(dir))`
    and records the path's `FileInfo`.
  * `checkDir` re-checks it before the replace and before commit, and
    returns `ErrConcurrentUpdate` on a mismatch.
  * The check runs first in `replaceLocked`. As first written it came
    after the staging `Lstat`, and the swap test then reported "no such
    file" instead of `ErrConcurrentUpdate`; the order was corrected.
* **B9, planted staging symlink.** `replaceLocked` requires the staging
  path to `Lstat` as a regular file. The fail-first run below shows the
  old code **installed** a symlink planted at the staging path, so this is
  a fixed defect, found by the test PLAN §8 required.
* **B8.** `TestManagedRollbackErrorJoined` asserts `errors.Is(err,
  rec.restoreE)` and `restores == 1` unconditionally.
* **Test helpers.** `fakeLife` gained `installedErr`, `runningErr`,
  `onHealth` and `startCtxErrs`.
* **New tests** (`install_hardening_test.go`):
  `TestInstallSyncAndRollbackFailureReported`,
  `TestManagedApplyFailureRetriesRollback`,
  `TestManagedRecoveryIgnoresCancelledContext`,
  `TestBackupCopyFallbackPreservesMode`,
  `TestInstallFailureBeforeRenameRemovesStagingOnClose`,
  `TestLockRejectsRelativeSymlink`,
  `TestLockRejectsDanglingRelativeSymlink`,
  `TestInstallDetectsSwappedDirectory`, `TestStagingRejectsPlantedSymlink`,
  `TestInstallInjectedFailures`, `TestInstallPermissionDenied`,
  `TestInstallCancelledBeforeAndAfterStaging`,
  `TestRunBadBodyLeavesNoStaging`, `TestManagedFailureMatrix`.

**Fail-first** against a `git archive` of `e5bb201`.

* **Failed, as required** (9):
  `TestInstallSyncAndRollbackFailureReported`,
  `TestManagedApplyFailureRetriesRollback`,
  `TestManagedRecoveryIgnoresCancelledContext`,
  `TestBackupCopyFallbackPreservesMode`,
  `TestInstallFailureBeforeRenameRemovesStagingOnClose`,
  `TestLockRejectsRelativeSymlink`,
  `TestLockRejectsDanglingRelativeSymlink`,
  `TestInstallDetectsSwappedDirectory`,
  `TestStagingRejectsPlantedSymlink`.
* **Passed there, proven by mutation:**

  | Mutation | Test | Failure |
  |---|---|---|
  | `no-sync-restore` | `TestInstallInjectedFailures` | `target "new-bytes", want "old-bytes"` |
  | `no-install-ctx-check` | `TestInstallCancelledBeforeAndAfterStaging` | `Install err = <nil>` |
  | `no-close-staging-removal` | `TestRunBadBodyLeavesNoStaging` | `leftovers: [.demo.selfupdate-…]` |
  | `no-recovery-restart` | `TestManagedFailureMatrix` | `starts = 1, want 2` |
  | `drop-recov-join` (B8) | `TestManagedRollbackErrorJoined` | `restore error not joined` |

  * `no-close-staging-removal` as first written left a variable unused,
    so the "failure" was a build error. It was rewritten to remove a wrong
    path, which builds; the recorded failure is from that version.
  * `TestInstallPermissionDenied` has no product-side check to mutate: the
    refusal comes from the OS. It guards that the target is left intact
    when the OS refuses.

**Gates.**

* `make pre-add-check`: `51 file(s) clean`.
* `go test -race -cover`: coverage **85.0 %**. `go mod tidy -diff`: 0.
* G-api: unchanged since Phase 2.
* **Windows gate:** `go vet` 0, `go test -race` 0, `overall=0`,
  `cleanup ok`.
  * PASS on the Windows host: `TestInstallSyncAndRollbackFailureReported`,
    `TestManagedApplyFailureRetriesRollback`,
    `TestManagedRecoveryIgnoresCancelledContext`,
    `TestInstallFailureBeforeRenameRemovesStagingOnClose`,
    `TestLockRejectsRelativeSymlink`,
    `TestLockRejectsDanglingRelativeSymlink`,
    `TestStagingRejectsPlantedSymlink`. The two lock-symlink tests did not
    skip, as Phase 4 step 5 requires. This is also the first Windows
    exercise of `openLockFile` and of `os.SameFile` on directories.
  * Skipped on Windows, by design and with the reason logged:
    * `TestBackupCopyFallbackPreservesMode` (POSIX mode bits);
    * `TestInstallPermissionDenied` (POSIX directory permissions);
    * `TestInstallDetectsSwappedDirectory`: "The process cannot access the
      file because it is being used by another process". Windows refuses
      to rename a directory the session holds open, which itself prevents
      the swap.

Commit: `339fd17`.

### Phase 4: install path, Windows (2026-09-30)

**Changes.**

* **B6.** The portable `validateReceiptBackup` (in `cleanup.go`, with
  `backupPrefix`) requires:
  * a non-empty `filepath.IsLocal` bare basename with no separators;
  * the prefix `"." + target.Base + ".selfupdate-bak-"`, with something
    after it;
  * a name that is not `target.Base`.

  `processCleanupReceipt` uses it, and now `Lstat`s and removes through
  `root` (`root.Lstat`, `root.Remove`).
* **B7.** A receipt whose backup does not exist is removed, the directory
  is synced, and `nil` is returned. A digest mismatch, a non-regular or
  reparse backup, and a failed removal still fail closed.
* **B10, Windows.** `ctx` is plumbed through the replace seam:
  `replacePath` is now `func(ctx, old, new)`, and `replaceTarget`,
  `rollbackReplacement`, `replaceLocked` and `installSession.rollback`
  take `ctx`. The managed `recover` passes its recovery context.
  `moveFileReplace(ctx, …)` calls `MoveFileEx` through the new
  `moveFileExFn` seam and stops retrying when `ctx` is done, returning
  both the last error and `ctx.Err()`. The plumbing was one count-asserted
  script across five files and two test files.

#### Deviation D2 (2026-09-30): `PendingBackup` is a path, not a basename

* **Found** while reading the Windows commit path for this phase.
  `commitReplacement` (windows) returns `result.backup`, an **absolute
  path** from `os.CreateTemp(target.Dir, …)`, and the `v1.6.0` code
  returned the same. The doc comments on `InstallResult.PendingBackup`
  and `Result.PendingBackup` said "basename".
* **Effect.** Phase 1's C12 change,
  `filepath.Join(target.Dir, PendingBackup)`, would have printed a doubled
  path on Windows. `TestRunPendingBackupDetail` missed it because its fake
  installer returned a bare name.
* **Resolution,** within C12's scope:
  * the detail uses `PendingBackup` as-is when it is absolute, and joins
    it onto the directory otherwise;
  * both doc comments now say it is a path. That is a doc-only G-api
    change (two comments), matching what the code always returned;
  * `TestRunPendingBackupAbsolutePath` added. On `339fd17` it fails with
    the doubled path (`…/001/var/folders/…/001/…`).
* **Scope added:** `types.go` (two doc comments).

#### Deviation D3 (2026-09-30): the seam tests' fail-first method

* **Found.** `TestMoveFileReplaceHonoursContext` and
  `TestMoveFileReplaceRetriesAccessDenied` use `moveFileExFn` and
  `moveFileReplace(ctx, …)`, which this phase introduces. They cannot
  compile against `339fd17`.
* **Resolution.** Each was proven on the Windows host by a mutation that
  restores exactly the pre-fix behaviour in the current code: no
  `ctx.Done()` case, and `isSharingViolation` as the retry condition. The
  receipt tests did run fail-first against `339fd17`, with a scratch shim
  giving `backupPrefix`, and giving `validateReceiptBackup` as an exact
  copy of the old inline check.

**Fail-first, on the Windows host,** against `339fd17` with the new
`cleanup_windows_test.go`, `cleanup_test.go` and the shim:

* `TestValidateReceiptBackup` FAIL. The old check accepted `demo.exe`,
  the lock, the receipt, the bare prefix, another product's backup,
  `victim`, `/home/<user>/bin/…`, `.` and `..`. *(The fixture's account
  became the `<user>` placeholder in
  0036-PLAN-align-docs-ci-and-disclosure-gates.md, phase 1.)*
* `TestWindowsCleanupReceiptMissingBackup` FAIL:
  `stale receipt blocked the update: selfupdate: stat pending backup: GetFileAttributesEx …`.
* `TestWindowsCleanupReceiptMalformed` FAIL on *names target*, *names
  lock*, *other product* and *bare prefix* ("accepted"). Its *names
  target* case shows that the old code would delete the live executable
  given a receipt naming it with a matching digest.
* `TestWindowsCleanupReceiptReparseBackup`,
  `TestWindowsReceiptConsumedByBegin`, `…RoundTrip` and
  `…DigestMismatch` passed there, being existing protections. They are
  proven by the mutations below.

**Mutations, on the Windows host:**

| Mutation | Test | Failure |
|---|---|---|
| `movefile-ignores-ctx` | `TestMoveFileReplaceHonoursContext` | returns the sharing violation after **5.01 s**, without `context.DeadlineExceeded` |
| `movefile-no-access-denied-retry` | `TestMoveFileReplaceRetriesAccessDenied` | `err=Access is denied. calls=1` |
| `receipt-no-reparse-check` | `TestWindowsCleanupReceiptReparseBackup` | `accepted a symlinked backup` |
| `begin-skips-receipt` | `TestWindowsReceiptConsumedByBegin` | `.demo.selfupdate-bak-begin not consumed by Begin` |
| `receipt-no-digest-check` | `TestWindowsCleanupReceiptDigestMismatch` | `err = <nil>, want the digest mismatch (ErrIntegrity)` |

* `receipt-no-reparse-check` as first written left `binfo` unused, so its
  "failure" was a build error. It was rewritten to keep the variable in
  use, and the recorded failure is from the rewrite.
* `TestWindowsCleanupReceiptDigestMismatch` itself was corrected. Its
  backup name `"bak"` is invalid under B6, which would have refused it for
  the wrong reason. It now uses a valid backup name and asserts
  `ErrIntegrity`.

**Gates.**

* `make pre-add-check`: `51 file(s) clean`.
* `go test -race -cover`: coverage **85.1 %**. `go mod tidy -diff`: 0.
* G-api: the two `PendingBackup` doc comments (D2) and nothing else.
* **Windows gate:** `go vet` 0, `go test -race` 0, script tests 0,
  `overall=0`, `cleanup ok`. PASS, with **no skips**:
  `TestValidateReceiptBackup`, `TestWindowsCleanupReceiptRoundTrip`,
  `…DigestMismatch`, `…MissingBackup`, `…Malformed`, `…ReparseBackup`,
  `TestWindowsReceiptConsumedByBegin`, `TestLockRejectsRelativeSymlink`,
  `TestLockRejectsDanglingRelativeSymlink`,
  `TestStagingRejectsPlantedSymlink`, `TestNativeReplaceRunningCopy`,
  `TestMoveFileReplaceHonoursContext`,
  `TestMoveFileReplaceRetriesAccessDenied`, `TestRunPendingBackupDetail`,
  `TestRunPendingBackupAbsolutePath`.

Commit: `14b40a8`.

### Phase 5: release tooling (2026-09-30)

**Changes.**

* **D1.** `verify-selfupdate-release.sh`'s `parse_sums` is now a port of
  `selfupdate/checksums.go`:
  * it splits on `\n` only;
  * a line of 4096 bytes or more fails (the scanner buffer);
  * trailing `\r` is stripped;
  * blank lines and comments are recognised, and fields split, with an
    explicit `go_isspace`. Python's `str.isspace` also counts
    U+001C–U+001F, so it could not be used;
  * one `*` marker is allowed; digests are hex; names are basenames;
    duplicates and a manifest with no entries are refused.

  23 fixtures in `selfupdate/testdata/manifest-parity/<case>/`
  (`SHA256SUMS` + `expect`), 9 accept and 14 reject, are run by
  `TestManifestParityFixtures` (Go) and by the verifier test (shell). A
  scratch generator built them from fixed bodies (`linux-body`,
  `win-body`), with special characters made by `chr()`.
* **D5.**
  * Extras must match the product-name class
    `^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`. The consumers' current extras
    (`install.sh`, `install.ps1`, `magic-cli-remote-v0.20.0-arm64.apk`)
    are accepted by name in the test.
  * The upload step builds a `files` array from
    `find staging -type f -print0 | sort -z`.
* **D6.** A staged entry that `lstat`s as anything but a regular file
  (symlink, device) is refused.
* **D7.** `refuse-existing-release.sh` refuses when `GH_REPO` is empty or
  `gh repo view "$GH_REPO"` fails. Its test stub answers `repo view`
  separately, with new cases "unreadable repository" and "empty
  `GH_REPO`" (8 cases).
* **D8.**
  * Every `run:` block reads `TAG` / `REF_TYPE` from `env:`; the only
    `github.ref_*` expressions left are in `env:` blocks.
  * New `check-workflow-expressions.sh` fails on `${{` inside a `run:`
    block or on a one-line `run:`.
  * Its `check-workflow-expressions_test.sh` covers a planted block
    expression, a planted one-line expression, an allowed `env:`
    expression, and the real workflow.
* **D4.** `check-workflow-gh-repo.sh`:
  * skips comment lines before matching `GH_REPO:`;
  * resets its state at any `^      - ` step;
  * counts `gh repo` and the refuse script's invocation as calls.

  New `check-workflow-gh-repo_test.sh` (6 cases).
* **D10.** The verifier test adds:
  * the 23 parity cases;
  * a Windows binary without `.exe`, empty platforms, and a nested
    directory;
  * a symlinked binary (only when the shell creates a real link);
  * four unsafe extra names, and two consumer extra names.
* Two SC2016 notes in the new tests are deliberate (literal `$TAG` / `$X`
  written into planted YAML). They are disabled per line with that reason.

#### Deviation D4 (2026-09-30): `.gitattributes` for the parity fixtures

* **Found.** `* text=auto eol=lf` would normalise the CRLF, lone-CR and
  multiple-CR fixtures on commit, silently turning them into LF cases.
  Shown in a scratch repository with the same attributes: `without -text:
  CR bytes in index = 0 (working file has 2)`.
* **Resolution.** `selfupdate/testdata/manifest-parity/** -text`, with a
  comment. In this repository, `git check-attr text` reports `unset`, and
  the staged bytes equal the working bytes for all three CR fixtures.
* **Scope added:** `.gitattributes`, one rule.

#### Incident (2026-09-30): a real `gh` ran during a demonstration

* **What happened.** The first D5 upload demonstration put a stub `gh`
  first on `PATH`. This host's shell startup re-orders `PATH`, which is the
  same trap `refuse-existing-release_test.sh` documents, so the real `gh`
  ran `gh release upload v1 staging/install me.sh` in a scratch directory.
  It failed at once: `failed to run git: fatal: not a git repository`.
  There was no repository context and no `GH_REPO`, so it could not
  address any repository.
* **Checked.** `gh release list -R maccavelli/go-core-lib` returned no
  releases.
* **Corrected.** The demonstration was redone with `gh` as a shell
  **function**, which cannot fall through to a binary. Old line: `gh got 3
  file argument(s): [staging/demo-linux-amd64] [staging/install] [me.sh]`.
  New: `gh got 2 file argument(s): … [staging/install me.sh]`.

**Fail-first**, against `14b40a8`'s tools:

* The old verifier **accepts** `lone-cr`, `vertical-tab`, `form-feed`,
  `file-separator`, `next-line` and `long-comment`, all of which the
  client rejects (D1). It also accepts the extras `install me.sh`, `*`
  and `a;b` (D5), and a symlinked binary (D6).
* The new refuse test against the old guard: `FAIL unreadable repository
  is refused`, `FAIL empty GH_REPO is refused`, `6 passed, 2 failed` (D7).
* The new checker test against the old checker: `FAIL refuse script
  without GH_REPO`, `FAIL commented-out GH_REPO`, `FAIL unnamed step
  inherits nothing`, `3 passed, 3 failed` (D4).
* The new expressions checker on the old workflow: fails, reporting 7
  interpolations (D8).
* `TestManifestParityFixtures` passes on the old Go code, whose parser
  was correct. Mutation `go-fields-lenient` (`len(fields) < 2`) fails it at
  `three-fields`.

**Gates.**

* `make pre-add-check`: `52 file(s) clean`.
* `go test -race -cover`: **85.2 %**. `go mod tidy -diff`: 0.
* `shellcheck scripts/*.sh`: 0. Both workflows parse.
* **Windows gate** (the scripts under the host's Git Bash):
  * `go vet` 0, `go test -race` 0 (including `TestManifestParityFixtures`);
  * refuse test `8 passed`, checker test `6 passed`, expressions checker
    and test `4 passed`;
  * verifier test `all fixtures passed`, with "skip - symlinked binary in
    staging (this shell cannot create a symlink)": Git Bash's `ln -s`
    copies. The symlink case ran and passed on this host;
  * `overall=0`, `cleanup ok`.

Commit: `d0902c4`.

### Phase 6: CI and tooling (2026-09-30)

**Changes** (`ci.yml`, and the `go-precheck.sh` install hint):

* **D11.** `concurrency: {group: ci-${{ github.ref }}, cancel-in-progress:
  true}`. The orphan GitHub App check suite is an installation setting,
  reported to the owner in the hand-off; no repository change.
* **D9.**
  * Linux `go test -race -count=1 ./...`.
  * `golangci-lint` is pinned at **v2.13.2** in `ci.yml` and in the
    install hint. No `v2.13.1` remains outside the records.
  * A Linux step runs `shellcheck scripts/*.sh`,
    `npx --yes markdownlint-cli2@0.23.2` and
    `go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12`.
  * The contract step also runs `check-workflow-gh-repo_test.sh`, and
    `check-workflow-expressions.sh` on both workflows, plus its test.
    `ci.yml` itself has no `${{` in a `run:` block.

**Local proof.**

* Both workflows parse.
* `actionlint` v1.7.12 on the repository: exit 0.
* `markdownlint-cli2@0.23.2` via `npx`: `0 issues in 0 files` (6 files).
* `check-workflow-expressions.sh .github/workflows/ci.yml`: ok.

**Fail-first** for each new CI check, on a scratch copy with one planted
defect:

| Check | Planted | Result |
|---|---|---|
| `go test -race` | a goroutine and the test both write `n` | exit 1, `WARNING: DATA RACE` |
| `shellcheck` | `echo $x` in a new script | exit 1, `SC2086` |
| `markdownlint-cli2@0.23.2` | a `*` list item in `README.md` | exit 1, `MD004/ul-style` |
| `actionlint` | `runs-on: ${{ matrix.nope }}` | exit 1, `property "nope" is not defined in object type {os: string}` |

* The first actionlint run in the scratch copy printed no message. That
  copy was not a git repository, so auto-discovery could not be trusted.
  It was re-run with explicit file paths: the unmodified workflows exit 0,
  and the planted one gives the failure above.
* Because the push's own CI is what finally proves these steps run, that
  run is recorded under 0002-PLAN Phase 6.

**Gates.** `make pre-add-check`: `52 file(s) clean`. `shellcheck`: 0.
Windows gate: `overall=0`, `cleanup ok`.

Commit: `8e68c0e`.

### Phase 7: documentation and close-out (2026-09-30)

**Documentation.**

* **D3.** The guide resolves the pin with the peeled ref
  `'refs/tags/v1.0.0^{}'` and says why (annotated tags).
* **The guide** no longer claims "no behaviour your program can observe
  changes". It gains "Behaviour you may notice", with ten items (B11, C1,
  C6, C2, C3, A1, A3, the event order with C5, C10, D2). Its workflow
  section states the stricter extras rule, the `SHA256SUMS` rule, and the
  regular-files-only rule.
* **`README.md`** gains a pointer to this record and to the guide, the
  pinned `--version`, the https-only redirect, EOF as a decline, and the
  gate's ported parser and name rules.
* **`docs/architecture.md`** gets the file counts from the tree (27
  non-test, 25 test, 23 parity cases), the new scripts, the coordinator and
  install-path guarantees, the workflow's `env:`-only ref handling, and the
  full CI list including `concurrency`. Nested `*` list markers failed
  MD004 and were changed to `-`.

**Close-out evidence.** Every probe, re-run on a snapshot of the final
tree. The B probes' `replacePath` seam closures were adapted to the Phase 4
`ctx` signature and nothing else; a first blanket substitution also hit the
two-argument `osLink` seam, and that one line was reverted.

* **A1:** extras `err=<nil>`.
* **A2:** `github returned release "v0.1.0" for tag "v1.2.3"`.
* **A3:** `refusing redirect to a non-https location`.
* **A6:** all three inputs give `trailing github json`.
* **A7:** `RetryAfter=0s`. **A8:** `isRateLimited=true`.
* **A10:** `github owner must not be a dot segment`.
* **B1:** `Backup:` reported, error joined. **B3:** mode kept. **B4:** no
  leak. **B5:** `lock is not a regular file`.
* **B2:** the probe calls `t.Errorf` unconditionally, so it is read from
  its log: the joined error now holds `context canceled` **once**, the
  origin. The pre-fix run held it twice, because the probe's `Start`
  returns `ctx.Err()` and recovery then ran on the cancelled context.
* **B6:** the reviewer's probe replicates the old inline expression, not
  the new function, so it is unchanged by design. `TestValidateReceiptBackup`
  covers the fix.
* **C1:** `source returned release "v1.1.0" for requested "v1.0.5"`.
* **C2:** `installer reported no committed replacement exit=1`.
* **C3:** the probe fails at `New` with `verifier 0 is nil`, which is the
  fix.
* **C4:** `hasESC=false hasNL=false`. **C6:** EOF gives `ok=false err=<nil>`.
* **C8:** `… verified transforming …`.
* **D1:** shown in Phase 5's fail-first; both parsers now agree on all 23
  fixtures.

**Final gates** (the working tree of this commit):

* `make pre-add-check`: `52 file(s) clean`.
* `go test -race -count=1 -cover ./...`: coverage **85.2 %** (75.5 %
  before this PLAN).
* `go mod tidy -diff`: 0; `go mod verify`: `all modules verified`.
* `shellcheck scripts/*.sh`: 0. `markdownlint-cli2@0.23.2` (npx):
  `0 issues`. `actionlint` v1.7.12: 0.
* All six script tests and checkers: 0. `check-workflow-expressions.sh` on
  `ci.yml`: 0.
* **G-api** against `mcplib` `v1.6.0`: 34 differing lines, **0** at
  signature level. The differences are doc comments only: the import
  header, the package comment, `Config.Limits`, `NewTerminalConfirmer`,
  `New`, and the two `PendingBackup` fields.
* **G-diff:** 31 files of `selfupdate/` differ from `v1.6.0`. Since
  `004d13b`, `selfupdate/`, `scripts/` and `.github/` show 86 files
  changed, +2906 −240 (fixtures included).
* **Windows gate:** `go vet` 0; `go test -race` 0; the Windows and
  cross-platform named tests pass; all script tests 0 (the Git Bash
  symlink skip, as in Phase 5); `overall=0`; `cleanup ok`.

**V1: finding → fix → evidence.**

| ID | Phase | Test (fail-first unless marked M, a mutation proof) |
|---|---|---|
| B11 | 0b | `TestNativeReplaceRunningCopy` (9 of 15 fail → 20 of 20 pass, Windows) |
| B1 | 3 | `TestInstallSyncAndRollbackFailureReported`, `TestManagedApplyFailureRetriesRollback` |
| B2 | 3 | `TestManagedRecoveryIgnoresCancelledContext` |
| B3 | 3 | `TestBackupCopyFallbackPreservesMode` |
| B4 | 3 | `TestInstallFailureBeforeRenameRemovesStagingOnClose` |
| B5 | 3 | `TestLockRejectsRelativeSymlink`, `TestLockRejectsDanglingRelativeSymlink` |
| B6 | 4 | `TestValidateReceiptBackup`, `TestWindowsCleanupReceiptMalformed` |
| B7 | 4 | `TestWindowsCleanupReceiptMissingBackup` |
| B8 | 3 | `TestManagedRollbackErrorJoined` (M: `drop-recov-join`) |
| B10 | 3, 4 | `TestInstallDetectsSwappedDirectory`; `TestMoveFileReplaceHonoursContext` (M, D3) |
| B9 | 3, 4 | `TestStagingRejectsPlantedSymlink`; M: `TestInstallInjectedFailures`, `…CancelledBeforeAndAfterStaging`, `TestRunBadBodyLeavesNoStaging`, `TestManagedFailureMatrix`; `TestInstallPermissionDenied` (OS-enforced); Windows receipt tests |
| C1 / A2 | 1, 2 | `TestRunRejectsTagMismatch`, `TestGitHubByTagRejectsMismatchedTag` |
| C2 | 1 | `TestRunInstallerNoCommitIsError` |
| C3 | 1 | `TestNewRejectsNilCollaborators` |
| C4 | 1 | `TestRunEscapesUntrustedTag` |
| C5 | 1 | `TestRunLocalBuildCheckHintsForce` |
| C6 | 1 | `TestTerminalConfirmerEOFDeclines` |
| C7 | 1 | `TestTerminalConfirmerCancelKeepsLine` |
| C8 | 1 | `TestRunEventOrderWithTransformer` |
| C9 / A9 | 1 | `TestRunRejectsManifestBeforeStaging` |
| C10 | 1 | `TestRunErrorsNameProduct` |
| C11 | 1 | doc comment only |
| C12 | 1, 4 | `TestRunPendingBackupDetail`, `TestRunPendingBackupAbsolutePath` (D2), `ExampleNewManagedInstaller`, `ExampleExitCode` |
| C13 | 1 | M: `TestRunCallOrderFailureBoundaries`, `TestRunOperationsMatrix`, `TestRunTransformerSizeLimit`, `TestOverlappingRun`, `TestRunCloseErrorJoinedAfterCommit` |
| C14 | 1 | `TestRunCompleteAfterClose` |
| A1 / A5 | 2 | `TestGitHubExtraAssetsDoNotPoisonRelease`, `TestRunValidatesSelectedAssets` |
| A3 | 2 | `TestGitHubRedirectRequiresHTTPS` |
| A4 | 2 | M: the four A4 tests, five mutations |
| A6 | 2 | `TestDecodeJSONRejectsTrailingDelimiters` |
| A7 | 2 | `TestRateLimitRetryAfterOverflow` |
| A8 | 2 | `TestGitHubLargeErrorBodyKeepsRateLimit` |
| A10 | 2 | `TestNewGitHubSourceRejectsDotNames` |
| A11 | 2 | `TestSanitizeRemovesFormatControls` |
| D1 | 5 | the old gate accepts 6 client-rejected manifests; `TestManifestParityFixtures` (M) + the shell parity loop |
| D2 | 0 | record correction |
| D3 | 7 | guide correction (the peeled ref) |
| D4 | 5 | `check-workflow-gh-repo_test.sh` (3 of 6 fail on the old checker) |
| D5 | 5 | the old gate accepts 3 unsafe extras; the upload array (3 → 2 arguments) |
| D6 | 5 | the old gate accepts a symlinked binary |
| D7 | 5 | `refuse-existing-release_test.sh` (2 of 8 fail on the old guard) |
| D8 | 5 | `check-workflow-expressions.sh` (7 interpolations in the old workflow) + its test |
| D9 | 6 | planted race, SC2086, MD004, actionlint property error |
| D10 | 5 | the verifier test's parity loop and staging cases |
| D11 | 6 | `concurrency` group. The App check suite is the owner's; no repository change |
| D12 | 0 | record correction |

**Not done here, and why.**

* The orphan GitHub App check suite (D11) is an installation setting
  outside the repository; the owner is told in the hand-off.
* A CI run of the new steps needs a push, which is 0002-PLAN Phase 6 and
  needs the owner's ask. Until then the steps are proven locally, as above.
* Back-porting to `mcplib`, and the consumers' migrations, are out of scope
  (MADR "What B means").

~~This PLAN is `complete`. 0002-PLAN Phase 6 resumes: a push on the
owner's ask, CI on three operating systems, then `v1.0.0` on this commit.~~
*Reopened 2026-09-30 by deviation D5: the push's CI failed on Linux.*

Commit: `a7d5f01`.

#### Deviation D5 (2026-09-30): CI's shellcheck differs from the local gate's

* **Found.** On the owner's ask ("Stage, commit, and push all
  outstanding"), `main` was pushed (`6b9fb99..a7d5f01`), and CI run
  `36726867210` followed.
  * macOS and Windows passed. This was the first CI run of the Phase 4
    Windows tests and the Windows script steps.
  * `validate (ubuntu-24.04)` failed at the new
    `shellcheck, markdownlint, actionlint` step:
    `In scripts/verify-selfupdate-release.sh line 45: [ -n "$DIR" ] && [ -n "$PRODUCTS_JSON" ] && [ -n "$PLATFORMS_JSON" ] || usage — SC2015 (info)`.
  * The line is unchanged from `mcplib` `v1.6.0`.
* **Cause.** Phase 6 added `shellcheck` to CI without a version. The step
  used the runner's distribution shellcheck, which is older than this
  host's 0.11.0; 0.11.0 does not report the line. Phase 6's fail-first
  proved the step can fail, but not that it agrees with the local version.
  The line itself is correct: `usage` exits whenever any argument is
  missing.
* **Owner's decision:** "Option 1, but pin to the most recent shellcheck
  version supported by CI so they are both up to date and also match."
* **Measured.** The latest `koalaman/shellcheck` release is **v0.11.0**
  (2025-08-04), and this host has 0.11.0 (Homebrew stable 0.11.0), so
  only CI changes. The `linux.x86_64.tar.xz` asset's SHA-256, computed from
  a scratch download, is
  `8c3be12b05d5c177a04c29e3c78ce89ac86f1595681cab149b65b97c4e227198`, the
  digest GitHub publishes for it.
* **Phase 8 (added).**
  1. `verify-selfupdate-release.sh`: line 45 becomes an explicit
     `if [ -z … ] || …; then usage; fi`, with the same behaviour. The
     verifier test gains a "missing required arguments" usage case.
  2. `ci.yml`: the step downloads shellcheck v0.11.0 for linux x86_64,
     verifies the pinned SHA-256 with `sha256sum -c`, and puts it first on
     `PATH`, so both `shellcheck scripts/*.sh` and actionlint's embedded
     shellcheck use it. It prints `shellcheck --version`.
  3. `docs/architecture.md` records the pinned version.
  4. Proof: an older shellcheck release flags the old line and not the
     new one; a wrong digest fails the checksum step; local gates and the
     Windows gate pass; then a push, and CI green on three operating
     systems.
* **Scope added:** `scripts/verify-selfupdate-release.sh`,
  `scripts/verify-selfupdate-release_test.sh`, `.github/workflows/ci.yml`,
  `docs/architecture.md`.

### Phase 8: shellcheck parity (2026-09-30)

* **Line 45** is now
  `if [ -z "$DIR" ] || [ -z "$PRODUCTS_JSON" ] || [ -z "$PLATFORMS_JSON" ]; then usage; fi`.
  The verifier test's new case `missing required arguments` gets exit 2
  (43 `ok` lines).
* **`ci.yml`:**
  * `SHELLCHECK_VERSION: v0.11.0` and `SHELLCHECK_SHA256: 8c3be12b…7198`;
  * a `curl` of the release asset into `$RUNNER_TEMP`, then
    `sha256sum -c -`, then `tar -xJf`, and the pinned binary first on
    `PATH`, printed with `shellcheck --version`;
  * then `shellcheck scripts/*.sh`, markdownlint and actionlint as before.
* **`docs/architecture.md`** records the pinned v0.11.0 and why it is
  first on `PATH`.

**Proof.**

* **The version difference, reproduced.** The v0.10.0 darwin binary was
  downloaded to scratch (the release publishes no digest for that asset;
  the local `shasum` is recorded only for identification).
  * v0.10.0 on the old script: exit 1, `SC2015`. On the new script: exit 0.
  * v0.11.0 (local) on both: exit 0.
  * v0.10.0 on all of `scripts/*.sh` now: exit 0.
* **The checksum step.** Checked on this host with `shasum -a 256 -c -`, the
  equivalent of CI's `sha256sum -c -`, on the downloaded linux archive:
  * the pinned digest gives `OK`, exit 0;
  * an all-zero digest gives `FAILED … did NOT match`, exit 1, which fails
    the step under the runner's `bash -e -o pipefail`.
* **Gates.**
  * `shellcheck` 0.11.0: 0. The script tests and both checkers: 0, and the
    expressions checker on `ci.yml`: 0.
  * The YAML parses. `actionlint` v1.7.12: 0. `markdownlint-cli2@0.23.2`: 0.
  * `make pre-add-check`: `52 file(s) clean`.
  * Windows gate: `overall=0`, `cleanup ok`.
* ~~**Pending:** the push's CI on three operating systems, recorded below
  when it finishes. This PLAN stays `in-progress` until then.~~ Done;
  see below.

Commit: `efe1e80`, pushed after the disclosure guard (exit 0):
`a7d5f01..efe1e80 main -> main`.

**CI run `36728547077`: `success`.**

* `validate (macos-15)` and `validate (windows-2025)` pass.
* `validate (ubuntu-24.04)` passes. Read from the job's full raw log:
  `gh run view --log` and the jobs API returned only 7 of its step groups,
  and the API showed steps as `in_progress` on a completed job. The full
  log has 424 lines, all 10 `Run` groups, and no `##[error]`:
  * `go test` and `go test -race -count=1`: `ok …/selfupdate`;
  * `make lint`: `0 issues.` for each of the three targets;
  * `govulncheck`: `No vulnerabilities found.`;
  * `…/shellcheck-v0.11.0.linux.x86_64.tar.xz: OK`, `version: 0.11.0`, and
    `markdownlint-cli2`: `Summary: 0 issues in 0 files`;
  * the verifier test: `all fixtures passed`; the refuse test:
    `8 passed, 0 failed`;
  * `check-workflow-gh-repo: ok`, and its test `6 passed, 0 failed`;
  * `check-workflow-expressions: ok` (twice: both workflows), and its test
    `4 passed, 0 failed`.
* The first CI run of the Phase 6 steps (`36726867210`, on `a7d5f01`)
  failed as D5 records. This run is the first green one.

This PLAN is `complete`. 0002-PLAN Phase 6 steps 3–5 (tag `v1.0.0`, tag CI,
proxy check, the guide's SHA) wait for the owner's ask to tag.
