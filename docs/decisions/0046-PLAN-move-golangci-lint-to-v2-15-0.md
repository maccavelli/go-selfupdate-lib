---
status: in-progress
date: 2026-11-18
associated-madr: "0046-MADR-move-golangci-lint-to-v2-15-0.md"
---
# Implement the golangci-lint v2.15.0 move

Associated MADR: [0046-MADR-move-golangci-lint-to-v2-15-0.md](0046-MADR-move-golangci-lint-to-v2-15-0.md)
(option A, within the owner's ruling of 2026-11-18).

## Goal

Every statement of the lint pin names `v2.15.0`, and `make lint` is clean
on `linux`, `darwin` and `windows` with it, with no `nolint` and no config
exclusion.

## Scope

* `Makefile`, `scripts/go-precheck.sh`, `.github/workflows/ci.yml`,
  `docs/architecture.md`: the pin.
* `selfupdate/service/servicetest/lifecycle.go`: `drain` discards the
  stray's error explicitly, with its reason.
* `selfupdate/service/servicetest/lifecycle_test.go`: one test.
* Not in scope: `docs/README.md` index titles of 0006, other records'
  history, any release or tag.

## Implementation Steps

1. Read the red: v2.15.0 on a `git archive HEAD` copy, three targets.
2. Move the pin in the four places above.
3. Change `drain` to `case late := <-done: _ = late` with the comment (`check-blank` flags `case _ = <-done:`); add the test.
4. See the test fail on a scratch copy whose `drain` appends the error.
5. Gates: gofmt, vet (also `GOOS=windows`), `make lint`, `make
   pre-add-check`, `go mod tidy -diff`, the full tests and `-race` through
   `run-heavy`, `make check-records`, `make markdownlint`, `actionlint`.

## Verification

* Red: one `errcheck` finding at `lifecycle.go:272:8` on each target.
* Green: `0 issues` on each target; the new test passes and fails on the
  plant.

## Rollout and Rollback

Two commits by the coordinator (records, then change). Rollback is a
revert of the change commit; no release or tag.
