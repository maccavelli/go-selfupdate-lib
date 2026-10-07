---
status: complete
date: 2026-10-03
associated-madr: "0010-MADR-remediate-second-debugging-pass-findings.md"
---
# Implement v1.6.0: the contracts the owner decided (Q1–Q6, A14)

Associated MADR: [0010-MADR-remediate-second-debugging-pass-findings.md](0010-MADR-remediate-second-debugging-pass-findings.md)

## Goal

* Each owner answer in the MADR (Q1–Q6), and A14, is implemented,
  documented, and tested.
* Every exported change is an addition. `make apicheck` reports
  `compatible with v1.5.1`. Each behaviour change is named in the release
  notes and in the guides.
* `v1.6.0` is tagged and published.

## Scope

### In scope

| Phase | Answer | Change |
| :--- | :--- | :--- |
| S1 | Q1 (A3) | `CheckCached` caches deterministic outcomes for `maxAge` |
| S2 | Q2 (B3) | an optional `EnabledLifecycle`; start a stopped service only when it is configured to start; `InstallResult.ServiceStarted` |
| S3 | Q3 (C2, C11) | one terminal event; `EventWarning` and `Result.Warnings` after `complete`; a dry run's own `Detail` |
| S4 | Q4 (C7) | `Request.Platform` other than the running platform refused on apply |
| S5 | Q5 (C9) | the zero value of `ConfirmNeeded` and `CredentialNeeded` is safe |
| S6 | Q6 (B11, B12) | crash-leftover cleanup under the lock; special mode bits refused unless allowed; owner and group carried over |
| S7 | A14 | `selfupdatetest.GitHubServer` records credential-bearing header names, and `RequireToken` takes a header name |
| S8 | — | docs, release notes, release |

### Out of scope

* **Anything in the other two PLANs.** This PLAN runs after
  [0010-PLAN-v1-5-1-contract-preserving-fixes.md](0010-PLAN-v1-5-1-contract-preserving-fixes.md)
  has released `v1.5.1`.
* **Reference `EnabledLifecycle` implementations** for systemd, launchd and
  SCM. They belong to the 0004 Phase 4 `selfupdate/service` packages.
* **Push and tags,** which are the owner's.

## Rules for every phase

The v1.5.1 PLAN's rules apply, with two differences:

* `make apicheck` must report `compatible with v1.5.1`. Additions are
  expected; nothing may be removed or changed.
* Each phase updates the godoc, `docs/guides/extending-selfupdate.md`, and
  `docs/architecture.md`, wherever they describe the behaviour it changes.

*(2026-10-07)* The session tools these records cite are now in the
repository: `gate.sh` as `scripts/gate.sh` (`make gate`), `doccheck.py` as
`scripts/check-docs.sh`, and `plantcopy.py` as `scripts/plant-copy.sh`
([0015-PLAN-remediate-third-debugging-pass-findings.md](0015-PLAN-remediate-third-debugging-pass-findings.md)
R1).

## Implementation Steps

### Phase S1: what `CheckCached` caches (Q1)

1. The cache record gains an outcome field. It caches, for `maxAge`:
   * a success;
   * `ErrLatestOlder`;
   * `ErrUnsupportedPlatform` (no asset for this platform);
   * `ErrMutableRelease`.

   A rate limit keeps its clamped `NotBefore`, from v1.5.1 A4. Every
   other error is not cached.
2. The schema version goes up. A record of an older schema is a miss,
   never an error.
3. **Tests:**
   * five calls in `maxAge` with `ErrLatestOlder` make one network call;
   * `CheckCached` returns the cached error class;
   * an older-schema file is a miss;
   * a transient error is not cached.

### Phase S2: a stopped service (Q2)

*(Deviation D1, 2026-10-04: `ServiceStarted` is also copied to `Result`
and to the JSON document; MADR amendment A3.)*

1. **The interface.** An exported optional interface:

   ```go
   // EnabledLifecycle is a Lifecycle that can report whether a service is
   // configured to start (systemd is-enabled, launchd RunAtLoad or
   // KeepAlive, Windows SCM automatic start).
   type EnabledLifecycle interface {
       Enabled(context.Context, string) (bool, error)
   }
   ```

2. **The rule in `managedSession.Install`:**
   * start after the replacement, and wait for health, only when the
     service was running, or the `Lifecycle` implements `EnabledLifecycle`
     and `Enabled` reports true;
   * otherwise the binary is replaced, reconciled and committed, and the
     service stays stopped;
   * an `Enabled` error fails the install before anything is stopped or
     replaced, as an `Installed` or `Running` error does.
3. `InstallResult.ServiceStarted` (new) reports whether the service was
   started.
4. `recover` restarts only what was running, or was started by this
   install.
5. **Tests:**
   * stopped and not enabled: lifecycle `[]` after the probes, not
     started;
   * stopped and enabled: `[start health]`;
   * running: `[stop start health]`, whatever `Enabled` says;
   * a `Lifecycle` without `Enabled`, stopped: not started;
   * an `Enabled` error: nothing is replaced.

### Phase S3: one terminal event (Q3)

*(Deviation D2, 2026-10-04: `Warnings` is an exported field of a
string-backed type whose JSON form is an array, not a `[]string`; MADR
amendment A4.)*

