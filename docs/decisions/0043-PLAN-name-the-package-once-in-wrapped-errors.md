---
status: in-progress
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
* **Amendment A1 (2026-11-10).** A managed install's failure names `managed install failed` once on every path (MADR Amendment A1 D8-B), and `v1.26.2` ships it.

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
| C4 | Amendment A1: `managed.go`'s two returns, `TestManagedFailureNamesPhraseOnce`, the plants; this PLAN's record | one commit |
| C5a | `CHANGELOG.md` `v1.26.2`, and the guide's "From v1.26.1 to v1.26.2"; this PLAN's record | the commit the owner tags |
| C5b | the pins, after the tag's CI is green; this PLAN set to `complete` | records and guides |

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

### Phase C4: a managed install's failure names its phrase once (Amendment A1)

MADR Amendment A1 D8-B. The owner chose D8-B on 2026-11-10. These phases wait for the owner's approval of this PLAN as amended.

**Precondition.** `git diff --name-only v1.26.1 HEAD -- '*.go'` prints nothing. It held at `d7353b5`.

1. **Write the test first, and see it fail on the tree.** `selfupdate/managed_test.go` gains `TestManagedFailureNamesPhraseOnce`, a table test over MADR FA1.2's five paths:
   * four through `managedEnv` and `fakeLife`, with the injected errors probe A1-P2 used (`unhealthy`, `partial write`, `stop wait timed out` with `stopTakesEffect`, and `refused inside the service`);
   * one through `replaceFirst`, `firstLife` and `early`, with `stop refused`.

   Each case passes `Install`'s error through `wrapRun(Request{Product: "demo"}, err)`, as `Run` does. Then it asserts:
   * the exact text of the Amendment A1 outcome's table;
   * `errors.Is` against `ErrManagedInstall` and against the injected error;
   * `strings.Count(text, "managed install failed") == 1`.

   The test's comment cites `0043-MADR Amendment A1 D8-B`. Run it before step 2. It must fail exactly the four rows that pass through `recover`, and pass `the stop fails, and the service still runs`. Record the output.
2. **`recover`** (`selfupdate/managed.go:359`) returns `result, errors.Join(ErrManagedInstall, origin, recov)`, with a comment that cites D8-B: the sentinel's text is the phrase.
3. **`recoverStop`** (`:293-299`):
   * when nothing was replaced, it returns `errprefix.Wrap("selfupdate: stop service", errors.Join(ErrManagedInstall, stopErr, rerr))`, as today;
   * when `ReplaceBeforeStop` replaced the binary, it passes `recover` `errprefix.Wrap("selfupdate: stop service", errors.Join(stopErr, rerr))`, without the sentinel, which `recover` joins. A comment cites D8-B.
4. **Tests.** `TestManagedFailureNamesPhraseOnce` passes. `go test -count=1 ./...` gives `ok` for every package. MADR FA1.5 found no other test that pins the text. A test that does fail is a deviation: the work stops, and it is recorded here.
5. **Seen failing.** With `scripts/plant-copy.sh`, make these copies, each with one change, and run `TestManagedFailureNamesPhraseOnce` in each. Record each failure's output. `git status --short` is empty afterwards.

   | Plant | Must fail with |
   | :--- | :--- |
   | step 2's return back to `errprefix.Wrap("selfupdate: managed install failed", errors.Join(ErrManagedInstall, origin, recov))` | the four rows through `recover` |
   | step 3's `errors.Join(stopErr, rerr)` back to `errors.Join(ErrManagedInstall, stopErr, rerr)` | the `ReplaceBeforeStop` row alone |

6. **Checks.**
   * `make pre-add-check FILES="selfupdate/managed.go selfupdate/managed_test.go"`.
   * `make gate`, ending `overall=0`.
   * `make apicheck`: compatible with `v1.26.1`, and no addition.
   * `make markdownlint`, `make check-records`, `make docs-check` and `make disclosure-check`.
   * Record the output in this PLAN, in the same commit. The owner commits.

   No `.go` file is added or removed, so `docs/architecture.md`'s package table and `AGENTS.md` do not change. The Linux and Windows test hosts are not run. `managed.go` and `managed_test.go` carry no build constraint, and the push's CI runs `validate` on Linux, macOS and Windows before Phase C5a's tag.

### Phase C5a: prepare `v1.26.2`

