#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: `make spot`: the tests of what the working tree changed, and of what imports it, in
# ABOUTME: about 30 seconds, for checking a small change while working (D279, principles §23).
"""The spot check: run only what a change can affect, within 30 seconds (D279).

A small change is checked in seconds; `make check` waits for the commit and `make check-pr` for an
accumulated batch (testing.md §10). This finds what changed since `--base` (by default HEAD, with
the working tree's uncommitted and untracked files), and runs, at once:

* **Go:** `go test` of each package a changed Go file is in, and of every package that imports one
  of them directly (its tests included). What imports those in turn is `make check`'s. And
  golangci-lint of each changed package alone, with make check's integration build tag: a long line
  or an unchecked error was found only by the turn's-end gate, four whole runs in one session (the
  owner, 2026-10-31: "lint the one package first" was a habit, not a check).
* **Web:** the unit tests make check would run for the change, chosen as it chooses them
  (web_tests.command_for): the changed tests, those using a changed declaration at any distance by
  name or loading a module whose loading changed (web_test_reach.cjs), those reading the sources by
  path, and `vitest related` when the reach cannot tell. When the change calls for the whole suite
  (a deleted file, one every test reads), the spot check names it as make check's. And the type
  check, `tsc -b`, which is incremental (about 3 s), under make check's own record (area_stamp.py),
  so a pass here spares make check's; and eslint of the changed files, about 6 s. Depth 1 ran only
  the tests using a changed name themselves: a gallery test reading a changed component's markup
  through the page that draws it ran first in check-pr, and a needless `!` failed check-pr's lint,
  each a round of it (the owner, 2026-11-24, agreeing to the test-time stock-take's item 2: "lint
  issues are the most likely outcome of any code change an LLM makes"). The cost is the reach's:
  about 25 s for a component the pages draw, a minute for a file the player's every test loads.
  A stylesheet only a browser renders: the browser-mode tests are make check's.
* **Python:** the gates' test modules that import a changed script or hook, directly or not, or
  name it (revert_red_dependents.py), and every module when ./dev, the Makefile or a git hook
  changed, which no import follows: every module for any script took 20 s, the spot check's floor (the
  owner, 2026-11-20: test duration "sets the minimum duration of a dev cycle").
* **Go tests that read other files:** a package whose tests climb out of it to read a file by path
  (`filepath.Join("..", "..", "web", ...)`), when a changed path is under the one they name. A web
  file's change ran no Go test, and a test reading the SDK's limits from its source failed only at
  the turn's end (the owner, 2026-11-21: "yes that sounds good"). Go's test cache keys a result on
  the files the test opened, so a package that read none of the changed files is served from it.
* **Documentation:** the docs gate (`scripts/check_docs.py`), when Markdown changed.
* **The owner's domains:** the test that only suggested.go names them, when a shipped source changed
  and its package is not already run whole.

What it cannot run in the time (the dev build's Go lint, generated code, the integration and browser
tiers) it names, so a green is never read as `make check`'s.
"""

from __future__ import annotations

import argparse
import fnmatch
import os
import re
import shlex
import subprocess
import sys
import time
from concurrent.futures import ThreadPoolExecutor
from dataclasses import dataclass, field
from pathlib import Path
from collections.abc import Callable

import gate_runs
from revert_red_dependents import python_tests_reaching_any
from web_tests import UNIT, command_for, reached_tests

BUDGET_S = gate_runs.BUDGET_S["spot"]
# How many of its runs go at once, by default: two, so a spot check beside staging or another
# agent's gate does not take the machine (the owner's staging at a load of 46, 2026-10-11).
DEFAULT_JOBS = 2
HERMETIC = {"TZ": "UTC", "LC_ALL": "C.UTF-8", "GIT_CONFIG_GLOBAL": "/dev/null", "GIT_CONFIG_NOSYSTEM": "1"}

WEB_SOURCE = re.compile(r"^web/(packages|apps)/[^/]+/(src|e2e)/.*\.tsx?$")
WEB_STYLE = re.compile(r"^web/(packages|apps)/[^/]+/src/.*\.css$")
# What the web's type check and lint read: its TypeScript and JavaScript, and the TypeScript configuration.
WEB_TYPED = re.compile(r"^web/(?!.*node_modules/).*(\.(ts|tsx|mts|cts|js|mjs|cjs)|/tsconfig[^/]*\.json)$")
WEB_LINTED = re.compile(r"\.(ts|tsx|mts|cts|js|mjs|cjs)$")
WEB_TYPECHECK = [sys.executable, "scripts/area_stamp.py", "web-typecheck", "web", "--",
                 "npm", "--prefix", "web", "run", "--silent", "typecheck"]
