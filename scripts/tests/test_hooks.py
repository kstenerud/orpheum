# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for the Claude Code hooks: each refusal fires on the case it exists for and
# ABOUTME: stays quiet on ordinary work, run as Claude Code runs them (JSON on stdin).
import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
HOOKS = ROOT / ".claude" / "hooks"
sys.path.insert(0, str(ROOT / "scripts"))
import gate_paths  # noqa: E402


# A hook sees only what a test names, never the session's environment or this clone's own git
# config, which differs between machines: CI's fresh clone has no core.hooksPath, and the commit
# guard refused every commit there while it passed where `make setup` had run (2026-10-19). So the
# hooks are installed by GIT_CONFIG_COUNT, which git reads over the clone's config, and an owner's
# flag (ORPHEUM_ALLOW_GATE_EDIT, ORPHEUM_ALLOW_DEPS) is set only by the test it is about.
HOOKS_INSTALLED = {"GIT_CONFIG_COUNT": "1", "GIT_CONFIG_KEY_0": "core.hooksPath", "GIT_CONFIG_VALUE_0": ".githooks"}
# A check-pr running on the machine is set aside the same way: the tests of its refusals say when one is.
HERMETIC = {"TZ": "UTC", "LC_ALL": "C.UTF-8", "GIT_CONFIG_GLOBAL": "/dev/null", "GIT_CONFIG_NOSYSTEM": "1",
            "ORPHEUM_HOOKS_IGNORE_CHECK_PR": "1"}


# Where a hook keeps its agents' records while these tests run: never the repository's own
# (.claude/hooks/agents.py); the subagent tests' "a1" was written there (2026-10-27).
AGENTS_STATE = tempfile.mkdtemp()


def run_hook(name: str, event: dict, **env: str) -> dict:
    named = {"PATH": os.environ["PATH"], "HOME": os.environ["HOME"], "CLAUDE_PROJECT_DIR": str(ROOT),
             "ORPHEUM_AGENTS_STATE_DIR": AGENTS_STATE}
    done = subprocess.run(
        [sys.executable, str(HOOKS / name)],
        input=json.dumps(event), capture_output=True, text=True, check=True,
        env=named | HERMETIC | HOOKS_INSTALLED | env,
    )
    return json.loads(done.stdout) if done.stdout.strip() else {}


def decision(result: dict) -> str:
    return result.get("hookSpecificOutput", {}).get("permissionDecision", "")


