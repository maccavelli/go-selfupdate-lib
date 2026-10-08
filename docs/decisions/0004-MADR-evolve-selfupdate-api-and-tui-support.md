---
status: accepted
date: 2026-10-02
decision-makers: go-core-lib maintainers
consulted: ocp-login maintainers (the TUI updater this record generalises); go-tui-lib maintainers (the proposed home of the Bubble Tea adapter)
informed: owners of the six selfupdate consumers
---
# Evolve `selfupdate` into a layered, framework-neutral update toolkit

## Context and Problem Statement

On 2026-09-30, after `v1.0.0` was tagged, the owner asked for a second
evaluation of `selfupdate`:

> evaluate the self-update codebase, assess its api, features, functions,
> and protocol/command surfaces. run a multi-analysis pass part debugging,
> part feature assessment. look for bugs and gaps, missing wiring, ways to
> improve its harness and tests, and also ways to improve/enhance it's
> functionality and expand it's api. assess the self-update wrapper in the
> ocp-login repo TUI code that calls self-update, and determine if we can
> enhance it to provide standard, idiomatic, support for TUI applications to
> extend it similarly to how ocp-login does today. write findings into an
> madr for review. i want to write this to be as flexible, extensible, and
> canonical as possible.

The record has two parts:

* **What is wrong or missing in `v1.0.0`:** defects, API gaps, harness gaps,
  and defects in the programs that use it.
* **How `selfupdate` should grow:** into a toolkit that a plain CLI, a
  service, and a TUI can each extend without forking. It takes ocp-login's
  hand-written updater and Bubble Tea front end as the benchmark.

### Method

* Four read-only reviews ran in parallel, one per area:
  * **API and features**: the exported surface, its extension points, and
    a comparison with `creativeprojects/go-selfupdate`,
    `rhysd/go-github-selfupdate`, `minio/selfupdate` and
    `sanbornm/go-selfupdate`.
  * **Consumers**: the six programs on `mcplib/selfupdate`, their
    command surfaces, and how much code they duplicate.
  * **ocp-login**: its GitLab updater, its update command and its Bubble
    Tea step runner. *(Re-worded to patterns by amendment P1.)*
  * **Round-2 debugging and harness**: the code that
    [0003-MADR-remediate-debugging-pass-findings.md](0003-MADR-remediate-debugging-pass-findings.md)
    added, coverage, fuzzing, and the workflow checkers.
* Every experiment ran on scratch copies, including a `git archive HEAD`
  copy of this repository. No repository was modified.
* The author re-read the code behind every finding marked **verified**
  below before writing it down. Reproductions quote the probe's own output.

### 1. Defects in `v1.0.0`

| ID | Sev | Finding | Evidence |
| :--- | :--- | :--- | :--- |
| R1 | Medium | **The 0003 B1 fix stops short of `Run`.** `Install` returns the backup path with `Applied:false` when the new binary is live and the restore failed (`session.go:69-72`). `apply` copies only `PendingBackup` (`updater.go:295`), so the caller never learns the path, and on Unix nothing ever cleans the backup. | Verified. Reproduced: `Applied=false PendingBackup="" exit=1`, and the directory still held `.demo.selfupdate-bak-*`. |
| R2 | Medium | **The 0003 C7 confirmer reader never stops reading.** The goroutine loops on `Scanner.Scan()` for the life of the process (`confirmer.go:42-56`). After an answered `Confirm` it swallows whatever the host program reads next. A second confirmer on the same descriptor loses its input to the first. With two lines pending and no further `Confirm`, the goroutine blocks forever on the channel send. | Verified. Reproduced: host read `n=0 err=EOF` after its `"host-command\n"` was swallowed; `c2 … context deadline exceeded` because c1 took its `y`. This is a regression from 0003. |
| R3 | Low | **`checkDir` finds a directory swap after the rename; it does not prevent one.** It is a path-based `os.Stat` (`session.go:74-76`). A swap after the rename returns `Applied:true` with the backup left behind, and no rollback runs. On Windows, the stored `dirInfo` stays pinned only because `beginSession` happens to call `SameFile` at once. | Reproduced: `Applied:true … target directory changed … concurrent update`. |
| R4 | Low | **`moveFileReplace` retries every access-denied error for 5 s** (`replace_windows.go:80-99`) and ignores `InstallOptions.LockTimeout`. A genuine denial now fails slowly. The standalone restore after a sync failure runs on the caller's context, so cancellation can abort a busy restore. That is the standalone counterpart of 0003 B2. | Reasoning only. |
| R5 | Low | **Windows receipt processing runs before the directory identity check** (`session.go:204-219`, `cleanup_windows.go:82-92`). It hashes by joined path but removes through `root`, so after a swap it can hash one file and delete another. | Reasoning only. |
| R6 | Low | **The release verifier's regexes accept a trailing newline.** They use `re.match` with `^…$` (`scripts/verify-selfupdate-release.sh:57-60,107`), and Python's `$` matches before a final `\n`. That defeats 0003 D5's "never a word separator" guarantee. | Verified. Reproduced: `extras=['notes\n'] … exit=0`, `tag='v1.2.3\n' … exit=0`. |
| R7 | Low | **`check-workflow-expressions.sh` misses valid YAML shapes.** It misses `run: \|  # c`, `run: \|2`, a plain multi-line scalar, and a quoted `"run":` key. It also flags `- run: \|` followed by an `env:` that holds `${{ }}`. | Each planted case was checked with PyYAML to see what GitHub would parse. |
| R8 | Low | **`check-workflow-gh-repo.sh` has false negatives:** an `echo "GH_REPO: …"` line, `gh run` / `gh workflow`, the vendored `refuse-existing-release.sh`, and a `\|\| gh release --help` tail. It also has **false positives:** an `env:` placed after `run:`, and a job-level `env:`. | Planted cases. |
| R9 | Info | **`validateAssetStructure` accepts `.` and `..` as asset names** (`github.go:280-288`). Found by `FuzzGitHubReleaseJSON`. It is not exploitable: selection is by exact name, and asset names never become paths. | Verified. |

Checked and fine in round 2:

* Real GitHub downloads work under the https-only redirect rule. A 302 to
  `release-assets.githubusercontent.com` succeeds, and `Authorization` is
  dropped on the host change (`github.go:145-157`).
* The Python and Go manifest parsers agreed on 40,000 random manifests. A
  planted `>=`→`>` mutation was caught 12 times in 20,000.
* `openLockFile`, `closeSession`, `isNil` and `errNotCommitted` behave as
  0003 intended.
* `go test -race -count=5` and `-shuffle=on -count=3` pass.

R1 and R2 are in code that 0003 added or changed. `mcplib` `v1.6.0` has
neither of those changes, so its versions of R1 and R2 differ, and this
record does not assess them.

### 2. API and design gaps

Each gap below limits a caller that is not the GitHub-plus-terminal case
`v1.0.0` was built for.