1. **`EventWarning`,** appended after `EventRolledBack` so every earlier
   value keeps its number. It is advisory: a reporter error on it is
   ignored.
2. **The rule.** Once `EventComplete` is reported, a later error is not a
   failure:
   * a failed `Close`;
   * a failed report of `complete`;
   * an installer that returned `Applied:true` with an error.

   Each error is reported as an `EventWarning`, and appended to
   `Result.Warnings` (new, `[]string`, sanitized). `Run` returns nil, and
   the exit code is 0.
3. **The dry run** reports `EventComplete` with the `Detail` `dry-run`, and
   `EventComplete`'s godoc says so (C11).
4. **The CLI** prints each warning to stderr as `warning: …`. Under
   `--json`, the result object carries `warnings`, the JSON schema version
   goes up, and `cli/doc.go` documents it.
5. **Tests:**
   * C's four probes give one terminal event, exit 0, and the warning in
     `Result.Warnings`;
   * a golden test for the `--json` result with warnings;
   * the `TextReporter` and JSON reporter goldens include
     `EventWarning`.

### Phase S4: `Request.Platform` (Q4)

1. `validateRequest` refuses, when neither `CheckOnly` nor `DryRun` is set,
   a `Platform` other than the running `GOOS`/`GOARCH`, with
   `ErrUnsupportedPlatform`.
2. The running-copy end-to-end tests set the platform through an internal
   test seam (`setSeam`), not through `Request.Platform`.
3. **Tests:** apply with a foreign platform is refused before any download;
   `CheckOnly` and `DryRun` with a foreign platform still work.

### Phase S5: the zero-value reply types (Q5)

1. `Answer`, `Supply` and `Cancel` on a zero value return at once and do
   nothing, through a `select` with a `default`. The godoc says so.
2. **Tests:** the zero values return within 10 ms, with no goroutine left.
   A real reply still delivers.

### Phase S6: leftovers and mode bits (Q6)

1. **Leftovers.** Under the lock, in `beginSession` and `CleanupPending`,
   remove the siblings of the target that match this product's staging
   (`.<base>.selfupdate-*`) or backup (`.<base>.selfupdate-bak-*`)
   prefixes. Skip:
   * the backup a receipt names;
   * `.<base>.previous`;
   * anything this session created;
   * anything that is not a regular file.

   Removal is best-effort. A failure is not an error, because a leftover
   is harmless and must never block an update, as B5 showed for a busy
   backup.
2. **Special bits.**
   * A target with setuid or setgid set is refused, unless the new
     `TargetPolicy.AllowSpecialModeBits` is true.
   * When allowed, staging copies the full mode, special bits included.
   * The sticky bit is carried over.
3. **Ownership.** On Unix, staging is given the target's uid and gid with
   `Lchown`, when permitted. `EPERM` is ignored, because an unprivileged
   updater owns what it writes. Other errors fail the install.
4. **Tests:**
   * leftovers removed;
   * a receipt's backup and `.previous` kept;
   * a symlink leftover not followed;
   * setgid refused, and allowed with the policy;
   * ownership carried over. That test skips unless it runs as root.
     ci.yml gains a Linux step that runs it with `sudo`.

### Phase S7: the test server (A14)

*(Deviation D3, 2026-10-04: `RequireCredential` and a `HeaderNames` field,
not `GitHubServerOptions` and a slice; MADR amendment A5.)*

1. `RecordedRequest` gains `CredentialHeaders []string`: the names, never
   the values, of the configured credential headers present.
   `GitHubServerOptions` gains `TokenHeader string`, which defaults to
   `Authorization` with `Bearer`.
2. **Tests:** a custom-header credential is required on the API, and is
   absent on the download origin.

### Phase S8: docs and release

*(Amended 2026-10-04, at the owner's answers when this PLAN started. The
pin moves after the tag, as the v1.5.1 PLAN's deviation D3 did: step 3
lands in a commit after `v1.6.0`. The docs step also corrects the
`VersionPolicy` comment, MADR amendment A2, D16.)*

1. **Docs:**
   * the guides and `architecture.md` describe each change;
   * the migration guide gains "From v1.5 to v1.6", covering `Warnings`,
     the platform refusal, the managed start rule and the special-bits
     policy;
   * D16: the `VersionPolicy` comment says the policy decides which tags
     it accepts.
2. **Release notes** in this PLAN's execution record.
3. After the tag, a separate commit moves the `README.md` and migration
   guide pins of the reusable workflow to the `v1.6.0` tag's commit,
   labelled `# v1.6.0`.
4. **The owner** pushes, waits for CI, tags `v1.6.0` (annotated), and pushes
   the tag.
5. **The agent** checks:
   * CI on the tag;
   * the proxy's `@latest`;
   * a scratch consumer builds;
   * prepare-commit-msg on `v1.6.0`, on a scratch clone, notes any change
     it needs. Moving it is a prepare-commit-msg record.

## Verification

* **V1.** Each phase's tests failed before the change, and pass after it.
* **V2.** `make apicheck`: `compatible with v1.5.1`, additions only.
* **V3.** The v1.5.1 PLAN's checks, and the Windows test host for S2, S4 and
  S6.
* **V4.** CI is green on `main` and on `v1.6.0`.
* **V5.** Every changed behaviour is named in the guides and in the release
  notes.

