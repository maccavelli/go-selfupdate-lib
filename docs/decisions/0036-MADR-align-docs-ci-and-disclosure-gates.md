---
status: proposed
date: 2026-10-25
---
# Align docs-only CI and disclosure gates

## Context and Problem Statement

This repository needs a focused Docs run for documentation-only pushes and pull requests. The earlier local decision is 0028-MADR-skip-full-ci-for-docs-only-pushes.md. This new decision adds a common quality and disclosure contract across the seven repositories.

Observed in this checkout on 2026-10-25: `ci.yml` ignores `**.md`; `docs.yml` runs Markdownlint, `scripts/check-docs.sh --links` and its architecture-table check. The local `--ids` mode needs the owner's private deny list; CI currently runs neither it nor a portable disclosure scan. The full job runs `check-docs_test.sh`, but the Docs job does not. There is no numbered-record/index gate.

Across the repositories, the common documentation contract is pinned Markdownlint, numbered-record and relative-link validation, a portable disclosure check for real home paths in tracked text, and any repository-specific check that reads Markdown. The owner's global pre-push disclosure guard uses private deny patterns and remains mandatory before a real push. Go lint, race, vulnerability, build and release checks remain in full CI.

## Decision Drivers

* A documentation-only change has a successful, visible Docs run.
* A Markdown file read by code must run that reader check in Docs or full CI.
* Non-document changes, tags, schedules and manual runs keep full CI behavior.
* Both workflows use the same documentation commands and pins.
* CI disclosure checks work without private host data; the pre-push guard still protects publication.

## Considered Options

* Keep the current workflow split and existing checks.
* Align the documentation gates and add a portable disclosure check, with a path and gate parity test.
* Detect changed paths inside one full workflow and conditionally skip expensive jobs.

## Decision Outcome

Chosen option: "Align the documentation gates and add a portable disclosure check, with a path and gate parity test", because it gives both workflows the same common documentation checks, covers local Markdown readers, and makes filter drift observable. This is proposed for the owner; it does not approve CI implementation.

### Consequences

* Good, because documentation-only changes can complete after focused checks.
* Good, because records, links, Markdown style and generic disclosure faults are checked consistently.
* Neutral, because a mixed change may start both workflows.
* Bad, because private identity patterns remain enforceable only by the local pre-push guard; CI has narrower coverage.
* Bad, because path-filtered full-CI jobs may be pending for a required pull-request check; required-status settings need review before rollout.

### Confirmation

The paired [0036-PLAN-align-docs-ci-and-disclosure-gates.md](0036-PLAN-align-docs-ci-and-disclosure-gates.md) requires failing planted inputs for each new gate, static workflow checks, and observed Docs-only and full-CI runs after an approved push. Local checks alone do not prove GitHub path-filter behavior.

## Pros and Cons of the Options

### Keep the current workflow split and existing checks

* Good, because it requires little work.
* Bad, because the Docs job omits at least one common gate.

### Align the documentation gates and add a portable disclosure check, with a path and gate parity test

* Good, because it establishes the same baseline while preserving local Markdown readers.
* Bad, because it adds a maintained scanner and parity test.

### Detect changed paths inside one full workflow and conditionally skip expensive jobs

* Good, because a single workflow can expose one stable status.
* Bad, because conditional jobs add control logic and can obscure whether documents passed.

## More Information

Retain both workflows' current Markdownlint, link and architecture commands and release-branch triggers. Add a numbered-record/pair/index target and run it in both workflows. Run `scripts/check-docs.sh --ids-optional` on every tracked text file in both workflows as the public home-path disclosure baseline, and keep `--ids` plus the global pre-push guard locally for the private deny list. Add a workflow contract test to `scripts/workflow-shape_test.sh` for trigger symmetry and docs gate parity; update `AGENTS.md` and `docs/architecture.md`.

This fleet alignment decision cites 0028-MADR-skip-full-ci-for-docs-only-pushes.md without rewriting its history. The implementation is [0036-PLAN-align-docs-ci-and-disclosure-gates.md](0036-PLAN-align-docs-ci-and-disclosure-gates.md).

