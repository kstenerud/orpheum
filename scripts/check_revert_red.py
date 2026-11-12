#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# Adapted from yoloAI's scripts/check_revert_red.py (MIT, same author).
# ABOUTME: CI gate. For each source file a branch changes, reverts it to the base
# ABOUTME: version and asserts some test goes red — the mechanical form of AGENTS.md
# ABOUTME: rule (D190), which otherwise depends on the author remembering to check.

"""Revert each changed file and require something to fail.

D190 and AGENTS.md say every behaviour change carries a test that fails when that
change is reverted, and the load-bearing word is *every*: count the behaviour changes,
count the tests that would go red, make the numbers match. yoloAI learned to actually
revert each one and watch it fail, because a test that merely covers the line looks
identical until you try.

**Orpheum's addition (D190):** a `Verified-By:` trailer must name a verification-queue
item — `VQ<n>` — that exists in `docs/contributors/verification-queue.md` and is ticked.
yoloAI's free-text trailer was never used, because nothing checked it.

That is a mechanical procedure described in prose and left to a human to run, which
is the shape of every rule yoloAI had to build a gate for. This runs it.

**What it proves and what it does not.** Red-on-revert shows a test is wired to the
code. It cannot show the code is wired to the *program* — reverting a line inside a
function nothing can reach still turns its test red, and six tests in yoloAI's
history certified exactly that kind of dead path. Rule 10's guidance about naming the
implementations that pass a capability guard is the answer to that one, and it stays human.

**Scope, deliberately narrow, for the reason `check_citation_provenance.py` gives
about itself: a noisy gate gets disabled.**

* Only files whose changes are claimed to be behavioral, read off the conventional
  commit type we already write. `feat`, `fix` and `perf` must go red; `refactor`,
  `docs`, `test`, `ci`, `build`, `chore` need not, because a refactor that changed
  behaviour is a mislabelled commit and that is a different gate's problem. A file
  touched by several commits requires red if *any* of them claims behaviour.
* Only kinds of file whose tests this can scope and run: Go, Python and web TypeScript (an added
  file by the tests importing it, revert_red_web), each widening only to what it can reach
  (revert_red_dependents); web CSS in the browser tier alone, by halves (stylesheets_together); a migration step with its checksum line
  and golden, and cache.db's schema, by internal/store's integration tier (revert_red_units.py). Every other
  file a feat/fix/perf commit changed is listed as UNCHECKED, by name and why, rather than
  passed — silence is what makes a green mean two things at once, and a `fix` that changed
  only a stylesheet once passed a gate that never looked at it (audit-2026-09-29 §6).
* A Go file whose change only removed declarations nothing uses is reported UNREACHABLE, not
  probed: reverting it restores dead code (revert_red_unreachable.py).
* A file whose part of a behaviour commit is a refactor can say so: a
  `Refactor-Only: <path> — <why>` trailer on any commit of the branch. Like `Verified-By:`,
  it is listed under "on the author's word" for review to weigh, never passed silently.

**A build failure counts as red, and is reported separately.** It proves the change
is load-bearing; it does not prove a test asserts its behaviour. Conflating the two
would let a compile-time dependency stand in for a missing test.

The distinction is weaker in Python: a test names what it imports, so removing a newly-added
function breaks collection first, and **an added Python function essentially only ever reports
`RED_BUILD`**, however well tested; Go reaches `RED_TEST` whenever the symbol survives the revert.

Everything happens in a throwaway git worktree. This tool checks out old versions of
files over a tree, and doing that in the user's working tree would put uncommitted
work one interrupted run away from being lost.
"""

from __future__ import annotations

import argparse
import functools
import json
import os
import queue
import signal
import subprocess
import sys
import threading
import time
from concurrent.futures import ThreadPoolExecutor
from dataclasses import dataclass
from pathlib import Path

import background_probe
import browser_map
import revert_red_known as known_red
import revert_red_reruns as reruns
from revert_red_queries import callers_of_change, users_of_change
from revert_red_report import OWED, failure_advice, report
from revert_red_parts import (BROWSER_DEFERRED, BROWSER_INLINE, BROWSER_ONLY, CSS_SOURCE, PARTS,  # noqa: F401 (tests reach them here)
                              browser_mode, is_in_part)
from revert_red_attempts import (BROWSER_MODE_RELATED, FIRST_RED_GO, FIRST_RED_VITEST, TS_SOURCE, Attempt,  # noqa: F401 (tests reach them here)
                                 browser_mode_tests, browser_tier, browser_tier_command, is_browser_run,
                                 compiled_only_elsewhere, other_platforms)
import revert_red_commands
from revert_red_commands import go_test, vitest, web_suite
from revert_red_run import RUNNER_MISSING, Stopped, is_build_failure, run_whole_group, stop_every_run  # noqa: F401 (tests reach RUNNER_MISSING here)
from revert_red_claims import VQ_FILE, refactor_only, unticked_items, verification_queue_problem, verified_out_of_suite  # noqa: F401 (tests reach VQ_FILE here)
from emitted_js import emits_the_same_between
from revert_red_cosmetic import GO_SOURCE, PY_SOURCE, is_cosmetic, is_package_clause_only
from revert_red_changes import (GO_GENERATED_QUERIES, GO_TEST_SUPPORT, SHELL_SOURCE, STORE_SCHEMA,  # noqa: F401 (tests reach them here)
                                Change, changed_sources, claims_behavior_change, commit_type, is_probed_kind,
                                is_python, is_test_file, types_touching)
