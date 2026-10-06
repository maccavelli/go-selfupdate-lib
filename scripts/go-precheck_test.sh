#!/bin/sh
# Offline tests for scripts/go-precheck.sh (docs/decisions/0010-MADR-remediate-second-debugging-pass-findings.md
# D3 and D4; docs/decisions/0013-PLAN-build-and-stage-release-workflow.md D4).
# The Go tools are stubs on PATH, in a throwaway git repository, so each case
# controls what go vet, go test and govulncheck report.
set -eu

ROOT=$(cd -- "$(dirname "$0")/.." && pwd)
SCRIPT="${SCRIPT:-$ROOT/scripts/go-precheck.sh}"
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

PASS=0
FAIL=0
ok() { echo "  ok   $1"; PASS=$((PASS + 1)); }
bad() { echo "  FAIL $1"; FAIL=$((FAIL + 1)); }
# rc0 LABEL: ok when the last run exited 0.
rc0() { if [ "$rc" -eq 0 ]; then ok "$1"; else bad "$1: rc=$rc"; fi; }

# Stubs. Each records its arguments in $WORK/calls; go test exits with
# $STUB_TEST_RC, go -C DIR test with $STUB_NESTED_RC, and govulncheck exits
# with $STUB_VULN_RC, after printing a network error when STUB_VULN_NET=1.
STUBS="$WORK/bin"
mkdir -p "$STUBS"
cat >"$STUBS/go" <<'EOF'
#!/bin/sh
echo "go $*" >>"$CALLS"
case "$1" in
test) exit "${STUB_TEST_RC:-0}" ;;
-C) [ "$3" = test ] && exit "${STUB_NESTED_RC:-0}" ;;
esac
exit 0
EOF
cat >"$STUBS/gofmt" <<'EOF'
#!/bin/sh
echo "gofmt $*" >>"$CALLS"
EOF
cat >"$STUBS/golangci-lint" <<'EOF'
#!/bin/sh
exit 0
EOF
cat >"$STUBS/govulncheck" <<'EOF'
#!/bin/sh
echo "govulncheck $*" >>"$CALLS"
[ "${STUB_VULN_NET:-0}" = 1 ] && echo "dial tcp: lookup vuln.go.dev: no such host"
exit "${STUB_VULN_RC:-0}"
EOF
chmod +x "$STUBS/go" "$STUBS/gofmt" "$STUBS/golangci-lint" "$STUBS/govulncheck"

# repo: a fresh repository holding pkg/a.go and pkg/b.go.
repo() {
	r="$WORK/repo-$1"
	mkdir -p "$r/pkg"
	printf 'package pkg\n' >"$r/pkg/a.go"
	printf 'package pkg\n' >"$r/pkg/b.go"
	git -C "$r" init -q
	git -C "$r" add pkg
	echo "$r"
}

# run DIR [ENV=VALUE ...] -- ARG...: run the script in DIR, with the stubs
# first on PATH, and leave its exit status in $rc and its output in $out.
run() {
	dir="$1"
	shift
	envs=""
	while [ "$1" != "--" ]; do
		envs="$envs $1"
		shift
	done
	shift
	: >"$WORK/calls"
	rc=0
	# shellcheck disable=SC2086 # $envs is a list of VAR=value words.
	out=$(cd "$dir" && env -u BASH_ENV PATH="$STUBS:$PATH" CALLS="$WORK/calls" \
		GOLANGCI_LINT="$STUBS/golangci-lint" $envs bash "$SCRIPT" "$@" 2>&1) || rc=$?
}

# D3: a deleted file still has its package checked.
r=$(repo deleted)
rm "$r/pkg/a.go"
run "$r" STUB_TEST_RC=1 -- pkg/a.go
if [ "$rc" -eq 1 ] && grep -q '^go test \./pkg$' "$WORK/calls"; then
	ok "a deleted file's package is tested, and its failure fails the check"
else
	bad "deleted file: rc=$rc out=[$out] calls=[$(tr '\n' ';' <"$WORK/calls")]"
fi

# D3: a deleted package falls back to ./... .
r=$(repo gone)
rm -r "$r/pkg"
run "$r" -- pkg/a.go
if [ "$rc" -eq 0 ] && grep -q '^go vet \./\.\.\.$' "$WORK/calls" && ! grep -q '^gofmt' "$WORK/calls"; then
	ok "a deleted package falls back to ./..., and gofmt is not run on nothing"
