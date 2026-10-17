# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for revert_red_run.py: a probe's run is ended whole, so a process it started, as a
# ABOUTME: browser tier starts a server, does not outlive it, whether it finished or timed out.
import os
import subprocess
import sys
import tempfile
import time
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
import check_revert_red as rr  # noqa: E402
import revert_red_attempts as attempts  # noqa: E402
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
        with tempfile.TemporaryDirectory() as tree:
            pid_file = Path(tree) / "pid"
            command = ["sh", "-c", f"sleep 60 & echo $! > {pid_file}; {script}"]
            try:
                done = run_whole_group(command, tree, timeout)
            except subprocess.TimeoutExpired:
                done = None
            return done, int(pid_file.read_text())

    def test_a_process_the_run_left_behind_is_ended_with_it(self):
        done, left = self.started("echo said; exit 3", timeout=30)
        assert done is not None
        self.assertEqual((done.returncode, done.stdout), (3, "said\n"))
        self.assertTrue(gone_soon(left), "the process the run started outlived it")

    def test_a_run_past_its_timeout_is_ended_with_everything_it_started(self):
        done, left = self.started("sleep 30", timeout=1)
        self.assertIsNone(done)
        self.assertTrue(gone_soon(left), "the process the timed-out run started outlived it")


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
