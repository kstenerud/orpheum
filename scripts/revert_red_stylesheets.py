# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Settles changed stylesheets by halves: every one reverted in one browser run, and when that
# ABOUTME: run fails, each half in its own, down to the sheets a test noticed.
"""Which of the changed stylesheets a browser run notices, in as few runs as halving allows.

A browser run costs some 160 s, one at a time. The run with every changed sheet reverted settles
them all when it passes; when it failed, each sheet took a run of its own: thirteen, 22 minutes of
check-pr, after that one run had failed in 5 s for a reason nothing kept (check-pr's timings,
2026-10-13). Halving finds k noticed sheets among n in about 2k·log2(n) runs, and a failing run's
last lines are kept for the report, so a run that failed for its own reasons says so.

Before it, each sheet's own browser-mode tests (own_browser_mode_reds) run once for every sheet at
once, a red put on the sheet whose revert alone fails the tests that failed.
"""

from __future__ import annotations

import contextlib
import functools
import queue
import subprocess
import time
from concurrent.futures import ThreadPoolExecutor
from dataclasses import dataclass, field
from typing import Callable

import revert_red_commands as commands
import revert_red_repeat as repeat
from revert_red_dependents import git_run
from revert_red_run import run_whole_group
from revert_red_timings import Timings, timed
from revert_red_units import restore

# A failing group this small is run sheet by sheet, which costs no more than halving it.
ONE_BY_ONE_AT_MOST = 3
# How much of a failing run's output the report keeps.
KEPT_LINES = 15


@dataclass
class Halving:
    """What the runs said: the sheets no run noticed, each noticed sheet with its run's output, the
    sheets a timed-out run left unsettled, and the failing runs' last lines."""

    passed: list[str] = field(default_factory=list)
    noticed: dict[str, str] = field(default_factory=dict)
    unsettled: list[str] = field(default_factory=list)
    failures: list[str] = field(default_factory=list)


