# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests that revert-red's code part takes no browser run, leaving a file its cheap runs did not
# ABOUTME: notice to the browser part, which then takes only that run (the owner, 2026-10-17).
import sys
import unittest
from pathlib import Path
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parent))
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
import check_revert_red as rr  # noqa: E402
import revert_red_reruns as reruns  # noqa: E402
from gitrepo import Repo  # noqa: E402

CODE = "web/apps/web/src/a.ts"
# A run the probe takes for the browser tier's, by the word in its command, and one it does not.
CHEAP_PASSES = rr.Attempt(["true"], "")
BROWSER_FAILS = rr.Attempt(["sh", "-c", "exit 1", "browser-tier"], ", found by the browser tier")


class BrowserPart(unittest.TestCase):
    def setUp(self):
        self.repo = Repo({CODE: "export const a = 1;\n"})
        self.base = self.repo.commit("chore: base")
        self.repo.commit("fix(web): a is two\n\nIt was one.", {CODE: "export const a = 2;\n"})

    def outcome(self, browser: str, attempts: list[rr.Attempt]) -> rr.Outcome:
        with self.repo.cwd(), mock.patch.object(rr, "attempts_for", return_value=attempts):
            return rr.probe(str(self.repo.root), self.base, "HEAD", rr.Change(CODE, "M"), 60, browser)

    def test_the_code_part_leaves_the_browser_run_owed_and_fails_nothing(self):
        outcome = self.outcome(rr.BROWSER_DEFERRED, [CHEAP_PASSES, BROWSER_FAILS])
        self.assertEqual(outcome.verdict, "BROWSER_OWED")
        self.assertFalse(outcome.is_failure)
        self.assertIn("Left to the browser tier", rr.report([outcome]))

    def test_a_cheap_red_settles_it_in_the_code_part(self):
        cheap_fails = rr.Attempt(["false"], "")
        self.assertEqual(self.outcome(rr.BROWSER_DEFERRED, [cheap_fails, BROWSER_FAILS]).verdict, "RED_TEST")

    def test_the_browser_part_takes_the_browser_run_alone(self):
        cheap_fails = rr.Attempt(["false"], "")
        outcome = self.outcome(rr.BROWSER_ONLY, [cheap_fails, BROWSER_FAILS])
        self.assertEqual((outcome.verdict, outcome.detail),
                         ("RED_TEST", "a test fails when it is reverted, found by the browser tier"))

    def test_a_run_by_hand_takes_every_run_in_turn(self):
        self.assertEqual(self.outcome(rr.BROWSER_INLINE, [CHEAP_PASSES, BROWSER_FAILS]).verdict, "RED_TEST")

    def test_a_change_of_types_alone_takes_no_browser_run_as_the_browser_runs_the_same_javascript(self):
        self.repo.commit("fix(web): a is typed\n\nIt was not.", {CODE: "export const a: number = 2;\n"})
        with self.repo.cwd(), mock.patch.object(rr, "attempts_for", return_value=[CHEAP_PASSES, BROWSER_FAILS]):
            outcome = rr.probe(str(self.repo.root), "HEAD~1", "HEAD", rr.Change(CODE, "M"), 60, rr.BROWSER_DEFERRED)
        self.assertEqual(outcome.verdict, "GREEN")



class ClaimedBrowserRun(unittest.TestCase):
    def setUp(self):
        self.repo = Repo({CODE: "export const a = 1;\n"})
        self.base = self.repo.commit("chore: base")

    def outcome(self) -> rr.Outcome:
        with self.repo.cwd(), mock.patch.object(rr, "attempts_for", return_value=[CHEAP_PASSES, BROWSER_FAILS]):
            return rr.probe(str(self.repo.root), self.base, "HEAD", rr.Change(CODE, "M"), 60, rr.BROWSER_DEFERRED)

    def test_a_claimed_file_owes_no_browser_run_since_none_could_change_its_verdict(self):
        self.repo.commit(f"refactor(web): a is two\n\nOne name for it.\n\nRefactor-Only: {CODE} — renamed",
                         {CODE: "export const a = 2;\n"})
        self.assertEqual(self.outcome().verdict, "CLAIMED")

    def test_an_unclaimed_file_still_owes_it(self):
        self.repo.commit("fix(web): a is two\n\nIt was one.", {CODE: "export const a = 2;\n"})
        self.assertEqual(self.outcome().verdict, "BROWSER_OWED")


class OwedRecord(unittest.TestCase):
    def test_a_targeted_code_run_replaces_only_the_rows_it_probed(self):
        repo = Repo({"a": "1\n"})
        repo.commit("chore: a")
        with repo.cwd():
            reruns.owe_browser("b1", ["x.ts", "y.ts"], ["x.ts", "y.ts"])
            reruns.owe_browser("b1", ["y.ts", "z.ts"], ["z.ts"])
            self.assertEqual(reruns.owed_browser("b1"), frozenset({"x.ts", "z.ts"}))
            self.assertEqual(reruns.owed_browser("another base"), frozenset())
            reruns.owe_browser("b2", ["w.ts"], ["w.ts"])
            self.assertEqual(reruns.owed_browser("b2"), frozenset({"w.ts"}))


if __name__ == "__main__":
    unittest.main()
