#!/usr/bin/env bash
# Offline tests for check-workflows.sh. Each plant is a workflow shape GitHub
# accepts; it carries the cases of the two line-scanning checkers it replaced
# (docs/decisions/0003-MADR-remediate-debugging-pass-findings.md D4, D8) and
# the shapes they got wrong (docs/decisions/0004-MADR-evolve-selfupdate-api-and-tui-support.md
# R7, R8).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CHECK="$ROOT/scripts/check-workflows.sh"
WORKFLOW="$ROOT/.github/workflows/publish-selfupdate-release.yml"
CI="$ROOT/.github/workflows/ci.yml"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

PASS=0
FAIL=0

expect() { # name want-rc rule file
	set +e
	"$CHECK" --rule "$3" "$4" >/dev/null 2>&1
	rc=$?
	set -e
	if [ "$rc" -eq "$2" ]; then
		echo "  ok   $1"
		PASS=$((PASS + 1))
	else
		echo "  FAIL $1: want exit $2, got $rc"
		FAIL=$((FAIL + 1))
	fi
}

# plant <file> <yaml>: the release workflow with a step appended to its last
# job. printf %s keeps the YAML byte for byte.
plant() {
	cp "$WORKFLOW" "$1"
	printf '%s\n' "$2" >>"$1"
}

# drop_repo_in <step name>: the release workflow without that step's GH_REPO.
drop_repo_in() {
	awk -v step="- name: $1" '
		index($0, step) { in_step = 1 }
		in_step && /GH_REPO:/ { in_step = 0; next }
		{ print }
	' "$WORKFLOW"
}

# The expression text is built at run time so this file holds no literal
# expression of its own.
EXPR='$'"{{ github.ref_name }}"
# The literal $TAG in planted YAML is workflow content, not shell here.
# shellcheck disable=SC2016
VIEW='gh release view "$TAG"'

echo "real workflows"
expect "release workflow, expressions" 0 expressions "$WORKFLOW"
expect "release workflow, gh-repo" 0 gh-repo "$WORKFLOW"
expect "ci workflow, expressions" 0 expressions "$CI"

echo "expressions"
awk -v expr="$EXPR" '
	/gh release edit "\$TAG"/ { sub(/"\$TAG"/, "\"" expr "\"") }
	{ print }
' "$WORKFLOW" >"$WORK/block.yml"
grep -qF "gh release edit \"$EXPR\"" "$WORK/block.yml"
expect "in a run block" 1 expressions "$WORK/block.yml"

plant "$WORK/oneline.yml" "      - name: One line
        run: echo \"$EXPR\""
expect "on a one-line run" 1 expressions "$WORK/oneline.yml"

# shellcheck disable=SC2016
plant "$WORK/env.yml" "      - name: Env
        env:
          X: $EXPR
        run: |
          echo \"\$X\""
expect "in env is allowed" 0 expressions "$WORK/env.yml"

plant "$WORK/comment.yml" "      - name: Commented header
        run: |  # a comment after the indicator
          echo \"$EXPR\""
expect "R7: block header with a comment" 1 expressions "$WORK/comment.yml"

plant "$WORK/indent.yml" "      - name: Explicit indentation
        run: |2
            echo \"$EXPR\""
expect "R7: explicit indentation indicator" 1 expressions "$WORK/indent.yml"

plant "$WORK/plain.yml" "      - name: Plain multi-line scalar
        run: echo start
          $EXPR"
expect "R7: plain multi-line scalar" 1 expressions "$WORK/plain.yml"

plant "$WORK/quoted.yml" "      - name: Quoted key
        \"run\": |
          echo \"$EXPR\""
expect "R7: quoted run key" 1 expressions "$WORK/quoted.yml"

# shellcheck disable=SC2016
plant "$WORK/dashrun.yml" "      - run: |
          echo \"\$X\"
        env:
          X: $EXPR"
expect "R7: env after a dash-run block is allowed" 0 expressions "$WORK/dashrun.yml"

