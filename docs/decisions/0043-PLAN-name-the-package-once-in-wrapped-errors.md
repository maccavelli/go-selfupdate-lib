---
status: proposed
date: 2026-11-10
associated-madr: "0043-MADR-name-the-package-once-in-wrapped-errors.md"
---
<!-- markdownlint-disable MD004 MD013 MD024 MD060 -->

# Implement one package name per wrapped error: `internal/errprefix`, 132 literal-prefix wraps and `wrapRun` through `Wrap`, a source scan, and `v1.26.1`

Associated MADR: [0043-MADR-name-the-package-once-in-wrapped-errors.md](0043-MADR-name-the-package-once-in-wrapped-errors.md)
(D1-A to D7-A, chosen by the owner on 2026-11-10).

## Goal

* **`internal/errprefix`** offers `Error`, `Wrap` and `Strip`, as the MADR's D2-A describes.
* **Every literal-prefix wrap outside `internal/procfs/`, 132 calls, and `wrapRun` use `errprefix.Wrap`** (D1-A, D5-A). Each run error names the package once.
  * MADR F2's lines read `update failed: selfupdate: demo: pass --yes to apply without prompting: confirmation required` and `update failed: selfupdate: demo: staged digest does not match SHA256SUMS: integrity check failed`.
* **`TestLiteralPrefixWrapsUseWrap` holds the rule** (D6-A). It is seen to fail on the tree before the sites change, and on planted copies. 0042's scan reads `Wrap`'s literals.
* **`v1.26.1` ships it** (D7-A). Issue #11 is closed on the owner's word.

## Scope

### Preconditions

* `HEAD`'s Go code is `v1.26.0`'s: `git diff --name-only v1.26.0 HEAD -- '*.go'` prints nothing. It held at `8b19f1d`.
* The owner has approved this PLAN. The MADR is `accepted`.

### In scope

| Phase | What | Commit |
| :--- | :--- | :--- |
| C0 | this PLAN, its MADR, and their rows in `docs/README.md` | records only |
| C1 | `internal/errprefix`, the scan, the 132 sites and `wrapRun`, the tests and golden files, `.golangci.yml`, `AGENTS.md`, `docs/architecture.md`, this PLAN's record | one commit |
| C2 | the runs on each host, and the planted failures; this PLAN's record | records only |
| C3a | `CHANGELOG.md` `v1.26.1`, and the migration guide's "From v1.26.0 to v1.26.1"; this PLAN's record | the commit the owner tags |
| C3b | the pins and the release-history line, after the tag's CI is green; this PLAN set to `complete` | records and guides |

Files that Phase C1 changes:

* **New.** `internal/errprefix/errprefix.go` and `internal/errprefix/errprefix_test.go`.
* **The sites, 41 files.** Every file of MADR F3 except `internal/procfs/procfs.go` and `procfs_linux.go`. That is 93 calls under `selfupdate/`, 27 under `selfupdate/service/` and its backends, and 12 under `archive`, `codesign`, `ghattest` and `signednote`. `selfupdate/updater.go` also holds `wrapRun`.
* **Tests.**
  * `internal/testproc/testproc_test.go`: `TestLiteralPrefixWrapsUseWrap`, and `errorMessages` reading `errprefix.Wrap`.
  * `selfupdate/cli/flags_test.go:128`.
  * The six golden files of MADR F5.
  * `selfupdate/updater_contract_test.go`: MADR F2's two cases.
  * Any other test that the full run shows pins a changed text. Each one is listed in the record with its old and new text.
* **Config and docs.** `.golangci.yml`, `AGENTS.md` and `docs/architecture.md`.

### Out of scope

* MADR F4's mid-line wraps, and `internal/procfs/`'s four calls (D5-A).
* `internal/cmd/`, `selfupdatetest`, `servicetest`, and `cli`'s `update failed: ` (MADR More Information).
* Any exported identifier. `make apicheck` must list no addition.
* Pushes, tags, and closing issue #11. These happen only on the owner's word in the same turn.

## Rules

* Mutation starts only after the owner approves this PLAN.
* The owner commits and pushes. The agent runs the checks and leaves the tree.
* A deviation stops the work: it is recorded here, dated, and the owner decides before it goes on.
* Every planted failure runs in a scratch copy (`scripts/plant-copy.sh`), never in the tree.
* Nothing committed names a host, an account or a real-machine path.

## Implementation Steps

### Phase C0: the records

1. This PLAN, its MADR and their two rows in `docs/README.md`. Run `make markdownlint`, `make check-records`, `make docs-check`, and `scripts/check-docs.sh --ids` on the three files. The owner commits.

### Phase C1: the helper, the scan and the sites

