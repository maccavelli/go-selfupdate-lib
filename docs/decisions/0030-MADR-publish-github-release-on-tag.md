---
status: rejected
date: 2026-10-21
decision-makers: go-selfupdate-lib maintainers
consulted: "maccavelli/magic-cli-remote's .github/workflows/ci.yml at 3e6d15b5; this repository's CI, publish workflow and release scripts at 18849cd; the GitHub releases API, read 2026-10-21"
informed: the repository's owner
---
# Publish this module's GitHub release from the tag's own CI run, after every job passes, with notes from the changelog

## Context and Problem Statement

A `v*` tag of this module runs `.github/workflows/ci.yml` in full, and
the Go module proxy serves the tag. No workflow creates a GitHub release
for it. Each of the 29 tags up to `v1.17.2` had none. The owner noticed
this for `v1.17.2`, and on 2026-10-21 that release was created by hand
with `gh release create v1.17.2 --verify-tag --latest`, with notes taken
from `CHANGELOG.md`.

The owner asked, the same day, to "publish a release on tag, like
magic-cli-remote does".

**Facts [observed 2026-10-21]:**

* **magic-cli-remote.** Its `ci.yml` runs on `push` to `master` and on
  `v*` tags. On a tag, a `build` job that `needs` its test jobs calls
  this repository's `build-selfupdate-release.yml`, and a `publish` job
  calls `publish-selfupdate-release.yml`, both pinned by SHA. Its
  concurrency group cancels an older run except on a tag
  (`cancel-in-progress: ${{ github.ref_type != 'tag' }}`), because a
  cancelled publish could leave a partial release. Its repository has
  immutable releases on: `v0.22.2` is the latest release.
* **The publish workflow does not fit a library.** It takes a staged
  artifact and requires a non-empty product list and platform list, and
  `verify-selfupdate-release.sh` checks a `SHA256SUMS` over binaries. This
  module ships no binary (AGENTS.md: "It is a library only").
* **The guards it uses are scripts here** and each has a test:
  * `scripts/check-release-tag.sh TAG '[]'` admits `vX.Y.Z` only;
  * `scripts/refuse-existing-release.sh TAG` refuses a tag that already
    has a release, a draft included, or whose state it cannot read;
  * `scripts/release-latest-flag.sh TAG` prints `--latest=false` for a
    stable tag below the current latest, a backport, and nothing
    otherwise.
* **This repository's `ci.yml`.** It has `permissions: contents: read`
  at the top. Its concurrency group `ci-${{ github.ref }}` cancels in
  progress on every ref, a tag included. Its jobs are `validate` (three
  OSes), `go126`, `installer-containers`, `release-rehearsal`,
  `release-rehearsal-archive` and `release-rehearsal-check`. The
  "Verify the workflow contract" step runs `check-workflows.sh`'s
  expression, permission and pin rules on `ci.yml`, not its `gh-repo`
  rule.
* **The changelog** has one `## vX.Y.Z - YYYY-MM-DD` section per release,
  newest first, and links records and guides relative to the repository
  root. A relative link is broken on a release page.
* **Pull requests.** The owner commits straight to `main`. GitHub's
  `--generate-notes` builds its list from merged pull requests, so here
  it would give only a "Full Changelog" compare link.
* **Immutable releases** are off for this repository
  (`GET repos/maccavelli/go-selfupdate-lib/immutable-releases`:
  `{"enabled":false}`). The hand-made `v1.17.2` release has
  `isImmutable: false`. The setting applies only to releases published
  after it is on. With it on, GitHub locks the release's tag and assets,
  and `gh release verify TAG` checks the release attestation GitHub
  makes.

## Decision Drivers

* No release before the tag's CI has passed in full: a release says the
  tag is good.
* One source for the notes, which the release procedure already writes:
  the changelog.
* The guards the publish workflow uses, not copies of them.
* No new third-party action and no new module.
* A job added to `ci.yml` later cannot quietly run after the release, or
  not gate it.
* The publish workflow, which callers pin, is left unchanged.

## Considered Options

* **A.** A `release` job in `ci.yml`, run only on a tag, that `needs`
  every other job, and creates the release with `gh` and this
  repository's scripts.
