---
status: proposed
date: 2026-11-02
associated-madr: "0040-MADR-close-four-open-items-after-v1-23-1.md"
---
<!-- markdownlint-disable MD004 MD013 MD024 MD060 -->

# Implement D5: make the architecture table check require a row for every directory Go builds

Associated MADR: [0040-MADR-close-four-open-items-after-v1-23-1.md](0040-MADR-close-four-open-items-after-v1-23-1.md)
(Amendment A1, D5-A, proposed).

## Goal

* **`scripts/check-docs.sh --arch` fails on a directory that holds a `.go` file and has no row** in `docs/architecture.md`'s package table, as well as on a row whose counts differ. Today it compares only the rows the table has (MADR F5.1, F5.2).
* **Directories that `go`'s `./...` leaves out need no row:** a path with a part named `testdata` or `vendor`, or starting with `.` or `_` (F5.3). Today the only such Go directories are the two fixture programs under `internal/cmd/selfupdate-release/testdata/`.
* **The new behaviour is seen failing** in the script's own tests, run against HEAD's script, and its exemption is seen failing with the exemption planted out.
* **No release.** The change is to a build and documentation gate, which no program imports.

## Scope

### In scope

| Phase | What | Commit |
| :--- | :--- | :--- |
| A0 | the records: the MADR's Amendment A1, this PLAN, the D2 PLAN's change to rely on it, and the index row | records only |
| A1 | `scripts/check-docs.sh` (the `--arch` mode and its header), `scripts/check-docs_test.sh`, and the three descriptions of the check: `AGENTS.md`, `scripts/gate.sh`'s comment, `docs/architecture.md`'s CI section; this PLAN's record; its row in `docs/README.md` | one commit; its push runs full CI |

### Out of scope

* **The other modes,** `--links` and `--ids`, and the count comparison itself, which stay as they are.
* **Nested modules and `go.mod` `ignore` directives,** which `./...` also leaves out. The tree has neither outside `testdata/` (F5.3). A directory under one would be reported as missing a row, which fails loudly, not silently. The record that adds one amends the check.
* **A package list from `go list ./...`** (D5-B). The Docs workflow installs no Go (F5.4).
* **`docs/architecture.md`'s table.** Every package already has a row (F5.2), so no row changes.
* **`CHANGELOG.md`.** No released file changes.
* **Pushes,** which happen only on the owner's word, in the same turn.

## Rules

* A1 ends in one commit, `git commit --no-edit`, made by the owner.
* Every edit is a Python script under the scratchpad that asserts each old string occurs exactly once before replacing it.
* Every plant runs on a scratch copy, never in the tree, and the copy is removed after its run.
* Check output goes to the scratchpad, with its exit status captured before any filter.
* Nothing staged names a host, an account or a real-machine path.
* **Deviations.** A step that finds anything outside its files, or a check that fails, stops. It is recorded here, dated, and in the MADR as an amendment when a fact or decision changes.

## Implementation Steps

### Phase A0: records

1. The MADR's Amendment A1, this PLAN `proposed`, the D2 PLAN's change (its step 8 relies on this check, and it runs after this plan), and this plan's row in `docs/README.md`.
2. `make markdownlint`, `make check-records`, `make docs-check`, and `scripts/check-docs.sh --links` and `--ids` on the changed files exit 0.
3. On the owner's decision on D5, the MADR records it. On the owner's approval of this plan, it becomes `in-progress`, and the owner commits the records.

### Phase A1: the check