1. **`internal/errprefix/errprefix.go`.**
   * The package comment says what the package is for: a wrapped error names the package once (`0043-MADR D2-A`).
   * `const prefix = "selfupdate: "`.
   * `type Error struct { Op string; Err error }`:
     * `Error()` returns `e.Op + ": " + strings.TrimPrefix(e.Err.Error(), prefix)`;
     * `Unwrap()` returns `e.Err`.
   * `func Wrap(op string, err error) error`: nil for a nil `err`, and otherwise `&Error{Op: op, Err: err}`.
   * `func Strip(err error) error`:
     * nil for nil;
     * `err` itself when `err.Error()` does not begin with `prefix`;
     * otherwise an unexported `stripped{err}`, whose `Error()` is the trimmed text and whose `Unwrap()` is `err`.
   * Each exported name has a godoc comment.
2. **`internal/errprefix/errprefix_test.go`.** One table test over `Wrap` and `Strip`, with the cases of MADR Confirmation, and one `errors.As` case to `*selfupdate.SymlinkInvocationError`.
   * `errprefix` cannot import `selfupdate` outside tests, because of the cycle. That case therefore lives in `selfupdate`'s own test (step 8).
   * Here, `errors.As` targets a local error type.
3. **`.golangci.yml`.**
   * A new rule, `internal-errprefix`, after `internal-redact`: files `**/internal/errprefix/*.go` and `!$test`; allow `$gostd`. Its comment cites `0043-MADR D2-A`.
   * `other-packages` gains `"!**/internal/errprefix/*.go"`.
   * Each of the ten rules of MADR F7 allows `github.com/maccavelli/go-selfupdate-lib/internal/errprefix$`.
4. **Write the scan first, and see it fail on the tree.** In `internal/testproc/testproc_test.go`, after `TestRunTimeMessagesNameNoGoIdentifier`, add `TestLiteralPrefixWrapsUseWrap`:
   * **Which files.** It walks the same files as 0042's scan, with the same skip list, and also skips `internal/procfs/`. A comment there cites `0043-MADR D5-A`.
   * **What it refuses.** Each `fmt.Errorf` call whose first argument is one string literal that, unquoted:
     * begins `selfupdate: `;
     * ends `: %w`;
     * has no other `%` verb.
   * **What it counts.** Calls to `errprefix.Wrap`. Fewer than 130 fails, which catches a wrong root.
   * **What a failure says.** `path:line` and the format, and the rule with its record (`0043-MADR D6-A`).
   * **Seeing it fail.** Run it before step 6. It must name exactly the 132 sites, and the count of `Wrap` calls is 0. Record the count and the first and last lines of the output.
5. **0042's scan reads `Wrap`.** `errorMessages` also matches a call whose function is the selector `errprefix.Wrap`, and joins the string literals of its first argument, as it does for `fmt.Errorf`. Its comment cites `0043-MADR D6-A`.
6. **The sites.**
   * A Python script in the scratchpad rewrites each site: `fmt.Errorf("selfupdate: <text>: %w", x)` becomes `errprefix.Wrap("selfupdate: <text>", x)`.
     * It balances the parentheses to find `x`, so `errors.Join(…)` arguments and nested calls stay whole.
     * It adds the import.
     * It drops `fmt` where no other use is left.
     * It asserts each file's count against MADR F3 before it replaces.
   * It runs first on a scratch copy of the tree, where `go build ./...` and `go vet ./...` pass for `GOOS=linux`, `darwin` and `windows` with `CGO_ENABLED=0`. Then it runs on the tree.
   * `service/handoff.go:198`, the one multi-line format, is checked by hand after the script.
7. **`wrapRun`** (`selfupdate/updater.go:768-773`) becomes `return errprefix.Wrap("selfupdate: "+req.Product, err)`, with the nil check kept. Its comment cites `0043-MADR D2-A` and D4-A.
8. **Tests.**
   * `flags_test.go:128` expects `"selfupdate: demo: " + strings.TrimPrefix(reqErr.Error(), "selfupdate: ")`.
   * `go test ./selfupdate/cli/ -run Golden -update` rewrites the golden files. `git diff --stat` must show only MADR F5's six files, and their diff only the repeated prefixes.
   * `updater_contract_test.go` gains `TestRunErrorNamesPackageOnce`, with MADR F2's two cases built as probe P1 built them:
     * the exact text;
     * `errors.Is` against `ErrConfirmationRequired` and `ErrIntegrity`;
     * `errors.As` to `*SymlinkInvocationError` through `wrapRun`, for the case step 2 could not hold.
   * Run `go test -count=1 ./...`. Update each other test that pins a changed text, and list it in the record.
9. **Docs.**
   * `AGENTS.md`:
     * the scope paragraph names `internal/errprefix`, which composes a wrapped error's prefix (`0043-MADR D2-A`);
     * the depguard list gains `internal-errprefix`;
     * after the 0042 paragraph, one paragraph: a wrap whose text is a literal prefix uses `errprefix.Wrap`, and `TestLiteralPrefixWrapsUseWrap` holds it.
   * `docs/architecture.md`:
     * the tree block gains `internal/errprefix/`;
     * the package table gains its row (`errprefix`, 1, 1, none);
     * the ten packages' parentheticals name `internal/errprefix`;
     * the tooling paragraph that names 0042's scan (`:437`) names the new one beside it.
