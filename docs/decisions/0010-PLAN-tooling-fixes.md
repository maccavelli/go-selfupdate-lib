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
