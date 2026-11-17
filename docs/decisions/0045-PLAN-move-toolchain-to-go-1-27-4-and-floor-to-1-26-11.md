---
status: in-progress
date: 2026-11-17
associated-madr: "0045-MADR-move-toolchain-to-go-1-27-4-and-floor-to-1-26-11.md"
---
# Implement Go 1.27.4 as the toolchain and `go 1.26.11` as the floor, and prepare `v1.26.3`

Associated MADR: [0045-MADR-move-toolchain-to-go-1-27-4-and-floor-to-1-26-11.md](0045-MADR-move-toolchain-to-go-1-27-4-and-floor-to-1-26-11.md)
(option B, as the owner ruled).

## Goal

* **`go.mod` says `go 1.26.11` and `toolchain go1.27.4`,** and every
  current statement in the tree and every test fixture agrees.
* **Every build this repository runs uses a toolchain `govulncheck`
  reports clean:** CI's `go.mod`-driven jobs on 1.27.4, the `go126` job
  on 1.26.11.
* **`v1.26.3` is ready to tag from `main`.** The plan stops before the
  tag; the tag and the release each need the owner's word.

## Scope

### In scope

| Phase | What | Price (model class, effort) | Commit |
| :--- | :--- | :--- | :--- |
| P0 | host re-read, the red re-run, 0018-MADR A3, 0029-MADR A1, 0025-MADR A8 | small, low (amendments: small, medium) | records only |
| P1 | `go.mod`, `ci.yml`, ten fixtures, the current docs and the changelog entry | mainstream, medium | one commit; its push runs CI in full |
| P2 | the version-agreement check, and its two planted failures | small, low | none (scratchpad) |
| P3 | gates, and the green re-run of `govulncheck` | small, low (runs only; heavy) | none |
| P4 | CI on `main` | small, low | none |
| P5 | release `v1.26.3`: pre-tag change, tag, tag CI, pin commit, proxy | mainstream, medium | pre-tag commit, the tag, a pin commit |
| P6 | closing | small, low | records only |

Prices read from the bottom: the cheapest class that can do the step with
its checks. P1 and P5 write prose other files cite, so they sit one class
up.

### Out of scope

* **Other repositories** and their workflow pins. `gobble-cli` and
  `magic-cli-remote` (both `go 1.27.4`, on `v1.26.2`) and
  `prepare-commit-msg` (`go 1.27.3`, on `v1.26.0`) move their own pins in
  their own records (MADR E8).
* **`selfupdate/githubupdater.go:104`'s "Go 1.27.3's" comment,** which
  stays: the `DefaultTransport` block is identical in 1.27.3 and 1.27.4
  (MADR E7).
* **The govulncheck pin** (`v1.8.0` in `Makefile:57`, `ci.yml:222`,
  `:352`, `scripts/go-precheck.sh:92`) against v1.9.0 on this Mac.
* **Installing Go anywhere.** Go 1.27.4 is on every host; Go 1.26.11 is on
  none as an SDK, and this plan installs nothing (see P3).
* **History:** the changelog's earlier releases, the guide's earlier
  sections, the records, and test data that names a version as data
  (`internal/cmd/selfupdate-release/stage_test.go:305`, `:315`, `:340`;
  `buildinfo/buildinfo_test.go:62`, `:76`).
* **The tag and the release,** until the owner's word (P5). Every push
  before them is under the owner's ruling D3 in the MADR, for this plan
  only.

## Rules

* Each phase that mutates ends in one commit, `git commit --no-edit`,
  after its checks pass; the commit hook writes the message. Under the
  owner's ruling D3 each commit is pushed to `main`, after the disclosure
  guard runs over the outgoing commits. The authority covers this plan
  and no other.
* Every edit is a Python script under the scratchpad, run by path (no
  heredoc, no long inline script). It asserts that each old string occurs
  the expected number of times before replacing it, and the result is
  read back.
* A `.go` file passes `make pre-add-check FILES="..."` before it is
  staged.
* Gate and tool output goes to the scratchpad, with its exit status
  captured before any filter.