10. **Checks.**
    * `make pre-add-check FILES="<every changed .go file>"`.
    * `make gate`, ending `overall=0`.
    * `make apicheck`: compatible with `v1.26.0`, and no addition.
    * `make markdownlint`, `make check-records`, `make docs-check` and `make disclosure-check`.
    * Record the output in this PLAN, in the same commit. The owner commits.

### Phase C2: the hosts and the planted failures

1. Run the tests on each host, on a bundle of the Phase C1 tree:
   * `go test -count=1 ./...` on the macOS development host and the Linux test host;
   * `go test -count=1 ./selfupdate/... ./internal/...` on the Windows test host, where `cleanup_windows.go` and `replace_windows.go` build.

   Record each result.
2. With `scripts/plant-copy.sh`, make these copies, each with one change, and run the named test in each. Record each failure's output.

   | Plant | Test | Must fail with |
   | :--- | :--- | :--- |
   | `target.go`'s first `errprefix.Wrap("selfupdate: stat target", err)` back to `fmt.Errorf("selfupdate: stat target: %w", err)` | `TestLiteralPrefixWrapsUseWrap` | that line, and only it |
   | the scan's `internal/procfs/` skip removed | `TestLiteralPrefixWrapsUseWrap` | exactly `procfs`'s four calls |
   | `Strip` returning `err` unchanged | `errprefix`'s table test; `cli`'s golden tests | the prefixed cases; the six golden files |
   | `strings.TrimPrefix` replaced by `strings.ReplaceAll(…, prefix, "")` | `errprefix`'s table test | the "later in the text" case |
   | one `Wrap` literal given `TargetPolicy.ExecutablePath` | `TestRunTimeMessagesNameNoGoIdentifier` | that literal |

3. Record every result in this PLAN. No commit but the record.

### Phase C3a: prepare `v1.26.1`

1. `CHANGELOG.md`: `## v1.26.1 - <date>`.
   * **Changed:** a run error, and every error this module wraps behind a literal prefix, names the package once.
   * **Fixed:** issue #11.
   * Links: the guide's new section, the MADR and this PLAN.
2. The migration guide:
   * `### From v1.26.0 to v1.26.1` at the end of §26, before its `### Check`, in the form of `### From v1.22.0 to v1.22.1`. It says:
     * `make apicheck` reports `v1.26.1` compatible;
     * which texts change, with MADR F2's two lines old and new;
     * that the sentinels match as before;
     * that a program matching the full text should match the sentinel instead.
   * The intro (`:84-92`) gains a `v1.26.1` line.
3. `scripts/check-release-tag.sh v1.26.1` exits 0, and `git tag -l v1.26.1` prints nothing. Run the Markdown checks. Record the results here. The owner commits, pushes, tags `v1.26.1` and pushes the tag.

### Phase C3b: the pins, after the tag's CI is green

1. Check that `.github`, `internal/cmd` and `scripts/*release*` are unchanged since `v1.26.0`.
2. Move each pin from `v1.26.0` and its commit to `v1.26.1` and its commit, as 0042-PLAN Phase B3b did:
   * `README.md`, `docs/architecture.md`, `docs/guides/building-releases.md` and the migration guide;
   * by a scratchpad script that asserts each count before it replaces, run first on scratch copies.
3. Add the release-history line.
4. Set this PLAN to `complete`, with its row in `docs/README.md`.
5. Run the checks, and record them. The owner commits and pushes.
6. Closing issue #11, with one comment, is the owner's, or the agent's when asked in that turn.

## Verification

* `TestLiteralPrefixWrapsUseWrap` passes on the changed tree. It fails on the tree before step 6, naming the 132 sites, and on Phase C2's two scan plants.
* `errprefix`'s table test, `TestRunErrorNamesPackageOnce` and the golden files pass, and fail on Phase C2's `Strip` plants.
* `TestRunTimeMessagesNameNoGoIdentifier` passes, and fails on the identifier plant. Its file and call counts stay above its floor.
* `make gate` ends `overall=0`. `make apicheck` lists no addition. The four documentation checks are clean.
* Phase C2's host runs pass on macOS, Linux and Windows.

## Rollout and Rollback

* **Rollout.**
  * `v1.26.1` is a patch, so a program takes it with `go get github.com/maccavelli/go-selfupdate-lib@v1.26.1`.
  * Nothing is configured. Only text changes, and `errors.Is` and `errors.As` behave as before.
* **Rollback.**
  * Before the tag: revert Phase C1's commit.
  * After the tag: a program pins `v1.26.0`. A later patch could revert the change, but a published tag is never moved or deleted ([0004-PLAN-v1-3-1-ocp-login-pattern-rewording.md](0004-PLAN-v1-3-1-ocp-login-pattern-rewording.md), Out of scope).

## Execution record

Not started.

### Deviations

None.
