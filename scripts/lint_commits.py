#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# Adapted from yoloAI's scripts/lint_commits.py (MIT, same author).
# ABOUTME: Commit-message gate for a PR's own commits: subject shape, prose body
# ABOUTME: on code commits, and AI attribution trailers. Runs in CI, not in
# ABOUTME: `make check` — it needs a base ref, and fork PRs never run our hooks.
"""Lint the commit messages a pull request adds.

The rules are stated in AGENTS.md; this is the machine that enforces them. It
checks only `<base>..<head>`, never history: the design-only commits before code
began used a `docs: D<n> — …` style that is none of a PR's business.

Why this is CI-only rather than a `make check` prerequisite: it needs a base ref
to diff against, which `make check` has no notion of (on `main` the range is
empty and the check is vacuous). `make lint-commits` runs it locally against
origin/main for anyone who wants it before pushing.
"""

from __future__ import annotations

import argparse
import re
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path

from check_media_names import REGISTRY, find, read_registry
from commit_hashes import cited_commits, subject_of

# The type set, from yoloAI's settled convention, with Conventional Commits' `style` for a change
# of formatting alone, which `check_revert_red.py` already exempted (the owner, 2026-10-05).
# `check_revert_red.py` reads the same types: feat/fix/perf owe a red-on-revert, the rest do not.
TYPES = ("feat", "fix", "docs", "test", "refactor", "build", "ci", "chore", "perf", "style")

# Types whose commits must explain themselves in a body; a `docs:` typo fix need not.
CODE_TYPES = ("feat", "fix", "refactor", "perf")

# The scope charclass allows commas so a change that genuinely spans two packages can
# say so (yoloAI rejected `feat(tart,seatbelt)!:` once and left a release branch
# permanently red — a gate that cannot go green is one people learn to skip).
SUBJECT_RE = re.compile(rf"^({'|'.join(TYPES)})(\([a-z0-9.\-/,]+\))?(!)?: (.+)$")

# git's own default subject for `git revert`; rejecting it would mean hand-editing a
# message git wrote correctly.
REVERT_RE = re.compile(r'^Revert "')

TRAILER_RE = re.compile(r"^Co-Authored-By:\s*(.+?)\s*<", re.IGNORECASE | re.MULTILINE)

# The line `git revert` writes, which names the reverted commit by hash; git wrote it, not us.
REVERTS_LINE_RE = re.compile(r"^This reverts commit [0-9a-f]+\.?$", re.MULTILINE)

# Trailers and tool noise are not a body.
NOISE_RE = re.compile(
    r"^(Co-Authored-By:|Co-authored-by:|Signed-off-by:|🤖 Generated with)", re.IGNORECASE
)


@dataclass(frozen=True)
class Commit:
    sha: str
    subject: str
    body: str


@dataclass(frozen=True)
class Problem:
    sha: str
    subject: str
    rule: str
    detail: str


def check_subject(c: Commit) -> list[Problem]:
    """Subject must be `type(scope): summary` — imperative, no trailing period."""
    out: list[Problem] = []
    if REVERT_RE.match(c.subject):
        return out
    m = SUBJECT_RE.match(c.subject)
    if not m:
        out.append(
            Problem(
                c.sha,
                c.subject,
                "subject-shape",
                f"expected `type(scope): summary` with type in {', '.join(TYPES)}; "
                "breaking changes mark the type with `!` (e.g. `fix(config)!: ...`)",
            )
        )
        return out
    if m.group(4).endswith("."):
        out.append(
            Problem(
                c.sha, c.subject, "trailing-period",
                "drop the trailing period",
            )
        )
    return out


def check_body(c: Commit) -> list[Problem]:
    """Code commits carry prose saying what was wrong, not a restatement of the diff."""
    m = SUBJECT_RE.match(c.subject)
    if not m or m.group(1) not in CODE_TYPES:
        return []
    real = [
        line for line in c.body.splitlines()
        if line.strip() and not NOISE_RE.match(line.strip())
    ]
    if real:
        return []
    return [
        Problem(
            c.sha, c.subject, "missing-body",
            f"a `{m.group(1)}` commit needs a body saying what was wrong or missing "
            "and why this is the fix (trailers alone don't count)",
        )
    ]


def check_trailers(c: Commit) -> list[Problem]:
    """AI attribution names the model; the tool's own footer is stripped."""
    out: list[Problem] = []
    full = f"{c.subject}\n{c.body}"

    if "Generated with [Claude Code]" in full or "🤖" in full:
        out.append(
            Problem(
                c.sha, c.subject, "robot-footer",
                "drop the `🤖 Generated with [Claude Code]` footer; attribution goes "
                "in the Co-Authored-By trailer",
            )
        )

    # Presence is NOT required: human commits legitimately have no trailer. Only its
    # shape is checked. No model allowlist either — the set is open and grows.
    for who in TRAILER_RE.findall(c.body):
        if who.strip().lower() == "claude":
            out.append(
                Problem(
                    c.sha, c.subject, "bare-claude-trailer",
                    "name the exact model, e.g. `Co-Authored-By: Claude Opus 5.5 "
                    "<noreply@anthropic.com>` — a bare `Claude` says nothing about "
                    "what wrote it",
                )
            )
    return out


