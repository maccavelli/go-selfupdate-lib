---
status: complete
date: 2026-10-03
associated-madr: "0010-MADR-remediate-second-debugging-pass-findings.md"
---
# Implement v1.5.1: the fixes that keep every documented contract

Associated MADR: [0010-MADR-remediate-second-debugging-pass-findings.md](0010-MADR-remediate-second-debugging-pass-findings.md)

## Goal

* Every finding in the MADR's §2 is fixed. Each fix comes with a regression
  test that fails on `48653e4`.
* `make apicheck` reports `compatible with v1.5.0`. No exported name is
  added, removed or changed.
* `v1.5.1` is tagged and published. prepare-commit-msg takes it with no code
  change.

## Scope

### In scope

| Phase | Findings | Main paths |
| :--- | :--- | :--- |
| P1 | A1, A8, A12, A13 | `manifestverify.go`, `updater.go`, `checksums.go`, `scripts/selfupdate_manifest.py`, `github.go`, `checker.go` |
| P2 | A2, A4, A5, A6, A7, A9, A10, A11, C10 | `github.go`, `credentials.go`, `checkcache.go` |
| P3 | B1, B2, B7, B8, B9, B10, B13, A15 | `probe.go`, `target.go`, `session.go`, `replace*.go`, `lock_*.go`, `verify.go`, `updater.go` |
| P4 | B4, C5, and the managed test gaps | `managed.go` |
| P5 | B5, B6 | `cleanup_windows.go`, `standalone.go`, `session.go`, `replace_windows.go` |
| P6 | C3, C4, C6, D12, the C12 test gaps | `selfupdate/cli/run.go`, `cli/command.go`, `updater.go`, `types.go` |
| P7 | the release | the execution record, `README.md`, the migration guide |

### Out of scope

* **Contract changes** (MADR §3): A3, B3, B11, B12, C2, C7, C9, C11, A14.
  They belong to
  [0010-PLAN-v1-6-0-owner-contracts.md](0010-PLAN-v1-6-0-owner-contracts.md).
* **Tooling,** which belongs to
  [0010-PLAN-tooling-fixes.md](0010-PLAN-tooling-fixes.md). This PLAN runs
  after it, so the `v1.5.1` tag carries T4.
* **Push and tags,** which are the owner's.

## Rules for every phase

1. **Test first.** Port the reviewer's probe into a committed regression
   test, and see it fail on the unfixed code. Then fix, and see it pass.
   Record both outputs.
2. **No API change.** `make apicheck` stays `compatible with v1.5.0` after
   every phase.
3. **Checks before each commit:**
   * `make pre-add-check` on the changed Go files;
   * `make lint` (three GOOS);
   * `go test -race -count=1 ./...`;
   * `go test -shuffle=on -count=2 ./...`;
   * `make fuzz`;
   * `make vuln`;
   * `go mod tidy -diff`.
4. **Commits.** One per phase, with `git commit --no-edit`, after the owner
   authorizes commits to `main` in that turn.
5. **Windows.** P3, P4 and P5 also pass on the Windows test host, because
   they change install paths.

## Implementation Steps

### Phase P1: integrity

1. **A1.** `checkedAsset.Read` compares the digest on the read that brings
   `n` to `size`, returning `ErrIntegrity` with the bytes. `Close`
   reports `ErrIntegrity` when the body was fully read but never compared.
   Tests: `io.ReadFull` and `io.CopyN` of exactly `Size` tampered bytes
   fail; `ReadAll` still fails as before; good bytes pass every way.
2. **A8.** `runVerifiers` wraps a verifier's error as
   `errors.Join(ErrIntegrity, err)`, as `runManifestVerifiers` does. Test:
   `errors.Is(err, ErrIntegrity)`, and `EventFailed.Detail` is
   `integrity`.
3. **A12.** `validateChecksumName` refuses `:` on every OS, and
   `scripts/selfupdate_manifest.py` does the same. A new differential case
   `a:b` must agree. Test on the Windows host as well.
4. **A13.** Killing tests for the four surviving mutations:
   * the release id or tag missing, in `validateFetchedRelease` and in
     `validateReleaseStructure`;
   * a redirect loop longer than `maxRedirects`;
   * a draft release from a custom `ReleaseSource`, on `Latest` and on
     `ByTag`.

   Re-run each mutation on a scratch copy; each must now be killed.

### Phase P2: network and credentials

1. **A2.** The source keeps an `ErrNoCredential` or refused outcome only for
   the current run. `Run`, `RunWith`, `Check` and each `Start` resolve again.
   Tests: a startup `Check`, then `Start`, prompts; a skipped prompt, then a
   second `Start`, prompts again.
2. **A7.** Resolution and refresh run outside `s.cred.mu`, behind a
   single-flight that waits with the caller's context. `checkRedirect`
   reads a snapshot of the header name. Test: a second request with a 100 ms
   deadline returns within it while the provider blocks.