| ID | Gap | Evidence |
| :--- | :--- | :--- |
| G1 | **The model is GitHub-shaped.** The Updater requires `Asset.ID > 0`, `State == "uploaded"` and `Release.Immutable` of *every* `ReleaseSource`. A GitLab, Gitea or plain-HTTP source has to fabricate all three. `Result.ReleaseURL` is documented as the GitHub HTML URL. | `github.go:281,325`, `updater.go:107`, `types.go:116` |
| G2 | **The version policy is only nominally pluggable.** `validateRequest` hardcodes `NewStrictVersionPolicy()` for the current and target versions. Prereleases are rejected unconditionally. A custom `VersionPolicy` can only be stricter. | Verified: `version.go:72,77`; `updater.go:110` |
| G3 | **A check needs the whole install stack.** `ResolveTarget` runs before any network call, even with `CheckOnly`. `Confirmer` and `Reporter` are mandatory. "Update available" is returned as an error. So a startup banner or a `doctor` command must build an `Installer`, and it fails for a binary outside the allowed roots. | Verified: `updater.go:94`, `updater.go:38-43`, `updater.go:156` |
| G4 | **Some outcomes cannot be told apart.** `errForceRequired` and `errLatestOlder` are unexported, so a CLI cannot distinguish "needs `--force`" from a network failure. | `errors.go:36-37` |
| G5 | **Reporting is push-only and coarse.** A reporter error aborts the run. There is one `EventDownloadingBinary`, whose `Bytes` is the advertised size, sent before the copy. The copy loop has no progress hook. There is no failed, declined or rolled-back event. There is no structured (JSON) reporter or result document. | `updater.go:230`, `download.go:88`, `reporter.go:16-28` |
| G6 | **The built-in confirmer needs a real `*os.File`.** It calls `Fd()`. There is no `io.Reader` variant and no function adapters (`ReporterFunc`, `ConfirmerFunc` and the like). | `confirmer.go:38,65` |
| G7 | **Custom installers are second-class.** `ManagedInstaller` accepts only `*StandaloneInstaller` and type-asserts `*installSession`. `sessOwns` returns `true` for any other session type, so the staging-ownership check is silently skipped. A caller cannot build a `Target` that passes `revalidateTarget`. | Verified: `managed.go:20,44`; `updater.go:366-379`; `types.go:305` |
| G8 | **Some documentation contradicts the code.** `StagedArtifact.Size` is documented as the size after any transform, but the Updater passes the pre-transform `sel.Binary.Size`. `runVerifiers` passes the release digest in a field named `ManifestSHA256`. | Verified: `types.go:278` vs `updater.go:287`; `updater.go:329` |
| G9 | **Integrity stops at the checksum.** There is no seam for a signature over the manifest. The manifest must be SHA256SUMS, parsed by an unexported parser. Nothing checks that the binary is an executable for this OS and architecture, and nothing runs it before or after install. The release workflow already produces build attestations that nothing verifies at runtime. | `updater.go:216-229`, `checksums.go:42`, `doc.go:9-13`, `.github/workflows/publish-selfupdate-release.yml` |
| G10 | **Credentials are a fixed string.** The token is a constructor field, or `GH_TOKEN` / `GITHUB_TOKEN`. There is no provider chain, no prompt fallback, and no hook for "this credential was accepted, save it". | `github.go:102` |
| G11 | **Lifecycle operations are missing.** There is no dry run and no verify-only mode. There is no startup call that cleans a Windows pending backup (cleanup happens only inside the next `beginSession`). There is no way to keep the previous binary for a local rollback. | `session.go:204`; `replace_unix.go:60-66` |
| G12 | **Limits are split.** `Config.Limits.ReleaseJSON` and `ErrorBody` are validated but take effect only through the source's own limits. This is documented but easy to miss. | `types.go:536-540` |

### 3. Harness gaps

* **Coverage is 85.2 %**, up from 75.5 % before 0003. Several 0003 defences
  are still untested:
  * the race and `SameFile` branches of `openLockFile` (69.6 %);
  * the "changed while locking" branch of `beginSession`;
  * the check after the rename;
  * the rollback and sync failures in `rollbackReplacement` and
    `commitReplacement`;
  * a rollback failure inside managed recovery;
  * the error branches of `copyFile` and `readTruncated`.
* **There are no fuzz targets.** Four were written and run for 30 s each,
  and are ready to commit: `FuzzParseSHA256SUMS`, `FuzzSanitize`,
  `FuzzVersionPolicy` and `FuzzGitHubReleaseJSON`. The last one found R9.
* **There is no end-to-end test.** Nothing drives `Updater.Run`, with the
  real standalone installer, against a server that redirects asset
  downloads to a second origin.
* **There are no golden tests for `TextReporter` output.**
* **Test isolation.** No test calls `t.Parallel`. Thirteen package-level
  seams are mutated, one of them by hand (`fs_test.go:13`). The confirmer
  tests use `time.Sleep` handshakes and have no goroutine-leak check, which
  is why R2 went unseen.
* **CI:**
  * `-race` runs on Linux only;
  * there is no shuffled run;
  * there is no cross `go vet` for the BSDs or 32-bit;
  * the workflow checkers have no planted-shape tests (R7, R8).

### 4. Defects in the programs that use it

These are outside this repository and are listed so they are not lost.
Each needs a fix in its own repository, under that repository's records.

| ID | Program | Finding | Evidence |
| :--- | :--- | :--- | :--- |
| C1 | mcp-server-socratic-thinker | **Self-update can never select an asset.** `Product` is `cliName`, which is `"socratic-thinker"`. The published assets are `mcp-server-socratic-thinker-<os>-<arch>`. The User-Agent and output prefix are wrong for the same reason. | Verified: `cmd/mcp-server-socratic-thinker/update.go:109`, `constants.go:5` |
| C2 | mcp-server-recall, mcp-server-magictools, mcp-server-socratic-thinker | **Released binaries are stamped as local builds.** CI runs `make build-all VERSION="$TAG"` without `BUILD_KIND=release`, and each Makefile defaults to `local`. `--check` therefore always exits 10, and apply always needs `--force`. mcp-server-duckduckgo, magic-cli-remote and prepare-commit-msg are correct. | Verified in each `ci.yml` and `Makefile` |
| C3 | the four cobra MCP servers | **Exit 10 prints a spurious error.** None sets `SilenceErrors` / `SilenceUsage`, so cobra prints `Error: selfupdate: update available` and the full usage before exiting 10. | Observed on a published socratic binary |
| C4 | five of six | **Ctrl-C does not cancel an update.** No signal context is set, so Ctrl-C kills the process mid-update. Only prepare-commit-msg wires signals. | `main.go` in each |
| C5 | all six | **CI still passes `bridge-release`**, which this repository's workflow does not declare. Each migration will fail at CI until the input is removed. | Already recorded in [0002-MADR-rehome-selfupdate-from-mcplib.md](0002-MADR-rehome-selfupdate-from-mcplib.md) §3 |
| C6 | five of six | **They are on `mcplib` `v1.4.1`, not `v1.6.0`.** | `go.mod` in each |

ocp-login does not use `selfupdate`. Its own updater has these defects:

| ID | Finding | Evidence |
| :--- | :--- | :--- |
| O1 | **On Windows the update receipt is never read, and stale helper files are never cleaned.** The code that would do both has no production caller. | Verified in ocp-login's source |
| O2 | **On Windows the post-install smoke test runs the old binary.** The commit only starts a helper that waits for this process to exit. The smoke test that follows runs the old executable, which reports the old version, so the update is reported as failed and then installed anyway. | Verified in ocp-login's source |
| O3 | **The installed version may not be the one the user confirmed.** The prompt names the latest version found. The download step then resolves "latest" again without pinning the tag. | Verified in ocp-login's source |
| O4 | **A credential-handling defect, for ocp-login's maintainers to fix.** | Verified in ocp-login's source |
| O5 | **The library writes straight to `os.Stderr`, which corrupts a live Bubble Tea frame.** | Read in ocp-login's source |
| O6 | **Ctrl-C during "Installing" reports "cancelled" while the install goroutine keeps running.** | Read in ocp-login's source |

*(Re-worded to patterns by amendment P1.)*

### 5. What ocp-login does that `selfupdate` cannot yet carry

ocp-login is the benchmark for the TUI question. Its updater has features
`selfupdate` lacks. It also lacks guarantees `selfupdate` has.

| Capability | ocp-login | `selfupdate` v1.0.0 |
| :--- | :--- | :--- |
| Release source | GitLab, with a credential chain and prompt | GitHub / GHES only (G1, G10) |
| Save a credential only after it is accepted | yes, with read-back | no seam (G10) |
| Byte progress, throttled, with an unknown-total rule | yes | no (G5) |
| Executable format and architecture check | yes (ELF, Mach-O, PE) | no (G9) |
| Smoke test before and after install | yes | no, and staging is not runnable (mode 0600, no `.exe`) (G9) *(corrected: on Windows an extensionless staging file runs; see the §3 amendments)* |
| Dry run | yes | no (G11) |
| Inline Bubble Tea front end, ctrl+c cancels | yes *(re-worded to patterns by amendment P1)* | no framework-neutral bridge exists |
| Immutable releases, advertised-size check, lock, backup and restore, rate-limit errors | no | yes |
| Confirmation happens after selection, inside one run | no (O3) | yes |
| Synchronous Windows replacement, no helper | no (helper and receipt) | yes |

Every row where ocp-login leads is generic, not GitLab-specific, except the
source itself. So a TUI program like ocp-login could adopt `selfupdate`
without losing behaviour if `selfupdate` gains:

* a progress event;
* a way to drive a run from an event loop;
* credential seams;
* probes and an image check;
* a GitLab source.

The last one needs a security decision, because GitLab has no immutable
releases.

## Decision Drivers

* **Canonical.** There should be one command surface, one exit-code
  contract, one JSON document and one set of build stamps across every
  program. Today they vary (§4 and the consumer survey).
* **Extensible without forking.** Every seam must be first-class: no
  concrete-type assertion should decide whether a custom implementation
  gets the same guarantees (G7).
* **Framework-neutral core.** `selfupdate` must serve a plain CLI, a service
  and any TUI framework. It must never import a UI framework.
* **Compatible within v1.** Changes are additive under Go module semver:
  * no method is added to an existing interface;
  * a new field on `Request`, `Result` or `Event` must be of a comparable
    type, because those structs are comparable today;
  * the JSON encoding of the existing enums does not change.
