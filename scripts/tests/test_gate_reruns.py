# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for the targeted re-runs of the slow branch gates: revert-red's --only and
# ABOUTME: --rerun-failed with its record of failures, and check-pr-failed re-running only the steps
# ABOUTME: that failed; each shown failing on a case built to fail (D190).
import os
import re
import subprocess
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
import check_pr_steps as steps  # noqa: E402
import check_revert_red as rr  # noqa: E402
import last_run  # noqa: E402
import revert_red_reruns as reruns  # noqa: E402
from gitrepo import Repo, mock_argv  # noqa: E402

SCRIPTS = Path(__file__).resolve().parent.parent
REPO = SCRIPTS.parent

COVERED, UNCOVERED, EXEMPT = "scripts/a.py", "scripts/b.py", "scripts/c.py"
TEST = "scripts/tests/test_ab.py"
TEST_HEAD = ("import sys\nimport unittest\nsys.path.insert(0, 'scripts')\nimport a\nimport b\n\n\n"
             "class T(unittest.TestCase):\n    def test_ab(self):\n        a.f()\n        b.g()\n")


class RevertRedTargeted(unittest.TestCase):
    """A branch whose fix changed a covered file, an uncovered one, and a refactor another."""

    def setUp(self):
        self.repo = Repo({COVERED: "def f():\n    return 1\n", UNCOVERED: "def g():\n    return 1\n",
                          EXEMPT: "def h():\n    return 1\n", TEST: TEST_HEAD})
        self.base = self.repo.commit("chore: base")
        self.repo.commit("fix(ab): answer two\n\nThey answered one.", {
            # Each a different size from its base: a probe tree's Python caches bytecode by size and
            # time, and a same-sized restore within the second would run the stale copy.
            COVERED: "def f():\n    return 20\n", UNCOVERED: "def g():\n    return 200\n",
            TEST: TEST_HEAD + "        assert a.f() == 20\n"})
        self.repo.commit("refactor(c): tidy\n\nIt was untidy.", {EXEMPT: "def h():\n    return 1 + 0\n"})

    def gate(self, *args: str, base: str | None = None) -> tuple[int, str]:
        argv = ["check_revert_red.py", "--base", base or self.base, "--jobs", "1", *args]
        with self.repo.cwd() as out, mock_argv(argv):
            status = rr.main()
        return status, out.getvalue()

    def recorded(self) -> list[str]:
        with self.repo.cwd():
            return reruns.recorded_failures(self.repo.git("rev-parse", self.base))[0]

    def test_only_probes_the_paths_it_names_and_says_why_it_skips_one(self):
        status, out = self.gate("--only", COVERED, EXEMPT)
        self.assertEqual(status, 0, out)
        self.assertIn(f"{COVERED}  — a test fails", out)
        self.assertIn("Reverted 1 of the 2 file(s) named.", out)
        self.assertNotIn("exempt by commit type", out, "the branch's count, not the named files'")
        self.assertNotIn(UNCOVERED, out)
        self.assertIn(f"Not probed: {EXEMPT}  — exempt: only refactor commits touched it", out)
        self.assertIn("still required before the branch is done", out)
        status, out = self.gate("--only", UNCOVERED)
        self.assertEqual(status, 1, out)
        self.assertIn(f"{UNCOVERED}  — every test still passes", out)

    def test_only_a_path_the_branch_did_not_change_fails_rather_than_passing_unprobed(self):
        status, out = self.gate("--only", "scripts/nope.py")
        self.assertEqual(status, 1, out)
        self.assertIn("Not probed: scripts/nope.py  — not changed in", out)

    def test_every_run_records_its_failures_and_a_rerun_probes_exactly_those(self):
        status, out = self.gate()
        self.assertEqual(status, 1, out)
        self.assertEqual(self.recorded(), [UNCOVERED])
        status, out = self.gate("--only", COVERED)
        self.assertEqual(status, 0, out)
        self.assertEqual(self.recorded(), [UNCOVERED], "a targeted run forgot a failure it did not probe")

        status, out = self.gate("--rerun-failed")
        self.assertEqual(status, 1, out)
        self.assertIn("Re-running the 1 file(s)", out)
        self.assertIn(UNCOVERED, out)
        self.assertNotIn(COVERED, out)

        # The fix is a new commit: the head moved, the base did not.
        self.repo.commit("test(b): cover g", {TEST: TEST_HEAD + "        assert a.f() == 20\n"
                                                             "        assert b.g() == 200\n"})
        status, out = self.gate("--rerun-failed")
        self.assertEqual(status, 0, out)
        self.assertEqual(self.recorded(), [])
        status, out = self.gate("--rerun-failed")
        self.assertEqual((status, "failed on nothing" in out), (0, True), out)

    def test_a_rerun_against_another_base_or_after_an_unfinished_run_is_refused(self):
        self.gate()
        other = self.repo.git("rev-parse", "HEAD~1")
        status, out = self.gate("--rerun-failed", base=other)
        self.assertEqual(status, 1, out)
        self.assertIn("answer a different question", out)
        with self.repo.cwd():
            reruns.begin(self.repo.git("rev-parse", self.base))
        status, out = self.gate("--rerun-failed")
        self.assertEqual(status, 1, out)
        self.assertIn("did not finish", out)


