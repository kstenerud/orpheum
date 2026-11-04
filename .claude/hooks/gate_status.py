#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: A status line's segment: while the Stop hook's gate runs, its target and how long it has run;
# ABOUTME: nothing otherwise. A personal status line calls it with the project's folder; it never fails.
from __future__ import annotations

import json
import os
import sys
import time
from pathlib import Path

# on_stop.py's PENDING and RUNNING, which write the mark this reads.
MARK = Path(".claude") / ".cache" / "gate-running.json"


def segment(root: Path, now: float) -> str:
    """'make check: 2m10s' while the gate the mark names runs; '' with no mark, an unreadable one, or one
    whose hook is gone (killed, so it never removed the mark)."""
    try:
        mark = json.loads((root / MARK).read_text())
        target, start, pid = str(mark["target"]), float(mark["start"]), int(mark["pid"])
        os.kill(pid, 0)
    except (OSError, ValueError, KeyError, TypeError):
        return ""
    seconds = max(0, int(now - start))
    return f"make {target}: {seconds // 60}m{seconds % 60:02d}s"


if __name__ == "__main__":
    print(segment(Path(sys.argv[1]) if len(sys.argv) > 1 else Path.cwd(), time.time()))
