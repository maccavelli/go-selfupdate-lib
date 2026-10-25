"""Check the numbered records and their index (AGENTS.md, Records).

The records gate that full CI and the Docs run share
(docs/decisions/0036-MADR-align-docs-ci-and-disclosure-gates.md, Amendment
A1 G1). Standard library only. It reads the files a commit would hold:
tracked, and untracked but not ignored.

Usage: python3 scripts/check_records.py [--next]
    Run from inside the repository. Checks that:
    - a record is named NNNN-KIND-slug.md, KIND MADR or PLAN in
      docs/decisions/, REPORT or GATES in docs/reports/, and the slug
      lowercase kebab case; those directories hold records alone;
    - the numbers run from 0001 with no gap, and a number has at most one
      MADR;
    - a PLAN has its number's MADR, copies its slug when it is the only
      PLAN, and an associated-madr in its front matter names that MADR;
    - a MADR or PLAN has a front-matter status its kind allows;
    - docs/README.md's records table has one row per record, contiguous
      and in number order, whose number, kind and link match the file and
      whose status matches its front matter ("—" for a record with none).
    Each finding prints the file, the line where there is one, and what is
    wrong. Exit 0 when clean, 1 on a finding, 2 on a usage error or when
    the index has no records table.

    --next prints the next unused number and exits 0.
"""
import os
import re
import subprocess
import sys

KINDS = {"MADR": "docs/decisions", "PLAN": "docs/decisions", "REPORT": "docs/reports", "GATES": "docs/reports"}
NAME = re.compile(r"^(\d{4})-(MADR|PLAN|REPORT|GATES)-([a-z0-9]+(?:-[a-z0-9]+)*)\.md$")
# Anything that looks like a record, in any case, so a misnamed one is caught.
LOOKS = re.compile(r"^\d{4}-(?i:madr|plan|report|gates)-")
STATUS = {
    "MADR": re.compile(r"^(proposed|accepted|rejected|deprecated|superseded( by .+)?)$"),
    "PLAN": re.compile(r"^(proposed|in-progress|complete|rejected|superseded( by .+)?)$"),
}
INDEX = "docs/README.md"
HEADER = re.compile(r"^\|\s*Number\s*\|\s*Kind\s*\|\s*Record\s*\|\s*Status\s*\|\s*$")
ROW = re.compile(r"^\|\s*(\d{4})\s*\|\s*([A-Z]+)\s*\|\s*\[.*\]\(([^)\s]+)\)\s*\|\s*([^|]*?)\s*\|\s*$")
ANY_ROW = re.compile(r"^\|\s*\d{4}\s*\|")


def repo_files():
    out = subprocess.run(["git", "ls-files", "-z", "--cached", "--others", "--exclude-standard"],
                         capture_output=True, check=True).stdout
    return sorted({p for p in out.decode().split("\0") if p and os.path.isfile(p)})


def front_matter(path):
    """The front matter's keys and values, quotes removed; {} without it."""
    with open(path, encoding="utf-8") as f:
        lines = f.read().split("\n")
    if not lines or lines[0].strip() != "---":
        return {}
    meta = {}
    for line in lines[1:]:
        if line.strip() == "---":
            return meta
        key, sep, value = line.partition(":")
        if sep and not line.startswith((" ", "\t")):
            meta[key.strip()] = value.strip().strip("\"'")
    return {}


class Record:
    def __init__(self, path, m):
        self.path = path
        self.number, self.kind, self.slug = m.group(1), m.group(2), m.group(3)
        self.name = os.path.basename(path)
        self.meta = front_matter(path)


