# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for the Stop hook's gate at a turn's end: what it runs for what changed, when it waits or
# ABOUTME: defers, and that a gate which cannot run blocks once. Split from test_hooks.py by concern.
import json
import sys
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
HOOKS = ROOT / ".claude" / "hooks"
sys.path.insert(0, str(ROOT / "scripts"))


class StopWithoutMarks(unittest.TestCase):
    def test_the_stop_hook_gates_a_tree_changed_by_no_edit_it_saw(self):
        # A shell command's write or a picked commit leaves no mark; the tree is still not the one the
        # gate passed (the owner, 2026-10-30).
        import tempfile
        from unittest import mock

        sys.path.insert(0, str(HOOKS))
        import on_stop  # noqa: PLC0415

        targets = []

        def command(target, env):
            targets.append(target)
            return ["true"]

        passed = {"value": False}
        with mock.patch.object(on_stop, "PENDING", Path(tempfile.mkdtemp())), \
                mock.patch.object(on_stop.hooklib, "read_event", lambda: {}), \
                mock.patch.object(on_stop.hooklib, "check_pr_running", lambda: None), \
                mock.patch.object(on_stop, "already_passed", lambda: passed["value"]), \
                mock.patch.object(on_stop, "changed_since_pass", lambda: ["internal/core/x.go", "docs/x.md"]), \
                mock.patch.object(on_stop, "gate_held", lambda: False), \
                mock.patch.object(on_stop.worktrees, "clean", lambda root: ([], [])), \
                mock.patch.object(on_stop.run_gate, "gate_command", command):
            on_stop.main()
            self.assertEqual(targets, ["check"])
            passed["value"] = True
            on_stop.main()
        self.assertEqual(targets, ["check"], "a tree the gate passed is not gated again")

    def test_a_tree_changed_only_in_documentation_behind_the_hooks_back_gets_the_documentation_check(self):
        # A docs commit made by a shell command left no mark and ran the whole make check (the owner,
        # 2026-10-31: "the checks done should be limited to the kind of change made").
        import tempfile
        from unittest import mock

        sys.path.insert(0, str(HOOKS))
        import on_stop  # noqa: PLC0415

        targets = []

        def command(target, env):
            targets.append(target)
            return ["true"]

        with mock.patch.object(on_stop, "PENDING", Path(tempfile.mkdtemp())), \
                mock.patch.object(on_stop.hooklib, "read_event", lambda: {}), \
                mock.patch.object(on_stop.hooklib, "check_pr_running", lambda: None), \
                mock.patch.object(on_stop, "already_passed", lambda: False), \
                mock.patch.object(on_stop, "changed_since_pass", lambda: ["docs/contributors/lead.md", "AGENTS.md"]), \
                mock.patch.object(on_stop, "gate_held", lambda: False), \
                mock.patch.object(on_stop.worktrees, "clean", lambda root: ([], [])), \
                mock.patch.object(on_stop.run_gate, "gate_command", command):
            on_stop.main()
        self.assertEqual(targets, ["check-documentation"])