1. **Precondition.** The push of Phase C4's commit passed CI on `main`.
2. `CHANGELOG.md`: `## v1.26.2 - <date>`.
   * **Fixed:** a managed install's failure names `managed install failed` once, with the health check's old and new text from the MADR's tables.
   * Links: the guide's new section, the MADR's Amendment A1 and this PLAN.
3. The migration guide:
   * `### From v1.26.1 to v1.26.2`, after `### From v1.26.0 to v1.26.1` and before §26's `### Check`, in the same form. It says:
     * `make apicheck` reports `v1.26.2` compatible;
     * the old and new text of the health check's row and the `ReplaceBeforeStop` row;
     * that `ErrManagedInstall` and each cause match as before;
     * that a program matching the full text should match the sentinel instead.
   * The intro gains a `v1.26.2` line after `v1.26.1`'s.
4. `scripts/check-release-tag.sh v1.26.2` exits 0, and `git tag -l v1.26.2` prints nothing. Run `make apicheck` and the Markdown checks, and `scripts/check-docs.sh --ids` on the two files. Record the results here. The owner commits, pushes, tags `v1.26.2` and pushes the tag.

### Phase C5b: the pins, after `v1.26.2`'s CI is green

1. Check that `.github`, `internal/cmd` and `scripts/*release*` are unchanged since `v1.26.1`.
2. Move each pin from `v1.26.1` and `9fc9dd7265c305caf995083663dd0df5b2a0ad39` to `v1.26.2` and its commit, in the four files of Phase C3b step 2. Use a scratchpad script built on `pin1261.py`, which asserts each count before it replaces and runs first on scratch copies.
3. Set this PLAN to `complete`, with its row in `docs/README.md`.
4. Run the checks, and record them. The owner commits and pushes.

## Verification

* `TestLiteralPrefixWrapsUseWrap` passes on the changed tree. It fails on the tree before step 6, naming the 132 sites, and on Phase C2's two scan plants.
* `errprefix`'s table test, `TestRunErrorNamesPackageOnce` and the golden files pass, and fail on Phase C2's `Strip` plants.
* `TestRunTimeMessagesNameNoGoIdentifier` passes, and fails on the identifier plant. Its file and call counts stay above its floor.
* `make gate` ends `overall=0`. `make apicheck` lists no addition. The four documentation checks are clean.
* Phase C2's host runs pass on macOS, Linux and Windows.
* Amendment A1: `TestManagedFailureNamesPhraseOnce` passes, and fails on the tree before Phase C4 step 2 and on Phase C4's two plants. `make gate` ends `overall=0`, and `make apicheck` lists no addition against `v1.26.1`. The push's CI passes on Linux, macOS and Windows.

## Rollout and Rollback

* **Rollout.**
  * `v1.26.1` is a patch, so a program takes it with `go get github.com/maccavelli/go-selfupdate-lib@v1.26.1`.
  * Nothing is configured. Only text changes, and `errors.Is` and `errors.As` behave as before.
* **Rollback.**
  * Before the tag: revert Phase C1's commit.
  * After the tag: a program pins `v1.26.0`. A later patch could revert the change, but a published tag is never moved or deleted ([0004-PLAN-v1-3-1-ocp-login-pattern-rewording.md](0004-PLAN-v1-3-1-ocp-login-pattern-rewording.md), Out of scope).
* **Rollback of Amendment A1.** Before `v1.26.2`'s tag, revert Phase C4's commit. After it, a program pins `v1.26.1`.

## Execution record

The owner accepted the MADR and approved this PLAN on 2026-11-10 ("accept the madr, plan approved to proceed").

### Phase C0

* The owner committed the MADR, this PLAN and their rows as `9d44e04`.

### Phase C1

* **Precondition.** `git diff --name-only v1.26.0 HEAD -- '*.go'` printed nothing at `8b19f1d`.
* **Steps 1 and 2.** `internal/errprefix/errprefix.go` holds `Error`, `Wrap`, `Strip` and the unexported `stripped`. `errprefix_test.go` holds:
  * `TestWrapAndStrip`, over six cases: a prefixed sentinel, a wrapped sentinel, no prefix, a prefix later only, a join with both parts prefixed, and an error type of the test's own;
  * `TestStripReturnsUnprefixedErrorItself`;
  * `TestNil`.

  `go test ./internal/errprefix/` gave `ok`. `golangci-lint`'s `errorlint` refused three identity comparisons. They are written `any(x) != any(y)`, as `spawn_test.go:148` writes its own.
