---
status: accepted
date: 2026-11-17
decision-makers: go-selfupdate-lib maintainers
consulted: "govulncheck v1.9.0 (vulnerability database updated 2026-11-11 02:51 UTC) under four toolchains, 2026-11-17; go.dev's release list and release history page, and actions/go-versions' manifest, read 2026-11-17; the three sibling repositories that call this module's workflows, read-only"
informed: the fleet's programs, which require this module and call its workflows
---
# Build and test with Go 1.27.4 through the `toolchain` line, and raise the floor to `go 1.26.11`

## Context and Problem Statement

The owner's first statement of 2026-11-17, relayed by the coordinator:

> "go-selfupdate-lib is pinning the wrong go versions. the floor should be 1.26.10 and the version/toolchain should be 1.27.4"

At that statement `go.mod` already said `go 1.26.10` (`go.mod:3`), so the
floor was as the owner named it, and only the `toolchain` line was
behind: `toolchain go1.27.3` (`go.mod:5`). A scan the same day (E4) then
showed that Go 1.26.10 itself carries GO-2026-6859, fixed in 1.26.11, so
a floor of 1.26.10 would leave CI's floor job red whatever was done to
the toolchain. The owner was asked and ruled (the Decided list below,
D1) to raise the floor to 1.26.11. **That ruling supersedes the 1.26.10
in the owner's first statement, by the owner's own later ruling.** The
toolchain of 1.27.4 in the first statement stands. The title names both
numbers because the first statement named both.

Everything in this record other than the quoted statement and the quoted
labels is evidence or the coordinator's and author's text.

**Evidence index.** Each item is read or observed on 2026-11-17 at HEAD
`bab09ae` with a clean tree, unless it says otherwise.

* **E1, the pins [read].** The places that state a Go version are in the
  PLAN's version table. The ones that name the toolchain are `go.mod:5`,
  `.github/workflows/ci.yml:323`, `AGENTS.md:57-58`, `README.md:26-28`,
  `SECURITY.md:41`, `docs/architecture.md:21-24` and
  `docs/guides/migrating-from-mcplib-selfupdate.md:20`. The ones that name
  the floor are `go.mod:3`, `ci.yml:322`, `:337` and `:339`, `AGENTS.md:57`
  and `:62`, `README.md:26`, `SECURITY.md:42`, `docs/architecture.md:17`,
  `:26` and `:1016-1018`, the guide's section 1, and ten test fixtures.
  Every one agreed with `go 1.26.10` and `toolchain go1.27.3` at `bab09ae`.
  No file stated a different floor.
* **E2, the reusable workflows [read].** Neither names a Go version.
  `build-selfupdate-release.yml:105-114` sets up Go from the caller's
  `go.mod` (`go-version-file: src/<module-dir>/go.mod`, with
  `GOTOOLCHAIN: local`). `publish-selfupdate-release.yml:133-135` and
  `:308-310` set up Go from this module's own `go.mod` at the pinned
  release (`go-version-file: .core-lib-release-tools/go.mod`). The floor
  is a statement about consumers; the toolchain line decides what a
  caller's release tools are built with.
* **E3, the hosts [observed, read-only].** Go 1.27.4 is installed and is
  the default on every machine this repository's gates run on:

  | Host | `go version` | `GOTOOLCHAIN` |
  | :--- | :--- | :--- |
  | this Mac | `go1.27.4 darwin/arm64` | `go1.27.4` |
  | the Linux machine | `go1.27.4 linux/amd64` | `go1.27.4` |
  | the Windows laptop | `go1.27.4 windows/amd64` | `go1.27.4` |
  | the Windows laptop's WSL distribution (never for `-race`) | `go1.27.4 linux/amd64` | `go1.27.4` |

  Each keeps only `~/sdk/go1.27.4` as its Go SDK. **Go 1.26.11 is installed
  on none of them as an SDK.** Only this Mac has its toolchain in the
  module cache (`toolchain@v0.0.1-go1.26.11.darwin-arm64`, beside 1.26.10).
  The Linux machine's module cache holds 1.26.6, 1.27.3 and 1.27.4; the
  Windows laptop's holds 1.26.10 and no 1.26.11. So a local run of the
  floor job's steps works on this Mac without a download, and on the
  others `GOTOOLCHAIN=go1.26.11` would download it. Nothing was installed.
  A `toolchain go1.27.4` line changes nothing on a host, because each
  host's `GOTOOLCHAIN` already names 1.27.4; only CI's `setup-go` steps
  follow the line.
