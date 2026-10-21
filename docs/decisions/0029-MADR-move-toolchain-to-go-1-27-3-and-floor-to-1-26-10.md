---
status: accepted
date: 2026-10-21
decision-makers: go-selfupdate-lib maintainers
consulted: "govulncheck v1.8.0 under four toolchains, 2026-10-21; magic-cli-remote's 0169-MADR, Amendment of 2026-10-21; go.dev's release list and actions/go-versions' manifest, read 2026-10-21"
informed: the fleet's programs, which require this module and call its workflows
---
# Build and test with Go 1.27.3, raise the floor to `go 1.26.10`, and release `v1.17.2`

## Context and Problem Statement

On 2026-10-21 the Go vulnerability database published GO-2026-6809,
"Distrust compromised intermediate certificates in crypto/x509". It is
fixed in Go 1.27.3 and 1.26.10. This repository builds with 1.27.2 (the
`toolchain` line,
[0018-MADR](0018-MADR-move-toolchain-to-go-1-27-2.md)) and declares a
1.26.9 floor, which CI's `go126` job tests
([0025-MADR](0025-MADR-align-with-go-sdk-design-practices.md) D8,
Amendment A1). Both are affected.

**`govulncheck ./...` at `be32614`, on this Mac, 2026-10-21
[observed]:** govulncheck v1.8.0, its database updated 2026-10-21
00:27:29 UTC, run once under each toolchain with `GOTOOLCHAIN=<version>`:

| Toolchain | Exit | Result |
| :--- | :--- | :--- |
| `go1.27.2` | 3 | GO-2026-6809, found in `crypto/x509@go1.27.2`, fixed in `crypto/x509@go1.27.3` |
| `go1.26.9` | 3 | GO-2026-6809, found in `crypto/x509@go1.26.9`, fixed in `crypto/x509@go1.26.10` |
| `go1.27.3` | 0 | "No vulnerabilities found." |
| `go1.26.10` | 0 | "No vulnerabilities found." |

The trace is reached through code that existed before this record:

```text
#1: selfupdate/manifestverify.go:125:20: selfupdate.checkedAsset.Read calls io.LimitedReader.Read, which eventually calls x509.Certificate.Verify
```

**Facts that shape the fix [read, observed 2026-10-21]:**

* **CI is red on its next run.**
  * `ci.yml`'s `validate` job and its `go126` job ("govulncheck, the
    floor toolchain", from
    [0027-MADR](0027-MADR-documentation-pass-after-v1-17-0.md) G1) each
    run `govulncheck ./...`, on 1.27.2 and 1.26.9.
  * The last CI run, on `d16b58a` (2026-10-20), passed before the
    advisory was published.
  * The two commits since changed only Markdown, so CI did not run on
    them ([0028-MADR](0028-MADR-skip-full-ci-for-docs-only-pushes.md)).
  * The next code push, and any tag, would fail both steps.
* **The fleet rule.** magic-cli-remote's
  `docs/decisions/0169-MADR-standardize-toolchains-on-current-supported-advisory-free-releases.md`,
  "Amendment — 2026-10-21: Go 1.27.3 is the standard (D2)", applies its D1
  to this advisory: Go 1.27.3 on every host and in CI, and **1.26.10 as
  the floor**.
  * That repository's `v0.22.1` tag run failed at `make vulncheck` on
    the same advisory.
  * go-llmprovider-sdk and magic-cli-remote already declare `go 1.27.3`.
* **The releases.**
  * `https://go.dev/dl/?mode=json` lists `go1.27.3` and `go1.26.10`.
  * The manifest `actions/setup-go` installs from
    (`actions/go-versions`, `versions-manifest.json`) lists `1.27.3` and
    `1.26.10`.
* **The library needs nothing from 1.27.** 0025-MADR D8 kept the floor on
  the older supported line for that reason, and nothing since has used a
  1.27 feature. Go 1.26 is still supported.
* **The default HTTP client is unchanged.** `defaultHTTPClient`'s comment
  (`selfupdate/githubupdater.go:101-103`) says it takes "Go 1.27.2's"
  `DefaultTransport` settings. That block of `net/http/transport.go` is
  identical in the installed go1.27.2 and go1.27.3 trees (`diff` of the
  `var DefaultTransport` block, exit 0).
* **The release workflows.**
  * The build workflow sets up Go from the *caller's* `go.mod`.
  * The publish workflow sets up Go from the pinned release's own
    `go.mod` (`go-version-file: .core-lib-release-tools/go.mod`). So a
    caller pinned to `v1.17.1` builds the release tools with 1.27.2
    until a release carrying `toolchain go1.27.3` exists and the caller
    pins it.
