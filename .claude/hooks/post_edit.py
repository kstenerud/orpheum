#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# ABOUTME: PostToolUse hook for file edits: records that the tree changed, so the Stop hook
# ABOUTME: knows to run the gate — the whole of it for code, the docs gate for docs.
from __future__ import annotations

import hooklib
from gate_paths import relative

PENDING = hooklib.ROOT / ".claude" / ".cache"


def main() -> None:
    event = hooklib.read_event()
    tool_input = event.get("tool_input") or {}
    path = tool_input.get("file_path") or tool_input.get("notebook_path") or ""
    rel = relative(str(path), hooklib.ROOT) if path else None
    if rel is None or rel.startswith((".git/", ".claude/.cache/")):
        return
    PENDING.mkdir(parents=True, exist_ok=True)
    kind = "docs" if rel.startswith("docs/") else "code"
    (PENDING / f"check-pending-{kind}").touch()


if __name__ == "__main__":
    main()
