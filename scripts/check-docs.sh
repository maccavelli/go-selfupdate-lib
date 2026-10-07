#!/usr/bin/env bash
# Check documents before they are committed
# (docs/decisions/0015-PLAN-remediate-third-debugging-pass-findings.md R1).
# It replaces the session checker the 0013 and 0014 PLANs cite as
# doccheck.py. Two modes:
#
#   --links FILE...  every relative Markdown link outside code names a file
#                    that exists, and a #fragment into a .md file names one
#                    of its headings, by GitHub's slug: lowercase, keep
#                    letters, digits, "_", "-" and spaces, then spaces become
#                    "-"; a repeated heading gains -1, -2, ... A link that
#                    starts with a scheme, such as https:, is not checked.
#
#   --ids [--ids-optional] FILE...
#                    no file holds an identifier (AGENTS.md, Identifiers):
#                    nothing the deny list's content rules match, and no
#                    macOS or Linux home directory path other than the
#                    <user> placeholder. The deny list is the pre-push
#                    guard's, DISCLOSURE_DENY or ~/.config/git/disclosure-deny,
#                    one "<regex> TAB <placeholder> [TAB identity]" rule per
#                    line; identity rules apply to commit authors, not to
#                    content, and are skipped. A finding prints the file, the
#                    line and the placeholder, never what matched. Without a
#                    readable deny list it exits 2, unless --ids-optional,
#                    which checks home paths alone.
#
# Exit 0 when clean, 1 on a finding, 2 on a usage or deny-list error.
set -euo pipefail

usage() {
	echo "usage: check-docs.sh --links FILE... | --ids [--ids-optional] FILE..." >&2
	exit 2
}

[ $# -ge 1 ] || usage
MODE=$1
shift
OPTIONAL=0
case "$MODE" in
--links) ;;
--ids)
	if [ "${1:-}" = "--ids-optional" ]; then
		OPTIONAL=1
		shift
	fi
	;;
*) usage ;;
esac
DENY="${DISCLOSURE_DENY:-$HOME/.config/git/disclosure-deny}"

python3 -I - "$MODE" "$OPTIONAL" "$DENY" "$@" <<'PY'
import os
import re
import sys
from urllib.parse import unquote

mode, optional, deny_path, files = sys.argv[1], sys.argv[2] == "1", sys.argv[3], sys.argv[4:]

LINK = re.compile(r"\]\(([^)\s]+)(?:\s+\"[^\"]*\")?\)")
CODE = re.compile(r"`[^`]*`")
SCHEME = re.compile(r"[A-Za-z][A-Za-z0-9+.-]*:")
HEADING = re.compile(r"^(#{1,6})\s+(.*?)\s*#*\s*$")
FENCE = re.compile(r"^\s*(```|~~~)")
# A home directory: the macOS or the Linux root, then a name that is not the
# placeholder. Not inside a host or a longer path, such as a URL's.
HOME = re.compile(r"(?<![\w.-])/(?:Users|home)/(?!<user>)[^/\s`'\")<>\]]+")


def read_lines(path):
    with open(path, encoding="utf-8", errors="replace") as f:
        return f.read().split("\n")


def prose(lines):
    """Yield (line number, text) for lines outside fenced code."""
    fence = None
    for n, line in enumerate(lines, 1):
        m = FENCE.match(line)
        if m:
            if fence is None:
                fence = m.group(1)
            elif m.group(1) == fence:
                fence = None
            continue
        if fence is None:
            yield n, line


def slug(text):
    s = CODE.sub(lambda m: m.group(0).strip("`"), text).strip().lower()
    s = re.sub(r"[^\w\- ]", "", s)
    return s.replace(" ", "-")


_anchors = {}


def anchors(path):
    if path not in _anchors:
        seen, out = {}, set()
        for _, line in prose(read_lines(path)):
            m = HEADING.match(line)
            if not m:
                continue
            base = slug(m.group(2))
            n = seen.get(base, 0)
            seen[base] = n + 1
            out.add(base if n == 0 else f"{base}-{n}")
        _anchors[path] = out
    return _anchors[path]


def check_links():
    bad, count = [], 0
    for f in files:
        base = os.path.dirname(f)
        for n, line in prose(read_lines(f)):
            for target in LINK.findall(CODE.sub("", line)):
                if SCHEME.match(target):
                    continue
                count += 1
                path, _, frag = target.partition("#")
                full = os.path.normpath(os.path.join(base, unquote(path))) if path else f
                if not os.path.exists(full):
                    bad.append(f"{f}:{n}: broken link: {target}")
                elif frag and full.endswith(".md") and unquote(frag) not in anchors(full):
                    bad.append(f"{f}:{n}: missing anchor: {target}")
    for b in bad:
        print(b)
    print(f"check-docs: {count} links in {len(files)} files, {len(bad)} broken")
    return 1 if bad else 0


def load_rules():
    try:
        with open(deny_path, encoding="utf-8") as fh:
            text = fh.read()
    except OSError:
        return None
    rules = []
    for n, line in enumerate(text.splitlines(), 1):
        if not line.strip() or line.lstrip().startswith("#"):
            continue
        fields = line.split("\t")
        if len(fields) not in (2, 3) or (len(fields) == 3 and fields[2] != "identity"):
            print(f"check-docs: deny list line {n}: want <regex> TAB <placeholder> [TAB identity]",
                  file=sys.stderr)
            sys.exit(2)
        if len(fields) == 3:
            continue
        try:
            rules.append((re.compile(fields[0]), fields[1]))
        except re.error as err:
            print(f"check-docs: deny list line {n}: bad regex: {err}", file=sys.stderr)
            sys.exit(2)
    return rules


def check_ids():
    rules = load_rules()
    if rules is None:
        if not optional:
            print("check-docs: no deny list (DISCLOSURE_DENY, or ~/.config/git/disclosure-deny); "
                  "--ids-optional checks home paths alone", file=sys.stderr)
            return 2
        rules = []
    bad = []
    for f in files:
        for n, line in enumerate(read_lines(f), 1):
            for pattern, placeholder in rules:
                if pattern.search(line):
                    bad.append(f"{f}:{n}: {placeholder}")
            if HOME.search(line):
                bad.append(f"{f}:{n}: home directory path")
    for b in bad:
        print(b)
    print(f"check-docs: {len(files)} files, {len(rules)} deny-list rules, {len(bad)} findings")
    return 1 if bad else 0


try:
    sys.exit(check_links() if mode == "--links" else check_ids())
except OSError as err:
    print(f"check-docs: {err.filename}: {err.strerror}", file=sys.stderr)
    sys.exit(2)
PY
