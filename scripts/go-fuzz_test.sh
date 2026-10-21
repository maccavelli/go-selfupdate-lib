#!/usr/bin/env bash
# Offline tests for go-fuzz.sh, on throwaway modules in a temporary directory
# (docs/decisions/0004-PLAN-h2-fuzzing-and-manifest-differential.md Step 4).
#
# The failing-target case is the one that matters: a fuzz gate that reported
# success while a target failed would be no gate at all.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
FUZZ="${FUZZ:-$ROOT/scripts/go-fuzz.sh}"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

PASS=0
FAIL=0

check() { # name want got
	if [ "$2" = "$3" ]; then
		echo "  ok   $1"
		PASS=$((PASS + 1))
	else
		echo "  FAIL $1: want $2, got $3"
		FAIL=$((FAIL + 1))
	fi
}

# module DIR NAME BODY: a module whose package has one test file.
module() {
	mkdir -p "$1"
	printf 'module example.com/%s\n\ngo 1.26.10\n' "$2" >"$1/go.mod"
	printf 'package %s\n\nimport "testing"\n\n%s\n' "$2" "$3" >"$1/fz_test.go"
}

# run DIR ARGS...: the script's exit status, with output in $WORK/out.
run() {
	local dir="$1"
	shift
	set +e
	(cd "$dir" && "$FUZZ" "$@") >"$WORK/out" 2>&1
	local rc=$?
	set -e
	echo "$rc"
}

module "$WORK/clean" clean '
func FuzzA(f *testing.F) {
	f.Add([]byte("a"))
	f.Fuzz(func(t *testing.T, b []byte) { _ = len(b) })
}

func FuzzB(f *testing.F) {
	f.Add("b")
	f.Fuzz(func(t *testing.T, s string) { _ = len(s) })
}'

# 1. Two clean targets, two required: both are fuzzed, and it passes.
check "clean targets pass" 0 "$(run "$WORK/clean" -t 1s -m 2 ./)"
check "both targets were fuzzed" 2 "$(grep -c '^go-fuzz: Fuzz[AB] for 1s ' "$WORK/out" || true)"

# 1b. -z reaches go test as -fuzzminimizetime, through a recording GO.
REAL_GO="$(command -v go)"
mkdir -p "$WORK/bin"
printf '#!/usr/bin/env bash\nprintf "%%s\\n" "$*" >>"%s"\nexec "%s" "$@"\n' "$WORK/go-args" "$REAL_GO" >"$WORK/bin/go"
chmod +x "$WORK/bin/go"
rc="$(GO="$WORK/bin/go" run "$WORK/clean" -t 1s -z 3s -m 2 ./)"
check "a recorded run passes" 0 "$rc"
check "-z reaches each fuzz run" 2 "$(grep -c -- '-fuzzminimizetime 3s' "$WORK/go-args" || true)"

# 2. Fewer targets than required: refused, naming the count.
check "too few targets is refused" 1 "$(run "$WORK/clean" -t 1s -m 3 ./)"
check "the refusal names the count" 1 "$(grep -c 'found 2 fuzz targets' "$WORK/out" || true)"

# 3. THE CASE THAT MATTERS. A target that fails on any input but its seed:
#    the fuzzer finds one at once, so the script must fail and Go must leave
#    the input behind.
module "$WORK/boom" boom '
func FuzzBoom(f *testing.F) {
	f.Add([]byte("a"))
	f.Fuzz(func(t *testing.T, b []byte) {
		if string(b) != "a" {
			t.Fatalf("boom on %q", b)
		}
	})
}'
rc="$(run "$WORK/boom" -t 5s -m 1 ./)"
if [ "$rc" -ne 0 ]; then rc=nonzero; fi
check "a failing target fails the run" nonzero "$rc"
saved=0
if [ -d "$WORK/boom/testdata/fuzz/FuzzBoom" ]; then
	saved="$(find "$WORK/boom/testdata/fuzz/FuzzBoom" -type f | wc -l | tr -d ' ')"
fi
check "the failing input is saved" 1 "$saved"

# 4. Usage errors.
check "no package is a usage error" 2 "$(run "$WORK/clean")"
check "a non-numeric minimum is a usage error" 2 "$(run "$WORK/clean" -m x ./)"
check "a zero minimum is a usage error" 2 "$(run "$WORK/clean" -m 0 ./)"
check "an unknown flag is a usage error" 2 "$(run "$WORK/clean" -z ./)"

echo "go-fuzz_test: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