class PreEdit(unittest.TestCase):
    def edit(self, rel: str, **env: str) -> str:
        return decision(run_hook("pre_edit.py", {"tool_name": "Edit",
                                                 "tool_input": {"file_path": str(ROOT / rel)}}, **env))

    def test_gates_generated_and_ordinary_files(self):
        self.assertEqual(self.edit(".golangci.yml"), "deny")
        self.assertEqual(self.edit("web/tsconfig.base.json"), "deny")
        self.assertEqual(self.edit("internal/archtest/arch_test.go"), "deny")
        self.assertEqual(self.edit("internal/store/sqlcgen/models.go"), "deny")
        self.assertEqual(self.edit("internal/store/cachegen/models.go"), "deny")
        self.assertEqual(self.edit("internal/core/api/handler.go"), "")
        self.assertEqual(self.edit("docs/contributors/README.md"), "")

    def test_the_owners_flag_covers_the_session_not_its_subagents(self):
        """A subagent inherits the session's environment; `1` allows the session alone (2026-10-26)."""
        def by_agent(value: str) -> str:
            return decision(run_hook("pre_edit.py", {"tool_name": "Edit", "agent_id": "a1", "agent_type": "general-purpose",
                                                     "tool_input": {"file_path": str(ROOT / "Makefile")}},
                                     ORPHEUM_ALLOW_GATE_EDIT=value))
        self.assertEqual(by_agent("1"), "deny")
        self.assertEqual(by_agent("agents"), "")
        self.assertEqual(self.edit("Makefile", ORPHEUM_ALLOW_GATE_EDIT="agents"), "")
        shell = run_hook("pre_bash.py", {"agent_id": "a1", "tool_input": {"command": "echo x > Makefile"}},
                         ORPHEUM_ALLOW_GATE_EDIT="1")
        self.assertEqual(decision(shell), "deny")

    def test_owner_override_opens_gates_but_not_generated_code(self):
        self.assertEqual(self.edit("Makefile", ORPHEUM_ALLOW_GATE_EDIT="1"), "")
        self.assertEqual(self.edit("api/openapi.json", ORPHEUM_ALLOW_GATE_EDIT="1"), "deny")

    def test_a_committed_step_is_refused_with_both_routes_a_schema_change_takes(self):
        """Before production runs a change folds the last step; after, it appends (D228, D272 §5)."""
        # A step HEAD tracks: the commit that adds a new step runs this test before that step is
        # committed, and a step not yet committed may still be written.
        tracked = subprocess.run(["git", "ls-tree", "--name-only", "HEAD", "internal/store/migrations/"],
                                 cwd=ROOT, capture_output=True, text=True, check=True).stdout.split()
        # The baseline alone when no step follows it (D303's fold): with a CHECKPOINT, production has
        # run it, so it is as committed as any step (D272 section 5).
        steps = [path for path in sorted(tracked) if path.endswith(".sql")]
        step = ROOT / next((path for path in steps if "baseline" not in path), steps[0])
        result = run_hook("pre_edit.py", {"tool_name": "Edit", "tool_input": {"file_path": str(step)}})
        self.assertEqual(decision(result), "deny")
        reason = result["hookSpecificOutput"]["permissionDecisionReason"]
        self.assertIn("folds this last step into the baseline", reason)
        self.assertIn("CHECKPOINT", reason)

    def test_outside_the_project_is_not_our_business(self):
        self.assertEqual(self.edit("../elsewhere/Makefile"), "")


class Worktrees(unittest.TestCase):
    """An agent's worktree, at .claude/worktrees/<name>/ inside the project, is judged from its own
    root; a repository made inside the project is not a worktree, and its files stay judged from the
    project's (hooklib.root_of; 2026-10-26)."""

    def setUp(self) -> None:
        import shutil
        import tempfile
        self.project = Path(tempfile.mkdtemp(prefix="hooks-worktrees-"))
        self.addCleanup(shutil.rmtree, self.project)
        shutil.copytree(ROOT / "scripts", self.project / "scripts", ignore=shutil.ignore_patterns("__pycache__"))
        git = ["git", "-C", str(self.project), "-c", "user.name=t", "-c", "user.email=t@t"]
        subprocess.run([*git, "init", "-q"], check=True, env=os.environ | HERMETIC)
        subprocess.run([*git, "add", "scripts"], check=True, env=os.environ | HERMETIC)
        subprocess.run([*git, "commit", "-q", "-m", "x", "--no-verify"], check=True, env=os.environ | HERMETIC)
        self.tree = self.project / ".claude" / "worktrees" / "agent"
        subprocess.run([*git, "worktree", "add", "-q", str(self.tree)], check=True, env=os.environ | HERMETIC)

    def edit(self, path: Path) -> str:
        return decision(run_hook("pre_edit.py", {"tool_name": "Edit", "tool_input": {"file_path": str(path)}},
                                 CLAUDE_PROJECT_DIR=str(self.project)))

    def test_a_worktrees_files_are_judged_from_its_own_root(self):
        self.assertEqual(self.edit(self.tree / "internal" / "x.go"), "")
        self.assertEqual(self.edit(self.tree / "scripts" / "check_docs.py"), "deny")

    def test_a_repository_made_inside_the_project_is_no_way_round_a_gate(self):
        subprocess.run(["git", "init", "-q", str(self.project / "scripts")], check=True, env=os.environ | HERMETIC)
        self.assertEqual(self.edit(self.project / "scripts" / "check_docs.py"), "deny")

    def test_a_worktrees_edit_owes_its_own_gate_not_the_projects(self):
        run_hook("post_edit.py", {"tool_input": {"file_path": str(self.tree / "internal" / "x.go")}},
                 CLAUDE_PROJECT_DIR=str(self.project))
        self.assertTrue((self.tree / ".claude" / ".cache" / "check-pending-code").exists())
        self.assertFalse((self.project / ".claude" / ".cache" / "check-pending-code").exists())

    def test_from_a_worktree_that_is_the_project_its_surroundings_are_not_our_business(self):
        """Run in an agent's worktree, the project is the worktree; the main checkout around it is outside."""
        result = run_hook("pre_edit.py", {"tool_name": "Edit",
                                          "tool_input": {"file_path": str(self.tree / ".." / "elsewhere" / "Makefile")}},
                          CLAUDE_PROJECT_DIR=str(self.tree))
        self.assertEqual(decision(result), "")

    def test_a_shell_write_into_a_worktrees_gate_is_refused(self):
        target = self.tree / "scripts" / "check_docs.py"
        result = run_hook("pre_bash.py", {"tool_input": {"command": f"echo x > {target}"}},
                          CLAUDE_PROJECT_DIR=str(self.project))
        self.assertEqual(decision(result), "deny")