* **Heavy runs** (`make gate`, `-race`, fuzz, `go test ./...`, `make
  pre-add-check` over the module) go through
  `~/.agents/skills/available-dev-hosts/scripts/run-heavy -- <command>`
  when they stay on this Mac, or
  `~/.agents/skills/available-dev-hosts/scripts/run-on-host --need <os>
  [--race] -- <command>` for another host, one at a time per host, after
  `pick-host`. Every `-race` run is passed `--race` (so it never lands in
  a WSL distribution) and carries an explicit `-timeout`, because
  `make gate`'s race step takes go's default per package: the plan runs
  `go test -race -count=1 -timeout 25m ./...` on its own beside the gate.
  Darwin-only tests (the codesign live tests) stay on this Mac.
* Nothing staged names a host, an account or a real-machine path. Hosts
  are named by role.

## Implementation Steps

### Phase P0: preflight and the amendments

**Files:**

* `docs/decisions/0018-MADR-move-toolchain-to-go-1-27-2.md`
* `docs/decisions/0029-MADR-move-toolchain-to-go-1-27-3-and-floor-to-1-26-10.md`
* `docs/decisions/0025-MADR-align-with-go-sdk-design-practices.md`
* `docs/README.md` (only if a row's status changes; the pair's rows read
  `accepted` and `in-progress`)

1. **Re-read the hosts, read-only,** by role, in the execution record:
   `go version`, `go env GOTOOLCHAIN` and the SDK directory of this Mac,
   the Linux machine and the Windows laptop. **If any host reports less
   than 1.27.4, stop**; an install is a new step with its own approval.
   Also record, by role, whether Go 1.26.11 is in each module cache
   (2026-11-17: this Mac yes; the Linux machine and the Windows laptop no).
2. **Re-run the red** from the module cache with `GOPROXY=off`:
   `GOTOOLCHAIN=go1.27.3 govulncheck ./...` and
   `GOTOOLCHAIN=go1.26.10 govulncheck ./...` exit 3 with GO-2026-6859;
   under `go1.27.4` and `go1.26.11` they exit 0 (measured 2026-11-17,
   MADR E4). Record the database's update time.
3. **0018-MADR** gains `### A3 (<date>): the toolchain line is go1.27.4`,
   after A2. **0029-MADR** gains `## Amendments` and `### A1 (<date>): the
   toolchain line is go1.27.4 and the floor is go 1.26.11`. **0025-MADR**
   gains `### Amendment A8 (<date>): the floor is go 1.26.11`, after A7,
   keeping D8's reason (the library needs nothing from 1.27). Each names
   GO-2026-6859, cites 0045-MADR by full filename, records that it is the
   coordinator's application of the author's recommendation and not the
   owner's ruling (MADR D4), and keeps the rest of its record as written.
4. **Checks** on the changed files: `make markdownlint` (which skips
   record files, so a copy of each under a neutral name is linted with the
   same config), `make check-records`, `make docs-check`,
   `make disclosure-check`; each exit status before any filter.
5. **Commit and push** (records only).

### Phase P1: the toolchain, the floor, CI, the fixtures and the docs

One commit, so the tree never names two floors or two toolchains.

**Files and lines** (numbers at `bab09ae`):

