# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: A throwaway git repository for the gate tests to run the gates in, as the working
# ABOUTME: directory, with commits made as a fixed author.
import contextlib
import io
import os
import subprocess
import sys
import tempfile
from pathlib import Path

# As in test_gates.py: a hook's GIT_DIR would point every git command below at the real repository.
for _variable in ("GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR", "GIT_PREFIX",
                  "GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES"):
    os.environ.pop(_variable, None)

GIT_ENV = {"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@t", "GIT_COMMITTER_NAME": "t",
           "GIT_COMMITTER_EMAIL": "t@t"}


class Repo:
    """A throwaway repository the gates run in, as the working directory."""

    def __init__(self, files: dict[str, str]):
        self.root = Path(tempfile.mkdtemp(prefix="branch-gate-test-"))
        subprocess.run(["git", "init", "-q"], cwd=self.root, check=True)
        self.write(files)

    def write(self, files: dict[str, str | None]) -> None:
        for path, text in files.items():
            full = self.root / path
            if text is None:
                full.unlink()
                continue
            full.parent.mkdir(parents=True, exist_ok=True)
            full.write_text(text)

    def commit(self, message: str, files: dict[str, str | None] | None = None) -> str:
        if files:
            self.write(files)
        env = {**os.environ, **GIT_ENV}
        subprocess.run(["git", "add", "-A"], cwd=self.root, check=True, env=env)
        subprocess.run(["git", "commit", "-qm", message], cwd=self.root, check=True, env=env)
        return self.git("rev-parse", "HEAD")

    def git(self, *args: str) -> str:
        return subprocess.run(["git", *args], cwd=self.root, capture_output=True, text=True,
                              check=True).stdout.strip()

    @contextlib.contextmanager
    def cwd(self):
        saved = os.getcwd()
        os.chdir(self.root)
        try:
            with contextlib.redirect_stdout(io.StringIO()) as out:
                yield out
        finally:
            os.chdir(saved)


@contextlib.contextmanager
def mock_argv(argv: list[str]):
    saved = sys.argv
    sys.argv = argv
    try:
        yield
    finally:
        sys.argv = saved


def git_repo(files: dict[str, str]) -> Path:
    """A throwaway git repository holding `files`, so the gates' `git ls-files` works."""
    root = Path(tempfile.mkdtemp(prefix="gate-test-"))
    for path, text in files.items():
        full = root / path
        full.parent.mkdir(parents=True, exist_ok=True)
        full.write_text(text)
    subprocess.run(["git", "init", "-q"], cwd=root, check=True)
    return root