* **Dependency floor.** Core stays on the standard library plus the current
  `golang.org/x/{mod,sys,term}`. Any new module needs a MADR (`AGENTS.md`).
  Charm packages never enter this module.
* **The security posture does not erode by default.** Immutable releases,
  strict tags, HTTPS, and credentials scoped to their origin stay the
  default. Each relaxation is an explicit opt-in with its own MADR.
* **Defects before features.** R1 and R2 are live in `v1.0.0`.

## Considered Options

* **A. A phased, additive roadmap.** Ship a defect release first, then grow
  the API in v1 minor releases in layers. Framework adapters live outside
  this module.
* **B. A `v2` redesign.** Build a source-neutral model with new interfaces,
  in one breaking release.
* **C. Fix the defects only.** Leave extension to each program, as today.
* **D. Build TUI support into go-core-lib.** Put a Bubble Tea adapter in
  `selfupdate`, or in a nested module in this repository.

## Decision Outcome

Chosen option: **"A. A phased, additive roadmap"**, because:

* it fixes the live defects first;
* every proposal found can be made additively within v1;
* it keeps Charm out of the module every consumer imports;
* it gives each security relaxation its own record rather than folding it
  into a feature release.

### 1. The target shape

The design has four layers. Each layer depends only on the layers below it.

```text
 go-tui-lib/updatetea         Bubble Tea adapter: Msgs, Cmds, embeddable Model
 (any other framework)        tview / gocui / plain loops use the same Stream
───────────────────────────────────────────────────────────────────────────────
 selfupdate/cli               canonical command surface, exit codes, --json
 buildinfo                    library-owned build stamps
───────────────────────────────────────────────────────────────────────────────
 selfupdate/<extension>       gitlab, httpmanifest, archive, codesign,
                              service/{systemd,launchd,scm}, verify/*,
                              selfupdatetest
───────────────────────────────────────────────────────────────────────────────
 selfupdate                   coordinator, seams, Checker, Stream, events,
                              GitHub source, standalone and managed installers
```

| Package | Purpose | Dependencies | Needs its own MADR |
| :--- | :--- | :--- | :--- |
| `selfupdate` | Everything in §3 and §4 | stdlib, x/mod, x/sys, x/term | covered here |
| `selfupdate/cli` | §5 | stdlib (`flag`) | covered here |
| `buildinfo` | §5 | stdlib (`runtime/debug`) | covered here |
| `selfupdate/selfupdatetest` | Fakes and the test server (§8) | stdlib | covered here |
| `selfupdate/archive` | tar.gz and zip selector plus extract transformer | stdlib | covered here |
| `selfupdate/codesign` | darwin re-sign transformer, moved from magic-cli-remote | stdlib (`os/exec`) | covered here |
| `selfupdate/service/...` | `PollHealthy`, `ExecReconciler`, and systemd, launchd and Windows SCM lifecycles | stdlib, x/sys | covered here |
| `selfupdate/verify/signednote` | C2SP signed-note release statement, `SHA256SUMS.note` | stdlib plus x/mod (already required) | deferred: designed in the 0004 report; built with the §6 record on releases that are not immutable |
| `selfupdate/verify/ghattest` | runtime attestation check | the exec variant is dependency-free; the sigstore variant is a new module | yes |
| `selfupdate/gitlab` | GitLab release source | stdlib | yes: the meaning of mutable releases (§6) |
| `selfupdate/httpmanifest` | signed JSON index source | stdlib | yes: only safe together with a manifest signature |
| `go-tui-lib/updatetea` | Bubble Tea adapter (§4) | Charm, in go-tui-lib only | a record in go-tui-lib |

### 2. Phase 0: defect release (`v1.0.1`)

This phase makes no API change. Its PLAN is [0004-PLAN-v1-0-1-defect-release.md](0004-PLAN-v1-0-1-defect-release.md).

Five bullets below carry a clarification added on 2026-09-30, when the
PLAN was written against the code. Each is marked *(clarified)*.

* **R1.** When `Applied` is false and `Backup` is set, carry the backup path
  into `Result.PendingBackup`, and name it, sanitised, in the error.
* **R2.** Read on demand: the goroutine reads one line for each request it
  receives on a request channel. A line typed after a cancelled `Confirm`
  still answers the next one, which keeps 0003 C7, and no read is
  outstanding once a `Confirm` is answered.
  *(clarified)* The confirmer starts one read per outstanding request
  and reads byte by byte up to the newline. A buffered reader would read
  ahead and consume the host's next input, which is R2 again by another
  route.
* **R3.** Store the identity of the root *handle* (`root.Stat(".")`) as
  `dirInfo`, and roll back when the check after the rename fails.
  *(clarified)* The rollback renames the backup over the target through
  the locked directory handle (`os.Root.Rename`), not through the path.
  A path names whatever directory is there now; the handle names the one
  that was locked, which is where the rename went.
* **R4.**
  * Bound the Windows busy-image retry with `InstallOptions.LockTimeout`.
  * Retry only while the image is busy, not on every access-denied.
    *(clarified)* A busy running image refuses with access-denied as
    well as a sharing violation (0003 B11), so the error code alone
    cannot tell busy from denied. The retry therefore stops at once when
    the destination carries the read-only attribute, the one genuine
    denial that can be told apart cheaply. Every other access-denied is
    retried only within the `LockTimeout` bound.
  * Run the standalone restore on `context.WithoutCancel`, bounded like
    managed recovery.
* **R5.** Check the directory identity before processing the receipt, and
  hash through `root.Open`.
* **R6.** Use `re.fullmatch` in the verifier.
* **R7 and R8.** Replace both line-scanning workflow checkers with one
  YAML-parsing checker (Python stdlib plus the PyYAML the runner ships).
  Keep the planted shapes as its tests.
  *(clarified)* The Ubuntu 24.04 runner image does **not** ship PyYAML: its
  software list names `libyaml-dev` and `yq`, but no Python YAML package
  (checked 2026-09-30). CI therefore installs `PyYAML==6.0.3` into a
  virtual environment from a hash-pinned requirements file. The development
  hosts already have it. This is a CI tool, not a Go module, so `AGENTS.md`'s
  dependency rule does not apply.
* **R9.** Refuse `.` and `..` as asset names.
* **G8.** Correct the two documentation mismatches. Pass the
  post-transform size, and name the digest field accurately in a new field,
  keeping the old one.
  *(clarified)* `Verification.ManifestSHA256` is already documented as
  "the digest from the SHA256SUMS entry"; `runVerifiers` passes the
  staged digest instead. The fix is the value, so no new field is
  needed.
* **Harness.**
  * Add R1 and R2 regression tests, with a goroutine-leak check.
  * Cover the dark 0003 branches listed in §3 of the context.
  * Commit the four fuzz targets, seeded from `testdata/manifest-parity`.
  * Put every seam behind `setSeam`, and document the no-`t.Parallel`
    rule.

### 3. Phase 1: core API (`v1.1.0`)

Everything here is in package `selfupdate`, standard library only, and
additive.

**Amended 2026-09-30 by [0004-PLAN-v1-1-0-core-api.md](0004-PLAN-v1-1-0-core-api.md).**

* **The PLAN's shapes supersede the sketches.** This section left names
  and shapes to the PLAN, and the PLAN's API blocks are authoritative
  where they differ from the sketches below. That includes
  `NewFileCheckStore`, `NewVersionProber` and `NewImageVerifier`, which
  each also return an error.
* **The amendments.** The owner approved eight with the PLAN. Each is
  marked where it applies:
  * **A1.** A zero `Config.ProgressInterval` emits no progress events;
    progress is opt-in.
  * **A2.** `Prober.Probe(ctx, ProbeRequest)`.
  * **A3.** `Availability` is named like `Result`: `Product`,
    `CurrentVersion`, `TargetVersion`, `ReleaseURL`, `AssetName`,
    `Operation`, `Available`, `ForceRequired`.
  * **A4.** `Apply` returns `AppliedReplacement{Target, Backup string;
    State any}`.
  * **A5.** New fields: `InstallResult.RolledBack` and `.Previous`,
    `InstallRequest.TargetVersion`, and `Result.DryRun` and `.Previous`.
  * **A6.** An empty `Credential.Header` means
    `Authorization: Bearer <Value>`.
  * **A7.** `EventDeclined`, `EventFailed` and `EventRolledBack` are
    advisory, like progress.
  * **A8.** `NewTextReporter` skips `EventProgress`.
* **A corrected fact (Step 8 deviation D3).** The Windows test host ran an
  extensionless staging file, so a missing `.exe` never made staging
  unrunnable; on Unix the `0600` mode does. The suffix is kept as a
  convention.

**Querying without installing (G3, G4).**