FAKE_MAKEFILE = f"""\
MAKEFLAGS += --no-print-directory
fast slow check-revert-red check-revert-red-failed:
\t@echo $@ >> ran.log
\t@test ! -f fail-$@
pr-step-%:
\t@python3 {SCRIPTS}/check_pr_steps.py record $* -- $(MAKE) $*
"""


class CheckPrFailed(unittest.TestCase):
    """check-pr's steps in a project of their own: `fast` and `slow` in parallel, then revert-red."""

    def setUp(self):
        self.repo = Repo({"Makefile": FAKE_MAKEFILE, ".gitignore": "ran.log\nfail-*\n"})
        self.base = self.repo.commit("chore: base")
        self.repo.commit("feat: one\n\nIt was missing.", {"one.txt": "1\n"})

    def make(self, *targets: str) -> int:
        return subprocess.run(["make", *targets], cwd=self.repo.root, capture_output=True, check=False).returncode

    def check_pr(self) -> None:
        """What `make check-pr` does, with the fake project's steps."""
        with self.repo.cwd():
            steps.start(self.base)
        if self.make("-k", "pr-step-fast", "pr-step-slow") == 0:
            self.make("pr-step-check-revert-red")
        (self.repo.root / "ran.log").unlink()

    def rerun(self) -> tuple[int, str, list[str]]:
        # In a process of its own, so the makes it runs print into what the test reads, not the
        # suite's output, where their expected failures would read as the suite's.
        done = subprocess.run([sys.executable, str(SCRIPTS / "check_pr_steps.py"), "rerun", "--base", self.base,
                               "--last", "check-revert-red", "--jobs", "2", "fast", "slow"],
                              cwd=self.repo.root, capture_output=True, text=True, check=False)
        log = self.repo.root / "ran.log"
        ran = log.read_text().split() if log.exists() else []
        log.unlink(missing_ok=True)
        return done.returncode, done.stdout + done.stderr, ran

    def test_only_what_failed_runs_again_and_the_rest_is_named_as_passed_on_an_older_commit(self):
        (self.repo.root / "fail-slow").write_text("")
        self.check_pr()
        self.repo.commit("fix: slow\n\nIt was slow.", {"two.txt": "2\n"})
        status, out, ran = self.rerun()
        self.assertEqual((status != 0, ran), (True, ["slow"]), out)
        self.assertIn("  fast  — passed on", out)
        self.assertIn("an older commit", out)
        self.assertIn("check-revert-red did not run", out)
        self.assertIn("the full `make check-pr` is still required", out)

        (self.repo.root / "fail-slow").unlink()
        status, out, ran = self.rerun()
        self.assertEqual((status, ran), (0, ["slow", "check-revert-red"]), out)
        status, out, ran = self.rerun()
        self.assertEqual((status, ran), (0, []), out)
        self.assertIn("the full `make check-pr` is still required", out)
        self.assertFalse((self.repo.root / ".git" / "orpheum-check-passed").exists(), "a re-run wrote the stamp")

    def test_revert_red_that_ran_and_failed_reruns_only_its_failed_files(self):
        (self.repo.root / "fail-check-revert-red").write_text("")
        self.check_pr()
        with self.repo.cwd():
            head = self.repo.git("rev-parse", "HEAD")
            reruns.begin(self.base)
            reruns.finish(self.base, head, [], ["x.go"], targeted=False)
        status, out, ran = self.rerun()
        self.assertEqual((status, ran), (0, ["check-revert-red-failed"]), out)
        self.check_pr()  # revert-red fails again
        with self.repo.cwd():
            reruns.begin(self.base)  # a revert-red run that never finished: its list is not whole
        status, out, ran = self.rerun()
        self.assertEqual((status != 0, ran), (True, ["check-revert-red"]), out)
        self.assertIn("So all of it runs again", out)

    def test_each_step_is_timed_and_the_longest_named_first(self):
        (self.repo.root / "fail-slow").write_text("")
        self.check_pr()
        with self.repo.cwd():
            listed = steps.durations(["fast", "slow", "check-revert-red"])
        self.assertRegex(listed, r"(?m)^\s+\d+ s  fast$")
        self.assertRegex(listed, r"(?m)^\s+\d+ s  slow  \(failed\)$")
        self.assertIn("did not run: check-revert-red", listed)
        with self.repo.cwd():
            record_ = last_run.read(steps.GATE, steps.NAME)
        self.assertIsInstance(record_["results"]["fast"]["seconds"], int)

    def test_a_rerun_against_another_base_or_with_no_record_is_refused(self):
        with self.repo.cwd() as out:
            self.assertEqual(steps.rerun(self.base, ["fast"], "check-revert-red", jobs=1), 1)
        self.assertIn("No check-pr run is recorded", out.getvalue())
        self.check_pr()
        with self.repo.cwd() as out:
            self.assertEqual(steps.rerun(self.repo.git("rev-parse", "HEAD"), ["fast"], "check-revert-red", 1), 1)
        self.assertIn("answer a different question", out.getvalue())


