#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: PostToolUse hook for file edits: formats the file with the gate's formatters, and records that
# ABOUTME: the tree changed, so the Stop hook runs the gate — the whole of it for code, the docs gate for docs.
from __future__ import annotations

import hooklib
from documentation_only import is_documentation
from format_files import format_files
from gate_paths import relative

PENDING = hooklib.ROOT / ".claude" / ".cache"


def main() -> None:
    event = hooklib.read_event()
    tool_input = event.get("tool_input") or {}
    path = tool_input.get("file_path") or tool_input.get("notebook_path") or ""
    rel = relative(str(path), hooklib.ROOT) if path else None
    if rel is None or rel.startswith((".git/", ".claude/.cache/")):
        return
    # A formatting slip is fixed where it is made, not found by the gate after (the owner, 2026-10-20).
    format_files([rel], hooklib.ROOT)
    PENDING.mkdir(parents=True, exist_ok=True)
    kind = "docs" if is_documentation(rel) else "code"
    (PENDING / f"check-pending-{kind}").touch()


if __name__ == "__main__":
    main()
