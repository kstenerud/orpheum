# SPDX-License-Identifier: GPL-3.0-or-later
# ABOUTME: Tests for documentation_only.py: what counts as documentation alone, staged for a commit
# ABOUTME: or changed since check-pr's base.
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
sys.path.insert(0, str(Path(__file__).resolve().parent))
import documentation_only  # noqa: E402
from gitrepo import Repo  # noqa: E402


class DocumentationOnlyTest(unittest.TestCase):
    def test_documentation_is_the_docs_tree_and_markdown_anywhere(self):
        self.assertTrue(documentation_only.is_documentation_alone(
            ["docs/contributors/test-weight.md", "web/packages/sdk/README.md", "AGENTS.md"]))
        self.assertFalse(documentation_only.is_documentation_alone(["docs/x.md", "internal/x.go"]))
        self.assertFalse(documentation_only.is_documentation_alone(["Makefile"]))
        self.assertFalse(documentation_only.is_documentation_alone([]))

    def test_a_staged_change_and_a_branch_are_read_from_git(self):
        repo = Repo({"docs/a.md": "1", "internal/x.go": "1"})
        base = repo.commit("first")
        with repo.cwd():
            repo.write({"docs/a.md": "2"})
            repo.git("add", "docs/a.md")
            self.assertEqual(documentation_only.main(["--staged"]), 0)
            repo.write({"internal/x.go": "2"})
            repo.git("add", "internal/x.go")
            self.assertEqual(documentation_only.main(["--staged"]), 1)
        repo.commit("second")
        with repo.cwd():
            self.assertEqual(documentation_only.main(["--base", base]), 1)
            self.assertEqual(documentation_only.main(["--base", repo.git("rev-parse", "HEAD")]), 1)


if __name__ == "__main__":
    unittest.main()
