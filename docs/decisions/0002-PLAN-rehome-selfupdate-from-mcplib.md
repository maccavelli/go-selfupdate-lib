---
status: complete
date: 2026-09-30
associated-madr: "0002-MADR-rehome-selfupdate-from-mcplib.md"
---
# Implement the re-home of `selfupdate` and its release tooling into go-core-lib

Associated MADR: [0002-MADR-rehome-selfupdate-from-mcplib.md](0002-MADR-rehome-selfupdate-from-mcplib.md)

## Goal

`github.com/maccavelli/go-core-lib/selfupdate` is `mcplib` `v1.6.0`'s
package, changed only as MADR §3 allows. The reusable release workflow and
its scripts run from this repository. Lint covers linux, darwin and windows.
The result, together with the 0001 scaffold, is pushed and tagged `v1.0.0`,
with CI green on all three operating systems.

## Scope

### In scope (this repository only)

| Path | Phase |
|---|---|
| `selfupdate/**` (49 files) | 1, 2 |
| `go.mod`, `go.sum` | 1 |
| `Makefile` (`lint` target), `scripts/go-precheck.sh` (step 2), `AGENTS.md` ("Pre-add checks") | 3 |
| `scripts/verify-selfupdate-release.sh`, `…_test.sh`, `scripts/refuse-existing-release.sh`, `…_test.sh`, `scripts/check-workflow-gh-repo.sh` | 4 |
| `.github/workflows/publish-selfupdate-release.yml` | 4 |
| `.github/workflows/ci.yml` | 3, 4 |
| `README.md`, `docs/README.md`, `docs/architecture.md`, `docs/guides/migrating-from-mcplib-selfupdate.md` | 0, 5 |
| `docs/decisions/0002-*` | all |

### Out of scope

* Any change in `mcplib`, `go-llmprovider-sdk` or a consumer (MADR §9).
* Moving any dependency version (MADR §4).
* Any exported API change (MADR §3).
* The records of `mcplib`, and its 0005 open items (MADR §2).

### Fixed inputs

* Source: `mcplib` at tag `v1.6.0`, commit `4e1f9a53e265`. Read it with
  `git -C ../mcplib show v1.6.0:<path>` or `git archive`, never from its
  working tree, which carries someone else's uncommitted edits to its own
  `0015` records.
* Go 1.27.1, `golangci-lint` 2.13.2, `govulncheck` v1.7.0, `shellcheck`,
  `markdownlint-cli2`, `python3`, and `gh` (Phase 6 only).
* Every cross-target command sets `CGO_ENABLED=0`, because this host's
  `go env` sets `CGO_ENABLED=1`.

## Implementation Steps

Every phase ends with its checks passing, `git add` of exactly its paths,
and `git commit --no-edit`. Long outputs go to scratch files with `$?`
captured before any filter.

### Phase 0: accept the records

1. The owner approves. Set the MADR `accepted` and this PLAN `in-progress`.
2. Add the two 0002 rows to the `docs/README.md` record index.
3. Commit the two records and `docs/README.md`.

### Phase 1: copy the package, change only the import path and `doc.go`

1. Extract `selfupdate/` from `mcplib` `v1.6.0` with
   `git -C ../mcplib archive v1.6.0 selfupdate | tar -x -C .`. Assert 49
   files.
2. Rewrite `github.com/maccavelli/mcplib/selfupdate` to
   `github.com/maccavelli/go-core-lib/selfupdate`. Assert exactly one
   occurrence, in `example_test.go`, before and after.
3. Edit the `doc.go` package comment as MADR §3 says: "fleet programs"; the
   workflow "at the exact go-core-lib module-tag commit".
4. `go get golang.org/x/mod@v0.40.0 golang.org/x/sys@v0.47.0 golang.org/x/term@v0.43.0`,
   then `go mod tidy`. Assert that `go.mod` has exactly those three direct
   requirements, and that `go.sum`'s `h1:` lines equal `mcplib` `v1.6.0`'s
   lines for them.
