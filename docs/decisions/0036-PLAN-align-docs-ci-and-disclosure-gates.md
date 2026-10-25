---
status: in-progress
date: 2026-10-25
associated-madr: "0036-MADR-align-docs-ci-and-disclosure-gates.md"
---
# Implement aligned docs-only CI and disclosure gates

Associated MADR: [0036-MADR-align-docs-ci-and-disclosure-gates.md](0036-MADR-align-docs-ci-and-disclosure-gates.md)

## Goal

A documentation-only push or pull request gets a Docs run with the common Markdown, record/link and portable disclosure gates plus this repository's Markdown-reading checks. Other changes retain full CI. The private pre-push guard remains part of publication.

## Scope

Affected files: `.github/workflows/ci.yml`, `.github/workflows/docs.yml`, `Makefile`, `scripts/check_records.py` (new), `scripts/check_records_test.py` (new), `scripts/workflow-shape_test.sh`, `scripts/check-docs.sh`, `scripts/check-docs_test.sh`, `AGENTS.md`, `docs/architecture.md`, `docs/README.md`; and, for G3, `selfupdate/cleanup_test.go`, `selfupdate/service/managerenv_test.go` and `docs/decisions/0003-PLAN-remediate-debugging-pass-findings.md`.

Repository-specific work: Retain both workflows' current Markdownlint, link and architecture commands and release-branch triggers. Add a numbered-record/pair/index target and run it in both workflows. Run `scripts/check-docs.sh --ids-optional` on every tracked text file in both workflows as the public home-path disclosure baseline, and keep `--ids` plus the global pre-push guard locally for the private deny list. Add a workflow contract test to `scripts/workflow-shape_test.sh` for trigger symmetry and docs gate parity; update `AGENTS.md` and `docs/architecture.md`.

Boundary: The owner deny list cannot be reproduced on an untrusted pull-request runner. Preserve `--ids` as the local pre-push check and explain the narrower CI baseline explicitly.

## Implementation Steps

### Phase 1: inventory and shared checks

1. Re-read workflow triggers, job commands, Markdownlint globs, tracked Markdown files and every code or script reader of tracked Markdown. Record the docs-safe path set in this PLAN before editing filters. Include hidden agent-rule trees and test fixtures. Review required-status settings; if a filtered job would stay pending, amend this pair with a concrete migration before changing filters.
2. Add four `Makefile` targets, each wrapping today's command where one exists: `check-records` (`python3 scripts/check_records.py`); `markdownlint` (`npx --yes markdownlint-cli2@0.23.2` over the tracked Markdown, as `docs.yml` runs it); `docs-check` (`scripts/check-docs.sh --links` over the tracked Markdown, `--arch docs/architecture.md`, and `scripts/check-docs_test.sh`); and `disclosure-check` (`scripts/check-docs.sh --ids --ids-optional` over `git ls-files`). Both workflows call these targets, so full CI, Docs and a local run share one command and one pin. Keep Go lint, tests, vulnerability, builds and releases in full CI. *Amended 2026-10-25 (MADR Amendment A1, G1): the step named record/link and Markdownlint targets only, while phase 3, step 2 runs all four.*
3. Extend `scripts/check-docs.sh --ids --ids-optional`, the existing standard-library home-path check, rather than adding a second scanner. Add a Windows user-home rule (`C:\Users\<user>` and `C:/Users/<user>` as the placeholder forms, either drive letter case) beside the macOS and Linux rule, accepting the `<user>` placeholder alone. Make an unreadable or missing tracked file a reported failure, not a traceback. Keep the finding format (file, line, category; never the match). Add cases to `scripts/check-docs_test.sh`. Run `make disclosure-check` in full CI and Docs; `make gate`'s ids step keeps `--ids` with the private deny list. Retain the global pre-push guard locally. *Amended 2026-10-25 (MADR Amendment A1, G2): `--ids-optional` already checks macOS and Linux home paths without the private list, but not Windows.*
4. Before the gate runs in CI, clear the four existing findings: `selfupdate/cleanup_test.go:22` and `:32` and `selfupdate/service/managerenv_test.go:34` use a made-up Linux home directory for an account named `u`, and `docs/decisions/0003-PLAN-remediate-debugging-pass-findings.md:932` quotes the first. Rewrite each to `/home/<user>/…`; annotate the 0003-PLAN line in place as changed by this plan. Both tests compare strings only; re-run them on this host and with `GOOS=windows` `go vet`, and run `make pre-add-check` on the two Go files. Record the scan at 4 findings before and 0 after. *Amended 2026-10-25 (MADR Amendment A1, G3): a scan of all 697 tracked files on 2026-10-25 found these four, all made-up fixtures.* *Deviation D1, 2026-10-25: the Windows rule of step 3 finds eight more; see the execution record.*
5. Add fixture tests for new gates. Prove each fails on deliberately broken input in a scratch copy and passes on clean input. Record commands, statuses and redacted diagnostics. Commit the approved phase after checks pass.