* **Since `v1.17.1`, `main` holds only 0028's work:** its records, CI,
  the docs workflow and the docs. The `## Unreleased` changelog entry
  is 0028's. `make apicheck` has nothing new to compare.
* **The hosts [observed 2026-10-21, read-only]:**

  | Host | `go version` | `GOTOOLCHAIN` |
  | :--- | :--- | :--- |
  | this Mac | `go1.27.3 darwin/arm64` | `go1.27.3` |
  | the Linux test host | `go1.27.3 linux/amd64` | `go1.27.3` |
  | the Windows test host | `go1.27.3 windows/amd64` | `go1.27.3` |
  | its WSL distribution | `go1.27.2 linux/amd64` | `go1.27.2` |

  This Mac's `golangci-lint` v2.14.0 and `govulncheck` v1.8.0 were built
  with go1.27.2. govulncheck takes the standard library's version from
  the `go` command it runs, not from its own build, as the table above
  shows. 0169's PLAN, P14, moves the hosts. In 0018 too, another record
  moved the hosts.
* **The precedent.**
  * 0018-MADR moved the toolchain in a patch release, `v1.11.1`.
  * [0025-PLAN](0025-PLAN-align-with-go-sdk-design-practices.md) S11
    moved the floor, and the test fixtures' `go` lines with it.

The question: how should this repository's builds, its declared floor
and its consumers get off GO-2026-6809?

## Decision Drivers

* **Ship nothing on a toolchain with a published advisory** (0169 D1).
  This applies to CI, the release tools and the gates.
* **Keep supporting Go 1.26**, as 0025-MADR D8 chose, while Go 1.26 is
  supported and the library needs nothing from 1.27.
* **The `go126` job must test the floor `go.mod` declares.** A job on
  one version and a `go` line on another prove nothing about either.
* **Every record and gate stays honest.** No `govulncheck` step is
  skipped, and no finding is excluded.
* **A fix reaches callers only in a release.** The publish workflow
  builds its tools with the pinned release's toolchain.

## Considered Options

* **A. `toolchain go1.27.3` and `go 1.26.10`; the `go126` job and the
  fixtures move to 1.26.10; release `v1.17.2` from `main`.**
* **B. `toolchain go1.27.3` only; the floor stays `go 1.26.9`.**
* **C. `go 1.27.3`, with no `toolchain` line and no `go126` job.**
* **D. Option A's tree changes, without a release.**

## Decision Outcome

Chosen option: **"A"**, the owner's choice on 2026-10-21 ("option a,
approved to proceed"). It moves every build this
repository runs (CI on both lines, the gates, and the release tools of
any caller that pins `v1.17.2`) off GO-2026-6809. It keeps Go 1.26
programs able to use the module, and the declared floor stays the
version CI tests.

1. **`go.mod`:** `go 1.26.9` becomes `go 1.26.10`, and
   `toolchain go1.27.2` becomes `toolchain go1.27.3`. No `require`
   changes, and `go mod tidy -diff` stays clean.
2. **CI, `.github/workflows/ci.yml`:**
   * every job that reads `go-version-file: go.mod` follows the
     `toolchain` line, through setup-go;
   * the `go126` job's `go-version` and its `GOVERSION` check become
     `1.26.10`, and its comment names the new floor and toolchain;
   * nothing else changes.
3. **The test fixtures** that write a `go 1.26.9` line become
   `go 1.26.10`, as 0025-PLAN S11 moved them with the floor, so that the
   tree names one floor. There are ten (the PLAN lists them).
4. **The docs** that state the current floor or toolchain say 1.26.10
   and 1.27.3, and cite this record: AGENTS.md, README, the architecture
   page, `SECURITY.md` and the migration guide's §1 and adoption steps.
   Historical lines stay as they are: the changelog's earlier releases,
   the guide's earlier sections, and the records.
5. **`defaultHTTPClient`'s comment stays.** "Go 1.27.2's" names the
   version whose `DefaultTransport` settings were copied, and 1.27.3's
   are identical.
6. **Release `v1.17.2` from `main`,** by the architecture page's
   "Releasing this module".
   * It holds 0028's CI split and this record's changes.
   * The tag's CI run is also 0028-PLAN's V4: the first tag after the
     split, which must run CI in full.
