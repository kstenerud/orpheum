# SPDX-License-Identifier: GPL-3.0-or-later
# ABOUTME: Says whether check-pr's browser tier has anything to check: whether a file changed since
# ABOUTME: the base is one the tier's server, node or client is built or run from.
"""Whether the browser tier can see what changed since a base.

The tier builds the server, the node and the client and plays a film through them in two
browsers, about two minutes of every check-pr (2026-10-06). A batch that changed only what none
of them is built or run from (documentation, the gates' own scripts and hooks, tests) has nothing
for it to find, and the owner chose to skip it then. Everything else runs it: the tier crosses the
whole stack, so no narrower list of "playback" paths would be safe.

Exit status 0: run the tier. 1: skip it, with why on stderr.
"""

from __future__ import annotations

import argparse
import fnmatch
import subprocess
import sys

# What the tier never builds or runs: changed alone, these cannot change what it sees.
NOT_IN_THE_TIER = (
    "docs/*",
    "scripts/*",
    ".claude/*",
    ".githooks/*",
    ".github/*",
    "*.md",
    "*_test.go",
    "*.test.ts",
    "*.test.tsx",
)


def is_outside_the_tier(path: str) -> bool:
    """Whether the tier never builds or runs path; e2e specs are the tier itself, so they are in."""
    return any(fnmatch.fnmatch(path, pattern) for pattern in NOT_IN_THE_TIER)


def changed_since(base: str) -> list[str]:
    """The files changed between base and HEAD, and in the working tree."""
    done = subprocess.run(["git", "diff", "--name-only", base], capture_output=True, text=True, check=True)
    return [line for line in done.stdout.splitlines() if line]


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--base", required=True, help="the commit check-pr compares against")
    args = ap.parse_args()
    changed = changed_since(args.base)
    inside = [path for path in changed if not is_outside_the_tier(path)]
    if inside or not changed:
        return 0
    print(f"test-browser: skipped; the {len(changed)} file(s) changed since {args.base[:12]} are none the "
          "tier builds or runs (docs, gate scripts, tests)", file=sys.stderr)
    return 1


if __name__ == "__main__":
    sys.exit(main())
