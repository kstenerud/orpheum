#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: The token ledger: what each Claude Code session and subagent of this project cost, from their
# ABOUTME: transcripts, so a batch's spend is read in numbers at its end rather than found in the owner's quota.
"""What the sessions cost, from Claude Code's transcripts (~/.claude/projects/<project>/).

Every model call re-reads the whole context from the prompt cache, so a session costs about its calls
times its context (the measurement of 2026-10-31, docs/contributors/lead.md §1). Weighted cost counts
one uncached input token as 1: cache writes 1.25, cache reads 0.1, output 5, the ratios of the prices.
Run it at a batch's end: `python3 scripts/token_usage.py --since 2026-11-01`.
"""
from __future__ import annotations

import argparse
import json
import sys
from dataclasses import dataclass, field
from datetime import datetime
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
WEIGHTS = {"input_tokens": 1.0, "cache_creation_input_tokens": 1.25, "cache_read_input_tokens": 0.1,
           "output_tokens": 5.0}
CONTEXT_KEYS = ("input_tokens", "cache_creation_input_tokens", "cache_read_input_tokens")


@dataclass
class Session:
    name: str
    is_agent: bool
    calls: int = 0
    weighted: float = 0.0
    peak_context: int = 0
    tools: int = 0
    models: set[str] = field(default_factory=set)
    task: str = ""


def transcripts_of(project: Path) -> Path:
    """Claude Code's directory for the project: its absolute path with every / and . made a -."""
    return Path.home() / ".claude" / "projects" / str(project).replace("/", "-").replace(".", "-")


def read(path: Path) -> Session:
    session = Session(name=path.stem, is_agent=path.parent.name == "subagents")
    seen: set[str] = set()
    for line in path.open(encoding="utf-8", errors="replace"):
        try:
            entry = json.loads(line)
        except ValueError:
            continue
        message = entry.get("message") if isinstance(entry, dict) else None
        if not isinstance(message, dict):
            continue
        if not session.task and entry.get("type") == "user" and isinstance(message.get("content"), str):
            session.task = message["content"][:60].replace("\n", " ")
        if entry.get("type") != "assistant":
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
        session.models.add(str(message.get("model")))
    return session


def report(sessions: list[Session], top: int) -> str:
    total = sum(s.weighted for s in sessions) or 1.0
    agents = sum(s.weighted for s in sessions if s.is_agent)
    lines = [f"{len(sessions)} sessions, {sum(s.calls for s in sessions)} model calls, "
             f"{total / 1e6:.1f}M weighted tokens; agents {100 * agents / total:.0f}%, "
             f"lead {100 * (total - agents) / total:.0f}%"]
    for s in sorted(sessions, key=lambda s: -s.weighted)[:top]:
        lines.append(f"{s.weighted / 1e6:7.2f}M {100 * s.weighted / total:4.1f}%  calls {s.calls:4}  tools {s.tools:4}  "
                     f"peak {s.peak_context // 1000:4}k  {'agent' if s.is_agent else 'lead '}  "
                     f"{','.join(sorted(s.models))[:24]:24}  {s.task}")
    return "\n".join(lines)


def main(argv: list[str]) -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--since", help="only transcripts written on or after this day (YYYY-MM-DD)")
    ap.add_argument("--top", type=int, default=15, help="how many of the costliest sessions to list")
    ap.add_argument("--dir", type=Path, help="the transcripts' directory (default: this project's)")
    args = ap.parse_args(argv)
    directory = args.dir or transcripts_of(ROOT)
    since = datetime.fromisoformat(args.since).timestamp() if args.since else 0.0
    paths = [p for p in directory.rglob("*.jsonl") if p.stat().st_mtime >= since]
    if not paths:
        print(f"token_usage: no transcripts in {directory}", file=sys.stderr)
        return 1
    print(report([read(p) for p in sorted(paths)], args.top))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
