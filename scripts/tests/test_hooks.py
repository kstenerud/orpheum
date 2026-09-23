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


def run_hook(name: str, event: dict, **env: str) -> dict:
    done = subprocess.run(
        [sys.executable, str(HOOKS / name)],
        input=json.dumps(event), capture_output=True, text=True, check=True,
        env={**os.environ, "CLAUDE_PROJECT_DIR": str(ROOT), **env},
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
        self.assertEqual(self.edit("internal/core/api/handler.go"), "")
        self.assertEqual(self.edit("docs/contributors/README.md"), "")

    def test_owner_override_opens_gates_but_not_generated_code(self):
        self.assertEqual(self.edit("Makefile", ORPHEUM_ALLOW_GATE_EDIT="1"), "")
        self.assertEqual(self.edit("api/openapi.json", ORPHEUM_ALLOW_GATE_EDIT="1"), "deny")

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

    def test_obvious_shell_writes_to_gates_are_refused(self):
        self.assertEqual(self.bash("sed -i 's/15/40/' .golangci.yml"), "deny")
        self.assertEqual(self.bash("echo x >> Makefile"), "deny")
        self.assertEqual(self.bash("cat Makefile"), "")
        self.assertEqual(self.bash("ls scripts/ 2>/dev/null"), "")
        self.assertEqual(self.bash("echo x > scripts/check_docs.py"), "deny")

    def test_commit_messages_are_text_not_writes(self):
        heredoc = ("git commit -q -F - <<'EOF'\nbuild: gates in scripts/ and the Makefile\n\n"
                   "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>\nEOF\n"
                   "python3 scripts/lint_commits.py --base HEAD~1")
        self.assertEqual(self.bash(heredoc), "")
        quoted = 'git commit -m "fix: tidy scripts/ output > noise"'
        self.assertEqual(self.bash(quoted), "")


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
        run_hook("post_edit.py", {"tool_input": {"file_path": str(ROOT / "internal/x.go")}})
        self.assertTrue((cache / "check-pending-code").exists())
        for kind in ("docs", "code"):
            (cache / f"check-pending-{kind}").unlink(missing_ok=True)


if __name__ == "__main__":
    unittest.main()