from revert_red_dependents import (GO_SHIPPED_SOURCE_READERS, git_run as _git, go_dependents, go_module,  # noqa: F401 (tests reach rev_exists here)
                                   python_tests_naming, python_tests_reaching, rev_exists, toplevel,
                                   web_source_readers, web_stylesheet_readers)
from revert_red_locales import LOCALE_SOURCE, locale_runs
from revert_red_naming import naming_runs
from revert_red_tags import TYPECHECK, other_builds  # noqa: F401 (tests reach other_builds here)
from revert_red_stylesheets import own_browser_mode_reds, settle as settle_stylesheets
from revert_red_trees import hold_probe_trees, link_installed_packages, probe_trees, seed_type_builds  # noqa: F401 (tests reach them here)
from revert_red_units import MIGRATION_STEP, companions, query_source, restore, revert_companion
from revert_red_clean import clean_failure, clean_run, failed_pass_clean, probe_kind, runs_clean
from revert_red_unreachable import is_unreachable_removal
from revert_red_web import added_file_scope, direct_then_types
from revert_red_timings import EVERY_STYLESHEET, Timings, kind_of, timed
from revert_red_timings import keep as keep_timings, summary as timings_summary

GREEN_DETAIL = "every test still passes with this change reverted"
STORE_INTEGRATION = go_test("./internal/store/...", tags="integration")

# Every Python test is in scripts/tests (revert_red_changes.PY_TEST), so every Python source outside it is in their
# scope: the gate scripts, the Claude hooks in .claude/hooks, and ./dev. A list of roots left the
# hooks and ./dev unprobed (2026-10-01).
PY_TESTS_COMMAND = [sys.executable, str(Path(__file__).resolve().parent / "py_tests.py")]


@dataclass(frozen=True)
class Outcome:
    path: str
    verdict: str  # RED_TEST | RED_BUILD | GREEN | NO_RUNNER | TIMED_OUT | PROBE_BROKEN | BROWSER_OWED | CLAIMED | COSMETIC | UNREACHABLE | UNCHECKED
    detail: str = ""

    @property
    def is_failure(self) -> bool:
        return self.verdict in ("GREEN", "NO_RUNNER", "TIMED_OUT", "PROBE_BROKEN")


def unchecked_behavioral_files(base: str, head: str, probed: frozenset[str] = frozenset(),
                               include_tests: bool = True) -> list[tuple[str, str]]:
    """Files a feat/fix/perf commit changed that this gate did not probe, and why.

    Printed with every report, and alone when nothing was probed at all, to turn a
    reassuring silence into a named blind spot. Deliberately reports *reasons* rather than a
    count: "no behavioral source changes" was technically true of the run that
    missed a scheduling rewrite, and a number would have been just as true and
    just as useless. `probed` holds the files a probe undid, companions included.
    """
    out = _git("diff", "--name-status", f"{base}...{head}").stdout
    unchecked: list[tuple[str, str]] = []
    for line in out.splitlines():
        parts = line.split("\t")
        if len(parts) < 2:
            continue
        path = parts[-1]
        if path in probed or not claims_behavior_change(types_touching(base, head, path)):
            continue
        if is_test_file(path):
            if include_tests:
                unchecked.append((path, "classified as test code, so never asked to go red"))
        elif not is_probed_kind(path):
            unchecked.append((path, f"no probe for this kind of file ({Path(path).suffix or Path(path).name})"))
        elif scope_for(path) is None:
            unchecked.append((path, "no test scope covers it"))
    return unchecked


def scope_for(path: str) -> list[str] | None:
    """The narrowest test command covering `path`, or None when there is no scope.

    Package-scoped on purpose: the happy path is a fast red. Widening to the full
    suite happens only for a file that survives its own package, which is the
    suspicious case and the only one worth paying for.
    """
    if path.startswith(GO_GENERATED_QUERIES) or MIGRATION_STEP.match(path) or path == STORE_SCHEMA:
        return STORE_INTEGRATION
    if CSS_SOURCE.search(path):
        return browser_tier_command()
    if GO_SOURCE.search(path):
        return go_test("./" + str(Path(path).parent) + "/...")
    if is_python(path):
        # The test modules that import it or name it, all of them when none does (revert_red_dependents).
        return [*PY_TESTS_COMMAND, *(python_tests_reaching(toplevel(), path) or [])]
    if path == GENERATED_API_TYPES:
        # Types alone, which vitest strips: only the type check can notice it, and its importers'
        # tests were nearly the whole suite, 72 s (check-pr's timings, 2026-10-13).
        return TYPECHECK
    if TS_SOURCE.search(path):
        # The tests that import it, by vitest's import graph, as a Go file's package; none, and it
        # widens to the whole suite (wide_scope): all of it for each file took 54 s of check-pr.
        return [*VITEST_RELATED, str(Path(path).relative_to("web"))]
    if SHELL_SOURCE.search(path):
        # The Python tests that run it, which name it; none names it, and no test covers it.
        naming = python_tests_naming(toplevel(), path)
        return [*PY_TESTS_COMMAND, *naming] if naming else None
    return None


GENERATED_API_TYPES = "web/packages/sdk/src/gen/api.ts"
VITEST_RELATED = vitest("related")
WEB_SUITE = web_suite()
WEB_BUILD = ["npm", "--prefix", "web", "run", "build"]


