# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for gate_lock.py: one gate at a time on a repository, a second waits, a gate's own
# ABOUTME: sub-runs pass through, and whether one runs can be asked without waiting.
import fcntl
import os
import shutil
import signal
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


def alive(pid: int) -> bool:
    try:
        os.kill(pid, 0)
    except ProcessLookupError:
        return False
    return True


def until(condition, seconds: float = 30) -> None:
    deadline = time.monotonic() + seconds
    while not condition():
        if time.monotonic() > deadline:
            raise AssertionError("the condition never held")
        time.sleep(0.05)


class TheLockGoesWithTheRun(unittest.TestCase):
    """The lock's wargaming, 2026-11-04: a wrapper killed by the Bash tool's timeout let the lock go while
    the tests it ran went on outside it, loading the machine beside the next run."""

    def started_run(self, root: Path, *lock_args: str) -> tuple[subprocess.Popen[str], int]:
        """A run holding the lock whose command starts a grandchild and waits on it; and that grandchild."""
        started, grandchild = root / "started", root / "grandchild"
        run = run_lock(root, *lock_args, "sh", "-c", f"sleep 60 & echo $! > {grandchild}; touch {started}; wait")
        self.addCleanup(run.kill)
        wait_for(started)
        pid = int(grandchild.read_text())
        self.addCleanup(lambda: alive(pid) and os.kill(pid, 9))
        return run, pid

    def test_a_wrapper_asked_to_end_ends_its_whole_run_and_lets_the_lock_go(self):
        root = repository(self)
        run, grandchild = self.started_run(root)
        run.terminate()
        run.wait(timeout=30)
        until(lambda: not alive(grandchild))
        self.assertIsNone(gate_lock.holder(root))

    def test_a_wrapper_killed_outright_leaves_the_lock_with_its_command(self):
        root = repository(self)
        run, grandchild = self.started_run(root)
        run.kill()
        run.wait(timeout=30)
        self.assertTrue(alive(grandchild))
        self.assertIsNotNone(gate_lock.holder(root), "the command still holds the lock")
        os.kill(grandchild, 9)
        until(lambda: gate_lock.holder(root) is None)

    def test_a_run_past_its_lease_is_ended_and_says_so(self):
        root = repository(self)
        run, grandchild = self.started_run(root, "--lease", "1")
        out, _ = run.communicate(timeout=30)
        self.assertEqual(run.returncode, 124)
        self.assertIn("for its whole lease", out)
        until(lambda: not alive(grandchild))
        self.assertIsNone(gate_lock.holder(root))

    def test_a_waiting_run_ends_a_holder_past_its_lease_whose_wrapper_is_gone(self):
        root = repository(self)
        run, grandchild = self.started_run(root, "--as", "test", "--lease", "1")
        # Its command can start before the wrapper records its group; killed before that, the waiting run
        # could only name it (a race make check's load hit, 2026-11-04).
        until(lambda: "group" in (gate_lock.holder_record(root) or {}))
        run.kill()
        run.wait(timeout=30)
        second = run_lock(root, "true")
        out, _ = second.communicate(timeout=30)
        self.assertEqual(second.returncode, 0, out)
        self.assertIn("past its lease", out)
        self.assertFalse(alive(grandchild))


class TheQueue(unittest.TestCase):
    def test_a_finished_hold_records_its_kind_and_length(self):
        root = repository(self)
        self.assertEqual(run_lock(root, "--as", "spot", "true").wait(timeout=30), 0)
        self.assertEqual(list(gate_lock.hold_lengths(root)), ["spot"])

    def test_the_wait_is_the_holders_rest_and_each_waiting_runs_usual_length(self):
        root = repository(self)
        self.assertEqual(gate_lock.expected_wait(root), (0.0, ""))
        for _ in range(3):
            gate_lock.record_hold(root, "check", 300)
            gate_lock.record_hold(root, "spot", 40)
        with open(gate_lock.lock_path(root), "a+", encoding="utf-8") as lock:
            fcntl.flock(lock, fcntl.LOCK_EX)
            gate_lock.mark(lock, "check", 1800, started=time.time() - 100)
            wait, said = gate_lock.expected_wait(root)
            self.assertAlmostEqual(wait, 200, delta=5)
            self.assertIn("check has held it 2 min", said)
            with mock.patch.object(gate_lock.os, "getpid", return_value=os.getpid()), \
                    gate_lock._registered_waiting(root, "spot"):
                wait, said = gate_lock.expected_wait(root)
            self.assertAlmostEqual(wait, 240, delta=5)
            self.assertIn("1 more waits before it (spot)", said)

    def test_a_run_held_past_every_recorded_hold_is_expected_to_take_a_while_yet(self):
        self.assertEqual(gate_lock.remaining("check", 1000, {"check": [100, 200]}), gate_lock.OVERDUE_SECONDS)
        self.assertEqual(gate_lock.remaining("check", 150, {"check": [100, 200]}), 50)


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