| File | Line | Change |
| :--- | :--- | :--- |
| `go.mod` | 3 | `go 1.26.10` becomes `go 1.26.11` |
| `go.mod` | 5 | `toolchain go1.27.3` becomes `toolchain go1.27.4` |
| `.github/workflows/ci.yml` | 322-326 | comment: floor "go 1.26.11", "switching to 1.27.4", cites 0045-MADR |
| `.github/workflows/ci.yml` | 337 | `go-version: '1.26.11'` |
| `.github/workflows/ci.yml` | 339 | `test "$(go env GOVERSION)" = go1.26.11` |
| `AGENTS.md` | 57-62 | "Requires Go 1.26.11; built and tested with Go 1.27.4"; the `go126` job on 1.26.11; the citations gain 0045-MADR by filename |
| `README.md` | 26-31 | Status: from `v1.26.3` the floor is 1.26.11 (`v1.17.2` to `v1.26.2` 1.26.10, `v1.16.0` to `v1.17.1` 1.26.9, `v1.15.2` and earlier 1.27.1); built and tested with Go 1.27.4; CI also tests 1.26.11; the dated no-advisory sentence names 2026-11-17 and 0045-MADR |
| `SECURITY.md` | 41-43 | the two `govulncheck` runs: "Go 1.27.4 toolchain" and "its 1.26.11 floor"; cites 0045-MADR |
| `docs/architecture.md` | 17-26 | the module paragraph: `go 1.26.11`, `toolchain go1.27.4`, what 1.27.4 and 1.26.11 fixed (GO-2026-6859), Amendments A8 and 0045-MADR |
| `docs/architecture.md` | 1016-1018 | the `go126` bullet: 1.26.11 twice |
| `docs/guides/migrating-from-mcplib-selfupdate.md` | 14-24 | section 1: "Go 1.26.11 or later", the floor by release, "Build and test with Go 1.27.4", 1.27.3 and 1.26.10 named as carrying GO-2026-6859; the heading's anchor is `git grep`-checked for links before it is renamed |
| `CHANGELOG.md` | before 11 | a new `## Unreleased` with a "Security" entry (the toolchain, the floor, GO-2026-6859, that a program on exactly 1.26.10 moves to 1.26.11, and what a caller's release tools build with) and links to the guide section and this pair |

**The ten fixtures.** Each is decided from the test that reads it. Two
require this module through `replace`; a module with a lower `go` line
than the module it requires fails under `-mod=readonly` (MADR E10), so
those are live pins and must follow the floor. The other eight are
standalone modules whose test result does not depend on the number; they
change too, so the tree names one floor, as 0025-PLAN S11 and 0029-PLAN
W1 did, and so the version-agreement check needs no allowlist for them.

| Fixture | The test reads it as | Class | Change |
| :--- | :--- | :--- | :--- |
| `internal/cmd/selfupdate-release/testdata/fixture/go.mod:3` | the stand-in caller of the release rehearsal; it `require`s and `replace`s this module (`=> ../../../../..`) | **live pin** | `go 1.26.11`, required |
| `buildinfo/stamp_test.go:41` | `TestStampedBinary` writes a module that `require`s and `replace`s this module, then links it | **live pin** | `go 1.26.11`, required |
| `internal/cmd/selfupdate-release/installer_ps_test.go:574` | `module standin`, no requirement, built by the host's Go | data | `go 1.26.11` (one floor) |
| `internal/cmd/selfupdate-release/plan_test.go:214` | `libModule`: only parsed by `go mod edit -json`; the tests assert requirement versions, not the `go` line | data | `go 1.26.11` (one floor) |
| `internal/testproc/testproc_test.go:209` | `module fixture`, built to test `GoBuild`'s `-buildvcs=false` | data | `go 1.26.11` (one floor) |
| `scripts/go-fuzz_test.sh:30` | a throwaway module for the fuzz script's tests | data | `go 1.26.11` (one floor) |
| `selfupdate/archive/main_test.go:79` | `module fixture`, built for a platform | data | `go 1.26.11` (one floor) |
| `selfupdate/codesign/live_darwin_test.go:49` | `module fixture`, built on macOS for the live codesign test | data | `go 1.26.11` (one floor) |
| `selfupdate/e2e_running_test.go:71` | `module example.com/demo`, the helper's build | data | `go 1.26.11` (one floor) |
| `selfupdate/imageverify_test.go:85` | `module fixture`, built for the image verifier | data | `go 1.26.11` (one floor) |

A data fixture may stay at 1.26.10 without a test failing, and no gate
here shows otherwise; the choice to move them is the tree's one-floor
rule, not a test's need. The plan counts each `go 1.26.10` occurrence
once before replacing it.

**The job's name stays `go126`:** it is still the 1.26 line. Its
`GOTOOLCHAIN: local` and `go-version` (not `go-version-file`) stay.

**Steps:**

1. Re-run the red (P0 step 2), then make the edits above by script.
2. `go mod tidy -diff` is clean. `git diff --stat` names only the files
   above.
3. After P2 and P3 pass locally: `make pre-add-check FILES="<the eight
   .go files>"` through `run-heavy`; then commit, and push (D3), after the
   disclosure guard.

### Phase P2: every stated version agrees

A scratchpad script (not committed), `check_versions.py ROOT`, reads
`ROOT/go.mod` for the floor `F` (the `go` line) and the toolchain `T` (the
`toolchain` line), and compares, after joining wrapped lines:

* `ci.yml`'s `go126` `go-version` equals `F`, its `go env GOVERSION` test
  equals `goF`, and its comment names `F` and `T`;
* each of the ten fixtures above has the line `go F`, and none has another
  `go 1.2x.y` line;
* `AGENTS.md` says "Requires Go F; built and tested with Go T";
  `SECURITY.md` says `Go T toolchain` and `its F floor`; the architecture
  page says `requires Go F`, `toolchain goT` and, in its `go126` bullet,
  `Go F` and `goF`; the guide's section 1 is "Go F or later" and says
  "Build and test with Go T"; `README.md`'s Status names `F` as the
  current floor and `T` as the toolchain;
* a sweep: every `1.26.y`, `1.27.y`, `go1.26.y` or `go1.27.y` outside
  `docs/decisions/`, `CHANGELOG.md`'s earlier releases and a short named
  allowlist (the test data above, `githubupdater.go:104`, README's
  floor-by-release sentence, and the guide's historical sections) equals
  `F` or `T`.

**Seen to fail, for both numbers,** on scratch copies made with
`scripts/plant-copy.sh`, which refuses a break unless OLD occurs once:

* **the toolchain:** `plant-copy.sh go.mod "'toolchain go1.27.4'"
  "'toolchain go1.27.3'"`; the script run over the copy must exit
  non-zero and name `go.mod`, `ci.yml` and every doc that still says
  1.27.4;
* **the floor:** `plant-copy.sh go.mod "'go 1.26.11'" "'go 1.26.10'"`; it
  must exit non-zero and name the fixtures, `ci.yml:337` and `:339` and
  the docs that say 1.26.11;
* **a stray old pin:** `plant-copy.sh .github/workflows/ci.yml
  "\"go1.26.11\"" "\"go1.26.10\""` (OLD sized to occur once in the file);
  it must exit non-zero on the job's version test.

The copy has no `.git`, so the script walks files and does not call
`git`. On the real tree it exits 0. The handoff says the experiment ran
and what each failure looked like.

### Phase P3: gates

Hosts first. Go 1.26.11 is **not installed as an SDK on any gate host**
(MADR E3). This Mac has its toolchain in the module cache, so the floor
job's steps run here with `GOTOOLCHAIN=go1.26.11 GOPROXY=off`, no
download. The Linux machine and the Windows laptop would download it on
`GOTOOLCHAIN=go1.26.11`, which writes their module caches; the plan does
not do that. The floor is proven on this Mac and, authoritatively, by
CI's `go126` job on Linux (P4). The other hosts run the gates on their
installed 1.27.4.

1. On this Mac, `run-heavy -- make gate` with `SELFUPDATE_REQUIRE_PYTHON=1`
   ends `overall=0`, with `vuln rc=0` and `apicheck` compatible with
   `v1.26.2`.
2. On this Mac, the `go126` job's steps under
   `GOTOOLCHAIN=go1.26.11 GOPROXY=off`, each through `run-heavy`: the
   version test (`go env GOVERSION` is `go1.26.11`), `go build ./...`,
   `go vet ./...` for linux, windows and darwin with `CGO_ENABLED=0`,
   `go test -count=1 -timeout 20m ./...`, and `govulncheck ./...`
   ("No vulnerabilities found.").
3. **The green re-run:** `govulncheck ./...` under `go1.27.4` and
   `go1.26.11` exits 0 on the changed tree.
4. On the Linux machine, after `pick-host`:
   `run-on-host --need linux --race -- go test -race -count=1 -timeout 25m
   ./...`, then `run-on-host --need linux --race -- make gate`. On the
   Windows laptop: `run-on-host --need windows -- go test -count=1
   -timeout 20m ./...` (no `-race`, no WSL distribution for the race run).
   Each ends with every package `ok`.
5. `actionlint` and `scripts/check-workflows.sh` on `ci.yml`, plus
   `check-workflows_test.sh` and `workflow-shape_test.sh`.
6. `make markdownlint`, `make check-records`, `make docs-check`,
   `make disclosure-check`, each exit status before any filter.

**Not demonstrable locally:** that a runner installs what the `toolchain`
line names. Every host's `GOTOOLCHAIN` already names 1.27.4, so the line
changes nothing there; CI's `setup-go` logs show it (P4).

### Phase P4: CI on `main`

After P1's push:

1. "CI" on the pushed commit passes every job. The `setup-go` logs of the
   `go.mod`-driven jobs print `go1.27.4`; the `go126` job prints
   `go1.26.11`; both `govulncheck` steps print "No vulnerabilities
   found."; the two rehearsal build jobs, which build the stand-in caller
   from its own `go.mod`, run on 1.26.11.
2. "Docs" passes.
3. A failure ends the phase: it is a deviation, with evidence, not a
   retry.

### Phase P5: release `v1.26.3` (the owner's word at each step marked)

The steps 0029-PLAN W2 and W3 used, which are the architecture page's
"Releasing this module" steps 1 to 7, with `vX = v1.26.3` and `v1.26.2`
the previous release. **The plan stops before the tag.**

1. **The pre-tag change** (Markdown only): `CHANGELOG.md`'s `## Unreleased`
   becomes `## v1.26.3 - <date>`, keeps its "Security" entry, and links the
   guide section and both records. The guide gains `### From v1.26.2 to
   v1.26.3` before its `### Check`: no exported API changed (`make
   apicheck` reports it compatible with `v1.26.2`), the floor, the
   toolchain, GO-2026-6859, that a program builds with 1.27.4 or 1.26.11
   to take the fix, and under "Adopting it" that a caller moves its
   workflow pins to `v1.26.3` so the release tools build with 1.27.4.
2. `make gate` ends `overall=0`. Commit and push (D3); this commit changes
   only Markdown, so only "Docs" runs, and P4's CI run is the proof of
   the code.
3. **On the owner's word:** `scripts/check-release-tag.sh v1.26.3` and the
   disclosure guard over the outgoing tag exit 0; then the annotated tag
   and its push.
4. The tag's CI runs every job and passes; its identity legs print
   `v1.26.3 (release) <12-hex>`; no Docs run starts for the tag.
5. **The pin commit** (Markdown only): `git grep -n 'v1\.26\.2'` outside
   the records and the changelog lists the current-release pointers
   (README's current release, `go get` line and publish workflow pin; the
   architecture page's current release and its commit;
   `docs/guides/building-releases.md`'s pins and `ls-remote` line; the
   migration guide's `go get` lines, "gives `v1.26.2`" checks, workflow
   pin and `go.mod` adoption line). Each that names the current release
   moves to `v1.26.3` and its commit, and each change is named in the
   record; lines that name `v1.26.2` as history stay. Commit and push.
6. **The proxy:** `go list -m -json
   github.com/maccavelli/go-selfupdate-lib@v1.26.3` gives the release
   commit and `@latest` resolves to it; the proxy's `go.mod` for
   `v1.26.3` says `go 1.26.11` and `toolchain go1.27.4`.

Files P5 changes: `CHANGELOG.md`, `docs/guides/migrating-from-mcplib-selfupdate.md`,
`README.md`, `docs/architecture.md`, `docs/guides/building-releases.md`
(pre-tag and pin commits), and the execution record.

### Phase P6: closing

1. The verification below, with its output.
2. This plan becomes `complete` and its index row follows; the MADR stays
   `accepted`.
3. Open items are named: the callers' workflow pins (`gobble-cli`,
   `magic-cli-remote` on `v1.26.2`; `prepare-commit-msg` on `v1.26.0`,
   `go 1.27.3`) and the govulncheck pin.

## Verification

* **V1 (red):** `govulncheck` exits 3 under `go1.27.3` and `go1.26.10`,
  and 0 under `go1.27.4` and `go1.26.11` (P0 step 2).
* **V2 (agreement):** the P2 script exits 0 on the tree and non-zero on
  all three planted copies.
* **V3 (local):** `make gate` ends `overall=0` with `vuln rc=0`; the
  `go126` steps pass under `GOTOOLCHAIN=go1.26.11`; `govulncheck` exits 0
  under `go1.27.4` and `go1.26.11`; the workflow checks pass.
* **V4 (CI):** the push's CI passes every job on the toolchains P4 names.
* **V5 (release):** the tag's CI passes and the proxy serves `v1.26.3`
  with the right `go.mod`. Open until the owner's word.
* **V6 (docs):** `git grep -n '1\.27\.3\|go1\.27\.3\|1\.26\.10\|go1\.26\.10'`
  outside the records, `CHANGELOG.md`'s earlier releases, the guide's
  earlier sections, README's floor-by-release sentence and
  `githubupdater.go:104` finds nothing. Links, identifiers, records and
  markdownlint are clean.

## Rollout and Rollback

* **Before the tag,** P0 and P1 are reverted by new commits. No history is
  rewritten.
* **After the tag,** releases are immutable. A fault is fixed in a new
  patch release.
* **A consumer is affected** only when it takes `v1.26.3`, and then only
  if it builds with exactly Go 1.26.10 and `GOTOOLCHAIN=local`; the `go`
  command otherwise selects 1.26.11 or later. No repository under the
  fleet's Go directory is in that position (MADR E8).
* **A host** is not changed by this plan.

## Execution Record

### Approval (2026-11-17)

The owner ruled, as the MADR's Decided list records (D1 to D3): "Raise
the floor to 1.26.11 (Recommended)", "Yes: v1.26.3 from `main`, tagged on
my word (Recommended)", and "Record my answers, commit and push the pair,
then execute the plan through CI (Recommended)". The MADR became
`accepted`, and this plan `in-progress`. The owner's approval covers
phases P0 to P4, with commits and pushes to `main` for this plan only;
P5's tag and release wait for a separate word. The third open question of
the first draft, which records are amended, was not put to the owner: the
coordinator applied the author's recommendation (MADR D4), and the owner
is told.

### P0 (2026-11-17)

Staged, not committed. The pair itself was committed and pushed as
`9801a4a`, on top of `bab09ae`, before this phase.

1. **The hosts, read-only,** `go version` and `go env GOTOOLCHAIN`, by
   role:

   | Host | `go version` | `GOTOOLCHAIN` |
   | :--- | :--- | :--- |
   | this Mac | `go1.27.4 darwin/arm64` | `go1.27.4` |
   | the Linux machine | `go1.27.4 linux/amd64` | `go1.27.4` |
   | the Windows laptop | `go1.27.4 windows/amd64` | `go1.27.4` |
   | the Windows laptop's WSL distribution | `go1.27.4 linux/amd64` | `go1.27.4` |

   None is below 1.27.4, so the phase went on. Go 1.26.11 in each module
   cache: this Mac yes (`toolchain@v0.0.1-go1.26.11.darwin-arm64`, beside
   1.26.10, 1.27.3 and 1.27.4); the Linux machine no (1.26.6, 1.27.3 and
   1.27.4); the Windows laptop no (1.26.10 is its newest 1.26). Nothing
   was installed or downloaded.
2. **The red re-run,** on this Mac, `govulncheck` v1.9.0, database
   updated 2026-11-11 02:51:11 UTC, from the module cache. Each command
   was `GOTOOLCHAIN=<version> GOFLAGS=-mod=mod GOPROXY=off govulncheck
   ./...` in the repository root; the tree stayed clean.

   | `GOTOOLCHAIN` | Exit | Output lines |
   | :--- | :--- | :--- |
   | `go1.27.3` | 3 | "Vulnerability #1: GO-2026-6859"; "Found in: net/http@go1.27.3"; "Fixed in: net/http@go1.27.4"; "Your code is affected by 1 vulnerability from the Go standard library." |
   | `go1.27.4` | 0 | "No vulnerabilities found." |
   | `go1.26.10` | 3 | "Vulnerability #1: GO-2026-6859"; "Found in: net/http@go1.26.10"; "Fixed in: net/http@go1.26.11"; "Your code is affected by 1 vulnerability from the Go standard library." |
   | `go1.26.11` | 0 | "No vulnerabilities found." |

   This is MADR E4's measurement, repeated, with the same four exits.
3. **The amendments,** each a dated addition that rewrites nothing,
   citing 0045-MADR by full filename:
   * `0018-MADR-move-toolchain-to-go-1-27-2.md`: A3, "the toolchain line
     is go1.27.4", after A2 (`:203`);
   * `0029-MADR-move-toolchain-to-go-1-27-3-and-floor-to-1-26-10.md`: a
     new `## Amendments` (`:240`) with A1, "the toolchain line is go1.27.4
     and the floor is go 1.26.11" (`:242`);
   * `0025-MADR-align-with-go-sdk-design-practices.md`: Amendment A8,
     "the floor is go 1.26.11", after A7 (`:880`). It cites the owner's D1
     label and says the text is the coordinator's application.

   Each states that its text is the coordinator's application (MADR D4),
   not the owner's ruling. `docs/README.md` needed no change: no row's
   status moved.
4. **Not done, and why:** no commit, push or tag (the coordinator commits
   each phase). The checks of the plan's step 4 are recorded in this
   phase's handoff, not here, because they run after this entry.
5. **CI observation, read-only:** `gh run list --commit
   9801a4a9d75cc3f7779d6e7c6269c7724c64fabb` lists one run, "Docs",
   `success`. No "CI" run started, because the commit changed Markdown
   only (0028-MADR); the red the MADR predicts (E5) is therefore not yet
   observed in CI.

## Deviations

None yet.