7. **The hosts are out of scope.** The WSL distribution's move, and
   rebuilding this Mac's two tools, belong to 0169 P14. This repository's
   live tests do not run on WSL until it reports go1.27.3.
8. **Amendments.**
   * 0018-MADR gains A2: the toolchain line is 1.27.3.
   * 0025-MADR gains A7: the floor is 1.26.10.

   Each one cites this record, and neither rewrites its history.

### Consequences

* Good, because CI, the gates and the release tools of a caller that
  pins `v1.17.2` build with a standard library that has no published
  advisory, on both supported lines.
* Good, because a program on Go 1.26 can still use the module.
* Good, because the `go126` job keeps testing exactly the declared floor.
* Good, because the release ships 0028's change and proves 0028's tag
  behaviour in one run.
* Neutral, because a consumer's binary takes the fix only when that
  program builds with 1.27.3 or 1.26.10 itself. A floor of 1.26.10
  makes the `go` command select at least that, unless
  `GOTOOLCHAIN=local` holds it on an older one.
* Bad, because a program on 1.26.9 exactly must move to 1.26.10 to
  take `v1.17.2`. This is a requirement change in a patch release, as
  0018's `v1.11.1` was for the toolchain. The changelog and the guide
  say so.
* Bad, because the WSL distribution stays on 1.27.2 until 0169 P14.
  Until then, live tests of this repository on WSL wait.

### Confirmation

* **Before the change:** the four govulncheck runs above, exits 3 and 3
  on 1.27.2 and 1.26.9. A pin cannot be shown failing on a broken input
  the way a test can (0018-PLAN T2 step 3), so the `go126` job's
  `GOVERSION` check is seen failing under `GOTOOLCHAIN=go1.26.9`
  instead.
* **After it, on this Mac:**
  * `make gate` ends `overall=0` with `vuln rc=0`;
  * under `GOTOOLCHAIN=go1.26.10`, the `go126` job's steps pass,
    `govulncheck` included.
* **CI on `main`** passes every job. Its setup-go logs name 1.27.3, and
  the `go126` job's name 1.26.10.
* **The `v1.17.2` tag's CI** runs every job, and passes. Its identity
  legs print `v1.17.2 (release) <12-hex>`.
* **The proxy** resolves `@latest` to `v1.17.2`, and its `go.mod` says
  `go 1.26.10` and `toolchain go1.27.3`.

## Pros and Cons of the Options

### A. `toolchain go1.27.3`, `go 1.26.10`, `v1.17.2`

* Good, because both supported lines this repository builds on are free
  of the advisory, and the floor CI tests is the floor `go.mod` declares.
* Good, because it follows the fleet rule's floor, and this module's own
  practice (0018, 0025 S11).
* Bad, because it raises consumers' minimum by one patch in a patch
  release.

### B. Toolchain only; floor stays 1.26.9

* Good, because no consumer's minimum changes.
* Bad, because the `go126` job's `govulncheck` step fails on 1.26.9.
  Keeping the job green would mean testing 1.26.10 while declaring
  1.26.9, or dropping that step. The first breaks the job's purpose and
  the second loosens a gate.

### C. `go 1.27.3`, no `toolchain` line, no `go126` job

* Good, because there is one version to track, as go-llmprovider-sdk and
  magic-cli-remote have.
* Bad, because it drops Go 1.26 support, which 0025-MADR D8 chose to
  keep while the library needs nothing from 1.27. It forces every
  program in the fleet onto 1.27.3 before it can take this release.

### D. The tree changes, no release

* Good, because nothing is tagged.
* Bad, because callers pinned to `v1.17.1` keep building the release
  tools with 1.27.2, and the module proxy keeps serving a 1.26.9 floor.
* Bad, because 0028's tag behaviour (V4) stays unproven until some later
  release.

## More Information

* `govulncheck` outputs of 2026-10-21, per toolchain, quoted above.
* [GO-2026-6809](https://pkg.go.dev/vuln/GO-2026-6809).
* [Go downloads](https://go.dev/dl/): `go1.27.3`, `go1.26.10`.
* magic-cli-remote,
  `docs/decisions/0169-MADR-standardize-toolchains-on-current-supported-advisory-free-releases.md`,
  D1, D2 and the Amendment of 2026-10-21.
* [0018-MADR](0018-MADR-move-toolchain-to-go-1-27-2.md), the toolchain
  line, and [0025-MADR](0025-MADR-align-with-go-sdk-design-practices.md)
  D8 and Amendment A1, the floor.
