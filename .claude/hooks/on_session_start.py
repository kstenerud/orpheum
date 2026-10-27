#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: SessionStart hook: tells the session what the last batch still owes, such as a full
# ABOUTME: check-pr after a targeted re-run passed (process review F2); shown, never run.
from __future__ import annotations

import json
import subprocess
import sys

import hooklib


def main() -> None:
    hooklib.read_event()
    done = subprocess.run([sys.executable, str(hooklib.ROOT / "scripts" / "check_pr_debt.py"), "show"],
                          cwd=hooklib.ROOT, capture_output=True, text=True, check=False)
    if done.stdout.strip():
        json.dump({"hookSpecificOutput": {"hookEventName": "SessionStart",
                                          "additionalContext": done.stdout.strip()}}, sys.stdout)


if __name__ == "__main__":
    main()
