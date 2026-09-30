#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# ABOUTME: Architecture map gate run by `make check`: every Go and web package has a row in the
# ABOUTME: map's package table and every row names a package that exists; schema.md's owners
# ABOUTME: table lists exactly the tables library.db and cache.db have.
"""Keep the architecture map and the owners table in step with the code.

The map (docs/contributors/design/architecture-map.md) says which package does which job; the
milestone audit re-derives it by hand (audit-checklist.md item 20). Two parts of that are
mechanical, and a map that silently misses a package or a table is the failure they catch:

1. **Packages.** Every Go package under cmd/ and internal/, and every web package and app, has
   a row in the map's package table (§1), named by its path in a code span; and every path a
   row names is a package that exists.
2. **Tables.** schema.md's "Who writes each table" lists each table of library.db (the newest
   schema golden) and cache.db (its one schema file) exactly once, and no table that is gone.
   Which package writes each table, against the one listed, is
   TestArch_EachTableHasOneWritingPackage's check: it type-checks the callers, which a text scan
   cannot do without mistaking a service method for a query of the same name.
"""

from __future__ import annotations

import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
MAP = "docs/contributors/design/architecture-map.md"
SCHEMA_PLAN = "docs/contributors/design/plans/schema.md"
GOLDENS = "internal/store/testdata/golden"
CACHE_SCHEMA = "internal/store/cache/schema.sql"

# The map's package table: from its heading to the next section's.
PACKAGE_SECTION = re.compile(r"^## 1\. Package table$(.*?)^## 2\.", re.M | re.S)
# A package path in a row's first cell.
PACKAGE_SPAN = re.compile(r"`((?:cmd|internal|web)/[\w./-]+)`")
# A table schema.md's owners table lists, in the same form the archtest reads.
OWNER_ROW = re.compile(r"^\| (`\w+`(?:, `\w+`)*) \| (?:library|cache)\.db \| ", re.M)
# A table the schema creates. FTS5's shadow tables are created with single-quoted names by the
# module itself, so they are not matched; sqlite_ tables are SQLite's own.
CREATE_TABLE = re.compile(r'^CREATE (?:VIRTUAL )?TABLE "?(\w+)"?', re.M)
GOLDEN_NAME = re.compile(r"^schema-v(\d+)\.sql$")


def go_packages(root: Path) -> set[str]:
    """Every directory under cmd/ and internal/ holding a Go file that is not a test."""
    packages = set()
    for top in ("cmd", "internal"):
        for source in (root / top).rglob("*.go"):
            relative = source.relative_to(root)
            if source.name.endswith("_test.go") or any(
                    part == "testdata" or part.startswith((".", "_")) for part in relative.parts):
                continue
            packages.add(relative.parent.as_posix())
    return packages


def web_packages(root: Path) -> set[str]:
    """Every web workspace package and app: a directory with a package.json."""
    return {manifest.parent.relative_to(root).as_posix()
            for pattern in ("web/packages/*/package.json", "web/apps/*/package.json")
            for manifest in root.glob(pattern)}


def mapped_packages(map_text: str) -> set[str]:
    """The package paths the map's package table names in its rows' first cells."""
    section = PACKAGE_SECTION.search(map_text)
    if section is None:
        return set()
    mapped = set()
    for line in section.group(1).splitlines():
        if line.startswith("| ") and not line.startswith("| ---"):
            first_cell = line.split(" | ", 1)[0]
            mapped.update(PACKAGE_SPAN.findall(first_cell))
    return mapped


def package_problems(existing: set[str], mapped: set[str]) -> list[str]:
    if not mapped:
        return [f"{MAP}: found no package table (\"## 1. Package table\"); nothing would be checked"]
    problems = [f"{MAP}: package {name} has no row in the package table; add one with its responsibility"
                for name in sorted(existing - mapped)]
    problems += [f"{MAP}: the package table names {name}, which is not a package; remove or rename its row"
                 for name in sorted(mapped - existing)]
    return problems


def schema_tables(*schemas: str) -> set[str]:
    return {name for schema in schemas for name in CREATE_TABLE.findall(schema)
            if not name.startswith("sqlite_")}


def listed_tables(schema_plan: str) -> list[str]:
    """The tables schema.md's owners table lists, in order, repeats included."""
    return [table.strip("`") for row in OWNER_ROW.findall(schema_plan) for table in row.split(", ")]


def table_problems(tables: set[str], listed: list[str]) -> list[str]:
    if not listed:
        return [f"{SCHEMA_PLAN}: found no owners table (\"Who writes each table\"); nothing would be checked"]
    problems = [f"{SCHEMA_PLAN}: table {name} has no row in the owners table; name its writing package"
                for name in sorted(tables - set(listed))]
    problems += [f"{SCHEMA_PLAN}: the owners table lists {name}, which no schema creates"
                 for name in sorted(set(listed) - tables)]
    problems += [f"{SCHEMA_PLAN}: the owners table lists {name} more than once"
                 for name in sorted({name for name in listed if listed.count(name) > 1})]
    return problems


def newest_golden(root: Path) -> Path:
    goldens = [(int(match.group(1)), path) for path in (root / GOLDENS).iterdir()
               if (match := GOLDEN_NAME.match(path.name))]
    if not goldens:
        raise SystemExit(f"check_architecture_map: no schema golden in {GOLDENS}")
    return max(goldens)[1]


def main() -> int:
    problems = package_problems(go_packages(ROOT) | web_packages(ROOT), mapped_packages((ROOT / MAP).read_text()))
    tables = schema_tables(newest_golden(ROOT).read_text(), (ROOT / CACHE_SCHEMA).read_text())
    problems += table_problems(tables, listed_tables((ROOT / SCHEMA_PLAN).read_text()))
    for problem in problems:
        print(problem)
    if problems:
        print(f"check_architecture_map: {len(problems)} problem(s)")
        return 1
    print("check_architecture_map: every package has a row in the map, and the owners table lists every table")
    return 0


if __name__ == "__main__":
    sys.exit(main())