def settle(sheets: list[str], run_reverted: Callable[[list[str]], subprocess.CompletedProcess[str] | None],
           halving: Halving | None = None) -> Halving:
    """Runs the browser tier with `sheets` reverted at once, then each half of a failing group."""
    halving = halving if halving is not None else Halving()
    if not sheets:
        return halving
    done = run_reverted(sheets)
    if done is None:
        halving.unsettled.extend(sheets)
        return halving
    if done.returncode == 0:
        halving.passed.extend(sheets)
        return halving
    output = done.stdout + done.stderr
    tail = "\n".join(output.strip().splitlines()[-KEPT_LINES:])
    halving.failures.append(f"with {', '.join(sheets)} reverted:\n{tail}")
    if len(sheets) == 1:
        halving.noticed[sheets[0]] = output
        return halving
    # Halves cost two runs a level; three sheets or fewer cost no more one by one.
    parts = [[sheet] for sheet in sheets] if len(sheets) <= ONE_BY_ONE_AT_MOST else \
        [sheets[:len(sheets) // 2], sheets[len(sheets) // 2:]]
    for part in parts:
        settle(part, run_reverted, halving)
    return halving


def own_browser_mode_reds(tree: str, base: str, head: str, sheets: list[str], related: list[str] | None,
                          timeout: int, timings: Timings, spare: queue.Queue[str] | None = None,
                          readers: list[str] | None = None) -> dict[str, str]:
    """The sheets whose own browser-mode tests turn red with it reverted alone, with what the run
    printed; each run timed in timings, each sheet put back. A run past its timeout settles nothing.

    `readers`, when given, runs first: the tests that read the stylesheets as text, which no import
    reaches, a few seconds; a red there settles the sheet. Without it, check-pr called eight sheets
    green on revert though stylesheet-choices.test.ts failed with them reverted (2026-11-12).

    Then the sheets left go into one browser-mode run, every one reverted (`together`): a sheet's
    own run was 340 to 640 s, seventeen of them 5795 s of one check-pr's probing, and every one
    passed (2026-11-24; the owner's go the same day). A pass settles them all, as the stylesheets'
    tier run does (D279). A red is found by its failed tests (`failed_alone`): each sheet reverted
    alone runs them, a red there is that sheet's own, and the run together goes again without it,
    so every red still stands on a run with its sheet alone reverted. Where the red names no
    browser-mode test, a sheet alone turns none of them red, or the run together times out, each
    sheet takes its own whole run, as before (`alone`).

    The runs go side by side, each in tree or in a probe tree it borrows from spare as one comes
    free: one after another in one tree, about two minutes each, they were 1000 s of a check-pr while
    the other three trees sat idle (2026-11-03). So a red's failed tests run again, the sheet still
    reverted, one repeat at a time, and must fail again (revert_red_repeat): title.css's red was a
    test failing beside the others (2026-11-24)."""
    if not related and not readers:
        return {}
    noticed: dict[str, str] = {}
    mine: queue.Queue[str] = queue.Queue()
    mine.put(tree)

    def borrowed() -> tuple[str, queue.Queue[str]]:
        while True:
            for pool in (mine, spare):
                if pool is None:
                    continue
                with contextlib.suppress(queue.Empty):
                    return pool.get(timeout=0.2), pool

    def reverted(paths: list[str], work: Callable[[str], None]) -> bool:
        """Runs work in a tree with paths reverted, and puts them back: False where a revert failed
        or work ran past its timeout, which settles nothing."""
        here, pool = borrowed()
        started = time.monotonic()
        try:
            for path in paths:
                if git_run("checkout", base, "--", path, cwd=here).returncode != 0:
                    return False
            work(here)
            return True
        except subprocess.TimeoutExpired:
            return False
        finally:
            for path in paths:
                restore(here, head, path)
            if pool is spare:
                timings.note_tree(here, time.monotonic() - started)
            pool.put(here)

    def each(work: Callable[[str], object], paths: list[str]) -> None:
        # With no spare trees, one tree: one run at a time, in the sheets' order.
        with ThreadPoolExecutor(max_workers=1 if spare is None else max(1, len(paths))) as runs:
            for done in [runs.submit(work, path) for path in paths]:
                done.result()

    def red_that_repeats(label: str, kind: str, command: list[str], here: str) -> str | None:
        """What command printed, when it failed in here and its failed browser tests failed again."""
        done = timed(timings, label, kind, None, functools.partial(run_whole_group, command, here, timeout))
        if done.returncode == 0:
            return None
        output = done.stdout + done.stderr
        # A red stands once its failed browser tests fail again, on their own (revert_red_repeat).
        return None if repeat.did_not_repeat(timings, label, command, output, here, timeout) else output

    def read_alone(path: str) -> None:
        def work(here: str) -> None:
            read = timed(timings, path, "vitest, the tests that read stylesheets", None,
                         functools.partial(run_whole_group, readers, here, timeout))
            if read.returncode != 0:
                noticed[path] = read.stdout + read.stderr
        reverted([path], work)

    def alone(path: str) -> None:
        def work(here: str) -> None:
            output = red_that_repeats(path, "vitest related (browser mode)", [*related, path.removeprefix("web/")],
                                      here)
            if output is not None:
                noticed[path] = output
        reverted([path], work)

    def together(group: list[str]) -> list[str] | None:
        """The sheets of group found red, none when the run with them all reverted passed, or None
        when what it said cannot be put on a sheet alone."""
        red: list[str] = []
        label = f"({len(group)} stylesheets at once)"

        stopped_early: list[bool] = []

        def work(here: str) -> None:
            command = [*related, *(path.removeprefix("web/") for path in group)]
            done = timed(timings, label, "vitest related (browser mode), the sheets together", None,
                         functools.partial(run_whole_group, command, here, timeout))
            if done.returncode == 0:
                return
            output = done.stdout + done.stderr
            # A red stands once its failed browser tests fail again, on their own (revert_red_repeat).
            if repeat.did_not_repeat(timings, label, command, output, here, timeout):
                stopped_early.append(True)
            else:
                red.append(output)
        if not reverted(group, work):
            return None
        # A run that failed on a test that then passed is no pass: it stops at its first failure, so
        # the tests after it never ran. Four sheets' run stopped at such a test of the Play page's
        # layout, the castee's layout's never ran, and cast.css was called green on revert though
        # its revert alone turns that test red (check-pr, 2026-11-26). Each sheet takes its own run.
        if stopped_early:
            return None
        if not red:
            return []
        # The run stops at its first failure, so its failed tests are one sheet's at most: which, its
        # revert alone says, and the run together goes again for any other.
        failed = repeat.failed_browser_files(red[0], related)
        if not failed:
            return None
        found: dict[str, str] = {}

        def failed_alone(path: str) -> None:
            def alone_work(here: str) -> None:
                output = red_that_repeats(path, "vitest, the sheets' failed browser tests, it alone reverted",
                                          commands.vitest("run", *failed, browser=True), here)
                if output is not None:
                    found[path] = output
            reverted([path], alone_work)
        each(failed_alone, group)
        noticed.update(found)
        return list(found) or None

    if readers:
        each(read_alone, sheets)
    if not related:
        return noticed
    left = [path for path in sheets if path not in noticed]
    while len(left) > 1:
        found = together(left)
        if found is None:
            break
        if not found:
            return noticed
        left = [path for path in left if path not in found]
    each(alone, left)
    return noticed
