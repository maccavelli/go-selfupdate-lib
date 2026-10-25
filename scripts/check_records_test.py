"""Offline tests for scripts/check_records.py
(docs/decisions/0036-MADR-align-docs-ci-and-disclosure-gates.md, Amendment
A1 G1). Standard library only.

Usage: python3 scripts/check_records_test.py
    Each case builds a throwaway repository from a clean tree with one
    change, runs the checker in it, and checks its exit status and output.
"""
import os
import subprocess
import sys
import tempfile
import unittest

SCRIPT = os.environ.get("SCRIPT") or os.path.join(os.path.dirname(os.path.abspath(__file__)), "check_records.py")

HEAD = "| Number | Kind | Record | Status |\n| :--- | :--- | :--- | :--- |\n"


def row(number, kind, link, status):
    return f"| {number} | {kind} | [A title](decisions/{link}) | {status} |\n" if kind in ("MADR", "PLAN") \
        else f"| {number} | {kind} | [A title](reports/{link}) | {status} |\n"


ROWS = [
    row("0001", "MADR", "0001-MADR-alpha.md", "accepted"),
    row("0001", "PLAN", "0001-PLAN-alpha.md", "complete"),
    row("0002", "REPORT", "0002-REPORT-beta.md", "—"),
]


def clean():
    return {
        "docs/decisions/0001-MADR-alpha.md": "---\nstatus: accepted\ndate: 2026-10-25\n---\n# Alpha\n",
        "docs/decisions/0001-PLAN-alpha.md":
            '---\nstatus: complete\nassociated-madr: "0001-MADR-alpha.md"\n---\n# Implement alpha\n',
        "docs/reports/0002-REPORT-beta.md": "# Beta\n",
        "docs/README.md": "# Docs\n\n## Records\n\n" + HEAD + "".join(ROWS) + "\n## Next\n",
    }


def index(rows, between=""):
    return "# Docs\n\n## Records\n\n" + HEAD + between.join(rows) + "\n## Next\n"


