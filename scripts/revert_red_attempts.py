# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: The runs a revert-red probe may make, and the browser tier's among them: narrowed first to
# ABOUTME: the tests that loaded the file by the browser map, then whole, each stopped at its first failure.
"""A probe's runs (check_revert_red.attempts_for), and how the browser tier's are made."""

from __future__ import annotations

import functools
import json
import platform
import re
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path

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


# A probe asks whether any test goes red, and the first red settles it: each run it makes stops at its
# first failure. Run to its end, a red probe of a locale file took 34 s, stopped at its first 6 s
# (check-pr's revert-red, 284 s, 2026-10-20). A green run still runs every test, so nothing it
# passes is passed on less. The browser tier's is FIRST_FAILURE_ENDS_IT.
FIRST_RED_VITEST = "--bail=1"
FIRST_RED_GO = "-failfast"

# The browser-mode tests that import a file, by vitest's import graph, in Chromium and Firefox with no
# server (web/vitest.browser.config.ts): seconds, where the browser tier takes minutes, so a probe
# makes this run among its cheap ones and a red here settles it (the owner, 2026-10-18).
BROWSER_MODE_RELATED = ["npx", "--prefix", "web", "vitest", "related", "--run", "--passWithNoTests", "--root", "web",
                        "--config", "vitest.browser.config.ts", FIRST_RED_VITEST]


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


GO_OS_SUFFIX = re.compile(r"_(darwin|windows|freebsd|openbsd|netbsd)(?:_[a-z0-9]+)?\.go$")
# A Linux file for another architecture than the machine's, compiled only there.
GO_LINUX_ARCH_SUFFIX = re.compile(r"_linux_(arm64|riscv64|arm|386|ppc64le|s390x)\.go$")
# A build line that leaves Linux out: the file is compiled only for another system.
GO_NOT_LINUX = re.compile(r"^//go:build .*!linux", re.MULTILINE)
# A build line that leaves the machine's architecture out: a Linux stub for architectures with no file of
# their own, compiled only by make check's riscv64 vet.
GO_NOT_AMD64 = re.compile(r"^//go:build .*!amd64", re.MULTILINE)


def other_platforms(path: str) -> list[Attempt]:
    """The compiles for another system or architecture that make check makes, for a Go file only they build."""
    attempts: list[Attempt] = []
    other_os = GO_OS_SUFFIX.search(path)
    if other_os:
        goos = other_os.group(1)
        attempts.append(Attempt(["go", "vet", "./..."], f", found by compiling for {goos}", {"GOOS": goos}, True))
    other_arch = GO_LINUX_ARCH_SUFFIX.search(path)
    not_linux = build_line_matches(path, GO_NOT_LINUX)
    stub = not other_os and not not_linux and build_line_matches(path, GO_NOT_AMD64)
    goarch = other_arch.group(1) if other_arch else "riscv64" if stub else None
    if goarch:
        attempts.append(Attempt(["go", "vet", "./..."], f", found by compiling for linux/{goarch}",
                                {"GOOS": "linux", "GOARCH": goarch}, True))
    if not other_os and not_linux:
        # macOS is the other system make check compiles for (lint-speculative-api).
        attempts.append(Attempt(["go", "vet", "./..."], ", found by compiling for darwin", {"GOOS": "darwin"}, True))
    return attempts


# The host's architecture as Go names it, for a file named for one.
HOST_GOARCH = {"x86_64": "amd64", "aarch64": "arm64", "arm64": "arm64", "riscv64": "riscv64"}.get(
    platform.machine(), platform.machine())


def compiled_only_elsewhere(path: str) -> bool:
    """Whether the host builds a Go file not at all: named for another system or another architecture,
    or with a build line that leaves Linux or the host's architecture out. Its revert can turn no test
    on the host red, so only another platform's compile can notice it (check-pr, 2026-10-22: a
    _linux_arm64.go file's dependents ran 610 s before the arm64 compile went red)."""
    other_arch = GO_LINUX_ARCH_SUFFIX.search(path)
    return bool(GO_OS_SUFFIX.search(path) or (other_arch and other_arch.group(1) != HOST_GOARCH)
                or build_line_matches(path, GO_NOT_LINUX) or build_line_matches(path, GO_NOT_AMD64))


def build_line_matches(path: str, pattern: re.Pattern[str]) -> bool:
    """Whether a Go file's build line, as the working tree has it, matches: marks a file only another build compiles."""
    try:
        head = Path(path).read_text(encoding="utf-8", errors="replace")[:2048]
    except OSError:
        return False
    return bool(pattern.search(head))
