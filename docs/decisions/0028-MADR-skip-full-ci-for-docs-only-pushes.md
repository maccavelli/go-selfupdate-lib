---
status: accepted
date: 2026-10-20
decision-makers: go-selfupdate-lib maintainers
consulted: "GitHub's workflow syntax reference (path filters), read 2026-10-20; the Actions timing API for run 39464144680; this repository's CI at 33aa6f5"
informed: the repository's owner
---
# Skip the full CI for a push or pull request that changes only Markdown, and check it with a one-job docs workflow instead

## Context and Problem Statement

Every push to `main` runs all of `.github/workflows/ci.yml`: 18 jobs,
among them a three-OS `validate` matrix, the `go126` job, two container
runs and two release rehearsals with ten identity legs. A commit that
changes only Markdown gets the same run. That covers a record, a phase
closing or an index row. In the 0027 pass, three such commits went
through the full run (`df67aa1`, `e8d4904` and `33aa6f5`), and its
closing commit would be a fourth.

The owner asked, on 2026-10-20, to stop the full runs for docs-only
pushes, to save Actions minutes.

**Facts [observed 2026-10-20]:**

* **Minutes:** the repository is public. GitHub's timing API reports 0
  billable ms for every job of run `39464144680`: 6 macOS, 20 Ubuntu and
  10 Windows job runs, counting reruns. The owner's ten private
  repositories have run no workflow since 2026-09-20. This repository's CI
  therefore bills no minutes. The owner chose to go ahead for the other
  reasons below ("Yes, write the 0028 pair").
* **What a docs-only run costs anyway:** 15–20 minutes of wall time. It is
  also exposed to macOS runner capacity: attempt 1 of `39464144680` lost
  two macOS jobs to "The job was not acquired by Runner of type hosted
  even after multiple attempts", and needed a rerun.
* **CI also checks the docs.** The `validate` job's Linux leg runs
  markdownlint (`npx --yes markdownlint-cli2@0.23.2`), the link check over
  every tracked `.md` file, and the architecture table check
  (`ci.yml:211-275`). The identifier check does not run in CI, since it
  needs the owner's deny list. Skipping CI outright would skip these too.
* **No code reads a tracked Markdown file.** No `//go:embed` names one, and
  the `.md` names in Go tests are archive members the tests build. The
  script tests write their own fixtures. A Markdown-only change cannot
  break a code test.
* **GitHub's path filters** (workflow syntax reference):
  * "Path filters are not evaluated for pushes of tags." A release tag
    keeps the full run.
  * Branch and path filters must both be satisfied.
  * A push's changed files are a two-dot diff of its base and head; a
    new branch is compared with the parent of its deepest pushed commit's
    ancestor.
  * A push of more than 1,000 commits, or a diff that times out, always
    runs.
  * A workflow skipped by a path filter leaves its checks "Pending", which
    blocks a pull request that requires them. No check is required today:
    the rulesets stay off by the owner's choice
    ([0025-MADR](0025-MADR-align-with-go-sdk-design-practices.md)
    Amendment A5).
* **The release procedure** wants CI green on `main` before the tag
  (docs/architecture.md, "Releasing this module", step 3). Its pre-tag
  commit is docs-only, as `e8d4904` was.

## Decision Drivers

* A docs-only commit keeps its docs checks.
* A commit that changes anything else keeps the full run, and so does
  every tag.
* No third-party action, and nothing that hides a failure: a skipped job
  must not read as a pass.
* The workflow stays simple enough that a later change cannot quietly
  disable CI for code.

## Considered Options

* **A.** `paths-ignore: ['**.md']` on `ci.yml`'s `push` and `pull_request`
  triggers, and nothing else.
* **B.** As A, plus a new `.github/workflows/docs.yml` with `paths:
  ['**.md', '.markdownlint-cli2.jsonc', '.github/workflows/docs.yml']`
  and one Linux job: markdownlint, the link check and the architecture
  table check.
* **C.** One workflow: a first job decides from `git diff --name-only`
  whether code changed, and every heavy job carries an `if:` on its
  output; a docs job always runs.
* **D.** `[skip ci]` in the commit message of a docs-only commit.

## Decision Outcome

Chosen option (the owner, 2026-10-20, "Approve B and proceed"): "B",
because it keeps the docs
checks for a docs-only commit at the cost of one small workflow, uses
GitHub's own filters, and leaves `ci.yml`'s jobs untouched. Tags are
never filtered.

### Consequences

* Good, because a docs-only push runs one Linux job of about a minute,
  with no macOS or Windows runner to wait for.
* Good, because a code push and every tag run as before.
* Neutral, because a push that changes code and Markdown runs both
  workflows, so the docs checks run twice.
* Neutral, because the release procedure's step 3 changes: a docs-only
  pre-tag commit gets the docs workflow on `main`, and the tag's full run
  is the code proof. The code is the same as at the last commit whose
  full run passed, since a docs-only commit changes none.
* Bad, because a required status check, if the rulesets are ever turned
  on, must not name a `ci.yml` job. A docs-only pull request would leave
  it "Pending". The record that turns them on must say so.
* Bad, because a second workflow is one more file for
  `scripts/check-workflows.sh` and actionlint to cover.

### Confirmation

* `actionlint` and `scripts/check-workflows.sh`'s expression, permission
  and pin rules pass on both workflows. CI runs them on `docs.yml` too.
* The docs job's commands fail on a scratch copy with a planted broken
  link and a planted markdownlint error, and pass on the tree.
* After the change lands, a docs-only push to `main` shows the docs
  workflow and no CI run, and a push that changes a non-Markdown file
  shows CI. The PLAN records both run IDs.
* The next release's tag push runs the full CI, recorded in that
  release's PLAN.

## Pros and Cons of the Options

### A. `paths-ignore` only

* Good, because it is one line per trigger.
* Bad, because a docs-only commit then gets no markdownlint, link or
  table check, and a broken link reaches `main` unchecked.

### B. `paths-ignore` and a docs workflow

* Good, because the docs keep their checks, and the full CI keeps its
  jobs unchanged.
* Good, because both filters are GitHub's own, documented behaviour.
* Bad, because there are two workflows to keep in step: a new kind of
  docs file must be named in both.

### C. One workflow, gated jobs

* Good, because one file holds it all.
* Bad, because every heavy job gains an `if:`, and a wrong condition can
  skip CI for code with nothing to show it; a skipped job reads as
  neither pass nor fail.
* Bad, because the change-detection job runs on every push, macOS-free
  but still a runner.

### D. `[skip ci]`

* Bad, because the global `prepare-commit-msg` hook writes the commit
  messages, and a person would have to remember it each time. It also
  skips the docs checks.

## More Information

* The PLAN:
  [0028-PLAN-skip-full-ci-for-docs-only-pushes.md](0028-PLAN-skip-full-ci-for-docs-only-pushes.md).
* The CI being changed is described in `docs/architecture.md`'s CI bullet
  under "Tooling", and the release procedure in its "Releasing this
  module".