5. Checks:
   * **G-diff.** `diff -r <v1.6.0 archive>/selfupdate selfupdate` shows only
     the `example_test.go` import line and the `doc.go` comment.
   * **G-api.** `go doc -all ./selfupdate` here and in a `v1.6.0` archive,
     each written to a scratch file. Their `diff` shows only the package
     comment.
   * `go test -race -count=1 -coverprofile=… ./...` passes. Record the
     coverage against 75.5 %.
   * Host `make lint`, `make vuln`, `go mod tidy -diff` and
     `make pre-add-check`.
   * `CGO_ENABLED=0 GOOS=<t> go test -c -o /dev/null ./selfupdate` for
     linux, darwin and windows.
6. Commit. The staged `selfupdate/` is the verbatim copy plus the two
   changes, so the commit itself is the provenance record.

### Phase 2: the four Windows lint fixes

1. Make exactly MADR §3's four edits: `root.ReadFile(name)`;
   `windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token)`;
   `osRename` moved to `replace_unix.go`; `applyResult.pendingBackup`
   deleted.
2. Checks:
   * `CGO_ENABLED=0 GOOS=<t> golangci-lint run -c .golangci.yml ./...`
     gives `0 issues.` for linux, darwin and windows. Before this phase,
     `GOOS=windows` reports the four findings. Record both outputs.
   * `CGO_ENABLED=0 GOOS=<t> go vet ./...` and `go test -c` for the three
     targets.
   * `go test -race -count=1 ./...` passes, with coverage recorded.
   * **G-api** is re-run; it is unchanged. **G-diff** now also shows exactly
     the four edits.
3. Commit.

### Phase 3: cross-target lint in `make lint`, the pre-add gate and CI

1. `Makefile` `lint`: loop over `linux darwin windows`, running
   `CGO_ENABLED=0 GOOS=$$os $(GOLANGCI_LINT) run -c $(FLEET_LINT_CFG) ./...`.
   Fail if any run fails, and name the failing target.
2. `scripts/go-precheck.sh` step 2: the same loop, each run's output under
   `golangci-lint (GOOS=<t>):`. The comments say why: MADR §5.
3. `ci.yml`: no change is needed for lint, since it already runs
   `make lint`. Assert that by reading the workflow.
4. Update `AGENTS.md` "Pre-add checks" to say the lint step runs for three
   targets, and remove its "until the first package lands" paragraph.
5. `shellcheck scripts/go-precheck.sh` exits 0.
6. **First-fail experiment,** on a scratch clone at this phase's working
   state:
   * with Phase 2 reverted in the clone (`git revert --no-edit`, scratch
     only), `make lint` and `make pre-add-check` fail, naming
     `GOOS=windows` and all four findings;
   * with each fix reverted alone, that finding alone is reported;
   * on the fixed clone, both pass.

   Record each exit code and its failure line. Time `make lint` before and
   after the loop (MADR "Consequences").
7. Commit.

### Phase 4: the release tooling

1. Extract the five scripts and the workflow from `v1.6.0` the same way,
   keeping the scripts' `0755` modes.
2. Workflow: `.mcplib-release-tools` becomes `.core-lib-release-tools`
   (assert 5 occurrences before and 0 after). Remove the `bridge-release`
   input, its `BRIDGE` env line and the `--bridge` argument. Make citations
   repository-named.
3. `verify-selfupdate-release.sh`: remove `--bridge` from the usage, the
   argument parser and the Python checks (the compatibility set,
   `SHA256SUMS-0.16.0` and the alias comparison). Make citations
   repository-named.
4. `verify-selfupdate-release_test.sh`: remove the four bridge cases. Add
   one case asserting that `--bridge true` is now a usage error (exit 2).
5. `refuse-existing-release.sh`, its test and `check-workflow-gh-repo.sh`:
   citations only.
6. `ci.yml`: add `./scripts/verify-selfupdate-release_test.sh` on Linux.
   Add `./scripts/refuse-existing-release_test.sh` and
   `./scripts/check-workflow-gh-repo.sh` on every OS, as in `mcplib`'s CI.
