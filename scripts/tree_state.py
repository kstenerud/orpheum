# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: A fingerprint of the working tree, which check-pr takes as it starts and compares as it
# ABOUTME: ends: a file changed while it ran makes its verdict about another tree than HEAD's.
"""A fingerprint of the working tree: HEAD, every tracked file's change from it, and every untracked
file's name and content.

check-pr saves one as it starts and checks it as it ends. Its steps read the tree for minutes, and
revert-red rewrites files in it and puts them back; an edit made meanwhile was gated partly or not
at all, and could be overwritten by a probe putting its file back. The agent did that three times
in one day though a written rule forbade it, so the gate says so (the owner, 2026-10-06). Since
2026-10-20 check-pr runs in its own tree (check_pr_snapshot.py), which nothing edits, so a change
found there is a step's own: a probe that left its file reverted.
"""

from __future__ import annotations

import argparse
import hashlib
import subprocess
import sys
from pathlib import Path


def _git(*args: str) -> bytes:
    return subprocess.run(["git", *args], capture_output=True, check=True).stdout


def fingerprint() -> str:
    """The tree's fingerprint, from the repository's root."""
    digest = hashlib.sha256()
    digest.update(_git("rev-parse", "HEAD"))
    digest.update(_git("diff", "HEAD", "--binary"))
    root = Path(_git("rev-parse", "--show-toplevel").decode().strip())
    for name in sorted(_git("ls-files", "--others", "--exclude-standard", "-z").split(b"\0")):
        if name:
            path = root / name.decode()
            digest.update(name + b"\0")
            digest.update(path.read_bytes() if path.is_file() else b"")
    return digest.hexdigest()


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    group = ap.add_mutually_exclusive_group(required=True)
    group.add_argument("--save", metavar="FILE", help="write the tree's fingerprint to FILE")
    group.add_argument("--check", metavar="FILE", help="fail when the tree's fingerprint is not FILE's")
    args = ap.parse_args()
    if args.save:
        Path(args.save).write_text(fingerprint() + "\n")
        return 0
    if Path(args.check).read_text().strip() != fingerprint():
        print("check-pr: the working tree changed while it ran; its verdict is about a tree that is no "
              "longer there. Run it again with nothing editing the tree.", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