* **E4, `govulncheck ./...` at `bab09ae` [observed].** govulncheck v1.9.0
  (the Makefile and CI install v1.8.0), database updated 2026-11-11
  02:51:11 UTC, run once under each toolchain with `GOTOOLCHAIN=<version>`
  from the local module cache and `GOPROXY=off`:

  | Toolchain | Exit | Result |
  | :--- | :--- | :--- |
  | `go1.27.3` (the old `toolchain` line) | 3 | GO-2026-6859, "HTTP/1 server crash due to MaxBytesReader header race in net/http", found in `net/http@go1.27.3`, fixed in `net/http@go1.27.4` |
  | `go1.27.4` | 0 | "No vulnerabilities found." |
  | `go1.26.10` (the old floor, and the `go126` job's toolchain) | 3 | the same advisory, found in `net/http@go1.26.10`, fixed in `net/http@go1.26.11` |
  | `go1.26.11` | 0 | "No vulnerabilities found." |

  `make vuln` on this Mac, which runs the host's Go 1.27.4, exits 0. The
  traces under 1.27.3 and 1.26.10 run through
  `selfupdate.GitHubSource.do` (`selfupdate/github.go:656`),
  `selfupdate.rateLimitedForbidden` (`:927`) and
  `selfupdatetest.HeaderNames.Has`
  (`selfupdate/selfupdatetest/githubserver.go:50`): code that existed
  before this record.
* **E5, CI [observed].** The newest "CI" run on `main`, on `cc21690`,
  concluded `success` at 2026-11-11 00:26 UTC, 2 h 25 min before the
  database entry E4 shows. No "CI" run has started since, because the
  commits after `cc21690` changed Markdown only. `ci.yml`'s `validate`
  job (`:219-223`) and its `go126` job (`:350-353`) both run
  `govulncheck ./...`, so on E4's evidence the next full CI run, and the
  next tag, fail both [inferred, not observed: CI installs govulncheck
  v1.8.0, and only v1.9.0 was run here].
* **E6, the releases [read, documentation, not measurement].** go.dev's
  `?mode=json&include=all` and `actions/go-versions`' manifest list
  `1.27.4`, `1.27.3`, `1.26.11` and `1.26.10`. go.dev's release history
  page says go1.27.4 and go1.26.11 were released 2026-11-10 and "include
  security fixes to cgo, the go command, the compiler, the linker, and the
  net/http package". It does not name GO-2026-6859 on that page.
* **E7, the default HTTP client [observed].** `defaultHTTPClient`'s
  comment names "Go 1.27.3's" `DefaultTransport` settings
  (`selfupdate/githubupdater.go:104`). The `var DefaultTransport` block of
  `net/http/transport.go` is identical in the Go 1.27.3 and 1.27.4 trees
  (`diff`, exit 0), so the comment stays true.
* **E8, who consumes this [read, read-only `grep` of siblings].**

  | Repository | `go.mod` | Requires this module | Calls this module's workflows |
  | :--- | :--- | :--- | :--- |
  | gobble-cli | `go 1.27.4` | `v1.26.2` | build and publish, pinned to `v1.26.2` |
  | magic-cli-remote | `go 1.27.4` | `v1.26.2` | build and publish, pinned to `v1.26.2` |
  | prepare-commit-msg | `go 1.27.3` | `v1.26.0` | build and publish, pinned to `v1.26.0` |

  No other repository under the fleet's Go directory requires this
  module or calls its workflows. `go-tui-lib` (`go 1.27.2`), the MCP
  servers (`go 1.26.6`) and `mcplib` do not.
* **E9, earlier moves [read].**
  [0018-MADR](0018-MADR-move-toolchain-to-go-1-27-2.md) put the toolchain
  in a `toolchain` line and released `v1.11.1`, and
  [0029-MADR](0029-MADR-move-toolchain-to-go-1-27-3-and-floor-to-1-26-10.md)
  moved the toolchain to 1.27.3 and the floor to 1.26.10 and released
  `v1.17.2`. Both released because the publish workflow builds a caller's
  release tools with the pinned release's own `toolchain` line
  (0029-MADR's Decision Drivers, last one), so a fix reaches callers only
  in a release. 0029-MADR's option B (toolchain only, floor stays) was
  refused because the floor job's `govulncheck` step then fails on the old
  floor, and keeping the job green meant testing one version while
  declaring another, or dropping the step. The current release is
  `v1.26.2` (`README.md:30`, `CHANGELOG.md:11`).
* **E10, which fixtures need the floor [observed in a scratch copy,
  2026-11-17].** A module with `go 1.26.10` that requires, through
  `replace`, a module with `go 1.26.11` fails
  `GOTOOLCHAIN=local GOFLAGS=-mod=readonly go build ./...` ("updates to
  go.mod needed, disabled by -mod=readonly") and builds once its own line
  is `go 1.26.11` (Go 1.27.4). So a test fixture that requires this
  module through `replace` is a live pin of the floor; a standalone one is
  data. The PLAN's fixture table decides each.

The question: how should this repository's builds, its release tools and
its declared floor follow the owner's rulings, given that Go 1.26.10 and
Go 1.27.3 both carry GO-2026-6859?

## Decision Drivers

* **The owner's rulings** (D1 to D3 below): a toolchain of 1.27.4, a floor
  of 1.26.11, a release, and execution through CI.
* **Ship nothing on a toolchain with a published advisory**, the rule
  0018-MADR and 0029-MADR applied. E4 shows GO-2026-6859 in 1.27.3 and in
  1.26.10.
* **The `go126` job must test the floor `go.mod` declares**
  (0029-MADR's Decision Drivers). A job on one version and a `go` line on
  another prove nothing about either.
* **No gate is loosened.** No `govulncheck` step is skipped, and no
  finding is excluded.
* **A fix reaches callers only in a release** (E9).
* **The tree states one toolchain and one floor.** After the change,
  every current statement agrees with `go.mod`.
* **No compatibility engineering.** The owner is the only user of the
  fleet's repositories.

## Considered Options

* **A. `toolchain go1.27.4`; `go 1.26.10` stays.**
* **B. `toolchain go1.27.4`, and the floor moves to `go 1.26.11`.**
* **C. Drop the `toolchain` line; the `go` line stays `go 1.26.10`.**
* **D. Change nothing until a failing run forces it.**

Within A, three ways to run the `go126` job were put to the owner (D1
below): raise the floor (this record's B), keep the floor and run the job
on 1.26.11, or keep the floor, build on 1.26.10 and scan on 1.26.11.

## Decision Outcome

Chosen option: **"B"**, the owner's ruling D1 together with the owner's
statement of the toolchain, because it moves every build this repository
controls (CI's toolchain jobs, the floor job, the release tools of any
caller that pins the next release) onto a standard library that
`govulncheck` reports clean (E4), and keeps the floor job testing and
scanning exactly the floor `go.mod` declares, as 0029-MADR required.

1. **`go.mod`:** `go 1.26.10` becomes `go 1.26.11` (`:3`), and
   `toolchain go1.27.3` becomes `toolchain go1.27.4` (`:5`). No `require`
   changes, and `go mod tidy -diff` stays clean.
2. **CI, `.github/workflows/ci.yml`:** every job that reads
   `go-version-file: go.mod` (`:48`, `:369`, `:422`) follows the
   `toolchain` line through `setup-go`. The `go126` job's `go-version`
   (`:337`) and its version test (`:339`) become 1.26.11, and its comment
   (`:322-326`) names 1.26.11 and 1.27.4. The job keeps its name: it is
   still the 1.26 line.
3. **The test fixtures** that write a `go` line follow, as 0029-MADR
   moved them. The two that require this module through `replace` must
   (E10); the PLAN decides the other eight from the test that reads each.
4. **The docs** that state the current floor or toolchain say 1.26.11 and
   1.27.4 and cite this record, in the same commit as `go.mod`: AGENTS.md,
   README, SECURITY.md, the architecture page, the migration guide's
   section 1, and a changelog entry. Historical lines stay.
5. **`defaultHTTPClient`'s comment stays** (E7).
6. **The hosts need no install.** Go 1.27.4 is on all of them (E3).
   Go 1.26.11 is on none as an SDK; the floor job runs in CI, and locally
   on this Mac from its module cache.
7. **A release follows** (D2): `v1.26.3` from `main`. The plan stops
   before the tag; the tag and the release each need the owner's word.
8. **Amendments** (D4): 0018-MADR gains A3 (the toolchain line is
   go1.27.4), 0029-MADR gains A1 (the toolchain line is go1.27.4 and the
   floor is go 1.26.11), and 0025-MADR gains A8 (the floor is
   `go 1.26.11`), each linking this record and none rewriting history.
   This record amends those three and supersedes none.

### Consequences

* Good, because CI, the gates and the release tools of a caller that pins
  `v1.26.3` build with a standard library `govulncheck` reports clean
  (E4), on both supported lines.
* Good, because the `go126` job keeps testing and scanning exactly the
  declared floor.
* Bad, because **consumers' minimum rises by one patch**, in a patch
  release, as 0029's did. Who that touches, from E8:
  * **gobble-cli** (`go 1.27.4`) and **magic-cli-remote** (`go 1.27.4`)
    already declare more than 1.26.11. Nothing breaks. Neither has to
    change its `go.mod`; each moves its workflow pins to `v1.26.3` in its
    own repository to take the release tools' 1.27.4.
  * **prepare-commit-msg** (`go 1.27.3`, requiring `v1.26.0`) declares more
    than 1.26.11, so taking `v1.26.3` breaks nothing. Its own `go` line
    stays on a toolchain with GO-2026-6859 until that repository moves it;
    that is its owner's record to write.
  * **A program on exactly Go 1.26.10** with `GOTOOLCHAIN=local` cannot
    build against `v1.26.3`; the `go` command otherwise selects 1.26.11.
    No repository under the fleet's Go directory is in that position.
* Bad, because the change touches `go.mod`, `ci.yml`, ten fixtures and six
  docs, runs full CI, and adds a tag, its CI run and a pin commit.
* Neutral, because a consumer's own binary takes the fix only when that
  program builds with 1.27.4 or 1.26.11 itself.
* Neutral, because hosts already run 1.27.4 (E3): local gates cannot show
  the new `toolchain` line taking effect, and CI's `setup-go` logs do.
* Bad, because Go 1.26.11 is on no gate host as an SDK (E3): the floor
  job's steps run locally only on this Mac, and CI runs them on Linux.

### Confirmation

* **Before the change:** E4's runs, exit 3 under `go1.27.3` and
  `go1.26.10`, are the red. The PLAN repeats them at its start.
* **After it:**
  * `govulncheck` under `go1.27.4` and `go1.26.11` exits 0, re-run on the
    changed tree;
  * the PLAN's version-agreement check passes on the tree, and fails on a
    scratch copy with a planted wrong toolchain and on one with a planted
    wrong floor;
  * `make gate` ends `overall=0` with `vuln rc=0`;
  * the `go126` job's steps pass under `GOTOOLCHAIN=go1.26.11`;
  * CI on `main` passes every job; its `setup-go` logs name 1.27.4 for the
    `go.mod`-driven jobs and 1.26.11 for `go126`.
* **For the release:** the tag's CI passes every job, its identity legs
  print `v1.26.3 (release) <12-hex>`, and the proxy's `go.mod` for the
  tag says `go 1.26.11` and `toolchain go1.27.4`.

## Pros and Cons of the Options

### A. `toolchain go1.27.4`; `go 1.26.10` stays

* Good, because it is what the owner first stated, and it changes the
  fewest lines.
* Bad, because 1.26.10 is itself affected (E4): the floor job stays red,
  or its toolchain stops matching the floor, which 0029-MADR's option B
  refused (E9).

### B. `toolchain go1.27.4`, floor `go 1.26.11` (chosen)

* Good, because floor, floor job, fixtures and docs all name an
  advisory-free patch, and the job still tests what `go.mod` declares.
* Good, because it follows 0029-MADR's own argument and its practice of
  moving the fixtures with the floor.
* Bad, because it departs from the 1.26.10 first named, and raises
  consumers' minimum by one patch in a patch release.

### C. Drop the `toolchain` line

* Good, because there would be one version line to track.
* Bad, because `setup-go`'s `go-version-file` then falls back to the `go`
  line (0018-MADR's `setup-go` evidence), so every CI job and every
  release tool would build with the floor, and the owner asked for a
  1.27.4 toolchain.

### D. Change nothing

* Good, because nothing is tagged.
* Bad, because the next CI run and the next tag fail `govulncheck` (E5),
  and callers that pin `v1.26.2` keep building release tools with 1.27.3.

## More Information

### Decided

The owner's rulings of 2026-11-17, relayed by the coordinator. Each
label is quoted exactly and only as a label; the text beside it says what
was asked.

* **D1, the floor.** Chosen label: "Raise the floor to 1.26.11
  (Recommended)". Asked: `go.mod` already had the floor the owner named,
  and only the `toolchain` line was behind; Go 1.26.10 itself carries
  GO-2026-6859, fixed in 1.26.11 and 1.27.4, so the floor job would go red
  on its next run whatever was done to the toolchain; the chosen option
  moves `go.mod`, the floor job, ten test fixtures and the docs to
  1.26.11 so the job keeps testing and scanning the declared floor, at the
  cost of departing from the 1.26.10 first named, and programs using the
  library then need Go 1.26.11 or later. The other two options kept
  `go 1.26.10` and ran the job, or only its scan, on 1.26.11.
* **D2, the release.** Chosen label: "Yes: v1.26.3 from `main`, tagged on
  my word (Recommended)". Asked: whether a release follows. The tag and
  the release each still need the owner's word when the time comes; the
  PLAN stops before them.
* **D3, how this pair proceeds.** Chosen label: "Record my answers,
  commit and push the pair, then execute the plan through CI
  (Recommended)". Asked: whether to record the answers and stop, or
  proceed. This is the owner's approval of the PLAN through its CI phase,
  and the authority for commits and pushes to `main` in this repository
  for this plan only. The tag and the release wait for a separate word.
* **D4, which records are amended; not put to the owner.** This was the
  third open question of the first draft. The coordinator applied the
  author's recommended option, following 0029's practice: 0018-MADR A3,
  0029-MADR A1, and 0025-MADR A8 because the floor moves. This is the
  coordinator's application, not the owner's ruling, and the owner is
  told.

### Not verified

* That CI's `govulncheck` v1.8.0 reports GO-2026-6859 (E5): only v1.9.0
  ran here. The database is shared, so it is likely, not shown.
* What GO-2026-6859 is beyond its title and its fixed versions: the
  description was not read. The Go release history names fixes in
  `net/http` for 1.27.4 and 1.26.11 without naming the advisory.
* Whether `actions/setup-go` v7.0.0 installs 1.27.4 and 1.26.11 on the
  runners: the manifest lists both (documentation), and no run has
  installed them.
* What 1.27.4's other fixes (cgo, the go command, the compiler, the
  linker) mean for this module: none was assessed.
* Whether the reachable `net/http` trace is exploitable here: this module
  is a client library and the advisory's title names an HTTP/1 server.
  `govulncheck` exits 3 on a reachable symbol regardless.
* Why 1.27.4: the owner's statement is the decision either way.

### Links

* [GO-2026-6859](https://pkg.go.dev/vuln/GO-2026-6859).
* [Go downloads](https://go.dev/dl/) and the
  [release history](https://go.dev/doc/devel/release): `go1.27.4`,
  `go1.26.11`.
* [0018-MADR](0018-MADR-move-toolchain-to-go-1-27-2.md), the `toolchain`
  line;
  [0029-MADR](0029-MADR-move-toolchain-to-go-1-27-3-and-floor-to-1-26-10.md),
  the previous toolchain and floor; and
  [0025-MADR](0025-MADR-align-with-go-sdk-design-practices.md) D8 and
  Amendments A1 and A7, the floor.
