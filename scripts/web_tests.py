#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: make check's web tests: only those a change since their last pass can reach, by the names it
# ABOUTME: changed, or the whole suite when what every test reads changed or there is no record.
"""Run the web tests a change can reach, rather than every one.

Any change under web/ ran the whole suite, 60 to 125 s, for a one-file change whose importers' tests
take a few seconds (the owner, 2026-10-13: "Our tests should be intelligent enough and
compartmentalized enough that we can test only the parts actually affected by a change"). The tree the
suite last passed on is recorded, as area_stamp.py records it; against it:

- nothing under web/ changed: nothing runs, as before;
- something every test reads changed (a package manifest, the lockfile, the vitest or TypeScript
  configuration), a file was deleted (a test still importing it would fail, and no graph reaches it
  any more), or there is no record: the whole suite;
- otherwise the tests web_test_reach.cjs finds the changed files reach: what uses the declarations
  they changed, followed by name through re-exports, and what loads them when their loading changed,
  each suite's own run by name; and the tests
  that read the sources by path (revert_red_dependents.web_source_readers), which no import graph
  reaches. `vitest related` followed files, and through @orpheum/sdk's index.ts, which re-exports
  every module, a change to one module ran 400 of the 540 tests (the owner, 2026-11-21: "go ahead").
  A change the reach cannot follow (a stylesheet, JSON, a reach that fails) runs `vitest related`.

A pass records the tree. That is sound by induction: the tests not run were passing on the recorded
tree and import nothing that changed since. The last PASSES_KEPT passed trees are kept, newest first,
and the run is measured from the nearest: the record is shared by every worktree, and with one tree
kept, the lead's checkout and an agent's worktree on another base each found the other's pass and ran
the tests of both trees' differences (the lock's wargaming, 2026-11-04).

`--browser` does the same for the browser-mode tests (web/vitest.browser.config.ts), under their own
record: any change under web/ ran all of them, 66 s and growing, last in make check, where every
turn that touched the web waited for them (the caching audit's finding 8, 2026-10-28). Each browser
runs in a run of its own, one after the other: run together, vitest opened Firefox pages mid-run as
Chromium's files ended, and each took the keyboard's focus from a running test's frame, which lost
the keys pressed meanwhile (idiosyncrasies.md, "A page opening in Firefox takes the focus from the
test frame opened before it"; make check typed "k" for "karl", 2026-11-20). Apart, a full run took
217 s against 208 s.

With `--lock PATH` (make check's browser-mode run, the Makefile's BROWSER_MODE_LOCK) the run holds
that lock, one browser-mode run at a time on the machine, and its hold has a lease
(BROWSER_LEASE_SECONDS). A run finding it held waits while the holder is within its lease; past it,
it says the browser-mode tests did not run, naming the holder, and fails: a run left behind held it
for about 12 hours, and every make check in that time hung at the browser tests with nothing said
(the owner, 2026-11-24, agreeing to the test-time stock-take's item 3).
"""

from __future__ import annotations

import argparse
import contextlib
import datetime
import fcntl
import json
import os
import subprocess
import sys
import time
from collections.abc import Callable, Iterator
from dataclasses import dataclass
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from area_stamp import stamp_path  # noqa: E402
from check_stamp import working_tree_id  # noqa: E402
from impact import WEB_SOURCE, web_sources  # noqa: E402
from revert_red_dependents import web_source_readers  # noqa: E402

WEB = "web"
# How long a browser-mode run may hold the lock before a run waiting for it gives up: a whole run takes
# about four minutes, twice that under load.
BROWSER_LEASE_SECONDS = 20 * 60
# What every test reads, beside the files it imports.
EVERY_TEST_READS = ("package.json", "package-lock.json", "vitest.config.ts", "vitest.browser.config.ts", "tsconfig")


@dataclass(frozen=True)
class Suite:
    """A vitest suite: its record's name, its whole run, its related run's command, its run of named
    test files, the files every test of it loads first (its setupFiles, relative to web/), and the
    vitest projects it runs one after the other, each in a run of its own (none: one run of them all),
    and whether its tests are the browser-mode ones (*.browser.test.*) or the others."""

    step: str
    whole: list[str]
    related: list[str]
    named: list[str]
    setup: tuple[str, ...] = ()
    apart: tuple[str, ...] = ()
    browser: bool = False

    def holds(self, test: str) -> bool:
        """Whether test, a test file, is one of this suite's (its configuration's include)."""
        return (".browser.test." in test) == self.browser


