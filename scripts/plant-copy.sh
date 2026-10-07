#!/usr/bin/env bash
# Copy the working tree with one planted break, to see a new test fail
# without dirtying the tree
# (docs/decisions/0015-PLAN-remediate-third-debugging-pass-findings.md R1).
# It replaces the session helper the 0013 and 0014 PLANs cite as
# plantcopy.py.
#
# Usage: plant-copy.sh FILE OLD NEW
#
# FILE is relative to the repository's root. OLD and NEW are Python string
# literals, such as "'if err != nil {'", so any text can be written. FILE must
# hold OLD exactly once. The copy holds the tracked and the untracked files,
# not the ignored ones, and has no .git, so make apicheck cannot run in it;
# go test can. The copy's path is printed on stdout, and a removal hint on
# stderr:
#
#   copy=$(scripts/plant-copy.sh selfupdate/session.go "'old'" "'new'")
#   (cd "$copy" && go test -count=1 -run '^TestX$' ./selfupdate)
#   rm -rf "$copy"
#
# Exit 0 with a copy, 1 when OLD is not found exactly once (no copy is
# made), 2 on a usage error.
set -euo pipefail

if [ $# -ne 3 ]; then
	echo "usage: plant-copy.sh FILE OLD NEW (OLD and NEW are Python string literals)" >&2
	exit 2
fi
ROOT=$(git rev-parse --show-toplevel)

python3 -I - "$ROOT" "$@" <<'PY'
import ast
import os
import shutil
import subprocess
import sys
import tempfile

root, rel, old_lit, new_lit = sys.argv[1:5]


def literal(s):
    try:
        v = ast.literal_eval(s)
    except (ValueError, SyntaxError):
        v = None
    if not isinstance(v, str):
        print(f"plant-copy: {s!r} is not a Python string literal", file=sys.stderr)
        sys.exit(2)
    return v


old, new = literal(old_lit), literal(new_lit)
src = os.path.join(root, rel)
with open(src, encoding="utf-8") as f:
    text = f.read()
n = text.count(old)
if n != 1:
    print(f"plant-copy: {rel} holds the old text {n} times, not once", file=sys.stderr)
    sys.exit(1)

names = subprocess.run(["git", "-C", root, "ls-files", "-z", "-co", "--exclude-standard"],
                       capture_output=True, check=True).stdout.decode().split("\0")
dest = tempfile.mkdtemp(prefix="plant-copy-")
try:
    for name in filter(None, names):
        s = os.path.join(root, name)
        if not os.path.lexists(s) or os.path.isdir(s):
            continue
        d = os.path.join(dest, name)
        os.makedirs(os.path.dirname(d), exist_ok=True)
        shutil.copy2(s, d, follow_symlinks=False)
    with open(os.path.join(dest, rel), "w", encoding="utf-8") as f:
        f.write(text.replace(old, new, 1))
except BaseException:
    shutil.rmtree(dest, ignore_errors=True)
    raise
print(dest)
print(f"plant-copy: remove it with: rm -rf {dest}", file=sys.stderr)
PY
