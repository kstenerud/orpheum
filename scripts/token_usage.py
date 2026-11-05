#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: The token ledger: what each Claude Code session and subagent of this project cost, from their
# ABOUTME: transcripts, so a batch's spend is read in numbers at its end rather than found in the owner's quota.
"""What the sessions cost, from Claude Code's transcripts (~/.claude/projects/<project>/).

Every model call re-reads the whole context from the prompt cache, so a session costs about its calls
times its context (the measurement of 2026-10-31, docs/contributors/lead.md §1). Weighted cost counts
one uncached input token as 1: cache writes 1.25, cache reads 0.1, output 5, the ratios of the prices.
Cost is in dollars at each model's API price (PRICES), so an Opus call and a Sonnet call are told apart:
weighted alike, the first trial's ledger could not say what moving a job to Sonnet saved (2026-10-31).
Run it at a batch's end: `python3 scripts/token_usage.py --since 2026-11-01`.
"""
from __future__ import annotations

import argparse
import json
import sys
from dataclasses import dataclass, field
from datetime import datetime, timezone
from pathlib import Path

import call_budget

ROOT = Path(__file__).resolve().parents[1]
WEIGHTS = {"input_tokens": 1.0, "cache_creation_input_tokens": 1.25, "cache_read_input_tokens": 0.1,
           "output_tokens": 5.0}
CONTEXT_KEYS = ("input_tokens", "cache_creation_input_tokens", "cache_read_input_tokens")
# Dollars per million tokens: input, output, cache read (the claude-api skill's table, cached 2026-10-06;
# Haiku 5.5's cache read, not in it, taken as a tenth of its input, as Sonnet's is).
# A cache write is 2x input: Claude Code caches for an hour. Keyed by the model id's prefix.
PRICES = {"claude-fable-5-1": (10.0, 50.0, 0.25), "claude-opus-5-5": (4.0, 20.0, 0.20),
          "claude-sonnet-5-5": (2.0, 10.0, 0.20), "claude-haiku-5-5": (0.10, 0.50, 0.01)}
CACHE_WRITE_TIMES_INPUT = 2.0
# A model not in PRICES: its weighted tokens at Opus 5.5's input price, and the report names it.
UNKNOWN_INPUT_PRICE = PRICES["claude-opus-5-5"][0]


def cost(model: str, usage: dict[str, object]) -> tuple[float, bool]:
    """The call's dollars, and whether its model's price is known."""
    def tokens(key: str) -> float:
        return float(usage.get(key) or 0)  # type: ignore[arg-type]
    price = next((p for prefix, p in PRICES.items() if model.startswith(prefix)), None)
    if price is None:
        return sum(weight * tokens(key) for key, weight in WEIGHTS.items()) * UNKNOWN_INPUT_PRICE / 1e6, False
    paid_in, paid_out, paid_read = price
    return (tokens("input_tokens") * paid_in + tokens("cache_creation_input_tokens") * paid_in * CACHE_WRITE_TIMES_INPUT
            + tokens("cache_read_input_tokens") * paid_read + tokens("output_tokens") * paid_out) / 1e6, True


@dataclass
class Session:
    name: str
    is_agent: bool
    calls: int = 0
    weighted: float = 0.0
    dollars: float = 0.0
    unpriced: set[str] = field(default_factory=set)
    peak_context: int = 0
    tools: int = 0
    models: set[str] = field(default_factory=set)
    task: str = ""
    budgets: list[int] = field(default_factory=list)  # as its brief and the lead's messages state them

    @property
    def budget(self) -> int:
        """The calls the agent was given: the brief's budget, or the last a resume stated, to the ceiling."""
        first = self.budgets[0] if self.budgets else call_budget.DEFAULT_BUDGET
        return max(first, min(self.budgets[-1] if self.budgets else first, call_budget.CEILING_TIMES * first))


def transcripts_of(project: Path) -> Path:
    """Claude Code's directory for the project: its absolute path with every / and . made a -."""
    return Path.home() / ".claude" / "projects" / str(project).replace("/", "-").replace(".", "-")