RELATED = ["npx", "--prefix", WEB, "vitest", "related", "--run", "--passWithNoTests", "--root", WEB]
NAMED = ["npx", "--prefix", WEB, "vitest", "run", "--passWithNoTests", "--root", WEB]
BROWSER_CONFIG = ["--config", "vitest.browser.config.ts"]
UNIT = Suite("web-test", ["npm", "--prefix", WEB, "run", "--silent", "test"], RELATED, NAMED)
BROWSER = Suite("web-test-browser-related", ["npm", "--prefix", WEB, "run", "--silent", "test:browser"],
                [*RELATED, *BROWSER_CONFIG], [*NAMED, *BROWSER_CONFIG], setup=("e2e/frame-focus-setup.ts", "e2e/frame-storage-setup.ts"),
                apart=("chromium", "firefox"), browser=True)
# The script that follows a change to the tests that can see it, run by node in web/.
WEB_TEST_REACH = Path(__file__).resolve().parent / "web_test_reach.cjs"
TEST_FILE = (".test.ts", ".test.tsx")
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


def reached_tests(base: str, changed: list[str], suite: Suite, typescript: Path,
                  depth: int | None = None) -> tuple[list[str] | None, str]:
    """The test files, relative to web/, that changed (relative to web/) since base reach
    (web_test_reach.cjs); None, and why, when it cannot tell, and ["*"] when every test is reached.
    `depth` keeps the tests within that many uses or loads of a change, a barrel passing it on not
    counted (spot_check.py's 1: the tests using a changed name themselves)."""
    unread = [path for path in changed if not WEB_SOURCE.match(f"{WEB}/{path}") or path.endswith(".d.ts")]
    if unread:
        return None, f"{unread[0]} is not a source the reach reads"
    try:
        done = subprocess.run(["node", str(WEB_TEST_REACH)], cwd=WEB, capture_output=True, text=True, check=False,
                              input=json.dumps({"typescript": str(typescript), "base": base, "changed": changed,
                                                "universe": web_sources(), "roots": list(suite.setup),
                                                **({} if depth is None else {"depth": depth})}),
                              timeout=300)
    except (OSError, subprocess.TimeoutExpired) as failed:
        return None, f"{WEB_TEST_REACH.name} did not run: {failed}"
    if done.returncode != 0:
        return None, f"{WEB_TEST_REACH.name} failed: {done.stderr.strip().splitlines()[-1:] or done.returncode}"
    answer = json.loads(done.stdout)
    if "wide" in answer:
        return ["*"], answer["wide"]
    return [path for path in answer["reached"] if path.endswith(TEST_FILE)], ""


def command_for(changes: list[tuple[str, str]], root: str, suite: Suite = UNIT, base: str = "",
                reach_of=None) -> list[str] | None:
    """The run the changes since base call for: None for none, the whole suite, the tests they reach
    ([] when they reach none of the suite's: nothing runs, and the pass is recorded), or their related
    tests when the reach cannot tell (or node_modules holds no TypeScript to read with)."""
    if not changes:
        return None
    for status, path in changes:
        if status == "D" or Path(path).name.startswith(EVERY_TEST_READS):
            return suite.whole
    within = [path.removeprefix(WEB + "/") for _, path in changes]
    readers = list(web_source_readers(root))
    typescript = Path(root, WEB, "node_modules", "typescript")
    tests = None
    if base and typescript.is_dir():
        tests, why = (reach_of or reached_tests)(base, within, suite, typescript)
        if why:
            print(f"{suite.step}: {why}")
    if tests is None:
        return [*suite.related, *within, *readers]
    if tests == ["*"]:
        return suite.whole
    # vitest runs every test when given no file to filter by, so a run of none is no run.
    named = sorted(test for test in set(tests) | set(readers) if suite.holds(test))
    return [*suite.named, *named] if named else []


