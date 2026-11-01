---
status: proposed
date: 2026-11-01
associated-madr: "0040-MADR-close-four-open-items-after-v1-23-1.md"
---
<!-- markdownlint-disable MD004 MD013 MD024 MD060 -->

# Implement D4: record in 0038-PLAN that the host leftovers were removed

Associated MADR: [0040-MADR-close-four-open-items-after-v1-23-1.md](0040-MADR-close-four-open-items-after-v1-23-1.md)
(D4-A, proposed).

## Goal

* **[0038-PLAN-pinned-plans-lifecycle-conformance-and-bubble-tests.md](0038-PLAN-pinned-plans-lifecycle-conformance-and-bubble-tests.md) says what is true of the hosts.** Its "Not done here" bullet says the leftovers "stay on the hosts" (`:920-923`). They were removed on 2026-10-30, after that plan closed (MADR F4.1, F4.2).
* **Its history stays as written.** The bullet is annotated, not rewritten, and the removal is a dated entry after "The plan is complete." (`:927`), the last line of the file.
* **Nothing committed names a host, an account or a real-machine path** (MADR F4.3).

## Scope

### In scope

| Phase | What | Commit |
| :--- | :--- | :--- |
| H0 | the records: the 0040 MADR, its three PLANs and their index rows | records only, shared with the D2 and D3 PLANs |
| H1 | 0038-PLAN: the annotation, the entry and its front-matter `date`; this PLAN's record; its row in `docs/README.md` | records only |

### Out of scope

* **Any host.** Nothing is run on the Windows test host, the WSL, the Linux test host or this Mac. The entry records the run of 2026-10-30 from its scratch record (MADR F4.2). It is not a new inventory.
* **The launchd override entries** of 0038-PLAN Q19 step 3. They stay, as that step says, and the bullet at `:924` is unchanged.
* **Any other line of 0038-PLAN or its MADR.** 0038-PLAN stays `complete`.
* **Pushes,** which happen only on the owner's word, in the same turn.

## Rules

* H1 ends in one commit, `git commit --no-edit`, made by the owner.
* The edit is a Python script under the scratchpad. It asserts that each old string occurs exactly once before replacing it, and that the file ends in "The plan is complete.\n" before appending.
* Check output goes to the scratchpad, with its exit status captured before any filter.
* The entry describes the hosts as "the Windows test host", "the WSL" and "this Mac". It names no host, account, user name, process ID or absolute path, and quotes no command that holds one.
* **Deviations.** A step that finds anything outside its files, or a check that fails, stops. It is recorded here, dated, and in the MADR as an amendment when a fact or decision changes.

## Implementation Steps

### Phase H0: records

1. The 0040 MADR, `proposed`, and three PLANs, this one and [0040-PLAN-redact-env-in-formatted-values.md](0040-PLAN-redact-env-in-formatted-values.md) and [0040-PLAN-test-task-scheduler-grace-timeout.md](0040-PLAN-test-task-scheduler-grace-timeout.md), each `proposed`, with their rows in `docs/README.md` after 0039's. The D1 PLAN is not written yet (MADR, More Information).
2. `make markdownlint`, `make check-records`, `make docs-check` and `scripts/check-docs.sh --ids` on the changed files exit 0.
3. On the owner's decision, the MADR's status follows it. On the owner's approval of this plan, it becomes `in-progress`, and the owner commits the records.

### Phase H1: the entry

