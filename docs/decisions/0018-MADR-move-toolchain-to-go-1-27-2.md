---
status: accepted
date: 2026-10-08
decision-makers: go-selfupdate-lib maintainers
consulted: govulncheck v1.8.0's report of 2026-10-08; magic-cli-remote's 0169-MADR (the fleet toolchain rule); actions/setup-go v7.0.0's README
informed: the fleet's programs, which call this module's workflows
---
# Build and test with Go 1.27.2 through a `toolchain` line, keep `go 1.27.1` as the floor, and release `v1.11.1` from a branch

## Context and Problem Statement

On 2026-10-08, during
[0017-PLAN-verify-build-provenance-and-close-0015-open-items.md](0017-PLAN-verify-build-provenance-and-close-0015-open-items.md)
Phase Q2, `make gate`'s `vuln` step failed. `govulncheck` v1.8.0 reported
ten advisories in Go 1.27.1's standard library, all fixed in Go 1.27.2:

| Advisory | Package | Summary |
| :--- | :--- | :--- |
| GO-2026-6617 | `net/http/internal/http2` | HTTP/2 server crash due to an HPACK encoder race |
| GO-2026-6613 | `net/http` | HTTP/1 server desynchronization after a 2xx CONNECT response |
| GO-2026-6612 | `net/http/internal/http2` | double flow-control refund on HTTP/2 server streams |
| GO-2026-6611 | `net/http/internal/http2` | excessive CPU from repeated initial window changes |
| GO-2026-6610 | `net/http/internal/http2` | the HTTP/2 transport accepts malformed framing headers |
| GO-2026-6608 | `mime/multipart` | memory limit bypass when parsing MIME headers |
| GO-2026-6607 | `crypto/tls` | malformed ECH outer extension references are not rejected |
| GO-2026-6605 | `net/http` | HTTP/1 client desynchronization after a CONNECT rejection |
| GO-2026-6604 | `os` (Windows) | `Root.Mkdir(All)` can follow junctions out of the root |
| GO-2026-6603 | `net/http/internal/http2` | HTTP/2 server memory exhaustion through Trailer headers |

**Each is reached through code that existed before 0017.** The traces run
through `GitHubSource`'s HTTP client, `cli.Exit`'s writer, and
`selfupdatetest`'s TLS server. The gate passed earlier the same day, so the
advisories are new, not the code.

**Facts that shape the fix [read, observed 2026-10-08]:**

* **The module:** `go.mod` says `go 1.27.1`, with no `toolchain` line.
  [0001-MADR-scaffold-shared-go-library.md](0001-MADR-scaffold-shared-go-library.md)
  (`:65-70`, `:154`) chose that, following the fleet rule.
* **The fleet rule.** magic-cli-remote's
  `docs/decisions/0169-MADR-standardize-toolchains-on-current-supported-advisory-free-releases.md`
  D1 makes the standard "the newest stable patch of the newest line that
  … has no published advisory affecting it". That is now Go 1.27.2: go.dev
  lists `go1.27.2` first, and `proxy.golang.org` serves its toolchain
  modules for darwin-arm64, linux-amd64 and windows-amd64. 0169 D2 still
  names 1.27.1, and that record is magic-cli-remote's to amend.
* **CI.** `ci.yml`'s `actions/setup-go` steps (pinned to v7.0.0) set up Go
  with `go-version-file: go.mod`. Its README at that pin: "If the
  `toolchain` directive is present, its version is used; otherwise, the
  action falls back to the `go` directive." CI then runs
  `govulncheck ./...`.
* **The workflows callers use.**
  * The build workflow sets up Go from the *caller's* `go.mod`.
  * The publish workflow sets up Go from this module's own `go.mod`, at the
    pinned release (`go-version-file: .core-lib-release-tools/go.mod`), to
    build its release tools.
* **The hosts.** All three pin `GOTOOLCHAIN=go1.27.1` in their Go
  environment file, so a `toolchain` line alone changes nothing there:
  * this Mac: `~/.local/go1.27.1`, behind `~/.local/bin/go`;
  * the Linux test host and the Windows test host: `~/sdk/go1.27.1`.
* **The precedent for hosts.**
  [0007-MADR-adopt-govulncheck-v1-8.md](0007-MADR-adopt-govulncheck-v1-8.md)
  and its PLAN installed a tool on every development host as part of a
  record of this repository.
* **CI's triggers.** `ci.yml` runs on pushes to `main`, on `v*` tags and on
  pull requests (`:2-6`), so a pushed release branch runs nothing.
* **`v1.11.1` is not tagged.**
  * Its release commit, `50eafb6`, passed CI run 37848111118 before the
    advisories were published.
  * `main` has 0017's Q0 and Q1 after it.
  * Its tag's CI would now fail `govulncheck`, and 0017's release
    procedure (step 4) requires that run to pass.

