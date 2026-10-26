# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: How long revert-red's probes took: each test run of each file, by kind, kept beside its
# ABOUTME: failures and summed in its report, so an overrun names what grew without a second run.
"""How long each of revert-red's probes took.

check-pr's steps are timed (check_pr_steps.py), and revert-red was 2222 s of a 41-minute run with
nothing saying which of its 164 files, or which of their runs, took the time (the owner, 2026-10-13:
"Do the tests produce enough logs that we can tell how long each part took without having to run
them again?"). Each probe now notes every run it makes: what kind of run, how long it took, how long
it waited for the one browser tier at a time, and whether it passed. The run keeps them at
`<git-common-dir>/revert-red/last-timings` and its report ends with the time by kind of run and the
slowest files.
"""

from __future__ import annotations

import contextlib
import subprocess
import threading
import time
from dataclasses import dataclass, field
from typing import Any, Callable

import last_run
import revert_red_commands as commands
from revert_red_locales import CATALOGUE_TEST

GATE = "revert-red"
NAME = "last-timings"

# How many of the slowest files the report names.
SLOWEST_SHOWN = 10

# The name the timings give the one browser run with every stylesheet reverted.
EVERY_STYLESHEET = "(every stylesheet at once)"


@dataclass(frozen=True)
class Run:
    """One run a probe made: its kind, its seconds, the seconds it waited to start, and whether it
    passed."""

    kind: str
    seconds: float
    waited: float
    passed: bool


@dataclass
class Timings:
    """Every probe's runs, by path, noted from the probing threads."""

    runs: dict[str, list[Run]] = field(default_factory=dict)
    probe_seconds: dict[str, float] = field(default_factory=dict)
    lock: threading.Lock = field(default_factory=threading.Lock)

    def clear(self) -> None:
        """Forgets every run: a new run of the gate starts with none."""
        with self.lock:
            self.runs.clear()
            self.probe_seconds.clear()

    def note_run(self, path: str, run: Run) -> None:
        with self.lock:
            self.runs.setdefault(path, []).append(run)

    def note_probe(self, path: str, seconds: float) -> None:
        """The whole probe of a file, its checkout, checks and runs, from start to verdict."""
        with self.lock:
            self.probe_seconds[path] = self.probe_seconds.get(path, 0.0) + seconds


def timed(timings: Timings, path: str, kind: str, lock: threading.Lock | None,
          run: Callable[[], subprocess.CompletedProcess[str]]) -> subprocess.CompletedProcess[str]:
    """Runs `run` for the probe of `path`, holding `lock` when given, and notes how long it waited
    for the lock and how long it ran."""
    asked = time.monotonic()
    with lock if lock is not None else contextlib.nullcontext():
        started = time.monotonic()
        done = run()
    timings.note_run(path, Run(kind, time.monotonic() - started, started - asked, done.returncode == 0))
    return done


def kind_of(command: list[str], label: str, env: dict[str, str] | None) -> str:
    """A run's kind, as the report sums them: the browser tier, the store's integration tier, a Go
    package or the whole Go suite, vitest's related tests or its whole suite, the type check, a
    compile for another system, or the Python tests."""
    text = " ".join(command)
    if "browser-tier" in text:
        # ORPHEUM_E2E_ONLY, browser_map.ONLY_THESE_TESTS: the tests the browser map says load it.
        return "browser tier, the tests that load it" if env and "ORPHEUM_E2E_ONLY" in env else "browser tier"
    if "typecheck" in text:
        return "web type check"
    if "vitest" in text and "related" in text:
        return "vitest related"
    if "vitest" in text and CATALOGUE_TEST in text:
        return "vitest, the catalogue's own test"
    if "vitest" in text:
        return "vitest, an added file's importers"
    if command[-2:] == ["run", "build"]:
        return "web build"
    if command[:3] == ["npm", "--prefix", "web"]:
        return "vitest whole suite"
    if env and ("GOOS" in env or "GOARCH" in env):
        return "go compile, another system"
    if commands.is_go_test(command):
        if "integration" in command:
            return "go test, store integration"
        if "-tags" in command:
            return "go test, extra tags"
        if command[-1] == "./...":
            return "go test, whole suite"
        if commands.is_narrowed(command):
            return "go test, the tests that name it"
        packages = [part for part in command if part.startswith("./")]
        return "go test, its dependents" if len(packages) > 1 else "go test, package"
    if "py_tests.py" in text:
        return "python tests" if command[-1].endswith("py_tests.py") else "python tests, those reaching it"
    return (command[0] if command else "?") + (label and f" ({label.strip(', ')})")


def summary(timings: Timings, wall_seconds: float, jobs: int) -> str:
    """The time by kind of run, and the slowest files with their runs, for the report's end."""
    by_kind: dict[str, list[float]] = {}
    waited = 0.0
    for runs in timings.runs.values():
        for run in runs:
            by_kind.setdefault(run.kind, []).append(run.seconds)
            waited += run.waited
    lines = [f"\nHow long it took: {wall_seconds:.0f} s on {jobs} worktree(s), "
             f"{sum(timings.probe_seconds.values()):.0f} s of probing in all, "
             f"{waited:.0f} s of it waiting for the browser tier:"]
    for kind, seconds in sorted(by_kind.items(), key=lambda item: -sum(item[1])):
        lines.append(f"  {sum(seconds):>6.0f} s  {len(seconds):>3} run(s)  {kind}")
    slowest = sorted(timings.probe_seconds.items(), key=lambda item: -item[1])[:SLOWEST_SHOWN]
    if slowest:
        lines.append(f"The {len(slowest)} slowest files:")
    for path, seconds in slowest:
        runs = ", ".join(f"{run.kind} {run.seconds:.0f} s" + ("" if run.passed else " (red)")
                         + (f", waited {run.waited:.0f} s" if run.waited >= 1 else "")
                         for run in timings.runs.get(path, []))
        lines.append(f"  {seconds:>6.0f} s  {path}  — {runs or 'no test run'}")
    return "\n".join(lines)


def keep(timings: Timings, base: str, head: str, wall_seconds: float, jobs: int) -> None:
    """Keeps the run's timings, replacing the last run's, for reading after it."""
    def record(_old: dict[str, Any] | None) -> dict[str, Any]:
        paths = sorted(set(timings.probe_seconds) | set(timings.runs))
        return {
            "base": base, "head": head, "wallSeconds": round(wall_seconds, 1), "jobs": jobs,
            "files": [
                {"path": path, "seconds": round(timings.probe_seconds.get(path, 0.0), 1),
                 "runs": [{"kind": run.kind, "seconds": round(run.seconds, 1), "waited": round(run.waited, 1),
                           "passed": run.passed} for run in timings.runs.get(path, [])]}
                for path in paths
            ],
        }
    last_run.update(GATE, NAME, record)
