---
status: complete
date: 2026-10-09
associated-madr: "0019-MADR-apply-go-fix-modernizers.md"
---
# Implement Go 1.27.2's `go fix` modernizers across every target OS

Associated MADR: [0019-MADR-apply-go-fix-modernizers.md](0019-MADR-apply-go-fix-modernizers.md)

## Goal

* **`go fix -diff ./...` is empty** for darwin, linux and windows.
* **The change is `go fix`'s output and nothing else:** the 19 files the
  MADR lists, no exported API change, and every gate green.

## Scope

### In scope

| Phase | What | Commit |
| :--- | :--- | :--- |
| F0 | this pair and its two `docs/README.md` rows | records only |
| F1 | `go fix ./...` for the three OSes, and its checks | code only |
| F2 | this PLAN's execution record and closing | records only |

### Out of scope

* **Hand edits to the code.** If a check fails on `go fix`'s output, the
  phase stops (see Rollout and Rollback); nothing is fixed by hand inside
  F1. *Amended by Deviation D1: one line, `main.go:83`, is written by
  hand.*
* **New lint rules** (for example golangci-lint's `modernize`) to keep the
  idiom. That is a decision of its own.
* **A release, the migration guide and the pins.** No exported API or
  behaviour changes.
* **Push,** which happens only on the owner's ask in the same turn.

## Rules

* **`main`, no branch.** The owner decided on 2026-10-09: "we will not be
  using a branch. we will continue to commit and push to main." Each phase
  commits on `main`, starting from `9dd2da8`. A push still needs the
  owner's ask in the same turn.
* **Commits:** `git commit --no-edit` only. The `prepare-commit-msg` hook
  writes the message.
* **Checks:** `make pre-add-check` on the changed Go files before staging,
  and `make gate` ending `overall=0` before each commit. Long output goes
  to a scratch file, with `$?` captured before any filter.
* **No new test or gate** is added, so there is no "seen failing first"
  experiment. That is stated here, not omitted.

## Implementation Steps

### Phase F0: records

**Files:**
* `docs/decisions/0019-MADR-apply-go-fix-modernizers.md`
* `docs/decisions/0019-PLAN-apply-go-fix-modernizers.md`
* `docs/README.md`

1. `git rev-parse HEAD` is `9dd2da8`, on `main`.
2. `docs/README.md` gains the two 0019 rows after 0018's.
3. On approval, this PLAN becomes `status: in-progress`, with an
   `### Approval` entry quoting the owner's words; the MADR becomes
   `status: accepted`.
4. `scripts/check-docs.sh --links` and `--ids` and `markdownlint-cli2` on
   the three files, then `make gate`; commit.

### Phase F1: `go fix`

1. **Clean start:** `git status --short` is empty.
2. **Apply,** in this order, each with its exit status recorded:

   ```bash
   CGO_ENABLED=0 GOOS=darwin  go fix ./...
   CGO_ENABLED=0 GOOS=linux   go fix ./...
   CGO_ENABLED=0 GOOS=windows go fix ./...
   ```

3. **Nothing left:** `CGO_ENABLED=0 GOOS=<os> go fix -diff ./...` for each
   OS exits 0 with empty output.
4. **The file list:** `git diff --name-only | sort` equals the 19 files in
   the MADR's table, exactly. A file more or less stops the phase.
5. **The diff read in full,** each hunk matched to a row of the MADR's
   table. This includes the `managed_stopped_test.go` hunk whose preview
   header was three lines off: the applied diff must change exactly the
   three literals at the old lines 95, 99 and 226, and remove no other
   line.
6. **`gofmt -l`** on the 19 files is empty.
7. **`make pre-add-check FILES="<the 19 files>"`** exits 0.
8. **`make gate`** ends `overall=0`; the `apicheck` step reports no
   change.
9. **Commit** the 19 files only.

### Phase F2: records

1. This PLAN gains `### Execution record`: each run's exit status, the file
   list, the gate's line per step, and the commit.
2. After the push the owner asks for, CI on the F1 commit is checked, its
   Windows job included.
3. `### Verification, at closing`, `### Closing`, `status: complete`, and
   the index row. Checks as in F0; commit.

## Verification

* **V1:** `go fix -diff ./...` is empty for darwin, linux and windows.
* **V2:** the F1 commit changes exactly the MADR's 19 files.
* **V3:** `make gate` ends `overall=0` at F0, F1 and F2, `apicheck`
  included.
* **V4:** CI passes on the pushed F1 commit, the Windows job included.

## Rollout and Rollback

* **Rollout:** the commits land on `main` and reach `origin` on the
  owner's push. No tag or release follows.
* **A failed check in F1** stops the phase. The agent reports the file,
  the line and the failure, and offers resolutions per the deviation rule.
  The tree is not reverted without the owner's ask.
* **Rollback after merge:** `git revert` of the F1 commit. No data, API or
  release depends on it.

## Execution record

### Approval (2026-10-09)

* The owner asked "run go fix ./... in this repo". The preview and this
  pair followed.
* The owner then decided "we will not be using a branch. we will continue
  to commit and push to main." (see Rules).
* The owner approved execution: "proceed".

### Deviation D1 (2026-10-09): one `go fix` line fails errcheck

* **Found:** after F1 steps 1–6 held (each `go fix` run exited 0, the three
  `-diff` runs were empty, the file list matched the 19 files exactly,
  the diff matched the MADR's table, `gofmt -l` was empty),
  `make pre-add-check` exited 2 and `make gate` ended `overall=1` on `lint`:

  ```text
  internal/cmd/selfupdate-release/main.go:83:5: Error return value of `errors.AsType` is not checked (errcheck)
  	if _, ok := errors.AsType[usageError](err); ok {
  ```

  The same line failed for linux, darwin and windows. Every other gate
  step passed, `apicheck` "compatible with v1.12.1" included. The failure
  is new with `go fix`'s output.
* **Tried on a scratch copy of the tree,** not in the tree: with
  `if errors.As(err, new(usageError)) {` at that line, `go fix -diff` on
  the package was empty, golangci-lint exited 0, both for each of the three
  OSes, and `go test ./internal/cmd/selfupdate-release/` passed.
* **Decision (the owner):** "One-line hand form". MADR Amendment A1
  records it.
* **Scope change:** F1's commit is the 19 files, as before, with that one
  line written by hand after the three `go fix` runs. The F1 rule "nothing
  is fixed by hand inside F1" is amended for this line only. F1 resumes at
  step 3 (each OS's `go fix -diff` empty) and runs every step after it
  again.

### F0 (2026-10-09)

* `HEAD` was `9dd2da8`, on `main`. The hooks path is the global hooks
  directory.
* Links: "86 links in 3 files, 0 broken". Identifiers: "0 findings".
  markdownlint: exit 0. `make gate`: `overall=0`.
* Commit `adb54a8`.

### F1 (2026-10-09)

1. `git status --short` was empty.
2. `go fix ./...` exited 0, with no output, for darwin, linux and
   windows.
3. `go fix -diff ./...` exited 0, with no output, for each OS.
4. `git diff --name-only` equalled the 19 files of the preview, with no
   difference.
5. The diff (19 files, 32 insertions, 39 deletions) was read in full. Each
   hunk matched a row of the MADR's table. The `managed_stopped_test.go`
   hunk's header was `@@ -223,7 +223,7 @@`, so the preview's three-line
   offset was in the preview's display only. The applied diff changes the
   three literals and removes no other line.
6. `gofmt -l` on the 19 files: empty.
7. `make pre-add-check` failed on errcheck: Deviation D1. After its hand
   line, steps 3–6 were run again with the same results, and
   `make pre-add-check` exited 0.
8. `make gate` ended `overall=0`, one line per step:
   * gofmt: clean;
   * lint: "0 issues.";
   * vet, race, shuffle, tidy, fuzz, shellcheck and crossvet: rc=0;
   * apicheck: "compatible with v1.12.1";
   * vuln: "No vulnerabilities found.";
   * scripts: "all script tests passed";
   * links: "406 links in 56 files, 0 broken";
   * ids: "21 files, 16 deny-list rules, 0 findings".
9. Commits: `a849b7a`, D1 and MADR A1; `a94730b`, the 19 files. The
   disclosure guard over the three unpushed commits exited 0. The owner
   pushed them.

### F2 (2026-10-09)

* CI run 37948055929 on `main` at `a94730b` passed all 17 jobs.
* The Windows job, `validate (windows-2025)`, reported
  `ok  github.com/maccavelli/go-selfupdate-lib/internal/cmd/selfupdate-release  142.021s`.
  `installer_ps_test.go` has no skip, so its three tests ran there.

### Verification, at closing (2026-10-09)

* **V1:** `go fix -diff ./...` is empty for darwin, linux and windows
  (F1 step 3, run again after D1).
* **V2:** `a94730b` changes exactly the MADR's 19 files.
* **V3:** `make gate` ended `overall=0` at F0 and F1, and at F2 before
  this commit; `apicheck` was compatible with `v1.12.1` each time.
* **V4:** CI passed on `a94730b`, the Windows job included (F2).

### Closing (2026-10-09)

* V1–V4 hold, with Deviation D1's one hand-written line. This PLAN is
  `complete`, and its row in `docs/README.md` follows. The MADR stays
  `accepted`.
* No release follows (MADR Decision Outcome item 3).