class AgentWorktreeBase(unittest.TestCase):
    def test_an_agents_worktree_starts_from_the_sessions_head(self):
        """The default base is the remote's default branch, which this sandbox never fetches: agents
        started from a commit days old and hit gates main had fixed (2026-10-26). `head` is the
        session's own HEAD (code.claude.com/docs/en/worktrees)."""
        settings = json.loads((ROOT / ".claude" / "settings.json").read_text())
        self.assertEqual(settings.get("worktree", {}).get("baseRef"), "head")


class StandingOrdersAndDebts(unittest.TestCase):
    """What the session is told without being asked to remember it (process review F2, F13)."""

    def project(self):
        import shutil
        import tempfile
        project = Path(tempfile.mkdtemp(prefix="hooks-orders-"))
        self.addCleanup(shutil.rmtree, project)
        shutil.copytree(ROOT / "scripts", project / "scripts", ignore=shutil.ignore_patterns("__pycache__"))
        subprocess.run(["git", "init", "-q", str(project)], check=True, env=os.environ | HERMETIC)
        return project

    def test_every_prompt_carries_the_standing_orders_when_there_are_some(self):
        project = self.project()
        self.assertEqual(run_hook("on_prompt.py", {}, CLAUDE_PROJECT_DIR=str(project)), {})
        (project / ".claude").mkdir()
        (project / ".claude" / "standing-orders.md").write_text("1. Finish the worktrees.\n")
        said = run_hook("on_prompt.py", {}, CLAUDE_PROJECT_DIR=str(project))
        self.assertIn("Finish the worktrees.", said["hookSpecificOutput"]["additionalContext"])

    def test_a_session_sees_the_orders_again_only_when_they_change_or_a_compaction_dropped_them(self):
        project = self.project()
        (project / ".claude").mkdir()
        orders = project / ".claude" / "standing-orders.md"
        orders.write_text("1. Finish the worktrees.\n")
        transcript = project / "transcript.jsonl"
        def prompt(context: int) -> str:
            transcript.write_text(json.dumps({"message": {"usage": {"cache_read_input_tokens": context}}}) + "\n")
            said = run_hook("on_prompt.py", {"session_id": "s1", "transcript_path": str(transcript)},
                            CLAUDE_PROJECT_DIR=str(project))
            return said.get("hookSpecificOutput", {}).get("additionalContext", "")
        self.assertIn("Finish the worktrees.", prompt(50_000))
        self.assertNotIn("Finish the worktrees.", prompt(60_000))
        orders.write_text("1. Finish the worktrees.\n2. Then the gate patch.\n")
        self.assertIn("Then the gate patch.", prompt(70_000))
        self.assertNotIn("Then the gate patch.", prompt(80_000))
        self.assertIn("Then the gate patch.", prompt(30_000), "a compaction drops the orders shown before it")

    def test_a_session_past_the_hand_off_size_is_told_to_hand_off(self):
        project = self.project()
        transcript = project / "transcript.jsonl"
        transcript.write_text(json.dumps({"message": {"usage": {"input_tokens": 5, "cache_creation_input_tokens": 2000,
                                                                 "cache_read_input_tokens": 130_000}}}) + "\n")
        event = {"session_id": "s2", "transcript_path": str(transcript)}
        said = run_hook("on_prompt.py", event, CLAUDE_PROJECT_DIR=str(project))
        self.assertIn("132k tokens", said["hookSpecificOutput"]["additionalContext"])
        self.assertEqual(run_hook("on_prompt.py", event, CLAUDE_PROJECT_DIR=str(project),
                                  ORPHEUM_CONTEXT_WARN_TOKENS="200000"), {})

    def test_an_owed_full_check_pr_is_shown_at_a_sessions_start_until_paid(self):
        project = self.project()
        def debt(*args: str) -> None:
            subprocess.run([sys.executable, "scripts/check_pr_debt.py", *args], cwd=project, check=True,
                           env=os.environ | HERMETIC)
        self.assertEqual(run_hook("on_session_start.py", {}, CLAUDE_PROJECT_DIR=str(project)), {})
        debt("owe", "0123456789abcdef")
        said = run_hook("on_session_start.py", {}, CLAUDE_PROJECT_DIR=str(project))
        self.assertIn("0123456789ab", said["hookSpecificOutput"]["additionalContext"])
        debt("paid")
        self.assertEqual(run_hook("on_session_start.py", {}, CLAUDE_PROJECT_DIR=str(project)), {})

    def test_the_settings_register_both(self):
        hooks = json.loads((ROOT / ".claude" / "settings.json").read_text())["hooks"]
        self.assertIn("on_session_start.py", json.dumps(hooks["SessionStart"]))
        self.assertIn("on_prompt.py", json.dumps(hooks["UserPromptSubmit"]))
        self.assertIn("on_subagent_stop.py", json.dumps(hooks["SubagentStop"]))


