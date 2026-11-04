#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: PR gate (`make check-pr`): lists every way a branch made the tests weaker —
# ABOUTME: deleted tests, new skips, fewer assertions, changed goldens, retired names let go —
# ABOUTME: and fails unless a commit that touched that file explains it with a `Test-Weakened:`
# ABOUTME: trailer. A gate file (scripts/gate_paths.py) needs a `Gate-Changed:` trailer the same way.
"""Refuse silent test weakening.

The cheapest way to make a failing check pass is to make the check ask less. Agents do this
in recognisable ways: they delete the failing test, add a skip, drop an assertion, loosen a
tolerance, or re-record a golden. yoloAI's A19 is the local specimen, *"silenced a linter by
narrowing a test"*.

Revert-and-require-red (`check_revert_red.py`) proves a test is wired to a change. It cannot
see a test that got weaker in the same branch. This gate lists the weakening itself:

* a Go `func Test…`, a TS `it(`/`test(` or a Python `def test_…` that existed at the base and
  no longer does, unless a test with the same body (its name aside) is in the branch's test
  files: that test was renamed or moved, not removed. A moved test's checks leave its old file
  with it, so they are not counted as that file's loss. A test renamed with its body, when the
  rename reaches inside it (a renamed setting it names), is one too when a commit in the range
  says so with `Test-Renamed: <file>: "<old name>" -> "<new name>"`, and the new test is in that
  file with at least the old one's checks. A test moved to another file and changed there says
  so with `Test-Renamed: <file>: "<old name>" -> <new file>: "<new name>"`, and the new test is in
  the new file with at least the old one's checks. The claim names its file and is checked against it, so
  unlike `Test-Weakened:` it need not ride on a commit that touched the file; one that does not
  hold is listed;
* a new `t.Skip`/`t.SkipNow`/`.skip(`/`.todo(`/`.only(` line, or in Python `self.skipTest(`,
  `@unittest.skip…` or `@unittest.expectedFailure`;
* a test file whose assertion count went down (`t.Error*`, `t.Fatal*`, `expect(`, Python's
  `self.assert…(`, `self.fail(` and `assert`), where each
  row of a Go test table a `for … range` ranges over inline (`range []struct{…}{…}`,
  `range map[…]…{…}`) counts as one: a row is a check, so turning one assertion per case into a
  table of cases is not weakening, and dropping a row is. A table bound to a name first is not
  counted, since a slice literal of fixture data looks the same;
* any change to, or deletion of, a file under a `testdata/` golden or baseline directory
  (D110: owner-edited only). Adding a new golden weakens nothing and is not listed; as a gate
  path it still needs `Gate-Changed:`.
  Until migrations are live (D228), persistent.db's schema goldens are rewritten by each fold of
  the last step into the baseline, and are not listed either; they too need `Gate-Changed:`;
* a name that left the list of retired names (docs/contributors/retired-names.md): the list is
  what a test searches the tree for, so dropping a name stops that search.

Each item fails the gate unless a commit in the range that touched that file carries a
`Test-Weakened: <why>` trailer, and each changed gate file unless a commit that touched it
carries `Gate-Changed: <why>`. A trailer anywhere in the range once excused everything in it, so
one explained edit let every other through unexplained (audit-2026-09-29 §6). The trailer is a
claim on the record that review can weigh, not a bypass: the reviewer is shown this same list.
"""

from __future__ import annotations

import argparse
import ast
import os
import re
import subprocess
import sys
import time
from collections.abc import Callable
from dataclasses import dataclass
from pathlib import Path

import check_claims

