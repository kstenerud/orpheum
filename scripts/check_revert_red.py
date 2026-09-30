#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# Adapted from yoloAI's scripts/check_revert_red.py (MIT, same author).
# ABOUTME: CI gate. For each source file a branch changes, reverts it to the base
# ABOUTME: version and asserts some test goes red — the mechanical form of AGENTS.md
# ABOUTME: rule (D190), which otherwise depends on the author remembering to check.

"""Revert each changed file and require something to fail.

D190 and AGENTS.md say every behaviour change carries a test that fails when that
change is reverted, and the load-bearing word is *every*: count the behaviour changes,
count the tests that would go red, make the numbers match. yoloAI learned to actually
revert each one and watch it fail, because a test that merely covers the line looks
identical until you try.

**Orpheum's addition (D190):** a `Verified-By:` trailer must name a verification-queue
item — `VQ<n>` — that exists in `docs/contributors/verification-queue.md` and is ticked.
yoloAI's free-text trailer was never used, because nothing checked it.

That is a mechanical procedure described in prose and left to a human to run, which
is the shape of every rule yoloAI had to build a gate for. This runs it.

**What it proves and what it does not.** Red-on-revert shows a test is wired to the
code. It cannot show the code is wired to the *program* — reverting a line inside a
function nothing can reach still turns its test red, and six tests in yoloAI's
history certified exactly that kind of dead path. Rule 10's guidance about naming the
implementations that pass a capability guard is the answer to that one, and it stays human.

**Scope, deliberately narrow, for the reason `check_citation_provenance.py` gives
about itself: a noisy gate gets disabled.**

* Only files whose changes are claimed to be behavioral, read off the conventional
  commit type we already write. `feat`, `fix` and `perf` must go red; `refactor`,
  `docs`, `test`, `ci`, `build`, `chore` need not, because a refactor that changed
  behaviour is a mislabelled commit and that is a different gate's problem. A file
  touched by several commits requires red if *any* of them claims behaviour.
* Only kinds of file whose tests this can actually scope and run: Go, Python and web
  TypeScript; web CSS, in the browser tier alone, since only a browser renders it; a
  migration step, as one unit with its checksum line and schema golden, and cache.db's
  schema, against internal/store's integration tier (revert_red_units.py). Every other
  file a feat/fix/perf commit changed is listed as UNCHECKED, by name and why, rather than
  passed — silence is what makes a green mean two things at once, and a `fix` that changed
  only a stylesheet once passed a gate that never looked at it (audit-2026-09-29 §6).
* A file whose part of a behaviour commit is a refactor can say so: a
  `Refactor-Only: <path> — <why>` trailer on a commit that touched it. Like `Verified-By:`,
  it is listed under "on the author's word" for review to weigh, never passed silently.

**A build failure counts as red, and is reported separately.** It proves the change
is load-bearing; it does not prove a test asserts its behaviour. Conflating the two
would let a compile-time dependency stand in for a missing test.

That distinction is much weaker in Python than in Go, and the difference is worth
knowing before reading a report. A Python test names what it imports, so removing a
newly-added function breaks collection before any assertion runs: **an added Python
function can essentially only ever report `RED_BUILD`**, however well it is tested.
Go reaches `RED_TEST` for the same change whenever the symbol survives the revert.
Measured by running this gate against its own commit.

Everything happens in a throwaway git worktree. This tool checks out old versions of
files over a tree, and doing that in the user's working tree would put uncommitted
work one interrupted run away from being lost.
"""

from __future__ import annotations

import argparse
import contextlib
import fcntl
import functools
import ast
import os
import re
import queue
import subprocess
import sys
import threading
from concurrent.futures import ThreadPoolExecutor
from dataclasses import dataclass
from pathlib import Path

import revert_red_reruns as reruns
from revert_red_trees import link_installed_packages
from revert_red_units import MIGRATION_STEP, companions, query_source, restore, revert_companion

# A commit whose type claims a behaviour change owes a red-on-revert.
BEHAVIOR_TYPES = frozenset({"feat", "fix", "perf"})
# Everything else in the AGENTS.md rule-4 vocabulary. Listed rather than inferred by
# negation so an unrecognised type is a question, not a silent exemption.
EXEMPT_TYPES = frozenset({"docs", "test", "refactor", "build", "ci", "chore", "style", "revert"})

