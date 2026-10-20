---
status: in-progress
date: 2026-10-20
associated-madr: "0028-MADR-skip-full-ci-for-docs-only-pushes.md"
---
# Implement skipping the full CI for docs-only pushes, with a docs workflow

Associated MADR: [0028-MADR-skip-full-ci-for-docs-only-pushes.md](0028-MADR-skip-full-ci-for-docs-only-pushes.md)
(B, as the owner chose).

## Goal

A push or pull request that changes only Markdown runs one Linux docs job
instead of `ci.yml`'s 18. Any other change, and every tag, runs `ci.yml`
as before.

## Scope

### In scope

| Phase | What | Commit |
| :--- | :--- | :--- |
| Y0 | this pair and its index rows | records only |
| Y1 | `ci.yml`'s filters, `docs.yml`, and CI's workflow checks covering it | CI |
| Y2 | the documents: architecture's CI bullet and release procedure, AGENTS.md, the changelog | docs |
| Y3 | the live checks, and closing | records |

### Out of scope

* Any change to `ci.yml`'s jobs or steps.
* The build and publish workflows, which callers run in their own
  repositories.
* Required status checks: none exist (0025-MADR Amendment A5).
* No release follows: the change is CI and documents only. The next
  release's changelog names it.

## Rules

* AGENTS.md's gate and records rules. `actionlint` on each changed
  workflow; `scripts/check-workflows.sh` with its expression, permission
  and pin rules on both workflows.
* A new check is seen failing on a scratch copy before it is relied on.
* Each phase is one commit on the owner's word; each push on the owner's
  word.

## Implementation Steps

### Phase Y0: records

This pair and `docs/README.md`'s rows for it.

### Phase Y1: the filters and the docs workflow

1. **`ci.yml`:**
   * `on.push` gains `paths-ignore: ['**.md']`, beside its `branches` and
     `tags`. GitHub does not evaluate path filters for tag pushes, so
     tags keep the full run.
   * `on.pull_request` gains the same.
   * A comment cites this pair and states the tag rule.
2. **`.github/workflows/docs.yml`,** new:
   * the triggers are `push` to `main` and `release/**`, and
     `pull_request`, each with `paths: ['**.md',
     '.markdownlint-cli2.jsonc', '.github/workflows/docs.yml']`;
   * `permissions: contents: read`, and a concurrency group
     `docs-${{ github.ref }}` that cancels an older run;
   * one `ubuntu-24.04` job with `timeout-minutes: 10`;
   * checkout pinned by SHA, as `ci.yml`'s is, with `persist-credentials:
     false`;
   * it runs the commands `ci.yml` runs for these checks, unchanged:
     `npx --yes markdownlint-cli2@0.23.2`, `git ls-files -z '*.md' |
     xargs -0 ./scripts/check-docs.sh --links`, and
     `./scripts/check-docs.sh --arch docs/architecture.md`;
   * a header comment says what it is for and cites this pair.
3. **CI covers the new workflow:** the "Verify the workflow contract" step
   runs the expression, permission and pin rules on `docs.yml` too.
   `actionlint` already reads every workflow.
4. **Seen failing,** on `scripts/plant-copy.sh` copies, by running the
   docs job's three commands:
   * a broken link in a guide fails the link check;
   * a markdownlint error in the README fails markdownlint;
   * a package-table count off by one fails the arch check;
   * the tree passes all three.

   Also `scripts/check-workflows.sh --rule pins` on a copy of `docs.yml`
   with the checkout pinned by tag fails.
5. **Checks:** `actionlint`, the workflow checker's rules and tests, the
   workflow-shape test, and `make gate`.

### Phase Y2: the documents

1. **`docs/architecture.md`:**
   * the CI bullet says a Markdown-only push or pull request runs
     `docs.yml` instead, and that a tag always runs the full CI;
   * "Releasing this module", step 3: a docs-only pre-tag commit gets the
     docs workflow on `main`, and the tag's full run is the code proof.
2. **AGENTS.md:** where it says what CI runs, one sentence on the split.
3. **`CHANGELOG.md`:** an `## Unreleased` entry, Changed: a docs-only
   change runs a docs job, not the full CI.
4. **Checks:** links, ids, markdownlint.

### Phase Y3: live checks and closing

1. After Y1 and Y2 are pushed, on the owner's word:
   * the next docs-only push to `main` shows a "Docs" run and no "CI" run;
   * a push that changes a non-Markdown file shows "CI".

   Both run IDs are recorded. Y1's own push changes workflow files, so
   it runs both.
2. The next release's tag shows a full "CI" run; that release's PLAN
   records it, and this plan names it as left to that release.
3. `### Verification, at closing`, `status: complete`, the index row.

## Verification

* **V1 (filters):** `ci.yml` has `paths-ignore` on push and pull request;
  `docs.yml` exists with the three checks; actionlint and the workflow
  rules pass on both.
