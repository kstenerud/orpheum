#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Records the working tree `make check` passed on, so a commit of exactly that tree
# ABOUTME: need not run the gate a second time (the Stop hook has just run it).
"""The tree `make check` last passed on.

`begin` notes the working tree as the gate starts, and `write` records it as passed only if the tree
is still that one when the gate ends: an edit made while the gate ran (a subagent in the same tree, an
editor's save) was recorded as passed, and a commit of it skipped the gate (2026-10-26). The tree is
the one git would commit with `git add -A` (tracked files and untracked ones not ignored). `fresh` succeeds only when the tree being committed, the index, is
that same tree and the working tree has not changed since: the gate already passed on exactly
what the commit records. Anything else, a partial commit or an edit since, runs the gate. `matches`
succeeds when the working tree is the one the gate passed on, whatever the index: the Stop hook's
question, so a turn that committed a checked tree does not run the gate on it again.
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


# The mark a design cycle leaves while the owner iterates on a look (standards/design-cycle.md): the
# Stop hook runs no gate while it is there, and make check refuses to (the owner, 2026-10-30: "make
# check should fail if that end-of-turn blocker file is present").
DEFERRED_MARK = ".claude/.cache/check-deferred"


def deferred_path() -> Path:
    """Where the working tree's design-cycle mark is, at its top."""
    return Path(_git("rev-parse", "--show-toplevel")) / DEFERRED_MARK


def stamp_path() -> Path:
    return Path(_git("rev-parse", "--git-path", "orpheum-check-passed"))


def begun_path() -> Path:
    return Path(_git("rev-parse", "--git-path", "orpheum-check-begun"))


def passed_tree() -> str | None:
    stamp = stamp_path()
    return stamp.read_text().strip() if stamp.exists() else None


def main() -> int:
    action = sys.argv[1] if len(sys.argv) > 1 else ""
    if action == "begin":
        if deferred_path().exists():
            print(f"check_stamp: a design cycle is still deferring the gate ({DEFERRED_MARK}); a make check ends "
                  "the cycle with every safeguard back, so remove the mark first (standards/design-cycle.md)")
            return 1
        begun_path().write_text(working_tree_id() + "\n")
        return 0
    if action == "write":
        begun = begun_path()
        started = begun.read_text().strip() if begun.exists() else None
        begun.unlink(missing_ok=True)
        now = working_tree_id()
        if started != now:
            stamp_path().unlink(missing_ok=True)
            print("check_stamp: the tree changed while the gate ran, so it is not recorded as passed")
            return 0
        stamp_path().write_text(now + "\n")
        return 0
    if action == "fresh":
        passed = passed_tree()
        committing = _git("write-tree")  # the index being committed, as the hook's git sets it
        return 0 if passed is not None and passed == working_tree_id() == committing else 1
    if action == "matches":
        passed = passed_tree()
        return 0 if passed is not None and passed == working_tree_id() else 1
    print("usage: check_stamp.py begin|write|fresh|matches", file=sys.stderr)
    return 2


if __name__ == "__main__":
    sys.exit(main())
