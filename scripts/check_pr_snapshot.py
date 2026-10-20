#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: check-pr's own tree: HEAD checked out in a worktree kept in the git directory, where
# ABOUTME: check-pr runs holding a lock the hooks read, while the working tree stays free to edit.
"""Runs a check-pr target in check-pr's own tree, holding the lock that says one is running.

check-pr reads the tree for minutes, and an edit made meanwhile was gated partly or not at all; a
written rule against editing failed again and again, and a test run beside it (the Stop hook's
`make check`, a commit's) made both slow and flaky (the owner, 2026-10-20: "You're nondeterministic,
so trying to remember will lead to failures"). So check-pr runs on a copy:

1. It takes an exclusive `flock` on `<git-common-dir>/check-pr/lock` and holds it to the end; its
   children inherit it, and the kernel lets it go when the last of them dies, so it is never left
   stale. Held, the hooks refuse a command that starts tests or a commit (pre_bash.py), and the Stop
   hook leaves its `make check` for later (on_stop.py); an edit is let through, as it touches only
   the working tree.
2. It sets the worktree `<git-common-dir>/check-pr/tree` to HEAD (revert_red_trees.set_kept_tree:
   kept, so its Go and type-check caches are found again), and links into it what the working tree
   has and a fresh checkout lacks: the fetched tools and the browser tier's map and films (SHARED).
3. It runs `make -C <tree> <target>` with ORPHEUM_CHECK_PR_TREE set, which the Makefile reads as
   "already in check-pr's tree".

    python3 scripts/check_pr_snapshot.py check-pr-verbose [BASE=<commit>]
"""

from __future__ import annotations

import fcntl
import json
import os
import subprocess
import sys
import time
from pathlib import Path

import revert_red_trees

# Where every suite keeps its timings (web/e2e/test-timings.ts), made in the working tree if it is not
# there yet, so check-pr's runs, the slow ones, are kept where the working tree reads them.
TIMINGS = ".dev/test-timings"
# Set, to the tree's path, in the environment of the make that runs in check-pr's tree.
IN_TREE = "ORPHEUM_CHECK_PR_TREE"
# What a fresh checkout lacks and check-pr's steps read, or write for the working tree to read (every
# suite's timings): ignored, so the tree keeps the links.
SHARED = (".tools", ".dev/browser-map", ".dev/browser-films", TIMINGS)
# The make variables of the make that ran this one, which name its jobserver's descriptors.
MAKE_STATE = ("MAKEFLAGS", "MFLAGS", "MAKELEVEL", "MAKEOVERRIDES")


def _git(*args: str, cwd: Path | None = None) -> str:
    done = subprocess.run(["git", *args], capture_output=True, text=True, check=False, cwd=cwd)
    return done.stdout.strip() if done.returncode == 0 else ""


def folder_of(root: Path) -> Path | None:
    """`<git-common-dir>/check-pr` of root's repository; None outside one."""
    common = _git("rev-parse", "--path-format=absolute", "--git-common-dir", cwd=root)
    return Path(common) / "check-pr" if common else None


def running(root: Path) -> dict[str, str] | None:
    """The check-pr run that holds the lock of root's repository, as it recorded itself ({} when its
    record is unreadable); None when none does. None too when root is a tree kept in the git
    directory, check-pr's own or a revert-red probe's: the gate is what runs there."""
    folder = folder_of(root)
    if folder is None or root.resolve().is_relative_to(folder.parent.resolve()):
        return None
    lock = folder / "lock"
    if not lock.exists():
        return None
    with open(lock, "a", encoding="utf-8") as held:
        try:
            fcntl.flock(held, fcntl.LOCK_SH | fcntl.LOCK_NB)
        except BlockingIOError:
            try:
                return json.loads((folder / "running").read_text(encoding="utf-8"))
            except (OSError, ValueError):
                return {}
        fcntl.flock(held, fcntl.LOCK_UN)
    return None


def describe(run: dict[str, str]) -> str:
    """One line naming a running check-pr, for a hook's refusal."""
    head = run.get("head", "")[:12] or "an unknown commit"
    return f"make {run.get('target', 'check-pr')} on {head}, started {run.get('started', 'at an unknown time')}"


def link_shared(source: Path, tree: Path) -> None:
    """Links each of SHARED that the working tree has into the tree, where it is not already."""
    for name in SHARED:
        have, link = source / name, tree / name
        if not have.exists() or link.is_symlink() or link.exists():
            continue
        link.parent.mkdir(parents=True, exist_ok=True)
        link.symlink_to(have.resolve())


def main(argv: list[str]) -> int:
    if not argv:
        print("usage: check_pr_snapshot.py <make target> [VARIABLE=value...]", file=sys.stderr)
        return 2
    source = Path(_git("rev-parse", "--show-toplevel"))
    folder = folder_of(source)
    head = _git("rev-parse", "HEAD")
    if folder is None or not head:
        print("check_pr_snapshot.py: not in a git repository with a commit", file=sys.stderr)
        return 2
    folder.mkdir(parents=True, exist_ok=True)
    with open(folder / "lock", "a", encoding="utf-8") as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            print("Another check-pr is running; waiting for it to finish.", flush=True)
            fcntl.flock(lock, fcntl.LOCK_EX)
        record = {"head": head, "target": argv[0], "started": time.strftime("%Y-%m-%d %H:%M:%S %Z")}
        (folder / "running").write_text(json.dumps(record) + "\n", encoding="utf-8")
        tree = folder / "tree"
        began = time.monotonic()
        revert_red_trees.set_kept_tree(source, tree, head)
        (source / TIMINGS).mkdir(parents=True, exist_ok=True)
        link_shared(source, tree)
        print(f"check-pr: checking {head[:12]} in its own tree, {tree}, set up in "
              f"{time.monotonic() - began:.0f} s (not in check-pr's own time); the working tree is free to "
              "edit, and tests and commits wait for it (the hooks refuse them until it ends).", flush=True)
        env = {name: value for name, value in os.environ.items() if name not in MAKE_STATE}
        env[IN_TREE] = str(tree)
        done = subprocess.run(["make", "--no-print-directory", "-C", str(tree), *argv], env=env,
                              pass_fds=(lock.fileno(),), check=False)
        return done.returncode


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