```go
type CheckerConfig struct {
    Source   ReleaseSource
    Assets   AssetSelector
    Versions VersionPolicy
    Limits   Limits
}
func NewChecker(CheckerConfig) (*Checker, error)
func (u *Updater) Checker() *Checker

type CheckRequest struct {
    Product, CurrentVersion, TargetVersion string
    CurrentBuild                           BuildKind
    Platform                               Platform
}
type Availability struct { // amended A3
    Operation      Operation
    Current, Latest string
    ReleaseURL     string
    Available      bool
}
func (c *Checker) Check(ctx context.Context, r CheckRequest) (Availability, error)

// Cached, rate-limit-aware check for startup banners; never prompts.
type CheckRecord struct { /* CheckedAt, NotBefore, Current, Latest, ReleaseURL, Available */ }
type CheckStore interface {
    Load(context.Context) (CheckRecord, error)
    Save(context.Context, CheckRecord) error
}
func NewFileCheckStore(path string) CheckStore
func (c *Checker) CheckCached(ctx context.Context, r CheckRequest, s CheckStore, maxAge time.Duration) (CheckRecord, error)

var ErrForceRequired = errForceRequired // same values, now exported
var ErrLatestOlder = errLatestOlder
```

* `Check` resolves no target and needs no `Confirmer`. It returns
  `Available` as a value rather than as an error. `Run --check` keeps its
  current behaviour.
* `CheckCached` sets `NotBefore` from a `RateLimitError`, and treats the
  cached record as stale when `Current` changes.
* It reopens the scope of mcplib `0005-MADR` ("no background polling") only
  for a *check*, never for an apply.

**Honouring the configured version policy (G2).** `validateRequest` uses
`Config.Versions` rather than hardcoding the strict policy. `v1.0.0`'s
default behaviour is unchanged, because the default policy is the strict
one. Allowing prereleases is **not** part of this phase (§6).

**Progress and events (G5).**

* A new `EventProgress` kind is appended after the last existing value, so
  every existing value keeps its number.
* A new comparable field, `Event.Total int64`, is added. For
  `EventProgress`, `Bytes` is the number of bytes done and `Total` is the
  total, with `-1` meaning unknown.
* The download copy emits progress no more often than
  `Config.ProgressInterval` (default 100 ms), and always emits a final event.
  *(Amended A1: a zero interval emits none; progress is opt-in. A8:
  `NewTextReporter` skips progress.)*
  Progress events are **advisory**: an error returned from reporting one
  does not abort the run. An error from any other event still aborts, as
  today.
* New kinds `EventDeclined`, `EventFailed` and `EventRolledBack` are
  appended. `EventFailed` carries the sanitised error class in `Detail`.
  *(Amended A7: an error from reporting any of the three is ignored.)*

**Adapters and defaults (G6).**

* Function adapters: `ReporterFunc`, `ConfirmerFunc`, `VerifierFunc`,
  `TransformerFunc`.
* Built-in values: `DiscardReporter()`, `MultiReporter(...)` and
  `NonInteractiveConfirmer()`, which always returns
  `ErrConfirmationRequired`.
* `NewPromptConfirmer(in io.Reader, out io.Writer, interactive bool)`.
  The existing `NewTerminalConfirmer` becomes a wrapper around it.

**Structured output.**

* `NewJSONReporter(w)` writes NDJSON with stable snake_case keys and the
  kind as a string.
* `Result.Document()` returns a `ResultDocument` with json tags, a
  `schema_version`, and `Operation` as a string. The existing enums get
  **no** `MarshalText`. Adding it would change the `json.Marshal` output of
  `Result` for anyone who marshals it today. The ocp-login review suggested
  `MarshalText`; this record declines it for that reason.

**Credentials (G10).**

```go
type Credential struct {
    Header string
    Value  []byte
    Source string
}
type CredentialRequest struct {
    Origin      *url.URL
    Cause       error // non-nil on a retry after 401/403
    Interactive bool
}
type CredentialProvider interface {
    Credential(context.Context, CredentialRequest) (Credential, error)
}
type CredentialObserver interface {
    Accepted(context.Context, Credential)
}
func ChainCredentials(p ...CredentialProvider) CredentialProvider
func EnvCredential(header string, names ...string) CredentialProvider
```

* *(Amended A6: an empty `Header` means `Authorization: Bearer <Value>`;
  otherwise `Header: Value` is sent verbatim.)*
* `GitHubOptions` gains `Credentials` and `Observer`. The existing `Token`
  field keeps working and is treated as the first link of the chain.
* A source sends a credential only to the origin it was requested for, and
  only over HTTPS; loopback is exempt, as today. This rule is what prevents
  O4 by construction.

**Integrity seams (G9).**

```go
type ManifestVerification struct {
    Product   string
    Release   Release
    Selection Selection
    Manifest  []byte
    OpenAsset func(ctx context.Context, name string, limit int64) (io.ReadCloser, error)
}
type ManifestVerifier interface {
    VerifyManifest(context.Context, ManifestVerification) error
}
// Config.ManifestVerifiers []ManifestVerifier: run after the manifest
// download, before the binary download. A failure aborts the run and
// wraps ErrIntegrity. No built-in verifier ships in Phase 1; see
// "Release signing: deferred, with the hook built".

type Prober interface { // amended A2: Probe(ctx, ProbeRequest) error
    Probe(ctx context.Context, path string, rel Release) error
}
func NewVersionProber(args []string, want func(tag string) string, timeout time.Duration) Prober
func NewImageVerifier(p Platform) Verifier // debug/elf, debug/macho, debug/pe
// Config.Probes []Prober run on the staged file. Staging becomes runnable
// for this: mode 0700, plus ".exe" on Windows (a convention, not needed
// to run it: see the §3 amendments).
// InstallOptions.PostInstall Prober runs on the installed path; a failure
// rolls back.

func ParseSHA256SUMS(data []byte) (map[string]string, error)
func ExactAssetName(product string, p Platform) string
const AssetStateUploaded = "uploaded"
```

**First-class custom installers (G7).**

```go
type TwoPhaseSession interface { // amended A4: Applied is AppliedReplacement
    InstallSession
    Apply(context.Context, InstallRequest) (Applied, error)
    Commit(context.Context, Applied) (InstallResult, error)
    Rollback(context.Context, Applied) error
}
type StagingOwner interface {
    Owns(path string) bool
}
func NewManagedInstallerFor(inner Installer, life Lifecycle, rec Reconciler) (*ManagedInstaller, error)
```

* `sessOwns` asks the `StagingOwner` interface and **fails closed** when a
  session does not implement it. Today it returns `true` for any session
  type it does not recognise. The change applies only to sessions from
  custom installers, which are exactly the case where that check is being
  skipped now.
* `NewManagedInstaller` stays as it is.

**Lifecycle (G11).**

* `Request.DryRun`: resolve, select and download to staging, verify,
  transform and probe; then discard. Nothing is replaced.
* `InstallOptions.KeepPrevious`: keep `.<base>.previous` for a local
  rollback.
* `(*StandaloneInstaller).CleanupPending(ctx)`: a program calls it at
  startup to process a Windows pending backup.
* *(Amended A5: `Result.DryRun`, `Result.Previous`,
  `InstallResult.Previous`, `InstallResult.RolledBack` and
  `InstallRequest.TargetVersion` carry these outcomes.)*

### 4. Phase 2: framework-neutral interaction, and the TUI adapter

A TUI cannot sit blocked inside `Run`: it has to keep rendering. So its
event loop must pull from the run. ocp-login does this by hand in its step
runner, with a bounded channel and a self-reissuing `tea.Cmd`.
*(Re-worded to patterns by amendment P1.)*
That pattern is right; it should live in the library once, not in each
program.

**Amended 2026-10-01 by [0004-PLAN-v1-2-0-interaction-stream.md](0004-PLAN-v1-2-0-interaction-stream.md).**

* **The PLAN's shapes supersede the sketches** below where they differ, as
  for §3.
* **The amendments.** The owner approved six with the PLAN. Each is marked
  where it applies:
  * **B1.** Mid-run credentials use both candidates, one job each:
    * `WithCredentials(p)` needs the source to implement
      `CredentialedSource`, whose `WithCredentials(p)` returns a per-run
      copy with `p` in its provider slot and fresh credential state;
    * the prompt is the provider `PromptCredential()`, which finds the
      run's `Stream` through the run's context, and returns
      `ErrNoCredential` outside one.
  * **B2.** `Start(ctx, u, req, opts ...RunOption)`. There `WithReporter`
    adds a reporter after the `Stream`'s, and `WithConfirmer` replaces
    `ConfirmNeeded`. The `Updater`'s own reporter and confirmer are not
    used.
  * **B3.** `WithProgressInterval(d)` is a `RunOption`, which keeps
    amendment A1's promise that the adapter sets its own interval.
  * **B4.** `Progressed` carries every event in order. Only
    `EventProgress` is coalesced, and only over a progress event that is
    still the newest undelivered item.
  * **B5.** `ConfirmNeeded.Cancel(nil)` fails with `context.Canceled`.
    `CredentialNeeded.Cancel(nil)` returns `ErrNoCredential`. The first
    reply wins.
  * **B6.** `Next` returns `io.EOF` only after `Finished`, and
    `ctx.Err()` without consuming when its context ends. Breaking out of
    `All` does not cancel the run.