* **V2 (seen failing):** each planted break fails its check.
* **V3 (live):** a docs-only push ran only "Docs"; a code push ran "CI".
* **V4 (tags):** the next tag ran the full CI, left to that release.
* **V5 (docs):** architecture, AGENTS.md and the changelog say so; links,
  ids and markdownlint clean; `make gate` `overall=0`.

## Rollout and Rollback

* **Rollout:** Y1 and Y2 on `main`, pushed on the owner's word.
* **Rollback:** `git revert` of Y1 restores the full CI for every push.
  Nothing outside the repository depends on the split.

## Execution record

### Approval (2026-10-20)

The owner chose B and approved the pair ("Approve B and proceed"). The
MADR became `accepted`, and this plan `in-progress`.

### Y0 (2026-10-20)

This pair, and `docs/README.md`'s rows for it: the MADR `accepted`, this
plan `in-progress`. Links, ids and markdownlint clean.

### Y1 (2026-10-20)

1. **`ci.yml`:**
   * `on.push` gains `paths-ignore: ['**.md']` beside `branches` and
     `tags`, and `on.pull_request` the same. A comment cites this pair and
     says a tag push is never filtered.
   * The "Verify the workflow contract" step runs the expression,
     permission and pin rules on `docs.yml` too.
2. **`.github/workflows/docs.yml`,** new:
   * `push` to `main` and `release/**`, and `pull_request`, each with
     `paths: ['**.md', '.markdownlint-cli2.jsonc',
     '.github/workflows/docs.yml']`;
   * top-level `permissions: contents: read`, and the concurrency group
     `docs-${{ github.ref }}` with `cancel-in-progress`;
   * one `ubuntu-24.04` job, `timeout-minutes: 10`. Its checkout is
     pinned to `ci.yml`'s SHA with `persist-credentials: false`, and it
     runs `ci.yml`'s three commands unchanged: markdownlint, the link check
     over `git ls-files -z '*.md'`, and the arch check;
   * a header comment.

   Like `ci.yml`, it uses the runner image's Node and Python, with no
   setup action.
3. **Seen failing.** The three commands ran verbatim on the tree and on
   `scripts/plant-copy.sh` copies. Each copy was given a throwaway git
   index, since a copy has no `.git` and both checks list files with
   git. All output went to the scratch directory.

   | Run | markdownlint | links | arch |
   | :--- | :--- | :--- | :--- |
   | the tree | 0 | 0, "682 links in 79 files, 0 broken" | 0, "18 rows …, 0 differ" |
   | a broken anchor in `building-releases.md` | 0 | **1**, `docs/guides/building-releases.md:387: missing anchor: ../../README.md#no-such-heading` | 0 |
   | `##Status` in README | **1**, `README.md:24:1 error MD018/no-missing-space-atx` | 0 | 0 |
   | `buildinfo`'s count 1 → 2 | 0 | 0 | **1**, `docs/architecture.md:136: buildinfo: table 2/3, tree 1/3` |

   Each plant fails only its own check. `check-workflows.sh --rule pins`
   on a copy of `docs.yml` with `actions/checkout@v7.0.1` exits 1:
   `jobs.docs.steps[0]: action not pinned to a commit SHA`.
4. **Checks:**
   * `actionlint` on both workflows;
   * `check-workflows.sh`: the expression, permission and pin rules on
     `docs.yml`; pins on `ci.yml`; and all rules on the two release
     workflows;
   * `check-workflows_test.sh` and `workflow-shape_test.sh`;
   * `make gate` with `SELFUPDATE_REQUIRE_PYTHON=1`: `overall=0`, links 682
     in 79 files, 0 broken.

   All pass. The filters themselves are proven only on GitHub, at Y3.

### Y2 (2026-10-20)

1. **The push of Y1:** the owner committed Y1 as `d16b58a` and pushed
   `efee0cb`, `286d32e` and `d16b58a` together ("committed and pushed").
   The guard over `33aa6f5..d16b58a` exits 0. The push changed workflow
   files and Markdown, so both workflows started: "CI" `39479631637` and
   "Docs" `39479631571`, the first Docs run. Their results go in Y3.
2. **`docs/architecture.md`:**
   * the CI bullet: a Markdown-only push or pull request skips CI, and
     `docs.yml` runs its three checks on one Linux runner instead; a tag
     push is never filtered;
   * "Releasing this module", step 3: a docs-only pre-tag commit runs only
     the docs workflow on `main`, and the tag's full run in step 5 is the
     code proof.
3. **AGENTS.md:** one sentence after the gate's by-hand list, citing the
   MADR in backticks, as AGENTS.md cites records.
4. **`CHANGELOG.md`:** `## Unreleased`, Changed: the split, with the pair
   linked.
5. **Checks:** links 102 in 3 files, 0 broken; markdownlint 0 issues;
   ids 0 findings; arch 0 differ. This commit changes Markdown only, so
   its push is Y3's docs-only test.
