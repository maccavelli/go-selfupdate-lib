#!/bin/sh
# Lints the release installers
# (docs/decisions/0014-PLAN-shared-installer-templates.md I5): shellcheck
# (-s sh) and dash -n on each .sh file; PowerShell's parser and
# PSScriptAnalyzer (warnings and errors) on each .ps1 file.
#
# With --staged, it first checks a staged release's installers: install.sh
# and install.ps1 both present, rendered for the repository and the tag,
# and not listed in SHA256SUMS. Then it lints them.
#
# Usage: check-installers.sh FILE...
#        check-installers.sh --staged DIR --repository OWNER/NAME --tag TAG
#
# Exit 0 clean; 1 a finding; 2 a usage error or a missing tool (shellcheck,
# dash, or pwsh with PSScriptAnalyzer). PWSH names the pwsh to run.
set -eu

usage() {
	echo "usage: check-installers.sh FILE... | --staged DIR --repository OWNER/NAME --tag TAG" >&2
	exit 2
}

failed=0
finding() {
	echo "check-installers: $*" >&2
	failed=1
}

need() {
	command -v "$1" >/dev/null 2>&1 || {
		echo "check-installers: $1 is required" >&2
		exit 2
	}
}

WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

# The analyzer's runner: parse errors and findings on stderr; exit 1 for
# any, 2 when PSScriptAnalyzer is missing.
cat >"$WORK/lint.ps1" <<'PS1'
param([string]$Path)
$tokens = $null
$errors = $null
[void][System.Management.Automation.Language.Parser]::ParseFile($Path, [ref]$tokens, [ref]$errors)
foreach ($e in $errors) { [Console]::Error.WriteLine("${Path}: parse error: $e") }
if (-not (Get-Module -ListAvailable PSScriptAnalyzer)) {
    [Console]::Error.WriteLine('PSScriptAnalyzer is required')
    exit 2
}
$found = @(Invoke-ScriptAnalyzer -Path $Path -Severity Warning, Error)
foreach ($f in $found) { [Console]::Error.WriteLine("${Path}:$($f.Line): $($f.RuleName): $($f.Message)") }
if ($errors.Count -gt 0 -or $found.Count -gt 0) { exit 1 }
exit 0
PS1

lint() {
	case $1 in
	*.sh)
		need shellcheck
		need dash
		shellcheck -s sh "$1" || finding "$1: shellcheck"
		dash -n "$1" || finding "$1: dash -n"
		;;
	*.ps1)
		pwsh=${PWSH:-pwsh}
		need "$pwsh"
		rc=0
		"$pwsh" -NoProfile -NonInteractive -File "$WORK/lint.ps1" -Path "$1" || rc=$?
		case $rc in
		0) ;;
		2) exit 2 ;;
		*) finding "$1: PSScriptAnalyzer" ;;
		esac
		;;
	*) finding "$1: neither .sh nor .ps1" ;;
	esac
}

if [ "${1:-}" = --staged ]; then
	[ $# -eq 6 ] && [ "$3" = --repository ] && [ "$5" = --tag ] || usage
	dir=$2
	repository=$4
	tag=$6
	[ -d "$dir" ] || usage
	if [ -f "$dir/install.sh" ]; then
		grep -qxF "REPOSITORY='$repository'" "$dir/install.sh" || finding "$dir/install.sh is not rendered for $repository"
		grep -qxF "TAG='$tag'" "$dir/install.sh" || finding "$dir/install.sh is not rendered for $tag"
		lint "$dir/install.sh"
	else
		finding "$dir has no install.sh"
	fi
	if [ -f "$dir/install.ps1" ]; then
		grep -qF "\$Repository = '$repository'" "$dir/install.ps1" || finding "$dir/install.ps1 is not rendered for $repository"
		grep -qF "\$Tag = '$tag'" "$dir/install.ps1" || finding "$dir/install.ps1 is not rendered for $tag"
		lint "$dir/install.ps1"
	else
		finding "$dir has no install.ps1"
	fi
	if [ -f "$dir/SHA256SUMS" ] && grep -qE '  install\.(sh|ps1)$' "$dir/SHA256SUMS"; then
		finding "$dir/SHA256SUMS lists an installer"
	fi
else
	[ $# -gt 0 ] || usage
	for f in "$@"; do
		[ -f "$f" ] || usage
		lint "$f"
	done
fi
if [ "$failed" -ne 0 ]; then
	exit 1
fi
echo "check-installers: clean"
