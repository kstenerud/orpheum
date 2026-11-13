# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for quiet_gate.py: a failure keeps every line that is not known passing noise,
# ABOUTME: a pass prints one line, and the full output always reaches the log.
import io
import os
import subprocess
import sys
import tempfile
import time
import unittest
from contextlib import redirect_stdout
from pathlib import Path
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
import quiet_gate as qg  # noqa: E402

PASSING_NOISE = """ok  \tgithub.com/x/orpheum/internal/core/api\t13.9s
?   \tgithub.com/x/orpheum/internal/core/turn\t[no test files]
warning: internal/core/api/api_test.go: 509 lines (over 400; consider splitting)
check_source: 701 source files, 0 problems
check_docs: 141 files, 5413 links, 0 problems
>> unused --tests=false linux/amd64
 Test Files  46 passed (46)
      Tests  155 passed (155)
Ran 115 tests in 26.157s
OK
"""


class Summary(unittest.TestCase):
    def test_a_failure_keeps_every_line_that_is_not_passing_noise(self):
        failure = "internal/x.go:3:1: The line is 125 characters long (lll)\n--- FAIL: TestSomething (0.1s)\n"
        shown = qg.summary(PASSING_NOISE + failure + "some line nobody listed\n", Path("/log"))
        self.assertIn("(lll)", shown)
        self.assertIn("--- FAIL: TestSomething", shown)
        self.assertIn("some line nobody listed", shown)  # an unknown line is never hidden
        self.assertNotIn("ok  \t", shown)
        self.assertNotIn("consider splitting", shown)
        self.assertNotIn("Test Files", shown)
        self.assertIn("/log", shown)

    def test_a_steps_clock_lines_are_noise_but_its_failure_is_shown(self):
        out = ("check-pr step fast: started at 10:50:45 UTC\ncheck-pr step fast: passed at 10:50:46 UTC, after 1 s\n"
               "check-pr step slow: failed at 11:15:28 UTC, after 1483 s\n")
        shown = qg.summary(out, Path("/log"))
        self.assertNotIn("started at", shown)
        self.assertNotIn("fast: passed", shown)
        self.assertIn("check-pr step slow: failed at 11:15:28 UTC", shown)

    def test_a_long_failure_is_capped_and_says_how_much_is_left(self):
        shown = qg.summary("\n".join(f"error {n}" for n in range(qg.MAX_LINES + 7)), Path("/log"))
        self.assertIn(f"error {qg.MAX_LINES - 1}", shown)
        self.assertNotIn(f"error {qg.MAX_LINES}\n", shown)
        self.assertIn("… 7 more lines", shown)


STEP_CLOCK_SH = Path(__file__).resolve().parent.parent / "step_clock.sh"
REPO = Path(__file__).resolve().parents[2]


class StepTimes(unittest.TestCase):
    """Gate-throughput G0 (the owner, 2026-11-13): each step's time, from step_clock.sh's lines."""

    def test_a_steps_time_runs_from_its_first_lines_start_to_its_last_lines_end_the_longest_first(self):
        clock = "a\t100.0\t101.0\t0\na\t101.0\t104.5\t0\nb\t100.5\t102.0\t1\nbroken line\n"
        self.assertEqual(qg.step_times(clock), [("a", 0.0, 4.5, False), ("b", 0.5, 1.5, True)])

    def test_the_report_names_each_step_and_its_failure(self):
        report = qg.steps_report("make gate", "lint\t10.0\t12.0\t2\n")
        self.assertIn("lint  (failed)", report)
        self.assertIn("     2.0  +   0.0  lint", report)

    def test_the_projects_makefile_runs_its_recipes_through_the_clock_only_when_asked(self):
        def shell(env: dict[str, str]) -> str:
            done = subprocess.run(["make", "-pn", "help"], cwd=REPO, capture_output=True, text=True, check=False,
                                  env={k: v for k, v in env.items() if not k.startswith("MAKE")})
            return next(line for line in done.stdout.splitlines() if line.startswith("SHELL :="))
        self.assertTrue(shell({**os.environ, "STEP_CLOCK": "/dev/null"}).endswith("scripts/step_clock.sh"))
        self.assertEqual(shell({k: v for k, v in os.environ.items() if k != "STEP_CLOCK"}), "SHELL := bash")


