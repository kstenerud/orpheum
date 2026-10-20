#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Reads a test suite's kept timings (web/e2e/test-timings.ts): the slowest tests and files of its
# ABOUTME: latest pass, each against its median over the passes kept, with the steps its time went to.
"""Where a suite's time goes, from the timings every pass keeps (web/e2e/test-timings.ts).

Tests grow heavier over time; when a run is found slow, this says which tests, files and steps,
and whether one is slower than it used to be (the owner, 2026-10-20). The suites: browser and
acceptance (Playwright, with each test's steps), vitest and vitest-browser, go, go-dev and
go-integration (a test's file is its package).

    python3 scripts/test_timings.py [--suite browser] [--pass side-by-side] [--top N] [--steps N]
    python3 scripts/test_timings.py --list
"""

from __future__ import annotations

import argparse
import json
import statistics
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent / ".dev" / "test-timings"


def passes(directory: Path, kind: str = "*") -> list[dict]:
    """The kept passes of one kind ("*" for every kind), oldest first."""
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
    """The latest pass's slowest tests, each with its median over the earlier passes and its longest
    steps, then its slowest files where the suite keeps them."""
    if not kept:
        return ["No timings are kept for this suite yet; each suite writes them as it runs "
                "(web/e2e/test-timings.ts)."]
    latest, earlier = kept[-1], kept[:-1]
    lines = [f"Pass of {latest['startedAt']} ({latest['pass']}): {latest['ms'] / 1000:.0f} s, "
             f"{len(latest['tests'])} tests; medians over the {len(earlier)} passes before it"]
    before = medians(earlier, "tests", key)
    for test in sorted(latest["tests"], key=lambda each: -each["ms"])[:top]:
        lines.append(f"{test['ms'] / 1000:6.1f} s  ({usual(before.get(key(test)))})  {key(test)}")
        for step in sorted(test["steps"], key=lambda each: -each["ms"])[:steps]:
            lines.append(f"           {step['ms'] / 1000:6.1f} s  {'  ' * step['depth']}{step['category']}: "
                         f"{step['title'][:90]}")
    if latest.get("files"):
        before_files = medians(earlier, "files", file_key)
        lines.append("Slowest files, their loading and setup included:")
        for file in sorted(latest["files"], key=lambda each: -each["ms"])[:top]:
            loading = f", loading {file['loadMs'] / 1000:.1f} s" if file.get("loadMs") else ""
            lines.append(f"{file['ms'] / 1000:6.1f} s  ({usual(before_files.get(file_key(file)))}{loading})  "
                         f"{file_key(file)}")
    return lines


def file_key(file: dict) -> str:
    return f"[{file['project']}] {file['file']}"


def medians(earlier: list[dict], field: str, key_of) -> dict[str, list[float]]:
    """Each test's or file's times over the earlier passes, by its key."""
    times: dict[str, list[float]] = {}
    for run in earlier:
        for each in run.get(field, []):
            times.setdefault(key_of(each), []).append(each["ms"])
    return times


def usual(times: list[float] | None) -> str:
    return f"median {statistics.median(times) / 1000:5.1f} s" if times else "no earlier pass"


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--suite", default="browser", help="the suite's directory under .dev/test-timings")
    parser.add_argument("--pass", dest="kind",
                        help="one kind of pass, for a suite of several: the browser's side-by-side (its "
                             "default) or timed; every other suite has one")
    parser.add_argument("--top", type=int, default=10, help="how many of the slowest (default 10)")
    parser.add_argument("--steps", type=int, default=5, help="how many of each test's longest steps (default 5)")
    parser.add_argument("--list", action="store_true", help="list the suites kept, with their passes")
    parser.add_argument("--root", type=Path, default=ROOT)
    args = parser.parse_args(argv)
    if args.list:
        for suite in sorted(path for path in args.root.glob("*") if path.is_dir()):
            print(f"{suite.name}: {len(list(suite.glob('*.json')))} passes kept")
        return 0
    kind = args.kind or ("side-by-side" if args.suite == "browser" else "*")
    print("\n".join(report(passes(args.root / args.suite, kind), args.top, args.steps)))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
