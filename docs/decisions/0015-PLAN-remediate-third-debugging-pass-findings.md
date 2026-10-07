---
status: proposed
date: 2026-10-07
associated-madr: "0015-MADR-remediate-third-debugging-pass-findings.md"
---
# Implement the third debugging pass's remediation: records and the gate on main, v1.10.1 of contract-preserving fixes, and v1.11.0 for the owner's contracts

Associated MADR: [0015-MADR-remediate-third-debugging-pass-findings.md](0015-MADR-remediate-third-debugging-pass-findings.md)

## Goal

* Every one of the MADR's 63 findings is fixed, decided, or recorded as
  not fixable here with the reason, each with the test that would have
  caught it, seen to fail on the unfixed code and to pass after the fix.
* **Track 1, on `main` with no release:** the gate, the document checker
  and the plant helper live in the repository; the roadmap and record
  amendments; the documentation fixes.
* **Track 2, `v1.10.1`:** the two High findings and every fix that keeps
  the documented contract. `make apicheck` reports it compatible with
  `v1.10.0`, and it adds no exported identifier.
* **Track 3, `v1.11.0`:** the contracts the owner decides, as this PLAN's
  "Decisions this PLAN assumes" table states them.
* CI is green on `main`, `v1.10.1` and `v1.11.0`, and the live service
  tests pass on the test hosts before each tag.

## Scope

### In scope

| Phase | Track | Findings | Main paths |
| :--- | :--- | :--- | :--- |
| R0 | 1 | records | 0015 MADR (accepted, answers, corrections), this PLAN, `docs/README.md` |
| R1 | 1 | G2 | `scripts/gate.sh`, `scripts/check-docs.sh`, `scripts/plant-copy.sh` and their tests, `Makefile`, `ci.yml`, `AGENTS.md`, `docs/architecture.md`, notes in the 0010–0014 PLANs |
| R2 | 1 | G1, G3 (record), G10 (record) | 0004 MADR amendment P3, 0011 MADR A6 and PLAN D8, 0010 MADR note, 0013 PLAN note |
| R3 | 1 | G8, G9, F3 (docs), F4 (docs), C5 | the guides, `README.md`, `docs/architecture.md`, `docs/README.md`, godoc in `types.go` and `doc.go`, one test |
| P1 | 2 | B7, B4, B1, B3 (High) | `session.go`, `managed.go`, `leftovers.go`, `replace_*.go`, `updater.go`, `service/launchd`, `service/systemd`, `service/scm` |
| P2 | 2 | A1 (selectors), A2, A3, A4, A6, A7 | `github.go`, `assets.go`, `archive/select.go`, tests |
| P3 | 2 | B2, B6, B8, B9, B10 | `target.go`, `replace_*.go`, `probe.go`, `leftovers.go`, `imageverify.go`, tests |
| P4 | 2 | C1, C2, C4, C6, C7, A9, G5, G6 | `cli/run.go`, `cli/command.go`, `checker.go`, `updater.go`, reporters, confirmers, godoc, goldens |
| P5 | 2 | D2, D3, D4, D5, D7, D8, D11 | `service/systemd`, `service/launchd`, `service/scm`, live tests |
| P6 | 2 | E1, E2, E3, E4, E5, E6 | `archive/unpack.go`, `releasespec/validate.go`, `check.go`, `codesign/codesign.go`, tests, fuzz seeds |
| P7 | 2 | F1, F2, F4, F5, F6, F7, F8, F9, G7 | `installer/install.sh`, `installer/install.ps1`, `publish-selfupdate-release.yml`, `scripts/release-latest-flag.sh`, `main.go`, tests |
| P8 | 2 | release | migration guide, release notes, the tag (owner), the pin commit, the checks |
| Q1 | 3 | C3, C8, C9, G3, A1 (404) | `cli/run.go`, `service/handoff.go`, `updater.go`, `checker.go`, `stream.go`, `types.go`, `document.go`, `errors.go`, `checkcache.go`, goldens, live tests |
| Q2 | 3 | A5, B5 | `checkcache.go`, `session.go`, `replace.go` |
| Q3 | 3 | D6, D10, G4 | `service/scm`, `service/poll.go`, the backends, `service/execreconciler.go`, `types.go`, `managed.go`, `updater.go` |
| Q4 | 3 | E7, E8, F3 | `releasespec/spec.go`, `archive/unpack.go`, `plan.go`, `build-selfupdate-release.yml` |
| Q5 | 3 | release | migration guide §11, release notes, the tag (owner), the pin commit, the checks |

The MADR's aliases are planned once, under the first ID: D1 is B3, D9 is
C3, and A8 is merged into G9.

### Out of scope

* **Not fixable here, and recorded as open in R2's 0004 amendment:**
  * a zip local entry that no central record names, in a gap between
    entries, which only a streaming reader sees (planning residual of E1;
    closing it means parsing the end of central directory, which
    `archive/zip` hides);
  * a crash between `Apply` and `Commit` (B1's remainder): the backup is
    then a crash leftover with the verified new binary live, and the sweep
    keeps removing it, as 0010 Q6 decided;
  * refusing `:` in archive entry names on every host (planning note on
    E2);
  * `queue: max` on the publish workflow's concurrency, which needs an
    actionlint newer than v1.7.12 (F9).
* **`simulate_build.sh`** (0013-PLAN) is not committed: CI's
  `release-rehearsal` jobs, which call the build workflow by its local
  path, are its reproducible equivalent. R1 notes this in 0013-PLAN.
* **Moving any program onto these releases**, and anything 0004's open
  list names beyond this MADR.
* **Push and tags,** which are the owner's.

## Decisions this PLAN assumes

The MADR's 15 owner questions are unanswered. This PLAN takes each
recommended answer, and nine more decisions that planning surfaced (N1–N9).
**Approving this PLAN accepts every row below.** An answer the owner
changes before approval rewrites only the phase named in its row.

| # | Finding | Decision | Phase |
| :--- | :--- | :--- | :--- |
| Q1 | C3 | `service.HandOffFunc` refuses, inside the service and without `Yes`, with `ErrConfirmationRequired` before detaching; `cli.Run` hands off only when a check finds an operation (or `--force`) | Q1 |
| Q2 | C5 | Document that an up-to-date or check-only run's last event is `selected`; no new event | R3 |
| Q3 | C8 | New sentinel `ErrNotConstructed`; zero or nil `Updater`, `Checker` and `Start` return it; a zero `Stream.Cancel` is a no-op | Q1 |
| Q4 | C9 | A dry run for another platform skips staged probes and reports `Result.ProbesSkipped` (schema 3) | Q1 |
| Q5 | A5 | `X-RateLimit-Reset` defers only when `Remaining` is 0 | Q2 |
| Q6 | B5 | Clear setuid and setgid on `.previous` and on a kept backup | Q2 |
| Q7 | B7 | Refuse a second `Commit` or `Rollback` of one replacement (moved to `v1.10.1`: see Corrections) | P1 |
| Q8 | D6 | Record the dependents `Stop` stopped; `Start` and recovery start them again | Q3 |
| Q9 | D10 | New `service.PollOptions.Validate`; the backend constructors refuse a settle window at or above the timeout; the stop wait says "not ready within" | Q3 |
| Q10 | E7 | `Parse` refuses `null` for every field | Q4 |
| Q11 | E8 | Skip a PAX global header; refuse one carrying `path`, `linkpath`, `size` or any `GNU.sparse.*` record (with N3) | Q4 |
| Q12 | F3 | `plan` refuses a module whose go-selfupdate-lib requirement is below the spec's field floor (`installer` needs `v1.10.0`); a directory `replace` is not checked (corrected: see Corrections) | Q4 |
| Q13 | F4 | The timeout error names the repository setting and the recovery; the docs name both prerequisites; no pre-check | P7, R3 |
| Q14 | G2 | Commit the gate, the document checker and the plant helper under `scripts/`, with tests; the identifier check reads the pre-push guard's deny list; CI runs the link check only | R1 |
| Q15 | G3 | `Result.RolledBack` and `ResultDocument.RolledBack` (schema 3), and the live handoff test 0011 promised | Q1, R2 |
| N1 | E2 | Archive entry names must be printable ASCII, and no path element may end in a dot or a space: this is the only exact fold without a new module | P6 |
| N2 | E5 | Besides the 128-character asset name, `Validate` refuses a tar.gz platform whose program name exceeds USTAR's 100 characters, which `pack` cannot encode | P6 |
| N3 | E8 | The global-header refusal list includes `size` and `GNU.sparse.*` | Q4 |
| N4 | E1 | Unreferenced local entries are recorded open, not fixed | R2 |
| N5 | B1 | A crash between `Apply` and `Commit` is recorded, not changed | R2 |
| N6 | A1 | New sentinel `ErrNoRelease` for a 404 from `releases/latest` or a missing tag, and a cached outcome `CheckNoRelease` | Q1 |
| N7 | G4 | New field `ReconcileResult.Warnings` (type `Warnings`); `ExecReconciler` fills it, plus one warning when `Changed` and not `Reloaded`; they reach `Result.Warnings` after `complete` | Q3 |
| N8 | B3 | Each backend finishes its stop wait once the stop is issued, up to the service manager's own kill bound: launchd `ExitTimeOut` (5 s when absent, the value 0011's probe evidence records) plus 30 s; systemd `max(Poll.Timeout, TimeoutStopUSec + 30 s)`; SCM its existing deadline | P1 |
| N9 | F9 | Job-level `concurrency`, group `go-selfupdate-lib-publish-${{ github.repository }}`, `cancel-in-progress: false`, the default queue | P7 |

## Corrections to the MADR found while planning

R0 records these in the MADR as amendment A1 before any code changes.

* **B7 moves to `v1.10.1`.** B1's rename of a kept backup needs to know
  the replacement's state in `Rollback` and managed recovery, which B7's
  state provides. B7 adds no exported identifier, and the refusal fixes a
  false `Applied=true` after a rollback.