The owner chose, on 2026-10-08, both parts of the outcome below, when
asked during 0017 Q2.

## Decision Drivers

* **Ship nothing on a toolchain with published advisories** (0169 D1).
* **Do not raise consumers' minimum Go for a fix only the toolchain
  needs.** Programs build with their own `go.mod`. This module's code is
  fine on 1.27.1 with a patched toolchain.
* **CI, the release tools and every host gate on the same toolchain.**
* **`v1.11.1` holds what 0017 chose, and only that:** the archive
  refusals, not Q0, Q1 or Q2.
* **Every record and gate stays honest.** Nothing is skipped or loosened
  to get green.

## Considered Options

* **A. A `toolchain go1.27.2` line; `go 1.27.1` stays; hosts move to
  1.27.2; `v1.11.1` from a release branch.**
* **B. Raise the `go` directive to 1.27.2.**
* **C. Change nothing until the fleet rule's own record moves.**
* **D. Skip the `vuln` step until then.**

## Decision Outcome

Chosen option: **"A"**, the owner's choice, because it moves every build
this module controls (CI, the release tools, the hosts' gates) off the
advisories, while consumers keep `go 1.27.1` as their floor.

1. **`go.mod`** gains `toolchain go1.27.2`; the `go` line stays `go
   1.27.1`.
2. **CI** follows through `setup-go`. `ci.yml`'s push trigger gains
   `release/**`, so that a release branch's commit is tested before it is
   tagged.
3. **The hosts:**
   * this Mac, the Linux test host, the Windows test host and that
     host's default WSL distribution get Go 1.27.2 beside 1.27.1;
   * their `go` command and `GOTOOLCHAIN` point at it;
   * `golangci-lint` v2.14.0 and `govulncheck` v1.8.0 are rebuilt with
     it, as 0169 D2 rebuilds the standard tools.

   Nothing is deleted.
4. **`v1.11.1`** is tagged on a `release/v1.11.1` branch cut from `50eafb6`,
   holding one commit more: this record's `go.mod` and `ci.yml` changes.
   `main` gets the same changes. 0017's release procedure applies to that
   commit, not to `50eafb6`.
5. **The fleet rule's record (0169 D2)** is magic-cli-remote's to amend.
   This record only notes it.

### Consequences

* Good, because CI, the release tools and the hosts build and test with a
  standard library that has no published advisory.
* Good, because consumers' minimum stays `go 1.27.1`; a program on 1.27.1
  keeps building.
* Good, because `v1.11.1` still holds only the archive fixes.
* Neutral, because a consumer's own build uses its own toolchain. The fix
  reaches a program's binary when that program moves to 1.27.2 itself.
* Bad, because the hosts change outside the tree, as 0007's did. The
  PLAN records each host's before and after.
* Bad, because a short-lived release branch and a duplicated commit are
  new to this repository's release practice.

### Confirmation

* **Before the change,** `make gate`'s `vuln` step fails on `main` at the
  0017 Q2 work: the red, recorded in 0017's PLAN.
* **After it,** `go version` is `go1.27.2` on every host, and `make gate`
  ends `overall=0` with `vuln rc=0`.
* **CI** on the release branch's commit, then on `main`, passes; its
  setup-go step installs 1.27.2.
* **The `v1.11.1` tag's CI** passes, and its identity legs print
  `v1.11.1 (release) <12-hex>` of the branch commit.

## Pros and Cons of the Options

### A. A `toolchain` line, hosts moved, `v1.11.1` from a branch

* Good, because it fixes every build this module controls, and no
  consumer must move.
* Bad, because it needs host changes and a release branch.

### B. Raise the `go` directive

* Good, because every consumer's build then needs 1.27.2.
* Bad, because it forces every program in the fleet to move before it can
  take any release of this module, and about 77 lines of docs change.

### C. Change nothing

* Bad, because the gate and CI fail on every commit, so neither 0017's
  remaining phases nor any tag could pass their checks.

### D. Skip `vuln`

* Bad, because it loosens a gate to get green, which this repository's
  rules forbid.

## More Information

* `govulncheck ./...` output of 2026-10-08, in 0017 Q2's gate output: the
  ten advisories, "Your code is affected by 10 vulnerabilities from the Go
  standard library".
* [actions/setup-go at v7.0.0](https://github.com/actions/setup-go/blob/b7ad1dad31e06c5925ef5d2fc7ad053ef454303e/README.md):
  the `toolchain` directive.
* [Go downloads](https://go.dev/dl/): `go1.27.2`.
* magic-cli-remote,
  `docs/decisions/0169-MADR-standardize-toolchains-on-current-supported-advisory-free-releases.md`,
  D1, D2, D11 (the standard file) and D12 (the audit).
