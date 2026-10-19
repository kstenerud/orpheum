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
import tempfile
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

# Relative to where it runs, so revert-red runs the gate's own copy of this file against the tree
# it probes.
TESTS = Path("scripts/tests")


# A test sees only what is named here, as the code it tests is handed its configuration rather
# than reading the machine's (testing.md section 12): PATH, to find the tools, and the hermetic
# settings, with a home of its own, so no ~/.gitconfig, owner's flag, session variable or git hook's
# GIT_DIR (which once had a test's `git init` reinitialise this repository, 2026-10-01) reaches it.
# The rest is the test's own to set.
PASSED_THROUGH = ("PATH",)
HERMETIC = {"TZ": "UTC", "LC_ALL": "C.UTF-8", "GIT_CONFIG_GLOBAL": "/dev/null", "GIT_CONFIG_NOSYSTEM": "1"}


def run(module: Path) -> tuple[Path, subprocess.CompletedProcess[str]]:
    with tempfile.TemporaryDirectory(prefix="py-tests-home-") as home:
        env = {name: os.environ[name] for name in PASSED_THROUGH if name in os.environ}
        env |= HERMETIC | {"HOME": home}
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
