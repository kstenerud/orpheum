# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: An agent's call budget, as its brief and the lead's messages state it: read by the hook that
# ABOUTME: tells an agent when to stop, and by the token ledger that sets each budget against the calls made.
"""A brief states its agent's call budget on a line of its own, `Call budget: 300`; a message resuming the
agent may state a new one. One budget for every brief was too small for the schema rounds, two of three
ending mid-stage with their work uncommitted, and too large for a small fix (the owner, 2026-11-05)."""
from __future__ import annotations

import json
import re
from pathlib import Path

# The budget of a brief that states none.
DEFAULT_BUDGET = 150
# An agent's calls over all its runs never pass this many times its brief's budget, however a resume
# extends it: the runs of 200-640 calls this budget exists to end (the token measurement, 2026-10-31).
CEILING_TIMES = 2
BUDGET_LINE = re.compile(r"^Call budget:\s*(\d+)\s*$", re.MULTILINE)


def stated_in(text: str) -> list[int]:
    return [int(found) for found in BUDGET_LINE.findall(text)]


def said(message: object) -> str:
    """The words of a message the agent was given: its text, never a tool's result, which may quote a brief."""
    if not isinstance(message, dict):
        return ""
    content = message.get("content")
    if isinstance(content, str):
        return content
    if isinstance(content, list):
        return "\n".join(str(part.get("text", "")) for part in content
                         if isinstance(part, dict) and part.get("type") == "text")
    return ""


def stated_budgets(transcript: Path) -> list[int] | None:
    """Every budget the agent's transcript states in a message given to it, in order; None when the
    transcript cannot be read (not yet written: the budget is asked again at the next call)."""
    try:
        lines = transcript.open(encoding="utf-8", errors="replace")
    except OSError:
        return None
    budgets: list[int] = []
    with lines:
        for line in lines:
            if '"user"' not in line or "Call budget" not in line:
                continue
            try:
                entry = json.loads(line)
            except ValueError:
                continue
            if isinstance(entry, dict) and entry.get("type") == "user":
                budgets += stated_in(said(entry.get("message")))
    return budgets
