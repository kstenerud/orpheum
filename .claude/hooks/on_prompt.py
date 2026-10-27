#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: UserPromptSubmit hook: puts the owner's standing orders in front of the agent at every prompt
# ABOUTME: (process review F13), and sweeps away what stopped subagents left running.
from __future__ import annotations

import json
import subprocess
import sys

import agents
import hooklib

STANDING_ORDERS = hooklib.ROOT / ".claude" / "standing-orders.md"
# A page at most: it rides on every prompt.
MOST_CHARACTERS = 5000


def standing_orders() -> str:
    """The standing orders, whole or cut at MOST_CHARACTERS with a line saying so: cut silently at
    3000, their last items were dropped from every prompt unseen (2026-10-27)."""
    if not STANDING_ORDERS.exists():
        return ""
    orders = STANDING_ORDERS.read_text(encoding="utf-8").strip()
    if not orders:
        return ""
    if len(orders) > MOST_CHARACTERS:
        orders = (orders[:MOST_CHARACTERS] + f"\n[cut: the standing orders are {len(orders)} characters, over "
                  f"{MOST_CHARACTERS}; shorten them, moving what is done out]")
    return "The owner's standing orders (.claude/standing-orders.md):\n" + orders


def sweep() -> str:
    """Ends every process a stopped subagent left (agents.end_left_by_stopped), and names the
    session's own shells that have run for hours; what it did, for the session to read."""
    said = []
    ended = agents.end_left_by_stopped()
    if ended:
        said.append(f"Ended {len(ended)} process(es) stopped subagents had left running: pids "
                    + ", ".join(map(str, ended)) + ".")
    old = agents.old_untagged_shells()
    if old:
        said.append("Shells of this session running for hours (stop them if nothing needs them):\n- "
                    + "\n- ".join(old))
    return "\n".join(said)


def main() -> None:
    hooklib.read_event()
    try:
        swept = sweep()
    except (OSError, subprocess.SubprocessError) as failed:  # a failed sweep says so, never stops the prompt
        swept = f"The sweep for stopped subagents' processes failed: {failed}"
    context = "\n\n".join(part for part in (standing_orders(), swept) if part)
    if context:
        json.dump({"hookSpecificOutput": {"hookEventName": "UserPromptSubmit", "additionalContext": context}},
                  sys.stdout)


if __name__ == "__main__":
    main()
