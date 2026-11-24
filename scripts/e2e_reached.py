#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: make check's end-to-end step: the browser tier's specs a change to the web client reaches, both
# ABOUTME: passes, so a spec a change left stale is found at the turn's end, not first in check-pr.
"""The end-to-end specs a change to the web client reaches, run by make check.

Ten specs a design cycle left stale were first seen in check-pr's browser tier, an hour into it (the
owner, 2026-11-24, agreeing to the test fixes after a design cycle, item 1: "make check runs them for
a change to the web client, chosen by reach, as make spot chooses unit tests").

Only a change under web/ that the browser tier builds or runs calls for a run: the client's sources
and stylesheets, the specs and their helpers, the build's configuration; not a unit test, a document
or M4's acceptance. A change of the server, the gates or the docs is check-pr's tier's to find.

The reach is check-pr's (impact.browser_scope): the tests that loaded a chunk holding a changed
source or drew what a changed stylesheet's rules select, by the last complete browser map, and each
changed spec whole. It is measured from the nearest tree this step passed on (its record, kept as
web_tests.py keeps its own), or, before any pass, from the map's commit; what the map cannot place
runs the whole tier, as check-pr's does. The run is the browser tier's (`make browser-tier`), both
passes, side by side and timed, and records no map.
"""

from __future__ import annotations

import fnmatch
import json
import os
import subprocess
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import impact  # noqa: E402
from area_stamp import stamp_path  # noqa: E402
from check_stamp import working_tree_id  # noqa: E402
from web_tests import passed_trees, record_pass  # noqa: E402

STEP = "test-e2e-reached"
# Under web/, what the browser tier neither builds nor runs: tests of other suites, documents, and M4's
# acceptance with its own configuration and server (test-acceptance-home runs them).
NOT_RUN = (*impact.NOT_IN_THE_TIER, "web/e2e/acceptance*")


def read_by_the_tier(path: str) -> bool:
    """Whether path, changed, is one the browser tier builds or runs."""
    return path.startswith("web/") and not any(fnmatch.fnmatch(path, pattern) for pattern in NOT_RUN)


def changes_between(before: str, after: str) -> list[str] | None:
    """The paths the browser tier reads that differ between two trees; None when git no longer has
    before (pruned)."""
    done = subprocess.run(["git", "diff", "--name-only", "--no-renames", before, after, "--", "web"],
                          capture_output=True, text=True, check=False)
    if done.returncode != 0:
        return None
    return [path for path in done.stdout.splitlines() if path and read_by_the_tier(path)]


def nearest_pass(trees: list[str], tree: str) -> tuple[str, list[str]] | None:
    """The passed tree the fewest such paths differ from, the newest of those, and its changes; None
    when no passed tree is usable."""
    nearest = None
    for passed in trees:
        changes = changes_between(passed, tree)
        if changes is not None and (nearest is None or len(changes) < len(nearest[1])):
            nearest = (passed, changes)
    return nearest


def scope(tree: str, trees: list[str], scope_of=impact.browser_scope) -> tuple[list[str] | None, str]:
    """The tests to run: [] for none, None for the whole tier, else by name or by spec file; and why."""
    nearest = nearest_pass(trees, tree)
    if nearest is None:
        tests, why = scope_of()  # before any pass: every change since the map's commit
        return tests, why
    since, changes = nearest
    if not changes:
        return [], f"nothing the browser tier builds or runs changed since it last passed ({since[:12]})"
    return scope_of(paths=changes, since=since)


def run(tests: list[str] | None) -> int:
    """The browser tier's two passes over tests (None: every one); its exit status."""
    env = dict(os.environ)
    env.pop("ORPHEUM_BROWSER_MAP_DIR", None)  # a narrowed or dirty tree's run is no map
    if tests is not None:
        env["ORPHEUM_E2E_ONLY"] = json.dumps(tests)
    for target in ("browsers", "browser-tier"):
        done = subprocess.run(["make", "--no-print-directory", target], env=env, check=False)
        if done.returncode != 0:
            return done.returncode
    return 0


def main(scope_of=impact.browser_scope, run_tests=run) -> int:
    tree = working_tree_id()
    stamp = stamp_path(STEP)
    tests, why = scope(tree, passed_trees(stamp), scope_of)
    if tests == []:
        print(f"{STEP}: {why or 'no end-to-end test reaches what changed'}; none run")
        record_pass(stamp, tree)
        return 0
    if tests is None:
        print(f"{STEP}: every end-to-end test, both passes: {why}", flush=True)
    else:
        print(f"{STEP}: {len(tests)} end-to-end test(s) or spec(s), both passes: {why}", flush=True)
    status = run_tests(tests)
    if status == 0:
        record_pass(stamp, tree)
    return status


if __name__ == "__main__":
    sys.exit(main())