* **G4 moves to `v1.11.0`:** reporting a reconciler's warnings needs the
  new exported field N7.
* **A1 splits:** the selectors wrap `ErrUnsupportedPlatform` in `v1.10.1`;
  caching a release that does not exist yet needs N6 in `v1.11.0`.
* **C9 is reproduced** (the MADR's "—"): a probe ran and failed with
  `exec format error`.
* **F3's rule** is the spec's field floor, not the tool's version: the
  tool is built from a tagless checkout, so its own version reads as a
  pseudo-version or `(devel)`; and CI's rehearsal fixture `replace`s the
  library by a directory, which must not be checked.
* **F7(a) also holds in `install.ps1`:** `Test-ReleaseTag` (`:129`) ends
  its pattern with `$`, which .NET matches before a final newline.
* **G7** is `main.go:11`, not `:13`.
* **F4:** the README's "the tag can never be reused" is false when
  immutable releases are off.
* **E5:** `pack` writes tar.gz as USTAR (`pack.go:41-43`), so a program
  name over 100 characters cannot be packed at all (N2).
* **D2's external fact** is pinned by the fix's rule, not relied on: the
  baseline is read after the start, whatever systemd does with the counter.

## Rules for every phase

1. **Order.** R0, R1, R2, R3, then P1–P8, then Q1–Q5. Track 1 needs no
   release and may run beside track 2 once R0 and R1 are in. Q1–Q4 start
   only after `v1.10.1` is tagged.
2. **Commits.** One commit per phase, staged by the agent, committed by
   the owner with `git commit --no-edit`. Push and tags are the owner's.
3. **Test first.** Each new test is written first and seen to fail on the
   unfixed code, with the failure text recorded. A test that pins a
   documented behaviour that already holds (C5, A7, A9, B10) is marked so,
   and its plant proves it instead.
4. **Plant after.** After the fix, the phase's listed plant, applied in a
   scratch copy made with `scripts/plant-copy.sh` (before R1 lands, the
   session's copy helper), must make the new test fail, with the failure
   text recorded. Never in the tree.
5. **Checks before each commit:** `make gate` (every step rc 0; before R1
   lands, the session's gate); `make pre-add-check FILES=…` on every
   staged `.go` file; `scripts/check-docs.sh --links` on every changed
   Markdown file and `--ids` on every changed file; shellcheck 0.11.0 on
   changed scripts and templates; actionlint v1.7.12 and
   `scripts/check-workflows.sh` (with `expressions`, `permissions` and
   `pins` on `ci.yml`) on changed workflows; `scripts/check-installers.sh`
   on changed templates.
6. **API.** Every track-2 phase: `make apicheck` reports compatible with
   `v1.10.0`, and `go doc -all` of each package lists no exported
   identifier `v1.10.0` lacks. Track 3: compatible with `v1.10.1`;
   additions only.
7. **No new module.** `go.mod` and `go.sum` stay unchanged; `go mod tidy
   -diff` is clean; depguard's 14 rules are unchanged.
8. **Goldens** are regenerated only by `go test ./selfupdate/cli -run
   'TestGolden|TestCommandGolden' -update`, and the diff is read line by
   line in the phase record.
9. **Live tests** named in a phase run on the owner's test hosts (the
   Linux host, WSL on the Windows host, the Windows host) and on macOS CI
   before the tag of their track; their output goes into P8 or Q5.
10. **Deviations:** stop and prompt with resolutions; record the chosen
    one as a dated deviation here, and in the MADR when a decision or a
    fact changes.
11. **Identifiers:** nothing committed carries a hostname, account name or
    real path; `scripts/check-docs.sh --ids` before each commit.

## Implementation Steps

Line numbers are at `d6a570f` (`v1.10.0` plus records), where every Go,
template and workflow line equals `18e0575`'s.

### Phase R0: records

1. **The MADR** (`0015-MADR-remediate-third-debugging-pass-findings.md`):
   * `status: accepted`, the date of approval;
   * "Proposed:" becomes "Chosen option:" in Decision Outcome;
   * an **Answered** paragraph opening "Owner questions", quoting the
     owner's approval of this PLAN, and naming this table as the answers;
   * `### 4. Owner answers` after §3, restating the `v1.11.0` scope;
   * `## Amendments` before "More Information", with **A1 (date):
     corrections found while planning**, listing this PLAN's Corrections;
   * the last bullet of "More Information" names this PLAN by filename.
2. **This PLAN:** `status: in-progress`.
3. **`docs/README.md`:** the MADR row reads `accepted`; this PLAN's row is
   added after it.
4. **Commit** the records alone (the bootstrap exception).

### Phase R1: the gate in the repository (G2)

1. **`scripts/gate.sh`** (bash), from the session's gate:
   * `ROOT=$(cd -- "$(dirname "$0")/.." && pwd)`; no `$HOME` path;
   * output in `GATE_OUT`, or a `mktemp -d` printed at the start;
   * the steps: gofmt, `make lint`, `go vet ./...`, `go test -race
     -count=1 ./...`, `go test -shuffle=on -count=2 ./...`, `go mod tidy
     -diff`, `make apicheck`, `make fuzz` (`FUZZTIME` passed through),
     `make vuln`, every `scripts/*_test.sh` except `gate_test.sh`,
     shellcheck on `scripts/*.sh`, cross `go vet` for `freebsd/amd64`,
     `openbsd/amd64`, `linux/386` and `windows/amd64`;
   * each step's rc captured before any filter, a line `name rc=N <last
     line>` per step, and `overall=0|1`; exit 1 when any step failed;
   * the three `sh -c '…'` steps become functions (shellcheck SC2016);
   * a missing tool fails and names it.
2. **`scripts/check-docs.sh`** (embedded Python, as `check-workflows.sh`):
   * `--links FILE…`: relative links and `#anchors` with GitHub's slug,
     code spans skipped, `http(s):` skipped;
   * `--ids FILE…`: the deny list from `DISCLOSURE_DENY`, else
     `~/.config/git/disclosure-deny` (the pre-push guard's); a macOS or
     Linux home directory (the capitalised macOS root, or the Linux
     `home` root, case-sensitive) followed by a path segment other than
     `<user>`; never prints a match, only the file and line; without a
     deny list it fails ("no deny list") unless `--ids-optional`;
   * the 0009-only `--markers` mode is dropped.
3. **`scripts/plant-copy.sh`** (embedded Python): `FILE OLD NEW` (Python
   literals); the root from `git rev-parse --show-toplevel`; copies `git
   ls-files -co --exclude-standard` into a `mktemp -d`, without `.git`;
   requires OLD exactly once; UTF-8; prints the copy's path and a
   removal hint. Its header says the copy has no `.git`, so `make
   apicheck` cannot run there.
4. **Tests,** each `#!/bin/sh`, `set -eu`, `SCRIPT=${SCRIPT:-…}`, a
   `mktemp -d` work area and stubs on `PATH`, as `go-precheck_test.sh`:
   * `scripts/gate_test.sh`: stub `go`, `make`, `gofmt`, `shellcheck`;
     cases: all green → `overall=0`; `make lint` failing → `lint rc=1`,
     exit 1; gofmt listing a file → `gofmt rc=1`; a failing script test
     named; the stubs see the copy's root as their directory. Plants:
     `[ "$rc" -eq 0 ] || overall=1` → `true` (the failing-step case fails);
     a `cd "$HOME/…"` (the root case fails).
   * `scripts/check-docs_test.sh`: fixtures for a good link, a broken
     file, a missing anchor, a link in a code span, an `https:` link; a
     `DISCLOSURE_DENY` file naming `exampleuser`; a home path under
     `<user>` passes, a lowercase `users` segment in an `https:` URL
     passes, a home path under `bob` fails; the output never contains
     `exampleuser`. The test composes its home-path fixtures at run time
     from parts, so the test file itself passes `--ids`. Plants: drop
     `.lower()` from the slug; make the home-path search
     case-insensitive.
   * `scripts/plant-copy_test.sh`: a throwaway git repository with a
     tracked, an untracked and an ignored file; the copy has the first two
     only, the plant lands once, a non-unique OLD exits non-zero, the
     source is byte-identical after. Plant: drop the uniqueness check.
5. **`Makefile`:** `gate` in `.PHONY`, target `gate: ## Runs the full
   pre-commit gate (scripts/gate.sh)`.
6. **`ci.yml`:** a Linux step "Verify the gate and document tools":
   the three tests, then `./scripts/check-docs.sh --links $(git ls-files
   '*.md')`.
7. **Docs:** `AGENTS.md` "Pre-add checks" gains `make gate`;
   `docs/architecture.md` lists the three scripts in the tree and `gate`
   among the Make targets; `docs/README.md` gains "run the full gate
   before a commit".
8. **Notes in the earlier PLANs** (execution records unchanged): one dated
   line in the Rules of 0010-PLAN-v1-5-1, 0010-PLAN-v1-6-0, 0011-PLAN,
   0012-PLAN, 0013-PLAN and 0014-PLAN: "The session tools these records
   cite now live as `scripts/gate.sh`, `scripts/check-docs.sh` and
   `scripts/plant-copy.sh` (0015-PLAN R1)." 0013-PLAN's note adds that
   `simulate_build.sh` is replaced by CI's `release-rehearsal` jobs.
9. **Acceptance:** `make gate` exits 0 on a clean checkout; each test's
   plants fail it; `--links` over every tracked Markdown file reports 0
   failures; CI runs the new step.

### Phase R2: roadmap and record amendments (G1, G3, G10, N4, N5)

1. **0004-MADR, `### P3 (date): Phase 4 is built; the open work in one
   place`** under `## Amendments`, as P1 and P2 are written (`*Status*`,
   **Found.**, **Decided.**):
   * Phase 4 against what shipped: the build workflow and the platform
     file (0013, `v1.9.0`); the installer templates (0014, `v1.10.0`);
     `selfupdate/service` (0011, `v1.7.0`); `selfupdate/codesign` and
     `selfupdate/archive` (0012, `v1.8.0`);
   * P2's "none of it is built" (`:1091-1094`) superseded;
   * the §1 table (`:251-264`) corrected: `cli` imports `x/term`,
     `selfupdate` and `buildinfo`; `archive` is an `Unpacker` in the core
     extract stage handling tar.gz, zip and gz; `codesign` is
     `NewSigner` and `NewChecker` over `/usr/bin/codesign` through
     `service.Runner`; `service` has its own record (0011); rows added for
     `releasespec` and the internal release tool;
   * one table of open work, each with its record and line:
     `verify/signednote`, `verify/ghattest`, `gitlab`, `httpmanifest`
     and the §6 record on mutable releases; `go-tui-lib/updatetea`;
     0011's "replace before stop" and a D-Bus systemd backend; 0012 §8's
     other formats, several programs per archive, app bundles,
     notarization and pure-Go signature checks; 0013 §10's macOS signing
     in CI, a build-side attestation, GoReleaser names, files beside the
     program, other hosts, a combined workflow; 0014 §7's package
     managers, installer signing, system-wide installs, completion and
     service setup; moving each program onto the workflow and the
     installers; this PLAN's N4, N5, `:` in entry names and `queue: max`.
     0012:677 and 0013:666 are marked done.
2. **`docs/README.md:84`:** "see where `selfupdate` is going" links to
   P3's anchor and reads "…, and what is still open".
3. **0011-MADR `### A6 (date): the rollback is reported in the result`**
   (G3): the Confirmation item restated for `Result.RolledBack` (Q1), and
   the live handoff test Q1 adds. **0011-PLAN deviation D8 (date):** V4
   dropped the item; Q1 of 0015-PLAN restores it.
4. **0010-MADR** (G10): after `:187`, an annotation: every `Example` has
   an output check, except `ExampleHandOff`, `ExampleNewSigner` and
   `ExampleNewChecker`, which exit the process or depend on the OS.
5. **Acceptance:** `check-docs.sh --links` clean; the anchors resolve.

### Phase R3: documentation (G8, G9, F3, F4, C5)

1. **G8:** migration guide `:530-531`: "Since `v1.9.0` this repository's
   workflows build and publish archives too, from a spec with
   `"packaging": "archive"`", linking the building guide's §7 (anchor
   `#7-archives`).
   Extending guide `:116-117`: "seven whole runs". `docs/README.md:68`:
   "copy its file into `<package>/testdata/fuzz/<Name>/` of the package
   that owns the target (`selfupdate`, `selfupdate/archive` or
   `selfupdate/releasespec`)".
2. **G9, `docs/architecture.md`:**
   * the scripts tree gains `release-latest-flag.sh` ("the latest-flag
     rule: a stable tag below the current latest is published
     `--latest=false`");
   * `:155-157`: "the cache file is schema 3, which keys the channel and
     holds the cached outcome; an older record reads as a miss";
   * build step 4 gains `GOWORK=off`;
   * publish step 7 gains the backport rule;
   * the CI list gains, on Linux, the "ownership as root" step, the
     systemd live tests, `release-latest-flag_test.sh` and
     `go-precheck_test.sh` ("the gate's own test" at `:415` becomes "the
     API gate's test"); on macOS the launchd and codesign live tests; on
     Windows the SCM live test.
3. **F3, docs:** building guide step 12, after its first paragraph:
   "Your program's module must require `v1.10.0` or later, the version
   the workflow is pinned to: an older `releasespec.Parse` refuses the
   spec's `installer` field, and the shipped program cannot self-update.
   Move `go.mod` and both pins together." Migration guide §10's "Move
   both" bullet gains: "Until `v1.11.0`, the build workflow does not check
   this."
4. **F4, docs:** building guide step 4 gains "**Before the first
   release**": turn on immutable releases (Settings → General → Releases
   → "Enable release immutability", or the organization's policy; it
   applies only to releases made after), and attestations need a public
   repository or GitHub Enterprise Cloud. `README.md:94-98` says the same;
   "the tag can never be reused" is qualified by "with immutable releases
   on"; "When a publish fails" gains the mutable-release case: delete the
   release, turn the setting on, re-run.
5. **C5:** `selfupdate/doc.go:75-76`, `types.go:588` (`EventSelected`)
   and `:600-602`, extending guide `:100`, `docs/architecture.md:229-230`:
   "A run that installs nothing because it is up to date, and a check, end
   at `selected`: the result's `Operation` is the outcome, and no
   `complete` follows." Test `terminal_event_test.go`,
   `TestRunEndsAtSelected` (`newContractEnv`, `recReporter`): a check
   that finds an update, an up-to-date check and an up-to-date apply each
   end at `EventSelected`, with no `complete`, `failed` or `declined`. It
   passes before the change (it pins documented behaviour). Plant: emit
   `EventComplete` for `OperationNone` before the return at
   `updater.go:187-196`.
6. **Acceptance:** markdownlint, `check-docs.sh`, `make pre-add-check` on
   the two Go files.

### Phase P1: the High findings and their session prerequisites (B7, B4, B1, B3)

In this order, one commit.

1. **B7, a second `Commit` or `Rollback` refused** (`session.go:279-325`).
   * `AppliedReplacement.State` holds `*replacement{sess *installSession;
     applied applyResult; finished bool}`; `appliedState` refuses a wrong
     type, nil or another session's (`errForeignReplacement`).
   * New unexported `errReplacementFinished` ("selfupdate: the
     replacement was already committed or rolled back"), checked under
     `s.mu` after the closed check. `Commit` marks the state finished once
     `checkDir` passes; `Rollback` on success or `errRestoredUnsynced`;
     `Apply` when it returns an error with no live backup.
   * Test `twophase_test.go`, `TestSecondCommitOrRollbackRefused`
     (`standaloneSession`, `stageNew`): rollback then commit, commit then
     rollback, commit twice, rollback twice; each an error, `!Applied`,
     target unchanged. Fails on HEAD: `commit after rollback:
     target="old-bytes" Applied=true`. Plant: delete the `finished` check
     in `Commit`.
   * Docs: `types.go:496-511`.
2. **B4, recovery reports a restored binary** (`managed.go:170-186`,
   `session.go:127-147,195-199,329-337`, `replace_unix.go:80`,
   `replace_windows.go:64`).
   * `rollbackInRoot` returns `errors.Join(errRestoredUnsynced, err)` when
     only the sync failed; `Install` returns `RolledBack: true` and no
     backup on `errRestoredUnsynced` and on `errProbeRolledBack`; `Apply`
     joins `errProbeRolledBack` to its undo and to an unsynced restore;
     both `replaceTarget`s join it at the restore-success return; `recover`
     reports `RolledBack` on `errRestoredUnsynced`.
   * New seam `syncRootFn = syncRoot`.
   * Tests: `managed_regression_test.go`,
     `TestManagedRecoveryReportsRestoredBinary` (`managedEnv`,
     `swapAfterFirstReplace`; `setSeam(&syncDirFn, …)` failing the second
     call with `fakeLife{healthErr}`); `install_hardening_test.go`,
     `TestInstallReportsRestoreAfterSyncFailure`. Each asserts
     `RolledBack`, no `Backup`, `"old-bytes"`. Fails on HEAD:
     `RolledBack=false Backup=""`. Plants: drop the `errRestoredUnsynced`
     branch in `recover`; join `derr` alone in `Apply`.
3. **B1, a kept backup survives** (`leftovers.go:22-79`,
   `session.go:127-147,195-205,321-324,400-406`, `managed.go:172-177`,
   `updater.go:219,374-391`).
   * New `keptName(base, name)` maps `.<base>.selfupdate-bak-<digits>` to
     `.<base>.selfupdate-kept-<digits>`; `isLeftover` matches neither
     `kept`.
   * New `(*installSession).retainLocked(backup) string`: refuses to
     overwrite an existing kept file; renames through `s.root.Rename`,
     then `advisory(syncRoot(s.root))`; returns the new path, or the old
     one if the rename failed.
   * Every path that returns a live backup with `Applied` false calls it:
     `Install` at `:130`, `:139`, `:147`; `Apply` at `:197` and `:205`
     (the new name in `Backup` and in the state); `Rollback` on failure;
     `recover` after a failed `Rollback`, through an unexported
     `backupOf(AppliedReplacement) string` and an `Lstat`.
   * A dry run sweeps nothing: unexported context key `dryRunKey{}` with
     `withDryRun` and `isDryRun`; `updater.go:219` passes `withDryRun(ctx)`
     to `Begin` for `req.DryRun`; `beginSession` then skips
     `processCleanupReceipt` and `removeLeftovers`.
   * `Result.PendingBackup` and the "kept at" message already use
     `installed.Backup`, so they name the new file.
   * Tests: `recovery_test.go`, `TestKeptBackupSurvivesLaterSessions`,
     subtests `CleanupPending`, `Run`, `DryRun`, `Managed`
     (`withTempHome`, `setSeam(&syncDirFn, …)`, a failing restore,
     `managedEnv` with `fakeLife{healthErr}`): the reported backup still
     holds `"old-bytes"`. Fails on HEAD: the backup `no such file or
     directory`. `leftovers_test.go`, `TestDryRunSweepsNothing`
     (`plantLeftovers`, `checkLeftovers` expecting all kept).
     `modepolicy_test.go` `TestIsLeftover`: `.demo.selfupdate-kept-42` is
     not a leftover. Plants: `retainLocked` returns its argument (the
     `CleanupPending` and `Run` subtests fail); drop the dry-run key (only
     `TestDryRunSweepsNothing` fails).
   * Docs: `types.go:156-163` and `:451-452` name
     `.<base>.selfupdate-kept-<n>` and say no later session removes it;
     `doc.go:15-17` says a dry run sweeps nothing; `leftovers.go:8-21`;
     `docs/architecture.md:243-249`.
4. **B3, a failed stop restarts the service** (`managed.go:104-108`).
   * New `(*managedSession).recoverStop(ctx, product, stopErr)`: asks
     `Running` again under `context.WithoutCancel(parent)` and
     `recoveryTimeout`; when not running, calls `recover(…, restart=true,
     stopFirst=false, fmt.Errorf("selfupdate: stop service: %w", …))`,
     which runs `Start` and `WaitHealthy` and joins every error; when still
     running, or the probe fails, returns as today with the probe error
     joined. `service.ErrInsideService` keeps today's path.
   * Backends (N8): each `Stop`, once the stop is issued, finishes its
     wait under `context.WithoutCancel(ctx)`, bounded by its kill bound,
     and returns the caller's `ctx.Err()` joined:
     * launchd: before `bootout`, `plutil -extract ExitTimeOut raw -o -
       -- <plist>`; the wait is `ExitTimeOut + stopGrace` (`stopGrace = 30
       s`, `defaultExitTimeOut = 5 s` when the key is absent, both package
       variables); 0 or a non-integer falls back to `Poll.Timeout`
       (`DefaultPollTimeout` when unset);
     * systemd: `TimeoutStopUSec` read in the same `show` as `Start`'s
       baseline; the wait is `max(Poll.Timeout, TimeoutStopUSec + 30 s)`;
     * SCM: its existing deadline, under `WithoutCancel`.
   * Tests: `managed_test.go`, `TestManagedStopFailsAfterStoppingRestarts`
     (`fakeLife.stopTakesEffect`): `starts==1`, `healths==1`, the error
     matches `ErrManagedInstall` and the injected error, target
     `"old-bytes"`. Fails on HEAD: `stops=1 starts=0 healths=0`.
     `service/launchd/launchd_test.go`, `TestManagedStopTimeoutRestartsJob`
     (`newFake`, `pid=99999999`, `goneAfter=1<<30`, through
     `selfupdate.NewManagedInstaller`): `kickstart` follows. Fails on HEAD:
     verbs end `[bootout print print print]`.
     `TestStopWaitFollowsExitTimeOut`: key `ExitTimeOut: "90"`, the fake
     records `ctx.Deadline()` for `print`: about 120 s, not
     `Poll.Timeout`. Analogous tests beside `systemd_test.go`
     `TestStopStalls` and in `scm_test.go`. `TestStopTimesOut` is changed
     to set the seams (recorded as a deviation when it lands). Plants:
     `restart=false` in `recoverStop`; launchd's wait back to
     `Poll.Timeout`.
   * Live: `launchd/live_darwin_test.go` `TestLiveStopWaitsForSlowExit`
     gains `ExitTimeOut 20` with `Poll.Timeout 5 s`: `Stop` succeeds.
   * Docs: `launchd/lifecycle.go:99-102`, the `Poll` field comments
     (`launchd/job.go:96`, `systemd/unit.go:54`, `scm/service.go:35`), the
     extending guide's managed-update section ("a stop that fails after
     stopping restarts the service").
5. **Acceptance:** every test above fails on HEAD and passes; every
   existing `selfupdate` and `service` test passes; `make apicheck`
   compatible with `v1.10.0`; Windows cross-compile and the windows-2025 CI
   leg pass.

### Phase P2: network and integrity (A1 selectors, A2, A3, A4, A6, A7)

1. **A1, a missing platform asset is cached** (`assets.go:108-113`,
   `archive/select.go:130-156`). `assets.go:109` wraps
   `ErrUnsupportedPlatform` with `%w`; `exactlyOne` gains a `missing
   error` argument: `ErrUnsupportedPlatform` for the archive, none for
   `SHA256SUMS`. Tests: `checker_test.go` `checkRows` gains "release lacks
   the platform's asset" (`wantErr: ErrUnsupportedPlatform`);
   `checkcache_outcome_test.go:28` counts 4 outcome rows;
   `archive/select_test.go` `TestSelectRefusals`: "no archive" matches
   `ErrUnsupportedPlatform`, "no manifest" does not. Fails on HEAD. Plant:
   drop the `%w`. Docs: `errors.go:22-24`; migration note (P8): a missing
   asset's failure class is `unsupported-platform`.
2. **A2, an env-token fallback is per run** (`github.go:681-709,724-744`).
   `credentialState` gains `perRun`, set when the token came from the
   environment after the provider declined; the cache test at `:681`
   becomes `s.cred.resolved && ((s.cred.has && !s.cred.perRun) ||
   s.cred.anonRun == mark)`. Test `credentials_run_test.go`,
   `TestPromptAfterStartupCheckWithEnvToken` (`t.Setenv("GH_TOKEN",
   "stale")`, `RequireToken("good")`, `answerCredentials`): one prompt,
   applied. Fails on HEAD: `prompts=0 … github http 401`. Plant: drop
   `!s.cred.perRun`. Docs: `github.go:49-53`, `doc.go:83-87`, extending
   guide `:121-124`.
3. **A3, a cross-origin hop keeps only fixed headers** (`github.go:224-234`).
   A package `forwardedHeaders` set (`Accept`, `Accept-Encoding`,
   `User-Agent`, `X-Github-Api-Version`); every other header is deleted
   on a cross-origin hop. Test `network_hardening_test.go`,
   `TestRedirectKeepsOnlyFixedHeaders` (calls `checkRedirect` directly
   with `X-Old-Key` while the credential is `X-New-Key`). Fails on HEAD.
   Plant: keep one non-fixed header. Docs: `github.go:647-651`,
   `doc.go:85-86`, extending guide `:127-128`.
4. **A4, a headerless secondary limit** (`github.go:789-809`).
   `rateLimitedForbidden(resp, body)` is also true when the lowercased
   body contains `secondary rate limit`; `mapStatus` passes the
   truncated body. Test `github_hardening_test.go`,
   `TestGitHubSecondaryRateLimitWithoutHeaders`: a `*RateLimitError`, and
   one network call over three `CheckCached`. Fails on HEAD:
   `isRateLimited=false calls=3`. Plant: drop the body clause.
5. **A6, asset bytes as served** (`github.go:556-572`). `newRequest` sets
   `Accept-Encoding: identity` for `gitHubAcceptAsset`. Test
   `github_hardening_test.go`, `TestOpenAssetKeepsContentEncodedBytes`
   (the server always answers `Content-Encoding: gzip`): the bytes arrive
   undecoded, and the server saw `identity`. Fails on HEAD. Plant: drop
   the header. Depends on A3's set including `Accept-Encoding`.
6. **A7, the stored-deferral cap is tested** (`checkcache.go:167-169`).
   `network_hardening_test.go:60`: `c, src, req, key := cacheEnv(t)`, and
   the stored record's `Request` is `key`. Passes on HEAD; plant `if false
   && …` at `:167` makes it fail ("check deferred by an earlier rate
   limit").
7. **Acceptance:** as P1.

### Phase P3: install (B2, B6, B8, B9, B10)

1. **B2, the locked target is required at the replace**
   (`replace_unix.go:47-60`, `replace_windows.go:32`, `target.go`). New
   `lockedTarget(target) (os.FileInfo, error)`: `Lstat`, a regular file,
   `sameTargetIdentity`; otherwise `fmt.Errorf("selfupdate: target
   changed during the update: %w", ErrConcurrentUpdate)`. It replaces the
   `os.Lstat` in both `replaceTarget`s. Test `install_hardening_test.go`,
   `TestInstallRefusesTargetReplacedAfterBegin`, subtests `symlink`
   (`symlinkOrSkip`) and `file`: `ErrConcurrentUpdate`, `!Applied`, the
   swapped-in file untouched. Fails on HEAD: `applied=true err=<nil>`.
   Plants: drop the identity check; drop `IsRegular`. Docs:
   `errors.go:16-18`, `docs/architecture.md:237-239`.
2. **B6, a probe ended by the run's context** (`probe.go:92-107`). The
   parameter becomes `parent`; when `cmd.Run` fails or the derived context
   ended, and `parent.Err() != nil`, return `fmt.Errorf("selfupdate: %s
   probe of %s stopped: %w", phase, product, parent.Err())`. Test
   `probe_test.go`, `TestVersionProberHonoursRunContext`
   (`SELFUPDATE_TEST_SLEEP=10s`, a 10 s prober timeout, a 100 ms parent
   deadline, and a cancel): `context.DeadlineExceeded` and
   `context.Canceled`, `failureClass` `deadline-exceeded`. Fails on HEAD.
   Plant: drop the `parent.Err()` branch. Docs: `probe.go:75-78`.
3. **B8, the receipt's temporary file is a leftover**
   (`leftovers.go:22-36`, `cleanup_windows.go:267`). `isLeftover` matches
   `.<base>.selfupdate.cleanup-tmp-<digits>`. Tests `TestIsLeftover` rows
   (`…cleanup-tmp-12` true; `…cleanup-tmp-`, `…cleanup-tmp-1x` false);
   `TestRemoveLeftoversKeepsListed` removes it with `sweepBackups` false.
   Fails on HEAD. Plant: drop the branch.
4. **B9, a PE DLL is refused** (`imageverify.go:139-140`).
   `&& f.Characteristics&pe.IMAGE_FILE_DLL == 0`. Test `imageverify_test.go`
   `TestImageVerifier`: the Windows fixture with characteristics bit
   0x2000 set at `e_lfanew+4+18` is `ErrIntegrity` for `NewImageVerifier`
   and `CheckImage`. Fails on HEAD. Plant: drop the clause. Docs:
   `imageverify.go:30-35,44-49`.
5. **B10, the coverage gaps** (tests only). `target_test.go`,
   `TestResolveTargetDefaultExecutable` (`setSeam(&osExecutable, …)`:
   success, error, empty) and `TestRawExecutablePathRelative`
   (`t.Chdir`); `managed_test.go`, `TestManagedSessionTarget`. They pass
   on HEAD; plants: `osExecutable` not called; `Target` returns
   `Target{}`; `raw = abs` dropped. Each fails its test.
6. **Acceptance:** as P1; `rawExecutablePath` and `managedSession.Target`
   above 80 % and 100 % in module-wide coverage.

### Phase P4: API and command surface (C1, C2, C4, C6, C7, A9, G5, G6)

1. **C1, the result object carries the run's exit** (`cli/run.go:143-176`).
   After `RunWith`: `late := o.warn(res)`; `late = errors.Join(late,
   o.HandOff.Report(res, joinLate(err, late)))`; `err = joinLate(err,
   late)`; `return res, joinLate(err, o.finish(res, err))`. `joinLate`
   keeps 0010 C3's rule: under `ErrUpdateAvailable` the late error alone
   decides. Test `cli/run_test.go`, `TestResultObjectExitCodeMatchesExit`:
   a failing `Report`, a failing warning write (`buildUpdaterClosing` with
   "unlock failed" and a `failOn` stderr), and every `scenarios` entry:
   the last line's `exit_code` equals `Exit`'s, with `error` set when
   non-zero. Fails on HEAD: `exit_code:0`, exit 1. Plant: `finish` before
   `Report`. Docs: `cli/doc.go:14-18`, `cli/run.go:62-64`.
2. **C2, an early failure reaches `Report`** (`cli/command.go:63-71`).
   `Options.report` calls `o.HandOff.Report(res, err)` first and joins its
   error; help is not reported. Test `cli/command_test.go`,
   `TestCommandEarlyFailureReachesReport`: `newUpdater` failing, nil, a
   positional argument, `--check --yes`; `Report` once with an error,
   exit 1; under JSON the object's `error` includes a failing `Report`'s.
   Fails on HEAD: `Report calls=0`. Plant: drop the call. Docs:
   `cli/run.go:62-64`, `cli/command.go:19-21`.
3. **C4, `Check` agrees with `Run`** (`checker.go:95-97,134-150`,
   `updater.go:165-167,409-420`, `runoptions.go:156-158`). `Checker` gains
   unexported `unpacker` and `fromUpdater`, set by `Updater.Checker()` and
   `run.checker()`; `matchUnpacker` becomes a `Checker` method that runs
   only when `fromUpdater`, at the end of `discover`; the separate call in
   `Run` goes. Test: `checker_test.go` `TestCheckerAgreesWithRunCheck`
   extended (`newPackEnv`, `cfg.Unpacker=nil`, and the reverse):
   `Checker().Check` and `CheckCached` (`memStore`) fail with `Run
   --check`'s text. Fails on HEAD: `available=true`. Plant: skip the match
   in `discover`. Docs: `checker.go:10-26,93-94`.
4. **C6, a failed run's result names it** (`updater.go:139-185`). After
   `validateProduct`, `base := Result{Product, CurrentVersion}`, with
   `Checked: req.CheckOnly` and `DryRun: req.DryRun` after
   `validateRequest`; `base` replaces `Result{}` on every later failure.
   Test `updater_contract_test.go`, `TestDiscoveryFailureResultNamesRun`.
   Fails on HEAD. Plant: `Result{}` at the discovery error. Goldens:
   `failed.json.stdout`, `contradiction.json.stdout` (rule 8).
5. **C7, typed-nil writers are refused** (`reporter.go:19-27`,
   `jsonreporter.go:34-39`, `confirmer.go:42,67,138`, `cli/run.go:89-91`,
   `cli/command.go:27`). The constructors normalise `isNil(w)` to nil, so
   the existing "writer is nil" errors fire; `cli` gains `isNilWriter` for
   `Options.check` and `Command`. Tests: `events_test.go`,
   `TestTypedNilWriters`; `cli/run_test.go` `TestOptionsRefused` gains
   "typed-nil stderr" and "typed-nil stdout with JSON". Fail on HEAD
   (panics). Plant: drop one normalisation.
6. **A9, G5, G6, godoc:** `types.go:188-189` and `github.go:97-98,647-651`
   say the clone's `CheckRedirect` is the source's own (at most 10 hops,
   HTTPS only, plain http only between loopback hosts, the credential
   stripped off another origin) and the caller's is never called; test
   `github_test.go`, `TestNewGitHubSourceReplacesCheckRedirect` (passes on
   HEAD; plant: chain the caller's). `types.go:597`: `InstallSession.Install`.
   `doc.go:96-98`: verifiers see the asset as published, the archive with
   an Unpacker, and `New` refuses `NewImageVerifier` beside one;
   `releasespec/doc.go:1-4` names the generated installers and 0014 §2.
7. **Acceptance:** as P1; the golden diff reviewed.

### Phase P5: service backends (D2, D3, D4, D5, D7, D8, D11)

1. **D2, the restart baseline is read after the start**
   (`systemd/lifecycle.go:91-104,138`). `Start` reads `InvocationID`
   before `systemctl start` and `NRestarts` in a second `show` after it
   exits 0. Test `systemd_test.go`, `TestStartBaselineIsAfterStart`
   (`NRestarts` 3 before, 0 after, a new `InvocationID`): `WaitHealthy`
   nil. Fails on HEAD: `not healthy: … NRestarts=0`.
   `TestWaitHealthyFailsFast` "restarted" changes to `f.showQueue`. Plant:
   read before the start. Docs: `lifecycle.go:91-93,116-119`; 0011 §7 gets
   a note through R2's style in P8's record. Live:
   `systemd/live_linux_test.go`, `TestLiveUpdateAfterAutoRestart`.
2. **D3, a loaded idle launchd job is reloaded**
   (`launchd/reconcile.go:29-34`, `lifecycle.go:151-176`). In `rewrite`:
   loaded and running → refuse ("stop the job first"); loaded and not
   running → `Inside` check, write, `bootout`, the stop wait, `bootstrap`
   without `kickstart`; not loaded → unchanged. `Restore` mirrors it. An
   unexported `reloaded` flag on `Job` keeps `Start` from taking the
   reload's process as `previous`. Test `launchd_test.go`,
   `TestReconcileReloadsLoadedJob`: `bootout` before `bootstrap`; `Start`
   `kickstart`s; `WaitHealthy` passes; `Restore` reloads too. Fails on HEAD:
   verbs `[list enable kickstart]`. `TestReconcileRewriteAndRestore` uses
   `pid=0`. Plant: drop the reload. Docs: `reconcile.go:29-34`,
   `lifecycle.go:151-153`, `job.go:100`, extending guide `:400-403`,
   `docs/architecture.md:199-202`. Live: `TestLiveRewriteLoadedIdleJob`.
3. **D4, the effective `ExecStart` is verified**
   (`systemd/reconcile.go:17,88-146`). After `reload`, `show -p ExecStart`
   (format `ExecStart={ path=… ; argv[]=… ; … }`) must name the new
   binary (`service.SameExecutable`); otherwise the receipt is returned
   with an error naming the overriding drop-in, so recovery's `Restore`
   removes it. The drop-in keeps its name. Test `reconcile_test.go`,
   `TestReconcileVerifiesEffectiveExecStart` (fragment plus
   `override.conf`; the fake gains `onReload`): an error, `Changed`, and
   `Restore` removes the drop-in. Fails on HEAD: `changed=true err=<nil>`.
   `TestReconcileRewrite` sets the new `ExecStart` in `onReload`. Plant:
   delete the check. Docs: `reconcile.go:31-35,70-73`, `unit.go:56-57`,
   guide `:400-403`. Live: `TestLiveRewriteWithOverride`.
4. **D5, an unquoted SCM command line with spaces**
   (`scm/reconcile.go:48-61,88-108`). For an unquoted line with a space:
   try each prefix ending at a space, and the whole line, appending `.exe`
   to one without an extension; the first existing regular file is the
   program (the `CreateProcess` order); if none exists, the first prefix
   ending in `.exe`, case-insensitively; if none, refuse ("cannot tell
   where the program ends in the unquoted command line %q; quote it").
   Under `RewritePath`, refuse when the existence choice and the `.exe`
   choice differ. Seam `statFile`. Test `scm_test.go`,
   `TestReconcileUnquotedPathWithSpaces`: the moved-binary case names
   `C:\Program Files\Old\demo.exe` and rewrites to `"C:\Program
   Files\New\demo.exe" run`; with `C:\Program.exe` present, `RewritePath`
   refuses. Fails on HEAD: `demo runs C:\Program`. Plant: plain
   `decompose`. Windows-only: `compose_windows_test.go`
   `TestReconcileRealCommandLine` with real files under a directory with a
   space. Docs: `reconcile.go:23-29,88-91`, `service.go:38-40`. Live:
   `TestLiveUnquotedPathWithSpace`.
5. **D7, an unreadable plist is an error** (`launchd/lifecycle.go:38-75`).
   One helper `plistValue(ctx, key) (typ, raw string, present bool, err
   error)`: `plutil -lint -- <plist>` (non-zero is an error; a missing file
   wraps `ErrNotInstalled`); `plutil -type <key> -- <plist>` (exit 1 after
   a clean lint, with "No value at that key path", is missing); `-extract
   <key> raw` for `bool`. `RunAtLoad` is true only for a bool `true`;
   `KeepAlive` for bool `true` or a dictionary. Tests: `TestEnabled` cases
   "unreadable" and "RunAtLoad integer" (the fake's `plutil` gains `-lint`
   and `-type`); `plutil_darwin_test.go`, `TestEnabledRealPlutil`
   (corrupt, mode 0000 skipped as root, `<integer>0</integer>`). Fail on
   HEAD: `Enabled=false,<nil>`, and `true` for the integer. Plant: `false,
   nil` on a lint failure. Docs: `lifecycle.go:38-40,57-62`. Live:
   `TestLiveRunAtLoadInteger` pins launchd's own reading.
6. **D8, handoff files per unit** (`systemd/detach.go:80-84,172-189`).
   `<envDir>/<unit>/handoff-<id>.env`, both levels `privateDir` (0700);
   the sweep reads only that directory with `os.ReadDir` and prefix and
   suffix checks; legacy flat files are left alone. Test
   `detach_unix_test.go`, `TestDetachKeepsOtherUnitsEnvFile` (systemd 239,
   two units, one `getenv`). Fails on HEAD: `other.service's env file is
   gone`. `TestDetach:58`, `TestDetachOldSystemd:82` and
   `TestDetachFailureRemovesFile:122` follow the layout. Plant: glob the
   parent. Docs: 0011 A2's layout noted in P8's record.
7. **D11, "not found" for the unit's own name** (`systemd/unit.go:224-237`).
   Match only `unit <unit> not found`, `unit <unit> not loaded`, `unit
   file <unit> does not exist`, lowercased. Test `TestCommandErrors` gains
   `Failed to start demo.service: Unit missing-dep.service not found.`
   (not `ErrNotInstalled`). Fails on HEAD. Plant: bare `Contains("not
   found")`. Live: `TestLiveMissingDependency`.
8. **Acceptance:** as P1; Linux and Windows cross-compile; the live tests
   named here are added and run per rule 9.

### Phase P6: archives, the spec and codesign (E1–E6)

1. **E1, a zip's local header must agree** (`archive/unpack.go:328-391`).
   * A `headerRecorder` (`io.ReaderAt` recording the last read's offset
     and length) wraps the archive passed to `zip.NewReader`; before each
     `zf.DataOffset()` its length is reset; afterwards a length other than
     30 is refused ("its local header could not be located").
   * `checkLocal(ra, zf, hdr, data)`, after the overlap sort: the 30-byte
     header (`PK\x03\x04`; flags at 6; method at 8; name length at 26;
     extra length at 28) with `hdr+30+n+m == data`, the name bytes equal
     to `zf.Name`, the method equal to `zf.Method`, local flags
     `&0x2041 == 0`.
   * `checkExtra` walks `[tag u16][size u16][data]` on the central and the
     local extra, refusing an overrun and tag 0x7075.
   * Tests `TestUnpackRefuses` with `main_test.go` helpers
     `localNameZip(t, central, local, body)` and `unicodePathExtra(name,
     u)`: "local header names it" and "Info-ZIP Unicode Path", each
     `ErrIntegrity`. Fail on HEAD (`Unpack = <nil>`). Both builders seed
     `FuzzUnpackZip`. Plants: the name comparison off; the 0x7075 check
     off; `checkLocal` before the overlap check
     (`TestUnpackRefuses/overlapping_entries` fails).
2. **E2, names that fold** (`archive/unpack.go:209`; N1). In
   `entries.add`, after `checkName`: a byte outside 0x20–0x7e is refused
   ("is not printable ASCII"); an element of the cleaned name ending in
   `.` or a space is refused. Tests: `relays` plus `relayſ`; `prog+"."`;
   `prog+" "`. Fail on HEAD. The long-s tar seeds `FuzzUnpackTarGz`.
   Plants: drop the ASCII loop; drop the suffix test (keep the variable
   used, so a plant is a failure, not a compile error).
3. **E3, a directory is an entry ending in `/`** (`unpack.go:293-296,348-351`).
   Zip: `mode.IsDir()` must equal `strings.HasSuffix(zf.Name, "/")`
   ("directory attribute does not match its name"), and a directory entry
   with data is refused ("holds data"). Tar: a regular entry whose name
   ends in `/` is refused ("regular file %q is named as a directory").
   Tests: a FAT entry with attribute 0x10 and data, plus `x/relay`; a
   `zfile` with `SetMode(fs.ModeDir|0755)`; a raw tar built with
   `slashRegTarGz`; a directory entry with data (built by name
   replacement). Fail on HEAD. Seeds. Plants: each rule off.
4. **E4, encrypted entries** (`unpack.go:352-354`). `zf.Flags&0x2041 != 0`
   is refused ("is encrypted (flags %#04x)"); `checkLocal` refuses the
   local flags with "(local flags". Tests: stored entries with flags 0x1
   and 0x40. Fail on HEAD. Plant: the central check off (the message
   differs).
5. **E5, composed names the client accepts** (`releasespec/validate.go`,
   `spec.go:257-272`, `internal/cmd/selfupdate-release/check.go:46-56`;
   N2). New `validateArchiveNames()` in `Validate` after
   `validatePlatforms`: under `archive` packaging, every `AssetName` must
   match the 128-character rule (`releasespec: products[%d].name: the
   asset name %q is %d characters; the archive selector accepts at most
   128`), and for a tar.gz platform the program name (with `.exe` on
   Windows) at most 100 characters. `check` builds `archive.NewSelector`
   from the packed platforms and selects each name ("the client's selector
   refuses it"). Tests: `spec_test.go` `TestParseRefuses` (120 characters
   under archive refused; 110 plus `-windows-amd64.zip` = 128 accepted; 111
   refused; 120 under binary accepted; a 101-character tar.gz program
   refused); `pack_test.go` `TestCheck` with a 120-character gz product.
   Fail on HEAD. A 128-character seed for `FuzzParse`. Plants: the call
   removed; 129 allowed; binary included; `Select` removed. Docs: 0013-MADR
   §2 (`:339-343`) noted in P8's record; building guide §7 and step 1.
6. **E6, the requirement in its own run** (`codesign/codesign.go:124-181`).
   `Transform` runs `codesign --verify --strict -R=identifier
   "<Identifier>" <file>`, then, when set, `codesign --verify --strict
   -R=<Requirement> <file>`; a factored `verify(ctx, requirement, file)`;
   `requirement()` goes. Tests: `TestSignerRequirementIsSeparate` (`anchor
   apple) or (always` and the comment form: three argv lines, exact `-R=`
   values); `TestSignerErrors` with outputs `{},{},{ExitCode:3}`;
   `TestSignerArguments` expects three calls. Fail on HEAD. Plants:
   composition restored; the requirement run skipped. Docs:
   `codesign.go:48-50`; 0012 §5 noted in P8's record.
7. **Docs for E1–E4:** 0012 §4's refusal list and the extending guide's
   "Ship an archive" (`:213-217`) name the new refusals.
8. **Acceptance:** as P1; each fuzz target 60 s clean with the new seeds;
   real archives from Info-ZIP `zip`, `ditto -c -k`, bsdtar and `git
   archive` (the last after Q4) unpack.

### Phase P7: installers and release tooling (F1, F2, F4, F5–F9, G7)

1. **F1, a repeated `--product` counts once.** `install.sh`: at parse,
   `case $2 in '' | *[!A-Za-z0-9._-]*) die 1 "--product needs a name, not
   \"$2\"" ;; esac`; a helper `in_list WORD LIST` (no `local`); the
   validation loop builds `chosen` without repeats. `install.ps1` (`:323-327`):
   a loop that refuses unknown names and skips repeats with
   `-cnotcontains`. Tests: `installer_sh_test.go` "a repeated --product
   installs it once" (`standIn("old")`; exit 0; `relay.prev` holds the
   old); `installer_ps_test.go` the same in `psBlock` with `-Product
   'relay','relay'`. Fail on HEAD (`exit 1 … mv: rename …relay.new…`).
   Plant: drop the repeat guard. Docs: building guide `:385`.
2. **F2, hooks and identity commands get no stdin.** `install.sh`:
   `"$exe" $args </dev/null` (`:210`) and `first=$("$1/$prod" $args
   2>/dev/null </dev/null | head -n 1)` (`:241`). Tests with a new
   `renderWith(t, kind, edit)` and `stdinReader(role, version)`: "an
   identity command that reads stdin hides no product" (a second identity
   product reporting `v9.9.9`: exit 2); "a hook that reads stdin skips no
   hook" (a second `after_install` hook: both logged). Fail on HEAD.
   Plants: either `</dev/null` removed. Docs: building guide `:410` and
   step 3 ("they run with no standard input").
3. **F4, the immutability timeout explains itself**
   (`publish-selfupdate-release.yml:244-263`). `immutable` kept outside
   the loop; after the deadline, when not immutable: "$tag is published,
   but GitHub has not marked it immutable after 120 s. Immutable releases
   must be on for $GH_REPO: Settings > General > Releases > "Enable
   release immutability" (or the organization's release policy); it
   applies only to releases published after it is on. The release is live
   and mutable. Delete it (gh release delete $tag --repo $GH_REPO --yes),
   turn the setting on, and re-run all jobs."; when immutable, "… is
   immutable, but gh release verify did not pass within 120 s". Test:
   `workflow-shape_test.sh` asserts the step's `run` contains `Enable
   release immutability`. Fails on HEAD. Plant: drop the phrase.
4. **F5, a relative install directory is made absolute.** `install.sh`,
   after `mkdir -p "$DIR"`: `case $DIR in /*) ;; *) DIR=$(CDPATH='' cd --
   "$DIR" && pwd) || die 1 "cannot resolve $DIR" ;; esac`. `install.ps1`,
   after `:320`: `GetUnresolvedProviderPathFromPSPath($dir, [ref]$prov,
   [ref]$drv)`, refusing a non-`FileSystem` provider. Tests: sh "a relative
   --dir is advised as an absolute PATH entry" (`shCase.cwd`, the expected
   path through `EvalSymlinks`); ps `-InstallDir rel` writes the absolute
   folder to the scratch PATH key. Fail on HEAD. Plant: drop the resolve.
   Docs: building guide `:384`.
5. **F6, hashes from stdin.** `install.sh:123-133`: `sha256sum <"$1"`,
   `shasum -a 256 <"$1"`, `openssl dgst -sha256 -r <"$1"`. Test
   `TestInstallShHashToolsEscapes`, one subtest per tool (`linkTools`,
   `needStubs`, `--dir home/a\b`): exit 0. The `shasum` leg fails on HEAD:
   `SHA-256 \… does not match`. Plant: `shasum -a 256 "$1"` restored.
6. **F7, edge cases.** (a) `check_tag` first refuses `''` and anything
   outside `[A-Za-z0-9.-]`; `install.ps1` `Test-ReleaseTag` (`:129`) ends
   with `\z`. (b) closed by F1's parse rule. (c) a `PREVIOUS` list beside
   the `mv` to `.prev`; `restore` puts back only those (`$hadPrevious` in
   ps1). (d) a preflight after the directory is resolved: a directory at
   `$DIR/$prod` or `$DIR/$prod.prev` exits 1 ("… is a directory; nothing
   was changed"); ps1 `Test-Path -PathType Container`. Tests: "a --version
   with a newline is refused" (exit 1, no request); "an empty --product is
   refused"; a stale `relay.prev` stays when identity fails; a directory at
   the target (exit 1, the directory untouched); the ps equivalents for
   (a) and (d). Fail on HEAD. Plants: each rule off. Docs: the guide's
   exit-code table and Options.
7. **F8, a latest release that is not `vX.Y.Z`**
   (`scripts/release-latest-flag.sh:53-62`). Compare with the highest
   published stable `vX.Y.Z` from `gh release list --exclude-drafts
   --exclude-pre-releases --limit 1000 --json tagName --jq
   '.[].tagName'`; none → no flag, with a notice naming the old latest; a
   failed list → exit 1. Test `release-latest-flag_test.sh` (stub answers
   `release list` from `STUB_LIST`): `legacy-2024` latest with `v1.5.0
   v1.4.1 nightly-7 v2.0.0-rc.1`: `v1.4.2` → `--latest=false`, `v1.6.0` →
   none; no `vX.Y.Z` → none; list error → rc 1. Fail on HEAD. Plants: skip
   the fallback; `min` for `max`.
8. **F9, one publish at a time** (`publish-selfupdate-release.yml:39-41`;
   N9). Under `jobs.publish`: `concurrency: {group:
   go-selfupdate-lib-publish-${{ github.repository }}, cancel-in-progress:
   false}`. Test: `workflow-shape_test.sh` asserts the group and
   `cancel-in-progress` false. Fails on HEAD. Plant: `cancel-in-progress:
   true`. Docs: building guide step 4 (the group's name, not to be
   reused), `docs/architecture.md` publish section.
9. **G7, the usage line** (`main.go:11`): `[-repository OWNER/NAME]`. Test
   `usage_test.go`, `TestUsageListsEveryFlag` (`go/parser` over the
   non-test files: each `f.str(name, required)` per `runX` against the
   usage line). Fails on HEAD: `stage: -repository is not in the usage
   line`. Plant: remove it.
10. **Acceptance:** as P1; `scripts/check-installers.sh` on both
    templates; shellcheck, dash `-n`; the sh tests on this Mac and the
    Linux host, the container job in CI; the ps tests on the Windows host
    under 5.1 and 7; the workflow checks.

### Phase P8: the `v1.10.1` release

1. **Migration guide:** `### v1.10.1` at the end of §10: a kept backup's
   new name and that a dry run sweeps nothing (B1); a second `Commit` or
   `Rollback` refused (B7); `ErrConcurrentUpdate` for a target replaced
   mid-update (B2); a missing asset's class `unsupported-platform` (A1);
   the archives now refused (E1–E4) and the spec names refused (E5); the
   requirement's own run (E6); the installers' changes (F1, F2, F5–F7);
   the publish workflow's concurrency and messages (F4, F8, F9).
2. **Records noted here, not amended elsewhere:** 0011 §7 (D2's baseline,
   D3's reload), 0011 A2 (D8's layout), 0012 §4 and §5 (E1–E4, E6),
   0013 §2 (E5), each a dated note in this PLAN's execution record and an
   amendment line in the record named.
3. **Release notes** for `v1.10.1` in this PLAN.
4. **Before the tag:** CI green on `main`; the live tests of P1 and P5
   pass on the test hosts and macOS CI (rule 9).
5. **The owner** pushes, tags `v1.10.1` (annotated, message `v1.10.1`)
   and pushes the tag; the agent checks the tag rule and the tag's CI.
6. **The pin commit:** the README, the guides, `docs/architecture.md`
   (the current release and its commit) move to `v1.10.1`; every `go get`
   line and `go list` check names `v1.10.1`.
7. **Checks:** `proxy.golang.org` serves `v1.10.1` and `@latest` resolves
   to it; on the owner's ask, a live rehearsal of the installers as
   0014-PLAN I7 step 3 did, in a throwaway repository the owner removes.

### Phase Q1: API and command-surface contracts (C3, C8, C9, G3, A1's 404)

1. **C3, a handoff needs `--yes` and an operation** (`service/handoff.go:146-156`,
   `cli/run.go:123-132`). `HandOffFunc` uses `req`: inside the service
   (`d.Inside`), with `EnvHandOff` unset and `!req.Yes`, it returns
   `fmt.Errorf("selfupdate: service: an update from inside the service
   runs detached and cannot ask; pass --yes: %w",
   selfupdate.ErrConfirmationRequired)`; outside it returns `false, "",
   nil`. `cli.Run` calls `Detach` only when `wouldInstall(ctx, u, req)`:
   `u.Checker().Check(…)` from the request's fields; an error or
   `ForceRequired && !req.Force` → false; otherwise `Available ||
   req.Force`. Tests `cli/handoff_test.go`: `TestHandOffNeedsYes` (an
   `insideDetacher`, interactive `y`, no `--yes`: not detached, exit 1,
   `ErrConfirmationRequired`, target unchanged); `TestHandOffOnlyWhenUpdateFound`
   (`latest v1.0.0`, `--yes`: no `Detach`, exit 0, "up to date").
   `service/handoff_test.go` `TestHandOffFunc` passes `Yes: true` and
   gains the refusal case. Fail on HEAD. Plants: drop `wouldInstall`;
   drop the `!req.Yes` branch. Docs: `cli/run.go:57-61`, `cli/doc.go:31-35`,
   extending guide `:449-458`. Needs P4's C4.
2. **C8, zero values** (`runoptions.go:110`, `checker.go:104,123`,
   `stream.go:161,168,263-267`, `errors.go`). Sentinel `ErrNotConstructed
   = errors.New("selfupdate: not constructed by its constructor")`;
   `RunWith`, `Stream.prepare`, `Checker.Check` and `CheckCached` return it
   for a nil or unconstructed receiver; `Stream.Cancel` returns at once
   when `s == nil || s.cancel == nil`. Test `TestZeroValues`: `Run`,
   `RunWith`, `Check`, `CheckCached` on zero and nil receivers match it;
   `Start(&Updater{})`'s `Finished.Err` matches it; a zero `Cancel` does
   not panic. Fails on HEAD (panics). Plant: drop the `Stream.prepare`
   check. Docs: `types.go:803-804`, `checker.go:24-26`, `stream.go:263-265`,
   `doc.go`'s errors.
3. **C9 and G3, the result document's schema 3** (`updater.go:334,367`,
   `types.go:123-173`, `document.go:3-30`).
   * `Result.RolledBack` ("the previous binary was restored after the new
     one was installed"), set at `updater.go:367`; `ResultDocument`
     `rolled_back` (no `omitempty`).
   * `Result.ProbesSkipped`, set when `req.DryRun` and `req.Platform` is
     not the running platform and probes are configured (they are then
     not run); `ResultDocument` `probes_skipped`. The `complete` Detail
     stays `dry-run`.
   * `resultDocumentSchema` is 3; `HandOffResultSchema` stays 1.
   * Tests: `managed_started_test.go`, `TestRunReportsRolledBack`
     (`fakeLife{installed, running, healthErr}`): `ErrManagedInstall`,
     `res.RolledBack`, `"rolled_back":true`, `EventRolledBack`;
     `platform_apply_test.go`, `TestDryRunForeignPlatformSkipsProbes` and
     the running-platform control; `TestResultDocumentCoversEveryField`
     covers both. Literal strings follow in `events_test.go:368,382`,
     `warnings_test.go:102,107`, `example_test.go:319`,
     `managed_started_test.go:65-67`. Plants: drop the copy at `:367`;
     drop the platform condition.
   * Goldens: all 14 `cli/testdata/golden/*.json.stdout` (rule 8).
   * Live, in each of `launchd/live_darwin_test.go`,
     `systemd/live_linux_test.go`, `scm/live_windows_test.go`:
     `TestLiveHandOffHealthFailureReportsRollback` (a failing binary as
     the new one; `runLiveUpdate` copies `installed.RolledBack`): the
     handoff result has `ExitCode` 1, `!Applied`, `RolledBack`, and the
     service runs the old binary.
   * Docs: `cli/doc.go:18`, extending guide `:97-98,447`.
4. **A1's 404, a release that does not exist** (`github.go:789-809`,
   `checkcache.go:21-41`, `errors.go`). Sentinel `ErrNoRelease =
   errors.New("selfupdate: no such release")`, wrapped for a 404 from
   `releases/latest` and from a tag lookup; `CheckNoRelease` outcome,
   cached for `maxAge`, its name `no-release`, the `EventFailed` class
   `no-release`. Tests: `github_test.go`, `TestLatestNotFoundIsNoRelease`;
   `checkcache_outcome_test.go` gains the row (5 outcomes). Fail on HEAD
   (`github http 404`, five network calls). Plant: drop the wrap.
5. **Acceptance:** `make apicheck` compatible with `v1.10.1`, additions
   only (`ErrNotConstructed`, `ErrNoRelease`, `CheckNoRelease`,
   `Result.RolledBack`, `Result.ProbesSkipped`, the document fields).

### Phase Q2: install and network contracts (A5, B5)

1. **A5** (`checkcache.go:219-240`): `var nb time.Time; if rl.Remaining
   == 0 { nb = rl.Reset }`. Test `TestNotBeforeClamped` rows: Retry-After
   1 m with quota left and a reset 50 m away → 1 m; `Remaining` 0 with a
   reset 30 m away → 30 m. The first fails on HEAD. Plant: `nb :=
   rl.Reset`. Docs: `checkcache.go:139-142,219-225`; a dated note in
   0010-PLAN-v1-5-1 at deviation D1.
2. **B5** (`session.go:164-180`, `replace.go`): `clearSpecialBits(path)`
   (`Lstat`, then `osChmod(path, perm|sticky)` when setuid or setgid is
   set), called in `commitLocked` before the backup becomes `.previous`
   (on failure the backup is removed and `keep previous: …` joined), and
   in P1's `retainLocked`. Test `modebits_unix_test.go`,
   `TestKeepPreviousClearsSpecialBits` (setuid and setgid; the new binary
   keeps the bit). Fails on HEAD (`mode=urwxr-xr-x`). Plant: no chmod.
   Docs: `types.go:156-163,409-413,566-569` (a restored kept backup has
   lost setuid and setgid), `docs/architecture.md:250-253`.

### Phase Q3: service contracts (D6, D10, G4)

1. **D6** (`scm/lifecycle.go:83-211`): unexported `stoppedDeps
   map[string][]string` under `s.mu`; `Stop` records each dependent right
   after `stopOne` succeeds; `Start`, once the service is out of
   `START_PENDING`, starts them in reverse stop order (`errAlreadyRunning`
   counts as success) and clears the record on success; a dependent's
   failure is a `Start` error. Test `scm_test.go`,
   `TestStopDependentsAreRestarted` (order `demo`, `grandchild`, `child`;
   a `Stop` failing on `demo` after `child` stopped, then `Start`: `child`
   runs). Fails on HEAD. Plant: no record. Docs: `service.go:41-43`,
   `lifecycle.go:83-88,179-180`, 0011 §7 noted. Live:
   `TestLiveStopDependentsRestarted`.
2. **D10** (`service/poll.go:28-65,150`): `func (o PollOptions) Validate()
   error` applies the defaults and refuses `Settle >= 0 && Settle >=
   Timeout`; called from `systemd.newUnit`, `scm.newService` and
   `launchd.newJob` (on `max(Settle, 10 s)` unless negative), not from
   `PollHealthy`; `timeoutError` says "not ready within %s". Tests:
   `poll_test.go`, `TestPollOptionsValidate`; each backend's
   `TestNewRefuses` case. `launchd_test.go:304-310` builds valid options
   and then sets `j.o.Poll`. Fail on HEAD. Plant: one constructor without
   the call.
3. **G4** (`types.go:539-547`, `service/execreconciler.go:35-47,104-107`,
   `managed.go:132-144`, `updater.go:409`): `ReconcileResult.Warnings
   Warnings`; `ExecReconciler` sets it from the receipt's warnings, plus
   "<product>: <path> was rewritten and the service manager was not
   reloaded; the next start may run the previous definition" when
   `Changed` and not `Reloaded`; `managed.go` joins an unexported
   `reconcileWarnings` error after a successful commit; `updater.go`
   splits it out with `errors.As` and emits one `EventWarning` per entry
   after `complete`. Tests: `execreconciler_test.go`,
   `TestExecReconcilerSurfacesWarnings`; `managed_started_test.go`,
   `TestRunReportsReconcileWarnings` (`fakeRec.warnings`). Fail on HEAD
   (the field does not exist). Plant: drop the join. Docs:
   `types.go:170-172,627-631`, `doc.go:76-77`, extending guide `:92-98,400-403`.

### Phase Q4: spec, archive and tooling contracts (E7, E8, F3)

1. **E7** (`releasespec/spec.go:154-198`): `walk` refuses a `null` token
   (`releasespec: %q: null is not allowed; leave the field out`; at the
   root `releasespec: null is not allowed`). Tests `TestParseRefuses`:
   `null` for `installer`, `extras`, `packaging`, `prerelease_channels`,
   `tags`, `identity_args`, `installer.name`, `[null]`, the root. Existing
   tests follow: `spec_test.go:137`; `installer_test.go:39-41` (`args`
   `[]any{}`); `render_test.go:23-34`. Seed `"installer":null` for
   `FuzzParse`. Plant: `tok == nil && false`. Docs: 0013 §2 noted,
   building guide step 1, `installer.go:30-31`, `releasespec/doc.go`.
2. **E8** (`archive/unpack.go:282-298`; N3): before `seen.add`, a
   `tar.TypeXGlobalHeader` with any of `path`, `linkpath`, `size` or a
   `GNU.sparse.` key is refused ("a PAX global header sets %q"); otherwise
   it counts toward `MaxEntries` and is skipped. Tests:
   `TestUnpackAccepts` (a `pax_global_header` with a `comment` record);
   `TestUnpackRefuses` (`path`, `linkpath`, `size`). The accept case fails
   on HEAD. Seeds. Plants: no skip; an empty key list. Docs: 0012 §4
   noted, extending guide `:213-217`, `archive/doc.go:15-16`.
3. **F3** (`plan.go`, `build-selfupdate-release.yml:110-142`): `plan` gains
   a required `-module-dir`; `specFloors` (field, version, uses), with
   `installer` → `v1.10.0`; the requirement from `go mod edit -json` run
   through the tool's `goTool` in the module directory; a stdlib semver
   precedence compare (no `x/mod`: depguard's `other-packages`); rules:
   the library's own module, skip; no requirement, error; a directory
   `replace`, skip with a summary row; a module `replace`, its version;
   below the floor, "module <path> requires go-selfupdate-lib <v>; this
   spec's \"installer\" needs v1.10.0 or later, or the program cannot
   parse the spec it embeds (go get
   github.com/maccavelli/go-selfupdate-lib@v1.10.0)". The workflow's Plan
   step passes `-module-dir "src/$MODULE_DIR"` from `env`. Tests
   `plan_test.go`, `TestPlanRefusesAnOldLibrary` (nine cases: `v1.9.0` with
   `installer` refused; `v1.10.0` ok; `v1.9.0` without ok; a pseudo-version
   above ok; `v1.10.0-rc.1` refused; a directory replace ok; a module
   replace at `v1.9.0` refused; the own module ok; no requirement refused)
   and `TestSpecFloorsCoverEveryField` (every JSON path not in `v1.9.0`'s
   set has a floor); `workflow-shape_test.sh` asserts the flag; G7's
   usage test covers it. Fail on HEAD (unknown flag). Plants: delete the
   `installer` row; flip the compare. Docs: building guide step 4
   (`module-dir`), step 12, §9; migration guide §10; `docs/architecture.md`
   `plan` step.

### Phase Q5: the `v1.11.0` release

1. **Migration guide:** "11. From v1.10 to v1.11": `ErrNotConstructed`,
   `ErrNoRelease` and `CheckNoRelease`; the JSON result schema 3
   (`rolled_back`, `probes_skipped`); handoffs need `--yes` and an
   operation; `ReconcileResult.Warnings`; `PollOptions.Validate` and the
   refused options; `StopDependents` restarts dependents; setuid and
   setgid cleared on `.previous`; the back-off rule; `null` refused in a
   spec; `git archive` tarballs accepted; the workflow's module check.
   §2 lists `v1.11.0`.
2. **Release notes** for `v1.11.0` in this PLAN.
3. **Before the tag:** CI green; the live tests of Q1 and Q3 on the test
   hosts and macOS CI.
4. **The owner** pushes and tags `v1.11.0`; the pin commit; the proxy
   check; the 0011 and 0013 records' notes as P8 did.
5. **This PLAN** is `complete` when V1–V10 hold; the MADR's row and this
   PLAN's in `docs/README.md` follow.

## Verification

* **V1.** Every finding of the MADR is fixed with a test that failed on
  the unfixed code and passes after, decided by a row of "Decisions this
  PLAN assumes", or recorded open in R2 with its reason; the execution
  record lists each ID against its test, its HEAD failure text and its
  plant's failure text.
* **V2.** Every plant listed here was applied in a scratch copy and made
  its test fail.
* **V3.** `v1.10.1`: `make apicheck` compatible with `v1.10.0`, and no new
  exported identifier in any package.
* **V4.** `v1.11.0`: compatible with `v1.10.1`, its additions exactly
  Q1–Q4's.
* **V5.** `go.mod`, `go.sum` and depguard unchanged; `go mod tidy -diff`
  clean.
* **V6.** `make gate` exits 0 at every phase from R1 on, and its tests
  catch their plants.
* **V7.** The live tests named in P1, P5, Q1 and Q3 pass on the Linux test
  host, WSL, the Windows test host and macOS CI before the tag of their
  track.
* **V8.** The installer tests pass under sh, dash, bash and BusyBox (CI's
  Alpine job), and under Windows PowerShell 5.1 and 7 in the three forms.
* **V9.** `scripts/check-docs.sh --links` over every tracked Markdown file
  reports no failure; `--ids` over every changed file reports none.
* **V10.** CI is green on `main` after every phase, and on `v1.10.1` and
  `v1.11.0`.

## Rollout and Rollback

* **Track 1** changes no released artifact; a phase reverts on its own.
* **`v1.10.1`** keeps every documented contract, so a program moves to it
  with `go get` and nothing else; consumers move their workflow pins to
  take the template and workflow fixes. If a fix misbehaves after the tag,
  fix forward in `v1.10.2`; a consumer can stay on `v1.10.0`, whose
  behaviour is unchanged.
* **`v1.11.0`** changes the contracts listed in its migration section; a
  consumer that cannot take one stays on `v1.10.1`. Fix forward in
  `v1.11.1`.
* **Before a tag,** phases revert with their dependants: B1 needs B7 (P1);
  C3 needs C4 (P4); Q1's schema 3 gathers C9 and G3; Q3's G4 needs P1's
  managed changes.

## Execution Record

No phase has run.