class AgentLimits(unittest.TestCase):
    """The owner's usage rules applied at the launch and the call, not remembered (2026-10-31)."""

    def records(self) -> Path:
        directory = Path(tempfile.mkdtemp(prefix="hooks-agents-"))
        self.addCleanup(__import__("shutil").rmtree, directory)
        return directory

    def launch(self, records: Path, **tool_input: str) -> str:
        return decision(run_hook("pre_agent.py", {"tool_name": "Agent", "tool_input": {"prompt": "x", **tool_input}},
                                 ORPHEUM_AGENTS_STATE_DIR=str(records)))

    def test_a_launch_names_its_model_unless_it_is_a_fork(self):
        records = self.records()
        self.assertEqual(self.launch(records), "deny")
        self.assertEqual(self.launch(records, model="sonnet"), "")
        self.assertEqual(self.launch(records, subagent_type="fork"), "")

    def test_a_fourth_agent_waits_for_one_of_three_running(self):
        import time
        records = self.records()
        for agent in ("a1", "a2", "a3"):
            (records / f"{agent}.json").write_text(json.dumps({"active": time.time()}))
        self.assertEqual(self.launch(records, model="sonnet"), "deny")
        (records / "a3.json").write_text(json.dumps({"active": time.time() - 5, "stopped": time.time()}))
        self.assertEqual(self.launch(records, model="sonnet"), "")
        (records / "a2.json").write_text(json.dumps({"active": time.time() - 3600}))
        (records / "a4.json").write_text(json.dumps({"active": time.time()}))
        self.assertEqual(self.launch(records, model="sonnet"), "", "an agent silent for an hour is not running")

    def test_an_agent_past_its_briefs_size_is_told_to_report_and_stop(self):
        records = self.records()
        def call(agent: str = "a7") -> str:
            said = run_hook("on_tool_done.py", {"agent_id": agent, "tool_name": "Read"},
                            ORPHEUM_AGENTS_STATE_DIR=str(records))
            return said.get("hookSpecificOutput", {}).get("additionalContext", "")
        (records / "a7.json").write_text(json.dumps({"active": 1.0, "calls": 148}))
        self.assertEqual(call(), "")
        self.assertIn("tool call 150", call())
        self.assertEqual(call(), "")
        self.assertEqual(json.loads((records / "a7.json").read_text())["calls"], 151)
        self.assertEqual(run_hook("on_tool_done.py", {"tool_name": "Read"}, ORPHEUM_AGENTS_STATE_DIR=str(records)),
                         {}, "the session itself is not counted")
        sys.path.insert(0, str(HOOKS))
        import on_tool_done  # noqa: PLC0415
        self.assertIsNotNone(on_tool_done.reminder(175))
        self.assertIsNone(on_tool_done.reminder(176))

    def test_the_settings_register_them(self):
        hooks = json.dumps(json.loads((ROOT / ".claude" / "settings.json").read_text())["hooks"])
        self.assertIn("pre_agent.py", hooks)
        self.assertIn("on_tool_done.py", hooks)


