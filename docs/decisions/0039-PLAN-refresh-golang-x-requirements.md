---
status: in-progress
date: 2026-10-30
associated-madr: "0039-MADR-refresh-golang-x-requirements.md"
---
<!-- markdownlint-disable MD013 -->

# Implement the move of `golang.org/x/sys`, `x/term` and `x/mod` to their current releases, and release `v1.23.1`

Associated MADR: [0039-MADR-refresh-golang-x-requirements.md](0039-MADR-refresh-golang-x-requirements.md)
(option A, as the owner chose on 2026-10-30).

## Goal

* **`go.mod` requires `golang.org/x/sys v0.49.0`, `golang.org/x/term v0.47.0`
  and `golang.org/x/mod v0.42.0`,** with no other change to the module
  graph, the `go` line or the `toolchain` line, and no source change.
* **Every gate and every CI job passes on them,** the live tests and the
  `go126` floor job included.
* **`v1.23.1` is tagged from `main`,** its tag's CI passes every job, and
  the proxy serves it as `@latest`.

## Scope

### In scope

| Phase | What | Commit |
| :--- | :--- | :--- |
| R0 | the records: this pair and their index rows | records only |
| R1 | `go.mod`, `go.sum`, `docs/architecture.md:17-18`, `CHANGELOG.md`'s `v1.23.1` entry, the migration guide's "From v1.23.0 to v1.23.1" | one commit; its push runs full CI |
| R2 | `v1.23.1`: the tag, its CI, the pin commit, the proxy | the owner's tag, then a pin commit |
| R3 | closing | records only |

### Out of scope

* **Any `.go` file.** No identifier this module uses changed (MADR,
  Context), and no new `x/` API is adopted.
* **The Go toolchain and floor,** `go1.27.3` and `go 1.26.10`.
* **Tool versions:** `golangci-lint`, `govulncheck`, `markdownlint-cli2`,
  `apidiff`, `shellcheck`, `actionlint` and the Actions in the workflows.
* **Other repositories.** magic-cli-remote and gobble-cli move their own
  pins, by their own records.
* **An automated updater** (MADR option E).
* **Pushes and the tag,** which happen only on the owner's word, in the
  same turn.

## Rules

* Each phase ends in one commit, `git commit --no-edit`, made on the
  owner's word.
* Every edit to a document is a Python script under the scratchpad, which
  asserts that each old string occurs once before replacing it.
* `go.mod` and `go.sum` change only through `go get` and `go mod tidy`,
  never by hand. No `go list -m all` or other command runs with
  `-mod=mod` against the tree: in the MADR's probe that added a stray
  `x/tools` line to `go.sum`.
* Gate and tool output goes to the scratchpad, with its exit status
  captured before any filter.
* Before each push the owner is asked for, the disclosure guard runs over
  the outgoing commits.
* Nothing staged names a host, an account or a real-machine path.
* **Deviations.** A phase that finds anything outside its files, or a
  check that fails, stops. It is recorded here, dated, and in the MADR as
  an amendment when a fact or decision changes.

## Implementation Steps

### Phase R0: records

1. This PLAN and its MADR, `proposed`, and their rows in
   `docs/README.md` after 0038's.
2. `make markdownlint`, `make check-records`, `make docs-check` and
   `scripts/check-docs.sh --ids` on the three files exit 0.
3. On the owner's approval, both become `accepted` and `in-progress`, and
   the owner commits the records.

### Phase R1: the move

Files: `go.mod`, `go.sum`, `docs/architecture.md`, `CHANGELOG.md`,
`docs/guides/migrating-from-mcplib-selfupdate.md`, and this PLAN's record.

1. **Baseline.** `git status --short` is clean; `go list -m all` under
   `GOFLAGS=-mod=readonly` lists the three pinned versions and
   `x/tools v0.49.0`.
2. **The move.**
   `go get golang.org/x/sys@v0.49.0 golang.org/x/term@v0.47.0 golang.org/x/mod@v0.42.0`,
   then `go mod tidy`.
   * `git diff --stat -- go.mod go.sum`: two files, 9 insertions and
     9 deletions;
   * `git diff go.mod` shows only the three `require` lines;
   * `go mod tidy -diff` exits 0;
   * `go list -m all` under `-mod=readonly` lists `x/mod v0.42.0`,
     `x/sys v0.49.0`, `x/term v0.47.0` and `x/tools v0.51.0`.
