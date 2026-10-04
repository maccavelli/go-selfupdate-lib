#!/bin/sh
# Check that a release tag is one the self-update release workflow may
# publish (docs/decisions/0005-MADR-opt-in-prerelease-channels.md §5,
# amendments E2 and E4). It mirrors selfupdate.NewSemverPolicy:
#
#   vMAJOR.MINOR.PATCH         always, with no leading zeroes
#   vMAJOR.MINOR.PATCH-NAME.N  only when NAME is in CHANNELS-JSON, and N is
#                              a decimal with no leading zero
#
# Build metadata is never admitted. CHANNELS-JSON is a JSON array of channel
# names, most stable first, in strictly descending ASCII order, each matching
# ^[a-z][a-z0-9]{0,15}$. It defaults to [], which admits stable tags only.
#
# Usage: check-release-tag.sh TAG [CHANNELS-JSON]
# Exit 0 when the tag is admitted, 1 when it is not, 2 on a usage error,
# which includes a CHANNELS-JSON that breaks the rules above.
set -eu

if [ $# -lt 1 ] || [ $# -gt 2 ]; then
	echo "usage: check-release-tag.sh TAG [CHANNELS-JSON]" >&2
	exit 2
fi

python3 -B - "$1" "${2:-[]}" <<'PY'
import json, re, sys

tag, channels_raw = sys.argv[1:3]

# fullmatch throughout: Python's "$" also matches before a final newline
# (0004-MADR R6). [0-9], not \d: Python's \d matches any Unicode digit,
# Go's only ASCII (0010-MADR D1).
core_re = re.compile(r"v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)")
name_re = re.compile(r"[a-z][a-z0-9]{0,15}")
num_re = re.compile(r"0|[1-9][0-9]*")


def usage(msg):
    print("check-release-tag: " + msg, file=sys.stderr)
    sys.exit(2)


try:
    channels = json.loads(channels_raw)
except json.JSONDecodeError as e:
    usage("channels: invalid JSON: %s" % e)
if not isinstance(channels, list):
    usage("channels must be a JSON array")
for i, name in enumerate(channels):
    if not isinstance(name, str) or not name_re.fullmatch(name):
        usage("channel name %r must match ^[a-z][a-z0-9]{0,15}$" % (name,))
    if i > 0 and channels[i - 1] <= name:
        usage("channel %r must come after %r: list channels in descending "
              "ASCII order, most stable first" % (channels[i - 1], name))

core, dash, pre = tag.partition("-")
admitted = core_re.fullmatch(core) is not None
if admitted and dash:
    name, dot, num = pre.partition(".")
    admitted = bool(dot) and name in channels and num_re.fullmatch(num) is not None
if not admitted:
    print("check-release-tag: %r is not an admitted release tag (channels %s)"
          % (tag, json.dumps(channels)), file=sys.stderr)
    sys.exit(1)
PY