def wide_scope(path: str) -> list[str] | None:
    """What a file that survived its own tests may still turn red: for Go, the packages that import
    its own and the tests that read every package's sources (revert_red_dependents); for TypeScript,
    whose importers' tests vitest's `related` already ran, the tests that read sources by path."""
    root = toplevel()
    if is_python(path):
        return PY_TESTS_COMMAND
    if GO_SOURCE.search(path):
        dependents = go_dependents(root, path)
        return go_test(*dependents) if dependents else None
    if TS_SOURCE.search(path):
        readers = web_source_readers(root)
        return [*VITEST_RUN, *readers] if readers else None
    return None


VITEST_RUN = vitest("run")
# The label of a run of the Go tests that read every shipped source (GO_SHIPPED_SOURCE_READERS).
SHIPPED_SOURCE_LABEL = ", found by the Go test that reads every shipped source"
# The wide run's label (wide_scope), which a claimed file's cheap probe leaves out.
WIDE_LABEL = ", found by the tests its revert can reach"


def attempts_for(change: Change, first: list[str], base: str = "", head: str = "") -> list[Attempt]:
    """The runs that may notice a file's revert, in order, the first that fails deciding: its own
    tests (`first`), the whole suite, the other builds, the browser-mode tests, the browser tier. A new stylesheet's revert
    deletes it, which breaks the build of whatever imports it with no browser to start; one nothing
    imports changes nothing. Generated query code's runs begin with the tests of the packages that
    call the queries its change touched (revert_red_queries), given base and head."""
    if change.status == "A" and CSS_SOURCE.search(change.path):
        return [Attempt(WEB_BUILD, ", found by building the web client", None, True)]
    if GO_SOURCE.search(change.path) and compiled_only_elsewhere(change.path):
        return other_platforms(change.path)
    attempts = [Attempt(first, "")]
    if change.path.startswith(GO_GENERATED_QUERIES) and base and head:
        callers = callers_of_change(toplevel(), base, head, change.path)
        if callers:
            attempts.insert(0, Attempt(go_test(*callers), ", found by the tests of the code that calls its changed queries"))
    elif GO_SOURCE.search(change.path) and base and head:
        dependents = go_dependents(toplevel(), change.path)
        # The tests naming what changed: its package's first, its dependents' after its run (revert_red_naming).
        naming_first, naming_later = naming_runs(toplevel(), base, head, change.path, dependents)
        attempts = [*naming_first, *attempts, *naming_later]
        # Before every dependent, those that use what the change touched: a red is nearly always there
        # (check-pr, 2026-10-21: a store file 37 s green in its own package, then 177 s of dependents).
        users = users_of_change(toplevel(), base, head, change.path, dependents)
        if users:
            attempts.append(Attempt(go_test(*users), ", found by the tests of the code that uses what it changed"))
    elif LOCALE_SOURCE.search(change.path) and base and head:
        # The catalogue's own test, then the tests rendering its changed keys, first (revert_red_locales).
        attempts = [*locale_runs(toplevel(), base, head, change.path), *attempts]
    elif TS_SOURCE.search(change.path) and change.status == "M" and change.path != GENERATED_API_TYPES:
        # The tests importing it directly, then the type check, seconds each, before every test that
        # reaches it: eight changed files' related runs, 110-140 s green each, came before a type check
        # red in 30 s (the test-regime audit of 2026-10-31, R3). An added file's first run is already
        # the tests importing it (added_file_scope).
        attempts = [*direct_then_types(toplevel(), change.path), *attempts]
    if TS_SOURCE.search(change.path) or SHELL_SOURCE.search(change.path):
        # The Go tests that read every shipped source as text, a few seconds, before the wide run.
        attempts.extend(Attempt(go_test(package, tests=[test]), SHIPPED_SOURCE_LABEL)
                        for package, test in GO_SHIPPED_SOURCE_READERS)
    wide = wide_scope(change.path)
    if wide is not None and wide != scope_for(change.path):
        attempts.append(Attempt(wide, WIDE_LABEL))
    # The browser tier's runs last, whatever made them first: a stylesheet's scope is one, and its
    # browser-mode tests, seconds, settle most sheets before it (the owner, 2026-10-18).
    later = [*other_builds(change.path), *browser_mode_tests(change.path), *browser_tier(change.path)]
    every = [*attempts, *(attempt for attempt in later if attempt not in attempts)]
    return sorted(every, key=is_browser_run)


# Whether a probe takes a red the record holds for the same revert over the same code
# (revert_red_known), and whether a claimed file takes only its cheap runs (probe); the nightly run,
# trusting no record, sets it off (--trust-no-record), and probes a claimed file whole.
TRUST_RECORD = True


def red_is_kept(began: float) -> bool:
    """Whether a red found by a probe that began at began is kept: not the background probe's while
    a session's run went beside it (gate_lock.ran_since), whose load can fake a red, never a green;
    check-pr probes the file again."""
    import gate_lock  # noqa: PLC0415 (the background probe's alone)

    return os.environ.get(background_probe.IN_BACKGROUND) != "1" or not gate_lock.ran_since(began)


