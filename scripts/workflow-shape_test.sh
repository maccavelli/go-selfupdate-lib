#!/usr/bin/env bash
# Shape tests for the reusable release workflows, read as YAML the way
# GitHub reads them
# (docs/decisions/0013-PLAN-build-and-stage-release-workflow.md B3, B4).
#
# publish-selfupdate-release.yml:
#   - the tag check is the first step after the tools checkout;
#   - "Validate the staged file set" has id verify and passes
#     --github-output "$GITHUB_OUTPUT" to the verifier;
#   - the two archive-check steps come after it and before "Create a draft
#     release", are guarded by exactly steps.verify.outputs.packed == 'true',
#     and neither has continue-on-error, so a refused archive stops the job
#     before anything is published.
#
# Usage: workflow-shape_test.sh [PUBLISH-WORKFLOW]
# Exit 0 when every assertion holds, 1 otherwise, 2 when PyYAML is missing.
# PyYAML is required: pip install -r scripts/requirements-workflow-check.txt
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PUBLISH="${1:-$ROOT/.github/workflows/publish-selfupdate-release.yml}"

python3 - "$PUBLISH" <<'PY'
import sys

try:
    import yaml
except ImportError:
    print("workflow-shape_test: PyYAML is required: "
          "pip install -r scripts/requirements-workflow-check.txt", file=sys.stderr)
    sys.exit(2)

failures = []


def check(cond, msg):
    print(("  ok   " if cond else "  FAIL ") + msg)
    if not cond:
        failures.append(msg)


def steps_of(path, job):
    with open(path, encoding="utf-8") as f:
        doc = yaml.safe_load(f)
    return doc["jobs"][job]["steps"]


def index(steps, name):
    for i, s in enumerate(steps):
        if s.get("name") == name:
            return i
    return -1


publish = steps_of(sys.argv[1], "publish")
checkout = index(publish, "Check out the called workflow commit")
tag = index(publish, "Require an admitted tag")
verify = index(publish, "Validate the staged file set")
setup = index(publish, "Set up Go for the archive check")
archives = index(publish, "Check archived programs")
draft = index(publish, "Create a draft release")
GUARD = "steps.verify.outputs.packed == 'true'"

check(checkout >= 0 and tag == checkout + 1, "the tag check is the first step after the tools checkout")
check(verify >= 0 and publish[verify].get("id") == "verify", "the verifier step has id verify")
check(verify >= 0 and '--github-output "$GITHUB_OUTPUT"' in publish[verify].get("run", ""),
      "the verifier writes its packed output")
check(0 <= verify < setup < archives < draft, "the archive check runs after the verifier and before the draft")
for i, what in ((setup, "Go setup"), (archives, "archive check")):
    step = publish[i] if i >= 0 else {}
    check(step.get("if") == GUARD, "the %s runs exactly when the release is packed" % what)
    check("continue-on-error" not in step, "the %s has no continue-on-error" % what)
check(archives >= 0 and "selfupdate-release check" in publish[archives].get("run", ""),
      "the archive check runs selfupdate-release check")

if failures:
    print("workflow-shape_test: %d failed" % len(failures))
    sys.exit(1)
print("workflow-shape_test: all assertions hold")
PY
