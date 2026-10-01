#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# ABOUTME: Stop hook: if the turn changed files, run the gate (`make check`, or `make
# ABOUTME: check-docs` after a docs-only turn) where the tools are, and block the turn if it fails.
from __future__ import annotations

import json
import os
import subprocess
import sys

import hooklib
import run_gate

PENDING = hooklib.ROOT / ".claude" / ".cache"


def main() -> None:
    hooklib.read_event()
    code = PENDING / "check-pending-code"
    docs = PENDING / "check-pending-docs"
    if code.exists():
        target = "check"
    elif docs.exists():
        target = "check-docs"
    else:
        return
    try:
        # Here, or in the development image through ./dev, wherever the tools are.
        command = run_gate.gate_command(target, os.environ)
    except run_gate.NoWayToRun as missing:
        block(target, str(missing))
        return
    done = subprocess.run(command, cwd=hooklib.ROOT, capture_output=True, text=True, check=False)
    if done.returncode == 0:
        code.unlink(missing_ok=True)
        docs.unlink(missing_ok=True)
        return
    # make check prints only failures and the log's path (scripts/quiet_gate.py); cap it anyway.
    block(target, (done.stdout + done.stderr)[-6000:])


def block(target: str, output: str) -> None:
    json.dump(
        {
            "decision": "block",
            "reason": f"`make {target}` failed at the end of the turn. Fix these before "
            f"stopping:\n\n{output}",
        },
        sys.stdout,
    )


if __name__ == "__main__":
    main()
