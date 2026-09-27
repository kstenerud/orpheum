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
* Only languages whose tests this can actually scope and run. Anything else is
  reported as unchecked rather than passed — silence is what makes a green mean two
  things at once.

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
import ast
import os
import re
import shutil
import subprocess
import sys
import tempfile
from dataclasses import dataclass
from pathlib import Path

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
PY_SOURCE = re.compile(r"\.py$")

# A Python file is a test because of WHERE it lives, not what it is called. Every
# one of our Python tests sits in `scripts/tests/`. yoloAI identified them by
# filename once and got both directions wrong: its `scripts/smoke_test.py` was the only
# `*_test.py` path in the tree and is a 4000-line harness rather than a test, so
# every `fix` to it was silently exempt; while `tests/conftest.py` matches neither
# name pattern and was therefore treated as behavioral source that must go red.
# internal/testutil is test code by construction: only tests may import it
# (TestArch_TestutilIsImportedOnlyByTests), so no change to it can alter what the binary
# does, and a helper only an integration test uses has no test in this suite to turn red
# (audit-2026-09-25 section 5.3).
GO_TEST_SUPPORT = "internal/testutil/"
# Generated query code has no tests of its own, and a query's body can change without its
# signature: a fix that makes a slow query fast is noticed only by the reference-scale test in
# internal/store's integration tier, which is where such a file is reverted against
# (audit-2026-09-25 section 5.3).
GO_GENERATED_QUERIES = ("internal/store/sqlcgen/", "internal/store/cachegen/")
# `*_test.go` is safe to match by name because in Go the compiler agrees; the same
# suffix in Python is a borrowed idiom that only ever matched the wrong file.
PY_TEST = re.compile(r"(^|/)tests/")

TS_SOURCE = re.compile(r"^web/(packages|apps)/[^/]+/src/.*\.tsx?$")
TS_TEST = re.compile(r"\.test\.tsx?$")

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
        if GO_SOURCE.search(path) or PY_SOURCE.search(path) or TS_SOURCE.search(path):
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


def unchecked_behavioral_files(base: str, head: str) -> list[tuple[str, str]]:
    """Files a feat/fix/perf commit changed that this gate could not check, and why.

    Only ever consulted when nothing was checked at all, to turn a reassuring
    silence into a named blind spot. Deliberately reports *reasons* rather than a
    count: "no behavioral source changes" was technically true of the run that
    missed a scheduling rewrite, and a number would have been just as true and
    just as useless.
    """
    out = _git("diff", "--name-status", f"{base}...{head}").stdout
    unchecked: list[tuple[str, str]] = []
    for line in out.splitlines():
        parts = line.split("\t")
        if len(parts) < 2:
            continue
        path = parts[-1]
        if not claims_behavior_change(types_touching(base, head, path)):
            continue
        if is_test_file(path):
            unchecked.append((path, "classified as test code, so never asked to go red"))
        elif not (GO_SOURCE.search(path) or PY_SOURCE.search(path) or TS_SOURCE.search(path)):
            unchecked.append((path, "not a language this gate can scope or run"))
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
    if path.startswith(GO_GENERATED_QUERIES):
        return ["go", "test", "-tags", "integration", "./internal/store/..."]
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
    return attempts


# The shell's exit status for a command it could not find: `npm run test` in a tree with no
# installed packages, which once passed every TypeScript file as red because vitest was absent.
RUNNER_MISSING = 127

# Package caches a run writes, which the worktree keeps its own of rather than share.
PACKAGE_CACHES = (".vite", ".vite-temp", ".vitest", ".cache")


def link_installed_packages(source: Path, tree: Path) -> None:
    """Give the worktree the web packages the working tree has installed, without installing.

    git worktree copies only tracked files, so node_modules is absent, and the web suite cannot
    run. Each node_modules under web/ is made in the worktree as a directory of links: an
    installed package links to the working tree's copy, and a workspace link (@orpheum/sdk ->
    ../../packages/sdk) is copied as the relative link it is, so it resolves to the worktree's
    own package, the one a revert changes.
    """
    web = source / "web"
    if not web.is_dir():
        return
    for installed in sorted(web.rglob("node_modules")):
        relative = installed.relative_to(source)
        if "node_modules" in relative.parts[:-1] or not installed.is_dir():
            continue
        _mirror_packages(installed, tree / relative, scoped=True)


def _mirror_packages(installed: Path, into: Path, scoped: bool) -> None:
    into.mkdir(parents=True, exist_ok=True)
    for entry in installed.iterdir():
        if entry.name in PACKAGE_CACHES:
            continue
        target = into / entry.name
        if entry.is_symlink():
            target.symlink_to(os.readlink(entry))
        elif scoped and entry.name.startswith("@") and entry.is_dir():
            _mirror_packages(entry, target, scoped=False)
        else:
            target.symlink_to(entry.resolve())