# make check's web-lint, on the files given: its cache, shared with make check's, keyed by content.
WEB_LINT = ["npx", "--no-install", "eslint", "--max-warnings", "0", "--no-warn-ignored", "--cache",
            "--cache-strategy", "content", "--cache-location", "node_modules/.cache/eslint/"]
PYTHON_GATES = re.compile(r"^(scripts/.*\.py|\.claude/hooks/.*\.py|dev|Makefile|mk/.*\.mk|\.githooks/.*)$")
GENERATED_FROM = re.compile(r"^(internal/store/(cache/)?queries/.*\.sql|internal/core/api/.*\.go)$")


# The media-name gate and its registry: a change to either is checked against every file.
MEDIA_NAMES_GATE = {"scripts/check_media_names.py", "scripts/media_names.allow"}

# The test that only suggested.go of the shipped sources names the owner's domains: it reads every
# file, so a change to any shipped source runs it, about a second. It was met only at the turn's end
# twice in a day, a comment quoting the owner (the owner, 2026-11-20: "make it run the domain test").
# SHIPPED_SOURCE is the test's own isShippedSource, by extension, a test file excepted.
DOMAIN_TEST = "TestSuggested_IsTheOnlyShippedSourceThatNamesTheOwnersDomains"
DOMAIN_TEST_PACKAGE = "internal/core/discovery"
SHIPPED_SOURCE = re.compile(r"\.(go|ts|tsx|mts|js|mjs|sh|html|yaml|toml)$")
UNSHIPPED_TEST = re.compile(r"(_test\.go|\.test\.|\.spec\.)")
GO_TEST_ONE = ["go", "test", "-run"]

# A Go test naming a path out of its package: filepath.Join, its leading string literals, the first "..".
CLIMBING_JOIN = re.compile(r'filepath\.Join\(\s*((?:"[^"\n]*"\s*,\s*)*"[^"\n]*")')
STRING_LITERAL = re.compile(r'"([^"\n]*)"')
# Or a literal that climbs by itself: "../../deploy/omv/compose.yaml".
CLIMBING_LITERAL = re.compile(r'"(\.\./[^"\n]*)"')


@dataclass
class Plan:
    """What the spot check runs, and what it names as left to `make check`."""

    go_packages: list[str] = field(default_factory=list)
    lint_packages: list[str] = field(default_factory=list)
    web_files: list[str] = field(default_factory=list)
    # Every changed path under web/, which make check's choice of web tests reads (web_tests.command_for).
    web_changes: list[str] = field(default_factory=list)
    # The changed files the web's type check and lint read.
    web_typed: list[str] = field(default_factory=list)
    python: bool = False
    # The gate test modules to run, when python: None for every one.
    python_tests: list[str] | None = None
    docs: bool = False
    # The changed files the media-name gate reads, and whether it reads every file instead: when the
    # gate or its registry changed.
    named: list[str] = field(default_factory=list)
    named_all: bool = False
    # Whether a shipped source changed, so the domain test runs (DOMAIN_TEST).
    shipped: bool = False
    not_run: list[str] = field(default_factory=list)


def changed_paths(base: str) -> list[str]:
    """The paths changed since base, the working tree's uncommitted and untracked ones included."""
    def git(*args: str) -> list[str]:
        out = subprocess.run(["git", *args], capture_output=True, text=True, check=False).stdout
        return [line for line in out.splitlines() if line]
    paths = git("diff", "--name-only", base) + git("ls-files", "--others", "--exclude-standard")
    return sorted(set(paths))


def without_removed_packages(paths: list[str], is_dir=os.path.isdir) -> list[str]:
    """`paths` but a Go file whose directory is gone: a package the change removed, which has nothing
    left to test or lint (its importers changed too, and are in `paths` themselves)."""
    return [path for path in paths if not path.endswith(".go") or is_dir(str(Path(path).parent))]