def probe(tree: str, base: str, head: str, change: Change, timeout: int,
          browser: str = BROWSER_INLINE) -> Outcome:
    """One file's verdict: its revert probed, then the branch's claims if nothing went red.

    A file a valid claim covers passes red or green, so it takes only its cheap runs, which may still
    show it covered; the wide runs and the browser's decide nothing for it. They took 1640 s of one
    check-pr's probing (the test-regime audit of 2026-10-31, R1). The nightly run probes it whole."""
    claim = claimed(tree, base, head, change.path) if TRUST_RECORD else None
    if claim is not None and claim.verdict == "CLAIMED":
        cheap = probe_unclaimed(tree, base, head, change, timeout, browser, cheap=True)
        return claim if cheap.verdict in ("GREEN", "BROWSER_OWED") else cheap
    outcome = probe_unclaimed(tree, base, head, change, timeout, browser)
    if outcome.verdict == "BROWSER_OWED":
        # A claim passes it red or green, so a browser run decides nothing (client-main.tsx: 15 min, 2026-10-17).
        claim = claimed(tree, base, head, change.path)
        return claim if claim is not None and claim.verdict == "CLAIMED" else outcome
    if outcome.verdict != "GREEN":
        return outcome
    return claimed(tree, base, head, change.path) or outcome


def claimed(tree: str, base: str, head: str, path: str) -> Outcome | None:
    """What the branch's trailers say of a file whose revert turned nothing red, or None.

    A claim is consulted only after the probe: a file a test covers is covered whatever a
    trailer on its commit says. Read first, a `Verified-By:` for one file of a commit excused
    the commit's every file from probing, and an unticked VQ item then failed files that tests
    did cover (8fd8beb's on_stop.py and run_gate.py, 2026-10-01).
    """
    refactor = refactor_only(base, head, path)
    if refactor:
        return Outcome(path, "CLAIMED", f"Refactor-Only: {refactor}")
    claim = verified_out_of_suite(base, head, path)
    if claim:
        problem = verification_queue_problem(claim, tree)
        if problem:
            return Outcome(path, "GREEN", f"Verified-By: {claim} — {problem}")
        owed = unticked_items(claim, tree)
        return Outcome(path, "CLAIMED", f"Verified-By: {claim}" + (f"{OWED}{', '.join(owed)}" if owed else ""))
    return None


