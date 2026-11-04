# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for check_test_weakening.py's commit-msg mode: the commit being written is held
# ABOUTME: to the branch's rules, explained only by its own trailers (D270 §4).
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
sys.path.insert(0, str(Path(__file__).resolve().parent))
import check_test_weakening as tw  # noqa: E402
from gitrepo import Repo  # noqa: E402


class CommitBeingWritten(unittest.TestCase):
    def message(self, text: str) -> str:
        path = Path(tempfile.mkdtemp()) / "COMMIT_EDITMSG"
        path.write_text(text)
        return str(path)

    def test_a_test_removed_without_its_trailer_is_refused_and_with_it_is_let_through(self):
        two = 'func TestA(t *testing.T) { t.Fatal("a") }\nfunc TestB(t *testing.T) { t.Fatal("b") }\n'
        repo = Repo({"x_test.go": two})
        repo.commit("first")
        repo.write({"x_test.go": 'func TestA(t *testing.T) { t.Fatal("a") }\n'})
        with repo.cwd():
            repo.git("add", "x_test.go")
            self.assertEqual(tw.main(["--message-file", self.message("test: drop B\n")]), 1)
            explained = "test: drop B\n\nTest-Weakened: B tested what A does\n"
            self.assertEqual(tw.main(["--message-file", self.message(explained)]), 0)

    def test_names_the_commit_being_written_not_its_stand_ins_hash(self):
        # Agents reported the stand-in's hash as their commit, and a pick of it recorded the
        # placeholder author "commit-msg check" (2026-11-04).
        import contextlib
        import io
        repo = Repo({"x_test.go": 'func TestA(t *testing.T) { t.Fatal("a") }\n'})
        repo.commit("first")
        repo.write({"x_test.go": 'func TestA(t *testing.T) { t.Fatal("a") }\nfunc TestB(t *testing.T) {}\n'})
        with repo.cwd():
            repo.git("add", "x_test.go")
            said = io.StringIO()
            with contextlib.redirect_stdout(said):
                self.assertEqual(tw.main(["--message-file", self.message("test: add B\n")]), 0)
        self.assertIn("nothing weakened in the commit being written", said.getvalue())
        self.assertNotIn("..", said.getvalue())

    def test_an_index_locked_for_a_moment_by_another_git_is_waited_for(self):
        # An agent committing beside another found the index locked, and the check failed with
        # "not a valid object name" (2026-10-24).
        import threading
        repo = Repo({"x_test.go": 'func TestA(t *testing.T) { t.Fatal("a") }\n'})
        repo.commit("first")
        repo.write({"x_test.go": 'func TestA(t *testing.T) { t.Fatal("a") }\nfunc TestB(t *testing.T) {}\n'})
        with repo.cwd():
            repo.git("add", "x_test.go")
            lock = Path(".git") / "index.lock"
            lock.write_text("")
            threading.Timer(0.3, lock.unlink).start()
            self.assertEqual(tw.main(["--message-file", self.message("test: add B\n")]), 0)

    def test_an_index_locked_for_good_fails_the_check_saying_why(self):
        repo = Repo({"x_test.go": 'func TestA(t *testing.T) {}\n'})
        repo.commit("first")
        with repo.cwd():
            (Path(".git") / "index.lock").write_text("")
            tries = tw.WRITE_TREE_RETRY_S
            tw.WRITE_TREE_RETRY_S = 0.01
            try:
                with self.assertRaises(tw.CannotCheck) as cannot:
                    tw.commit_being_written(self.message("test: anything\n"))
            finally:
                tw.WRITE_TREE_RETRY_S = tries
                (Path(".git") / "index.lock").unlink()
            self.assertIn("index.lock", str(cannot.exception))

    def test_nothing_to_check_before_a_first_commit(self):
        repo = Repo({"x_test.go": 'func TestA(t *testing.T) {}\n'})
        with repo.cwd():
            repo.git("add", "x_test.go")
            self.assertEqual(tw.main(["--message-file", self.message("first\n")]), 0)


if __name__ == "__main__":
    unittest.main()
