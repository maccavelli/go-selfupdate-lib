---
status: in-progress
date: 2026-10-03
associated-madr: "0010-MADR-remediate-second-debugging-pass-findings.md"
---
# Implement the tooling fixes and the roadmap amendment (no release)

Associated MADR: [0010-MADR-remediate-second-debugging-pass-findings.md](0010-MADR-remediate-second-debugging-pass-findings.md)

## Goal

Close the release-tooling, CI and build findings on `main`, with no tag:
D1–D11, D13 and D15. Then amend 0004 for the roadmap (Q8).

Every gate this PLAN touches is seen to fail on the break it is meant to
catch, in its own `*_test.sh` or by a planted break, before the fix lands.

## Scope

### In scope

| Phase | Findings | Paths |
| :--- | :--- | :--- |
| T1 | D1 | `scripts/check-release-tag.sh`, its test |
| T2 | D2, D8 | `scripts/verify-selfupdate-release.sh`, its test |
| T3 | D3, D4 | `scripts/go-precheck.sh`, a new `scripts/go-precheck_test.sh` |
| T4 | D5, D6, D9 (Q7) | `.github/workflows/publish-selfupdate-release.yml`, `scripts/check-workflows.sh`, a new `scripts/release-latest-flag.sh`, their tests |
| T5 | D7, D10, D11 | `.github/workflows/ci.yml`, `Makefile` |
| T6 | D13 | `.golangci.yml` |
| T7 | D15 | `README.md` |
| T8 | Q8, the roadmap | `docs/decisions/0004-MADR-evolve-selfupdate-api-and-tui-support.md`, `docs/README.md` |

### Out of scope

* **Every Go source change.** Those are in
  [0010-PLAN-v1-5-1-contract-preserving-fixes.md](0010-PLAN-v1-5-1-contract-preserving-fixes.md)
  and
  [0010-PLAN-v1-6-0-owner-contracts.md](0010-PLAN-v1-6-0-owner-contracts.md).
* **Moving the README's and the migration guide's `uses:` pin.** The
  v1.5.1 PLAN moves it to the `v1.5.1` commit, which carries T4.
* **Building any Phase 4 package.** T8 only schedules them.
* **Push and tags,** which are the owner's.

## Rules for every phase

1. **Order.** T1–T8, each in its own commit.
2. **Commits.** `git commit --no-edit`, after the owner authorizes commits to
   `main` in that turn.
3. **Proof first.** Each new test case is added, and seen to fail against
   the unfixed script on a scratch copy, before the fix.
4. **Checks before each commit:**
   * every `scripts/*_test.sh`;
   * shellcheck;
   * actionlint v1.7.12;
   * markdownlint-cli2;
   * `scripts/check-workflows.sh`.

   T5 and T6 also run `make lint` and `make pre-add-check`.
5. **Session tooling is Python.** The repository's own scripts run as they
   always do.

## Implementation Steps

### Phase T0: records

1. The MADR is `accepted` with the owner's answers, and the three PLANs are
   `proposed`. Once approved, this PLAN is `in-progress`.
2. `docs/README.md` indexes the MADR and the three PLANs.
3. Commit the records alone.

### Phase T1: the tag gate (D1)

1. **Test first.** Add to `check-release-tag_test.sh`:
   * `v1.0.1١` (Arabic-Indic digit) refused with `[]`;
   * `v1.0.0-rc.1٣` refused with `["rc"]`;
   * a full-width `v１.0.0` refused.
2. **Fix.** In `core_re` and `num_re`, `[0-9]` in place of `\d`.
3. **Prove the mirror.** A scratch Go program runs each new case through
   `NewStrictVersionPolicy` and `NewSemverPolicy`. It must agree with the
   gate on every case of the test file.

### Phase T2: the release verifier (D2, D8)

1. **Tests first,** as `run_fail` cases:
   * a binary whose bytes differ from its `SHA256SUMS` line;
   * a `SHA256SUMS` that also lists an extra such as `install.sh`;
   * an extra named `SHA256SUMS-x`;
   * a zero-byte canonical binary.
2. **Fix D8.** Refuse a canonical binary of size 0, with
   `verify-selfupdate-release: <name> is empty`.
3. **Prove D2.** On scratch copies, replace each of the two core checks with
   `if False:`. The test must now fail. It survived both before.

### Phase T3: the pre-add check (D3, D4)

1. **A new `scripts/go-precheck_test.sh`,** with stubbed `golangci-lint`,
   `go` subcommands and `govulncheck` on `PATH`. It covers:
   * a deleted `.go` argument: its package is still linted, vetted and
     tested, and a package that no longer builds fails;
   * an unreachable vulnerability database: exit 2, unless
     `GO_PRECHECK_SKIP_VULN=1`, which prints "skipped";
   * the summary line names only the checks that ran.
