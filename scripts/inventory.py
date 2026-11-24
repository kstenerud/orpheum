#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: make inventory: every test of every tier run to its end, nothing stopping at a first failure and
# ABOUTME: no revert-red, then one list of what failed, each with its file and first failing line.
"""The inventory run at a design cycle's end: every tier, each to its end, and one list of failures.

The fixes after the 11-23 design cycle took about eight hours: the reds came one kind at a time, unit,
then browser, then end-to-end, the last only in check-pr, each round stopping at its first failed
tier. The owner's proposal (2026-11-24, the test fixes after a design cycle, items 4 and 5): the
cycle stays free of tests while designing, and at its end one run takes every test of every tier to
its end with nothing stopping early, and writes one list of failures, each with its file and its
first failing line, from which every fix starts. The end-to-end specs run both passes ("yes both
passes"). Revert-red is not in it: a probe means something only against tests that pass.

Its tiers are check-everything's (the nightly run), but revert-red, the race detector and the
isolated web run: make check's steps with the stamps it trusts cleared (`make -k`, so every step
runs), the browser-mode tests, check-pr's quick tier against BASE, the integration tier and the full
web lint whole, the browser tier whole, both passes, and M4's acceptance under home.

The sweep the list starts turns the background revert-red probe off (background_probe.SWEEP_OFF,
beside the owner's own background-off): a commit of the sweep probes
tests not yet trusted. A passing check-pr removes it. The owner's background-off is never touched.

Each tier's output is kept in .dev/inventory/<n>-<tier>.log; the list is .dev/inventory/failures.md,
and is printed at the end.
"""

from __future__ import annotations

import argparse
import re
import shutil
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from background_probe import SWEEP_OFF, folder  # noqa: E402
from revert_red_known import COLOUR  # noqa: E402

OUT = Path(".dev") / "inventory"
# The records make check trusts to run less, cleared as check-everything clears them.
STAMPS = ("orpheum-area-stamps",)

GO_FAILED = re.compile(r"^\s*--- FAIL: (\S+)")
GO_LINE = re.compile(r"(\S+_test\.go:\d+)")
VITEST_FAILED = re.compile(r"^\s*(?:×|FAIL)\s+(?:(?:chromium|firefox|\|[\w-]+\|)\s+)?(\S+\.test\.tsx?)(?: > (.+?))?\s*$")
PLAYWRIGHT_FAILED = re.compile(r"^\s*\d+\) \[(\w+)\] › (\S+?:\d+):\d+ › (.+?)\s*─*$")
SPEC_LINE = re.compile(r"(web/e2e/[\w./-]+\.ts:\d+)")
PYTHON_FAILED = re.compile(r"^(?:FAIL|ERROR): (test_\w+) \((\S+)\)")
PYTHON_LINE = re.compile(r'File "(\S+/tests/test_\w+\.py)", line (\d+)')
# A linter's, type checker's or vet's finding: dir/path:line[:col] or dir/path(line,col), with its message;
# a directory, so a Go test's own `x_test.go:12: message` lines are not read as findings.
FINDING = re.compile(r"^((?:[\w.@-]+/)+[\w.@-]+\.(?:go|tsx?|jsx?|py|css|md|sh|json))(?::(\d+)(?::\d+)?|\((\d+),\d+\))[:\s]+(.+)$")
ESLINT_FILE = re.compile(r"^(/\S+\.(?:tsx?|jsx?|cjs|mjs))$")
ESLINT_FINDING = re.compile(r"^\s+(\d+):\d+\s+error\s+(.+)$")
MAKE_FAILED = re.compile(r"^make(?:\[\d+\])?: \*\*\* \[\S+: (\S+)\] Error")


@dataclass(frozen=True)
class Failure:
    what: str
    where: str


def first_line_of(log: str, start: int, line: re.Pattern[str], fallback: str) -> str:
    """The first file:line that pattern finds in log from start, else fallback."""
    found = line.search(log, start)
    return ":".join(group for group in found.groups() if group) if found else fallback


def failures_in(log: str) -> list[Failure]:
    """What a tier's output says failed, each with its file and first failing line, in the order met;
    the findings of linters and type checkers by their own lines; nothing for a passing run."""
    log = COLOUR.sub("", log)
    found: list[Failure] = []
    seen: set[str] = set()

    def add(what: str, where: str) -> None:
        if what not in seen:
            seen.add(what)
            found.append(Failure(what, where))
    offset = 0
    eslint_file = ""
    for text in log.splitlines(keepends=True):
        line = text.rstrip("\n")
        if (go := GO_FAILED.match(line)) and "/" not in go.group(1):
            add(f"go {go.group(1)}", first_line_of(log, offset, GO_LINE, "(no line: see the log)"))
        elif vitest := VITEST_FAILED.match(line):
            file, test = vitest.group(1), vitest.group(2)
            where = first_line_of(log, offset, re.compile(rf"({re.escape(file)}:\d+)"), file)
            add(f"vitest {file}" + (f" > {test}" if test else ""), where)
        elif playwright := PLAYWRIGHT_FAILED.match(line):
            browser, declared, test = playwright.groups()
            add(f"e2e [{browser}] {test}", first_line_of(log, offset + len(text), SPEC_LINE, declared))
        elif python := PYTHON_FAILED.match(line):
            add(f"python {python.group(2)}", first_line_of(log, offset, PYTHON_LINE, python.group(2)))
        elif eslint := ESLINT_FILE.match(line):
            eslint_file = eslint.group(1)
        elif eslint_file and (problem := ESLINT_FINDING.match(line)):
            add(f"lint {problem.group(2).strip()} ({eslint_file}:{problem.group(1)})", f"{eslint_file}:{problem.group(1)}")
        elif finding := FINDING.match(line.strip()):
            path, line_no = finding.group(1), finding.group(2) or finding.group(3)
            add(f"{finding.group(4).strip()[:160]} ({path}:{line_no})", f"{path}:{line_no}")
        offset += len(text)
    return found


