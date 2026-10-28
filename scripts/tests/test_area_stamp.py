# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for area_stamp.py: a step runs again only when what it reads changed since it
# ABOUTME: passed, and a failure records nothing.
import os
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
sys.path.insert(0, str(Path(__file__).resolve().parent))
import area_stamp  # noqa: E402
from gitrepo import Repo  # noqa: E402


class AreaStampTest(unittest.TestCase):
    def test_a_step_runs_again_only_when_what_it_reads_changed(self) -> None:
        repo = Repo({"web/a.ts": "1", "internal/x.go": "1"})
        repo.commit("first")
        ran = Path(tempfile.mkdtemp()) / "ran"  # outside the repository: its writes change nothing
        step = ["web-test", "web", "--", "sh", "-c", f"echo x >> {ran}"]
        with repo.cwd():
            self.assertEqual(area_stamp.main(step), 0)
            self.assertEqual(area_stamp.main(step), 0)  # nothing changed: not run again
            repo.write({"internal/x.go": "2"})
            self.assertEqual(area_stamp.main(step), 0)  # a Go change is not the web's
            repo.write({"web/a.ts": "2"})
            self.assertEqual(area_stamp.main(step), 0)
        self.assertEqual(ran.read_text(), "x\nx\n")

    def test_a_failure_records_nothing_and_an_exclusion_is_honoured(self) -> None:
        repo = Repo({"scripts/s.py": "1", "internal/x.go": "1"})
        repo.commit("first")
        ran = Path(tempfile.mkdtemp()) / "ran"
        failing = ["check-scripts", ".", ":(exclude,glob)**/*.go", "--", "sh", "-c", f"echo x >> {ran}; exit 3"]
        passing = ["check-scripts", ".", ":(exclude,glob)**/*.go", "--", "sh", "-c", f"echo y >> {ran}"]
        with repo.cwd():
            self.assertEqual(area_stamp.main(failing), 3)
            self.assertEqual(area_stamp.main(passing), 0)  # the failure recorded no pass
            repo.write({"internal/x.go": "2"})
            self.assertEqual(area_stamp.main(passing), 0)  # excluded: not run again
            repo.write({"scripts/s.py": "2"})
            self.assertEqual(area_stamp.main(passing), 0)
        self.assertEqual(ran.read_text().splitlines(), ["x", "y", "y"])

    def test_a_file_changed_in_the_second_the_index_was_written_is_seen(self) -> None:
        """Git re-hashes a file whose time is not before the index's own: the scratch copy of the
        index must keep that time, or a same-size edit in that second reads as unchanged."""
        repo = Repo({"web/a.ts": "1"})
        repo.commit("first")
        # Git also compares the time a file's inode changed, which no test can set: an edit made
        # a second later is caught by it, and the case would pass by luck. Only mtime counts here.
        repo.git("config", "core.trustctime", "false")
        past = 1_700_000_000
        file = repo.root / "web" / "a.ts"
        os.utime(file, (past, past))
        repo.git("update-index", "--refresh")  # the index records the file at that time
        os.utime(repo.root / ".git" / "index", (past, past))  # and was written in that second
        ran = Path(tempfile.mkdtemp()) / "ran"
        step = ["web-test", "web", "--", "sh", "-c", f"echo x >> {ran}"]
        with repo.cwd():
            self.assertEqual(area_stamp.main(step), 0)
            file.write_text("2")  # the same size, in the same second
            os.utime(file, (past, past))
            self.assertEqual(area_stamp.main(step), 0)
        self.assertEqual(ran.read_text(), "x\nx\n")

    def test_a_pass_holds_in_another_worktree_of_the_same_tree(self) -> None:
        """The caching audit's finding 9: a pass names a tree by its content, so an agent's new
        worktree, holding the same tree, does not run the step again."""
        repo = Repo({"web/a.ts": "1"})
        repo.commit("first")
        other = Path(tempfile.mkdtemp()) / "other"
        repo.git("worktree", "add", "--detach", str(other))
        ran = Path(tempfile.mkdtemp()) / "ran"
        step = ["web-test", "web", "--", "sh", "-c", f"echo x >> {ran}"]
        with repo.cwd():
            self.assertEqual(area_stamp.main(step), 0)
        here = Path.cwd()
        try:
            os.chdir(other)
            self.assertEqual(area_stamp.main(step), 0)  # the same tree: not run again
        finally:
            os.chdir(here)
        self.assertEqual(ran.read_text(), "x\n")

    def test_a_step_without_its_command_is_refused(self) -> None:
        self.assertEqual(area_stamp.main(["web-test", "web"]), 2)
        self.assertEqual(area_stamp.main(["web-test", "web", "--"]), 2)


if __name__ == "__main__":
    unittest.main()
