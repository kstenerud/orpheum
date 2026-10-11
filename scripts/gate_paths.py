# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: The one list of paths that are gates or generated output. The Claude Code hooks
# ABOUTME: refuse agent edits to them, and the branch gate requires a trailer when they change.
"""Which files an agent may not change on its own, and why.

Three classes, each with its own remedy:

* **Gates** — the configuration and scripts that decide what "passing" means. Loosening one is
  the cheapest way to make failing code pass, so changing one is the owner's act. The pre-edit
  hook refuses it unless the session was started with `ORPHEUM_ALLOW_GATE_EDIT=1`. On a
  branch, a change needs a `Gate-Changed: <why>` trailer (`check_test_weakening.py`).
* **Generated** — output of a generator. The remedy is to edit the source and regenerate.
* **Committed migrations** — append-only (D181). The remedy is the next migration. Until
  migrations are live (D228) the baseline is the exception: it is rewritten by each fold, until
  production runs (D272 §5). From then on the checkpoint holds it, and only the owner moves that.

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
    # The schema version production last ran: the baseline is held at it (D272 §6).
    r"^internal/store/migrations/CHECKPOINT$",
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
# Its presence says migrations are live (D228); internal/store's steps_test.go names it too.
MIGRATIONS_LIVE_MARKER = MIGRATIONS_DIR + "LIVE"
# Its presence says production runs, at the version it holds (D272 §6); steps_test.go names it too.
MIGRATIONS_CHECKPOINT = MIGRATIONS_DIR + "CHECKPOINT"
# The line that marks a step as the baseline (internal/store/steps.go).
BASELINE_DIRECTIVE = "-- orpheum:baseline folds"
# persistent.db's schema goldens: the baseline's and the current version's (D228).
SCHEMA_GOLDENS_DIR = "internal/store/testdata/golden/"

_GATE = [re.compile(p) for p in GATE_PATTERNS]
_GENERATED = [re.compile(p) for p in GENERATED_PATTERNS]


def is_gate(rel: str) -> bool:
    return any(p.search(rel) for p in _GATE)


def is_generated(rel: str) -> bool:
    return any(p.search(rel) for p in _GENERATED)


def _git_show(root: Path, rev: str, rel: str) -> str | None:
    """The file's text at `rev`, or None when git does not have it there."""
    done = subprocess.run(
        ["git", "show", f"{rev}:{rel}"], cwd=root, capture_output=True, text=True, check=False
    )
    return done.stdout if done.returncode == 0 else None


def migrations_are_live(root: Path, rev: str | None = None) -> bool:
    """Whether the owner has declared migrations live (D228) by committing the marker: at `rev`,
    or, with no `rev`, in the working tree or at HEAD. Until then the baseline is folded forward
    and may be edited; from then on every committed step is frozen."""
    if rev is None and (root / MIGRATIONS_LIVE_MARKER).exists():
        return True
    return _git_show(root, rev or "HEAD", MIGRATIONS_LIVE_MARKER) is not None


def production_runs(root: Path) -> bool:
    """Whether a checkpoint says production runs (D272 §6), in the working tree or at HEAD. From
    then the baseline is the version production last ran, frozen until the owner moves it."""
    return (root / MIGRATIONS_CHECKPOINT).exists() or _git_show(root, "HEAD", MIGRATIONS_CHECKPOINT) is not None


def is_committed_migration(rel: str, root: Path) -> bool:
    """A migration step that git already tracks at HEAD; a new one may still be written. Until
    migrations are live, the baseline is not one before production runs: each fold rewrites it
    (D228). The steps after it are, because a database has applied them; and while production
    runs the baseline is too, since production's database recorded it (D272 §5)."""
    if not rel.startswith(MIGRATIONS_DIR) or not rel.endswith(".sql"):
        return False
    committed = _git_show(root, "HEAD", rel)
    if committed is None:
        return False
    is_baseline = any(line.strip().startswith(BASELINE_DIRECTIVE) for line in committed.splitlines())
    return not is_baseline or migrations_are_live(root) or production_runs(root)


def relative(path: str, root: Path) -> str | None:
    """`path` relative to the project root, or None when it lies outside the project."""
    try:
        return Path(path).resolve().relative_to(root.resolve()).as_posix()
    except ValueError:
        return None
