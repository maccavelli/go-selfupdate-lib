#!/bin/sh
# Offline tests for scripts/gate.sh
# (docs/decisions/0015-PLAN-remediate-third-debugging-pass-findings.md R1).
# The gate runs in a throwaway git repository holding a copy of it. go, make,
# gofmt and shellcheck are stubs, and PATH holds only them and the few real
# tools the gate itself needs, so each case controls what every step sees.
set -eu

ROOT=$(cd -- "$(dirname "$0")/.." && pwd)
SCRIPT="${SCRIPT:-$ROOT/scripts/gate.sh}"
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

PASS=0
FAIL=0
ok() { echo "  ok   $1"; PASS=$((PASS + 1)); }
bad() { echo "  FAIL $1"; FAIL=$((FAIL + 1)); }

# SYS: links to the real tools the gate uses, and nothing else, so a tool
# the stubs leave out is missing.
SYS="$WORK/sys"
mkdir -p "$SYS"
for t in bash sh env git dirname basename mkdir mktemp tail cut cat rm sort; do
	p=$(command -v "$t") || {
		echo "gate_test: $t is required" >&2
		exit 2
	}
	ln -s "$p" "$SYS/$t"
done

# Stubs: make exits with $STUB_RC_<target>; go with $STUB_RC_go; gofmt
# prints $STUB_GOFMT_OUT; shellcheck exits 0. Each logs to $CALLS.
STUBS="$WORK/bin"
mkdir -p "$STUBS"
cat >"$STUBS/make" <<'EOF'
#!/bin/sh
echo "make $*" >>"$CALLS"
eval "rc=\${STUB_RC_$1:-0}"
exit "$rc"
EOF
cat >"$STUBS/go" <<'EOF'
#!/bin/sh
echo "go $*" >>"$CALLS"
exit "${STUB_RC_go:-0}"
EOF
cat >"$STUBS/gofmt" <<'EOF'
#!/bin/sh
echo "gofmt $*" >>"$CALLS"
[ -n "${STUB_GOFMT_OUT:-}" ] && echo "$STUB_GOFMT_OUT"
exit 0
EOF
cat >"$STUBS/shellcheck" <<'EOF'
#!/bin/sh
echo "shellcheck $*" >>"$CALLS"
exit 0
EOF
chmod +x "$STUBS/make" "$STUBS/go" "$STUBS/gofmt" "$STUBS/shellcheck"
# A second stub directory without shellcheck.
NOSC="$WORK/bin-nosc"
mkdir -p "$NOSC"
for t in make go gofmt; do ln -s "$STUBS/$t" "$NOSC/$t"; done

# repo NAME: a fresh repository holding the gate, a document checker stub
# (exit $STUB_RC_docs), a passing script test, and a failing gate_test.sh,
# which the gate must not run.
repo() {
	r="$WORK/repo-$1"
	mkdir -p "$r/scripts"
	cp "$SCRIPT" "$r/scripts/gate.sh"
	cat >"$r/scripts/check-docs.sh" <<'EOF'
#!/bin/sh
echo "check-docs $*" >>"$CALLS"
exit "${STUB_RC_docs:-0}"
EOF
	printf '%s\n' '#!/bin/sh' 'exit 0' >"$r/scripts/pass_test.sh"
	printf '%s\n' '#!/bin/sh' 'exit 1' >"$r/scripts/gate_test.sh"
	chmod +x "$r/scripts/"*.sh
	printf '# demo\n' >"$r/README.md"
	git -C "$r" init -q
	git -C "$r" add .
	git -C "$r" -c user.name=t -c user.email=t@example.invalid commit -q -m init
	echo "$r"
}

# run DIR BINDIR [ENV=VALUE ...]: run the gate in DIR with BINDIR and SYS as
# PATH; its exit status goes to $rc and its output to $out.
run() {
	dir="$1"
	bin="$2"
	shift 2
	: >"$WORK/calls"
	rc=0
	out=$(cd "$dir" && "$SYS/env" -i HOME="$WORK" PATH="$bin:$SYS" CALLS="$WORK/calls" \
		GATE_OUT="$WORK/out" "$@" "$SYS/bash" scripts/gate.sh 2>&1) || rc=$?
}

has() { printf '%s\n' "$out" | grep -Eq "$1"; }

r=$(repo green)
run "$r" "$STUBS"
if [ "$rc" -eq 0 ] && has '^overall=0$' && ! has 'rc=[1-9]'; then
	ok "every step green: overall=0, exit 0"
else
	bad "all green: rc=$rc out=[$out]"
fi
steps="gofmt lint vet race shuffle tidy apicheck fuzz vuln scripts shellcheck crossvet links ids"
missing=""
for s in $steps; do has "^$s +rc=0" || missing="$missing $s"; done
if [ -z "$missing" ]; then ok "all fourteen steps report"; else bad "steps not reported:$missing"; fi
if [ -f "$WORK/out/lint.txt" ] && [ -f "$WORK/out/ids.txt" ]; then
	ok "each step's output is kept in GATE_OUT"
else
	bad "GATE_OUT: $(ls "$WORK/out" 2>&1)"
fi
if grep -q '^make fuzz FUZZTIME=20s$' "$WORK/calls"; then
	ok "fuzz runs for FUZZTIME, 20s by default"
else
	bad "fuzz call: $(grep fuzz "$WORK/calls" || true)"
fi

run "$r" "$STUBS" STUB_RC_lint=1
if [ "$rc" -eq 1 ] && has '^lint +rc=1' && has '^overall=1$'; then
	ok "a failing make lint fails the gate"
else
	bad "lint: rc=$rc out=[$out]"
fi

run "$r" "$STUBS" STUB_GOFMT_OUT=pkg/x.go
if [ "$rc" -eq 1 ] && has '^gofmt +rc=1'; then
	ok "an unformatted file fails the gate"
else
	bad "gofmt: rc=$rc out=[$out]"
fi

r=$(repo script)
printf '%s\n' '#!/bin/sh' 'exit 1' >"$r/scripts/x_test.sh"
run "$r" "$STUBS"
if [ "$rc" -eq 1 ] && has '^scripts +rc=1 .*x_test\.sh'; then
	ok "a failing script test is named"
else
	bad "script test: rc=$rc out=[$out]"
fi

r=$(repo nosc)
run "$r" "$NOSC"
if [ "$rc" -eq 1 ] && has '^shellcheck +rc=127 .*shellcheck is not on PATH'; then
	ok "a missing tool fails its step, named"
else
	bad "missing shellcheck: rc=$rc out=[$out]"
fi

run "$r" "$STUBS" GATE_SKIP=fuzz,vuln
if [ "$rc" -eq 0 ] && has '^fuzz +skipped$' && has '^vuln +skipped$' &&
	! grep -q '^make fuzz' "$WORK/calls"; then
	ok "GATE_SKIP skips the named steps, and says so"
else
	bad "skip: rc=$rc out=[$out]"
fi

run "$r" "$STUBS" STUB_RC_docs=1
if [ "$rc" -eq 1 ] && has '^links +rc=1'; then
	ok "a broken link fails the gate"
else
	bad "links: rc=$rc out=[$out]"
fi

echo "gate_test: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
