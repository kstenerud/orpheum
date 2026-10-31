# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: revert-red's web scope for a file the branch added: the test files that import it, read
# ABOUTME: from the import graph before the probe deletes it, which vitest's own graph then cannot.
"""The tests that can notice an added web file, found before its revert deletes it (D279).

A file the branch modified is scoped by `vitest related`, which follows vitest's own import graph to
the tests that import it. A file the branch *added* is reverted by deleting it, and `vitest related`
cannot follow a file that is not there: it found nothing, so every added component fell through to
the whole web suite, about a minute each, five of them in one batch (2026-10-11). Read from the tree
while the file is still in it, the relative imports (`./x.tsx`, `../y.ts`) give the same answer in
milliseconds, and the probe runs just those tests. When none is found the probe goes on as before,
widening to the whole suite, so this only ever makes a red come sooner.
"""

from __future__ import annotations

import re
from pathlib import Path

from revert_red_attempts import Attempt
from revert_red_commands import vitest
from revert_red_tags import TYPECHECK

# A relative import or re-export: `from "./x.tsx"`, `import "./x.css"`, `import("./x.ts")`.
RELATIVE_IMPORT = re.compile(r"""(?:\bfrom\s*|\bimport\s*\(?\s*)["'](\.{1,2}/[^"']+)["']""")
SOURCE_GLOBS = ("packages/*/src/**/*.ts", "packages/*/src/**/*.tsx", "apps/*/src/**/*.ts", "apps/*/src/**/*.tsx")
TEST_FILE = re.compile(r"\.test\.tsx?$")
RESOLVE_SUFFIXES = ("", ".ts", ".tsx", "/index.ts", "/index.tsx")


def _resolve(importer: Path, specifier: str) -> Path | None:
    base = (importer.parent / specifier)
    for suffix in RESOLVE_SUFFIXES:
        candidate = Path(str(base) + suffix)
        if candidate.is_file():
            return candidate.resolve()
    return None


def importing_tests(tree: str, path: str, depth: int | None = None) -> list[str]:
    """The test files under web/ that import `path` (relative to the tree), directly or through other
    files, by relative imports; relative to web/, sorted. Empty when none does, or `path` is not a
    file of the tree. `depth` 1 keeps only the tests importing `path` itself, as the spot check
    does (spot_check.py); None follows every importer."""
    web = Path(tree) / "web"
    target = (Path(tree) / path).resolve()
    if not target.is_file():
        return []
    importers: dict[Path, set[Path]] = {}
    for pattern in SOURCE_GLOBS:
        for source in web.glob(pattern):
            if "node_modules" in source.parts:
                continue
            try:
                text = source.read_text(encoding="utf-8", errors="replace")
            except OSError:
                continue
            for specifier in RELATIVE_IMPORT.findall(text):
                imported = _resolve(source, specifier)
                if imported is not None:
                    importers.setdefault(imported, set()).add(source.resolve())
    seen = {target}
    frontier = [target]
    level = 0
    while frontier and (depth is None or level < depth):
        level += 1
        following = []
        for current in frontier:
            for importer in importers.get(current, ()):
                if importer not in seen:
                    seen.add(importer)
                    following.append(importer)
        frontier = following
    web_root = web.resolve()
    return sorted(str(found.relative_to(web_root)) for found in seen
                  if found != target and TEST_FILE.search(found.name))


# The web client's own packages, by the name its files import them by, at their entry point
# (each package.json's "exports").
WORKSPACE_IMPORT = re.compile(r"""(?:\bfrom\s*|\bimport\s*\(?\s*)["']@orpheum/([\w-]+)["']""")
WORKSPACE_ENTRY = "packages/{}/src/index.ts"
# What every vitest run reads besides its tests' modules: the configurations, with what they import,
# and what installs the packages.
RUN_CONFIGURATION = ("vitest.config.ts", "vitest.browser.config.ts", "apps/web/vite.config.ts", "package.json",
                     "package-lock.json", "tsconfig.json", "tsconfig.base.json", "apps/web/package.json",
                     "apps/web/tsconfig.json", "packages/sdk/package.json", "packages/sdk/tsconfig.json",
                     "packages/player/package.json", "packages/player/tsconfig.json")
# A module that reads files no import names: what it reads cannot be known from its imports.
READS_UNNAMED = re.compile(r"""["']node:fs(?:/promises)?["']|import\.meta\.glob""")


def _imported(web: Path, source: Path, text: str) -> list[Path]:
    """The web files source imports: relatively, or a workspace package's entry point."""
    found = [imported for specifier in RELATIVE_IMPORT.findall(text)
             if (imported := _resolve(source, specifier.split("?", 1)[0])) is not None]
    for package in WORKSPACE_IMPORT.findall(text):
        entry = web / WORKSPACE_ENTRY.format(package)
        if entry.is_file():
            found.append(entry.resolve())
    return found


def _closure(web: Path, roots: set[Path], refuses_unnamed: bool) -> set[Path] | None:
    """roots and the web files they import, directly or through others; None when refuses_unnamed
    and one of them reads files no import names, or one cannot be read."""
    seen = set(roots)
    frontier = list(roots)
    while frontier:
        following = []
        for current in frontier:
            if current.suffix not in (".ts", ".tsx"):
                continue
            try:
                text = current.read_text(encoding="utf-8", errors="replace")
            except OSError:
                return None
            if refuses_unnamed and READS_UNNAMED.search(text):
                return None
            for imported in _imported(web, current, text):
                if imported not in seen and imported.is_relative_to(web):
                    seen.add(imported)
                    following.append(imported)
        frontier = following
    return seen


def modules_read(tree: str, tests: list[str]) -> list[str] | None:
    """Every file a vitest run of tests (relative to web/) reads, relative to the tree: the tests and
    what they import, directly or through other files, and the runs' configurations with theirs; None
    when one of the tests' modules reads files by a path no import names (node:fs, import.meta.glob),
    or a test is not in the tree. The configurations' own reads (the timings they write) decide no
    test's result. Revert-red keys a web verdict by these files (revert_red_known.py) where it once
    took the whole web client, which any web commit changed."""
    web = (Path(tree) / "web").resolve()
    roots = {(web / test).resolve() for test in tests}
    if not roots or not all(root.is_file() for root in roots):
        return None
    configuration = _closure(web, {(web / name).resolve() for name in RUN_CONFIGURATION if (web / name).is_file()},
                             refuses_unnamed=False)
    modules = _closure(web, roots, refuses_unnamed=True)
    if configuration is None or modules is None:
        return None
    top = Path(tree).resolve()
    return sorted(str(path.relative_to(top)) for path in configuration | modules)


VITEST_RUN = vitest("run", first_red=False)
# A probe's run, which its first red settles; the spot check runs VITEST_RUN to its end, to list every
# failure.
PROBE_RUN = vitest("run")


def added_file_scope(tree: str, path: str) -> list[str] | None:
    """The vitest run of the tests importing an added web file, or None when none does."""
    tests = importing_tests(tree, path)
    return [*PROBE_RUN, *tests] if tests else None


def direct_then_types(tree: str, path: str) -> list[Attempt]:
    """A web file's first runs: the tests importing it itself (browser-mode files left to their own
    runs), then the type check, as other_builds makes it, before every test that reaches it."""
    direct = [test for test in importing_tests(tree, path, depth=1) if ".browser.test." not in test]
    first = [Attempt([*PROBE_RUN, *direct], ", found by the tests that import it")] if direct else []
    return [*first, Attempt(TYPECHECK, ", found by the type check", None, True)]