### Phase 2: workflow routing and parity

1. Keep push and pull-request CI ignore sets identical and limited to audited docs-safe paths. Make Docs paths cover every ignored path and the Docs workflow itself. Mixed changes run full CI. Preserve tags, schedules, dispatch, release jobs and permissions.
2. Add or extend a Python workflow contract check. It validates actual YAML structure, compares event filters, confirms ignored tracked paths are in Docs, refuses known Markdown inputs to code tests unless their reader runs in Docs, and compares common docs-gate commands and pins, and fails when full CI loses a tag, schedule or dispatch trigger it had, or gains a `paths` filter. Integrate it in the workflow/tool test target.
3. Run actionlint and the contract check. In independent scratch copies, plant a mismatched path, a Markdown reader wrongly ignored, a missing Docs gate and a differing pin; require each to fail. Commit the approved phase after checks pass.

### Phase 3: documentation and observed rollout

1. Explain routing, docs-safe paths, common checks, local readers and the private pre-push guard in AGENTS.md and the architecture or CI guide. Verify this pair's exact filenames and current statuses in the records index; use the repository index generator if present.
2. Run `make check-records markdownlint docs-check disclosure-check`; `./scripts/check-docs_test.sh`; `./scripts/workflow-shape_test.sh`; actionlint; scratch plants for a bad record pair/index, link, architecture count, MD018, macOS or Linux home path, Windows home path and docs/full gate mismatch. Capture status before filtering output. Check this pair and edited documents for real-machine identifiers. Read each edited file in full and verify links.
3. After explicit authorization to push in that same turn, observe a docs-only push and a non-doc or mixed push. Tag, schedule and dispatch routing is not waited for: GitHub evaluates no path filter for a tag push, `schedule` and `workflow_dispatch` carry none, and the contract check of phase 2, step 2 fails if a trigger is lost or a `paths` filter is added. Record run IDs, conclusions and which workflow started. Mark this PLAN complete only after the criteria below hold; otherwise record the outstanding observation. Commit at the end of each approved phase under repository rules. *Amended 2026-10-25: the wait for the next tag or scheduled run is removed, as acp-go-sdk2's 0037-PLAN-align-docs-ci-and-disclosure-gates.md did in its step 3.*

## Verification

* Both workflows pass pinned Markdownlint, numbered-record/index and relative-link validation, and portable tracked-text disclosure scanning, each through its `make` target.
* `make disclosure-check` over every tracked file reports 0 findings, after the 4 found before phase 1, and a planted Windows home path fails it.
* Each Markdown reader runs on changes to its input, either in Docs or full CI.
* A docs-only push starts Docs but not full CI; a non-doc or mixed push starts full CI. Tags, schedules and dispatch retain existing behavior, shown by the contract check and the workflows' structure, not by waiting for a run.
* Scratch plants fail their intended gates; filter or gate drift fails the parity test.
* Before an authorized push, the local pre-push guard checks outgoing commits. Its private deny list is never copied into CI.

## Rollout and Rollback

Land gates before relying on the filters. If routing misclassifies a change, revert the routing commit through a new reviewed commit so full CI runs for every push again. Keep the shared gates while repairing the audit. This PLAN does not authorize a push or tag.

## Execution record

### Decisions before approval (2026-10-25)

A read-only review found four gaps, which the owner decided, each on the recommended option; MADR Amendment A1 records them. G1: all four `make` targets in phase 1, called by both workflows. G2: extend `--ids-optional` with a Windows rule. G3: rewrite the four fixtures that name an account `u` to `/home/<user>`. G4: the MADR becomes `accepted` with this amendment; this plan stays `proposed` until the owner approves phase 1.

