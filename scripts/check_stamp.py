#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Records the working tree `make check` passed on, so a commit of exactly that tree
# ABOUTME: need not run the gate a second time (the Stop hook has just run it).
"""The tree `make check` last passed on.

`write` records the working tree as git would commit it with `git add -A` (tracked files and
untracked ones not ignored). `fresh` succeeds only when the tree being committed, the index, is
that same tree and the working tree has not changed since: the gate already passed on exactly
what the commit records. Anything else, a partial commit or an edit since, runs the gate.
"""

from __future__ import annotations

import os
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path


def _git(*args: str, env: dict[str, str] | None = None) -> str:
    done = subprocess.run(["git", *args], capture_output=True, text=True, check=True, env=env)
    return done.stdout.strip()


def working_tree_id() -> str:
    """The tree `git add -A` would record, built in a scratch index so the real one is untouched."""
    index = Path(_git("rev-parse", "--git-path", "index"))
    with tempfile.TemporaryDirectory() as scratch:
        copy = Path(scratch) / "index"
        if index.exists():
            # Its stat cache spares hashing unchanged files. Its own time is kept, by which git
            # re-hashes a file changed in the second the index was written: a fresh copy's time
            # took such a file, edited to the same size, for unchanged (2026-10-08).
            shutil.copy2(index, copy)
        env = {k: v for k, v in os.environ.items() if not k.startswith("GIT_")}
        env["GIT_INDEX_FILE"] = str(copy)
        _git("add", "-A", env=env)
        return _git("write-tree", env=env)


def stamp_path() -> Path:
    return Path(_git("rev-parse", "--git-path", "orpheum-check-passed"))


def main() -> int:
    action = sys.argv[1] if len(sys.argv) > 1 else ""
    if action == "write":
        stamp_path().write_text(working_tree_id() + "\n")
        return 0
    if action == "fresh":
        stamp = stamp_path()
        if not stamp.exists():
            return 1
        passed = stamp.read_text().strip()
        committing = _git("write-tree")  # the index being committed, as the hook's git sets it
        return 0 if passed == working_tree_id() == committing else 1
    print("usage: check_stamp.py write|fresh", file=sys.stderr)
    return 2


if __name__ == "__main__":
    sys.exit(main())
