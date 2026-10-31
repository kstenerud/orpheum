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
they import (finding 3). A web red is kept by the modules of the tests it failed, read from its
output, and what they import, relative and through the workspace packages, with the runs'
configurations (revert_red_web.modules_read; the owner, 2026-10-30: the whole web client's tree,
which every web commit changed, re-ran 140 related runs, half a 75-minute check-pr). A key missing an
import would take a stale red, and a red passes a file: a test whose module reads files no import
names (node:fs, import.meta.glob) keeps the whole tree, and the nightly run trusts no record. A web
green, and a red whose output names no test, still keeps the whole tree.

**The test that caught it before.** A red also records which test failed, by the file's path alone,
at any base: a Go test's name and package, a vitest file. A later probe of the same file, whose
revert the record cannot answer, runs those first (finding 2: 88 of a check-pr's 145 reds had an
earlier red for the same file). It decides nothing: a red there is a red as any run's, a green goes
on to the usual runs.

`make check-everything`, the nightly run, reads no record (`--trust-no-record`).
"""

from __future__ import annotations

import functools
import hashlib
import re
import subprocess
import sys
from pathlib import Path
from typing import Any

import last_run
import revert_red_commands as commands
import revert_red_web
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


def files_digest(root: str, rev: str, paths: list[str]) -> str:
    """One digest of the blobs paths have at rev, read in one git run: a red's web key names
    hundreds of files, a git run each too slow for every probe's lookup."""
    listed = subprocess.run(["git", "ls-tree", "-r", "--full-tree", rev, "--", *paths], cwd=root,
                            capture_output=True, text=True, check=False).stdout
    return hashlib.sha256(listed.encode()).hexdigest()


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


def web_modules_read(root: str, command: list[str], output: str) -> list[str] | None:
    """The files a red vitest run read that a test it failed rests on: the failed tests' modules
    and the runs' configurations (revert_red_web.modules_read), or None when the run is not
    vitest's, its output names no failed test, or what those read cannot be known."""
    if not command or command[0] not in ("npx", "npm") or not output:
        return None
    failed = sorted({match.group(1) for match in VITEST_FAILED.finditer(output)})
    return revert_red_web.modules_read(root, failed) if failed else None


def remember(root: str, head: str, key: str, ran: list[str] | list[list[str]], verdict: str, detail: str,
             part: str = "", output: str = "") -> None:
    """Records a revert's verdict, with the trees at head of what its runs read: a red's one run, or a
    green's every run (ran, a list of commands), the part it was probed in. A red vitest run whose
    output names the tests it failed is kept by those tests' modules alone (output), so a later web
    commit elsewhere keeps it. Nothing is kept when a run read something unknown."""
    runs = ran if ran and isinstance(ran[0], list) else [ran]
    files = web_modules_read(root, runs[0], output) if len(runs) == 1 and verdict != "GREEN" else None
    dirs: list[str] = []
    for command in ([] if files else runs):
        read = dirs_read(command, root)
        if not read:
            return
        dirs += read
    trees = {directory: tree(root, head, directory) for directory in dict.fromkeys(dirs)}
    kept_files = {"paths": files, "digest": files_digest(root, head, files)} if files else None

    def change(old: dict[str, Any] | None) -> dict[str, Any]:
        reverts = dict((old or {}).get("reverts", {}))
        stored = verdict_key(key, verdict, part)
        reverts.pop(stored, None)
        entry: dict[str, Any] = {"trees": trees, "verdict": verdict, "detail": detail, "head": head}
        if kept_files:
            entry["files"] = kept_files
        reverts[stored] = entry
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
        if entry and all(tree(root, head, directory) == was for directory, was in entry["trees"].items()) and (
                "files" not in entry or files_digest(root, head, entry["files"]["paths"]) == entry["files"]["digest"]):
            return entry
    return None


# A failed Go test's line, and the line ending its package's output.
GO_FAILED_TEST = re.compile(r"^\s*--- FAIL: (Test\w+)", re.MULTILINE)
GO_FAILED_PACKAGE = re.compile(r"^FAIL\s+(\S+)", re.MULTILINE)
# A failed vitest file, with its project where the suite has several, or its browser in browser mode
# (" FAIL   chromium  apps/…browser.test.tsx"): named, a browser-mode red's confirming run is its failed
# files alone, seconds, not every browser-mode test that imports the file (the test-regime audit of
# 2026-10-31, R5: 224 s for one file).
VITEST_FAILED = re.compile(r"FAIL\s+(?:\|[\w-]+\|\s+|(?:chromium|firefox|webkit)\s+)?(\S+\.test\.tsx?)")
# A terminal colour, which vitest may print about a project's or browser's name.
COLOUR = re.compile(r"\x1b\[[0-9;]*m")
# A failed module of the gate scripts' tests, as py_tests.py names it.
PYTHON_FAILED = re.compile(r"^--- (test_\w+\.py) failed ---$", re.MULTILINE)
PYTHON_TESTS = "scripts/tests"
HINTS = "hints"


def runner_of(command: list[str]) -> str | None:
    """Which runner a run is, "go", "vitest" or "python", or None for one that runs several (the
    browser tier, a whole suite)."""
    if any(part.endswith("py_tests.py") for part in command):
        return "python"
    if commands.is_vitest(command):
        return "vitest"
    if commands.is_go_test(command):
        return "go"
    return None


def failed_tests(output: str, module: str, command: list[str] | None = None) -> dict[str, Any]:
    """The tests a red run's output says failed: Go's top-level tests by package (`./dir`),
    vitest's files, and the gate scripts' test modules; of command's own runner alone where it has
    one. A gate script's tests print vitest's and Go's failures as fixtures, and their probe, taking
    those for its own red, ran vitest on a file no tree has (check-pr, 2026-10-30)."""
    runner = runner_of(command) if command is not None else None
    output = COLOUR.sub("", output)
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
    python = sorted({match.group(1) for match in PYTHON_FAILED.finditer(output)})
    return {key: value for key, value in (("go", go), ("vitest", vitest), ("python", python))
            if value and runner in (None, key)}


def runs_of(failed: dict[str, Any], label: str = "") -> list[Attempt]:
    """The runs of failed tests (failed_tests' answer), each kind's own: Go's by package, vitest's
    in jsdom and in browser mode, the gate scripts' by module."""
    runs = [Attempt(commands.go_test(directory, tests=tests), label)
            for directory, tests in sorted((failed.get("go") or {}).items())]
    files = failed.get("vitest") or []
    plain = [file for file in files if ".browser.test." not in file]
    browser = [file for file in files if ".browser.test." in file]
    if plain:
        runs.append(Attempt(commands.vitest("run", *plain), label))
    if browser:
        runs.append(Attempt(commands.vitest("run", *browser, browser=True), label))
    if failed.get("python"):
        # The gate's own runner, as the probe's (check_revert_red.PY_TESTS_COMMAND), against the tree's tests.
        runs.append(Attempt([sys.executable, str(Path(__file__).resolve().parent / "py_tests.py"),
                             *(f"{PYTHON_TESTS}/{name}" for name in failed["python"])], label))
    return runs


def remember_failed(path: str, output: str, module: str, command: list[str] | None = None) -> None:
    """Records, by path alone, the tests a red run of its revert, command, failed."""
    failed = failed_tests(output, module, command)
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
    return runs_of(hint, ", found by the test that caught its revert before") if hint else []
