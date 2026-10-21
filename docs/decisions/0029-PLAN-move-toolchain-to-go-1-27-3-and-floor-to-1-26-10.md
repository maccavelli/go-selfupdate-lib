---
status: in-progress
date: 2026-10-21
associated-madr: "0029-MADR-move-toolchain-to-go-1-27-3-and-floor-to-1-26-10.md"
---
# Implement Go 1.27.3 and the `go 1.26.10` floor, and release `v1.17.2`

Associated MADR: [0029-MADR-move-toolchain-to-go-1-27-3-and-floor-to-1-26-10.md](0029-MADR-move-toolchain-to-go-1-27-3-and-floor-to-1-26-10.md)
(option A, as the owner chose).

## Goal

* **Every build this repository runs uses an advisory-free Go:**
  * CI's toolchain jobs, the gates and the release tools use 1.27.3;
  * the `go126` job uses 1.26.10.
* **`go.mod` says `go 1.26.10` and `toolchain go1.27.3`,** and the tree's
  fixtures and current docs say the same.
* **`v1.17.2` is tagged from `main`.** Its tag's CI passes every job,
  which also closes
  [0028-PLAN](0028-PLAN-skip-full-ci-for-docs-only-pushes.md)'s V4.

## Scope

### In scope

| Phase | What | Commit |
| :--- | :--- | :--- |
| W0 | the records: this pair's status, the index, 0018-MADR A2, 0025-MADR A7 | records only |
| W1 | `go.mod`, `ci.yml`'s `go126` job, the ten fixtures | one commit; its push runs CI |
| W2 | the current docs, and the release's pre-tag change | Markdown only; its push runs only Docs |
| W3 | `v1.17.2`: the tag, its CI, the pin commit, the proxy | the tag, then a pin commit |
| W4 | closing | records only |

### Out of scope

* **The hosts.** The WSL distribution's move to 1.27.3, and rebuilding
  this Mac's `golangci-lint` and `govulncheck` with 1.27.3, belong to
  magic-cli-remote's 0169 PLAN, P14. This plan only reads the hosts.
* **`selfupdate/githubupdater.go`'s "Go 1.27.2's" comment,** which stays
  (MADR outcome 5).
* **Other repositories,** and the callers' workflow pins.
* **Pushes and the tag,** which happen only on the owner's word, in the
  same turn.

## Rules

* Each phase ends in one commit, `git commit --no-edit`, made on the
  owner's word.
* Every edit is a Python script under the scratchpad. It asserts that
  each old string occurs once before replacing it, and reads the file
  back afterwards.
* A `.go` file passes `make pre-add-check FILES=…` before it is staged.
* Gate and tool output goes to the scratchpad, with its exit status
  captured before any filter.
* Before each push the owner is asked for, the disclosure guard runs
  over the outgoing commits.
* Nothing staged names a host, an account or a real-machine path.

## Implementation Steps

### Phase W0: records

**Files:**
* this pair;
* `docs/README.md`;
* `docs/decisions/0018-MADR-move-toolchain-to-go-1-27-2.md`;
* `docs/decisions/0025-MADR-align-with-go-sdk-design-practices.md`.

1. The MADR becomes `accepted`, with the owner's words. This plan
   becomes `in-progress`, with an Approval entry, and the two index rows
   follow.
2. **0018-MADR**, after A1: `### A2 (2026-10-21): the toolchain line is
   go1.27.3`. It names GO-2026-6809, links 0029-MADR, and keeps the rest
   of the record as written.
3. **0025-MADR**, after A6: `### Amendment A7 (2026-10-21): the floor is
   go 1.26.10`. It says 1.26.9 now has GO-2026-6809 and 1.26.10 has
   none, gives the `go126` job's new version, links 0029-MADR, and keeps
   D8's reason, that the library needs nothing from 1.27.
4. **Checks:** links, markdownlint and ids on the changed files.

### Phase W1: the toolchain, the floor, CI and the fixtures

**Files:**
* `go.mod`
* `.github/workflows/ci.yml`
* `buildinfo/stamp_test.go:41`
* `internal/cmd/selfupdate-release/installer_ps_test.go:574`
* `internal/cmd/selfupdate-release/plan_test.go:214`
* `internal/cmd/selfupdate-release/testdata/fixture/go.mod:3`
* `internal/testproc/testproc_test.go:205`
* `scripts/go-fuzz_test.sh:30`
* `selfupdate/archive/main_test.go:79`
* `selfupdate/codesign/live_darwin_test.go:49`
* `selfupdate/e2e_running_test.go:71`
* `selfupdate/imageverify_test.go:84`

1. **`go.mod`:** `go 1.26.9` becomes `go 1.26.10`, and
   `toolchain go1.27.2` becomes `toolchain go1.27.3`.
2. **`ci.yml`, the `go126` job:**
   * the comment becomes "The module's floor, go 1.26.10;
     GOTOOLCHAIN=local stops the toolchain line switching to 1.27.3",
     and so on, citing 0025-MADR D8 and 0029-MADR;
   * `go-version: '1.26.10'`;
   * `test "$(go env GOVERSION)" = go1.26.10`.