GO_TEST_FUNC = re.compile(r"^func (Test\w+|Fuzz\w+|Benchmark\w+)\(", re.M)
# Any function or method of a Go test file: a helper is one that is not a test.
GO_FUNC = re.compile(r"^func (?:\([^)]*\) )?(\w+)\(", re.M)
TS_TEST_CASE = re.compile(r"""\b(?:it|test)\(\s*(["'`])(.+?)\1""")
PY_TEST_FUNC = re.compile(r"^\s*def (test_\w+)\(", re.M)
NEW_SKIP = re.compile(r"\bt\.Skip(?:Now|f)?\(|\.(?:skip|todo|only)\(")
PY_SKIP = re.compile(r"\bself\.skipTest\(|@(?:unittest\.)?(?:skip\w*|expectedFailure)\b")
ASSERTION = re.compile(r"\bt\.(?:Error|Errorf|Fatal|Fatalf|Fail|FailNow)\(|\bexpect\(")
PY_ASSERTION = re.compile(r"\bself\.(?:assert\w*|fail)\(|^\s*assert\b", re.M)
# A `for … range` over a literal slice or map: a Go test table written inline.
INLINE_TABLE = re.compile(r"\brange\s+(?=\[\]|map\[)")
TEST_FILE = re.compile(r"(_test\.go|\.test\.tsx?|/tests/test_\w+\.py)$")
GOLDEN = re.compile(r"(^|/)testdata/(golden|baselines?)/")
TRAILER = re.compile(r"^Test-Weakened:\s*(\S.*)$", re.M | re.I)
# Test-Renamed: <file>: "<old name>" -> [<new file>:] "<new name>", a rename that reached into the
# test's body, in its file or moving it to another.
RENAMED_TRAILER = re.compile(r'^Test-Renamed:\s*(\S+):\s*"(.+)"\s*->\s*(?:(\S+):\s*)?"(.+)"\s*$', re.M | re.I)
GATE_TRAILER = re.compile(r"^Gate-Changed:\s*(\S.*)$", re.M | re.I)


@dataclass(frozen=True)
class Weakening:
    path: str
    what: str


def git(*args: str) -> str:
    done = subprocess.run(["git", *args], capture_output=True, text=True, check=False)
    return done.stdout


def test_names(path: str, text: str) -> set[str]:
    return set(test_bodies(path, text))


def test_bodies(path: str, text: str) -> dict[str, str]:
    """Each test in a test file, by name, with its source: a Go test function, a TS `it`/`test`
    call, a Python `test_` method (named `Class.test_x`, so two classes' same names differ)."""
    if path.endswith(".go"):
        found = {}
        for m in GO_TEST_FUNC.finditer(text):
            brace = text.find("{", m.end())
            found[m.group(1)] = text[m.start():_block_end(text, brace) if brace >= 0 else len(text)]
        return found
    if path.endswith(".py"):
        return _python_tests(text)
    found = {}
    for m in TS_TEST_CASE.finditer(text):
        found[m.group(2)] = text[m.start():_bracket_end(text, text.index("(", m.start()), "(", ")")]
    return found


def helper_bodies(path: str, text: str) -> dict[str, str]:
    """Each helper in a Go or Python test file, by name, with its source: a function that is not a
    test. A helper moved to another test file takes its checks with it (process review F18, 2026-10-26:
    a helper's two checks moved with D351's assembly and counted as lost). TypeScript is not parsed."""
    if path.endswith(".go"):
        found = {}
        for m in GO_FUNC.finditer(text):
            if GO_TEST_FUNC.match(text, m.start()):
                continue
            brace = text.find("{", m.end())
            found[m.group(1)] = text[m.start():_block_end(text, brace) if brace >= 0 else len(text)]
        return found
    if path.endswith(".py"):
        try:
            tree = ast.parse(text)
        except SyntaxError:
            return {}
        return {node.name: ast.get_source_segment(text, node) or node.name for node in tree.body
                if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)) and not node.name.startswith("test_")}
    return {}


def _python_tests(text: str) -> dict[str, str]:
    try:
        tree = ast.parse(text)
    except SyntaxError:
        return {name: name for name in PY_TEST_FUNC.findall(text)}
    found = {}
    for node in tree.body:
        functions = [(node.name + ".", f) for f in node.body] if isinstance(node, ast.ClassDef) else [("", node)]
        for prefix, function in functions:
            if isinstance(function, (ast.FunctionDef, ast.AsyncFunctionDef)) and function.name.startswith("test_"):
                found[prefix + function.name] = ast.get_source_segment(text, function) or function.name
    return found


