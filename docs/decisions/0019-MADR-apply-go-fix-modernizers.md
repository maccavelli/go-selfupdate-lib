---
status: accepted
date: 2026-10-09
decision-makers: go-selfupdate-lib maintainers
consulted: "`go fix -diff ./...` with Go 1.27.2, for GOOS darwin, linux and windows, of 2026-10-09"
informed: the fleet's programs, which take this module's releases
---
# Apply Go 1.27.2's `go fix` modernizers across every target OS, as one behaviour-preserving change

## Context and Problem Statement

The owner asked, on 2026-10-09, to run `go fix ./...` in this repository.
Since Go 1.26, `go fix` applies the "modernizer" analyzers: rewrites to
newer standard-library and language forms that keep behaviour. This record
decides whether to apply them, and how to cover the code that only one
target OS builds.

**Facts [observed 2026-10-09, `main` at `9dd2da8`, Go 1.27.2]:**

* **`go.mod`** has `go 1.27.1` and `toolchain go1.27.2`
  ([0018-MADR-move-toolchain-to-go-1-27-2.md](0018-MADR-move-toolchain-to-go-1-27-2.md)).
  Every form below is valid at language version 1.27.
* **`go fix` sees only one build configuration per run.** The tree has 55
  `//go:build` lines, every one an OS constraint (`windows` 20, `unix` 17,
  `darwin` 4, `linux` 3, and 11 negations), and no other build tag.
* **The preview,** `CGO_ENABLED=0 GOOS=<os> go fix -diff ./...`, wrote
  nothing and exited 1 with a diff for each OS: 277 lines for darwin, 266
  for linux, 277 for windows. Their union is 19 files:

  | Rewrite | Files |
  | :--- | :--- |
  | `strings.Split` / `bytes.Split` / `strings.Fields` in a `range` → `SplitSeq` / `FieldsSeq` | `selfupdate/archive/unpack.go`, `selfupdate/releasespec/validate.go`, `internal/cmd/selfupdate-release/build.go` (2), `render.go`, and the tests `live_darwin_test.go`, `manifest_differential_test.go`, `installer_ps_test.go`, `stage_test.go`, `usage_test.go` |
  | `var x *T; errors.As(err, &x)` → `errors.AsType[*T](err)` | `selfupdate/checkcache.go`, `selfupdate/github.go`, `selfupdate/service/runner.go`, `internal/cmd/selfupdate-release/main.go` |
  | a counted loop whose index is unused → `for range n` | `selfupdate/lock.go`, `selfupdate/credentials_test.go` (3) |
  | a reverse index loop → `slices.Backward` | `selfupdate/service/scm/lifecycle.go` |
  | `wg.Add(1); go func(){ defer wg.Done() … }()` → `wg.Go` | `selfupdate/credentials_test.go` |
  | a search loop → `slices.Contains` | `internal/cmd/selfupdate-release/helpers_test.go` |
  | `for i := range t.NumField()` → `for field := range t.Fields()` | `internal/cmd/selfupdate-release/plan_test.go` |
  | a nested literal of an embedded struct → its promoted fields | `selfupdate/managed_stopped_test.go` (3) |

* **Coverage by OS:** 17 files appear in all three runs.
  `installer_ps_test.go` appears only in the windows run, and
  `live_darwin_test.go` only in the darwin run. A single host-OS run would
  leave the Windows file unmodernized.
* **No exported identifier changes.** Every rewrite is inside a function
  body or a test.
* **One preview oddity:** the second hunk of `managed_stopped_test.go` has
  the header `@@ -223,7 +220,7 @@`, three lines off, although no hunk
  removes a line. The applied diff, not the preview, is what gets checked.

## Decision Drivers

* **No behaviour change.** This is a library that replaces running
  binaries. A rewrite may change form, not what the code does.
* **One idiom across the tree.** Code built only on Windows or macOS
  should read like the rest.
