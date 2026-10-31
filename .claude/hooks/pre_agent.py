#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: PreToolUse hook on Agent: refuses a launch that names no model, or one past the agents allowed to
# ABOUTME: run at once (the owner's usage rules, 2026-10-26, applied by no one on 2026-10-31).
"""An agent's cost is its calls times its context, and the owner's quota went twice to agents launched on
the default model, up to seven at once (docs/contributors/lead.md §1-2). A rule the lead had to remember
was not applied in fifteen launches (2026-10-31), so the launch itself checks."""
from __future__ import annotations

import os

import agents
import hooklib

# How many agents may run at once (the owner, 2026-10-26: "at most 2-3"). ORPHEUM_MOST_AGENTS overrides it.
MOST_AGENTS = 3


def refusal(event: dict[str, object], running: list[str]) -> str | None:
    tool_input = event.get("tool_input")
    launch = tool_input if isinstance(tool_input, dict) else {}
    if launch.get("subagent_type") != "fork" and not launch.get("model"):
        return ("Name the agent's model (docs/contributors/lead.md §2): `sonnet` for mechanical work and briefs "
                "naming the files, `opus` only for design-heavy or wide-reaching work, `haiku` for lookups.")
    most = int(os.environ.get("ORPHEUM_MOST_AGENTS") or MOST_AGENTS)
    if len(running) >= most:
        return (f"{len(running)} agents are running ({', '.join(running)}), the most at once is {most} "
                "(docs/contributors/lead.md §1). Wait for one to finish, or do the change yourself.")
    return None


def main() -> None:
    event = hooklib.read_event()
    said = refusal(event, agents.running())
    if said:
        hooklib.deny(said)


if __name__ == "__main__":
    main()
