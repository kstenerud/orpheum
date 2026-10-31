#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: PostToolUse hook on every tool: counts a subagent's tool calls and, past the size a brief is
# ABOUTME: meant to fit in, tells it to commit, report and stop.
"""Build agents ran 200-640 tool calls each, their context growing with every one, and every call re-reads
it all (the token measurement, 2026-10-31). A brief is sized for about CALL_LIMIT calls; past it the agent
is told so, again every CALL_REMINDER calls, rather than trusted to notice."""
from __future__ import annotations

import json
import sys

import agents
import hooklib

CALL_LIMIT = 150
CALL_REMINDER = 25


def reminder(calls: int) -> str | None:
    if calls < CALL_LIMIT or (calls - CALL_LIMIT) % CALL_REMINDER:
        return None
    return (f"This is your tool call {calls}; a brief is sized for about {CALL_LIMIT}. Commit what works, "
            "report what is done and what is left, and stop: the lead splits the rest or finishes it.")


def main() -> None:
    event = hooklib.read_event()
    agent_id = event.get("agent_id")
    if not agent_id:
        return
    said = reminder(agents.count_call(str(agent_id)))
    if said:
        json.dump({"hookSpecificOutput": {"hookEventName": "PostToolUse", "additionalContext": said}}, sys.stdout)


if __name__ == "__main__":
    main()
