# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for subagent cleanup: a subagent's commands carry its id and no stdin, a stopped agent's
# ABOUTME: processes end, the sweep finds what was left, and a merged worktree goes while any other stays.
import io
import json
import os
import subprocess
import sys
import tempfile
import time
import unittest
from pathlib import Path
from unittest import mock

ROOT = Path(__file__).resolve().parents[2]
HOOKS = ROOT / ".claude" / "hooks"
sys.path.insert(0, str(HOOKS))
sys.path.insert(0, str(ROOT / "scripts"))
import agents  # noqa: E402
import worktrees  # noqa: E402

sys.path.insert(0, str(Path(__file__).resolve().parent))
from test_hooks import HERMETIC, HOOKS_INSTALLED  # noqa: E402


# This run's own agent ids: the kill scans the whole machine, and two gates running these tests at
# once, an agent's and the lead's, ended each other's processes under the same fixed ids (2026-10-27).
RUN = f"t{os.getpid()}"


def ident(name: str) -> str:
    return f"{name}-{RUN}"


def tagged_sleep(agent_id: str) -> subprocess.Popen:
    """A process an agent's command left behind: a sleep carrying its tag, returned once /proc shows
    the tag. Until the child execs, it carries the test's own environment: under the gate's load a
    scan made at once missed it, and the test waited in vain (2026-10-27)."""
    process = subprocess.Popen(["sleep", "60"], env={**os.environ, agents.TAG: agent_id})
    deadline = time.monotonic() + 10
    while agents.tagged_processes().get(process.pid) != agent_id:
        if time.monotonic() > deadline:
            process.kill()
            process.wait()
            raise AssertionError(f"the tagged sleep {process.pid} never showed its tag")
        time.sleep(0.01)
    return process


class StateDir(unittest.TestCase):
    """Each test keeps the agents' records in a directory of its own, never the repository's."""

    def setUp(self) -> None:
        self.state = Path(tempfile.mkdtemp())
        patcher = mock.patch.dict(os.environ, {agents.STATE_DIR_VARIABLE: str(self.state)})
        patcher.start()
        self.addCleanup(patcher.stop)


