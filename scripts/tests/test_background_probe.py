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

# A background run as another process holds it: the lock, until the stop file appears; one never asked
# gives up after ten seconds and ends 1, so a stop that never asks fails its test rather than hangs it.
HOLDS_THE_LOCK_UNTIL_STOPPED = """
import fcntl, sys, time
from pathlib import Path
here = Path(sys.argv[1])
with open(here / "background.lock", "a") as lock:
    fcntl.flock(lock, fcntl.LOCK_EX)
    print("holding", flush=True)
    given_up_at = time.monotonic() + 10
    while not (here / "stop").exists():
        if time.monotonic() > given_up_at:
            sys.exit(1)
        time.sleep(0.05)
"""


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

        def fake(_here, command):
            made.append(command)
            if len(made) <= again_during:
                (self.here / bp.AGAIN).touch()
            return 0

        with self.repo.cwd(), mock.patch.object(bp, "base", return_value="abc"), \
                mock.patch.object(bp, "probe_until_stopped", side_effect=fake):
            bp.run()
        return made

    def test_a_run_probes_the_code_part_niced_on_two_trees_recording_no_failures(self):
        made = self.probes()
        self.assertEqual(len(made), 1)
        command = made[0]
        self.assertEqual(command[:6], ["ionice", "-c", "3", "nice", "-n", "19"])
        for part in (["--part", "code"], ["--jobs", "2"], ["--base", "abc"]):
            self.assertIn(" ".join(part), " ".join(command))
        self.assertIn("--unrecorded", command)

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

    def test_the_gates_own_run_stops_it_by_the_stop_file_and_says_how_long_it_waited(self):
        # 2026-11-03: check-pr, in the gate's container, signalled a process group numbered on the
        # host, reached nothing, and waited eight minutes in silence for the run to end on its own.
        going = subprocess.Popen([sys.executable, "-c", HOLDS_THE_LOCK_UNTIL_STOPPED, str(self.here)],
                                 stdout=subprocess.PIPE, text=True)
        self.addCleanup(going.kill)
        self.assertEqual(going.stdout.readline().strip(), "holding")
        with self.repo.cwd() as out, mock.patch.dict(os.environ, {bp.IN_BACKGROUND: ""}):
            waited = bp.stop(self.repo.root)
        self.assertEqual(going.wait(timeout=10), 0, "the run saw the stop file and ended")
        self.assertGreaterEqual(waited, 0.0)
        self.assertIn("background probe: asked the run going to stop at", out.getvalue())
        self.assertRegex(out.getvalue(), r"background probe: it ended \d+ s after it was asked")
        self.assertFalse((self.here / bp.STOP).exists(), "the stop file goes once the run has ended")

    def test_with_none_going_it_waits_for_nothing_and_leaves_no_stop_file(self):
        with self.repo.cwd() as out, mock.patch.dict(os.environ, {bp.IN_BACKGROUND: ""}):
            self.assertEqual(bp.stop(self.repo.root), 0.0)
        self.assertEqual(out.getvalue(), "")
        self.assertFalse((self.here / bp.STOP).exists())

    def test_a_run_of_its_own_does_not_stop_itself(self):
        with open(self.here / bp.LOCK, "a", encoding="utf-8") as held, \
                mock.patch.dict(os.environ, {bp.IN_BACKGROUND: "1"}):
            fcntl.flock(held, fcntl.LOCK_EX)
            self.assertEqual(bp.stop(self.repo.root), 0.0)
        self.assertFalse((self.here / bp.STOP).exists())

    def test_a_probe_going_ends_when_the_stop_file_appears(self):
        (self.here / bp.STOP).touch()
        with self.repo.cwd() as out, mock.patch.object(bp, "STOP_POLL_SECONDS", 0.05):
            status = bp.probe_until_stopped(self.here, [sys.executable, "-c", "import time; time.sleep(60)"])
        self.assertEqual(status, -bp.signal.SIGTERM)
        self.assertIn("background probe: stopped at", out.getvalue())

    def test_a_run_starts_no_probe_while_a_stop_is_asked(self):
        (self.here / bp.STOP).touch()
        self.assertEqual(self.probes(), [])

    def test_its_log_says_when_each_run_started_and_ended(self):
        with self.repo.cwd() as out, mock.patch.object(bp, "base", return_value="abc"), \
                mock.patch.object(bp, "probe_until_stopped", return_value=0):
            bp.run()
        self.assertRegex(out.getvalue(), r"background probe: abc\.\.\w+, started at \d\d:\d\d:\d\d UTC")
        self.assertRegex(out.getvalue(), r"background probe: ended at \d\d:\d\d:\d\d UTC, after \d+ s \(exit 0\)")

    def test_a_pass_waits_behind_the_gates_own_run_and_goes_beside_a_spot_check(self):
        # The owner's 11-24 batch, item 4 (2026-11-12): stopped by every session run in turn, the probe
        # left check-pr few files probed; it now goes beside a spot check, and gives way to make check.
        seen: list[str | None] = []
        with self.repo.cwd(), open(bp.gate_lock.lock_path(), "a+", encoding="utf-8") as other:
            bp.fcntl.flock(other, fcntl.LOCK_EX)
            bp.gate_lock.mark(other, "check")

            def check_ends_and_a_spot_starts(_seconds):
                # Bounded: a probe that waits behind the spot check too would sleep here forever,
                # each sleep instant, and hang the suite rather than fail it (check-pr, 2026-11-13).
                if len(seen) >= 3:
                    raise AssertionError(f"it still waits behind {seen[-1]!r} after {len(seen)} waits")
                seen.append(bp.gate_lock.holder_kind(bp.gate_lock.holder()))
                bp.gate_lock.mark(other, "spot")

            def fake(_here, _command):
                seen.append(bp.gate_lock.holder_kind(bp.gate_lock.holder()))
                return 0

            with mock.patch.object(bp, "base", return_value="abc"), \
                    mock.patch.object(bp.time, "sleep", side_effect=check_ends_and_a_spot_starts), \
                    mock.patch.object(bp, "probe_until_stopped", side_effect=fake):
                bp.run()
        self.assertEqual(seen, ["check", "spot"], "it waited behind make check, then probed beside the spot check")

    def test_a_stop_asked_while_it_waits_behind_the_gates_own_run_ends_it_unprobed(self):
        with self.repo.cwd(), open(bp.gate_lock.lock_path(), "a+", encoding="utf-8") as other:
            fcntl.flock(other, fcntl.LOCK_EX)
            bp.gate_lock.mark(other, "check-pr")
            with mock.patch.object(bp.time, "sleep", side_effect=lambda _s: (self.here / bp.STOP).touch()):
                self.assertEqual(self.probes(), [])

    def test_it_says_whether_a_run_is_going(self):
        self.assertFalse(bp.is_running(self.repo.root))
        with open(self.here / bp.LOCK, "a", encoding="utf-8") as lock:
            fcntl.flock(lock, fcntl.LOCK_EX)
            done = subprocess.run([sys.executable, "-c", "import sys; sys.path.insert(0, sys.argv[1]); "
                                   "import background_probe as bp; from pathlib import Path; "
                                   "sys.exit(0 if bp.is_running(Path(sys.argv[2])) else 1)",
                                   str(Path(bp.__file__).parent), str(self.repo.root)], check=False)
            self.assertEqual(done.returncode, 0, "the run holding its lock is going")
        self.assertFalse(bp.is_running(self.repo.root))

    def test_a_probe_runs_revert_red_as_a_sub_run_of_the_lock_it_holds(self):
        with self.repo.cwd() as out, mock.patch.dict(os.environ, {bp.gate_lock.GATE_LOCK_HELD: ""}):
            status = bp.probe_until_stopped(self.here, [
                sys.executable, "-c", f"import os, sys; sys.exit(os.environ['{bp.gate_lock.GATE_LOCK_HELD}'] != '1')"])
        self.assertEqual(status, 0, out.getvalue())


class Environment(unittest.TestCase):
    def test_the_run_takes_none_of_the_hooks_git_variables(self):
        # Its first run, 2026-10-31: a hook's GIT_INDEX_FILE sent every probe tree's git to the commit's index.
        hook = {"PATH": "/bin", "GIT_DIR": ".git", "GIT_INDEX_FILE": ".git/index", "GIT_PREFIX": ""}
        self.assertEqual(bp.run_environment(hook), {"PATH": "/bin", bp.IN_BACKGROUND: "1"})


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
