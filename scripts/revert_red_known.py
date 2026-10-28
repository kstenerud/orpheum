# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Revert-red's record of the reverts it has seen turn a test red, by the file's two versions and
# ABOUTME: what the red run read, so a later probe of the same revert over the same code takes the red.
"""Reverts known red, or known green (check_revert_red.probe_unclaimed).

check-commit-red probes each commit's files as it is made, and check-pr probed them all again: a
batch of 24 commits took 754 s, most of it repeating reds already found (2026-10-21). A probe
that goes red records, at `<git-common-dir>/revert-red/known-red`, the file's path and the blobs
its revert went between, those of the files undone with it, and the git tree of every directory
the red run read. A later probe of the same revert, the same blobs, where every one of those
trees is still the same at its head, takes the red without running: the code that turned red
and the tests that saw it are unchanged.

**A green is kept too** (the caching audit's finding 1, 2026-10-28), keyed by every run the probe
made and what each read, and for the part it was probed in (code, browser, all): a check-pr-failed
re-probed 9 files for 602 s and found all 9 green again. A green is a failure, or a claim once a
`Verified-By:` is checked, which happens after the probe as ever; so a kept green can never pass a
file a fresh probe would fail.

**What a Go run read** is every package of the module it compiled, its tests' imports included
(`go list -deps -test`), and go.mod and go.sum: the named packages alone missed a changed package
they import (finding 3). The web runs' are the web client's whole tree, which a narrower key by
vitest's own module graph would improve (finding 3, not built: a key missing an edge would take a
stale red, and a red passes a file).

**The test that caught it before.** A red also records which test failed, by the file's path alone,
at any base: a Go test's name and package, a vitest file. A later probe of the same file, whose
revert the record cannot answer, runs those first (finding 2: 88 of a check-pr's 145 reds had an
earlier red for the same file). It decides nothing: a red there is a red as any run's, a green goes
on to the usual runs.

`make check-everything`, the nightly run, reads no record (`--trust-no-record`).
"""

from __future__ import annotations

import functools
import re
import subprocess
from pathlib import Path
from typing import Any

import last_run
import revert_red_commands as commands
from revert_red_attempts import Attempt

GATE = "revert-red"
NAME = "known-red"
# Enough reverts for a batch and the commits after it; the oldest go first.
KEPT = 2000


def blob(root: str, rev: str, path: str) -> str:
    """The blob path has at rev, or "" when it has none there."""
    return subprocess.run(["git", "rev-parse", "--verify", "--quiet", f"{rev}:{path}"], cwd=root,
                          capture_output=True, text=True, check=False).stdout.strip()


def tree(root: str, rev: str, directory: str) -> str:
    """The git tree of a directory at rev, or "" when it has none there."""
    return blob(root, rev, directory.removeprefix("./").rstrip("/") or ".")


def revert_key(root: str, base: str, head: str, paths: list[str]) -> str:
    """The revert of paths, the probed file first and its companions after, by their blobs."""
    return "|".join(f"{path}:{blob(root, base, path)}>{blob(root, head, path)}" for path in paths)


GO_PACKAGE = re.compile(r"^\./(.+?)(/\.\.\.)?$")


# Files every Go run reads besides its packages' directories.
GO_MODULE_FILES = ("go.mod", "go.sum")


@functools.cache
def _go_dirs(root: str, packages: tuple[str, ...]) -> tuple[str, ...]:
    """The module's directories packages compile, their tests' imports included, relative to root;
    () when `go list` cannot say."""
    done = subprocess.run(["go", "list", "-e", "-deps", "-test", "-f", "{{if not .Standard}}{{.Dir}}{{end}}",
                           *packages], cwd=root, capture_output=True, text=True, check=False)
    if done.returncode != 0:
        return ()
    top = Path(root).resolve()
    found = set()
    for line in done.stdout.splitlines():
        directory = Path(line.strip()).resolve() if line.strip() else None
        if directory is not None and directory.is_relative_to(top):
            found.add(str(directory.relative_to(top)) or ".")
    return tuple(sorted(found))


def dirs_read(command: list[str], root: str = "") -> list[str]:
    """The directories (and files) a run read: for Go, every module package the packages it names
    compile, and go.mod and go.sum, or the packages it names where root is not given; the web client
    for a vitest or npm run; the scripts for the Python tests. Another run's are unknown, and its
    result is not kept."""
    go = [match.group(1) for argument in command if (match := GO_PACKAGE.match(argument))]
    if commands.is_go_test(command) and go:
        if root:
            compiled = _go_dirs(root, tuple(argument for argument in command if GO_PACKAGE.match(argument)))
            if compiled:
                return [*compiled, *GO_MODULE_FILES]
        return go
    if command and command[0] in ("npx", "npm"):
        return ["web"]
    if any(argument.endswith("py_tests.py") for argument in command):
        return ["scripts", ".claude/hooks"]
    return []