3. **The floor.** Under `GOTOOLCHAIN=go1.26.10`, `go vet ./...` and
   `go test -count=1 ./...` exit 0, as CI's `go126` job runs them.
4. **The documents.**
   * `docs/architecture.md:17-18`: the three new versions, and a citation
     of 0039-MADR.
   * `CHANGELOG.md`: `## v1.23.1 - <the tag's date>`, under Changed: the
     three requirements move; no exported identifier and no behaviour
     changes; `make apicheck` reports `v1.23.1` compatible with
     `v1.23.0`. It links the guide's new section, 0039-MADR and this
     PLAN. R2 moves the date if the tag is cut on another day.
   * The migration guide's §23 gains `### From v1.23.0 to v1.23.1` before
     its `### Check`: no exported API and no behaviour changes, and there
     is nothing to adopt; a program's `go.mod` may gain these versions of
     the three modules when it takes `v1.23.1`; it cites 0039-MADR.
5. **Checks.**
   * `make gate`: every step rc=0, `tidy`, `vuln` and `apicheck`
     ("compatible with v1.23.0") among them; `overall=0`.
   * `make markdownlint`, `make docs-check`, `make check-records`, and
     `scripts/check-docs.sh --ids` on the changed files: exit 0 each.
   * `go test -race -count=1 ./...` with `CGO_ENABLED=1`, on a copy of the
     tree, on the Linux test host and the Windows test host: `ok` for
     every package. The copies are removed after each run.
6. **Seen failing.** This phase adds no test and no gate. The gate's tidy
   step, which guards this change, failed on the stray `x/tools` line in
   the MADR's probe (`go mod tidy -diff` exit 1).
7. The owner commits. The disclosure guard runs before the owner is asked
   to push; the push runs full CI, which must pass every job, the three
   `validate` jobs' live tests and `go126` among them.

### Phase R2: `v1.23.1`

As [0038-PLAN](0038-PLAN-pinned-plans-lifecycle-conformance-and-bubble-tests.md)
Q18, for `v1.23.1`:

1. **Before the tag:** `make apicheck` reports compatibility with
   `v1.23.0`, and `apidiff` between a `git archive` of `v1.23.0` and the
   tree lists no change. `scripts/check-release-tag.sh v1.23.1` exits 0.
   With `main` level with `origin`, the disclosure guard passes.
2. **The tag:** the owner makes the annotated tag `v1.23.1` on R1's
   commit, or on a later commit that changes only records, and pushes it.
3. **The tag's CI** completes `success`, every job; its identity legs
   print `v1.23.1 (release) <12-hex>`.
4. **The pin commit.** `git diff --stat v1.23.0 v1.23.1 -- .github internal/cmd scripts`
   is empty, so the workflows are unchanged and only the pins move. Moved
   to `v1.23.1` and its commit, as Q18 step 5 moved them:
   * `README.md`'s current release, `go get` line and publish pin;
   * `docs/architecture.md`'s current release and commit;
   * `docs/guides/building-releases.md`'s "examples below pin", three
     workflow pins and `git ls-remote` example;
   * in the migration guide, the ~~18~~ 19 `go get` and ~~16~~ 17 `go list -m` lines
     (deviation of 2026-10-30, below),
     "is the current release", §3's workflow pin, "the example pins" and
     its range (`v1.17.2` to `v1.23.1`), `git ls-remote`, §5's "Require
     go-selfupdate-lib", and §2's release list, which gains `v1.23.1` and
     links its section.

   Lines that name `v1.23.0` as history stay. `make markdownlint`,
   `make docs-check` and `--ids` on the changed files exit 0. The owner
   commits and pushes; the push runs Docs.
5. **The proxy:** `go list -m -json github.com/maccavelli/go-selfupdate-lib@v1.23.1`
   gives its commit and `Ref refs/tags/v1.23.1`; `@latest` resolves to
   `v1.23.1`; and the proxy's `go.mod` for `v1.23.1` requires the three
   new versions.

### Phase R3: closing

1. The execution record complete, the Verification below answered with
   output, this PLAN `complete`, the MADR's status as decided, and the
   index rows to match.
2. No copy of the tree left on a test host; `git status --short` clean
   but for the owner's commits.

## Verification

* **V1 (the change):** `go.mod` and `go.sum` differ from `v1.23.0` only in
  the three `require` lines and six sums; `go mod tidy -diff` exits 0; no
  `.go` file changed (`git diff --stat v1.23.0 v1.23.1 -- '*.go'` is
  empty).
