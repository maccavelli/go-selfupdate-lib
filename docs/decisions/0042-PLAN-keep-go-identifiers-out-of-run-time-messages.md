---
status: proposed
date: 2026-11-09
associated-madr: "0042-MADR-keep-go-identifiers-out-of-run-time-messages.md"
---
<!-- markdownlint-disable MD004 MD013 MD024 MD060 -->

# Implement the rule that run-time error messages name no Go identifier and cite no record, with four sentinels, two failure classes and a source scan

Associated MADR: [0042-MADR-keep-go-identifiers-out-of-run-time-messages.md](0042-MADR-keep-go-identifiers-out-of-run-time-messages.md)
(D1-A, D2-A, D3-A and D4-A, chosen by the owner on 2026-11-09).

## Goal

* **The MADR's M1 to M10 and M12 read as the MADR's table says,** and none names a Go identifier or cites a record (D1-A).
* **Four sentinels:** `selfupdate.ErrSpecialModeBits`, `selfupdate.ErrNoAllowedRoot`, `service.ErrBinaryMismatch` and `taskscheduler.ErrLogonType`. Each is wrapped by the messages the table names (D2-A).
* **Two failure classes:** `special-mode-bits` and `no-allowed-root`, after `symlink-invocation` (D3-A).
* **`TestRunTimeMessagesNameNoGoIdentifier` holds the rule.** It is seen to fail on the tree before this PLAN's message changes, and on three planted copies (D4-A).
* **One `v1.26.0` carries this change and 0041's Amendment A1,** as the owner chose on 2026-11-09 (MADR More Information). Phases B3a and B3b here make it, in place of [0041-PLAN-type-the-symlink-refusal-and-spare-startup-maintenance.md](0041-PLAN-type-the-symlink-refusal-and-spare-startup-maintenance.md) Phases A3a and A3b.

## Scope

### Preconditions

* [0041-PLAN-type-the-symlink-refusal-and-spare-startup-maintenance.md](0041-PLAN-type-the-symlink-refusal-and-spare-startup-maintenance.md) Phase A1 is committed. Then M11, the symlink refusal, already names no Go identifier, and the scan's first run on the tree sees only this PLAN's messages.
* The owner has accepted the MADR and approved this PLAN.

### In scope

| Phase | What | Commit |
| :--- | :--- | :--- |
| B0 | this PLAN and its MADR, in the same records commit as 0041's Amendment A1 (that PLAN's Phase A0) | records only |
| B1 | the sentinels, the messages, the classes, the godoc, the tests, the scan, `AGENTS.md`, `docs/architecture.md`, `docs/guides/extending-selfupdate.md`, this PLAN's record | one commit |
| B2 | live runs and planted failures; this PLAN's record | records only |
| B3a | `CHANGELOG.md` `v1.26.0` and the migration guide's §26, for both PLANs; both PLANs' records | the commit the owner tags |
| B3b | the pins and the release-history line, after the tag's CI is green; this PLAN and 0041's second PLAN set to `complete` | records and guides |

Files that Phase B1 changes:

* **The core package.**
  * `selfupdate/errors.go`: the two sentinels.
  * `selfupdate/target.go`: M1 (`:47-48`) and M2 (`:180`).
  * `selfupdate/updater.go`: the two classes.
  * `selfupdate/types.go`: the comments of `AllowedRoots` and `AllowSpecialModeBits`.
* **The service packages.**
  * `selfupdate/service/errors.go`: `ErrBinaryMismatch`.
  * `selfupdate/service/supervised_lifecycle.go`: M7 (`:306`), and `Reconcile`'s comment.
  * The four backends' reconcilers, for M3 to M6: `selfupdate/service/launchd/reconcile.go`, `systemd/reconcile.go`, `scm/reconcile.go` and `taskscheduler/reconcile.go`.
  * Their `RewritePath` comments: `launchd/job.go`, `systemd/unit.go`, `scm/service.go` and `taskscheduler/task.go`.
  * `taskscheduler`:
    * `task.go`: `ErrLogonType`, and `HopDetach`'s comment;
    * `handoff.go`: M8;
    * `reconcile.go`: M9;
    * `lifecycle.go`: M10;
    * `register.go`: M12.
* **Tests.**
  * `selfupdate/modebits_unix_test.go`, `selfupdate/install_regression_test.go` and `selfupdate/events_test.go`.
  * `selfupdate/service/launchd/launchd_test.go`, `scm/scm_test.go` and `systemd/reconcile_test.go`.
  * `taskscheduler/reconcile_test.go`, `handoff_test.go`, `lifecycle_test.go` and `register_test.go`.
  * `selfupdate/service/supervised_health_test.go`.
  * `internal/testproc/testproc_test.go`.
