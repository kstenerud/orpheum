# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: For a probe of Go code, the packages that use what its change touched: a generated query file's
# ABOUTME: changed queries, or any file's changed exported names; run before every dependent's tests.
"""The callers' run of a Go file's probe (check_revert_red.attempts_for).

A sqlc file's revert is red in the tests of the code that calls the query it changed, nearly
always, and those were reached only after the store's integration tier and every dependent
package's tests: series.sql.go's probe took 291 s, 148 of them green in the store's tier
(check-pr, 2026-10-21). The changed queries are read from the two versions of the file: each
query is the stretch from its `-- name:` constant to the next, holding its SQL, its row and
parameter types and its method, and a query whose stretch differs, or is new, changed. Their
callers are the packages whose own Go code, outside the generated code, calls one by name.

A green run here settles nothing: the probe goes on to its usual runs, so nothing is passed on
less than before.
"""

from __future__ import annotations

import re
import subprocess

QUERY_START = re.compile(r"^const \w+ = `-- name: (\w+) ", re.MULTILINE)
GENERATED_DIRS = ("internal/store/sqlcgen", "internal/store/cachegen")


def queries_of(source: str) -> dict[str, str]:
    """Each query of a generated file, by its name: the stretch of code from its constant to the
    next query's."""
    starts = list(QUERY_START.finditer(source))
    return {match.group(1): source[match.start(): starts[index + 1].start() if index + 1 < len(starts) else None]
            for index, match in enumerate(starts)}


def changed_queries(before: str, after: str) -> list[str]:
    """The queries whose code differs between the two versions, or that only the newer has."""
    old, new = queries_of(before), queries_of(after)
    return sorted(name for name, code in new.items() if old.get(name) != code)


def caller_packages(root: str, rev: str, names: list[str]) -> list[str]:
    """The packages, as `./dir`, whose Go code at rev outside the generated code calls one of the
    named methods: `.Name(`."""
    if not names:
        return []
    pattern = r"\.(" + "|".join(re.escape(name) for name in names) + r")\("
    found = subprocess.run(
        ["git", "grep", "-l", "-E", pattern, rev, "--", "*.go", *(f":!{d}" for d in GENERATED_DIRS)],
        cwd=root, capture_output=True, text=True, check=False).stdout
    dirs = set()
    for line in found.splitlines():
        path = line.split(":", 1)[1] if ":" in line else line
        dirs.add(path.rsplit("/", 1)[0] if "/" in path else ".")
    return sorted("./" + d for d in dirs)


def callers_of_change(root: str, base: str, head: str, path: str) -> list[str]:
    """The packages that call the queries a generated file's change touched, by the two versions in
    git; none when either cannot be read."""
    before, after = versions_of(root, base, head, path)
    if after is None:
        return []
    return caller_packages(root, head, changed_queries(before or "", after))


def versions_of(root: str, base: str, head: str, path: str) -> tuple[str | None, str | None]:
    """A file's text at base and at head, None where it has none."""
    def at(rev: str) -> str | None:
        shown = subprocess.run(["git", "show", f"{rev}:{path}"], cwd=root, capture_output=True, text=True,
                               check=False)
        return shown.stdout if shown.returncode == 0 else None

    return at(base), at(head)


# A top-level declaration's first line: a function or method, a type, or a var or const, alone or
# opening a group.
DECLARATION = re.compile(r"^(?:func (?:\([^)]*\) )?(\w+)|type (\w+)|(?:var|const) (\w+|\())", re.MULTILINE)
GROUPED_NAME = re.compile(r"^\t(\w+)", re.MULTILINE)


def declarations_of(source: str) -> dict[str, str]:
    """Each top-level declaration of a Go file, by its name: the stretch from its first line to the
    next declaration's; a grouped var or const block gives each name it declares the whole block."""
    starts = list(DECLARATION.finditer(source))
    found: dict[str, str] = {}
    for index, match in enumerate(starts):
        code = source[match.start(): starts[index + 1].start() if index + 1 < len(starts) else None]
        name = match.group(1) or match.group(2) or match.group(3)
        names = GROUPED_NAME.findall(code.split(")\n", 1)[0]) if name == "(" else [name]
        for each in names:
            found[each] = found.get(each, "") + code
    return found


def changed_exported_names(before: str, after: str) -> list[str]:
    """The exported names whose declarations differ between the two versions, or that only one has:
    what another package can use, and so what a revert can break there."""
    old, new = declarations_of(before), declarations_of(after)
    names = {name for name in old.keys() | new.keys() if name[:1].isupper() and old.get(name) != new.get(name)}
    return sorted(names)


def packages_using(root: str, rev: str, path: str, names: list[str], among: list[str]) -> list[str]:
    """The packages among among, as `./dir`, whose Go code at rev outside the file's own package uses
    one of names as a word."""
    if not names or not among:
        return []
    pattern = r"\b(" + "|".join(re.escape(name) for name in names) + r")\b"
    own = "./" + path.rsplit("/", 1)[0]
    found = subprocess.run(["git", "grep", "-l", "-E", pattern, rev, "--", "*.go"], cwd=root, capture_output=True,
                           text=True, check=False).stdout
    using = set()
    for line in found.splitlines():
        file = line.split(":", 1)[1] if ":" in line else line
        using.add("./" + (file.rsplit("/", 1)[0] if "/" in file else "."))
    return sorted((using & set(among)) - {own})


def users_of_change(root: str, base: str, head: str, path: str, dependents: list[str]) -> list[str]:
    """The dependents of a Go file's package that use an exported name its change touched, by the two
    versions in git; none when its head cannot be read."""
    before, after = versions_of(root, base, head, path)
    if after is None:
        return []
    return packages_using(root, head, path, changed_exported_names(before or "", after), dependents)
