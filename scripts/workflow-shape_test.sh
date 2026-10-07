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
# build-selfupdate-release.yml:
#   - the token is contents: read only, at the top and in no job;
#   - the tools and the source are checked out to sibling directories,
#     without persisted credentials, so neither makes the other dirty;
#   - setup-go has its cache off;
#   - the publish verifier runs on the staged set before it is uploaded;
#   - the identity job needs the build job and runs the tool's identity
#     check; no step or job has continue-on-error.
#
# Usage: workflow-shape_test.sh [PUBLISH-WORKFLOW [BUILD-WORKFLOW]]
# Exit 0 when every assertion holds, 1 otherwise, 2 when PyYAML is missing.
# PyYAML is required: pip install -r scripts/requirements-workflow-check.txt
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PUBLISH="${1:-$ROOT/.github/workflows/publish-selfupdate-release.yml}"
BUILD="${2:-$ROOT/.github/workflows/build-selfupdate-release.yml}"

python3 - "$PUBLISH" "$BUILD" <<'PY'
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


def load(path):
    with open(path, encoding="utf-8") as f:
        return yaml.safe_load(f)


def steps_of(path, job):
    return load(path)["jobs"][job]["steps"]


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

doc = load(sys.argv[2])
jobs = doc["jobs"]
build = jobs["build"]["steps"]
identity = jobs.get("identity", {})
check(doc.get("permissions") == {"contents": "read"}, "the build workflow's token is contents: read")
check(all("permissions" not in j for j in jobs.values()), "no build job widens the token")
tools = build[index(build, "Check out the called workflow commit")] if index(build, "Check out the called workflow commit") >= 0 else {}
source = build[index(build, "Check out the source")] if index(build, "Check out the source") >= 0 else {}
tpath = str(tools.get("with", {}).get("path", ""))
spath = str(source.get("with", {}).get("path", ""))
check(tpath and spath and "/" not in tpath.strip("/") and "/" not in spath.strip("/") and tpath != spath,
      "the tools and the source are sibling checkouts (%r, %r)" % (tpath, spath))
check(tools.get("with", {}).get("persist-credentials") is False and source.get("with", {}).get("persist-credentials") is False,
      "neither checkout persists credentials")
setup = build[index(build, "Set up Go")] if index(build, "Set up Go") >= 0 else {}
check(setup.get("with", {}).get("cache") is False, "setup-go has its cache off")
stage = index(build, "Check, pack and stage")
verified = index(build, "Verify the staged release")
upload = index(build, "Upload the staged release")
check(0 <= stage < verified < upload, "the publish verifier runs on the staged set before it is uploaded")
# The installers are rendered for the calling repository
# (docs/decisions/0014-PLAN-shared-installer-templates.md I5).
staging = build[stage] if stage >= 0 else {}
check('-repository "$REPOSITORY"' in staging.get("run", "")
      and staging.get("env", {}).get("REPOSITORY") == "${{ github.repository }}",
      "stage renders the installers for the calling repository (-repository from github.repository)")
check(verified >= 0 and "verify-selfupdate-release.sh" in build[verified].get("run", ""), "that step runs the publish verifier")
check(identity.get("needs") == "build", "the identity job needs the build job")
check(any("identity" in st.get("run", "") and "-want-version" in st.get("run", "") for st in identity.get("steps", [])),
      "the identity job runs the tool's identity check")
check(all("continue-on-error" not in st for j in jobs.values() for st in j.get("steps", []))
      and all("continue-on-error" not in j for j in jobs.values()),
      "nothing in the build workflow has continue-on-error")

if failures:
    print("workflow-shape_test: %d failed" % len(failures))
    sys.exit(1)
print("workflow-shape_test: all assertions hold")
PY