* **Docs.** `AGENTS.md`, `docs/architecture.md` and `docs/guides/extending-selfupdate.md`.

No `.go` file is added or removed, so `docs/architecture.md`'s package table keeps its counts.

### Out of scope

* The 23 configuration messages (MADR F1), which keep naming their fields.
* The hints in CLI terms (F7), the systemd drop-in's comment line (F6), `internal/cmd/`, `selfupdatetest` and `servicetest` (MADR More Information).
* Messages built outside `fmt.Errorf` and `errors.New` (MADR Consequences).
* A failure class for `ErrBinaryMismatch` or `ErrLogonType` (D3-A).
* Pushes, tags and closing issue #10, which happen only on the owner's word in the same turn.

## Rules

* Mutation starts only after the owner accepts the MADR and approves this PLAN, and after the precondition holds.
* The owner commits and pushes. The agent runs the checks and leaves the tree.
* A deviation stops the work: it is recorded here, dated, and the owner decides before it goes on.
* Nothing committed names a host, an account or a real-machine path.

## Implementation Steps

### Phase B0: the records

1. This PLAN and its MADR, with their rows in `docs/README.md`, in the records commit of 0041's second PLAN's Phase A0.

### Phase B1: the rule, the sentinels and the scan

1. **Write the scan first, and see it fail on the tree.** In `internal/testproc/testproc_test.go`, after `TestNoDirectFixtureBuild`, add `TestRunTimeMessagesNameNoGoIdentifier`:
   * **Which files.** It walks `../..` as `TestNoDirectFixtureBuild` does, and skips `.git` and `testdata`. It reads every `.go` file that is not a test, except those under `internal/cmd/`, `internal/testproc/`, `selfupdate/selfupdatetest/` and `selfupdate/service/servicetest/`.
   * **What it matches.** For each call to `fmt.Errorf` or `errors.New`, it joins the call's string literals, unquoted, and matches MADR F1's two patterns.
   * **When it fails.**
     * A citation fails at once.
     * An identifier fails unless the file's slash path and the identifier are in `allowedIdentifiers`. That allowlist is a `map[string][]string` of MADR F1's 23 configuration matches, each with a comment naming the option it validates.
     * An allowlist entry with no match fails.
     * Fewer than 100 files, or 500 calls, read fails. That catches a wrong root. MADR F1 read 136 and 689.
   * **What a failure says.** `path:line: <identifier>` and the message, and the rule with its record (`0042-MADR D1-A`).
   * **Seeing it fail.** Run it on the tree before steps 2 to 10. It must fail with exactly M1 to M10's and M12's sites, which is 11. M11 is gone by the precondition. Record the output.
2. **`selfupdate/errors.go`.** Add the sentinels after `ErrSymlinkInvocation`. Each comment says when the sentinel is returned and gives the program's remedy:
   * `ErrSpecialModeBits = errors.New("selfupdate: refusing to replace a setuid or setgid target")`. Its remedy is `TargetPolicy.AllowSpecialModeBits`.
   * `ErrNoAllowedRoot = errors.New("selfupdate: no allowed self-update root")`. Its remedy is an `AllowedRoots` entry that covers the target.
3. **`selfupdate/target.go`.**
   * M1: `fmt.Errorf("%w: %s", ErrSpecialModeBits, resolved)`.
   * M2: `fmt.Errorf("%w: the home directory is unavailable", ErrNoAllowedRoot)`.
   * A comment above each cites `0042-MADR D1-A, D2-A`.
4. **`selfupdate/updater.go`.** Add `{ErrSpecialModeBits, "special-mode-bits"}` and `{ErrNoAllowedRoot, "no-allowed-root"}` after the `symlink-invocation` row.
5. **`selfupdate/service/errors.go`.** Add `ErrBinaryMismatch = errors.New("selfupdate: service: binary mismatch")`. Its comment says that the service's definition, or the process it ran, names another binary than the one being updated. The remedy is `RewritePath` on the backend's `Options`, or `NewExecReconciler` for `Supervised`.
6. **The reconcilers.**
   * M3 to M6 each become `fmt.Errorf("selfupdate: <backend>: %s runs %s, not %s: %w", …, service.ErrBinaryMismatch)`, keeping each backend's arguments.
   * M7 becomes `fmt.Errorf("the service ran %s, not %s: %w", ran, executable, ErrBinaryMismatch)`.
   * Each `RewritePath` comment, and `Supervised.Reconcile`'s comment (`:284-290`), names `ErrBinaryMismatch`.