7. Checks:
   * the three tests pass; `shellcheck scripts/*.sh` exits 0; both YAML
     files parse with PyYAML;
   * **G-diff for tooling:** `diff` of each file against `v1.6.0` shows only
     steps 2–5;
   * `git grep -n -i 'mcplib' -- scripts .github selfupdate` shows only the
     repository-named citations;
   * **first-fail**, on scratch copies: a verifier that no longer rejects a
     missing `SHA256SUMS` fails its test; a workflow with `GH_REPO` removed
     from one `gh release` step fails `check-workflow-gh-repo.sh`; a `gh`
     stub that returns an unrecognised error fails the refuse test's
     fail-closed case.
8. Commit.

### Phase 5: documentation

1. `README.md`: status (the first package, releases from `v1.0.0`), a
   `selfupdate` section carrying the substance of `mcplib`'s README
   "Self-update" section, the reusable-workflow contract, and "I want to…"
   rows.
2. `docs/architecture.md`: the package, the tooling, the three-target lint
   and the new CI steps, with "What is not here" updated.
3. `docs/guides/migrating-from-mcplib-selfupdate.md`, per MADR §7. Its
   `uses:` SHA is a placeholder that names the `v1.0.0` commit and is filled
   in Phase 6.
4. `docs/README.md`: "I want to…" rows for the guide and the MADR.
5. Checks: `markdownlint-cli2` clean over non-record Markdown; the proven
   link resolver over every non-record Markdown file; identifier scan over
   the tree.
6. Commit. **This is the `v1.0.0` commit** (MADR §8).

### Phase 6: push, CI, tag `v1.0.0` (owner-run; each mutation needs the owner's same-turn ask)

1. Run the pre-push disclosure guard from `AGENTS.md` over
   `origin/main..main`. That covers every 0001 and 0002 commit.
2. **On the owner's ask:** `git push origin main`. Watch CI (`gh run watch`)
   until it finishes. It must pass on ubuntu-24.04, macos-15 and
   windows-2025. That is the first run of the Windows tests for the Phase 2
   fixes (MADR "Confirmation"). A failure stops the phase for a decision.
3. **On the owner's ask:** `git tag v1.0.0 <Phase 5 commit>` and
   `git push origin v1.0.0`. The tag's CI must pass.
4. `GOPROXY=https://proxy.golang.org go list -m github.com/maccavelli/go-core-lib@v1.0.0`
   resolves. Record the output.
5. Fill the guide's `uses:` SHA with the `v1.0.0` commit, set this PLAN
   `complete`, and commit. It is pushed on the owner's ask.

## Verification

* **V1. Same code.** G-diff and G-api outputs recorded, showing only MADR
  §3's changes.
* **V2. Module.** Three direct requirements at `mcplib`'s pins, `go.sum`
  `h1:` lines equal to `mcplib`'s, and `go mod tidy -diff` clean.
* **V3. Tests.** `go test -race` passes, with coverage recorded against
  75.5 %. The Windows leg passes in Phase 6.
* **V4. Lint on three targets.** Clean, and proven to fail with each fix
  reverted.
* **V5. Release tooling.** The three tests pass, and each was seen to fail
  on its planted input. `shellcheck` is clean. No `bridge` remains.
* **V6. Docs.** markdownlint and links are clean, and the resolver was
  proven first.
* **V7. Identifiers.** No account name, hostname or real-machine path in
  the tree.
* **V8. Release.** CI is green on three operating systems at the pushed
  `main` and at `v1.0.0`, and the proxy resolves `v1.0.0`.

## Rollout and Rollback

* **Rollout.** Phases 0–5 are local commits. Phase 6 pushes the 0001 and
  0002 commits together (0001 MADR §6) and tags `v1.0.0`, each on the
  owner's explicit ask. Consumers migrate later, under their own records.
* **Rollback before the push.** `git revert` of the phase commit, here only.
* **Rollback after the push, before the tag.** Fix forward with a new
  commit, and re-run CI.
* **After `v1.0.0`.** Never move or delete the tag. Fix in `v1.0.1`.
  Nothing consumes the module until a consumer's own record pins it.

## Execution Record

### Phase 0: accept the records (2026-09-29)

The owner approved: "Proceed, drop bridge release and fix windows
findings". The MADR is `accepted` and this PLAN is `in-progress`. The two
0002 rows are added to `docs/README.md`.

