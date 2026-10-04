#!/bin/sh
# Print the --latest flag a release of TAG must be created and published with,
# or nothing (docs/decisions/0010-MADR-remediate-second-debugging-pass-findings.md
# D9, Q7).
#
# GitHub marks a newly published release "latest" by default, and stable
# clients read the latest release. A stable tag lower than the current latest,
# such as a v1.4.x backport after v1.5.0, would otherwise become latest, and
# every client on v1.5.0 would see an older release as the update. Such a tag
# gets --latest=false. A prerelease is never latest, so it always gets
# --latest=false. A higher stable tag, or the first release, gets nothing.
#
# TAG has already passed check-release-tag.sh. GH_REPO names the repository.
#
# Usage: release-latest-flag.sh TAG
# Exit 0 with the flag (or nothing) on stdout; 1 when gh fails other than
# "release not found"; 2 on a usage error.
set -eu

if [ $# -ne 1 ]; then
	echo "usage: release-latest-flag.sh TAG" >&2
	exit 2
fi
tag="$1"

case "$tag" in
*-*)
	echo "--latest=false"
	exit 0
	;;
esac

err=$(mktemp)
trap 'rm -f "$err"' EXIT
rc=0
latest=$(gh release view --json tagName --jq .tagName 2>"$err") || rc=$?
if [ "$rc" -ne 0 ]; then
	if grep -qi 'release not found' "$err"; then
		exit 0
	fi
	echo "release-latest-flag: gh release view failed (exit $rc):" >&2
	cat "$err" >&2
	exit 1
fi

python3 -B - "$tag" "$latest" <<'PY'
import re
import sys

core = re.compile(r"v([0-9]+)\.([0-9]+)\.([0-9]+)")


def parse(tag, what):
    m = core.fullmatch(tag.split("-", 1)[0])
    if not m:
        print("release-latest-flag: %s %r is not vX.Y.Z" % (what, tag), file=sys.stderr)
        sys.exit(1)
    return tuple(int(n) for n in m.groups())


tag, latest = sys.argv[1:3]
if parse(tag, "tag") < parse(latest, "latest release"):
    print("--latest=false")
PY