**Core (`selfupdate`, standard library).**

```go
type RunOption interface{ runOption() }
func WithReporter(Reporter) RunOption
func WithConfirmer(Confirmer) RunOption
func WithCredentials(CredentialProvider) RunOption
func (u *Updater) RunWith(ctx context.Context, req Request, opts ...RunOption) (Result, error)

type Stream struct{ /* reliable lifecycle queue + latest-wins progress slot */ }
func Start(ctx context.Context, u *Updater, req Request) *Stream // amended B2: opts ...RunOption
func (s *Stream) Next(ctx context.Context) (Interaction, error) // io.EOF after Finished
func (s *Stream) All(ctx context.Context) iter.Seq[Interaction]
func (s *Stream) Cancel()

type Interaction interface{ interaction() }
type Progressed struct{ Event Event } // amended B4: every event; only progress coalesces
type ConfirmNeeded struct{ Prompt Prompt /* reply slot */ }
func (c *ConfirmNeeded) Answer(ok bool)
func (c *ConfirmNeeded) Cancel(err error)
type CredentialNeeded struct{ Request CredentialRequest /* reply slot */ }
func (c *CredentialNeeded) Supply(Credential)
func (c *CredentialNeeded) Cancel(err error)
type Finished struct {
    Result Result
    Err    error
}
```

* **Per-run overrides.** `RunWith` overrides the reporter, confirmer or
  credentials for a single run. One `Updater` can then serve a `--json`
  CLI, a terminal prompt and a TUI without being rebuilt. `Start` is built
  on it, so a program cannot wire a `Stream` as the reporter but forget it
  as the confirmer. (The ocp-login review sketched `Stream` as a value the
  host passes to `New`; `RunWith` removes that wiring hazard.)
* **Delivery.** Lifecycle interactions are delivered in order and never
  dropped. Progress is latest-wins, so a slow UI never slows the download.
* **Mid-run credentials.** A credential prompt reaches the source through
  the provider that `RunWith` installs for the run. How that provider
  reaches a source constructed before the run is left to the phase's PLAN.
  The two candidates are a request-scoped context value, and an optional
  `ReleaseSource` interface that takes the provider per call.
  *(Amended B1: both, one job each.)*
* **Cancellation.** `Stream.Cancel` cancels the run and still delivers
  `Finished`. That fixes O6 by construction: the UI waits for the run to
  finish instead of reporting "cancelled" while the run continues.
* **Fit.** `Next` fits Bubble Tea's "wait for activity" command, a `select`
  loop, tview's `QueueUpdate`, and a plain CLI loop alike.

**Adapter (`go-tui-lib/updatetea`, Charm stays there).**

```go
type EventMsg struct{ Event selfupdate.Event }
type ConfirmMsg struct{ Req *selfupdate.ConfirmNeeded }
type CredentialMsg struct{ Req *selfupdate.CredentialNeeded }
type DoneMsg struct {
    Result selfupdate.Result
    Err    error
}
type BannerMsg struct {
    Record selfupdate.CheckRecord
    Err    error
}

func Listen(s *selfupdate.Stream) tea.Cmd // self-reissuing
func Answer(r *selfupdate.ConfirmNeeded, ok bool) tea.Cmd
func Supply(r *selfupdate.CredentialNeeded, secret []byte) tea.Cmd
func CheckInBackground(ctx context.Context, c *selfupdate.Checker, r selfupdate.CheckRequest,
    store selfupdate.CheckStore, maxAge time.Duration) tea.Cmd

type Model struct{ /* progress bar, spinner, masked input, confirm */ }
func New(s *selfupdate.Stream, opts ...Option) Model
// Options: WithLabels, WithBudget, WithLinger, WithStyles, WithKeyMap.
```

* A program can use the messages and commands alone with its own view, or
  embed `Model`, which renders inline and never takes the alternate screen.
  That matches ocp-login's rule in its `0024` record.
* When the total is unknown the Model shows only a spinner; when it is
  known it shows a bar.
* ctrl+c calls `Stream.Cancel` and then waits for `DoneMsg`.

### 5. Phase 3: the canonical command surface

**Amended 2026-10-02 by [0004-PLAN-v1-4-0-command-surface.md](0004-PLAN-v1-4-0-command-surface.md).**

* **The PLAN's shapes supersede the sketches** below where they differ, as
  for §3 and §4.
* **The amendments.** The owner approved nine with the PLAN. Each is marked
  where it applies:
  * **F1.** A release identity needs the stamped kind to be exactly
    `release`, and the version to be `vMAJOR.MINOR.PATCH` or the 0005
    prerelease form `vMAJOR.MINOR.PATCH-NAME.N`. Anything else is local,
    and `Info.Reason` says why a `release` stamp was refused.
  * **F2.** `buildinfo` does not import `selfupdate`. It has its own
    `Kind`, which `cli` maps. It exports `VersionVar`, `KindVar` and
    `LDFlags(version)`.
  * **F3.** `Flags` gains `Channel`, bound to `--channel` (0005-MADR §6).
  * **F4.** `(*Flags).Parse(args, stderr)`, and
    `Command(ctx, args, product, id, newUpdater, o) int`, which builds the
    updater only after parsing and the request succeed.
  * **F5.** `HelpText` is the flag list and the exit-status paragraph.
    `Help(prog)` adds the usage line.
  * **F6.** `Options{Stdout, Stderr, Stdin, Interactive, JSON, Confirmer,
    Timeout, Signals}`. `StdioOptions()` uses `golang.org/x/term`, which is
    already required.
  * **F7.** `Exit`'s line is `update failed: <message>`, on one line. It
    prints nothing for `ErrUpdateAvailable`.
  * **F8.** Under `--json` the last line is
    `{"kind":"result","exit_code":N,"error":"…","result":{…}}`, exactly
    once, on every path past flag parsing. `--help` writes text only.
  * **F9.** Usage errors exit 1; `-h` and `--help` exit 0.

**`buildinfo`** (top-level package, standard library).

* The library owns the stamp variables, so every program uses one ldflags
  string: `-X github.com/maccavelli/go-core-lib/buildinfo.version=…` and
  `….kind=release`.
* *(Amended F1, F2.)* `Identity()` returns an `Info` whose kind is `ReleaseBuild` only when
  the stamped kind is `release` and the version is a strict tag.
* It falls back to `debug.ReadBuildInfo` for display (module version,
  `vcs.revision`, `vcs.modified`), never to decide the release kind.

This would have prevented C2.

**`selfupdate/cli`** (standard library; no cobra).

```go
type FlagSet interface {
    BoolVar(*bool, string, bool, string)
    StringVar(*string, string, string, string)
}
// *flag.FlagSet and pflag's *FlagSet both satisfy FlagSet.
type Flags struct {
    Check, Yes, Force, DryRun, JSON bool
    Version                         string // amended F3: and Channel
}
func (f *Flags) Bind(fs FlagSet) // -y through an optional BoolVarP interface
func (f Flags) Request(product string, id buildinfo.Info) (Request, error) // rejects contradictions
func Run(ctx context.Context, u *Updater, req Request, o Options) (Result, error) // signal ctx, 15 m timeout, streams; amended F4, F6
func Exit(stderr io.Writer, res Result, err error) int                      // one canonical error line; amended F7
const HelpText = "…" // amended F5: and Help(prog)
```

The canonical protocol is:

* `<prog> update [--check] [--yes|-y] [--force] [--dry-run] [--json] [--version vX.Y.Z]`,
  with no positional arguments.
* *(Amended F9.)* Exit codes: 0 when up to date, declined or applied; 10 when `--check`
  finds a target; 1 otherwise.
* **Stdout carries machine-readable protocol output and nothing else**
  (owner decision, 2026-09-30):
  * JSON-RPC, for a program serving on stdio;
  * *(amended F8)* under `--json`, a JSONL stream: one object per line for each event,
    then a final object with `"kind":"result"` that carries the
    `ResultDocument`.

  Everything else goes to **stderr**: progress, prompts, human-readable
  results, banners, warnings, errors and logs. Without `--json`, stdout
  stays empty.

  `cli.Run` takes both streams in `Options` and never touches `os.Stdout`
  itself. The MCP servers redirect `os.Stdout` to stderr to protect
  JSON-RPC, so under `--json` they pass the original stdout explicitly.