def remember(root: str, head: str, key: str, ran: list[str] | list[list[str]], verdict: str, detail: str,
             part: str = "") -> None:
    """Records a revert's verdict, with the trees at head of what its runs read: a red's one run, or a
    green's every run (ran, a list of commands), the part it was probed in. Nothing is kept when a
    run read something unknown."""
    runs = ran if ran and isinstance(ran[0], list) else [ran]
    dirs: list[str] = []
    for command in runs:
        read = dirs_read(command, root)
        if not read:
            return
        dirs += read
    trees = {directory: tree(root, head, directory) for directory in dict.fromkeys(dirs)}

    def change(old: dict[str, Any] | None) -> dict[str, Any]:
        reverts = dict((old or {}).get("reverts", {}))
        stored = verdict_key(key, verdict, part)
        reverts.pop(stored, None)
        reverts[stored] = {"trees": trees, "verdict": verdict, "detail": detail, "head": head}
        return {**(old or {}), "reverts": dict(list(reverts.items())[-KEPT:])}
    last_run.update(GATE, NAME, change)


def verdict_key(key: str, verdict: str, part: str) -> str:
    """Where a verdict is kept: a red by its revert, whatever part found it; a green by its revert
    and its part, as a green in one part says nothing of another's runs."""
    return key if verdict != "GREEN" else f"{key}#green:{part}"


def known(root: str, head: str, key: str, part: str = "") -> dict[str, Any] | None:
    """The red recorded for this revert, else the green recorded for it in this part, if everything
    its runs read is the same at head."""
    reverts = (last_run.read(GATE, NAME) or {}).get("reverts", {})
    for stored in (key, verdict_key(key, "GREEN", part)):
        entry = reverts.get(stored)
        if entry and all(tree(root, head, directory) == was for directory, was in entry["trees"].items()):
            return entry
    return None


# A failed Go test's line, and the line ending its package's output.
GO_FAILED_TEST = re.compile(r"^\s*--- FAIL: (Test\w+)", re.MULTILINE)
GO_FAILED_PACKAGE = re.compile(r"^FAIL\s+(\S+)", re.MULTILINE)
# A failed vitest file, with its project where the suite has several.
VITEST_FAILED = re.compile(r"FAIL\s+(?:\|[\w-]+\|\s+)?(\S+\.test\.tsx?)")
HINTS = "hints"


def failed_tests(output: str, module: str) -> dict[str, Any]:
    """The tests a red run's output says failed: Go's top-level tests by package (`./dir`), and
    vitest's files."""
    go: dict[str, list[str]] = {}
    tests: list[str] = []
    for line in output.splitlines():
        if (test := GO_FAILED_TEST.match(line)) and "/" not in test.group(1):
            tests.append(test.group(1))
        elif (package := GO_FAILED_PACKAGE.match(line)) and package.group(1).startswith(module):
            directory = "./" + package.group(1).removeprefix(module).lstrip("/") if package.group(1) != module else "."
            if tests:
                go[directory] = sorted(set(go.get(directory, []) + tests))
            tests = []
    vitest = sorted({match.group(1) for match in VITEST_FAILED.finditer(output)})
    return {key: value for key, value in (("go", go), ("vitest", vitest)) if value}


def remember_failed(path: str, output: str, module: str) -> None:
    """Records, by path alone, the tests a red run of its revert failed."""
    failed = failed_tests(output, module)
    if not failed:
        return

    def change(old: dict[str, Any] | None) -> dict[str, Any]:
        hints = dict((old or {}).get(HINTS, {}))
        hints.pop(path, None)
        hints[path] = failed
        return {**(old or {}), HINTS: dict(list(hints.items())[-KEPT:])}
    last_run.update(GATE, NAME, change)


def caught_before(path: str) -> list[Attempt]:
    """The runs of the tests an earlier red of path's revert failed, to try first."""
    hint = ((last_run.read(GATE, NAME) or {}).get(HINTS) or {}).get(path)
    if not hint:
        return []
    label = ", found by the test that caught its revert before"
    runs = [Attempt(commands.go_test(directory, tests=tests), label)
            for directory, tests in sorted((hint.get("go") or {}).items())]
    files = hint.get("vitest") or []
    plain = [file for file in files if ".browser.test." not in file]
    browser = [file for file in files if ".browser.test." in file]
    if plain:
        runs.append(Attempt(commands.vitest("run", *plain), label))
    if browser:
        runs.append(Attempt(commands.vitest("run", *browser, browser=True), label))
    return runs
