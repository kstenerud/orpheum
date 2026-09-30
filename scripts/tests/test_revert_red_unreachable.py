# SPDX-License-Identifier: GPL-3.0-or-later
# ABOUTME: Tests for revert_red_unreachable.py: dropping declarations nothing reaches is reported
# ABOUTME: UNREACHABLE, and every change that might reach something is still probed.
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
import check_revert_red as rr  # noqa: E402
import revert_red_unreachable as unreachable  # noqa: E402
from gitrepo import Repo  # noqa: E402

BEFORE = '''package a

import (
	"slices"
	"strings"
)

// Keep is used.
func Keep(s []string) bool { return slices.Contains(s, "x") }

// shout was the last user of strings.
func shout(s string) string {
	return strings.ToUpper(s)
}

type (
	gone  int
	other string
)

const limit = 3
'''


def dropped(before: str, after: str) -> list[str] | None:
    names = unreachable.dropped_names(before, after)
    return None if names is None else [name for name, _ in names]


class DroppedNames(unittest.TestCase):
    def test_dropping_whole_funcs_types_and_consts_with_their_imports_is_found(self):
        after = BEFORE.replace('\t"strings"\n', "").split("// shout")[0].rstrip() + "\n"
        self.assertEqual(dropped(BEFORE, after), ["shout", "gone", "other", "limit"])

    def test_a_kept_declaration_that_changed_declines(self):
        after = BEFORE.replace('"x")', '"y")').replace("const limit = 3\n", "")
        self.assertIsNone(dropped(BEFORE, after))

    def test_a_reworded_doc_comment_on_a_kept_declaration_still_counts(self):
        after = BEFORE.replace("// Keep is used.", "// Keep says whether x is in s.").replace(
            "const limit = 3\n", "")
        self.assertEqual(dropped(BEFORE, after), ["limit"])

    def test_dropping_a_var_an_init_a_directive_or_a_blank_import_declines(self):
        for before, after in (
            ("package a\n\nvar started = start()\n", "package a\n"),
            ("package a\n\nfunc init() {\n\tregister()\n}\n", "package a\n"),
            ("package a\n\n//go:noinline\nfunc f() {}\n", "package a\n"),
            ('package a\n\nimport _ "embed"\n\nfunc f() {}\n', "package a\n\nfunc f() {}\n"),
        ):
            with self.subTest(before=before):
                self.assertIsNone(dropped(before, after))

    def test_adding_anything_declines(self):
        self.assertIsNone(dropped("package a\n", "package a\n\nfunc f() {}\n"))
        self.assertIsNone(dropped(BEFORE, BEFORE.replace("package a", "package b")))

    def test_a_block_comment_declines(self):
        self.assertIsNone(dropped("package a\n\n/* x */\nfunc f() {}\n", "package a\n\n/* x */\n"))


class Probe(unittest.TestCase):
    CODE = "internal/a/a.go"
    OTHER = "internal/a/b.go"
    TYPE = "package a\n\ntype filer struct{}\n\nfunc (f *filer) Keep() {}\n"

    def outcome(self, repo: Repo, base: str, status: str = "M") -> rr.Outcome:
        with repo.cwd():
            return rr.probe(str(repo.root), base, "HEAD", rr.Change(self.CODE, status), timeout=60)

    def test_an_unexported_method_nothing_names_is_unreachable(self):
        repo = Repo({self.CODE: self.TYPE + "\nfunc (f *filer) drop() {}\n", self.OTHER: "package a\n"})
        base = repo.commit("chore: base")
        repo.commit("feat(a): stop dropping\n\nIt dropped.", {self.CODE: self.TYPE})
        outcome = self.outcome(repo, base)
        self.assertEqual((outcome.verdict, outcome.detail),
                         ("UNREACHABLE", "only removed drop, which nothing uses"))
        self.assertFalse(outcome.is_failure)
        self.assertIn("Only removed code nothing reached", rr.report([outcome]))

    def test_a_method_its_package_still_names_is_probed(self):
        repo = Repo({self.CODE: self.TYPE + "\nfunc (f *filer) drop() {}\n",
                     self.OTHER: "package a\n\ntype dropper interface{ drop() }\n"})
        base = repo.commit("chore: base")
        repo.commit("feat(a): stop dropping\n\nIt dropped.", {self.CODE: self.TYPE})
        self.assertNotEqual(self.outcome(repo, base).verdict, "UNREACHABLE")

    def test_an_exported_method_is_probed(self):
        repo = Repo({self.CODE: self.TYPE + "\nfunc (f *filer) String() string { return \"\" }\n"})
        base = repo.commit("chore: base")
        repo.commit("feat(a): plain\n\nIt printed.", {self.CODE: self.TYPE})
        self.assertNotEqual(self.outcome(repo, base).verdict, "UNREACHABLE")

    def test_a_deleted_file_of_dead_declarations_is_unreachable(self):
        repo = Repo({self.CODE: "package a\n\nfunc helper() {}\n", self.OTHER: "package a\n"})
        base = repo.commit("chore: base")
        repo.commit("refactor(a): drop helper\n\nNothing called it.", {self.CODE: None})
        self.assertEqual(self.outcome(repo, base, "D").verdict, "UNREACHABLE")


if __name__ == "__main__":
    unittest.main()