class SubagentStop(unittest.TestCase):
    """An agent that stops to wait for its own background gate is told to wait in the foreground (F14)."""

    def verdict(self, said: str, running: bool, active: bool = False) -> dict:
        from unittest import mock
        sys.path.insert(0, str(HOOKS))
        import on_subagent_stop  # noqa: PLC0415
        import io
        out = io.StringIO()
        event = {"last_assistant_message": said, "stop_hook_active": active}
        with mock.patch.object(on_subagent_stop.hooklib, "read_event", lambda: event), \
                mock.patch.object(on_subagent_stop, "gate_running", lambda: running), \
                mock.patch.object(on_subagent_stop.sys, "stdout", out):
            on_subagent_stop.main()
        return json.loads(out.getvalue()) if out.getvalue() else {}

    def test_waiting_on_a_running_gate_is_blocked_once(self):
        said = "I'll wait for the background make check to finish."
        self.assertEqual(self.verdict(said, running=True).get("decision"), "block")
        self.assertEqual(self.verdict(said, running=False), {}, "nothing is running: let it stop")
        self.assertEqual(self.verdict("Done; all gates passed.", running=True), {}, "it is not waiting")
        self.assertEqual(self.verdict(said, running=True, active=True), {}, "told once already")


class PreBash(unittest.TestCase):
    def bash(self, command: str, **env: str) -> str:
        return decision(run_hook("pre_bash.py", {"tool_name": "Bash",
                                                 "tool_input": {"command": command}}, **env))

    def test_dependency_additions_are_refused(self):
        for cmd in ("go get golang.org/x/exp", "npm install left-pad", "npm i -D left-pad",
                    "npm --prefix web install react-query", "pnpm add x", "pip install requests"):
            self.assertEqual(self.bash(cmd), "deny", cmd)
        for cmd in ("npm ci", "npm --prefix web ci", "go mod download", "npm install", "npm run check"):
            self.assertEqual(self.bash(cmd), "", cmd)
        self.assertEqual(self.bash("go get golang.org/x/exp", ORPHEUM_ALLOW_DEPS="1"), "")

    def test_hook_bypass_and_force_push_are_refused(self):
        self.assertEqual(self.bash("git commit --no-verify -m x"), "deny")
        self.assertEqual(self.bash("git push --force origin main"), "deny")
        self.assertEqual(self.bash("git push origin work/scan"), "")

    def test_shell_writes_to_gates_are_refused(self):
        for cmd in ("sed -i 's/15/40/' .golangci.yml", "echo x >> Makefile",
                    "echo x > scripts/check_docs.py", "cp /tmp/x.py scripts/check_docs.py",
                    "mv new.yml .golangci.yml", "rm scripts/__pycache__/*.pyc",
                    "cat a | tee -a deps/go.allow", "chmod -x .githooks/pre-commit",
                    "cd internal && echo x > archtest/rules_test.go",
                    "perl -pi -e 's/a/b/' sqlc.yaml", "echo x >| Makefile",
                    "cp -t scripts/ x.py", "go build ./... 2>&1 > Makefile",
                    "echo x > $UNKNOWN_ROOT/Makefile"):
            self.assertEqual(self.bash(cmd), "deny", cmd)
        self.assertEqual(self.bash("echo x >> Makefile", ORPHEUM_ALLOW_GATE_EDIT="1"), "")

    def test_reading_or_mentioning_a_gate_is_not_writing_one(self):
        """The specimens audit-2026-09-24 section 4.1 records, and their relatives."""
        scratch = "/tmp/scratchpad"
        for cmd in ("cat Makefile", "ls scripts/ 2>/dev/null", "git config core.hooksPath",
                    "git config --get core.hooksPath",
                    'grep -rn "go get" docs/ | head',
                    f"grep -rn Clock internal/archtest > {scratch}/arch.txt 2>&1",
                    f"cd {scratch}/sq && cat > sqlc.yaml",
                    f"echo '- [x](x.md)' >> {Path.home()}/.claude/projects/p/memory/MEMORY.md",
                    f"python3 scripts/check_docs.py > {scratch}/docs.log 2>&1",
                    "git diff -- Makefile scripts/ > /yoloai/files/gates.patch",
                    f"S={scratch}; rm -rf $S/gp && git clone -q . $S/gp && cd $S/gp && wc -l Makefile",
                    "python3 - <<'EOF'\nprint('go get and --no-verify are words here')\nEOF",
                    "go test ./... 2>&1 | tee /tmp/test.log",
                    "sed -n 1,20p Makefile", "sed 's/a/b/' scripts/x.py > /tmp/x.py",
                    "cp scripts/check_docs.py /tmp/", "make check 2>&1 | tail -3"):
            self.assertEqual(self.bash(cmd), "", cmd)

    def test_hooks_path_may_be_read_but_not_repointed(self):
        for cmd in ("git config core.hooksPath /dev/null", "git config --unset core.hooksPath",
                    "git -c core.hooksPath=/dev/null commit -m x", "git config set core.hooksPath x"):
            self.assertEqual(self.bash(cmd), "deny", cmd)

    def test_commit_messages_are_text_not_writes(self):
        heredoc = ("git commit -q -F - <<'EOF'\nbuild: gates in scripts/ and the Makefile\n\n"
                   "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>\nEOF\n"
                   "python3 scripts/lint_commits.py --base HEAD~1")
        self.assertEqual(self.bash(heredoc), "")
        quoted = 'git commit -m "fix: tidy scripts/ output > noise"'
        self.assertEqual(self.bash(quoted), "")
        self.assertEqual(self.bash('git commit -m "docs: never use go get or --no-verify"'), "")


