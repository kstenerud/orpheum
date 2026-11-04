#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Stop hook: removes stopped agents' merged worktrees, then, if the turn changed files, runs the
# ABOUTME: gate (`make check`, or `make check-documentation` after docs alone) where the tools are; blocks on failure.
from __future__ import annotations

import contextlib
import json
import os
import subprocess
import sys
import time

import hooklib
import run_gate
import worktrees

PENDING = hooklib.ROOT / ".claude" / ".cache"
# Its presence in PENDING defers the turn-end gate while the owner iterates on a design, which make
# check refuses to run beside (scripts/check_stamp.py DEFERRED_MARK; standards/design-cycle.md).
DEFERRED = "check-deferred"
# One line a run of the turn-end gate: when it started, how long it ran, the load either side and the
# FFmpeg jobs running as it started, so a slow turn can be explained after it (the owner, 2026-11-01:
# "why is make check taking so long?"; agreed for the 11-05 batch). The newest RUNS_KEPT lines stay.
RUNS = "gate-runs.jsonl"
RUNS_KEPT = 1000
# Present while the turn-end gate runs, naming its target, start and this hook's process, for a status
# line to show (gate_status.py): Claude Code shows a hook's output only once it ends, so a turn whose gate
# ran for minutes showed nothing of it (the owner, 2026-11-04: "go with your recommendation").
RUNNING = "gate-running.json"


def main() -> None:
    event = hooklib.read_event()
    # Claude Code sets it when the turn is ending again after this hook blocked it.
    blocked_before = event.get("stop_hook_active") is True
    # Every turn's end, whatever it changed: a stopped agent's worktree whose work main holds goes,
    # with its branch (worktrees.py); one holding anything else stays, for `make handoff` to list.
    # Never let the cleanup stand between the turn and its gate.
    with contextlib.suppress(OSError, subprocess.SubprocessError):
        worktrees.clean(hooklib.ROOT)
    code = PENDING / "check-pending-code"
    docs = PENDING / "check-pending-docs"
    if (PENDING / DEFERRED).exists():
        # A design being iterated on with the owner, checked once when the owner is happy (the owner,
        # 2026-10-30: "Treat this whole cycle as if it were a single change ... with the checks
        # deferred"). The marks stay; removing the file brings the gate back at the next turn's end.
        return
    if code.exists():
        target = "check"
    elif docs.exists():
        target = "check-documentation"
    elif not already_passed():
        # Nothing marked, but the tree is not the one make check last passed: a shell command wrote
        # it, or commits were picked in, which no edit hook sees. A commit runs only the spot check
        # (the owner, 2026-10-30), so a 62-line function written by a script went unlinted until an
        # agent's gate found it (c9189b2a); the gate runs here instead. What runs is chosen by what changed
        # since that pass, as for an edit's mark: documentation alone gets the documentation check (the
        # owner, 2026-10-31: a docs commit ran the whole make check for five minutes).
        import documentation_only  # noqa: PLC0415 — scripts/ is on the path once hooklib is imported

        target = "check-documentation" if documentation_only.is_documentation_alone(changed_since_pass()) else "check"
    else:
        return
    if already_passed():
        # The gate already passed on exactly this tree, as a commit's does in the same turn: running
        # it again ran the same gate twice (gate-throughput G2, 2026-10-26).
        code.unlink(missing_ok=True)
        docs.unlink(missing_ok=True)
        return
    if hooklib.check_pr_running() is not None or gate_held():
        # Run beside check-pr, make check slowed it and failed a test on the load (2026-10-20). The
        # mark stays, so the first turn to end after check-pr does runs it.
        return
    try:
        # Here, or in the development image through ./dev, wherever the tools are.
        command = run_gate.gate_command(target, os.environ)
    except run_gate.NoWayToRun as missing:
        cannot_run(target, str(missing), blocked_before)
        return
    # Never build the image here: a build takes minutes past the hook's limit (./dev says to run it).
    env = {**os.environ, "ORPHEUM_DEV_NO_BUILD": "1"}
    started, load, transcodes = time.time(), os.getloadavg()[0], ffmpeg_jobs()
    running = PENDING / RUNNING
    with contextlib.suppress(OSError):
        PENDING.mkdir(parents=True, exist_ok=True)
        running.write_text(json.dumps({"target": target, "start": started, "pid": os.getpid()}))
    try:
        done = subprocess.run(command, cwd=hooklib.ROOT, capture_output=True, text=True, check=False, env=env)
    finally:
        running.unlink(missing_ok=True)
    with contextlib.suppress(OSError):
        record_run({"start": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(started)), "target": target,
                    "seconds": round(time.time() - started), "exit": done.returncode,
                    "loadBefore": round(load, 1), "loadAfter": round(os.getloadavg()[0], 1),
                    "ffmpegJobs": transcodes})
    if done.returncode == 0:
        code.unlink(missing_ok=True)
        docs.unlink(missing_ok=True)
        return
    # make check prints only failures and the log's path (scripts/quiet_gate.py); cap it anyway.
    output = (done.stdout + done.stderr)[-6000:]
    if done.returncode == run_gate.load_launcher().NEEDS_BUILD:
        cannot_run(target, output, blocked_before)
        return
    block(target, output)


