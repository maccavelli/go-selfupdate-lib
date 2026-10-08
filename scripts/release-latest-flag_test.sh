#!/bin/sh
# Offline tests for scripts/release-latest-flag.sh, with a stubbed gh
# (docs/decisions/0010-MADR-remediate-second-debugging-pass-findings.md D9).
set -eu

ROOT=$(cd -- "$(dirname "$0")/.." && pwd)
SCRIPT="${SCRIPT:-$ROOT/scripts/release-latest-flag.sh}"
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

# The stub answers `gh release view --json tagName --jq .tagName` from
# STUB_LATEST: a tag, "none" (no release yet), or "error"; and `gh release
# list` from STUB_LIST: space-separated tags, one per line out, or "error".
mkdir -p "$WORK/bin"
cat >"$WORK/bin/gh" <<'EOF'
#!/bin/sh
echo "gh $*" >>"$CALLS"
if [ "$1 $2" = "release list" ]; then
	case "${STUB_LIST:-}" in
	error) echo "HTTP 502: Bad Gateway" >&2; exit 1 ;;
	*) for t in ${STUB_LIST:-}; do echo "$t"; done; exit 0 ;;
	esac
fi
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
	out=$(env PATH="$WORK/bin:$PATH" CALLS="$WORK/calls" STUB_LATEST="$2" STUB_LIST="${STUB_LIST:-}" sh "$SCRIPT" "$3" 2>/dev/null) || rc=$?
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
# A latest release that is not vX.Y.Z: the highest published stable vX.Y.Z
# decides (docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md F8).
STUB_LIST="v1.5.0 v1.4.1 nightly-7 v2.0.0-rc.1"
expect "legacy latest: backport below the highest" legacy-2024 v1.4.2 0 "--latest=false"
expect "legacy latest: above the highest" legacy-2024 v1.6.0 0 ""
STUB_LIST="nightly-7"
expect "legacy latest: no vX.Y.Z published" legacy-2024 v1.0.0 0 ""
STUB_LIST="error"
expect "legacy latest: the list fails" legacy-2024 v1.0.0 1 ""
STUB_LIST=""
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
