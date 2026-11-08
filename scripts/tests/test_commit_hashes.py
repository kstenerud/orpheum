# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for commit_hashes.py and the two gates that use it: a commit message or a
# ABOUTME: document citing a commit of this repository by its hash fails; other hex passes.
import contextlib
import os
import shutil
import subprocess
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
import check_docs as cd  # noqa: E402
import commit_hashes as ch  # noqa: E402
import lint_commits as lc  # noqa: E402
from gitrepo import git_repo  # noqa: E402

ENV = {**os.environ, "GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@t",
       "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@t"}


@contextlib.contextmanager
def repo_with_a_commit():
    """A throwaway repository with one commit, entered; yields that commit's 8-digit hash."""
    root = git_repo({"a.txt": "a\n"})
    cwd = Path.cwd()
    try:
        os.chdir(root)
        subprocess.run(["git", "add", "-A"], check=True, env=ENV)
        subprocess.run(["git", "commit", "-qm", "feat(a): the first thing"], check=True, env=ENV)
        yield subprocess.run(["git", "rev-parse", "--short=8", "HEAD"], capture_output=True,
                             text=True, check=True).stdout.strip()
    finally:
        os.chdir(cwd)
        shutil.rmtree(root)


class Candidates(unittest.TestCase):
    def test_a_lone_hash_is_a_candidate_and_other_hex_is_not(self):
        text = ("built at 1a2b3c4d (and `5e6f7a8b9`); colour #ffaa0080, image app-9cbe6f0d, "
                "digest sha256:0123abcd0123, the number 12345678, the word deadbeef, a URL "
                "https://x.org/c/0a1b2c3d, a UUID 0a1b2c3d-1111-2222-3333-444455556666")
        self.assertEqual(ch.candidates(text), ["1a2b3c4d", "5e6f7a8b9"])

    def test_only_a_token_naming_a_commit_here_is_cited(self):
        cited = ch.cited_commits("upstream at 1a2b3c4d, ours at 5e6f7a8b",
                                 resolve=lambda tokens: {"5e6f7a8b"} & set(tokens))
        self.assertEqual(cited, ["5e6f7a8b"])


class InARepository(unittest.TestCase):
    def test_the_commit_lint_refuses_a_cited_hash_and_names_the_subject(self):
        with repo_with_a_commit() as short:
            problems = lc.lint([lc.Commit("0" * 40, "docs: x", f"Fixes what {short} broke.")])
            self.assertEqual([p.rule for p in problems], ["commit-hash"])
            self.assertIn('"feat(a): the first thing"', problems[0].detail)
            self.assertEqual(lc.lint([lc.Commit("0" * 40, "docs: x", "Fixes what 1a2b3c4e broke.")]), [])

    def test_the_line_git_revert_writes_is_allowed(self):
        with repo_with_a_commit() as short:
            body = f"This reverts commit {short}.\n\nIt broke the scan."
            self.assertEqual(lc.lint([lc.Commit("0" * 40, 'Revert "feat(a): the first thing"', body)]), [])

    def test_a_document_citing_a_commit_by_hash_fails(self):
        with repo_with_a_commit() as short:
            files = {"docs/a.md": f"# A\n\nBuilt in {short}.\n", "docs/b.md": "# B\n\nUpstream at 1a2b3c4e.\n"}
            errors = cd.cited_hash_errors(sorted(files), files)
            self.assertEqual(len(errors), 1, errors)
            self.assertTrue(errors[0].startswith(f"docs/a.md:3: cites commit {short}"), errors)
            self.assertIn('"feat(a): the first thing"', errors[0])


if __name__ == "__main__":
    unittest.main()
