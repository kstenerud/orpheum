# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests that a probe whose run outlasts its timeout is a failing outcome naming its file, not an
# ABOUTME: exception that ends every other probe with it, and that nothing the run started outlives it.
import sys
import unittest
from pathlib import Path
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parent))
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
import check_revert_red as rr  # noqa: E402
from gitrepo import Repo  # noqa: E402


class ProbeTimeout(unittest.TestCase):
    CODE = "internal/a/a.go"

    def test_a_run_past_its_timeout_settles_nothing_and_names_the_file(self):
        repo = Repo({self.CODE: "package a\n\nconst wait = true\n"})
        base = repo.commit("chore: base")
        repo.commit("fix(a): stop waiting\n\nIt waited.", {self.CODE: "package a\n\nconst wait = false\n"})
        hanging = [rr.Attempt(["sleep", "30"], "")]
        with repo.cwd(), mock.patch.object(rr, "attempts_for", return_value=hanging):
            outcome = rr.probe(str(repo.root), base, "HEAD", rr.Change(self.CODE, "M"), timeout=1)
        self.assertEqual(outcome.path, self.CODE)
        self.assertEqual(outcome.verdict, "TIMED_OUT")
        self.assertTrue(outcome.is_failure)
        self.assertIn("ran past 1 s", outcome.detail)
        self.assertIn("Not settled", rr.report([outcome]))


if __name__ == "__main__":
    unittest.main()