Commit: `49affdd`.

### Phase 1: copy the package (2026-09-29)

* **Extraction.** `git -C ../mcplib archive v1.6.0 selfupdate | tar -x`
  wrote 49 files, and `diff -r` against a separate `v1.6.0` reference
  archive was empty before the edits. There was exactly one
  `github.com/maccavelli/mcplib/selfupdate` occurrence (`example_test.go:10`)
  before the rewrite, and none after.
* **Module.** `go get` of the three pins, then `go mod tidy` (exit 0), gave
  three direct requirements: `golang.org/x/mod v0.40.0`,
  `golang.org/x/sys v0.47.0` and `golang.org/x/term v0.43.0`. The six
  `go.sum` lines are identical to `mcplib` `v1.6.0`'s lines for those
  modules. `go mod tidy -diff`: exit 0.
* **G-diff** against the `v1.6.0` reference (exit 1, as expected) shows
  three changed lines: `doc.go:2` (`mcplib consumers.` → `fleet
  programs.`), `doc.go:16` (`mcplib` → `go-core-lib`), and
  `example_test.go:10` (the import path).
* **G-api.** `go doc -all ./selfupdate`, 612 lines each side. The diff has
  two parts: the package comment's two sentences (as MADR §3 allows), and
  the header line
  `package selfupdate // import "github.com/maccavelli/go-core-lib/selfupdate"`.
  That is the module path MADR §1 decides, not an API change. No
  identifier, signature or other doc comment differs.
* `go test -race -count=1 ./...`:
  `ok github.com/maccavelli/go-core-lib/selfupdate 3.880s coverage: 75.5% of statements`.
  That is the same coverage as `mcplib`'s own run.
* `make lint` (host): `0 issues.` `make vuln`:
  `No vulnerabilities found.` `make pre-add-check`:
  `go-precheck: 47 file(s) clean (gofmt, golangci-lint, go vet, go test, govulncheck).`
* `CGO_ENABLED=0 GOOS=<t> go test -c -o /dev/null ./selfupdate`: exit 0 for
  linux, darwin and windows.
* **Before Phase 2** (recorded now, on this copy):
  `CGO_ENABLED=0 GOOS=<t> golangci-lint run` exits 0 for linux and darwin.
  For windows it exits 1 with `4 issues:` at `cleanup_windows.go:54:15`
  G304, `cleanup_windows.go:123:16` SA1019, `replace.go:11:2` unused
  `osRename`, and `replace_windows.go:16:2` unused `pendingBackup`.

Commit: `2b7fdae`.

#### Deviation D1 (2026-09-29): the Phase 1 commit message does not name the source

* **Found.** MADR §2 says "the first commit that adds them names that tag
  and commit". The global `prepare-commit-msg` hook writes every message,
  and `2b7fdae`'s says "Copy the self-update implementation and tests into
  `selfupdate`" without naming `v1.6.0` or `4e1f9a53e265`. The repository's
  rules forbid `--amend` here, except for an identifier fix.
* **Resolution.** The provenance is carried by the records instead, which
  is where MADR §2 now puts it (amended the same day). `2b7fdae`'s own diff
  adds this execution record, which names `v1.6.0`. This PLAN's "Fixed
  inputs", committed in `49affdd`, names `4e1f9a53e265`.
  `docs/architecture.md` (Phase 5) states both. The message is not
  rewritten. If the owner asks, it can be amended before the Phase 6 push,
  because nothing here has been pushed.

### Phase 2: the four Windows lint fixes (2026-09-29)

* **Edits,** exactly MADR §3's four:
  1. `cleanup_windows.go:54`: `root.ReadFile(name)`. The only caller,
     `session.go:154-166`, opens `root` on `original.Dir` and passes
     `original` as `target`, so this reads the same file as
     `filepath.Join(target.Dir, name)`.
  2. `cleanup_windows.go:123-126`:
     `var token windows.Token` /
     `if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token); err != nil { return err }`.
     That is the body of the deprecated `OpenCurrentProcessToken` in
     `golang.org/x/sys@v0.47.0/windows/security_windows.go:658-662`.
     A first draft, `if err = …`, raised a new gocritic `sloppyReassign`
     finding under `GOOS=windows`. It was changed to the scoped `:=`. The
     explicit `return err` still sets the named result, and the deferred
     `token.Close()` runs only after a successful open, as before.
  3. `osRename` moved from `replace.go`'s `var` block to `replace_unix.go`,
     its only user. No test references it.
  4. `applyResult.pendingBackup` deleted from `replace_windows.go`. No code
     read or wrote it. The pending backup is `commitReplacement`'s return
     value.