def same_body_key(name: str, body: str) -> str:
    """A test's body with its own name taken out and its spacing made uniform: two tests with
    the same key are one test under two names, or in two places."""
    short = name.rsplit(".", 1)[-1]
    return re.sub(r"\s+", " ", body.replace(short, "\x00")).strip()


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
    return _bracket_end(text, i, "{", "}")


def _bracket_end(text: str, i: int, opening: str, closing: str) -> int:
    """The index after the bracketed span opening at text[i]."""
    depth = 0
    while i < len(text):
        skipped = _skip_quoted(text, i)
        if skipped != i:
            i = skipped
            continue
        if text[i] == opening:
            depth += 1
        elif text[i] == closing:
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
    pattern = PY_ASSERTION if path.endswith(".py") else ASSERTION
    return len(pattern.findall(text)) + table_rows(path, text)


def skips(path: str, text: str) -> int:
    return len((PY_SKIP if path.endswith(".py") else NEW_SKIP).findall(text))


def compare(path: str, before: str, after: str,
            elsewhere: dict[str, str] | None = None) -> list[Weakening]:
    """Weakenings between two versions of one test file; `after` is '' if deleted. `elsewhere`
    holds the branch's other tests, by `same_body_key`, naming where each is: a test gone from
    this file whose body is there, or in this file under another name, was moved."""
    return compare_with_moves(path, before, after, elsewhere)[0]


def compare_with_moves(path: str, before: str, after: str,
                       elsewhere: dict[str, str] | None = None,
                       renames: dict[str, tuple[str, str]] | None = None,
                       tests_in: Callable[[str], dict[str, str]] | None = None,
                       helpers_elsewhere: dict[str, str] | None = None,
                       ) -> tuple[list[Weakening], list[str]]:
    """compare's weakenings, and the moves it did not count as removals, as sentences. `renames`
    are this file's Test-Renamed claims, old name to (the file it is now in, new name); `tests_in`
    reads another file's tests at the range's head, for a claim that names one."""
    found: list[Weakening] = []
    moves: list[str] = []
    before_tests, after_tests = test_bodies(path, before), test_bodies(path, after)
    here = {same_body_key(name, body): f"{path}: {name!r}" for name, body in after_tests.items()}
    moved_out = 0
    for gone in sorted(set(before_tests) - set(after_tests)):
        key = same_body_key(gone, before_tests[gone])
        destination = here.get(key) or (elsewhere or {}).get(key)
        if destination is None and gone in (renames or {}):
            new_path, new_name = (renames or {})[gone]
            now_in = after_tests if new_path == path else (tests_in or (lambda _: {}))(new_path)
            problem = rename_problem(before_tests[gone], now_in, new_name, new_path)
            if problem:
                found.append(Weakening(path, f"test {gone!r} {problem}"))
                continue
            where = repr(new_name) if new_path == path else f"{new_path}: {new_name!r}"
            moves.append(f"{path}: test {gone!r} is now {where} (Test-Renamed), its checks kept")
            if new_path != path:
                moved_out += checks(path, before_tests[gone])
            continue
        if destination is None:
            found.append(Weakening(path, f"test {gone!r} was removed"))
            continue
        moves.append(f"{path}: test {gone!r} is now {destination}, with the same body")
        if key not in here:
            moved_out += checks(path, before_tests[gone])
    before_helpers, after_helpers = helper_bodies(path, before), helper_bodies(path, after)
    for gone in sorted(set(before_helpers) - set(after_helpers)):
        destination = (helpers_elsewhere or {}).get(same_body_key(gone, before_helpers[gone]))
        if destination is not None:
            moves.append(f"{path}: helper {gone!r} is now {destination}, with the same body")
            moved_out += checks(path, before_helpers[gone])
    before_skips, after_skips = skips(path, before), skips(path, after)
    if after_skips > before_skips:
        found.append(Weakening(path, f"{after_skips - before_skips} new skip/only/todo"))
    before_asserts = checks(path, before) - moved_out
    after_asserts = checks(path, after)
    if after and after_asserts < before_asserts:
        found.append(Weakening(path, f"assertions went from {before_asserts} to {after_asserts}"))
    return found, moves