* `--check` combined with `--yes` or `--force` is rejected.
* SIGINT and SIGTERM cancel the run.

It is documented with a cobra recipe (`SilenceErrors` and `SilenceUsage`,
set inside `RunE`), which fixes C3 and C4 wherever it is adopted.

**`selfupdate.UserAgent(product, version string) string`** returns
`product/version (goos/goarch)`, sanitised.

### 6. Security relaxations: each needs its own record

This record adopts none of these. It names the seam each one would use.
The owner's decisions of 2026-09-30 (More Information) set their order.

* **Releases that are not immutable (GitLab, Gitea, plain HTTP).**
  * The seam: a source-declared guarantee, for example
    `Release.Integrity = TagBoundByHost | TagBoundBySignature | None`.
  * An Updater opt-in, `Config.AcceptMutableReleases`, that is valid only
    when a `ManifestVerifier` is configured or the owner accepts the weaker
    guarantee in writing.
  * The `gitlab` and `httpmanifest` sources wait for this record.
  * ocp-login's migration waits for it too. The owner has **deferred**
    both the record and the migration.
* **Prerelease channels.** `NewSemverPolicy(SemverOptions{AllowPrerelease})`,
  `Request.AllowPrerelease`, and an optional `ReleaseLister`, because
  GitHub's `latest` endpoint excludes prereleases. mcplib `0005-MADR` rejected
  prereleases on purpose. The owner **wants** prerelease channels. They
  still get their own record, because they change what an unattended
  `update` may install. That record is scheduled after Phase 1, which adds
  the `Config.Versions` fix (G2) that they depend on.
* **Release signing** is deferred to the record on releases that are not
  immutable above, which needs it. The next subsection gives the reasons,
  and says which part is built now.
* **Runtime provenance** (`verify/ghattest`) still needs its own record:
  its sigstore variant is a nested module with a large dependency tree,
  and its exec variant needs `gh` at run time.

### Release signing: deferred, with the hook built

The owner asked for signing to be researched (More Information, decision
4). The research is recorded in
[0004-REPORT-release-signing-research.md](../reports/0004-REPORT-release-signing-research.md). It found a design that
meets every requirement the owner set with no new module: a C2SP signed
note through `golang.org/x/mod/sumdb/note`. It also found that the
signature would add almost nothing today.

* **Today's releases are already covered.** HTTPS, GitHub immutable
  releases, `SHA256SUMS`, per-asset digests, the strict version policy and
  `refuse-existing-release` cover tampering in transit, file substitution,
  corruption, replay and overwrite.
* **A CI-held, auto-approved key adds little.** Anyone who can push a `v*`
  tag gets a valid signature. They can also change the workflow at that tag
  and read the secret, so repository write access is effectively key
  access. Account and repository compromise is the realistic threat, and
  signing does not stop it.
* **Signing becomes necessary with a host that cannot guarantee
  immutability** (GitLab, mirrors, S3, plain HTTP). There nothing else
  binds a tag to its files. So signing becomes a requirement of the §6
  record on releases that are not immutable, and is not built before it.

**Decided (2026-09-30): defer signing. Build and test the hook now.**

* **Phase 1 builds `ManifestVerifier` into the Updater (§3).**
  * It runs after the manifest download and before the binary download.
  * Its `OpenAsset` reads sibling assets within a caller-given limit.
  * A failure aborts the run and wraps `ErrIntegrity`.
  * No built-in verifier ships. Phase 1 tests the hook with a test
    verifier.

  A signed-note verifier, or any other scheme, can then be added later
  without an API change.
* **The rest of the design waits for the §6 record.** That covers the
  `verify/signednote` package, the signer, the workflow input and key
  custody. The report holds that design and its open questions.
* **Hardening that does address the realistic threat** is repository
  settings rather than code. It is listed here so it is not lost, and each
  repository applies it under its own records:
  * a tag ruleset that restricts creating `v*` tags to the owner;
  * two-factor authentication, and fine-grained, short-lived tokens;
  * immutable releases turned on in every consumer repository.

### 7. Phase 4: shared release and install tooling

* **A build-and-stage reusable workflow**, plus one platform-matrix file
  read by Go (`go:embed`) and by CI. It builds with the canonical ldflags,
  asserts that the built binary reports `build_kind=release`, then
  generates `SHA256SUMS` and uploads. This would have caught C1 and C2.
  It changes the workflow contract, so it needs a record.
* **Installer templates** (`install.sh`, `install.ps1` and their tests),
  parametrised by product, repository and default directory. They replace
  about 1,500 near-duplicate lines across four programs. The legacy
  versioned-manifest branch is dropped.
* **`selfupdate/service`**:
  * `PollHealthy` (the loop copied verbatim in two programs) and
    `ExecReconciler` with one documented JSON receipt;
  * reference systemd, launchd and Windows SCM lifecycles. The launchd one
    carries magic-cli-remote's bootout-wait fix, which mcp-server-magictools
    lacks.
  * schtasks stays in magic-cli-remote behind `Lifecycle`.
* **`selfupdate/codesign`**, moved from magic-cli-remote's
  `internal/updateclient/codesign_darwin.go`.

### 8. Harness, across every phase

* **H1.** Regression tests for every R-finding, and branch tests for the
  dark 0003 defences (Phase 0).
* **H2.** Commit the four fuzz targets. Run them for 20 s each in a Linux
  CI step, and add the Python/Go manifest differential (N=5000) to the
  verifier test.
  *(Amended 2026-10-01 by
  [0004-PLAN-h2-fuzzing-and-manifest-differential.md](0004-PLAN-h2-fuzzing-and-manifest-differential.md):)*
  * *C1. The differential is a Go test, `TestManifestDifferential`, that
    drives the verifier's own parser. That parser moves out of the
    verifier's heredoc into `scripts/selfupdate_manifest.py`, unchanged in
    behaviour and messages, and the verifier imports it. CI requires
    `python3` for the test on Linux and macOS.*
  * *C2. `scripts/go-fuzz.sh` fuzzes every fuzz target it finds in
    `selfupdate`, for 20 s each, and fails when it finds fewer than four.
    CI runs it on Linux through `make fuzz`, and keeps a crasher as an
    artifact.*
* **H3.** `selfupdatetest` provides:
  * a fake GitHub server serving `latest`, `tags/{tag}` and `assets/{id}`,
    with each asset returned as a 302 to a second TLS origin;
  * knobs for immutable, draft, state, digest, 403/429, and oversized or
    truncated bodies;
  * `FakeSource`, `RecordingReporter` and `ScriptedConfirmer`.

  It starts under `internal/` and is exported in Phase 1, once its shape
  has been used by this package's own tests.
* **H4.** An end-to-end test on all three CI operating systems. A copied
  test binary reports its version, H3 serves a "v2" build, and
  `Updater.Run` drives the real standalone installer while the old copy is
  running. Negative cases assert the target is byte-identical afterwards.
  *(Amended 2026-10-01 by
  [0004-PLAN-h4-running-copy-end-to-end.md](0004-PLAN-h4-running-copy-end-to-end.md),
  D1: the test builds a small helper program twice, with its version
  stamped by `-ldflags -X main.version=…`. The v1 build is copied to the
  target and started, and H3 serves the v2 build. The version must be a
  property of the bytes: the test binary takes its version from the
  environment.)*
* **H5.** Golden tests for `TextReporter` and `NewJSONReporter`, with an
  `-update` flag.
* **H6.** Seams go behind `setSeam`, `time.Sleep` handshakes are replaced
  with channels, and every confirmer and `Stream` test has a goroutine-leak
  check.
* **H7.** CI adds:
  * `-race` on macOS;
  * a `-shuffle=on -count=2` run;
  * cross `go vet` for freebsd, openbsd and linux/386;
  * the fuzz seed corpus.

### What this record does not adopt

* **Delta updates:** they need a bsdiff dependency, and the binaries are
  small.
