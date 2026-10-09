# SPDX-License-Identifier: GPL-3.0-or-later
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


# The owner's flags, which a session may be started with. A hook's decision depends on them, so
# each test sets the ones it is about and none leaks in from the session that runs the tests.
OWNER_FLAGS = ("ORPHEUM_ALLOW_GATE_EDIT", "ORPHEUM_ALLOW_DEPS")


def run_hook(name: str, event: dict, **env: str) -> dict:
    inherited = {key: value for key, value in os.environ.items() if key not in OWNER_FLAGS}
    done = subprocess.run(
        [sys.executable, str(HOOKS / name)],
        input=json.dumps(event), capture_output=True, text=True, check=True,
        env={**inherited, "CLAUDE_PROJECT_DIR": str(ROOT), **env},
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
        step = next(path for path in sorted((ROOT / "internal/store/migrations").glob("*.sql"))
                    if "baseline" not in path.name)
        result = run_hook("pre_edit.py", {"tool_name": "Edit", "tool_input": {"file_path": str(step)}})
        self.assertEqual(decision(result), "deny")
        reason = result["hookSpecificOutput"]["permissionDecisionReason"]
        self.assertIn("folds this last step into the baseline", reason)
        self.assertIn("CHECKPOINT", reason)

    def test_outside_the_project_is_not_our_business(self):
        self.assertEqual(self.edit("../elsewhere/Makefile"), "")


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


class GatePaths(unittest.TestCase):
    def test_classification(self):
        self.assertTrue(gate_paths.is_gate("web/apps/web/tsconfig.json"))
        self.assertTrue(gate_paths.is_gate("testdata/baselines/scan.json"))
        self.assertFalse(gate_paths.is_gate("web/apps/web/src/App.tsx"))
        self.assertTrue(gate_paths.is_generated("web/package-lock.json"))
        self.assertFalse(gate_paths.is_committed_migration("internal/core/x.sql", ROOT))


class PostEditAndStop(unittest.TestCase):
    def test_docs_and_code_edits_leave_different_marks(self):
        cache = ROOT / ".claude" / ".cache"
        for kind in ("docs", "code"):
            (cache / f"check-pending-{kind}").unlink(missing_ok=True)
        run_hook("post_edit.py", {"tool_input": {"file_path": str(ROOT / "docs/x.md")}})
        self.assertTrue((cache / "check-pending-docs").exists())
        self.assertFalse((cache / "check-pending-code").exists())
        # A package's README is documentation too: its edit runs no test of the code.
        run_hook("post_edit.py", {"tool_input": {"file_path": str(ROOT / "web/packages/sdk/README.md")}})
        self.assertFalse((cache / "check-pending-code").exists())
        run_hook("post_edit.py", {"tool_input": {"file_path": str(ROOT / "internal/x.go")}})
        self.assertTrue((cache / "check-pending-code").exists())
        for kind in ("docs", "code"):
            (cache / f"check-pending-{kind}").unlink(missing_ok=True)


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
