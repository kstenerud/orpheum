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

import os
import subprocess
import sys
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

# Relative to where it runs, so revert-red runs the gate's own copy of this file against the tree
# it probes.
TESTS = Path("scripts/tests")


# Run from a git hook, the environment names the repository being committed to, and a test's own
# `git init` in a scratch directory then reinitialised that repository instead: it set core.bare
# on this one (2026-10-01). No test may inherit them.
HOOK_GIT_VARIABLES = ("GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR", "GIT_PREFIX",
                      "GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES")


def run(module: Path) -> tuple[Path, subprocess.CompletedProcess[str]]:
    env = {name: value for name, value in os.environ.items() if name not in HOOK_GIT_VARIABLES}
    done = subprocess.run([sys.executable, "-m", "unittest", "-q", str(module)], capture_output=True,
                          text=True, check=False, env=env)
    return module, done


def main(argv: list[str]) -> int:
    """Every test module, or only those named (revert-red's narrow run, revert_red_dependents)."""
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