class CommitNeedsHooks(unittest.TestCase):
    def test_a_commit_is_refused_where_gits_hooks_are_not_installed(self):
        import tempfile

        sys.path.insert(0, str(HOOKS))
        import pre_bash  # noqa: PLC0415

        for cmd in ("git commit -q -m x", "make check; git commit -m x", "env A=1 git -C . commit -F -",
                    "cd /x && git commit --amend"):
            self.assertTrue(pre_bash.runs_git_commit(cmd), cmd)
        for cmd in ("git log --grep commit", "git status", "echo git commit", "grep -n commit x"):
            self.assertFalse(pre_bash.runs_git_commit(cmd), cmd)
        bare = Path(tempfile.mkdtemp())
        subprocess.run(["git", "init", "-q", str(bare)], check=True)
        self.assertFalse(pre_bash.hooks_installed(bare))
        subprocess.run(["git", "-C", str(bare), "config", "core.hooksPath", ".githooks"], check=True)
        self.assertTrue(pre_bash.hooks_installed(bare))
        # A tool that adds a worktree rewrites the path as absolute: the same hooks, still installed.
        subprocess.run(["git", "-C", str(bare), "config", "core.hooksPath", str(bare / ".githooks")], check=True)
        self.assertTrue(pre_bash.hooks_installed(bare))
        subprocess.run(["git", "-C", str(bare), "config", "core.hooksPath", "/dev/null"], check=True)
        self.assertFalse(pre_bash.hooks_installed(bare))