SUBJECT_RE = re.compile(r"^(?P<type>[a-z]+)(?:\([^)]*\))?(?P<bang>!)?:")

GO_SOURCE = re.compile(r"\.go$")
GO_TEST = re.compile(r"_test\.go$")
# `make check` also tests the dev build and vets for macOS, so a file only those compile — a
# `//go:build dev` file, a `_darwin.go` shim — is reverted against them too, rather than
# reported as noticed by nothing.
GO_EXTRA_TAGS = "dev"
GO_OS_SUFFIX = re.compile(r"_(darwin|windows|freebsd|openbsd|netbsd)(?:_[a-z0-9]+)?\.go$")
# A Linux file for another architecture than the machine's, compiled only there.
GO_LINUX_ARCH_SUFFIX = re.compile(r"_linux_(arm64|riscv64|arm|386|ppc64le|s390x)\.go$")
# A build line that leaves Linux out: the file is compiled only for another system.
GO_NOT_LINUX = re.compile(r"^//go:build .*!linux", re.MULTILINE)
PY_SOURCE = re.compile(r"\.py$")

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
STORE_INTEGRATION = ["go", "test", "-tags", "integration", "./internal/store/..."]
# `*_test.go` is safe to match by name because in Go the compiler agrees; the same
# suffix in Python is a borrowed idiom that only ever matched the wrong file.
PY_TEST = re.compile(r"(^|/)tests/")

TS_SOURCE = re.compile(r"^web/(packages|apps)/[^/]+/src/.*\.tsx?$")
TS_TEST = re.compile(r"\.test\.tsx?$")
# A stylesheet only a browser renders: vitest and the type check never read it.
CSS_SOURCE = re.compile(r"^web/(packages|apps)/[^/]+/src/.*\.css$")

# `Refactor-Only: <path> — <why>`: this file's part of a behaviour commit changed no behaviour.
REFACTOR_ONLY = re.compile(r"^Refactor-Only:\s*(\S+)\s+(?:—|--|-)\s*(\S.*)$", re.M | re.I)

VQ_FILE = "docs/contributors/verification-queue.md"
VQ_REF = re.compile(r"\bVQ(\d+)\b")
VQ_TICKED = re.compile(r"^- \[[xX]\] \*\*VQ(\d+)\b", re.M)
VQ_ANY = re.compile(r"^- \[[ xX]\] \*\*VQ(\d+)\b", re.M)

# Directories pytest is pointed at; a Python change outside them has no scope to run.
PY_ROOTS = ("scripts",)


@dataclass(frozen=True)
class Change:
    path: str
    status: str  # A, M, D


@dataclass(frozen=True)
class Outcome:
    path: str
    verdict: str  # RED_TEST | RED_BUILD | GREEN | NO_RUNNER | CLAIMED | COSMETIC | UNCHECKED
    detail: str = ""

    @property
    def is_failure(self) -> bool:
        return self.verdict in ("GREEN", "NO_RUNNER")


def _git(*args: str, cwd: str | None = None) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["git", *args], capture_output=True, text=True, cwd=cwd, check=False
    )


def rev_exists(ref: str) -> bool:
    return _git("rev-parse", "--verify", "--quiet", f"{ref}^{{commit}}").returncode == 0


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
    return bool(GO_SOURCE.search(path) or PY_SOURCE.search(path) or TS_SOURCE.search(path)
                or CSS_SOURCE.search(path) or MIGRATION_STEP.match(path) or path == STORE_SCHEMA)


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
    if PY_SOURCE.search(path):
        return bool(PY_TEST.search(path))
    if TS_SOURCE.search(path):
        return bool(TS_TEST.search(path))
    return False