* **Step 3.** A scratchpad script made the depguard changes, asserting each anchor once, first on a copy of `.golangci.yml`:
  * the `internal-errprefix` rule, after `internal-redact`;
  * the `other-packages` exclusion;
  * the allow line in the ten rules.

  The copy's diff showed each line inside its own rule. The tree's result was byte-identical to the copy.
* **Step 4: the scan, seen failing.**
  * `TestLiteralPrefixWrapsUseWrap` and the helpers it shares with the 0042 scan, `walkScanned`, `scanSkip` and `isCall`, are in `internal/testproc/testproc_test.go`.
  * On the tree before step 6 it failed. It named 132 sites, none under `internal/procfs/`, and its floor line read `read 134 files and 0 errprefix.Wrap calls; the module has more: is the root wrong?`. The first and last sites were:

    ```text
    selfupdate/adapters.go:98: "selfupdate: pass --yes to apply without prompting: %w": a wrap behind a literal prefix uses errprefix.Wrap, so the line names the package once (0043-MADR D6-A)
    selfupdate/verify.go:16: "selfupdate: staged digest does not match github asset digest: %w": a wrap behind a literal prefix uses errprefix.Wrap, so the line names the package once (0043-MADR D6-A)
    ```

  * In the same run, `TestRunTimeMessagesNameNoGoIdentifier` passed.
* **Step 5.** `errorMessages` reads `fmt.Errorf`, `errors.New`, and the first argument of `errprefix.Wrap`.
* **Step 6: the sites.**
  * The scratchpad's `convert_sites.py` took the 132 expected sites from step 4's output and asserted each file's count before writing.
  * On a scratch copy of the tree, with no `.git`, it gave `files 41 sites 132`. `gofmt` re-sorted the new import in 17 files. `go vet ./...` passed for `GOOS=linux`, `darwin` and `windows` with `CGO_ENABLED=0`.
  * On the tree it gave the same, and after `gofmt -w` on the same 17 files, every changed `.go` file was byte-identical to the copy.
  * `service/handoff.go:198`, the multi-line format, became `errprefix.Wrap("selfupdate: service: an update from inside the service runs detached and cannot ask; pass --yes",` with its second line unchanged.
  * **Found, out of scope:** `managed.go:357` wraps `ErrManagedInstall` behind its own phrase. It now reads `selfupdate: managed install failed: managed install failed` and the joined parts. Before, it read `selfupdate: managed install failed: selfupdate: managed install failed`. It is no worse, and a later record can reword it.
    * *2026-11-10:* MADR Amendment A1 decides it (D8-B), and Phases C4 to C5b carry it out. Probe A1-P2 also found a second join of the sentinel, on the `ReplaceBeforeStop` path, that this entry missed.
* **Step 7.** `wrapRun` returns `errprefix.Wrap("selfupdate: "+req.Product, err)`, with a comment citing D2-A and D4-A.
* **Step 8: tests.** The first full run after step 7 failed four tests:
  * `TestBatchErrorChain`: Deviation 1, below. `chainReturn` now accepts `errprefix.Wrap(op, e)`, and `TestErrBatchFileIsWinproc` asserts `errors.Is` through `Wrap`.
  * `selfupdate/archive`'s `TestNewSelectorRefusesPlatforms` pinned the repeat: `NewSelector = selfupdate: archive: duplicate platform linux/amd64; the exact selector says selfupdate: duplicate platform linux/amd64`. It now requires exactly `"selfupdate: archive: "` and the exact selector's text without its prefix. The old check was `strings.Contains`.
  * `cli`'s `TestRequestContradictions` (`flags_test.go:128`) expects one prefix.
  * `cli`'s `TestGolden` and `TestCommandGolden`. `go test ./selfupdate/cli/ -run Golden -update` rewrote exactly MADR F5's six files, one line each. A scratchpad check showed each new file equals `HEAD`'s with only the repeated prefixes removed.
  * `TestRunErrorNamesPackageOnce` (`updater_contract_test.go`) passes its three cases, `confirmation`, `digest` and `symlink refusal through wrapRun`:
    * `selfupdate: demo: pass --yes to apply without prompting: confirmation required`, matching `ErrConfirmationRequired`;
    * `selfupdate: demo: staged digest does not match SHA256SUMS: integrity check failed`, matching `ErrIntegrity`;
    * `errors.As` to the same `*SymlinkInvocationError`.

  Then `go test -count=1 ./...` gave `ok` for all 22 packages.