def rename_problem(old_body: str, after_tests: dict[str, str], new_name: str, path: str) -> str:
    """Why a Test-Renamed claim does not hold, or "" when it does: the new test is in `path`, the
    file the claim names, and keeps at least the old one's checks."""
    if new_name not in after_tests:
        return (f"was claimed renamed to {new_name!r} (Test-Renamed), but no test of that name is in "
                f"the file ({path})")
    kept, had = checks(path, after_tests[new_name]), checks(path, old_body)
    if kept < had:
        return f"was claimed renamed to {new_name!r} (Test-Renamed), but its checks went from {had} to {kept}"
    return ""


def renamed_tests(base: str, head: str, path: str) -> dict[str, tuple[str, str]]:
    """The range's Test-Renamed claims for a file, old name to (the file it is now in, new name).
    Claims are read oldest first, so a test renamed again is followed to its last name (a claim
    whose old name is an earlier claim's new name, in that file) and a later claim for the same
    old name replaces an earlier one."""
    log = git("log", "--reverse", "--format=%B", f"{base}..{head}")
    renamed: dict[str, tuple[str, str]] = {}
    for claimed, old, new_path, new in RENAMED_TRAILER.findall(log):
        now = (new_path or claimed, new)
        earlier = [first for first, was in renamed.items() if was == (claimed, old)]
        for first in earlier:
            renamed[first] = now
        if claimed == path and not earlier:
            renamed[old] = now
    return renamed


def golden_weakening(status: str, path: str, are_migrations_live: bool = True) -> Weakening | None:
    """A golden or baseline that was changed, deleted or renamed may now ask less. A new one
    adds a check and weakens nothing; it is still a gate path, so the owner's `Gate-Changed:`
    trailer is still required for it. Until migrations are live (D228), persistent.db's schema
    goldens follow each fold of the last step into the baseline, and the test that compares
    them with a fresh schema still runs, so rewriting or removing one weakens nothing either."""
    import gate_paths

    if status == "A":
        return None
    if not are_migrations_live and path.startswith(gate_paths.SCHEMA_GOLDENS_DIR):
        return None
    return Weakening(path, "a golden or baseline changed (D110: owner-edited only)")


def retired_names_weakening(before: str, after: str) -> list[Weakening]:
    """Names that left the list of retired names, which a test searches the tree for."""
    gone = set(check_claims.retired_names(before)) - set(check_claims.retired_names(after))
    return [Weakening(check_claims.RETIRED_NAMES, f"retired name {name!r} left the list, so the tree "
                                                  "is no longer searched for it")
            for name in sorted(gone)]


def tests_at(head: str, path: str) -> dict[str, str]:
    """A test file's tests at head, or none when it is not there."""
    return test_bodies(path, git("show", f"{head}:{path}"))


def changed(base: str, head: str) -> list[tuple[str, str]]:
    """(status, path) for each file the branch changed; a renamed file is its new path."""
    rows = []
    for line in git("diff", "--name-status", f"{base}...{head}").splitlines():
        parts = line.split("\t")
        if len(parts) >= 2:
            rows.append((parts[0][:1], parts[-1]))
    return rows


