---
status: proposed
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
4. Before the gate runs in CI, clear the four existing findings: `selfupdate/cleanup_test.go:22` and `:32` and `selfupdate/service/managerenv_test.go:34` use a made-up Linux home directory for an account named `u`, and `docs/decisions/0003-PLAN-remediate-debugging-pass-findings.md:932` quotes the first. Rewrite each to `/home/<user>/…`; annotate the 0003-PLAN line in place as changed by this plan. Both tests compare strings only; re-run them on this host and with `GOOS=windows` `go vet`, and run `make pre-add-check` on the two Go files. Record the scan at 4 findings before and 0 after. *Amended 2026-10-25 (MADR Amendment A1, G3): a scan of all 697 tracked files on 2026-10-25 found these four, all made-up fixtures.*
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

