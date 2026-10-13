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
tree and import nothing that changed since.
"""

from __future__ import annotations

import subprocess
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from area_stamp import stamp_path  # noqa: E402
from check_stamp import working_tree_id  # noqa: E402
from revert_red_dependents import web_source_readers  # noqa: E402

STEP = "web-test"
WEB = "web"
WHOLE_SUITE = ["npm", "--prefix", WEB, "run", "--silent", "test"]
RELATED = ["npx", "--prefix", WEB, "vitest", "related", "--run", "--passWithNoTests", "--root", WEB]
# What every test reads, beside the files it imports.
EVERY_TEST_READS = ("package.json", "package-lock.json", "vitest.config.ts", "tsconfig")


def changes_since(passed_tree: str, tree: str) -> list[tuple[str, str]]:
    """(status, path) of every file under web/ that differs between the two trees."""
    done = subprocess.run(["git", "diff", "--name-status", "--no-renames", passed_tree, tree, "--", WEB],
                          capture_output=True, text=True, check=True)
    changes = []
    for line in done.stdout.splitlines():
        status, _, path = line.partition("\t")
        changes.append((status, path))
    return changes


def command_for(changes: list[tuple[str, str]], root: str) -> list[str] | None:
    """The run the changes call for: None for none, the whole suite, or the related tests."""
    if not changes:
        return None
    for status, path in changes:
        if status == "D" or Path(path).name.startswith(EVERY_TEST_READS):
            return WHOLE_SUITE
    within = [path.removeprefix(WEB + "/") for _, path in changes]
    return [*RELATED, *within, *web_source_readers(root)]


def main() -> int:
    root = subprocess.run(["git", "rev-parse", "--show-toplevel"], capture_output=True, text=True,
                          check=True).stdout.strip()
    tree = working_tree_id()
    stamp = stamp_path(STEP)
    if stamp.exists():
        command = command_for(changes_since(stamp.read_text().strip(), tree), root)
        if command is None:
            print(f"{STEP}: nothing it reads changed since it last passed; not run again")
            return 0
    else:
        command = WHOLE_SUITE
    if command is not WHOLE_SUITE:
        print(f"{STEP}: only the tests the changes since the last pass reach")
    done = subprocess.run(command, check=False)
    if done.returncode != 0:
        return done.returncode
    stamp.parent.mkdir(parents=True, exist_ok=True)
    stamp.write_text(tree + "\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
