# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for check-pr's own tree: it runs on HEAD however the working tree is edited, holds the
# ABOUTME: lock the hooks read while it runs, and lets it go when it ends.
import fcntl
import json
import subprocess
import sys
import unittest
from pathlib import Path

SCRIPTS = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(SCRIPTS))
sys.path.insert(0, str(Path(__file__).resolve().parent))
import check_pr_snapshot  # noqa: E402
from gitrepo import Repo  # noqa: E402

# The make target a test runs in check-pr's tree: where it ran, what it read, and what the hooks'
# question about the working tree (SOURCE) answered while it ran.
PROBE = (
    "probe:\n"
    "\t@echo \"tree=$$ORPHEUM_CHECK_PR_TREE\"; echo \"film=$$(cat film.txt)\"; "
    f"python3 -c 'import sys; sys.path.insert(0, \"{SCRIPTS}\"); import check_pr_snapshot, pathlib; "
    "print(\"running=\" + str(check_pr_snapshot.running(pathlib.Path(\"$(SOURCE)\"))))'\n"
)


class ItsOwnTree(unittest.TestCase):
    def test_it_checks_head_in_its_own_tree_holding_the_lock_and_the_working_tree_stays_editable(self):
        repo = Repo({"Makefile": PROBE, "film.txt": "committed\n", ".gitignore": ".tools/\n"})
        head = repo.commit("a film")
        repo.write({"film.txt": "edited, not committed\n", ".tools/bin/ffmpeg": "fetched\n"})
        done = subprocess.run(
            [sys.executable, str(SCRIPTS / "check_pr_snapshot.py"), "probe", f"SOURCE={repo.root}"],
            cwd=repo.root, capture_output=True, text=True, check=True,
        )
        tree = repo.root / ".git" / "check-pr" / "tree"
        self.assertIn(f"tree={tree}", done.stdout)
        # HEAD's file, not the edit made beside it.
        self.assertIn("film=committed", done.stdout)
        self.assertIn(f"'head': '{head}'", done.stdout)
        self.assertEqual((tree / ".tools").resolve(), (repo.root / ".tools").resolve())
        # Ended, it holds nothing: the hooks let tests run again.
        self.assertIsNone(check_pr_snapshot.running(repo.root))
        # The working tree's edit is untouched.
        self.assertEqual((repo.root / "film.txt").read_text(), "edited, not committed\n")

    def test_a_held_lock_is_a_run_except_in_a_tree_kept_in_the_git_directory(self):
        repo = Repo({"film.txt": "x\n"})
        repo.commit("a film")
        self.assertIsNone(check_pr_snapshot.running(repo.root))
        folder = repo.root / ".git" / "check-pr"
        folder.mkdir()
        (folder / "running").write_text(json.dumps({"head": "abc", "target": "check-pr-verbose"}))
        with open(folder / "lock", "a", encoding="utf-8") as lock:
            fcntl.flock(lock, fcntl.LOCK_EX)
            self.assertEqual(check_pr_snapshot.running(repo.root),
                             {"head": "abc", "target": "check-pr-verbose"})
            # check-pr's own tree and revert-red's probes are the gate at work, never refused.
            kept = repo.root / ".git" / "revert-red" / "wt-0"
            repo.git("worktree", "add", "--detach", str(kept), "HEAD")
            self.assertIsNone(check_pr_snapshot.running(kept))
        self.assertIsNone(check_pr_snapshot.running(repo.root))


class TheMakefileHandsItOver(unittest.TestCase):
    def test_check_pr_and_its_rerun_run_in_its_own_tree(self):
        root = SCRIPTS.parent
        for target in ("check-pr-verbose", "check-pr-failed"):
            recipe = subprocess.run(["make", "--no-print-directory", "-n", target], cwd=root,
                                    capture_output=True, text=True, check=False).stdout
            self.assertIn(f'if [ -z "$ORPHEUM_CHECK_PR_TREE" ]; then exec python3 scripts/check_pr_snapshot.py '
                          f"{target}", recipe, target)


if __name__ == "__main__":
    unittest.main()
