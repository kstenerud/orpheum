# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for revert_red_run.py: a probe's run is ended whole, so a process it started, as a
# ABOUTME: browser tier starts a server, does not outlive it, whether it finished or timed out.
import os
import subprocess
import sys
import tempfile
import threading
import time
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
import check_revert_red as rr  # noqa: E402
import revert_red_attempts as attempts  # noqa: E402
import revert_red_commands  # noqa: E402
import revert_red_run as rr_run  # noqa: E402
from revert_red_commands import shared  # noqa: E402
from revert_red_run import run_whole_group  # noqa: E402


def is_running(pid: int) -> bool:
    try:
        os.kill(pid, 0)
    except ProcessLookupError:
        return False
    # A killed child of ours that no one reaped yet is gone in all but name.
    stat = Path(f"/proc/{pid}/stat")
    return not stat.exists() or stat.read_text().split(") ")[-1][0] != "Z"


def gone_soon(pid: int) -> bool:
    deadline = time.monotonic() + 5
    while time.monotonic() < deadline:
        if not is_running(pid):
            return True
        time.sleep(0.05)
    return False


class WholeGroup(unittest.TestCase):
    def started(self, script: str, timeout: int) -> tuple[subprocess.CompletedProcess[str] | None, int]:
        """The run of script, after starting a process it leaves behind, and that process's pid. A run
        its timeout ends before the shell even writes the pid, as under a loaded gate's 1 s (make
        check, 2026-10-20), proves nothing, so it is run again, each second for up to 5 s."""
        deadline = time.monotonic() + 5
        while True:
            with tempfile.TemporaryDirectory() as tree:
                pid_file = Path(tree) / "pid"
                # Written whole or not at all: the pid's file is moved into place once written.
                leave = f"sleep 60 & echo $! > {pid_file}.part && mv {pid_file}.part {pid_file}"
                command = ["sh", "-c", f"{leave}; {script}"]
                try:
                    done = run_whole_group(command, tree, timeout)
                except subprocess.TimeoutExpired:
                    done = None
                if pid_file.exists():
                    return done, int(pid_file.read_text())
            if time.monotonic() > deadline:
                self.fail("no run lived long enough to start the process it leaves behind, in 5 s")
            time.sleep(1)

    def test_a_process_the_run_left_behind_is_ended_with_it(self):
        done, left = self.started("echo said; exit 3", timeout=30)
        assert done is not None
        self.assertEqual((done.returncode, done.stdout), (3, "said\n"))
        self.assertTrue(gone_soon(left), "the process the run started outlived it")

    def test_a_run_past_its_timeout_is_ended_with_everything_it_started(self):
        done, left = self.started("sleep 30", timeout=1)
        self.assertIsNone(done)
        self.assertTrue(gone_soon(left), "the process the timed-out run started outlived it")


class StoppedFromOutside(unittest.TestCase):
    """A stop (the background probe's, 2026-11-03) ends the runs going, and none counts as red."""

    def setUp(self):
        self.addCleanup(rr_run.STOPPING.clear)

    def test_a_run_going_when_the_stop_comes_ends_and_is_not_a_result(self):
        threading.Timer(0.5, rr_run.stop_every_run).start()
        started = time.monotonic()
        with tempfile.TemporaryDirectory() as tree, self.assertRaises(rr_run.Stopped):
            run_whole_group(["sh", "-c", "sleep 30; exit 1"], tree, 60)
        self.assertLess(time.monotonic() - started, 10, "the stop ended the run, not its own end")

    def test_a_run_asked_for_after_the_stop_never_starts(self):
        rr_run.stop_every_run()
        with tempfile.TemporaryDirectory() as tree:
            with self.assertRaises(rr_run.Stopped):
                run_whole_group(["sh", "-c", f"touch {tree}/ran"], tree, 60)
            self.assertFalse((Path(tree) / "ran").exists())


class BrowserProbe(unittest.TestCase):
    def test_a_probes_browser_run_stops_at_its_first_failure(self):
        attempts._installed_ffmpeg.cache_clear()
        original = attempts._installed_ffmpeg
        attempts._installed_ffmpeg = lambda: "/ffmpeg"  # type: ignore[assignment]
        try:
            self.assertIn("E2E_ARGS=--max-failures=1", rr.browser_tier_command())
        finally:
            attempts._installed_ffmpeg = original


if __name__ == "__main__":
    unittest.main()


class CpuShare(unittest.TestCase):
    """A probe's run keeps within its share of the CPUs, so the probes at once fit the machine."""

    def test_a_go_test_runs_its_share_of_packages_with_its_share_of_threads(self) -> None:
        command, env = shared(["go", "test", "-failfast", "./x/..."], {"PATH": "/bin"}, 2)
        self.assertEqual(command, ["go", "test", "-p", "2", "-failfast", "./x/..."])
        self.assertEqual(env, {"PATH": "/bin", "GOMAXPROCS": "2"})

    def test_a_vitest_run_takes_its_share_of_workers(self) -> None:
        related = ["npx", "--prefix", "web", "vitest", "related", "--run", "a.ts"]
        self.assertEqual(shared(related, None, 3)[0], [*related, "--maxWorkers=3"])
        suite = ["npm", "--prefix", "web", "run", "test", "--", "--bail=1"]
        self.assertEqual(shared(suite, None, 3)[0], [*suite, "--maxWorkers=3"])

    def test_another_command_and_a_run_without_a_share_are_unchanged(self) -> None:
        typecheck = ["npm", "--prefix", "web", "run", "typecheck"]
        self.assertEqual(shared(typecheck, None, 2), (typecheck, None))
        go = ["go", "test", "./x/..."]
        self.assertEqual(shared(go, None, None), (go, None))

    def test_the_run_reports_the_command_as_given(self) -> None:
        old = revert_red_commands.CPU_SHARE
        revert_red_commands.CPU_SHARE = 1
        try:
            done = run_whole_group(["true"], tempfile.gettempdir(), 10)
        finally:
            revert_red_commands.CPU_SHARE = old
        self.assertEqual(done.args, ["true"])

    def test_the_run_takes_the_probes_share_of_the_cpus(self) -> None:
        """run_whole_group hands every command to revert_red_commands.shared with the probe's share."""
        seen: list[tuple[list[str], int | None]] = []
        real, old = revert_red_commands.shared, revert_red_commands.CPU_SHARE

        def recording(command: list[str], env: dict[str, str] | None,
                      share: int | None) -> tuple[list[str], dict[str, str] | None]:
            seen.append((command, share))
            return real(command, env, share)

        revert_red_commands.shared, revert_red_commands.CPU_SHARE = recording, 3
        try:
            run_whole_group(["true"], tempfile.gettempdir(), 10)
        finally:
            revert_red_commands.shared, revert_red_commands.CPU_SHARE = real, old
        self.assertEqual(seen, [(["true"], 3)])
