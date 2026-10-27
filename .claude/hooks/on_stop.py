#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Stop hook: if the turn changed files, run the gate (`make check`, or `make
# ABOUTME: check-documentation` after a documentation-only turn) where the tools are; block on failure.
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
        target = "check-documentation"
    else:
        return
    if already_passed():
        # The gate already passed on exactly this tree, as a commit's does in the same turn: running
        # it again ran the same gate twice (gate-throughput G2, 2026-10-26).
        code.unlink(missing_ok=True)
        docs.unlink(missing_ok=True)
        return
    if hooklib.check_pr_running() is not None or gate_held():
        # Run beside check-pr, make check slowed it and failed a test on the load (2026-10-20). The
        # mark stays, so the first turn to end after check-pr does runs it.
        return
    try:
        # Here, or in the development image through ./dev, wherever the tools are.
        command = run_gate.gate_command(target, os.environ)
    except run_gate.NoWayToRun as missing:
        block(target, str(missing))
        return
    # Never build the image here: a build takes minutes past the hook's limit (./dev says to run it).
    env = {**os.environ, "ORPHEUM_DEV_NO_BUILD": "1"}
    done = subprocess.run(command, cwd=hooklib.ROOT, capture_output=True, text=True, check=False, env=env)
    if done.returncode == 0:
        code.unlink(missing_ok=True)
        docs.unlink(missing_ok=True)
        return
    # make check prints only failures and the log's path (scripts/quiet_gate.py); cap it anyway.
    block(target, (done.stdout + done.stderr)[-6000:])


def gate_held() -> bool:
    """Whether some gate holds the repository's gate lock (scripts/gate_lock.py). The Stop hook never
    waits for one: it has ten minutes, and check-pr takes longer; the mark stays for the next turn."""
    done = subprocess.run([sys.executable, str(hooklib.ROOT / "scripts" / "gate_lock.py"), "--held"],
                          cwd=hooklib.ROOT, capture_output=True, check=False)
    return done.returncode == 0


def already_passed() -> bool:
    """Whether `make check` passed on exactly the working tree as it is now (scripts/check_stamp.py)."""
    done = subprocess.run([sys.executable, str(hooklib.ROOT / "scripts" / "check_stamp.py"), "matches"],
                          cwd=hooklib.ROOT, capture_output=True, check=False)
    return done.returncode == 0


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