* `gofmt -l selfupdate`: empty.
* `CGO_ENABLED=0 GOOS=<t>`: `golangci-lint run` gives `0 issues.`, and
  `go vet ./...` and `go test -c` exit 0, for linux, darwin and windows.
  Before this phase windows had `4 issues:` (Phase 1 record).
* `go test -race -count=1 -cover ./...`:
  `ok … 3.499s coverage: 75.5% of statements`, unchanged.
* **G-api** is byte-identical to Phase 1's. **G-diff** now shows the
  Phase 1 lines plus exactly these edits: `cleanup_windows.go` 54 and
  123-124, `replace.go` 11 (removed), `replace_unix.go` 10-11 (added), and
  `replace_windows.go` 15-17 → 15-16.
* `make pre-add-check`: `47 file(s) clean`.
* The Windows tests that exercise fixes 1 and 2
  (`TestWindowsCleanupReceiptRoundTrip`,
  `TestWindowsCleanupReceiptDigestMismatch`) compile here and run first on
  the Windows CI leg in Phase 6.

Commit: `71d9e1d`.

### Phase 3: cross-target lint (2026-09-29)

* `Makefile`: `LINT_GOOS := linux darwin windows`. `lint` runs
  `CGO_ENABLED=0 GOOS=$$os golangci-lint run -c .golangci.yml ./...` for
  each, prints `golangci-lint (GOOS=<t>)` before each run and
  `golangci-lint failed for GOOS=<t>` on a failure, and exits non-zero if
  any run failed.
* `scripts/go-precheck.sh` step 2: the same three runs, each failure under
  `golangci-lint (GOOS=<t>):`. `shellcheck`: exit 0.
* `ci.yml` needed no change: its Linux step already runs `make lint`
  (`ci.yml:29`).
* `AGENTS.md` "Pre-add checks" now describes the three runs and the explicit
  `CGO_ENABLED=0`. The "until the first package lands" paragraph is removed.
  `markdownlint-cli2 AGENTS.md`: exit 0.
* `make lint`: `0 issues.` for each target. `make pre-add-check`:
  `47 file(s) clean`.
* **Timing** (`/usr/bin/time -p make lint`, after
  `golangci-lint cache clean`, then warm):

  | | Cold | Warm |
  |---|---|---|
  | host only (before) | 4.65 s | 0.94 s |
  | three targets (after) | 13.86 s | 2.64 s |

* **First-fail experiment,** on a scratch clone with this phase's `Makefile`
  and script, no commits made in the clone:
  * `selfupdate/` at `2b7fdae` (all four fixes absent): `make lint` exited 2
    with `golangci-lint failed for GOOS=windows` and the four findings.
    `go-precheck.sh` exited 1 and listed them under
    `golangci-lint (GOOS=windows):`.
  * Each fix reverted alone (`revert_fix.py`, scratch only): `make lint`
    exited 2 with `failed for GOOS=windows`, reporting only that fix's
    finding. Fix 1 gave G304 `cleanup_windows.go:54:15`, fix 2 SA1019
    `:123:16`, fix 3 unused `osRename` `replace.go:11:2`, and fix 4 unused
    `pendingBackup` `replace_windows.go:16:2`.
  * The fixed tree: `make lint` exit 0, and `go-precheck.sh` exit 0,
    `47 file(s) clean`.

  The clone was deleted afterwards. By contrast, Phase 1's host-only
  `make lint` passed the unfixed code with `0 issues.`

Commit: `80820b8`.

### Phase 4: the release tooling (2026-09-29)

* **Extraction.** `git -C ../mcplib archive v1.6.0 <five scripts> <workflow> | tar -x`.
  All five scripts are `-rwxr-xr-x`.