def weakenings(base: str, head: str, moves: list[str] | None = None) -> list[Weakening]:
    """Every weakening in the range; the renamed or moved tests go to `moves`, when given."""
    import gate_paths

    found: list[Weakening] = []
    are_live = gate_paths.migrations_are_live(Path.cwd(), head)
    rows = changed(base, head)
    elsewhere: dict[str, str] = {}
    helpers_elsewhere: dict[str, str] = {}
    for status, path in rows:
        if TEST_FILE.search(path) and status != "D":
            text = git("show", f"{head}:{path}")
            for name, body in test_bodies(path, text).items():
                elsewhere.setdefault(same_body_key(name, body), f"{path}: {name!r}")
            for name, body in helper_bodies(path, text).items():
                helpers_elsewhere.setdefault(same_body_key(name, body), f"{path}: {name!r}")
    for status, path in rows:
        if GOLDEN.search(path):
            weakened = golden_weakening(status, path, are_live)
            if weakened is not None:
                found.append(weakened)
            continue
        if path == check_claims.RETIRED_NAMES and status != "A":
            after = "" if status == "D" else git("show", f"{head}:{path}")
            found += retired_names_weakening(git("show", f"{base}:{path}"), after)
            continue
        if not TEST_FILE.search(path) or status == "A":
            continue
        before = git("show", f"{base}:{path}")
        after = "" if status == "D" else git("show", f"{head}:{path}")
        weakened, moved = compare_with_moves(path, before, after, elsewhere, renamed_tests(base, head, path),
                                             lambda other: tests_at(head, other), helpers_elsewhere)
        found += weakened
        if moves is not None:
            moves += moved
    return found


def trailers(base: str, head: str, pattern: re.Pattern[str] = TRAILER, path: str | None = None) -> list[str]:
    """The trailers matching `pattern` on the range's commits: all of them, or, given a path,
    only those on the commits that touched it."""
    return pattern.findall(git("log", "--format=%B", f"{base}..{head}", *(["--", path] if path else [])))


def explain(base: str, head: str, paths: list[str],
            pattern: re.Pattern[str]) -> tuple[dict[str, list[str]], list[str]]:
    """({path: the reasons on the commits that touched it}, [paths none of them explains])."""
    explained: dict[str, list[str]] = {}
    unexplained: list[str] = []
    for path in paths:
        reasons = trailers(base, head, pattern, path)
        if reasons:
            explained[path] = reasons
        else:
            unexplained.append(path)
    return explained, unexplained


def gate_changes(base: str, head: str) -> list[str]:
    """Gate files the branch changed (scripts/gate_paths.py), which need the owner's trailer."""
    import gate_paths

    return [p for p in git("diff", "--name-only", f"{base}...{head}").splitlines() if gate_paths.is_gate(p)]


def check_gates(base: str, head: str) -> int:
    changed_gates = gate_changes(base, head)
    if not changed_gates:
        return 0
    explained, unexplained = explain(base, head, changed_gates, GATE_TRAILER)
    print(f"Gate files changed in {base}..{head}:")
    for path, reasons in explained.items():
        print(f"  {path} — Gate-Changed: {'; '.join(dict.fromkeys(reasons))}")
    for path in unexplained:
        print(f"  {path} — no commit that touched it says why")
    if not unexplained:
        print("Each is explained by a Gate-Changed: trailer on a commit that touched it — the owner's to accept.")
        return 0
    print(
        "\nGates decide what passing means, so a change to one needs the owner's\n"
        "`Gate-Changed: <why>` trailer on a commit that touched it (AGENTS.md rule 2). A trailer\n"
        "on another commit explains that commit's gate files, not these."
    )
    return 1


# How many times, and how far apart, the commit being written is made before the check gives up:
# another git process holds the index's lock for well under a second.
WRITE_TREE_TRIES = 5
WRITE_TREE_RETRY_S = 0.5


class CannotCheck(Exception):
    """The commit being written could not be made into a commit to check."""