def ffmpeg_jobs() -> int:
    """How many FFmpeg processes run here: staging's node transcodes in this machine's Docker, and a
    transcode slows the gate (the owner, 2026-11-01). 0 when it cannot be told."""
    try:
        found = subprocess.run(["pgrep", "-c", "-x", "ffmpeg"], capture_output=True, text=True, check=False)
    except OSError:
        return 0
    return int(found.stdout.strip() or 0) if found.stdout.strip().isdigit() else 0


def record_run(run: dict) -> None:
    """Appends a run of the gate to PENDING's record, keeping the newest RUNS_KEPT."""
    record = PENDING / RUNS
    lines = record.read_text().splitlines() if record.exists() else []
    lines = [*lines, json.dumps(run)][-RUNS_KEPT:]
    PENDING.mkdir(parents=True, exist_ok=True)
    record.write_text("\n".join(lines) + "\n")


def cannot_run(target: str, output: str, blocked_before: bool) -> None:
    """The gate could not run at all (no tools, or ./dev's image needs building, which the hook never
    does). It blocks the turn once, so the agent sees it; blocking again looped an agent the owner had
    told not to build the image, the hook re-blocking each of its replies (the owner's Mac, 2026-11-01:
    "Block once"). The second time the turn ends with a warning, and the mark stays for the next turn.
    A failure of the gate itself still blocks every time."""
    if not blocked_before:
        block(target, output)
        return
    json.dump({"systemMessage": f"`make {target}` could not run, so this turn ends ungated; its mark stays "
               f"for the next turn's end:\n\n{output}"}, sys.stdout)


def gate_held() -> bool:
    """Whether some gate holds the repository's gate lock (scripts/gate_lock.py). The Stop hook never
    waits for one: it has ten minutes, and check-pr takes longer; the mark stays for the next turn."""
    done = subprocess.run([sys.executable, str(hooklib.ROOT / "scripts" / "gate_lock.py"), "--held"],
                          cwd=hooklib.ROOT, capture_output=True, check=False)
    return done.returncode == 0


def already_passed() -> bool:
    """Whether `make check` passed on exactly the working tree as it is now (scripts/check_stamp.py)."""
    done = subprocess.run([sys.executable, str(hooklib.ROOT / "scripts" / "check_stamp.py"), "matches"],
                          cwd=hooklib.ROOT, capture_output=True, check=False)
    return done.returncode == 0


def changed_since_pass() -> list[str]:
    """The paths the working tree changed since make check last passed; empty when it never did."""
    done = subprocess.run([sys.executable, str(hooklib.ROOT / "scripts" / "check_stamp.py"), "changed"],
                          cwd=hooklib.ROOT, capture_output=True, text=True, check=False)
    return [line for line in done.stdout.splitlines() if line] if done.returncode == 0 else []


def block(target: str, output: str) -> None:
    json.dump(
        {
            "decision": "block",
            "reason": f"`make {target}` failed at the end of the turn. Fix these before "
            f"stopping:\n\n{output}",
        },
        sys.stdout,
    )


if __name__ == "__main__":
    main()
