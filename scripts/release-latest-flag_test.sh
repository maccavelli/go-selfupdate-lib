#!/bin/sh
# Offline tests for scripts/release-latest-flag.sh, with a stubbed gh
# (docs/decisions/0010-MADR-remediate-second-debugging-pass-findings.md D9).
set -eu

ROOT=$(cd -- "$(dirname "$0")/.." && pwd)
SCRIPT="${SCRIPT:-$ROOT/scripts/release-latest-flag.sh}"
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

# The stub answers `gh release view --json tagName --jq .tagName` from
# STUB_LATEST: a tag, "none" (no release yet), or "error".
mkdir -p "$WORK/bin"
cat >"$WORK/bin/gh" <<'EOF'
#!/bin/sh
echo "gh $*" >>"$CALLS"
case "${STUB_LATEST:-}" in
none) echo "release not found" >&2; exit 1 ;;
error) echo "HTTP 502: Bad Gateway" >&2; exit 1 ;;
*) echo "$STUB_LATEST" ;;
esac
EOF
chmod +x "$WORK/bin/gh"

PASS=0
FAIL=0

# expect NAME LATEST TAG WANT-RC WANT-OUTPUT
expect() {
	: >"$WORK/calls"
	rc=0
	out=$(env PATH="$WORK/bin:$PATH" CALLS="$WORK/calls" STUB_LATEST="$2" sh "$SCRIPT" "$3" 2>/dev/null) || rc=$?
	if [ "$rc" -eq "$4" ] && [ "$out" = "$5" ]; then
		echo "  ok   $1"
		PASS=$((PASS + 1))
	else
		echo "  FAIL $1: want rc $4 [$5], got rc $rc [$out]"
		FAIL=$((FAIL + 1))
	fi
}

expect "higher stable tag" v1.4.1 v1.5.0 0 ""
expect "backport below latest" v1.5.0 v1.4.2 0 "--latest=false"
expect "numeric, not lexical: higher" v1.9.0 v1.10.0 0 ""
expect "numeric, not lexical: lower" v1.10.0 v1.9.9 0 "--latest=false"
expect "lower patch" v1.5.1 v1.5.0 0 "--latest=false"
expect "first release" none v1.0.0 0 ""
expect "gh failure" error v1.0.0 1 ""
expect "prerelease" v1.4.1 v1.6.0-rc.1 0 "--latest=false"
if [ -s "$WORK/calls" ]; then
	echo "  FAIL a prerelease asked gh: $(cat "$WORK/calls")"
	FAIL=$((FAIL + 1))
else
	echo "  ok   a prerelease asks gh nothing"
	PASS=$((PASS + 1))
fi
rc=0
sh "$SCRIPT" >/dev/null 2>&1 || rc=$?
if [ "$rc" -eq 2 ]; then
	echo "  ok   no TAG is a usage error"
	PASS=$((PASS + 1))
else
	echo "  FAIL no TAG: want rc 2, got $rc"
	FAIL=$((FAIL + 1))
fi

echo "release-latest-flag_test: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
