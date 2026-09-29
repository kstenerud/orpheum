#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# ABOUTME: PR gate (`make check-pr`): lists every way a branch made the tests weaker —
# ABOUTME: deleted tests, new skips, fewer assertions, changed goldens — and fails unless
# ABOUTME: a commit in the range explains it with a `Test-Weakened:` trailer. Gate files
# ABOUTME: (scripts/gate_paths.py) changed on the branch need a `Gate-Changed:` trailer.
"""Refuse silent test weakening.

The cheapest way to make a failing check pass is to make the check ask less. Agents do this
in recognisable ways: they delete the failing test, add a skip, drop an assertion, loosen a
tolerance, or re-record a golden. yoloAI's A19 is the local specimen, *"silenced a linter by
narrowing a test"*.

Revert-and-require-red (`check_revert_red.py`) proves a test is wired to a change. It cannot
see a test that got weaker in the same branch. This gate lists the weakening itself:

* a Go `func Test…` or a TS `it(`/`test(` that existed at the base and no longer does;
* a new `t.Skip`/`t.SkipNow`/`.skip(`/`.todo(`/`.only(` line;
* a test file whose assertion count went down (`t.Error*`, `t.Fatal*`, `expect(`), where each
  row of a Go test table a `for … range` ranges over inline (`range []struct{…}{…}`,
  `range map[…]…{…}`) counts as one: a row is a check, so turning one assertion per case into a
  table of cases is not weakening, and dropping a row is. A table bound to a name first is not
  counted, since a slice literal of fixture data looks the same;
* any change to, or deletion of, a file under a `testdata/` golden or baseline directory
  (D110: owner-edited only). Adding a new golden weakens nothing and is not listed; as a gate
  path it still needs `Gate-Changed:`.
  Until migrations are live (D228), library.db's schema goldens are rewritten by each fold of
  the last step into the baseline, and are not listed either; they too need `Gate-Changed:`.

Each item fails the gate unless some commit in the range carries a
`Test-Weakened: <why>` trailer. The trailer is a claim on the record that review can weigh,
not a bypass: the reviewer is shown this same list.
"""

from __future__ import annotations

import argparse
import re
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path

GO_TEST_FUNC = re.compile(r"^func (Test\w+|Fuzz\w+|Benchmark\w+)\(", re.M)
TS_TEST_CASE = re.compile(r"""\b(?:it|test)\(\s*(["'`])(.+?)\1""")
NEW_SKIP = re.compile(r"\bt\.Skip(?:Now|f)?\(|\.(?:skip|todo|only)\(")
ASSERTION = re.compile(r"\bt\.(?:Error|Errorf|Fatal|Fatalf|Fail|FailNow)\(|\bexpect\(")
# A `for … range` over a literal slice or map: a Go test table written inline.
INLINE_TABLE = re.compile(r"\brange\s+(?=\[\]|map\[)")
TEST_FILE = re.compile(r"(_test\.go|\.test\.tsx?|/tests/test_\w+\.py)$")
GOLDEN = re.compile(r"(^|/)testdata/(golden|baselines?)/")
TRAILER = re.compile(r"^Test-Weakened:\s*(\S.*)$", re.M | re.I)
GATE_TRAILER = re.compile(r"^Gate-Changed:\s*(\S.*)$", re.M | re.I)


@dataclass(frozen=True)
class Weakening:
    path: str
    what: str


def git(*args: str) -> str:
    done = subprocess.run(["git", *args], capture_output=True, text=True, check=False)
    return done.stdout


def test_names(path: str, text: str) -> set[str]:
    if path.endswith(".go"):
        return set(GO_TEST_FUNC.findall(text))
    return {m.group(2) for m in TS_TEST_CASE.finditer(text)}


def _skip_quoted(text: str, i: int) -> int:
    """The index after the string, rune or comment starting at text[i], or i when none does."""
    if text.startswith("//", i):
        end = text.find("\n", i)
        return len(text) if end < 0 else end
    if text.startswith("/*", i):
        end = text.find("*/", i + 2)
        return len(text) if end < 0 else end + 2
    quote = text[i]
    if quote == "`":
        end = text.find("`", i + 1)
        return len(text) if end < 0 else end + 1
    if quote in "\"'":
        j = i + 1
        while j < len(text) and text[j] != quote:
            j += 2 if text[j] == "\\" else 1
        return j + 1
    return i


def _literal_start(text: str, i: int) -> int | None:
    """The index of the opening brace of the literal whose type starts at text[i]: past the
    type's brackets and any struct or interface body inside it."""
    depth = 0
    while i < len(text):
        skipped = _skip_quoted(text, i)
        if skipped != i:
            i = skipped
            continue
        char = text[i]
        if char in "[(":
            depth += 1
        elif char in "])":
            depth -= 1
        elif char == "{":
            if re.search(r"\b(struct|interface)\s*$", text[:i]):
                i = _block_end(text, i)
                continue
            if depth == 0:
                return i
        elif char in ";\n" and depth == 0:
            return None
        i += 1
    return None


def _block_end(text: str, i: int) -> int:
    """The index after the brace block opening at text[i]."""
    depth = 0
    while i < len(text):
        skipped = _skip_quoted(text, i)
        if skipped != i:
            i = skipped
            continue
        if text[i] == "{":
            depth += 1
        elif text[i] == "}":
            depth -= 1
            if depth == 0:
                return i + 1
        i += 1
    return i