* **Edits** (`phase4_edits.py`, scratch, with every target's count asserted
  before replacement):
  * **Workflow.** `.mcplib-release-tools` becomes `.core-lib-release-tools`
    (5 occurrences before, 0 after). The `bridge-release` input, the
    `BRIDGE:` env line and `--bridge "$BRIDGE"` are removed, and so is
    `--repository` (D2). The two "MADR 0007" comments cite
    `mcplib docs/0007-MADR-restore-repository-context-in-the-reusable-release-workflow.md`.
  * **Verifier.** `--bridge` and `--repository` (D2) are removed from the
    usage, the variables, the parser and the Python `argv`, along with the
    compatibility set, `SHA256SUMS-0.16.0` and the alias comparison. The
    strict `--tag` check is kept, as
    `if tag and not tag_re.match(tag): fail(…)`. Diff against `v1.6.0`:
    48 lines removed, 4 added.
  * **Verifier test.** The four bridge cases are removed. Added: a
    `run_usage` helper (asserts exit 2), `--bridge` and `--repository`
    usage cases, and a strict/non-strict `--tag` pair (D2).
  * **`refuse-existing-release.sh`, its test and
    `check-workflow-gh-repo.sh`:** citation text only.
* **`ci.yml`:** `verify self-update release fixtures` on Linux, and
  `Verify the release guard and its workflow contract` on every OS, both
  with `shell: bash` (D2).
* **Checks:**
  * `verify-selfupdate-release_test.sh` passes 9 of 9 cases.
    `refuse-existing-release_test.sh`: `6 passed, 0 failed`.
    `check-workflow-gh-repo.sh`:
    `ok — every repository-scoped gh step sets GH_REPO`.
  * `shellcheck scripts/*.sh`: exit 0. PyYAML loads `ci.yml` and the
    reusable workflow.
  * `grep -rn -i mcplib scripts .github selfupdate`, excluding record
    filenames: one hit, the new test comment "bridge was not carried over
    from mcplib". `grep -rn -i 'bridge\|0\.16\.0'`: only that comment and
    the `--bridge` usage case. (A first sweep used `git grep`, which skips
    untracked files. It was re-run with `grep`.)
* **First-fail,** on scratch copies of `scripts/` and the workflow, with
  one plant each (`plant_tooling.py`):

  | Plant | Test | Result |
  |---|---|---|
  | a. file-set check disabled (`if False:`) | verifier test | exit 1, `not ok - undeclared extra file (expected failure)` |
  | b. `GH_REPO` removed from "Create a draft release" | `check-workflow-gh-repo.sh` | exit 1, `gh steps missing GH_REPO:` naming both `gh release create` and `upload` |
  | c. guard exits 0 when gh's error is unrecognised | refuse test | exit 1, `FAIL undiagnosed gh failure is refused: want exit 1, got 0`, `4 passed, 2 failed` |
  | d. verifier accepts `--bridge` again | verifier test | exit 1, `not ok - --bridge is not an option (expected usage exit 2, got 0)` |
  | e. strict-tag check disabled | verifier test | exit 1, `not ok - non-strict tag rejected (expected failure)` |

  With no plant, all three pass. The scratch copies were deleted.

#### Deviation D2 (2026-09-29): bridge-only leftovers, tag coverage, and the CI shell

* **Found.**
  1. The verifier's `--repository` argument had one reader, the bridge
     guard (`v1.6.0` `verify-selfupdate-release.sh:136`). Removing the
     bridge left it accepted and unused. MADR §3 named `--bridge` but not
     `--repository`.
  2. The only `v1.6.0` test cases that passed `--tag` were the bridge
     cases. Removing them left the strict-tag check, which every release
     still runs, with no test.
  3. `mcplib`'s CI runs the shell-script steps with no `shell:`. On the
     Windows leg that is PowerShell.
* **Done.** All three are consequences of the owner's decision to drop the
  bridge, or of running the moved steps here. None changes the workflow's
  inputs beyond MADR §3.
  1. `--repository` is removed from the verifier and the workflow, and
     asserted to be a usage error.
  2. A strict `--tag v1.2.3` accept case and a `--tag v1.2` reject case
     are added. Plant e shows the reject case catches a disabled check.
  3. Both script steps in `ci.yml` set `shell: bash`. Their first Windows
     run is in Phase 6.