Files: `scripts/check-docs.sh`, `scripts/check-docs_test.sh`, `AGENTS.md`, `scripts/gate.sh`, `docs/architecture.md`, this PLAN, and `docs/README.md` (this plan's row).

1. **Baseline.**
   * `git status --short` is clean.
   * `git diff --stat 3acd87f HEAD -- scripts/check-docs.sh scripts/check-docs_test.sh` prints nothing, so the scripts are as the pre-check read them.
   * `make docs-check` exits 0, with `20 rows in docs/architecture.md, 0 differ`.
2. **The check.** In `check_arch`, after the loop over the rows, verbatim:

   ```python
       # Every directory Go builds needs a row: one that holds a .go file,
       # unless a part of its path is testdata or vendor, or starts with "."
       # or "_": go's ./... leaves those out
       # (docs/decisions/0040-MADR-close-four-open-items-after-v1-23-1.md D5-A).
       listed = {d for _, d, _ in rows}
       missing = []
       for d, (plain, test) in sorted(counts.items()):
           if d in listed or any(p in ("testdata", "vendor") or p.startswith((".", "_")) for p in d.split("/")):
               continue
           missing.append(f"{f}: {d or '.'}: {plain} non-test and {test} test .go files, and no row in the table")
   ```

   Then the findings print `bad + missing`. The summary becomes `check-docs: <N> rows in <file>, <M> differ, <K> without a row`, and the exit is 1 when `bad or missing`. A broken row still exits 2. `counts` is the existing `tracked_go()`, so an untracked file counts, and an ignored one does not, as for the counts. Nothing parses the summary line (F5.4), and the existing test matches only its first part.
3. **The header.** `--arch`'s paragraph in the script's header gains, after "both counts.":

   ```text
   A directory that holds a .go file and has
   no row prints the file, the directory and its counts,
   unless a part of its path is testdata or vendor, or
   starts with "." or "_", which go's ./... leaves out
   (docs/decisions/0040-MADR-close-four-open-items-after-v1-23-1.md
   D5-A). A missing table or a row for a directory with no
   .go file exits 2.
   ```

   Each line carries the paragraph's `#` and indent.
4. **The tests,** in `scripts/check-docs_test.sh`'s throwaway repository:
   * The fixture gains `pkg/testdata/fix/main.go`, `pkg/_old/x.go`, `.tools/x.go` and `vendor/v/x.go`, which need no row. The comment above the fixture says so.
   * The `good.md` case also requires `0 without a row` in the summary, and its name says that directories `./...` leaves out need no row.
   * A new page, `missing.md`, is `good.md` without its `pkg/sub/` row. A new case requires exit 1, `docs/missing.md: pkg/sub: 2 non-test and 0 test .go files, and no row in the table`, and `1 rows in docs/missing.md, 0 differ, 1 without a row`. `pkg/sub`'s untracked `new.go` is among the 2.
5. **Seen failing.**
   * **T1, the tests against HEAD's script:** `SCRIPT=<a copy of HEAD's scripts/check-docs.sh> sh scripts/check-docs_test.sh` must exit 1, with exactly the `good` and `missing` cases failing, `rc=0` each.
   * **P1, a row removed:** on a scratch copy of the tree, `internal/procfs/`'s row is removed from `docs/architecture.md`. `--arch` must exit 1 with `docs/architecture.md: internal/procfs: 3 non-test and 4 test .go files, and no row in the table`.
   * **P2, D2's case:** on a scratch copy, an untracked `internal/redact/redact.go`. `--arch` must exit 1 with `internal/redact: 1 non-test and 0 test .go files, and no row in the table`, where HEAD's script exits 0.
   * **P3, the exemption planted out:** a copy of the new script with the condition reduced to `if d in listed:`, run as `SCRIPT=` under the new tests. The `good` and `missing` cases must fail, naming `.tools`, `pkg/_old` and `pkg/testdata/fix`.
6. **The descriptions.**
   * **`AGENTS.md`** (`:247-250`): "a change that adds or removes a `.go` file updates that page's package table in the same commit" gains "and a directory that `go`'s `./...` builds needs a row", citing 0040-MADR D5-A beside 0022-MADR D2.
   * **`scripts/gate.sh`** (`:25-27`): "its package table's file counts against the tree's files" gains "and a row for every directory Go builds".
   * **`docs/architecture.md`** (`:856-858`): "the check of this page's package table against the tracked `.go` files" gains "and its directories", citing 0040-MADR D5-A.
7. **Checks.** Each exits 0:
   * `make docs-check`, whose test now reports 20 passed, and whose `--arch` reports `20 rows in docs/architecture.md, 0 differ, 0 without a row`;
   * `make gate`, `overall=0`, its `shellcheck` and `arch` steps among them;
   * `make markdownlint` and `make check-records`;
   * `scripts/check-docs.sh --ids` on the changed files.
8. This PLAN's execution record, the plan `complete`, and its row to match. The owner commits. Before the owner is asked to push, the disclosure guard runs over the outgoing commits. The push runs full CI, since scripts change, and the Docs workflow, since Markdown changes too (`docs.yml`'s `paths`). Every job of both must pass, and each runs the new test through `make docs-check`.

**Pre-check, 2026-11-02.** Steps 2 to 5 were run on a scratch copy of `3acd87f`'s tree, not in the tree (scratchpad script `p40d5.py`), with the code and tests exactly as written above. Results:

* the tests with the new script: `20 passed, 0 failed`;
* T1, the tests with HEAD's script: `18 passed, 2 failed`, the `good` case (`out=[check-docs: 2 rows in docs/good.md, 0 differ]`) and the `missing` case (`rc=0`);
* `--arch` on the real page: `20 rows in docs/architecture.md, 0 differ, 0 without a row`, exit 0;
* `shellcheck` on both scripts: exit 0;
* P1 and P2 exited 1 with the lines in step 5. P2 against HEAD's script exited 0;
* P3 failed the `good` and `missing` cases, naming the three exempt directories.

## Verification

* **V1 (the gap closed):** a Go directory with no row fails `--arch`, by P1 and P2, and by the `missing` case in CI.
* **V2 (no false finding):** the real page passes with `0 without a row`. The `testdata` fixtures, and the exempt fixture directories, need no row.
* **V3 (seen failing):** T1 and P3 fail as step 5 says.
* **V4 (the gates):** `make docs-check` and `make gate` exit 0, and full CI passes on the push.

## Rollout and Rollback

* **Rollout:** one commit on `main`, pushed on the owner's word. From then on, every `make docs-check`, `make gate` and CI run requires the rows.
* **Rollback:** `git revert` of the commit restores the count-only check. Nothing else depends on it, except the D2 PLAN's step 8, which then asserts its row by name again.

## Execution record

### Asked (2026-11-02)

The owner asked to fix the gap that the D2 PLAN's pre-check found ("fix this"). This plan, and the MADR's Amendment A1, are the answer for review. The MADR's D5 is proposed, and nothing is executed.
