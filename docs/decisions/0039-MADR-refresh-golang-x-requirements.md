---
status: accepted
date: 2026-10-30
decision-makers: go-selfupdate-lib maintainers
consulted: "the module proxy's version lists and module files for golang.org/x/sys, x/term and x/mod, read 2026-10-30; a diff of each package this module imports, between the pinned and the latest version; a scratch copy of main at 833f49f with the three moved; the go.mod files of magic-cli-remote and gobble-cli, read 2026-10-30"
informed: the fleet's programs, which require this module and call its workflows
---
<!-- markdownlint-disable MD013 -->

# Move `golang.org/x/sys`, `x/term` and `x/mod` to their current releases, and release `v1.23.1`

## Context and Problem Statement

The module requires three modules, named by
[0002-MADR](0002-MADR-rehome-selfupdate-from-mcplib.md) when the package
was rehomed on 2026-09-29: `golang.org/x/mod v0.40.0`,
`golang.org/x/sys v0.47.0` and `golang.org/x/term v0.43.0`. None has
moved since. Each now has a newer release, and no record has decided
when this module moves them.

**The releases [read from the module proxy, 2026-10-30]:**

| Module | Pinned | Its date | Latest | Its date | Releases between | `go` line, pinned → latest |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| `golang.org/x/sys` | `v0.47.0` | 2026-06-30 | `v0.49.0` | 2026-10-06 | `v0.48.0` | `1.25.0` → `1.26.0` |
| `golang.org/x/term` | `v0.43.0` | 2026-05-08 | `v0.47.0` | 2026-10-09 | `v0.44.0` to `v0.46.0` | `1.25.0` → `1.26.0` |
| `golang.org/x/mod` | `v0.40.0` | 2026-08-13 | `v0.42.0` | 2026-10-09 | `v0.41.0` | `1.25.0` → `1.26.0` |

`x/term v0.47.0` requires `x/sys v0.49.0`; `x/mod v0.42.0` requires
`x/tools v0.51.0`, marked `tagx:ignore`, which `go mod tidy` prunes from
this module's `go.sum`, as it prunes `v0.49.0` today
([0008-MADR](0008-MADR-enforce-import-rules-with-depguard.md) notes
`x/tools` in the graph). Every new `go` line is below this module's floor,
`go 1.26.10`
([0029-MADR](0029-MADR-move-toolchain-to-go-1-27-3-and-floor-to-1-26-10.md)).

**The programs already run the newer code [read 2026-10-30].** Minimal
version selection builds a program with the highest version any module in
its graph requires, so this module's pins are a floor, not what ships:

| Program | `x/sys` | `x/term` | `x/mod` | go-selfupdate-lib |
| :--- | :--- | :--- | :--- | :--- |
| magic-cli-remote | `v0.49.0` | `v0.47.0` (indirect) | `v0.41.0` | `v1.22.3` |
| gobble-cli | `v0.49.0` | `v0.47.0` | `v0.42.0` (indirect) | `v1.23.0` |

So both programs run `selfupdate`'s Windows and Unix code on
`x/sys v0.49.0`, while this module's CI, gates and live tests run it on
`v0.47.0`.