3. **A6.** A 401 refreshes only when `resp.Request.URL` is on the API
   origin. Test: a 401 from the download origin does not call the provider.
4. **A5.** `do` rewrites a `*url.Error`'s URL to its scheme, host and path,
   dropping the query and fragment. Test: refused-redirect and
   connection-refused errors carry no query.
5. **A9.** `validateCredential` refuses any control byte except HTAB, as
   `httpguts` does. Test: `\x01` and DEL refused, and tab accepted.
6. **A10.** A plain-http loopback hop is allowed only when the API base is
   loopback. Test: HTTPS API to `http://127.0.0.1` refused; the loopback
   test server is unchanged.
7. **A11.** `ByTag` refuses `.` and `..`. Test: no request is sent.
8. **A4.** *(Deviation D1: the minute applies only when the headers give no future time.)* `NotBefore` is clamped to [now + 1 min, now + 1 h], and a stored
   value beyond the cap is a miss. Tests: a reset in 2200, a reset in the
   past, and no headers.
9. **C10.** The run sets `CredentialRequest.Interactive` when its context
   carries a Stream, and the field's comment is corrected. Test: a provider
   sees `true` under `Start`, and `false` under `Run`.

### Phase P3: filesystem and install

1. **B1.** `cappedBuffer` holds its `bytes.Buffer` in a named field. Test: a
   probed script printing 200 KiB, then the version, fails the probe, and
   memory stays within the cap (`Len() == maxProbeOutput`).
2. **B2.** `allowedRoots` skips the home directory when it is unset, is
   `/`, or does not exist, and fails only when no root remains that covers
   the target. Tests: the three home states, with and without an
   `AllowedRoots` that covers the target.
3. **B7.** `Apply` checks the directory after the rename, and undoes through
   `rollbackInRoot` on a swap. Test: B's swap probe, through `Apply`.
4. **B8, B9.** `Backup` is reported only while the file exists, and `Run`
   copies a non-empty `Backup` into `Result.PendingBackup` whatever
   `Applied` is. `rollbackReplacement` returns the rename's outcome apart
   from the sync error, so a completed rename sets `RolledBack`. Tests:
   B's two probes.
5. **B10.** `Commit` and `Rollback` on a closed session return
   `selfupdate: session is closed`. Test: B's probe.
6. **B13, A15.** Remove:
   * `installSession.policy`;
   * `acquireLock`'s unreachable `timeout == 0` branch;
   * `mapStatus`'s 2xx branch;
   * `OpenAsset`'s duplicate ID check;
   * `verifyManifest`, together with its tests, after checking that each
     behaviour they covered is covered elsewhere.

### Phase P4: managed install

1. **B4.** When `Start` has succeeded, `recover` calls `Stop` before the
   rollback. Test: lifecycle `[stop start health stop start health]`.
2. **C5.** `NewManagedInstallerFor` uses `isNil` for the `Lifecycle` and
   the `Reconciler`. Test: typed nils are refused.
3. **The managed test gaps.** Tests for:
   * a Transformer with `ManagedInstaller`, covering `Owns` and `Target`;
   * a failed rollback that reports the backup.

### Phase P5: Windows busy backups (B5, B6)

*(Deviation D2, 2026-10-03: the receipt holds a list. MADR amendment A1
records the mechanism; the steps below stand, carried out through it.)*

1. When the receipt's backup is still a running image (`isBusyRunningImage`)
   in `beginSession` and `CleanupPending`, the receipt is kept, and the
   session continues. The next `Begin` tries again.
2. With `KeepPrevious`, a busy `.previous` keeps the new backup under a
   receipt, and does not fail `Commit`.
3. **Tests,** Windows only:
   * a helper process runs the old binary from its pending backup name;
   * `Begin` and `CleanupPending` succeed while it runs;
   * the receipt is removed after it exits;
   * the same for a busy `.previous`.

   They run on the Windows test host and in CI's Windows job.

### Phase P6: coordinator and CLI

1. **C3.** When `finish` fails to write the result or the summary, `Run`
   returns the write error, without `ErrUpdateAvailable`. `Exit` prints it,
   and the code is 1. Tests: C's two probes.
2. **C4.** `Command` checks the updater and the `Options` before `Run`. A
   failure after flag parsing writes the `--json` result object. Tests: a
   nil updater and a negative timeout, under `--json`.
3. **C6.** The transformed-staging check always requires
   `sessOwns(sess, path)`. Test: C's same-basename probe.
4. **D12.** Correct the `Request.CurrentVersion` and `TargetVersion`
   comments.
5. **The C12 test gaps:**
   * `StdioOptions`;
   * a goroutine-leak check in `cli/run_test.go`.

### Phase P7: release