3. **The fixtures:** each file's `go 1.26.9` becomes `go 1.26.10`.
   * Afterwards, `git grep -n '1\.26\.9' -- ':!docs' ':!CHANGELOG.md'
     ':!*.md'` lists nothing.
   * `stage_test.go`'s `"go1.26.6"` and `buildinfo_test.go`'s version
     strings are test data, and stay.
4. **Red:**
   * the MADR's govulncheck table: exit 3 on 1.27.2 and 1.26.9;
   * the `go126` job's check, run verbatim under
     `GOTOOLCHAIN=go1.26.9`: `test "$(go env GOVERSION)" = go1.26.10`
     exits 1. Under `GOTOOLCHAIN=go1.26.10` it exits 0.
5. **Checks,** on this Mac:
   * `go mod tidy -diff` is clean;
   * `make pre-add-check FILES="<the eight .go files>"` passes;
   * `make gate` with `SELFUPDATE_REQUIRE_PYTHON=1` ends `overall=0`,
     with `vuln rc=0`;
   * the `go126` job's steps, under `GOTOOLCHAIN=go1.26.10`: `go build
     ./...`, `go vet ./...` on three targets, `go test -count=1 ./...`,
     and `govulncheck ./...`, each exit 0;
   * actionlint and `scripts/check-workflows.sh` pass on `ci.yml`, as
     its contract step runs them.
6. **Commit.** On the owner's word, push. CI on that commit must pass
   every job. Its setup-go logs name 1.27.3, and the `go126` job's
   names 1.26.10.

### Phase W2: the docs, and the pre-tag change

**Files:**
* `AGENTS.md`
* `README.md`
* `docs/architecture.md`
* `SECURITY.md`
* `docs/guides/migrating-from-mcplib-selfupdate.md`
* `CHANGELOG.md`

1. **The current statements of the floor and the toolchain** say
   1.26.10 and 1.27.3, and cite 0029-MADR:
   * `AGENTS.md:39-45`: "Requires Go 1.26.10; built and tested with Go
     1.27.3", and the `go126` job;
   * `README.md:26-29`: the `v1.17.2` floor, the toolchain, and the date
     on which neither line had an advisory;
   * `docs/architecture.md:17-24` (the module and its toolchain line)
     and `:831-833` (the `go126` job);
   * `SECURITY.md:40-42`: the two `govulncheck` runs;
   * the guide's §1 (`:14`, `:16`), and its release-workflow paragraph
     (`:133`): from `v1.17.2` the release tools build with 1.27.3.

   Before §1's heading is renamed, `git grep` checks that no link
   targets `#1-go-1269-or-later`. If one does, it is changed in the same
   step.
2. **Historical lines stay:** the changelog's earlier releases, the
   guide's §16 and its earlier sections, and the records.