class Wrapped(StateDir):
    def run_wrapped(self, command: str, *, is_background: bool) -> subprocess.CompletedProcess:
        # stdin is a pipe held open, as the Bash tool's is: a command reading it would wait forever.
        wrapped = agents.wrapped(command, ident("a1"), is_background=is_background)
        process = subprocess.Popen(["bash", "-c", wrapped],
                                   stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        try:
            returncode = process.wait(timeout=20)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait()
            self.fail(f"{command!r} was still running after 20 s, its stdin held open")
        finally:
            process.stdin.close()
        stdout, stderr = process.stdout.read(), process.stderr.read()
        process.stdout.close()
        process.stderr.close()
        return subprocess.CompletedProcess(process.args, returncode, stdout, stderr)

    def test_a_command_waiting_on_its_stdin_ends_at_once_and_carries_its_agent(self):
        """The review agent's `rg` with no path read its stdin for twelve hours (2026-10-27)."""
        done = self.run_wrapped('cat; echo "agent=$ORPHEUM_AGENT_ID"', is_background=False)
        self.assertEqual(done.stdout.strip(), f"agent={ident('a1')}")

    def test_a_cd_in_the_foreground_carries_on_in_the_shell(self):
        done = self.run_wrapped("cd /tmp; pwd", is_background=False)
        self.assertEqual(done.stdout.strip(), "/tmp")

    def test_a_command_in_the_background_ends_at_its_limit(self):
        with mock.patch.object(agents, "BACKGROUND_LIMIT_SECONDS", 1):
            started = time.monotonic()
            done = self.run_wrapped("sleep 30", is_background=True)
        self.assertEqual(done.returncode, 124, done.stderr)
        self.assertLess(time.monotonic() - started, 15)

    def test_an_odd_agent_id_is_never_put_into_a_command(self):
        self.assertFalse(agents.is_agent_id("a1; rm -rf /"))
        self.assertTrue(agents.is_agent_id("a0a2d6fb75afa4e95"))


class PreBashRewrites(StateDir):
    def bash(self, event: dict) -> dict:
        named = {"PATH": os.environ["PATH"], "HOME": os.environ["HOME"], "CLAUDE_PROJECT_DIR": str(ROOT),
                 agents.STATE_DIR_VARIABLE: str(self.state)}
        done = subprocess.run([sys.executable, str(HOOKS / "pre_bash.py")], input=json.dumps(event),
                              capture_output=True, text=True, check=True,
                              env=named | HERMETIC | HOOKS_INSTALLED)
        return json.loads(done.stdout) if done.stdout.strip() else {}

    def test_a_subagents_command_is_tagged_and_the_agent_recorded_as_acting(self):
        said = self.bash({"agent_id": "a1", "tool_input": {"command": "ls", "run_in_background": True}})
        rewritten = said["hookSpecificOutput"]["updatedInput"]
        self.assertTrue(rewritten["run_in_background"])
        self.assertIn("export ORPHEUM_AGENT_ID=a1; exec </dev/null; timeout", rewritten["command"])
        self.assertIn("active", agents.state("a1"))

    def test_the_sessions_own_command_is_left_as_it_is(self):
        self.assertEqual(self.bash({"tool_input": {"command": "ls"}}), {})

    def test_a_refused_command_is_refused_not_rewritten(self):
        said = self.bash({"agent_id": "a1", "tool_input": {"command": "git push --force"}})
        self.assertEqual(said["hookSpecificOutput"]["permissionDecision"], "deny")
        self.assertNotIn("updatedInput", said["hookSpecificOutput"])


class StoppedAgentsProcesses(StateDir):
    def test_an_agent_that_stops_has_every_process_it_left_ended_and_no_other(self):
        import on_subagent_stop  # noqa: PLC0415

        left, other = tagged_sleep(ident("a1")), tagged_sleep(ident("a2"))
        self.addCleanup(lambda: (other.kill(), other.wait()))
        event = {"agent_id": ident("a1"), "last_assistant_message": "Done."}
        with mock.patch.object(on_subagent_stop.hooklib, "read_event", lambda: event), \
                mock.patch.object(on_subagent_stop.sys, "stdout", io.StringIO()):
            on_subagent_stop.main()
        self.assertIsNotNone(left.wait(timeout=10))
        self.assertIsNone(other.poll())
        self.assertTrue(agents.has_stopped(ident("a1")))

    def test_the_sweep_ends_what_a_stopped_agent_left_and_spares_one_acting_again(self):
        agents.mark_stopped(ident("gone"), now=100.0)
        agents.mark_stopped(ident("back"), now=100.0)
        agents.mark_active(ident("back"), now=200.0)  # resumed by a message after it stopped
        left, working = tagged_sleep(ident("gone")), tagged_sleep(ident("back"))
        self.addCleanup(lambda: (working.kill(), working.wait()))
        self.assertIn(left.pid, agents.end_left_by_stopped())
        self.assertIsNotNone(left.wait(timeout=10))
        self.assertIsNone(working.poll())

    def test_a_record_a_week_old_is_forgotten_at_the_next_write(self):
        agents.mark_stopped(ident("old"), now=1000.0)
        old = self.state / f"{ident('old')}.json"
        os.utime(old, (1000.0, 1000.0))
        agents.mark_stopped(ident("new"), now=1000.0 + agents.RECORD_KEPT_SECONDS + 1)
        self.assertFalse(old.exists())
        self.assertTrue((self.state / f"{ident('new')}.json").exists())

    def test_an_agent_never_seen_stopping_has_not_stopped(self):
        self.assertFalse(agents.has_stopped("never"))


class OldShells(unittest.TestCase):
    def test_the_sessions_shells_running_for_hours_are_named(self):
        proc = Path(tempfile.mkdtemp())
        (proc / "uptime").write_text("100000.0 1.0\n")
        ticks = os.sysconf("SC_CLK_TCK")

        def process(pid: int, started_at: float, arguments: str, environment: str = "") -> None:
            entry = proc / str(pid)
            entry.mkdir()
            (entry / "cmdline").write_bytes(arguments.replace(" ", "\0").encode())
            fields = ["S"] + ["0"] * 18 + [str(int(started_at * ticks))]
            (entry / "stat").write_text(f"{pid} (bash) " + " ".join(fields) + "\n")
            (entry / "environ").write_bytes(environment.encode())

        shell = "/bin/bash -c source /home/x/.claude/shell-snapshots/s.sh && eval 'rg -o x'"
        process(11, 100000.0 - 5 * 3600, shell)  # five hours: named
        process(12, 100000.0 - 600, shell)  # ten minutes: a run in progress
        process(13, 100000.0 - 5 * 3600, shell, f"{agents.TAG}=a1\0")  # an agent's: the stop ends it
        process(14, 100000.0 - 5 * 3600, "/usr/bin/dockerd")  # not a shell of Claude Code's
        found = agents.old_untagged_shells(proc)
        self.assertEqual(len(found), 1, found)
        self.assertTrue(found[0].startswith("pid 11, 5.0 h:"), found)


class StandingOrdersCut(unittest.TestCase):
    def test_orders_over_the_limit_are_cut_with_a_line_saying_so(self):
        import on_prompt  # noqa: PLC0415

        orders = Path(tempfile.mkdtemp()) / "standing-orders.md"
        orders.write_text("x" * (on_prompt.MOST_CHARACTERS + 10))
        with mock.patch.object(on_prompt, "STANDING_ORDERS", orders):
            said = on_prompt.standing_orders()
        self.assertIn("[cut: the standing orders are", said)


def git(repo: Path, *args: str) -> str:
    return subprocess.run(["git", *args], cwd=repo, capture_output=True, text=True, check=True,
                          env={**os.environ, **HERMETIC, "GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@t",
                               "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@t"}).stdout


class MergedWorktrees(StateDir):
    def setUp(self) -> None:
        super().setUp()
        self.repo = Path(tempfile.mkdtemp())
        git(self.repo, "init", "-q", "-b", "main")
        (self.repo / "a.txt").write_text("a\n")
        git(self.repo, "add", "a.txt")
        git(self.repo, "commit", "-q", "-m", "a")

    def agent_worktree(self, agent_id: str, change: str) -> Path:
        """An agent's worktree with one commit, change, on its own branch."""
        path = self.repo / ".claude" / "worktrees" / f"agent-{agent_id}"
        git(self.repo, "worktree", "add", "-q", "-b", f"worktree-agent-{agent_id}", str(path))
        (path / f"{agent_id}.txt").write_text(change)
        git(path, "add", ".")
        git(path, "commit", "-q", "-m", agent_id)
        return path

    def merge_as_rebased(self, path: Path) -> None:
        """Main takes the worktree's change as a new commit, as a rebase onto it gives."""
        git(self.repo, "cherry-pick", git(path, "rev-parse", "HEAD").strip())

    def test_a_stopped_agents_merged_clean_worktree_goes_with_its_branch(self):
        path = self.agent_worktree("a1", "one\n")
        git(self.repo, "commit", "-q", "--allow-empty", "-m", "main moved on")
        self.merge_as_rebased(path)
        agents.mark_stopped("a1")
        removed, kept = worktrees.clean(self.repo)
        self.assertEqual(removed, ["agent-a1 (worktree-agent-a1)"], kept)
        self.assertFalse(path.exists())
        self.assertNotIn("worktree-agent-a1", git(self.repo, "branch"))

    def test_any_other_worktree_stays_with_what_holds_it(self):
        running = self.agent_worktree("run", "r\n")
        self.merge_as_rebased(running)
        agents.mark_active("run")
        unmerged = self.agent_worktree("new", "n\n")
        agents.mark_stopped("new")
        dirty = self.agent_worktree("dirty", "d\n")
        self.merge_as_rebased(dirty)
        (dirty / "half-written.go").write_text("package x\n")
        agents.mark_stopped("dirty")
        untracked = self.agent_worktree("old", "o\n")
        self.merge_as_rebased(untracked)
        removed, kept = worktrees.clean(self.repo)
        self.assertEqual(removed, [])
        self.assertEqual(sorted(kept), [
            "agent-dirty: 1 uncommitted file(s)",
            "agent-new: 1 commit(s) main does not hold",
            "agent-old: its agent has not stopped",
            "agent-run: its agent has not stopped",
        ])
        for path in (running, unmerged, dirty, untracked):
            self.assertTrue(path.exists())

    def test_by_hand_a_merged_worktree_of_an_agent_never_tracked_goes_too(self):
        path = self.agent_worktree("old", "o\n")
        self.merge_as_rebased(path)
        removed, _ = worktrees.clean(self.repo, also_untracked=True)
        self.assertEqual(removed, ["agent-old (worktree-agent-old)"])


if __name__ == "__main__":
    unittest.main()
