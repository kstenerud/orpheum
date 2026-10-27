# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for in_dev_image.py: a target runs at once in the development image, and otherwise
# ABOUTME: through the main checkout's ./dev in the same directory, a worktree inside it included.
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
import in_dev_image  # noqa: E402
from gitrepo import Repo  # noqa: E402

MAIN = Path("/home/karl/orpheum")


class Command(unittest.TestCase):
    def test_in_the_image_make_runs_at_once(self):
        self.assertEqual(
            in_dev_image.command(["test-acceptance-home-here", "BASE=abc"], MAIN, {"ORPHEUM_IN_DEV_IMAGE": "1"}, MAIN),
            ["make", "--no-print-directory", "test-acceptance-home-here", "BASE=abc"])

    # check-pr ran the acceptance on the host, where it failed asking to be run through ./dev by hand
    # (2026-10-23); its tree is a worktree inside the main checkout, which the main ./dev mounts.
    def test_outside_it_the_main_checkouts_dev_runs_it_in_this_directory(self):
        tree = MAIN / ".git" / "check-pr" / "tree"
        self.assertEqual(
            in_dev_image.command(["test-acceptance-here"], tree, {}, MAIN),
            [str(MAIN / "dev"), "--gate", "sh", "-c", in_dev_image.IN_DIRECTORY, "sh", str(tree), "test-acceptance-here"])

    def test_a_directory_outside_the_main_checkout_is_refused(self):
        with self.assertRaises(SystemExit):
            in_dev_image.command(["test-acceptance-here"], Path("/tmp/elsewhere"), {}, MAIN)

    def test_the_main_checkout_is_found_from_it_and_from_a_worktree_of_it(self):
        # It once compared against this file's own checkout, which failed in every agent's worktree
        # under .claude/worktrees/ (2026-10-26): a worktree's main checkout is the repository's.
        repo = Repo({"a.txt": "a\n"})
        repo.commit("chore: base")
        worktree = Path(tempfile.mkdtemp()) / "tree"
        subprocess.run(["git", "worktree", "add", "-q", "--detach", str(worktree)], cwd=repo.root, check=True)
        (repo.root / "sub").mkdir()
        self.assertEqual(in_dev_image.main_checkout(repo.root / "sub").resolve(), repo.root.resolve())
        self.assertEqual(in_dev_image.main_checkout(worktree).resolve(), repo.root.resolve())


if __name__ == "__main__":
    unittest.main()