7. **`taskscheduler`.**
   * `task.go`: add `ErrLogonType = errors.New("selfupdate: taskscheduler: unsupported logon type")` after `ErrTaskDisabled`. Its comment names `HopDetach` for the handoff, and says that a rewrite needs an interactive-token principal. Its citations are `0032-MADR D8-A` and `0021-MADR Amendment A1, Q6`. `HopDetach`'s comment says that a non-interactive principal refuses the one-shot handoff, with `ErrLogonType`.
   * M8 and M9: the MADR's texts, ending `: %w` with `ErrLogonType`. Each citation moves to a comment above the site.
   * M10: `fmt.Errorf("selfupdate: taskscheduler: the program's stop request: %w", err)`, and `requestError`'s comment keeps `Options.RequestStop`.
   * M12: drop ` (0032-MADR Amendment A1, P1-2)` from the text, and put it in a comment above.
8. **`selfupdate/types.go`.**
   * `AllowedRoots`'s comment says that with no usable home directory, an entry must cover the target, or the update fails with `ErrNoAllowedRoot`.
   * `AllowSpecialModeBits`'s comment names `ErrSpecialModeBits`.
9. **Tests.**
   * Each test of MADR F8 checks `errors.Is` with its sentinel and the exact text, in place of `strings.Contains` of an identifier:
     * `modebits_unix_test.go:48`: M1;
     * `launchd_test.go:533`, `scm_test.go:576`, `systemd/reconcile_test.go:34` and `taskscheduler/reconcile_test.go:38`: M3 to M6;
     * `supervised_health_test.go:142`: M7;
     * `handoff_test.go:190`: M8.
   * New assertions:
     * `TestHomeUnavailableRoot` (`install_regression_test.go:106`): M2's `errors.Is` and text;
     * `taskscheduler/reconcile_test.go`: M9, with `RewritePath` set and the fake's `logon` set to `logonS4U`, as `handoff_test.go:189` does;
     * `taskscheduler/lifecycle_test.go`: `requestError` gives M10's text and wraps its argument;
     * `register_test.go`: the S4U case's text has no citation.
   * `selfupdate/events_test.go`'s `TestFailureClass` gains two rows, one for each new class.
10. **Docs.**
    * `AGENTS.md`, after `:271`: one paragraph. A run-time error message names no Go identifier and cites no record. A configuration message may name the field. `TestRunTimeMessagesNameNoGoIdentifier` holds this, with its allowlist (`0042-MADR`).
    * `docs/architecture.md`:
      * the `failed` event's class list (`:483-487`) gains the two classes after `symlink-invocation`;
      * `:433-434` names the new scan beside `TestNoDirectFixtureBuild`;
      * the install path's setuid and allowed-root bullets name their sentinels;
      * the service section names `ErrBinaryMismatch`.
    * `docs/guides/extending-selfupdate.md`:
      * "Read JSON output" (`:171-174`): the two classes;
      * "Replace a setuid or setgid binary" (`:588-601`): `ErrSpecialModeBits`;
      * "Run as a service" (`:741-744`, `:849-851` and `:984-987`): `ErrBinaryMismatch` and `ErrLogonType`.
11. **Checks.**
    * `make pre-add-check FILES="<every changed .go file>"`.
    * `make gate`, ending `overall=0`.
    * `make markdownlint`, `make check-records`, `make docs-check` and `make disclosure-check`.
    * `make apicheck` lists exactly the four sentinels as additions, all compatible.
    * The record goes in this PLAN, in the same commit. The owner commits.

### Phase B2: live runs and planted failures

1. Run each package's tests on its host, on a bundle of the Phase B1 tree:
   * `go test -count=1 ./selfupdate/ ./selfupdate/service/... ./internal/testproc/` on the macOS development host and the Linux test host;
   * `go test -count=1 ./selfupdate/service/scm/ ./selfupdate/service/taskscheduler/ ./selfupdate/ ./internal/testproc/` on the Windows test host.

   Record each result. `systemd`'s and `launchd`'s fake-based tests run on every host, and each live test keeps its own switch.
2. With `scripts/plant-copy.sh`, make three copies, each with one change, and run `go test -count=1 -run TestRunTimeMessagesNameNoGoIdentifier ./internal/testproc/` in each:
   * M5's text given `; Options.RewritePath allows rewriting it` back: fails, naming `scm/reconcile.go` and `Options.RewritePath`;
   * M12's text given its citation back: fails, naming the citation;
   * one allowlist entry's message changed so it no longer names its field: fails, naming the stale entry.

   Record each failure's output.