def plan(paths: list[str], module: str, importers: dict[str, set[str]],
         python_reach: Callable[[list[str]], list[str] | None] | None = None,
         readers: dict[str, set[str]] | None = None) -> Plan:
    """What to run for `paths`: `module` is go.mod's module path, and `importers` maps each Go package
    to the packages that import it directly, their tests included. python_reach gives the gate test
    modules that changed Python paths reach, None for every one; by default, by their imports.
    `readers` maps a path to the Go packages whose tests read it or the files under it (go_test_readers)."""
    result = Plan()
    read_by = {package for path in paths for read, packages in (readers or {}).items()
               if path == read or path.startswith(read + "/") or fnmatch.fnmatchcase(path, read)
               for package in packages}
    python_paths = []
    result.named, result.named_all = list(paths), bool(MEDIA_NAMES_GATE & set(paths))
    result.shipped = any(SHIPPED_SOURCE.search(path) and not UNSHIPPED_TEST.search(Path(path).name) for path in paths)
    changed_packages: set[str] = set()
    styles = []
    result.web_changes = [path for path in paths if path.startswith("web/") and "node_modules/" not in path]
    result.web_typed = [path for path in result.web_changes if WEB_TYPED.match(path)]
    for path in paths:
        if path.endswith(".go"):
            directory = str(Path(path).parent)
            changed_packages.add(module if directory == "." else f"{module}/{directory}")
            if GENERATED_FROM.match(path) and "/api/" in path:
                result.not_run.append("the API document and SDK types `make generate` writes from the handlers")
        elif GENERATED_FROM.match(path):
            result.not_run.append(f"{path}: sqlc's code from it (`make generate`)")
        elif WEB_SOURCE.match(path):
            result.web_files.append(path)
        elif WEB_STYLE.match(path):
            styles.append(path)
        elif PYTHON_GATES.match(path):
            result.python = True
            python_paths.append(path)
        elif path.endswith(".md"):
            result.docs = True
        elif path in ("go.mod", "go.sum") or path.startswith("web/package"):
            result.not_run.append(f"{path}: a dependency change is `make check`'s")
    if result.python:
        reach = python_reach or (lambda changed: python_tests_reaching_any(".", changed))
        result.python_tests = reach(python_paths)
        if result.python_tests == []:
            result.python = False
            result.not_run.append("the gates' Python tests: none reaches " + ", ".join(python_paths))
    packages = set(changed_packages)
    for package in changed_packages:
        packages |= importers.get(package, set())
    result.go_packages = sorted(packages | read_by)
    # Lint takes the changed packages alone: an importer's lint does not change with them.
    result.lint_packages = sorted(changed_packages)
    if styles:
        result.not_run.append("stylesheets, which only the browser tier renders: " + ", ".join(styles))
    if any(is_browser_test(path) for path in result.web_files):
        result.not_run.append("browser-mode tests, which run in a browser (`make check`)")
    if result.go_packages:
        result.not_run.append("what imports the importers, the dev build's tests and its Go lint (`make check`)")
    result.not_run = list(dict.fromkeys(result.not_run))
    return result


def go_module() -> str:
    """go.mod's module path."""
    for line in Path("go.mod").read_text().splitlines():
        if line.startswith("module "):
            return line.split()[1]
    return ""


def go_test_readers(module: str, test_files: list[str]) -> dict[str, set[str]]:
    """Each path a Go package's tests climb out of the package to read, to those packages: the
    leading literals of a filepath.Join that starts with "..", or a literal starting "../", resolved
    from the test's directory, a glob kept as one (`*.sql.go`). A literal that names no file the
    repository has (test data such as "../etc/passwd") maps a path no change touches. A path that ends in a directory stands for the files under it, which the test names at run time
    (`filepath.Join("..", "..", "web", "src", file)`). A path inside the package, or one that
    resolves to the repository's root or above it, is left out: the root would run the package on
    every change, and the root's walkers have runs of their own (DOMAIN_TEST)."""
    readers: dict[str, set[str]] = {}
    for test in test_files:
        directory = os.path.dirname(test) or "."
        try:
            source = Path(test).read_text(encoding="utf-8")
        except OSError:
            continue
        climbs = [STRING_LITERAL.findall(joined.group(1)) for joined in CLIMBING_JOIN.finditer(source)]
        climbs += [[literal] for literal in CLIMBING_LITERAL.findall(source)]
        for parts in climbs:
            if not parts[0].startswith(".."):
                continue
            read = os.path.normpath(os.path.join(directory, *parts))
            if read == "." or read.startswith("..") or read == directory or read.startswith(directory + "/"):
                continue
            readers.setdefault(read, set()).add(module if directory == "." else f"{module}/{directory}")
    return readers