### Approval (2026-10-25)

The owner committed the amended pair as `92c2097` and said "proceed", which approves this plan; it becomes `in-progress`.

### Deviation D1 (2026-10-25): the Windows rule finds eight more lines

* **Found** in step 2's first `make disclosure-check`, after step 3 added the Windows rule: 8 findings beyond step 4's four, which the macOS and Linux rule alone had found. The search for Windows paths during step 1 had escaped the backslash wrongly, and missed them. None names a person:
  * `selfupdate/service/taskscheduler/testdata/oneshot.stored.xml:26-28` and `handoff_test.go:30` and `:43` use `user`, the account redaction `testdata/definitions/README.md` documents, because "an angle-bracket placeholder would not be valid XML";
  * `selfupdate/service/taskscheduler/reconcile_test.go:30` uses a made-up account, `Demo`;
  * `docs/decisions/0015-PLAN-remediate-third-debugging-pass-findings.md:3437-3438` quotes the GitHub-hosted runner's account in its 8.3 short form and its long form, from a CI log.
* **Decision (the owner, 2026-10-25, "Honour `user`, rewrite the rest").**
  * The Windows rule also accepts `user` as the whole account segment, the tree's documented XML redaction, which is an existing redaction and so a standing instruction. `check-docs.sh` says why. `username` and the like still fail.
  * `reconcile_test.go`'s account becomes `<user>`.
  * The 0015-PLAN lines become `<user>~1` and `<user>`, keeping the short-against-long point, annotated in place.
* **Files added to phase 1's scope:** `selfupdate/service/taskscheduler/reconcile_test.go`, and the 0015-PLAN. The MADR gets Amendment A2.

### Phase 1 (2026-10-25)

* **Step 1, inventory** (read before any edit):
  * **Triggers.** `ci.yml`: `push` to `main` and `release/**` and tags `v*`, with `paths-ignore: ['**.md']`; `pull_request` with the same `paths-ignore`. It has no `schedule` or `workflow_dispatch`. `docs.yml`: `push` to `main` and `release/**`, and `pull_request`, each with `paths: ['**.md', '.markdownlint-cli2.jsonc', '.github/workflows/docs.yml']`. `build-selfupdate-release.yml` and `publish-selfupdate-release.yml` are reusable workflows that this change leaves alone.
  * **Commands before this phase.** Both ran `npx --yes markdownlint-cli2@0.23.2`, `scripts/check-docs.sh --links` over `git ls-files '*.md'` and `--arch docs/architecture.md`. Only `ci.yml` ran `scripts/check-docs_test.sh`. Neither ran a records check or a disclosure check.
  * **Tracked Markdown.** 97 files: 81 in `docs/decisions/`, 3 in `docs/reports/`, `docs/README.md`, `docs/architecture.md`, three guides, `README.md`, `CHANGELOG.md`, `SECURITY.md`, `AGENTS.md`, the agent-rule files `.claude/rules/madr-and-plan-skill.md`, `.grok/rules/madr-plan-before-mutating-work.md` and `.opencode/rules.md`, and one fixture note, `selfupdate/service/taskscheduler/testdata/definitions/README.md`.
  * **Markdownlint's reach.** It lints 14 of them: the config excludes every file whose name holds MADR or PLAN in either case, which also covers the `.claude` and `.grok` rule files. It lints by the config's globs over the checkout, which in CI is the tracked set.
  * **Readers of tracked Markdown.** `scripts/check-docs.sh` (`--links`, `--arch`), which the Docs run already runs. No Go test reads a tracked `.md` file: the archive tests build `README.md` entries in memory, and the Task Scheduler tests read named `.xml` fixtures, never the definitions directory's `README.md`. No script under `scripts/` or `internal/cmd/` reads one.
  * **Docs-safe path set, for phase 2:** `**.md`, every tracked Markdown file, hidden agent-rule trees and the fixture note included. No reader of one runs outside the Docs run.
  * **Required status checks.** `main` has no branch protection (`gh api …/branches/main/protection`: 404, "Branch not protected") and no ruleset (`…/rulesets` and `…/rules/branches/main`: `[]`). So no filtered job can stay pending as a required check, and no migration is needed.