3. Record every result in this PLAN, with output. No commit but the record.

### Phase B3a: prepare `v1.26.0`, for both PLANs

This phase replaces 0041's second PLAN's Phase A3a (owner, 2026-11-09). It runs once that PLAN's Phases A1 and A2 and this PLAN's Phases B1 and B2 are recorded.

1. `CHANGELOG.md`: one `## v1.26.0 - <date>` entry.
   * **Added:**
     * `SymlinkInvocationError`, `SymlinkSource` and its three constants (0041 Amendment A1 D3-A);
     * the four sentinels, `ErrSpecialModeBits`, `ErrNoAllowedRoot`, `service.ErrBinaryMismatch` and `taskscheduler.ErrLogonType`;
     * the classes `special-mode-bits` and `no-allowed-root`.
   * **Changed:**
     * the symlink refusal's text, by source (D4-A);
     * the nine texts of this MADR's table, M8's and M9's shape among them;
     * the two target refusals' class, from `error` to their new classes.
   * **Fixed:** issue #10.
     * `CleanupPending` and `KeptBackups` no longer fail for a program started through a link (D5-A). That failure was new on Linux in `v1.25.0`.
     * Run-time messages name no Go identifier.

   It links §26, 0041-MADR's Amendment A1, this MADR, and both PLANs.
2. `docs/guides/migrating-from-mcplib-selfupdate.md`: `## 26. From v1.25 to v1.26`, in §25's form, with "What is new", "Behaviour you may notice", "Adopting it" and "Check". It covers both changes. It lists each changed text, old and new, with the sentinel to match. It also says what `CleanupPending` and `KeptBackups` now do through a link.
3. `scripts/check-release-tag.sh v1.26.0` exits 0, and `git tag -l v1.26.0` prints nothing. Run the Markdown checks. Both PLANs' records gain this phase's results. The owner commits, pushes, tags `v1.26.0` and pushes the tag.

### Phase B3b: the pins, after the tag's CI is green

This phase replaces 0041's second PLAN's Phase A3b.

1. Do that phase's steps 1 to 3 for `v1.26.0`:
   * check that `.github`, `internal/cmd` and `scripts/*release*` are unchanged since `v1.25.0`;
   * run the asserted pin script over `README.md`, `docs/architecture.md`, `docs/guides/building-releases.md` and the migration guide;
   * add the `v1.26.0` release-history line;
   * run the checks.
2. Set this PLAN and 0041-PLAN-type-the-symlink-refusal-and-spare-startup-maintenance.md to `complete`, with their rows in `docs/README.md`, in the same commit. The owner commits and pushes.
3. Closing issue #10, with one comment for both PLANs, is the owner's, or the agent's when asked in that turn.

## Verification

* `TestRunTimeMessagesNameNoGoIdentifier` passes on the changed tree, and fails before it (Phase B1 step 1) and on each planted copy (Phase B2 step 2).
* Each M site's test checks its sentinel with `errors.Is` and its exact text, and passes on each host that runs it (Phase B2 step 1).
* `TestFailureClass` covers the two classes.
* `make gate` ends `overall=0`. `make apicheck` reports only the four additions.
* CI, after the owner pushes: the Linux, macOS, Windows and `go126` jobs pass, and the tag's run passes.

## Rollout and Rollback

* **Rollout** is `v1.26.0`, in Phase B3, with 0041's Amendment A1.
  * A program gets the new texts, the sentinels and the classes when it moves to that version.
  * A program that matched an old text loses the match, and the migration guide names the sentinel to match instead. No fleet checkout on the development host matches one (MADR F10).
* **Rollback, before the tag:** revert Phase B1's commit.
* **Rollback, after the tag:** a `v1.26.1` that restores the old texts and the `error` class. It keeps the four sentinels and wraps them, since an exported name stays within `v1`. It can be done apart from 0041's rollback, since the two PLANs' commits are separate.

## Execution Record

Not started. The owner chose the MADR's options on 2026-11-09. They have not yet approved this PLAN for execution, and the precondition does not hold yet.

### Owner decisions before execution

* **2026-11-09.** Three options were put to the owner for each decision, with the recommended one first. They chose D1-A, D2-A, D3-A and D4-A, as proposed. D3-C, classes for all four sentinels, was added to the MADR as the third option put to them.
* **2026-11-09, the release.** The owner chose one `v1.26.0` for both PLANs, over the recommended `v1.26.0` and then `v1.27.0`. Phases B3a and B3b were rewritten for it, before any execution, and now replace 0041's second PLAN's Phases A3a and A3b.

### Deviations

None.
