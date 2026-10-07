---
status: in-progress
date: 2026-10-07
associated-madr: "0015-MADR-remediate-third-debugging-pass-findings.md"
---
# Implement the third debugging pass's remediation: records and the gate on main, v1.10.1 of contract-preserving fixes, and v1.11.0 for the owner's contracts

Associated MADR: [0015-MADR-remediate-third-debugging-pass-findings.md](0015-MADR-remediate-third-debugging-pass-findings.md)

## Goal

* Each of the MADR's 64 findings (63, and A10 from its amendment A2) ends
  in one of three states:
  * fixed, with a test that failed on the unfixed code and passes after;
  * decided, by a row of "Decisions this PLAN assumes";
  * recorded as open in 0004-MADR P3, with the reason.
* **Track 1, on `main`, no release:**
  * the gate, the document checker and the plant helper live in the
    repository, with tests;
  * the roadmap and the record amendments;
  * the documentation fixes.
* **Track 2, `v1.10.1`:** the two High findings, and every fix that keeps
  the documented contract. `make apicheck` reports it compatible with
  `v1.10.0`, and it adds no exported identifier.
* **Track 3, `v1.11.0`:** the contracts that the owner's answers decide.
* **Before each tag,** CI is green, and the live service tests pass on the
  test hosts.

## Scope

### In scope

| Phase | Track | Findings | Commit |
| :--- | :--- | :--- | :--- |
| R0 | 1 | records | the MADR accepted, this PLAN in progress, the index |
| P0 | 2 | A10 (deviation D1) | a manifest name that fits its canonical line, in both parsers; committed before R1 |
| R1 | 1 | G2 | `scripts/gate.sh`, `scripts/check-docs.sh`, `scripts/plant-copy.sh` and their tests; `make gate`; a CI step |
| R2 | 1 | G1, G3 (record), G10 (record), N4, N5 | 0004 P3; 0011 A6 and its PLAN's D8; the 0010 note |
| R3 | 1 | C5, G8, G9, F3 (docs), F4 (docs) | the guides, the READMEs, `docs/architecture.md`, godoc, one test |
| P1 | 2 | B7, B4, B1, B3 | the High findings and what they rest on |
| P2 | 2 | A1 (selectors), A2, A3, A4, A6, A7 | network and the check cache |
| P3 | 2 | B2, B6, B8, B9, B10 | install |
| P4 | 2 | C1, C2, C4, C6, C7, A9, G5, G6, G10 | the API and the command surface |
| P5 | 2 | D2, D3, D4, D5, D7, D8, D11 | the service backends |
| P6 | 2 | E1, E2, E3, E4, E5, E6 | archives, the spec, codesign |
| P7 | 2 | F1, F2, F4, F5, F6, F7, F8, F9, G7 | the installers and the release tooling |
| P8 | 2 | release | the migration guide, release notes, record amendments, the tag, the pin commit |
| Q1 | 3 | C3, C8, C9, G3, A1 (404) | the API and the command-surface contracts |
| Q2 | 3 | A5, B5 | the install and network contracts |
| Q3 | 3 | D6, D10, G4 | the service contracts |
| Q4 | 3 | E7, E8, F3 | the spec, archive and tooling contracts |
| Q5 | 3 | release | migration §11, release notes, the tag, the pin commit |

The MADR's aliases are planned once, under the first ID: D1 is B3, D9 is
C3, and A8 is merged into G9.

### Out of scope

* **Recorded as open in R2's 0004 amendment P3, not fixed:**
  * a zip local entry that no central record names, in a gap between
    entries, which only a streaming reader sees (N4); closing it means
    parsing the end of central directory, which `archive/zip` hides;
  * a crash between `Apply` and `Commit` (N5): the backup is then a crash
    leftover with the verified new binary live, and the sweep keeps
    removing it, as 0010 Q6 decided;
  * refusing `:` in archive entry names on every host;
  * `queue: max` on the publish workflow's concurrency, which needs an
    actionlint newer than v1.7.12 (N9).
* **`simulate_build.sh`** (0013-PLAN) is not committed. CI's
  `release-rehearsal` jobs, which call the build workflow by its local
  path, are its reproducible equivalent; R1 notes this in 0013-PLAN.
* **Moving any program onto these releases.**
* **Push and tags,** which happen only on the owner's ask in the same turn.

## Decisions this PLAN assumes

**Answered 2026-10-07.** The owner chose the recommended answer to each of
the MADR's 15 questions, as rows Q1–Q15 state them. That includes Q7 in
`v1.10.1`, Q11 with N3's wider refusal list, and Q12 as the per-field
floor. R0 copies these answers into the MADR. The nine decisions that
planning surfaced (N1–N9) are accepted when this PLAN is approved. An
answer the owner changes before approval rewrites only the phase named in
its row.

