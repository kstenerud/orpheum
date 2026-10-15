# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Says whether check-pr's browser tier has anything to check: whether a file changed since
# ABOUTME: the base is one the tier's server, node or client is built or run from.
"""Whether the browser tier can see what changed since a base.

The tier builds the server, the node and the client and plays a film through them in two
browsers, about two minutes of every check-pr (2026-10-06). A batch that changed only what none
of them is built or run from (documentation, the gates' own scripts and hooks, tests) has nothing
for it to find, and the owner chose to skip it then. Everything else runs it: the tier crosses the
whole stack, so no narrower list of "playback" paths would be safe.

With --acceptance it says instead whether M4's acceptance under the home profile has anything to
check: a change to what keeps two browsers in step and talking (Watch Together, voice, the WebRTC
transport and the path to the core, the player, playback's chunks and decisions). It left
check-pr's every run for those batches alone, the owner keeping the rule that a batch changing them
is tested so (2026-10-15: "after making a set of changes that would affect those areas in a
significant enough way, we test them like we are now").

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


# What M4's acceptance under home crosses that the rest of the browser tier does not dwell on: two
# browsers in step and talking, over WebRTC and the core's paths, for minutes.
ACCEPTANCE_AREAS = (
    "internal/core/together/*",
    "internal/core/rtcapi/*",
    "internal/core/turn/*",
    "internal/core/playback/*",
    "internal/core/chunks/*",
    "internal/core/decide/*",
    "web/packages/player/*",
    "web/packages/sdk/src/together*",
    "web/packages/sdk/src/rtc-*",
    "web/packages/sdk/src/voice*",
    "web/packages/sdk/src/path-keeper*",
    "web/packages/sdk/src/direct-path*",
    "web/apps/web/src/together*",
    "web/apps/web/src/watch-together*",
    "web/apps/web/src/use-together*",
    "web/apps/web/src/play*",
    "web/apps/web/src/film-player*",
    "web/e2e/acceptance*",
)


def is_in_the_acceptance(path: str) -> bool:
    """Whether path is one of what M4's acceptance under home crosses, and not a test of it."""
    return not is_outside_the_tier(path) and any(fnmatch.fnmatch(path, pattern) for pattern in ACCEPTANCE_AREAS)


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
    ap.add_argument("--acceptance", action="store_true",
                    help="whether M4's acceptance under home has anything to check, not the whole tier")
    args = ap.parse_args()
    changed = changed_since(args.base)
    if args.acceptance:
        if any(is_in_the_acceptance(path) for path in changed):
            return 0
        print(f"test-acceptance-home: skipped; none of the {len(changed)} file(s) changed since {args.base[:12]} "
              "is Watch Together, voice, the WebRTC transport, the player or playback", file=sys.stderr)
        return 1
    inside = [path for path in changed if not is_outside_the_tier(path)]
    if inside or not changed:
        return 0
    print(f"test-browser: skipped; the {len(changed)} file(s) changed since {args.base[:12]} are none the "
          "tier builds or runs (docs, gate scripts, tests)", file=sys.stderr)
    return 1


if __name__ == "__main__":
    sys.exit(main())