def unchecked_behavioral_files(base: str, head: str, probed: frozenset[str] = frozenset(),
                               include_tests: bool = True) -> list[tuple[str, str]]:
    """Files a feat/fix/perf commit changed that this gate did not probe, and why.

    Printed with every report, and alone when nothing was probed at all, to turn a
    reassuring silence into a named blind spot. Deliberately reports *reasons* rather than a
    count: "no behavioral source changes" was technically true of the run that
    missed a scheduling rewrite, and a number would have been just as true and
    just as useless. `probed` holds the files a probe undid, companions included.
    """
    out = _git("diff", "--name-status", f"{base}...{head}").stdout
    unchecked: list[tuple[str, str]] = []
    for line in out.splitlines():
        parts = line.split("\t")
        if len(parts) < 2:
            continue
        path = parts[-1]
        if path in probed or not claims_behavior_change(types_touching(base, head, path)):
            continue
        if is_test_file(path):
            if include_tests:
                unchecked.append((path, "classified as test code, so never asked to go red"))
        elif not is_probed_kind(path):
            unchecked.append((path, f"no probe for this kind of file ({Path(path).suffix or Path(path).name})"))
        elif scope_for(path) is None:
            unchecked.append((path, "no test scope covers it (outside PY_ROOTS)"))
    return unchecked


def changed_lines(base: str, head: str, path: str, cwd: str | None = None) -> list[str]:
    """The added and removed content lines for one path, without their +/- marker.

    `-U0` so context lines are not mistaken for changes, and `--ignore-all-space`
    so a reindent is not either.
    """
    out = _git(
        "diff", "-U0", "--ignore-all-space", f"{base}...{head}", "--", path, cwd=cwd
    ).stdout
    lines = []
    for line in out.splitlines():
        if line.startswith(("+++", "---", "@@", "diff ", "index ", "new file", "deleted file")):
            continue
        if line.startswith(("+", "-")):
            lines.append(line[1:])
    return lines


def is_cosmetic(tree: str, base: str, head: str, path: str) -> bool:
    """Whether the change to `path` cannot affect behaviour, so no test could notice.

    A `feat` commit that adjusts a doc comment alongside its real work leaves this
    gate reporting the comment's file as an uncovered behaviour change, because
    reverting a comment of course breaks nothing. That is a false positive, and this
    gate's whole claim on a contributor's attention is that it does not produce them
    — yoloAI's specimen was a two-line doc-comment edit on `applyBrokerOption`,
    reported identically to a real missing test.

    Both tests below are *exact within their guard* and decline to classify whenever
    they cannot be sure. Declining means the file is probed as before, so the worst
    case is the behaviour we already had.

    * **Go**: every changed line is a `//` comment or blank, and neither version of
      the file contains `/*` anywhere. Without that second condition a changed line
      inside a block comment is indistinguishable from code by inspection, and a
      guard that reads `*p = 0` as a comment would suppress a real finding.
    * **Python**: the two versions parse to the same AST. That covers `#` comments
      and formatting together. A docstring edit is *not* covered — docstrings are
      AST nodes — so it still reports as it does today.
    """
    if GO_SOURCE.search(path):
        before = _git("show", f"{base}:{path}", cwd=tree).stdout
        after = _git("show", f"{head}:{path}", cwd=tree).stdout
        if "/*" in before or "/*" in after:
            return False  # cannot tell a block-comment line from code
        changed = changed_lines(base, head, path, cwd=tree)
        return bool(changed) and all(
            not line.strip() or line.strip().startswith("//") for line in changed
        )

    if PY_SOURCE.search(path):
        try:
            before = ast.dump(ast.parse(_git("show", f"{base}:{path}", cwd=tree).stdout))
            after = ast.dump(ast.parse(_git("show", f"{head}:{path}", cwd=tree).stdout))
        except SyntaxError:
            return False  # unparseable on either side; let the probe speak
        return before == after

    return False


def types_touching(base: str, head: str, path: str) -> set[str | None]:
    out = _git("log", "--no-merges", "--format=%s", f"{base}..{head}", "--", path).stdout
    return {commit_type(line) for line in out.splitlines() if line.strip()}


