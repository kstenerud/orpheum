# ABOUTME: Tests for check_docs.py — pins GitHub's slug rules, the code-span and fence
# ABOUTME: exclusions, and each gate on a small synthetic tree, so a broken gate fails loudly.
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
import check_docs as cd  # noqa: E402

LIVE, ARCH = cd.LIVE_LOG, cd.ARCHIVE_LOG


class Slug(unittest.TestCase):
    def test_github_rules(self):
        cases = {
            "D5 — Database: PostgreSQL only": "d5--database-postgresql-only",
            "D40 — `error` and `skip` records": "d40--error-and-skip-records",
            "D96 — An episode's position is `(ordering, season, number)`":
                "d96--an-episodes-position-is-ordering-season-number",
            "Why *this* / that.": "why-this--that",
            "snake_case and-hyphen": "snake_case-and-hyphen",
            "[linked](x.md) heading": "linked-heading",
            "Überprüfung 2": "überprüfung-2",
        }
        for heading, want in cases.items():
            self.assertEqual(cd.slug(heading), want, heading)

    def test_duplicate_headings_get_suffixes(self):
        self.assertEqual(cd.anchors("# A\n## A\n### A\n"), {"a", "a-1", "a-2"})

    def test_fenced_headings_define_nothing(self):
        self.assertEqual(cd.anchors("```\n# not a heading\n```\n# real\n"), {"real"})


class Links(unittest.TestCase):
    def test_code_is_not_a_link(self):
        body = "a `[x](nope.md)` b [y](yes.md)\n```\n[z](fenced.md)\n```\n"
        self.assertEqual([t for _, _, t in cd.links(body)], ["yes.md"])

    def test_link_text_with_brackets(self):
        self.assertEqual([t for _, _, t in cd.links("[see [1]](a.md)")], ["a.md"])


def entry(n, title="t", note=""):
    return f"## D{n} — {title}\n\n{note}Body.\n\n---\n\n"


class Gates(unittest.TestCase):
    def run_tree(self, files):
        with tempfile.TemporaryDirectory() as tmp:
            old = cd.ROOT
            cd.ROOT = Path(tmp)
            try:
                for rel, body in files.items():
                    p = Path(tmp) / rel
                    p.parent.mkdir(parents=True, exist_ok=True)
                    p.write_text(body, encoding="utf-8")
                return cd.check(sorted(files), dict(files))
            finally:
                cd.ROOT = old

    def base(self, **extra):
        files = {
            LIVE: entry(1, "one") + entry(3, "three"),
            ARCH: entry(2, "two", "> **Archived 2026-09-23.** Superseded by [D3](working-notes.md#d3--three).\n\n"),
        }
        files.update(extra)
        return files

    def test_clean_tree_passes(self):
        self.assertEqual(self.run_tree(self.base()), [])

    def test_missing_file_and_anchor(self):
        errs = self.run_tree(self.base(**{"docs/a.md": "[x](nope.md) [y](b.md#gone)", "docs/b.md": "# here\n"}))
        self.assertTrue(any("does not exist" in e for e in errs), errs)
        self.assertTrue(any("#gone" in e for e in errs), errs)

    def test_gap_and_duplicate(self):
        errs = self.run_tree({LIVE: entry(1) + entry(3) + entry(3), ARCH: ""})
        self.assertTrue(any("D2 is defined nowhere" in e for e in errs), errs)
        self.assertTrue(any("D3 is defined 2 times" in e for e in errs), errs)

    def test_decision_link_must_land_on_its_file_and_anchor(self):
        errs = self.run_tree(self.base(**{
            "docs/contributors/design/p.md":
                "[D2](../decisions/working-notes.md) [D1](../decisions/working-notes.md#d3--three)",
        }))
        self.assertTrue(any("but D2 is in" in e for e in errs), errs)
        self.assertTrue(any("D3's anchor, not D1's" in e for e in errs), errs)

    def test_live_docs_may_not_cite_archived(self):
        errs = self.run_tree(self.base(**{
            "docs/contributors/design/plans/p.md": "As D2 says, and [D2](../../decisions/working-notes-archive.md).",
        }))
        self.assertEqual(sum("cites archived D2" in e for e in errs), 2, errs)
        self.assertIn("(D3)", errs[0])

    def test_history_may_cite_archived(self):
        errs = self.run_tree(self.base(**{
            "docs/contributors/design/research/r.md": "D2 was wrong.",
        }))
        self.assertEqual(errs, [])

    def test_archived_entry_needs_a_note(self):
        errs = self.run_tree({LIVE: entry(1) + entry(3), ARCH: entry(2)})
        self.assertTrue(any("no '> **Archived" in e for e in errs), errs)


if __name__ == "__main__":
    unittest.main()