* **Scope added to Phase 4.** None beyond the files already listed.

Commit: `da7da95`.

### Phase 5: documentation (2026-09-29)

* `README.md`: the status (Go 1.27.1, the first release to be `v1.0.0`, not
  yet tagged) and a package table. A "Self-update" section carries the
  substance of `mcplib`'s: the bindings, the `User-Agent` and token
  variables, and `ExitCode` (0 with no error, 10 when an update is
  available, 1 otherwise). "Publishing releases" gives the workflow's
  contract and a call example with no `bridge-release`. Also the "I want
  to…" rows.
* `docs/architecture.md`: the tree with the package, the release scripts and
  the reusable workflow; the package table (26 non-test and 21 test files,
  source `mcplib` `v1.6.0` `4e1f9a53e265`, the four differences); the
  workflow's steps; three-target lint; the new CI steps; and "What is not
  here" (the tag, consumer migrations, `docs/reports/`). This is the
  provenance statement D1 points to.
* `docs/guides/migrating-from-mcplib-selfupdate.md`: Go 1.27.1 first, the
  import and `go get`, the `uses:` change with `bridge-release` deleted, and
  checks. The `uses:` SHA stays a named placeholder (`<v1.0.0 commit SHA>`),
  with the `git ls-remote` command that resolves it. Phase 6 fills it in.
* `docs/README.md`: four new "I want to…" rows.
* **Claims checked against the code before commit:** `cleanup_other.go` is
  `//go:build !windows`. A declined apply returns a nil error, so
  `ExitCode` is 0 for it (the first README draft said "current or applied"
  and was corrected). The token order is `GH_TOKEN`, then `GITHUB_TOKEN`
  (`github.go:102-105`).
* `markdownlint-cli2` over `README.md`, `AGENTS.md`, `docs/README.md`,
  `docs/architecture.md` and the guide: `0 issues`. The link resolver
  (proven in 0001-PLAN Phase 3) over those files and every record:
  `checked 31 relative links, 0 broken`. The identifier scan found no match.
* **Whole-tree gates** before commit: `make test`, `vet`, `lint` (three
  targets), `vuln` and `pre-add-check` all exit 0. `go mod tidy -diff`
  exit 0, `go mod verify` `all modules verified`, `gofmt -l .` empty, the
  three script tests exit 0, and `shellcheck scripts/*.sh` exit 0.

This Phase 5 commit is the `v1.0.0` commit (MADR §8). Its SHA is recorded
in Phase 6, which has not started: it waits for the owner's ask to push,
then to tag.

Commit: `3700381` (`370038110816f9f97a4f2edb2c658ff0f7abd16d`).

### Phase 6: push, CI, tag (2026-09-29 to 2026-09-30)

* **Step 1.** The pre-push disclosure guard from `AGENTS.md`, over
  `origin/main..main` (the ten 0001 and 0002 commits): exit 0, no output.
* **Step 2.** The owner: "Push". `git push origin main`:
  `8ebd95e..3700381  main -> main`, exit 0. Only this repository was pushed.
  The shellcheck fixes in the four other repositories (0001-PLAN D1) stay
  local.
  * GitHub started **two** `CI` runs for the one push, `36664052134` and
    `36664053095`. Both are `.github/workflows/ci.yml`, event `push`, head
    `3700381`, workflow id `370764591`. The cause was not established; it is
    GitHub's duplicate trigger, not a second workflow. Both were watched to
    completion.
  * Both concluded `success`, with `validate (ubuntu-24.04)`,
    `validate (macos-15)` and `validate (windows-2025)` each `success`. On
    ubuntu every step succeeded: `go test`; `vet, gofmt, tidy, lint` (the
    three-target `make lint`); `govulncheck`;
    `verify self-update release fixtures`; and
    `Verify the release guard and its workflow contract`.
  * **Windows** (job `109724815442`): `Run go test ./...` printed
    `ok  github.com/maccavelli/go-core-lib/selfupdate 4.827s`. That run
    includes the `//go:build windows` tests
    `TestWindowsCleanupReceiptRoundTrip` and
    `TestWindowsCleanupReceiptDigestMismatch`, which exercise fixes 1 and 2
    — their first run. Under `shell: bash`,
    `Verify the release guard and its workflow contract` printed
    `6 passed, 0 failed` and
    `check-workflow-gh-repo: ok — every repository-scoped gh step sets GH_REPO`.
  * A GitHub annotation on `validate (macos-15)` warned of queue delays for
    macOS arm64 runners. It is informational; the job passed.