else
	bad "deleted package: rc=$rc out=[$out] calls=[$(tr '\n' ';' <"$WORK/calls")]"
fi

# Control: an existing file is formatted, vetted and tested.
r=$(repo control)
run "$r" -- pkg/a.go
if [ "$rc" -eq 0 ] && grep -q '^gofmt -l pkg/a.go$' "$WORK/calls" && grep -q '^go test \./pkg$' "$WORK/calls"; then
	ok "an existing file is formatted, vetted and tested"
else
	bad "control: rc=$rc out=[$out]"
fi

# Non-Go arguments alone are still nothing to check.
run "$r" -- README.md
case "$out" in
*"no Go files to check"*) rc0 "non-Go arguments alone: nothing to check" ;;
*) bad "non-Go: rc=$rc out=[$out]" ;;
esac

# D4: an unreachable vulnerability database fails, unless skipped.
run "$r" STUB_VULN_RC=1 STUB_VULN_NET=1 -- pkg/a.go
if [ "$rc" -eq 2 ]; then
	ok "an unreachable vulnerability database fails with exit 2"
else
	bad "unreachable database: rc=$rc out=[$out]"
fi
run "$r" GO_PRECHECK_SKIP_VULN=1 -- pkg/a.go
case "$out" in
*"(gofmt, golangci-lint, go vet, go test)."*) rc0 "a skipped govulncheck is not claimed in the summary" ;;
*) bad "skip summary: rc=$rc out=[$out]" ;;
esac
run "$r" -- pkg/a.go
case "$out" in
*"(gofmt, golangci-lint, go vet, go test, govulncheck)."*) rc0 "a run govulncheck is named in the summary" ;;
*) bad "vuln summary: rc=$rc out=[$out]" ;;
esac

# 0013 D4: a file in a nested module is vetted and tested in that module,
# and the main module's packages still in the main module.
nested() {
	r=$(repo "$1")
	mkdir -p "$r/fix/cmd/x"
	printf 'module example.com/fix\n' >"$r/fix/go.mod"
	printf 'package main\n' >"$r/fix/cmd/x/main.go"
	printf 'package fix\n' >"$r/fix/root.go"
	git -C "$r" add fix
	echo "$r"
}
r=$(nested nested)
run "$r" -- fix/cmd/x/main.go fix/root.go pkg/a.go
if [ "$rc" -eq 0 ] && grep -q '^go -C fix vet \. \./cmd/x$' "$WORK/calls" &&
	grep -q '^go -C fix test \. \./cmd/x$' "$WORK/calls" && grep -q '^go test \./pkg$' "$WORK/calls" &&
	! grep -q '^go vet .*fix' "$WORK/calls"; then
	ok "a nested module's files are vetted and tested in that module"
else
	bad "nested: rc=$rc out=[$out] calls=[$(tr '\n' ';' <"$WORK/calls")]"
fi
run "$r" STUB_NESTED_RC=1 -- fix/cmd/x/main.go
if [ "$rc" -eq 1 ]; then
	ok "a nested module's failing test fails the check"
else
	bad "nested failure: rc=$rc out=[$out]"
fi
run "$r" --
if [ "$rc" -eq 0 ] && grep -q '^go vet \./\.\.\.$' "$WORK/calls" && grep -q '^go -C fix vet \./\.\.\.$' "$WORK/calls"; then
	ok "with no arguments, every module holding a tracked Go file is checked"
else
	bad "nested, no arguments: rc=$rc calls=[$(tr '\n' ';' <"$WORK/calls")]"
fi
rm -r "$r/fix/cmd/x"
run "$r" -- fix/cmd/x/main.go
if [ "$rc" -eq 0 ] && grep -q '^go -C fix vet \./\.\.\.$' "$WORK/calls" && ! grep -q '^go vet' "$WORK/calls"; then
	ok "a deleted package in a nested module falls back to that module's ./..."
else
	bad "nested deletion: rc=$rc calls=[$(tr '\n' ';' <"$WORK/calls")]"
fi

echo "go-precheck_test: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
