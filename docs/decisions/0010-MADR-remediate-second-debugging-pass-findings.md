---
status: accepted
date: 2026-10-03
decision-makers: go-selfupdate-lib maintainers
consulted: prepare-commit-msg (the first consumer, at v1.5.0)
informed: go-tui-lib, pi-go, go-llmprovider-sdk
---
# Fix the second debugging pass's findings: a v1.5.1 of contract-preserving fixes, the tooling fixes with no release, and a v1.6.0 for the contracts the owner decides

## Context and Problem Statement

On 2026-10-03, after the rename to go-selfupdate-lib
([0009-MADR-rename-to-go-selfupdate-lib.md](0009-MADR-rename-to-go-selfupdate-lib.md))
and the first consumer's move to `v1.5.0` (prepare-commit-msg
`0008-MADR-adopt-go-llmprovider-sdk-and-go-selfupdate-lib.md`), the owner
asked: "assess the codebase. do a debugging pass. find bugs, find gaps. find
missing wiring, and incomplete features or functionality. report back with
an madr for review."

This record lists what the pass found and proposes how to fix it. It is the
second such pass. The first,
[0003-MADR-remediate-debugging-pass-findings.md](0003-MADR-remediate-debugging-pass-findings.md),
ran before `v1.0.0`. Its fixes still hold: each reviewer re-checked the
0003 findings in its area, and found no regression.

### Method

* Four read-only reviews ran in parallel, one per area, against `HEAD`
  `48653e4` (`v1.5.0` plus records):
  * **A**, network and integrity: `github.go`, the download, checksum,
    verify and manifest-verify paths, assets, versions and channels,
    `checkcache.go`, credentials;
  * **B**, filesystem, locking and install: `target.go`, `lock_*`,
    `replace*`, `cleanup*`, `session.go`, `standalone.go`, `managed.go`,
    probes, the Windows files;
  * **C**, the coordinator, the public API and the command surface:
    `updater.go`, `checker.go`, `stream.go`, events, reporters,
    confirmers, `types.go`, `selfupdate/cli`, `buildinfo`,
    `selfupdatetest`;
  * **D**, release tooling, CI, build, documentation and the roadmap.
* Each review measured the code against its own documentation, this
  repository's records (0003, 0004 and its PLANs, 0005) and the guides.
  Each finding was reproduced with a probe test or a planted break where
  that was possible.
* Every experiment ran on scratch copies made with `git archive` or
  `git clone`. The repository was not modified, and `git status` was clean
  after each review.
* The author re-ran the headline evidence: each reviewer's probe tests, in
  that reviewer's scratch copy, and the D1 and D3 checks directly.

**Evidence** column:

* **R:** reproduced in the author's re-run;
* **R\*:** reproduced by the reviewer only;
* **C:** confirmed by reading the cited code;
* **—:** reasoning only.

A finding raised by two reviewers keeps one ID and names the other.

**Baseline:**

* `go test -race ./...` passes, with total coverage 90.5 %: `buildinfo`
  100 %, `selfupdate` 90.3 %, `selfupdate/cli` 93.5 %, `selfupdatetest`
  90.1 %.
* `make lint` is clean on three targets.
* `go vet` and `govulncheck` are clean.
* CI was green on `48653e4` (run `37126883550`).
* The repository's own gates all pass on a scratch clone: every
  `scripts/*_test.sh`, `make apicheck` (compatible with `v1.5.0`),
  shellcheck, markdownlint, actionlint, cross `go vet`, and
  `go mod tidy -diff`.

No finding is rated High. None loses data or installs an unverified binary
through the shipped `Updater` and `cli` paths. The Medium findings break a
documented contract, or fail a supported setup.

### Medium

