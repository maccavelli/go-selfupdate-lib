#!/bin/sh
# Offline tests for scripts/check-docs.sh
# (docs/decisions/0015-PLAN-remediate-third-debugging-pass-findings.md R1).
# The fixtures that hold an identifier or a home path are composed here at
# run time from parts, so this file itself passes --ids.
set -eu

ROOT=$(cd -- "$(dirname "$0")/.." && pwd)
SCRIPT="${SCRIPT:-$ROOT/scripts/check-docs.sh}"
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

PASS=0
FAIL=0
ok() { echo "  ok   $1"; PASS=$((PASS + 1)); }
bad() { echo "  FAIL $1"; FAIL=$((FAIL + 1)); }

D="$WORK/docs"
mkdir -p "$D/sub"
cat >"$D/a.md" <<'EOF'
# Title

## Mixed Case Heading

## Mixed Case Heading

[good](b.md) [anchor](b.md#the-heading) [self](#mixed-case-heading)
[second](#mixed-case-heading-1) [up](sub/c.md#c) [web](https://example.com/missing.md)
Inline code is ignored: `[code](missing.md)`.

```
[fenced](missing.md)
```
EOF
printf '%s\n' '# The Heading' >"$D/b.md"
printf '%s\n' '# C' '' '[back](../a.md#title)' >"$D/sub/c.md"
printf '%s\n' '[gone](nope.md)' >"$D/bad.md"
printf '%s\n' '[x](b.md#no-such-heading)' >"$D/badanchor.md"

u=example
printf '%s\t%s\n' "${u}user" '<user>' >"$WORK/deny"
printf '%s\t%s\t%s\n' 'idonly' '<someone>' 'identity' >>"$WORK/deny"
mac="/Us""ers"
lin="/ho""me"
printf '%s\n' "this file names ${u}user once" >"$D/leak.md"
printf '%s\n' "a placeholder path: $mac/<user>/x and $lin/<user>/y" \
	"a URL: https://example.com${lin}/bob and a lowercase path /users/shared/x" \
	'an identity-only rule does not apply to content: idonly' >"$D/clean.md"
printf '%s\n' "a real path: $mac/bob/x" >"$D/machome.md"
printf '%s\n' "a real path: $lin/bob/y" >"$D/linhome.md"

# run [ENV=VALUE ...] -- ARG...: run the checker in $D; its exit status goes
# to $rc and its output to $out.
run() {
	envs=""
	while [ "$1" != "--" ]; do
		envs="$envs $1"
		shift
	done
	shift
	rc=0
	# shellcheck disable=SC2086 # $envs is a list of VAR=value words.
	out=$(cd "$D" && env DISCLOSURE_DENY="$WORK/deny" $envs bash "$SCRIPT" "$@" 2>&1) || rc=$?
}
has() { printf '%s\n' "$out" | grep -q -- "$1"; }

run -- --links a.md b.md sub/c.md
if [ "$rc" -eq 0 ]; then ok "good links, anchors, a repeated heading, code and URLs pass"; else bad "good links: rc=$rc out=[$out]"; fi

run -- --links bad.md
if [ "$rc" -eq 1 ] && has 'bad.md:1' && has 'nope.md'; then ok "a link to a missing file fails, located"; else bad "missing file: rc=$rc out=[$out]"; fi

run -- --links badanchor.md
if [ "$rc" -eq 1 ] && has 'badanchor.md:1' && has 'no-such-heading'; then ok "a missing anchor fails, located"; else bad "missing anchor: rc=$rc out=[$out]"; fi

run -- --ids clean.md
if [ "$rc" -eq 0 ]; then ok "placeholders, URLs, lowercase paths and identity rules pass"; else bad "clean ids: rc=$rc out=[$out]"; fi

run -- --ids leak.md
if [ "$rc" -eq 1 ] && has 'leak.md:1' && has '<user>' && ! has "${u}user"; then
	ok "a deny-list match fails, by placeholder, without printing it"
else
	bad "deny list: rc=$rc out=[$out]"
fi

run -- --ids machome.md linhome.md
if [ "$rc" -eq 1 ] && has 'machome.md:1' && has 'linhome.md:1' && ! has 'bob'; then
	ok "a macOS or Linux home path fails, without printing it"
else
	bad "home paths: rc=$rc out=[$out]"
fi

run DISCLOSURE_DENY="$WORK/none" -- --ids clean.md
if [ "$rc" -eq 2 ] && has 'no deny list'; then ok "no deny list is an error"; else bad "no deny list: rc=$rc out=[$out]"; fi

run DISCLOSURE_DENY="$WORK/none" -- --ids --ids-optional clean.md
if [ "$rc" -eq 0 ]; then ok "--ids-optional checks home paths alone"; else bad "optional, clean: rc=$rc out=[$out]"; fi

run DISCLOSURE_DENY="$WORK/none" -- --ids --ids-optional machome.md
if [ "$rc" -eq 1 ]; then ok "--ids-optional still fails a home path"; else bad "optional, home: rc=$rc out=[$out]"; fi

printf '%s\n' 'no tab here' >"$WORK/malformed"
run DISCLOSURE_DENY="$WORK/malformed" -- --ids clean.md
if [ "$rc" -eq 2 ]; then ok "a malformed deny list is an error"; else bad "malformed: rc=$rc out=[$out]"; fi

run -- --nonsense a.md
if [ "$rc" -eq 2 ]; then ok "an unknown mode is a usage error"; else bad "usage: rc=$rc out=[$out]"; fi

echo "check-docs_test: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