def verified_out_of_suite(base: str, head: str, path: str) -> str:
    """Any `Verified-By:` claim on the commits touching `path`.

    Some behaviour genuinely cannot go red in `go test` — here, typically a symptom
    only a television or a real network shows, which the device lab confirms (D190).

    Without a way to say that, the only route past this gate would be relabelling a
    `fix` as a `chore`, and a gate satisfiable only by lying about the commit type is
    worse than no gate. The trailer turns "I could not unit-test this" from a silent
    bypass into a claim on the record that review can weigh. It deliberately takes
    free text: the point is that a human wrote down what they actually did.
    """
    body = _git("log", "--no-merges", "--format=%B", f"{base}..{head}", "--", path).stdout
    claims = [
        line.split(":", 1)[1].strip()
        for line in body.splitlines()
        if line.lower().startswith("verified-by:")
    ]
    return "; ".join(c for c in claims if c)


def refactor_only(base: str, head: str, path: str) -> str:
    """Why the commits touching `path` say its part of them changed no behaviour: their
    `Refactor-Only: <path> — <why>` trailers naming this path, or '' when none does.

    A refactor inside a `fix` commit owes a red it cannot give, and a made commit is never
    rewritten, so without this the file stayed flagged until the base moved past it (80ee9fe's
    playback.go, audit-2026-09-29 §6). The better practice is a separate refactor commit.
    """
    body = _git("log", "--no-merges", "--format=%B", f"{base}..{head}", "--", path).stdout
    return "; ".join(why.strip() for named, why in REFACTOR_ONLY.findall(body) if named == path)


def verification_queue_problem(claim: str, tree: str) -> str:
    """Why a Verified-By claim does not count, or '' when it names a ticked VQ item (D190)."""
    refs = VQ_REF.findall(claim)
    if not refs:
        return f"names no VQ<n> item in {VQ_FILE}"
    queue = Path(tree) / VQ_FILE
    text = queue.read_text() if queue.exists() else ""
    known = set(VQ_ANY.findall(text))
    ticked = set(VQ_TICKED.findall(text))
    for n in refs:
        if n not in known:
            return f"VQ{n} is not in {VQ_FILE}"
        if n not in ticked:
            return f"VQ{n} is not ticked — the verification has not been done"
    return ""


def scope_for(path: str) -> list[str] | None:
    """The narrowest test command covering `path`, or None when there is no scope.

    Package-scoped on purpose: the happy path is a fast red. Widening to the full
    suite happens only for a file that survives its own package, which is the
    suspicious case and the only one worth paying for.
    """
    if path.startswith(GO_GENERATED_QUERIES) or MIGRATION_STEP.match(path) or path == STORE_SCHEMA:
        return STORE_INTEGRATION
    if CSS_SOURCE.search(path):
        return browser_tier_command()
    if GO_SOURCE.search(path):
        return ["go", "test", "./" + str(Path(path).parent) + "/..."]
    if PY_SOURCE.search(path) and any(path.startswith(r + "/") for r in PY_ROOTS):
        root = next(r for r in PY_ROOTS if path.startswith(r + "/"))
        return ["python3", "-m", "unittest", "discover", "-q", "-s", f"{root}/tests"]
    if TS_SOURCE.search(path):
        return ["npm", "--prefix", "web", "run", "test"]
    return None


def wide_scope(path: str) -> list[str] | None:
    if GO_SOURCE.search(path):
        return ["go", "test", "./..."]
    return None


@dataclass(frozen=True)
class Attempt:
    """One run that may notice a reverted file. `compile_only` runs cannot fail an assertion,
    so any failure of theirs is a build failure."""

    command: list[str]
    label: str
    env: dict[str, str] | None = None
    compile_only: bool = False


def other_builds(path: str) -> list[Attempt]:
    """The runs beyond the default suite that `make check` also makes, for a file only they
    compile: the dev-tagged suite, and a compile for the operating system a file is named for;
    for TypeScript, the type check, since vitest drops types unchecked and a file of types alone
    (the SDK's generated API) can break nothing else."""
    if TS_SOURCE.search(path):
        return [Attempt(["npm", "--prefix", "web", "run", "typecheck"], ", found by the type check", None, True)]
    if not GO_SOURCE.search(path):
        return []
    attempts = [Attempt(["go", "test", "-tags", GO_EXTRA_TAGS, "./..."], f", found by the -tags {GO_EXTRA_TAGS} suite")]
    other_os = GO_OS_SUFFIX.search(path)
    if other_os:
        goos = other_os.group(1)
        attempts.append(Attempt(["go", "vet", "./..."], f", found by compiling for {goos}", {"GOOS": goos}, True))
    other_arch = GO_LINUX_ARCH_SUFFIX.search(path)
    if other_arch:
        goarch = other_arch.group(1)
        attempts.append(Attempt(["go", "vet", "./..."], f", found by compiling for linux/{goarch}",
                                {"GOOS": "linux", "GOARCH": goarch}, True))
    if not other_os and is_not_linux(path):
        # macOS is the other system make check compiles for (lint-speculative-api).
        attempts.append(Attempt(["go", "vet", "./..."], ", found by compiling for darwin", {"GOOS": "darwin"}, True))
    return attempts


