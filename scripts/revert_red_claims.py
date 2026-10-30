# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: What a branch's trailers claim of a file whose revert turned nothing red: a Verified-By
# ABOUTME: naming a verification-queue item, ticked or owed (D378), or a Refactor-Only naming the file (D190).
"""The claims revert-red consults after a probe turned nothing red (check_revert_red.claimed)."""

from __future__ import annotations

import re
import subprocess
from pathlib import Path

# `Refactor-Only: <path> — <why>`: this file's part of a behaviour commit changed no behaviour.
REFACTOR_ONLY = re.compile(r"^Refactor-Only:\s*(\S+)\s+(?:—|--|-)\s*(\S.*)$", re.M | re.I)

VQ_FILE = "docs/contributors/verification-queue.md"
VQ_REF = re.compile(r"\bVQ(\d+)\b")
VQ_TICKED = re.compile(r"^- \[[xX]\] \*\*VQ(\d+)\b", re.M)
VQ_ANY = re.compile(r"^- \[[ xX]\] \*\*VQ(\d+)\b", re.M)


def _git(*args: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(["git", *args], capture_output=True, text=True, check=False)


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
    """Why the branch's commits say `path`'s part of them changed no behaviour: their
    `Refactor-Only: <path> — <why>` trailers naming this path, or '' when none does.

    A refactor inside a `fix` commit owes a red it cannot give, and a made commit is never
    rewritten, so without this the file stayed flagged until the base moved past it (80ee9fe's
    playback.go, audit-2026-09-29 §6). The better practice is a separate refactor commit.
    The trailer names its path, so any commit on the branch may carry it: requiring one that
    touched the file left a claim about a file nobody had reason to touch again only a pointless
    edit to ride on (2026-09-30).
    """
    body = _git("log", "--no-merges", "--format=%B", f"{base}..{head}").stdout
    return "; ".join(why.strip() for named, why in REFACTOR_ONLY.findall(body) if named == path)


def verification_queue_problem(claim: str, tree: str) -> str:
    """Why a Verified-By claim does not count, or '' when it names VQ items in the queue (D190, D378).
    An item still unticked counts: the queue is the record of what is owed, and the owner alone
    decides when a batch goes out with it owed (unticked_items says which)."""
    refs = VQ_REF.findall(claim)
    if not refs:
        return f"names no VQ<n> item in {VQ_FILE}"
    known = set(VQ_ANY.findall(_queue(tree)))
    for n in refs:
        if n not in known:
            return f"VQ{n} is not in {VQ_FILE}"
    return ""


def unticked_items(claim: str, tree: str) -> list[str]:
    """The VQ items a Verified-By claim names that are not ticked yet: VQ<n>, in the claim's order."""
    ticked = set(VQ_TICKED.findall(_queue(tree)))
    return [f"VQ{n}" for n in dict.fromkeys(VQ_REF.findall(claim)) if n not in ticked]


def _queue(tree: str) -> str:
    queue = Path(tree) / VQ_FILE
    return queue.read_text() if queue.exists() else ""