* **Steps 3–5** (tag `v1.0.0` on `3700381`, tag CI, proxy check, filling in
  the guide's SHA, closing this PLAN) wait for the owner's ask to tag.

#### Entry 2026-09-29: steps 3–5 wait for 0003

The owner decided to fix every finding of
[0003-MADR-remediate-debugging-pass-findings.md](0003-MADR-remediate-debugging-pass-findings.md)
before the first tag (0002-MADR, "Amendment 2026-09-29" under "More
Information").

* ~~Tag `v1.0.0` on `3700381`.~~ `v1.0.0` goes on the commit that
  completes 0003's PLAN.
* Before the tag, the 0003 commits are pushed on the owner's ask, and the
  push's CI must pass on all three operating systems.
* Steps 3–5 resume only when 0003's PLAN is `complete`.

#### Entry 2026-09-30: the 0003 commits pushed, CI green

* The owner: "Stage, commit, and push all outstanding".
  * `main` `6b9fb99..a7d5f01` gave CI `36726867210`: failure on Linux,
    from the runner's unpinned shellcheck. That is 0003-PLAN deviation D5,
    whose Windows and macOS legs passed.
  * The owner chose to fix the line and pin shellcheck v0.11.0 (the
    latest, and this host's version). `a7d5f01..efe1e80` gave CI
    `36728547077`: **success** on all three operating systems.
* 0003's PLAN is `complete`. `v1.0.0` goes on the commit that records
  this (the next one on `main`), on the owner's ask to tag.

That commit is `b36ca44`. Its CI, run `36729679152`, was `success` on all
three operating systems.

#### Steps 3–5: `v1.0.0` (2026-09-30)

* **Step 3.** The owner: "Tag v1.0.0 and push it."
  * Checked first: no `v*` tag locally or on `origin`; `HEAD` =
    `origin/main` = `b36ca4494b86e52cf1b4a315554603f1c6ee3a21`, whose CI
    was `success`.
  * `git tag -a v1.0.0 b36ca44… -m "go-core-lib v1.0.0"`. The tag is
    annotated, as the fleet's tags are (`mcplib` `v1.6.0`: type `tag`,
    subject `mcplib v1.6.0`).
  * `git push origin v1.0.0`: `* [new tag] v1.0.0 -> v1.0.0`.
  * `git ls-remote`: `refs/tags/v1.0.0` = `084140b7…` (the tag object),
    and `refs/tags/v1.0.0^{}` = `b36ca449…` (the commit). That confirms the
    guide's peeled-ref command.
* **Tag CI.** Run `36731692522` (`push`, `v1.0.0`, `b36ca44`):
  `success` on ubuntu-24.04, macos-15 and windows-2025.
* **Step 4.**
  `GOPROXY=https://proxy.golang.org go list -m -json github.com/maccavelli/go-core-lib@v1.0.0`
  exits 0 with `"Version": "v1.0.0"`, `"GoVersion": "1.27.1"`, and
  `Origin.Hash` `b36ca4494b86e52cf1b4a315554603f1c6ee3a21`,
  `Ref: refs/tags/v1.0.0`. `…/@v/list` returns `v1.0.0`.
* **Step 5.**
  * The guide's `uses:` line and the README's workflow example name
    `@b36ca4494b86e52cf1b4a315554603f1c6ee3a21 # go-core-lib v1.0.0`.
  * The README's status gives the release and `go get` line.
  * `docs/architecture.md` states the release, and drops "A tag" from
    "What is not here".
  * This PLAN is `complete`. The commit is made on `main`, and pushed only
    on the owner's ask.