class Wiring(unittest.TestCase):
    MAKEFILE = (REPO / "Makefile").read_text()

    def test_the_real_check_pr_records_every_step_and_the_loop_targets_are_listed(self):
        help_text = subprocess.run(["make", "-f", str(REPO / "Makefile"), "help"], capture_output=True,
                                   text=True, check=True, cwd=REPO).stdout
        for target in ("check-pr-failed", "check-revert-red-failed", "check-revert-red"):
            self.assertIn(f"  {target} ", help_text)
        # The tiers, quickest first, each step recorded; PR_STEPS, which check-pr-failed re-runs, is
        # every tier's (the owner, 2026-10-15).
        tiers = ("PR_FAST_STEPS", "PR_MEDIUM_STEPS", "PR_SERIAL_STEP", "PR_SLOW_STEPS", "PR_LAST_STEP")
        places = []
        for tier in tiers:
            found = re.search(rf"(?m)^\t.*\$\({tier}:%=pr-step-%\)", self.MAKEFILE)
            self.assertIsNotNone(found, tier)
            places.append(found.start())
        self.assertEqual(places, sorted(places), "check-pr runs its tiers out of order")
        self.assertRegex(self.MAKEFILE,
                         r"(?m)^PR_STEPS := \$\(PR_FAST_STEPS\) \$\(PR_MEDIUM_STEPS\) \$\(PR_SLOW_STEPS\)$")
        self.assertRegex(self.MAKEFILE, r"(?m)^\t.*\$\(PR_LAST_STEP:%=pr-step-%\)")
        self.assertRegex(self.MAKEFILE, r"(?m)^pr-step-%:\n\t.*check_pr_steps\.py record \$\*")
        self.assertRegex(self.MAKEFILE, r"(?m)^\t.*check_pr_steps\.py durations \$\(PR_STEPS\) "
                         r"\$\(PR_SERIAL_STEP\) \$\(PR_LAST_STEP\)")
        self.assertRegex(self.MAKEFILE, r"(?m)^\t.*check_revert_red\.py .*\$\(if \$\(ONLY\),--only \$\(ONLY\)\)")


class Records(unittest.TestCase):
    def test_a_record_lives_in_the_common_directory_every_worktree_shares(self):
        repo = Repo({"a": "1\n"})
        repo.commit("chore: a")
        other = repo.root.parent / (repo.root.name + "-wt")
        repo.git("worktree", "add", "-q", "--detach", str(other))
        with repo.cwd():
            last_run.update("g", "n", lambda _old: {"x": 1})
        saved = os.getcwd()
        os.chdir(other)
        try:
            self.assertEqual(last_run.read("g", "n"), {"x": 1})
        finally:
            os.chdir(saved)


if __name__ == "__main__":
    unittest.main()
