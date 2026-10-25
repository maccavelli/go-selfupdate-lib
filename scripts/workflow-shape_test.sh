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
#     before anything is published;
#   - signing is opt-in: the sign input is a boolean, default false; the
#     secret is declared, not required, and read only by the sign job,
#     which has the environment selfupdate-signing, a read-only token, and
#     no continue-on-error; publish needs sign, runs after a successful or
#     a skipped sign only, and adds the note after the archive check and
#     before the draft
#     (docs/decisions/0025-MADR-align-with-go-sdk-design-practices.md D10).
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
# ci.yml and docs.yml, the routing contract
# (docs/decisions/0036-PLAN-align-docs-ci-and-disclosure-gates.md, phase 2):
#   - ci.yml keeps its push to main and release/**, its v* tags and its
#     pull_request, and has no paths filter; its push and pull_request
#     ignore the same paths, and only the audited docs-safe set (DOCS_SAFE
#     below: every reader of those files runs in docs.yml);
#   - docs.yml's push and pull_request take the same paths, covering every
#     path ci.yml ignores and docs.yml itself, on ci.yml's branches;
#   - both run every documentation target (DOC_TARGETS) through make, and
#     neither runs a Markdown reader any other way, so no reader runs only
#     in the workflow an ignored path skips; markdownlint's pin is the
#     Makefile's alone;
#   - both check out with the same pinned actions/checkout.
#
# Usage: workflow-shape_test.sh [PUBLISH-WORKFLOW [BUILD-WORKFLOW [CI-WORKFLOW [DOCS-WORKFLOW]]]]
# Exit 0 when every assertion holds, 1 otherwise, 2 when PyYAML is missing.
# PyYAML is required: pip install -r scripts/requirements-workflow-check.txt
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PUBLISH="${1:-$ROOT/.github/workflows/publish-selfupdate-release.yml}"
BUILD="${2:-$ROOT/.github/workflows/build-selfupdate-release.yml}"
CI="${3:-$ROOT/.github/workflows/ci.yml}"
DOCS="${4:-$ROOT/.github/workflows/docs.yml}"

python3 - "$PUBLISH" "$BUILD" "$CI" "$DOCS" "$ROOT/Makefile" <<'PY'
import re
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
# docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md F4, F9.
wait = index(publish, "Wait for immutability and release attestation")
wait_run = publish[wait].get("run", "") if wait >= 0 else ""
check("Enable release immutability" in wait_run and "gh release delete" in wait_run,
      "the immutability timeout names the setting and how to recover")
group = load(sys.argv[1])["jobs"]["publish"].get("concurrency", {})
check(group.get("group") == "go-selfupdate-lib-publish-${{ github.repository }}"
      and group.get("cancel-in-progress") is False,
      "one publish runs at a time per repository, none cancelled")

# docs/decisions/0025-MADR-align-with-go-sdk-design-practices.md D10: the
# opt-in sign job. PyYAML reads the key on as True.
wf = load(sys.argv[1])
call = (wf.get("on") or wf.get(True) or {}).get("workflow_call", {})
inputs = call.get("inputs", {})
check(inputs.get("sign", {}).get("type") == "boolean" and inputs.get("sign", {}).get("default") is False,
      "the sign input is a boolean, default false")
check(inputs.get("note-verifier-key", {}).get("type") == "string" and inputs.get("note-verifier-key", {}).get("default") == "",
      "the note-verifier-key input is a string, default empty")
check(call.get("secrets", {}).get("SELFUPDATE_NOTE_KEY", {}).get("required") is False,
      "the signer key is a declared secret, not required")
sign_job = wf["jobs"].get("sign", {})
check(sign_job.get("if") == "inputs.sign", "the sign job runs only when sign is true")
check(sign_job.get("environment") == "selfupdate-signing", "the sign job has the environment selfupdate-signing")
check(sign_job.get("permissions") == {"contents": "read"}, "the sign job's token is contents: read")
sign_steps = sign_job.get("steps", [])
signer = sign_steps[index(sign_steps, "Sign the manifest")] if index(sign_steps, "Sign the manifest") >= 0 else {}
check(signer.get("env", {}).get("SELFUPDATE_NOTE_KEY") == "${{ secrets.SELFUPDATE_NOTE_KEY }}"
      and "secrets." not in signer.get("run", "") and "selfupdate-note sign" in signer.get("run", ""),
      "the signer reads the key through env only and runs selfupdate-note sign")
check("continue-on-error" not in sign_job and all("continue-on-error" not in st for st in sign_steps),
      "nothing in the sign job has continue-on-error")
check(all("SELFUPDATE_NOTE_KEY" not in yaml.safe_dump(j) for name, j in wf["jobs"].items() if name != "sign"),
      "no job but sign reads the signer key")
pjob = wf["jobs"]["publish"]
check(pjob.get("needs") == "sign", "publish needs sign")
check(pjob.get("if") == "${{ !cancelled() && (needs.sign.result == 'success' || (needs.sign.result == 'skipped' && !inputs.sign)) }}",
      "publish runs after a successful sign, or a sign skipped because signing is off")