2. **Fix D3.** A `.go` argument that is not on disk maps to its directory's
   package. When the whole directory is gone, the check falls back to
   `./...`.
3. **Fix D4.** Remove the "could not reach … skipped" exit 0. The summary
   lists govulncheck only when it ran.
4. ci.yml gains a step that runs the new test, as it has one for each
   script test.

### Phase T4: the release workflow and its checker (D5, D6, D9)

1. **D6.** `check-workflows.sh` strips comments with `shlex`
   (`comments=True`), and accepts an optional path before `gh`. New cases:
   * `echo "build #1"; gh release create …` with no `GH_REPO`;
   * `/usr/bin/gh release view …` with no `GH_REPO`.

   Both must fail.
2. **D5.** The capability step parses `gh release --help` for `verify`, and
   `gh release view --help` for `isImmutable`. It also requires a
   minimum `gh version`: the first release with both, read from gh's
   release notes when this phase runs, and recorded here. A check in
   `check-workflows_test.sh` asserts that the step has no `--help`
   exit-code probe.
3. **D9 (Q7).** A new `scripts/release-latest-flag.sh TAG`:
   * for a stable tag, it reads the current latest with `gh release view
     --json tagName`, and compares the cores numerically;
   * it prints `--latest=false` when the tag is lower, and nothing
     otherwise;
   * a prerelease is unchanged: it is never "latest".

   The workflow passes the output to `gh release create`, and writes a
   line to the job summary when it is `--latest=false`.
4. **D9's test,** `release-latest-flag_test.sh`, uses a stubbed `gh`. It
   covers higher, equal, lower, no latest yet, a prerelease tag, and a gh
   failure, which must exit non-zero. ci.yml gains a step that runs it.

### Phase T5: CI and the Makefile (D7, D10, D11)

1. **D7.** ci.yml gains a top-level `permissions: contents: read`, and
   `persist-credentials: false` on every checkout. `check-workflows.sh`
   gains a rule that every workflow has a top-level `permissions:`, with a
   failing case.
2. **D10.** Wrap `$(GOPATH_BIN)/…` in `$(wildcard …)` for `GOVULNCHECK` and
   `GOTESTSUM`. Prove it on a scratch copy: a stub only on `PATH` is found.
   It was "not found" before.
3. **D11.** The `make lint` hint names `@v2.14.0`.

### Phase T6: depguard's catch-all (D13)

1. A new rule, sorting after `banned`, covers every package that no other
   rule names. It allows only `$gostd` and this module.
2. **Prove it** on a scratch copy:
   * a planted `internal/helper` importing `golang.org/x/sys/unix` is
     refused;
   * each existing package still lints clean on three GOOS.
3. AGENTS.md's depguard paragraph names the catch-all.

### Phase T7: the recovery runbook (D15)

`README.md`, under the reusable workflow, gains "When a publish fails":

* before publication, delete the draft with `gh release delete TAG
  --yes`, and re-run the job;
* after publication, the tag is immutable: fix forward with a new patch
  tag.

Check it with markdownlint and the link checker.

### Phase T8: the roadmap amendment (Q8)

A dated amendment to 0004, in its Amendments section. It:

* schedules `selfupdate/archive` in Phase 4, beside `codesign`;
* restates that Phase 4's `codesign`, `service/*`, the build-and-stage
  workflow and the installer templates stay planned, each needing its own
  PLAN, and the workflow its own record;
* notes that 0009 narrowed the module to self-update, and that each of
  these is self-update tooling.

`docs/README.md`'s 0004 row is unchanged; its status stays `accepted`.

## Verification

* **V1.** Each new test case failed on the unfixed script, and passes after
  the fix. The output is recorded per phase.
* **V2.** Every `scripts/*_test.sh`, shellcheck, actionlint,
  markdownlint-cli2 and `check-workflows.sh` pass at the end of every
  phase.
* **V3.** CI is green on `main` after the owner's push, including the new
  `permissions:` block.
* **V4.** The release workflow's changes are exercised by their script tests
  here. Their first live run is the `v1.5.1` tag, recorded in that PLAN.

## Rollout and Rollback

* **Rollout.** Commits on `main`, no tag. Consumers pick up T4 when the
  README's pin moves to `v1.5.1`.
* **Rollback.** Each phase reverts alone.

## Execution Record

### Phase T0: records (2026-10-03)

