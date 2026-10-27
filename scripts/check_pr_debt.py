#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: The full check-pr a batch still owes after a targeted re-run passed: recorded, shown at a
# ABOUTME: session's start, cleared by a full pass; never run by itself (process review F2).
"""A visible debt, never a forced run.

`check-pr-failed` re-runs only what failed, and its green is never the gate's (makefile.md §4): a
full `check-pr` is still owed. On 2026-10-26 a batch ended there and the next full run re-checked
30 commits for 34 minutes. `owe <base>` records the debt when a targeted re-run passes, `paid` clears
it when a full check-pr passes, and `show` prints it (the SessionStart hook's line). Nothing here runs
a gate: during an iteration of small changes the debt waits, as the owner wants (§25).
"""

from __future__ import annotations

import subprocess
import sys
import time
from pathlib import Path


def debt_path(root: Path | None = None) -> Path:
    common = subprocess.run(["git", "rev-parse", "--path-format=absolute", "--git-common-dir"],
                            capture_output=True, text=True, check=True, cwd=root).stdout.strip()
    return Path(common) / "check-pr" / "owed"


def main(argv: list[str]) -> int:
    path = debt_path()
    if argv[:1] == ["owe"] and len(argv) == 2:
        path.parent.mkdir(parents=True, exist_ok=True)
        if not path.exists():  # the first owed base stands until a full pass
            path.write_text(f"{argv[1]} {time.strftime('%Y-%m-%d %H:%M %Z')}\n", encoding="utf-8")
        return 0
    if argv == ["paid"]:
        path.unlink(missing_ok=True)
        return 0
    if argv == ["show"]:
        if path.exists():
            base, _, since = path.read_text(encoding="utf-8").strip().partition(" ")
            print(f"A full `make check-pr` is owed since {since}: a targeted re-run passed, but no full run "
                  f"has since base {base[:12]}. Run it at the end of the batch.")
        return 0
    print("usage: check_pr_debt.py owe <base> | paid | show", file=sys.stderr)
    return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