* **Step 9: docs.**
  * `AGENTS.md`: the scope paragraph, the depguard list, and a paragraph after 0042's.
  * `docs/architecture.md`: the tree block, the `internal/errprefix/` row, the ten packages' parentheticals, and the tooling paragraph.
  * `GOOS=linux`, `darwin` and `windows` `go list` found exactly the ten packages of MADR F7 importing `internal/errprefix`.
* **Step 10: checks.**
  * `make pre-add-check` on the 48 changed and new `.go` files: `go-precheck: 48 file(s) clean (gofmt, golangci-lint, go vet, go test, govulncheck).`
  * `make gate` gave `overall=0`, with every step `rc=0`. The steps were gofmt, lint (`0 issues.`), vet, race, shuffle, tidy, apicheck (`check-api-compat: compatible with v1.26.0`, and no line but the internal packages it ignores), fuzz, vuln (`No vulnerabilities found.`), scripts, shellcheck, crossvet, links (`992 links in 115 files, 0 broken`), arch (`22 rows … 0 differ, 0 without a row`) and ids (`59 files, 20 deny-list rules, 0 findings`).
  * `make markdownlint` first refused `AGENTS.md:291`, `MD038/no-space-in-code`, on a code span ending in a space. After rewording it gave `0 issues`.
  * `make check-records`: `102 records, 0 findings`. `make docs-check`: exit 0, with 22 architecture rows. `make disclosure-check`: `744 files, 20 deny-list rules, 0 findings`.
* The owner committed Phase C1 as `a92d319`.

### Phase C2

* **Step 1: the hosts.** A bundle of `a92d319` was cloned on each test host and tested there natively, with go1.27.3 and `TMPDIR` inside the clone's directory:
  * the macOS development host, `go test -count=1 ./...` on the tree: `ok` for all 22 packages;
  * the Linux test host, `go test -count=1 ./...`: `ok` for all 22 packages;
  * the Windows test host, `go test -count=1 ./selfupdate/... ./internal/...`: `ok` for all 21 packages.
* **Step 2: the plants.** The scratchpad's `plants.py` made each copy with `scripts/plant-copy.sh`, ran the named test in it, and removed it. `git status --short` was empty after the run. Deviation 1's walk plant is the last row.

  | Plant | Result |
  | :--- | :--- |
  | `target.go`'s `errprefix.Wrap("selfupdate: stat target", err)` back to `fmt.Errorf(…: %w", err)` | `TestLiteralPrefixWrapsUseWrap` failed with that line alone: `selfupdate/target.go:20: "selfupdate: stat target: %w": a wrap behind a literal prefix uses errprefix.Wrap, …` |
  | the scan's `internal/procfs/` skip removed | it failed with exactly `procfs`'s four calls: `procfs.go:58` and `:62`, and `procfs_linux.go:19` and `:38` |
  | `Strip` returning `err` | see Deviation 2 for the first run. After it: `TestWrapAndStrip` failed its four prefixed cases, `TestRunErrorNamesPackageOnce` its three, and the golden tests' `no-confirm` and `contradiction` cases (`no-confirm.text.stderr differs`; `contradiction.text.stderr differs`). For example, `Run = selfupdate: demo: selfupdate: pass --yes to apply without prompting: selfupdate: confirmation required` |
  | `trim`'s `strings.TrimPrefix` as `strings.ReplaceAll(s, prefix, "")` | `TestWrapAndStrip` failed `prefix later only`, `wrapped sentinel` and `join, both prefixed`. For example, `Wrap text = "selfupdate: op: read x: integrity check failed", want "selfupdate: op: read x: selfupdate: integrity check failed"` |
  | `target.go`'s `Wrap` literal given `TargetPolicy.ExecutablePath` | `TestRunTimeMessagesNameNoGoIdentifier` failed: `selfupdate/target.go:20: names TargetPolicy.ExecutablePath: "selfupdate: stat TargetPolicy.ExecutablePath": …` |
  | `detach.go:31`'s argument as `fmt.Errorf("x: %v", err)` | `TestBatchErrorChain` failed: `service/detach.go:31:14: errprefix.Wrap's error is not err itself (0022-MADR H1: …)`, and `the walk did not reach …/selfupdate/service.DetachProcess` |

* **Deviation 2's change.**
  * `Error()` returns `e.Op + ": " + Strip(e.Err).Error()`.
  * `make pre-add-check FILES="internal/errprefix/errprefix.go"`: `1 file(s) clean (gofmt, golangci-lint, go vet, go test, govulncheck)`.
  * `go test -count=1` of `internal/errprefix`, `selfupdate`, `selfupdate/cli`, `selfupdate/archive` and `internal/testproc` gave `ok` on all three hosts. On the Linux and Windows test hosts, the changed file was copied into the `a92d319` clone.
  * All six plants were run again on the changed tree, and each failed as the table says.
* The owner committed Deviation 2's change and this record as `16d0ed2`.

### Phase C3a

* **Step 1.** `CHANGELOG.md` has `## v1.26.1 - 2026-11-10`:
  * **Changed:** an error names the package once, with the refused confirmation's old and new text, `ErrCheckDeferred`, and what stays;
  * **Fixed:** issue #11.

  It links the guide's new section, the MADR and this PLAN.