def main(argv):
    if argv not in ([], ["--next"]):
        print("usage: check_records.py [--next]", file=sys.stderr)
        return 2
    top = subprocess.run(["git", "rev-parse", "--show-toplevel"], capture_output=True, text=True)
    if top.returncode != 0:
        print("check-records: run it from inside the repository", file=sys.stderr)
        return 2
    os.chdir(top.stdout.strip())

    bad, records = [], []
    for path in repo_files():
        base, parent = os.path.basename(path), os.path.dirname(path)
        m = NAME.match(base)
        if m and parent == KINDS[m.group(2)]:
            records.append(Record(path, m))
        elif m:
            bad.append(f"{path}: a {m.group(2)} belongs in {KINDS[m.group(2)]}/")
        elif LOOKS.match(base):
            bad.append(f"{path}: not NNNN-KIND-slug.md, with KIND in capitals and a lowercase kebab-case slug")
        elif parent in set(KINDS.values()):
            bad.append(f"{path}: not a record; {parent}/ holds records alone")

    numbers = sorted({int(r.number) for r in records})
    if argv == ["--next"]:
        print(f"{(numbers[-1] if numbers else 0) + 1:04d}")
        return 0
    for want, got in enumerate(numbers, 1):
        if got != want:
            bad.append(f"docs/: no record numbered {want:04d}; the numbers leave a gap before {got:04d}")
            break

    by_number = {}
    for r in records:
        by_number.setdefault(r.number, []).append(r)
    for number, group in sorted(by_number.items()):
        madrs = [r for r in group if r.kind == "MADR"]
        plans = [r for r in group if r.kind == "PLAN"]
        for extra in madrs[1:]:
            bad.append(f"{extra.path}: number {number} already has a MADR, {madrs[0].name}")
        madr = madrs[0] if madrs else None
        for p in plans:
            if madr is None:
                bad.append(f"{p.path}: no MADR numbered {number}")
                continue
            if len(plans) == 1 and p.slug != madr.slug:
                bad.append(f"{p.path}: the only PLAN for {number} must copy its MADR's slug, {madr.slug}")
            assoc = p.meta.get("associated-madr")
            if assoc is not None and assoc != madr.name:
                bad.append(f"{p.path}: associated-madr is {assoc}, not {madr.name}")
        for r in madrs + plans:
            status = r.meta.get("status")
            if status is None:
                bad.append(f"{r.path}: no status in its front matter")
            elif not STATUS[r.kind].match(status):
                bad.append(f"{r.path}: status {status!r} is not one a {r.kind} takes")

    bad += check_index(records)
    for b in bad:
        print(b)
    print(f"check-records: {len(records)} records, {len(bad)} findings")
    return 1 if bad else 0


def check_index(records):
    if not os.path.isfile(INDEX):
        print(f"check-records: {INDEX} is missing", file=sys.stderr)
        sys.exit(2)
    with open(INDEX, encoding="utf-8") as f:
        lines = f.read().split("\n")
    start = next((i for i, line in enumerate(lines) if HEADER.match(line)), None)
    if start is None:
        print(f"check-records: {INDEX} has no | Number | Kind | Record | Status | table", file=sys.stderr)
        sys.exit(2)
    end = start + 2  # past the header and its delimiter row
    while end < len(lines) and lines[end].startswith("|"):
        end += 1

    bad, rows, seen, prev = [], {}, set(), ""
    for i, line in enumerate(lines):
        n = i + 1
        if not ANY_ROW.match(line):
            continue
        if not start + 2 <= i < end:
            bad.append(f"{INDEX}:{n}: a record row outside the records table, which ends at line {end}")
            continue
        m = ROW.match(line)
        if not m:
            bad.append(f"{INDEX}:{n}: a row not | NNNN | KIND | [title](link) | status |")
            continue
        number, kind, link, status = m.groups()
        if number < prev:
            bad.append(f"{INDEX}:{n}: {number} after {prev}; rows go in number order")
        prev = number
        path = os.path.normpath(os.path.join(os.path.dirname(INDEX), link))
        if path in seen:
            bad.append(f"{INDEX}:{n}: a second row for {path}")
            continue
        seen.add(path)
        rows[path] = (n, number, kind, status)

    by_path = {r.path: r for r in records}
    for path, (n, number, kind, status) in rows.items():
        r = by_path.get(path)
        if r is None:
            bad.append(f"{INDEX}:{n}: links {path}, which is not a record")
            continue
        if (number, kind) != (r.number, r.kind):
            bad.append(f"{INDEX}:{n}: says {number} {kind} for {r.name}")
        want = r.meta.get("status", "—")
        if status != want:
            bad.append(f"{INDEX}:{n}: status {status!r}, but {r.name} says {want!r}")
    for r in records:
        if r.path not in rows:
            bad.append(f"{r.path}: no row in {INDEX}")
    return bad


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
