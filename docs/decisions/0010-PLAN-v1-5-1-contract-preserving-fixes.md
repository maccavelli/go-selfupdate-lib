---
status: proposed
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
8. **A4.** `NotBefore` is clamped to [now + 1 min, now + 1 h], and a stored
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

Not started.