| # | Finding | Decision | Phase |
| :--- | :--- | :--- | :--- |
| Q1 | C3 | `service.HandOffFunc` refuses, inside the service and without `Yes`, with `ErrConfirmationRequired`, before detaching. `cli.Run` hands off only when a check finds an operation, or with `--force` | Q1 |
| Q2 | C5 | Document that an up-to-date or check-only run's last event is `selected`. No new event | R3 |
| Q3 | C8 | New sentinel `ErrNotConstructed`. A zero or nil `Updater` or `Checker`, and `Start` on one, return it. A zero `Stream.Cancel` is a no-op | Q1 |
| Q4 | C9 | A dry run for another platform skips staged probes and reports `Result.ProbesSkipped` (schema 3) | Q1 |
| Q5 | A5 | `X-RateLimit-Reset` defers only when `Remaining` is 0 | Q2 |
| Q6 | B5 | Clear setuid and setgid on `.previous` and on a kept backup | Q2 |
| Q7 | B7 | Refuse a second `Commit` or `Rollback` of one replacement, in `v1.10.1` | P1 |
| Q8 | D6 | Record the dependents `Stop` stopped; `Start` starts them again | Q3 |
| Q9 | D10 | New `service.PollOptions.Validate`. The backend constructors refuse a settle window at or above the timeout. Wait timeouts say "not ready within" | Q3 |
| Q10 | E7 | `Parse` refuses `null` for every field | Q4 |
| Q11 | E8 | Skip a PAX global header. Refuse one with a `path`, `linkpath` or `size` record, or any `GNU.sparse.` record (with N3) | Q4 |
| Q12 | F3 | `plan` refuses a module whose go-selfupdate-lib requirement is below the spec's field floor (`installer` needs `v1.10.0`). A directory `replace` is not checked | Q4 |
| Q13 | F4 | The timeout error names the repository setting and the recovery; the docs name both prerequisites; no pre-check | R3, P7 |
| Q14 | G2 | Commit the gate, the document checker and the plant helper under `scripts/`, with tests. The identifier check reads the pre-push guard's deny list. CI runs the link check | R1 |
| Q15 | G3 | `Result.RolledBack` and `ResultDocument` `rolled_back` (schema 3), and the live handoff test 0011 promised | Q1, R2 |
| N1 | E2 | Archive entry names must be printable ASCII, and no path element may end in a dot or a space: the only exact fold check without a new module | P6 |
| N2 | E5 | Besides the 128-character asset name, `Validate` refuses a tar.gz platform whose program name exceeds USTAR's 100 characters, which `pack` cannot write | P6 |
| N3 | E8 | The global-header refusal list includes `size` and `GNU.sparse.*` | Q4 |
| N4 | E1 | Unreferenced local zip entries are recorded open, not fixed | R2 |
| N5 | B1 | A crash between `Apply` and `Commit` is recorded, not changed | R2 |
| N6 | A1 | New sentinel `ErrNoRelease` for a 404 from a release lookup, and a cached outcome `CheckNoRelease` | Q1 |
| N7 | G4 | New field `ReconcileResult.Warnings` of type `Warnings`. `ExecReconciler` fills it, and adds one warning when `Changed` and not `Reloaded`. The warnings reach `Result.Warnings` after `complete` | Q3 |
| N8 | B3 | Once its stop is issued, each backend finishes the stop wait whatever the caller's context does, bounded by the service manager's own kill bound: launchd `max(Poll.Timeout, ExitTimeOut + 30 s)`, with `ExitTimeOut` 5 s when absent (0011's probe evidence); systemd `max(Poll.Timeout, TimeoutStopUSec + 30 s)`; SCM `Poll.Timeout`. A cancelled caller still gets its context error | P1 |
| N9 | F9 | Job-level `concurrency`: group `go-selfupdate-lib-publish-${{ github.repository }}`, `cancel-in-progress: false`, the default queue | P7 |

## Corrections to the MADR found while planning

R0 records these in the MADR as amendment A1, before any code changes.

* **B7 moves to `v1.10.1`.** B1's kept backup needs the replacement's state
  in `Rollback` and in managed recovery, which B7's state provides. B7 adds
  no exported identifier, and its refusal fixes a false `Applied=true`.
* **G4 moves to `v1.11.0`:** reporting a reconciler's warnings needs the
  new exported field (N7).
* **A1 splits.** The selectors wrap `ErrUnsupportedPlatform` in `v1.10.1`.
  Caching a release that does not exist yet needs N6, in `v1.11.0`.
* **C9 is reproduced** (the MADR's "—"): a probe ran and failed with
  `exec format error`.
* **F3's rule** is the spec's field floor, not the tool's version. The tool
  is built from a tagless checkout, so its own version reads as a
  pseudo-version or `(devel)`. CI's rehearsal fixture `replace`s the
  library with a directory, which is not checked.
* **F7(a) also holds in `install.ps1`:** `Test-ReleaseTag` (`:129`) ends its
  pattern with `$`, which .NET matches before a final newline.
* **F7(c) also holds in `install.ps1`:** with no `<name>.exe` but a stale
  `<name>.exe.prev`, the identity failure's restore (`:411-423`) moves the
  stale file into place.
* **F7(d)'s fix is two rules:** a hook runs only a regular file, and the
  install refuses a directory at a product's path before changing anything.
* **G7** is `main.go:11`, not `:13`. **G5** is `types.go:598`.
* **C1** is `cli/run.go:142-157`.
* **The cli goldens:** 13 `*.json.stdout` files, not 14.
* **F4:** the README's "Its tag can never be reused" is false when
  immutable releases are off.
* **E5:** `pack` writes tar.gz as USTAR (`pack.go:43`), so a program name
  over 100 characters cannot be packed at all (N2).
* **D2's external fact** is pinned by the fix's rule, not relied on. The
  baseline is read after the start, whatever systemd does with the counter.
* **D7, probe evidence** (macOS 26.6.2, `/usr/bin/plutil`, 2026-10-07):
  * `plutil -lint` passes a file holding only `garbage`: a bare word is a
    valid old-style property list;
  * `plutil -convert xml1 -o - <file>` prints its root as `<string>garbage</string>`;
  * `-type <key>` prints `integer` for `<integer>0</integer>`, and exits 1
    with "No value at that key path" for a missing key or a non-dictionary
    root;
  * `-extract <key> raw -expect bool` exits 1 for an integer;
  * a missing file and a mode-0000 file each exit 1, with a message.

  So the readable-plist check converts to XML and requires a dictionary
  root. The live test `TestLiveRunAtLoadInteger` pins launchd's own reading.

## Rules for every phase

1. **Order.** R0, P0 (deviation D1), R1, R2, R3, then P1–P8, then Q1–Q5.
   Track 1 changes no
   release and may interleave with track 2 once R0 and R1 are committed.
   Q1–Q4 start only after `v1.10.1` is tagged.
2. **One commit per phase.** The agent stages exactly the phase's
   **Files** list. The owner commits with `git commit --no-edit`. The
   global `prepare-commit-msg` hook writes the message. Push and tags happen
   only on the owner's ask in that turn.
3. **Test first.** Each new test is written first, and run on the unfixed
   code. Its failure line is recorded. A test marked **pin** passes
   before the change (it pins behaviour that already holds), and its plant
   proves it instead.
4. **Plant after.** After the fix, each listed plant goes into a scratch
   copy, never the tree. Before R1 lands, the copy is made with the
   session's copy helper; from R1, with `scripts/plant-copy.sh`. The
   phase's named test must then fail in the copy; its failure line is
   recorded, and the copy is removed.
5. **Checks before staging,** each rc 0, and their summary lines recorded:
   * `make gate`, or the session's gate before R1 lands;
   * `make pre-add-check FILES="<every staged .go file>"`;
   * `scripts/check-docs.sh --links` on every changed Markdown file;
   * `scripts/check-docs.sh --ids` on every changed file;
   * `npx --yes markdownlint-cli2@0.23.2` on every changed Markdown file
     the repository's config covers;
   * on changed scripts and templates: shellcheck 0.11.0, and
     `scripts/check-installers.sh` on templates;
   * on changed workflows: actionlint v1.7.12, and
     `scripts/check-workflows.sh`, with `--rule expressions`,
     `--rule permissions` and `--rule pins` on `ci.yml`.
6. **API.** Every track-2 phase: `make apicheck` exits 0 against `v1.10.0`,
   and the full apidiff report (Verification V3) is empty. Track 3:
   `make apicheck` exits 0 against `v1.10.1`, and the report lists only the
   phase's additions.
7. **No new module.** `go.mod` and `go.sum` are unchanged; `go mod tidy
   -diff` is clean. Depguard's rules are unchanged; `releasespec`'s and
   `other-packages`' allow-lists already cover every import below.
8. **Goldens** are regenerated only with `go test ./selfupdate/cli -run
   'TestGolden|TestCommandGolden' -count=1 -update`. Every changed line is
   read and listed in the phase record.
9. **Live tests** named in a phase run as **Live tests** below says, before
   the tag of their track.
10. **Deviations:** stop, and prompt with resolutions that leave the tree
    correct. Record the chosen one as a dated deviation here, and in the
    MADR when a decision or a stated fact changes.
11. **Identifiers:** nothing committed carries a hostname, an account name
    or a real path. `scripts/check-docs.sh --ids` runs before each
    staging.

## Phase procedure

Every phase runs these steps, in order. A phase lists only its own
specifics: **Files**, **Steps** (each with its **Test**, **Red**, **Fix**,
**Plant** and **Docs**), **Run**, and **Acceptance**.

1. **Red.** Write the phase's tests. Run the phase's **Run** command and
   save the output to `$GATE_OUT/red.txt`, capturing `$?` before any
   filter. Each new test fails with the phase's **Red** text, or a test
   marked pin passes.
2. **Fix.** Make the changes the steps name, and only those.
3. **Green.** The same **Run** command passes. Save it to
   `$GATE_OUT/green.txt`.
4. **Plants.** For each plant, `scripts/plant-copy.sh FILE OLD NEW` (OLD
   and NEW as Python string literals). In the printed copy, run `go test
   -count=1 -run '^<Test>$' ./<package>`. It must fail; record the failing
   line, then `rm -rf` the copy.
5. **Docs** named by the steps are edited.
6. **Checks** of rule 5, and rule 6's API check.
7. **Record.** This PLAN's execution record gains the phase's section,
   dated, with:
   * each test's red line and its plant's failing line;
   * the gate's `name rc=N` lines;
   * apicheck's last line;
   * any deviation.
8. **Stage** exactly the **Files** list with `git add -- <files>`.
   `git diff --cached --name-only` must equal the list, and `git status
   --short` must show nothing else modified. Report "staged" and stop. The
   owner commits.

`GATE_OUT` is the gate's output directory, a `mktemp -d` unless set.

## Live tests

| Backend | Where | Command |
| :--- | :--- | :--- |
| systemd, system scope | CI's Linux leg; the Linux test host and WSL on the Windows host, over ssh, in a scratch clone at the commit under test | `go test -c -o "$T/systemd.test" ./selfupdate/service/systemd && sudo env SELFUPDATE_REQUIRE_SYSTEMD=1 FAKE_LIVE_ROOT=/var/tmp "$T/systemd.test" -test.run '^TestLive' -test.count=1 -test.v` |
| systemd, user scope | the same | `env SELFUPDATE_REQUIRE_SYSTEMD=1 SELFUPDATE_SYSTEMD_SCOPE=user FAKE_LIVE_ROOT=/var/tmp XDG_RUNTIME_DIR="/run/user/$(id -u)" "$T/systemd.test" -test.run '^TestLive' -test.count=1 -test.v` |
| launchd | CI's macOS leg; this Mac | `SELFUPDATE_REQUIRE_LAUNCHD=1 go test -count=1 -v -run '^TestLive' ./selfupdate/service/launchd` |
| SCM | CI's Windows leg; the Windows test host, in an elevated session | `$env:SELFUPDATE_REQUIRE_SCM='1'; go test -count=1 -v -run '^TestLive' ./selfupdate/service/scm` |

`$T` is a `mktemp -d`. The scratch clone is made with `git clone` of a `git
bundle` of the commit, copied to the host; the host's own checkout is not
used. Each run's last 40 lines, and each `--- PASS`/`--- FAIL` line, go into
the phase record. Nothing is left behind: the tests remove their own units,
jobs and services, and the clone is removed after.

## Release procedure

The same for `v1.10.1` (P8) and `v1.11.0` (Q5). `vX` is the release.

1. CI on `main` at the release commit is green: all jobs of `ci.yml`.
2. The **Live tests** of the track pass, recorded.
3. On the owner's ask in that turn:
   * `scripts/check-release-tag.sh vX` exits 0;
   * `git tag -a vX -m vX <commit>`;
   * `git push origin vX`.

   Before the push, the disclosure guard runs over the outgoing tag:

   ```bash
   echo "refs/tags/vX $(git rev-parse vX) refs/tags/vX 0000000000000000000000000000000000000000" |
     python3 ~/.global-git-hooks/github-disclosure.py pre-push origin "$(git remote get-url origin)"
   ```

4. `git ls-remote origin 'refs/tags/vX^{}'` gives the commit. The tag's CI
   run passes all jobs, and its identity legs print `vX (release)
   <12-hex>`.
5. **The pin commit,** staged by the agent:
   * `README.md`: the status, the `go get` line, and the publish example's
     pin;
   * `docs/architecture.md`: the current release and its commit;
   * the building guide: both workflow pins in step 4, the extras
     example's pin, and the `ls-remote` example;
   * the migration guide: every `go get` line and `go list` check; §2's
     current release and the list of releases that only add; §3's pin;
     §5's `go.mod` step.

   A version section's statements about its own release stay. The pin is
   `<40-hex> # vX`. The candidates are listed with `git grep -n
   'v1\.10\.0\|a0a26b6' -- README.md docs/architecture.md docs/guides`
   (for `v1.11.0`, the `v1.10.1` pin), and each changed line is named in
   the record.
6. **Proxy:** `GOPROXY=https://proxy.golang.org GOFLAGS=-mod=mod go list -m
   -json github.com/maccavelli/go-selfupdate-lib@vX` gives `vX`, with
   `Origin.Hash` the commit, and `@latest` resolves to `vX`.
7. On the owner's ask only: a live installer rehearsal, as 0014-PLAN I7
   step 3 did, in a throwaway repository the owner removes.

## Implementation Steps

Line numbers are at `d6a570f` (`v1.10.0` plus records). Every Go, template
and workflow line there equals `18e0575`'s. Bare Go file names are in
`selfupdate/`; `cli/` means `selfupdate/cli/`; `service/…` means
`selfupdate/service/…`; `tool/` means `internal/cmd/selfupdate-release/`;
`installer/` means `internal/cmd/selfupdate-release/installer/`.

### Phase R0: records

**Files:** `docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md`,
`docs/decisions/0015-PLAN-remediate-third-debugging-pass-findings.md`,
`docs/README.md`.

1. **The MADR:**
   * front matter `status: accepted`, `date:` the approval date;
   * in Decision Outcome, "Proposed:" becomes "Chosen option:";
   * "Owner questions" opens with a paragraph **Answered (date).**: it
     quotes the owner's approval, and names this PLAN's "Decisions this
     PLAN assumes" table as the answers;
   * a new `### 4. Owner answers`, after `### 3.`, lists the `v1.11.0`
     scope as this PLAN's track 3;
   * a new `## Amendments` before `## More Information`, holding `### A1
     (date): corrections found while planning`, which lists this PLAN's
     Corrections, each with its evidence;
   * the last bullet of "More Information" names this PLAN by file name,
     replacing "A PLAN follows once the owner has answered the questions
     above."
2. **This PLAN:** `status: in-progress`.
3. **`docs/README.md`:** the 0015 MADR row reads `accepted`; the PLAN row
   reads `in-progress`.
4. **Checks:** `--links` and `--ids` (the session's checker) on the three
   files; markdownlint on `docs/README.md`. This is the bootstrap
   exception: records only, no code.

### Phase P0: a manifest name that fits its canonical line (A10)

*Added by deviation D1 (2026-10-07). It runs after R0 and is committed
before R1, whose gate found it.*

**Files:** `selfupdate/checksums.go`, `scripts/selfupdate_manifest.py`,
`selfupdate/checksums_test.go`, `selfupdate/manifest_differential_test.go`,
`selfupdate/testdata/fuzz/FuzzParseSHA256SUMS/ec5a3a374e24bbcf`,
`docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md`
(amendment A2), this PLAN.

1. **A10** (`checksums.go:11-15,104-115`; `selfupdate_manifest.py:21,69-90`).
   * **Fix:**
     * Go: a new constant `maxChecksumName = maxChecksumLine - 1 -
       sha256HexLen - 2` (4029). `validateChecksumName` refuses a longer
       name: `filename is %d bytes; at most %d fit a SHA256SUMS line`,
       wrapping `ErrIntegrity`;
     * Python: `MAX_CHECKSUM_NAME = MAX_CHECKSUM_LINE - 1 - 64 - 2`, and
       `parse_manifest` refuses a name whose UTF-8 bytes exceed it, after
       the basename check.
   * **Tests:**
     * `checksums_test.go` `TestChecksumNameFitsTheLine`. A 4029-byte name
       after one space is accepted, and so is its canonical line. A
       4030-byte name after one space (a 4095-byte line) is
       `ErrIntegrity`, and so is `validateChecksumName` on it;
     * `manifest_differential_test.go`: the generated cases gain both
       boundary manifests, so Go and Python must agree on each. The result
       count check uses the number of cases written;
     * the fuzz input is committed as `FuzzParseSHA256SUMS`'s regression
       seed, and runs in every `go test`.
   * **Red:** `TestChecksumNameFitsTheLine`: the 4030-byte name is
     accepted; the seed: `round trip rejected: … token too long`.
   * **Plants:**
     * the Go check off: `TestChecksumNameFitsTheLine` and the seed fail;
     * the Python check off: `TestManifestDifferential` fails, with Go
       refusing and Python accepting.
2. **Run:** `SELFUPDATE_REQUIRE_PYTHON=1 go test -count=1 -run
   'TestChecksumNameFitsTheLine|TestManifestDifferential|FuzzParseSHA256SUMS|TestParseSHA256SUMS|TestChecksumNameRefusesColon'
   ./selfupdate`.
3. **Checks:** the session gate (R1's `make gate` is not yet committed, but
   its uncommitted copy in the tree is the same program); `make
   pre-add-check` on the Go files; `make apicheck` against `v1.10.0`;
   `scripts/verify-selfupdate-release_test.sh`, which runs the Python
   parser.

### Phase R1: the gate in the repository (G2)

**Files:** `scripts/gate.sh`, `scripts/gate_test.sh`,
`scripts/check-docs.sh`, `scripts/check-docs_test.sh`,
`scripts/plant-copy.sh`, `scripts/plant-copy_test.sh`, `Makefile`,
`.github/workflows/ci.yml`, `AGENTS.md`, `docs/architecture.md`,
`docs/README.md`, `docs/decisions/0010-PLAN-v1-5-1-contract-preserving-fixes.md`,
`docs/decisions/0010-PLAN-v1-6-0-owner-contracts.md`,
`docs/decisions/0011-PLAN-reference-service-lifecycles.md`,
`docs/decisions/0012-PLAN-archive-assets-and-macos-codesign.md`,
`docs/decisions/0013-PLAN-build-and-stage-release-workflow.md`,
`docs/decisions/0014-PLAN-shared-installer-templates.md`, this PLAN.

1. **`scripts/gate.sh`** (bash, `set -uo pipefail`). It is the session gate,
   with these changes:
   * `ROOT=$(cd -- "$(dirname -- "$0")/.." && pwd)`; `cd "$ROOT"`. No
     `$HOME` path.
   * The output directory is `GATE_OUT`, or a `mktemp -d` whose path is the
     first line printed.
   * Each step runs as `step NAME CMD…`, which:
     * writes the step's output to `$GATE_OUT/NAME.txt`;
     * captures `rc=$?` on the next line;
     * prints `NAME rc=N <last line, 110 characters>`;
     * sets `overall=1` on a non-zero rc.
   * The steps, in order:

     | Step | Command |
     | :--- | :--- |
     | `gofmt` | `gofmt -l .`, which must print nothing |
     | `lint` | `make lint` |
     | `vet` | `go vet ./...` |
     | `race` | `go test -race -count=1 ./...` |
     | `shuffle` | `go test -shuffle=on -count=2 ./...` |
     | `tidy` | `go mod tidy -diff` |
     | `apicheck` | `make apicheck` |
     | `fuzz` | `make fuzz FUZZTIME=${FUZZTIME:-20s}` |
     | `vuln` | `make vuln` |
     | `scripts` | each `scripts/*_test.sh` except `gate_test.sh`, under bash; it names the first that fails |
     | `shellcheck` | `shellcheck scripts/*.sh` |
     | `crossvet` | `CGO_ENABLED=0 go vet ./...` for `freebsd/amd64`, `openbsd/amd64`, `linux/386` and `windows/amd64` |
     | `links` | `scripts/check-docs.sh --links $(git ls-files '*.md')` |
     | `ids` | `scripts/check-docs.sh --ids` on the changed files: `git diff --name-only --diff-filter=d HEAD` plus `git ls-files -o --exclude-standard` |

   * The `gofmt`, `scripts` and `crossvet` steps are shell functions, not
     `sh -c '…'` strings, which avoids shellcheck SC2016.
   * A missing tool (`go`, `make`, `gofmt`, `shellcheck`, `python3`) fails
     its step, naming the tool.
   * It ends with `overall=0|1`, and exits with that value.
   * `GATE_SKIP=fuzz,vuln` skips the named steps and prints `NAME
     skipped`. Records must name any skip.
2. **`scripts/check-docs.sh`** (bash wrapping an embedded `python3 -I -`
   program, as `check-workflows.sh` does). Exit 0 when clean, 1 on a
   finding, 2 on a usage error.
   * `--links FILE…`:
     * every relative Markdown link outside code spans and fences must
       name an existing file;
     * a `#fragment` into a `.md` file must match a heading's slug:
       lowercase, keep `[a-z0-9 _-]`, spaces become `-`;
     * a link that starts with a scheme (`[a-z]+:`) is skipped.
   * `--ids FILE…`:
     * the deny list is `DISCLOSURE_DENY`, else
       `~/.config/git/disclosure-deny`, in the pre-push guard's format:
       `<regex> TAB <placeholder> [TAB identity]`. Only the non-identity
       rules apply;
     * a home directory path is also a finding: the capitalised macOS
       home root or the Linux `home` root, case-sensitive, followed by a
       segment other than `<user>`;
     * a finding prints `FILE:LINE: <placeholder>` (for a home path, `home
       directory path`), never the match;
     * with no readable deny list, it exits 2 ("no deny list") unless
       `--ids-optional` is given; the home-path rule runs either way.
   * The 0009-only `--markers` mode is not carried over.
3. **`scripts/plant-copy.sh FILE OLD NEW`** (embedded Python):
   * the root is `git rev-parse --show-toplevel`;
   * it copies `git ls-files -co --exclude-standard` into a `mktemp -d`,
     without `.git`;
   * OLD and NEW are Python string literals, read with `ast.literal_eval`;
   * FILE must hold OLD exactly once; otherwise it exits 1 and leaves no
     copy;
   * it prints the copy's path, then `rm -rf <path>` as a hint;
   * its header says the copy has no `.git`, so `make apicheck` cannot run
     there.
4. **Tests,** each `#!/bin/sh` with `set -eu`, `SCRIPT=${SCRIPT:-…}`, a
   `mktemp -d` work area and stubs first on `PATH`, as
   `scripts/go-precheck_test.sh` does.
   * **`scripts/gate_test.sh`:**
     * Setup: a scratch git repository holding a copy of `gate.sh` under
       `scripts/`, and a `Makefile`, plus stubs for `go`, `make`, `gofmt`,
       `shellcheck` and `python3` that log their arguments and exit with
       `STUB_RC_<name>`.
     * Cases: all green gives `overall=0` and exit 0; `make lint` exiting
       1 gives `lint rc=1` and exit 1; `gofmt -l` printing a file gives
       `gofmt rc=1`; a failing `scripts/x_test.sh` gives `scripts rc=1`,
       naming `x_test.sh`; without `shellcheck` on `PATH`, `shellcheck
       rc=1` naming it; `GATE_SKIP=fuzz` gives `fuzz skipped`.
     * Plants: `[ "$rc" -eq 0 ] || overall=1` becomes `true` (the lint case
       fails); the `ROOT=` line becomes `ROOT=$HOME` (every case fails).
   * **`scripts/check-docs_test.sh`:**
     * Fixtures, composed at run time from parts, so that the test file
       itself passes `--ids`:
       * a good link, a link to a missing file, a missing anchor, a broken
         link inside a code span (ignored), and an `https:` link (ignored);
       * a `DISCLOSURE_DENY` file with the rule `exampleuser TAB <user>`, a
         file containing `exampleuser`, and an identity-only rule naming
         `idonly` (ignored);
       * a home path under `<user>` (passes), under `bob` (fails), and a
         lowercase `users/bob` inside an `https:` URL (passes).
     * The output never contains `exampleuser` or `bob`.
     * No deny list: exit 2; with `--ids-optional`, exit 0 on a clean file.
     * Plants: drop `.lower()` from the slug (the anchor case fails); make
       the home-path search case-insensitive (the URL case fails); print
       the match (the no-leak assertion fails).
   * **`scripts/plant-copy_test.sh`:**
     * Setup: a scratch git repository with a tracked, an untracked and an
       ignored file.
     * Assertions: the copy has the first two and no `.git`; the plant
       lands once; an OLD found twice exits 1 and leaves no copy; the
       source is byte-identical after.
     * Plant: drop the count check (the twice case fails).
5. **`Makefile`:**
   * `gate` is added to `.PHONY`;
   * a new target: `gate: ## Runs the full pre-commit gate
     (scripts/gate.sh)`, with the recipe `@./scripts/gate.sh`;
   * a comment cites this PLAN, R1.
6. **`ci.yml`:** a step after "Verify the pre-add gate", with this comment
   and body:

   ```yaml
      # The gate's own tests and the document checker
      # (docs/decisions/0015-PLAN-remediate-third-debugging-pass-findings.md R1).
      - name: Verify the gate and document tools
        if: runner.os == 'Linux'
        shell: bash
        run: |
          ./scripts/gate_test.sh
          ./scripts/check-docs_test.sh
          ./scripts/plant-copy_test.sh
          git ls-files -z '*.md' | xargs -0 ./scripts/check-docs.sh --links
   ```

   The existing `shellcheck scripts/*.sh` covers the new scripts.
7. **Docs:**
   * `AGENTS.md` "Pre-add checks" gains a paragraph: `make gate` runs every
     check a commit needs (`scripts/gate.sh`). It also names `GATE_OUT`,
     `GATE_SKIP` and the deny list `--ids` reads.
   * `docs/architecture.md`, Tree: the six new scripts, each with one line.
   * `docs/architecture.md`, Tooling: `make gate`, and the CI step.
   * `docs/README.md`, "I want to…": a new row, "run every check before a
     commit" → `make gate`, linking `../AGENTS.md`.
8. **Notes in the earlier PLANs.** Their execution records are unchanged.
   One dated line goes at the end of the "Rules for every phase" section of
   each of `0010-PLAN-v1-5-1-…`, `0010-PLAN-v1-6-0-…`, `0012-PLAN-…`,
   `0013-PLAN-…` and `0014-PLAN-…`, and after the opening rules of
   `0011-PLAN-…` (`:53`):

   > *(2026-MM-DD)* The session tools these records cite are now in the
   > repository: `gate.sh` as `scripts/gate.sh` (`make gate`), `doccheck.py`
   > as `scripts/check-docs.sh`, `plantcopy.py` as `scripts/plant-copy.sh`
   > ([0015-PLAN](0015-PLAN-remediate-third-debugging-pass-findings.md) R1).

   In 0013-PLAN the line adds: "`simulate_build.sh` is not: CI's
   `release-rehearsal` jobs, which call the build workflow by its local
   path, are its reproducible equivalent."
9. **Red:** the three tests fail on HEAD, because the scripts do not exist.
   **Run:** `sh scripts/gate_test.sh && sh scripts/check-docs_test.sh &&
   sh scripts/plant-copy_test.sh`.
10. **Acceptance:**
    * `make gate` on a clean checkout ends `overall=0`;
    * each test's plants fail it;
    * `--links` over every tracked Markdown file finds nothing (at
      `d6a570f` the session checker checked 305 links, with no failure);
    * CI runs the new step green;
    * from here on, the phase procedure uses `make gate` and
      `scripts/plant-copy.sh`.

### Phase R2: roadmap and record amendments (G1, G3, G10, N4, N5)

**Files:** `docs/decisions/0004-MADR-evolve-selfupdate-api-and-tui-support.md`,
`docs/decisions/0010-MADR-remediate-second-debugging-pass-findings.md`,
`docs/decisions/0011-MADR-reference-service-lifecycles.md`,
`docs/decisions/0011-PLAN-reference-service-lifecycles.md`, `docs/README.md`,
this PLAN.

1. **0004-MADR, `### P3 (date): Phase 4 is built; the open work in one
   place`**, after P2 (`:1071`). It uses P2's shape: a `*Status:*` line,
   then **Found.** and **Decided.**
   * **Phase 4 against what shipped:** `selfupdate/service` (0011,
     `v1.7.0`); `selfupdate/archive` and `selfupdate/codesign` (0012,
     `v1.8.0`); the build-and-stage workflow and `releasespec` (0013,
     `v1.9.0`); the installer templates (0014, `v1.10.0`).
   * **P2's bullet** "The rest of Phase 4 stays planned, and none of it is
     built" (`:1091-1094`) is superseded. P2's text is not edited.
   * **§1's target-shape table** (`:251-264`) is corrected here, without
     editing §1:
     * `cli` imports `x/term`, `selfupdate` and `buildinfo`;
     * `archive` is an `Unpacker` in the core extract stage, for tar.gz,
       zip and gz;
     * `codesign` is `NewSigner` and `NewChecker`, running
       `/usr/bin/codesign` through `service.Runner`;
     * `service` has its own record (0011);
     * new rows: `releasespec`, and the internal release tool.
   * **One table of open work,** each row with its record and line:
     * `verify/signednote`, `verify/ghattest`, `gitlab` and `httpmanifest`,
       and §6's record on mutable releases;
     * `go-tui-lib/updatetea`;
     * 0011's "replace before stop", and a D-Bus systemd backend;
     * 0012 §8's other formats, several programs per archive, app bundles,
       notarization, and pure-Go signature checks;
     * 0013 §10's macOS signing in CI, a build-side attestation,
       GoReleaser names, files beside the program, other hosts, and a
       combined workflow;
     * 0014 §7's package managers, installer signing, system-wide
       installs, completion and service setup;
     * moving each program onto the workflow and the installers;
     * this PLAN's out-of-scope items: N4, N5, `:` in entry names, and
       `queue: max`.

     The items 0012 and 0013 list as open but that later releases built
     are marked done, each with the release that built it.
2. **`docs/README.md`:** the "see where `selfupdate` is going" row links to
   P3's anchor, `#p3-<date>-phase-4-is-built-the-open-work-in-one-place`,
   and its text gains ", and what is still open".
3. **0011-MADR `### A6 (date): the rollback is reported in the result`**
   (G3), after A5 (`:730`):
   * the Confirmation item (`:555-558`) is restated for `Result.RolledBack`
     and the document's `rolled_back` (Q1);
   * the live handoff test that Q1 adds is named.

   **0011-PLAN, `### Deviation D8 (date)`**, after D7 (`:955-978`): V4
   dropped the item with no entry; 0015-PLAN Q1 restores it.
4. **0010-MADR** (G10): an annotation after `:187`, in the record's style,
   says that every `Example` has an output check except three, which
   exit the process or depend on the OS: `ExampleHandOff`,
   `ExampleNewSigner` and `ExampleNewChecker`. `ExampleNewUnpacker` gains
   its check in P4.
5. **Acceptance:** `check-docs.sh --links` on the four records and the
   index finds nothing; the new anchors resolve.

### Phase R3: documentation (C5, G8, G9, F3, F4)

**Files:** `docs/guides/migrating-from-mcplib-selfupdate.md`,
`docs/guides/extending-selfupdate.md`, `docs/guides/building-releases.md`,
`README.md`, `docs/architecture.md`, `docs/README.md`, `selfupdate/doc.go`,
`selfupdate/types.go`, `selfupdate/terminal_event_test.go`, this PLAN.

1. **C5:**
   * The docs say one thing: "An up-to-date run, and a check, end at
     `selected`; the result's `Operation` is the outcome, and no `complete`
     follows." It goes in:
     * `doc.go:75-77`, after "A run has one terminal event.";
     * `types.go:588` (`EventSelected`) and `:600-602` (`EventComplete`);
     * the extending guide, "Read JSON output";
     * `docs/architecture.md`'s events paragraph.
   * **Test, pin:** `terminal_event_test.go` `TestRunEndsAtSelected`
     (`newContractEnv`, `recReporter`). Three runs each end at
     `EventSelected`, with no `complete`, `failed` or `declined`: a check
     that finds an update, an up-to-date check, and an up-to-date apply.
   * **Plant:** emit `EventComplete` for `OperationNone` before the return
     at `updater.go:194-196`.
2. **G8:**
   * Migration guide `:530-531`, "This repository's release workflow
     still publishes bare binaries only." becomes: "Since `v1.9.0` this
     repository's workflows build and publish archives too, from a spec
     whose platforms set `format`", linked to the building guide's
     `#7-archives`.
   * Extending guide `:116-117`: "six whole runs" becomes "seven whole
     runs".
   * `docs/README.md`, the crasher row: "copy its file into
     `<package>/testdata/fuzz/<Name>/` of the package that owns the target
     (`selfupdate`, `selfupdate/archive` or `selfupdate/releasespec`)".
3. **G9, `docs/architecture.md`:**
   * Tree: `release-latest-flag.sh` ("a stable tag below the current latest
     is published `--latest=false`") and its test.
   * The check-cache paragraph (`:155-157`): "the cache file is schema 3,
     which keys the channel and holds the cached outcome; a record of an
     older schema reads as a miss".
   * Build workflow step 4: `GOWORK=off`.
   * Release workflow: the backport latest-flag rule.
   * Tooling, the CI list:
     * Linux: "ownership as root", the systemd live tests,
       `release-latest-flag_test.sh` and `go-precheck_test.sh`;
     * macOS: the launchd and codesign live tests;
     * Windows: the SCM live test.

     "the gate's own test" (`:415`) becomes "the API gate's test".
4. **F3, docs:**
   * Building guide step 12, after its first paragraph: "Your program's
     module must require `v1.10.0` or later, the version the workflow is
     pinned to: an older `releasespec.Parse` refuses the spec's `installer`
     field, and the shipped program cannot update itself. Move `go.mod` and
     both pins together."
   * Migration guide §10, the "Move both" bullet, gains: "Until `v1.11.0`,
     the build workflow does not check this."
5. **F4, docs:**
   * **Building guide step 4** gains a **Before the first release**
     paragraph:
     * turn on immutable releases: Settings → General → Releases →
       "Enable release immutability", or the organization's release
       policy. It applies only to releases published after it is on;
     * attestations need a public repository, or GitHub Enterprise Cloud.
   * **`README.md`:**
     * the publish bullet (`:94-98`) names the same prerequisite;
     * "When a publish fails", the last bullet: "Its tag can never be
       reused" becomes "With immutable releases on, its tag can never be
       reused";
     * a new bullet: **Timed out waiting for immutability with the setting
       off.** The release is live and mutable: delete it (`gh release
       delete <tag> --repo <owner>/<repo> --yes`), turn the setting on, and
       re-run all jobs.
6. **Run:** `go test -count=1 -run '^TestRunEndsAtSelected$' ./selfupdate`.
7. **Acceptance:** markdownlint and `check-docs.sh` on the changed
   Markdown; `make pre-add-check FILES="selfupdate/doc.go selfupdate/types.go
   selfupdate/terminal_event_test.go"`.

### Phase P1: the High findings and what they rest on (B7, B4, B1, B3)

**Files:**

* Core: `selfupdate/session.go`, `selfupdate/leftovers.go`,
  `selfupdate/managed.go`, `selfupdate/updater.go`, `selfupdate/replace.go`,
  `selfupdate/replace_unix.go`, `selfupdate/replace_windows.go`,
  `selfupdate/types.go`, `selfupdate/doc.go`.
* Core tests: `selfupdate/twophase_test.go`,
  `selfupdate/managed_regression_test.go`,
  `selfupdate/install_hardening_test.go`, `selfupdate/recovery_test.go`,
  `selfupdate/leftovers_test.go`, `selfupdate/modepolicy_test.go`,
  `selfupdate/managed_test.go`.
* launchd: `service/launchd/lifecycle.go`, the new
  `service/launchd/plist.go`, `service/launchd/job.go`,
  `service/launchd/fake_test.go`, `service/launchd/launchd_test.go`, the
  new `service/launchd/plutil_darwin_test.go`,
  `service/launchd/live_darwin_test.go`.
* systemd: `service/systemd/lifecycle.go`, the new
  `service/systemd/timespan.go`, `service/systemd/unit.go`,
  `service/systemd/systemd_test.go`.
* SCM: `service/scm/lifecycle.go`, `service/scm/service.go`,
  `service/scm/scm_test.go`.
* Docs: `docs/guides/extending-selfupdate.md`, `docs/architecture.md`,
  this PLAN.

The steps land in the order B7, B4, B1, B3: each rests on the one before.

1. **B7: a second `Commit` or `Rollback` is refused** (`session.go:106-118`,
   `:185-209`, `:279-327`).
   * **Fix:**
     * New unexported `type replacement struct { sess *installSession;
       applied applyResult; finished bool }`.
     * `Apply` returns `State: &replacement{sess: s, applied: applied}` on
       every path. On the paths that return an error with no live backup
       (`:200` and the probe-rolled-back case at `:203-205`),
       `finished` is true.
     * `appliedState(a)` becomes `(s *installSession) stateOf(a
       AppliedReplacement) (*replacement, error)`. It refuses a non-pointer
       State, a nil one, or `st.sess != s`, each with `errForeignReplacement`.
     * New `errReplacementFinished = errors.New("selfupdate: the
       replacement was already committed or rolled back")`, checked under
       `s.mu`, after the closed check, in `Commit` and `Rollback`.
     * `Commit` sets `finished` once `commitLocked` has run, whatever it
       returned. `Rollback` sets it on success and on `errRestoredUnsynced`.
   * **Test:** `twophase_test.go` `TestSecondCommitOrRollbackRefused`
     (`standaloneSession`, `stageNew`). Four subtests: rollback then
     commit; commit then rollback; commit twice; rollback twice. The second
     call returns an error matching `errReplacementFinished`; no
     `Applied=true` follows a rollback; the target holds what the first
     call left. A foreign state (`AppliedReplacement{State: 1}`) is
     `errForeignReplacement`.
   * **Red:** `commit after rollback: target="old-bytes" Applied=true err=<nil>`.
   * **Plant:** delete the `finished` check in `Commit`.
   * **Docs:** `types.go:496-506` says the State is single-use: a second
     `Commit` or `Rollback` is refused.
2. **B4: recovery reports a restored binary** (`managed.go:170-186`,
   `session.go:127-141,195-199,329-337`, `replace_unix.go:69-79`,
   `replace_windows.go:53-63`).
   * **Fix:**
     * `errProbeRolledBack` is renamed `errRolledBack`, with `gofmt -r
       'errProbeRolledBack -> errRolledBack' -w selfupdate/*.go`. Its
       comment becomes "marks a failure whose replacement was undone: the
       previous binary is in place".
     * `rollbackInRoot` returns `errors.Join(errRestoredUnsynced, err)` when
       the rename succeeded and `syncRoot` failed. It calls a new seam,
       `syncRootFn = syncRoot`, in `replace.go`.
     * `Install` (`:133-141`): on `errRestoredUnsynced` from
       `rollbackInRoot`, it returns `RolledBack: true` and no backup.
     * `Apply` (`:195-199`): a successful `rollbackInRoot` returns
       `errors.Join(derr, errRolledBack)`; an unsynced one returns
       `errors.Join(derr, rerr, errRolledBack)`, with no backup.
     * Both `replaceTarget`s: the sync-failure path whose restore succeeded
       returns `errors.Join(syncErr, errRolledBack)`.
     * `Install`'s first branch (`:127-131`): an error wrapping
       `errRolledBack` returns `RolledBack: true`.
     * `recover` (`managed.go:170-186`): a `Rollback` error wrapping
       `errRestoredUnsynced` counts as rolled back (`RolledBack: true`, no
       backup), and its error is still joined.
   * **Tests:**
     * `managed_regression_test.go` `TestManagedRecoveryReportsRestoredBinary`
       (`managedEnv`, `fakeLife{installed, running, healthErr}`). Two
       subtests:
       * `unsynced rollback`: `setSeam(&syncDirFn, …)` fails its second
         call;
       * `undone apply`: `swapAfterFirstReplace` swaps the directory.

       Each asserts `RolledBack`, an empty `Backup`, and the target
       `"old-bytes"`.
     * `install_hardening_test.go`
       `TestInstallReportsRestoreAfterSyncFailure`: through `syncRootFn`
       and `syncDirFn`, the same assertions for `Install`.
   * **Red:** `RolledBack=false Backup=""`, and for `rollbackInRoot` a
     `Backup` naming a file that no longer exists.
   * **Plants:** drop the `errRestoredUnsynced` branch in `recover`; join
     `derr` alone in `Apply`.
3. **B1: a kept backup survives** (`leftovers.go:8-79`, `session.go`,
   `managed.go:172-177`, `updater.go:219,374-391`, `doc.go:15-17`).
   * **Fix:**
     * **The kept name.** New `keptName(base, backupName string) (string,
       bool)` maps `.<base>.selfupdate-bak-<digits>` to
       `.<base>.selfupdate-kept-<digits>`. `isLeftover` already returns
       false for the kept name, and it does not change.
     * **Retaining.** New `(s *installSession) retainLocked(backup string)
       string`:
       * an existing file at the kept name means no rename;
       * otherwise `s.root.Rename(old, kept)`, then
         `advisory(syncRootFn(s.root))`;
       * it returns the kept path joined to `s.target.Dir`, or `backup`
         when the rename failed or did not run.
     * **Every path that returns a live backup with `Applied` false**
       retains it, and reports the new path:
       * `Install` at `:130`, `:139` and `:147`;
       * `Apply` at `:197`, and at `:205` when `err != nil` with a backup;
         the state's `applied.backup` is updated too;
       * `Rollback`, when it fails with the backup still present.
     * **Managed recovery.** The backup name is read after `Rollback`
       through a new `backupOf(a AppliedReplacement) string`, which returns
       the state's `applied.backup` when State is a `*replacement`, else
       `a.Backup`. `recover` then checks it with `os.Lstat`.
     * **A dry run sweeps nothing:**
       * a new unexported context key `dryRunKey{}`, with
         `withDryRun(ctx)` and `isDryRun(ctx)`;
       * `updater.go:219` calls `Begin(withDryRun(ctx), target)` when
         `req.DryRun`;
       * `beginSession` skips `processCleanupReceipt` and `removeLeftovers`
         when `isDryRun(ctx)`.
     * `Result.PendingBackup` and the "kept at" message already use
       `installed.Backup` (`updater.go:374-381`), so they name the kept
       file.
   * **Tests:**
     * `recovery_test.go` `TestKeptBackupSurvivesLaterSessions`
       (`withTempHome`, `standaloneSession`, `stageNew`, `syncDirFn` and
       `replacePath` seams as in the MADR's probe). Subtests:
       * `CleanupPending`: a new `StandaloneInstaller.CleanupPending`;
       * `Run`: a later `Begin` and `Close`;
       * `DryRun`: a later `Begin` under `withDryRun`;
       * `Managed`: `managedEnv` with `fakeLife{healthErr}` and a failing
         restore.

       Each asserts that the reported backup's base name starts with
       `.demo.selfupdate-kept-`, and that the file still holds
       `"old-bytes"` after the later session.
     * `leftovers_test.go` `TestDryRunSweepsNothing` (`plantLeftovers`,
       then `checkLeftovers` expecting every file kept).
     * `modepolicy_test.go` `TestIsLeftover`: new rows
       `.demo.selfupdate-kept-42` and, on Windows,
       `.demo.selfupdate-kept-42.exe` are false. Pin.
   * **Red:** `backup .demo.selfupdate-bak-<n>: no such file or directory`
     after `CleanupPending`.
   * **Plants:** `retainLocked` returns its argument (the `CleanupPending`
     and `Run` subtests fail); `isDryRun` returns false (only
     `TestDryRunSweepsNothing` and `DryRun` fail).
   * **Docs:**
     * `types.go:156-163` (`PendingBackup`) and `:451-452` (`Backup`): the
       file is `.<base>.selfupdate-kept-<n>`, and no later session removes
       it;
     * `doc.go:15-17`: a dry run sweeps nothing;
     * `leftovers.go:8-13`: kept backups are not leftovers;
     * `docs/architecture.md`, the leftovers paragraph;
     * the extending guide, "Keep the previous binary".
4. **B3: a failed stop restarts the service** (`managed.go:104-108`; N8).
   * **Fix, core:**
     * New `(s *managedSession) recoverStop(ctx, product, stopErr)`, called
       for every `Stop` error at `managed.go:104-108`. It asks
       `s.life.Running` again under
       `context.WithTimeout(context.WithoutCancel(ctx), recoveryTimeout)`.
     * When the service is not running: `return s.recover(ctx, product,
       AppliedReplacement{}, ReconcileResult{}, true, false,
       fmt.Errorf("selfupdate: stop service: %w", stopErr))`, which runs
       `Start` and `WaitHealthy` and joins every error.
     * When it is still running, or the probe fails: today's error, with
       the probe's error joined.
     * There is no special case for `service.ErrInsideService`. The core
       does not import `service` and must not match error text. A refused
       stop from inside the service leaves it running, so the re-probe
       returns today's error unchanged.
   * **Fix, launchd** (`lifecycle.go:103-151`):
     * new `plist.go` with `plistValue(ctx, key) (typ, raw string, present
       bool, err error)`:
       * `plutil -convert xml1 -o - -- <plist>`; a non-zero exit is an
         error, and a missing file (`statFile`) wraps
         `service.ErrNotInstalled`;
       * the first element after `<plist version="1.0">` must be `<dict>`
         or `<dict/>`; otherwise "<plist> is not a property-list
         dictionary";
       * `plutil -type <key> -- <plist>`: exit 0 prints the type; exit 1
         after the convert passed means missing;
       * for `bool` and `integer`, `-extract <key> raw -o - -- <plist>`
         gives `raw`;
     * `Stop` reads `ExitTimeOut` before `bootout`. An `integer` above 0
       gives `exit = n s`; a missing key gives `defaultExitTimeOut` (5 s);
       0, another type, or an error gives no bound of its own;
     * the wait: `bound = max(Poll.Timeout or DefaultPollTimeout, exit +
       stopGrace)`, with `stopGrace = 30 * time.Second`. `stopGrace` and
       `defaultExitTimeOut` are package variables, for the tests;
     * `waitGone` runs under `context.WithTimeout(context.WithoutCancel(ctx),
       bound)`, with `poll.Timeout = bound`, and returns
       `errors.Join(waitErr, ctx.Err())`.
   * **Fix, systemd** (`lifecycle.go:65-89`):
     * new `timespan.go`, `parseTimespan(s string) (time.Duration, bool)`
       for `systemctl show`'s format: space-separated `<int><unit>`, units
       `us`, `ms`, `s`, `min`, `h`, `d`; `infinity`, empty, or anything
       else is not ok;
     * `Stop` reads `u.show(ctx, unit, "TimeoutStopUSec")` before `stop`;
       `bound = max(Poll.Timeout or default, t + 30 s)` when ok, else the
       poll timeout;
     * the `stop` command and `waitState` run under
       `context.WithTimeout(context.WithoutCancel(ctx), bound)`;
       `waitState` takes the bound as its timeout; the return joins
       `ctx.Err()`.
   * **Fix, SCM** (`lifecycle.go:101`): `s.deadline(context.WithoutCancel(ctx))`
     replaces `s.deadline(ctx)`; the return joins `ctx.Err()`.
   * **Tests:**
     * **Core:**
       * `managed_test.go` `TestManagedStopFailsAfterStoppingRestarts`.
         `fakeLife` gains `stopTakesEffect bool`: `Stop` sets `running =
         false` before returning `stopErr`. Asserts `starts == 1`,
         `healths == 1`, an error matching `ErrManagedInstall` and the
         injected error, and the target `"old-bytes"`.
       * `TestManagedStopFailsStillRunning`: without `stopTakesEffect`,
         `starts == 0` (the control).
     * **launchd:**
       * the fake (`fake_test.go`) gains `-convert` (a `<dict>` root
         unless `f.corrupt`), `-type` (`f.types[key]`, else `bool` for
         `true`/`false`, `integer` for digits, `dictionary` otherwise), and
         `printDeadlines []time.Duration`, recorded from `ctx.Deadline()`
         on each `print`;
       * `launchd_test.go` `TestStopBound`, a table: no key (Poll 60 s) →
         60 s; `ExitTimeOut 90` → 120 s; `ExitTimeOut 0` → 60 s; `Poll 1 s`
         and no key → 35 s; a string value → Poll;
       * `TestStopFinishesAfterCancel`: `onBootout` cancels the caller's
         context and `goneAfter = 3`. `Stop` keeps printing until gone,
         and returns `context.Canceled`;
       * `TestManagedStopTimeoutRestartsJob`: through
         `selfupdate.NewManagedInstaller`, with `pid = 99999999`,
         `goneAfter = 1<<30`, `stopGrace = 0`, `defaultExitTimeOut = 0` and
         `Poll.Timeout = 100 ms`. The verbs after `bootout` include
         `bootstrap` and `kickstart`;
       * `TestStopTimesOut` (`:149`) sets `stopGrace = 0` and
         `defaultExitTimeOut = 0`, so that its 100 ms poll still times out:
         a planned edit;
       * `plutil_darwin_test.go` `TestPlistValueRealPlutil`, with the real
         `/usr/bin/plutil` and the corrections' four files: the integer,
         the missing key, the `garbage` file (not a dictionary), and mode
         0000 (skipped as root).
     * **systemd** (`systemd_test.go`):
       * `TestParseTimespan`: `1min 30s` → 90 s; `5s`; `100ms`; `2h`;
         `infinity` → not ok; empty → not ok; `1x` → not ok;
       * `TestStopBound`;
       * `TestStopFinishesAfterCancel`: `onStop` cancels the context, and
         `showQueue` reports `deactivating` twice, then `inactive`.
     * **SCM** (`scm_test.go`): `TestStopFinishesAfterCancel`, where the
       control cancels the context, and the fake's queue stops after two
       ticks.
   * **Red:**
     * core: `stops=1 starts=0 healths=0`;
     * launchd manager test: verbs end `[bootout print print print]`;
     * the cancel tests: `Stop returned … while the job was still loaded`;
     * `TestStopBound` and `TestParseTimespan`: the functions do not exist
       (the build fails), recorded as the red.
   * **Plants:** `true` → `false` for `restart` in `recoverStop`;
     launchd's bound back to `Poll.Timeout`; systemd's `WithoutCancel`
     dropped.
   * **Live:** `launchd/live_darwin_test.go` `TestLiveStopWaitsForSlowExit`
     (`:439`) gains `ExitTimeOut` 20 in its plist, with `Poll.Timeout` 5 s
     and `FAKE_IGNORE_TERM=1`: `Stop` succeeds after launchd's kill.
     Before the fix it times out at 5 s.
   * **Docs:**
     * launchd `lifecycle.go:99-102`, `job.go:96-98` (the `Poll` comment);
     * systemd `unit.go:54-55`; SCM `service.go`'s `Poll` comment: the stop
       wait's bound;
     * the extending guide, "Run as a service": "A stop that fails after
       the service stopped starts it again; the update fails with
       `ErrManagedInstall`."
   * **Test, the inside case:** `TestManagedStopFailsStillRunning` also
     covers a `stopErr` wrapping a sentinel named like the inside error,
     with the service still running: `starts == 0`, and the error is
     today's.
5. **Run:**

   ```bash
   go test -count=1 -run 'TestSecondCommitOrRollbackRefused|TestManagedRecoveryReportsRestoredBinary|TestInstallReportsRestoreAfterSyncFailure|TestKeptBackupSurvivesLaterSessions|TestDryRunSweepsNothing|TestIsLeftover|TestManagedStopFails' ./selfupdate
   go test -count=1 -run 'TestStopBound|TestStopFinishesAfterCancel|TestManagedStopTimeoutRestartsJob|TestStopTimesOut|TestPlistValueRealPlutil|TestParseTimespan' ./selfupdate/service/...
   ```

6. **Acceptance:**
   * every new test was red and is green;
   * every existing test passes;
   * `make apicheck` against `v1.10.0` passes, and the V3 report is empty;
   * the Windows cross-vet passes;
   * the live launchd test passes on this Mac.

### Phase P2: network and the check cache (A1 selectors, A2, A3, A4, A6, A7)

**Files:** `selfupdate/assets.go`, `selfupdate/archive/select.go`,
`selfupdate/github.go`, `selfupdate/errors.go`, `selfupdate/doc.go`,
`selfupdate/checker_test.go`, `selfupdate/checkcache_outcome_test.go`,
`selfupdate/archive/select_test.go`, `selfupdate/credentials_run_test.go`,
`selfupdate/network_hardening_test.go`,
`selfupdate/github_hardening_test.go`, `docs/guides/extending-selfupdate.md`,
this PLAN.

1. **A1: a missing platform asset is cached** (`assets.go:108-110`,
   `archive/select.go:130-156`).
   * **Fix:**
     * `assets.go:109` becomes `fmt.Errorf("selfupdate: release %s has no
       exact asset %q: %w", rel.Tag, want, ErrUnsupportedPlatform)`;
     * `exactlyOne` gains a `missing error` parameter: the archive passes
       `selfupdate.ErrUnsupportedPlatform`; `SHA256SUMS` passes nil, and
       its message is unchanged.
   * **Tests:**
     * `checker_test.go` `checkRows` gains "release lacks the platform's
       asset": a release without `demo-linux-arm64`, `wantErr:
       ErrUnsupportedPlatform`;
     * `checkcache_outcome_test.go:28` counts four outcome rows, the new
       one with one network call over three `CheckCached`;
     * `archive/select_test.go` `TestSelectRefusals`: "no archive" matches
       `ErrUnsupportedPlatform`, and "no manifest" does not.
   * **Red:** `apiRequests=3`, and `errors.Is` false.
   * **Plant:** `%w` → `%v` in `assets.go`.
   * **Docs:** `errors.go:22-24`: a release without the platform's asset
     is unsupported too.
2. **A2: an environment-token fallback is per run** (`github.go:54-62`,
   `:681`, `:724-744`).
   * **Fix:**
     * `credentialState` gains `perRun bool`: true when `resolve`
       returned an environment token after a provider declined;
     * the cache test at `:681` becomes `s.cred.resolved &&
       ((s.cred.has && !s.cred.perRun) || s.cred.anonRun == mark)`;
     * `:708-709` set `perRun` with the rest.
   * **Test:** `credentials_run_test.go`
     `TestPromptAfterStartupCheckWithEnvToken`
     (`t.Setenv("GH_TOKEN", "stale")`, a server requiring `good`,
     `answerCredentials`): a startup `Check`, then `Start`, prompts once,
     and the update applies.
   * **Red:** `prompts=0 … github http 401`.
   * **Plant:** drop `!s.cred.perRun`.
   * **Docs:** the `credentialState` comment; `doc.go:83-87`; the extending
     guide, "Plug in a credential".
3. **A3: a cross-origin hop keeps only fixed headers** (`github.go:224-234`).
   * **Fix:** a package-level `forwardedHeaders` set: `Accept`,
     `Accept-Encoding`, `User-Agent`, `X-Github-Api-Version`, in canonical
     form. On a cross-origin hop, every other header of `req.Header` is
     deleted. The credential-header lookup goes.
   * **Test:** `network_hardening_test.go` `TestRedirectKeepsOnlyFixedHeaders`.
     It calls `checkRedirect` directly, with a request carrying
     `X-Old-Key` while the source's credential header is `X-New-Key`. Only
     the four fixed headers remain.
   * **Red:** `X-Old-Key` present.
   * **Plant:** keep headers named `X-*`.
   * **Docs:** `github.go:647-651`, `doc.go:85-86`.
4. **A4: a secondary limit without headers** (`github.go:789-809`).
   * **Fix:** `rateLimitedForbidden(resp, body)` is also true when the
     lowercased body contains `secondary rate limit`. `mapStatus` passes
     the body it has.
   * **Test:** `github_hardening_test.go`
     `TestGitHubSecondaryRateLimitWithoutHeaders`: a 403 with that body and
     no headers is a `*RateLimitError`, and three `CheckCached` make one
     network call.
   * **Red:** `isRateLimited=false calls=3`.
   * **Plant:** drop the body clause.
5. **A6: asset bytes as served** (`github.go:556-572`).
   * **Fix:** `newRequest` sets `Accept-Encoding: identity` when `accept ==
     gitHubAcceptAsset`.
   * **Test:** `github_hardening_test.go`
     `TestOpenAssetKeepsContentEncodedBytes`. The server always answers
     with `Content-Encoding: gzip` and a gzip body. The bytes arrive
     undecoded, and the server saw `identity`.
   * **Red:** decoded bytes.
   * **Plant:** drop the header. A3's set includes `Accept-Encoding`, so a
     redirect keeps it.
6. **A7: the stored-deferral cap is tested** (`checkcache.go:165-169`).
   * **Fix (test only):** `network_hardening_test.go:60` stores
     `Request: key` from `cacheEnv`, not `req`, so the record matches.
   * **Test, pin:** the existing test.
   * **Plant:** `if matched && rec.NotBefore.After(` → `if false && matched
     && rec.NotBefore.After(`. The test then fails with "check deferred by
     an earlier rate limit".
7. **Run:** `go test -count=1 -run
   'TestCheck|TestCheckCachedOutcomes|TestSelectRefusals|TestPromptAfterStartupCheckWithEnvToken|TestRedirectKeepsOnlyFixedHeaders|TestGitHubSecondaryRateLimitWithoutHeaders|TestOpenAssetKeepsContentEncodedBytes|TestStoredDeferral'
   ./selfupdate ./selfupdate/archive`.
8. **Acceptance:** as P1's API and test items.

### Phase P3: install (B2, B6, B8, B9, B10)

**Files:** `selfupdate/target.go`, `selfupdate/replace_unix.go`,
`selfupdate/replace_windows.go`, `selfupdate/probe.go`,
`selfupdate/leftovers.go`, `selfupdate/imageverify.go`,
`selfupdate/errors.go`, `selfupdate/install_hardening_test.go`,
`selfupdate/probe_test.go`, `selfupdate/modepolicy_test.go`,
`selfupdate/imageverify_test.go`, `selfupdate/target_test.go`,
`selfupdate/managed_test.go`, `docs/architecture.md`, this PLAN.

1. **B2: the locked target is required at the replace**
   (`replace_unix.go:47-51`, `replace_windows.go:31-35`, `target.go:209-220`).
   * **Fix:** new `lockedTarget(target Target) (os.FileInfo, error)`:
     `os.Lstat`; a regular file; `sameTargetIdentity(target, info)`.
     Otherwise `fmt.Errorf("selfupdate: target changed during the update:
     %w", ErrConcurrentUpdate)`. It replaces the `os.Lstat` in both
     `replaceTarget`s.
   * **Test:** `install_hardening_test.go`
     `TestInstallRefusesTargetReplacedAfterBegin`. Subtests:
     * `symlink` (`symlinkOrSkip`): the target becomes a symlink to a
       0777 file after `Begin`;
     * `file`: the target is replaced by another regular file.

     Each gives `ErrConcurrentUpdate` and `!Applied`, and the swapped-in
     file is untouched.
   * **Red:** `applied=true err=<nil>`.
   * **Plants:** drop the identity check (the `file` case fails); drop
     `IsRegular` (the `symlink` case fails).
   * **Docs:** `errors.go:16-18`; `docs/architecture.md`, the target-checks
     paragraph.
2. **B6: a probe ended by the run's context** (`probe.go:92-107`).
   * **Fix:** the parameter becomes `parent`. When `cmd.Run` fails and
     `parent.Err() != nil`, return `fmt.Errorf("selfupdate: %s probe of %s
     stopped: %w", r.Phase, r.Product, parent.Err())`.
   * **Test:** `probe_test.go` `TestVersionProberHonoursRunContext`
     (`SELFUPDATE_TEST_SLEEP=10s`, a 10 s prober timeout). Two subtests:
     a parent deadline of 100 ms (`context.DeadlineExceeded`), and a
     cancel (`context.Canceled`). `failureClass` is `deadline-exceeded`
     and `canceled`.
   * **Red:** `… timed out after 10s isDeadline=false`.
   * **Plant:** drop the `parent.Err()` branch.
   * **Docs:** `probe.go:75-78`.
3. **B8: the receipt's temporary file is a leftover** (`leftovers.go:22-36`,
   `cleanup_windows.go:267`).
   * **Fix:** `isLeftover` also matches
     `.<base>.selfupdate.cleanup-tmp-<digits>`, on every OS (the name is
     harmless off Windows).
   * **Tests:** `modepolicy_test.go` `TestIsLeftover` rows:
     `….cleanup-tmp-12` is true; `….cleanup-tmp-` and `….cleanup-tmp-1x`
     are false. `TestRemoveLeftoversKeepsListed` removes it with
     `sweepBackups` false.
   * **Red:** false.
   * **Plant:** drop the branch.
4. **B9: a PE DLL is refused** (`imageverify.go:139-140`).
   * **Fix:** `&& f.Characteristics&pe.IMAGE_FILE_DLL == 0`.
   * **Test:** `imageverify_test.go` `TestImageVerifier`: the Windows
     fixture, with bit 0x2000 set in the COFF characteristics (at
     `e_lfanew + 4 + 18`), is `ErrIntegrity` for `NewImageVerifier` and
     `CheckImage`.
   * **Red:** `<nil>`.
   * **Plant:** drop the clause.
   * **Docs:** `imageverify.go:30-35,44-49`.
5. **B10: the coverage gaps** (tests only).
   * **Tests, pin:**
     * `target_test.go` `TestResolveTargetDefaultExecutable`
       (`setSeam(&osExecutable, …)`: success, an error, an empty path);
     * `TestRawExecutablePathRelative` (`t.Chdir`);
     * `managed_test.go` `TestManagedSessionTarget`.
   * **Plants:** `osExecutable` not called; `Target` returns `Target{}`;
     `raw = abs` dropped. Each fails its test.
   * **Acceptance item:** `go test -coverprofile` over the module shows
     `rawExecutablePath` at 80 % or more, and `managedSession.Target` at
     100 %.
6. **Run:** `go test -count=1 -run
   'TestInstallRefusesTargetReplacedAfterBegin|TestVersionProberHonoursRunContext|TestIsLeftover|TestRemoveLeftoversKeepsListed|TestImageVerifier|TestResolveTargetDefaultExecutable|TestRawExecutablePathRelative|TestManagedSessionTarget'
   ./selfupdate`.
7. **Acceptance:** as P1's API and test items, plus the coverage item.

### Phase P4: the API and the command surface (C1, C2, C4, C6, C7, A9, G5, G6, G10)

**Files:** `selfupdate/cli/run.go`, `selfupdate/cli/command.go`,
`selfupdate/cli/doc.go`, `selfupdate/checker.go`, `selfupdate/updater.go`,
`selfupdate/runoptions.go`, `selfupdate/reporter.go`,
`selfupdate/jsonreporter.go`, `selfupdate/confirmer.go`,
`selfupdate/types.go`, `selfupdate/github.go`, `selfupdate/doc.go`,
`selfupdate/releasespec/doc.go`, `selfupdate/cli/run_test.go`,
`selfupdate/cli/command_test.go`, `selfupdate/checker_test.go`,
`selfupdate/updater_contract_test.go`, `selfupdate/events_test.go`,
`selfupdate/github_test.go`, `selfupdate/archive/example_test.go`,
`selfupdate/cli/testdata/golden/failed.json.stdout`,
`selfupdate/cli/testdata/golden/contradiction.json.stdout`, this PLAN.

1. **C1: the result object carries the run's exit** (`cli/run.go:142-157`).
   * **Fix:** after `RunWith`:
     1. `late := o.warn(res)`;
     2. when `o.HandOff.Report != nil`, `late = errors.Join(late,
        o.HandOff.Report(res, joinLate(err, late)))`;
     3. `err = joinLate(err, late)`;
     4. `return res, joinLate(err, o.finish(res, err))`.

     `joinLate(err, late)` keeps 0010 C3: under `ErrUpdateAvailable`, the
     late error alone decides; otherwise both are joined; with no late
     error, `err`.
   * **Test:** `cli/run_test.go` `TestResultObjectExitCodeMatchesExit`, over
     three runs:
     * a failing `Report`;
     * a failing warning write (`buildUpdaterClosing` with "unlock failed",
       stderr `failOn("warning:")`);
     * every `scenarios` entry.

     For each, the last stdout line's `exit_code` equals `Exit`'s code, and
     `error` is set when it is non-zero.
   * **Red:** `exit_code:0`, exit 1.
   * **Plant:** call `finish` before `Report`.
   * **Docs:** `cli/doc.go:14-18`; `cli/run.go:62-64`.
2. **C2: an early failure reaches `Report`** (`cli/command.go:63-71`).
   * **Fix:** `Options.report` first calls `o.HandOff.Report(res, err)`
     when it is set, and joins its error. Help is not reported.
   * **Test:** `cli/command_test.go` `TestCommandEarlyFailureReachesReport`.
     The early failures: `newUpdater` failing; `newUpdater` nil; a
     positional argument; `--check --yes`. Each calls `Report` once with
     an error, and exits 1. Under `--json`, the object's `error` includes a
     failing `Report`'s.
   * **Red:** `Report calls=0`.
   * **Plant:** drop the call.
   * **Docs:** `cli/run.go:62-64`; `cli/command.go:19-21`.
3. **C4: `Check` agrees with `Run`** (`checker.go:95-97,134-150`,
   `updater.go:163-167,409-420`, `runoptions.go`).
   * **Fix:**
     * `Checker` gains unexported `unpacker Unpacker` and `fromUpdater
       bool`, set by `Updater.Checker()` and `run.checker()`;
     * `matchUnpacker` becomes a `Checker` method, run at the end of
       `discover` when `fromUpdater`;
     * the separate call in `Run` (`updater.go:163-167`) goes.
   * **Test:** `checker_test.go` `TestCheckerAgreesWithRunCheck` gains two
     rows (`newPackEnv`): an archive with `Unpacker` nil, and a bare binary
     with an unpacker. `Checker().Check` and `CheckCached` (`memStore`)
     fail with `Run --check`'s text.
   * **Red:** `available=true`.
   * **Plant:** skip the match in `discover`.
   * **Docs:** `checker.go:10-26,93-94`.
4. **C6: a failed run's result names it** (`updater.go:139-162`).
   * **Fix:** after `validateRequest`, `base := Result{Product:
     req.Product, CurrentVersion: req.CurrentVersion, Checked:
     req.CheckOnly, DryRun: req.DryRun}`. It replaces `Result{}` at `:148`,
     `:152`, `:155`, `:161` and `:166`. A failed `validateRequest` keeps
     `Result{}` when the product is invalid, and `base` otherwise.
   * **Test:** `updater_contract_test.go`
     `TestDiscoveryFailureResultNamesRun`: a discovery failure under
     `--check` returns `Product`, `CurrentVersion` and `Checked`.
   * **Red:** `Product=""`.
   * **Plant:** `Result{}` at `:161`.
   * **Goldens:** `failed.json.stdout` and `contradiction.json.stdout`
     change, through rule 8.
5. **C7: typed-nil writers are refused** (`reporter.go:19-27`,
   `jsonreporter.go:34-39`, `confirmer.go`, `cli/run.go:85-97`,
   `cli/command.go:27`).
   * **Fix:**
     * a shared unexported `isNilWriter(w io.Writer) bool` (`w == nil`, or
       a nil pointer, map, slice, func or chan, by `reflect`);
     * the four constructors (`NewTextReporter`, `NewJSONReporter`,
       `NewTerminalConfirmer`, `NewPromptConfirmer`) set such a writer to
       nil, so their existing "writer is nil" errors fire;
     * `cli` has its own copy, used in `Options.check` and `Command`.
   * **Tests:**
     * `events_test.go` `TestTypedNilWriters`: a `(*bytes.Buffer)(nil)`
       for each constructor;
     * `cli/run_test.go` `TestOptionsRefused` gains "typed-nil stderr" and
       "typed-nil stdout with JSON".
   * **Red:** a panic.
   * **Plant:** drop the `NewJSONReporter` normalisation.
6. **A9, G5, G6, G10: godoc and an example:**
   * **A9:** `types.go:188-189` and `github.go:97-98`: the clone's
     `CheckRedirect` is the source's own (at most 10 hops; HTTPS only, with
     plain http only between loopback hosts; the credential stripped off
     another origin), and the caller's is never called. **Test, pin:**
     `github_test.go` `TestNewGitHubSourceReplacesCheckRedirect`.
     **Plant:** call the caller's function from the source's.
   * **G5:** `types.go:598`: `InstallSession.Install`.
   * **G6:** `doc.go:96-98`: "Config.Verifiers run on the asset as
     published: the program, or the archive when an Unpacker is set; `New`
     refuses NewImageVerifier beside an Unpacker."
     `releasespec/doc.go:1-4` names the generated installers, and 0014
     §2.
   * **G10:** `archive/example_test.go` `ExampleNewUnpacker` selects from a
     release with `relay-linux-amd64.tar.gz` and `SHA256SUMS`, and prints
     `got.Binary.Name, got.Packed, cfg.Unpacker != nil` with `// Output:
     relay-linux-amd64.tar.gz true true`.
7. **Run:** `go test -count=1 -run
   'TestResultObjectExitCodeMatchesExit|TestCommandEarlyFailureReachesReport|TestCheckerAgreesWithRunCheck|TestDiscoveryFailureResultNamesRun|TestTypedNilWriters|TestOptionsRefused|TestNewGitHubSourceReplacesCheckRedirect|ExampleNewUnpacker|TestGolden|TestCommandGolden'
   ./selfupdate/...`.
8. **Acceptance:** as P1's API and test items; the golden diff read and
   listed.

### Phase P5: the service backends (D2, D3, D4, D5, D7, D8, D11)

**Files:** `service/systemd/lifecycle.go`, `service/systemd/reconcile.go`,
`service/systemd/detach.go`, `service/systemd/unit.go`,
`service/systemd/fake_test.go`, `service/systemd/systemd_test.go`,
`service/systemd/reconcile_test.go`, `service/systemd/detach_unix_test.go`,
`service/systemd/live_linux_test.go`, `service/launchd/lifecycle.go`,
`service/launchd/reconcile.go`, `service/launchd/job.go`,
`service/launchd/fake_test.go`, `service/launchd/launchd_test.go`,
`service/launchd/plutil_darwin_test.go`,
`service/launchd/live_darwin_test.go`, `service/scm/reconcile.go`,
`service/scm/service.go`, `service/scm/fake_test.go`,
`service/scm/scm_test.go`, `service/scm/compose_windows_test.go`,
`service/scm/live_windows_test.go`, `docs/guides/extending-selfupdate.md`,
`docs/architecture.md`, this PLAN.

1. **D2: the restart baseline is read after the start**
   (`systemd/lifecycle.go:94-114`).
   * **Fix:** `Start` reads `InvocationID` before `systemctl start`, and
     `NRestarts` in a second `show` after it exits 0. Both go into the
     baseline then.
   * **Test:** `systemd_test.go` `TestStartBaselineIsAfterStart`.
     `showQueue` gives `NRestarts` 3 before the start and 0 after, with a
     new `InvocationID`. `WaitHealthy` returns nil.
   * **Red:** `not healthy: … NRestarts=0`.
   * **Edit:** `TestWaitHealthyFailsFast` "restarted" (`:235`) queues its
     after-start `show`.
   * **Plant:** read both before the start.
   * **Docs:** `lifecycle.go:91-93`, `:116-119`.
   * **Live:** `live_linux_test.go` `TestLiveUpdateAfterAutoRestart`. The
     unit has `Restart=always`; its process is killed once
     (`systemctl kill --signal=KILL`), and `NRestarts` reaches 1; then a
     managed update succeeds.
2. **D3: a loaded, idle launchd job is reloaded** (`launchd/reconcile.go:72-125`,
   `lifecycle.go:154-191`).
   * **Fix:**
     * `rewrite`, when the job is loaded and running: refuse ("stop the job
       first").
     * When it is loaded and not running:
       1. check `Inside`;
       2. write the plist;
       3. `bootout`, and `waitGone` with P1's bound;
       4. `bootstrap`, without `kickstart`;
       5. set an unexported `reloaded` flag on `Job`.
     * When it is not loaded: unchanged.
     * `Restore` mirrors the same steps.
     * `Start` does not take the reload's process as `previous` while
       `reloaded` is set, and clears the flag.
   * **Test:** `launchd_test.go` `TestReconcileReloadsLoadedJob` (`pid=0`,
     `loaded=true`, `RewritePath`):
     * the verbs hold `bootout` before `bootstrap`;
     * `Start` `kickstart`s;
     * `WaitHealthy` passes;
     * `Restore` reloads too.
   * **Edit:** `TestReconcileRewriteAndRestore` uses `pid=0`.
   * **Red:** verbs `[… list enable kickstart]`.
   * **Plant:** drop the reload.
   * **Docs:** `reconcile.go:29-34`; `lifecycle.go:151-153`; the
     `RewritePath` comment (`job.go:100`); the extending guide, "Run as a
     service"; `docs/architecture.md`, the launchd paragraph.
   * **Live:** `TestLiveRewriteLoadedIdleJob`: a `RunAtLoad` job whose
     program exits 0; the plist rewritten to a moved binary; `Start`
     runs the new path (`launchctl print` shows `program = <new>`).
3. **D4: the effective `ExecStart` is verified** (`systemd/reconcile.go:74-105`).
   * **Fix:** after `reload`, `u.show(ctx, unit, "ExecStart")`, read with
     `execStartPath`, must name the new binary (`service.SameExecutable`).
     Otherwise return the receipt with an error: "a later drop-in
     overrides ExecStart; <unit> still runs <path>", listing
     `DropInPaths`. Recovery's `Restore` then removes the drop-in. The
     drop-in keeps its name, `90-selfupdate.conf`.
   * **Test:** `reconcile_test.go`
     `TestReconcileVerifiesEffectiveExecStart`. A fragment plus
     `override.conf`; the fake gains `onReload func(map[string]string)`,
     which leaves `ExecStart` at the old path. Asserts an error,
     `Changed`, and that `Restore` removes the drop-in.
   * **Edit:** `TestReconcileRewrite` sets the new `ExecStart` in
     `onReload`.
   * **Red:** `changed=true err=<nil>`.
   * **Plant:** delete the check.
   * **Docs:** `reconcile.go:31-35`, `:70-73`; `unit.go:56-57`; the
     extending guide.
   * **Live:** `TestLiveRewriteWithOverride`: an `override.conf` that
     resets `ExecStart` makes `Reconcile` fail, and the unit is unchanged
     afterwards.
4. **D5: an unquoted SCM command line with spaces** (`scm/reconcile.go:88-112`).
   * **Fix:** `programAndArgs`, for an unquoted line with a space that does
     not start with `executable`:
     * candidates are each prefix ending before a space, then the whole
       line, in that order; `.exe` is appended to one with no extension;
     * the first that is an existing regular file (a new seam, `statFile
       = os.Stat`) is the program, as `CreateProcess` resolves it;
     * if none exists, the first candidate ending in `.exe`,
       case-insensitively;
     * if none, it refuses: "cannot tell where the program ends in the
       unquoted command line %q; quote it".

     Under `RewritePath`, it refuses when the existence choice and the
     `.exe` choice differ.
   * **Test:** `scm_test.go` `TestReconcileUnquotedPathWithSpaces`:
     * `C:\Program Files\Old\demo.exe run`, with nothing on disk, names
       `C:\Program Files\Old\demo.exe`; under `RewritePath`, the line
       becomes `"C:\Program Files\New\demo.exe" run`;
     * with `C:\Program.exe` present, `RewritePath` refuses.
   * **Red:** `demo runs C:\Program`.
   * **Plant:** plain `decompose`.
   * **Windows-only test:** `compose_windows_test.go`
     `TestReconcileRealCommandLine`, with real files under a directory
     whose name has a space.
   * **Docs:** `reconcile.go:23-29`, `:88-91`; the `RewritePath` comment
     in `service.go`.
   * **Live:** `TestLiveUnquotedPathWithSpace`: a throwaway service
     registered with an unquoted path under a spaced directory, and
     reconciled after a move.
5. **D7: an unreadable plist is an error** (`launchd/lifecycle.go:38-77`).
   * **Fix:** `Enabled` reads both keys through P1's `plistValue`.
     `RunAtLoad` counts only as a `bool` `true`; `KeepAlive` as a `bool`
     `true` or a `dictionary`. A `plistValue` error is returned. The
     `plistBool` helper goes.
   * **Tests:**
     * `TestEnabled` gains "unreadable" (`f.corrupt`: an error) and
       "RunAtLoad integer" (`types{"RunAtLoad": "integer"}`, raw `0`:
       false);
     * `plutil_darwin_test.go` `TestEnabledRealPlutil`: corrupt; mode
       0000 (skipped as root); `<integer>0</integer>`.
   * **Red:** `Enabled=false,<nil>`, and `true` for the integer.
   * **Plant:** return `false, nil` on a `plistValue` error.
   * **Docs:** `lifecycle.go:38-40`, `:57-62`.
   * **Live:** `TestLiveRunAtLoadInteger`: a plist with `RunAtLoad`
     `<integer>1</integer>`, bootstrapped. Whether launchd starts it is
     recorded, and `Enabled` must agree with it.
6. **D8: handoff files per unit** (`systemd/detach.go:158-200`).
   * **Fix:** `writeEnvFile` writes `<envDir>/<unit>/handoff-<id>.env`;
     both directory levels go through `privateDir` (0700). The sweep reads
     only `<envDir>/<unit>` with `os.ReadDir`, matching the `handoff-`
     prefix and `.env` suffix. Flat files from earlier releases are left
     alone.
   * **Test:** `detach_unix_test.go` `TestDetachKeepsOtherUnitsEnvFile`:
     systemd 239, two units, one handoff each; the first's file survives
     the second's handoff.
   * **Red:** `other.service's env file is gone`.
   * **Edits:** `TestDetach` (`:30`), `TestDetachOldSystemd` (`:71`) and
     `TestDetachFailureRemovesFile` (`:115`) follow the layout.
   * **Plant:** glob the parent directory.
7. **D11: "not found" for the unit's own name** (`systemd/unit.go:226-243`).
   * **Fix:** the not-installed case matches only, lowercased: `unit <unit>
     not found`, `unit <unit> not loaded`, `unit <unit> could not be
     found`, and `unit file <unit> does not exist`. The unit is quoted as
     systemd prints it.
   * **Test:** `TestCommandErrors` (`:188`) gains `Failed to start
     demo.service: Unit missing-dep.service not found.`, which is not
     `ErrNotInstalled`; and `Unit demo.service not found.`, which is.
   * **Red:** `ErrNotInstalled` for the dependency.
   * **Plant:** a bare `strings.Contains(lower, "not found")`.
   * **Live:** `TestLiveMissingDependency`: a unit with `Requires=` a
     missing unit fails `Start`, without `ErrNotInstalled`.
8. **Run:** `go test -count=1 -run
   'TestStartBaselineIsAfterStart|TestWaitHealthyFailsFast|TestReconcile|TestEnabled|TestDetach|TestCommandErrors'
   ./selfupdate/service/...`. On Windows (the CI leg, and the Windows
   test host), the same run covers `TestReconcileRealCommandLine`.
9. **Acceptance:** as P1's API and test items; Linux and Windows
   cross-vet; the **Live tests**, for each backend.

### Phase P6: archives, the spec and codesign (E1–E6)

**Files:** `selfupdate/archive/unpack.go`, `selfupdate/archive/doc.go`,
`selfupdate/archive/main_test.go`, `selfupdate/archive/unpack_test.go`,
`selfupdate/archive/fuzz_test.go`, the new seed files under
`selfupdate/archive/testdata/fuzz/`, `selfupdate/releasespec/validate.go`,
`selfupdate/releasespec/spec_test.go`, the new seed under
`selfupdate/releasespec/testdata/fuzz/FuzzParse/`,
`internal/cmd/selfupdate-release/check.go`,
`internal/cmd/selfupdate-release/pack_test.go`,
`selfupdate/codesign/codesign.go`, `selfupdate/codesign/codesign_test.go`,
`docs/guides/extending-selfupdate.md`, `docs/guides/building-releases.md`,
this PLAN.

1. **E1: a zip's local header must agree** (`archive/unpack.go:328-391`).
   * **Fix:**
     * a `headerRecorder` wraps the `io.ReaderAt` given to
       `zip.NewReader`. It records the offset and length of the last
       `ReadAt`, and its length is reset before each `zf.DataOffset()`;
     * after `DataOffset`, a recorded length other than 30 is refused:
       "entry %q: its local header could not be located";
     * after the overlap check (`:373-378`), `checkLocal(ra, zf, hdr,
       data)` reads the 30-byte header and requires:
       * the signature `PK\x03\x04`;
       * name length `n` (at 26) and extra length `m` (at 28) with `hdr +
         30 + n + m == data`;
       * the name bytes equal to `zf.Name`;
       * the method (at 8) equal to `zf.Method`;
       * local flags (at 6) with `&0x2041 == 0`;
     * `checkExtra(b []byte)` walks `[tag u16][size u16][data]` over the
       central extra (`zf.Extra`) and the local extra. It refuses an
       overrun, and tag 0x7075: "entry %q has an Info-ZIP Unicode Path
       field".
   * **Test helpers** (`main_test.go`):
     * `localNameZip(t, central, local string, body []byte) []byte` builds
       a zip, then rewrites the local name bytes;
     * `unicodePathExtra(name, u string) []byte` builds a 0x7075 field.
   * **Tests:** `TestUnpackRefuses` gains "local header names another file"
     and "Info-ZIP Unicode Path". Each is `ErrIntegrity`.
   * **Red:** `Unpack = <nil>`.
   * **Seeds:** both archives seed `FuzzUnpackZip`, as files under
     `testdata/fuzz/FuzzUnpackZip/`.
   * **Plants:** the name comparison off; the 0x7075 check off; `checkLocal`
     moved before the overlap check (`TestUnpackRefuses/overlapping_entries`
     fails).
2. **E2: names that fold** (`unpack.go:200-219`; N1).
   * **Fix:** in `entries.add`, after `checkName`:
     * any byte outside 0x20–0x7e is refused: "entry name %q is not
       printable ASCII";
     * any element of the cleaned name ending in `.` or a space is
       refused, except the elements `.` and `..`, which `checkName`
       already handles.
   * **Tests:** `TestUnpackRefuses` gains `relays` plus `relayſ`, `relay.`,
     and `relay ` (tar and zip each).
   * **Red:** `<nil>`.
   * **Seeds:** the long-s tar seeds `FuzzUnpackTarGz`.
   * **Plants:** drop the ASCII loop; drop the suffix test.
3. **E3: a directory is an entry ending in `/`** (`unpack.go:293-296,348-351`).
   * **Fix:**
     * zip: `mode.IsDir()` must equal `strings.HasSuffix(zf.Name, "/")`,
       else "entry %q: its directory attribute does not match its name";
       a directory entry with `UncompressedSize64 > 0` or
       `CompressedSize64 > 0` is refused ("directory %q holds data");
     * tar: a regular entry whose name ends in `/` is refused ("regular
       file %q is named as a directory").
   * **Tests:**
     * a zip entry with FAT attribute 0x10 and data, plus `x/relay`;
     * a `zfile` with `SetMode(fs.ModeDir|0o755)` and data;
     * a tar built with the new `slashRegTarGz` helper (`tarBlock` with
       name `relay/` and type `0`);
     * a zip directory entry with data.
   * **Red:** `<nil>`.
   * **Seeds:** the tar for `FuzzUnpackTarGz`, the zips for
     `FuzzUnpackZip`.
   * **Plants:** each rule off in turn.
4. **E4: encrypted entries** (`unpack.go:352-354`).
   * **Fix:** `zf.Flags&0x2041 != 0` is refused: "entry %q is encrypted
     (flags %#04x)". `checkLocal` refuses the local flags with "(local
     flags %#04x)".
   * **Tests:** stored entries with flags 0x1 and 0x40.
   * **Red:** `ciphertext… err=nil`.
   * **Plant:** the central check off. The local check then fires with a
     different message, which the test pins.
5. **E5: composed names the client accepts** (`releasespec/validate.go:42-60`,
   `spec.go:257-290`, `tool/check.go:46-56`; N2).
   * **Fix:**
     * a new `validateArchiveNames()`, called in `Validate` after
       `validatePlatforms`. Under `archive` packaging:
       * every `AssetName(product, platform)` must match the archive
         selector's 128-character rule: "releasespec: products[%d]: the
         asset name %q is %d characters; the archive selector accepts at
         most 128";
       * for a tar.gz platform, the program's name (with `.exe` on
         Windows) is at most 100 characters: "…: %q is %d characters; a
         tar.gz entry name holds at most 100";
     * `check` builds `archive.NewSelector` from the packed platforms, and
       calls `Select` for each staged archive: "the client's selector
       refuses it: %v".
   * **Tests:**
     * `spec_test.go` `TestParseRefuses`, under archive packaging: a
       120-character product is refused; a 110-character product with
       `-windows-amd64.zip` (128 in all) is accepted; 111 is refused; 120
       under binary packaging is accepted; a 101-character tar.gz program
       is refused;
     * `pack_test.go` `TestCheck` (`:156`): a staged archive whose name
       the selector refuses.
   * **Red:** `<nil>`.
   * **Seeds:** a 128-character name for `FuzzParse`.
   * **Plants:** the call removed; 129 allowed; binary packaging
     included; `Select` removed.
   * **Docs:** the building guide, step 1 and §7.
6. **E6: the requirement in its own run** (`codesign/codesign.go:131-220`).
   * **Fix:**
     * `Transform` runs `codesign --verify --strict -R=identifier
       "<Identifier>" <file>`, then, when `Requirement` is set,
       `codesign --verify --strict -R=<Requirement> <file>`. Each exit
       code maps as today, naming its own requirement;
     * `requirement()` goes.
   * **Tests:**
     * `codesign_test.go` `TestSignerRequirementIsSeparate`, with
       `anchor apple) or (always` and a `/* comment */` form: three argv
       lines, with exact `-R=` values;
     * `TestSignerErrors` with outputs `{}`, `{}`, `{ExitCode: 3}`: the
       requirement's error;
     * `TestSignerArguments` expects three calls.
   * **Red:** two calls; a composed `-R=`.
   * **Plants:** the composition restored; the requirement run skipped.
   * **Docs:** `codesign.go:48-50`.
7. **Docs for E1–E4:** `archive/doc.go`, and the extending guide's "Ship
   an archive", name the new refusals.
8. **Run:** `go test -count=1 -run
   'TestUnpackRefuses|TestUnpackAccepts|TestParseRefuses|TestCheck|TestSigner'
   ./selfupdate/archive ./selfupdate/releasespec ./selfupdate/codesign
   ./internal/cmd/selfupdate-release`.
9. **Acceptance:**
   * as P1's API and test items;
   * `make fuzz FUZZTIME=60s` clean, with the new seeds;
   * these real archives still unpack, in a scratch test run on this Mac:
     * from Info-ZIP `zip`, `ditto -c -k` and bsdtar: `TestUnpackAccepts`
       fixtures made by the tools, kept only in the record;
     * from `git archive`: only after Q4's E8, so it is recorded there.

### Phase P7: the installers and the release tooling (F1, F2, F4, F5, F6, F7, F8, F9, G7)

**Files:** `installer/install.sh`, `installer/install.ps1`,
`tool/installer_sh_test.go`, `tool/installer_ps_test.go`,
`tool/installer_harness_test.go`, `tool/main.go`, the new
`tool/usage_test.go`, `.github/workflows/publish-selfupdate-release.yml`,
`scripts/workflow-shape_test.sh`, `scripts/release-latest-flag.sh`,
`scripts/release-latest-flag_test.sh`, `docs/guides/building-releases.md`,
`docs/architecture.md`, this PLAN.

1. **F1: a repeated `--product` counts once** (`install.sh:342,358-367`;
   `install.ps1:323-327`).
   * **Fix, sh:**
     * at parse, `--product` refuses an empty or unsafe name: `case $2 in
       '' | *[!A-Za-z0-9._-]*) die 1 "--product needs a product name, not
       \"$2\"" ;; esac`;
     * a new helper `in_list WORD LIST` (no `local`);
     * the validation loop builds `chosen` without repeats, and
       `SELECTED=$chosen`.
   * **Fix, ps1:** the loop builds `$chosen` with `-cnotcontains`, and
     refuses unknown names as today.
   * **Tests:**
     * `installer_sh_test.go` "a repeated --product installs it once"
       (`standIn("old")`): exit 0; `relay.prev` holds the old binary;
     * `installer_ps_test.go`, the same in `psBlock` with `-Product
       'relay','relay'`.
   * **Red:** `exit 1 … mv: … relay.new …`.
   * **Plant:** drop the repeat guard.
   * **Docs:** building guide, Options.
2. **F2: hooks and identity commands get no stdin** (`install.sh:215,242`).
   * **Fix:** `"$exe" $args </dev/null` and `first=$("$1/$prod" $args
     2>/dev/null </dev/null | head -n 1)`.
   * **Test helpers** (`installer_harness_test.go`): `renderWith(t, kind,
     edit func(*releasespec.Spec))` and `stdinReader(role, version string)
     []byte`, a stand-in that reads stdin to EOF, then acts.
   * **Tests:**
     * "an identity command that reads stdin hides no product": a second
       identity product reporting `v9.9.9` exits 2;
     * "a hook that reads stdin skips no hook": a second `after_install`
       hook runs, and both are logged.
   * **Red:** exit 0, one hook logged.
   * **Plants:** either `</dev/null` removed.
   * **Docs:** building guide, Hooks, and step 3: "they run with no
     standard input".
3. **F4: the immutability timeout explains itself**
   (`publish-selfupdate-release.yml:244-263`).
   * **Fix:** `immutable` is kept after the loop. After the deadline:
     * when not immutable: "publish-selfupdate-release: $tag is published,
       but GitHub has not marked it immutable after 120 s. Immutable
       releases must be on for $GH_REPO: Settings > General > Releases >
       \"Enable release immutability\" (or the organization's release
       policy); it applies only to releases published after it is on. The
       release is live and mutable: delete it (gh release delete $tag
       --repo $GH_REPO --yes), turn the setting on, and re-run all jobs.";
     * when immutable: "… is immutable, but gh release verify did not pass
       within 120 s".
   * **Test:** `workflow-shape_test.sh` asserts the step's `run` contains
     `Enable release immutability` and `gh release delete`.
   * **Red:** the assertion fails.
   * **Plant:** drop the phrase.
4. **F5: a relative install directory is made absolute.**
   * **Fix, sh** (after `mkdir -p "$DIR"`, `:395`): `case $DIR in /*) ;;
     *) DIR=$(CDPATH='' cd -- "$DIR" && pwd) || die 1 "cannot resolve
     $DIR" ;; esac`.
   * **Fix, ps1** (after `:320`):
     `$dir = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($dir,
     [ref]$prov, [ref]$drv)`, refusing a provider other than `FileSystem`
     (exit 1).
   * **Tests:**
     * sh "a relative --dir is advised as an absolute PATH entry"
       (`shCase` with a working directory; the expected path through
       `filepath.EvalSymlinks`);
     * ps: `-InstallDir rel` writes the absolute folder to the scratch
       PATH key.
   * **Red:** `export PATH="rel:$PATH"`.
   * **Plant:** drop the resolve.
   * **Docs:** building guide, Options.
5. **F6: hashes from stdin** (`install.sh:123-133`).
   * **Fix:** `sha256sum <"$1"`, `shasum -a 256 <"$1"`, `openssl dgst
     -sha256 -r <"$1"`.
   * **Test:** `TestInstallShHashToolsEscapes`, one subtest per tool
     (`linkTools`, `needStubs`, `--dir 'home/a\b'`): exit 0.
   * **Red:** the `sha256sum` and `shasum` legs: `SHA-256 \… does not
     match`.
   * **Plant:** `shasum -a 256 "$1"` restored.
6. **F7: edge cases.**
   * **(a)** `check_tag` (`:152`) first refuses anything outside
     `[A-Za-z0-9.-]`, and the empty string: `case $1 in '' |
     *[!A-Za-z0-9.-]*) die 1 …`. In `install.ps1`, `Test-ReleaseTag`
     (`:129`) ends with `\z`.
   * **(b)** closed by F1's parse rule.
   * **(c)** `install.sh` keeps a `PREVIOUS` list beside the `mv` to
     `.prev` (`:412-414`); `restore` puts back only those. `install.ps1`
     keeps `$hadPrevious` the same way (`:399-423`).
   * **(d)** `run_hooks` runs only a regular file: `[ -f "$exe" ] && [ -x
     "$exe" ]`. After the directory is resolved, a preflight refuses a
     directory at `$DIR/$prod` or `$DIR/$prod.prev`: exit 1, "… is a
     directory; nothing was changed". ps1 uses `Test-Path -PathType
     Container`.
   * **Tests:** sh:
     * "a --version with a newline is refused": exit 1, no request;
     * "an empty --product is refused";
     * "a stale .prev stays when the identity fails";
     * "a directory at the target is refused": exit 1, the directory
       untouched;
     * "a hook named by a directory is skipped".

     ps: (a), (c) and (d).
   * **Red:** each one's current exit or effect.
   * **Plants:** each rule off in turn.
   * **Docs:** building guide, Exit codes and Options.
7. **F8: a latest release that is not `vX.Y.Z`**
   (`scripts/release-latest-flag.sh:53-62`).
   * **Fix:** when the current latest is not `vX.Y.Z`, compare against the
     highest published stable `vX.Y.Z` from `gh release list
     --exclude-drafts --exclude-pre-releases --limit 1000 --json tagName
     --jq '.[].tagName'`:
     * none: no flag, with a notice naming the old latest on stderr;
     * a failed list: exit 1.
   * **Test:** `release-latest-flag_test.sh`, whose stub answers `release
     list` from `STUB_LIST`. With `legacy-2024` as latest and `v1.5.0
     v1.4.1 nightly-7 v2.0.0-rc.1` listed:
     * `v1.4.2` gives `--latest=false`;
     * `v1.6.0` gives nothing;
     * with no `vX.Y.Z` listed, nothing;
     * a list error gives rc 1.
   * **Red:** exit 1, "is not a release tag".
   * **Plants:** skip the fallback; `min` for `max`.
8. **F9: one publish at a time** (`publish-selfupdate-release.yml:39-41`;
   N9).
   * **Fix:** under `jobs.publish`:

     ```yaml
         concurrency:
           group: go-selfupdate-lib-publish-${{ github.repository }}
           cancel-in-progress: false
     ```

     `check-workflows.sh --rule expressions` allows `github.repository` in
     `concurrency.group`; if it does not, that is a deviation.
   * **Test:** `workflow-shape_test.sh` asserts the group and
     `cancel-in-progress: false`.
   * **Red:** the assertion fails.
   * **Plant:** `cancel-in-progress: true`.
   * **Docs:** building guide step 4 (the group's name, which a caller
     must not reuse); `docs/architecture.md`, Release workflow.
9. **G7: the usage line** (`tool/main.go:11`).
   * **Fix:** `stage`'s line gains `[-repository OWNER/NAME]`.
   * **Test:** `usage_test.go` `TestUsageListsEveryFlag`. It parses the
     package's non-test files with `go/parser`, collects each `f.str(name,
     required)` per `runX` function, and checks the usage line of that
     subcommand in `main.go`'s doc comment: a required flag appears as
     `-name`, an optional one inside `[…]`.
   * **Red:** `stage: -repository is not in the usage line`.
   * **Plant:** remove it again.
10. **Run:**

    ```bash
    SELFUPDATE_INSTALL_REQUIRE_SHELLS=sh,bash go test -count=1 -run 'TestInstallSh|TestInstallPs|TestUsageListsEveryFlag' ./internal/cmd/selfupdate-release
    sh scripts/release-latest-flag_test.sh
    sh scripts/workflow-shape_test.sh
    ```

11. **Acceptance:**
    * as P1's API and test items;
    * `scripts/check-installers.sh` on both templates; shellcheck; `dash
      -n`;
    * the sh tests pass on this Mac (sh, bash) and on the Linux test host
      (dash, bash), and CI's Alpine and Debian jobs pass;
    * the ps tests pass on the Windows test host under Windows PowerShell
      5.1 and PowerShell 7;
    * actionlint and `check-workflows.sh` pass on the publish workflow.

### Phase P8: the `v1.10.1` release

**Files** (the release commit): `docs/guides/migrating-from-mcplib-selfupdate.md`,
`docs/decisions/0011-MADR-reference-service-lifecycles.md`,
`docs/decisions/0012-MADR-archive-assets-and-macos-codesign.md`,
`docs/decisions/0013-MADR-build-and-stage-release-workflow.md`, this PLAN.
The pin commit follows the tag, with the release procedure's step 5 files.

1. **Migration guide:** `### From v1.10.0 to v1.10.1`, inside §10 before
   its `### Check`:
   * `go get …@v1.10.1`; it changes no API, and `make apicheck` reports it
     compatible with `v1.10.0`;
   * **Behaviour changes** (one bullet each):
     * a backup the run could not restore is kept as
       `.<base>.selfupdate-kept-<n>`, and nothing removes it; a dry run
       sweeps nothing (B1);
     * a second `Commit` or `Rollback` is refused (B7);
     * a target replaced during the update is `ErrConcurrentUpdate` (B2);
     * a release without the platform's asset fails as
       `unsupported-platform`, and is cached (A1);
     * a stop that fails after the service stopped starts it again (B3);
     * the refused archives (E1–E4), and the refused spec names (E5);
     * a codesign requirement is checked in its own run (E6);
     * the installers' changes (F1, F2, F5–F7);
     * the publish workflow's concurrency group and messages (F4, F8,
       F9).
   * **Check:** `go list -m` gives `v1.10.1`.
2. **Record amendments,** each dated, each citing this PLAN:
   * 0011-MADR `### A7`: §7's systemd baseline is read after the start
     (D2); the launchd reload (D3); the stop wait's bound (B3, N8); D8's
     per-unit layout, which amends A2's;
   * 0012-MADR, a new `## Amendments` before `## More Information`, with
     `### A1`: §4's new refusals (E1–E4); §5's requirement in its own run
     (E6);
   * 0013-MADR, a new `## Amendments` before `## More Information`, with
     `### A1`: §2's archive-name limits (E5).
3. **Release notes:** `### Release notes for v1.10.1 (date)` in this
   PLAN's execution record, one line per finding, in the migration guide's
   order.
4. **The release procedure,** steps 1–7, for `v1.10.1`. Its live tests are
   P1's and P5's. The release commit is the last of P1–P8's commits.
5. **This PLAN's record** gains the release's facts: the commit, the CI
   run ids, the proxy time, and the pin commit's changed lines.

### Phase Q1: the API and the command-surface contracts (C3, C8, C9, G3, A1's 404)

**Files:** `selfupdate/cli/run.go`, `selfupdate/cli/doc.go`,
`selfupdate/cli/handoff_test.go`, `selfupdate/service/handoff.go`,
`selfupdate/service/handoff_test.go`, `selfupdate/errors.go`,
`selfupdate/runoptions.go`, `selfupdate/checker.go`,
`selfupdate/stream.go`, `selfupdate/types.go`, `selfupdate/document.go`,
`selfupdate/updater.go`, `selfupdate/github.go`,
`selfupdate/checkcache.go`, `selfupdate/doc.go`, `selfupdate/events_test.go`,
`selfupdate/warnings_test.go`, `selfupdate/example_test.go`,
`selfupdate/managed_started_test.go`, `selfupdate/platform_apply_test.go`,
the new `selfupdate/zero_test.go`, `selfupdate/github_test.go`,
`selfupdate/checkcache_outcome_test.go`, the 13
`selfupdate/cli/testdata/golden/*.json.stdout` files, the three backends'
`live_*_test.go`, `docs/guides/extending-selfupdate.md`, this PLAN.

1. **C3: a handoff needs `--yes` and an operation**
   (`service/handoff.go:146-157`, `cli/run.go:123-132`).
   * **Fix:**
     * `HandOffFunc` uses its `req`. Inside the service (`d.Inside`), in a
       run that is not itself a handoff (`EnvHandOff` unset), with
       `!req.Yes`, it returns `false, "", fmt.Errorf("selfupdate: service:
       an update from inside the service runs detached and cannot ask;
       pass --yes: %w", selfupdate.ErrConfirmationRequired)`;
     * `cli.Run` calls `Detach` only when `wouldInstall(ctx, u, req)`:
       * it runs `u.Checker().Check(ctx, CheckRequest{…})` from the
         request's fields;
       * an error, or `ForceRequired && !req.Force`, gives false;
       * otherwise `Available || req.Force`.
     * A check error is not reported here: the run that follows reports
       it.
   * **Tests:**
     * `cli/handoff_test.go` `TestHandOffNeedsYes`: a new
       `insideDetacher` helper, interactive `y`, no `--yes`. Not detached;
       exit 1; `ErrConfirmationRequired`; target unchanged;
     * `TestHandOffOnlyWhenUpdateFound`: latest `v1.0.0`, `--yes`. No
       `Detach`; exit 0; "up to date";
     * `service/handoff_test.go` `TestHandOffFunc` passes `Yes: true`, and
       gains the refusal case.
   * **Red:** `Detach called=true yes=false exit=0`.
   * **Plants:** drop `wouldInstall`; drop the `!req.Yes` branch.
   * **Docs:** `cli/run.go:57-61`; `cli/doc.go:31-35`; the extending guide,
     "Updating from inside the service".
2. **C8: zero values** (`runoptions.go:110`, `checker.go:104,123`,
   `stream.go`).
   * **Fix:**
     * the sentinel `ErrNotConstructed = errors.New("selfupdate: not
       constructed by its constructor")`;
     * `RunWith`, `Stream.prepare`, `Checker.Check` and `CheckCached`
       return it for a nil receiver, or one whose required collaborator
       field is nil;
     * `Stream.Cancel` returns at once when `s == nil || s.cancel == nil`.
   * **Test:** `zero_test.go` `TestZeroValues`:
     * `Run`, `RunWith`, `Check` and `CheckCached` on zero and nil
       receivers match `ErrNotConstructed`;
     * `Start(&Updater{})`'s `Finished.Err` matches it;
     * a zero `Cancel` does not panic.
   * **Red:** a panic.
   * **Plant:** drop the `Stream.prepare` check.
   * **Docs:** the `Updater` comment (`types.go:803-804`), `checker.go:24-26`,
     the `Stream.Cancel` comment, and `doc.go`'s errors.
3. **C9 and G3: the result document's schema 3** (`updater.go:334,367`,
   `types.go:123-173`, `document.go`).
   * **Fix:**
     * `Result.RolledBack bool` ("the previous binary was restored after
       the new one was installed"), set at `updater.go:367` from
       `installed.RolledBack`;
     * `Result.ProbesSkipped bool`: when `req.DryRun`, `req.Platform` is
       not the running platform, and probes are configured, the probes are
       not run and it is set;
     * `ResultDocument` gains `rolled_back` and `probes_skipped`, with no
       `omitempty`;
     * `resultDocumentSchema` becomes 3; `HandOffResultSchema` stays 1;
     * the `complete` event's Detail stays `dry-run`.
   * **Tests:**
     * `managed_started_test.go` `TestRunReportsRolledBack`
       (`fakeLife{installed, running, healthErr}`): `ErrManagedInstall`,
       `res.RolledBack`, `"rolled_back":true`, and an `EventRolledBack`;
     * `platform_apply_test.go` `TestDryRunForeignPlatformSkipsProbes`,
       with the running-platform control (probes run);
     * `events_test.go` `TestResultDocumentCoversEveryField` covers both
       fields.
   * **Edits** (the schema number and the new fields, in literal strings):
     `events_test.go`, `warnings_test.go`, `example_test.go`,
     `managed_started_test.go`.
   * **Red:** the fields do not exist (the build fails); and `exec format
     error` for C9.
   * **Plants:** drop the copy at `:367`; drop the platform condition.
   * **Goldens:** all 13 `*.json.stdout`, through rule 8.
   * **Live:** in each of `launchd/live_darwin_test.go`,
     `systemd/live_linux_test.go` and `scm/live_windows_test.go`, a new
     `TestLiveHandOffHealthFailureReportsRollback`:
     * the handed-off build exits at once, so `WaitHealthy` fails;
     * the handoff result has `ExitCode` 1, `!Applied` and `RolledBack`;
     * the service runs the old binary afterwards.

     `runLiveUpdate` copies `installed.RolledBack` into its result.
   * **Docs:** `cli/doc.go:18`; the extending guide, "Read JSON output"
     and "Updating from inside the service".
4. **A1's 404: a release that does not exist** (`github.go:292-335`,
   `checkcache.go:21-60`, `errors.go`; N6).
   * **Fix:**
     * the sentinel `ErrNoRelease = errors.New("selfupdate: no such
       release")`;
     * `getRelease` wraps it for a 404: `fmt.Errorf("%w: %w",
       ErrNoRelease, statusErr)`;
     * `CheckNoRelease` is added after `CheckMutableRelease`, with the name
       `no-release`, cached for `maxAge`;
     * `failureClass` maps it to `no-release`.
   * **Tests:**
     * `github_test.go` `TestLatestNotFoundIsNoRelease`, and a missing tag
       for `ByTag`;
     * `checkcache_outcome_test.go` gains the row: five outcomes, one
       network call over three `CheckCached`.
   * **Red:** `github http 404`, three calls.
   * **Plant:** drop the wrap.
   * **Docs:** `checkcache.go`'s outcome list; `doc.go`; the extending
     guide, "Show an update banner".
5. **Run:** `go test -count=1 -run
   'TestHandOff|TestZeroValues|TestRunReportsRolledBack|TestDryRunForeignPlatformSkipsProbes|TestResultDocumentCoversEveryField|TestLatestNotFoundIsNoRelease|TestCheckCachedOutcomes|TestGolden|TestCommandGolden'
   ./selfupdate/...`.
6. **Acceptance:** `make apicheck` against `v1.10.1` passes, and the V4
   report lists exactly:
   * `ErrNotConstructed`, `ErrNoRelease` and `CheckNoRelease`;
   * `Result.RolledBack` and `Result.ProbesSkipped`;
   * `ResultDocument.RolledBack` and `ResultDocument.ProbesSkipped`.

### Phase Q2: the install and network contracts (A5, B5)

**Files:** `selfupdate/checkcache.go`, `selfupdate/session.go`,
`selfupdate/replace.go`, `selfupdate/types.go`,
`selfupdate/network_hardening_test.go`, `selfupdate/modebits_unix_test.go`,
`docs/architecture.md`, `docs/decisions/0010-PLAN-v1-5-1-contract-preserving-fixes.md`,
this PLAN.

1. **A5** (`checkcache.go:220-240`).
   * **Fix:** `var nb time.Time; if rl.Remaining == 0 { nb = rl.Reset }`.
     The comment drops "the later of the reset and".
   * **Test:** `TestNotBeforeClamped` gains two rows:
     * `Retry-After` 1 m, with quota left and a reset 50 m away, gives 1 m;
     * `Remaining` 0 with a reset 30 m away gives 30 m.
   * **Red:** the first row gives `49m59s`.
   * **Plant:** `nb := rl.Reset`.
   * **Docs:** `checkcache.go:138-142`; a dated note at 0010-PLAN-v1-5-1's
     deviation D1 (`:326`).
2. **B5** (`session.go:164-180`, P1's `retainLocked`).
   * **Fix:** a new `clearSpecialBits(path string) error`. It calls `Lstat`,
     then, when setuid or setgid is set, `osChmod(path, mode.Perm() |
     (mode & os.ModeSticky))`. It is called in `commitLocked` before the
     backup becomes `.previous`: on failure the backup is removed, and
     `keep previous: …` is joined. It is called in `retainLocked` too.
   * **Test:** `modebits_unix_test.go` `TestKeepPreviousClearsSpecialBits`,
     with setuid and setgid targets (`AllowSpecialModeBits`,
     `KeepPrevious`): `.previous` has neither bit, and the new binary keeps
     its bit.
   * **Red:** `mode=urwxr-xr-x`.
   * **Plant:** skip the chmod.
   * **Docs:**
     * `types.go:409-413` and `:566-569`: a kept or previous binary loses
       setuid and setgid, and a restore must set them again;
     * `docs/architecture.md`, the mode-bits paragraph.
3. **Run:** `go test -count=1 -run
   'TestNotBeforeClamped|TestKeepPreviousClearsSpecialBits' ./selfupdate`.
4. **Acceptance:** `make apicheck` against `v1.10.1` lists no change.

### Phase Q3: the service contracts (D6, D10, G4)

**Files:** `service/scm/lifecycle.go`, `service/scm/service.go`,
`service/scm/scm_test.go`, `service/scm/live_windows_test.go`,
`service/poll.go`, `service/poll_test.go`, `service/systemd/unit.go`,
`service/systemd/systemd_test.go`, `service/launchd/job.go`,
`service/launchd/launchd_test.go`, `service/execreconciler.go`, `service/execreconciler_test.go`,
`selfupdate/types.go`, `selfupdate/managed.go`, `selfupdate/updater.go`,
`selfupdate/managed_started_test.go`, `selfupdate/doc.go`,
`docs/guides/extending-selfupdate.md`, this PLAN.

1. **D6** (`scm/lifecycle.go:89-215`).
   * **Fix:**
     * an unexported `stoppedDeps map[string][]string` on `Service`,
       under `s.mu`;
     * `Stop` records each dependent right after its `stopOne` succeeds;
     * `Start`, once the service has left `START_PENDING` running, starts
       them in reverse stop order (`errAlreadyRunning` counts as success),
       and clears the record on success;
     * a dependent's failure is a `Start` error, naming it.
   * **Test:** `scm_test.go` `TestStopDependentsAreRestarted`:
     * `demo`, with dependents `child` and `grandchild`; after `Stop` and
       `Start`, both run;
     * a `Stop` that fails on `demo` after `child` stopped, then `Start`:
       `child` runs.
   * **Red:** `child stopped`.
   * **Plant:** skip the record.
   * **Docs:** `service.go:40-42` (`StopDependents`), and the `Stop` and
     `Start` comments.
   * **Live:** `TestLiveStopDependentsRestarted`: a throwaway dependent
     service.
2. **D10** (`service/poll.go:28-65,149-155`).
   * **Fix:**
     * `func (o PollOptions) Validate() error` applies the defaults, and
       refuses `Settle >= Timeout`: "service: Poll.Settle %s is not less
       than Poll.Timeout %s". A negative Settle is no window;
     * it is called from `systemd.newUnit`, `scm.newService` and
       `launchd.newJob`; launchd validates with `Settle` raised to `max(Settle,
       10 s)` unless negative;
     * `PollHealthy` does not call it;
     * `timeoutError` says "not ready within %s".
   * **Tests:**
     * `poll_test.go` `TestPollOptionsValidate`: zero is valid; 10 s
       settle and a 5 s timeout are refused; settle -1 with a 1 s timeout
       is valid;
     * each backend's `TestNewRefuses` gains the case.
   * **Edit:** `launchd_test.go` `TestWaitHealthySettlesAtLeastThrottle`
     (`:304`) builds valid options, then sets `j.o.Poll` directly.
   * **Red:** `New` succeeds.
   * **Plant:** drop one constructor's call.
3. **G4** (`types.go:538-546`, `service/execreconciler.go:104-140`,
   `managed.go:123-144`, `updater.go`; N7).
   * **Fix:**
     * `ReconcileResult.Warnings Warnings`;
     * `ExecReconciler.Reconcile` sets it from `Receipt.Warnings`, plus
       "<product>: <path> was rewritten and the service manager was not
       reloaded; the next start may run the previous definition" when
       `Changed && !Reloaded`;
     * `managedSession.Install`, after a successful commit, returns its
       result with an unexported `reconcileWarnings{Warnings}` error
       joined;
     * the updater splits that error out with `errors.As`, and emits one
       `EventWarning` per entry after `complete`, listing them in
       `Result.Warnings`.
   * **Tests:**
     * `execreconciler_test.go` `TestExecReconcilerSurfacesWarnings`;
     * `managed_started_test.go` `TestRunReportsReconcileWarnings`
       (`fakeRec` with warnings): an applied run with two warnings, and
       exit 0 under `cli`.
   * **Red:** the field does not exist (the build fails).
   * **Plant:** drop the join.
   * **Docs:** `types.go:170-172`, `:627-631`; `doc.go:76-77`; the
     extending guide, "Read JSON output" and "Run as a service".
4. **Run:** `go test -count=1 -run
   'TestStopDependentsAreRestarted|TestPollOptionsValidate|TestNewRefuses|TestWaitHealthySettlesAtLeastThrottle|TestExecReconcilerSurfacesWarnings|TestRunReportsReconcileWarnings'
   ./selfupdate/...`.
5. **Acceptance:** the V4 report lists exactly `PollOptions.Validate` and
   `ReconcileResult.Warnings`; the SCM live test passes.

### Phase Q4: the spec, archive and tooling contracts (E7, E8, F3)

**Files:** `selfupdate/releasespec/spec.go`,
`selfupdate/releasespec/installer.go`, `selfupdate/releasespec/doc.go`,
`selfupdate/releasespec/spec_test.go`,
`selfupdate/releasespec/installer_test.go`, the new seed under
`selfupdate/releasespec/testdata/fuzz/FuzzParse/`,
`internal/cmd/selfupdate-release/render_test.go`,
`selfupdate/archive/unpack.go`, `selfupdate/archive/doc.go`,
`selfupdate/archive/main_test.go`, `selfupdate/archive/unpack_test.go`, the
new seeds under `selfupdate/archive/testdata/fuzz/FuzzUnpackTarGz/`,
`internal/cmd/selfupdate-release/plan.go`,
`internal/cmd/selfupdate-release/plan_test.go`,
`internal/cmd/selfupdate-release/main.go`,
`.github/workflows/build-selfupdate-release.yml`,
`scripts/workflow-shape_test.sh`, `docs/guides/building-releases.md`,
`docs/guides/extending-selfupdate.md`, `docs/architecture.md`, this PLAN.

1. **E7** (`releasespec/spec.go:154-198`).
   * **Fix:** `walk` refuses a `nil` token: `releasespec: %q: null is not
     allowed; leave the field out`, with the key; at the root,
     `releasespec: null is not allowed`.
   * **Tests:** `TestParseRefuses` gains `null` for `installer`, `extras`,
     `packaging`, `prerelease_channels`, `tags`, `identity_args`, and
     `installer.name`; `[null]` inside a list; and `null` at the root.
   * **Edits:** `spec_test.go:137`; `installer_test.go:39-41` (`args` as
     `[]any{}`); `tool/render_test.go:23-34`.
   * **Red:** `<nil>`.
   * **Seeds:** `{"installer":null}` for `FuzzParse`.
   * **Plant:** `tok == nil && false`.
   * **Docs:** `installer.go:30-31`; `releasespec/doc.go`; building guide
     step 1.
2. **E8** (`archive/unpack.go:282-299`; N3).
   * **Fix:** before `seen.add`, a `tar.TypeXGlobalHeader`:
     * is refused when its `PAXRecords` hold `path`, `linkpath`, `size` or
       any `GNU.sparse.` key: "a PAX global header sets %q";
     * otherwise counts toward `MaxEntries` (a new `entries.skip()`) and is
       skipped.
   * **Tests:**
     * `TestUnpackAccepts` gains a `pax_global_header` with a `comment`
       record;
     * `TestUnpackRefuses` gains `path`, `linkpath` and `size`.
   * **Red:** the accept case: "not a regular file or a directory".
   * **Seeds:** for `FuzzUnpackTarGz`.
   * **Plants:** no skip; an empty key list.
   * **Docs:** `archive/doc.go:15-16`; the extending guide, "Ship an
     archive".
   * **Real archive:** `git archive --format=tar.gz` of a scratch
     repository holding `relay` unpacks, in a scratch test run, recorded.
3. **F3** (`tool/plan.go`, `build-selfupdate-release.yml:122-142`; Q12).
   * **Fix:**
     * `plan` gains a required `-module-dir DIR`;
     * a `specFloors` table (field path, minimum version, why), with
       `installer` → `v1.10.0`;
     * the requirement is read with `go mod edit -json` through the
       tool's `goTool`, in DIR;
     * versions compare by semver precedence, in the standard library
       only, since depguard's `other-packages` allows no `x/mod`;
     * the rules, in order:
       1. a module that is the library itself is skipped;
       2. a module with no requirement is an error;
       3. a directory `replace` is skipped, with a summary row;
       4. a module `replace` uses its version;
       5. a version below the floor is refused: "module <path> requires
          go-selfupdate-lib <v>; this spec's \"installer\" needs v1.10.0
          or later, or the program cannot parse the spec it embeds (go get
          github.com/maccavelli/go-selfupdate-lib@v1.10.0)";
     * the workflow's Plan step passes `-module-dir "src/$MODULE_DIR"`,
       with `MODULE_DIR: ${{ inputs.module-dir }}` in its `env`;
     * `main.go:9`'s usage line gains `-module-dir DIR`.
   * **Tests:**
     * `plan_test.go` `TestPlanRefusesAnOldLibrary`, nine cases:
       1. `v1.9.0` with `installer`: refused;
       2. `v1.10.0`: ok;
       3. `v1.9.0` without `installer`: ok;
       4. a pseudo-version above `v1.10.0`: ok;
       5. `v1.10.0-rc.1`: refused;
       6. a directory replace: ok;
       7. a module replace at `v1.9.0`: refused;
       8. the library itself: ok;
       9. no requirement: refused;
     * `TestSpecFloorsCoverEveryField`: every JSON path that `v1.9.0`'s
       `Spec` lacks has a floor;
     * `workflow-shape_test.sh` asserts the flag;
     * P7's `TestUsageListsEveryFlag` covers the usage line.
   * **Red:** `flag provided but not defined: -module-dir`.
   * **Plants:** delete the `installer` row; flip the comparison.
   * **Docs:** building guide step 4 (`module-dir`), step 12 and §9;
     migration guide §10; `docs/architecture.md`, the `plan` step.
4. **Run:**

   ```bash
   go test -count=1 -run 'TestParseRefuses|TestUnpackAccepts|TestUnpackRefuses|TestPlanRefusesAnOldLibrary|TestSpecFloorsCoverEveryField|TestUsageListsEveryFlag' ./selfupdate/releasespec ./selfupdate/archive ./internal/cmd/selfupdate-release
   sh scripts/workflow-shape_test.sh
   ```

5. **Acceptance:** `make apicheck` against `v1.10.1` lists no change;
   `make fuzz FUZZTIME=60s` clean; CI's `release-rehearsal` jobs green
   with the new flag.

### Phase Q5: the `v1.11.0` release

**Files** (the release commit): `docs/guides/migrating-from-mcplib-selfupdate.md`,
`docs/decisions/0011-MADR-reference-service-lifecycles.md`,
`docs/decisions/0012-MADR-archive-assets-and-macos-codesign.md`,
`docs/decisions/0013-MADR-build-and-stage-release-workflow.md`, this PLAN,
`docs/README.md`, and the 0015 MADR (its status line only, when V1–V10
hold).

1. **Migration guide:**
   * `## 11. From v1.10 to v1.11`, with `### What changes`, `### Adopting
     it` and `### Check`, covering:
     * `ErrNotConstructed`; `ErrNoRelease` and `CheckNoRelease`;
     * the JSON result's schema 3 (`rolled_back`, `probes_skipped`);
     * handoffs need `--yes` and an operation;
     * `ReconcileResult.Warnings`;
     * `PollOptions.Validate`, and the refused options;
     * `StopDependents` restarts dependents;
     * setuid and setgid cleared on `.previous` and kept backups;
     * the back-off rule;
     * `null` refused in a spec;
     * `git archive` tarballs accepted;
     * the workflow's module check;
   * §2 lists `v1.11.0`.
2. **Record amendments,** dated:
   * 0011-MADR `### A8`: D6, D10 and G4;
   * 0012-MADR `### A2`: E8;
   * 0013-MADR `### A2`: E7, F3.
3. **Release notes:** `### Release notes for v1.11.0 (date)` in the
   execution record.
4. **The release procedure** for `v1.11.0`; its live tests are Q1's and
   Q3's.
5. **Closing:** when V1–V10 hold, this PLAN becomes `complete`, and its row
   in `docs/README.md` follows. The MADR stays `accepted`.

## Verification

* **V1.** Each finding has an entry in the execution record's coverage
  table: ID, phase, test, red line, plant line. A decided or open finding
  has its decision row or its 0004 P3 row instead. All 64 IDs appear.
* **V2.** Every plant listed was applied in a scratch copy, and made its
  test fail.
* **V3.** For `v1.10.1`, the full API report is empty. In a scratch
  directory:

  ```bash
  A=golang.org/x/exp/cmd/apidiff@v0.0.0-20260908205506-85c1c2202aba
  git worktree add --detach "$W/base" v1.10.0
  (cd "$W/base" && go run "$A" -m -w "$W/base.api" github.com/maccavelli/go-selfupdate-lib)
  go run "$A" -m "$W/base.api" github.com/maccavelli/go-selfupdate-lib
  git worktree remove --force "$W/base"
  ```

  The last command prints nothing.
* **V4.** For `v1.11.0`, the same report against `v1.10.1` lists exactly
  Q1's and Q3's additions, under "Compatible changes".
* **V5.** `go.mod`, `go.sum` and `.golangci.yml`'s depguard rules are
  unchanged since `v1.10.0`: `git diff v1.10.0 -- go.mod go.sum
  .golangci.yml` is empty. `go mod tidy -diff` is clean.
* **V6.** `make gate` ends `overall=0` at every phase from R1 on, and its
  tests catch their plants.
* **V7.** The **Live tests** named in P1, P5, Q1 and Q3 pass, on CI's three
  legs and the test hosts, before the tag of their track.
* **V8.** The installer tests pass under sh, dash, bash and BusyBox (CI's
  Alpine job), and under Windows PowerShell 5.1 and PowerShell 7, in the
  three invocation forms.
* **V9.** `scripts/check-docs.sh --links` over every tracked Markdown file
  finds nothing; `--ids` over every changed file finds nothing.
* **V10.** CI is green on `main` after every phase, and on the `v1.10.1` and
  `v1.11.0` tags.

## Rollout and Rollback

* **Track 1** changes no released artifact. A phase reverts on its own.
* **`v1.10.1`** keeps every documented contract: a program moves to it with
  `go get` and nothing else. Consumers move their workflow pins to take the
  template and workflow fixes.
  * If a fix misbehaves after the tag, fix forward in `v1.10.2`.
  * A consumer can stay on `v1.10.0`.
* **`v1.11.0`** changes the contracts in its migration section.
  * A consumer that cannot take one stays on `v1.10.1`.
  * Fix forward in `v1.11.1`.
* **Before a tag,** a phase reverts together with its dependants:
  * P1's B1 needs B7;
  * P3's B2 and P5's D3 use P1's code;
  * Q1's C3 needs P4's C4;
  * Q1's schema 3 gathers C9 and G3;
  * Q2's B5 changes P1's `retainLocked`;
  * Q3's G4 needs P1's managed changes.

## Execution Record

### Approval (2026-10-07)

The owner answered the MADR's 15 questions one by one, choosing every
recommended answer, then asked for this PLAN to be written in full, and
approved it: "proceed". The PLAN as approved is commit `47f0f97`.

### Phase R0: records (2026-10-07)

* **The MADR** is `accepted`:
  * "Chosen option:" replaces "Proposed:";
  * a new **Answered (2026-10-07)** paragraph opens "Owner questions";
  * a new `### 4. Owner answers` states track 3's scope;
  * a new `## Amendments` holds `### A1 (2026-10-07): corrections found
    while planning`, this PLAN's Corrections with their evidence;
  * the last bullet of "More Information" names this PLAN.
* **This PLAN** is `in-progress`. **`docs/README.md`:** the MADR row reads
  `accepted`, and the PLAN row `in-progress`.
* **Checks:**
  * the session's document checker, `--links` and `--ids` on the three
    files: 88 links checked, 0 failures;
  * markdownlint-cli2 0.23.2: "0 issues in 0 files". The repository's
    config excludes MADR and PLAN files, so only `docs/README.md` was
    linted.

  No code changed, so the gate was not run (rule 5's Go and script checks
  do not apply).
* **Bootstrap exception:** records and the index only.
* Committed by the owner as `bae9813`.

### Deviation D1 (2026-10-07): R1's first gate run found A10

* **Found.** R1's scripts were written and their tests passed, and the
  first full `make gate` ended `overall=1`. 13 of its 14 steps passed;
  `fuzz` exited 2. `FuzzParseSHA256SUMS` found, in 13 s, a manifest that
  `ParseSHA256SUMS` accepts and whose canonical form it refuses ("round
  trip rejected: … bufio.Scanner: token too long"). This is pre-existing,
  in code R1 does not touch, and the MADR did not list it. Go wrote the
  input into the tree, as
  `selfupdate/testdata/fuzz/FuzzParseSHA256SUMS/ec5a3a374e24bbcf`, where
  `go test ./selfupdate` then failed on it.
* **Options put to the owner:**
  1. fix it now as P0, committed before R1 (recommended);
  2. commit R1 with the gate failing, and fix it in P2;
  3. a stricter 255-byte name rule, which changes the contract.
* **Decision:** the owner chose "Fix now as P0, before R1".
* **Records:** MADR amendment A2 adds finding A10. This PLAN gains Phase
  P0, a scope row, the order in rule 1, and 64 findings in the Goal and
  V1.
* **Files added:** P0's own list. R1's files, already written in the tree,
  wait uncommitted until P0 is committed.

### Phase P0: a manifest name that fits its canonical line (2026-10-07)

* **The input** (decoded): a 64-character digest, one space, and a
  4030-byte name, a 4095-byte line. Its canonical form, `<digest>
  <name>`, is 4096 bytes before its newline, one past what the 4096-byte
  scanner buffer holds with the newline.
* **Fix:**
  * `selfupdate/checksums.go`: `maxChecksumName = maxChecksumLine - 1 -
    sha256HexLen - 2` (4029), refused in `validateChecksumName` with
    "filename is %d bytes; at most %d fit a SHA256SUMS line";
  * `scripts/selfupdate_manifest.py`: `MAX_CHECKSUM_NAME`, the same rule
    on the name's UTF-8 bytes, after the basename check.
* **Red,** on the unfixed code:
  * `TestChecksumNameFitsTheLine`: `a 4030-byte name in a 4096-byte line:
    err = <nil>, want ErrIntegrity`;
  * the seed: `round trip rejected: selfupdate: SHA256SUMS is malformed:
    bufio.Scanner: token too long`;
  * `TestManifestDifferential`, with its two new boundary cases, passed
    there: both parsers accepted the 4030-byte name.
* **Green:** the P0 run passed (`TestManifestDifferential`: N=5000,
  accepted 1274, rejected 3726); `scripts/verify-selfupdate-release_test.sh`:
  "all fixtures passed".
* **Plants,** each in a scratch copy:
  * the Go check off: `TestChecksumNameFitsTheLine`,
    `TestManifestDifferential` and `FuzzParseSHA256SUMS` fail ("case 5001
    … go: <nil>; python: ok=false … filename longer than a SHA256SUMS line
    holds");
  * the Python check off: `TestManifestDifferential` fails ("go: … filename
    is 4030 bytes; at most 4029 fit a SHA256SUMS line").
* **Lint:** the first pre-add check failed on `makezero`: an `append` to the
  differential's cases, made with a length. They are now built with a
  capacity, and both plants were run again, each caught.
* **Checks:**
  * `make pre-add-check` on the three Go files: "3 file(s) clean (gofmt,
    golangci-lint, go vet, go test, govulncheck)";
  * the gate (R1's `scripts/gate.sh`, uncommitted in the tree), every
    step `rc=0`, `overall=0`:

    | Step | Last line |
    | :--- | :--- |
    | gofmt | gofmt: clean |
    | lint | 0 issues. |
    | vet | (none) |
    | race | ok … `service/systemd` |
    | shuffle | ok … `service/systemd` |
    | tidy | (none) |
    | apicheck | check-api-compat: compatible with v1.10.0 |
    | fuzz | go-fuzz: 1 fuzz targets ran clean in ./selfupdate/releasespec |
    | vuln | No vulnerabilities found. |
    | scripts | all script tests passed |
    | shellcheck | (none) |
    | crossvet | cross go vet clean |
    | links | check-docs: 317 links in 49 files, 0 broken |
    | ids | check-docs: 24 files, 16 deny-list rules, 0 findings |

    The run covered R1's uncommitted files as well; R1's own record cites
    its own run.
* Committed on the owner's ask ("Commit to main then proceed") as
  `cdc5ebc`.

### Phase R1: the gate in the repository (2026-10-07)

* **Red,** before the scripts existed:
  * `gate_test.sh`: `cp: …/scripts/gate.sh: No such file or directory`,
    exit 1;
  * `check-docs_test.sh`: "0 passed, 11 failed";
  * `plant-copy_test.sh`: "2 passed, 8 failed". The two that passed check
    only that nothing was changed.
* **Written:**
  * `scripts/gate.sh`, 14 steps;
  * `scripts/check-docs.sh`, `--links` and `--ids [--ids-optional]`;
  * `scripts/plant-copy.sh`;
  * their three tests, as the PLAN's R1 steps 1–4 describe.

  Differences from the steps as written:
  * **`GATE_SKIP`:** the gate reports a skipped step as `<step> skipped`;
  * **step functions:** shellcheck's SC2329 (functions it sees no call
    of) is disabled once at the file's head, since the steps run through
    `step "$@"`;
  * **tool checks:** the `ids` step also checks for `sort`;
  * **test stubs:** the tests write their stub scripts with quoted
    heredocs, as `go-precheck_test.sh` does, which keeps shellcheck clean
    (SC2016);
  * **the gate test's PATH:** links to the real tools the gate needs
    (`bash sh env git dirname basename mkdir mktemp tail cut cat rm
    sort`) and the stubs, nothing else, so a missing tool is truly
    missing.
* **Green:**
  * `gate_test.sh`: 10 passed;
  * `check-docs_test.sh`: 11 passed;
  * `plant-copy_test.sh`: 10 passed;
  * shellcheck 0.11.0 on `scripts/*.sh`: clean;
  * `dash -n` on the three tests: clean.
* **Plants,** each in a scratch copy, each caught:

  | Plant | Test | First failure |
  | :--- | :--- | :--- |
  | gate: `[ "$rc" -eq 0 ] \|\| overall=1` → `true` | `gate_test.sh` (5 failed) | `FAIL lint: rc=0` |
  | gate: `ROOT=$HOME` | `gate_test.sh` (4 failed) | `FAIL all green: rc=1` |
  | check-docs: slug keeps case | `check-docs_test.sh` (1 failed) | `a.md:7: missing anchor: b.md#the-heading` |
  | check-docs: home path in any case | `check-docs_test.sh` (2 failed) | `clean.md:2: home directory path` |
  | check-docs: prints the match | `check-docs_test.sh` (1 failed) | the fixture's identifier printed |
  | plant-copy: `if n != 1:` → `if n == 0:` | `plant-copy_test.sh` (1 failed) | `FAIL twice: rc=0` |

* **Other files:**
  * `Makefile`: `gate` in `.PHONY`, and its target;
  * `ci.yml`: the step "Verify the gate and document tools";
  * `AGENTS.md`: a `make gate` paragraph in "Pre-add checks";
  * `docs/architecture.md`: the three scripts in the tree, a `make gate`
    and a `plant-copy.sh` bullet in Tooling, and the CI step;
  * `docs/README.md`: a "run every check before a commit" row;
  * the dated note in the rules of the 0010-PLAN-v1-5-1, 0010-PLAN-v1-6-0,
    0011, 0012, 0013 and 0014 PLANs, inserted by a script that asserted
    each place. 0013's note adds the `simulate_build.sh` sentence.
* **Checks:**
  * actionlint v1.7.12: clean;
  * `check-workflows.sh --rule expressions|permissions|pins` on `ci.yml`:
    ok;
  * markdownlint-cli2 0.23.2: 0 issues;
  * `check-docs.sh --links` on the changed Markdown: 106 links, 0 broken;
  * `check-docs.sh --ids` on the changed and new files: 0 findings;
  * `--links` over every tracked Markdown file: 308 links in 49 files, 0
    broken. The session checker counted 305 at `d6a570f`; the new one
    also reads `#fragment`-only and fenced-code boundaries its own way.
  * **`make gate`,** on `cdc5ebc` with R1's changes, every step `rc=0`,
    `overall=0`:
    * gofmt "clean"; lint "0 issues."; vet; race; shuffle; tidy;
    * apicheck "compatible with v1.10.0"; fuzz, all nine targets clean;
      vuln "No vulnerabilities found.";
    * scripts "all script tests passed", including the three new ones;
      shellcheck; crossvet "cross go vet clean";
    * links "317 links in 49 files, 0 broken"; ids "18 files, 16 deny-list
      rules, 0 findings".

    The first gate run, before P0, is deviation D1's.
* **Acceptance:**
  * `make gate` ends `overall=0`, and each test's plants fail it;
  * the link check over every tracked Markdown file finds nothing;
  * CI's new step runs on the next push.

  From here on, the phase procedure uses `make gate` and
  `scripts/plant-copy.sh`.
* Committed by the owner as `fdc5a89`.

### Phase R2: roadmap and record amendments (2026-10-07)

* **0004-MADR, `### P3 (2026-10-07): Phase 4 is built; the open work in
  one place`,** after P2:
  * the Phase 4 table (0011 `v1.7.0`, 0012 `v1.8.0`, 0013 `v1.9.0`, 0014
    `v1.10.0`), superseding P2's "none of it is built" bullet without
    editing P2;
  * §1's rows as built: `cli`, `archive`, `codesign`, `service`, and the
    new `releasespec` and the internal release tool. Each dependency was
    read from `.golangci.yml`'s depguard allow-list;
  * the open-work table, eleven rows, each with its record and line.
    Every line was checked against the file after the edits. The 0011
    "replace before stop" citation moved from `:884` to `:909` when A6
    was inserted above it, and was corrected.

  The items 0012 §8 and 0013 §10 list that later releases built (archives
  through the publish workflow, `v1.9.0`; the installer templates,
  `v1.10.0`) are marked done, and §6's prerelease channels point to 0005.
* **`docs/README.md`:** the "see where `selfupdate` is going" row reads
  "…, and what is still open", and links to P3's anchor. The link check
  resolved the anchor.
* **0011-MADR `### A6 (2026-10-07): the rollback is reported in the
  result`,** after A5. **0011-PLAN `### Deviation D8 (2026-10-07)`** goes
  at the end of its record, after "This PLAN is `complete`", rather than
  after D7, so the record keeps its order. The PLAN stays `complete`.
* **0010-MADR:** a dated, italic annotation on the "Every `Example` has an
  output check" bullet (`:187`) names the three Examples that cannot have
  one, and `ExampleNewUnpacker`, which gains its check in P4.
* **Checks:**
  * `check-docs.sh --links` on the five changed records: 131 links, 0
    broken;
  * `--ids`: 0 findings;
  * markdownlint on `docs/README.md`: 0 issues;
  * `make gate` on `fdc5a89` with R2's changes: all 14 steps `rc=0`,
    `overall=0`. apicheck "compatible with v1.10.0"; fuzz clean; vuln "No
    vulnerabilities found."; links "330 links in 49 files, 0 broken"; ids
    "6 files, 16 deny-list rules, 0 findings".
* Committed by the owner as `8607cef`.

### Phase R3: documentation (2026-10-07)

* **C5,** a pin test: `terminal_event_test.go` `TestRunEndsAtSelected`.
  Its three subtests:
  * a check that finds an update (`ErrUpdateAvailable`, `upgrade`);
  * an up-to-date check (`none`, no error);
  * an up-to-date apply.

  Each ends at `selected`, with no `complete`, `failed` or `declined`. Its
  first run failed on the test's own mistake, not the code: it set `Yes`
  on the checks, which the library refuses as "--check and --yes are
  contradictory". With `Yes` set only for the apply, it passes on the
  unchanged code, as a pin test must.
  * **Plant** (`scripts/plant-copy.sh`): a `complete` reported before the
    up-to-date apply's return in `updater.go`. Its failure:
    `TestRunEndsAtSelected/up-to-date_apply: events [resolving-target
    fetching-release selected complete]: want the last to be selected`.
  * **Docs:** `doc.go`'s Reporter paragraph; the `EventSelected` and
    `EventComplete` comments in `types.go`; the extending guide, "Read
    JSON output"; `docs/architecture.md`, the coordinator bullet.
* **G8:**
  * the migration guide's "still publishes bare binaries only" now says
    archives are built and published since `v1.9.0`, linking the building
    guide's §7;
  * "six whole runs" → "seven" (the seven `jsonl-*` and `text-*` pairs in
    `selfupdate/testdata/golden/`);
  * the docs index's crasher row names the owning package's
    `testdata/fuzz/<Name>/`.
* **G9, `docs/architecture.md`:**
  * `release-latest-flag.sh` in the tree;
  * the cache file is schema 3, read from `checkRecordSchema` and
    `oldestCheckRecordSchema` (both 3) in `checkcache.go`;
  * build step 4 adds `GOWORK=off`, as `gotool.go:35` sets it;
  * publish step 7 states the backport rule;
  * the CI list gains the live tests by OS with their `SELFUPDATE_REQUIRE_*`
    variables, the latest-release rule's test and the pre-add gate's test;
  * "the gate's own test" became "the API gate's test", as the new `make
    gate` would otherwise be confused with it.
* **F3:** building guide step 12 states the `v1.10.0` floor; the migration
  guide's "Move both" bullet adds "Until `v1.11.0`, the build workflow does
  not check this."
* **F4:**
  * building guide step 4 gains **Before the first release**: the
    immutable-releases setting, and what happens without it;
    attestations' prerequisite;
  * `README.md`: the publish bullet names both; "Its tag can never be
    reused" is now conditioned on the setting; a new "When a publish fails"
    bullet covers a timeout with the setting off, with the `gh release
    delete` command.
* **Checks:**
  * `check-docs.sh --links` on the six changed Markdown files: 147 links,
    0 broken;
  * markdownlint: three MD004 errors (`*` list markers in the new CI
    sub-list), fixed to `-`, then clean;
  * `make pre-add-check` on `doc.go`, `types.go` and the test: "3 file(s)
    clean";
  * `make gate` on `8607cef` with R3's changes: all 14 steps `rc=0`,
    `overall=0`; links "331 links in 49 files, 0 broken"; ids "9 files, 16
    deny-list rules, 0 findings".
* Committed by the owner as `4d28fe4`.

### Phase P1: the High findings and what they rest on (2026-10-07)

* **Before the red,** one refactor that changes no behaviour: a new seam,
  `syncRootFn = syncRoot` (`replace.go`), called by `rollbackInRoot`, so
  the B4 tests can fail the in-root sync.
* **Red,** the core tests on the unfixed code, each failing as below. No
  existing test changed its result:
  * `TestSecondCommitOrRollbackRefused`, all four subtests:
    * "commit after rollback: Applied=true err=selfupdate: remove backup:
      …";
    * "rollback after commit: Applied=false err=selfupdate: restore
      backup: rename …";
    * the same for twice each.
  * `TestManagedRecoveryReportsRestoredBinary`, both subtests:
    `RolledBack=false Backup=""`.
  * `TestInstallReportsRestoreAfterSyncFailure`:
    * after a failed sync: `RolledBack=false Backup=""`;
    * unsynced in the locked directory: `RolledBack=false
      Backup=".../.demo.selfupdate-bak-678316334"`, a file the rename had
      already consumed.
  * `TestKeptBackupSurvivesLaterSessions`, all four subtests: "backup
    .demo.selfupdate-bak-<n> after a later session: "", open …: no such
    file or directory".
  * `TestDryRunSweepsNothing`: ".demo.selfupdate-1234567 was removed" and
    ".demo.selfupdate-bak-7654321 was removed".
  * `TestManagedStopFailsAfterStoppingRestarts`: "stops=1 starts=0
    healths=0".
  * Passing as planned: `TestManagedStopFailsStillRunning` (the control)
    and the new `TestIsLeftover` rows (a pin).
  * The first red run of `TestKeptBackupSurvivesLaterSessions` failed two
    subtests on the test's own setup. They reused the target identity
    resolved before the first install replaced the binary, and `Begin`
    rightly refused: "target changed during confirmation: concurrent
    update". The later sessions now resolve the target again, as a later
    run does.
* **Red, the backends** (`stopwait_test.go` in each), on the unfixed
  backends:
  * launchd `TestStopFinishesAfterCancel`: "verbs [managername list list
    bootout print], loaded true: Stop returned while the job was still
    loaded";
  * systemd: "verbs [show stop show]; want three probes after the stop";
  * SCM: "Stop = selfupdate: service: timed out: demo STOP_PENDING:
    context canceled".

  `TestManagedStopTimeoutRestartsJob` goes through the core, which was
  already fixed. Its red is the core plant below, which fails it.

  `TestStopBound`, `TestStopWaitFollowsExitTimeOut`, `TestParseTimespan`
  and `TestPlistValueRealPlutil` name functions the fix adds. Their red is
  the build failing, and their plants prove them.
* **Fix:**
  * **B7** (`session.go`): `replacement{sess, applied, finished}` as the
    State; `stateOf` replaces `appliedState`; `errReplacementFinished`.
    `Commit` finishes once `commitLocked` ran. `Rollback` finishes on
    success or `errRestoredUnsynced`. `Apply` finishes a failure that left
    no live backup.
  * **B4:**
    * `errProbeRolledBack` was renamed `errRolledBack` with `gofmt -r`;
    * `rollbackInRoot` joins `errRestoredUnsynced`;
    * `Install` reports `RolledBack` for `errRolledBack` and
      `errRestoredUnsynced`;
    * `Apply` joins `errRolledBack` to an undo;
    * both `replaceTarget`s join it when the restore after a failed sync
      succeeded;
    * `recover` counts an unsynced `Rollback` as rolled back.
  * **B1:**
    * `keptName` (`leftovers.go`);
    * `retainLocked` on every path that returns a live backup with
      `Applied` false, and after a failed `Rollback`;
    * `backupOf` reads the State's current name in `recover`;
    * `dryRunKey`, set by the updater for `req.DryRun`; `beginSession`
      then skips the receipt and the sweep.

    `isLeftover` needed no change: it already matches no kept name.
  * **B3, core:** `recoverStop` (`managed.go`) re-probes `Running` under a
    recovery context. When the service is down it runs `recover` with
    `restart`. Otherwise, a stop refused inside the service included, the
    update ends as before. The core matches no error text.
  * **B3, launchd:**
    * `plistValue` (new `plist.go`): `-convert xml1`, a dictionary root,
      `-type`, `-extract raw` for bool and integer;
    * `stopBound` = max(poll timeout, ExitTimeOut, 5 s by default, plus
      `stopGrace`, 30 s);
    * `waitGone` polls under `context.WithoutCancel` with the bound as
      its timeout, and joins the caller's error.

    A first version also set the bound as the wait context's deadline.
    That deadline beat `PollHealthy`'s own timeout, and
    `TestStopTimesOut` saw "deadline exceeded" rather than `ErrTimeout`.
    `PollHealthy` alone now bounds it.
  * **B3, systemd:** `parseTimespan` (new `timespan.go`); `stopBound` =
    max(poll timeout, `TimeoutStopUSec` + 30 s), read in `Stop`; the
    `stop` command and `waitState` (which takes the bound) under
    `WithoutCancel`; a context already ended before the stop returns at
    once.
  * **B3, SCM:** the deadline is derived from `WithoutCancel`, and the
    caller's error is joined on return.
* **Green:**
  * every package of `./selfupdate/...` passes;
  * the eight core tests and the backend tests pass;
  * Windows and Linux vet of the changed packages: clean.
* **Plants,** 15, each in a `scripts/plant-copy.sh` copy, each caught:

  | Plant | Fails |
  | :--- | :--- |
  | `Commit` skips the finished check | `TestSecondCommitOrRollbackRefused` (rollback then commit, commit twice) |
  | `recover` ignores an unsynced restore | `TestManagedRecoveryReportsRestoredBinary/unsynced_rollback` |
  | `Apply`'s undo returns the directory error alone | `…/undone_apply` |
  | `rollbackInRoot`'s sync error unmarked | `TestInstallReportsRestoreAfterSyncFailure/rolled_back_in_the_locked_directory,_unsynced` |
  | `retainLocked` keeps the backup's name | `TestKeptBackupSurvivesLaterSessions`, all four |
  | a dry run sweeps | `TestDryRunSweepsNothing` |
  | a kept name read as a backup's | `TestIsLeftover` |
  | a failed stop ends the update (the old line) | `TestManagedStopFailsAfterStoppingRestarts`, launchd `TestManagedStopTimeoutRestartsJob` |
  | launchd bound is the poll timeout | `TestStopBound`, `TestStopWaitFollowsExitTimeOut` |
  | launchd wait ends with the caller | launchd `TestStopFinishesAfterCancel` |
  | launchd plist root not checked | `TestPlistValueRealPlutil`, `TestStopBound` |
  | systemd wait ends with the caller | systemd `TestStopFinishesAfterCancel` |
  | systemd bound is the poll timeout | systemd `TestStopBound` |
  | SCM wait ends with the caller | SCM `TestStopFinishesAfterCancel` |
  | live: launchd bound is the poll timeout | `TestLiveStopWaitsForSlowExit`: "Stop after 3.19s: … timed out" |

  The dry-run plant was run again after a lint fix changed its line
  (`isDryRun` checks the assertion's `ok`), and was caught again.
* **Live, this Mac:** `TestLiveStopWaitsForSlowExit`, now with
  `ExitTimeOut` 8 and `Poll.Timeout` 3 s, passed in 8.91 s. The job was
  gone afterwards (`launchctl print` exit 113). The PLAN's "20 and 5 s"
  became 8 and 3 s: the same relation (the kill bound beyond the poll
  timeout), in less time.
* **Placement:** the backend B3 tests are in a new `stopwait_test.go` in
  each backend, not in `launchd_test.go`, `systemd_test.go` and
  `scm_test.go`. Their imports differ, and they stay together.
  `TestStopTimesOut` sets `ExitTimeOut` to 0 in the fake's plist, rather
  than changing package seams, so its 100 ms poll still bounds it.
* **Docs:**
  * `types.go`: `PendingBackup`, `InstallResult.Backup`, and
    `AppliedReplacement`, single-use;
  * `doc.go`: a dry run removes no leftovers;
  * `leftovers.go`'s header;
  * the launchd `Stop` comment and the three `Poll` comments;
  * the extending guide: "Keep the previous binary" (the kept backup),
    and "Run as a service" (the stop wait's bound, and a failed stop's
    restart);
  * `docs/architecture.md`, the install-path bullets.
* **Checks:**
  * `make pre-add-check` on the 31 changed Go files: "31 file(s) clean".
    The first run failed on `errcheck` at `isDryRun`'s `v, _ :=` type
    assertion; fixed.
  * The full apidiff report against `v1.10.0` (V3) holds only "Ignoring
    internal package …": no exported change.
  * markdownlint: 0 issues. `check-docs.sh --links`: 0 broken.
  * `make gate` on `4d28fe4` with P1's changes: all 14 steps `rc=0`,
    `overall=0`. race and shuffle ok; apicheck "compatible with v1.10.0";
    fuzz clean; vuln "No vulnerabilities found."; links "331 links in 49
    files, 0 broken"; ids "34 files, 16 deny-list rules, 0 findings".
* **Not yet run:** the systemd and SCM live tests (rule 9). They run with
  P5's, on the test hosts and in CI, before the `v1.10.1` tag (P8); CI's
  three legs run the existing live tests on the push of this commit.
