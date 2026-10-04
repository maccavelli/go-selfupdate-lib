#!/bin/sh
# Offline tests for scripts/check-release-tag.sh, and for the release
# workflow's use of it (docs/decisions/0005-PLAN-opt-in-prerelease-channels.md
# Step 6). The tag table is the one selfupdate's TestSemverPolicyValidate
# uses, so the workflow and the client admit the same tags.
set -eu

ROOT=$(cd -- "$(dirname "$0")/.." && pwd)
SCRIPT="${SCRIPT:-$ROOT/scripts/check-release-tag.sh}"
WORKFLOW="${WORKFLOW:-$ROOT/.github/workflows/publish-selfupdate-release.yml}"
CHANNELS='["rc","beta","alpha"]'

PASS=0
FAIL=0

# expect NAME WANT-EXIT ARG...
expect() {
	name="$1"
	want="$2"
	shift 2
	rc=0
	sh "$SCRIPT" "$@" >/dev/null 2>&1 || rc=$?
	if [ "$rc" -eq "$want" ]; then
		echo "  ok   $name"
		PASS=$((PASS + 1))
	else
		echo "  FAIL $name: want exit $want, got $rc"
		FAIL=$((FAIL + 1))
	fi
}

# tag WANT-ON-CHANNELS WANT-STABLE TAG
tag() {
	expect "[$3] on $CHANNELS" "$1" "$3" "$CHANNELS"
	expect "[$3] stable only" "$2" "$3"
}

tag 0 0 v1.2.3
tag 0 0 v0.0.0
tag 0 1 v1.2.3-rc.1
tag 0 1 v1.2.3-beta.10
tag 0 1 v1.2.3-alpha.0
tag 1 1 v1.2.3+meta
tag 1 1 v1.2.3-rc.1+meta
tag 1 1 v1.2
tag 1 1 v01.2.3
tag 1 1 1.2.3
tag 1 1 v1.2.3-rc
tag 1 1 v1.2.3-rc.01
tag 1 1 v1.2.3-RC.1
tag 1 1 v1.2.3-gamma.1
tag 1 1 v1.2.3-rc.1.2
tag 1 1 v1.2.3-rc.x
tag 1 1 v1.2.3-pre-view.1
tag 1 1 ""
# Non-ASCII digits: Go's \d is ASCII, so the client refuses these
# (0010-MADR D1): U+0661 and U+0663 ARABIC-INDIC DIGIT ONE and THREE, and
# U+FF11 FULLWIDTH DIGIT ONE.
tag 1 1 "v1.0.1١"
tag 1 1 "v1.0.0-rc.1٣"
tag 1 1 "v１.0.0"
nl='
'
# A trailing newline is part of no tag (0004-MADR R6).
expect "[v1.2.3-rc.1 + newline] on $CHANNELS" 1 "v1.2.3-rc.1$nl" "$CHANNELS"
expect "[v1.2.3 + newline] stable only" 1 "v1.2.3$nl"

# The default refuses every prerelease (0005-MADR E4).
expect "the default refuses a prerelease" 1 v1.2.3-rc.1
expect "an explicit [] refuses a prerelease" 1 v1.2.3-rc.1 '[]'
expect "one channel admits only itself" 1 v1.2.3-beta.1 '["rc"]'

# The channel array is checked as NewSemverPolicy checks it (E2).
expect "channels out of order" 2 v1.2.3 '["alpha","beta"]'
expect "a duplicate channel" 2 v1.2.3 '["rc","rc"]'
expect "an upper-case channel" 2 v1.2.3 '["RC"]'
expect "a channel name too long" 2 v1.2.3 '["abcdefghijklmnopq"]'
expect "channels not an array" 2 v1.2.3 '"rc"'
expect "channels not JSON" 2 v1.2.3 '[rc]'
expect "a non-string channel" 2 v1.2.3 '[1]'
expect "no arguments" 2
expect "too many arguments" 2 v1.2.3 '[]' extra

# The workflow: the input defaults to [], the tag step calls this script
# through env:, and only a suffixed tag is created as a prerelease that can
# never become latest. The step bodies are read as text, as the steps run.
rc=0
python3 -B - "$WORKFLOW" <<'PY' || rc=$?
import re, sys

text = open(sys.argv[1], encoding="utf-8").read()
fails = []


def step(name):
    m = re.search(r"^ *- name: " + re.escape(name) + r"\n(.*?)(?=^ *- name: |\Z)",
                  text, re.M | re.S)
    if not m:
        fails.append("no step %r" % name)
        return ""
    return m.group(1)


inp = re.search(r"^ {6}prerelease-channels-json:\n((?: {8}.*\n)+)", text, re.M)
if not inp:
    fails.append("no prerelease-channels-json input")
else:
    body = inp.group(1)
    if not re.search(r"^ {8}default: '\[\]'$", body, re.M):
        fails.append("prerelease-channels-json must default to '[]'")
    if not re.search(r"^ {8}required: false$", body, re.M):
        fails.append("prerelease-channels-json must be optional")

admit = step("Require an admitted tag")
if 'CHANNELS_JSON: ${{ inputs.prerelease-channels-json }}' not in admit:
    fails.append("the tag step must read the channels through env:")
if not re.search(r'check-release-tag\.sh "\$TAG" "\$CHANNELS_JSON"', admit):
    fails.append("the tag step must call check-release-tag.sh with the tag and channels")

verify = step("Validate the staged file set")
if '--channels "$CHANNELS_JSON"' not in verify:
    fails.append("the verifier must get the channels")

create = step("Create a draft release")
m = re.search(r'case "\$TAG" in\n\s*\*-\*\)\s*args\+=\(--prerelease --latest=false\)\s*;;\n\s*esac', create)
if not m:
    fails.append("the create step must add --prerelease --latest=false for a suffixed tag")
if 'gh release create "$TAG" "${args[@]}"' not in create:
    fails.append("the create step must pass the computed arguments")

for f in fails:
    print("  FAIL workflow: " + f)
sys.exit(1 if fails else 0)
PY
if [ "$rc" -eq 0 ]; then
	echo "  ok   the workflow publishes prereleases only as non-latest prereleases"
	PASS=$((PASS + 1))
else
	FAIL=$((FAIL + 1))
fi

echo "check-release-tag_test: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