## Rollout and Rollback

* **Rollout.** Phases land on `main`; the owner tags `v1.6.0`. A consumer
  opts in with `go get`.
* **Rollback.** Before the tag, any phase reverts alone. After it, fix
  forward in `v1.6.1`.

## Execution Record

### Start (2026-10-04)

* `v1.5.1` is released (the v1.5.1 PLAN is `complete`), so this PLAN may
  run. The owner said "Proceed to the v1.6.0 plan", and the status is
  `in-progress`.
* **Amendments before S1,** at the owner's answers:
  * S8's pin moves after the tag, as the v1.5.1 PLAN's D3 did;
  * S8's docs step corrects the `VersionPolicy` comment (MADR amendment
    A2, D16).

### Phase S1: what `CheckCached` caches, Q1 (A3) (2026-10-04)

* **Tests first.** `checkcache_outcome_test.go`. Its behaviour tests use
  only the `v1.5.1` API, so they ran against the unfixed code:

  ```text
  TestCheckCachedDeterministicErrorCached  latest older, mutable release, unsupported platform:
                                           network calls 5, saves 0; want 1 and 1
  TestFileCheckStoreOlderSchemaIsMiss      an older record loaded: {Request:{Product:demo …
  ```

  `TestCheckCachedTransientErrorNotCached` guards what was already true,
  and passed on both trees. Four plants, in scratch copies, each made it
  fail:
  * `outcomeOf` caching every error;
  * the `context.Canceled` exclusion removed;
  * the `context.DeadlineExceeded` exclusion removed;
  * the `ErrRateLimited` exclusion removed.

  The first draft listed only a plain error and a plain deadline. Neither
  carries a sentinel, so removing the deadline exclusion went unseen. It
  now also lists a cancellation, a deadline and a rate limit, each joined
  with a deterministic sentinel.
* **The API, additions only.**
  * `CheckOutcome`, with `CheckAnswered` (the zero value),
    `CheckLatestOlder`, `CheckUnsupportedPlatform` and
    `CheckMutableRelease`. `String` gives the stable name, the same as
    EventFailed's Detail class, or `CheckOutcome(N)` for an unknown value.
    `Err` gives the sentinel.
  * `CheckRecord.Outcome`. A custom `CheckStore` written before `v1.6.0`
    returns the zero value, an answer, as it always meant. A record with
    an outcome this package does not know is a miss.
* **`CheckCached`.**
  * A deterministic error is saved with its outcome, `CheckedAt`, and an
    `Availability` holding only the product and the current version.
  * For `maxAge`, a cached one returns
    `selfupdate: <product>: cached check from <time>: <sentinel>`.
  * A rate limit, a cancellation, a deadline or any other error is not
    saved, as before.
* **The file.**
  * Schema 3 adds `outcome`, after `channel`.
  * Schemas 1 and 2 load as `ErrNoCheckRecord`, a miss. The first check
    after the upgrade goes to the network once.
* **Existing tests moved to the new contract.**
  * `TestFileCheckStoreReadsSchema1` asserted that a schema-1 record
    loads. S1 step 2 makes it a miss. `TestFileCheckStoreOlderSchemaIsMiss`
    replaces it, and covers schemas 1 and 2.
  * The golden document is schema 3, with `"outcome":"answered"`.
  * The schema-mismatch test uses 4 as the future schema. The corrupt
    record cases use schema 3, and add an unknown outcome.
* **Lint caught three things in my first draft.** `errorlint` flagged two
  error comparisons in the test, now `errors.Is`. `goconst` flagged a third
  `"unknown"` literal, which became `CheckOutcome(N)`.
* **Docs.** The `CheckCached` and `CheckRecord` godoc, `doc.go`, the
  extending guide, and `architecture.md`.
* **Checks** (`gate.sh`, every one rc 0):
  * `make lint`, 0 issues;
  * race and shuffle;
  * `make apicheck`: `compatible with v1.5.1`;
  * fuzz, vuln, tidy;
  * every script test;
  * cross vet;
  * markdownlint on the changed docs.

### Deviation D1 (2026-10-04): `ServiceStarted` reaches `Result`

* **Found,** before any S2 code. Step 3 adds `InstallResult.ServiceStarted`
  only. `Updater.Run` copies the other service fields into `Result`
  (`updater.go`, after `sess.Install`) and `Result.Document` writes them as
  JSON. Without the same copy, `Run`'s and the CLI's callers cannot see
  the new field.
* **Decision.** The owner chose "Mirror to Result and JSON". MADR amendment
  A3 records it. S2 also:
  * adds `Result.ServiceStarted`, set in `Run`;
  * adds `ResultDocument.ServiceStarted`, key `service_started`, after
    `service_was_running`;
  * updates the goldens that carry the document.

### Phase S2: a stopped service, Q2 (B3) (2026-10-04)