* **V2 (the gates):** `make gate` `overall=0`; the documentation checks
  exit 0; the floor's `vet` and `test` pass under `GOTOOLCHAIN=go1.26.10`.
* **V3 (the platforms):** `go test -race ./...` passes on this Mac, the
  Linux test host and the Windows test host; CI on R1's push and on the
  tag passes every job, every live test among them.
* **V4 (the API):** `apidiff` `v1.23.0` → `v1.23.1` lists no change.
* **V5 (the release):** the tag's CI is green, its identity legs name
  `v1.23.1`, and the proxy serves it as `@latest` with the new
  requirements.

## Rollout and Rollback

* **Rollout:** R1 is one commit on `main`, pushed on the owner's word; the
  tag follows a green CI run. A program takes the new requirements when it
  requires `v1.23.1`.
* **Rollback:** before the tag, `git revert` of R1's commit restores the
  old requirements. After it, fix forward in `v1.23.2`. Lowering the
  requirements again would not lower a program's: the two programs read
  above already require `x/sys v0.49.0` and `x/term v0.47.0` themselves.

## Execution record

### The owner's answers (2026-10-30)

Asked four questions, with three options each, the owner chose the
recommended option in each:

* **Approach:** "Move all, release v1.23.1 (Recommended)", MADR option A.
* **Commits:** "One commit (Recommended)": R1 is one commit, as written.
* **Host tests:** "Mac + Linux + Windows (Recommended)": R1 step 5's runs,
  with the live tests from CI's three `validate` jobs.
* **Release docs:** "Changelog + §23 subsection (Recommended)": R1 step 4,
  as written.

The MADR is `accepted`. This plan stays `proposed` until the owner
approves its execution.

### Approval (2026-10-30)

The owner committed the records as `fc43aa3` and said "committed, proceed".
This plan is `in-progress`.

### R1: the move (2026-10-30)

Files: `go.mod`, `go.sum`, `docs/architecture.md`, `CHANGELOG.md`, the migration guide, and this PLAN's record, as listed, with this plan's row in `docs/README.md` (`in-progress`).

1. **Baseline:** `git status --short` clean at `fc43aa3`; `go list -m all` under `-mod=readonly`: `x/mod v0.40.0`, `x/sys v0.47.0`, `x/term v0.43.0`, `x/tools v0.49.0`.
2. **The move:** `go get golang.org/x/sys@v0.49.0 golang.org/x/term@v0.47.0 golang.org/x/mod@v0.42.0` exited 0 ("upgraded" for each of the three), then `go mod tidy` exited 0.
   * `git diff --stat -- go.mod go.sum`: 2 files changed, 9 insertions, 9 deletions;
   * `git diff go.mod`: the three `require` lines only;
   * `go mod tidy -diff`: exit 0;
   * `go list -m all` under `-mod=readonly`: `x/mod v0.42.0`, `x/sys v0.49.0`, `x/term v0.47.0`, `x/tools v0.51.0`.
3. **The floor:** under `GOTOOLCHAIN=go1.26.10` (`go version go1.26.10 darwin/arm64`), `go vet ./...` exit 0 and `go test -count=1 ./...` exit 0, 20 packages `ok`.
4. **The documents:**
   * `docs/architecture.md:17-20`: the three new versions, "their releases of October 2026", citing 0039-MADR.
   * `CHANGELOG.md`: `## v1.23.1 - 2026-10-30`, under Changed, linking the guide's new section, 0039-MADR and this PLAN. R2 moves the date if the tag is cut on another day.
   * The migration guide's §23: `### From v1.23.0 to v1.23.1`, before its `### Check`.
5. **Checks:**
   * `make gate`: every step rc=0; `tidy` clean; `apicheck` "compatible with v1.23.0"; `vuln` "No vulnerabilities found."; `links` 883 in 103 files, 0 broken; `arch` 20 rows, 0 differ; `ids` 7 files, 0 findings; `overall=0`.
   * `make markdownlint`: 0 issues. `make check-records`: 90 records, 0 findings.
   * `go test -race -count=1 ./...` with `CGO_ENABLED=1`, on a copy of the tree: the Linux test host, 20 packages `ok`, exit 0; the Windows test host, 20 packages `ok`, exit 0. Neither host holds a copy afterwards.
