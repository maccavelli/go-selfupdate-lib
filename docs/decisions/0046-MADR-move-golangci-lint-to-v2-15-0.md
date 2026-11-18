---
status: accepted
date: 2026-11-18
decision-makers: owner
consulted: "golangci-lint v2.15.0 run on a git archive copy of HEAD for linux, darwin and windows, 2026-11-18"
informed: the fleet's programs, which require this module
---
# Move the golangci-lint pin to v2.15.0, settling the one finding at its source

## Context and Problem Statement

The owner's ruling of 2026-11-18: "standardize on golangci-lint v2.15.0".
Asked how the moves proceed in the repositories still pinned lower, the
owner chose "One short record pair per repository, approved now through CI
(Recommended)". The text below the quoted labels is evidence or the
author's.

[0006-MADR-adopt-golangci-lint-v2-14.md](0006-MADR-adopt-golangci-lint-v2-14.md)
decided the pin at `v2.14.0`. This record supersedes that decision for the
version only; 0006's findings, options and reasoning stand.

The pin appears in `Makefile:44` (the install hint), `scripts/go-precheck.sh:145`
(the install hint), `.github/workflows/ci.yml:207` (the CI install) and
`docs/architecture.md:990` (the CI section). Earlier records and the index
titles of 0006 name `v2.14.0` as history and stay.

v2.15.0 on a `git archive HEAD` copy, with `.golangci.yml`, reports one
finding on each of `linux`, `darwin` and `windows`; v2.14.0 reports none:

```text
selfupdate/service/servicetest/lifecycle.go:272:8: Error return value is not checked (errcheck)
```

`verifier.drain` receives from each channel in `verifier.strays` and drops
the error. A stray is a call (`Stop` or `Start` of the lifecycle under
test) that outlived its bound; the check that started it has already
returned a violation naming it ("did not return within ..."), at every
site that appends to `strays` (`call`, and two branches of
`cancelDuringStop`). `drain` runs after the lab has removed the service,
and only waits, so no call outlives `VerifyLifecycle`. The error such a
call returns late comes from a service pulled out from under it and says
nothing the recorded violation does not.

## Considered Options

* A. Move the pin now and discard the stray's error explicitly, with the
  reason in a comment.
* B. Move the pin and surface the stray's error through `VerifyLifecycle`'s
  result, or log it.
* C. Move the pin and suppress the finding with `//nolint` or a config
  exclusion.
* D. Stay on v2.14.0.

## Decision Outcome

Chosen option: "A", because the stray is already reported as a violation
when it becomes one, so discarding its late error loses no information and
changes nothing a test using `servicetest` sees. B would add a second line
for one fault and change what a consumer's test reports (a behaviour change
in an exported package, with no case that needs it). C hides a finding the
fix settles. D leaves this repository off the fleet standard.

### Consequences

* Good, because the pin agrees across the fleet and the discard is stated.
* Good, because no exported behaviour changes, so the changelog has no
  entry.
* Neutral, because a new test pins that a stray's late error is not
  reported.
* Bad, because a late error from a stray is never shown; the "did not
  return" violation is the only line.

### Confirmation

* v2.15.0 reports the finding on the `git archive HEAD` copy and `0 issues`
  after the change, on all three targets.
* `TestVerifyLifecycleStrayErrorIsNotReported` fails on a scratch copy of
  `drain` that appends the received error.
* `make gate`-class checks, `make check-records` and `make markdownlint` pass.
* This repository holds no committed version-agreement check for the lint
  pin; 0045's was a scratchpad script. None is invented here.

## More Information

* [0006-MADR-adopt-golangci-lint-v2-14.md](0006-MADR-adopt-golangci-lint-v2-14.md)
* [0046-PLAN-move-golangci-lint-to-v2-15-0.md](0046-PLAN-move-golangci-lint-to-v2-15-0.md)
