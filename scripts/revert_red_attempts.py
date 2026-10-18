# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: The runs a revert-red probe may make, and the browser tier's among them: narrowed first to
# ABOUTME: the tests that loaded the file by the browser map, then whole, each stopped at its first failure.
"""A probe's runs (check_revert_red.attempts_for), and how the browser tier's are made."""

from __future__ import annotations

import functools
import json
import re
import subprocess
import sys
from dataclasses import dataclass

import browser_map
from revert_red_parts import CSS_SOURCE

TS_SOURCE = re.compile(r"^web/(packages|apps)/[^/]+/src/.*\.tsx?$")


@dataclass(frozen=True)
class Attempt:
    """One run that may notice a reverted file. `compile_only` runs cannot fail an assertion,
    so any failure of theirs is a build failure."""

    command: list[str]
    label: str
    env: dict[str, str] | None = None
    compile_only: bool = False


# The browser-mode tests that import a file, by vitest's import graph, in Chromium and Firefox with no
# server (web/vitest.browser.config.ts): seconds, where the browser tier takes minutes, so a probe
# makes this run among its cheap ones and a red here settles it (the owner, 2026-10-18).
BROWSER_MODE_RELATED = ["npx", "--prefix", "web", "vitest", "related", "--run", "--passWithNoTests", "--root", "web",
                        "--config", "vitest.browser.config.ts"]


def browser_mode_tests(path: str) -> list[Attempt]:
    """For web source and stylesheets, the browser-mode tests that import it: what only a real
    engine shows (a layout, a stylesheet), before any run of the browser tier."""
    if not (TS_SOURCE.search(path) or CSS_SOURCE.search(path)):
        return []
    return [Attempt([*BROWSER_MODE_RELATED, path.removeprefix("web/")], ", found by the browser-mode tests")]


def browser_tier(path: str) -> list[Attempt]:
    """For web source, the browser tier, tried last: a change only a browser shows — a stylesheet
    a component imports, a layout — is invisible to vitest, which renders no CSS, and to the type
    check. It runs `make browser-tier`, which is `make test-browser` without its install step: the
    worktree's packages are links to the working tree's, which an install would replace. FFmpeg is
    the working tree's, which the worktree does not have."""
    if not TS_SOURCE.search(path):
        return []
    whole = Attempt(browser_tier_command(), ", found by the browser tier")
    # First the tests that loaded a chunk holding it (the browser map), where a red is likeliest and
    # comes soonest; only if none turns red, every test, since the map may be another commit's and a
    # verdict of green is never the map's (browser_map.tests_loading).
    loading = browser_map.tests_loading(path)
    if not loading:
        return [whole]
    return [Attempt(browser_tier_command(), ", found by the browser tests that load it",
                    {browser_map.ONLY_THESE_TESTS: json.dumps(loading)}), whole]


def is_browser_run(attempt: Attempt) -> bool:
    return "browser-tier" in attempt.command


def browser_tier_command() -> list[str]:
    """The browser tier as a probe runs it: stopped at its first failing test, since one red settles
    the probe. A test its revert hangs fails by Playwright's own timeout, two minutes, where the whole
    tier ran on past the probe's 15 and settled nothing (check-pr, 2026-10-17)."""
    return ["make", "--no-print-directory", "browser-tier", f"FFMPEG_DIR={_installed_ffmpeg()}",
            f"E2E_ARGS={FIRST_FAILURE_ENDS_IT}"]


FIRST_FAILURE_ENDS_IT = "--max-failures=1"


@functools.cache
def _installed_ffmpeg() -> str:
    """The FFmpeg directory the working tree's `make ffmpeg` installed."""
    root = subprocess.run(["git", "rev-parse", "--show-toplevel"], capture_output=True, text=True,
                          check=False).stdout.strip()
    done = subprocess.run([sys.executable, "scripts/fetch_ffmpeg.py"], cwd=root, capture_output=True,
                          text=True, check=False)
    return done.stdout.strip()
