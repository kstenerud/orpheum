#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: PreToolUse hook for file edits: refuses changes to gates, generated code and
# ABOUTME: committed migrations, each with the sanctioned route instead.
from __future__ import annotations

import agents
import hooklib
from gate_paths import is_committed_migration, is_gate, is_generated, relative


def main() -> None:
    event = hooklib.read_event()
    if agents.is_agent_id(event.get("agent_id")):
        # An agent editing is acting: its worktree is not a stopped agent's (agents.has_stopped).
        agents.mark_active(str(event["agent_id"]))
    tool_input = event.get("tool_input") or {}
    path = tool_input.get("file_path") or tool_input.get("notebook_path") or ""
    if not path:
        return
    # The checkout the file is in: ROOT, or an agent's worktree of the same repository (hooklib.root_of).
    root = hooklib.root_of(path)
    rel = relative(str(path), root)
    if rel is None:
        return
    if is_generated(rel):
        hooklib.deny(
            f"{rel} is generated. Edit its source (queries/*.sql, the handlers, package.json) "
            "and run `make generate` or the package manager instead."
        )
    elif is_committed_migration(rel, root):
        hooklib.deny(
            f"{rel} is a committed migration, and migrations are append-only (D181). "
            "Add the next numbered migration instead. Until migrations are live (D228), a schema "
            "change folds this last step into the baseline, which may be edited, and adds the "
            "change as the one step after it; once internal/store/migrations/CHECKPOINT exists, "
            "production has run the baseline, so it is frozen too and a change appends a step "
            "(D272 §5)."
        )
    elif is_gate(rel) and not hooklib.owner_allows("ORPHEUM_ALLOW_GATE_EDIT", event):
        hooklib.deny(
            f"{rel} is a gate: it decides what passing means, so the owner changes it. "
            "If the gate is wrong, stop and tell the owner what to change and why. "
            "(Sessions started with ORPHEUM_ALLOW_GATE_EDIT=1 may edit gates; =agents lets their "
            "subagents too.)"
        )


if __name__ == "__main__":
    main()
