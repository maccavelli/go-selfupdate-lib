---
status: in-progress
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