* **Tests first.** `managed_stopped_test.go` uses only the `v1.5.1` API;
  its fakes' `Enabled` method compiles there, and is never called. Against
  `HEAD`'s code:

  ```text
  TestManagedStartRule                     stopped, not enabled / stopped, no Enabled: lifecycle [start health], want []
  TestManagedEnabledErrorReplacesNothing   res = {… Applied:true …}: the target was replaced
  TestManagedRecoveryRestartsOnlyWhatRan   stopped, reconcile fails (with and without Enabled): lifecycle [start health], want []
  TestManagedCommitRefusedStartRule        stopped, not enabled: lifecycle [start health stop start health], want []
  ```

  The running cases and "stopped, enabled" pass on both trees: they were
  right before. Two plants, in scratch copies:
  * a refused commit always restarting: `lifecycle [stop start health],
    want []`;
  * a failed health check after this install's start not restarting the
    old binary: `lifecycle [start health stop], want [start health stop
    start health]`.

  `managed_started_test.go` covers `ServiceStarted` on `InstallResult`, on
  `Result` through `Run`, and in the document.
* **The API, additions only.** `EnabledLifecycle`;
  `InstallResult.ServiceStarted`; and, by deviation D1,
  `Result.ServiceStarted` and `ResultDocument.ServiceStarted`
  (`service_started`).
* **The rule** in `managedSession.Install`:
  * `Enabled` is asked only when the service is stopped. A running service
    is restarted whatever `Enabled` would say, and an `Enabled` error
    matters only for a stopped one. It fails the install before anything
    is replaced.
  * Start and the health check run only when the service was running or
    is enabled.
  * Recovery restarts what was running. Once this install's `Start`
    succeeded, recovery stops the new binary and starts the old one. A
    refused commit recovers with `start` for both.
* **One existing test asserted the reversed contract.**
  `TestManagedDownHeals` expected a stopped service to be started. It is
  now `TestManagedStoppedStaysStopped`: no stop, start or health check,
  and `ServiceStarted` false.
* **Goldens.** The document gains `service_started`. Ten cli goldens were
  regenerated with `-update`. A script checked that each changed line
  differs only by the added key. `events_test.go` and `example_test.go`
  were edited the same way.
* **Lint caught one thing in my first draft.** `staticcheck` ST1008:
  `installWith` returned its error before a string. It now returns it
  last.
* **Docs.** The `EnabledLifecycle` and `ServiceStarted` godoc, `doc.go`, the
  extending guide, and `architecture.md`.
* **Windows test host.**
  * `go test -race -count=1 ./...` rc 0 for all four packages;
    `selfupdate` took 49.5 s.
  * The S2 tests, run verbose, pass.
  * `TestManagedCommitRefusedStartRule` skips there: Windows refuses to
    rename the locked directory, as it does for the existing
    `TestManagedCommitRefusesMovedDirectory`.
* **Checks** (`gate.sh`, every one rc 0):
  * `make lint`, 0 issues;
  * race and shuffle;
  * `make apicheck`: `compatible with v1.5.1`;
  * fuzz, vuln, tidy;
  * every script test;
  * cross vet;
  * markdownlint on the changed docs.

### Deviation D2 (2026-10-04): warnings as a comparable exported field

* **Found** in S3, after `EventWarning` and a `Result.Warnings []string`
  field were added and before any other S3 code. `make apicheck`:

  ```text
  check-api-compat: incompatible with v1.5.1:
  - ./selfupdate.Finished: old is comparable, new is not
  - ./selfupdate.Result: old is comparable, new is not
  ```

  Step 2's field breaks the Goal's "every exported change is an
  addition". `ResultDocument` would break the same way at step 4.
* **Decision.**
  * The owner first chose accessor methods over an unexported field. That
    version was being written when the owner asked for "exported known
    fields and json schema where possible for a consistent api". It was
    replaced, uncommitted.
  * MADR amendment A4 records the result. `Result.Warnings` and
    `ResultDocument.Warnings` (`json:"warnings,omitempty"`) are of type
    `Warnings`, a string whose entries are joined by newlines, and whose
    JSON form is an array. `NewWarnings`, `Add`, `List` and `Len` work
    on it.
  * `make apicheck` then reported `compatible with v1.5.1`.

### Phase S3: one terminal event, Q3 (C2, C11) (2026-10-04)

* **Tests first.** `terminal_event_test.go` uses only the `v1.5.1` API.
  Against the unfixed code:

  ```text
  TestLateErrorIsNotAFailure  close fails:            err = selfupdate: demo: fixture: unlock failed, exit 1
                              complete report fails:  err = selfupdate: demo: fixture: reporter closed, exit 1
                              applied with an error:  err = selfupdate: demo: fixture: backup removal failed, exit 1
                              dry run, close fails:   err = selfupdate: demo: fixture: unlock failed, exit 1
  TestDryRunCompleteDetail    complete Detail = "dry run: verified, nothing installed", want dry-run
  ```

  `warnings_test.go` covers the new API. Plants, in scratch copies:
  * the dry run dropping its Close error: `events after complete: []`;
  * the CLI's warning lines removed: `warning.text.stderr differs`;
  * a `[]string` field added to `Result`: the build fails with `Result
    does not satisfy comparable` and `Finished does not satisfy
    comparable`.
* **The API, additions only.**
  * `EventWarning`, appended in its own block after `EventRolledBack`.
  * The `Warnings` type, from deviation D2: `NewWarnings`, `Add`, `List`,
    `Len`, `MarshalJSON` and `UnmarshalJSON`.
  * `Result.Warnings` and `ResultDocument.Warnings`
    (`json:"warnings,omitempty"`). The document's `schema_version` is 2.