* **Step 2, `Makefile`:**
  * The four targets are `check-records` (`check_records_test.py`, then `check_records.py`), `markdownlint` (`$(MARKDOWNLINT)`, default `npx --yes markdownlint-cli2@0.23.2`), `docs-check` (`check-docs_test.sh`, then `--links` over `git ls-files -z '*.md'`, then `--arch`) and `disclosure-check` (`--ids --ids-optional` over `git ls-files -z`).
  * `GOPATH_BIN` and `GOBIN` are now expanded only when used (`=`, not `:=`). With `:=`, every `make` ran `go env` twice while reading the file: with no Go on `PATH` it printed "make: go: Command not found" twice. On the Docs runner, Go reading `go.mod`'s toolchain line could fetch a toolchain. With `PATH=/usr/bin:/bin`, the three non-`npx` targets ran with no `go` call.
  * `docs.yml` runs `make markdownlint`, `make check-records`, `make docs-check` and `make disclosure-check`. `ci.yml`'s validate job runs `make markdownlint` in place of its `npx` line, and `make check-records`, `make docs-check` and `make disclosure-check` in "Verify the gate and document tools", after `gate_test.sh` and `plant-copy_test.sh`. Triggers are unchanged here; they are phase 2's.
* **Step 3, `check-docs.sh`:**
  * `WINHOME` matches a drive, `Users` or `Documents and Settings` in any case, and one or more slashes or backslashes, so a Go-escaped path matches too. A line is one finding when either rule matches.
  * An unreadable file is a finding, `<file>: unreadable: <reason>`, and the scan goes on. Step 3's text said this replaced "a traceback". That premise was wrong: the script's top level already caught `OSError` and exited 2 at the first such file, as plant (c) shows. What changes is that every unreadable file is now reported, located, as a finding.
  * `check-docs_test.sh` gains Windows placeholders in the clean file, a file of four Windows paths that must each fail without printing the name, and an unreadable file. 19 cases pass.
* **Step 4:** the four lines are now `/home/<user>/…`. `gofmt -w` realigned `cleanup_test.go`'s comment column. The 0003-PLAN line is annotated in place. Then Deviation D1, below: `reconcile_test.go` and the 0015-PLAN lines. `TestValidateReceiptBackup`, `TestManagerEnvNames`, `TestScrubManagerEnv`, `TestReconcileMoved` and `TestOneShotGolden` pass, and `GOOS=windows go vet` is clean on their packages. The scan of every tracked file went from 4 findings (12 with the Windows rule) to 0, with the private deny list and without it.
* **Step 5, `scripts/check_records.py`** (new), with `--next`, and **`scripts/check_records_test.py`** (new), 20 cases over throwaway repositories.
  * Its first run on the tree found the one real defect: a blank line in `docs/README.md` before the 0036 rows had ended the table, so those rows rendered outside it. The line is removed, and the run reports "84 records, 0 findings". `--next` prints `0037`.
  * **Seen failing** (`plants_36p1.py`; (a) to (k) through `scripts/plant-copy.sh`, (l) to (n) in a scratch copy of the repository with its `.git`):

    | Plant | Failure |
    | :--- | :--- |
    | (a) the Windows rule not applied | `FAIL Windows home paths: rc=0 … 0 findings`; "18 passed, 1 failed" |
    | (b) the `user` redaction not accepted | `FAIL clean ids: rc=1 … clean.md:3: home directory path`, and the `--ids-optional` case |
    | (c) an unreadable file not caught | `FAIL unreadable: rc=2 out=[check-docs: missing.md: No such file or directory]` |
    | (d) rows outside the table not caught | `test_row_outside_the_table`: `0 != 1 : check-records: 3 records, 0 findings` |
    | (e) the row status not compared | `test_row_status_kind_and_duplicate`: the status finding not found |
    | (f) a gap not caught | `test_gap`: `0 != 1 : check-records: 4 records, 0 findings` |
    | (g) a lone PLAN's slug not compared | `test_lone_plan_slug`: `0 != 1` |
    | (h) `associated-madr` not compared | `test_associated_madr`: `0 != 1` |
    | (i) a second MADR not caught | `test_second_madr`: the finding not found |
    | (j) a record with no row not caught | `test_missing_row` and `test_untracked_counts_and_ignored_does_not`: `0 != 1` |
    | (k) a misplaced record not caught | `test_wrong_directory`: "a MADR belongs in docs/decisions/" not found |
    | (l) a blank line splits the records table | `make check-records`: "84 records, 4 findings", `Error 1` |
    | (m) a macOS home path in `README.md` | `make disclosure-check`, no deny list: "697 files, 0 deny-list rules, 1 findings", `Error 1` |
    | (n) a Windows home path in a Go test | the same, 1 finding |

    Each passes on the clean tree. The first try at the expected lines put the index's rows from line 6; they start at line 7, and the test was corrected, with the checker unchanged. An out-of-order row also flagged the correct row after it, so order is now checked against the previous row, and the test asserts the single finding.