3. **The pre-tag change** (the architecture page's "Releasing this
   module", step 1):
   * `CHANGELOG.md`: `## Unreleased` becomes `## v1.17.2 - <date>`. It
     gains a "Security" entry (GO-2026-6809; the toolchain and the
     floor; what a program on 1.26.9 does), keeps 0028's "Changed"
     entry, and links the guide section and both pairs;
   * the guide, under §17 and before its `### Check`: `### From v1.17.1
     to v1.17.2`. It says that no exported API changed, describes the
     floor, the toolchain and the advisory, and that a program builds
     with 1.27.3 or 1.26.10 to take the fix. Under "Adopting it" it says
     to move the workflow pins to `v1.17.2`, so that the release tools
     build with 1.27.3.
4. **Checks:**
   * links, ids, markdownlint and the architecture table check on the
     changed files;
   * `make gate` ends `overall=0`;
   * `make apicheck` reports `v1.17.2`'s tree compatible with `v1.17.1`.
5. **Commit.** On the owner's word, push. This commit changes only
   Markdown, so only "Docs" runs. That is release step 3 under
   0028-MADR, and W1's CI run is the proof of the code.

### Phase W3: `v1.17.2`

Steps 4 to 7 of the architecture page's "Releasing this module", with
`vX = v1.17.2` and `<commit>` W2's commit:

1. `scripts/check-release-tag.sh` and the disclosure guard over the
   outgoing tag. Then, on the owner's word, the annotated tag and its
   push.
2. **The tag's CI run:** every job passes, and the identity legs print
   `v1.17.2 (release) <12-hex>`. The run is also 0028-PLAN's V4: a tag
   push runs CI in full, and does not run Docs.
3. **The pin commit:** `git grep -n 'v1\.17\.1'` lists the current-release
   pointers. Each one that names the current release moves to `v1.17.2`
   and its commit, and each change is named in the record. Lines that
   name `v1.17.1` as history stay.
4. **The proxy:** `go list -m -json github.com/maccavelli/go-selfupdate-lib@v1.17.2`
   gives W2's commit, and `@latest` resolves to it. The proxy's
   `go.mod` for `v1.17.2` says `go 1.26.10` and `toolchain go1.27.3`.

### Phase W4: closing

1. The verification below, with its output.
2. This plan becomes `complete`, and its index row follows. The MADR
   stays `accepted`.
3. The open items are named: the WSL distribution and this Mac's two
   tools, which wait on 0169 P14, and the callers' pins.

## Verification

* **V1 (red):** govulncheck exits 3 on 1.27.2 and 1.26.9, as recorded
  in the MADR. The `go126` `GOVERSION` check exits 1 under 1.26.9.
* **V2 (local):** `make gate` ends `overall=0` with `vuln rc=0` at W1
  and W2. The `go126` steps pass under 1.26.10.
* **V3 (CI):** W1's push passes every job, on 1.27.3, with the `go126`
  job on 1.26.10. W2's push runs only "Docs", which passes.
* **V4 (the tag):** the `v1.17.2` tag's CI passes every job, with
  identity legs `v1.17.2 (release) <12-hex>`. This is also 0028-PLAN's
  V4.
* **V5 (the proxy):** `@latest` is `v1.17.2`, at W2's commit, with
  `go 1.26.10` and `toolchain go1.27.3`.
* **V6 (the docs):**
  * `git grep` for `1.26.9` and `1.27.2` outside the records, the
    changelog's earlier releases and the guide's earlier sections
    finds only `githubupdater.go`'s comment, which outcome 5 keeps;
  * links, ids and markdownlint are clean.

## Rollout and Rollback

* **Before the tag,** W1 and W2 are reverted by new commits. No history
  is rewritten.
* **After the tag,** releases are immutable. A fault is fixed in a new
  patch release, and `v1.17.2` would be retracted only by a record that
  says so.
* **A consumer is affected** only when it takes `v1.17.2`, and then only
  if it builds with Go 1.26.9 exactly and `GOTOOLCHAIN=local`. The `go`
  command otherwise selects 1.26.10 or later.

## Execution Record

### Approval (2026-10-21)

The owner approved option A and execution: "option a, approved to
proceed". The MADR became `accepted`, and this plan `in-progress`.

### W0 (2026-10-21)

1. **This pair:** the MADR is `accepted`, with the owner's words, and
   this plan `in-progress`. The index rows read `accepted` and
   `in-progress`.
2. **0018-MADR** gains `### A2 (2026-10-21): the toolchain line is
   go1.27.3`, after A1.
3. **0025-MADR** gains Amendment A7 (2026-10-21), "the floor is
   `go 1.26.10`", after A6. It keeps D8's reason for a 1.26 floor.
4. **Checks,** on the five changed files: markdownlint "0 issues in 0
   files"; links "139 links in 5 files, 0 broken"; ids "5 files, 16
   deny-list rules, 0 findings". Each exit 0.

### W1 (2026-10-21)

1. **`go.mod`:** `go 1.26.10` and `toolchain go1.27.3`. `go mod tidy
   -diff` is clean.
2. **`ci.yml`, the `go126` job:** `go-version: '1.26.10'`, the check
   `test "$(go env GOVERSION)" = go1.26.10`, and the comment, which now
   names 1.26.10 and 1.27.3 and cites 0025-MADR's Amendments A1 and A7
   and 0029-MADR.
3. **The ten fixtures:** each one's `go 1.26.9` occurred once, and is now
   `go 1.26.10`. `git grep -n '1\.26\.9\b' -- ':!*.md'` lists nothing
   (exit 1). Outside Markdown, `1.27.2` remains only in
   `githubupdater.go:102`, which outcome 5 keeps.
4. **Red:** the MADR's govulncheck table (exit 3 on 1.27.2 and 1.26.9).
   The `go126` check ran verbatim:

   | `GOTOOLCHAIN` | `go env GOVERSION` | exit |
   | :--- | :--- | :--- |
   | `go1.26.9` | `go1.26.9` | **1** |
   | `go1.26.10` | `go1.26.10` | 0 |

5. **Checks,** on this Mac, with `go version go1.27.3 darwin/arm64`:
   * the `go126` job's steps under `GOTOOLCHAIN=go1.26.10`: the version
     check, `go build ./...`, `go vet ./...` and its windows and darwin
     runs, `go test -count=1 ./...` (18 packages `ok`), and
     `govulncheck ./...` ("No vulnerabilities found."). Each exit 0;
   * `make pre-add-check` on the eight `.go` files: "8 file(s) clean
     (gofmt, golangci-lint, go vet, go test, govulncheck)";
   * actionlint on `ci.yml` and `docs.yml`; `check-workflows.sh` with no
     arguments and `--rule pins` on `ci.yml`; `check-workflows_test.sh`
     and `workflow-shape_test.sh`. Each exit 0;
   * `make gate` with `SELFUPDATE_REQUIRE_PYTHON=1`: every step `rc=0`,
     with `apicheck` "compatible with v1.17.1", `vuln` "No
     vulnerabilities found.", links "699 links in 81 files, 0 broken",
     and `overall=0`.