def probe_unclaimed(tree: str, base: str, head: str, change: Change, timeout: int,
                    browser: str = BROWSER_INLINE, cheap: bool = False) -> Outcome:
    """Revert one file in the worktree, with its unit's companions, run its tests, restore it; cheap,
    only the runs before the wide one and the browser's, a green of which is kept in no record."""
    if change.status == "M" and is_cosmetic(tree, base, head, change.path):
        return Outcome(change.path, "COSMETIC", "comment or formatting only; no behaviour to cover")
    if change.status == "A" and is_package_clause_only(tree, change.path):
        return Outcome(change.path, "COSMETIC", "a package clause and comments only; no behaviour to cover")
    if change.status in ("M", "D"):
        dropped = is_unreachable_removal(tree, base, head, change.path)
        if dropped:
            return Outcome(change.path, "UNREACHABLE", "only removed " + ", ".join(dropped)
                           + ", which nothing uses")
    scope = scope_for(change.path)
    if scope is None:
        return Outcome(change.path, "UNCHECKED", "no test scope covers this path")
    if change.status == "D":
        return Outcome(change.path, "UNCHECKED", "deleted; reverting would re-add it")

    unit = companions(change.path)
    began = time.time()
    revert = known_red.revert_key(tree, base, head, [change.path, *unit])
    if TRUST_RECORD and (seen := known_red.known(tree, head, revert, browser)) is not None:
        return Outcome(change.path, seen["verdict"], f"{seen['detail']}, as its probe at {seen['head'][:12]} "
                       "found; nothing that run read has changed since")
    first = added_file_scope(tree, change.path) if change.status == "A" and TS_SOURCE.search(change.path) else None
    try:
        if change.status == "A":
            (Path(tree) / change.path).unlink(missing_ok=True)
        else:
            restored = _git("checkout", base, "--", change.path, cwd=tree)
            if restored.returncode != 0:
                return Outcome(change.path, "UNCHECKED", "could not check out the base version")
        for companion in unit:
            revert_companion(tree, base, change.path, companion)

        unreached = False
        attempts = attempts_for(change, first or scope, base, head)
        attempts = [*(a for a in known_red.caught_before(change.path, tree) if a not in attempts), *attempts]
        if TS_SOURCE.search(change.path) and emits_the_same_between(tree, base, head, change.path):
            # The browser runs the same JavaScript either way, so no browser run can notice it (emitted_js.py).
            attempts = [a for a in attempts if not is_browser_run(a)]
        if cheap:
            attempts = [a for a in attempts if not is_browser_run(a) and a.label != WIDE_LABEL]
        deferred = [a for a in attempts if is_browser_run(a)] if browser == BROWSER_DEFERRED else []
        if browser == BROWSER_ONLY and not CSS_SOURCE.search(change.path):
            attempts = [a for a in attempts if is_browser_run(a)]
        ran: list[list[str]] = []  # every run made, which a green's record holds
        for attempt in (a for a in attempts if a not in deferred):
            # Python caches a module by its size and mtime; a probe that restores a file to the same
            # size within a second would run the stale cache, so the probes write none.
            env = {**os.environ, "PYTHONDONTWRITEBYTECODE": "1", **(attempt.env or {})}
            if is_browser_run(attempt) and CSS_SOURCE.search(change.path):
                reaching = browser_map.tests_reaching(tree, base, head, [change.path])
                if reaching == []:
                    unreached = True
                    continue
                if reaching is not None:
                    env[browser_map.ONLY_THESE_TESTS] = json.dumps(reaching)
            # One browser tier at a time: each starts a server and three browsers.
            try:
                done = timed(TIMINGS, change.path, kind_of(attempt.command, attempt.label, attempt.env),
                             BROWSER_TIER_LOCK if is_browser_run(attempt) else None,
                             functools.partial(run_whole_group, attempt.command, tree, timeout, env))
                ran.append(attempt.command)
            except subprocess.TimeoutExpired:
                # Neither red nor green: a slow run that would have passed is not coverage. Escaping, it
                # ended the whole probe pool and 27 minutes of check-pr named no file (2026-10-17). A
                # browser test that hangs fails by its own timeout first, and the run stops there
                # (browser_tier_command).
                return Outcome(change.path, "TIMED_OUT", f"the tests ran past {timeout} s with it "
                               f"reverted{attempt.label}, so the probe could not tell")
            if done.returncode == 0:
                continue
            if done.returncode == RUNNER_MISSING:
                return Outcome(change.path, "NO_RUNNER", f"the tests could not start: {attempt.command[0]} "
                               "found no runner, so nothing was checked")
            for path in (change.path, *unit):
                restore(tree, head, path)
            kind = probe_kind(attempt.command, attempt.label, attempt.env)
            combined = done.stdout + done.stderr
            failed_runs = [] if attempt.compile_only or is_build_failure(combined) else \
                known_red.runs_of(known_red.failed_tests(combined, go_module(toplevel()), attempt.command))
            again, said = failed_pass_clean(failed_runs, lambda command: timed(
                TIMINGS, change.path, f"{kind_of(command, '', env)}, nothing reverted", None,
                functools.partial(run_whole_group, command, tree, timeout, env)))
            if again is False:
                return Outcome(change.path, "PROBE_BROKEN", f"the tests that failed{attempt.label} fail here with "
                               f"nothing reverted too, so its red says nothing; with nothing reverted:\n{said}")
            confirm, confirm_kind = clean_run(attempt.command, kind, is_build_failure(combined))
            clean = functools.partial(run_whole_group, confirm, tree, timeout, env)
            if again is None and not runs_clean(tree, confirm_kind, lambda: timed(
                    TIMINGS, change.path, f"{confirm_kind}, nothing reverted", None, clean), head):
                failed = clean_failure(tree, confirm_kind)
                return Outcome(change.path, "PROBE_BROKEN", f"its probe{attempt.label} fails here with nothing "
                               "reverted too, so its red says nothing" + (f"; with nothing reverted:\n{failed}" if failed else ""))
            if attempt.compile_only or is_build_failure(combined):
                red = Outcome(change.path, "RED_BUILD", f"reverting it breaks the build{attempt.label}")
            else:
                red = Outcome(change.path, "RED_TEST", f"a test fails when it is reverted{attempt.label}")
            if red_is_kept(began):
                known_red.remember(tree, head, revert, attempt.command, red.verdict, red.detail, output=combined)
                known_red.remember_failed(change.path, combined, go_module(toplevel()), attempt.command)
            return red
        if deferred:
            return Outcome(change.path, "BROWSER_OWED", "no cheap run turned red; its browser runs come in "
                           "the last tier, after check-pr's own browser run")
        if not unreached and not cheap:
            known_red.remember(tree, head, revert, ran, "GREEN", GREEN_DETAIL, browser)
        return Outcome(change.path, "GREEN", browser_map.NO_TEST_REACHES if unreached else GREEN_DETAIL)
    finally:
        restore(tree, head, change.path)
        for companion in unit:
            restore(tree, head, companion)


def is_stylesheet(change: Change) -> bool:
    """A changed stylesheet, which the one browser run with every one reverted settles. A new one is
    probed alone, by a build: deleted, it broke that run's build, and every sheet then took a
    browser run of its own, one at a time, 22 minutes of check-pr (its timings, 2026-10-13)."""
    return bool(CSS_SOURCE.search(change.path)) and change.status == "M"


