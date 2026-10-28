# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for seed_worktree.py: a new linked worktree gets the main checkout's type builds once;
# ABOUTME: the main checkout and a worktree with its own are left alone.
import os
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parent.parent / "seed_worktree.py"
HERMETIC = {"GIT_CONFIG_GLOBAL": "/dev/null", "GIT_CONFIG_NOSYSTEM": "1", "GIT_AUTHOR_NAME": "t",
            "GIT_AUTHOR_EMAIL": "t@t", "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@t"}


class SeedWorktree(unittest.TestCase):
    def setUp(self):
        self.temp = Path(tempfile.mkdtemp(prefix="seed-worktree-"))
        self.addCleanup(shutil.rmtree, self.temp, ignore_errors=True)
        self.env = os.environ | HERMETIC
        self.main = self.temp / "main"
        (self.main / "web" / "apps" / "web" / ".tsbuild").mkdir(parents=True)
        (self.main / "web" / "apps" / "web" / ".tsbuild" / "info").write_text("built")
        (self.main / "web" / "readme").write_text("x")
        self.git(self.main, "init", "-q")
        (self.main / ".gitignore").write_text(".tsbuild\n")
        self.git(self.main, "add", "-A")
        self.git(self.main, "commit", "-qm", "first")
        self.tree = self.temp / "tree"
        self.git(self.main, "worktree", "add", "-q", str(self.tree))

    def git(self, cwd: Path, *args: str) -> None:
        subprocess.run(["git", *args], cwd=cwd, check=True, env=self.env)

    def seed(self, cwd: Path) -> str:
        return subprocess.run([sys.executable, str(SCRIPT)], cwd=cwd, capture_output=True, text=True, check=True,
                              env=self.env).stdout

    def test_a_new_worktree_gets_the_main_checkouts_type_builds(self):
        self.assertIn("1 TypeScript build", self.seed(self.tree))
        self.assertEqual((self.tree / "web" / "apps" / "web" / ".tsbuild" / "info").read_text(), "built")

    def test_a_worktree_with_its_own_and_the_main_checkout_are_left_alone(self):
        own = self.tree / "web" / "apps" / "web" / ".tsbuild"
        own.mkdir(parents=True)
        (own / "info").write_text("its own")
        self.assertEqual(self.seed(self.tree), "")
        self.assertEqual((own / "info").read_text(), "its own")
        self.assertEqual(self.seed(self.main), "")


if __name__ == "__main__":
    unittest.main()
