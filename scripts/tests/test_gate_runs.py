# SPDX-License-Identifier: AGPL-3.0-or-later
"""Tests for scripts/gate_runs.py: every gate run's time, kept and reported against its budget."""

from __future__ import annotations

import os
import re
import subprocess
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


class Cpu(unittest.TestCase):
    def write(self, text: str) -> Path:
        path = Path(tempfile.mkdtemp()) / "stat"
        path.write_text(text)
        return path

    def test_the_host_counts_its_work_and_not_its_idle_its_disk_waits_or_its_steal(self):
        # user nice system idle iowait irq softirq steal: 100 + 50 + 30 + 10 + 10 ticks of work.
        stat = self.write("cpu  100 50 30 9000 400 10 10 700 0 0\ncpu0 1 2 3 4 5 6 7 8 0 0\n")
        self.assertEqual(gate_runs.host_busy_s(stat), 200 / os.sysconf("SC_CLK_TCK"))

    def test_the_sandbox_reads_its_cgroups_usage(self):
        stat = self.write("usage_usec 2500000\nuser_usec 2000000\nsystem_usec 500000\n")
        self.assertEqual(gate_runs.sandbox_busy_s(stat), 2.5)

    def test_a_counter_not_there_is_none(self):
        missing = Path(tempfile.mkdtemp()) / "none"
        self.assertIsNone(gate_runs.host_busy_s(missing))
        self.assertIsNone(gate_runs.sandbox_busy_s(missing))
        self.assertIsNone(gate_runs.sandbox_busy_s(self.write("user_usec 5\n")))

    def test_the_cpu_used_is_split_between_the_gate_the_sandbox_and_the_host(self):
        before = {"host": 1000.0, "sandbox": 400.0, "gate": 1.0}
        after = {"host": 1100.0, "sandbox": 460.0, "gate": 41.0}
        self.assertEqual(gate_runs.cpu_used(before, after),
                         {"gate": 40.0, "sandboxOthers": 20.0, "hostOthers": 40.0})

    def test_without_a_cgroup_the_host_others_are_what_the_gate_did_not_use(self):
        before = {"host": 1000.0, "sandbox": None, "gate": 0.0}
        after = {"host": 1100.0, "sandbox": None, "gate": 30.0}
        self.assertEqual(gate_runs.cpu_used(before, after), {"gate": 30.0, "hostOthers": 70.0})

    def test_a_gates_own_children_are_counted(self):
        before = gate_runs.cpu_now()
        subprocess.run([sys.executable, "-c", "sum(range(3_000_000))"], check=True)
        used = gate_runs.cpu_used(before)
        self.assertGreater(used["gate"], 0.0)

    def test_the_report_says_the_cores_each_used(self):
        runs = [{**run("check", 100), "cpu": {"gate": 200.0, "sandboxOthers": 100.0, "hostOthers": 300.0}},
                {**run("check", 50), "cpu": {"gate": 150.0, "sandboxOthers": 0.0, "hostOthers": 50.0}},
                run("check", 70)]
        report = gate_runs.report(runs, "2026-11-20T00:00:00Z")
        self.assertIn("cores used, the median of 2 run(s): the gate 2.5, the sandbox's other work 0.5, "
                      "the host's other work 2.0", report)


if __name__ == "__main__":
    unittest.main()
