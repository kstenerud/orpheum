#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: UserPromptSubmit hook: puts the owner's standing orders in front of the agent when they changed or
# ABOUTME: a compaction dropped them (process review F13), says when the context is large, and sweeps away what
# ABOUTME: stopped subagents left running.
from __future__ import annotations

import hashlib
import json
import os
import subprocess
import sys
from pathlib import Path

import agents
import hooklib

STANDING_ORDERS = hooklib.ROOT / ".claude" / "standing-orders.md"
# A page at most: it rides on every prompt that shows it.
MOST_CHARACTERS = 5000
# What each session was last shown: the orders' hash and the context it had then.
SEEN = hooklib.ROOT / ".claude" / ".cache" / "prompt-seen"
# Above this many tokens of context the session is told to hand off (the owner, 2026-10-31: the
# context, re-read on every model call, was 85% of the bill). ORPHEUM_CONTEXT_WARN_TOKENS overrides it.
CONTEXT_WARN_TOKENS = 120_000
# A context smaller than this share of the last one seen was compacted, and the orders went with it.
COMPACTED_SHARE = 0.6


def standing_orders() -> str:
    """The standing orders, whole or cut at MOST_CHARACTERS with a line saying so: cut silently at
    3000, their last items were dropped from every prompt unseen (2026-10-27)."""
    if not STANDING_ORDERS.exists():
        return ""
    orders = STANDING_ORDERS.read_text(encoding="utf-8").strip()
    if not orders:
        return ""
    if len(orders) > MOST_CHARACTERS:
        orders = (orders[:MOST_CHARACTERS] + f"\n[cut: the standing orders are {len(orders)} characters, over "
                  f"{MOST_CHARACTERS}; shorten them, moving what is done out]")
    return "The owner's standing orders (.claude/standing-orders.md):\n" + orders


def context_tokens(transcript: str) -> int | None:
    """The context of the session's last model call, from its transcript's usage; 0 when unknown, None
    when the session was compacted since that call. Right after `/compact` the last usage is the old
    context's, and the hook told the session it was at 148k with 15k left (the owner's first trial of
    the meter, 2026-10-31). Reads only the file's tail: a day's transcript runs to tens of megabytes."""
    path = Path(transcript)
    if not transcript or not path.is_file():
        return 0
    with path.open("rb") as file:
        file.seek(max(0, path.stat().st_size - 512_000))
        lines = file.read().decode("utf-8", errors="replace").splitlines()
    for line in reversed(lines):
        try:
            entry = json.loads(line)
        except ValueError:
            continue
        if isinstance(entry, dict) and entry.get("subtype") == "compact_boundary":
            return None
        message = entry.get("message") if isinstance(entry, dict) else None
        usage = message.get("usage") if isinstance(message, dict) else None
        if isinstance(usage, dict):
            total = sum(int(usage.get(key) or 0) for key in
                        ("input_tokens", "cache_creation_input_tokens", "cache_read_input_tokens"))
            if total:
                return total
    return 0


def remembered(session: str) -> dict[str, object]:
    try:
        seen = json.loads((SEEN / f"{session}.json").read_text(encoding="utf-8"))
    except (OSError, ValueError):
        return {}
    return seen if isinstance(seen, dict) else {}


def remember(session: str, seen: dict[str, object]) -> None:
    SEEN.mkdir(parents=True, exist_ok=True)
    (SEEN / f"{session}.json").write_text(json.dumps(seen), encoding="utf-8")


def orders_and_meter(event: dict[str, object]) -> list[str]:
    """The standing orders when this session has not seen them as they are (a new session, a change, or
    a compaction since), and a warning when its context is past the hand-off size."""
    session = "".join(c for c in str(event.get("session_id") or "") if c.isalnum() or c == "-")
    orders = standing_orders()
    tokens = context_tokens(str(event.get("transcript_path") or ""))
    seen = remembered(session) if session else {}
    digest = hashlib.sha256(orders.encode("utf-8")).hexdigest()
    compacted = tokens is None or (
        tokens and isinstance(seen.get("tokens"), int) and tokens < COMPACTED_SHARE * int(seen["tokens"]))
    said = []
    if orders and (not session or seen.get("orders") != digest or compacted):
        said.append(orders)
    if session:
        remember(session, {"orders": digest, "tokens": tokens})
    limit = int(os.environ.get("ORPHEUM_CONTEXT_WARN_TOKENS") or CONTEXT_WARN_TOKENS)
    if tokens is not None and tokens > limit:
        said.append(f"Context: {tokens // 1000}k tokens, past the hand-off size of {limit // 1000}k. Every model "
                    "call re-reads it all (docs/contributors/lead.md §1). At the next phase boundary, not mid-step and "
                    "not before a short stretch, write the handoff memory and ask the owner to compact: a compaction "
                    "costs about $0.50 and pays back over about 25 calls.")
    return said


def sweep() -> str:
    """Ends every process a stopped subagent left (agents.end_left_by_stopped), and names the
    session's own shells that have run for hours; what it did, for the session to read."""
    said = []
    ended = agents.end_left_by_stopped()
    if ended:
        said.append(f"Ended {len(ended)} process(es) stopped subagents had left running: pids "
                    + ", ".join(map(str, ended)) + ".")
    old = agents.old_untagged_shells()
    if old:
        said.append("Shells of this session running for hours (stop them if nothing needs them):\n- "
                    + "\n- ".join(old))
    return "\n".join(said)


def main() -> None:
    event = hooklib.read_event()
    try:
        swept = sweep()
    except (OSError, subprocess.SubprocessError) as failed:  # a failed sweep says so, never stops the prompt
        swept = f"The sweep for stopped subagents' processes failed: {failed}"
    context = "\n\n".join(part for part in (*orders_and_meter(event), swept) if part)
    if context:
        json.dump({"hookSpecificOutput": {"hookEventName": "UserPromptSubmit", "additionalContext": context}},
                  sys.stdout)


if __name__ == "__main__":
    main()
