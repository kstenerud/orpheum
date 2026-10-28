# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Which of the packages a revert reaches build differently under a build tag: those whose own
# ABOUTME: files, or whose dependencies' files, the tag changes; the tagged probe run tests only them.
"""The packages a tagged run needs to test (the caching audit's finding 6, 2026-10-28).

Revert-red tested a Go file's package and every dependent again with `-tags dev`, as make check's
tagged suite does. Only `cmd/orpheum` has dev-tagged files; every other package compiles the same
program under the tag, yet Go keys its test results on the flags too, so each was run again: 445 s
of probing in one check-pr, 556 s of a re-run's 2221 s. A package's tagged build differs from its
untagged one exactly when one of the packages it compiles, itself or a dependency, its tests' too,
has another file list under the tag; `go list` says so for every package at once.
"""

from __future__ import annotations

import functools
import json
import subprocess
from pathlib import Path

from revert_red_attempts import TS_SOURCE, Attempt, other_platforms
from revert_red_commands import go_test
from revert_red_cosmetic import GO_SOURCE
from revert_red_dependents import go_dependents, toplevel

# `make check` also tests the dev build and vets for macOS, so a file only those compile — a
# `//go:build dev` file, a `_darwin.go` shim — is reverted against them too, rather than
# reported as noticed by nothing.
GO_EXTRA_TAGS = "dev"
# The web client's type check, which vitest skips: it drops types unchecked.
TYPECHECK = ["npm", "--prefix", "web", "run", "typecheck"]


def _listed(root: str, tags: str | None) -> list[dict]:
    """Every package of the module with its tests' variants, as `go list -json` describes them."""
    command = ["go", "list", "-e", "-test", "-json=ImportPath,Dir,GoFiles,CgoFiles,TestGoFiles,XTestGoFiles,"
               "EmbedFiles,Deps,ForTest", *(["-tags", tags] if tags else []), "./..."]
    done = subprocess.run(command, cwd=root, capture_output=True, text=True, check=False)
    decoder = json.JSONDecoder()
    text, at, found = done.stdout, 0, []
    while at < len(text):
        while at < len(text) and text[at].isspace():
            at += 1
        if at >= len(text):
            break
        package, at = decoder.raw_decode(text, at)
        found.append(package)
    return found


def _files(package: dict) -> tuple:
    return tuple(tuple(package.get(field) or ()) for field in
                 ("GoFiles", "CgoFiles", "TestGoFiles", "XTestGoFiles", "EmbedFiles"))


@functools.cache
def changed_by(root: str, tags: str) -> frozenset[str] | None:
    """The directories of the module's packages whose tagged test build differs from the untagged
    one; None when `go list` could not say, and every package is then to be tested."""
    plain = {package["ImportPath"]: package for package in _listed(root, None)}
    tagged = {package["ImportPath"]: package for package in _listed(root, tags)}
    if not plain or not tagged:
        return None
    differing = {path for path in plain.keys() | tagged.keys()
                 if path not in plain or path not in tagged or _files(plain[path]) != _files(tagged[path])}
    changed = set()
    for path, package in tagged.items():
        reaches = {path, *(package.get("Deps") or ())}
        if reaches & differing and package.get("Dir"):
            changed.add(str(Path(package["Dir"]).resolve()))
    return frozenset(changed)


def tag_reached(root: str, directories: list[str], tags: str) -> list[str]:
    """Of directories (`./internal/x`, relative to root), those whose tagged test build differs, in
    their order: all of them when `go list` could not say."""
    changed = changed_by(root, tags)
    if changed is None:
        return directories
    return [directory for directory in directories
            if str((Path(root) / directory).resolve()) in changed]


def has_build_constraint(file: Path) -> bool:
    """Whether a Go file names a build constraint, which its revert may change; a gone file is asked
    as one that may have."""
    try:
        return "//go:build" in file.read_text(encoding="utf-8")
    except OSError:
        return True


def tagged_packages(root: str, path: str, dependents: list[str], tags: str) -> list[str]:
    """Of a Go file's own package and its dependents, those the tagged run tests: the ones the tag
    builds otherwise, and its own whenever the file has a build constraint, which its revert may
    change."""
    own = "./" + str(Path(path).parent)
    reached = tag_reached(root, [own, *dependents], tags)
    if own not in reached and has_build_constraint(Path(root) / path):
        reached.insert(0, own)
    return reached


def other_builds(path: str) -> list[Attempt]:
    """The runs beyond the default suite that `make check` also makes, for a file only they
    compile: the dev-tagged suite, and a compile for the operating system a file is named for;
    for TypeScript, the type check, since vitest drops types unchecked and a file of types alone
    (the SDK's generated API) can break nothing else."""
    if TS_SOURCE.search(path):
        return [Attempt(TYPECHECK, ", found by the type check", None, True)]
    if not GO_SOURCE.search(path):
        return []
    # Of its own package and those its revert can reach (wide_scope), those the tag builds otherwise.
    reached = tagged_packages(toplevel(), path, go_dependents(toplevel(), path), GO_EXTRA_TAGS)
    attempts = [Attempt(go_test(*reached, tags=GO_EXTRA_TAGS),
                        f", found by the -tags {GO_EXTRA_TAGS} suite")] if reached else []
    return [*attempts, *other_platforms(path)]