def _elements(text: str, i: int) -> int:
    """How many elements the literal opening at text[i] holds."""
    depth, count, pending = 0, 0, False
    while i < len(text):
        skipped = _skip_quoted(text, i)
        if skipped != i:
            i, pending = skipped, pending or depth >= 1
            continue
        char = text[i]
        if char in "{[(":
            depth += 1
            pending = pending or depth > 1
        elif char in "}])":
            depth -= 1
            if depth == 0:
                return count + (1 if pending else 0)
        elif char == "," and depth == 1:
            count, pending = count + (1 if pending else 0), False
        elif not char.isspace() and depth >= 1:
            pending = True
        i += 1
    return count


def table_rows(path: str, text: str) -> int:
    """The rows of the Go test tables in text that a `for … range` ranges over inline."""
    if not path.endswith(".go"):
        return 0
    rows = 0
    for match in INLINE_TABLE.finditer(text):
        start = _literal_start(text, match.end())
        if start is not None:
            rows += _elements(text, start)
    return rows


def checks(path: str, text: str) -> int:
    """A test file's checks: its assertion calls, and the rows of its inline Go test tables."""
    return len(ASSERTION.findall(text)) + table_rows(path, text)


def compare(path: str, before: str, after: str) -> list[Weakening]:
    """Weakenings between two versions of one test file; `after` is '' if deleted."""
    found: list[Weakening] = []
    for gone in sorted(test_names(path, before) - test_names(path, after)):
        found.append(Weakening(path, f"test {gone!r} was removed"))
    before_skips = len(NEW_SKIP.findall(before))
    after_skips = len(NEW_SKIP.findall(after))
    if after_skips > before_skips:
        found.append(Weakening(path, f"{after_skips - before_skips} new skip/only/todo"))
    before_asserts = checks(path, before)
    after_asserts = checks(path, after)
    if after and after_asserts < before_asserts:
        found.append(Weakening(path, f"assertions went from {before_asserts} to {after_asserts}"))
    return found


def golden_weakening(status: str, path: str, are_migrations_live: bool = True) -> Weakening | None:
    """A golden or baseline that was changed, deleted or renamed may now ask less. A new one
    adds a check and weakens nothing; it is still a gate path, so the owner's `Gate-Changed:`
    trailer is still required for it. Until migrations are live (D228), library.db's schema
    goldens follow each fold of the last step into the baseline, and the test that compares
    them with a fresh schema still runs, so rewriting or removing one weakens nothing either."""
    import gate_paths

    if status == "A":
        return None
    if not are_migrations_live and path.startswith(gate_paths.SCHEMA_GOLDENS_DIR):
        return None
    return Weakening(path, "a golden or baseline changed (D110: owner-edited only)")


def weakenings(base: str, head: str) -> list[Weakening]:
    import gate_paths

    found: list[Weakening] = []
    are_live = gate_paths.migrations_are_live(Path.cwd(), head)
    for line in git("diff", "--name-status", f"{base}...{head}").splitlines():
        parts = line.split("\t")
        if len(parts) < 2:
            continue
        status, path = parts[0][:1], parts[1]
        if GOLDEN.search(path):
            weakened = golden_weakening(status, path, are_live)
            if weakened is not None:
                found.append(weakened)
            continue
        if not TEST_FILE.search(path) or status == "A":
            continue
        before = git("show", f"{base}:{path}")
        after = "" if status == "D" else git("show", f"{head}:{path}")
        found += compare(path, before, after)
    return found


def trailers(base: str, head: str, pattern: re.Pattern[str] = TRAILER) -> list[str]:
    return pattern.findall(git("log", "--format=%B", f"{base}..{head}"))


def gate_changes(base: str, head: str) -> list[str]:
    """Gate files the branch changed (scripts/gate_paths.py), which need the owner's trailer."""
    import gate_paths

    changed = git("diff", "--name-only", f"{base}...{head}").splitlines()
    return [p for p in changed if gate_paths.is_gate(p)]


def check_gates(base: str, head: str) -> int:
    changed = gate_changes(base, head)
    if not changed:
        return 0
    reasons = trailers(base, head, GATE_TRAILER)
    print(f"Gate files changed in {base}..{head}:")
    for path in changed:
        print(f"  {path}")
    if reasons:
        print("Explained by Gate-Changed: trailer(s) — the owner's to accept:")
        for r in reasons:
            print(f"  - {r}")
        return 0
    print(
        "\nNo commit explains this. Gates decide what passing means, so a change to one needs\n"
        "the owner's `Gate-Changed: <why>` trailer (AGENTS.md rule 2)."
    )
    return 1


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--base", required=True)
    ap.add_argument("--head", default="HEAD")
    args = ap.parse_args()
    gate_status = check_gates(args.base, args.head)
    found = weakenings(args.base, args.head)
    if not found:
        print(f"check_test_weakening: nothing weakened in {args.base}..{args.head}")
        return gate_status
    reasons = trailers(args.base, args.head)
    print(f"Tests weakened in {args.base}..{args.head}:")
    for w in found:
        print(f"  {w.path}: {w.what}")
    if reasons:
        print("\nExplained by Test-Weakened: trailer(s) — review weighs these:")
        for r in reasons:
            print(f"  - {r}")
        return gate_status
    print(
        "\nNo commit explains this. If the weakening is right — a test that tested the wrong\n"
        "thing, a golden re-recorded by the owner — add a `Test-Weakened: <why>` trailer to the\n"
        "commit. If it is not, restore the test and fix the code instead."
    )
    return 1


if __name__ == "__main__":
    sys.exit(main())