*(Deviation D3, 2026-10-03: the pin moves after the tag. Step 2 lands in
a commit after `v1.5.1`, pinned to the tag's own commit.)*

1. **Release notes** in this PLAN's execution record, one line per finding,
   with the behaviour a consumer may notice:
   * B2: services without `$HOME` can now update;
   * A2: prompts after a startup check;
   * C3: exit 1, not 10, on a failed result write.
2. `README.md` and the migration guide pin the reusable workflow to the
   `v1.5.1` commit, which carries the tooling PLAN's T4.
3. **The owner** pushes, waits for CI on `main`, tags `v1.5.1` (annotated),
   and pushes the tag.
4. **The agent** checks:
   * CI on the tag;
   * the proxy's `@latest` is `v1.5.1`;
   * a scratch consumer builds;
   * on a scratch clone of prepare-commit-msg, `go get …@v1.5.1` and `make
     verify` pass with no code change.

## Verification

* **V1.** Each finding has a test that failed on `48653e4` and passes now.
  The output is recorded.
* **V2.** `make apicheck` reports `compatible with v1.5.0` at every phase.
* **V3.** All of rule 3's checks pass, and the Windows test host passes P3–P5.
* **V4.** CI is green on `main` and on `v1.5.1`.
* **V5.** prepare-commit-msg builds and verifies on `v1.5.1` unchanged.

## Rollout and Rollback

* **Rollout.** Phases land on `main`; the owner tags `v1.5.1`.
* **Rollback.** Before the tag, any phase reverts alone. After the tag, fix
  forward in `v1.5.2`.

## Execution Record

### Phase P1: integrity, A1, A8, A12, A13 (2026-10-03)

* **Approval.** The owner approved the three PLANs and authorized commits
  ("plans are approved. proceed and you have explicit permissions to
  commit"). The tooling PLAN ran first, T0–T8. This PLAN is now
  `in-progress`.
* **Tests first.** Against the unfixed code:

  ```text
  --- FAIL: TestChecksumNameRefusesColon      "a:b": err = <nil>, want ErrIntegrity (and "C:x", "x:", via ParseSHA256SUMS too)
  --- FAIL: TestOpenAssetChecksDigestAtSize    ReadFull of bad / CopyN of bad / a byte at a time of bad: err = <nil>, want ErrIntegrity true
  --- FAIL: TestVerifierFailureIsIntegrity     err = selfupdate: demo: signature does not verify, want ErrIntegrity …
  ```

  `ReadAll` of the tampered asset already failed, which was the old check.
  The A13 tests (`TestReleaseIdentityRequired`, `TestRedirectCap`, and the
  `draft` and `draft by tag` checker rows) pass on the existing checks.
  They exist to kill deletions.
* **A1.** `checkedAsset` compares the digest on the read that reaches the
  advertised size, once. A mismatch there returns no bytes, with
  `ErrIntegrity`: `io.ReadFull` discards an error that comes with a full
  buffer, so returning the bytes with the error would have hidden it.
  The PLAN's `Close` check is not needed: with the comparison at `Size`, a
  body that was fully read but never compared cannot occur.
* **A8.** `runVerifiers` returns `errors.Join(ErrIntegrity, err)`, as
  `runManifestVerifiers` does. `EventFailed.Detail` is `integrity`.
* **A12.** `validateChecksumName` refuses `/`, `\` and `:` on every OS, and
  no longer calls `filepath`. `scripts/selfupdate_manifest.py` refuses `:`
  too. The differential's bad-name list gains `a:b` and `C:x`.
* **A13.** On scratch copies, each mutation that survived the review is now
  killed:

  ```text
  M1 fetched-release identity check deleted: KILLED rc=1 by ['TestReleaseIdentityRequired']
  M3 listed-release identity check deleted: KILLED rc=1 by ['TestReleaseIdentityRequired']
  M2 redirect cap disabled: KILLED rc=1 by ['TestRedirectCap']
  M10 draft refusal removed from discover: KILLED rc=1 by ['TestCheckerAvailability']
  ```

* **Checks** (`gate.sh`, every one rc 0):
  * gofmt;
  * `make lint` (3 GOOS, 0 issues);
  * `go vet`;
  * `go test -race -count=1 ./...`;
  * `go test -shuffle=on -count=2 ./...`;
  * `go mod tidy -diff`;
  * `make apicheck`: `compatible with v1.5.0`;
  * `make fuzz`: `5 fuzz targets ran clean`;
  * `make vuln`: `No vulnerabilities found.`;
  * every `scripts/*_test.sh`;
  * cross `go vet` (freebsd, openbsd, linux/386, windows).

  `TestManifestDifferential` passes with the new names.

### Phase P2: network and credentials, A2, A4–A7, A9–A11, C10 (2026-10-03)

* **Tests first.** `network_hardening_test.go` (internal) and
  `credentials_run_test.go` (external) hold one test per finding, ported
  from the reviewers' probes into assertions. Against the unfixed code,
  every one failed:

  ```text
  TestNotBeforeClamped                  reset in 2200: deferred 1518708h0m0s, want 1h0m0s; reset in the past: deferred -2m0s, want 1m0s
  TestRedirectErrorHidesQuery           error echoes the redirect query: Get "http://downloads.example.invalid/blob?X-Amz-Signature=SECRET1#frag": …
  TestForeign401DoesNotRefresh          the provider was asked 2 times, want 1
  TestCredentialWaitHonoursContext      second Latest returned after 2.001453666s … context deadline exceeded, want its own deadline
  TestCredentialRefusesControlBytes     "tok\x01en" accepted (and DEL, ESC)
  TestLoopbackRedirectOnlyFromLoopback  a remote API's redirect to plain-http loopback was followed
  TestByTagRefusesDotSegments           ByTag("..") = … github http 404 …; 2 requests were sent
  TestPromptAfterStartupCheck           later Start: prompts=0 applied=false err=… github http 401
  TestPromptAgainAfterSkip              second Start: prompts=0 applied=false err=… github http 401
  TestCredentialRequestInteractive      viaStart=true: Interactive = false
  ```

  The first version of `TestCredentialWaitHonoursContext` released the
  blocking provider only after the second request returned. On the old
  code that is a deadlock: the run hit `go test`'s 10-minute timeout. A
  `time.AfterFunc` now releases the provider after 2 s, whatever happens,
  so the old code fails the test instead of hanging it.
* **A2, A7: credential state.**
  * A resolved credential is shared for the source's life, as before.
    Anonymity (`anonRun`) and the one 401 retry (`retriedRun`) are kept
    only for the run that decided them.
  * `run.execute`, `Checker.Check` and `CheckCached` each put a fresh
    `runMark` in their context. A source used outside any run (nil mark)
    keeps both for its lifetime, as before.
  * The provider runs with no lock held. One resolution is in flight at a
    time, behind an `inflight` channel. A waiter selects on that channel
    and on its own context.
* **A6.** The refresh runs only for a 401 whose `resp.Request.URL` is on the
  API origin.
* **A5.** `do` rewrites a `*url.Error`'s URL with `redactURL`: no user
  information, query or fragment.
* **A9.** `validateCredential` refuses every control byte but HTAB.
* **A10.** A plain-http hop is allowed only when both it and the API base
  are on loopback.
* **A11.** `ByTag` refuses `.` and `..` before any request.
* **C10.** `resolve` and `refresh` set `Interactive` from the run's Stream.
  The field's comment no longer says "Phase 1 sources always set it false".
* **A4.** `NotBefore` is capped at now + 1 h. A stored record whose
  `NotBefore` is beyond the cap is a miss.

**Deviation D1 (2026-10-03): A4's one-minute floor.**

* **Found.** The step says to clamp to [now + 1 min, now + 1 h]. The
  existing `TestCheckCachedRateLimitRetryAfter` (`checkcache_test.go:121`)
  asserts that a server's explicit `Retry-After: 30s` defers by 30 s, the
  documented "later of reset and Retry-After". The floor broke it.
* **Decision.** The owner chose "Floor only when no future time". The
  headers are honoured when they give a time in the future. The minute
  applies only when they give none, or one already past: the clock-skew
  case A4 found. The one-hour cap stays. The existing test is unchanged.
  Step P2.8 is annotated.

**Checks:**

* `go test -race -count=5` over the credential, prompt, stream, redirect,
  `ByTag`, `NotBefore` and `CheckCached` tests: rc 0, no data race.
* `gate.sh`, every check rc 0. The first run failed lint on
  `m, _ := ctx.Value(runKey{}).(*runMark)` (errcheck, blank assertion
  result); it was rewritten with `ok`. After that:
  * `make lint`: 0 issues;
  * race and shuffle;
  * `make apicheck`: `compatible with v1.5.0`;
  * fuzz, vuln, tidy;
  * every script test;
  * cross vet.

### Phase P3: filesystem and install, B1, B2, B7–B10, B13, A15 (2026-10-03)

* **Tests first.** `install_regression_test.go` ports reviewer B's probes
  into assertions. It adds the swap helper the probes used. Against the
  unfixed code:

  ```text
  TestProbeOutputCapped                      buffered 4194304 bytes through io.Copy, want the cap 65536
  TestHomeUnavailableRoot                    refused although AllowedRoots covers the target: … lstat …/no-such-home / locate home directory: $HOME is not defined / filesystem root is not an allowed self-update root
  TestApplyUndoesSwap                        Apply err = <nil>, want ErrConcurrentUpdate
  TestRunReportsUnrestoredBackup             PendingBackup = "", backups left = [.demo.selfupdate-bak-…]: the surviving backup must be reported
  TestProbeRollbackSyncFailure               RolledBack=false Backup="…/.demo.selfupdate-bak-…", want rolled back and no backup named
  TestClosedSessionRefusesCommitAndRollback  Rollback after Close = <nil>
  ```

* **B1.** `cappedBuffer` holds its buffer in a named field. `Len` and
  `String` are explicit, so `io.Copy` has no `ReadFrom` to bypass the cap.
* **B2.** `allowedRoots` skips a home directory that is unavailable or
  unusable. It fails only when no root is left, with `no allowed
  self-update root: …`. An explicit `AllowedRoots` entry must still be
  valid.
* **B7.** `Apply` checks the directory after the rename, as `Install` does,
  and undoes the swap through `rollbackInRoot`. If the undo fails, the
  backup is returned with the error. The managed path's refused `Commit` is
  P4's.
* **B8.**
  * After a swap whose undo fails, `Install` returns `Applied: false` with
    `Backup`, like the restore failure at `session.go:126`. `Run` reports
    it as `PendingBackup`, with its "kept at" message.
  * When the commit's cleanup fails, `Run` reports `Backup` as
    `PendingBackup` while the file exists.
* **B9.** `rollbackReplacement`, on Unix and Windows, marks a sync failure
  after a successful restore rename with `errRestoredUnsynced`.
  `probeInstalled` treats that as rolled back: `RolledBack`, no `Backup`,
  and the sync error still reported.
* **B10.** `Commit` and `Rollback` on a closed session return `selfupdate:
  session is closed`.
* **B13.** `installSession.policy` is removed, and so is `acquireLock`'s
  `timeout == 0` branch on Unix and Windows. No test calls `acquireLock` or
  `beginSession` directly, and `NewStandaloneInstaller` maps 0 to
  `DefaultLockTimeout`.
* **A15.**
  * `OpenAsset`'s ID check is removed: `validateAssetStructure`, reached
    through `validateAssetMetadata`, already refuses `ID <= 0`.
  * `mapStatus`'s 2xx branch is removed; every caller filters 2xx.
  * `verifyManifest` is removed. Its test becomes `TestManifestTestdata`,
    which reads the same testdata through `ParseSHA256SUMS` and
    `checksumFor`. A mismatch stays covered by `TestVerifyIntegrity`.
  * `TestEqualDigestLength` covers `equalDigest`'s length branch, which had
    no test.
* **The Windows test host** (the owner named its SSH alias; it is not
  recorded). The working tree was copied with `COPYFILE_DISABLE=1 tar`.
  * The first run failed in `cli`'s `TestNoStdoutOutsideStdio`, which
    parses `*.go`: macOS tar had added AppleDouble `._*.go` files. That
    came from the copy, not the code.
  * Re-run without them: `go vet` rc 0, and `go test -race -count=1 ./...`
    rc 0 for `buildinfo`, `selfupdate` (45.4 s), `cli` and
    `selfupdatetest`.
  * The temp directory was removed after each run.
* **Checks** (`gate.sh`, every one rc 0):
  * `make lint`, 0 issues;
  * race and shuffle;
  * `make apicheck`: `compatible with v1.5.0`;
  * fuzz, vuln, tidy;
  * every script test;
  * cross vet.

### Phase P4: managed install, B4, B7 (managed), C5, test gaps (2026-10-03)

* **Tests first.** `managed_regression_test.go`. Against the unfixed code:

  ```text
  TestManagedStopsBeforeRollback   lifecycle "stop start health start health", want stop start health stop start health
  TestManagedCommitSwapRecovers    err = selfupdate: target directory changed during the update: selfupdate: concurrent update, want a managed failure from the concurrent update
  TestManagedRefusesTypedNil       a typed nil Lifecycle was accepted
  ```

  `TestManagedWithTransformer`, the test gap, passes on both. It runs a
  Transformer through `Run` with a `ManagedInstaller`: the session owns the
  transformed staging, and the service is stopped, updated and started
  once each.
* **B4.** `recover` takes `stopFirst`. The path where `Start` succeeded and
  the health check failed stops the new binary before the receipt restore
  and the rollback. The test also records the target at each `Stop`: the
  second `Stop` sees `new-bytes`, before the rollback.
* **B7, managed.**
  * A `Commit` refused with `ErrConcurrentUpdate` goes through `recover`
    (stop, restore, roll back, start). Any other commit error is a cleanup
    failure after a healthy update, and is returned as it was.
  * `installSession.Rollback` undoes through the locked directory's handle
    when the directory has changed since `Begin`, as `Install` already
    does.
* **C5.** `NewManagedInstallerFor` uses `isNil` for the `Lifecycle` and
  the `Reconciler`.
* **Two existing tests asserted the defects, and were brought to the
  fixed contract.** No assertion was loosened:
  * `TestManagedRollsBackCustomSession`: the expected call order gains the
    `Stop` before `Restore` (B4).
  * `TestManagedCommitRefusesMovedDirectory` expected the refused commit to
    stay `Applied` with the new binary live. It now expects a rollback, and
    reads the old binary back in the moved directory (B7). Its swap hook
    runs once: the recovery checks health again, and the second swap failed
    with `file exists`.
* **Windows test host:** `go vet` rc 0. `go test -race -count=1 ./...` rc 0
  for all four packages; `selfupdate` took 43.3 s.
* **Checks** (`gate.sh`, every one rc 0):
  * `make lint`, 0 issues;
  * race and shuffle;
  * `make apicheck`: `compatible with v1.5.0`;
  * fuzz, vuln, tidy;
  * every script test;
  * cross vet.

### Deviation D2 (2026-10-03): the cleanup receipt holds a list

* **Found,** before any P5 code was written. P5 step 1 keeps a busy
  receipt and lets `Begin` continue. But `writeCleanupReceipt` creates the
  target's one receipt, `.<base>.selfupdate.cleanup` (`cleanup.go:9`), with
  `O_EXCL` (`cleanup_windows.go:135`). The next update's commit, whose
  backup links the running image, could then write no receipt. The binary
  would be replaced, `Commit` would fail, and that backup would leak. Step 2
  (B6) needs the same "more than one pending backup".
* **Decision.** The owner chose "Receipt holds a list". MADR amendment A1
  records it:
  * a version 2 receipt with `backups`, written as version 1 when it holds
    one entry;
  * processing keeps the busy entries;
  * a commit adds its backup to the list, through an atomic rewrite.
* **Files.** As carried out, the phase changes `cleanup_windows.go`,
  `session.go` and `replace_windows.go`, and adds `keepAsPending` to
  `replace_unix.go`, where it is a stub that keeps nothing. `standalone.go`
  is unchanged. So are `commitReplacement`'s signature and the existing
  Windows receipt tests: `writeCleanupReceipt` adds to the receipt it finds,
  so the callers did not change.

### Phase P5: Windows busy backups, B5, B6 (2026-10-03)

* **Tests first.** `cleanup_busy_windows_test.go`, Windows only. Its
  helper `holdBusy` opens a file without `FILE_SHARE_DELETE`, as a running
  image is held. Against `HEAD`'s code with the new tests:

  ```text
  TestWindowsBusyPendingBackupKept        a busy pending backup failed the session: selfupdate: remove pending backup: … being used by another process.
  TestWindowsReceiptKeepsOnlyBusy         process: selfupdate: malformed cleanup receipt
  TestWindowsCommitAddsToPendingReceipt   commit with a pending receipt: selfupdate: remove backup: …
  TestWindowsKeepPreviousBusy             Install with a busy .previous: selfupdate: keep previous: Access is denied.
  TestWindowsKeepPreviousRunningPrevious  install SECOND: … err = selfupdate: keep previous: Access is denied.
  ```

  Step 3 names a running program. `TestWindowsKeepPreviousRunningPrevious`
  runs the test binary as one, and two updates with `KeepPrevious` make its
  image the `.previous` and then try to replace it. For B5,
  `TestE2EUpdateRunningCopy` already ran the old program from its pending
  backup. Its Windows branch asserted the defect: `CleanupPending` had to
  fail with "remove pending backup" while the old program ran. It now
  requires `nil`, with the receipt and the backup both kept, and still
  requires both gone after the program exits and `CleanupPending` runs
  again. `CleanupPending` goes through `beginSession`, so this covers
  `Begin` too.
* **B5.** `processCleanupReceipt` keeps every entry whose backup a running
  image holds (`isBusyRunningImage`), and removes the rest:
  * it returns `nil` when every entry is kept;
  * it removes the receipt when none is;
  * otherwise it rewrites the receipt with the kept entries.
* **Amendment A1.**
  * `cleanupReceipt` reads version 1 (`backup`, `digest`) and version 2
    (`backups`), and writes version 1 when it holds one entry.
  * `writeCleanupReceipt` adds to the receipt it finds.
  * `writeCleanupEntries` writes a temporary sibling, restricts it to the
    current user, and moves it over the receipt.
* **B6.** `commitLocked` calls `keepAsPending` when the keep-previous
  rename fails. On Windows, a busy `.previous` puts the new backup on the
  receipt and returns it as `PendingBackup`, and the commit stands. Any
  other error fails `Commit` as before. On other platforms the stub keeps
  nothing.
* **Mistake in my own test.** The first draft of
  `TestWindowsKeepPreviousRunningPrevious` passed one `Target` to both
  updates. `Begin` refused the stale target ("target changed during
  confirmation"), on both trees. The test now resolves the target before
  each update, as a caller does.
* **Windows test host:**
  * the five new tests, `TestWindowsReceiptConsumedByBegin`,
    `TestKeepPreviousRunningImage` and `TestE2EUpdateRunningCopy` pass, run
    verbose;
  * `go vet ./...` rc 0;
  * `go test -race -count=1 ./...` rc 0 for all four packages;
    `selfupdate` took 48.4 s.
* **Checks** (`gate.sh`, every one rc 0):
  * `make lint`, 0 issues;
  * race and shuffle;
  * `make apicheck`: `compatible with v1.5.0`;
  * fuzz, vuln, tidy;
  * every script test;
  * cross vet.

### Phase P6: coordinator and CLI, C3, C4, C6, D12, C12 (2026-10-03)

* **Tests first.** Against `HEAD`'s code with the new tests, in a scratch
  copy:

  ```text
  TestTransformSameBasenameRequiresStagingOwner  Run = <nil>, want the ownership refusal
  TestCommandJSONRefusedOptions                  nil updater: exit 1, stdout ""
                                                 negative timeout: exit 1, stdout ""
  TestCheckResultWriteFails                      json: err selfupdate: update available; exit 10, want 1; stderr ""
                                                 text: err selfupdate: update available; exit 10, want 1
  ```

  `TestRunsLeakNothing` and `TestStdioOptions` cover the C12 gaps, so they
  pass on both trees. Each was seen to fail on a planted defect, in a
  scratch copy:
  * a goroutine that outlives the test: `goroutines: 3 now, 2 when the
    test began`;
  * `StdioOptions` returning `os.Stderr` as `Stdout`: `streams …, want the
    process's own`.
* **C3.** When `finish` cannot write the result object or the summary, and
  the run's error is `ErrUpdateAvailable`, `Run` returns the write error
  alone, so the exit code is 1 and `Exit` prints it. Any other run error
  is still joined with the write error.
* **C4.** `Command` calls `Options.check` on the built updater before
  `Run`, and a refusal goes through `report`, which writes the `--json`
  result object. `Run` still checks for its own callers.
* **C6.** `hashAndValidateStaging` requires `sessOwns` for every
  transformed staging path; the basename exception is removed. Nothing
  recorded a reason for it; it came with the original move from mcplib.
  `TestTransformRequiresStagingOwner` and `TestManagedWithTransformer`
  still pass.
* **D12.** The `Request.CurrentVersion` and `TargetVersion` comments name
  the configured `VersionPolicy`, and the `ChannelPolicy` rule for a
  pinned prerelease.
* **C12.**
  * `TestStdioOptions`: the process's streams, `Interactive` exactly when
    standard input is a terminal, and every other option at its default.
  * `checkNoLeak` in `cli/run_test.go`, used by `TestRunsLeakNothing`. It
    covers runs with a prompt confirmer (yes, no, not interactive, text and
    JSON), a timed-out run and a cancelled one.
* **Mistakes in my own tests.**
  * `failOn` first embedded `bytes.Buffer`, so `io.WriteString` used the
    buffer's `WriteString` and never reached the failing `Write`. The text
    case passed the summary through on the fixed tree. The buffer is now a
    named field.
  * An empty `marker` matched every write. A writer with no marker now
    fails nothing.
* **Found, out of scope.** The `VersionPolicy` comment (`types.go`) still
  says it "validates and compares strict stable release tags", the same
  staleness as D12. D12 names only the `Request` comments, so it is left
  for the owner.
* **Windows test host,** not required for this phase: `go vet ./...` rc 0,
  and `go test -race -count=1 ./...` rc 0 for all four packages.
* **Checks** (`gate.sh`, every one rc 0):
  * `make lint`, 0 issues;
  * race and shuffle;
  * `make apicheck`: `compatible with v1.5.0`;
  * fuzz, vuln, tidy;
  * every script test;
  * cross vet.

### Deviation D3 (2026-10-03): the workflow pin moves after the tag

* **Found.** Step 2 pins `README.md` and the migration guide to "the
  `v1.5.1` commit". A commit cannot contain its own hash, so the pin cannot
  sit in the commit that is tagged. Both pin `58411f1` (`v1.4.1`) today.
  `v1.5.0`'s tree pins it too. That workflow lacks the tooling PLAN's T4
  and the empty-binary refusal.
* **Decision.** The owner chose "Pin after the tag":
  * the P7 commit holds these release notes;
  * the owner tags it `v1.5.1` and pushes;
  * a commit after the tag moves both pins to the tag's commit, labelled
    `# v1.5.1`, and the owner pushes it.
* **Consequence.** The `v1.5.1` tree shows the `v1.4.1` pin, as `v1.5.0`'s
  did. `main` shows the `v1.5.1` pin from the next commit. The MADR's "D5
  and D9 ship when the README's pin moves to the commit that carries them"
  still holds, and is unchanged.

### Phase P7: release notes for `v1.5.1` (2026-10-03)

No exported name changes (`make apicheck`: `compatible with v1.5.0`). What
a consumer may notice, by finding:

* **Integrity.**
  * A1: a body whose digest is wrong fails on the read that completes it,
    with `ErrIntegrity`, not later or never.
  * A8: a verifier's failure is wrapped in `ErrIntegrity`.
  * A12: a checksum name with `:` is refused on every OS.
* **Network and credentials.**
  * A2: a credential provider that found nothing, or was refused, is asked
    again on the next run. A program that checked at startup now prompts
    when the user runs an update.
  * A4: `CheckCached` defers at most 1 h, and at least 1 min only when the
    response gave no time in the future.
  * A5: an error's URL carries no query string.
  * A6: only a 401 from the API origin refreshes the credential.
  * A7: a slow credential provider no longer holds the source's lock.
  * A9: a credential with a control byte other than a tab is refused.
  * A10: a plain-http redirect to loopback is followed only when the API
    itself is on loopback.
  * A11: `ByTag` refuses `.` and `..`.
  * C10: `CredentialRequest.Interactive` is set when the run carries a
    stream.
* **Install.**
  * B1: probe output is capped on every write path.
  * B2: a service with no usable `$HOME` can now update, when another
    allowed root covers its target.
  * B4: a managed update whose health check fails stops the new binary
    before rolling back.
  * B5: on Windows, a pending backup that a running program still holds no
    longer fails `Begin` or `CleanupPending`. It stays on the cleanup
    receipt until a later run can remove it.
  * B6: on Windows, with `KeepPrevious`, a `.previous` that a running
    program holds no longer fails `Commit`. The new backup is reported as
    `PendingBackup` instead of `Previous`.
  * Amendment A1: the cleanup receipt can list several backups. It is still
    written as version 1 when it lists one. A release before `v1.5.1`
    refuses a receipt that lists two.
  * B7: a directory moved during an update is detected after the rename,
    and a managed `Commit` refused for it rolls back.
  * B8, B9: `Backup` is reported only while the file exists, and
    `PendingBackup` whenever one is left.
  * B10: `Commit` and `Rollback` on a closed session return an error.
* **Coordinator and CLI.**
  * C3: a `--check` whose result object or summary cannot be written exits
    1, not 10, and prints the error.
  * C4: under `--json`, a nil updater or a refused option now writes the
    result object.
  * C5: a typed-nil `Lifecycle` or `Reconciler` is refused.
  * C6: a Transformer with a custom session that is not a `StagingOwner`
    always fails before `Install`, whatever the staging file is named.
* **Documentation and cleanup.** D12 corrects the `Request` comments. A15
  and B13 remove dead code. Nothing a consumer can see changes.
* **Tests only.** A13's surviving mutations are killed, and the B14 and
  C12 gaps are covered.

### Phase P7: tag, pin and checks (2026-10-04)

* **Push and tag.** The owner asked the agent to push and tag, so the agent
  did step 3:
  * `main` to `c7a8b4c`, with CI green on all three runners;
  * `scripts/check-release-tag.sh v1.5.1` exit 0;
  * the annotated tag `v1.5.1` on `c7a8b4c`, with message `v1.5.1`, as
    `v1.5.0`'s was. `git ls-remote origin 'refs/tags/v1.5.1^{}'` gives
    `c7a8b4c`.
* **Step 2, after the tag (D3).**
  * `README.md` and the migration guide pin
    `publish-selfupdate-release.yml@c7a8b4ca8045bdb26b0908206b775192358c8253 # v1.5.1`.
  * The guide said the workflow was "unchanged from `v1.3.0` to
    `v1.4.1`", with a `v1.4.1` `ls-remote` command. It now says it is
    unchanged up to `v1.5.0`, names what `v1.5.1` changes, and resolves
    `v1.5.1`. Between `v1.4.1` and `v1.5.0` only the API-check scripts
    changed.
* **Step 4.**
  * **CI on the tag:** green on ubuntu-24.04, macos-15 and windows-2025.
  * **The proxy:** `go list -m …@latest` against `proxy.golang.org` gives
    `v1.5.1`.
  * **A scratch consumer** builds on `v1.5.1` and runs `cli.Command` with
    `StdioOptions` offline:
    * `--check` exits 10 with the summary;
    * `--check --json` exits 10 with one result object;
    * `--yes` applies, exit 0;
    * cross builds for linux and windows pass.

    My first two drafts left out the GitHub source's `Client` and the
    `Reporter`, which `New` requires. The second run showed C4 in the
    published module: under `--json` the updater's error came out as one
    result object.
  * **prepare-commit-msg**, on a scratch clone: `go get …@v1.5.1` changes
    only `go.mod` and `go.sum`, and `make verify` passes, with total
    coverage 84.1 %. No code change was needed.
* **Checks** on the pin commit (`gate.sh`, every one rc 0). `make apicheck`
  now compares against the newest tag: `compatible with v1.5.1`.

**Verification.**
* V1: every finding's test failed on the unfixed code; the outputs are
  recorded per phase.
* V2: `make apicheck` stayed compatible with `v1.5.0` through P1–P7.
* V3: rule 3's checks passed at every phase, and the Windows test host
  passed P3–P5, and P6 too.
* V4: CI is green on `main` and on `v1.5.1`.
* V5: prepare-commit-msg builds and verifies on `v1.5.1` unchanged.

This PLAN is `complete`.