* **The rule.** `warn` turns each error after the work is done into an
  advisory `EventWarning`, after `complete`, and an entry in
  `Result.Warnings`. `Run` returns nil. The errors are a failed Close, a
  failed report of complete, and an installer's error with `Applied` true,
  and in a dry run the same Close and report errors. The dry run's
  `complete` has `Detail` `dry-run`.
* **The CLI** writes `warning: <text>` to stderr for each warning, in
  either mode, before the summary or the result object. A write error there
  is handled as `finish`'s is, under C3's rule. `cli/doc.go` documents it.
* **Existing tests moved to the decided contract.**
  * `TestRunCloseErrorJoinedAfterCommit` expected the joined error. It is
    now `TestRunCloseErrorIsAWarning`: no error, and the warning listed.
  * `TestRunErrorsNameProduct` loses its "report complete" case, which is
    no longer an error. `TestLateErrorIsNotAFailure` covers it.
  * `TestDryRunLeavesTargetUntouched` expects `Detail` `dry-run`.
* **Goldens.**
  * The reporter goldens gain a `warning` case, with a session whose Close
    fails. `text-warning.golden` and `jsonl-warning.golden` end
    `complete`, then `warning … unlock failed`.
  * The cli scenarios gain `warning`: exit 0; stderr carries the warning
    event, `warning: unlock failed` and the summary; the result object
    carries `"warnings":["unlock failed"]`.
  * A script checked every other changed golden line: 14 lines, each only
    the result's `schema_version` 1 to 2, or the dry run's `Detail`. The
    migration fixtures did not change.
  * `events_test.go` and `example_test.go` expect schema 2.
* **Lint caught one thing in my first draft.** The compile-time
  comparability check written as `x == x` tripped `staticcheck` SA4000. It
  is now a generic `isComparable[T comparable]` instantiated for each
  type.
* **Docs.** The `EventComplete`, `EventWarning`, `Warnings` and `Result`
  godoc, `doc.go`, `cli/doc.go`, the extending guide's "Read JSON output",
  and `architecture.md`.
* **Checks** (`gate.sh`, every one rc 0):
  * `make lint`, 0 issues;
  * race and shuffle;
  * `make apicheck`: `compatible with v1.5.1`;
  * fuzz, vuln, tidy;
  * every script test;
  * cross vet;
  * markdownlint on the changed docs.

  The Windows host is not required for S3 (V3).

### Phase S4: `Request.Platform`, Q4 (C7) (2026-10-04)

* **Tests first.** `platform_apply_test.go` uses only the `v1.5.1` API.
  It builds its own release for a real foreign platform: `linux/amd64`, or
  `windows/amd64` on Linux. Against the unfixed code:

  ```text
  TestApplyForeignPlatformRefused  res = {… AssetName:demo-linux-amd64 Operation:upgrade … Applied:true …}
  ```

  The foreign platform's binary was installed. `TestCheckAndDryRunForeignPlatform`
  and `TestApplyRunningPlatformNamed` guard what stays allowed, and pass on
  both trees. A plant that removed the dry-run exemption made the first
  fail.
* **The rule.** `validateRequest` refuses an apply, neither `CheckOnly` nor
  `DryRun`, whose `Platform` is set and is not the running platform. The
  error wraps `ErrUnsupportedPlatform`. It comes before any network call:
  the test checks that `Latest`, `ByTag`, `ResolveTarget`, `Begin` and
  `Install` never ran. The `Request.Platform` godoc says so.
* **The seam.** `runningPlatform` in `updater.go`, which `normalizePlatform`
  also uses. `export_test.go` exports `SetRunningPlatform(t, p)` for the
  external tests.
* **Tests that applied a foreign platform.** The external tests apply
  fixtures built for `goldenPlatform`, `linux/amd64`, on every host:
  * `tempTarget`, which every such test calls, now makes `goldenPlatform`
    the running platform for the test. That covers the reporter goldens,
    the GitHub end-to-end tests, the Stream tests and the credential run
    tests.
  * The running-copy end-to-end tests already use the running platform,
    and needed no change.
  * Three examples applied a `linux/amd64` release with an explicit
    `Platform`, which is what Q4 now refuses. An example has no
    `*testing.T`, and it is what consumers copy. `exampleUpdater` now
    publishes the running platform, and `exampleRequest` leaves `Platform`
    zero, as a program does. `ExampleUpdater_RunWith` printed text
    reporter lines that name the asset, which now depends on the host. It
    prints each event's kind through a `ReporterFunc` instead.
    `exampleChecker` only checks, and keeps `linux/amd64`.
* **Docs.** Only the godoc describes `Request.Platform`. S8's "From v1.5
  to v1.6" names the refusal.
* **Windows test host:** `go test -race -count=1 ./...` rc 0 for all four
  packages; `selfupdate` took 48.9 s.
* **Checks** (`gate.sh`, every one rc 0):
  * `make lint`, 0 issues;
  * race and shuffle;
  * `make apicheck`: `compatible with v1.5.1`;
  * fuzz, vuln, tidy;
  * every script test;
  * cross vet.

### Phase S5: the zero-value reply types, Q5 (C9) (2026-10-04)

