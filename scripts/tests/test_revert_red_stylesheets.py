# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for revert_red_stylesheets.py: changed stylesheets settled by halves, in far fewer
# ABOUTME: browser runs than one per sheet, a failing run's last lines kept.
import subprocess
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
import revert_red_stylesheets as sheets  # noqa: E402


def runner(noticed: set[str], runs: list[list[str]], timing_out: frozenset[str] = frozenset()):
    def run(group: list[str]) -> subprocess.CompletedProcess[str] | None:
        runs.append(group)
        if timing_out & set(group):
            return None
        failing = bool(noticed & set(group))
        return subprocess.CompletedProcess(group, 1 if failing else 0, "line\n" * 30 if failing else "", "")
    return run


class Halving(unittest.TestCase):
    NAMES = [f"s{i}.css" for i in range(13)]

    def test_nothing_noticed_takes_one_run(self):
        runs: list[list[str]] = []
        halving = sheets.settle(self.NAMES, runner(set(), runs))
        self.assertEqual((len(runs), sorted(halving.passed), halving.noticed), (1, sorted(self.NAMES), {}))

    def test_one_noticed_sheet_among_thirteen_takes_far_fewer_runs_than_one_each(self):
        runs: list[list[str]] = []
        halving = sheets.settle(self.NAMES, runner({"s9.css"}, runs))
        self.assertEqual(list(halving.noticed), ["s9.css"])
        self.assertEqual(sorted(halving.passed), sorted(set(self.NAMES) - {"s9.css"}))
        self.assertLessEqual(len(runs), 9, "one sheet alone each would be 14 runs")
        self.assertTrue(all(len(failure.splitlines()) <= sheets.KEPT_LINES + 1 for failure in halving.failures),
                        "a failing run's last lines, not its whole output")

    def test_a_timed_out_run_leaves_its_sheets_unsettled(self):
        runs: list[list[str]] = []
        halving = sheets.settle(self.NAMES, runner(set(), runs, frozenset({"s0.css"})))
        self.assertEqual(sorted(halving.unsettled), sorted(self.NAMES))
        self.assertEqual(halving.passed, [])


if __name__ == "__main__":
    unittest.main()