Files: `docs/decisions/0038-PLAN-pinned-plans-lifecycle-conformance-and-bubble-tests.md`, this PLAN, and `docs/README.md` (this plan's row).

1. **Baseline.**
   * `git status --short` is clean.
   * `git diff --stat 833f49f HEAD -- docs/decisions/0038-PLAN-pinned-plans-lifecycle-conformance-and-bubble-tests.md` prints nothing, so the file is still as it was committed `complete`.
   * Line 920 is the "Leftovers … stay on the hosts" bullet, and line 927 is "The plan is complete.", the last line.
2. **The annotation.** Line 920 keeps its text and gains, after one space, at its end:

   ```markdown
   *Removed on 2026-10-30, after this plan closed: see "After closing (2026-10-30): the host leftovers removed", below.*
   ```

   Its three sub-bullets (`:921-923`) are unchanged.
3. **The entry.** Appended after "The plan is complete.", with one blank line before it:

   ```markdown
   ### After closing (2026-10-30): the host leftovers removed

   Recorded by [0040-PLAN-record-host-cleanup-in-0038.md](0040-PLAN-record-host-cleanup-in-0038.md), under [0040-MADR-close-four-open-items-after-v1-23-1.md](0040-MADR-close-four-open-items-after-v1-23-1.md) D4-A.

   This plan was committed `complete` as `833f49f`. Later that day, the owner said "proceed to item 1, cleanup.", and the leftovers that "Not done here" lists were removed. This was outside this plan's phases.

   1. **A look first, read-only.**
      * The Windows test host's temporary directory held 509 `selfupdate-artifact-*` directories, none of them holding anything. It also held 34 `sulib-*` copies, 242 MB in all, the newest from 2026-10-24, and the four test directories.
      * The WSL held the one `selfupdate-live-run-*` directory, with a test binary and its run script from 2026-10-04.
      * This Mac's temporary directory held the nine fixture directories.
   2. **The removal,** which stopped on any difference from those counts:
      * on the Windows test host, after checking the counts of 509 and 34 again: the artifact directories with `rmdir`, which removes only an empty directory; then the copies; then the four test directories, by name;
      * in the WSL, after checking its test binary and run script were still there: the directory;
      * on this Mac, after checking the count of nine again: the nine directories.
   3. **After.** Every step exited 0.
      * The Windows test host's temporary directory holds 0 `selfupdate-artifact-*`, 0 `sulib-*` and 0 `selfupdate-*` entries.
      * The WSL holds 0 `selfupdate-live-run-*` directories.
      * This Mac's temporary directory holds 0 `selfupdate-*` entries.

   The launchd override entries of Q19 step 3 stay.
   ```

4. **The front matter.** 0038-PLAN's `date:` moves to the day of this commit. Its `status:` stays `complete`.
5. **The diff is an annotation and an append.** A script reads `git diff -U0` of 0038-PLAN and asserts:
   * the only removed lines are the old `date:` line and line 920;
   * the new line 920 starts with the old line 920's whole text;
   * every other added line comes after line 927.
6. **Checks.** Each exits 0:
   * `make markdownlint`;
   * `make check-records`;
   * `make docs-check`, whose link check covers the entry's two links;
   * `scripts/check-docs.sh --ids` on the changed files, with the deny list. It checks for the local account name, the hostnames and home directory paths.
7. **Seen failing.** This phase adds no test and no gate, so nothing is planted. The `--ids` check is an existing gate.
8. This PLAN's execution record, the plan `complete`, and its row to match. The owner commits. Before the owner is asked to push, the disclosure guard runs over the outgoing commits. The push runs the Docs workflow alone, since only Markdown changes.

## Verification

* **V1 (the correction):** 0038-PLAN's line 920 carries the annotation, and the entry follows "The plan is complete.", by step 5's assertions.
* **V2 (the facts):** each count and state in the entry matches the scratch record of 2026-10-30 (MADR F4.2). These are the 509, 34, 242 MB, four, one, nine, every exit 0, and 0 after.
* **V3 (the identifiers):** `--ids` on the changed files finds 0. The entry holds no hostname, account, user name, process ID or absolute path.
* **V4 (the records):** `make check-records` 0 findings; the Docs run on the push `success`.

## Rollout and Rollback

* **Rollout:** one records commit on `main`, pushed on the owner's word.
* **Rollback:** `git revert` of that commit restores 0038-PLAN as it was. No code, host or release depends on it.

## Execution record

Not started. This plan is `proposed`.