* **Tests first.** `reply_zero_test.go` uses only the `v1.5.1` API.
  Against the unfixed code:

  ```text
  TestZeroValueRepliesReturn  ConfirmNeeded.Answer / ConfirmNeeded.Cancel / CredentialNeeded.Supply /
                              CredentialNeeded.Cancel / a second reply: … on a zero value blocked
                              goroutines: 7 now, 2 when the test began
  ```

  `TestRealRepliesDeliver` guards the real path: a request made as the
  Stream makes it gets its first reply, and only that one. It passes on
  both trees. A plant that dropped every reply made it fail.
* **The bound.** Step 2 says the zero values return within 10 ms. The test
  allows 1 s instead, because a 10 ms wall-clock bound under `-race` on a
  CI host would flake. A send on a nil channel never returns, so 1 s
  catches it just as well. The leak check confirms that no goroutine is
  left.
* **Mistake in my own test.** The first `TestRealRepliesDeliver` read the
  reply with a bare `<-c.reply`. Under the plant it blocked until `go
  test`'s 10-minute timeout, at `reply_zero_test.go:48`, instead of failing.
  It now reads with `queued`, which fails at once when no reply was sent,
  because each reply is sent before `Answer`, `Supply` or `Cancel` returns.
  The plant then failed in 0 s: `no reply was sent`.
* **The fix.** The four reply methods send through `trySend`, a `select`
  with a `default`. A reply channel that a run made has one slot, and
  `sync.Once` allows one send, so a real reply always lands. A zero
  value's nil channel takes nothing. The godoc says so.
* **Docs.** The reply methods' godoc, and the extending guide's Stream
  section.
* **Checks** (`gate.sh`, every one rc 0):
  * `make lint`, 0 issues;
  * race and shuffle;
  * `make apicheck`: `compatible with v1.5.1`;
  * fuzz, vuln, tidy;
  * every script test;
  * cross vet;
  * markdownlint on the guide.

### Phase S6: leftovers and mode bits, Q6 (B11, B12) (2026-10-04)

* **Tests first.** `leftovers_test.go` and `modebits_unix_test.go` use only
  the `v1.5.1` API. Against the unfixed code:

  ```text
  TestBeginRemovesLeftovers           leftover .demo.selfupdate-1234567 was not removed
                                      leftover .demo.selfupdate-bak-7654321 was not removed
  TestCleanupPendingRemovesLeftovers  the same
  TestSpecialBitTargetRefused         u---------, g---------: ResolveTarget = <nil>, want the special bit refused
  TestStickyBitCarriedOver            new binary mode -rwxr-xr-x
  ```

  `TestLeftoverSymlinkNotFollowed` guards what stays true, and passes on
  both trees. `modepolicy_test.go` and `modepolicy_unix_test.go` cover the
  new API and seams. Plants, in scratch copies, each caught:
  * the setuid/setgid refusal removed: `TestSpecialBitTargetRefused`;
  * chown ignoring every error: `TestStagingOwnership/EIO`;
  * any suffix accepted as a leftover: `.demo.selfupdate-abc was removed`;
  * the regular-file check removed: `the symlink was removed`. My first
    version of this plant left a variable unused and did not build; the
    second kept it in use.
* **Leftovers** (`leftovers.go`). `beginSession`, which `Begin` and
  `CleanupPending` both run, calls `removeLeftovers` under the lock, after
  the receipt is processed.
  * `isLeftover` matches only the names this package makes for the
    target. A staging file is `.<base>.selfupdate-<digits>`, plus `.exe`
    on Windows. A backup is `.<base>.selfupdate-bak-<digits>`. So another
    target whose name starts with this one's, the lock, the receipt and
    `.previous` never match.
  * It skips a backup the Windows receipt still lists (`listedBackups`),
    and every backup when the receipt cannot be read. It skips anything
    that is not a regular file, so a symlink is never followed.
  * Removal is best-effort, through the root (`leftoverRemove`, a seam).
* **Special bits.**
  * `TargetPolicy.AllowSpecialModeBits` is the new field. `resolveTarget`
    refuses a setuid or setgid target without it. That runs at
    `ResolveTarget`, before any download, and again when a session
    revalidates.
  * `Target` keeps the policy's answer, unexported. `chmodStaging` copies
    the permissions and the sticky bit, and setuid and setgid only when
    allowed, so a bit added after resolution is never carried.
* **Ownership.** On Unix, `chownStaging` runs before the chmod, because a
  chown clears setuid and setgid. It gives staging the target's uid and gid
  with `Lchown`. EPERM is ignored; any other error fails the install.
* **CI.** A Linux step builds the test binary as the runner, then runs only
  `TestStagingTakesOwnerAsRoot` under `sudo`, with
  `SELFUPDATE_REQUIRE_ROOT=1`. Here, not root: with the variable the test
  fails, rc 1; without it, it skips, rc 0. actionlint and
  `check-workflows.sh` pass. The root path first runs in CI after the
  owner's push; it was not run with `sudo` on this host.
* **Existing test moved.** `TestChmodStagingUsesPermOnly` named the B12
  behaviour. It is now `TestChmodStagingCopiesPerm`, with the same
  assertion, and calls the new `chmodStaging` signature.
* **A stale doc from `v1.5.1`'s P5.** `architecture.md` still said that
  `CleanupPending` refuses to clear a busy backup. P5 made it keep the
  backup without an error. It is corrected here, beside S6's own text on
  `CleanupPending`.
