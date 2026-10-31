# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for revert_red_clean.py: a clean run is asked once per tree and kind, a pass is kept on
# ABOUTME: disk for the same commit and toolchain, and a failure is never kept.
import os
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

SCRIPTS = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(SCRIPTS))
import revert_red_clean as clean  # noqa: E402
import revert_red_known as known  # noqa: E402

HERMETIC = {"GIT_CONFIG_GLOBAL": "/dev/null", "GIT_CONFIG_NOSYSTEM": "1"}


def done(code: int) -> subprocess.CompletedProcess[str]:
    return subprocess.CompletedProcess([], code, "out", "")


class CleanRuns(unittest.TestCase):
    def setUp(self):
        self.tree = tempfile.mkdtemp(prefix="revert-red-clean-")
        self.addCleanup(shutil.rmtree, self.tree, ignore_errors=True)
        subprocess.run(["git", "init", "-q", self.tree], check=True, env=os.environ | HERMETIC)
        clean._CLEAN.clear()
        clean._FAILED_OUTPUT.clear()
        self.addCleanup(clean._CLEAN.clear)
        patched = mock.patch.object(clean, "toolchain", return_value="go1.26 v22")
        patched.start()
        self.addCleanup(patched.stop)

    def test_a_pass_is_asked_once_and_kept_for_the_next_run_at_the_same_commit(self):
        run = mock.Mock(return_value=done(0))
        self.assertTrue(clean.runs_clean(self.tree, "go package", run, "abc123"))
        self.assertTrue(clean.runs_clean(self.tree, "go package", run, "abc123"))
        self.assertEqual(run.call_count, 1, "once per tree and kind in a run")
        clean._CLEAN.clear()  # the next check-pr-failed, a new process
        self.assertTrue(clean.runs_clean(self.tree, "go package", run, "abc123"))
        self.assertEqual(run.call_count, 1, "served from the kept pass")

    def test_a_kept_pass_holds_only_for_its_commit_kind_and_toolchain(self):
        clean.runs_clean(self.tree, "go package", mock.Mock(return_value=done(0)), "abc123")
        for tree_kind_head in (("vitest related", "abc123"), ("go package", "def456")):
            clean._CLEAN.clear()
            run = mock.Mock(return_value=done(0))
            clean.runs_clean(self.tree, tree_kind_head[0], run, tree_kind_head[1])
            self.assertEqual(run.call_count, 1, tree_kind_head)
        clean._CLEAN.clear()
        run = mock.Mock(return_value=done(0))
        with mock.patch.object(clean, "toolchain", return_value="go1.27 v24"):
            clean.runs_clean(self.tree, "go package", run, "def456")
        self.assertEqual(run.call_count, 1, "another toolchain asks again")

    def test_a_pass_in_one_tree_serves_every_tree_and_a_failure_stays_its_own(self):
        # The test-regime audit of 2026-10-31, R5: four trees each paid for the same clean run.
        other = self.tree + "/other"
        os.makedirs(other)  # a directory inside the same repository, as a second tree is
        run = mock.Mock(return_value=done(0))
        self.assertTrue(clean.runs_clean(self.tree, "go package", run, "abc123"))
        self.assertTrue(clean.runs_clean(other, "go package", run, "abc123"))
        self.assertEqual(run.call_count, 1, "the second tree takes the first's pass")
        clean._CLEAN.clear()  # a new process: the kept pass serves another tree too
        self.assertTrue(clean.runs_clean(other, "go package", run, "abc123"))
        self.assertEqual(run.call_count, 1)
        failing = mock.Mock(return_value=done(1))
        self.assertFalse(clean.runs_clean(self.tree, "vitest related", failing, "abc123"))
        self.assertTrue(clean.runs_clean(other, "vitest related", mock.Mock(return_value=done(0)), "abc123"))

    def test_a_failure_is_never_kept(self):
        self.assertFalse(clean.runs_clean(self.tree, "go package", mock.Mock(return_value=done(1)), "abc123"))
        self.assertIn("out", clean.clean_failure(self.tree, "go package"))
        clean._CLEAN.clear()
        run = mock.Mock(return_value=done(0))
        self.assertTrue(clean.runs_clean(self.tree, "go package", run, "abc123"))
        self.assertEqual(run.call_count, 1, "asked again")

    def test_with_no_commit_named_nothing_is_kept(self):
        self.assertTrue(clean.runs_clean(self.tree, "go package", mock.Mock(return_value=done(0))))
        self.assertFalse(clean.kept_path(self.tree).exists())


class FailedAgain(unittest.TestCase):
    """A red's failed tests, run again with nothing reverted, must pass for it to count (2026-10-28)."""

    def runs(self):
        return known.runs_of({"go": {"./internal/a": ["TestA"]}, "python": ["test_b.py"]})

    def test_a_red_counts_when_its_failed_tests_pass_with_nothing_reverted(self):
        run = mock.Mock(return_value=done(0))
        self.assertEqual(clean.failed_pass_clean(self.runs(), run), (True, ""))
        self.assertEqual(run.call_args_list[0].args[0][-3:], ["-run", "^(TestA)$", "./internal/a"])
        self.assertEqual(run.call_args_list[1].args[0][-1], "scripts/tests/test_b.py")

    def test_a_red_whose_failed_tests_fail_anyway_says_nothing_and_shows_why(self):
        run = mock.Mock(side_effect=[done(0), subprocess.CompletedProcess([], 1, "", "flaky under load")])
        passed, said = clean.failed_pass_clean(self.runs(), run)
        self.assertFalse(passed)
        self.assertIn("flaky under load", said)

    def test_a_run_past_its_timeout_has_not_passed(self):
        run = mock.Mock(side_effect=subprocess.TimeoutExpired(["go"], 1))
        self.assertEqual(clean.failed_pass_clean(self.runs(), run), (False, "it ran past its timeout"))

    def test_with_no_failed_tests_named_the_kinds_clean_run_decides(self):
        self.assertEqual(clean.failed_pass_clean([], mock.Mock()), (None, ""))


if __name__ == "__main__":
    unittest.main()
