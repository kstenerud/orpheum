#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Runs the Python tests, one process per test module and as many at once as there are
# ABOUTME: CPUs: the one way make check and revert-red's probes run them.
"""Run scripts/tests, each module in its own process, in parallel.

`unittest discover` ran the modules one after another, 8.6 s on the owner's machine, which made
the script tests the next step on make check's critical path once the web checks ran in parallel
(2026-10-01). Each module already stands alone: it puts what it imports on sys.path itself.
A module's output is printed only when it fails, so a pass stays one line.
"""

from __future__ import annotations

import functools
import os
import shutil
import subprocess
import sys
import tempfile
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from revert_red_dependents import python_tests_reaching_any  # noqa: E402

# Relative to where it runs, so revert-red runs the gate's own copy of this file against the tree
# it probes.
TESTS = Path("scripts/tests")


# A test sees only what is named here, as the code it tests is handed its configuration rather
# than reading the machine's (testing.md section 12): PATH, to find the tools, and the hermetic
# settings, with a home of its own, so no ~/.gitconfig, owner's flag, session variable or git hook's
# GIT_DIR (which once had a test's `git init` reinitialise this repository, 2026-10-01) reaches it.
# The rest is the test's own to set.
PASSED_THROUGH = ("PATH",)
# No bytecode cache: Python reuses one by its source's size and mtime, so a revert-red probe that
# restored a file of the same size within a second ran the reverted code (2026-10-26).
# No media library: the media-name gate compares none of the owner's titles, as a test never reads
# the real library (AGENTS.md rule 16); a test of that rule names a library of its own.
HERMETIC = {"TZ": "UTC", "LC_ALL": "C.UTF-8", "GIT_CONFIG_GLOBAL": "/dev/null", "GIT_CONFIG_NOSYSTEM": "1",
            "PYTHONDONTWRITEBYTECODE": "1", "ORPHEUM_MEDIA_LIBRARY": ""}


# A test that runs Go builds with the machine's module and build caches, and never fetches: each home
# of its own once got a fresh ~500 MB module cache, and a run the gate stopped part-way (fail-fast)
# left its homes behind: 26 of them filled the sandbox's disk in a night (2026-10-25). GOPROXY=off
# makes a module the cache lacks fail the test rather than reach the network (AGENTS.md rule 16).
GO_CACHES = ("GOMODCACHE", "GOCACHE")


@functools.cache
def go_caches() -> dict[str, str]:
    """The machine's Go module and build caches, from the environment or `go env`; none without Go."""
    found = {name: os.environ[name] for name in GO_CACHES if os.environ.get(name)}
    missing = [name for name in GO_CACHES if name not in found]
    if missing and shutil.which("go"):
        asked = subprocess.run(["go", "env", *missing], capture_output=True, text=True, check=False)
        found |= {name: value for name, value in zip(missing, asked.stdout.splitlines(), strict=False) if value}
    return found | {"GOPROXY": "off"} if found else {}


def run(module: Path) -> tuple[Path, subprocess.CompletedProcess[str]]:
    # The module's temporary files go in its home, deleted with it: tests that made directories with
    # mkdtemp and never removed them left them in /tmp at every run (the disk-full review, 2026-10-28).
    with tempfile.TemporaryDirectory(prefix="py-tests-home-") as home:
        temporary = Path(home) / "tmp"
        temporary.mkdir()
        env = {name: os.environ[name] for name in PASSED_THROUGH if name in os.environ}
        env |= HERMETIC | go_caches() | {"HOME": home, "TMPDIR": str(temporary)}
        done = subprocess.run([sys.executable, "-m", "unittest", "-q", str(module)], capture_output=True,
                              text=True, check=False, env=env)
    return module, done


def reached_modules(changed: str | None) -> list[Path] | None:
    """The test modules the changed paths (AREA_STAMP_CHANGED's lines) reach, by revert-red's reach
    (revert_red_dependents.python_tests_reaching_any): None, every module, where no change is known or
    one is not a Python file, which any test may read (the Makefile, a shell script, an allowlist)."""
    if changed is None:
        return None
    paths = [line for line in changed.splitlines() if line]
    reached = python_tests_reaching_any(".", paths)
    return None if reached is None else [Path(module) for module in reached]


def main(argv: list[str]) -> int:
    """Every test module, or only those named (revert-red's narrow run, revert_red_dependents), or with
    --reach only those the paths changed since the step last passed reach (area_stamp.py): make check
    ran all 68, 29 s and two minutes of CPU, for a change to one script (the owner, 2026-11-24)."""
    if argv[:1] == ["--reach"]:
        changed = os.environ.get("AREA_STAMP_CHANGED")
        reached = reached_modules(changed)
        if reached is not None:
            count = len([line for line in (changed or "").splitlines() if line])
            print(f"py_tests: {len(reached)} module(s) reach the {count} Python file(s) changed since the last pass")
            if not reached:
                return 0
            argv = [str(module) for module in reached]
        else:
            argv = []
    modules = [Path(name) for name in argv] if argv else sorted(TESTS.glob("test_*.py"))
    with ThreadPoolExecutor(max_workers=os.cpu_count() or 4) as pool:
        results = list(pool.map(run, modules))
    failed = [(module, done) for module, done in results if done.returncode != 0]
    for module, done in failed:
        print(f"--- {module.name} failed ---\n{done.stdout}{done.stderr}")
    print(f"py_tests: {len(modules)} modules, {len(failed)} failed")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
