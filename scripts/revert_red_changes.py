# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Which of a branch's files revert-red asks to go red: the commits' types, which files are
# ABOUTME: test code, and which kinds of file it has a probe for (check_revert_red.py).
"""The files a branch changed that revert-red probes, and the commits' claims about them.

Split from check_revert_red.py at the 800-line limit (2026-10-30); it reaches these from there.
"""

from __future__ import annotations

import functools
import re
from dataclasses import dataclass
from pathlib import Path

from revert_red_attempts import TS_SOURCE
from revert_red_cosmetic import GO_SOURCE, PY_SOURCE
from revert_red_dependents import git_run as _git
from revert_red_parts import CSS_SOURCE
from revert_red_units import MIGRATION_STEP

# A commit whose type claims a behaviour change owes a red-on-revert.
BEHAVIOR_TYPES = frozenset({"feat", "fix", "perf"})
# Everything else in the AGENTS.md rule-4 vocabulary. Listed rather than inferred by
# negation so an unrecognised type is a question, not a silent exemption.
EXEMPT_TYPES = frozenset({"docs", "test", "refactor", "build", "ci", "chore", "style", "revert"})

SUBJECT_RE = re.compile(r"^(?P<type>[a-z]+)(?:\([^)]*\))?(?P<bang>!)?:")

GO_TEST = re.compile(r"_test\.go$")

# A Python file is a test because of WHERE it lives, not what it is called. Every
# one of our Python tests sits in `scripts/tests/`. yoloAI identified them by
# filename once and got both directions wrong: its `scripts/smoke_test.py` was the only
# `*_test.py` path in the tree and is a 4000-line harness rather than a test, so
# every `fix` to it was silently exempt; while `tests/conftest.py` matches neither
# name pattern and was therefore treated as behavioral source that must go red.
# internal/testutil is test code by construction: only tests may import it
# (TestArch_TestSupportIsImportedOnlyByTests), so no change to it can alter what the binary
# does, and a helper only an integration test uses has no test in this suite to turn red
# (audit-2026-09-25 section 5.3).
GO_TEST_SUPPORT = "internal/testutil/"
# Generated query code has no tests of its own, and a query's body can change without its
# signature: a fix that makes a slow query fast is noticed only by the reference-scale test in
# internal/store's integration tier, which is where such a file is reverted against
# (audit-2026-09-25 section 5.3).
GO_GENERATED_QUERIES = ("internal/store/sqlcgen/", "internal/store/cachegen/")
# The schemas: a migration step (reverted with its checksum line and golden) and cache.db's,
# which only internal/store's tests read.
STORE_SCHEMA = "internal/store/cache/schema.sql"
# `*_test.go` is safe to match by name because in Go the compiler agrees; the same
# suffix in Python is a borrowed idiom that only ever matched the wrong file.
PY_TEST = re.compile(r"(^|/)tests/")

TS_TEST = re.compile(r"\.test\.tsx?$")

# A shell script is run by the Python tests that name it, as the install script and build.sh are;
# with no such kind, a fix to one alone was a blind spot (01827a66, 2026-10-18).
SHELL_SOURCE = re.compile(r"\.sh$")


@functools.cache
def is_python(path: str) -> bool:
    """Whether path is Python source: by its suffix, or, with none, by an interpreter line naming
    Python, as ./dev has. Read from the working tree, which is the branch's head."""
    if PY_SOURCE.search(path):
        return True
    if Path(path).suffix:
        return False
    try:
        with open(path, "rb") as source:
            first = source.readline(200)
    except OSError:
        return False
    return first.startswith(b"#!") and b"python" in first


@dataclass(frozen=True)
class Change:
    path: str
    status: str  # A, M, D


def commit_type(subject: str) -> str | None:
    """The conventional-commit type, or None when the subject does not carry one."""
    m = SUBJECT_RE.match(subject.strip())
    return m.group("type") if m else None


def claims_behavior_change(types: set[str | None]) -> bool:
    """Whether any commit touching a file claims to change behaviour.

    An unrecognised or missing type counts as claiming one. A subject this cannot
    parse is a commit-lint problem, and defaulting it to exempt would make "write an
    unparseable subject" a way out of the gate.
    """
    return any(t not in EXEMPT_TYPES for t in types)


def is_probed_kind(path: str) -> bool:
    """Whether this gate has a probe for this kind of file."""
    return bool(GO_SOURCE.search(path) or is_python(path) or TS_SOURCE.search(path)
                or CSS_SOURCE.search(path) or SHELL_SOURCE.search(path) or MIGRATION_STEP.match(path)
                or path == STORE_SCHEMA)


def changed_sources(base: str, head: str) -> list[Change]:
    out = _git("diff", "--name-status", f"{base}...{head}").stdout
    changes = []
    for line in out.splitlines():
        parts = line.split("\t")
        if len(parts) < 2:
            continue
        status, path = parts[0][:1], parts[-1]
        if is_test_file(path):
            continue  # a test file's own revert is not the question
        if is_probed_kind(path):
            changes.append(Change(path=path, status=status))
    return changes


def is_test_file(path: str) -> bool:
    """Whether this path is test code, and so not the thing being asked to go red.

    Each language is asked in its own terms: Go by the suffix its compiler already
    enforces, Python by the directory we actually keep its tests in. Kept
    as one function so the two rules cannot drift apart unnoticed, which is how the
    Python half of yoloAI's copy came to borrow Go's suffix and exempt a harness.
    """
    if GO_SOURCE.search(path):
        return bool(GO_TEST.search(path)) or path.startswith(GO_TEST_SUPPORT)
    if is_python(path):
        return bool(PY_TEST.search(path))
    if TS_SOURCE.search(path):
        return bool(TS_TEST.search(path))
    if SHELL_SOURCE.search(path):
        return path.startswith(GO_TEST_SUPPORT)  # the fake FFmpeg
    return False


def types_touching(base: str, head: str, path: str) -> set[str | None]:
    out = _git("log", "--no-merges", "--format=%s", f"{base}..{head}", "--", path).stdout
    return {commit_type(line) for line in out.splitlines() if line.strip()}
