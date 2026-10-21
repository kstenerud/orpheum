# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for revert_red_known.py: a red revert is taken again only for the same blobs over the
# ABOUTME: same trees of what its run read, and a probe takes it without running unless told to trust none.
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
import check_revert_red as rr  # noqa: E402
import revert_red_known as known  # noqa: E402
from gitrepo import Repo  # noqa: E402


class DirsRead(unittest.TestCase):
    def test_a_runs_directories_are_its_go_packages_the_web_client_or_the_scripts(self):
        self.assertEqual(known.dirs_read(["go", "test", "-failfast", "./internal/a/...", "./internal/b"]),
                         ["internal/a", "internal/b"])
        self.assertEqual(known.dirs_read(["npx", "--prefix", "web", "vitest", "related", "x.ts"]), ["web"])
        self.assertEqual(known.dirs_read([sys.executable, "/r/scripts/py_tests.py"]), ["scripts", ".claude/hooks"])
        self.assertEqual(known.dirs_read(["go", "vet", "./..."]), [])


class Known(unittest.TestCase):
    def setUp(self):
        self.repo = Repo({"internal/a/a.go": "package a\n", "internal/a/a_test.go": "package a\n",
                          "internal/b/b.go": "package b\n"})
        self.base = self.repo.commit("base")
        self.head = self.repo.commit("change a", {"internal/a/a.go": "package a\n\nvar x = 1\n"})
        self.root = str(self.repo.root)
        self.key = known.revert_key(self.root, self.base, self.head, ["internal/a/a.go"])

    def remember(self) -> None:
        known.remember(self.root, self.head, self.key, ["go", "test", "./internal/a/..."], "RED_TEST", "a test fails")

    def test_a_red_is_known_while_what_its_run_read_is_unchanged(self):
        with self.repo.cwd():
            self.remember()
            later = self.repo.commit("change b", {"internal/b/b.go": "package b\n\nvar y = 2\n"})
            self.assertEqual(known.known(self.root, later, self.key)["verdict"], "RED_TEST")

    def test_a_red_is_not_known_once_a_test_it_ran_changed(self):
        with self.repo.cwd():
            self.remember()
            later = self.repo.commit("weaken", {"internal/a/a_test.go": "package a\n\n// nothing\n"})
            self.assertIsNone(known.known(self.root, later, self.key))

    def test_a_red_is_not_known_for_another_revert_of_the_file(self):
        with self.repo.cwd():
            self.remember()
            later = self.repo.commit("again", {"internal/a/a.go": "package a\n\nvar x = 2\n"})
            other = known.revert_key(self.root, self.base, later, ["internal/a/a.go"])
            self.assertIsNone(known.known(self.root, later, other))

    def test_a_probe_takes_a_known_red_without_running_and_trusting_none_runs(self):
        with self.repo.cwd():
            self.remember()
            change = rr.Change("internal/a/a.go", "M")
            taken = rr.probe_unclaimed(self.root, self.base, self.head, change, 60)
            self.assertEqual(taken.verdict, "RED_TEST")
            self.assertIn("as its probe at", taken.detail)
            rr.TRUST_RECORD = False
            try:
                ran = rr.probe_unclaimed(self.root, self.base, self.head, change, 60)
            finally:
                rr.TRUST_RECORD = True
            self.assertNotIn("as its probe at", ran.detail)


if __name__ == "__main__":
    unittest.main()