class CheckRecords(unittest.TestCase):
    def run_in(self, tree, *args, git=True):
        with tempfile.TemporaryDirectory() as d:
            for path, text in tree.items():
                full = os.path.join(d, path)
                os.makedirs(os.path.dirname(full), exist_ok=True)
                with open(full, "w", encoding="utf-8") as f:
                    f.write(text)
            if git:
                subprocess.run(["git", "init", "-q", d], check=True)
            # A .git above the temporary directory must not stand in for one.
            env = dict(os.environ, GIT_CEILING_DIRECTORIES=os.path.dirname(os.path.realpath(d)))
            r = subprocess.run([sys.executable, "-I", SCRIPT, *args], cwd=d, env=env, capture_output=True, text=True)
            return r.returncode, r.stdout + r.stderr

    def finds(self, tree, *wants):
        rc, out = self.run_in(tree)
        self.assertEqual(rc, 1, out)
        for want in wants:
            self.assertIn(want, out)

    def test_clean(self):
        rc, out = self.run_in(clean())
        self.assertEqual(rc, 0, out)
        self.assertIn("check-records: 3 records, 0 findings", out)

    def test_next(self):
        rc, out = self.run_in(clean(), "--next")
        self.assertEqual((rc, out.strip()), (0, "0003"))

    def test_untracked_counts_and_ignored_does_not(self):
        t = clean()
        t[".gitignore"] = "docs/decisions/0009-MADR-ignored.md\n"
        t["docs/decisions/0009-MADR-ignored.md"] = "---\nstatus: accepted\n---\n"
        rc, out = self.run_in(t)
        self.assertEqual(rc, 0, out)
        t["docs/decisions/0003-MADR-gamma.md"] = "---\nstatus: accepted\n---\n"
        self.finds(t, "docs/decisions/0003-MADR-gamma.md: no row in docs/README.md")

    def test_misnamed(self):
        t = clean()
        t["docs/decisions/0003-madr-Gamma.md"] = ""
        self.finds(t, "docs/decisions/0003-madr-Gamma.md: not NNNN-KIND-slug.md")

    def test_wrong_directory(self):
        t = clean()
        t["docs/reports/0003-MADR-gamma.md"] = "---\nstatus: accepted\n---\n"
        t["docs/0003-PLAN-gamma.md"] = "---\nstatus: proposed\n---\n"
        self.finds(t, "docs/reports/0003-MADR-gamma.md: a MADR belongs in docs/decisions/",
                   "docs/0003-PLAN-gamma.md: a PLAN belongs in docs/decisions/")

    def test_not_a_record(self):
        t = clean()
        t["docs/decisions/notes.md"] = "# Notes\n"
        self.finds(t, "docs/decisions/notes.md: not a record")

    def test_gap(self):
        t = clean()
        t["docs/reports/0004-REPORT-delta.md"] = "# Delta\n"
        t["docs/README.md"] = index(ROWS + [row("0004", "REPORT", "0004-REPORT-delta.md", "—")])
        self.finds(t, "no record numbered 0003")

    def test_plan_without_madr(self):
        t = clean()
        t["docs/decisions/0003-PLAN-gamma.md"] = "---\nstatus: proposed\n---\n"
        t["docs/README.md"] = index(ROWS + [row("0003", "PLAN", "0003-PLAN-gamma.md", "proposed")])
        self.finds(t, "docs/decisions/0003-PLAN-gamma.md: no MADR numbered 0003")

    def test_lone_plan_slug(self):
        t = clean()
        t["docs/decisions/0001-PLAN-other.md"] = t.pop("docs/decisions/0001-PLAN-alpha.md")
        t["docs/README.md"] = index([ROWS[0], row("0001", "PLAN", "0001-PLAN-other.md", "complete"), ROWS[2]])
        self.finds(t, "0001-PLAN-other.md: the only PLAN for 0001 must copy its MADR's slug, alpha")

    def test_two_plans_take_their_own_slugs(self):
        t = clean()
        t["docs/decisions/0001-PLAN-second-part.md"] = "---\nstatus: proposed\n---\n"
        t["docs/README.md"] = index(ROWS[:2] + [row("0001", "PLAN", "0001-PLAN-second-part.md", "proposed"), ROWS[2]])
        rc, out = self.run_in(t)
        self.assertEqual(rc, 0, out)

    def test_associated_madr(self):
        t = clean()
        t["docs/decisions/0001-PLAN-alpha.md"] = '---\nstatus: complete\nassociated-madr: "0001-MADR-beta.md"\n---\n'
        self.finds(t, "associated-madr is 0001-MADR-beta.md, not 0001-MADR-alpha.md")

    def test_second_madr(self):
        t = clean()
        t["docs/decisions/0001-MADR-again.md"] = "---\nstatus: accepted\n---\n"
        t["docs/README.md"] = index(ROWS[:1] + [row("0001", "MADR", "0001-MADR-again.md", "accepted")] + ROWS[1:])
        self.finds(t, "0001-MADR-alpha.md: number 0001 already has a MADR, 0001-MADR-again.md")

    def test_status(self):
        t = clean()
        t["docs/decisions/0001-MADR-alpha.md"] = "# No front matter\n"
        t["docs/decisions/0001-PLAN-alpha.md"] = "---\nstatus: done\n---\n"
        self.finds(t, "0001-MADR-alpha.md: no status in its front matter",
                   "0001-PLAN-alpha.md: status 'done' is not one a PLAN takes")

    def test_missing_row(self):
        t = clean()
        t["docs/README.md"] = index(ROWS[:2])
        self.finds(t, "docs/reports/0002-REPORT-beta.md: no row in docs/README.md")

    def test_row_for_no_record(self):
        t = clean()
        t["docs/README.md"] = index(ROWS + [row("0003", "MADR", "0003-MADR-gone.md", "accepted")])
        self.finds(t, "docs/README.md:10: links docs/decisions/0003-MADR-gone.md, which is not a record")

    def test_row_status_kind_and_duplicate(self):
        t = clean()
        t["docs/README.md"] = index([row("0001", "MADR", "0001-MADR-alpha.md", "proposed"),
                                     row("0001", "MADR", "0001-PLAN-alpha.md", "complete"),
                                     ROWS[2], ROWS[2]])
        self.finds(t, "docs/README.md:7: status 'proposed', but 0001-MADR-alpha.md says 'accepted'",
                   "docs/README.md:8: says 0001 MADR for 0001-PLAN-alpha.md",
                   "docs/README.md:10: a second row for docs/reports/0002-REPORT-beta.md")

    def test_row_outside_the_table(self):
        t = clean()
        t["docs/README.md"] = index(ROWS[:2] + ["\n" + ROWS[2]])
        self.finds(t, "docs/README.md:10: a record row outside the records table, which ends at line 8")

    def test_order(self):
        t = clean()
        t["docs/README.md"] = index([ROWS[2], ROWS[0], ROWS[1]])
        self.finds(t, "docs/README.md:8: 0001 after 0002; rows go in number order")
        self.assertNotIn("README.md:9:", self.run_in(t)[1])

    def test_malformed_row(self):
        t = clean()
        t["docs/README.md"] = index(ROWS + ["| 0003 | MADR | no link | accepted |\n"])
        self.finds(t, "docs/README.md:10: a row not | NNNN | KIND | [title](link) | status |")

    def test_usage_and_setup_errors(self):
        t = clean()
        t["docs/README.md"] = "# Docs\n\nNo table.\n"
        self.assertEqual(self.run_in(t)[0], 2)
        self.assertEqual(self.run_in(clean(), "--bogus")[0], 2)
        self.assertEqual(self.run_in(clean(), git=False)[0], 2)


if __name__ == "__main__":
    unittest.main(verbosity=1)
