#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# ABOUTME: PreToolUse hook for file edits: refuses changes to gates, generated code and
# ABOUTME: committed migrations, each with the sanctioned route instead.
from __future__ import annotations

import hooklib
from gate_paths import is_committed_migration, is_gate, is_generated, relative


def main() -> None:
    event = hooklib.read_event()
    tool_input = event.get("tool_input") or {}
    path = tool_input.get("file_path") or tool_input.get("notebook_path") or ""
    rel = relative(str(path), hooklib.ROOT) if path else None
    if rel is None:
        return
    if is_generated(rel):
        hooklib.deny(
            f"{rel} is generated. Edit its source (queries/*.sql, the handlers, package.json) "
            "and run `make generate` or the package manager instead."
        )
    elif is_committed_migration(rel, hooklib.ROOT):
        hooklib.deny(
            f"{rel} is a committed migration, and migrations are append-only (D181). "
            "Add the next numbered migration instead. Until migrations are live (D228), a schema "
            "change folds this last step into the baseline, which may be edited, and adds the "
            "change as the one step after it; once internal/store/migrations/CHECKPOINT exists, "
            "production has run the baseline, so it is frozen too and a change appends a step "
            "(D272 §5)."
        )
    elif is_gate(rel) and not hooklib.owner_allows("ORPHEUM_ALLOW_GATE_EDIT"):
        hooklib.deny(
            f"{rel} is a gate: it decides what passing means, so the owner changes it. "
            "If the gate is wrong, stop and tell the owner what to change and why. "
            "(Sessions started with ORPHEUM_ALLOW_GATE_EDIT=1 may edit gates.)"
        )


if __name__ == "__main__":
    main()