def stylesheets_together(tree: str, base: str, head: str, changes: list[Change], timeout: int,
                         command: list[str] | None = None,
                         browser_mode: list[str] | None = None,
                         spare: queue.Queue[str] | None = None,
                         readers: list[str] | None = None) -> dict[str, Outcome]:
    """The stylesheets' verdicts, settled by halves (revert_red_stylesheets): every one reverted at
    once, run once, and each half of a failing group again, down to the sheets a test noticed.

    A run is first the browser-mode tests that import the reverted sheets, as a sheet's own probe
    runs them, and only when they stay green the browser tier: the tier alone missed app-shell.css
    and tokens.css, each red by the browser-mode tests alone (check-pr, 2026-10-26). The gate's own
    run uses BROWSER_MODE_RELATED; a test that hands its own tier command hands its own browser-mode
    one, or runs none. Before either, the tests that read stylesheets as text (`readers`; the gate's
    own run, revert_red_dependents.web_stylesheet_readers), which neither reaches.

    A sheet alone costs a whole browser tier, and a skin changes dozens: one run per sheet took a
    skin's branch three quarters of an hour (2026-10-06). A passing run settles every sheet it
    reverted, as its own run would (D279), never laxer: a sheet another's revert hid is flagged, not
    passed. A sheet whose run alone fails is red; a timed-out run leaves its sheets to their own.
    """
    sheets = [change for change in changes if is_stylesheet(change)]
    if len(sheets) < 2:
        return {}
    # What an earlier run settled, over the same code, is taken (revert_red_known).
    reads = known_red.stylesheet_runs_read(tree, head)
    # Keyed by which entries there are too: one added since is read, and in no tree kept.
    keys = {change.path: f"{known_red.revert_key(tree, base, head, [change.path])}|{','.join(reads)}"
            for change in sheets}
    taken: dict[str, Outcome] = {}
    for change in sheets:
        seen = known_red.known(tree, head, keys[change.path], known_red.STYLESHEETS_TOGETHER) if TRUST_RECORD else None
        if seen is not None:
            taken[change.path] = settled_outcome(tree, base, head, change.path, seen["verdict"], seen["detail"],
                                                 f", as the stylesheets' runs at {seen['head'][:12]} found; "
                                                 "nothing they read has changed since")
    sheets = [change for change in sheets if change.path not in taken]
    if len(sheets) < 2:
        return taken

    unreached: set[str] = set()
    related = browser_mode if command is not None else (browser_mode or BROWSER_MODE_RELATED)
    if readers is None and command is None:
        reading = web_stylesheet_readers(tree)
        readers = [*VITEST_RUN, *reading] if reading else None
    # Each sheet's own browser-mode tests first, alone, seconds each: a red settles it, and only the
    # rest go into the halving, over the browser tier alone. The halving's every level ran every
    # browser-mode test of its group, sixteen runs one after another, about 1500 s of one check-pr
    # (the test-regime audit of 2026-10-31, R2).
    # They run side by side, in whichever probe trees come free (spare).
    noticed_alone = own_browser_mode_reds(tree, base, head, [change.path for change in sheets], related, timeout,
                                          TIMINGS, spare, readers)

    def run_reverted(group: list[str]) -> subprocess.CompletedProcess[str] | None:
        label = EVERY_STYLESHEET if len(group) == len(sheets) else f"({len(group)} stylesheets at once)"
        try:
            for path in group:
                if _git("checkout", base, "--", path, cwd=tree).returncode != 0:
                    return None
            reaching = browser_map.tests_reaching(tree, base, head, group)
            if reaching == []:
                unreached.update(group)
                return subprocess.CompletedProcess(group, 0, "", "")
            only = {} if reaching is None else {browser_map.ONLY_THESE_TESTS: json.dumps(reaching)}
            asked = time.monotonic()
            done = timed(TIMINGS, label, "browser tier", BROWSER_TIER_LOCK,
                         functools.partial(run_whole_group, command or browser_tier_command(), tree, timeout,
                                           {**os.environ, "PYTHONDONTWRITEBYTECODE": "1", **only}))
            TIMINGS.note_probe(label, time.monotonic() - asked)
            return done
        except subprocess.TimeoutExpired:
            return None
        finally:
            for path in group:
                restore(tree, head, path)

    halving = settle_stylesheets([change.path for change in sheets if change.path not in noticed_alone],
                                 run_reverted)
    halving.noticed.update(noticed_alone)
    for failure in halving.failures:
        print(f"A browser run failed {failure}\n")
    settled: dict[str, Outcome] = dict(taken)
    ran = [[*(command or browser_tier_command())], *([related] if related else [])]
    for path in halving.passed:
        if path in unreached and not is_cosmetic(tree, base, head, path):
            claim = claimed(tree, base, head, path) or Outcome(path, "GREEN", browser_map.NO_TEST_REACHES)
            settled[path] = Outcome(path, claim.verdict, claim.detail if claim.verdict == "GREEN" else
                                    f"{claim.detail} ({browser_map.NO_TEST_REACHES})")
            continue
        known_red.remember(tree, head, keys[path], ran, "GREEN", GREEN_DETAIL, known_red.STYLESHEETS_TOGETHER,
                           reads=reads)
        settled[path] = settled_outcome(tree, base, head, path, "GREEN", GREEN_DETAIL, "")
    for path, output in halving.noticed.items():
        verdict = "RED_BUILD" if is_build_failure(output) else "RED_TEST"
        detail = ("a test fails when it is reverted, found by the browser tier" if verdict == "RED_TEST"
                  else "reverting it breaks the build, found by the browser tier")
        known_red.remember(tree, head, keys[path], ran, verdict, detail, known_red.STYLESHEETS_TOGETHER, reads=reads)
        settled[path] = Outcome(path, verdict, detail)
    return settled


def settled_outcome(tree: str, base: str, head: str, path: str, verdict: str, detail: str, source: str) -> Outcome:
    """A stylesheet's row from its runs' verdict: a red as it is; a pass cosmetic, claimed or green as
    the batch says now, a claim being checked after the runs as ever (D279)."""
    if verdict != "GREEN":
        return Outcome(path, verdict, detail + source)
    if is_cosmetic(tree, base, head, path):
        return Outcome(path, "COSMETIC", "comment or formatting only; no behaviour to cover")
    claim = claimed(tree, base, head, path) or Outcome(path, "GREEN", GREEN_DETAIL)
    return Outcome(path, claim.verdict, claim.detail + " (a browser run with it and others reverted turned "
                   "nothing red" + (source or "") + ")")


