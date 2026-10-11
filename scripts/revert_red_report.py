# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: How revert-red (check_revert_red.py) tells a run's outcomes: each file under the
# ABOUTME: heading of its verdict, and what to do about the files that failed.
"""What revert-red prints: the outcomes by verdict, and the advice for those that failed."""

from __future__ import annotations

from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from check_revert_red import Outcome


def report(outcomes: list[Outcome]) -> str:
    """Every outcome, under the heading of its verdict, in the order a reader acts on them."""
    lines = []
    for verdict, heading in (
        ("GREEN", "Changed with nothing to notice — no test goes red when these are reverted"),
        ("NO_RUNNER", "Not checked: the tests could not start — fix the environment, not the code"),
        ("RED_BUILD", "Load-bearing, but only at compile time"),
        ("RED_TEST", "Covered — a test fails when these are reverted"),
        ("CLAIMED", "On the author's word (Verified-By: a ticked VQ item; Refactor-Only: no behaviour "
                    "changed in this file) — review this"),
        ("COSMETIC", "No behaviour changed, so nothing could cover it"),
        ("UNREACHABLE", "Only removed code nothing reached, so reverting restores dead code"),
        ("UNCHECKED", "Not checked by this gate: a feat/fix/perf commit changed these, and no probe "
                      "reverted them"),
    ):
        rows = [o for o in outcomes if o.verdict == verdict]
        if not rows:
            continue
        lines.append(f"\n{heading}:")
        lines.extend(f"  {o.path}  — {o.detail}" for o in rows)
    return "\n".join(lines)


def failure_advice(failures: list[Outcome]) -> str:
    """What to do about the files that failed: the environment, when the tests could not start;
    else the four things one of which is true of a file whose revert broke nothing."""
    lines = []
    if any(o.verdict == "NO_RUNNER" for o in failures):
        lines.append(
            "\nThe tests could not start in the worktree, so nothing was checked. That is the\n"
            "environment (are the web packages installed? run `npm ci` in web/), not the code."
        )
    if all(o.verdict == "NO_RUNNER" for o in failures):
        return "\n".join(lines)
    lines.append(
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
    return "\n".join(lines)
