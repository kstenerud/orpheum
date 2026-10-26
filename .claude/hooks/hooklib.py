# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Shared plumbing for the Claude Code hooks: read the event, find the project root,
# ABOUTME: and emit the JSON decisions Claude Code understands.
"""Helpers for the hooks in this directory. Stdlib only; each hook stays a few lines."""

from __future__ import annotations

import json
import os
import sys
from pathlib import Path
from typing import Any

ROOT = Path(os.environ.get("CLAUDE_PROJECT_DIR") or Path(__file__).resolve().parents[2])
sys.path.insert(0, str(ROOT / "scripts"))


def _git_dirs(directory: Path) -> tuple[Path, Path] | None:
    """The checkout directory holds and its repository's common git directory, or None outside one."""
    import subprocess  # noqa: PLC0415 — only the hooks that resolve a path pay for it

    done = subprocess.run(["git", "-C", str(directory), "rev-parse", "--show-toplevel", "--git-common-dir"],
                          capture_output=True, text=True, check=False)
    lines = done.stdout.splitlines()
    if done.returncode != 0 or len(lines) != 2:
        return None
    top = Path(lines[0]).resolve()
    common = Path(lines[1])
    return top, (common if common.is_absolute() else directory / common).resolve()


def root_of(path: str | Path) -> Path:
    """The checkout path is in, when it is a worktree of this project's own repository; ROOT
    otherwise. Claude Code puts an agent's worktree at .claude/worktrees/<name>/, inside ROOT, so
    taken from ROOT every file in it read as .claude/… and was refused as a gate, and an agent wrote
    through Python instead, which no hook sees (2026-10-26). Only a linked worktree of the same
    repository counts: a repository made inside the project (`git init scripts`) has another common
    directory, and its files stay judged from ROOT, gates and all."""
    directory = Path(path).resolve()
    while not directory.is_dir() and directory != directory.parent:
        directory = directory.parent
    found, project = _git_dirs(directory), _git_dirs(ROOT)
    if found is None or project is None or found[1] != project[1]:
        return ROOT
    return found[0]


def read_event() -> dict[str, Any]:
    try:
        event = json.load(sys.stdin)
    except (json.JSONDecodeError, ValueError):
        return {}
    return event if isinstance(event, dict) else {}


def deny(reason: str) -> None:
    """Refuse the tool call. Enforced even when the session bypasses permission prompts."""
    json.dump(
        {
            "hookSpecificOutput": {
                "hookEventName": "PreToolUse",
                "permissionDecision": "deny",
                "permissionDecisionReason": reason,
            }
        },
        sys.stdout,
    )


def owner_allows(variable: str) -> bool:
    """An override the owner sets when launching a session for gate or dependency work.

    It is read from Claude Code's own environment, which a command the agent runs cannot
    change.
    """
    return os.environ.get(variable) == "1"


def check_pr_running() -> dict[str, str] | None:
    """The check-pr run that holds this project's lock (scripts/check_pr_snapshot.py), as it recorded
    itself; None when none does. ORPHEUM_HOOKS_IGNORE_CHECK_PR=1, which only the hooks' tests set,
    says none does, so their verdicts never hang on a run elsewhere on the machine."""
    if os.environ.get("ORPHEUM_HOOKS_IGNORE_CHECK_PR") == "1":
        return None
    import check_pr_snapshot  # noqa: PLC0415 — only the hooks that ask pay for it

    return check_pr_snapshot.running(ROOT)