def failed_steps(log: str) -> list[str]:
    """The make steps a tier's output says failed, innermost first as make prints them."""
    return list(dict.fromkeys(match.group(1) for line in COLOUR.sub("", log).splitlines()
                              if (match := MAKE_FAILED.match(line))))


def tiers(base: str, jobs: int, quick: list[str]) -> list[tuple[str, list[str]]]:
    """Each tier's name and command, in check-everything's order."""
    make = ["make", "--no-print-directory"]
    return [
        ("make-check", [*make, "-k", f"-j{jobs}", "--output-sync=target", "check-parallel"]),
        ("browser-mode", [*make, "web-test-browser-run"]),
        ("check-pr-quick", [*make, "-k", f"-j{jobs}", "--output-sync=target", f"BASE={base}", *quick]),
        ("integration", [*make, "test-integration-run"]),
        ("web-lint-full", ["npm", "--prefix", "web", "run", "--silent", "lint:full"]),
        ("browser-tier", [*make, "test-browser", "BASE="]),
        ("acceptance-home", [*make, "test-acceptance-home", "BASE="]),
    ]


def common_dir() -> Path:
    done = subprocess.run(["git", "rev-parse", "--path-format=absolute", "--git-common-dir"], capture_output=True,
                          text=True, check=True)
    return Path(done.stdout.strip())


def run_tier(name: str, command: list[str], log: Path) -> tuple[int, str]:
    """Runs a tier to its end, its output to log and, by its last lines, the screen."""
    print(f"inventory: {name}: {' '.join(command)}", flush=True)
    with open(log, "w", encoding="utf-8") as out:
        done = subprocess.run(command, stdout=out, stderr=subprocess.STDOUT, check=False)
    text = log.read_text(encoding="utf-8", errors="replace")
    print(f"inventory: {name}: {'passed' if done.returncode == 0 else f'FAILED (exit {done.returncode})'}", flush=True)
    return done.returncode, text


def report(results: list[tuple[str, int, str, Path]]) -> str:
    """The list: each failed tier, the steps make says failed, and each failure's file and line."""
    lines = ["# The inventory's failures", ""]
    failed = [result for result in results if result[1] != 0]
    if not failed:
        return "\n".join([*lines, "Every tier passed.", ""])
    for name, status, log, path in failed:
        lines += [f"## {name} (exit {status}; the whole output: {path})", ""]
        steps = failed_steps(log)
        if steps:
            lines += [f"Failed steps: {', '.join(steps)}", ""]
        found = failures_in(log)
        lines += [f"- {failure.where}: {failure.what}" for failure in found] or [
            "- no test or finding named: the log's end says why"]
        lines.append("")
    return "\n".join(lines)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--base", required=True, help="check-pr's base, for its quick tier")
    parser.add_argument("--jobs", type=int, default=8)
    parser.add_argument("--quick", nargs="*", default=[], help="check-pr's quick tier's steps")
    arguments = parser.parse_args(argv)
    common = common_dir()
    for stamp in STAMPS:
        shutil.rmtree(common / stamp, ignore_errors=True)
    shutil.rmtree(OUT, ignore_errors=True)
    OUT.mkdir(parents=True)
    results = []
    for n, (name, command) in enumerate(tiers(arguments.base, arguments.jobs, arguments.quick), start=1):
        log = OUT / f"{n}-{name}.log"
        status, text = run_tier(name, command, log)
        results.append((name, status, text, log))
    listing = report(results)
    (OUT / "failures.md").write_text(listing, encoding="utf-8")
    print(listing, flush=True)
    if all(status == 0 for _, status, _, _ in results):
        return 0
    # The sweep begins: its commits' background probes would probe tests not yet trusted.
    probes = folder()
    if probes is not None:
        probes.mkdir(parents=True, exist_ok=True)
        (probes / SWEEP_OFF).touch()
    print(f"inventory: the background revert-red probe is off for the sweep ({SWEEP_OFF}); a passing "
          "check-pr turns it on again", flush=True)
    return 1


if __name__ == "__main__":
    sys.exit(main())