def browser_tier(path: str) -> list[Attempt]:
    """For web source, the browser tier, tried last: a change only a browser shows — a stylesheet
    a component imports, a layout — is invisible to vitest, which renders no CSS, and to the type
    check. It runs `make browser-tier`, which is `make test-browser` without its install step: the
    worktree's packages are links to the working tree's, which an install would replace. FFmpeg is
    the working tree's, which the worktree does not have."""
    if not TS_SOURCE.search(path):
        return []
    return [Attempt(browser_tier_command(), ", found by the browser tier")]


def browser_tier_command() -> list[str]:
    return ["make", "--no-print-directory", "browser-tier", f"FFMPEG_DIR={_installed_ffmpeg()}"]


@functools.cache
def _installed_ffmpeg() -> str:
    """The FFmpeg directory the working tree's `make ffmpeg` installed."""
    root = _git("rev-parse", "--show-toplevel").stdout.strip()
    done = subprocess.run([sys.executable, "scripts/fetch_ffmpeg.py"], cwd=root, capture_output=True,
                          text=True, check=False)
    return done.stdout.strip()


def is_not_linux(path: str) -> bool:
    """Whether a Go file's build line leaves Linux out, as the working tree has it: a file only
    another system compiles, whose revert only that system's build can notice."""
    try:
        head = Path(path).read_text(encoding="utf-8", errors="replace")[:2048]
    except OSError:
        return False
    return bool(GO_NOT_LINUX.search(head))


# The shell's exit status for a command it could not find: `npm run test` in a tree with no
# installed packages, which once passed every TypeScript file as red because vitest was absent.
RUNNER_MISSING = 127

def is_build_failure(output: str) -> bool:
    """Whether a failing run failed to compile rather than to assert.

    Keyed on the toolchains' own markers rather than on error text: `go test` prints
    a `[build failed]` suffix on the FAIL line, and pytest reports an import-time
    problem as a collection error. Matching compiler prose ("undefined:", "cannot
    use") would misread a test that legitimately asserts on such a message.
    """
    markers = ("[build failed]", "ERROR collecting", "ImportError", "SyntaxError", "error TS",
               "error during build")
    return any(m in output for m in markers)


