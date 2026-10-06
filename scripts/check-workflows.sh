#!/usr/bin/env bash
# Check GitHub Actions workflows by parsing them as YAML, the way GitHub does,
# rather than scanning lines (docs/decisions/0004-MADR-evolve-selfupdate-api-and-tui-support.md
# R7, R8). Three rules:
#
#   expressions  no step's run script contains ${{ }}. GitHub substitutes an
#                expression into the script text before the shell runs it, so
#                an attacker-chosen value such as a tag name becomes shell
#                code. Values reach run scripts through env: instead
#                (docs/decisions/0003-MADR-remediate-debugging-pass-findings.md D8).
#
#   gh-repo      a step whose run script calls a repository-scoped gh command,
#                directly or by path, or runs refuse-existing-release.sh or
#                release-latest-flag.sh (which call gh), has
#                GH_REPO in the step's, the job's or the workflow's env. The
#                reusable release job never checks the caller out, so gh has no
#                local repository to infer from, and without GH_REPO it fails or
#                silently checks nothing (0003-MADR D4). A command that is
#                itself a --help probe resolves no repository and is exempt.
#                Scripts are split into commands by a shell tokenizer, so a #
#                inside quotes starts no comment
#                (docs/decisions/0010-MADR-remediate-second-debugging-pass-findings.md D6).
#
#   permissions  the workflow has a top-level permissions: block, so its
#                token never takes the repository's default scope (0010-MADR
#                D7).
#
#   pins         every action a step uses, and every reusable workflow a job
#                uses from another repository, is pinned to a full 40-hex
#                commit SHA. A local ./ path and a docker:// image are exempt
#                (docs/decisions/0013-PLAN-build-and-stage-release-workflow.md
#                B4).
#
# Usage: check-workflows.sh [--rule expressions|gh-repo|permissions|pins|all] [workflow...]
# With no workflow, the two reusable release workflows are checked. Exit 0 when
# clean, 1 on findings, 2 on a usage, parse or environment error. PyYAML is
# required: pip install -r scripts/requirements-workflow-check.txt
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
RULE=all
if [ "${1:-}" = "--rule" ]; then
	RULE="${2:-}"
	shift 2 || true
fi
case "$RULE" in
expressions | gh-repo | permissions | pins | all) ;;
*)
	echo "usage: check-workflows.sh [--rule expressions|gh-repo|permissions|pins|all] [workflow...]" >&2
	exit 2
	;;
