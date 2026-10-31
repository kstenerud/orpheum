#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: PreToolUse hook on Agent: refuses a launch that names no model, or one past the agents allowed to
# ABOUTME: run at once (the owner's usage rules, 2026-10-26, applied by no one on 2026-10-31).
"""An agent's cost is its calls times its context, and the owner's quota went twice to agents launched on
the default model, up to seven at once (docs/contributors/lead.md §1-2). A rule the lead had to remember
was not applied in fifteen launches (2026-10-31), so the launch itself checks."""
from __future__ import annotations

import os
from pathlib import Path

import agents
import hooklib

# How many agents may run at once (the owner, 2026-10-26: "at most 2-3"). ORPHEUM_MOST_AGENTS overrides it.
MOST_AGENTS = 3
# The agent types that cannot edit a file, so share the main tree harmlessly (their tool lists).
READ_ONLY_TYPES = frozenset({"Explore", "Plan", "claude-code-guide", "statusline-setup"})
# Where an agent that can edit works apart from the main tree.
APART = frozenset({"worktree", "remote"})


def refusal(event: dict[str, object], root: Path | None = None) -> str | None:
    """Why the launch is refused, or None, the launch then noted as one more agent running."""
    tool_input = event.get("tool_input")
    launch = tool_input if isinstance(tool_input, dict) else {}
    if launch.get("subagent_type") != "fork" and not launch.get("model"):
        return ("Name the agent's model (docs/contributors/lead.md §2): `sonnet` for mechanical work and briefs "
                "naming the files, `opus` only for design-heavy or wide-reaching work, `haiku` for lookups.")
    if launch.get("subagent_type") not in READ_ONLY_TYPES and launch.get("isolation") not in APART:
        # An agent sharing the main tree left half-written files that failed the Stop hook's gate three
        # times, then waited twenty minutes (the first token trial, 2026-10-31); lead.md §2 already said
        # build agents get a worktree, and it was not followed.
        return ('An agent that can edit files works in its own worktree (docs/contributors/lead.md §2): add '
                '`isolation: "worktree"`, and cherry-pick its commits when it reports. For reading alone, use '
                '`subagent_type: "Explore"`.')
    most = int(os.environ.get("ORPHEUM_MOST_AGENTS") or MOST_AGENTS)
    busy = agents.launch_if_room(most, root)
    if busy is None:
        return None
    return (f"{len(busy)} agents are running or starting ({', '.join(busy)}), the most at once is {most} "
            "(docs/contributors/lead.md §1). Wait for one to finish, or do the change yourself. One that died "
            "unseen: `python3 .claude/hooks/agents.py stopped <agent id>`.")


def main() -> None:
    said = refusal(hooklib.read_event())
    if said:
        hooklib.deny(said)


if __name__ == "__main__":
    main()
