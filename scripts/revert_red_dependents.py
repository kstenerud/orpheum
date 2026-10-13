# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: The tests a change can reach beyond its own: the Go packages that import the changed one,
# ABOUTME: and the tests that read the sources themselves rather than import them.
"""Which tests a reverted file can turn red, beyond its own package's or its importers' by vitest.

A file that survived its own tests widened to the whole suite: `go test ./...` and again with the
dev tag, 55 s each, and the whole web suite, 69 s, for files such as a `doc.go` whose revert nothing
could notice (check-pr's timings, 2026-10-13). The owner: "Our tests should be intelligent enough and
compartmentalized enough that we can test only the parts actually affected by a change." A Go file
can reach only the packages that import its own, directly or not, and the tests that read the
sources themselves (go/parser, go/ast, a `go build` of the binary); a TypeScript file only the tests
that import it, which vitest's `related` already finds, and the tests that read the sources by path.
"""

from __future__ import annotations

import functools
import re
import subprocess
from pathlib import Path

# The packages whose tests read every package's sources rather than import what they check
# (go/parser over the module), so any Go file's revert can turn them red. Others that parse source
# (internal/core/api's errors_test, internal/store's sqlcgen_test) read only their own directory's.
GO_SOURCE_READERS = ("./internal/archtest",)
# A Go test that builds the binary, and so compiles every package: none today.
GO_BINARY_BUILDER = re.compile(r'exec\.Command\("go"|"go", "build"')
# A web test that reads source files by path rather than importing them.
WEB_SOURCE_READER = re.compile(r"readFileSync|readdirSync|import\.meta\.glob")
# The Go build tags make check also tests with, whose files may import a package the untagged
# build does not.
GO_TAG_SETS = ((), ("dev",))


def toplevel() -> str:
    """The working tree's root."""
    return subprocess.run(["git", "rev-parse", "--show-toplevel"], capture_output=True, text=True,
                          check=False).stdout.strip()


def _run(command: list[str], root: str) -> str:
    done = subprocess.run(command, cwd=root, capture_output=True, text=True, check=False)
    return done.stdout


@functools.cache
def go_module(root: str) -> str:
    mod = Path(root) / "go.mod"
    if not mod.exists():
        return ""
    for line in mod.read_text(encoding="utf-8").splitlines():
        if line.startswith("module "):
            return line.split()[1]
    return ""


@functools.cache
def go_test_graph(root: str) -> dict[str, frozenset[str]]:
    """Each package's directory under root, "./x/y", to every import path its code and its tests
    depend on, directly or not, under every tag set make check builds with."""
    module = go_module(root)
    graph: dict[str, set[str]] = {}
    if not module:
        return {}
    for tags in GO_TAG_SETS:
        command = ["go", "list", "-e", "-test", *(["-tags", ",".join(tags)] if tags else []),
                   "-f", "{{.ImportPath}}\t{{join .Deps \" \"}}\t{{join .Imports \" \"}}", "./..."]
        for line in _run(command, root).splitlines():
            name, _, rest = line.partition("\t")
            deps, _, imports = rest.partition("\t")
            # A test variant is "p [p.test]", "p_test [p.test]" or "p.test"; each is p's.
            path = name.split(" ")[0].removesuffix(".test").removesuffix("_test")
            if not path.startswith(module):
                continue
            directory = "./" + path.removeprefix(module).lstrip("/") if path != module else "."
            graph.setdefault(directory, set()).update(deps.split(), imports.split())
    return {directory: frozenset(deps) for directory, deps in graph.items()}


@functools.cache
def go_source_readers(root: str) -> tuple[str, ...]:
    """The packages, "./x/y", whose tests read every package's Go sources or build the binary."""
    done = subprocess.run(["git", "grep", "-lE", GO_BINARY_BUILDER.pattern, "--", "*_test.go"], cwd=root,
                          capture_output=True, text=True, check=False)
    builders = {"./" + str(Path(path).parent) for path in done.stdout.split()}
    return tuple(sorted(builders | set(GO_SOURCE_READERS)))


def go_dependents(root: str, path: str) -> list[str]:
    """The packages, "./x/y", other than the file's own, that import its package, directly or not, or
    whose tests read the sources: every package a Go file's revert can turn red beyond its own."""
    module = go_module(root)
    own = "./" + str(Path(path).parent)
    import_path = module + own.removeprefix(".")
    reached = {directory for directory, deps in go_test_graph(root).items() if import_path in deps}
    return sorted((reached | set(go_source_readers(root))) - {own})


@functools.cache
def web_source_readers(root: str) -> tuple[str, ...]:
    """The web test files, relative to web/, that read source files by path."""
    done = subprocess.run(["git", "grep", "-lE", WEB_SOURCE_READER.pattern, "--", "web/*.test.ts",
                           "web/*.test.tsx"], cwd=root, capture_output=True, text=True, check=False)
    return tuple(sorted(path.removeprefix("web/") for path in done.stdout.split()
                        if "/node_modules/" not in path))