esac
if [ $# -eq 0 ]; then
	set -- "$ROOT/.github/workflows/publish-selfupdate-release.yml" \
		"$ROOT/.github/workflows/build-selfupdate-release.yml"
fi

python3 - "$RULE" "$@" <<'PY'
import os
import re
import shlex
import sys

try:
    import yaml
except ImportError:
    print("check-workflows: PyYAML is required: "
          "pip install -r scripts/requirements-workflow-check.txt", file=sys.stderr)
    sys.exit(2)


class UniqueKeyLoader(yaml.SafeLoader):
    """A SafeLoader that refuses duplicate mapping keys, which PyYAML would
    otherwise resolve silently to the last value."""


def construct_mapping(loader, node, deep=False):
    seen = set()
    for key_node, _ in node.value:
        key = loader.construct_object(key_node, deep=deep)
        if key in seen:
            raise yaml.constructor.ConstructorError(
                None, None, "duplicate key %r" % (key,), key_node.start_mark)
        seen.add(key)
    return yaml.SafeLoader.construct_mapping(loader, node, deep)


UniqueKeyLoader.add_constructor(
    yaml.resolver.BaseResolver.DEFAULT_MAPPING_TAG, construct_mapping)

# Repository-scoped gh command groups, and the scripts that call gh.
GH_GROUPS = {"release", "api", "repo", "run", "workflow", "pr", "issue", "attestation"}
GH_SCRIPTS = {"refuse-existing-release.sh", "release-latest-flag.sh"}
OPERATORS = set(";&|()\n")
# A pinned reference ends in @ and a full commit SHA.
PIN_RE = re.compile(r"@[0-9a-f]{40}$")


def pinned(uses):
    return uses.startswith("./") or uses.startswith("docker://") or PIN_RE.search(uses) is not None

rule = sys.argv[1]
paths = sys.argv[2:]


def strip_comments(script):
    """Remove shell comments: a # outside quotes, at the start of a word,
    up to the end of its line. The newline is kept, so it still ends the
    command before it."""
    out, quote, i = [], None, 0
    while i < len(script):
        c = script[i]
        if quote == "'":
            quote = None if c == "'" else quote
        elif c == "\\" and i + 1 < len(script):
            out.append(c + script[i + 1])
            i += 2
            continue
        elif quote == '"':
            quote = None if c == '"' else quote
        elif c in "'\"":
            quote = c
        elif c == "#" and (i == 0 or script[i - 1] in " \t\n;&|()"):
            while i < len(script) and script[i] != "\n":
                i += 1
            continue
        out.append(c)
        i += 1
    return "".join(out)


def commands(script):
    """Split a shell script into commands, each a list of words, with a
    shell tokenizer: quotes are honoured, a # starts a comment only outside
    them, and newlines, ;, &, | and parentheses end a command."""
    text = strip_comments(script.replace("\\\n", " "))
    lex = shlex.shlex(text, posix=True, punctuation_chars=";&|()\n")
    lex.whitespace = " \t\r"
    lex.whitespace_split = True
    lex.commenters = ""
    try:
        tokens = list(lex)
    except ValueError:
        # An unbalanced quote: fall back to whitespace words, so a call is
        # never hidden by a parse failure.
        tokens = text.replace("\n", " ; ").split()
    cmds, cmd = [], []
    for tok in tokens:
        if tok and set(tok) <= OPERATORS:
            if cmd:
                cmds.append(cmd)
            cmd = []
        else:
            cmd.append(tok)
    if cmd:
        cmds.append(cmd)
    return cmds


def calls_gh(script):
    for words in commands(script):
        if "--help" in words:
            continue
        names = [os.path.basename(w) for w in words]
        gh = any(n == "gh" and i + 1 < len(words) and words[i + 1] in GH_GROUPS
                 for i, n in enumerate(names))
        if gh or GH_SCRIPTS.intersection(names):
            return " ".join(words)
    return None


def has_repo(*envs):
    return any(isinstance(env, dict) and env.get("GH_REPO") not in (None, "")
               for env in envs)


findings = 0
for path in paths:
    try:
        with open(path, encoding="utf-8") as f:
            doc = yaml.load(f, Loader=UniqueKeyLoader)
    except (OSError, yaml.YAMLError) as e:
        print("check-workflows: %s: %s" % (path, e), file=sys.stderr)
        sys.exit(2)
    if not isinstance(doc, dict):
        print("check-workflows: %s: not a workflow mapping" % path, file=sys.stderr)
        sys.exit(2)
    if rule in ("permissions", "all") and "permissions" not in doc:
        print("%s: no top-level permissions: block" % path, file=sys.stderr)
        findings += 1
    jobs = doc.get("jobs") or {}
    for job_id, job in jobs.items():
        if not isinstance(job, dict):
            continue
        uses = job.get("uses")
        if rule in ("pins", "all") and isinstance(uses, str) and not pinned(uses):
            print("%s: jobs.%s: reusable workflow not pinned to a commit SHA: %s" % (path, job_id, uses), file=sys.stderr)
            findings += 1
        for i, step in enumerate(job.get("steps") or []):
            if not isinstance(step, dict):
                continue
            where = "%s: jobs.%s.steps[%d]" % (path, job_id, i)
            if step.get("name"):
                where += " (%s)" % step["name"]
            uses = step.get("uses")
            if rule in ("pins", "all") and isinstance(uses, str) and not pinned(uses):
                print("%s: action not pinned to a commit SHA: %s" % (where, uses), file=sys.stderr)
                findings += 1
            script = step.get("run")
            if not isinstance(script, str):
                continue
            if rule in ("expressions", "all") and "${{" in script:
                print("%s: ${{ }} inside a run script (use env:)" % where, file=sys.stderr)
                findings += 1
            if rule in ("gh-repo", "all"):
                cmd = calls_gh(script)
                if cmd and not has_repo(step.get("env"), job.get("env"), doc.get("env")):
                    print("%s: gh call without GH_REPO: %s" % (where, cmd), file=sys.stderr)
                    findings += 1

if findings:
    sys.exit(1)
print("check-workflows: ok (%s) — %d workflow(s)" % (rule, len(paths)))
PY
