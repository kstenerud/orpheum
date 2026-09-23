#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# ABOUTME: Stop hook: if the turn changed files, run the gate (`make check`, or `make
# ABOUTME: check-docs` after a docs-only turn) and block the turn with the output if it fails.
from __future__ import annotations

import json
import subprocess
import sys

import hooklib

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
    done = subprocess.run(
        ["make", target], cwd=hooklib.ROOT, capture_output=True, text=True, check=False
    )
    if done.returncode == 0:
        code.unlink(missing_ok=True)
        docs.unlink(missing_ok=True)
        return
    output = (done.stdout + done.stderr)[-12000:]
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