def probe(tree: str, base: str, head: str, change: Change, timeout: int) -> Outcome:
    """Revert one file in the worktree, with its unit's companions, run its tests, restore it."""
    refactor = refactor_only(base, head, change.path)
    if refactor:
        return Outcome(change.path, "CLAIMED", f"Refactor-Only: {refactor}")
    claim = verified_out_of_suite(base, head, change.path)
    if claim:
        problem = verification_queue_problem(claim, tree)
        if problem:
            return Outcome(change.path, "GREEN", f"Verified-By: {claim} — {problem}")
        return Outcome(change.path, "CLAIMED", f"Verified-By: {claim}")
    if change.status == "M" and is_cosmetic(tree, base, head, change.path):
        return Outcome(change.path, "COSMETIC", "comment or formatting only; no behaviour to cover")
    scope = scope_for(change.path)
    if scope is None:
        return Outcome(change.path, "UNCHECKED", "no test scope covers this path")
    if change.status == "D":
        return Outcome(change.path, "UNCHECKED", "deleted; reverting would re-add it")

    unit = companions(change.path)
    try:
        if change.status == "A":
            (Path(tree) / change.path).unlink(missing_ok=True)
        else:
            restored = _git("checkout", base, "--", change.path, cwd=tree)
            if restored.returncode != 0:
                return Outcome(change.path, "UNCHECKED", "could not check out the base version")
        for companion in unit:
            revert_companion(tree, base, change.path, companion)

        attempts = [Attempt(scope, "")]
        wide = wide_scope(change.path)
        if wide is not None and wide != scope:
            attempts.append(Attempt(wide, ", found by the whole suite"))
        attempts.extend(other_builds(change.path))
        attempts.extend(browser_tier(change.path))
        for attempt in attempts:
            env = {**os.environ, **attempt.env} if attempt.env else None
            # One browser tier at a time: each starts a server and three browsers.
            with BROWSER_TIER_LOCK if "browser-tier" in attempt.command else contextlib.nullcontext():
                done = subprocess.run(
                    attempt.command, cwd=tree, capture_output=True, text=True, timeout=timeout,
                    check=False, env=env,
                )
            if done.returncode == 0:
                continue
            if done.returncode == RUNNER_MISSING:
                return Outcome(change.path, "NO_RUNNER", f"the tests could not start: {attempt.command[0]} "
                               "found no runner, so nothing was checked")
            combined = done.stdout + done.stderr
            if attempt.compile_only or is_build_failure(combined):
                return Outcome(change.path, "RED_BUILD", f"reverting it breaks the build{attempt.label}")
            return Outcome(change.path, "RED_TEST", f"a test fails when it is reverted{attempt.label}")
        return Outcome(change.path, "GREEN", "every test still passes with this change reverted")
    finally:
        restore(tree, head, change.path)
        for companion in unit:
            restore(tree, head, companion)


def report(outcomes: list[Outcome]) -> str:
    lines = []
    for verdict, heading in (
        ("GREEN", "Changed with nothing to notice — no test goes red when these are reverted"),
        ("NO_RUNNER", "Not checked: the tests could not start — fix the environment, not the code"),
        ("RED_BUILD", "Load-bearing, but only at compile time"),
        ("RED_TEST", "Covered — a test fails when these are reverted"),
        ("CLAIMED", "On the author's word (Verified-By: a ticked VQ item; Refactor-Only: no behaviour "
                    "changed in this file) — review this"),
        ("COSMETIC", "No behaviour changed, so nothing could cover it"),
        ("UNCHECKED", "Not checked by this gate: a feat/fix/perf commit changed these, and no probe "
                      "reverted them"),
    ):
        rows = [o for o in outcomes if o.verdict == verdict]
        if not rows:
            continue
        lines.append(f"\n{heading}:")
        lines.extend(f"  {o.path}  — {o.detail}" for o in rows)
    return "\n".join(lines)


BROWSER_TIER_LOCK = threading.Lock()


@contextlib.contextmanager
def hold_probe_trees():
    """The kept trees are shared by every worktree of the repository, so two runs at once (two
    agents' worktrees, or a hook beside a manual run) would reset each other's trees mid-probe.
    A run holds this lock on the git common directory for as long as it uses them."""
    common = Path(_git("rev-parse", "--path-format=absolute", "--git-common-dir").stdout.strip())
    (common / "revert-red").mkdir(exist_ok=True)
    with open(common / "revert-red" / "lock", "w", encoding="utf-8") as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            print("Another revert-red run is using the probe trees; waiting for it to finish.", flush=True)
            fcntl.flock(lock, fcntl.LOCK_EX)
        yield


