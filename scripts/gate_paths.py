# SPDX-License-Identifier: GPL-3.0-or-later
# ABOUTME: The one list of paths that are gates or generated output. The Claude Code hooks
# ABOUTME: refuse agent edits to them, and the branch gate requires a trailer when they change.
"""Which files an agent may not change on its own, and why.

Three classes, each with its own remedy:

* **Gates** — the configuration and scripts that decide what "passing" means. Loosening one is
  the cheapest way to make failing code pass, so changing one is the owner's act. The pre-edit
  hook refuses it unless the session was started with `ORPHEUM_ALLOW_GATE_EDIT=1`. On a
  branch, a change needs a `Gate-Changed: <why>` trailer (`check_test_weakening.py`).
* **Generated** — output of a generator. The remedy is to edit the source and regenerate.
* **Committed migrations** — append-only (D181). The remedy is the next migration.

Kept in one module so the hooks and the branch gate cannot disagree about the list.
"""

from __future__ import annotations

import re
import subprocess
from pathlib import Path

GATE_PATTERNS = (
    r"^Makefile$",
    r"^\.golangci\.ya?ml$",
    r"^sqlc\.ya?ml$",
    r"^scripts/",
    r"^\.claude/",
    r"^\.githooks/",
    r"^\.github/",
    r"^deps/",
    r"^internal/archtest/",
    r"(^|/)testdata/(golden|baselines?)/",
    r"^web/eslint\.config\.[cm]?js$",
    r"^web/\.dependency-cruiser\.[cm]?js$",
    r"^web/\.prettierrc",
    r"^web/(.+/)?tsconfig[^/]*\.json$",
    r"^web/(.+/)?vitest\.config\.[cm]?[jt]s$",
)
GENERATED_PATTERNS = (
    r"^internal/store/sqlcgen/",
    r"^internal/store/cachegen/",
    r"^web/packages/sdk/src/gen/",
    r"^api/openapi\.json$",
    r"(^|/)package-lock\.json$",
)
MIGRATIONS_DIR = "internal/store/migrations/"

_GATE = [re.compile(p) for p in GATE_PATTERNS]
_GENERATED = [re.compile(p) for p in GENERATED_PATTERNS]


def is_gate(rel: str) -> bool:
    return any(p.search(rel) for p in _GATE)


def is_generated(rel: str) -> bool:
    return any(p.search(rel) for p in _GENERATED)


def is_committed_migration(rel: str, root: Path) -> bool:
    """A migration file that git already tracks at HEAD; a new one may still be written."""
    if not rel.startswith(MIGRATIONS_DIR) or not rel.endswith(".sql"):
        return False
    done = subprocess.run(
        ["git", "cat-file", "-e", f"HEAD:{rel}"], cwd=root, capture_output=True, check=False
    )
    return done.returncode == 0


def relative(path: str, root: Path) -> str | None:
    """`path` relative to the project root, or None when it lies outside the project."""
    try:
        return Path(path).resolve().relative_to(root.resolve()).as_posix()
    except ValueError:
        return None