* **Step 2.** The migration guide:
  * `### From v1.26.0 to v1.26.1` comes at the end of §26, before its `### Check`, in the form of `### From v1.22.0 to v1.22.1`;
  * it has a table of MADR F2's two cases and the symlink refusal, each old and new, then what stays, and "Adopting it";
  * the intro gains `v1.26.1`'s line after `v1.26.0`'s.

  The guide's other `v1.26.0` references wait for Phase C3b, as in `v1.26.0`'s release.
* **Step 3.**
  * `scripts/check-release-tag.sh v1.26.1`: exit 0. `git tag -l v1.26.1` printed nothing.
  * `make apicheck` at `16d0ed2` with these changes: `check-api-compat: compatible with v1.26.0`, and no other line but the ignored internal packages.
  * `make markdownlint` first refused four code spans ending in a space (`MD038/no-space-in-code`), one in `CHANGELOG.md` and three in the guide. After rewording it gave `0 issues`.
  * `make docs-check`: `997 links in 115 files, 0 broken`, the new `#from-v1260-to-v1261` anchor included, with 22 architecture rows and 0 differing.
  * `make check-records`: `102 records, 0 findings`. `make disclosure-check`: `746 files, 20 deny-list rules, 0 findings`.
  * `scripts/check-docs.sh --ids` on `CHANGELOG.md` and the guide: `2 files, 20 deny-list rules, 0 findings`.
