#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Reads the browser tier's kept timings (web/e2e/timings.ts): the slowest tests of the latest
# ABOUTME: pass, each against its median over the passes kept, with the steps its time went to.
"""Where the browser tier's time goes, from the timings every pass keeps (web/e2e/timings.ts).

Tests grow heavier over time; when a run is found slow, this says which tests and which of their
steps, and whether a test is slower than it used to be (the owner, 2026-10-20).

    python3 scripts/browser_timings.py [--top N] [--steps N] [--pass side-by-side|timed|acceptance]
"""

from __future__ import annotations

import argparse
import json
import statistics
import sys
from pathlib import Path

DIRECTORY = Path(__file__).resolve().parent.parent / ".dev" / "browser-timings"


def passes(directory: Path, kind: str) -> list[dict]:
    """The kept passes of one kind, oldest first."""
    found = []
    for path in sorted(directory.glob(f"*-{kind}.json")):
        try:
            found.append(json.loads(path.read_text()))
        except (OSError, ValueError):
            continue
    return found


def key(test: dict) -> str:
    return f"[{test['project']}] {test['file']}:{test['line']} {test['test']}"


def report(kept: list[dict], top: int, steps: int) -> list[str]:
    """The latest pass's slowest tests, each with its median over the earlier passes and its longest steps."""
    if not kept:
        return ["No browser timings are kept yet; the browser tier writes them as it runs (web/e2e/timings.ts)."]
    latest, earlier = kept[-1], kept[:-1]
    before: dict[str, list[float]] = {}
    for run in earlier:
        for test in run["tests"]:
            before.setdefault(key(test), []).append(test["ms"])
    lines = [f"Pass of {latest['startedAt']} ({latest['pass']}): {latest['ms'] / 1000:.0f} s, "
             f"{len(latest['tests'])} tests; medians over the {len(earlier)} passes before it"]
    for test in sorted(latest["tests"], key=lambda each: -each["ms"])[:top]:
        usual = before.get(key(test))
        median = f"median {statistics.median(usual) / 1000:5.1f} s" if usual else "no earlier pass"
        lines.append(f"{test['ms'] / 1000:6.1f} s  ({median})  {key(test)}")
        for step in sorted(test["steps"], key=lambda each: -each["ms"])[:steps]:
            lines.append(f"           {step['ms'] / 1000:6.1f} s  {'  ' * step['depth']}{step['category']}: {step['title'][:90]}")
    return lines


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--top", type=int, default=10, help="how many of the slowest tests (default 10)")
    parser.add_argument("--steps", type=int, default=5, help="how many of each test's longest steps (default 5)")
    parser.add_argument("--pass", dest="kind", default="side-by-side", help="side-by-side, timed or acceptance")
    parser.add_argument("--directory", type=Path, default=DIRECTORY)
    args = parser.parse_args(argv)
    print("\n".join(report(passes(args.directory, args.kind), args.top, args.steps)))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
