#!/bin/sh
# Offline tests for scripts/plant-copy.sh
# (docs/decisions/0015-PLAN-remediate-third-debugging-pass-findings.md R1),
# in a throwaway git repository.
set -eu

ROOT=$(cd -- "$(dirname "$0")/.." && pwd)
SCRIPT="${SCRIPT:-$ROOT/scripts/plant-copy.sh}"
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

PASS=0
FAIL=0
ok() { echo "  ok   $1"; PASS=$((PASS + 1)); }
bad() { echo "  FAIL $1"; FAIL=$((FAIL + 1)); }

R="$WORK/repo"
mkdir -p "$R/pkg" "$WORK/tmp"
printf 'value := alpha\n' >"$R/pkg/tracked.go"
printf 'twice twice\n' >"$R/pkg/dup.go"
printf 'ignored.txt\n' >"$R/.gitignore"
git -C "$R" init -q
git -C "$R" add .
git -C "$R" -c user.name=t -c user.email=t@example.invalid commit -q -m init
printf 'new file\n' >"$R/pkg/untracked.go"
printf 'secret\n' >"$R/ignored.txt"
cp "$R/pkg/tracked.go" "$WORK/tracked.orig"

# run ARG...: run the script from inside the repository's pkg directory, with
# TMPDIR in $WORK/tmp; stdout goes to $out, stderr to $err, the status to $rc.
run() {
	rc=0
	out=$(cd "$R/pkg" && TMPDIR="$WORK/tmp" bash "$SCRIPT" "$@" 2>"$WORK/err") || rc=$?
	err=$(cat "$WORK/err")
}

run pkg/tracked.go "'alpha'" "'beta'"
copy=$(printf '%s\n' "$out" | head -n 1)
if [ "$rc" -eq 0 ] && [ -d "$copy" ]; then ok "prints the copy's path"; else bad "copy: rc=$rc out=[$out] err=[$err]"; fi
if [ "$(cat "$copy/pkg/tracked.go" 2>/dev/null)" = 'value := beta' ]; then ok "the plant lands"; else bad "planted file: $(cat "$copy/pkg/tracked.go" 2>&1)"; fi
if [ -f "$copy/pkg/untracked.go" ] && [ -f "$copy/.gitignore" ]; then ok "tracked and untracked files are copied"; else bad "copied: $(ls -a "$copy" "$copy/pkg" 2>&1)"; fi
if [ ! -e "$copy/ignored.txt" ] && [ ! -e "$copy/.git" ]; then ok "ignored files and .git are not"; else bad "extra: $(ls -a "$copy" 2>&1)"; fi
if cmp -s "$R/pkg/tracked.go" "$WORK/tracked.orig"; then ok "the source is unchanged"; else bad "the source changed"; fi
case $err in *"rm -rf $copy"*) ok "the removal hint names the copy" ;; *) bad "hint: [$err]" ;; esac

before=$(find "$WORK/tmp" -mindepth 1 -maxdepth 1 | wc -l)
run pkg/dup.go "'twice'" "'once'"
after=$(find "$WORK/tmp" -mindepth 1 -maxdepth 1 | wc -l)
if [ "$rc" -eq 1 ] && [ "$before" -eq "$after" ] && [ -z "$out" ]; then
	ok "old text found twice: exit 1, and no copy"
else
	bad "twice: rc=$rc out=[$out] err=[$err] dirs $before -> $after"
fi

run pkg/tracked.go "'absent'" "'x'"
if [ "$rc" -eq 1 ]; then ok "old text not found: exit 1"; else bad "absent: rc=$rc"; fi

run pkg/tracked.go "alpha" "'beta'"
if [ "$rc" -eq 2 ]; then ok "a non-literal argument is a usage error"; else bad "literal: rc=$rc err=[$err]"; fi

run pkg/tracked.go
if [ "$rc" -eq 2 ]; then ok "too few arguments is a usage error"; else bad "usage: rc=$rc"; fi

echo "plant-copy_test: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