plant "$WORK/dupkey.yml" "      - name: Duplicate run key
        run: echo \"$EXPR\"
        run: echo hidden"
expect "a duplicate key is an error, not a pass" 2 expressions "$WORK/dupkey.yml"

echo "gh-repo"
drop_repo_in "Publish the draft" >"$WORK/control.yml"
expect "gh call without GH_REPO" 1 gh-repo "$WORK/control.yml"

drop_repo_in "Refuse an existing release" >"$WORK/refuse.yml"
expect "refuse script without GH_REPO" 1 gh-repo "$WORK/refuse.yml"

awk '
	index($0, "- name: Publish the draft") { in_step = 1 }
	in_step && /GH_REPO:/ { sub(/GH_REPO:/, "# GH_REPO:"); in_step = 0 }
	{ print }
' "$WORKFLOW" >"$WORK/commented.yml"
expect "commented-out GH_REPO" 1 gh-repo "$WORK/commented.yml"

plant "$WORK/unnamed.yml" "      - run: $VIEW"
expect "unnamed step inherits nothing" 1 gh-repo "$WORK/unnamed.yml"

plant "$WORK/help.yml" "      - name: Probe
        run: gh release list --help"
expect "--help probe exempt" 0 gh-repo "$WORK/help.yml"

plant "$WORK/echo.yml" "      - name: Echo
        run: |
          echo \"GH_REPO: owner/repo\"
          $VIEW"
expect "R8: GH_REPO text in the script sets nothing" 1 gh-repo "$WORK/echo.yml"

plant "$WORK/ghrun.yml" "      - name: Runs
        run: gh run list"
expect "R8: gh run" 1 gh-repo "$WORK/ghrun.yml"

plant "$WORK/ghworkflow.yml" "      - name: Workflows
        run: gh workflow list"
expect "R8: gh workflow" 1 gh-repo "$WORK/ghworkflow.yml"

# shellcheck disable=SC2016
plant "$WORK/bashrefuse.yml" "      - name: Refuse through bash
        run: bash .core-lib-release-tools/scripts/refuse-existing-release.sh \"\$TAG\""
expect "R8: refuse script run through bash" 1 gh-repo "$WORK/bashrefuse.yml"

plant "$WORK/helptail.yml" "      - name: Help tail
        run: $VIEW || gh release --help"
expect "R8: a --help tail exempts only itself" 1 gh-repo "$WORK/helptail.yml"

plant "$WORK/envafter.yml" "      - name: Env after run
        run: $VIEW
        env:
          GH_REPO: owner/repo"
expect "R8: env after run counts" 0 gh-repo "$WORK/envafter.yml"

plant "$WORK/jobenv.yml" "  planted:
    runs-on: ubuntu-latest
    env:
      GH_REPO: owner/repo
    steps:
      - run: $VIEW"
expect "R8: job-level GH_REPO counts" 0 gh-repo "$WORK/jobenv.yml"

echo "0010-MADR D7: permissions"
expect "release workflow, permissions" 0 permissions "$WORKFLOW"
expect "ci workflow, permissions" 0 permissions "$CI"
awk '
	/^permissions:/ { skip = 1; next }
	skip && /^[^ ]/ { skip = 0 }
	!skip { print }
' "$CI" >"$WORK/noperms.yml"
expect "a workflow without top-level permissions" 1 permissions "$WORK/noperms.yml"

echo "0010-MADR D6: comments and paths"
# The literal $TAG in planted YAML is workflow content, not shell here.
# shellcheck disable=SC2016
plant "$WORK/hashquote.yml" '      - name: Hash inside quotes
        run: |
          echo "build #1"; gh release create "$TAG"'
expect "a # inside quotes hides no gh call" 1 gh-repo "$WORK/hashquote.yml"
# shellcheck disable=SC2016
plant "$WORK/abspath.yml" '      - name: gh by path
        run: /usr/bin/gh release view "$TAG"'