class NothingTestsBesideCheckPr(unittest.TestCase):
    """While check-pr runs, a command that tests, lints or commits is refused and an edit is not; and the
    Stop hook leaves its make check for later (the owner, 2026-10-20)."""

    def test_tests_lints_and_commits_are_told_apart_from_reading_and_editing(self):
        sys.path.insert(0, str(HOOKS))
        import pre_bash  # noqa: PLC0415

        for cmd in ("./dev make check-pr > /tmp/log 2>&1", "make check", "make -j6 spot", "make test-browser",
                    "./dev go test -run X ./internal/core/api/", "go vet ./...", "npm --prefix web test -- x.ts",
                    "./dev npm --prefix web run test:browser -- x", "npx vitest run", "./dev git commit -q -F -",
                    "cd /x && env A=1 ./dev make check-pr-failed", "python3 scripts/check_revert_red.py",
                    "golangci-lint run ./..."):
            self.assertTrue(pre_bash.loads_the_machine(cmd, ROOT), cmd)
        for cmd in ("git status --short", "git diff 0b9a HEAD", "git stash", "make help", "make",
                    "grep -n check Makefile", "npm --prefix web ci", "go build ./...", "tail -5 /tmp/log",
                    "ps aux | grep '[c]heck_pr_snapshot'", "echo make check"):
            self.assertFalse(pre_bash.loads_the_machine(cmd, ROOT), cmd)

    def test_a_commit_in_another_repository_is_not_this_projects(self):
        """A commit in a sibling repository, orpheum-ops beside this clone, runs none of this project's gate,
        so check-pr does not hold it up (the owner, 2026-10-27); one in this clone, from anywhere, does."""
        sys.path.insert(0, str(HOOKS))
        import pre_bash  # noqa: PLC0415

        with tempfile.TemporaryDirectory() as other:
            subprocess.run(["git", "init", "-q", other], check=True, env={**os.environ, **HERMETIC})
            for cmd in (f"cd {other} && git commit -q -m x", f"git -C {other} commit -q -m x",
                        f"cd {other}; git add . && git commit -F -"):
                self.assertFalse(pre_bash.loads_the_machine(cmd, ROOT), cmd)
                self.assertTrue(pre_bash.loads_the_machine(cmd.replace(other, str(ROOT / "web")), ROOT), cmd)
            self.assertTrue(pre_bash.loads_the_machine("git commit -q -m x", ROOT))
            self.assertFalse(pre_bash.loads_the_machine("git commit -q -m x", Path(other)))
            self.assertTrue(pre_bash.loads_the_machine(f"git -C {other}/nowhere commit -m x", ROOT))

    def test_while_check_pr_runs_a_test_is_refused_and_a_read_is_not(self):
        from unittest import mock

        sys.path.insert(0, str(HOOKS))
        import pre_bash  # noqa: PLC0415

        def verdict(command, run):
            refused = []
            event = {"tool_name": "Bash", "tool_input": {"command": command}}
            with mock.patch.object(pre_bash.hooklib, "read_event", lambda: event), \
                    mock.patch.object(pre_bash.hooklib, "check_pr_running", lambda: run), \
                    mock.patch.object(pre_bash.hooklib, "deny", refused.append):
                pre_bash.main()
            return refused

        run = {"head": "1306f7b8fe48abc", "target": "check-pr-verbose", "started": "2026-10-20 19:05:00 UTC"}
        refused = verdict("./dev make check", run)
        self.assertEqual(len(refused), 1)
        self.assertIn("check-pr is running (make check-pr-verbose on 1306f7b8fe48", refused[0])
        self.assertEqual(verdict("git diff --stat", run), [])
        self.assertEqual(verdict("./dev make check", None), [])

    def test_the_stop_hook_leaves_its_check_for_after_check_pr(self):
        import tempfile
        from unittest import mock

        sys.path.insert(0, str(HOOKS))
        import on_stop  # noqa: PLC0415

        pending = Path(tempfile.mkdtemp())
        targets = []

        def command(target, env):
            targets.append(target)
            return ["true"]

        running = {"head": "abc"}
        with mock.patch.object(on_stop, "PENDING", pending), \
                mock.patch.object(on_stop.hooklib, "read_event", lambda: {}), \
                mock.patch.object(on_stop.hooklib, "check_pr_running", lambda: running or None), \
                mock.patch.object(on_stop, "already_passed", lambda: False), \
                mock.patch.object(on_stop, "gate_held", lambda: False), \
                mock.patch.object(on_stop.worktrees, "clean", lambda root: ([], [])), \
                mock.patch.object(on_stop.run_gate, "gate_command", command):
            (pending / "check-pending-code").touch()
            on_stop.main()
            self.assertEqual(targets, [])
            self.assertTrue((pending / "check-pending-code").exists())
            running.clear()
            on_stop.main()
        self.assertEqual(targets, ["check"])
        self.assertFalse((pending / "check-pending-code").exists())


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