* **Docs.** The `TargetPolicy.AllowSpecialModeBits` godoc, `doc.go`, the
  extending guide (a new section, "Replace a setuid or setgid binary"),
  and `architecture.md`.
* **Windows test host.**
  * `go test -race -count=1 ./...` rc 0 for all four packages;
    `selfupdate` took 49.5 s.
  * The S6 tests and the receipt tests pass, run verbose. The symlink test
    runs there and does not skip.
* **Checks** (`gate.sh`, every one rc 0):
  * `make lint`, 0 issues;
  * race and shuffle;
  * `make apicheck`: `compatible with v1.5.1`;
  * fuzz, vuln, tidy;
  * every script test;
  * cross vet;
  * markdownlint on the changed docs.

### Deviation D3 (2026-10-04): the test server's credential header

* **Found,** before any S7 code.
  * Step 1 names `GitHubServerOptions.TokenHeader`. No such type exists:
    `NewGitHubServer(t, owner, repo, releases...)` takes no options, and
    changing it or `RequireToken` would break the API.
  * Step 1's `RecordedRequest.CredentialHeaders []string` would make
    `RecordedRequest` non-comparable, which `make apicheck` refuses, as in
    D2.
* **Decision.** The owner chose "RequireCredential method" and
  "HeaderNames string type". MADR amendment A5 records it.
  * The question offered "an empty header, or Authorization, means
    Bearer". The library sends a named header's value as-is, even for
    `Authorization`, so `RequireCredential` follows the library: only an
    empty header means Bearer. A test then requires what the library
    really sends.

### Phase S7: the test server, A14 (2026-10-04)

* **No fail-first run on the old code.** S7 adds API to a test fixture.
  Every new test names `RequireCredential`, `CredentialHeaders` or
  `HeaderNames`, so on `v1.5.1` it does not compile. Plants show the tests
  can fail, in scratch copies:
  * recording a header's value with its name:
    `TestRequireCredentialForms/bearer` fails;
  * a named `Authorization` header treated as Bearer:
    `TestRequireCredentialForms/named_Authorization_is_raw` fails;
  * the custom header not watched: `TestCustomHeaderCredentialStaysOnAPI`,
    `API request /repos/owner/demo/releases/latest carried "",
    Authorization false`.
* **The API, additions only** (deviation D3).
  * `GitHubServer.RequireCredential(header, value)` requires what the
    library sends for that `Credential`. An empty header means
    `Authorization: Bearer <value>`. A named header, `Authorization`
    included, carries the value as it is. An empty value means anonymous
    again. `RequireToken(t)` is now `RequireCredential("", t)`, with its
    behaviour unchanged.
  * `RecordedRequest.CredentialHeaders` is of type `HeaderNames`:
    canonical names joined by `", "`, with `List` and `Has`. It records
    which of `Authorization` and the required header were present, never
    a value.
* **Tests.**
  * `TestRequireCredentialForms`: eight forms, each with its status and
    recorded names, and no value recorded.
  * `TestHeaderNames`.
  * `credential_header_e2e_test.go`:
    * a custom-header credential through `Updater.Run`: every API request
      carries `X-Demo-Key` and no `Authorization`, and no asset request
      carries either;
    * without the credential, or with a wrong value, the run fails with
      the 401.
* **Lint caught two things in my first draft.** `staticcheck` QF1002 asked
  for a tagged switch on `r.Host`. `goconst` flagged `"Authorization"`
  three times, which is now `authorizationHeader`.
* **Docs.** The `RequireCredential`, `RequireToken`, `RecordedRequest` and
  `HeaderNames` godoc, the extending guide's test section, and
  `architecture.md`.
* **Checks** (`gate.sh`, every one rc 0):
  * `make lint`, 0 issues;
  * race and shuffle;
  * `make apicheck`: `compatible with v1.5.1`;
  * fuzz, vuln, tidy;
  * every script test;
  * cross vet;
  * markdownlint on the changed docs.

  The Windows host is not required for S7 (V3).

### Phase S8: docs and release notes (2026-10-04)

* **Docs.**
  * The migration guide's step 2 now says `go get …@v1.6.0`, and points to
    a new last section, "6. From v1.5 to v1.6". It covers the warnings, the
    schema 2 document, the platform refusal, the managed start rule, the
    special-bits policy, and the smaller changes, each with what to do.
  * D16 (MADR amendment A2): the `VersionPolicy` comment says the policy
    decides which tags it accepts, and names both constructors.
  * `README.md` and `architecture.md` called `v1.5.0` the current release.
    The v1.5.1 PLAN's P7 should have moved them, and did not. They now
    name `v1.6.0`. `architecture.md` gives the tag's commit, which the
    commit after the tag adds, with the workflow pins (step 3, as
    amended).
  * The guides and `architecture.md` were updated phase by phase, S1 to S7.
* **Checks** (`gate.sh`, every one rc 0, and markdownlint on the changed
  docs).

### Release notes for `v1.6.0` (2026-10-04)

Additions only: `make apicheck` reports `compatible with v1.5.1`. What a
consumer may notice, by finding:

* **Q1 (A3), CheckCached.** It also caches `ErrLatestOlder`,
  `ErrUnsupportedPlatform` and `ErrMutableRelease`, for `maxAge`, as the new
  `CheckRecord.Outcome` (`CheckOutcome`). A cached one returns an error that
  matches its sentinel. The check file is schema 3, and an older one loads as
  a miss.