| ID | Where | Finding | Evidence |
| :--- | :--- | :--- | :--- |
| A1 | `manifestverify.go:123-139` | `Verification.OpenAsset` and `ManifestVerification.OpenAsset` are documented as "enforcing its advertised size and digest". `checkedAsset` compares the digest only when a read returns `io.EOF`. A verifier that reads exactly `Size` bytes (`io.ReadFull` into a buffer of that size, `io.CopyN`) receives tampered bytes with no error, and `Close` checks nothing. Latent: no shipped verifier reads sibling assets, but this is the seam 0004's release-signing design depends on. | R: `ReadFull: n=10 data="0123456789" readErr=<nil> closeErr=<nil>`; the same with `CopyN`; the control, `ReadAll`, gives `asset digest does not match its github digest … integrity check failed` |
| A2 (= C1) | `github.go:614-654`, `credentials.go:57-91` | The GitHub source resolves its credential once, for the source's lifetime. `PromptCredential` returns `ErrNoCredential` outside a Stream, and the source caches that as "anonymous". A startup `Checker().Check` or `CheckCached`, or a plain `Run`, therefore stops every later `Start` on the same `Updater` from prompting. One skipped prompt does the same. The example says one chain "serves Run, which never prompts, and Start, which does". The guide's startup banner plus TUI setup is exactly this case. | R: `startup Check: … github http 401` then `later Start: prompts=0 applied=false err=… 401`; control on a fresh `Updater`: `prompts=1 applied=true` |
| A3 (= C8) | `checkcache.go:81-99` vs `doc.go` | `CheckCached` saves only a success or a `RateLimitError`. A deterministic answer, such as `ErrLatestOlder` (0005 §4: a prerelease user back on the stable channel) or a missing platform asset, asks the network on every start. That contradicts doc.go's "asks the network at most once per interval". The code follows its PLAN's rule 7; the package docs do not. | R: five `CheckCached` calls with `maxAge` 1 h gave `Latest calls=5 … latest release v1.0.0 is older than running v1.1.0` |
| A4 (= C8) | `checkcache.go:75,105-116` | `NotBefore` comes unclamped from `X-RateLimit-Reset` or `Retry-After`. One bad response defers every check until the cache file is deleted. A reset already in the past gives no back-off. `CheckedAt` has a clock-skew guard; `NotBefore` has none. | R: a reset of 2200-01-01 still deferred the check five years later, `networkCalls=0`; R\*: a reset of now − 2 min made the immediate retry call the network |
| B1 | `probe.go:115-131` | `cappedBuffer` embeds `bytes.Buffer`, so it inherits `ReadFrom`. `exec` copies the child's stdout with `io.Copy`, which uses `ReadFrom`, so the capped `Write` is never called. The 64 KiB `maxProbeOutput` bound is not enforced. A probed binary can grow the updater's memory without limit, and a version printed after the cap still matches. `cappedBuffer.Write` has 0 % coverage. | R: a probed script printing 200 KiB, then `v9.9.9`: `probe err=<nil>`, "CAP BYPASSED"; R\*: a pipe test buffered 4 MiB against the 65 536-byte limit |
| B2 | `target.go:93-103` | The home directory is always canonicalised as an allowed root, and a failure there is fatal, even when `TargetPolicy.AllowedRoots` already covers the target. Self-update is refused when `$HOME` is unset (common under launchd and systemd), is `/`, or does not exist. Those are the service accounts `ManagedInstaller` serves. The `TargetPolicy` doc calls the roots "additive". | R: with `AllowedRoots` covering the target: `locate home directory: $HOME is not defined`; `filesystem root is not an allowed self-update root`; `lstat /nonexistent-…: no such file or directory` |
| B3 | `managed.go:91-118` | For a service that is installed but not running, `Install` skips `Stop` but still calls `Start` and `WaitHealthy`. It leaves the service running, and reports `ServiceWasRunning=false`. No document says an update starts a stopped service, and no test covers the case. | R: `res.ServiceWasRunning=false Applied=true err=<nil> lifecycle=[start health]` |
| B4 | `managed.go:109-114,130-162` | When `Start` succeeds and `WaitHealthy` fails, recovery restores the old binary under the running new process, then calls `Start` again with no `Stop`. On Unix the unhealthy new process keeps running, because `Start` on an active unit is usually a no-op. On Windows, restoring over a running image fails at the lock timeout (reasoning). | R: lifecycle `[stop start health start health]`, the target back to `"old-bytes"` with nothing stopped |
| B5 | `cleanup_windows.go:93-125`, `standalone.go:58-67` | A Windows cleanup receipt exists because a running image's last name cannot be deleted. The next `beginSession` removes the receipt's backup. If a process from the old version still runs (a long-lived server), that remove fails. The error is not `ErrConcurrentUpdate`, so every later update and every startup `CleanupPending` fails until that process exits. No Windows test covers a busy pending backup. | — (C for the path; not run on Windows) |
| C2 | `updater.go:96-107,294-299,340-350` | A run that commits, or a dry run, can report `EventComplete` and then `EventFailed`, and exit 1. This happens when `Close` fails, when reporting `complete` fails, or when an installer returns `Applied:true` with an error. It breaks "`EventFailed` is the last event of a failed run". A Stream or TUI sees two terminal events, and the CLI prints `update failed:` after the binary was replaced. | R: `close-error: applied=true exit=1 kinds=[… installing complete failed]`; the same for `complete-report-error` |
| C3 | `cli/run.go:113-116,179-186`, `errors.go:71-80` | Under `--check --json`, when writing the final result object fails, the write error is joined to `ErrUpdateAvailable`. `ExitCode` returns 10, and `Exit` prints nothing. cli/doc.go promises "exactly one result object" and exit 1 on any error. The same happens when the text summary write fails. | R: `--check --json, result write fails: exit=10 stdout_lines=3 stderr=""`; `--check text, summary write fails: exit=10` |
| C4 | `cli/run.go:84-87`, `cli/command.go:49-53` | Some failures after flag parsing write no `--json` result object: a `newUpdater` that returns `(nil, nil)`, and a negative `Timeout`. They return before `finish`. 0004 amendment F8 says the result line comes "exactly once, on every path past flag parsing". | R: `nil-updater --json: exit=1 stdout=""`; `negative-timeout --json: exit=1 stdout=""` |
| C5 | `managed.go:30-40` | `NewManagedInstallerFor`, and so `NewManagedInstaller`, rejects only an untyped nil `Lifecycle` or `Reconciler`. A typed nil passes, and panics in `Install`, after the full download and verify, while the lock is held. That is 0003 C3's class, and every other constructor uses `isNil`. | R: `NewManagedInstaller(typed-nil life, typed-nil rec): m!=nil=true err=<nil>`; `Install: panic: runtime error: invalid memory address or nil pointer dereference` |
| D1 | `scripts/check-release-tag.sh:31,33` | The tag gate "mirrors `selfupdate.NewSemverPolicy`", but uses Python `\d`, which matches any Unicode decimal digit. Go's `\d` is ASCII. The gate admits tags the client refuses. Published as stable, such a tag becomes "latest", every stable client fails `Validate`, and the immutable tag cannot be reused. | R: `check-release-tag.sh 'v1.0.1١' '[]'` rc 0; control `'v1.0.1x'` rc 1; R\*: the Go policies refuse both shapes |
| D2 | `scripts/verify-selfupdate-release.sh:172,185`, its test | The verifier's two core checks have no test: each binary matches its `SHA256SUMS` line, and `SHA256SUMS` lists exactly the canonical binaries. Either can be deleted and the test still passes. A regression would publish an immutable release every client fails with `ErrIntegrity`. | R\*: each check replaced by `if False:` on a scratch copy; the test exited 0 with no FAIL. Thirteen other plants were caught |
| D3 | `scripts/go-precheck.sh:45-58` | When its file arguments name only deleted Go files, the pre-add check drops them, prints "no Go files to check" and exits 0. A deletion that breaks the build passes `make pre-add-check FILES=…` and the agent commit gate. | R: on a scratch clone, `git rm selfupdate/version.go`, then the check: `no Go files to check.` rc 0; `go build ./...` rc 1 |

