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
from pathlib import Path

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
# A turn-end gate put off by a long gate's hold runs as the lock frees (the owner, 2026-11-24, agreeing to
# the test-time stock-take's item 7: a commit made while check-pr ran was linted by nothing until the next
# check-pr). While its waiter waits or runs, the file names it ({"pid", "target", "since"}); a failed run
# leaves its output there ({"target", "exit", "output", "ended"}) for the next turn's end to block with.
PUT_OFF = "gate-put-off.json"
WHEN_FREE = "--when-free"
PUT_OFF_POLL_SECONDS = 15
# A waiter gives up after this: a check-pr is an hour or two, and the next turn's end puts off another.
PUT_OFF_GIVE_UP_SECONDS = 6 * 3600


def main() -> None:
    if sys.argv[1:2] == [WHEN_FREE]:
        when_free()
        return
    event = hooklib.read_event()
    # Claude Code sets it when the turn is ending again after this hook blocked it.
    blocked_before = event.get("stop_hook_active") is True
    # Every turn's end, whatever it changed: a stopped agent's worktree whose work main holds goes,
    # with its branch (worktrees.py); one holding anything else stays, for `make handoff` to list.
    # Never let the cleanup stand between the turn and its gate.
    with contextlib.suppress(OSError, subprocess.SubprocessError):
        worktrees.clean(hooklib.ROOT)
    if (PENDING / DEFERRED).exists():
        # A design being iterated on with the owner, checked once when the owner is happy (the owner,
        # 2026-10-30: "Treat this whole cycle as if it were a single change ... with the checks
        # deferred"). The marks stay; removing the file brings the gate back at the next turn's end.
        return
    put_off = read_put_off()
    if put_off is not None:
        if waiter_running(put_off):
            return  # it gates the tree, this turn's changes too, once the lock frees
        (PENDING / PUT_OFF).unlink(missing_ok=True)
        if put_off.get("exit") not in (None, 0) and not already_passed():
            ended = time.strftime("%H:%M", time.gmtime(float(put_off.get("ended") or 0)))
            block(str(put_off.get("target")), f"(It ran at {ended} UTC, put off by the test lock until it "
                  f"freed, after the turn that owed it had ended.)\n\n{put_off.get('output', '')}")
            return
    target = owed_target()
    if target is None:
        return
    if hooklib.check_pr_running() is not None or gate_held():
        # Run beside check-pr, make check slowed it and failed a test on the load (2026-10-20). The
        # mark stays, and a waiter runs the gate as the lock frees (PUT_OFF).
        start_when_free()
        return
    try:
        # Here, or in the development image through ./dev, wherever the tools are.
        command = run_gate.gate_command(target, os.environ)
    except run_gate.NoWayToRun as missing:
        cannot_run(target, str(missing), blocked_before)
        return
    done = run_target(command, target)
    if done.returncode == 0:
        return
    # make check prints only failures and the log's path (scripts/quiet_gate.py); cap it anyway.
    output = (done.stdout + done.stderr)[-6000:]
    if done.returncode == run_gate.load_launcher().NEEDS_BUILD:
        cannot_run(target, output, blocked_before)
        return
    block(target, output)


def owed_target() -> str | None:
    """The gate the tree is owed (`check`, or `check-documentation` after documentation alone), None
    when it owes none; the marks of a tree the gate already passed on are cleared."""
    code = PENDING / "check-pending-code"
    docs = PENDING / "check-pending-docs"
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
        return None
    if already_passed():
        # The gate already passed on exactly this tree, as a commit's does in the same turn: running
        # it again ran the same gate twice (gate-throughput G2, 2026-10-26).
        code.unlink(missing_ok=True)
        docs.unlink(missing_ok=True)
        return None
    return target


def run_target(command: list[str], target: str) -> subprocess.CompletedProcess[str]:
    """Runs the gate's command, marked running for the status line and recorded in RUNS; a pass clears
    the marks."""
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
        (PENDING / "check-pending-code").unlink(missing_ok=True)
        (PENDING / "check-pending-docs").unlink(missing_ok=True)
    return done


def read_put_off() -> dict | None:
    try:
        said = json.loads((PENDING / PUT_OFF).read_text())
    except (OSError, ValueError):
        return None
    return said if isinstance(said, dict) else None


def waiter_running(put_off: dict) -> bool:
    """Whether the put-off gate's waiter still waits or runs: its process lives and is the waiter."""
    pid = put_off.get("pid")
    if not isinstance(pid, int):
        return False
    try:
        return WHEN_FREE in Path(f"/proc/{pid}/cmdline").read_bytes().decode(errors="replace").split("\0")
    except OSError:
        return False


def start_when_free() -> None:
    """Starts a waiter that runs the turn-end gate once no long gate holds the lock, unless one runs."""
    put_off = read_put_off()
    if put_off is not None and waiter_running(put_off):
        return
    with contextlib.suppress(OSError):
        PENDING.mkdir(parents=True, exist_ok=True)
        subprocess.Popen([sys.executable, str(Path(__file__).resolve()), WHEN_FREE], cwd=hooklib.ROOT,
                         stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                         start_new_session=True)


def when_free(sleep=time.sleep) -> None:
    """The waiter: once no check-pr or long gate holds the lock, runs the gate the tree then owes; a
    failure is left in PUT_OFF for the next turn's end. It gives up while the checks are deferred,
    and after PUT_OFF_GIVE_UP_SECONDS."""
    put_off = PENDING / PUT_OFF
    PENDING.mkdir(parents=True, exist_ok=True)
    put_off.write_text(json.dumps({"pid": os.getpid(), "since": time.time()}))
    given_up_at = time.monotonic() + PUT_OFF_GIVE_UP_SECONDS
    try:
        while hooklib.check_pr_running() is not None or gate_held():
            if (PENDING / DEFERRED).exists() or time.monotonic() > given_up_at:
                return
            sleep(PUT_OFF_POLL_SECONDS)
        target = None if (PENDING / DEFERRED).exists() else owed_target()
        if target is None:
            return
        try:
            command = run_gate.gate_command(target, os.environ)
        except run_gate.NoWayToRun:
            return  # the next turn's end says what is missing
        done = run_target(command, target)
        if done.returncode in (0, run_gate.load_launcher().NEEDS_BUILD):
            return
        put_off.write_text(json.dumps({"target": target, "exit": done.returncode, "ended": time.time(),
                                       "output": (done.stdout + done.stderr)[-6000:]}))
        put_off = None  # the failure stays for the next turn's end
    finally:
        if put_off is not None:
            put_off.unlink(missing_ok=True)


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
    """Whether a long gate holds the repository's test lock (scripts/gate_lock.py: make check,
    check-pr, revert-red). The Stop hook never waits for one: it has ten minutes, and check-pr takes
    longer; the mark stays for the next turn. Behind a spot check or a test run its make check waits,
    and the background probe gives way to it (the owner, 2026-11-04)."""
    import gate_lock  # noqa: PLC0415 (in scripts/, which hooklib puts on the path)

    try:
        kind = gate_lock.holder_kind(gate_lock.holder(hooklib.ROOT))
    except (OSError, subprocess.CalledProcessError):
        return False  # no git to find the lock by (a bare machine): the gate's own run says what is missing
    return kind is not None and kind in gate_lock.LONG


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