**What changed in the packages this module imports [a diff of each
package's non-test files, pinned → latest, 2026-10-30]:**

| Package | Identifiers this module uses | Change |
| :--- | :--- | :--- |
| `x/mod/semver` | 4 (`Canonical`, `Compare`, `IsValid`, `Prerelease`) | none |
| `x/mod/sumdb/note` | 9 (the signer, verifier and `Open`) | none |
| `x/sys/windows/svc` | 14 | none |
| `x/sys/windows/svc/mgr` | 6 | none |
| `x/sys/windows` | 126 | adds `SO_SNDTIMEO`; deletes `types_windows_arm.go`, the 32-bit windows/arm port's types |
| `x/sys/unix` | 7 (`Flock`, `LOCK_EX`, `LOCK_NB`, `LOCK_UN`, `O_NOFOLLOW`, `EAGAIN`, `EWOULDBLOCK`) | 81 files, mostly generated Linux and z/OS types; no changed line names one of the seven |
| `x/term` | 1 (`IsTerminal`) | `Terminal.ReadLine` keeps data read with an error; `IsTerminal` unchanged |

* The deleted windows/arm port is not one Go 1.26 or 1.27 builds:
  `go tool dist list` lists `windows/386`, `windows/amd64` and
  `windows/arm64` under both toolchains. No file of this repository names
  windows/arm; its `arm` entries are Linux and GoReleaser's.
* The Windows procedures this module loads by hand (`GetConsoleProcessList`,
  the COM calls in `taskscheduler/com_windows.go`, and test helpers) are
  absent from `x/sys/windows` in both versions alike, so none can be
  dropped.

**The move, in a scratch copy of `main` at `833f49f` [observed,
2026-10-30]:**

* `go get golang.org/x/sys@v0.49.0 golang.org/x/term@v0.47.0 golang.org/x/mod@v0.42.0`,
  then `go mod tidy`: three `require` lines and six `go.sum` lines change,
  and nothing else; the `go` and `toolchain` lines stay.
* `go vet ./...` for `GOOS=linux`, `darwin` and `windows` (`CGO_ENABLED=0`):
  exit 0 each. `go test -count=1 ./...` on this Mac: 20 packages `ok`.
  `govulncheck ./...`: "No vulnerabilities found."
* `go mod tidy -diff` failed (exit 1) on one stray line,
  `golang.org/x/tools v0.51.0/go.mod`, which the probe's own
  `go list -m all` under `-mod=mod` had added after the tidy. A second
  `go mod tidy` removed it, and `tidy -diff` then exited 0. The gate's
  tidy step catches that line.

**Where the versions are written [read 2026-10-30]:** `go.mod`, `go.sum`,
and one current statement, `docs/architecture.md:17-18`. Every other
mention is in a record, as history. `.golangci.yml`'s depguard rules
allow modules by path, not version.

**How a release reaches callers.** A program takes this module's
requirements when it requires a release that carries them. The publish
workflow builds its release tools from the pinned release's own `go.mod`
(`publish-selfupdate-release.yml`, `go-version-file:
.core-lib-release-tools/go.mod`), so a caller's tools build with the
module versions of the release it pins.

The question: should this module move its three `golang.org/x`
requirements now, and how should the move reach callers?

## Decision Drivers

* **Test what ships.** The library's tests, gates and live tests should
  run on the `x/` code the fleet's programs build with.
* **Keep the patch line honest.** No exported identifier and no behaviour
  of this module changes, so the release is a patch, as `v1.17.2` was for
  the toolchain
  ([0029-MADR](0029-MADR-move-toolchain-to-go-1-27-3-and-floor-to-1-26-10.md)).
* **Change nothing a caller does not need.** No new module, no change to
  the `go` floor or the toolchain, and no new `x/` API used.
* **Every gate stays as it is.** The bump passes `make gate` and full CI,
  the live tests included, or it waits.
* **The owner's workflow.** Changes land as records, then commits the
  owner makes on `main`; tags are the owner's.

## Considered Options

* **A. Move all three to their latest releases, with no code change, and
  release `v1.23.1`.**
* **B. Option A's tree change, with no release until the next one.**
* **C. Move `x/sys` alone.**
* **D. Keep the pins until an advisory, or a needed API, moves them.**
* **E. An automated updater (Dependabot or Renovate) that opens a pull
  request for each new release.**

## Decision Outcome

Chosen option: **"A"**, the owner's choice on 2026-10-30 ("Move all,
release v1.23.1"), because it is the only option under which the library's CI tests the
`x/` code its programs already ship, and callers who pin its workflows
build their release tools with it, at the cost of one patch release with
no API or behaviour change.

1. **`go.mod` and `go.sum`:** `golang.org/x/sys v0.49.0`,
   `golang.org/x/term v0.47.0` and `golang.org/x/mod v0.42.0`, by `go get`
   of those three versions and `go mod tidy`. Three `require` lines and
   six `go.sum` lines change. The `go 1.26.10` and `toolchain go1.27.3`
   lines stay, and `go mod tidy -diff` is clean.
2. **No source change.** No identifier this module uses changed. No new
   `x/` API is adopted here; a later record that wants one names it.
3. **The current docs:** `docs/architecture.md:17-18` names the three new
   versions and cites this record. The records that name the old versions
   stay as written.
4. **`v1.23.1`, a patch release from `main`,** by the architecture
   page's "Releasing this module", as 0038-PLAN Q8 and Q18 released
   `v1.22.3` and `v1.23.0`:
   * `CHANGELOG.md` gains `## v1.23.1`, under Changed;
   * the migration guide's §23 gains "From v1.23.0 to v1.23.1", before
     its Check, saying there is nothing to adopt and what a program's
     `go.mod` may gain;
   * after the tag, the pin commit moves the current-release pins to
     `v1.23.1`.
5. **Out of scope:** the Go toolchain and floor; the versions of
   `golangci-lint`, `govulncheck`, `markdownlint-cli2`, `apidiff` and the
   Actions; other repositories, which move their own pins; and an
   automated updater (option E).

### Consequences

* Good, because CI, the gates and the live tests run `selfupdate` on the
  `x/sys` and `x/term` its programs build with today.
* Good, because a caller that pins `v1.23.1`'s workflows builds its
  release tools with these versions.
* Good, because the change is three `require` lines, with no code, API
  or floor change, and `make apicheck` can show it.
* Neutral, because a program that takes `v1.23.1` has its `x/` modules
  raised to at least these versions. Of the two programs above, only
  magic-cli-remote's `x/mod` moves, from `v0.41.0` to `v0.42.0`, whose
  `semver` and `sumdb/note` are unchanged.
* Neutral, because the new versions' `go 1.26.0` lines raise no
  program's minimum: this module already requires 1.26.10.
* Bad, because it is a release with nothing a program calls differently,
  and a release cycle costs CI runs, a tag and a pin commit.

### Confirmation

* **The change:** `git diff` of `go.mod` and `go.sum` shows only the three
  `require` lines and six sums; `go mod tidy -diff` exits 0.
* **The gates:** `make gate` ends `overall=0`, with `tidy`, `vuln` and
  `apicheck` ("compatible with v1.23.0") among its steps; `make
  markdownlint`, `make docs-check` and `make check-records` exit 0.
* **The floor:** under `GOTOOLCHAIN=go1.26.10`, `go vet ./...` and
  `go test ./...` pass on this Mac; CI's `go126` job passes.
* **The platforms:** `go test -race ./...` passes on the Linux and
  Windows test hosts, and CI's three `validate` jobs pass with every
  live test.
* **The API:** `apidiff` between `v1.23.0` and `v1.23.1` lists no change.
* **The release:** the `v1.23.1` tag's CI passes every job, its identity
  legs print `v1.23.1 (release) <12-hex>`, and the proxy resolves
  `@latest` to `v1.23.1`, whose `go.mod` requires the three new versions.

## Pros and Cons of the Options

### A. Move all three, release `v1.23.1`

* Good, because what the library tests matches what its programs build.
* Good, because callers' release tools take the new versions through the
  workflow pin.
* Bad, because it is a release cycle for a change no program needs in
  order to build.

### B. Move them in the tree, release later

* Good, because CI on `main` tests the new versions at once, with no tag.
* Bad, because the newest release keeps the old requirements, so a
  caller's release tools keep building with them, and the release that
  ships the move is one whose changelog is about something else.

### C. Move `x/sys` alone

* Good, because `x/sys` is the module whose code differs by platform.
* Bad, because `x/term v0.47.0` requires `x/sys v0.49.0`, and holding
  `x/term` and `x/mod` back saves nothing: the packages this module
  imports from them changed only in `Terminal.ReadLine`, which it does
  not call, and not at all.

### D. Keep the pins

* Good, because nothing changes.
* Bad, because the gap between what is tested and what ships stays, and
  grows with each `x/` release.
* Bad, because an advisory would force the move under time pressure, in
  a module whose last move was at the rehome.

### E. An automated updater

* Good, because pins would not drift.
* Bad, because this repository's changes land as an approved MADR and
  PLAN, then a commit the owner makes on `main`; a bot's pull request
  carries neither, and the fleet takes no pull requests here.
* Bad, because it adds a configuration file and a GitHub app to audit,
  for three modules that release every few weeks.

## More Information

* [0002-MADR](0002-MADR-rehome-selfupdate-from-mcplib.md), which named the
  three modules and their first versions.
* [0029-MADR](0029-MADR-move-toolchain-to-go-1-27-3-and-floor-to-1-26-10.md),
  the precedent for a patch release that moves a requirement and changes
  no API.
* [0038-PLAN](0038-PLAN-pinned-plans-lifecycle-conformance-and-bubble-tests.md)
  Q8 and Q18, the release steps this record's PLAN repeats.
* The module proxy: `go list -m -versions` and `go mod download -json` of
  each module, read 2026-10-30.