def probe_trees(head: str, count: int) -> list[str]:
    """`count` worktrees to probe in, at fixed paths inside the repository's git directory, made
    once and kept: a Go build or test cached in one is found again next run, which a fresh path
    would miss. Each is set back to `head` and cleaned of untracked files, keeping the ignored
    ones (the linked node_modules).

    `head` is resolved to a commit here, in the working tree: a ref such as HEAD names a
    different commit inside each kept tree (the one it was last set to), so passing it through
    would set every tree back to where it already was and probe a stale copy of the code."""
    resolved = _git("rev-parse", "--verify", "--quiet", f"{head}^{{commit}}")
    if resolved.returncode != 0:
        raise RuntimeError(f"{head!r} is not a commit this repository can resolve")
    head = resolved.stdout.strip()
    common = Path(_git("rev-parse", "--path-format=absolute", "--git-common-dir").stdout.strip())
    source = Path(_git("rev-parse", "--show-toplevel").stdout.strip())
    trees = []
    for index in range(count):
        tree = common / "revert-red" / f"wt-{index}"
        if (tree / ".git").exists():
            reset = _git("checkout", "--detach", "--force", head, cwd=str(tree))
            cleaned = _git("clean", "-fdq", cwd=str(tree))
            if reset.returncode != 0 or cleaned.returncode != 0:
                raise RuntimeError(f"could not reset {tree}: {reset.stderr}{cleaned.stderr}")
        else:
            _git("worktree", "prune")
            added = _git("worktree", "add", "--detach", str(tree), head)
            if added.returncode != 0:
                raise RuntimeError(f"could not make {tree}: {added.stderr}")
            link_installed_packages(source, tree)
        trees.append(str(tree))
    return trees