def go_tests() -> list[str]:
    """The repository's Go test files, tracked or new."""
    listed = subprocess.run(["git", "ls-files", "--cached", "--others", "--exclude-standard", "--", "*_test.go"],
                            capture_output=True, text=True, check=False).stdout
    return [line for line in listed.splitlines() if line]


def go_importers() -> tuple[str, dict[str, set[str]]]:
    """go.mod's module, and each package's direct importers, tests included, by `go list`."""
    module = go_module()
    listed = subprocess.run(
        ["go", "list", "-e", "-f",
         "{{.ImportPath}}{{range .Imports}} {{.}}{{end}}{{range .TestImports}} {{.}}{{end}}"
         "{{range .XTestImports}} {{.}}{{end}}", "./..."],
        capture_output=True, text=True, check=False, env={**os.environ, **HERMETIC}).stdout
    importers: dict[str, set[str]] = {}
    for line in listed.splitlines():
        package, *imports = line.split()
        for imported in imports:
            if imported != package:
                importers.setdefault(imported, set()).add(package)
    return module, importers


@dataclass
class Run:
    label: str
    command: list[str]
    # Where it runs, when not the repository's root: eslint finds its configuration from there.
    cwd: str | None = None


def runs(spot: Plan, module: str, base: str = "HEAD") -> list[Run]:
    out = []
    if spot.go_packages:
        relative = [relative_package(p, module) for p in spot.go_packages]
        out.append(Run(f"go test, {len(relative)} package(s)", go_test_as_make_check(relative)))
    tested = web_test_run(spot.web_changes, base)
    if isinstance(tested, Run):
        out.append(tested)
    elif tested is not None:
        spot.not_run.append(tested)
    if spot.web_typed:
        out.append(Run("the web type check", WEB_TYPECHECK))
    linted_web = [path.removeprefix("web/") for path in spot.web_typed
                  if WEB_LINTED.search(path) and Path(path).is_file()]
    if linted_web:
        out.append(Run(f"the web lint, {len(linted_web)} file(s)", [*WEB_LINT, *linted_web], cwd="web"))
    if spot.python:
        some = spot.python_tests or []
        label = f"the gates' Python tests, {len(some)} module(s)" if some else "the gates' Python tests"
        out.append(Run(label, [sys.executable, "scripts/py_tests.py", *some]))
    if spot.docs:
        out.append(Run("the docs gate", [sys.executable, "scripts/check_docs.py"]))
    linted = [relative_package(p, module) for p in spot.lint_packages]
    linted = [p for p in linted if any(Path(p).glob("*.go"))]  # a package the change deleted has nothing to lint
    if linted:
        out.append(Run(f"golangci-lint, {len(linted)} package(s)", go_lint_as_make_check(linted)))
    if spot.named or spot.named_all:
        named = [] if spot.named_all else spot.named
        out.append(Run("the media-name gate", [sys.executable, "scripts/check_media_names.py", *named]))
    # The whole package's run, when it is planned, runs the domain test with the rest.
    if spot.shipped and f"{module}/{DOMAIN_TEST_PACKAGE}" not in spot.go_packages:
        out.append(Run("the owner's-domains test", [*GO_TEST_ONE, f"^{DOMAIN_TEST}$", f"./{DOMAIN_TEST_PACKAGE}"]))
    return out


def relative_package(package: str, module: str) -> str:
    return "./" + package.removeprefix(module).lstrip("/") if package != module else "."


def go_lint_as_make_check(packages: list[str]) -> list[str]:
    """golangci-lint of packages as make check's go-lint runs it (Makefile), waiting for the machine's
    lint lock rather than failing beside another run, so its cache serves both."""
    return ["golangci-lint", "run", "--allow-serial-runners", "--build-tags", "integration", *packages]