def read(path: Path, since: str = "") -> Session:
    """The session's cost, from its calls stamped on or after since (an ISO time; "": all). A session's
    transcript runs for days, so a file written today holds yesterday's calls too: --since counted the
    lead's whole week and showed agents at 30% where they were 77% (the token audit, 2026-10-31)."""
    session = Session(name=path.stem, is_agent=path.parent.name == "subagents")
    seen: set[str] = set()
    with path.open(encoding="utf-8", errors="replace") as lines:
        for line in lines:
            try:
                entry = json.loads(line)
            except ValueError:
                continue
            message = entry.get("message") if isinstance(entry, dict) else None
            if not isinstance(message, dict):
                continue
            if not session.task and entry.get("type") == "user" and isinstance(message.get("content"), str):
                session.task = message["content"][:60].replace("\n", " ")
            if entry.get("type") == "user":
                session.budgets += call_budget.stated_in(call_budget.said(message))
            if entry.get("type") != "assistant" or str(entry.get("timestamp", "")) < since:
                continue
            content = message.get("content")
            if isinstance(content, list):
                # One reply is written as a line per block, each with the reply's id and usage: tools from every
                # line, usage from the first alone.
                session.tools += sum(1 for part in content if isinstance(part, dict) and part.get("type") == "tool_use")
            usage = message.get("usage")
            if not isinstance(usage, dict) or message.get("id") in seen:
                continue
            seen.add(str(message.get("id")))
            session.calls += 1
            session.weighted += sum(weight * float(usage.get(key) or 0) for key, weight in WEIGHTS.items())
            session.peak_context = max(session.peak_context, sum(int(usage.get(key) or 0) for key in CONTEXT_KEYS))
            model = str(message.get("model"))
            session.models.add(model)
            paid, known = cost(model, usage)
            session.dollars += paid
            if not known:
                session.unpriced.add(model)
    return session


def report(sessions: list[Session], top: int) -> str:
    total = sum(s.dollars for s in sessions) or 1.0
    agents = sum(s.dollars for s in sessions if s.is_agent)
    weighted = sum(s.weighted for s in sessions)
    lines = [f"{len(sessions)} sessions, {sum(s.calls for s in sessions)} model calls, ${total:.2f} at API prices "
             f"({weighted / 1e6:.1f}M weighted tokens); agents {100 * agents / total:.0f}%, "
             f"lead {100 * (total - agents) / total:.0f}%"]
    unpriced = sorted(set().union(*(s.unpriced for s in sessions)))
    if unpriced:
        lines.append(f"no price for {', '.join(unpriced)}: counted at ${UNKNOWN_INPUT_PRICE:g}/M weighted (PRICES)")
    agent_sessions = [s for s in sessions if s.is_agent]
    over = [s for s in agent_sessions if s.tools > s.budget]
    if agent_sessions:
        # Set against what each brief stated, so a batch's end shows where the lead's estimates run low.
        lines.append(f"agents over their call budget: {len(over)} of {len(agent_sessions)}"
                     + "".join(f"; {s.tools}/{s.budget} {s.task[:30]}" for s in over))
    for s in sorted(sessions, key=lambda s: -s.dollars)[:top]:
        budget = f"/{s.budget:<4}" if s.is_agent else "     "
        lines.append(f"${s.dollars:7.2f} {100 * s.dollars / total:4.1f}%  calls {s.calls:4}  tools {s.tools:4}{budget}  "
                     f"peak {s.peak_context // 1000:4}k  {'agent' if s.is_agent else 'lead '}  "
                     f"{','.join(sorted(s.models))[:24]:24}  {s.task}")
    return "\n".join(lines)


def main(argv: list[str]) -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--since", help="only calls made on or after this day or time (YYYY-MM-DD[THH:MM], local time)")
    ap.add_argument("--top", type=int, default=15, help="how many of the costliest sessions to list")
    ap.add_argument("--dir", type=Path, help="the transcripts' directory (default: this project's)")
    args = ap.parse_args(argv)
    directory = args.dir or transcripts_of(ROOT)
    start = datetime.fromisoformat(args.since).astimezone(timezone.utc) if args.since else None
    since = start.strftime("%Y-%m-%dT%H:%M:%S") if start else ""
    paths = [p for p in directory.rglob("*.jsonl") if not start or p.stat().st_mtime >= start.timestamp()]
    if not paths:
        print(f"token_usage: no transcripts in {directory}", file=sys.stderr)
        return 1
    sessions = [s for s in (read(p, since) for p in sorted(paths)) if s.calls]
    if not sessions:
        print(f"token_usage: no calls in {directory} since {args.since}", file=sys.stderr)
        return 1
    print(report(sessions, args.top))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
