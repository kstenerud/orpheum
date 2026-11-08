#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Finds the commit hashes of this repository cited in a text. A commit message or a
# ABOUTME: document cites a commit by its subject instead, since no hash outlives the sandbox.
"""The commit hashes a text cites that name commits of this repository.

A commit made in a yoloAI sandbox reaches the owner's repository through `git am`, which gives
it a new committer date and so a new hash, and a public copy rewritten by git-filter-repo
changes every hash again (agent-failures A64). A hash cited in a commit message or a document
is therefore wrong everywhere but where it was written. Cite the commit's subject, or the
decision it carries, instead.

Only a token that resolves to a commit here counts: an upstream project's commit (a Jellyfin or
Debian hash in a research note) is a reference to something that lasts, and a hex colour, a
digest or a UUID is no commit at all.
"""

from __future__ import annotations

import re
import subprocess
from collections.abc import Callable, Iterable

# 7 to 40 hex digits, standing alone: not part of a word, a path, a URL, a colour (`#…`), an
# image tag (`name-1a2b3c4d`), a digest (`sha256:…`) or a larger hex run.
HASH_RE = re.compile(r"(?<![\w#/.:@-])([0-9a-f]{7,40})(?![\w-])")


def candidates(text: str) -> list[str]:
    """The tokens in text shaped like an abbreviated or full commit hash, in order, once each."""
    seen: dict[str, None] = {}
    for m in HASH_RE.finditer(text):
        token = m.group(1)
        # A run of digits alone is a number, and of letters alone a word.
        if re.search(r"\d", token) and re.search(r"[a-f]", token):
            seen.setdefault(token, None)
    return list(seen)


def commits_here(tokens: Iterable[str]) -> set[str]:
    """Those of tokens that name exactly one commit in the current repository."""
    tokens = list(tokens)
    if not tokens:
        return set()
    out = subprocess.run(
        ["git", "cat-file", "--batch-check"],
        input="".join(f"{t}^{{commit}}\n" for t in tokens),
        capture_output=True, text=True, check=True,
    ).stdout.splitlines()
    return {t for t, line in zip(tokens, out) if " commit " in line}


def cited_commits(text: str, resolve: Callable[[Iterable[str]], set[str]] = commits_here) -> list[str]:
    """The hashes text cites that name commits of this repository, in order."""
    tokens = candidates(text)
    here = resolve(tokens)
    return [t for t in tokens if t in here]


def subject_of(token: str) -> str:
    """The subject of the commit token names, to cite in its place."""
    return subprocess.run(
        ["git", "log", "-1", "--format=%s", f"{token}^{{commit}}", "--"],
        capture_output=True, text=True, check=True,
    ).stdout.strip()
