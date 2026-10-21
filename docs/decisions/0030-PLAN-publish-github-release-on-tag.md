---
status: rejected
date: 2026-10-21
associated-madr: "0030-MADR-publish-github-release-on-tag.md"
---
# Implement publishing this module's GitHub release from the tag's CI run

Associated MADR: [0030-MADR-publish-github-release-on-tag.md](0030-MADR-publish-github-release-on-tag.md)
(A, N1 and I1, as proposed).

**Rejected, not executed** (2026-10-21). The owner rejected the MADR
before approving this plan; see the MADR's Amendment A1. No phase ran.
The steps below are the plan as proposed.

## Goal

A `v*` tag whose `ci.yml` run passes in full gets a GitHub release made by
that run. The release has the tag's changelog section as notes, with
absolute links. With I1 it is immutable and verified. A tag that fails CI,
fails the tag rule, already has a release, or has no changelog section
gets none.

## Scope

### In scope

| Phase | What | Commit |
| :--- | :--- | :--- |
| R0 | this pair and its index rows | records only |
| R1 | `scripts/release-notes.sh` and its test | scripts |
| R2 | `ci.yml`'s `release` job, concurrency and contract step; the shape test | CI |
| R3 | immutable releases on for the repository (I1), on the owner's word | no commit |
| R4 | the documents: architecture's release procedure and CI bullet, the changelog | docs |
| R5 | closing; the live run is left to the next release | records |

### Out of scope

* `publish-selfupdate-release.yml` and `build-selfupdate-release.yml`,
  which callers pin.
* Releases for tags before `v1.17.2`. The owner chose to create only
  `v1.17.2`'s, by hand, on 2026-10-21.
* Re-creating `v1.17.2`'s release as immutable. It would mean deleting a
  published release.
* Prerelease tags. The tag rule runs with `[]`, so a `vX.Y.Z-NAME.N` tag
  fails the job. No such tag exists here.
* A release of this module: the change is CI, scripts and documents.
  The next release's changelog names it.

## Rules

* AGENTS.md's gate and records rules. `actionlint` on `ci.yml`;
  `check-workflows.sh` with all four rules on `ci.yml`.
* A new check is seen failing on a scratch copy (`scripts/plant-copy.sh`)
  before it is relied on.
* Each phase is one commit on the owner's word; each push on the owner's
  word.

## Implementation Steps

### Phase R0: records

This pair and `docs/README.md`'s rows for it.

### Phase R1: the notes script

1. **`scripts/release-notes.sh TAG [CHANGELOG]`**, `#!/bin/sh`, with the
   repository's header-comment style (usage, exit codes, the MADR):
   * `CHANGELOG` defaults to `CHANGELOG.md` at the repository root;
   * `GITHUB_REPOSITORY` must be set, as `owner/name`; without it, exit 2;
   * prints the lines after `## TAG - ` up to the next `## `, with leading
     and trailing blank lines dropped;
   * rewrites every Markdown link whose target is not `http(s)://`,
     `mailto:` or `#…` to
     `https://github.com/$GITHUB_REPOSITORY/blob/TAG/<target>`, keeping
     any `#anchor`;
   * exits 1 when the section is missing or its body is empty, and 2 on a
     usage error.

   Like `check-release-tag.sh`, it does its parsing in an embedded
   `python3 -B`.
2. **`scripts/release-notes_test.sh`,** fixtures written to a temp
   directory:
   * a fixture's middle section is cut cleanly, its neighbours excluded;
   * relative, anchored, absolute and in-page links rewritten or kept;
   * a missing tag, an empty section, a tag that is a prefix of another
     (`v1.1.1` against `v1.1.10`), and an unset `GITHUB_REPOSITORY` fail
     with the right code;
   * the real `CHANGELOG.md`'s `v1.17.2` section leaves no relative link.
3. **Seen failing:** on `plant-copy.sh` copies, the test fails when
   the section match drops its ` - ` anchor (the prefix case), and when
   the rewrite skips anchored links.
4. **Checks:** `shellcheck` on both files, the test, and `make gate`.

### Phase R2: the `release` job

1. **`ci.yml`:**
   * `concurrency.cancel-in-progress` becomes
     `${{ github.ref_type != 'tag' }}`; the comment cites this pair and
     says why;
   * a new last job, `release`:
     * `if: github.ref_type == 'tag'`;
     * `needs: [validate, go126, installer-containers, release-rehearsal,
       release-rehearsal-archive, release-rehearsal-check]`;
     * `permissions: contents: write`; `runs-on: ubuntu-24.04`;
       `timeout-minutes: 10`;
     * job-level `env:` `GH_TOKEN: ${{ github.token }}`,
       `GH_REPO: ${{ github.repository }}` and
       `TAG: ${{ github.ref_name }}`;
     * steps:
       1. checkout, pinned as the other jobs pin it,
          `persist-credentials: false`;
       2. "Require an admitted tag":
          `sh scripts/check-release-tag.sh "$TAG" '[]'`;
       3. "Refuse an existing release":
          `scripts/refuse-existing-release.sh "$TAG"`;
       4. "Write the release notes": `scripts/release-notes.sh "$TAG" >
          "$RUNNER_TEMP/notes.md"`;
       5. "Create the release": the latest flag from
          `release-latest-flag.sh`, then
          `gh release create "$TAG" --verify-tag --title "$TAG"
          --notes-file "$RUNNER_TEMP/notes.md"` with that flag when set;
       6. "Wait for immutability and release attestation": the publish
          workflow's last step, restated, with this repository's message.
   * a comment block over the job cites this pair, says why it does not
     use `publish-selfupdate-release.yml`, and says a new job must be
     added to `needs`.
