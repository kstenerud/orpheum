#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Says whether a change is documentation alone, which runs the documentation gate and no
# ABOUTME: tests of the code: the one definition the Stop hook, the pre-commit hook and check-pr use.
"""Whether a change touches documentation alone.

Documentation is anything under docs/ and any Markdown file. A package's README holds its
invariants, which cite tests by name, so the documentation gate (`make check-documentation`) still
checks those citations, every link and decision, and the architecture map and schema.md's owners
table, which the architecture check reads; it runs no test of the code. "A documentation-only
change shouldn't require tests of the code to run" (the owner, 2026-10-08).

    documentation_only.py --staged      the index against HEAD: what a commit records
    documentation_only.py --base REV    REV against the working tree: what check-pr checks

Exit status 0: documentation alone, and something changed. 1: anything else.
"""

from __future__ import annotations

import argparse
import subprocess
import sys


def is_documentation(rel: str) -> bool:
    """Whether the path, relative to the repository's root, is documentation."""
    return rel.startswith("docs/") or rel.endswith(".md")


def is_documentation_alone(paths: list[str]) -> bool:
    """Whether paths is a change, and of documentation alone."""
    return bool(paths) and all(is_documentation(path) for path in paths)


def changed(diff_args: list[str]) -> list[str]:
    done = subprocess.run(["git", "diff", "--name-only", *diff_args], capture_output=True, text=True, check=True)
    return [line for line in done.stdout.splitlines() if line]


def main(argv: list[str]) -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    which = ap.add_mutually_exclusive_group(required=True)
    which.add_argument("--staged", action="store_true", help="the index against HEAD")
    which.add_argument("--base", help="this commit against the working tree")
    args = ap.parse_args(argv)
    paths = changed(["--cached", "HEAD"] if args.staged else [args.base])
    return 0 if is_documentation_alone(paths) else 1


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