* **Resumable downloads:** low value under the 512 MiB limit.
* **Background auto-apply:** only *checks* may run unattended (§3).
* **A UI framework inside go-core-lib:** that is option D.
* **`MarshalText` on the existing enums:** §3 gives the reason.
* **Release signing now:** deferred; only the hook is built ("Release
  signing: deferred, with the hook built").

### Consequences

* Good, because R1 and R2 are fixed in a patch release before any feature
  work.
* Good, because each seam becomes first-class. A custom installer gets
  managed wrapping and the ownership check (G7). A custom source is no
  longer forced to fake GitHub fields for guarantees it provides another
  way (§6). A custom policy is honoured (G2).
* Good, because any TUI framework can drive an update through `Stream`
  with no UI code in this module. Bubble Tea programs get a ready adapter.
* Good, because one command surface, one build-stamp scheme and one JSON
  document replace three stamping schemes, four error formats and three
  stream conventions. C2, C3 and C4 become hard to reintroduce.
* Good, because the dependency floor does not move. Every Phase 0–3 item is
  standard library plus the current `x/*` modules.
* Neutral, because ocp-login can adopt Phases 1 and 2 only once the §6
  record on non-immutable releases decides how a GitLab source may be
  trusted. The owner has deferred that record. ocp-login keeps its own
  updater until then, and its defects O1–O6 remain ocp-login's to fix.
* Bad, because the exported surface roughly doubles. Every addition is a v1
  compatibility promise, so each phase's PLAN must settle names and shapes
  before its tag.
* Bad, because `sessOwns` failing closed (§3) changes behaviour for anyone
  who already wrote a custom `Installer`. No known program has, and the
  change is listed in the changelog.
* Bad, because making progress events advisory (§3) is a behaviour change
  for a reporter that returns an error on `EventProgress`. Today no such
  event exists, so no existing reporter can depend on it.

### Confirmation

* Each phase has its own PLAN under this number. It is complete only when:
  * `make pre-add-check` and `make lint` pass;
  * the three-OS CI is green;
  * each new test has been seen to fail on a deliberately broken copy.
* For Phase 0, the R1 and R2 probes from the round-2 review are committed
  as tests, and they fail on `v1.0.0`.
* For Phase 1, `apidiff` (or `gorelease`) against `v1.0.0` reports only
  compatible changes.
* For Phase 1, a test `ManifestVerifier` shows three things:
  * it runs before any binary byte is fetched;
  * it can read a sibling asset through `OpenAsset`;
  * its failure leaves the target unchanged and matches
    `ErrIntegrity`.
* For Phase 2, an example Bubble Tea program in go-tui-lib drives a real
  update against the H3 server, and ctrl+c leaves the target unchanged.
* For Phase 3, the migration guide shows one consumer (prepare-commit-msg)
  moved to `cli` and `buildinfo`, with a byte-for-byte check of its
  `--check` output and exit codes.

## Pros and Cons of the Options

### A. A phased, additive roadmap

* Good, because the defects ship first, on their own.
* Good, because every addition is additive under v1, so no consumer has to
  change import paths.
* Good, because each phase is independently useful and independently
  releasable.
* Good, because the security relaxations stay behind their own records.
* Neutral, because the GitHub-shaped fields (G1) stay in the model. They
  become one guarantee a source can declare rather than a requirement.
* Bad, because a v1 surface that grew in stages is less tidy than one
  designed at once.

### B. A `v2` redesign

* Good, because the model could be source-neutral from the start, with no
  compatibility constraints.
* Bad, because every consumer changes its import path, and none has
  migrated to `v1` yet.
* Bad, because it delays the defect fixes, or splits them across two
  major versions.
* Bad, because nothing found requires a break: every gap has an additive
  route.

### C. Fix the defects only

* Good, because it is the smallest change.
* Bad, because each TUI or service program keeps re-implementing progress,
  credentials, probes and the streaming bridge. ocp-login carries about
  4,000 non-test lines of updater, credential and step-runner code today,
  and it has O1–O6.
* Bad, because C1–C4 stay easy to reintroduce with no canonical `cli` or
  `buildinfo`.

### D. Build TUI support into go-core-lib

* Good, because everything would live in one repository.
* Bad, because a package in `selfupdate` would force Charm on every
  consumer, including the MCP servers that have no UI.
* Bad, because a nested module in this repository brings Charm into its CI,
  its vulnerability-scan scope and its dependency policy, and needs
  path-prefixed tags.
* Bad, because go-tui-lib already exists as the home for Charm-stack code.

## Amendments

### P1 (2026-10-02): ocp-login is described by pattern, not by file

*Status: accepted (2026-10-02). The owner answered: "questions: follow
recommendations, proceed." P1-Q1: O4 keeps its category only. P1-Q2:
`v1.3.1` is tagged.* Its plan is
[0004-PLAN-v1-3-1-ocp-login-pattern-rewording.md](0004-PLAN-v1-3-1-ocp-login-pattern-rewording.md).

**Found.** This repository is public. ocp-login is an org-internal program.
This record, and two of its plans, describe ocp-login at file level:
source paths, line numbers, and how one of its defects works. The same kind
of detail was removed from go-tui-lib before its first release (go-tui-lib
`docs/decisions/0001-MADR-scaffold-charm-tui-library.md`, amendment A2).

**Decided (owner, 2026-10-02).** Re-word that detail to patterns, and leave
published history as it is.

* **Rule.** ocp-login is described by what it does, never by its files. The
  re-worded text names no source path, line number, unexported identifier or
  environment variable, and does not say how a defect could be exploited.
* **What is kept:**
  * the defect IDs O1–O6, so that every cross-reference still reads;
  * one line each saying what the defect is;
  * citations of ocp-login's own records by number, as go-tui-lib does.
* **Marking.** Each re-worded passage is marked
  *(Re-worded to patterns by amendment P1.)* No decision, option or
  consequence changes, only evidence.
* **O4.** Its row says only that it is a credential-handling defect, for
  ocp-login's maintainers to fix (owner question P1-Q1). *(The draft said
  "reported to"; that was changed on 2026-10-02 before commit, because
  no report is recorded.)*
  `selfupdate`'s own rule (§3, "A source sends a credential only to the
  origin it was requested for, and only over HTTPS") already states the
  requirement in general terms.
* **History stays.** The detail remains in `v1.0.1` to `v1.3.0` and in the
  module proxy's copies of them. Rewriting history was considered and
  rejected:
  * the proxy serves each published version's original archive, so a
    rewrite cannot remove the text;
  * moved tags would no longer match the checksum database, which breaks
    any consumer that fetches the module directly.
* **Release.** The re-wording ships as `v1.3.1` (owner question P1-Q2).
  Its Go code, `go.mod` and `go.sum` are identical to `v1.3.0`'s. It also
  carries the govulncheck v1.8.0 pin of
  [0007-MADR-adopt-govulncheck-v1-8.md](0007-MADR-adopt-govulncheck-v1-8.md),
  which landed after `v1.3.0` and changes tooling only. *(Corrected
  2026-10-02, before the tag: the first text said the code was identical,
  and the PLAN's check found the tooling change.)* The newest module version,
  and the README that pkg.go.dev shows, are then clean.

**Owner questions for P1.**

* **P1-Q1. How much of O4 to keep.** Recommended: its category only, "a
  credential-handling defect, reported to ocp-login's maintainers". The
  alternative is a one-line description of the defect. That still tells a
  reader of a public repository what to look for in an internal program.
* **P1-Q2. A `v1.3.1` tag.** Recommended: yes, so that `@latest` resolves
  to clean docs. The alternative is no tag: HEAD is clean on GitHub, and
  the newest module version still carries the detail until the next
  release.

### P2 (2026-10-03): `selfupdate/archive` is scheduled in Phase 4

*Status: accepted (2026-10-03). The owner chose, for
[0010-MADR-remediate-second-debugging-pass-findings.md](0010-MADR-remediate-second-debugging-pass-findings.md)
Q8, to "follow recommendations". Its plan is
[0010-PLAN-tooling-fixes.md](0010-PLAN-tooling-fixes.md), Phase T8.*

**Found.** The second debugging pass (0010-MADR, D14) found that §1's
target-shape table lists `selfupdate/archive`, a tar.gz and zip selector
plus an extract transformer, as "covered here". But no phase schedules
it: §3–§5 do not, and §7's Phase 4 lists only the build-and-stage workflow,
the installer templates, `selfupdate/service` and `selfupdate/codesign`. No
PLAN names it.

**Decided.**

* **`selfupdate/archive` belongs to Phase 4,** beside `selfupdate/codesign`.
  An archive release is self-update input, so it fits the module's scope as
  [0009-MADR-rename-to-go-selfupdate-lib.md](0009-MADR-rename-to-go-selfupdate-lib.md)
  narrowed it. It still needs no record of its own (§1); it needs a PLAN.
* **The rest of Phase 4 stays planned, and none of it is built.** That
  covers the build-and-stage reusable workflow (which needs its own record,
  as §7 says), the installer templates, `selfupdate/service`, and
  `selfupdate/codesign`. Each needs its own PLAN before work starts.
* **No other decision changes.** §6's deferred items (`verify/signednote`,
  `verify/ghattest`, `gitlab`, `httpmanifest`) keep their own records.

### P3 (2026-10-07): Phase 4 is built; the open work in one place

