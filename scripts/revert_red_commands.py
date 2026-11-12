# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: The test runs revert-red's probes make, built and recognised in one place: Go's tests and
# ABOUTME: vitest's, each stopped at its first failure, and each kept within its probe's share of the CPUs.
"""Revert-red's test commands, one way each (AGENTS.md rule 11).

Every `go test` and vitest command a probe runs is built here, and only here is a command recognised
as one: the CPU share (`shared`) and every reader of a command go by the same definitions, so a new
kind of run cannot miss its share by being spelled differently. They were built in fourteen places
(2026-10-26).
"""

from __future__ import annotations

import os

# A probe asks whether any test goes red, and the first red settles it: each run it makes stops at its
# first failure. Run to its end, a red probe of a locale file took 34 s, stopped at its first 6 s
# (check-pr's revert-red, 284 s, 2026-10-20). A green run still runs every test, so nothing it
# passes is passed on less. The browser tier's is FIRST_FAILURE_ENDS_IT.
FIRST_RED_GO = "-failfast"
FIRST_RED_VITEST = "--bail=1"

GO_TEST = ["go", "test"]
VITEST = ["npx", "--prefix", "web", "vitest"]
WEB_SCRIPT = ["npm", "--prefix", "web", "run"]


ONLY_TESTS = "-run"


def go_test(*packages: str, tags: str | None = None, tests: list[str] | None = None) -> list[str]:
    """Go's tests of packages, stopped at the first failure, built with tags when given; only the
    top-level tests named when tests are given (with their subtests), every one otherwise."""
    only = [ONLY_TESTS, "^(" + "|".join(sorted(set(tests))) + ")$"] if tests else []
    return [*GO_TEST, FIRST_RED_GO, *(["-tags", tags] if tags else []), *only, *packages]


def is_narrowed(command: list[str]) -> bool:
    """Whether a Go test run runs only some tests (go_test's tests)."""
    return is_go_test(command) and ONLY_TESTS in command


def vitest(mode: str, *files: str, browser: bool = False, first_red: bool = True) -> list[str]:
    """vitest's `related` (the tests importing files, by its import graph) or `run` (the test files
    given), in the web client; in the browser-mode configuration when browser; stopped at the first
    failure unless first_red is False (the spot check runs to its end, to list every failure)."""
    command = [*VITEST, mode, *(["--run", "--passWithNoTests"] if mode == "related" else []), "--root", "web"]
    if browser:
        command += ["--config", "vitest.browser.config.ts"]
    return [*command, *([FIRST_RED_VITEST] if first_red else []), *files]


def web_suite() -> list[str]:
    """The web client's whole vitest suite, through its own script, stopped at the first failure."""
    return [*WEB_SCRIPT, "test", "--", FIRST_RED_VITEST]


def is_go_test(command: list[str]) -> bool:
    return command[:2] == GO_TEST


def compiled(command: list[str]) -> list[str] | None:
    """A Go test run that builds every test binary command would and runs none, `true` run in place
    of each (`-exec`, which overrides the GOFLAGS one `make setup` adds): what a clean tree must pass
    for a build red to stand. None for any other run."""
    if not is_go_test(command):
        return None
    return [*GO_TEST, "-exec", "true", *command[len(GO_TEST):]]


def is_vitest(command: list[str]) -> bool:
    return command[: len(VITEST)] == VITEST or command[: len(WEB_SCRIPT) + 1] == [*WEB_SCRIPT, "test"]


# The CPUs each probe's run may use, set by check_revert_red for the probes it runs at once; None
# leaves a run its tools' own width. Probes at once, each at every CPU's width, oversubscribed the
# machine about tenfold: on 8 CPUs, load 82, and two dozen probes failed with nothing reverted, while
# the same suites, three at once, passed (check-pr, 2026-10-26). A share keeps them within the CPUs.
CPU_SHARE: int | None = None


def shared(command: list[str], env: dict[str, str] | None,
           share: int | None) -> tuple[list[str], dict[str, str] | None]:
    """The command and environment a run takes within share CPUs: Go's tests build and run `-p`
    packages at once with GOMAXPROCS threads each, vitest runs `--maxWorkers` files at once. Any
    other command, and any run without a share, is unchanged."""
    if share is None:
        return command, env
    if is_go_test(command):
        within = dict(os.environ if env is None else env)
        within["GOMAXPROCS"] = str(share)
        return [*GO_TEST, "-p", str(share), *command[len(GO_TEST):]], within
    if is_vitest(command):
        return [*command, f"--maxWorkers={share}"], env
    return command, env
