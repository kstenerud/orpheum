# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: A revert's red from browser-mode tests stands only when they fail again with it still reverted,
# ABOUTME: run on their own: a test timed out under load is not the revert's red, and a red passes a file.
"""Whether a red from browser-mode tests is the revert's, by running its failed tests again.

A red passes a file. check-pr's second run took title.css's revert as red, a browser-mode test having
failed with it reverted, and the third found every test passing with it reverted, title.css and its
tests unchanged between them: the first red was a test that failed beside it, one run of the
stylesheets' own browser-mode tests going in each of four probe trees at once, each with Chromium and
Firefox, and kept, the file passed with nothing testing it, a whole hour of the next run spent
finding it (the owner, 2026-11-24, agreeing to the test-time stock-take's item 10). Ten Firefox
browser tests timed out at 30 s in one make check beside a revert-red run the same day, and all
passed alone.

So a red whose failed tests include browser-mode files has those files run again, the file still
reverted, one such run at a time (`ALONE`): they must fail again for the red to stand. One that
passes when run again says the first failure was not the revert's, and the probe goes on as if that
run had passed, printing what failed and why it was not taken (`not_the_reverts`). The run with
nothing reverted after it (revert_red_clean) still asks the other question, whether the tests can
pass here at all. A repeat that runs past its timeout settles nothing, as a run past its timeout
does not. A red that names no browser-mode test (a unit test's, a build's, the browser tier's
Playwright run) is left as it was: a unit test does not time out under load as a browser does.
"""

from __future__ import annotations

import functools
import subprocess
import threading
from collections.abc import Callable

import revert_red_commands as commands
from revert_red_known import failed_tests
from revert_red_run import run_whole_group
from revert_red_timings import Timings, timed

# The repeats go one at a time, so a repeat does not fail beside another as its first run did.
ALONE = threading.Lock()


def failed_browser_files(output: str, command: list[str] | None = None) -> list[str]:
    """The browser-mode test files a run's output says failed, as vitest names them from the web
    client's root (`apps/web/src/….browser.test.tsx`); of command's own runner alone, where it has
    one: a gate script's tests print vitest's failures as fixtures (revert_red_known.failed_tests)."""
    return [path for path in failed_tests(output, "", command).get("vitest", []) if ".browser.test." in path]


def repeats(output: str, rerun: Callable[[list[str]], subprocess.CompletedProcess[str]],
            command: list[str] | None = None) -> bool | None:
    """Whether the browser-mode tests a red run's output says failed fail again, run by rerun (in the
    probe's tree, the file still reverted); None when it names none. rerun's TimeoutExpired is the
    caller's, as a timed-out run is."""
    files = failed_browser_files(output, command)
    if not files:
        return None
    return rerun(commands.vitest("run", *files, browser=True)).returncode != 0


def did_not_repeat(timings: Timings, path: str, command: list[str], output: str, tree: str, timeout: int,
                   env: dict[str, str] | None = None) -> bool:
    """Whether path's red run, command, whose output this is, named browser-mode tests that passed
    (or ran past timeout) when run again in tree, path still reverted; said on the log when so."""
    try:
        again = repeats(output, lambda again_command: timed(
            timings, path, "vitest, its failed browser tests again", ALONE,
            functools.partial(run_whole_group, again_command, tree, timeout, env)), command)
    except subprocess.TimeoutExpired:
        again = False
    if again is not False:
        return False
    print(f"Not taken as {path}'s red: {', '.join(failed_browser_files(output, command))} failed with it "
          "reverted and did not fail when run again, still reverted, on their own.", flush=True)
    return True
