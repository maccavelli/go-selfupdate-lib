#!/bin/sh
# Offline fixtures for scripts/check-installers.sh. The templates serve as a
# staged set: their sample values are a rendering for maccavelli/relay at
# v1.2.3. A stub pwsh stands in for PSScriptAnalyzer, which CI runs for
# real. Needs shellcheck and dash.
set -eu

ROOT=$(cd -- "$(dirname "$0")/.." && pwd)
SCRIPT="$ROOT/scripts/check-installers.sh"
TEMPLATES="$ROOT/internal/cmd/selfupdate-release/installer"
WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT

pass() {
	echo "ok - $1"
}

fail() {
	echo "not ok - $1" >&2
	exit 1
}

# The stub exits with $STUB_PWSH_EXIT, saying so on stderr.
mkdir -p "$WORKDIR/bin"
cat >"$WORKDIR/bin/pwsh" <<'SH'
#!/bin/sh
echo "stub pwsh: exit ${STUB_PWSH_EXIT:-0}" >&2
exit "${STUB_PWSH_EXIT:-0}"
SH
chmod +x "$WORKDIR/bin/pwsh"
PWSH="$WORKDIR/bin/pwsh"
export PWSH

staged() {
	d="$WORKDIR/$1"
	rm -rf "$d"
	mkdir -p "$d"
	cp "$TEMPLATES/install.sh" "$TEMPLATES/install.ps1" "$d/"
	printf '%064d  relay-linux-amd64\n' 0 >"$d/SHA256SUMS"
	echo "$d"
}

# expect NAME CODE TEXT ARGS...: the script exits CODE, and with TEXT on
# its output when TEXT is not empty.
expect() {
	name=$1
	code=$2
	text=$3
	shift 3
	rc=0
	"$SCRIPT" "$@" >"$WORKDIR/out" 2>&1 || rc=$?
	[ "$rc" -eq "$code" ] || {
		cat "$WORKDIR/out" >&2
		fail "$name: exit $rc, want $code"
	}
	if [ -n "$text" ] && ! grep -qF -- "$text" "$WORKDIR/out"; then
		cat "$WORKDIR/out" >&2
		fail "$name: no \"$text\" in the output"
	fi
	pass "$name"
}

expect "the templates are clean" 0 "check-installers: clean" "$TEMPLATES/install.sh" "$TEMPLATES/install.ps1"

d=$(staged ok)
expect "a staged set rendered for its repository and tag" 0 "check-installers: clean" \
	--staged "$d" --repository maccavelli/relay --tag v1.2.3
expect "another repository" 1 "install.sh is not rendered for maccavelli/other" \
	--staged "$d" --repository maccavelli/other --tag v1.2.3
expect "another repository, in install.ps1" 1 "install.ps1 is not rendered for maccavelli/other" \
	--staged "$d" --repository maccavelli/other --tag v1.2.3
expect "another tag" 1 "install.sh is not rendered for v9.9.9" \
	--staged "$d" --repository maccavelli/relay --tag v9.9.9
expect "a repository that only shares a prefix" 1 "is not rendered for maccavelli/rel" \
	--staged "$d" --repository maccavelli/rel --tag v1.2.3

d=$(staged no-ps1)
rm "$d/install.ps1"
expect "a staged set without install.ps1" 1 "has no install.ps1" \
	--staged "$d" --repository maccavelli/relay --tag v1.2.3
d=$(staged none)
rm "$d/install.sh" "$d/install.ps1"
expect "a staged set without installers (a spec without installer)" 1 "has no install.sh" \
	--staged "$d" --repository maccavelli/relay --tag v1.2.3

d=$(staged listed)
printf '%064d  install.sh\n' 1 >>"$d/SHA256SUMS"
expect "an installer listed in SHA256SUMS" 1 "SHA256SUMS lists an installer" \
	--staged "$d" --repository maccavelli/relay --tag v1.2.3

cp "$TEMPLATES/install.sh" "$WORKDIR/finding.sh"
# shellcheck disable=SC2016 # the planted line is literal on purpose.
printf 'unquoted() {\n\tcp $1 $2\n}\n' >>"$WORKDIR/finding.sh"
expect "a shellcheck finding" 1 "finding.sh: shellcheck" "$WORKDIR/finding.sh"

cp "$TEMPLATES/install.sh" "$WORKDIR/broken.sh"
printf 'if true; then\n' >>"$WORKDIR/broken.sh"
expect "a syntax error" 1 "broken.sh: dash -n" "$WORKDIR/broken.sh"

STUB_PWSH_EXIT=1 expect "a PSScriptAnalyzer finding" 1 "install.ps1: PSScriptAnalyzer" "$TEMPLATES/install.ps1"
STUB_PWSH_EXIT=2 expect "no PSScriptAnalyzer" 2 "" "$TEMPLATES/install.ps1"
PWSH="$WORKDIR/bin/no-such-pwsh" expect "no pwsh" 2 "no-such-pwsh is required" "$TEMPLATES/install.ps1"

expect "no arguments" 2 "usage:"
expect "a half --staged" 2 "usage:" --staged "$WORKDIR/ok" --repository maccavelli/relay
expect "another kind of file" 1 "neither .sh nor .ps1" "$ROOT/go.mod"

echo "check-installers_test: all assertions hold"
