# SPDX-License-Identifier: AGPL-3.0-or-later
"""Tests for scripts/gate_runs.py: every gate run's time, kept and reported against its budget."""

from __future__ import annotations

import re
import sys
import tempfile
import unittest
from pathlib import Path

SCRIPTS = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(SCRIPTS))

import gate_runs  # noqa: E402


def run(target: str, seconds: int, exit_code: int = 0, steps=None, start: str = "2026-11-21T10:00:00Z") -> dict:
    return {"start": start, "target": target, "seconds": seconds, "exit": exit_code, "steps": steps or []}


class Report(unittest.TestCase):
    def test_each_target_says_its_median_and_how_many_went_over_its_budget(self):
        runs = [run("check", 40), run("check", 80), run("check", 200, steps=[["web-lint", 150.0]]),
                run("check", 300, steps=[["web-lint", 210.0]]), run("check", 30, exit_code=2),
                run("spot", 12), run("spot", 45, steps=[["go test, 3 package(s)", 40.0]])]
        report = gate_runs.report(runs, "2026-11-20T00:00:00Z")
        self.assertIn("check: 5 run(s), 1 failed; median 140, slowest tenth 300, max 300; 2 over its 90 s budget", report)
        self.assertIn("the longest step of the runs over it: web-lint 2", report)
        self.assertIn("spot: 2 run(s), 0 failed; median 28, slowest tenth 45, max 45; 1 over its 30 s budget", report)

    def test_a_run_before_the_window_is_left_out(self):
        self.assertIn("no run recorded", gate_runs.report([run("check", 40, start="2026-11-01T00:00:00Z")],
                                                          "2026-11-20T00:00:00Z"))

    def test_over_budget_names_the_budget_only_past_it(self):
        self.assertEqual(gate_runs.over_budget("check", 90), "")
        self.assertEqual(gate_runs.over_budget("check", 91), ", over its 90 s budget")
        self.assertEqual(gate_runs.over_budget("check-documentation", 900), "")


class Record(unittest.TestCase):
    def test_runs_are_appended_and_the_oldest_let_go(self):
        path = Path(tempfile.mkdtemp()) / "gate-logs" / "runs.jsonl"
        kept = gate_runs.KEPT
        gate_runs.KEPT = 10
        self.addCleanup(setattr, gate_runs, "KEPT", kept)
        for seconds in range(11):
            gate_runs.record(run("spot", seconds), path)
        self.assertEqual([r["seconds"] for r in gate_runs.read(path)], list(range(11)), "within a tenth over, kept")
        gate_runs.record(run("spot", 11), path)
        self.assertEqual([r["seconds"] for r in gate_runs.read(path)], list(range(2, 12)))

    def test_check_prs_budget_is_the_makefiles(self):
        makefile = (SCRIPTS.parent / "Makefile").read_text()
        self.assertEqual(int(re.search(r"^CHECK_PR_BUDGET_S := (\d+)$", makefile, re.M).group(1)),
                         gate_runs.BUDGET_S["check-pr"])


if __name__ == "__main__":
    unittest.main()