BROWSER_TIER_LOCK = threading.Lock()
TIMINGS = Timings()  # every probe's runs and how long they took (revert_red_timings)


def why_not_probed(base: str, head: str, path: str, changes: list[Change]) -> str:
    """Why a path a targeted run named is not probed: '' when it is owed a probe."""
    if any(c.path == path for c in changes):
        types = types_touching(base, head, path)
        if claims_behavior_change(types):
            return ""
        return f"exempt: only {', '.join(sorted(t or '?' for t in types))} commits touched it"
    if is_test_file(path):
        return "test code, never asked to go red"
    if not is_probed_kind(path):
        return f"no probe for this kind of file ({Path(path).suffix or Path(path).name})"
    return f"not changed in {base[:12]}..{head[:12]}"


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--base", required=True, help="ref the branch forked from")
    ap.add_argument("--head", default="HEAD")
    ap.add_argument("--timeout", type=int, default=900, help="seconds per test run")
    ap.add_argument("--jobs", type=int, default=0, help="files probed at once (default: half the CPUs)")
    only = ap.add_mutually_exclusive_group()
    only.add_argument("--only", nargs="+", metavar="PATH", help="probe only these paths, still judged "
                      "against --base..--head; a full run is still required before the branch is done")
    only.add_argument("--rerun-failed", action="store_true", help="probe only the paths the last run "
                      "against the same --base failed on (the head may have moved)")
    ap.add_argument("--part", choices=PARTS, default="all", help="probe only the code with its cheap runs "
                    "(Go, TypeScript, Python: a package's tests each), or only what takes the browser tier "
                    "(the stylesheets, and the code the code part left owing it), as check-pr's tiers do; "
                    "the record of failures keeps the other part's")
    ap.add_argument("--unrecorded", action="store_true", help="keep no record of the failures: a check of "
                    "one commit (make check-commit-red) leaves the batch's record to check-pr")
    ap.add_argument("--trust-no-record", action="store_true", help="probe every file, taking no red an "
                    "earlier probe of the same revert over the same code found (make check-everything)")
    args = ap.parse_args()
    if args.trust_no_record:
        global TRUST_RECORD  # noqa: PLW0603 (the run's one setting, read by each probe)
        TRUST_RECORD = False
    if args.unrecorded:
        reruns.begin = lambda _base: None  # type: ignore[assignment]
        reruns.finish = lambda *_args, **_kwargs: None  # type: ignore[assignment]
        reruns.owe_browser = lambda *_args: None  # type: ignore[assignment]

    for name, ref in (("--base", args.base), ("--head", args.head)):
        if not rev_exists(ref):
            print(f"{name} {ref!r} is not a commit this repository can resolve.")
            return 1
    base = _git("rev-parse", f"{args.base}^{{commit}}").stdout.strip()
    head = _git("rev-parse", f"{args.head}^{{commit}}").stdout.strip()

    wanted: list[str] | None = None
    if args.rerun_failed:
        wanted, ran_on, problem = reruns.recorded_failures(base)
        if problem:
            print(problem)
            return 1
        if not wanted:
            print(f"The last revert-red run against {base[:12]} failed on nothing; nothing to re-run.")
            return 0
        print(f"Re-running the {len(wanted)} file(s) the last run (at {ran_on[:12]}) failed on.")
    elif args.only:
        wanted = [p.removeprefix("./") for p in args.only]

    owed_browser = reruns.owed_browser(base) if args.part == "browser" else frozenset()
    changes = [c for c in changed_sources(base, head) if is_in_part(c.path, args.part, owed_browser)]
    owed = [c for c in changes if claims_behavior_change(types_touching(base, head, c.path))]
    if wanted is not None:
        wanted = [path for path in wanted if is_in_part(path, args.part, owed_browser)]
    exempt = len(changes) - len(owed)
    unknown = False
    if wanted is not None:
        owed = [c for c in owed if c.path in wanted]
        for path in wanted:
            why = why_not_probed(base, head, path, changes)
            if why:
                print(f"Not probed: {path}  — {why}")
                unknown = unknown or (args.only is not None and why.startswith("not changed"))
        if not owed:
            reruns.finish(base, head, wanted, [], targeted=True)
            return 1 if unknown else 0
    elif not owed:
        reruns.finish(base, head, [c.path for c in changes], [], targeted=args.part != "all")
        # A behavioral commit whose every file fell out of scope is not a pass; it
        # is the gate reporting that it checked nothing, and it must say which
        # files and why. Printing "no behavioral source changes" over a `fix` that
        # rewrote scheduling logic is exactly how yoloAI's smoke harness went unchecked
        # for as long as it did — the message was true of what the gate could see
        # and false about the commit.
        unchecked = unchecked_behavioral_files(base, head)
        if unchecked:
            print(
                f"Nothing was checked in {args.base}..{args.head}, but a feat/fix/perf commit "
                f"touched {len(unchecked)} file(s) this gate cannot scope:"
            )
            for path, why in unchecked:
                print(f"  {path}  — {why}")
            print(
                "\nThat is a blind spot, not a pass. Either the commit type is wrong, or the\n"
                "behaviour needs a test somewhere this gate can run, or it is genuinely\n"
                "unreachable here and belongs behind a Verified-By: trailer naming what you ran."
            )
            return 0
        print(f"No behavioral source changes in {args.base}..{args.head} ({exempt} exempt by type).")
        return 0

    jobs = args.jobs or max(1, (os.cpu_count() or 2) // 2)
    # Each probe's runs within its share of the CPUs, so the probes at once fit the machine.
    revert_red_commands.CPU_SHARE = max(1, (os.cpu_count() or 2) // min(jobs, max(1, len(owed))))
    # Stopped from outside (a SIGTERM: the background probe's stop, quiet_gate's fail-fast), every
    # test run going ends with it, and nothing it was running is recorded (revert_red_run).
    previous = signal.signal(signal.SIGTERM, lambda _signal, _frame: stop_every_run())
    try:
        return probe_owed(args, base, head, owed, exempt, jobs, wanted, unknown)
    finally:
        signal.signal(signal.SIGTERM, previous)


def probe_owed(args: argparse.Namespace, base: str, head: str, owed: list[Change], exempt: int, jobs: int,
               wanted: list[str] | None, unknown: bool) -> int:
    """The probes of main, once it knows what is owed: the background probe stopped, the trees held."""
    # A background probe of the batch (background_probe.py) gives the trees up to a run of the gate's
    # own; what it probed is kept, and this run takes it. The time each wait took is reported.
    before: dict[str, float] = {}
    asked = time.monotonic()
    background_probe.stop()
    before["waiting for the background probe to end"] = time.monotonic() - asked
    try:
        asked = time.monotonic()
        with hold_probe_trees():
            before["waiting for another run's probe trees"] = time.monotonic() - asked
            reruns.begin(base)
            status = probe_all(base, head, owed, exempt, min(jobs, len(owed)), args.timeout, wanted,
                               args.part != "all", browser_mode(args.part), before)
    except Stopped:
        print("revert-red: stopped, as asked; nothing it was running is recorded.", flush=True)
        return 143
    if wanted is not None:
        print("\nA targeted run: the full `make check-revert-red` (or `make check-pr`) is still "
              "required before the branch is done.")
    return 1 if unknown else status


def unprobed(base: str, head: str, owed: list[Change]) -> list[Outcome]:
    """The UNCHECKED rows for every file a behaviour commit changed that no probe undid."""
    probed = frozenset(p for c in owed for p in (c.path, *companions(c.path)))
    return [Outcome(path, "UNCHECKED", why)
            for path, why in unchecked_behavioral_files(base, head, probed, include_tests=False)]


def probe_all(base: str, head: str, owed: list[Change], exempt: int, jobs: int, timeout: int,
              wanted: list[str] | None, is_one_part: bool = False, browser: str = BROWSER_INLINE,
              before: dict[str, float] | None = None) -> int:
    asked = time.monotonic()
    try:
        trees = probe_trees(head, jobs)
    except RuntimeError as problem:
        print(f"Could not prepare the worktrees to test in:\n{problem}")
        return 1
    free: queue.Queue[str] = queue.Queue()
    for tree in trees:
        free.put(tree)

    def probe_in_a_free_tree(change: Change) -> Outcome:
        tree = free.get()
        started = time.monotonic()
        try:
            return probe(tree, base, head, change, timeout, browser)
        finally:
            TIMINGS.note_probe(change.path, time.monotonic() - started)
            TIMINGS.note_tree(tree, time.monotonic() - started)
            free.put(tree)

    def stylesheets_in_a_free_tree() -> dict[str, Outcome]:
        tree = free.get()
        started = time.monotonic()
        try:
            return stylesheets_together(tree, base, head, owed, timeout, spare=free)
        finally:
            TIMINGS.note_tree(tree, time.monotonic() - started)
            free.put(tree)

    TIMINGS.clear()
    for what, seconds in (before or {}).items():
        TIMINGS.note_before(what, seconds)
    TIMINGS.note_before("preparing the worktrees", time.monotonic() - asked)
    started = time.monotonic()
    with ThreadPoolExecutor(max_workers=len(trees)) as pool:
        together = pool.submit(stylesheets_in_a_free_tree)
        outcomes = list(pool.map(probe_in_a_free_tree, [c for c in owed if not is_stylesheet(c)]))
        settled = together.result()
        alone = [c for c in owed if is_stylesheet(c) and c.path not in settled]
        outcomes += list(pool.map(probe_in_a_free_tree, alone)) + list(settled.values())

    # A targeted run reports on the paths it was given; their skips were printed already.
    if wanted is None:
        outcomes += unprobed(base, head, owed)
    reruns.finish(base, head, wanted or [c.path for c in owed], [o.path for o in outcomes if o.is_failure],
                  targeted=wanted is not None or is_one_part)
    if browser == BROWSER_DEFERRED:
        reruns.owe_browser(base, [c.path for c in owed], [o.path for o in outcomes if o.verdict == "BROWSER_OWED"])
    if wanted is None:
        print(f"Reverted {len(owed)} behavioral source change(s); {exempt} exempt by commit type.")
    else:
        print(f"Reverted {len(owed)} of the {len(wanted)} file(s) named.")
    print(report(outcomes))
    wall_seconds = time.monotonic() - started
    keep_timings(TIMINGS, base, head, wall_seconds, len(trees))
    print(timings_summary(TIMINGS, wall_seconds, len(trees)))

    failures = [o for o in outcomes if o.is_failure]
    if not failures:
        return 0
    print(failure_advice(failures))
    return 1


if __name__ == "__main__":
    sys.exit(main())
