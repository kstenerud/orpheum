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

from revert_red_commands import vitest

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


VITEST_RUN = vitest("run", first_red=False)
# A probe's run, which its first red settles; the spot check runs VITEST_RUN to its end, to list every
# failure.
PROBE_RUN = vitest("run")


def added_file_scope(tree: str, path: str) -> list[str] | None:
    """The vitest run of the tests importing an added web file, or None when none does."""
    tests = importing_tests(tree, path)
    return [*PROBE_RUN, *tests] if tests else None
