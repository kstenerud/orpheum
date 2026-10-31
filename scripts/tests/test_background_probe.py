# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests that the background probe runs one at a time, again for a commit made meanwhile,
# ABOUTME: never while switched off or while check-pr runs, and gives the trees up to the gate's own.
import fcntl
import os
import subprocess
import sys
import unittest
from pathlib import Path
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parent))
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
import background_probe as bp  # noqa: E402
from gitrepo import Repo  # noqa: E402

REAL_RUN = subprocess.run


class BackgroundProbe(unittest.TestCase):
    def setUp(self):
        self.repo = Repo({"a.go": "package a\n"})
        self.repo.commit("chore: base")
        self.here = bp.folder(self.repo.root)
        self.here.mkdir(parents=True, exist_ok=True)
        idle = mock.patch.object(bp, "is_wanted", side_effect=lambda here: not (here / bp.OFF).exists())
        idle.start()
        self.addCleanup(idle.stop)

    def probes(self, again_during: int = 0) -> list[list[str]]:
        """The revert-red runs a run() makes; the first again_during of them see a commit made meanwhile."""
        made: list[list[str]] = []

        def fake(command, **_):
            if "check_revert_red.py" in " ".join(command):
                made.append(command)
                if len(made) <= again_during:
                    (self.here / bp.AGAIN).touch()
                return subprocess.CompletedProcess(command, 0, "", "")
            return REAL_RUN(command, capture_output=True, text=True, check=False)

        with self.repo.cwd(), mock.patch.object(bp, "base", return_value="abc"), \
                mock.patch.object(bp.subprocess, "run", side_effect=fake):
            bp.run()
        return made

    def test_a_run_probes_the_code_part_niced_on_two_trees_recording_no_failures(self):
        made = self.probes()
        self.assertEqual(len(made), 1)
        command = made[0]
        self.assertEqual(command[:3], ["nice", "-n", "19"])
        for part in (["--part", "code"], ["--jobs", "2"], ["--base", "abc"]):
            self.assertIn(" ".join(part), " ".join(command))
        self.assertIn("--unrecorded", command)
        self.assertFalse((self.here / bp.PID).exists(), "its pid is gone as it ends")

    def test_a_commit_made_meanwhile_makes_it_go_again_once(self):
        self.assertEqual(len(self.probes(again_during=1)), 2)

    def test_a_run_while_one_goes_marks_again_and_runs_nothing(self):
        with open(self.here / bp.LOCK, "a", encoding="utf-8") as held:
            fcntl.flock(held, fcntl.LOCK_EX)
            self.assertEqual(self.probes(), [])
        self.assertTrue((self.here / bp.AGAIN).exists())

    def test_switched_off_it_runs_nothing_and_launches_nothing(self):
        (self.here / bp.OFF).touch()
        self.assertEqual(self.probes(), [])
        with self.repo.cwd(), mock.patch.object(bp, "folder", return_value=self.here), \
                mock.patch.object(bp.subprocess, "Popen") as popen:
            bp.launch()
        popen.assert_not_called()

    def test_the_gates_own_run_stops_it_but_a_run_of_its_own_does_not(self):
        (self.here / bp.PID).write_text("4242\n")
        with mock.patch.object(bp.os, "killpg") as kill, mock.patch.dict(os.environ, {bp.IN_BACKGROUND: ""}):
            bp.stop(self.repo.root)
        kill.assert_called_once_with(4242, bp.signal.SIGTERM)
        with mock.patch.object(bp.os, "killpg") as kill, mock.patch.dict(os.environ, {bp.IN_BACKGROUND: "1"}):
            bp.stop(self.repo.root)
        kill.assert_not_called()


class Wanted(unittest.TestCase):
    def test_not_while_check_pr_runs_nor_in_its_tree(self):
        repo = Repo({"a.go": "package a\n"})
        here = bp.folder(repo.root)
        with repo.cwd(), mock.patch("check_pr_snapshot.running", return_value={}):
            self.assertFalse(bp.is_wanted(here), "a check-pr whose record is unreadable still runs")
        with repo.cwd(), mock.patch("check_pr_snapshot.running", return_value=None):
            self.assertTrue(bp.is_wanted(here))


if __name__ == "__main__":
    unittest.main()