* **Checks:**
  * `make pre-add-check` on the three Go files: "3 file(s) clean (gofmt, golangci-lint, go vet, go test, govulncheck)".
  * `shellcheck` 0.11.0 on `check-docs.sh` and `check-docs_test.sh`: clean. `actionlint` 1.7.12: clean. `check-workflows.sh` with its three rules over both workflows, `check-workflows_test.sh` and `workflow-shape_test.sh`: pass.
  * `make check-records`, `make markdownlint` ("0 issues"), `make docs-check` ("815 links in 97 files, 0 broken"; "19 rows … 0 differ") and `make disclosure-check` ("697 files, … 0 findings"): exit 0.
  * `make gate`: every step `rc=0`, apicheck "compatible with v1.22.0", ids "15 files, 16 deny-list rules, 0 findings", `overall=0`.

### Phase 2 (2026-10-25)

* Phase 1 was committed by the owner as `64c0cc8`.
* **Step 1, routing:** no trigger changes. Phase 1's inventory found `ci.yml`'s push and pull_request already ignoring the same set, `**.md`, which is the audited docs-safe set. `docs.yml` already watches that set and itself, on `ci.yml`'s push branches. A mixed change still runs full CI, since a `paths-ignore` skips a run only when every changed path matches. Tags, release branches and permissions are untouched. Each workflow's comment now points to the contract below.
* **Step 2, the contract** is a new section of `scripts/workflow-shape_test.sh`, which already reads the workflows as YAML with PyYAML. It runs in CI's "Verify the workflow contract" step and in `make gate`'s scripts step. Its usage gains two optional arguments, the CI and the Docs workflow; the Makefile is read from the root. It asserts:
  * `ci.yml` keeps its push to `main` and `release/**`, its `v*` tags and its pull_request, and has no `paths` filter;
  * `ci.yml`'s two events ignore the same paths, and only those in `DOCS_SAFE`, `{"**.md"}`, the set phase 1 audited;
  * `docs.yml`'s two events take the same paths, covering every ignored path and `docs.yml` itself, on `ci.yml`'s push branches;
  * each workflow runs the four documentation targets through `make`, and neither names `markdownlint-cli2`, `check-docs.sh` or `check_records.py` directly;
  * the Makefile pins `markdownlint-cli2` to an exact version;
  * both check out with one pinned `actions/checkout`.

  PyYAML reads the key `on` as the boolean `true` (YAML 1.1), and the test reads either. There is no `schedule` or `workflow_dispatch` in `ci.yml` to keep, so the trigger assertion holds the push branches, the tags and the pull_request.
