# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for gate_lock.py: one gate at a time on a repository, a second waits, a gate's own
# ABOUTME: sub-runs pass through, and whether one runs can be asked without waiting.
import os
import subprocess
import sys
import tempfile
import time
import unittest
from pathlib import Path

SCRIPTS = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(SCRIPTS))
import gate_lock  # noqa: E402

HERMETIC = {"GIT_CONFIG_GLOBAL": "/dev/null", "GIT_CONFIG_NOSYSTEM": "1"}


def repository() -> Path:
    root = Path(tempfile.mkdtemp(prefix="gate-lock-"))
    subprocess.run(["git", "init", "-q", str(root)], check=True, env=os.environ | HERMETIC)
    return root


def run_lock(root: Path, *command: str, held: bool = False) -> subprocess.Popen[str]:
    env = {k: v for k, v in os.environ.items() if k != gate_lock.GATE_LOCK_HELD} | HERMETIC
    if held:
        env[gate_lock.GATE_LOCK_HELD] = "1"
    return subprocess.Popen([sys.executable, str(SCRIPTS / "gate_lock.py"), *command], cwd=root, env=env,
                            stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)


class GateLock(unittest.TestCase):
    def test_a_second_gate_waits_for_the_first_and_says_so(self):
        root = repository()
        marker = root / "first-ran"
        first = run_lock(root, "sh", "-c", f"sleep 1; touch {marker}")
        time.sleep(0.3)  # the first holds the lock
        self.assertEqual(run_lock(root, "--held").wait(), 0, "a gate holds it")
        second = run_lock(root, "sh", "-c", f"test -e {marker}")
        out, _ = second.communicate(timeout=30)
        first.wait(timeout=30)
        self.assertEqual(second.returncode, 0, "the second ran only after the first finished")
        self.assertIn("waiting for it to finish", out)
        self.assertEqual(run_lock(root, "--held").wait(), 1, "none holds it once both ended")

    def test_a_gates_own_sub_run_passes_through(self):
        root = repository()
        first = run_lock(root, "sh", "-c", "sleep 2")
        time.sleep(0.3)
        inner = run_lock(root, "true", held=True)
        started = time.monotonic()
        self.assertEqual(inner.wait(timeout=30), 0)
        self.assertLess(time.monotonic() - started, 1.5, "a held run did not wait on its own lock")
        first.wait(timeout=30)


class Wiring(unittest.TestCase):
    def test_the_gates_take_the_lock(self):
        """make check, revert-red's targets and check-pr's snapshot each hold it (F17)."""
        makefile = (SCRIPTS.parent / "Makefile").read_text()
        recipe = makefile.split("\ncheck: ", 1)[1].split("\n\n", 1)[0]
        self.assertIn("$(GATE_LOCK)", recipe)
        for line in makefile.splitlines():
            if "scripts/check_revert_red.py" in line and line.startswith("\t"):
                self.assertIn("$(GATE_LOCK)", line, line)
        self.assertIn("gate_lock.holding(", (SCRIPTS / "check_pr_snapshot.py").read_text())


if __name__ == "__main__":
    unittest.main()
