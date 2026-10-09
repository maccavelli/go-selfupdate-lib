---
status: in-progress
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
  F1.
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