*Status: accepted (2026-10-07). The owner chose the recommended answers of
[0015-MADR-remediate-third-debugging-pass-findings.md](0015-MADR-remediate-third-debugging-pass-findings.md)
(finding G1). Its plan is
[0015-PLAN-remediate-third-debugging-pass-findings.md](0015-PLAN-remediate-third-debugging-pass-findings.md),
Phase R2.*

**Found.** The third debugging pass (0015-MADR, G1) found three gaps in
this record:

* P2 still says "the rest of Phase 4 stays planned, and none of it is
  built" (`:1091`), and Phase 4 has shipped;
* §1's target-shape table (`:251-264`) misdescribes `archive`, `cli` and
  `codesign`, and omits two packages;
* the work still open is listed only inside single records.

**Decided.**

* **Phase 4 is built,** each piece under its own record:

  | §7 item | Record | Release |
  | :--- | :--- | :--- |
  | `selfupdate/service`: `PollHealthy`, `ExecReconciler`, and the systemd, launchd and SCM lifecycles | [0011-MADR-reference-service-lifecycles.md](0011-MADR-reference-service-lifecycles.md) | `v1.7.0` |
  | `selfupdate/codesign`, and `selfupdate/archive` (P2) | [0012-MADR-archive-assets-and-macos-codesign.md](0012-MADR-archive-assets-and-macos-codesign.md) | `v1.8.0` |
  | the build-and-stage workflow, its spec and `selfupdate/releasespec` | [0013-MADR-build-and-stage-release-workflow.md](0013-MADR-build-and-stage-release-workflow.md) | `v1.9.0` |
  | the installer templates | [0014-MADR-shared-installer-templates.md](0014-MADR-shared-installer-templates.md) | `v1.10.0` |

  P2's bullet "The rest of Phase 4 stays planned, and none of it is built"
  is superseded by this table. P2's text is left as written.
* **§1's table, as built.** §1 is left as written; where it differs, these
  rows hold:

  | Package | Purpose | Dependencies |
  | :--- | :--- | :--- |
  | `selfupdate/cli` | §5's command surface, exit codes, `--json`, and the service handoff hook | stdlib, `x/term`, `selfupdate`, `buildinfo` |
  | `selfupdate/archive` | the tar.gz, zip and gz selector, and an `Unpacker` that the core's extract stage runs, not a transformer | stdlib, `selfupdate` |
  | `selfupdate/codesign` | `NewSigner` (a transformer) and `NewChecker` (a prober), running `/usr/bin/codesign` through `service.Runner` | stdlib, `selfupdate`, `selfupdate/service` |
  | `selfupdate/service/...` | its own record, 0011 | stdlib, `selfupdate`, `selfupdate/service`; `x/sys/windows` for `service` and the SCM |
  | `selfupdate/releasespec` | the release spec a program embeds and the build workflow reads (0013) | stdlib, `selfupdate`, `selfupdate/archive` |
  | `internal/cmd/selfupdate-release` | the build workflow's tool, built from source, never released (0013) | stdlib, this module |

* **The open work, in one place.** Each item keeps the record that
  describes it; none is scheduled.

  | Item | Where it is described |
  | :--- | :--- |
  | Releases that are not immutable: the §6 record, and with it the `selfupdate/gitlab` and `selfupdate/httpmanifest` sources | this record, §6 (`:765-773`) and §1 (`:262-263`) |
  | `selfupdate/verify/signednote`, the signed-note release statement | this record, §1 (`:260`) and "Release signing" (`:788-830`); [0004-REPORT-release-signing-research.md](../reports/0004-REPORT-release-signing-research.md) |
  | `selfupdate/verify/ghattest`, the runtime attestation check: its exec verifier | [0017-MADR](0017-MADR-verify-build-provenance-and-close-0015-open-items.md) track 3, `v1.12.0`; this record, §6 (`:784-786`) |
  | `selfupdate/verify/ghattest`'s `sigstore-go` variant | 0017-MADR, "Not decided here": its own repository and records |
  | `go-tui-lib/updatetea`, the Bubble Tea adapter | this record, §4 (`:636`) and §1 (`:264`); a record in go-tui-lib |
  | Replacing the binary before stopping the service | 0011-MADR, Related (`:909-912`) |
  | A D-Bus systemd backend | 0011-MADR, Consequences (`:536-537`) and option B (`:579`) |
  | xz, zstd and bzip2 archives; several programs from one archive; app bundles; notarization, `spctl` and quarantine; pure-Go signature checks | 0012-MADR §8 (`:670-697`) |
  | macOS signing or notarization in CI; an attestation from the build workflow; GoReleaser names in the fleet; files beside the program in an archive; hosts other than GitHub | 0013-MADR §10 (`:664-680`) |
  | Package managers; signing the installers; system-wide installs; completion, MCP registration and service setup in the templates | 0014-MADR §7 (`:454-466`) |
  | Moving each program onto the build workflow and the installers | 0013-MADR §10, 0014-MADR §7: each repository's own records |
  | Unreferenced local entries in a zip; names Windows reserves (`:` among them) in archive entries | 0017-MADR track 2, `v1.11.1`; 0015-PLAN, Out of scope (`:57-68`) |
  | A crash between `Apply` and `Commit` | 0017-MADR track 3, `v1.12.0`; 0015-PLAN, Out of scope (`:57-68`) |
  | `queue: max` on the publish workflow's concurrency | 0017-MADR item 5: waits for an actionlint release that accepts `queue`, after `v1.7.12`; 0015-PLAN, Out of scope (`:57-68`) |

  Two items that 0012 §8 and 0013 §10 list are done:
  * publishing archives through the publish workflow, done in `v1.9.0`
    (0013);
  * the installer templates, done in `v1.10.0` (0014).

  §6's prerelease channels were built under
  [0005-MADR-opt-in-prerelease-channels.md](0005-MADR-opt-in-prerelease-channels.md).

## More Information

### Owner decisions (2026-09-30)

The first draft of this record asked the owner five questions. The
answers:

1. **Stream convention.** Asked: human output on stderr and `--json` on
   stdout? prepare-commit-msg and magic-cli-remote send the reporter to
   stdout today.
   **Decided:** "Stdout pristine clean json-rpc, jsonl, etc. Use stderr
   for everything else." §5 states the rule. Both programs move their
   reporter to stderr when they migrate.
2. **ocp-login migration.** Asked: write the §6 record on non-immutable
   releases now, or leave ocp-login separate?
   **Decided:** deferred. ocp-login keeps its own updater. The `gitlab`
   source and that §6 record are not scheduled.
3. **Prerelease channels.** Asked: are they wanted?
   **Decided:** wanted. They get their own record after Phase 1 (§6).
4. **Release signing.** Asked: where would a signing key live?
   **Decided:** research it first. The solution must be idempotent, must
   work on Windows, Linux and macOS, and should be idiomatic Go, informed
   by what similar Go projects do.
   **Then decided, on reviewing the research:** defer signing. Build
   and test the `ManifestVerifier` hook in Phase 1. Record the research
   as a report ([0004-REPORT-release-signing-research.md](../reports/0004-REPORT-release-signing-research.md)).
   The report also records the owner's choice of auto-approval for a
   future signing environment.
5. **Consumer defects C1–C4.** Asked: fix them now, ahead of migration?
   **Decided:** each consumer is fixed and migrated separately, in its
   own repository and under that repository's own records. This record
   does not schedule that work.

### Evidence and related records

* [0002-MADR-rehome-selfupdate-from-mcplib.md](0002-MADR-rehome-selfupdate-from-mcplib.md):
  the re-home, and why `bridge-release` is gone (C5).
* [0003-MADR-remediate-debugging-pass-findings.md](0003-MADR-remediate-debugging-pass-findings.md):
  the fixes R1–R5 build on (B1, C7, B10, B2 and the receipt handling).
* mcplib `docs/0005-MADR-canonicalize-cli-self-update-in-mcplib.md`: the
  original scope. It put background polling, prereleases and runtime
  attestation out of scope; §3 and §6 revisit them.
* ocp-login's own records, cited by repository: `0001` (smoke test and
  inode reasoning), `0024` (inline rendering), `0029` (saving the token only
  once it is accepted), `0030` (one repository for every build target,
  which pins the asset names) and `0035` (the macOS release, and why it
  relies on the linker's ad-hoc signature).
* [0004-REPORT-release-signing-research.md](../reports/0004-REPORT-release-signing-research.md): the signing
  research, its sources, the design kept for later, and the backup-key
  options.
* The round-2 probe tests (`probe_r2_test.go`, `probe_net_test.go`,
  `fuzz_r2_test.go`) and the differential harness were kept in the review's
  scratch copy. They are not committed; Phase 0 commits them as tests.