def why_not_probed(base: str, head: str, path: str, changes: list[Change]) -> str:
    """Why a path a targeted run named is not probed: '' when it is owed a probe."""
    if any(c.path == path for c in changes):
        types = types_touching(base, head, path)
        if claims_behavior_change(types):
            return ""
        return f"exempt: only {', '.join(sorted(t or '?' for t in types))} commits touched it"
    if is_test_file(path):
        return "test code, never asked to go red"
    if not is_probed_kind(path):
        return f"no probe for this kind of file ({Path(path).suffix or Path(path).name})"
    return f"not changed in {base[:12]}..{head[:12]}"


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--base", required=True, help="ref the branch forked from")
    ap.add_argument("--head", default="HEAD")
    ap.add_argument("--timeout", type=int, default=900, help="seconds per test run")
    ap.add_argument("--jobs", type=int, default=0, help="files probed at once (default: half the CPUs)")
    only = ap.add_mutually_exclusive_group()
    only.add_argument("--only", nargs="+", metavar="PATH", help="probe only these paths, still judged "
                      "against --base..--head; a full run is still required before the branch is done")
    only.add_argument("--rerun-failed", action="store_true", help="probe only the paths the last run "
                      "against the same --base failed on (the head may have moved)")
    args = ap.parse_args()

    for name, ref in (("--base", args.base), ("--head", args.head)):
        if not rev_exists(ref):
            print(f"{name} {ref!r} is not a commit this repository can resolve.")
            return 1
    base = _git("rev-parse", f"{args.base}^{{commit}}").stdout.strip()
    head = _git("rev-parse", f"{args.head}^{{commit}}").stdout.strip()

    wanted: list[str] | None = None
    if args.rerun_failed:
        wanted, ran_on, problem = reruns.recorded_failures(base)
        if problem:
            print(problem)
            return 1
        if not wanted:
            print(f"The last revert-red run against {base[:12]} failed on nothing; nothing to re-run.")
            return 0
        print(f"Re-running the {len(wanted)} file(s) the last run (at {ran_on[:12]}) failed on.")
    elif args.only:
        wanted = [p.removeprefix("./") for p in args.only]

    changes = changed_sources(base, head)
    owed = [c for c in changes if claims_behavior_change(types_touching(base, head, c.path))]
    exempt = len(changes) - len(owed)
    unknown = False
    if wanted is not None:
        owed = [c for c in owed if c.path in wanted]
        for path in wanted:
            why = why_not_probed(base, head, path, changes)
            if why:
                print(f"Not probed: {path}  — {why}")
                unknown = unknown or (args.only is not None and why.startswith("not changed"))
        if not owed:
            reruns.finish(base, head, wanted, [], targeted=True)
            return 1 if unknown else 0
    elif not owed:
        reruns.finish(base, head, [], [], targeted=False)
        # A behavioral commit whose every file fell out of scope is not a pass; it
        # is the gate reporting that it checked nothing, and it must say which
        # files and why. Printing "no behavioral source changes" over a `fix` that
        # rewrote scheduling logic is exactly how yoloAI's smoke harness went unchecked
        # for as long as it did — the message was true of what the gate could see
        # and false about the commit.
        unchecked = unchecked_behavioral_files(base, head)
        if unchecked:
            print(
                f"Nothing was checked in {args.base}..{args.head}, but a feat/fix/perf commit "
                f"touched {len(unchecked)} file(s) this gate cannot scope:"
            )
            for path, why in unchecked:
                print(f"  {path}  — {why}")
            print(
                "\nThat is a blind spot, not a pass. Either the commit type is wrong, or the\n"
                "behaviour needs a test somewhere this gate can run, or it is genuinely\n"
                "unreachable here and belongs behind a Verified-By: trailer naming what you ran."
            )
            return 0
        print(f"No behavioral source changes in {args.base}..{args.head} ({exempt} exempt by type).")
        return 0

    jobs = args.jobs or max(1, (os.cpu_count() or 2) // 2)
    with hold_probe_trees():
        reruns.begin(base)
        status = probe_all(base, head, owed, exempt, min(jobs, len(owed)), args.timeout, wanted)
    if wanted is not None:
        print("\nA targeted run: the full `make check-revert-red` (or `make check-pr`) is still "
              "required before the branch is done.")
    return 1 if unknown else status


def unprobed(base: str, head: str, owed: list[Change]) -> list[Outcome]:
    """The UNCHECKED rows for every file a behaviour commit changed that no probe undid."""
    probed = frozenset(p for c in owed for p in (c.path, *companions(c.path)))
    return [Outcome(path, "UNCHECKED", why)
            for path, why in unchecked_behavioral_files(base, head, probed, include_tests=False)]


def probe_all(base: str, head: str, owed: list[Change], exempt: int, jobs: int, timeout: int,
              wanted: list[str] | None) -> int:
    try:
        trees = probe_trees(head, jobs)
    except RuntimeError as problem:
        print(f"Could not prepare the worktrees to test in:\n{problem}")
        return 1
    free: queue.Queue[str] = queue.Queue()
    for tree in trees:
        free.put(tree)

    def probe_in_a_free_tree(change: Change) -> Outcome:
        tree = free.get()
        try:
            return probe(tree, base, head, change, timeout)
        finally:
            free.put(tree)

    with ThreadPoolExecutor(max_workers=len(trees)) as pool:
        outcomes = list(pool.map(probe_in_a_free_tree, owed))

    # A targeted run reports on the paths it was given; their skips were printed already.
    if wanted is None:
        outcomes += unprobed(base, head, owed)
    reruns.finish(base, head, wanted or [c.path for c in owed], [o.path for o in outcomes if o.is_failure],
                  targeted=wanted is not None)
    if wanted is None:
        print(f"Reverted {len(owed)} behavioral source change(s); {exempt} exempt by commit type.")
    else:
        print(f"Reverted {len(owed)} of the {len(wanted)} file(s) named.")
    print(report(outcomes))

    failures = [o for o in outcomes if o.is_failure]
    if not failures:
        return 0
    if any(o.verdict == "NO_RUNNER" for o in failures):
        print(
            "\nThe tests could not start in the worktree, so nothing was checked. That is the\n"
            "environment (are the web packages installed? run `npm ci` in web/), not the code."
        )
    if all(o.verdict == "NO_RUNNER" for o in failures):
        return 1
    print(
        "\nEach file above changed, and reverting it broke nothing. One of four things\n"
        "is true, and only you can say which:\n\n"
        "  - it needs a test that fails when the change is undone (D190);\n"
        "  - the commit that touched it is mislabelled, and is not `feat`/`fix`/`perf`;\n"
        "  - its part of a behaviour commit was only a refactor. Say so with a\n"
        "    `Refactor-Only: <path> — <why>` trailer on a commit that touches it, and\n"
        "    keep the next refactor in a commit of its own;\n"
        "  - it genuinely cannot go red in this suite — a lab-only symptom, a\n"
        "    device-only symptom. Then add a VQ<n> item to\n"
        "    docs/contributors/verification-queue.md, do it, tick it, and name it in a\n"
        "    `Verified-By: VQ<n>` trailer. That is a claim on the record, not a bypass.\n\n"
        "Red-on-revert is necessary and not sufficient (D190): it proves a\n"
        "test is wired to the code, never that the code is wired to the program."
    )
    return 1


if __name__ == "__main__":
    sys.exit(main())
