#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Reads `go test -json` on stdin, prints the text go test would have, and keeps the run's
# ABOUTME: timings, each test's and each package's, as every suite keeps its own (web/e2e/test-timings.ts).
"""Keeps a Go test run's timings, always: tests grow heavier over time, and a run found slow is
explained from what it already wrote (the owner, 2026-10-20).

    go test -json ./... | python3 scripts/go_test_timings.py SUITE

`-json` runs the tests verbosely, so the text printed is what go test prints without it: a test's
output only when it fails, without its === RUN, PAUSE and CONT lines, held until it ends; a
package's own lines but its bare PASS; a test still running when its package fails (a panic, a
timeout) printed then; and a line that is not an event (go vet's, a build's) as it is. The exit
status is go test's, through the shell's pipefail: a record it cannot write is said, never failed. The record goes to `.dev/test-timings/SUITE/`, the newest
KEPT_PASSES kept, in the shape scripts/test_timings.py reads.
"""

from __future__ import annotations

import datetime
import json
import sys
from pathlib import Path
from typing import IO

ROOT = Path(__file__).resolve().parent.parent
TIMINGS_ROOT = ROOT / ".dev" / "test-timings"
KEPT_PASSES = 20  # as web/e2e/test-timings.ts's keptPasses


# A test's lines the non-verbose text leaves out.
VERBOSE_ONLY = ("=== RUN", "=== PAUSE", "=== CONT", "=== NAME")
# The line that ends a test's block, which the non-verbose text puts first.
RESULT_LINE = ("--- FAIL", "--- PASS", "--- SKIP")


class Held:
    """The text of the tests still running, printed for a failed one as go test prints it: its result
    line, its own lines, then each failed subtest's block, indented under it."""

    def __init__(self, echo: IO[str]) -> None:
        self.echo = echo
        self.lines: dict[tuple[str, str], list[str]] = {}
        self.failed_children: dict[tuple[str, str], list[str]] = {}

    def add(self, package: str, test: str, output: str) -> None:
        if not output.lstrip().startswith(VERBOSE_ONLY):
            self.lines.setdefault((package, test), []).append(output)

    def end(self, package: str, test: str, has_failed: bool) -> None:
        key = (package, test)
        lines, children = self.lines.pop(key, []), self.failed_children.pop(key, [])
        if not has_failed:
            return
        result = [line for line in lines if line.lstrip().startswith(RESULT_LINE)]
        block = result + [line for line in lines if line not in result] + children
        parent = test.rpartition("/")[0]
        if parent and (package, parent) in self.lines:
            self.failed_children.setdefault((package, parent), []).extend("    " + line for line in block)
        else:
            self.echo.write("".join(block))

    def end_package(self, package: str, has_failed: bool) -> None:
        """A package's end: a test of it still running when it failed, a panic's or a timeout's, is printed."""
        for key in [key for key in self.lines if key[0] == package]:
            self.end(package, key[1], has_failed)


def record_of(lines: IO[str], echo: IO[str], suite: str, started: datetime.datetime) -> dict:
    """Prints the run's non-verbose text to echo and returns the run's record."""
    tests: list[dict] = []
    files: list[dict] = []
    held = Held(echo)
    failed = False
    for line in lines:
        try:
            event = json.loads(line)
        except ValueError:
            echo.write(line)
            continue
        if not isinstance(event, dict):
            echo.write(line)
            continue
        package, test, action = event.get("Package", ""), event.get("Test", ""), event.get("Action")
        output = event.get("Output")
        if output is not None and test:
            held.add(package, test, output)
        elif output is not None and output.strip() != "PASS":
            if output.startswith(f"FAIL\t{package}"):
                held.end_package(package, True)  # a panic's or timeout's test goes before its package's FAIL
            echo.write(output)
        if action not in ("pass", "fail", "skip"):
            continue
        failed = failed or action == "fail"
        if test:
            held.end(package, test, action == "fail")
        else:
            held.end_package(package, action == "fail")
        if "Elapsed" not in event:
            continue
        elapsed = round(float(event["Elapsed"]) * 1000)
        if test:
            tests.append({"test": test, "project": suite, "file": package, "line": 0,
                          "status": action, "ms": elapsed, "worker": 0, "steps": []})
        else:
            files.append({"file": package, "project": suite, "ms": elapsed})
    ended = datetime.datetime.now(datetime.timezone.utc)
    return {"suite": suite, "pass": "run", "startedAt": started.isoformat().replace("+00:00", "Z"),
            "status": "failed" if failed else "passed", "ms": round((ended - started).total_seconds() * 1000),
            "tests": tests, "files": files}


def write(record: dict, root: Path = TIMINGS_ROOT) -> None:
    """Writes the record into its suite's directory and keeps only the newest KEPT_PASSES."""
    directory = root / record["suite"]
    directory.mkdir(parents=True, exist_ok=True)
    stamp = record["startedAt"].replace(":", "-")
    (directory / f"{stamp}-{record['pass']}.json").write_text(json.dumps(record))
    kept = sorted(directory.glob("*.json"))
    for old in kept[:max(0, len(kept) - KEPT_PASSES)]:
        old.unlink()


def main(argv: list[str]) -> int:
    if len(argv) != 1:
        print("usage: go test -json ... | go_test_timings.py SUITE", file=sys.stderr)
        return 2
    started = datetime.datetime.now(datetime.timezone.utc)
    record = record_of(sys.stdin, sys.stdout, argv[0], started)
    try:
        write(record)
    except OSError as failed:
        print(f"go_test_timings: could not keep this run's timings: {failed}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