def runs_of(command: list[str], suite: Suite) -> list[list[str]]:
    """The runs command makes in suite: one per project it keeps apart, else itself."""
    if not suite.apart:
        return [command]
    # npm passes what follows "--" to its script; npx and vitest take them as they are.
    split = ["--"] if command[:2] == ["npm", "--prefix"] else []
    return [[*command, *split, "--project", project] for project in suite.apart]


# The passed trees each suite keeps.
PASSES_KEPT = 8


def passed_trees(stamp: Path) -> list[str]:
    """The trees the suite passed on, newest first."""
    return [line.strip() for line in stamp.read_text().splitlines() if line.strip()] if stamp.exists() else []


def nearest_command(trees: list[str], tree: str, root: str, suite: Suite) -> list[str] | None:
    """The run the passed tree nearest tree calls for, the one fewest files under web/ differ from (the
    newest of those): None when one holds what tree holds, the whole suite when none is usable (a tree
    git has since pruned is passed over). Only the nearest is followed: following a change takes
    seconds, and every passed tree's would take each make check that many times."""
    nearest = None
    for passed in trees:
        try:
            changes = changes_since(passed, tree)
        except subprocess.CalledProcessError:
            continue
        if not changes:
            return None
        if nearest is None or len(changes) < len(nearest[1]):
            nearest = (passed, changes)
    if nearest is None:
        return suite.whole
    return command_for(nearest[1], root, suite, nearest[0])


def record_pass(stamp: Path, tree: str) -> None:
    trees = [tree, *(passed for passed in passed_trees(stamp) if passed != tree)][:PASSES_KEPT]
    stamp.parent.mkdir(parents=True, exist_ok=True)
    stamp.write_text("\n".join(trees) + "\n")


def _holder_of(lock, first_seen: float) -> dict:
    """What holds lock, as it wrote it ({"pid", "started"}); a holder that wrote nothing (flock(1), as
    a tree on an older Makefile takes it) is dated from when the waiter first found it held."""
    lock.seek(0)
    try:
        said = json.loads(lock.read() or "{}")
    except json.JSONDecodeError:
        said = {}
    said = said if isinstance(said, dict) else {}
    if not isinstance(said.get("started"), (int, float)):
        said["started"] = first_seen
    return said


@contextlib.contextmanager
def browser_lock(path: Path, lease: float = BROWSER_LEASE_SECONDS, sleep: Callable[[float], None] = time.sleep,
                 now: Callable[[], float] = time.time) -> Iterator[bool]:
    """Holds path's lock, waiting while its holder is within lease; yields True once held, or False,
    having said why, when the holder has held it past lease."""
    first_seen = now()
    with open(path, "a+", encoding="utf-8") as lock:
        while True:
            try:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                break
            except BlockingIOError:
                holder = _holder_of(lock, first_seen)
                held = now() - float(holder["started"])
                if held > lease:
                    since = datetime.datetime.fromtimestamp(float(holder["started"]), datetime.timezone.utc)
                    print(f"web-test-browser: the browser-mode tests did not run: {path} has been held by "
                          f"pid {holder.get('pid', 'unknown')} since {since:%H:%M} UTC ({held / 60:.0f} min), "
                          f"past its {lease / 60:.0f} min lease. It is probably a run left behind: end its "
                          "process group and run the gate again.", flush=True)
                    yield False
                    return
                sleep(1)
        lock.seek(0)
        lock.truncate()
        lock.write(json.dumps({"pid": os.getpid(), "started": now(), "tree": os.getcwd()}) + "\n")
        lock.flush()
        try:
            yield True
        finally:
            # A record left behind would date the next holder that writes none.
            lock.truncate(0)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--browser", action="store_true", help="the browser-mode tests")
    parser.add_argument("--lock", type=Path, help="hold this lock for the run, its hold leased (browser_lock)")
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
    if command == []:
        print(f"{suite.step}: the changes since the nearest pass reach none of its tests")
        record_pass(stamp, tree)
        return 0
    if command is not suite.whole:
        print(f"{suite.step}: only the tests the changes since the nearest pass reach")
    with browser_lock(arguments.lock) if arguments.lock else contextlib.nullcontext(True) as held:
        if not held:
            return 1
        for each in runs_of(command, suite):
            done = subprocess.run(each, check=False)
            if done.returncode != 0:
                return done.returncode
    record_pass(stamp, tree)
    return 0


if __name__ == "__main__":
    sys.exit(main())
