#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: PostToolUse hook on every tool: counts a subagent's tool calls against the budget its brief
# ABOUTME: states, warns it to land the stage it is in, and past the budget tells it to commit, report and stop.
"""Build agents ran 200-640 tool calls each, their context growing with every one, and every call re-reads
it all (the token measurement, 2026-10-31). A brief states its budget (scripts/call_budget.py; DEFAULT_BUDGET
when it states none). Stopped at the budget mid-stage, two schema agents left work uncommitted for a
finisher, and one handoff lost a line (2026-11-05): so the agent is warned at WARN_AT of it to land its
stage, and at the budget reports what the rest needs, so the lead resumes it or briefs a fresh one."""
from __future__ import annotations

import json
import math
import sys
from pathlib import Path

import agents
import hooklib
from call_budget import CEILING_TIMES, DEFAULT_BUDGET, stated_budgets

CALL_REMINDER = 25
# The share of the budget at which the agent is told to finish the stage it is in.
WARN_AT = 0.8


def warning(calls: int, budget: int, warned: bool) -> str | None:
    """The one warning, due from WARN_AT of the budget until the budget itself."""
    if warned or calls < math.ceil(WARN_AT * budget) or calls >= budget:
        return None
    return (f"This is your tool call {calls} of a budget of {budget}: about {budget - calls} left. Finish and "
            "commit the stage you are in; start another only if it fits.")


def reminder(calls: int, reminded: int = 0, budget: int = DEFAULT_BUDGET, at_ceiling: bool = False) -> str | None:
    """The reminder due at this call, given the call last reminded at (0: never). Due from the budget, then
    CALL_REMINDER calls after the last, so a count that skips a number still reminds."""
    if calls < budget or (reminded and calls - reminded < CALL_REMINDER):
        return None
    after = ("This is the ceiling: no extension follows, and a fresh agent takes the rest." if at_ceiling else
             "The lead resumes you, or briefs a fresh agent, from your report.")
    return (f"This is your tool call {calls}; your budget is {budget}. Commit what works, then report what is "
            f"done, what is left, and your estimate of the calls the rest needs, and stop. {after}")


def transcript_of(event: dict, agent_id: str) -> Path | None:
    """The agent's own transcript: Claude Code names the session's in transcript_path, and keeps each
    subagent's beside it, as <session>/subagents/agent-<id>.jsonl."""
    for key in ("agent_transcript_path", "transcript_path"):
        named = event.get(key)
        if named and Path(str(named)).name == f"agent-{agent_id}.jsonl":
            return Path(str(named))
    session = event.get("transcript_path")
    return Path(str(session)).with_suffix("") / "subagents" / f"agent-{agent_id}.jsonl" if session else None


def budget_of(event: dict, agent_id: str, recorded: dict) -> tuple[int, bool]:
    """This run's budget, and whether it reaches the ceiling. Read from the transcript once a run (a resume
    starts a run, and may state a new budget) and kept in the agent's record."""
    first = int(recorded.get("first_budget") or 0)
    ceiling = CEILING_TIMES * (first or DEFAULT_BUDGET)
    spent = int(recorded.get("spent") or 0)
    if recorded.get("budget"):
        budget = int(recorded["budget"])
        return budget, spent + budget >= ceiling
    path = transcript_of(event, agent_id)
    stated = stated_budgets(path) if path else None
    if stated is None:
        return DEFAULT_BUDGET, False  # not yet readable: asked again at the next call
    first = first or (stated[0] if stated else DEFAULT_BUDGET)
    ceiling = CEILING_TIMES * first
    budget = max(1, min(stated[-1] if stated else first, ceiling - spent))
    agents.note(agent_id, budget=budget, first_budget=first)
    return budget, spent + budget >= ceiling


def main() -> None:
    event = hooklib.read_event()
    agent_id = event.get("agent_id")
    if not agents.is_agent_id(agent_id):
        return
    agent_id = str(agent_id)
    calls, reminded = agents.count_call(agent_id)
    recorded = agents.state(agent_id)
    budget, at_ceiling = budget_of(event, agent_id, recorded)
    said = reminder(calls, reminded, budget, at_ceiling)
    if said:
        agents.mark_reminded(agent_id, calls)
    else:
        said = warning(calls, budget, bool(recorded.get("warned")))
        if said:
            agents.note(agent_id, warned=calls)
    if said:
        json.dump({"hookSpecificOutput": {"hookEventName": "PostToolUse", "additionalContext": said}}, sys.stdout)


if __name__ == "__main__":
    main()