expect "gh called by path" 1 gh-repo "$WORK/abspath.yml"
# shellcheck disable=SC2016
plant "$WORK/realcomment.yml" '      - name: A real comment
        run: echo hi # gh release view "$TAG"'
expect "a real comment is still not a call" 0 gh-repo "$WORK/realcomment.yml"
# shellcheck disable=SC2016
plant "$WORK/latestflag.yml" '      - name: Latest flag without GH_REPO
        run: sh scripts/release-latest-flag.sh "$TAG"'
expect "release-latest-flag.sh without GH_REPO" 1 gh-repo "$WORK/latestflag.yml"

echo "0010-MADR D5, D9: the release workflow"
# --help exits 0 for any subcommand or field, so an exit-code probe proves
# nothing; the capability step must parse the help text instead.
if grep -nE -- '--help *(>|2>|$)' "$WORKFLOW" >/dev/null; then
	echo "  FAIL the workflow still probes gh by --help exit code"
	FAIL=$((FAIL + 1))
else
	echo "  ok   no --help exit-code probe"
	PASS=$((PASS + 1))
fi
n=$(grep -c 'release-latest-flag.sh' "$WORKFLOW" || true)
if [ "$n" -eq 2 ]; then
	echo "  ok   create and publish both take the latest flag"
	PASS=$((PASS + 1))
else
	echo "  FAIL release-latest-flag.sh is used $n times, want 2 (create, publish)"
	FAIL=$((FAIL + 1))
fi

echo "pins (0013-PLAN B4)"
expect "release workflow, pins" 0 pins "$WORKFLOW"
expect "ci workflow, pins" 0 pins "$CI"
plant "$WORK/pintag.yml" "      - name: Tag pin
        uses: actions/checkout@v7"
expect "an action pinned to a tag" 1 pins "$WORK/pintag.yml"
plant "$WORK/pinshort.yml" "      - name: Short pin
        uses: actions/checkout@3d3c42e"
expect "an action pinned to a short SHA" 1 pins "$WORK/pinshort.yml"
plant "$WORK/pinbranch.yml" "      - name: Branch pin
        uses: actions/checkout@main"
expect "an action pinned to a branch" 1 pins "$WORK/pinbranch.yml"
plant "$WORK/pin39.yml" "      - name: Short by one
        uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b"
expect "an action pinned to 39 hex characters" 1 pins "$WORK/pin39.yml"
plant "$WORK/pinupper.yml" "      - name: Upper case
        uses: actions/checkout@3D3C42E5AAC5BA805825DA76410C181273BA90B1"
expect "an action pinned to upper-case hex" 1 pins "$WORK/pinupper.yml"
plant "$WORK/pinfull.yml" "      - name: Full pin
        uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1"
expect "an action pinned to a full SHA" 0 pins "$WORK/pinfull.yml"
plant "$WORK/pinlocal.yml" "      - name: Local action
        uses: ./.github/actions/local"
expect "a local action is exempt" 0 pins "$WORK/pinlocal.yml"
cat >"$WORK/reusable.yml" <<'EOF'
name: Reusable
on: push
permissions:
  contents: read
jobs:
  local:
    uses: ./.github/workflows/build-selfupdate-release.yml
  remote:
    uses: maccavelli/go-selfupdate-lib/.github/workflows/build-selfupdate-release.yml@3d3c42e5aac5ba805825da76410c181273ba90b1
EOF
expect "local and SHA-pinned reusable workflows" 0 pins "$WORK/reusable.yml"
sed 's/@3d3c42e5aac5ba805825da76410c181273ba90b1$/@v1.9.0/' "$WORK/reusable.yml" >"$WORK/reusabletag.yml"
grep -q '@v1.9.0$' "$WORK/reusabletag.yml"
expect "a reusable workflow pinned to a tag" 1 pins "$WORK/reusabletag.yml"

printf '\n%d passed, %d failed\n' "$PASS" "$FAIL"
[ "$FAIL" -eq 0 ]
