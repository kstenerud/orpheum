#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: What of check-pr's integration tier, full web lint and browser tier a change can reach: the
# ABOUTME: packages, files or tests to run, so check-pr checks only what the change could break.
"""What a change since a base can break, for the check-pr steps that otherwise run whole.

"What I want ultimately is that check-pr only checks what is actually possible to break based on
the changes made" (the owner, 2026-10-17), with check-everything running everything each night
behind it. At nas.10 the three steps took 41 s, 132 s and 396 s of check-pr whatever had changed.

    impact.py integration --base REV PKG...   the integration packages, of PKG, whose tests build a
                                              changed package (any file under it: testdata, embeds);
                                              every one when a change may reach any (go.mod, the
                                              Makefile, the pinned tools), none when no change does
    impact.py web-lint --base REV             "." for every file, or the web files, relative to web/,
                                              that are changed or import one, directly or not (a type
                                              flows through imports); nothing when web/ is unchanged
    impact.py browser                         "all", or the JSON list (ORPHEUM_E2E_ONLY) of the tests
                                              that loaded a chunk holding a changed client source or
                                              drew what a changed stylesheet's rules select, by the last
                                              complete browser map, and of each changed spec, whole;
                                              or "none"

The browser tier's narrowing reads the change since the map's own commit, not since check-pr's base:
the map says which tests loaded which source at that commit, and only a change since then can have
moved one. It narrows only a change of the client's own sources; anything else since the map (the
server, the e2e harness, a build configuration) runs every test, and so does a map with no commit
behind HEAD. A narrowed run records no map, so the last complete one stays.

Every answer errs wide: what this cannot place, it runs.
"""

from __future__ import annotations

import argparse
import fnmatch
import json
import re
import subprocess
import sys
from pathlib import Path, PurePosixPath

import browser_map
from browser_tier_needed import NOT_IN_THE_TIER
from emitted_js import emits_the_same

# Changed alone, these build into no Go test: what the Go tests read lies under their packages.
OUTSIDE_GO = ("docs/*", "web/*", "*.md", ".claude/*", ".githooks/*", ".github/*", "scripts/*")
# The gate scripts that fetch the integration tier's pinned FFmpeg and MKVToolNix.
GO_TOOL_PINS = ("scripts/fetch_ffmpeg.py", "scripts/fetch_mkvtoolnix.py", "scripts/fetch_tools.py")

# Under web/, what eslint lints and dependency-cruiser follows imports between.
WEB_SOURCE = re.compile(r"^web/(apps|packages|e2e)/.+\.(ts|tsx)$")
# A declaration file can declare globals, which no import follows.
WEB_DECLARATIONS = re.compile(r"\.d\.ts$")
# What eslint does not read and nothing imports it through: no lint of any file changes with it.
WEB_UNLINTED = re.compile(r"\.(css|md|svg|png|ico|webmanifest|html)$")

# A client source the browser map places in the chunks it is built into.
CLIENT_SOURCE = re.compile(r"^web/(apps|packages)/[^/]+/src/.+\.(ts|tsx)$")
CLIENT_TEST = re.compile(r"\.test\.tsx?$")
# A stylesheet the browser map places by the tokens each test's pages held (browser_map.reached).
CLIENT_STYLESHEET = re.compile(r"^web/(apps|packages)/[^/]+/src/.+\.css$")
# A spec of browser tests, run whole when it changes: the map knows no test a change adds to it. Its
# helpers beside it (pages.ts, server.ts, browser-map.ts) are every spec's, and run them all.
BROWSER_SPEC = re.compile(r"^web/e2e/[\w-]+\.spec\.ts$")
# M4's acceptance, which the browser tier ignores (playwright.config.ts) and test-acceptance-home runs.
BROWSER_ACCEPTANCE = "web/e2e/acceptance.spec.ts"


def changed(base: str) -> list[str]:
    """The files changed between base and the working tree."""
    done = subprocess.run(["git", "diff", "--name-only", base], capture_output=True, text=True, check=True)
    return [line for line in done.stdout.splitlines() if line]


