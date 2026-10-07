#!/usr/bin/env bash
# The full pre-commit gate: every check a commit in this repository needs,
# one step at a time, with each step's exit status captured before anything
# reads its output
# (docs/decisions/0015-PLAN-remediate-third-debugging-pass-findings.md R1).
# It replaces the session gate the 0010–0014 PLANs cite as gate.sh.
#
# Usage: gate.sh   (or make gate)
#
# Each step writes its output to $GATE_OUT/<step>.txt and prints one line,
# "<step> rc=<N> <its last line>". The run ends with overall=0 or overall=1,
# and exits with that value.
#
#   GATE_OUT   where the step outputs go; default a new mktemp -d, printed
#              first.
#   GATE_SKIP  comma-separated steps to skip, such as fuzz,vuln offline; a
#              skipped step prints "<step> skipped". Records must name any
#              skip.
#   FUZZTIME   passed to make fuzz; default 20s.
#
# The ids step runs scripts/check-docs.sh --ids on the changed and untracked
# files, and needs the pre-push guard's deny list (DISCLOSURE_DENY, or
# ~/.config/git/disclosure-deny).
# shellcheck disable=SC2329 # the step functions run through step's "$@"
set -uo pipefail

ROOT=$(cd -- "$(dirname -- "$0")/.." && pwd) || exit 2
cd "$ROOT" || exit 2
if [ -n "${GATE_OUT:-}" ]; then
	mkdir -p "$GATE_OUT" || exit 2
else
	GATE_OUT=$(mktemp -d "${TMPDIR:-/tmp}/gate.XXXXXX") || exit 2
fi
echo "gate output: $GATE_OUT"
overall=0

# need TOOL...: fail, naming it, when a tool is not on PATH.
need() {
	local t
	for t in "$@"; do
		if ! command -v "$t" >/dev/null 2>&1; then
			echo "gate: $t is not on PATH"
			return 127
		fi
	done
}

skipped() {
	case ",${GATE_SKIP:-}," in
	*",$1,"*) return 0 ;;
	esac
	return 1
}

# step NAME CMD...: run CMD, keep its output, and report its status.
step() {
	local name=$1 rc
	shift
	if skipped "$name"; then
		printf '%-11s skipped\n' "$name"
		return
	fi
	"$@" >"$GATE_OUT/$name.txt" 2>&1
	rc=$?
	printf '%-11s rc=%s  %s\n' "$name" "$rc" "$(tail -n 1 "$GATE_OUT/$name.txt" | cut -c1-110)"
	[ "$rc" -eq 0 ] || overall=1
}

run_go() {
	need go || return
	go "$@"
}

run_make() {
	need make || return
	make "$@"
}

gofmt_clean() {
	local files
	need gofmt || return
	files=$(gofmt -l .) || return
	if [ -n "$files" ]; then
		printf '%s\n' "$files"
		echo "gofmt: the files above need formatting"
		return 1
	fi
	echo "gofmt: clean"
}

# script_tests: every scripts/*_test.sh but this gate's own, which would run
# the gate again.
script_tests() {
	local t
	for t in scripts/*_test.sh; do
		[ "$(basename "$t")" = gate_test.sh ] && continue
		if ! bash "$t" >"$GATE_OUT/scripts-$(basename "$t" .sh).txt" 2>&1; then
			echo "FAIL $t"
			return 1
		fi
	done
	echo "all script tests passed"
}

shell_check() {
	need shellcheck || return
	shellcheck scripts/*.sh
}

cross_vet() {
	local p
	need go || return
	for p in freebsd/amd64 openbsd/amd64 linux/386 windows/amd64; do
		if ! CGO_ENABLED=0 GOOS=${p%/*} GOARCH=${p#*/} go vet ./...; then
			echo "go vet failed for $p"
			return 1
		fi
	done
	echo "cross go vet clean"
}

# doc_links: every tracked Markdown file. Tracked names hold no spaces.
doc_links() {
	local files rc
	need git || return
	files=$(git ls-files -- '*.md') || return
	set -f
	# shellcheck disable=SC2086 # one word per tracked file name
	scripts/check-docs.sh --links $files
	rc=$?
	set +f
	return "$rc"
}

# doc_ids: the files changed since HEAD, and the untracked ones.
doc_ids() {
	local files rc
	need git sort || return
	files=$({ git diff --name-only --diff-filter=d HEAD && git ls-files -o --exclude-standard; } | sort -u) || return
	if [ -z "$files" ]; then
		echo "ids: no changed files"
		return 0
	fi
	set -f
	# shellcheck disable=SC2086 # one word per file name
	scripts/check-docs.sh --ids $files
	rc=$?
	set +f
	return "$rc"
}

step gofmt gofmt_clean
step lint run_make lint
step vet run_go vet ./...
step race run_go test -race -count=1 ./...
step shuffle run_go test -shuffle=on -count=2 ./...
step tidy run_go mod tidy -diff
step apicheck run_make apicheck
step fuzz run_make fuzz "FUZZTIME=${FUZZTIME:-20s}"
step vuln run_make vuln
step scripts script_tests
step shellcheck shell_check
step crossvet cross_vet
step links doc_links
step ids doc_ids

echo "overall=$overall"
exit "$overall"
