"""The release verifier's SHA256SUMS parser, as a module.

It mirrors parseSHA256SUMS in selfupdate/checksums.go (0003-MADR D1):
verify-selfupdate-release.sh imports it, and selfupdate's
TestManifestDifferential compares the two parsers through the `parse`
command below (0004-MADR §8 H2, amendment C1). Standard library only; no
side effects on import.

Usage: python3 selfupdate_manifest.py parse DIR
    Parses every regular file in DIR, in sorted order, and prints one JSON
    object per line: {"case", "ok", "entries"} or {"case", "ok", "error"}.
    Entry names are hex-encoded bytes, so names that are not valid UTF-8
    compare exactly. Exit 0; a usage error exits 2.
"""
import json
import os
import re
import sys
import unicodedata

MAX_CHECKSUM_LINE = 4096  # selfupdate/checksums.go maxChecksumLine
# selfupdate/checksums.go maxChecksumName: the longest name whose canonical
# line, "<digest>  <name>" and its newline, fits the line cap
# (0015-MADR amendment A2, A10).
MAX_CHECKSUM_NAME = MAX_CHECKSUM_LINE - 1 - 64 - 2
HEX_RE = re.compile(r"^[0-9a-fA-F]{64}$")


class ManifestError(ValueError):
    """A manifest the client would refuse."""


def go_isspace(c):
    # Go's unicode.IsSpace, which differs from str.isspace (that also
    # counts U+001C..U+001F).
    return c in "\t\n\v\f\r \x85\xa0" or unicodedata.category(c) in ("Zs", "Zl", "Zp")


def go_trim_space(s):
    start, end = 0, len(s)
    while start < end and go_isspace(s[start]):
        start += 1
    while end > start and go_isspace(s[end - 1]):
        end -= 1
    return s[start:end]


def go_fields(s):
    fields, cur = [], []
    for c in s:
        if go_isspace(c):
            if cur:
                fields.append("".join(cur))
                cur = []
        else:
            cur.append(c)
    if cur:
        fields.append("".join(cur))
    return fields


def parse_manifest(raw, label):
    """Parse SHA256SUMS bytes; raise ManifestError where the client would.

    label names the manifest in messages, for example "SHA256SUMS".
    """
    # bufio.ScanLines splits on \n only; a final empty segment is no line.
    segments = raw.split(b"\n")
    if segments and segments[-1] == b"":
        segments.pop()
    entries = {}
    for i, seg in enumerate(segments, 1):
        # The scanner's 4096-byte buffer must hold the line and its newline.
        if len(seg) >= MAX_CHECKSUM_LINE:
            raise ManifestError("%s line %d: longer than the client accepts" % (label, i))
        line = seg.decode("utf-8", errors="surrogateescape").rstrip("\r")
        trimmed = go_trim_space(line)
        if trimmed == "" or trimmed.startswith("#"):
            continue
        fields = go_fields(line)
        if len(fields) != 2:
            raise ManifestError("%s line %d: want exactly two fields" % (label, i))
        digest, name = fields
        if name.startswith("*"):
            name = name[1:]
        if name == "" or "*" in name:
            raise ManifestError("%s line %d: malformed filename" % (label, i))
        if not HEX_RE.fullmatch(digest) or not digest.isascii():
            raise ManifestError("%s line %d: malformed digest" % (label, i))
        # ":" is refused, as the Go parser refuses it on every OS
        # (0010-MADR A12).
        if name in (".", "..") or "/" in name or "\\" in name or ":" in name or os.path.basename(name) != name:
            raise ManifestError("%s line %d: filename is not a basename" % (label, i))
        if len(name.encode("utf-8", errors="surrogateescape")) > MAX_CHECKSUM_NAME:
            raise ManifestError("%s line %d: filename longer than a SHA256SUMS line holds" % (label, i))
        if name in entries:
            raise ManifestError("%s duplicate filename %s" % (label, name))
        entries[name] = digest.lower()
    if not entries:
        raise ManifestError("%s has no entries" % label)
    return entries


def _hexname(name):
    return name.encode("utf-8", errors="surrogateescape").hex()


def _parse_dir(dirpath):
    for case in sorted(os.listdir(dirpath)):
        path = os.path.join(dirpath, case)
        if not os.path.isfile(path):
            continue
        with open(path, "rb") as f:
            raw = f.read()
        try:
            entries = parse_manifest(raw, case)
        except ManifestError as e:
            out = {"case": case, "ok": False, "error": str(e)}
        else:
            out = {"case": case, "ok": True, "entries": {_hexname(n): d for n, d in entries.items()}}
        # ensure_ascii keeps a message naming an odd filename printable.
        print(json.dumps(out, ensure_ascii=True))


def main(argv):
    if len(argv) != 3 or argv[1] != "parse" or not os.path.isdir(argv[2]):
        print("usage: selfupdate_manifest.py parse DIR", file=sys.stderr)
        return 2
    _parse_dir(argv[2])
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
