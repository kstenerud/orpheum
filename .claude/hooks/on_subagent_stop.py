#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: SubagentStop hook: an agent that stops to "wait" for a gate it started in the background is
# ABOUTME: told to wait in the foreground instead; agents that did so never resumed (process review F14).
from __future__ import annotations

import json
import re
import subprocess
import sys

import hooklib

# The agent's last words say it stopped to wait for something to finish.
WAITING = re.compile(r"\b(wait(?:ing)?|until .* (?:finish|complete)s?|notif(?:y|ication))\b", re.IGNORECASE)
# A gate's process, as `ps` shows it: make check's wrapper, revert-red, check-pr's tree.
GATE_PROCESS = re.compile(r"quiet_gate\.py|check_revert_red\.py|check_pr_snapshot\.py|gate_lock\.py")


def gate_running() -> bool:
    done = subprocess.run(["ps", "-eo", "args"], capture_output=True, text=True, check=False)
    return any(GATE_PROCESS.search(line) for line in done.stdout.splitlines())


def main() -> None:
    event = hooklib.read_event()
    if event.get("stop_hook_active"):
        return  # it was told once already this stop; never hold an agent in a loop
    said = str(event.get("last_assistant_message") or "")
    if WAITING.search(said) and gate_running():
        json.dump({"decision": "block", "reason": (
            "You stopped to wait for a run that is still going, and an agent that stops does not resume by "
            "itself (2026-10-26). Wait for it in the foreground (a blocking command with a timeout, or read its "
            "output file once it ends), then finish and report.")}, sys.stdout)


if __name__ == "__main__":
    main()
