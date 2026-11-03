# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Settles changed stylesheets by halves: every one reverted in one browser run, and when that
# ABOUTME: run fails, each half in its own, down to the sheets a test noticed.
"""Which of the changed stylesheets a browser run notices, in as few runs as halving allows.

A browser run costs some 160 s, one at a time. The run with every changed sheet reverted settles
them all when it passes; when it failed, each sheet took a run of its own: thirteen, 22 minutes of
check-pr, after that one run had failed in 5 s for a reason nothing kept (check-pr's timings,
2026-10-13). Halving finds k noticed sheets among n in about 2k·log2(n) runs, and a failing run's
last lines are kept for the report, so a run that failed for its own reasons says so.
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
                          timeout: int, timings: Timings, spare: queue.Queue[str] | None = None) -> dict[str, str]:
    """The sheets whose own browser-mode tests turn red with it reverted alone, with what the run
    printed; each run timed in timings, each sheet put back. A run past its timeout settles nothing.

    The runs go side by side, each in tree or in a probe tree it borrows from spare as one comes
    free: one after another in one tree, about two minutes each, they were 1000 s of a check-pr while
    the other three trees sat idle (2026-11-03)."""
    if not related:
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

    def one(path: str) -> None:
        here, pool = borrowed()
        started = time.monotonic()
        try:
            if git_run("checkout", base, "--", path, cwd=here).returncode != 0:
                return
            done = timed(timings, path, "vitest related (browser mode)", None,
                         functools.partial(run_whole_group, [*related, path.removeprefix("web/")], here, timeout))
            if done.returncode != 0:
                noticed[path] = done.stdout + done.stderr
        except subprocess.TimeoutExpired:
            pass
        finally:
            restore(here, head, path)
            if pool is spare:
                timings.note_tree(here, time.monotonic() - started)
            pool.put(here)

    # With no spare trees, one tree: one run at a time, in the sheets' order.
    with ThreadPoolExecutor(max_workers=1 if spare is None else max(1, len(sheets))) as runs:
        for done in [runs.submit(one, path) for path in sheets]:
            done.result()
    return noticed