class StopCannotRun(unittest.TestCase):
    """A gate that cannot run blocks the turn once, then lets it end (the owner, 2026-11-01: an agent told
    not to build the image looped, the hook re-blocking each reply)."""

    def verdict(self, exit_code: int, stop_hook_active: bool) -> dict:
        import contextlib
        import io
        import tempfile
        from unittest import mock

        sys.path.insert(0, str(HOOKS))
        import on_stop  # noqa: PLC0415

        said = io.StringIO()
        with mock.patch.object(on_stop, "PENDING", Path(tempfile.mkdtemp())), \
                mock.patch.object(on_stop.hooklib, "read_event", lambda: {"stop_hook_active": stop_hook_active}), \
                mock.patch.object(on_stop.hooklib, "check_pr_running", lambda: None), \
                mock.patch.object(on_stop, "already_passed", lambda: False), \
                mock.patch.object(on_stop, "changed_since_pass", lambda: ["internal/core/x.go"]), \
                mock.patch.object(on_stop, "gate_held", lambda: False), \
                mock.patch.object(on_stop.worktrees, "clean", lambda root: ([], [])), \
                mock.patch.object(on_stop.run_gate, "gate_command",
                                  lambda target, env: [sys.executable, "-c", f"raise SystemExit({exit_code})"]), \
                contextlib.redirect_stdout(said):
            on_stop.main()
        return json.loads(said.getvalue() or "{}")

    def test_an_image_needing_building_blocks_the_first_stop_and_warns_on_the_next(self):
        needs_build = self._needs_build()
        self.assertEqual(self.verdict(needs_build, stop_hook_active=False).get("decision"), "block")
        again = self.verdict(needs_build, stop_hook_active=True)
        self.assertNotIn("decision", again)
        self.assertIn("could not run", again.get("systemMessage", ""))

    def test_a_failing_gate_blocks_every_stop(self):
        self.assertEqual(self.verdict(2, stop_hook_active=True).get("decision"), "block")

    def test_each_run_of_the_gate_is_recorded_with_its_length_load_and_ffmpeg_jobs(self):
        # The owner, 2026-11-01: a 30-minute turn could not be explained after it.
        import tempfile
        from unittest import mock

        sys.path.insert(0, str(HOOKS))
        import on_stop  # noqa: PLC0415

        pending = Path(tempfile.mkdtemp())
        with mock.patch.object(on_stop, "PENDING", pending), \
                mock.patch.object(on_stop.hooklib, "read_event", lambda: {}), \
                mock.patch.object(on_stop.hooklib, "check_pr_running", lambda: None), \
                mock.patch.object(on_stop, "already_passed", lambda: False), \
                mock.patch.object(on_stop, "changed_since_pass", lambda: ["internal/core/x.go"]), \
                mock.patch.object(on_stop, "gate_held", lambda: False), \
                mock.patch.object(on_stop, "ffmpeg_jobs", lambda: 2), \
                mock.patch.object(on_stop, "RUNS_KEPT", 2), \
                mock.patch.object(on_stop.worktrees, "clean", lambda root: ([], [])), \
                mock.patch.object(on_stop.run_gate, "gate_command", lambda target, env: ["true"]):
            for _ in range(3):
                on_stop.main()
        runs = [json.loads(line) for line in (pending / on_stop.RUNS).read_text().splitlines()]
        self.assertEqual(len(runs), 2, "only the newest RUNS_KEPT runs are kept")
        self.assertEqual(runs[-1]["target"], "check")
        self.assertEqual(runs[-1]["exit"], 0)
        self.assertEqual(runs[-1]["ffmpegJobs"], 2)
        for field in ("start", "seconds", "loadBefore", "loadAfter"):
            self.assertIn(field, runs[-1])

    def test_the_gate_is_marked_running_while_it_runs_and_unmarked_after(self):
        # The owner, 2026-11-04: a status line shows the gate running, which the hook cannot print.
        import tempfile
        from unittest import mock

        sys.path.insert(0, str(HOOKS))
        import on_stop  # noqa: PLC0415

        pending = Path(tempfile.mkdtemp())
        seen = pending / "seen"
        mark = pending / on_stop.RUNNING
        with mock.patch.object(on_stop, "PENDING", pending), \
                mock.patch.object(on_stop.hooklib, "read_event", lambda: {}), \
                mock.patch.object(on_stop.hooklib, "check_pr_running", lambda: None), \
                mock.patch.object(on_stop, "already_passed", lambda: False), \
                mock.patch.object(on_stop, "changed_since_pass", lambda: ["internal/core/x.go"]), \
                mock.patch.object(on_stop, "gate_held", lambda: False), \
                mock.patch.object(on_stop, "ffmpeg_jobs", lambda: 0), \
                mock.patch.object(on_stop.worktrees, "clean", lambda root: ([], [])), \
                mock.patch.object(on_stop.run_gate, "gate_command",
                                  lambda target, env: ["cp", str(mark), str(seen)]):
            on_stop.main()
        during = json.loads(seen.read_text())
        self.assertEqual(during["target"], "check")
        self.assertIn("start", during)
        self.assertFalse(mark.exists(), "the mark goes once the gate ends")

    def test_the_status_segment_shows_a_running_gate_and_nothing_for_a_dead_or_absent_one(self):
        import os
        import subprocess
        import tempfile

        sys.path.insert(0, str(HOOKS))
        import gate_status  # noqa: PLC0415

        root = Path(tempfile.mkdtemp())
        self.assertEqual(gate_status.segment(root, 100.0), "")
        (root / gate_status.MARK).parent.mkdir(parents=True)
        (root / gate_status.MARK).write_text(json.dumps({"target": "check", "start": 100.0, "pid": os.getpid()}))
        self.assertEqual(gate_status.segment(root, 230.0), "make check: 2m10s")
        gone = subprocess.Popen(["true"])
        gone.wait()
        (root / gate_status.MARK).write_text(json.dumps({"target": "check", "start": 100.0, "pid": gone.pid}))
        self.assertEqual(gate_status.segment(root, 230.0), "", "a killed hook's mark shows nothing")

    def _needs_build(self) -> int:
        sys.path.insert(0, str(HOOKS))
        import on_stop  # noqa: PLC0415

        return on_stop.run_gate.load_launcher().NEEDS_BUILD


class StopDeferred(unittest.TestCase):
    def test_the_stop_hook_runs_no_gate_while_checks_are_deferred_and_keeps_its_mark(self):
        # The owner, 2026-10-30: a design iterated on is checked once, when the owner is happy.
        import tempfile
        from unittest import mock

        sys.path.insert(0, str(HOOKS))
        import on_stop  # noqa: PLC0415

        pending = Path(tempfile.mkdtemp())
        targets = []

        def command(target, env):
            targets.append(target)
            return ["true"]

        with mock.patch.object(on_stop, "PENDING", pending), \
                mock.patch.object(on_stop.hooklib, "read_event", lambda: {}), \
                mock.patch.object(on_stop.hooklib, "check_pr_running", lambda: None), \
                mock.patch.object(on_stop, "already_passed", lambda: False), \
                mock.patch.object(on_stop, "gate_held", lambda: False), \
                mock.patch.object(on_stop.worktrees, "clean", lambda root: ([], [])), \
                mock.patch.object(on_stop.run_gate, "gate_command", command):
            (pending / "check-pending-code").touch()
            (pending / on_stop.DEFERRED).touch()
            on_stop.main()
            self.assertEqual(targets, [])
            self.assertTrue((pending / "check-pending-code").exists())
            (pending / on_stop.DEFERRED).unlink()
            on_stop.main()
        self.assertEqual(targets, ["check"])