class GatePaths(unittest.TestCase):
    def test_classification(self):
        self.assertTrue(gate_paths.is_gate("web/apps/web/tsconfig.json"))
        self.assertTrue(gate_paths.is_gate("testdata/baselines/scan.json"))
        self.assertFalse(gate_paths.is_gate("web/apps/web/src/App.tsx"))
        self.assertTrue(gate_paths.is_generated("web/package-lock.json"))
        self.assertFalse(gate_paths.is_committed_migration("internal/core/x.sql", ROOT))


class PostEditAndStop(unittest.TestCase):
    def test_docs_and_code_edits_leave_different_marks(self):
        # In a project of its own: the repository's marks are its live hooks', which an agent's edit
        # sets and a Stop hook clears while the gate runs this test (2026-10-24), and which the test
        # would otherwise clear under them.
        import shutil
        import tempfile
        project = Path(tempfile.mkdtemp(prefix="post-edit-marks-"))
        shutil.copytree(ROOT / "scripts", project / "scripts", ignore=shutil.ignore_patterns("__pycache__"))
        cache = project / ".claude" / ".cache"

        def edited(rel: str) -> None:
            run_hook("post_edit.py", {"tool_input": {"file_path": str(project / rel)}},
                     CLAUDE_PROJECT_DIR=str(project))

        edited("docs/x.md")
        self.assertTrue((cache / "check-pending-docs").exists())
        self.assertFalse((cache / "check-pending-code").exists())
        # A package's README is documentation too: its edit runs no test of the code.
        edited("web/packages/sdk/README.md")
        self.assertFalse((cache / "check-pending-code").exists())
        edited("internal/x.go")
        self.assertTrue((cache / "check-pending-code").exists())
        shutil.rmtree(project)


class PostEditFormats(unittest.TestCase):
    def test_an_edited_go_file_is_formatted_where_it_is(self):
        # A formatting slip is fixed after the edit, not found by the gate (the owner, 2026-10-20).
        import shutil
        import tempfile
        project = Path(tempfile.mkdtemp(prefix="post-edit-format-"))
        shutil.copytree(ROOT / "scripts", project / "scripts", ignore=shutil.ignore_patterns("__pycache__"))
        tools = project / "bin"
        tools.mkdir()
        (tools / "golangci-lint").write_text('#!/bin/sh\nshift\nfor f in "$@"; do echo "// formatted" >> "$f"; done\n')
        (tools / "golangci-lint").chmod(0o755)
        edited = project / "internal" / "x.go"
        edited.parent.mkdir(parents=True)
        edited.write_text("package x\n")
        run_hook("post_edit.py", {"tool_input": {"file_path": str(edited)}},
                 CLAUDE_PROJECT_DIR=str(project), PATH=f"{tools}:{os.environ['PATH']}")
        self.assertIn("// formatted", edited.read_text())


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


if __name__ == "__main__":
    unittest.main()
