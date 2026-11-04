#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: make check's web tests: only those a change since their last pass can reach, by vitest's
# ABOUTME: import graph, or the whole suite when what every test reads changed or there is no record.
"""Run the web tests a change can reach, rather than every one.

Any change under web/ ran the whole suite, 60 to 125 s, for a one-file change whose importers' tests
take a few seconds (the owner, 2026-10-13: "Our tests should be intelligent enough and
compartmentalized enough that we can test only the parts actually affected by a change"). The tree the
suite last passed on is recorded, as area_stamp.py records it; against it:

- nothing under web/ changed: nothing runs, as before;
- something every test reads changed (a package manifest, the lockfile, the vitest or TypeScript
  configuration), a file was deleted (a test still importing it would fail, and no graph reaches it
  any more), or there is no record: the whole suite;
- otherwise `vitest related` with every changed file, which runs the tests that import any of them,
  directly or not, and any changed test itself, and the tests that read the sources by path
  (revert_red_dependents.web_source_readers), which no import graph reaches.

A pass records the tree. That is sound by induction: the tests not run were passing on the recorded
tree and import nothing that changed since. The last PASSES_KEPT passed trees are kept, newest first,
and the run is measured from the nearest: the record is shared by every worktree, and with one tree
kept, the lead's checkout and an agent's worktree on another base each found the other's pass and ran
the tests of both trees' differences (the lock's wargaming, 2026-11-04).

`--browser` does the same for the browser-mode tests (web/vitest.browser.config.ts), under their own
record: any change under web/ ran all of them, 66 s and growing, last in make check, where every
turn that touched the web waited for them (the caching audit's finding 8, 2026-10-28).
"""

from __future__ import annotations

import argparse
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from area_stamp import stamp_path  # noqa: E402
from check_stamp import working_tree_id  # noqa: E402
from revert_red_dependents import web_source_readers  # noqa: E402

WEB = "web"
# What every test reads, beside the files it imports.
EVERY_TEST_READS = ("package.json", "package-lock.json", "vitest.config.ts", "vitest.browser.config.ts", "tsconfig")


@dataclass(frozen=True)
class Suite:
    """A vitest suite: its record's name, its whole run, and its related run's command."""

    step: str
    whole: list[str]
    related: list[str]


RELATED = ["npx", "--prefix", WEB, "vitest", "related", "--run", "--passWithNoTests", "--root", WEB]
UNIT = Suite("web-test", ["npm", "--prefix", WEB, "run", "--silent", "test"], RELATED)
BROWSER = Suite("web-test-browser-related", ["npm", "--prefix", WEB, "run", "--silent", "test:browser"],
                [*RELATED, "--config", "vitest.browser.config.ts"])
# Kept for the tests and callers that name the unit suite's.
STEP, WHOLE_SUITE = UNIT.step, UNIT.whole


def changes_since(passed_tree: str, tree: str) -> list[tuple[str, str]]:
    """(status, path) of every file under web/ that differs between the two trees."""
    done = subprocess.run(["git", "diff", "--name-status", "--no-renames", passed_tree, tree, "--", WEB],
                          capture_output=True, text=True, check=True)
    changes = []
    for line in done.stdout.splitlines():
        status, _, path = line.partition("\t")
        changes.append((status, path))
    return changes


def command_for(changes: list[tuple[str, str]], root: str, suite: Suite = UNIT) -> list[str] | None:
    """The run the changes call for: None for none, the whole suite, or the related tests."""
    if not changes:
        return None
    for status, path in changes:
        if status == "D" or Path(path).name.startswith(EVERY_TEST_READS):
            return suite.whole
    within = [path.removeprefix(WEB + "/") for _, path in changes]
    return [*suite.related, *within, *web_source_readers(root)]


# The passed trees each suite keeps.
PASSES_KEPT = 8


def passed_trees(stamp: Path) -> list[str]:
    """The trees the suite passed on, newest first."""
    return [line.strip() for line in stamp.read_text().splitlines() if line.strip()] if stamp.exists() else []


def nearest_command(trees: list[str], tree: str, root: str, suite: Suite) -> list[str] | None:
    """The smallest run any passed tree calls for: None when one holds what tree holds under web/, the
    whole suite when none is usable (a tree git has since pruned is passed over)."""
    best = suite.whole
    for passed in trees:
        try:
            command = command_for(changes_since(passed, tree), root, suite)
        except subprocess.CalledProcessError:
            continue
        if command is None:
            return None
        if command is not suite.whole and (best is suite.whole or len(command) < len(best)):
            best = command
    return best


def record_pass(stamp: Path, tree: str) -> None:
    trees = [tree, *(passed for passed in passed_trees(stamp) if passed != tree)][:PASSES_KEPT]
    stamp.parent.mkdir(parents=True, exist_ok=True)
    stamp.write_text("\n".join(trees) + "\n")


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--browser", action="store_true", help="the browser-mode tests")
    parser.add_argument("--due", action="store_true",
                        help="only say whether a run is due (exit 0) or not (exit 1), running nothing")
    arguments = parser.parse_args(argv)
    suite = BROWSER if arguments.browser else UNIT
    root = subprocess.run(["git", "rev-parse", "--show-toplevel"], capture_output=True, text=True,
                          check=True).stdout.strip()
    tree = working_tree_id()
    stamp = stamp_path(suite.step)
    if stamp.exists():
        command = nearest_command(passed_trees(stamp), tree, root, suite)
        if command is None:
            print(f"{suite.step}: nothing it reads changed since it last passed; not run again")
            return 1 if arguments.due else 0
    else:
        command = suite.whole
    if arguments.due:
        return 0
    if command is not suite.whole:
        print(f"{suite.step}: only the tests the changes since the nearest pass reach")
    done = subprocess.run(command, check=False)
    if done.returncode != 0:
        return done.returncode
    record_pass(stamp, tree)
    return 0


if __name__ == "__main__":
    sys.exit(main())