* **The gates stay as they are.** Nothing is skipped or loosened to land
  the change.
* **Reviewable.** Mechanical changes stay apart from any hand change.

## Considered Options

* **A. Apply `go fix ./...` for darwin, linux and windows, and land only
  its output.**
* **B. Run `go fix ./...` once, on the host OS (darwin).**
* **C. Apply a subset, for example leaving the test files alone.**
* **D. Do not apply it.**

## Decision Outcome

Chosen option: **"A"**, approved by the owner on 2026-10-09, because it gives one idiom across
every file the module builds, and every rewrite is a behaviour-preserving
form valid at the module's language version.

1. `CGO_ENABLED=0 GOOS=<os> go fix ./...` runs for darwin, linux and
   windows, in that order, ~~and nothing else edits the code~~ and one
   hand-written line follows it (Amendment A1).
2. The result lands as one commit, apart from this record's commit.
3. No release follows from this record. The change reaches consumers with
   the next release that has its own reason.

### Consequences

* Good, because every file reads in the Go 1.26–1.27 idiom, whichever OS
  builds it.
* Good, because `errors.AsType` and the `Seq` forms drop a variable or a
  slice each, and `wg.Go` removes a `defer wg.Done()` that could be
  forgotten.
* Neutral, because `SplitSeq` and `FieldsSeq` no longer build a slice; no
  rewritten loop indexes or keeps the slice, so the result is the same.
* Neutral, because the promoted-field literals in `managed_stopped_test.go`
  depend on language version 1.27, which `go.mod` already requires.
* Bad, because `git blame` points at this commit for 19 files' lines.

### Confirmation

* After the change, `go fix -diff ./...` for each of the three OSes exits
  0 with no output.
* `git diff --stat` lists exactly the 19 files above, and nothing else.
* `make gate` ends `overall=0`; its `apicheck` step reports no API change.
* CI passes on the pushed commit, its Windows job included, since it is
  the only place `installer_ps_test.go` runs in CI.

## Pros and Cons of the Options

### A. Every OS

* Good, because no file is left on the old idiom.
* Bad, because it takes three runs, not one.

### B. The host OS only

* Good, because it is the literal command.
* Bad, because `installer_ps_test.go` stays unmodernized, and a later run
  on Windows changes it anyway.

### C. A subset

* Good, because it narrows the diff.
* Bad, because the test changes are as safe as the rest, and a partial run
  needs hand selection, which turns a mechanical change into a manual one.

### D. Leave it

* Good, because nothing changes.
* Bad, because the gap grows with every Go release, and the owner asked for
  the change.

## Amendments

### A1 (2026-10-09): one line `go fix` writes fails errcheck

* **Found in PLAN F1:** `go fix` rewrote
  `internal/cmd/selfupdate-release/main.go:83` to
  `if _, ok := errors.AsType[usageError](err); ok {`. Lint failed on it
  for linux, darwin and windows: "Error return value of `errors.AsType`
  is not checked (errcheck)". `.golangci.yml:216` sets errcheck's
  `check-blank: true`, and `usageError` implements `error`, so the blank
  first result counts as an unchecked error. The old two-line form passed.
* **Decision (the owner, 2026-10-09, "One-line hand form"):** that line
  is written by hand as `if errors.As(err, new(usageError)) {`. It has the
  same behaviour, `go fix` does not propose a change to it, and errcheck
  accepts it.
* **What this changes:** "nothing else edits the code" in Decision
  Outcome item 1 becomes "one hand-written line follows it". The 19 files
  and every Confirmation item stay as they were.
* **Not chosen:** keeping `main.go` unchanged (V1 would fail on it), and
  exempting `errors.AsType` from errcheck (a gate change for one line).

## More Information

* `go help fix`, and the `golang.org/x/tools/go/analysis/passes/modernize`
  analyzers it runs.
* The preview diffs of 2026-10-09 are not committed. The PLAN records the
  applied diff's file list.