class Running(unittest.TestCase):
    def run_gate(self, makefile: str, verbose: bool = False, root: Path | None = None,
                 fail_fast: bool = False) -> tuple[int, str, Path]:
        # A make the tests run under (CI's `make check`) passes its jobserver down in MAKEFLAGS; this
        # gate's own make, a top-level one, would warn that the jobserver is unavailable after the
        # gate's last line, which is the line a pass prints.
        parents_make = {"MAKEFLAGS", "MFLAGS", "MAKELEVEL", "MAKE_TERMOUT", "MAKE_TERMERR"}
        env = {k: v for k, v in os.environ.items()
               if not k.startswith("GIT_") and k != "VERBOSE" and k not in parents_make}
        if root is None:
            root = Path(tempfile.mkdtemp(prefix="quiet-gate-"))
            subprocess.run(["git", "init", "-q"], cwd=root, check=True, env=env)
        (root / "Makefile").write_text(makefile)
        if verbose:
            env["VERBOSE"] = "1"
        cwd = os.getcwd()
        out = io.StringIO()
        try:
            os.chdir(root)
            with mock.patch.dict(os.environ, env, clear=True), redirect_stdout(out):
                code = qg.main([*(["--fail-fast"] if fail_fast else []), "make gate", "gate"])
        finally:
            os.chdir(cwd)
        return code, out.getvalue(), root / ".git" / "gate-logs" / "gate.log"

    def test_a_pass_prints_one_line_and_keeps_the_rest_in_the_log(self):
        code, printed, log = self.run_gate("gate:\n\t@echo 'ok  \tpkg\t1s'\n\t@echo 'make gate: all gates passed'\n")
        self.assertEqual(code, 0)
        self.assertEqual(printed.strip(), "make gate: all gates passed")
        self.assertIn("ok  \tpkg", log.read_text())

    def test_a_pass_shows_the_vulnerability_report_too(self):
        # The owner, 2026-10-30: a prompt to upgrade, which a passing check-pr must not hide.
        code, printed, _ = self.run_gate("gate:\n\t@echo 'check-vulns: Go: 1 known vulnerability'\n"
                                         "\t@echo 'ok  \tpkg\t1s'\n\t@echo 'make gate: all gates passed'\n")
        self.assertEqual(code, 0)
        self.assertEqual(printed.strip().splitlines(),
                         ["check-vulns: Go: 1 known vulnerability", "make gate: all gates passed"])

    def test_a_failure_prints_its_errors_fails_and_names_the_log(self):
        code, printed, log = self.run_gate("gate:\n\t@echo 'ok  \tpkg\t1s'\n\t@echo 'broken thing'\n\t@false\n")
        self.assertNotEqual(code, 0)
        self.assertIn("broken thing", printed)
        self.assertNotIn("ok  \tpkg", printed)
        self.assertIn(str(log), printed)
        self.assertIn("make gate: FAILED", printed)

    def test_a_failures_log_outlives_the_passing_run_after_it(self):
        """A hang's diagnostic was overwritten by the passing re-run's log (audit-2026-09-30 §5)."""
        code, printed, log = self.run_gate("gate:\n\t@echo 'the hang, diagnosed'\n\t@false\n")
        failed = log.with_name("gate.failed.log")
        self.assertNotEqual(code, 0)
        self.assertIn(f"it is kept in {failed} until the next failure", printed)
        code, _, _ = self.run_gate("gate:\n\t@echo 'make gate: all gates passed'\n", root=log.parents[2])
        self.assertEqual(code, 0)
        self.assertNotIn("the hang", log.read_text())
        self.assertIn("the hang, diagnosed", failed.read_text())

    def test_fail_fast_stops_the_steps_still_running_at_the_first_failure(self):
        """A lint error waited for the Go linter and the test suites to finish (the owner, 2026-10-06)."""
        makefile = ("gate:\n\t@$(MAKE) --no-print-directory -j2 fast slow\n"
                    "fast:\n\t@sleep 0.5\n\t@echo 'lint: broken'\n\t@false\n"
                    "slow:\n\t@sleep 30\n\t@echo 'slow step finished'\n")
        started = time.monotonic()
        code, printed, log = self.run_gate(makefile, fail_fast=True)
        self.assertLess(time.monotonic() - started, 20, "the slow step was waited for")
        self.assertEqual(code, 2, "a stopped run fails as make does, not with the signal's status")
        self.assertIn("lint: broken", printed)
        self.assertIn(qg.STOPPED_NOTE, printed)
        self.assertNotIn("slow step finished", log.read_text())
        self.assertIn("make gate: FAILED", printed)

    def test_without_fail_fast_every_running_step_finishes(self):
        makefile = ("gate:\n\t@$(MAKE) --no-print-directory -j2 fast slow\n"
                    "fast:\n\t@sleep 0.5\n\t@echo 'lint: broken'\n\t@false\n"
                    "slow:\n\t@sleep 1\n\t@echo 'slow step finished'\n")
        code, printed, log = self.run_gate(makefile)
        self.assertNotEqual(code, 0)
        self.assertIn("slow step finished", log.read_text())
        self.assertNotIn(qg.STOPPED_NOTE, printed)

    def test_a_run_says_how_long_each_step_took_its_parse_time_shell_unclocked(self):
        makefile = (f"SHELL := {STEP_CLOCK_SH}\n.SHELLFLAGS = $@ -eu -o pipefail -c\nPARSED := $(shell echo parsed)\n"
                    "gate:\n\t@$(MAKE) --no-print-directory -j2 fast slow\n\t@echo 'make gate: $(PARSED)'\n"
                    "fast:\n\t@true\nslow:\n\t@sleep 0.4\n\t@set -o | grep -q 'pipefail.*on'\n")
        code, printed, log = self.run_gate(makefile)
        self.assertEqual((code, printed.strip()), (0, "make gate: parsed"))
        steps = log.with_name("gate.steps").read_text().splitlines()
        self.assertEqual([line.split()[-1] for line in steps[1:]], ["gate", "slow", "fast"])
        self.assertGreaterEqual(float(steps[2].split()[0]), 0.4)

    def test_verbose_prints_everything(self):
        code, _, log = self.run_gate("gate:\n\t@echo 'ok  \tpkg\t1s'\n", verbose=True)
        self.assertEqual(code, 0)
        self.assertFalse(log.exists())  # verbose streams to the terminal, as before


if __name__ == "__main__":
    unittest.main()
