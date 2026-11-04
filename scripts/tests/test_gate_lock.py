# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for gate_lock.py: one gate at a time on a repository, a second waits, a gate's own
# ABOUTME: sub-runs pass through, and whether one runs can be asked without waiting.
import fcntl
import os
import shutil
import subprocess
import sys
import tempfile
import time
import unittest
from pathlib import Path
from unittest import mock

SCRIPTS = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(SCRIPTS))
import gate_lock  # noqa: E402

HERMETIC = {"GIT_CONFIG_GLOBAL": "/dev/null", "GIT_CONFIG_NOSYSTEM": "1"}


def repository(case: unittest.TestCase) -> Path:
    root = Path(tempfile.mkdtemp(prefix="gate-lock-"))
    case.addCleanup(shutil.rmtree, root, ignore_errors=True)
    subprocess.run(["git", "init", "-q", str(root)], check=True, env=os.environ | HERMETIC)
    return root


def run_lock(root: Path, *command: str, held: bool = False) -> subprocess.Popen[str]:
    env = {k: v for k, v in os.environ.items() if k != gate_lock.GATE_LOCK_HELD} | HERMETIC
    if held:
        env[gate_lock.GATE_LOCK_HELD] = "1"
    return subprocess.Popen([sys.executable, str(SCRIPTS / "gate_lock.py"), *command], cwd=root, env=env,
                            stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)


def wait_for(path: Path, seconds: float = 30) -> None:
    """Waits until path exists: the locked command has started, so the lock is held."""
    deadline = time.monotonic() + seconds
    while not path.exists():
        if time.monotonic() > deadline:
            raise AssertionError(f"{path} never appeared")
        time.sleep(0.02)


class GateLock(unittest.TestCase):
    def test_a_second_gate_waits_for_the_first_and_says_so(self):
        root = repository(self)
        marker = root / "first-ran"
        started = root / "first-started"
        release = root / "release"
        # The first holds the lock until the second says it waits (30 s at most, so a second that says
        # nothing fails rather than hangs): a fixed 1 s ran out before the second asked, at a load of 48
        # (2026-10-31), and the second never had to wait.
        first = run_lock(root, "sh", "-c",
                         f"touch {started}; i=0; while [ ! -e {release} ] && [ $i -lt 1500 ]; do sleep 0.02; "
                         f"i=$((i+1)); done; touch {marker}")
        wait_for(started)  # the first holds the lock: a fixed 0.3 s missed it on a loaded machine
        held = run_lock(root, "--held")
        self.assertEqual(held.wait(), 0, "a gate holds it")
        held.stdout.close()
        second = run_lock(root, "sh", "-c", f"test -e {marker}")
        said = second.stdout.readline()
        release.touch()
        out, _ = second.communicate(timeout=30)
        out = said + out
        first.communicate(timeout=30)
        self.assertEqual(second.returncode, 0, "the second ran only after the first finished")
        self.assertIn("waiting for it to finish", out)
        self.assertEqual(run_lock(root, "--held").wait(), 1, "none holds it once both ended")

    def test_a_gates_own_sub_run_passes_through(self):
        root = repository(self)
        started = root / "first-started"
        first = run_lock(root, "sh", "-c", f"touch {started}; sleep 2")
        wait_for(started)
        inner = run_lock(root, "true", held=True)
        started = time.monotonic()
        self.assertEqual(inner.wait(timeout=30), 0)
        self.assertLess(time.monotonic() - started, 1.5, "a held run did not wait on its own lock")
        first.wait(timeout=30)

    def test_the_holder_says_what_it_is(self):
        root = repository(self)
        started = root / "first-started"
        release = root / "release"
        first = run_lock(root, "--as", "spot", "sh", "-c",
                         f"touch {started}; i=0; while [ ! -e {release} ] && [ $i -lt 1500 ]; do sleep 0.02; "
                         f"i=$((i+1)); done")
        wait_for(started)
        held = run_lock(root, "--held")
        said, _ = held.communicate(timeout=30)
        release.touch()
        first.wait(timeout=30)
        self.assertEqual(held.returncode, 0)
        self.assertRegex(said, r"^spot pid \d+ since \d\d:\d\d:\d\d UTC")
        self.assertEqual(gate_lock.holder_kind(said), "spot")
        self.assertIsNone(gate_lock.holder(root), "nothing holds it once the run ended")

    def test_a_run_behind_the_background_probe_asks_it_to_stop_and_takes_the_lock(self):
        # The probe gives way to every other run (the owner, 2026-11-04): one waiting behind a probe pass
        # of many minutes slowed agents' spot checks as much as running beside it loaded them.
        root = repository(self)
        asked: list[Path | None] = []
        with open(gate_lock.lock_path(root), "a+", encoding="utf-8") as probe:
            fcntl.flock(probe, fcntl.LOCK_EX)
            gate_lock.mark(probe, gate_lock.PROBE)

            def stop(where):
                asked.append(where)
                fcntl.flock(probe, fcntl.LOCK_UN)  # the probe's pass ends
                return 0.0

            import background_probe  # noqa: PLC0415

            with mock.patch.object(background_probe, "stop", side_effect=stop), \
                    mock.patch.dict(os.environ, {gate_lock.GATE_LOCK_HELD: ""}), \
                    gate_lock.holding(root, "test") as env:
                self.assertEqual(gate_lock.holder_kind(gate_lock.holder(root)), "test")
                self.assertEqual(env[gate_lock.GATE_LOCK_HELD], "1")
        self.assertEqual(asked, [root])

    def test_the_stop_hook_waits_behind_short_runs_only(self):
        for kind in ("spot", "test", "probe"):
            self.assertNotIn(kind, gate_lock.LONG)
        for kind in ("check", "check-pr", "revert-red", "gate", ""):
            self.assertIn(kind, gate_lock.LONG)


class Wiring(unittest.TestCase):
    def test_the_gates_take_the_lock(self):
        """make check, make spot, revert-red's targets and check-pr's snapshot each hold it (F17; spot,
        the owner, 2026-11-04)."""
        makefile = (SCRIPTS.parent / "Makefile").read_text()
        for target, kind in (("check", "check"), ("spot", "spot")):
            recipe = makefile.split(f"\n{target}: ", 1)[1].split("\n\n", 1)[0]
            self.assertIn(f"$(GATE_LOCK) --as {kind} ", recipe)
        for line in makefile.splitlines():
            if "scripts/check_revert_red.py" in line and line.startswith("\t"):
                self.assertIn("$(GATE_LOCK)", line, line)
        self.assertIn("gate_lock.holding(", (SCRIPTS / "check_pr_snapshot.py").read_text())


if __name__ == "__main__":
    unittest.main()