def check_hashes(c: Commit) -> list[Problem]:
    """A commit is cited by its subject: its hash changes when it leaves the sandbox (A64)."""
    text = REVERTS_LINE_RE.sub("", f"{c.subject}\n{c.body}")
    return [
        Problem(
            c.sha, c.subject, "commit-hash",
            f"cites commit {token} by its hash, which `yoloai apply` and any rewrite change; "
            f'cite its subject instead: "{subject_of(token)}"',
        )
        for token in cited_commits(text)
    ]


SESSION_LINE_RE = re.compile(r"^Claude-Session:", re.MULTILINE)


def check_session_line(c: Commit) -> list[Problem]:
    """No commit names the Claude Code session that wrote it: the link is private and the
    history is public (the owner, 2026-11-08)."""
    if not SESSION_LINE_RE.search(c.body):
        return []
    return [Problem(c.sha, c.subject, "session-line",
                    "carries a Claude-Session: line; the history is public and the session is not. "
                    "Drop the line (Co-Authored-By stays, rule 13)")]


def check_media_names(c: Commit) -> list[Problem]:
    """No message names a film, series or episode the registry does not list: the history is public,
    and the owner's library is never named (AGENTS.md rule 12)."""
    # Beside this script, not the working directory: a test runs the lint in a repository of its own.
    registry = read_registry((Path(__file__).resolve().parent / REGISTRY.name).read_text(encoding="utf-8"))
    return [Problem(c.sha, c.subject, "media-name",
                    f"names {f.text!r} ({f.rule}), which {REGISTRY} does not list. Name a film by its id "
                    "or as \"the owner's film\"")
            for f in find(c.subject + "\n" + c.body, registry, None)]


def read_commits(base: str, head: str) -> list[Commit]:
    sep, rec = "\x1f", "\x1e"
    out = subprocess.run(
        ["git", "log", "--no-merges", f"--format=%H{sep}%s{sep}%b{rec}", f"{base}..{head}"],
        capture_output=True, text=True, check=True,
    ).stdout
    commits: list[Commit] = []
    for chunk in out.split(rec):
        if not chunk.strip():
            continue
        parts = chunk.lstrip("\n").split(sep)
        if len(parts) < 3:
            continue
        commits.append(Commit(parts[0], parts[1], parts[2]))
    return commits


def commit_from_message(text: str) -> Commit:
    """A message as git hands it to commit-msg: comment lines dropped, subject first."""
    lines = [line for line in text.splitlines() if not line.startswith("#")]
    while lines and not lines[0].strip():
        lines.pop(0)
    subject = lines[0].strip() if lines else ""
    return Commit("(new commit)", subject, "\n".join(lines[1:]))


def lint(commits: list[Commit]) -> list[Problem]:
    problems: list[Problem] = []
    for c in commits:
        problems.extend(check_subject(c))
        problems.extend(check_body(c))
        problems.extend(check_trailers(c))
        problems.extend(check_hashes(c))
        problems.extend(check_session_line(c))
        problems.extend(check_media_names(c))
    return problems


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--base", default="origin/main", help="base ref (default: origin/main)")
    ap.add_argument("--head", default="HEAD", help="head ref (default: HEAD)")
    ap.add_argument("--message-file", help="lint one message being written (the commit-msg hook)")
    args = ap.parse_args()

    if args.message_file:
        commit = commit_from_message(open(args.message_file, encoding="utf-8").read())
        problems = lint([commit])
        for p in problems:
            print(f"commit message: [{p.rule}] {p.detail}", file=sys.stderr)
        return 1 if problems else 0

    commits = read_commits(args.base, args.head)
    if not commits:
        print(f"lint-commits: no commits in {args.base}..{args.head}")
        return 0

    problems = lint(commits)
    if not problems:
        print(f"lint-commits: {len(commits)} commit(s) OK")
        return 0

    print(f"lint-commits: {len(problems)} problem(s) in {len(commits)} commit(s)\n", file=sys.stderr)
    for p in problems:
        print(f"  {p.sha[:8]} {p.subject}", file=sys.stderr)
        print(f"      [{p.rule}] {p.detail}\n", file=sys.stderr)
    print("The rules are in AGENTS.md ('Preparing a PR').", file=sys.stderr)
    return 1


if __name__ == "__main__":
    sys.exit(main())