class StopRunsTheGateForWhatChanged(unittest.TestCase):
    def test_a_documentation_only_turn_runs_the_documentation_gate_and_code_the_whole(self):
        import tempfile
        from unittest import mock

        sys.path.insert(0, str(HOOKS))
        import on_stop  # noqa: PLC0415

        pending = Path(tempfile.mkdtemp())
        targets = []

        def command(target, env):
            targets.append(target)
            return ["true"]

        with mock.patch.object(on_stop, "PENDING", pending), \
                mock.patch.object(on_stop.hooklib, "read_event", lambda: {}), \
                mock.patch.object(on_stop.hooklib, "check_pr_running", lambda: None), \
                mock.patch.object(on_stop, "already_passed", lambda: False), \
                mock.patch.object(on_stop, "gate_held", lambda: False), \
                mock.patch.object(on_stop.worktrees, "clean", lambda root: ([], [])), \
                mock.patch.object(on_stop.run_gate, "gate_command", command):
            (pending / "check-pending-docs").touch()
            on_stop.main()
            (pending / "check-pending-docs").touch()
            (pending / "check-pending-code").touch()
            on_stop.main()
        # Documentation alone runs no test of the code (the owner, 2026-10-08).
        self.assertEqual(targets, ["check-documentation", "check"])

    def test_a_tree_the_gate_already_passed_on_is_not_gated_again(self):
        """A turn that committed a checked tree owes nothing more (gate-throughput G2)."""
        import tempfile
        from unittest import mock

        sys.path.insert(0, str(HOOKS))
        import on_stop  # noqa: PLC0415

        pending = Path(tempfile.mkdtemp())
        targets = []
        with mock.patch.object(on_stop, "PENDING", pending), \
                mock.patch.object(on_stop.hooklib, "read_event", lambda: {}), \
                mock.patch.object(on_stop.hooklib, "check_pr_running", lambda: None), \
                mock.patch.object(on_stop, "already_passed", lambda: True), \
                mock.patch.object(on_stop, "gate_held", lambda: False), \
                mock.patch.object(on_stop.run_gate, "gate_command", lambda target, env: targets.append(target) or ["true"]):
            (pending / "check-pending-code").touch()
            on_stop.main()
        self.assertEqual(targets, [])
        self.assertFalse((pending / "check-pending-code").exists(), "the owed gate is paid")

    def test_a_gate_running_defers_the_stop_hooks_gate_and_keeps_its_mark(self):
        """The Stop hook never queues behind another gate (scripts/gate_lock.py, F17)."""
        import tempfile
        from unittest import mock

        sys.path.insert(0, str(HOOKS))
        import on_stop  # noqa: PLC0415

        pending = Path(tempfile.mkdtemp())
        targets = []
        with mock.patch.object(on_stop, "PENDING", pending), \
                mock.patch.object(on_stop.hooklib, "read_event", lambda: {}), \
                mock.patch.object(on_stop.hooklib, "check_pr_running", lambda: None), \
                mock.patch.object(on_stop, "already_passed", lambda: False), \
                mock.patch.object(on_stop, "gate_held", lambda: True), \
                mock.patch.object(on_stop.run_gate, "gate_command", lambda target, env: targets.append(target) or ["true"]):
            (pending / "check-pending-code").touch()
            on_stop.main()
        self.assertEqual(targets, [])
        self.assertTrue((pending / "check-pending-code").exists(), "the gate is still owed")


class StopBehindTheTestLock(unittest.TestCase):
    def test_it_leaves_make_check_behind_a_long_gate_and_waits_behind_a_short_run(self):
        # Every test run takes the lock now (the owner, 2026-11-04): a turn ending during an agent's spot
        # check must still gate, waiting for it, not leave the turn ungated.
        from unittest import mock

        sys.path.insert(0, str(HOOKS))
        import gate_lock  # noqa: PLC0415
        import on_stop  # noqa: PLC0415

        for said, held in (("check-pr pid 1 since 10:00:00 UTC", True), ("check pid 1", True),
                           ("revert-red pid 1", True), ("", True), ("spot pid 1", False),
                           ("test pid 1", False), ("probe pid 1", False), (None, False)):
            with mock.patch.object(gate_lock, "holder", lambda root, said=said: said):
                self.assertEqual(on_stop.gate_held(), held, said)


if __name__ == "__main__":
    unittest.main()