The owner approved the three PLANs ("plans are approved. proceed and you
have explicit permissions to commit"). The MADR is `accepted`, this PLAN is
`in-progress`, and the v1.5.1 and v1.6.0 PLANs stay `proposed` until each
starts. The records are committed alone.

### Phase T1: the tag gate, D1 (2026-10-03)

* **Test first.** `check-release-tag_test.sh` gains three cases:
  `v1.0.1` + U+0661, `v1.0.0-rc.1` + U+0663, and `v` + U+FF11 + `.0.0`. Each
  must be refused on both channel lists. Against the old gate:

  ```text
    FAIL [v1.0.1١] on ["rc","beta","alpha"]: want exit 1, got 0
    FAIL [v1.0.1١] stable only: want exit 1, got 0
    FAIL [v1.0.0-rc.1٣] on ["rc","beta","alpha"]: want exit 1, got 0
  check-release-tag_test: 54 passed, 3 failed
  ```

  The full-width case was already refused, because `[1-9]` leads the
  number. It stays as a guard.
* **Fix.** `core_re` and `num_re` use `[0-9]`, with a comment that cites D1.
  After the fix: `check-release-tag_test: 57 passed, 0 failed`.
* **The mirror.** On a scratch copy, a probe test ran all 21 tags of the
  test file through `NewSemverPolicy` (`AllowPrerelease`, channels
  `rc,beta,alpha`) and `NewStrictVersionPolicy`. It compared them with the
  gate's answers: `21 tags agree`.
* **Checks:** shellcheck rc 0; every `scripts/*_test.sh` rc 0.

### Phase T2: the release verifier, D2 and D8 (2026-10-03)

* **Tests first.** `verify-selfupdate-release_test.sh` gains four
  `run_fail` cases:
  * a binary whose bytes differ from its `SHA256SUMS` line;
  * a `SHA256SUMS` that also lists `install.sh`;
  * an extra named `SHA256SUMS-x`;
  * a zero-byte canonical binary.

  Against the unfixed verifier, only the last failed, as D8 said:
  `not ok - empty canonical binary (expected failure)`.
* **Fix D8.** Before hashing, a canonical binary of size 0 fails with `<name>
  is empty`.
* **Prove D2.** On scratch copies, each check was disabled in turn. The
  test failed each time, on the new case meant for it:

  ```text
  digest check off: test rc=1 ['not ok - binary bytes differ from its SHA256SUMS line (expected failure)']
  exact-set check off: test rc=1 ['not ok - SHA256SUMS also lists an extra (expected failure)']
  SHA256SUMS- refusal off: test rc=1 ['not ok - extra named SHA256SUMS-x (expected failure)']
  D8 check off: test rc=1 ['not ok - empty canonical binary (expected failure)']
  real tree: test rc=0 []
  ```

* **Checks:** shellcheck rc 0; every `scripts/*_test.sh` rc 0.

### Phase T3: the pre-add check, D3 and D4 (2026-10-03)

* **Test first.** The new `scripts/go-precheck_test.sh` has seven cases. It
  stubs `go`, `gofmt`, `golangci-lint` and `govulncheck` on `PATH`, in a
  throwaway repository. Against the unfixed script:

  ```text
    FAIL deleted file: rc=0 out=[go-precheck: no Go files to check.] calls=[]
    FAIL deleted package: rc=0 out=[go-precheck: no Go files to check.] calls=[]
    ok   an existing file is formatted, vetted and tested
    ok   non-Go arguments alone: nothing to check
    FAIL unreachable database: rc=0 out=[govulncheck: could not reach the vulnerability database; skipped. …]
    FAIL skip summary: rc=0 out=[… clean (gofmt, golangci-lint, go vet, go test, govulncheck).]
    ok   a run govulncheck is named in the summary
  go-precheck_test: 3 passed, 4 failed
  ```

* **Fix D3.** Every Go argument names its package directory, whether or not
  the file still exists. `gofmt` runs only over the files that exist. A
  directory with no Go file left widens `go vet` and `go test` to `./...`.
* **Fix D4.** An unreachable vulnerability database fails with exit 2, and
  names `GO_PRECHECK_SKIP_VULN=1`. The summary lists only the checks that
  ran.
* **After the fix.** On the fixed script, `go-precheck_test: 7 passed, 0
  failed`. On `HEAD`'s script, given as `SCRIPT`: `3 passed, 4 failed`.
* **For real, with the real tools:**
  * a full run reports `113 file(s) clean (gofmt, golangci-lint, go vet, go
    test, govulncheck).`;
  * D3's reproduction, a scratch clone with `selfupdate/version.go`
    deleted, now exits 1 with `undefined: validateProduct`.
* **CI.** ci.yml runs the new test on Linux, beside the other script tests.
* **Checks:**
  * shellcheck on both scripts, after rewriting three `A && B || C` lines in
    the test as an `rc0` helper (SC2015);
  * actionlint v1.7.12;
  * `check-workflows.sh`;
  * every `scripts/*_test.sh`.

### Phase T4: the release workflow and its checker, D5, D6, D9 (2026-10-03)

* **Tests first.** `check-workflows_test.sh` gains six cases:
  * `echo "build #1"; gh release create` in a block scalar, with no
    `GH_REPO`;
  * `/usr/bin/gh release view`, with no `GH_REPO`;
  * a real trailing comment holding a `gh` call (allowed);
  * `release-latest-flag.sh` with no `GH_REPO`;
  * no `--help` exit-code probe in the release workflow;
  * the latest-flag script used twice, once to create and once to publish.

  Against `HEAD`'s checker and workflow, exactly the five that must fail
  did: `25 passed, 5 failed`.
* **D6.** `check-workflows.sh` tokenizes each run script:
  * a quote-aware pass strips comments and keeps newlines;
  * `shlex`, with `punctuation_chars=";&|()\n"`, splits commands;
  * a `gh` word is matched by basename, then a repository-scoped group;
  * `refuse-existing-release.sh` and `release-latest-flag.sh` count as gh
    calls.

  An unbalanced quote falls back to whitespace words, so a parse failure
  never hides a call.
* **D5.** The capability step parses help text, not exit codes:
  * `gh release --help` must list `verify:`;
  * `gh release view --help` must name `isImmutable`;
  * `gh --version` must be at least 2.81.0. The floor is read from gh's own
    release notes: 2.76.0 added "Display immutable field in `release view`",
    and 2.81.0 "introduces the `release verify` and `release verify-asset`
    commands".
* **D9 (Q7).** The new `scripts/release-latest-flag.sh TAG` prints
  `--latest=false` for a stable tag below the current latest, compared
  numerically. A prerelease always gets `--latest=false`. The first
  release, or a higher tag, gets nothing. Any gh failure other than
  "release not found" exits 1.
  * The create step keeps 0005's prerelease `case` block unchanged. Its
    shape is asserted by `check-release-tag_test.sh`. For a stable tag, it
    adds the script's flag, and writes a line to the job summary.
  * The publish step applies the flag again, because publication is where
    GitHub picks the latest release.
* **`release-latest-flag_test.sh`,** with a stubbed `gh`, passes `10 passed,
  0 failed`. Plants on scratch copies fail it:
  * a lexical comparison fails both "numeric, not lexical" cases;
  * dropping the prerelease branch fails "prerelease", and "a prerelease
    asks gh nothing".
* **Fixed during the phase:**
  * My first tokenizer joined lines with ` ; `, so a comment line swallowed
    the rest of the script. Three existing cases caught it. Comments are
    now stripped before tokenizing, with newlines kept as separators.
  * The first version of my `# inside quotes` plant used a plain YAML
    scalar, where ` #` is a YAML comment. It now uses `run: |`.
  * The existing expression plant was retargeted from `gh release edit
    "$TAG" --draft=false` to `gh release edit "$TAG"`, because the publish
    step now passes its arguments as an array.
* **CI.** ci.yml runs `release-latest-flag_test.sh` on Linux.
* **Checks:**
  * `check-workflows_test.sh`: `30 passed, 0 failed`;
  * `check-workflows.sh` over both workflows: ok;
  * actionlint v1.7.12;
  * shellcheck on `scripts/*.sh`;
  * every `scripts/*_test.sh`.

### Phase T5: CI and the Makefile, D7, D10, D11 (2026-10-03)

* **D7, test first.** `check-workflows_test.sh` gains three cases:
  * "release workflow, permissions": exit 0;
  * "ci workflow, permissions": exit 0;
  * a copy of ci.yml with its top-level `permissions:` block removed:
    exit 1.

  Against the old checker all three failed, with exit 2: it had no such
  rule.
* **D7, fix.**
  * `check-workflows.sh` gains a `permissions` rule, also run under
    `all`: a workflow without a top-level `permissions:` is a finding.
  * ci.yml gains `permissions: contents: read`, and `persist-credentials:
    false` on its one checkout. No CI step uses the token or gh. CI runs
    the new rule over ci.yml, beside the expressions rule.
  * After the fix: `33 passed, 0 failed`.
* **D10.** `$(GOPATH_BIN)/…` is wrapped in `$(wildcard …)` for
  `GOVULNCHECK` and `GOTESTSUM`. Proved with `GOENV=off`, an empty
  `GOPATH`, and a stub govulncheck only on `PATH`:
  * `HEAD`'s Makefile gave `govulncheck not found…`, rc 2;
  * the new one ran the stub, rc 0.
* **D11.** The `make lint` install hint names `golangci-lint@v2.14.0`.
* **Checks:**
  * `make lint`: 0 issues on all three GOOS;
  * `make pre-add-check`: `113 file(s) clean (gofmt, golangci-lint, go vet,
    go test, govulncheck).`;
  * actionlint v1.7.12;
  * `check-workflows.sh` over both workflows;
  * shellcheck;
  * every `scripts/*_test.sh`.
