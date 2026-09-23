#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# ABOUTME: Claims gate run by `make check`: every cited TestArch_ test exists and every one
# ABOUTME: is cited; every decision ID in code resolves to a live decision; every
# ABOUTME: ORPHEUM_TEST_ variable the code reads is set by something (D190).
"""Keep the prose and the code telling the same story.

Three checks. Each is for a way a true sentence silently becomes false:

1. **Architecture claims are tests** (D190). Prose that asserts a checkable fact about the
   code cites a test named `TestArch_<Claim>`. A cited name that no test defines is a
   claim nothing checks. A `TestArch_` test that nothing cites is an invariant nobody was
   told about, so both directions fail. History (the decision logs and research) may cite
   anything, because it records what was true.
2. **Decision IDs in code resolve.** A `D<n>` in a Go, TypeScript, Python or shell comment
   must name a decision that exists and is not archived. An agent inventing a plausible
   citation is the failure. The docs gate already covers Markdown.
3. **Test variables are set by something** (D190). Every `ORPHEUM_TEST_<NAME>` read by code
   must appear in a Makefile, a CI workflow or a script that sets it. A variable only ever
   read gates a test that no job will ever run.
"""

from __future__ import annotations

import argparse
import re
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path

DECISIONS = "docs/contributors/decisions/"
LIVE_LOG = DECISIONS + "working-notes.md"
ARCHIVE_LOG = DECISIONS + "working-notes-archive.md"
HISTORY_PREFIXES = (DECISIONS, "docs/contributors/design/research/")

ARCH_NAME = re.compile(r"\bTestArch_[A-Za-z0-9_]+")
GO_ARCH_DEF = re.compile(r"^func (TestArch_[A-Za-z0-9_]+)\(t \*testing\.T\)", re.M)
TS_ARCH_DEF = re.compile(r"""\b(?:it|test)\(\s*["'`](TestArch_[A-Za-z0-9_]+)""")
DECISION_HEADING = re.compile(r"^## D(\d+) — ", re.M)
CODE_DECISION = re.compile(r"(?<![\w/#.-])D(\d{1,4})(?![\w-])")
TEST_VAR = re.compile(r"\bORPHEUM_TEST_[A-Z0-9_]+\b")

CODE_SUFFIXES = (".go", ".ts", ".tsx", ".js", ".mjs", ".cjs", ".py", ".sh")
GENERATED_PARTS = ("internal/store/sqlcgen/", "web/packages/sdk/src/gen/")
# Where a test variable may be set: job definitions, not the code that reads it.
SETTERS = ("Makefile", ".github/workflows/", "scripts/")


@dataclass(frozen=True)
class Problem:
    where: str
    message: str


def tracked(root: Path) -> list[str]:
    out = subprocess.run(
        ["git", "ls-files", "--cached", "--others", "--exclude-standard"],
        cwd=root, capture_output=True, text=True, check=True,
    ).stdout
    return sorted({p for p in out.splitlines() if p and (root / p).is_file()})


def arch_definitions(files: dict[str, str]) -> dict[str, str]:
    """TestArch_ tests defined in Go test files and TS test files, name → path."""
    defined: dict[str, str] = {}
    for path, text in files.items():
        if path.endswith("_test.go"):
            for name in GO_ARCH_DEF.findall(text):
                defined[name] = path
        elif re.search(r"\.test\.tsx?$", path):
            for name in TS_ARCH_DEF.findall(text):
                defined[name] = path
    return defined


def arch_citations(files: dict[str, str]) -> dict[str, list[str]]:
    """TestArch_ names cited outside the tests themselves and outside history."""
    cited: dict[str, list[str]] = {}
    for path, text in files.items():
        if path.startswith(HISTORY_PREFIXES):
            continue
        if path.endswith("_test.go") or re.search(r"\.test\.tsx?$", path):
            continue
        is_prose = path.endswith(".md") or path.endswith("doc.go")
        if not is_prose:
            continue
        for name in set(ARCH_NAME.findall(text)):
            cited.setdefault(name, []).append(path)
    return cited


def check_arch(files: dict[str, str]) -> list[Problem]:
    defined = arch_definitions(files)
    cited = arch_citations(files)
    problems = [
        Problem(paths[0], f"cites `{name}`, which no test defines — a claim nothing checks")
        for name, paths in sorted(cited.items())
        if name not in defined
    ]
    problems += [
        Problem(path, f"defines `{name}`, which no doc cites — an invariant nobody was told "
                      "about; cite it in the owning package's doc.go or README")
        for name, path in sorted(defined.items())
        if name not in cited
    ]
    return problems


def decision_status(live: str, archive: str) -> tuple[set[int], set[int]]:
    return (
        {int(n) for n in DECISION_HEADING.findall(live)},
        {int(n) for n in DECISION_HEADING.findall(archive)},
    )


def check_code_decisions(files: dict[str, str], live: set[int], archived: set[int]) -> list[Problem]:
    problems: list[Problem] = []
    for path, text in files.items():
        if not path.endswith(CODE_SUFFIXES) or path.startswith(GENERATED_PARTS):
            continue
        if path.startswith("scripts/tests/"):
            continue  # gate tests construct synthetic decision IDs on purpose
        for lineno, line in enumerate(text.splitlines(), 1):
            for m in CODE_DECISION.finditer(line):
                n = int(m.group(1))
                if n in archived and n not in live:
                    problems.append(Problem(f"{path}:{lineno}", f"cites D{n}, which is archived; "
                                            "cite the decision that superseded it"))
                elif n not in live:
                    problems.append(Problem(f"{path}:{lineno}", f"cites D{n}, which does not exist"))
    return problems


def check_test_vars(files: dict[str, str]) -> list[Problem]:
    read: dict[str, str] = {}
    set_somewhere: set[str] = set()
    for path, text in files.items():
        names = set(TEST_VAR.findall(text))
        if not names:
            continue
        if path.startswith(SETTERS) or path == "Makefile":
            set_somewhere |= names
        elif path.endswith(CODE_SUFFIXES):
            for n in names:
                read.setdefault(n, path)
    return [
        Problem(path, f"reads {name}, which no Makefile target, CI workflow or script sets — "
                      "the test it gates never runs anywhere (D190)")
        for name, path in sorted(read.items())
        if name not in set_somewhere
    ]


def check(root: Path) -> list[Problem]:
    paths = tracked(root)
    files = {
        p: (root / p).read_text(errors="replace")
        for p in paths
        if p.endswith((".md", *CODE_SUFFIXES)) or p == "Makefile" or p.startswith(".github/")
    }
    live, archived = decision_status(files.get(LIVE_LOG, ""), files.get(ARCHIVE_LOG, ""))
    return check_arch(files) + check_code_decisions(files, live, archived) + check_test_vars(files)


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--root", default=".")
    args = ap.parse_args()
    problems = check(Path(args.root).resolve())
    for p in problems:
        print(f"{p.where}: {p.message}")
    if problems:
        print(f"check_claims: {len(problems)} problem(s)")
        return 1
    print("check_claims: architecture citations, code decision IDs and test variables all resolve")
    return 0


if __name__ == "__main__":
    sys.exit(main())
