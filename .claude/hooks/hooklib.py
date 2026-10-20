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