def is_build_failure(output: str) -> bool:
    """Whether a failing run failed to compile rather than to assert.

    Keyed on the toolchains' own markers rather than on error text: `go test` prints
    a `[build failed]` suffix on the FAIL line, and pytest reports an import-time
    problem as a collection error. Matching compiler prose ("undefined:", "cannot
    use") would misread a test that legitimately asserts on such a message.
    """
    markers = ("[build failed]", "ERROR collecting", "ImportError", "SyntaxError", "error TS")
    return any(m in output for m in markers)


def probe(tree: str, base: str, head: str, change: Change, timeout: int) -> Outcome:
    """Revert one file in the worktree, run its tests, restore it."""
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

    try:
        if change.status == "A":
            (Path(tree) / change.path).unlink(missing_ok=True)
        else:
            restored = _git("checkout", base, "--", change.path, cwd=tree)
            if restored.returncode != 0:
                return Outcome(change.path, "UNCHECKED", "could not check out the base version")

        attempts = [Attempt(scope, "")]
        wide = wide_scope(change.path)
        if wide is not None and wide != scope:
            attempts.append(Attempt(wide, ", found by the whole suite"))
        attempts.extend(other_builds(change.path))
        for attempt in attempts:
            env = {**os.environ, **attempt.env} if attempt.env else None
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
        _git("checkout", head, "--", change.path, cwd=tree)


def report(outcomes: list[Outcome]) -> str:
    lines = []
    for verdict, heading in (
        ("GREEN", "Changed with nothing to notice — no test goes red when these are reverted"),
        ("NO_RUNNER", "Not checked: the tests could not start — fix the environment, not the code"),
        ("RED_BUILD", "Load-bearing, but only at compile time"),
        ("RED_TEST", "Covered — a test fails when these are reverted"),
        ("CLAIMED", "Verified outside the suite, on the author's word — review this"),
        ("COSMETIC", "No behaviour changed, so nothing could cover it"),
        ("UNCHECKED", "Not checked by this gate"),
    ):
        rows = [o for o in outcomes if o.verdict == verdict]
        if not rows:
            continue
        lines.append(f"\n{heading}:")
        lines.extend(f"  {o.path}  — {o.detail}" for o in rows)
    return "\n".join(lines)


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--base", required=True, help="ref the branch forked from")
    ap.add_argument("--head", default="HEAD")
    ap.add_argument("--timeout", type=int, default=900, help="seconds per test run")
    args = ap.parse_args()

    for name, ref in (("--base", args.base), ("--head", args.head)):
        if not rev_exists(ref):
            print(f"{name} {ref!r} is not a commit this repository can resolve.")
            return 1

    changes = changed_sources(args.base, args.head)
    owed = [c for c in changes if claims_behavior_change(types_touching(args.base, args.head, c.path))]
    exempt = len(changes) - len(owed)
    if not owed:
        # A behavioral commit whose every file fell out of scope is not a pass; it
        # is the gate reporting that it checked nothing, and it must say which
        # files and why. Printing "no behavioral source changes" over a `fix` that
        # rewrote scheduling logic is exactly how yoloAI's smoke harness went unchecked
        # for as long as it did — the message was true of what the gate could see
        # and false about the commit.
        unchecked = unchecked_behavioral_files(args.base, args.head)
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

    tmp = tempfile.mkdtemp(prefix="revert-red-")
    tree = str(Path(tmp) / "wt")
    added = _git("worktree", "add", "--detach", tree, args.head)
    if added.returncode != 0:
        print(f"Could not create a worktree to test in:\n{added.stderr}")
        shutil.rmtree(tmp, ignore_errors=True)
        return 1
    try:
        link_installed_packages(Path(_git("rev-parse", "--show-toplevel").stdout.strip()), Path(tree))
        outcomes = [probe(tree, args.base, args.head, c, args.timeout) for c in owed]
    finally:
        _git("worktree", "remove", "--force", tree)
        shutil.rmtree(tmp, ignore_errors=True)

    print(f"Reverted {len(owed)} behavioral source change(s); {exempt} exempt by commit type.")
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
        "\nEach file above changed, and reverting it broke nothing. One of three things\n"
        "is true, and only you can say which:\n\n"
        "  - it needs a test that fails when the change is undone (D190);\n"
        "  - the commit that touched it is mislabelled, and is not `feat`/`fix`/`perf`;\n"
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
