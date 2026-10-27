#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: UserPromptSubmit hook: puts the owner's standing orders (.claude/standing-orders.md) in front
# ABOUTME: of the agent at every prompt, so the order of work is not left to its memory (process review F13).
from __future__ import annotations

import json
import sys

import hooklib

STANDING_ORDERS = hooklib.ROOT / ".claude" / "standing-orders.md"
# A few hundred words at most: it rides on every prompt.
MOST_CHARACTERS = 3000


def main() -> None:
    hooklib.read_event()
    if not STANDING_ORDERS.exists():
        return
    orders = STANDING_ORDERS.read_text(encoding="utf-8").strip()[:MOST_CHARACTERS]
    if orders:
        json.dump({"hookSpecificOutput": {"hookEventName": "UserPromptSubmit",
                                          "additionalContext": "The owner's standing orders (.claude/standing-orders.md):\n"
                                          + orders}}, sys.stdout)


if __name__ == "__main__":
    main()
