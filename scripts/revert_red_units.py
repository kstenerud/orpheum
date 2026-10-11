# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: What revert-red (check_revert_red.py) reverts together with a changed file: a
# ABOUTME: generated query file with its SQL, a migration step with its checksum line and golden.
"""The files one revert-red probe undoes as one unit.

Some changes are made of files that mean nothing apart. Undoing one alone either undoes nothing
a test reads, or breaks a check that has nothing to do with the change:

* **A generated query file and its SQL.** sqlc writes `sqlcgen/<name>.sql.go` from
  `queries/<name>.sql`, and a test such as the reference scale reads the SQL, so undoing the Go
  alone undoes nothing it checks.
* **A migration step, its line in SHA256SUMS, and its schema golden.** Undoing the step alone
  fails the checksum test whatever the step does, which would count as covered when nothing
  tested the step (audit-2026-09-29 §6). Its line in SHA256SUMS is undone with it, and only its
  line: another step the branch changed keeps its own. The golden `schema-v<NNNN>.sql` is the
  schema the step leads to, so it is undone too, and what is left to fail is a test that runs
  the steps.
"""

from __future__ import annotations

import re
import subprocess
from pathlib import Path

# Where each generated query file's source is: sqlc writes sqlcgen/<name>.sql.go from
# queries/<name>.sql, and cachegen/<name>.sql.go from cache/queries/<name>.sql.
QUERY_SOURCES = {
    "internal/store/sqlcgen/": "internal/store/queries/",
    "internal/store/cachegen/": "internal/store/cache/queries/",
}
MIGRATIONS = "internal/store/migrations/"
MIGRATION_STEP = re.compile(r"^internal/store/migrations/(\d{4})_[^/]+\.sql$")
MIGRATION_SUMS = MIGRATIONS + "SHA256SUMS"
SCHEMA_GOLDEN = "internal/store/testdata/golden/schema-v{version}.sql"


def _git(*args: str, cwd: str | None = None) -> subprocess.CompletedProcess[str]:
    return subprocess.run(["git", *args], capture_output=True, text=True, cwd=cwd, check=False)


def query_source(path: str) -> str | None:
    """The SQL a generated query file is written from, which a test such as the reference scale's
    reads instead of the Go: undoing the generated file alone undoes nothing it checks, so both
    are undone together."""
    for generated, sources in QUERY_SOURCES.items():
        if path.startswith(generated) and path.endswith(".sql.go"):
            return sources + path[len(generated):-len(".go")]
    return None


def companions(path: str) -> list[str]:
    """The files undone with `path` in one probe, beyond `path` itself."""
    source = query_source(path)
    if source is not None:
        return [source]
    step = MIGRATION_STEP.match(path)
    if step:
        return [MIGRATION_SUMS, SCHEMA_GOLDEN.format(version=step.group(1))]
    return []


def reverted_sums(base_sums: str, head_sums: str, step: str) -> str:
    """head's SHA256SUMS with only `step`'s line as base had it: gone when base had none."""
    def names(line: str) -> str:
        return line.split()[-1] if line.split() else ""

    base_line = next((line for line in base_sums.splitlines() if names(line) == step), None)
    lines = []
    for line in head_sums.splitlines():
        if names(line) != step:
            lines.append(line)
        elif base_line is not None:
            lines.append(base_line)
    return "\n".join(lines) + "\n" if lines else ""


def revert_to(tree: str, base: str, path: str) -> None:
    """Puts path in the worktree as base had it, or removes it when base had none."""
    if _git("cat-file", "-e", f"{base}:{path}", cwd=tree).returncode == 0:
        _git("checkout", base, "--", path, cwd=tree)
    else:
        (Path(tree) / path).unlink(missing_ok=True)


def revert_companion(tree: str, base: str, path: str, companion: str) -> None:
    """Undoes `companion` as `path`'s unit needs: SHA256SUMS by `path`'s line alone, any other
    file whole."""
    if companion != MIGRATION_SUMS:
        revert_to(tree, base, companion)
        return
    sums = Path(tree) / companion
    base_sums = _git("show", f"{base}:{companion}", cwd=tree).stdout
    head_sums = sums.read_text() if sums.exists() else ""
    sums.write_text(reverted_sums(base_sums, head_sums, Path(path).name))


def restore(tree: str, head: str, path: str) -> None:
    """Puts path back as head has it, or removes it when head has none."""
    if _git("checkout", head, "--", path, cwd=tree).returncode != 0:
        (Path(tree) / path).unlink(missing_ok=True)