* **B.** A separate workflow on `push: tags: ['v*']` that creates the
  release.
* **C.** A separate workflow on `workflow_run` of CI's completion.
* **D.** Call `publish-selfupdate-release.yml` from `ci.yml`, as
  magic-cli-remote does.

For the notes:

* **N1.** The tag's `CHANGELOG.md` section, its relative links made
  absolute at the tag; no section, no release.
* **N2.** `gh release create --generate-notes`.

For immutability:

* **I1.** Turn immutable releases on for the repository, and have the
  job wait until the release is immutable and `gh release verify`
  passes, as the publish workflow does.
* **I2.** Leave it off.

## Decision Outcome

**Rejected** by the owner, 2026-10-21 ("delete the release and flag the
pair rejected"). See Amendment A1. This module publishes no GitHub
releases; the text below is the proposal as it was made.

Proposed: "A", with "N1" and "I1". A is the magic-cli-remote shape
without the binaries, which is all of it this module can use. N1 reuses
the notes the release procedure already writes, and I1 gives this
module's releases the same guarantee magic-cli-remote's have. I1 is the
owner's call, because it changes a repository setting.

In detail:

* **D1 (where).** `ci.yml` gains a `release` job with
  `if: github.ref_type == 'tag'`, `needs:` every other job, and a
  job-level `permissions: contents: write`; the top level stays
  `contents: read`. It runs on `ubuntu-24.04`.
* **D2 (no cancelled tag run).** The concurrency group's
  `cancel-in-progress` becomes `${{ github.ref_type != 'tag' }}`, as in
  magic-cli-remote.
* **D3 (guards, in order, before any write).** The job runs
  `check-release-tag.sh "$TAG" '[]'`, then
  `refuse-existing-release.sh "$TAG"`, then builds the notes. Each step
  reads the tag through `env:`, and every `gh` step sets `GH_REPO`.
* **D4 (notes).** A new `scripts/release-notes.sh TAG [CHANGELOG]`
  prints the body of the `## TAG - …` section, with every relative link
  rewritten to `https://github.com/<repo>/blob/TAG/<path>`. The
  repository comes from `GITHUB_REPOSITORY`, with no default. It exits 1
  when the section is missing or empty. It has a `_test.sh`, which the
  gate's script tests run.
* **D5 (create).** `gh release create "$TAG" --verify-tag --title "$TAG"
  --notes-file …`, plus `release-latest-flag.sh`'s flag when it prints
  one. There are no assets, so there is no draft step.
* **D6 (immutability, with I1).** After creating the release, the job
  waits up to 120 s for `isImmutable: true` and a passing
  `gh release verify`, and fails with the setting's location when it
  does not get them. That is the publish workflow's last step, restated
  for one release with no assets.
* **D7 (checks).** "Verify the workflow contract" adds the `gh-repo`
  rule on `ci.yml`. A shape test asserts that the `release` job runs only
  on a tag, needs every other job of `ci.yml`, and is the only job with
  write permissions.

### Consequences

* Good, because every tag that passes CI gets a release, with the notes
  its changelog section holds, and no tag that fails CI gets one.
* Good, because the shape test fails a new job that the release does not
  wait for.
* Good, because the tag, notes and latest rules are the scripts already
  tested for callers.
* Neutral, because the release appears when the tag's full CI ends,
  about 45 minutes after the tag push.
* Neutral, because the tag's annotated message and the changelog stay
  separate; the release takes the changelog's text.
* Bad, because the immutability wait restates the publish workflow's
  last step; a change to one must be made in the other. The step is
  short, and the publish workflow stays unchanged for its callers.
* Bad, because the job is proven only by a real tag. A failure leaves
  the tag without a release, and the fix is the hand command used for
  `v1.17.2`.
* Bad, with I1, because a release of this module can no longer be
  deleted or have its tag moved. That matches the module proxy, which
  never forgets a version anyway.

### Confirmation

* `release-notes_test.sh` passes, and fails on a planted break: a wrong
  section, a link left relative.
* The shape test passes on `ci.yml`, and fails on copies with a job
  missing from `needs`, the tag guard removed, or `contents: write` at
  the top.
* `actionlint` and `check-workflows.sh`'s four rules pass on `ci.yml`.
* The next release's tag run creates its release; that release's PLAN
  records the run ID, the release URL, `isImmutable` and
  `gh release verify`.

## Pros and Cons of the Options

### A. A `release` job in `ci.yml`

* Good, because `needs:` waits for the tag's own CI, in the same run.
* Good, because the tag guard, permissions and concurrency sit in one
  file with the jobs they wait for.
* Bad, because `ci.yml` grows one job.

### B. A separate tag workflow

* Good, because `ci.yml` is untouched.
* Bad, because it does not wait for CI. A failing tag would still be
  released.

### C. `workflow_run` on CI

* Good, because it can wait for CI's conclusion.
* Bad, because a `workflow_run` workflow runs from the default branch's
  copy, not the tag's, and must find the tag from the triggering run;
  it is harder to read and to test.

### D. `publish-selfupdate-release.yml`

* Good, because it is the workflow magic-cli-remote uses.
* Bad, because it needs a staged binary set this module does not have,
  and making its inputs optional changes a workflow callers pin.

### N1. Notes from the changelog

* Good, because the release procedure already writes that section, and
  a tag without one fails before anything is published.
* Bad, because a new script and its test are needed to cut the section
  and fix its links.

### N2. `--generate-notes`

* Good, because it needs no code.
* Bad, because with no pull requests it lists nothing, only a compare
  link.

### I1. Immutable releases on

* Good, because a published release, its tag and its notes' source are
  fixed, as magic-cli-remote's are, and `gh release verify` proves it.
* Bad, because a mistaken release cannot be deleted; the fix is a new
  patch release.

### I2. Immutable releases off

* Good, because a mistake can be deleted and re-run.
* Bad, because a release's tag can be moved after it is published.

## More Information

* The PLAN:
  [0030-PLAN-publish-github-release-on-tag.md](0030-PLAN-publish-github-release-on-tag.md).
* The release procedure being changed is `docs/architecture.md`'s
  "Releasing this module".
* The tag rule:
  [0005-MADR](0005-MADR-opt-in-prerelease-channels.md). The latest flag:
  [0010-MADR](0010-MADR-remediate-second-debugging-pass-findings.md) D9.
  Tags reach scripts through `env:`:
  [0003-MADR](0003-MADR-remediate-debugging-pass-findings.md) D8.
  `GH_REPO` on every `gh` step: 0003-MADR D4.
* Tags are not path-filtered, so every tag runs `ci.yml` in full:
  [0028-MADR](0028-MADR-skip-full-ci-for-docs-only-pushes.md).

## Amendment A1 (2026-10-21): rejected; a tag is this module's release

Before approving the pair, the owner asked whether a library should have
GitHub releases at all, and rejected the proposal ("delete the release
and flag the pair rejected").

**The rule:** this module's release is its annotated `vX.Y.Z` tag. The
Go module proxy and pkg.go.dev serve a tag with no release object, and
`go get` resolves it. The release notes are the tag's `CHANGELOG.md`
section and the migration guide's "From vX to vY" section. The release
procedure in `docs/architecture.md`'s "Releasing this module" is
unchanged. It never had a GitHub release step.

**Why:**

* A library ships no asset, so a release object would carry only notes.
  The changelog and the migration guide already hold them.
* The `release` job would be a moving part in `ci.yml` that only a real
  tag can prove.
* Every tag up to `v1.17.2` had no release, so no consumer reads one.

**What is given up**, and accepted:

* Dependabot and Renovate pull requests in consumer repositories list
  commits, not release notes.
* Watching the repository's releases notifies no one; a consumer
  watches tags or the changelog instead.

**The hand-made `v1.17.2` release.** It was deleted on 2026-10-21 with
`gh release delete v1.17.2 --yes`, which keeps the tag. Kept alone, it
would have stayed "Latest" on the repository page after later tags.
After the deletion:

* `gh api repos/maccavelli/go-selfupdate-lib/releases --jq length` gives
  `0`;
* `git ls-remote --tags origin v1.17.2` gives the same tag object,
  `3f52f96a3e26`;
* the proxy's `@latest` is `v1.17.2`, commit `036289ecc701`.

**Revisit** when a consumer needs a release object. One case is a
release asset for this module, which would make it more than a library.
A new record would then take up the options above.
