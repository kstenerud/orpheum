# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Revert-red's parts, as check-pr's tiers run it: the code's cheap runs, then every browser run
# ABOUTME: last; which files each part probes, and how its probes take the browser tier.
"""Which part of revert-red probes a file, and how a probe takes the browser tier."""

from __future__ import annotations

import re

# A stylesheet only a browser renders: vitest and the type check never read it.
CSS_SOURCE = re.compile(r"^web/(packages|apps)/[^/]+/src/.*\.css$")

# How a probe takes the browser tier, its one expensive run: in turn after the cheap ones (a run by
# hand, "all"); left to the last tier, after check-pr's own browser run (the code part, "defer"), so
# no browser run holds back a cheap probe and the browser map it reads is the one just recorded; or
# that alone, the cheap runs having passed in the code part (the browser part, "only"). The owner,
# 2026-10-17: cheap tests first, and no expensive test blocks a cheap one.
BROWSER_INLINE, BROWSER_DEFERRED, BROWSER_ONLY = "inline", "defer", "only"


# check-pr's two parts of revert-red (the owner, 2026-10-15): the code's probes, a package's tests
# each, run right after the medium tier, so a change no test notices is found in minutes; every
# browser run, the stylesheets' and the code's the cheap runs left green (BROWSER_OWED), after
# check-pr's own browser run, in its last tier (the owner, 2026-10-17). "all" is both, for a run by hand.
PARTS = ("all", "code", "browser")


def is_in_part(path: str, part: str, owed_browser: frozenset[str] = frozenset()) -> bool:
    """Whether path is one of part's: every file is the code part's, a stylesheet too, whose cheap
    run is the browser-mode tests that import it (the owner, 2026-10-18); and a file the code part
    left owing its browser runs (BROWSER_OWED) is the browser part's as well."""
    if part == "all":
        return True
    if part == "browser":
        return path in owed_browser
    return True


def browser_mode(part: str) -> str:
    """How a part's probes take the browser tier (BROWSER_INLINE and the rest)."""
    return {"code": BROWSER_DEFERRED, "browser": BROWSER_ONLY}.get(part, BROWSER_INLINE)