def commit_being_written(message_file: str) -> tuple[str, str] | None:
    """(HEAD, a commit of the index with the message being written on HEAD), for the commit-msg
    hook: the commit is a dangling object no branch names, so the range's rules apply to the commit
    as it will be recorded, its own trailers alone explaining it (D270 §4). None before a first
    commit; CannotCheck when the commit cannot be made, which fails rather than passes unchecked.
    Another git process holding the index's lock, as an agent committing beside another does, makes
    write-tree fail for a moment: it is tried again, HEAD read anew, before it fails (2026-10-24)."""
    failure = ""
    for attempt in range(WRITE_TREE_TRIES):
        if attempt:
            time.sleep(WRITE_TREE_RETRY_S)
        head = git("rev-parse", "-q", "--verify", "HEAD").strip()
        if not head:
            return None
        written = subprocess.run(["git", "write-tree"], capture_output=True, text=True, check=False)
        tree = written.stdout.strip()
        if written.returncode != 0 or not tree:
            failure = written.stderr.strip()
            continue
        # The object is never recorded: a fixed name stands in where git has none, as under the gate.
        env = {**os.environ, "GIT_AUTHOR_NAME": "commit-msg check", "GIT_AUTHOR_EMAIL": "check@localhost",
               "GIT_COMMITTER_NAME": "commit-msg check", "GIT_COMMITTER_EMAIL": "check@localhost"}
        done = subprocess.run(["git", "commit-tree", tree, "-p", head, "-F", message_file], capture_output=True,
                              text=True, check=False, env=env)
        commit = done.stdout.strip()
        if done.returncode == 0 and commit:
            return head, commit
        failure = done.stderr.strip()
    raise CannotCheck(f"the commit being written could not be checked: {failure}")


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--base")
    ap.add_argument("--head", default="HEAD")
    ap.add_argument("--message-file", help="check the commit being written with this message (commit-msg)")
    args = ap.parse_args(argv)
    if args.message_file:
        try:
            being_written = commit_being_written(args.message_file)
        except CannotCheck as cannot:
            print(f"check_test_weakening: {cannot}", file=sys.stderr)
            return 1
        if being_written is None:
            return 0
        args.base, args.head = being_written
    elif not args.base:
        ap.error("--base is required unless --message-file is given")
    # The commit being written is a dangling stand-in under a placeholder author, never recorded:
    # its hash is not named, or an agent reports it as its commit and a pick records the stand-in
    # (2026-11-04: two commits picked as "commit-msg check <check@localhost>").
    span = "the commit being written" if args.message_file else f"{args.base}..{args.head}"
    gate_status = check_gates(args.base, args.head)
    moves: list[str] = []
    found = weakenings(args.base, args.head, moves)
    if moves:
        print(f"Tests renamed or moved in {span}, not counted as removed:")
        for move in moves:
            print(f"  {move}")
    if not found:
        print(f"check_test_weakening: nothing weakened in {span}")
        return gate_status
    paths = list(dict.fromkeys(w.path for w in found))
    explained, unexplained = explain(args.base, args.head, paths, TRAILER)
    print(f"Tests weakened in {span}:")
    for w in found:
        print(f"  {w.path}: {w.what}")
    if explained:
        print("\nExplained by a Test-Weakened: trailer on a commit that touched the file — review weighs these:")
        for path, reasons in explained.items():
            print(f"  {path}: {'; '.join(dict.fromkeys(reasons))}")
    if not unexplained:
        return gate_status
    print("\nNo commit that touched these explains them:")
    for path in unexplained:
        print(f"  {path}")
    print(
        "\nIf the weakening is right — a test that tested the wrong thing, a golden re-recorded\n"
        "by the owner — add a `Test-Weakened: <why>` trailer to a commit that touches the file.\n"
        "If a test was renamed and the rename reached into its body, say so on any commit with\n"
        "`Test-Renamed: <file>: \"<old name>\" -> \"<new name>\"`, or, for one moved to another file,\n"
        "`Test-Renamed: <file>: \"<old name>\" -> <new file>: \"<new name>\"`; the gate checks it keeps\n"
        "its checks.\n"
        "If it is not, restore the test and fix the code instead."
    )
    return 1


if __name__ == "__main__":
    sys.exit(main())