def go_test_as_make_check(packages: list[str]) -> list[str]:
    """Go's tests of packages run as make check's go-test runs them, `-json` through its timings, so
    each serves the other from Go's test cache: the flags are part of a result's key, and a plain
    `go test` here left make check to run again every package it had just passed (the caching
    audit's finding 4, 2026-10-28). The timings go to a suite of their own, apart from make check's."""
    listed = " ".join(shlex.quote(package) for package in packages)
    return ["bash", "-o", "pipefail", "-c",
            f"go test -json {listed} | {shlex.quote(sys.executable)} scripts/go_test_timings.py go-spot"]


# What the spot check says when the change calls for make check's whole web suite.
WHOLE_WEB_SUITE = "the whole web unit suite, which a deleted web file or one every web test reads calls for (`make check`)"


def web_test_run(paths: list[str], base: str = "HEAD", reach=reached_tests) -> Run | str | None:
    """The unit test run make check would make for the web paths changed since base (web_tests.py's
    command_for), every failure listed rather than the first alone: None when they call for none, and
    WHOLE_WEB_SUITE when they call for the whole suite, make check's to run."""
    changes = [("M" if Path(path).exists() else "D", path) for path in paths]
    # The root absolute: the reach reads TypeScript from under it, run in web/.
    command = command_for(changes, str(Path.cwd()), UNIT, base, reach) if changes else None
    if command == UNIT.whole:
        return WHOLE_WEB_SUITE
    if not command:
        return None
    if command[:len(UNIT.named)] == UNIT.named:
        return Run(f"vitest, {len(command) - len(UNIT.named)} test file(s)", command)
    return Run(f"vitest related, {len(command) - len(UNIT.related)} file(s)", command)


def is_browser_test(path: str) -> bool:
    """Whether path is a browser-mode test, which spot leaves to make check."""
    return ".browser.test." in path


def execute(run: Run) -> tuple[Run, int, float, str]:
    started = time.monotonic()
    done = subprocess.run(run.command, capture_output=True, text=True, check=False, cwd=run.cwd,
                          env={**os.environ, **HERMETIC})
    return run, done.returncode, time.monotonic() - started, done.stdout + done.stderr


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--base", default="HEAD", help="compare against this commit (default: HEAD, so the "
                    "uncommitted changes)")
    ap.add_argument("--jobs", type=int, default=DEFAULT_JOBS, help=f"runs at once (default {DEFAULT_JOBS})")
    args = ap.parse_args()
    started, began, load_before, cpu_before = time.monotonic(), time.time(), gate_runs.load(), gate_runs.cpu_now()
    paths = changed_paths(args.base)
    if not paths:
        print(f"spot: nothing changed since {args.base}; give --base to check commits (e.g. --base HEAD~1)")
        return 0
    module, importers = go_importers() if any(p.endswith(".go") for p in paths) else (go_module(), {})
    spot = plan(without_removed_packages(paths), module, importers, readers=go_test_readers(module, go_tests()))
    planned = runs(spot, module, args.base)
    with ThreadPoolExecutor(max_workers=max(1, min(args.jobs, len(planned)))) as pool:
        results = list(pool.map(execute, planned))
    failed = False
    for run, status, seconds, output in results:
        print(f"spot: {run.label}: {'passed' if status == 0 else 'FAILED'} in {seconds:.0f} s")
        if status != 0:
            failed = True
            print(output[-4000:])
    for gap in spot.not_run:
        print(f"spot: not run here: {gap}")
    took = time.monotonic() - started
    gate_runs.record({"start": gate_runs.started_at(began), "target": "spot", "seconds": round(took),
                      "exit": 1 if failed else 0, "worktree": gate_runs.worktree(), "loadBefore": load_before,
                      "loadAfter": gate_runs.load(), "cpu": gate_runs.cpu_used(cpu_before), "paths": len(paths),
                      "steps": [[run.label, round(seconds, 1)] for run, _, seconds, _ in
                                sorted(results, key=lambda done: -done[2])][:gate_runs.STEPS_KEPT]})
    over = f", over its {BUDGET_S} s budget (D279)" if took > BUDGET_S else ""
    print(f"spot: {len(paths)} changed path(s), {'FAILED' if failed else 'passed'} in {took:.0f} s{over}. "
          "Not make check: run it before the commit.")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