6. **Seen failing:** this phase adds no test and no gate (step 6 as written).
7. **The push:** the owner committed `c24f591` and pushed it.
   * CI run 41070697914: `success`, 18 of 18 jobs. Live tests, all PASS: `TestLiveConformance` under systemd (25.82 s, 21.42 s), launchd (54.34 s) and the Service Control Manager (37.81 s); `TestLiveSupervisedS6` (26.13 s); `TestLiveTaskConformance` (4.75 s). `go126` ran `go1.26.10`.
   * Docs run 41070697851: `success`.

### Deviation: R2 step 4's counts (2026-10-30)

* **Found:** R2 step 4 names 18 `go get` and 16 `go list -m` lines in the migration guide. At `c24f591` the guide has 19 `go get github.com/maccavelli/go-selfupdate-lib@v1.23.0` lines and 17 "`go list -m` … gives `v1.23.0`" lines. The counts came from the `v1.23.0` pin commit, `3e39b54`, which moved 18 and 16 because §23, written in the same release, already named `v1.23.0` in its own pair. That pair now names the previous release like every other.
* **Decision:** the owner chose "Move all, record it": all 19 and 17 move to `v1.23.1`, and step 4 is annotated. The MADR asserts no count, so it is unchanged. No file is added to the phase.

### R2: `v1.23.1` (2026-10-30)

1. **Before the tag:** `apidiff` between a `git archive` of `v1.23.0` and the tree at `c24f591` lists no change (exit 0); `make apicheck`, in R1 step 5's gate on the same tree, "compatible with v1.23.0". `scripts/check-release-tag.sh v1.23.1` exit 0. With `main` and `origin/main` both at `c24f591`, the disclosure guard exit 0.
2. **The tag:** the owner made the annotated tag `v1.23.1` on `c24f591`, message "v1.23.1" as the earlier tags, tagged 2026-10-30, and pushed it. The changelog's date stands.
3. **The tag's CI**, run 41094704802: `success`, 18 of 18 jobs. The ten identity legs, five relay binaries and five archives, each print `"v1.23.1 (release) c24f5919cd9e"`. No `--- FAIL` in the log. Live tests: `validate (ubuntu-24.04)` 24 PASS, `TestLiveConformance` twice and `TestLiveSupervisedS6` among them; `validate (macos-15)` 14 PASS and 1 SKIP, `TestLiveSignIdentity`, which needs a signing identity the runner lacks; `validate (windows-2025)` 18 PASS, `TestLiveConformance` and `TestLiveTaskConformance` among them. `go126` ran `go1.26.10`.
4. **The pin commit:** `git diff --stat v1.23.0 v1.23.1 -- .github internal/cmd scripts` printed nothing (exit 0). Moved to `v1.23.1` and `c24f591…`:
   * `README.md`: the current release, the `go get` line and the publish pin;
   * `docs/architecture.md`: the current release and its commit;
   * `docs/guides/building-releases.md`: "examples below pin", the three workflow pins and the `git ls-remote` example;
   * the migration guide: 19 `go get` and 17 `go list -m` lines, "is the current release", §3's workflow pin, "the example pins" and its range, `git ls-remote`, §5's "Require go-selfupdate-lib", and §2's release list, which gains `v1.23.1` and links its section.

   Lines that name `v1.23.0` as history stay: `docs/architecture.md`'s "Since `v1.23.0`", §2's and §23's account of `v1.23.0`, and the heading and text of "From v1.23.0 to v1.23.1".

   Checks: `make markdownlint` 0 issues; `make docs-check` exit 0, 884 links in 103 files, 0 broken, and 20 rows in `docs/architecture.md`, 0 differ; `scripts/check-docs.sh --ids` on the five changed files, 0 findings; `make check-records` 90 records, 0 findings.
5. **The proxy:** `go list -m -json github.com/maccavelli/go-selfupdate-lib@v1.23.1` gives `Hash c24f5919cd9e9e3ba3736e64bcf18a6e2a51489e`, `Ref refs/tags/v1.23.1` and `GoVersion 1.26.10`; `@latest` resolves to `v1.23.1`; the proxy's `go.mod` for `v1.23.1` requires `golang.org/x/mod v0.42.0`, `golang.org/x/sys v0.49.0` and `golang.org/x/term v0.47.0`.
6. **Waiting:** the owner's commit and push of this change, whose Docs run must pass. R3 closes the plan on the owner's word.