### Low

| ID | Where | Finding | Evidence |
| :--- | :--- | :--- | :--- |
| A5 | `github.go:180-185` | The comment says a redirect URL "is not echoed: a redirect target can carry signed query parameters". A refused redirect, or any transport failure on the redirected hop, returns Go's `Get "<full URL>": …`. That is the CDN URL with its signature, printed to stderr and CI logs. For a private repository it is a short-lived bearer URL. | R: `Get "http://downloads.example.invalid/blob?X-Amz-Signature=SECRET1": … refusing redirect`; the same after `connection refused` |
| A6 | `github.go:558-566` | The 401 retry keys on the final status, not the origin. A 401 from the download host makes the provider refresh, and possibly prompt, though the credential was never sent there. | R\*: `providerCalls=2 … foreignAuth=["" ""]` |
| A7 | `github.go:617-623,660-669` | The provider runs under the source mutex. A blocking provider (`PromptCredential` waiting on the host) stalls every other request on the source, and a waiter cannot honour its context. | R\*: a second `Latest` with a 100 ms deadline returned after 1.5 s |
| A8 | `updater.go:382-396` | The guide says both verifier hooks fail the run with `ErrIntegrity`. `runManifestVerifiers` wraps; `runVerifiers` does not, so `EventFailed.Detail` is `"error"`. | R: `signature does not verify isIntegrity=false EventFailed.Detail="error"` |
| A9 | `credentials.go:150-162` | Only CR, LF and NUL are refused in a credential `Value`. Other control bytes pass, then `net/http` refuses every request, with no refresh. No secret leaks. | R\*: `validateCredential=<nil>`, then `invalid header field value … serverHits=0` |
| A10 | `github.go:183` | The plain-http loopback exemption applies per hop. A remote HTTPS API may redirect to `http://127.0.0.1`. Credentials are scrubbed, and the body is still digest-checked. | R\*: `checkRedirect(https api.github.com -> http 127.0.0.1) = <nil>` |
| A11 | `github.go:221` | `ByTag` does not refuse `.` and `..`, which 0003 A10 refused for owner and repo. A normalising proxy sends `/releases/tags/..` to `/releases`; the tag-pin check then gives a misleading error. | R\*: the server saw `…/releases/tags/..` |
| A12 | `checksums.go:113-118` | `filepath.Base` strips a drive volume on Windows only. The exported parser refuses `a:b` there, and accepts it on Unix. The publish gate keeps such names out of releases. | C |
| B6 | `session.go:162-165`, `replace_windows.go:96-106` | With `KeepPrevious` on Windows, renaming the backup over a `.previous` whose binary still runs fails at the retry budget. `Commit` fails with `Applied:true`, and the backup leaks; there is no fallback to a receipt. | — |
| B7 | `session.go:172-185,253-273`, `managed.go:115-118` | 0004 R3's directory-swap undo exists only in `Install`. `Apply` has no check after the rename. `Commit` detects a swap but only refuses. The managed path returns that error with no recovery: the new binary stays live in the swapped directory, and the backup is left behind. | R\*: `Applied=true RolledBack=false … concurrent update`, `lifecycle=[stop start health]` |
| B8 | `session.go:131-133`, `updater.go:321-336` | When the rollback after a swap fails, `Install` returns `Applied:true` with `Backup`. `Run` copies `Backup` into `Result.PendingBackup` only when `Applied` is false, so the only copy of the old binary is not reported. | R\*: `Result.Applied=true Result.PendingBackup=""`, the backup left in place |
| B9 | `session.go:201-205`, `replace_unix.go:79-82` | When a post-install probe fails, and the rollback rename succeeds but the directory sync after it fails, `Install` names a `Backup` that no longer exists, with `RolledBack=false`. `Run` would then say the previous binary "was kept at" that path. | R: `MISREPORT: Install names Backup .demo.selfupdate-bak-…, which does not exist; target already holds "old-bytes", RolledBack=false` |
| B10 | `session.go:253-285` | `Commit` and `Rollback` do not check `s.closed`. After `Close` releases the lock, they still rename or remove by path while another session holds it. `ManagedInstaller` never does this; `TwoPhaseSession` is public. | R: `Rollback after Close while another session holds the lock: err=<nil> target="old-bytes"` |
| B12 | `replace.go:59,78-80` | Staging copies only `Mode().Perm()`. Setuid, setgid and sticky are dropped, and owner and group are not carried over, silently. The policy is undocumented. | R\*: `mode before=grwxr-xr-x after=-rwxr-xr-x` |
| C6 | `updater.go:517` | The transformed-staging check skips the `StagingOwner` test when the staging basename equals the target's. A session that is not a `StagingOwner` passes, against 0004 G7's "fails closed". | R: `sessionIsStagingOwner=false installRan=true err=<nil>` |
| C7 | `types.go:63-64`, `updater.go:115-178` | `Request.Platform` is honoured on apply. `Run` installs another platform's binary over the running executable, unless the optional `NewImageVerifier` is configured. The CLI never sets it; library callers are exposed. | R: `apply with Request.Platform={plan9 386} on {darwin arm64}: applied=true err=<nil>` |
| C9 | `stream.go:46-59,80-104` | The zero value of the exported `ConfirmNeeded` and `CredentialNeeded` deadlocks on `Answer`, `Supply` or `Cancel`: the reply channel is nil. A TUI adapter's tests that build one by hand would hang. | R: `zero ConfirmNeeded.Answer: blocked >500ms`; the same for `CredentialNeeded.Cancel` |
| C10 | `credentials.go:34-36`, `github.go:638,666` | `CredentialRequest.Interactive` is never true, even in a Stream where `PromptCredential` prompts. Its comment still says "Phase 1 sources always set it false". | C |
| C11 | `types.go:522`, `updater.go:295-298` | `EventComplete` is documented as "emitted after a healthy committed installation". A dry run emits it with nothing installed; only `Detail` tells them apart. | C |
| D4 | `scripts/go-precheck.sh:146-159` | govulncheck fails open. A failure mentioning `dial tcp`, `timeout` or `proxy` prints "skipped" and exits 0, and the summary still says govulncheck passed. `GO_PRECHECK_SKIP_VULN=1` already exists for that. | R\*: a stub printing a `dial tcp` error: `… skipped.` then `1 file(s) clean (…, govulncheck)` rc 0 |
| D5 | `publish-selfupdate-release.yml:48-49` | The "Check gh release capabilities" probe cannot fail. `gh release <anything> --help` exits 0. An old gh then fails only in the final wait loop, after the release is published. | R\*: `gh release frobnicate --help` rc 0 on gh 2.102.0 |
| D6 | `scripts/check-workflows.sh:79` | The gh-repo rule strips everything after whitespace and `#`, even inside quotes, and misses `/usr/bin/gh`. Neither shape is in today's workflow. | R\*: both planted steps gave rc 0 "ok"; the control gave rc 1 |
| D7 | `.github/workflows/ci.yml` | ci.yml has no `permissions:` block, and its checkout keeps `persist-credentials: true`, while CI runs fetched tools (`npx markdownlint-cli2`, `go run actionlint@…`). The release workflow already does both right. | C (the author's grep found neither) |
| D8 | `scripts/verify-selfupdate-release.sh` | The verifier accepts a zero-byte canonical binary. The client refuses `Size <= 0`, so such a release publishes but cannot be installed. | R\*: an empty `tool-linux-amd64` with a matching `SHA256SUMS`: `ok` rc 0 |
| D9 | `publish-selfupdate-release.yml:141` | A stable tag is published without `--latest`. GitHub marks the newest non-prerelease by date as latest, so a backport (`v1.4.x` after `v1.5.0`) would become latest. Clients on `v1.5.0` then get `ErrLatestOlder`, exit 1, on every check. | C; the client effect is reasoning |
| D10 | `Makefile:8-9` | The PATH fallback for `GOVULNCHECK` and `GOTESTSUM` is dead: `$(GOPATH_BIN)/govulncheck` is not wrapped in `$(wildcard)`. A tool only on PATH is reported "not found". | R\*: `govulncheck not found…` rc 2 with a stub first on PATH |
| D11 | `Makefile:40` | The `make lint` install hint says `golangci-lint@latest`. 0006 pinned v2.14.0, as `go-precheck.sh` and CI do. | C |
| D12 | `types.go:55-62` | The `Request.CurrentVersion` and `TargetVersion` comments still say a strict `vMAJOR.MINOR.PATCH`. Since 0005, the configured policy decides. | C |
| A15, B13 | `github.go:490-492,693-695`, `verify.go:21-34`, `session.go:16,368`, `lock_*.go:37` | Dead code: the 2xx branch of `mapStatus`; a duplicate `asset.ID` check; `verifyManifest`, with no non-test caller; `installSession.policy`, written and never read; and the `timeout == 0` branch of `acquireLock`, which no constructor can reach. | C |

### Gaps

| ID | Where | Gap | Evidence |
| :--- | :--- | :--- | :--- |
| A13 | `github.go:177,390,439`, `checker.go:166` | Four checks survive deletion: the release id/tag check in two places, the redirect cap, and the `Draft` refusal in `discover`. That refusal is the only draft guard for a custom `ReleaseSource`. | R\*: each mutation `SURVIVED rc=0`; fourteen other mutations were killed by a named test |
| A14 | `selfupdatetest/githubserver.go:18-26,181` | The exported fake records only whether `Authorization` was present, and `RequireToken` accepts only `Bearer`. A consumer with a custom-header credential cannot prove the header stays off the download origin. | C |
| B11 | `cleanup_other.go`, `standalone.go:51-67` | Nothing removes leftovers from a crash. A SIGKILL or power loss can leave `.<base>.selfupdate-*` staging files, as large as the binary, and `-bak-*` backups. On Unix, `CleanupPending` only takes and releases the lock. | C: no non-test code lists the directory |
| B14, C12 | the coverage report | Untested behaviour: <br>• the managed installer with a Transformer (`managedSession.Owns` and `Target` at 0 %); <br>• a managed service that is installed but stopped; <br>• the default `os.Executable` path and a relative `ExecutablePath`; <br>• a busy pending backup on Windows; <br>• the terminal-event sequence when `Close` or the `complete` report fails; <br>• `cli.StdioOptions`; <br>• a goroutine-leak check in `cli/run_test.go`, though `TestConfirmation` drives a prompt confirmer and 0004 H6 asks for one. | C |
| D13 | `.golangci.yml:54` | Only the four existing packages have per-package depguard rules. A new package, such as an `internal/` helper that AGENTS.md invites, gets only the module floor. | R\*: a planted `internal/helper` importing `x/sys/unix` passed depguard |
| D15 | README, `docs/architecture.md` | No document says how to recover when the publish job fails after "Create a draft release". A re-run is refused by `refuse-existing-release` ("drafts included"). | C |

### Incomplete features: the 0004 roadmap

No record claims an absent item is done. Every record's status agrees
between its front matter and `docs/README.md`.

| Item | Record | Status |
| :--- | :--- | :--- |
| `selfupdate`, `Checker`, `Stream`, events; `selfupdate/cli`; `buildinfo`; `selfupdatetest` | 0004 §3–§5 and its PLANs | built |
| Harness H1–H7 | 0004 §8 | built: regression tests, five fuzz targets and the differential, the fake server, the running-copy end to end on three OSes, golden tests, `setSeam` and leak checks, and CI's race, shuffle and cross vet |
| The `ManifestVerifier` signing seam | 0004 "Release signing" | built; A1 affects it |
| Prerelease channels | 0005 | built; D1 breaks its §5 claim that the publish gate mirrors §1 |
| `selfupdate/codesign` | 0004 §1, §7 Phase 4 | absent; no PLAN |
| `selfupdate/service/{systemd,launchd,scm}`, `PollHealthy`, `ExecReconciler` | 0004 §1, §7 Phase 4 | absent; no PLAN |
| The build-and-stage reusable workflow | 0004 §7 Phase 4 | absent; it needs its own record |
| The installer templates (`install.sh`, `install.ps1`) | 0004 §7 Phase 4 | absent; no PLAN |
| `selfupdate/archive` | 0004 §1 only, "covered here" | absent, and **never scheduled**: no phase or PLAN names it (D14) |
| `verify/signednote`, `verify/ghattest`, `gitlab`, `httpmanifest` | 0004 §1, §6 | absent, deferred to their own records, as recorded |
| `go-tui-lib/updatetea`, and the Phase 2 Bubble Tea confirmation | 0004 §4 | external; still open in go-tui-lib |

0009 narrowed this module to self-update. Phase 4's service lifecycles,
installers and codesign are still self-update tooling, and still fit. The
archive extractor fits too, but nothing schedules it.

### Checked and fine

* **0003's fixes, re-checked in each area:**
  * tag pinning; extras not poisoning a release; non-https redirects
    refused; trailing JSON refused; `Retry-After` overflow; owner and repo
    dot names; sanitised text;
  * the lock's `Lstat`, `O_EXCL` and `SameFile`; a failed restore
    reported; the mode kept in the copy fallback; staging deregistered only
    after the rename; receipt validation;
  * nil verifiers refused; `errNotCommitted`; EOF read as a decline; event
    order; the manifest parsed before staging; errors wrapped; `Close`
    before `complete`.
* **Cross-origin credential scrubbing,** of `Authorization` and of a custom
  header, including after `WithCredentials`. The digest checks on download
  and in `verifyIntegrity`. `SHA256SUMS` parsing. The channel grammar, and
  `Admits`.
* **`Stream`:** `Finished` comes last, then `io.EOF`; cancellation; progress
  coalescing; first reply wins. Option precedence in `RunWith` and `Start`.
  Every exported name in the v1.1.0, v1.2.0, v1.4.0 and 0005 PLANs' API
  blocks exists.
* **The CLI's flag rules,** stdout kept empty without `--json`, and
  `buildinfo`'s release rule. Every `Example` has an output check.
* **Windows files** vet and test-compile with cgo off.
* **The release workflow:**
  * every `${{ }}` reaches `run:` only through `env:`;
  * `GH_REPO` is set on every repository-scoped step;
  * actions are pinned to the peeled commits of their tags;
  * there is no `--clobber`.
* **Documentation:**
  * every backticked API name in the README, `architecture.md` and the
    guides resolves;
  * the migration guide's code compiles;
  * every relative link and anchor in 34 Markdown files resolves.
* **Identifiers:** none in `HEAD` or in history. The scan covered the local
  account name, the short hostname and the hostname domain.

## Decision Drivers

* **One consumer runs `v1.5.0` today,** and more are planned (go-tui-lib,
  pi-go). A fix that keeps every documented contract can ship at once, in a
  patch release.
* **Some findings are a gap between two documents, not just a defect.**
  They need the owner to pick a contract before code changes: A3, B3, C2,
  C7, C9, C11, B11, B12, D9.
* **The API gate.** A patch release must stay `compatible with v1.5.0`
  under `make apicheck`. A new exported name, or a changed documented
  behaviour, belongs in a minor release.
* **Release tooling ships with no tag.** Consumers pin the reusable
  workflow by commit. The scripts and CI change on `main`.
* **Every fix comes with the test that would have caught it.** Each test
  is seen to fail on `HEAD` (0003's practice).
* **The owner prefers a complete, extensible surface** to cuts made only
  for scope.

## Considered Options

* **A. Three tracks:**
  * tooling fixes on `main`, with no release;
  * `v1.5.1` for every code fix that keeps the documented contract;
  * `v1.6.0` for the contracts the owner decides.

  The roadmap gaps go to a 0004 amendment.
* **B. Everything in one `v1.6.0`.**
* **C. Medium findings only;** Low findings and gaps are tracked.
* **D. Record the findings, and fix nothing now.**

## Decision Outcome

Chosen option: **"A. Three tracks"**, because it ships the fixes that keep
every documented contract at once, without waiting on contract decisions.
It also keeps `v1.5.1` provably compatible, so the first consumer can take
it with no code change.

### 1. Tooling, on `main`, with no release

These fixes need no tag. Consumers pin the reusable workflow by commit, so
D5 and D9 ship when the README's pin moves to the commit that carries them.

* **D1:** `[0-9]` in the tag gate. Add Unicode-digit cases to its test.
* **D2:** add verifier tests for:
  * a binary that differs from its `SHA256SUMS` line;
  * a `SHA256SUMS` that also lists an extra file;
  * an extra named `SHA256SUMS-x`.
* **D3:** map a deleted `.go` argument to its package, and still lint,
  vet and test it.
* **D4:** an unreachable vulnerability database fails, unless
  `GO_PRECHECK_SKIP_VULN=1` is set. The summary names only the checks that
  ran.
* **D5:** probe gh's capabilities by parsing `gh release --help`, and set
  a `gh version` floor.
* **D6:** quote-aware comment stripping, and an optional path before `gh`.
* **D7:** `permissions: contents: read`, and `persist-credentials: false`,
  in ci.yml.
* **D8:** refuse an empty canonical binary.
* **D9:** see Q7.
* **D10, D11:** fix the Makefile's PATH fallback and its lint hint.
* **D13:** a catch-all depguard rule allowing only `$gostd` in any package
  no rule names.
* **D15:** a short recovery runbook, in the README.

### 2. `v1.5.1`: every fix that keeps the documented contract

Each item makes the code do what its documentation already says, or
removes a crash. `make apicheck` must report `compatible with v1.5.0`.

* **Integrity and credentials:**
  * **A1:** compare the digest on the read that reaches `Size`, and in
    `Close` once the body is complete.
  * **A2:** do not keep an `ErrNoCredential` or refused outcome across
    runs. Each run, and each `Start`, resolves again.
  * **A5:** strip the query from a `*url.Error`'s URL before returning
    it.
  * **A6:** refresh only on a 401 from the API origin.
  * **A7:** resolve outside the source mutex, with a context-aware wait.
  * **A8:** wrap verifier failures in `ErrIntegrity`, as the manifest path
    already does.
  * **A9:** refuse any control byte in a credential value, except HTAB.
  * **A10:** allow a plain-http loopback hop only when the API base is
    loopback.
  * **A11:** refuse `.` and `..` in `ByTag`.
  * **A12:** refuse `:` in checksum names on every OS, and in the Python
    mirror.
  * **A4:** clamp `NotBefore` to between now + 1 min and now + 1 h. Treat
    a stored value beyond the cap as invalid.
* **Filesystem and install:**
  * **B1:** a named buffer field, so only the capped `Write` is
    reachable.
  * **B2:** skip an unavailable or unusable home directory as a root. Fail
    only when no remaining root covers the target.
  * **B4:** stop the service before rolling back, once `Start` has
    succeeded.
  * **B5, B6:** a busy pending backup, or a busy `.previous`, keeps its
    receipt and does not fail `Begin` or `Commit`. Proven on the Windows
    test host.
  * **B7:** check the directory after the rename in `Apply`, and undo
    through the root. A refused `Commit` goes through the managed
    recovery.
  * **B8, B9:** report `Backup` only while the file exists. Surface it
    whenever it is non-empty.
  * **B10:** `Commit` and `Rollback` on a closed session return an error.
* **Coordinator and CLI:**
  * **C3:** a write failure at the end of `--check` exits 1, and is
    printed.
  * **C4:** every failure after flag parsing writes the `--json` result
    object.
  * **C5:** `isNil` for the managed installer's `Lifecycle` and
    `Reconciler`.
  * **C6:** always require `StagingOwner` for transformed staging.
  * **C10:** set `CredentialRequest.Interactive` when the run carries a
    Stream.
* **Documentation and cleanup:**
  * **D12:** correct the `Request` comments.
  * **A15, B13:** remove the dead code.
* **Tests:**
  * every finding above gets a regression test that fails on `HEAD`;
  * A13's four surviving mutations get a killing test;
  * the B14 and C12 gaps get tests, including a leak check in
    `cli/run_test.go`.

### 3. `v1.6.0`: the contracts the owner decides

Each of these changes a documented behaviour, or adds an exported name. The
recommendations are written as owner questions below.

* **A3:** what `CheckCached` caches (Q1).
* **B3:** whether an update starts a stopped service (Q2, as the owner
  answered it):
  * an optional `EnabledLifecycle` interface, `Enabled(ctx, product)
    (bool, error)`, reports whether the service is configured to start:
    systemd `is-enabled`, launchd `RunAtLoad` or `KeepAlive`, Windows SCM
    start type automatic;
  * after the replacement, `ManagedInstaller` starts the service, and waits
    for it to be healthy, only when it was running, or it is configured to
    start;
  * a stopped service that is not configured to start stays stopped;
  * when the `Lifecycle` does not implement `EnabledLifecycle`, only a
    service that was running is started again;
  * `InstallResult.ServiceStarted` reports whether it was started.
* **C2, C11:** the terminal-event rule (Q3). An appended `EventWarning`
  kind carries an error that follows `EventComplete`, and `Result.Warnings`
  lists them. The run then exits 0.
* **C7:** `Request.Platform` on apply (Q4).
* **C9:** the zero value of the reply types (Q5).
* **B11, B12:** crash-leftover cleanup, and the mode and ownership policy
  (Q6).
* **A14:** `selfupdatetest.GitHubServer` records credential-bearing header
  names, and `RequireToken` takes a header name. This is additive.

### 4. The roadmap

A 0004 amendment, in its own change:

* schedules `selfupdate/archive` in Phase 4, or drops it from §1 (Q8);
* restates that Phase 4's `codesign`, `service` and installer templates
  stay planned, with no PLAN yet.

This record builds none of them.

### Consequences

* Good, because the contract-preserving fixes, the integrity seam (A1)
  and the credential prompt (A2) among them, ship in a patch any consumer
  can take as is.
* Good, because the tooling holes close with no release: the tag gate
  (D1), the verifier's own checks (D2), and the pre-add gate (D3).
* Good, because each contract change waits for an explicit decision, and
  ships in a minor release with a named behaviour change.
* Neutral, because `v1.5.1` is a large patch: about 30 code findings.
  Each is small, and each has its own test.
* Bad, because two releases, plus a 0004 amendment, mean three rounds of
  records and review.
* Bad, because B5 and B6 are Windows-only, and this pass could only reason
  about them. They are proven on the Windows test host before `v1.5.1`, as
  0003's B11 was.

### Confirmation

* Each finding fixed in a track has a test, or a planted-break proof for
  scripts. It is seen to fail on `HEAD`, on a scratch copy, before the fix
  lands.
* `v1.5.1`:
  * `make apicheck` reports `compatible with v1.5.0`;
  * `make lint` passes for three GOOS;
  * `go test -race` passes on Linux and macOS;
  * the Windows test host passes, including B5 and B6;
  * `make fuzz` and `make vuln` pass;
  * CI is green on the tag.
* Tooling: each script's `*_test.sh` gains the cases above, and each is
  seen to fail on the old script.
* prepare-commit-msg takes `v1.5.1` with no code change: `go get`, then
  `make verify`.

## Pros and Cons of the Options

### A. Three tracks

* Good, because the fixes that keep every contract ship at once, and stay
  `apicheck`-compatible.
* Good, because the tooling, which carries no version, is fixed on `main`
  straight away.
* Good, because contract changes get a decision first.
* Bad, because there are more records and releases than with B.

### B. Everything in one `v1.6.0`

* Good, because there is one release and one round of review.
* Bad, because the integrity and credential fixes wait for every
  contract decision.
* Bad, because a consumer must take behaviour changes to get bug fixes.

### C. Medium only

* Good, because it is smaller.
* Bad, because it leaves known misreports (B8, B9), unlocked mutation
  (B10) and leaks of signed URLs (A5). Each is cheap to fix now.
* Bad, because it is contrary to the owner's stated preference.

### D. Record only

* Good, because it costs nothing now.
* Bad, because the first consumer stays on code with a broken integrity
  seam (A1), a broken prompt (A2), and a release gate that can publish an
  uninstallable "latest" (D1).

## Owner questions

Each has a recommendation, written into §3 and §4.

**Answered 2026-10-03.** The owner: "questions, follow recommendations,
except where i choose, q2 no, if a service was not configured to start do
not start it. option a." Option A is chosen. Q1 and Q3–Q8 follow their
recommendations. Q2 is answered below.

* **Q1 (A3).** Should `CheckCached` cache deterministic outcomes
  (`ErrLatestOlder`, a missing platform asset, a mutable release) for
  `maxAge`, so the docs' "at most once per interval" holds? *Recommended:
  yes, with a schema bump, reading the old schema as a miss.*
* **Q2 (B3).** Should an update start a service that was installed but
  not running? *Recommended: no. Start and health-check only a service
  that was running, and document it. A caller that wants a start can
  start the service after `Run`.*
  **Owner (2026-10-03): "no, if a service was not configured to start do
  not start it."** Read as: after the update, start the service only when
  it was running, or it is configured to start. A running service is
  always started again. §3 records the mechanism.
* **Q3 (C2, C11).** What is the terminal-event rule? *Recommended:*
  * *exactly one terminal event;*
  * *once `EventComplete` is reported, a later error becomes an advisory
    warning, and the exit code is 0, because the binary was replaced;*
  * *a dry run gets its own `Detail`, documented, and `EventComplete`'s
    doc says so.*
* **Q4 (C7).** Should `Request.Platform` on apply be refused when it is
  not the running platform? *Recommended: refuse it, except under
  `CheckOnly` and `DryRun`. The end-to-end tests set it explicitly through
  a test seam.*
* **Q5 (C9).** What should a zero-value `ConfirmNeeded` or
  `CredentialNeeded` do? *Recommended: a non-blocking reply, so the zero
  value is safe and does nothing. Document it.*
* **Q6 (B11, B12).**
  * *Crash leftovers. Recommended: under the lock, remove the staging and
    backup siblings that match this product's prefixes and are older than
    the lock's holder.*
  * *Special mode bits. Recommended: refuse to replace a target with
    setuid or setgid set, unless a new `TargetPolicy` field allows it.
    Carry owner and group over where permitted.*
* **Q7 (D9).** What should a stable tag lower than the current latest do?
  *Recommended: publish it with `--latest=false`, and say so in the job
  summary.*
* **Q8 (D14).** What happens to `selfupdate/archive`? *Recommended:
  schedule it in Phase 4, beside `codesign`, since archive releases are
  self-update input.*

## More Information

* **The reviewers' probe tests and plant scripts** stayed in the
  session's scratch space and are not committed. Each finding's evidence
  is quoted above. A PLAN for each track, after review, turns each probe
  into a committed regression test.
* **Related records:**
  * [0003-MADR-remediate-debugging-pass-findings.md](0003-MADR-remediate-debugging-pass-findings.md):
    the first pass;
  * [0004-MADR-evolve-selfupdate-api-and-tui-support.md](0004-MADR-evolve-selfupdate-api-and-tui-support.md):
    the roadmap, and the contracts most findings are measured against;
  * [0005-MADR-opt-in-prerelease-channels.md](0005-MADR-opt-in-prerelease-channels.md):
    the channel grammar D1 must mirror;
  * [0009-MADR-rename-to-go-selfupdate-lib.md](0009-MADR-rename-to-go-selfupdate-lib.md):
    the module's current scope.
* **The PLANs,** one per track:
  * [0010-PLAN-tooling-fixes.md](0010-PLAN-tooling-fixes.md): §1 and §4;
  * [0010-PLAN-v1-5-1-contract-preserving-fixes.md](0010-PLAN-v1-5-1-contract-preserving-fixes.md):
    §2;
  * [0010-PLAN-v1-6-0-owner-contracts.md](0010-PLAN-v1-6-0-owner-contracts.md):
    §3.