* The owner committed Phase C3a as `9fc9dd7` and pushed it. They tagged `v1.26.1` on `9fc9dd7265c305caf995083663dd0df5b2a0ad39` (annotated, 2026-11-10, the changelog's date) and pushed the tag.

### Phase C3b

* **The tag's CI.** Run 42897931015 on `9fc9dd7`: `success` on its first attempt, with all 18 jobs passing. They include `validate` on ubuntu-24.04, macos-15 and windows-2025, `go126`, `installer-containers`, and both release rehearsals with their identity legs. The push to `main` (CI run 42897881484 and Docs run 42897881136) also passed. `git ls-remote origin 'refs/tags/v1.26.1^{}'` gave `9fc9dd7265c305caf995083663dd0df5b2a0ad39`.
* **Step 1.** `git diff --name-only v1.26.0 v1.26.1 -- .github internal/cmd 'scripts/*release*'` printed nothing, so the guides' statements about the workflows hold for `v1.26.1`.
* **Step 2.** The scratchpad's `pin1261.py` moved the pins from `v1.26.0` and `7c677842dface4b14b65b2e8b46aa0a7543bed13` to `v1.26.1` and `9fc9dd7265c305caf995083663dd0df5b2a0ad39`.
  * It asserts each count before it replaces, and asserts afterwards that no old commit remains and what `v1.26.0` mentions are left. It ran first on scratch copies of the four files, where every count matched, and the tree's result was byte-identical to the copies.
  * The pins it moved:
    * `README.md`: the current release, the `go get` line, and the publish pin (1 each);
    * `docs/architecture.md`: the current release and its commit;
    * `docs/guides/building-releases.md`: "examples below pin", the three workflow pins, and the `ls-remote` ref;
    * the migration guide: the current release, §3's publish pin, "`v1.17.2` to `v1.26.1` nothing" and its example pin, the `ls-remote` ref, §5's `go.mod` requirement, the `go get` lines (22, §2's to §26's) and the `go list` checks (20, §7's to §26's).
  * Kept as history, 8 in the guide: the intro's `v1.26.0` line, the `v1.26.1` line's link text, §26's prose, the old-text headers of §26's two tables, and the `v1.26.1` section's heading, prose and table header.
* **Step 3.** The release-history line, the guide's intro line for `v1.26.1`, was added in Phase C3a (its step 2), so nothing was left to add here.
* **Step 4.** This PLAN is `complete`, with its row in `docs/README.md`.
* **Step 5: checks.**
  * `make markdownlint`: `0 issues`.
  * `make check-records`: `102 records, 0 findings`.
  * `make docs-check`: `997 links in 115 files, 0 broken`, with 22 architecture rows and 0 differing.
  * `make disclosure-check`: `746 files, 20 deny-list rules, 0 findings`.
  * `scripts/check-docs.sh --ids` on the six changed files: `6 files, 20 deny-list rules, 0 findings`.
* **Step 6.** Closing issue #11, with one comment, waits for the owner's word.
* The owner committed and pushed this phase as `d7353b5`. On the owner's word, issue #11 was given one comment and closed as completed on 2026-11-10.

### Phases C4 to C5b

* Proposed on 2026-11-10 with MADR Amendment A1. Nothing is executed until the owner approves.

### Deviations

* **Deviation 1 (2026-11-10), Phase C1 step 8: the 0022 H1 walk does not follow `Wrap`.**
  * **Found.** After steps 6 and 7, `go test ./selfupdate/` failed `TestBatchErrorChain` (`selfupdate/spawn_test.go:194`), the static check that `errors.Is(err, ErrBatchFile)` holds at the exported entry points (0022-MADR H1):

    ```text
    spawn_test.go:215: service/detach.go:31:14: err is returned through a call the walk does not follow (0022-MADR H1: errors.Is(err, ErrBatchFile) must hold at the caller)
    spawn_test.go:219: the walk did not reach github.com/maccavelli/go-selfupdate-lib/selfupdate/service.DetachProcess: it follows less than it should
    ```

    `chainReturn` (`:408-427`) follows only `errors.Join` and `fmt.Errorf` with `%w`. Step 6 had turned `service/detach.go:31` into `errprefix.Wrap(…, err)`. The failure is new with this PLAN, not pre-existing. The PLAN had not listed the test, because it checks the code's shape, not text.
  * **Decided (owner, 2026-11-10),** from three options, the recommended one first: "Teach the walk Wrap".
    * `chainReturn` accepts `errprefix.Wrap(op, e)` when `e` itself is the second argument, as it accepts `errors.Join`. It still refuses `Wrap(op, f(e))` and every other call.
    * A run-time check beside `spawn_test.go:151` asserts that `errors.Is(errprefix.Wrap("x", winproc.ErrBatchFile), ErrBatchFile)` holds.
    * Phase C2 gains a plant: `detach.go:31`'s argument changed to `fmt.Errorf("x: %v", err)`, which the walk must refuse.
    * The options not chosen were "Teach Wrap and Strip" (no site uses `Strip`) and "Exempt the chain sites" (contradicts D1-A).
  * **Files added to Phase C1:** `selfupdate/spawn_test.go`.
  * No MADR decision or fact changes. `Wrap` keeps the chain, as its `Unwrap` and its table test show.
* **Deviation 2 (2026-11-10), Phase C2 step 2: `Wrap` was not built on `Strip`.**
  * **Found.** In the `Strip`-returns-`err` plant, `errprefix`'s table test failed its four `Strip` cases, but `cli`'s golden tests passed (`ok … selfupdate/cli`). Step 2's table says the six golden files fail.
    * Phase C1's `Error.Error()` called the unexported `trim` directly, and `Strip` called the same `trim`. So `Wrap` shared `Strip`'s removal but did not use `Strip`.
    * That contradicts MADR D2-A ("`Strip(err error) error`, the primitive under it") and the owner's choice ("built on Strip").
    * The other five plants failed as the table says.
  * **Decided (owner, 2026-11-10),** from three options, the recommended one first: "Build Error on Strip".
    * `Error()` returns `e.Op + ": " + Strip(e.Err).Error()`.
    * The `Strip` plant is run again, and must fail the table test, `TestRunErrorNamesPackageOnce` and the six golden files.
    * The options not chosen were "Amend the records" (keep the code, and change D2-A's wording) and "Both" (also a plant on `trim`).
  * **Files added to Phase C2:** `internal/errprefix/errprefix.go`. It is a follow-up commit to Phase C1's code. The pre-add check and the affected packages' tests run again.
  * The MADR is unchanged: the code now does what D2-A says.