class ARunStartedAnyWayHoldsIt(unittest.TestCase):
    """The owner, 2026-11-07: every direct way of running tests (go, npm, npx, an editor) takes the lock
    through one script: gate_lock.py --for-pid, which go_test_exec and web/e2e/test-lock.ts call."""

    def process(self, root: Path) -> tuple[subprocess.Popen[str], Path]:
        """A process standing for a run (the go command, vitest), and the file whose touch ends it."""
        release = root / "release"
        run = subprocess.Popen(["sh", "-c", f"i=0; while [ ! -e {release} ] && [ $i -lt 1500 ]; do sleep 0.02; "
                                f"i=$((i+1)); done"])
        self.addCleanup(run.kill)
        return run, release

    def test_a_hold_for_a_process_lasts_while_it_runs_and_a_second_asking_joins_it(self):
        root = repository(self)
        run, release = self.process(root)
        first = run_lock(root, "--for-pid", str(run.pid))
        said, _ = first.communicate(timeout=30)
        self.assertEqual(first.returncode, 0, said)
        keeper = int(said.strip())
        self.addCleanup(lambda: alive(keeper) and os.kill(keeper, 9))
        self.assertEqual(gate_lock.holder_kind(gate_lock.holder(root)), "test", "held as a test run")
        second = run_lock(root, "--for-pid", str(run.pid))
        joined, _ = second.communicate(timeout=10)
        self.assertEqual((second.returncode, joined.strip()), (0, ""), "the second joined the first's hold")
        other = run_lock(root, "true")
        self.assertIn("waiting for it to finish", other.stdout.readline(), "another run waits behind the hold")
        release.touch()
        run.wait(timeout=30)
        self.assertEqual(other.wait(timeout=30), 0)
        other.stdout.close()
        until(lambda: gate_lock.holder(root) is None)
        self.assertEqual(list(root.glob(".git/gate-for-*")), [], "the hold's marker went with it")

    def test_ending_the_keeper_lets_the_lock_go_while_the_process_runs_on(self):
        root = repository(self)
        run, _release = self.process(root)
        first = run_lock(root, "--for-pid", str(run.pid))
        said, _ = first.communicate(timeout=30)
        os.kill(int(said.strip()), signal.SIGTERM)
        until(lambda: gate_lock.holder(root) is None)
        self.assertIsNone(run.poll(), "the process itself runs on")

    def test_go_test_exec_holds_the_lock_for_a_test_binary_alone(self):
        root = repository(self)
        (root / "scripts").mkdir()
        shutil.copy(SCRIPTS / "gate_lock.py", root / "scripts" / "gate_lock.py")
        report = f"{sys.executable} {SCRIPTS / 'gate_lock.py'} --held || echo none"
        for name in ("pkg.test", "program"):
            binary = root / name
            binary.write_text(f"#!/bin/sh\n{report}\n")
            binary.chmod(0o755)
        env = {k: v for k, v in os.environ.items() if k != gate_lock.GATE_LOCK_HELD} | HERMETIC

        def through_exec(name: str) -> str:
            # sh stands for the go command: the wrapper's parent, whose life the hold lasts.
            return subprocess.run(["sh", "-c", f"{SCRIPTS / 'go_test_exec'} ./{name}; true"], cwd=root, env=env,
                                  capture_output=True, text=True, timeout=60, check=True).stdout

        self.assertRegex(through_exec("pkg.test"), r"^test pid \d+", "a test binary runs holding the lock")
        until(lambda: gate_lock.holder(root) is None)  # the hold ends with its go command
        self.assertEqual(through_exec("program").strip(), "none", "go run's programs take nothing")
        until(lambda: gate_lock.holder(root) is None)
        held = subprocess.run([SCRIPTS / "go_test_exec", "./pkg.test"], cwd=root, capture_output=True, text=True,
                              env=env | {gate_lock.GATE_LOCK_HELD: "1"}, timeout=30, check=True).stdout
        self.assertEqual(held.strip(), "none", "a run that holds the lock already passes straight on")

    def test_the_web_runners_take_the_lock_in_their_global_setup(self):
        web = SCRIPTS.parent / "web"
        for config in ("vitest.config.ts", "vitest.browser.config.ts"):
            self.assertIn('globalSetup: ["./e2e/test-lock.ts"]', (web / config).read_text(), config)
        self.assertIn('globalSetup: ["./test-lock.ts", ', (web / "e2e" / "playwright.config.ts").read_text())
        self.assertIn('"--for-pid"', (web / "e2e" / "test-lock.ts").read_text())

    def test_make_setup_points_go_test_at_the_lock(self):
        makefile = (SCRIPTS.parent / "Makefile").read_text()
        self.assertRegex(makefile, r"\nsetup: [^\n]*\bgo-test-lock\b")
        self.assertIn("-exec=$(CURDIR)/scripts/go_test_exec", makefile)


if __name__ == "__main__":
    unittest.main()
