#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Runs one step of make check only when what it reads changed since it last passed:
# ABOUTME: the web tests on a Go-only turn, or the gate scripts' tests on a web-only one.
"""Run a gate step unless its inputs are unchanged since it last passed.

    area_stamp.py STEP PATHSPEC... -- COMMAND...

The step's inputs are git pathspecs (`:(exclude)` and `:(glob)` magic included), read in the working
tree as `git add -A` would record it. The tree is taken before COMMAND runs and recorded, under
STEP, once it passes, so a file edited while it ran is not blessed with its pass. When the
recorded tree and the tree now differ in none of the pathspecs, COMMAND is skipped and says so.

The web tests, the dependency rules and the gate scripts' tests ran on every turn of make check
whatever it changed, some 35 to 50 s of CPU beside the Go steps a Go-only turn changed (the test
audit, 2026-10-08; the owner's approval of its gate changes the same day). A fresh clone, CI's,
has no record and runs everything.
"""

from __future__ import annotations

import subprocess
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from check_stamp import working_tree_id  # noqa: E402


def stamp_path(step: str) -> Path:
    """Where step's passed tree is kept: in the git common directory, shared by every worktree. A
    record names a tree by its content, not its place, so a pass holds in any worktree, and an
    agent's new one no longer runs every web test and the gate scripts' first (the caching audit's
    finding 9, 2026-10-28); per worktree, each began with none."""
    done = subprocess.run(["git", "rev-parse", "--path-format=absolute", "--git-common-dir"], capture_output=True,
                          text=True, check=True)
    return Path(done.stdout.strip()) / "orpheum-area-stamps" / step


def is_unchanged(passed_tree: str, tree: str, pathspecs: list[str]) -> bool:
    """Whether passed_tree and tree hold the same files under pathspecs."""
    done = subprocess.run(["git", "diff", "--quiet", passed_tree, tree, "--", *pathspecs], capture_output=True,
                          check=False)
    return done.returncode == 0


def main(argv: list[str]) -> int:
    if "--" not in argv or argv.index("--") < 2 or argv.index("--") == len(argv) - 1:
        print("usage: area_stamp.py STEP PATHSPEC... -- COMMAND...", file=sys.stderr)
        return 2
    split = argv.index("--")
    step, pathspecs, command = argv[0], argv[1:split], argv[split + 1:]
    tree = working_tree_id()
    stamp = stamp_path(step)
    if stamp.exists() and is_unchanged(stamp.read_text().strip(), tree, pathspecs):
        print(f"{step}: nothing it reads changed since it last passed; not run again")
        return 0
    done = subprocess.run(command, check=False)
    if done.returncode != 0:
        return done.returncode
    stamp.parent.mkdir(parents=True, exist_ok=True)
    stamp.write_text(tree + "\n")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