2. **The contract step** adds
   `./scripts/check-workflows.sh --rule gh-repo .github/workflows/ci.yml`.
3. **`scripts/ci-release-shape_test.sh [CI-WORKFLOW]`,** PyYAML, run by
   the contract step and by the gate's script tests. It asserts:
   * the `release` job's `if` is exactly `github.ref_type == 'tag'`;
   * its `needs` is the set of every other job in the file;
   * it is the only job with any `write` permission, and the top level is
     `contents: read`;
   * no step in it has `continue-on-error`;
   * the tag check, the existing-release check and the notes step come
     before "Create the release";
   * `cancel-in-progress` is the tag expression.

   Like `workflow-shape_test.sh`, it exits 2 without PyYAML.
4. **Seen failing,** on `plant-copy.sh` copies of `ci.yml`, run with the
   shape test and the `gh-repo` rule:
   * `go126` dropped from `needs`;
   * the `if:` removed;
   * `contents: write` at the top;
   * "Create the release" moved before the existing-release check;
   * `GH_REPO` removed from the job's `env:` (`gh-repo` rule).
5. **Checks:** `actionlint`; `check-workflows.sh`'s four rules on
   `ci.yml` and its defaults; `check-workflows_test.sh`,
   `workflow-shape_test.sh`, the new test; and `make gate`.

### Phase R3: immutable releases (I1)

On the owner's word, since it changes a live repository setting:

1. `gh api -X PUT repos/maccavelli/go-selfupdate-lib/immutable-releases`,
   or the owner sets it under Settings > General > Releases.
2. `gh api repos/maccavelli/go-selfupdate-lib/immutable-releases` reads
   `"enabled":true`; the output is recorded.

If the owner chooses I2, R3 is not run, step 6 of R2's job is left out,
and the MADR records I2.

### Phase R4: the documents

1. **`docs/architecture.md`, "Releasing this module":**
   * step 5: the tag's CI run, every job, then its `release` job creates
     the GitHub release from the changelog section;
   * step 1: the changelog section is now also the release notes, and the
     tag fails to release without it;
   * a new last step: the release exists, and with I1, `isImmutable` and
     `gh release verify`.
2. **The CI bullet** under "Tooling": the `release` job, on a tag only.
3. **`CHANGELOG.md`:** `## Unreleased`, Changed: a tag's CI run publishes
   its GitHub release, with the pair linked.
4. **Checks:** links, ids, markdownlint, the arch table.

### Phase R5: closing

1. The next release's PLAN records its tag run: the `release` job's
   result and run ID, the release URL, its notes, `isImmutable` and
   `gh release verify`. This plan names that as V4, left to it.
2. `### Verification, at closing`, `status: complete`, the index row.

## Verification

* **V1 (notes):** `release-notes.sh` cuts `v1.17.2`'s section with every
  link absolute; its test passes and failed on its planted breaks.
* **V2 (job shape):** the shape test and the four workflow rules pass on
  `ci.yml`; each planted break in R2 step 4 failed.
* **V3 (setting):** immutable releases read `enabled: true` (I1).
* **V4 (live):** the next tag's run created its release, left to that
  release's PLAN.
* **V5 (docs):** the release procedure and the changelog say so; links,
  ids and markdownlint clean; `make gate` `overall=0`.

## Rollout and Rollback

* **Rollout:** R1, R2 and R4 on `main`, pushed on the owner's word; R3
  on the owner's word. Nothing runs until the next tag.
* **Rollback:** `git revert` of R2 removes the job, and a release is made
  by hand again: `gh release create TAG --verify-tag --notes-file …`
  with `release-notes.sh`'s output. Immutable releases can be turned off
  in the setting, but a release already published stays immutable.
* **A failed release job on a tag:** the tag stays, without a release.
  If the job failed before "Create the release", fix the cause and
  re-run the failed job. Otherwise read the step's output first, since
  the release may exist already.

## Execution record

### Rejection (2026-10-21)

The owner rejected the pair before approving it ("delete the release and
flag the pair rejected"). No phase ran:

* R0 was not committed as `proposed`. The pair is recorded as
  `rejected`.
* R1 to R5 made no change: no script, no `ci.yml` job and no test was
  written; the immutable-releases setting stays off; no document
  changed.

The one live change in this pair's span came before it. The `v1.17.2`
release was made by hand on 2026-10-21 and deleted the same day, and
the tag was kept. The MADR's Amendment A1 records the deletion and what
was checked after it.
