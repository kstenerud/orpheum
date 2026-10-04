#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# ABOUTME: Claims gate run by `make check`: every cited TestArch_ test exists and every one
# ABOUTME: is cited; every decision ID in code resolves to a live decision; every
# ABOUTME: ORPHEUM_TEST_ variable the code reads is set by something (D190); no retired name is left;
# ABOUTME: every Go test a doc.go, README or live document cites exists.
"""Keep the prose and the code telling the same story.

Five checks. Each is for a way a true sentence silently becomes false:

1. **Architecture claims are tests** (D190). Prose that asserts a checkable fact about the
   code cites a test named `TestArch_<Claim>`. A cited name that no test defines is a
   claim nothing checks, wherever it is written: a comment in `.golangci.yml` or a gate
   script once kept naming a test that no longer existed (audit-2026-09-29 §6), so every
   tracked file is read. A `TestArch_` test that nothing cites in a doc (a `.md` file or a
   `doc.go`) is an invariant nobody was told about, so both directions fail. History (the
   decision logs and research) may cite anything, because it records what was true.
2. **Decision IDs in code resolve.** A `D<n>` in a Go, TypeScript, Python or shell comment
   must name a decision that exists and is not archived. An agent inventing a plausible
   citation is the failure. The docs gate already covers Markdown.
3. **Test variables are set by something** (D190). Every `ORPHEUM_TEST_<NAME>` read by code
   must appear in a Makefile, a CI workflow or a script that sets it. A variable only ever
   read gates a test that no job will ever run.
4. **Retired names stay retired.** When a decision renames something, the old name goes in
   `docs/contributors/retired-names.md`, and no tracked file outside history (and the
   agent-failures record, which quotes them, and persistent.db's schema steps, their test and goldens,
   which move rows from them) may use it again as a whole word. Old names left live recurred three times after D227's renames, each found
   by an audit and never by a gate (agent-failures A12). Taking a name off the list is a
   weakening `check_test_weakening.py` asks a trailer for.
5. **A cited test exists.** A doc.go, README or document outside history that names a Go test
   names one some test file defines; `TestX_*`, or `TestX_…`, names every test with that prefix,
   and there must be one. A step's test folded into the baseline survived in internal/store's
   doc.go as a citation of nothing (audit-2026-09-30 §5, settled 2026-10-01), and scenarios.md
   named five scenario tests never written as what showed a scenario held (audit-2026-10-03 §4).
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
# The record of what agents got wrong quotes the old names it got wrong with; a schema step
# names the old names it moves stored rows from, and so do its test and the goldens of the schema
# before it.
QUOTES_RETIRED_NAMES = HISTORY_PREFIXES + (
    "docs/contributors/agent-failures.md", "internal/store/migrations/", "internal/store/testdata/golden/",
    "internal/store/steps_test.go",
)
RETIRED_NAMES = "docs/contributors/retired-names.md"
# A row of the list: | `old` | `new` | D<n> |. The old name is the first cell's code span.
RETIRED_ROW = re.compile(r"^\|\s*`([^`]+)`\s*\|", re.M)
# The gates' own tests build failing cases out of synthetic names on purpose.
SYNTHETIC = ("scripts/tests/",)
# The testing standard teaches the shape of a test's name with names no test has.
EXAMPLE_NAMES = ("docs/contributors/standards/testing.md",)

ARCH_NAME = re.compile(r"\bTestArch_[A-Za-z0-9_]+")
# A Go test named in prose: Test<Subject>_<Claim>, or Test<Subject>_<Prefix>_* for every test so
# named. A name with no underscore is a word ("TestMain"), not a citation.
CITED_TEST = re.compile(r"\bTest[A-Z][A-Za-z0-9]*_[A-Za-z0-9_]*(?:\*|…)?")
GO_TEST_DEF = re.compile(r"^func (Test[A-Za-z0-9_]+)\(t \*testing\.T\)", re.M)
GO_ARCH_DEF = re.compile(r"^func (TestArch_[A-Za-z0-9_]+)\(t \*testing\.T\)", re.M)
TS_ARCH_DEF = re.compile(r"""\b(?:it|test)\(\s*["'`](TestArch_[A-Za-z0-9_]+)""")
DECISION_HEADING = re.compile(r"^## D(\d+) — ", re.M)
CODE_DECISION = re.compile(r"(?<![\w/#.-])D(\d{1,4})(?![\w-])")
TEST_VAR = re.compile(r"\bORPHEUM_TEST_[A-Z0-9_]+\b")

CODE_SUFFIXES = (".go", ".ts", ".tsx", ".js", ".mjs", ".cjs", ".py", ".sh")
GENERATED_PARTS = ("internal/store/sqlcgen/", "internal/store/cachegen/", "web/packages/sdk/src/gen/")
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


def arch_citations(files: dict[str, str], docs_only: bool = True) -> dict[str, list[str]]:
    """TestArch_ names cited outside history: in the docs (`.md` files and `doc.go`), or, with
    docs_only False, in any file, the tests themselves included."""
    cited: dict[str, list[str]] = {}
    for path, text in files.items():
        if path.startswith(HISTORY_PREFIXES + SYNTHETIC):
            continue
        is_test = path.endswith("_test.go") or re.search(r"\.test\.tsx?$", path)
        is_doc = path.endswith(".md") or path.endswith("doc.go")
        if docs_only and (is_test or not is_doc):
            continue
        for name in set(ARCH_NAME.findall(text)):
            cited.setdefault(name, []).append(path)
    return cited


def check_arch(files: dict[str, str]) -> list[Problem]:
    defined = arch_definitions(files)
    cited_in_docs = arch_citations(files)
    problems = [
        Problem(paths[0], f"cites `{name}`, which no test defines — a claim nothing checks")
        for name, paths in sorted(arch_citations(files, docs_only=False).items())
        if name not in defined
    ]
    problems += [
        Problem(path, f"defines `{name}`, which no doc cites — an invariant nobody was told "
                      "about; cite it in the owning package's doc.go or README")
        for name, path in sorted(defined.items())
        if name not in cited_in_docs
    ]
    return problems


def check_cited_tests(files: dict[str, str]) -> list[Problem]:
    """Go tests a doc.go, README or document outside history names that no test file defines;
    TestArch_ names are check_arch's."""
    defined = {name for path, text in files.items() if path.endswith("_test.go")
               for name in GO_TEST_DEF.findall(text)}
    problems: list[Problem] = []
    for path, text in sorted(files.items()):
        if path.startswith(HISTORY_PREFIXES + SYNTHETIC + EXAMPLE_NAMES) or not path.endswith(("doc.go", ".md")):
            continue
        for name in sorted(set(CITED_TEST.findall(text))):
            if name.startswith("TestArch_"):
                continue
            if name.endswith(("*", "…")):
                if not any(test.startswith(name[:-1]) for test in defined):
                    problems.append(Problem(path, f"cites `{name}`, but no test's name starts so"))
            elif name not in defined:
                problems.append(Problem(path, f"cites `{name}`, which no test defines — an invariant "
                                              "nothing enforces any more"))
    return problems


def retired_names(text: str) -> list[str]:
    """The old names the list of retired names holds, in its order."""
    return [m.group(1) for m in RETIRED_ROW.finditer(text) if m.group(1) not in ("Retired",)]


def check_retired(files: dict[str, str], retired: list[str]) -> list[Problem]:
    """Uses of a retired name, as a whole word, in any tracked file outside history."""
    if not retired:
        return []
    word = re.compile(r"(?<![A-Za-z0-9_])(" + "|".join(map(re.escape, sorted(retired, key=len, reverse=True)))
                      + r")(?![A-Za-z0-9_])")
    problems: list[Problem] = []
    for path, text in sorted(files.items()):
        if path == RETIRED_NAMES or path.startswith(QUOTES_RETIRED_NAMES + SYNTHETIC):
            continue
        for lineno, line in enumerate(text.splitlines(), 1):
            for m in word.finditer(line):
                problems.append(Problem(f"{path}:{lineno}", f"uses `{m.group(1)}`, a name a decision "
                                        f"retired; {RETIRED_NAMES} gives the name now"))
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


def read_text_files(root: Path, paths: list[str]) -> dict[str, str]:
    """Every file that is text, by path: one with a NUL byte in its first 8 KiB is not."""
    files: dict[str, str] = {}
    for p in paths:
        data = (root / p).read_bytes()
        if b"\0" not in data[:8192]:
            files[p] = data.decode("utf-8", errors="replace")
    return files


def check(root: Path) -> list[Problem]:
    files = read_text_files(root, tracked(root))
    live, archived = decision_status(files.get(LIVE_LOG, ""), files.get(ARCHIVE_LOG, ""))
    return (check_arch(files) + check_code_decisions(files, live, archived) + check_test_vars(files)
            + check_retired(files, retired_names(files.get(RETIRED_NAMES, ""))) + check_cited_tests(files))


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
    print("check_claims: architecture citations, cited tests, code decision IDs and test variables all "
          "resolve; no retired name is used")
    return 0


if __name__ == "__main__":
    sys.exit(main())
