# SPDX-License-Identifier: GPL-3.0-or-later
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

    def test_nothing_to_check_before_a_first_commit(self):
        repo = Repo({"x_test.go": 'func TestA(t *testing.T) {}\n'})
        with repo.cwd():
            repo.git("add", "x_test.go")
            self.assertEqual(tw.main(["--message-file", self.message("first\n")]), 0)


if __name__ == "__main__":
    unittest.main()
