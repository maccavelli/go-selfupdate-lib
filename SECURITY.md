# Security policy

## Reporting a vulnerability

Report it privately. GitHub's private vulnerability reporting is not yet
turned on for this repository; until it is, write to the owner through
the contact on their GitHub profile, and once it is, use this repository's
**Security** tab, then **Report a vulnerability**. Please do not open a
public issue or pull request for it. Say what you found, the
release or commit, and how to reproduce it; a fix ships as a new patch
release of the newest minor, with a note in the
[changelog](CHANGELOG.md).

## Supported versions

Only the newest `v1` minor release gets fixes. Within `v1` every exported
change is an addition, so moving to it is a `go get` and the
[migration guide](docs/guides/migrating-from-mcplib-selfupdate.md)'s section
for each release between.

## What an update proves

In short; the [trust model](docs/architecture.md#trust-model) has the
whole of it:

- The release came over TLS from the API origin, is immutable on GitHub,
  and each asset matches its `SHA256SUMS` entry and, when GitHub gives
  one, its digest.
- `SHA256SUMS` comes from the same release as the binary, so anyone who
  can publish a release can publish a matching manifest: protect who may
  create `v*` tags.
- A publisher signature is verified only when the program configures
  `selfupdate/verify/signednote`; build provenance only with
  `selfupdate/verify/ghattest`.
- Freshness is not proven: a stale release served as the latest is
  mitigated only by TLS to the API origin.

## How this repository checks itself

`govulncheck` runs twice in CI, both on Linux: in the `validate` job on
the Go 1.27.3 toolchain the module builds with, and in the `go126` job
on its 1.26.10 floor
([0029-MADR](docs/decisions/0029-MADR-move-toolchain-to-go-1-27-3-and-floor-to-1-26-10.md)). `make gate` runs it once, on the local toolchain,
and also runs the linters, the race and shuffle tests and the fuzz
targets; see [AGENTS.md](AGENTS.md#pre-add-checks).