* **Seen failing** (`plants_36p2.py`: each a scratch copy of the tree without `.git`, with its edits, running the copy's test):

  | Plant | Failure |
  | :--- | :--- |
  | (o) pull_request ignores more than push | `FAIL ci.yml's push and pull_request ignore the same paths` |
  | (p) both events skip XML fixtures, which Go tests read, with `docs.yml` watching them | `FAIL ci.yml ignores only audited docs-safe paths ['**.md']` |
  | (q) a Markdown reader run directly in full CI | `FAIL ci.yml runs no Markdown reader except through make (['check-docs.sh'])` |
  | (r) a missing Docs gate | `FAIL docs.yml runs every documentation target through make (missing ['disclosure-check'])` |
  | (s) a differing checkout pin in `docs.yml` | `FAIL ci.yml and docs.yml check out with the same pinned actions/checkout` |
  | (t) markdownlint unpinned in the Makefile | `FAIL the Makefile pins markdownlint-cli2 to an exact version` |
  | (u) full CI loses its tag trigger | `FAIL ci.yml keeps its push to main and release/**, its v* tags and its pull_request` |
  | (v) full CI gains a `paths` filter | `FAIL ci.yml has no paths filter` |
  | (w) `docs.yml` stops watching itself, on push | `FAIL docs.yml's paths cover every path ci.yml ignores, and docs.yml itself`, and the two-events check |

  Each plant fails one assertion alone, except (w), whose single edit also breaks the two events' agreement. The clean tree passes: "all assertions hold".
* **Checks:** `actionlint` 1.7.12: clean. `check-workflows.sh`'s three rules over both workflows, its default run and `check-workflows_test.sh`: pass. `shellcheck` on the test: clean. `make gate`: every step `rc=0`, ids "3 files, 16 deny-list rules, 0 findings", `overall=0`.

### Phase 3 (2026-10-25, in progress)

* Phase 2 was committed by the owner as `39a83e1`.
* **Step 1, documents:**
  * `AGENTS.md`: the four targets and what each runs; that `docs.yml` and full CI both run them; that CI may skip only paths whose every reader runs in `docs.yml`, today `**.md`, held by `workflow-shape_test.sh` and its `DOCS_SAFE`; under Records, `check_records.py --next` and `make check-records`; under Identifiers, the narrower CI baseline, the `user` redaction (Deviation D1), and the deny list and pre-push guard kept local.
  * `docs/architecture.md`: the tree gains `docs.yml`, which it had left out, `check_records.py` and its test, and the wider descriptions of `check-docs.sh` and `workflow-shape_test.sh`; "Tooling" lists the targets and what `disclosure-check` covers; the CI paragraph names the four targets and the routing contract; the Linux list runs `make markdownlint` and the three other targets. The checker's three rules were listed for `ci.yml` alone, though CI also runs them on `docs.yml`; that now says so.
  * The index row for this pair reads `accepted` and `in-progress`, matching the front matter; `make check-records` checks it.
* **Step 2, checks** (statuses captured before any filter):
  * `make check-records markdownlint docs-check disclosure-check`: exit 0. "84 records, 0 findings"; "0 issues"; `check-docs_test.sh` 19 passed, "817 links in 97 files, 0 broken", "19 rows … 0 differ"; "699 files, 16 deny-list rules, 0 findings".
  * `workflow-shape_test.sh`: "all assertions hold". `actionlint` 1.7.12: clean.
  * `make gate`: every step `rc=0`, `overall=0`. After a last edit to `docs/architecture.md`, the four targets and `--ids` on the two documents were run again: clean.
  * Each edited document's diff was read in full.
  * **Seen failing** (`plants_36p3.py`: a scratch copy of the repository with its `.git`, one plant each):

    | Plant | Failure |
    | :--- | :--- |
    | (x1) a PLAN with no MADR, with its row | `make check-records`: "85 records, 1 findings", `Error 1` |
    | (x2) an index status that differs from the front matter | `make check-records`: "84 records, 1 findings" |
    | (x3) a broken link | `make docs-check`: `building-releases.md:666: broken link: no-such-guide.md` |
    | (x4) a `.go` file the architecture table does not count | `make docs-check`: "19 rows … 1 differ" |
    | (x5) MD018 in `README.md` | `make markdownlint`: `MD018/no-missing-space-atx` |
    | (x6) a Linux home path in a guide | `make disclosure-check`, no deny list: "1 findings" |
    | (x7) a Windows home path in `SECURITY.md` | the same |
    | (x8) full CI drops `make check-records` | `workflow-shape_test.sh`: `FAIL ci.yml runs every documentation target through make (missing ['check-records'])` |

* **Step 3** waits for the owner's word to push. `main` is five commits ahead of `origin`, from `868cfea` to `39a83e1`, and they change workflows, scripts and Go tests: a mixed push. The disclosure guard passed over them. This phase's commit changes Markdown alone. Pushed in that order, the first push should start CI and Docs, and the second Docs alone.