def _matches(path: str, patterns: tuple[str, ...]) -> bool:
    return any(fnmatch.fnmatch(path, pattern) for pattern in patterns)


# --- the integration tier -----------------------------------------------------------------------


def go_package_of(path: str, base: str) -> str | None:
    """The directory, "./x/y", of the nearest Go package holding path, at the working tree or base (a
    deleted package's files); None when no directory above it is one."""
    for directory in PurePosixPath(path).parents:
        if directory == PurePosixPath("."):
            return None
        if any(Path(directory).glob("*.go")) or _has_go_at(base, str(directory)):
            return f"./{directory}"
    return None


def _has_go_at(revision: str, directory: str) -> bool:
    done = subprocess.run(["git", "ls-tree", "--name-only", f"{revision}:{directory}"], capture_output=True,
                          text=True, check=False)
    return any(name.endswith(".go") for name in done.stdout.split())


def integration_deps(packages: list[str]) -> dict[str, frozenset[str]]:
    """Each integration package, "./x/y", to the directories of the module's packages its tests build
    under -tags integration, its own among them."""
    module = subprocess.run(["go", "list", "-m"], capture_output=True, text=True, check=True).stdout.strip()
    done = subprocess.run(["go", "list", "-e", "-test", "-tags", "integration", "-f",
                           "{{.ImportPath}}\t{{join .Deps \" \"}}", *packages], capture_output=True, text=True,
                          check=True)

    def directory(import_path: str) -> str:
        return "./" + import_path.removeprefix(module).lstrip("/")

    graph: dict[str, set[str]] = {}
    for line in done.stdout.splitlines():
        name, _, deps = line.partition("\t")
        own = directory(name.split(" ")[0].removesuffix(".test").removesuffix("_test"))
        graph.setdefault(own, {own}).update(directory(dep) for dep in deps.split() if dep.startswith(module))
    return {package: frozenset(deps) for package, deps in graph.items() if package in packages}


def integration_scope(paths: list[str], packages: list[str], base: str,
                      deps_of=integration_deps) -> tuple[list[str], str]:
    """The integration packages to test, and why."""
    reached: set[str] = set()
    for path in paths:
        if path in GO_TOOL_PINS:
            return packages, f"{path} pins the tier's tools"
        if _matches(path, OUTSIDE_GO):
            continue
        package = go_package_of(path, base)
        if package is None:
            return packages, f"{path} is in no Go package, so it may reach any"
        reached.add(package)
    if not reached:
        return [], "no Go package changed"
    deps = deps_of(packages)
    scoped = [package for package in packages if deps.get(package, frozenset({package})) & reached]
    return scoped, f"{len(reached)} changed Go package(s) reach {len(scoped)} of {len(packages)}"


# --- the full web lint --------------------------------------------------------------------------


def importers(sources: list[str]) -> list[str]:
    """The web sources, relative to web/, that import any of sources (relative to web/), directly or
    not, the sources among them: dependency-cruiser's reverse closure, type-only imports too."""
    reaches = "|".join(f"^{re.escape(source)}$" for source in sources)
    done = subprocess.run(["npx", "depcruise", "--config", ".dependency-cruiser.cjs", "--reaches", reaches,
                           "--output-type", "json", "packages", "apps", "e2e"], cwd="web", capture_output=True,
                          text=True, check=True)
    return sorted(module["source"] for module in json.loads(done.stdout)["modules"]
                  if not module.get("coreModule") and not module["source"].startswith("node_modules/"))


def web_lint_scope(paths: list[str], importers_of=importers) -> tuple[list[str], str]:
    """["."] for every file, the web files to lint, or none; and why."""
    web = [path for path in paths if path.startswith("web/") and not WEB_UNLINTED.search(path)]
    if not web:
        return [], "nothing eslint reads changed"
    sources = []
    for path in web:
        if not WEB_SOURCE.match(path) or WEB_DECLARATIONS.search(path):
            return ["."], f"{path} may change any file's lint"
        if Path(path).exists():
            sources.append(path.removeprefix("web/"))
    if not sources:
        return [], "only deleted web files, whose importers changed with them"
    scoped = sorted(set(importers_of(sources)) | set(sources))
    return scoped, f"{len(sources)} changed web file(s) and what imports them: {len(scoped)} file(s)"


