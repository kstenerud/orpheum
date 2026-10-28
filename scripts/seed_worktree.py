#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Gives a new worktree the main checkout's TypeScript build outputs, once, before its first
# ABOUTME: make check, so its type check is incremental rather than cold.
"""Seed a linked worktree's type builds from the main checkout (the caching audit's finding 9).

    seed_worktree.py

An agent's new worktree ran its first type check cold, 12.6 s against 0.13 s warm, as revert-red's
probe trees did before revert_red_trees.seed_type_builds gave them the working tree's `.tsbuild`.
The same seeding, here for a linked worktree whose web client has none: sound however stale the
copy, as `tsc -b` checks each source against the hash its build info records. In the main checkout,
or a worktree that has its own, it does nothing.
"""

from __future__ import annotations

import subprocess
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from revert_red_trees import seed_type_builds  # noqa: E402


APP_BUILD = Path("web") / "apps" / "web" / ".tsbuild"


def git_path(*args: str) -> Path:
    done = subprocess.run(["git", "rev-parse", "--path-format=absolute", *args], capture_output=True, text=True,
                          check=True)
    return Path(done.stdout.strip())


def main() -> int:
    tree = git_path("--show-toplevel")
    main_checkout = git_path("--git-common-dir").parent
    # The app's build info, the last its type check writes: a worktree with it has built its own.
    if tree.resolve() == main_checkout.resolve() or (tree / APP_BUILD).is_dir():
        return 0
    seeded = seed_type_builds(main_checkout, tree)
    if seeded:
        print(f"seed_worktree: gave this worktree the main checkout's {seeded} TypeScript build(s)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
