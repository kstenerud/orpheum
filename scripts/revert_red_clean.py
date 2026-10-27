# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: A probe's red counts only if the same kind of run passes in that worktree with nothing
# ABOUTME: reverted: a run that fails on a clean tree too says nothing of the revert (A45).
"""Whether a kind of probe can pass in a worktree, asked once per worktree and kind.

Vite refused every file under a `.git` directory, where revert-red's worktrees live, so every
browser-mode probe failed before a test ran, and each came back red whatever was reverted: web files
passed as covered with no test having noticed (A45, 2026-10-26). A red now stands only once the same
command, run with the file and its companions restored, has passed there. The first red of each kind
in each worktree pays for that run; the rest reuse its answer, so the check costs a run per kind, not
per file. It catches a broken environment, not a test that fails at HEAD, which make check refuses.
"""

from __future__ import annotations

import subprocess
import threading
from collections.abc import Callable

from revert_red_timings import kind_of

_CLEAN: dict[tuple[str, str], bool] = {}
# What a clean run that failed printed, its end: the failing test had to be found by reproducing the
# load by hand when it was not kept (2026-10-26, process review F8).
_FAILED_OUTPUT: dict[tuple[str, str], str] = {}
KEPT_LINES = 20
_LOCK = threading.Lock()


def probe_kind(command: list[str], label: str, env: dict[str, str] | None) -> str:
    """The kind of run a probe is, as the report names it, and the browser-mode tests apart from
    vitest's others: they run in a browser of their own, where an environment breaks on its own."""
    kind = kind_of(command, label, env)
    return f"{kind} (browser mode)" if "vitest.browser.config.ts" in command else kind


def runs_clean(tree: str, kind: str, run: Callable[[], subprocess.CompletedProcess[str]]) -> bool:
    """Whether a run of kind passes in tree with nothing reverted: run asks it, the first time for
    this tree and kind, with the probe's files already restored. A run past its timeout has not
    passed."""
    with _LOCK:
        known = _CLEAN.get((tree, kind))
    if known is not None:
        return known
    output = ""
    try:
        done = run()
        passed = done.returncode == 0
        output = (done.stdout or "") + (done.stderr or "")
    except subprocess.TimeoutExpired:
        passed = False
        output = "it ran past its timeout"
    with _LOCK:
        _CLEAN[(tree, kind)] = passed
        if not passed:
            _FAILED_OUTPUT[(tree, kind)] = "\n".join(output.strip().splitlines()[-KEPT_LINES:])
    return passed


def clean_failure(tree: str, kind: str) -> str:
    """The end of what the clean run of kind in tree printed when it failed, or "" when it passed."""
    with _LOCK:
        return _FAILED_OUTPUT.get((tree, kind), "")