# --- the browser tier ---------------------------------------------------------------------------


def _emits_as_at(commit: str, path: str) -> bool:
    """Whether the working tree's `path` compiles to the JavaScript it did at `commit`."""
    before = subprocess.run(["git", "show", f"{commit}:{path}"], capture_output=True, text=True, check=False)
    if before.returncode != 0 or not Path(path).is_file():
        return False
    return emits_the_same(path, before.stdout, Path(path).read_text(encoding="utf-8"))


def browser_scope(map_dir: Path = browser_map.BROWSER_MAP) -> tuple[list[str] | None, str]:
    """The tests to run, by name or by spec file, None for every one, and why."""
    stamp = map_dir / browser_map.COMPLETE
    if not stamp.is_file():
        return None, "no complete browser map"
    commit = stamp.read_text().strip()
    behind = subprocess.run(["git", "merge-base", "--is-ancestor", commit, "HEAD"], capture_output=True, check=False)
    if behind.returncode != 0:
        return None, f"the browser map's commit {commit[:12]} is not behind HEAD"
    loaded = browser_map.load_scripts(map_dir)
    recorded = browser_map.load(map_dir, commit)
    if loaded is None or recorded is None:
        return None, "the browser map recorded no scripts"
    chunks: set[str] = set()
    sheets: list[tuple[str, str]] = []
    specs: list[str] = []
    for path in changed(commit):
        if _matches(path, NOT_IN_THE_TIER) or path == BROWSER_ACCEPTANCE:
            continue
        if BROWSER_SPEC.match(path):
            specs += [PurePosixPath(path).name] if Path(path).exists() else []
            continue
        if CLIENT_STYLESHEET.match(path):
            before = subprocess.run(["git", "show", f"{commit}:{path}"], capture_output=True, text=True, check=False)
            if before.returncode != 0:
                return None, f"{path} is new since the browser map's commit {commit[:12]}, so it holds none of it"
            sheets.append((before.stdout, Path(path).read_text(encoding="utf-8") if Path(path).exists() else ""))
            continue
        if not CLIENT_SOURCE.match(path) or CLIENT_TEST.search(path):
            return None, f"{path} changed since the browser map's commit {commit[:12]}, and the map cannot place it"
        if _emits_as_at(commit, path):
            continue  # types or comments alone: every test's pages ran the same JavaScript (emitted_js.py)
        holding = browser_map.chunks_holding(map_dir, path)
        if holding is None:
            return None, f"{path} is in no chunk the map can narrow to (new, the service worker's, or none)"
        chunks |= holding
    styled = browser_map.reached(recorded, sheets) if sheets else []
    if styled is None:
        return None, "a changed stylesheet's rules may style any page (the browser map)"
    tests = [test for test, scripts in loaded if scripts is None or scripts & chunks or test in styled]
    return [*tests, *sorted(specs)], (f"{len(tests)} of {len(loaded)} tests loaded or drew what changed since "
                                      f"{commit[:12]}, and {len(specs)} changed spec(s) whole")


def main(argv: list[str]) -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = ap.add_subparsers(dest="step", required=True)
    integration = sub.add_parser("integration")
    integration.add_argument("--base", required=True)
    integration.add_argument("packages", nargs="+")
    sub.add_parser("web-lint").add_argument("--base", required=True)
    sub.add_parser("browser")
    args = ap.parse_args(argv)
    if args.step == "integration":
        scope, why = integration_scope(changed(args.base), args.packages, args.base)
        print(" ".join(scope))
    elif args.step == "web-lint":
        scope, why = web_lint_scope(changed(args.base))
        print(" ".join(scope))
    else:
        tests, why = browser_scope()
        print("all" if tests is None else json.dumps(tests) if tests else "none")
    print(f"{args.step}: {why}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
