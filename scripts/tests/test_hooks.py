# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for the Claude Code hooks: each refusal fires on the case it exists for and
# ABOUTME: stays quiet on ordinary work, run as Claude Code runs them (JSON on stdin).
import json
import os
import subprocess
import sys
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


def run_hook(name: str, event: dict, **env: str) -> dict:
    named = {"PATH": os.environ["PATH"], "HOME": os.environ["HOME"], "CLAUDE_PROJECT_DIR": str(ROOT)}
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

    def test_a_shell_write_into_a_worktrees_gate_is_refused(self):
        target = self.tree / "scripts" / "check_docs.py"
        result = run_hook("pre_bash.py", {"tool_input": {"command": f"echo x > {target}"}},
                          CLAUDE_PROJECT_DIR=str(self.project))
        self.assertEqual(decision(result), "deny")


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
            self.assertTrue(pre_bash.loads_the_machine(cmd), cmd)
        for cmd in ("git status --short", "git diff 0b9a HEAD", "git stash", "make help", "make",
                    "grep -n check Makefile", "npm --prefix web ci", "go build ./...", "tail -5 /tmp/log",
                    "ps aux | grep '[c]heck_pr_snapshot'", "echo make check"):
            self.assertFalse(pre_bash.loads_the_machine(cmd), cmd)

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
                mock.patch.object(on_stop.run_gate, "gate_command", command):
            (pending / "check-pending-code").touch()
            on_stop.main()
            self.assertEqual(targets, [])
            self.assertTrue((pending / "check-pending-code").exists())
            running.clear()
            on_stop.main()
        self.assertEqual(targets, ["check"])
        self.assertFalse((pending / "check-pending-code").exists())


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
                mock.patch.object(on_stop.run_gate, "gate_command", command):
            (pending / "check-pending-docs").touch()
            on_stop.main()
            (pending / "check-pending-docs").touch()
            (pending / "check-pending-code").touch()
            on_stop.main()
        # Documentation alone runs no test of the code (the owner, 2026-10-08).
        self.assertEqual(targets, ["check-documentation", "check"])


if __name__ == "__main__":
    unittest.main()