add = index(publish, "Add the release statement")
require = index(publish, "Require the release statement")
check(0 <= archives < add < require < draft, "the note is added after the archive check and before the draft")
for i, what in ((add, "note download"), (require, "note check")):
    step = publish[i] if i >= 0 else {}
    check(step.get("if") == "inputs.sign", "the %s runs exactly when signing" % what)
    check("continue-on-error" not in step, "the %s has no continue-on-error" % what)

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
# The plan checks the module's library version against the spec
# (docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md F3).
planned = build[index(build, "Plan")] if index(build, "Plan") >= 0 else {}
check('-module-dir "src/$MODULE_DIR"' in planned.get("run", "")
      and planned.get("env", {}).get("MODULE_DIR") == "${{ inputs.module-dir }}",
      "the plan step passes the module directory")
check(identity.get("needs") == "build", "the identity job needs the build job")
check(any("identity" in st.get("run", "") and "-want-version" in st.get("run", "") for st in identity.get("steps", [])),
      "the identity job runs the tool's identity check")
check(all("continue-on-error" not in st for j in jobs.values() for st in j.get("steps", []))
      and all("continue-on-error" not in j for j in jobs.values()),
      "nothing in the build workflow has continue-on-error")

# The routing contract (0036-PLAN phase 2). DOCS_SAFE is the audited set of
# paths full CI may skip: 0036-PLAN phase 1, step 1 found every reader of
# tracked Markdown in docs.yml. A new pattern is audited, then added here.
DOCS_SAFE = {"**.md"}
DOC_TARGETS = ("markdownlint", "check-records", "docs-check", "disclosure-check")
# Markdown readers, called other than through make.
DIRECT_READERS = ("markdownlint-cli2", "check-docs.sh", "check_records.py")
ci_wf, docs_wf = load(sys.argv[3]), load(sys.argv[4])


def on(wf):
    # PyYAML reads the key on as the boolean true (YAML 1.1).
    trig = wf.get("on", wf.get(True)) or {}
    return {k: (v or {}) for k, v in trig.items()}


def runs(wf):
    return [st.get("run", "") for j in wf.get("jobs", {}).values() for st in j.get("steps", [])]


def uses(wf, action):
    return {st["uses"] for j in wf.get("jobs", {}).values() for st in j.get("steps", [])
            if st.get("uses", "").startswith(action + "@")}


ci_on, docs_on = on(ci_wf), on(docs_wf)
ci_push, ci_pr = ci_on.get("push", {}), ci_on.get("pull_request", {})
docs_push, docs_pr = docs_on.get("push", {}), docs_on.get("pull_request", {})
check(set(ci_push.get("branches", [])) == {"main", "release/**"} and ci_push.get("tags") == ["v*"]
      and "pull_request" in ci_on,
      "ci.yml keeps its push to main and release/**, its v* tags and its pull_request")
check(all("paths" not in t for t in ci_on.values()), "ci.yml has no paths filter")
ignored = set(ci_push.get("paths-ignore", []))
check(ignored == set(ci_pr.get("paths-ignore", [])), "ci.yml's push and pull_request ignore the same paths")
check(ignored <= DOCS_SAFE, "ci.yml ignores only audited docs-safe paths %s" % sorted(DOCS_SAFE))
watched = set(docs_push.get("paths", []))
check(watched == set(docs_pr.get("paths", [])), "docs.yml's push and pull_request take the same paths")
check(ignored <= watched and ".github/workflows/docs.yml" in watched,
      "docs.yml's paths cover every path ci.yml ignores, and docs.yml itself")
check(docs_push.get("branches") == ci_push.get("branches"), "docs.yml runs on ci.yml's push branches")
for name, wf in (("ci.yml", ci_wf), ("docs.yml", docs_wf)):
    scripts = "\n".join(runs(wf))
    missing = [t for t in DOC_TARGETS if ("make " + t) not in scripts]
    check(not missing, "%s runs every documentation target through make%s" % (name, " (missing %s)" % missing if missing else ""))
    direct = [r for r in DIRECT_READERS if r in scripts]
    check(not direct, "%s runs no Markdown reader except through make%s" % (name, " (%s)" % direct if direct else ""))
with open(sys.argv[5], encoding="utf-8") as f:
    makefile = f.read()
check(re.search(r"^MARKDOWNLINT \?= npx --yes markdownlint-cli2@\d+\.\d+\.\d+$", makefile, re.M) is not None,
      "the Makefile pins markdownlint-cli2 to an exact version")
ci_co, docs_co = uses(ci_wf, "actions/checkout"), uses(docs_wf, "actions/checkout")
check(len(ci_co) == 1 and ci_co == docs_co, "ci.yml and docs.yml check out with the same pinned actions/checkout")

if failures:
    print("workflow-shape_test: %d failed" % len(failures))
    sys.exit(1)
print("workflow-shape_test: all assertions hold")
PY