* **Q2 (B3), a stopped service.** A managed update starts a service only
  when it was running, or when the `Lifecycle` also implements the new
  `EnabledLifecycle` and reports it configured to start. Otherwise it stays
  stopped; it used to be started. `ServiceStarted` is new on
  `InstallResult` and `Result`, and in the document (`service_started`).
* **Q3 (C2, C11), one terminal event.** An error after `complete`, such as
  a failed unlock, a failed report of `complete`, or an installer's error
  after commit, is now an `EventWarning`, listed in `Result.Warnings` (type
  `Warnings`), and the run succeeds with exit code 0. The CLI prints
  `warning: …`. The result document is schema 2 and carries `warnings`. A
  dry run's `complete` has the `detail` `dry-run`.
* **Q4 (C7), Request.Platform.** An apply for a platform other than the
  running one is refused with `ErrUnsupportedPlatform`, before any network
  call. A check or a dry run may still name one.
* **Q5 (C9), zero-value replies.** `Answer`, `Supply` and `Cancel` on a
  zero `ConfirmNeeded` or `CredentialNeeded` return at once.
* **Q6 (B11, B12), leftovers and modes.**
  * `Begin` and `CleanupPending` remove a crashed update's staging files
    and backups.
  * A setuid or setgid target is refused unless the new
    `TargetPolicy.AllowSpecialModeBits` is set. Then the bits are kept.
  * The sticky bit is kept.
  * On Unix, the new binary gets the old one's owner and group where
    permitted.
* **A14, selfupdatetest.**
  * `GitHubServer.RequireCredential(header, value)` requires a
    custom-header credential.
  * `RecordedRequest.CredentialHeaders` (type `HeaderNames`) names the
    credential headers each request carried.
* **D16.** The `VersionPolicy` comment is corrected.

`Result`, `Finished`, `ResultDocument` and `RecordedRequest` stay
comparable. The new list fields use string types whose JSON form is an
array (amendments A4 and A5).

### Phase S8: push, tag, pin and checks (2026-10-04)

* **Push.** At the owner's ask, the agent pushed `main` to `b1f1caa`. CI was
  green on ubuntu-24.04, macos-15 and windows-2025. The Linux "ownership as
  root" step ran `TestStagingTakesOwnerAsRoot` under `sudo`, and it passed:
  `--- PASS: TestStagingTakesOwnerAsRoot`. That is the root path's first
  run, which S6 left to CI.
* **Tag.** The owner tagged and pushed `v1.6.0`, annotated, with message
  `v1.6.0`. `git ls-remote origin 'refs/tags/v1.6.0^{}'` gives `b1f1caa`,
  and `scripts/check-release-tag.sh v1.6.0` exits 0.
* **Step 3, after the tag** (as amended).
  * `README.md` and the migration guide pin
    `publish-selfupdate-release.yml@b1f1caa01013d8ecbbc0a17639a55e21fcdf0763 # v1.6.0`.
  * Nothing under `.github/workflows/publish-selfupdate-release.yml` or
    `scripts/` changed between `v1.5.1` and `v1.6.0`. The guide says so, and
    its `ls-remote` example resolves `v1.6.0`.
  * `architecture.md` gives the tag's commit.
* **Step 5.**
  * **CI on the tag:** green on all three runners.
  * **The proxy:** `go list -m …@latest` against `proxy.golang.org` gives
    `v1.6.0`.
  * **A scratch consumer** builds on `v1.6.0`, lint-free under `go vet`, and
    uses the new API:
    * an `EnabledLifecycle`;
    * `TargetPolicy.AllowSpecialModeBits`;
    * `NewWarnings`, `Warnings.List`, `CheckLatestOlder` and `EventWarning`;
    * `Result.ServiceStarted`;
    * `Result` compared with `==`.

    Its document prints `"schema_version":2`, `"service_started":true` and
    `"warnings":["unlock failed"]`. Through `cli.Command`: `--check` exits
    10, `--check --json` exits 10 with one result object, and `--yes`
    applies with exit 0. The linux and windows cross builds pass.

    My first draft called `Result.WithWarnings`, from the accessor design
    that D2 replaced, which `v1.6.0` does not have. It was taken out
    before the first build.
  * **prepare-commit-msg**, on a scratch clone: `go get …@v1.6.0` changes
    only `go.mod` and `go.sum`, and `make verify` passes, with total
    coverage 84.1 % and no vulnerabilities. Its non-test code uses none of
    the changed behaviour. Moving it is a prepare-commit-msg record.

**Verification.**
* V1: each phase's tests failed on the unfixed code, or, for S7's new
  fixture API, plants showed they can fail. All of it is recorded per
  phase.
* V2: `make apicheck` reported `compatible with v1.5.1` at every phase.
  Deviations D2 and D3 kept it so.
* V3: the v1.5.1 PLAN's checks ran at every phase. The Windows test host
  passed S2, S4 and S6, and S1 to S8 ran in CI's Windows job.
* V4: CI is green on `main` and on `v1.6.0`.
* V5: the release notes and "6. From v1.5 to v1.6" name every changed
  behaviour.

This PLAN is `complete`.
