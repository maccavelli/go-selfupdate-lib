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

Affected files: `.github/workflows/ci.yml`, `.github/workflows/docs.yml`, `Makefile`, `scripts/check_records.py` (new), `scripts/check_records_test.py` (new), `scripts/workflow-shape_test.sh`, `AGENTS.md`, `docs/architecture.md`, `docs/README.md`.

Repository-specific work: Retain both workflows' current Markdownlint, link and architecture commands and release-branch triggers. Add a numbered-record/pair/index target and run it in both workflows. Run `scripts/check-docs.sh --ids-optional` on every tracked text file in both workflows as the public home-path disclosure baseline, and keep `--ids` plus the global pre-push guard locally for the private deny list. Add a workflow contract test to `scripts/workflow-shape_test.sh` for trigger symmetry and docs gate parity; update `AGENTS.md` and `docs/architecture.md`.

Boundary: The owner deny list cannot be reproduced on an untrusted pull-request runner. Preserve `--ids` as the local pre-push check and explain the narrower CI baseline explicitly.

## Implementation Steps

### Phase 1: inventory and shared checks

1. Re-read workflow triggers, job commands, Markdownlint globs, tracked Markdown files and every code or script reader of tracked Markdown. Record the docs-safe path set in this PLAN before editing filters. Include hidden agent-rule trees and test fixtures. Review required-status settings; if a filtered job would stay pending, amend this pair with a concrete migration before changing filters.
2. Add missing record/link and Markdownlint targets, reusing the current commands where they exist. Run identical pinned documentation commands in full CI and Docs. Keep Go lint, tests, vulnerability, builds and releases in full CI.
3. Implement a standard-library portable disclosure check over tracked text. It rejects real macOS, Linux and Windows user-home paths, accepts documented placeholders, prints only file, line and category, and fails on unreadable tracked input. Run the same target in full CI and Docs. Retain the private deny list and global pre-push guard locally.
4. Add fixture tests for new gates. Prove each fails on deliberately broken input in a scratch copy and passes on clean input. Record commands, statuses and redacted diagnostics. Commit the approved phase after checks pass.

### Phase 2: workflow routing and parity

1. Keep push and pull-request CI ignore sets identical and limited to audited docs-safe paths. Make Docs paths cover every ignored path and the Docs workflow itself. Mixed changes run full CI. Preserve tags, schedules, dispatch, release jobs and permissions.
2. Add or extend a Python workflow contract check. It validates actual YAML structure, compares event filters, confirms ignored tracked paths are in Docs, refuses known Markdown inputs to code tests unless their reader runs in Docs, and compares common docs-gate commands and pins. Integrate it in the workflow/tool test target.
3. Run actionlint and the contract check. In independent scratch copies, plant a mismatched path, a Markdown reader wrongly ignored, a missing Docs gate and a differing pin; require each to fail. Commit the approved phase after checks pass.

### Phase 3: documentation and observed rollout

1. Explain routing, docs-safe paths, common checks, local readers and the private pre-push guard in AGENTS.md and the architecture or CI guide. Verify this pair's exact filenames and current statuses in the records index; use the repository index generator if present.
2. Run `make check-records markdownlint docs-check disclosure-check`; `./scripts/check-docs_test.sh`; `./scripts/workflow-shape_test.sh`; actionlint; scratch plants for a bad record pair/index, link, architecture count, MD018, home path and docs/full gate mismatch. Capture status before filtering output. Check this pair and edited documents for real-machine identifiers. Read each edited file in full and verify links.
3. After explicit authorization to push in that same turn, observe a docs-only push, a non-doc or mixed push and the next tag or scheduled run. Record run IDs, conclusions and which workflow started. Mark this PLAN complete only after the criteria below hold; otherwise record the outstanding observation. Commit at the end of each approved phase under repository rules.

## Verification

* Both workflows pass pinned Markdownlint, numbered-record/index and relative-link validation, and portable tracked-text disclosure scanning.
* Each Markdown reader runs on changes to its input, either in Docs or full CI.
* A docs-only push starts Docs but not full CI; a non-doc or mixed push starts full CI. Tags, schedules and dispatch retain existing behavior.
* Scratch plants fail their intended gates; filter or gate drift fails the parity test.
* Before an authorized push, the local pre-push guard checks outgoing commits. Its private deny list is never copied into CI.

## Rollout and Rollback

Land gates before relying on the filters. If routing misclassifies a change, revert the routing commit through a new reviewed commit so full CI runs for every push again. Keep the shared gates while repairing the audit. This PLAN does not authorize a push or tag.

