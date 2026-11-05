# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Subagents' commands tagged with their agent, each agent's last action and stop recorded, and
# ABOUTME: the processes a stopped agent left behind found and ended: cleanup no agent must remember.
"""What the hooks know of subagents (the owner, 2026-10-27: "a more deterministic way to clean up").

A review agent's `rg`, given no path, read its stdin for twelve hours after the agent had finished,
and nine merged worktrees stayed behind (2026-10-27). So pre_bash rewrites every command a subagent
runs (`wrapped`): the agent's id in its environment, which every process it starts inherits, its
stdin closed, and a time limit on one run in the background. on_subagent_stop ends every process
carrying a stopped agent's id; on_prompt sweeps for any left; scripts/clean_worktrees.py removes a
stopped agent's worktree once main holds its work. Each agent's last action and stop are kept in
`<git-common-dir>/agents/<id>.json`: an agent counts as stopped when it stopped after it last acted,
so one resumed by a message is running again.
"""

from __future__ import annotations

import contextlib
import fcntl
import functools
import json
import os
import re
import shlex
import signal
import subprocess
import time
from collections.abc import Iterator
from pathlib import Path

import hooklib

# The variable a subagent's every process carries, naming its agent.
TAG = "ORPHEUM_AGENT_ID"
# The longest a subagent's command in the background runs: past check-pr's 21 minutes (2026-10-26),
# short of a forgotten one holding the machine for hours.
BACKGROUND_LIMIT_SECONDS = 30 * 60
# How long a process asked to end has before it is killed.
GRACE_SECONDS = 2.0
# An agent id as Claude Code gives it; anything else is never put into a command.
AGENT_ID = re.compile(r"^[A-Za-z0-9_-]{1,64}$")


def is_agent_id(agent_id: object) -> bool:
    return isinstance(agent_id, str) and bool(AGENT_ID.match(agent_id))


def wrapped(command: str, agent_id: str, *, is_background: bool) -> str:
    """The command a subagent runs in place of command: tagged with its agent, its stdin closed (a
    command waiting on input waits forever, as `rg` with no path did), and, in the background, ended
    after BACKGROUND_LIMIT_SECONDS. A command in the foreground runs in the shell itself, so a `cd`
    carries to the agent's next command as it did; the Bash tool's own timeout bounds it."""
    tag = f"export {TAG}={shlex.quote(agent_id)}; exec </dev/null; "
    if not is_background:
        return tag + command
    limit = f"timeout --kill-after={int(GRACE_SECONDS) + 28} {BACKGROUND_LIMIT_SECONDS}"
    return f"{tag}{limit} bash -c {shlex.quote(command)}"


# Where the agents' records are kept in place of <git-common-dir>/agents: the tests' own directory.
STATE_DIR_VARIABLE = "ORPHEUM_AGENTS_STATE_DIR"


def state_dir(root: Path | None = None) -> Path:
    if os.environ.get(STATE_DIR_VARIABLE):
        return Path(os.environ[STATE_DIR_VARIABLE])
    return _git_agents_dir(str(root or hooklib.ROOT))


@functools.lru_cache(maxsize=None)
def _git_agents_dir(root: str) -> Path:
    """Asked of git once per process: asked once per record, counting the running agents ran 12,000 git
    processes over 6,012 records and held every launch for 7 seconds (the token audit, 2026-10-31)."""
    done = subprocess.run(["git", "rev-parse", "--path-format=absolute", "--git-common-dir"],
                          cwd=root, capture_output=True, text=True, check=True)
    return Path(done.stdout.strip()) / "agents"


def _state_path(agent_id: str, root: Path | None) -> Path:
    return state_dir(root) / f"{agent_id}.json"


def _read(path: Path) -> dict[str, float]:
    try:
        found = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, ValueError):
        return {}
    return found if isinstance(found, dict) else {}


def state(agent_id: str, root: Path | None = None) -> dict[str, float]:
    return _read(_state_path(agent_id, root))


@contextlib.contextmanager
def _locked(directory: Path) -> Iterator[None]:
    """Holds the records' lock: one agent's parallel tool calls each read, change and write its record, and
    unlocked they lost each other's counts (the token audit, 2026-10-31)."""
    directory.mkdir(parents=True, exist_ok=True)
    with (directory / ".lock").open("a") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        try:
            yield
        finally:
            fcntl.flock(lock, fcntl.LOCK_UN)


def _write(path: Path, recorded: dict[str, float]) -> None:
    temporary = path.with_name(f".{path.stem}.{os.getpid()}.tmp")
    temporary.write_text(json.dumps(recorded), encoding="utf-8")
    temporary.replace(path)


# How long an agent's record is kept after it was last written. Claude Code fires SubagentStop for its
# own helpers too, about four a minute, and 231 records had gathered in an afternoon (2026-10-27); a
# worktree's agent stopped long before is judged stopped as one never recorded would not be, so a week
# keeps every record that can still matter.
RECORD_KEPT_SECONDS = 7 * 24 * 60 * 60
# A record of a stop alone, never an action, is one of those helpers: it ran no command and holds no
# worktree, so nothing reads it after an hour, and 6,012 had gathered by 2026-10-31.
STOP_ALONE_KEPT_SECONDS = 60 * 60
FORGET_EVERY_SECONDS = 10 * 60


def _record(agent_id: str, key: str, root: Path | None, now: float | None) -> None:
    if not is_agent_id(agent_id):
        return
    at = time.time() if now is None else now
    path = _state_path(agent_id, root)
    with _locked(path.parent):
        recorded = _read(path)
        if key == "active":
            recorded = _acting(recorded, path.parent, at)
        _write(path, {**recorded, key: at})
    _forget_old(path.parent, at)


def _acting(recorded: dict[str, float], directory: Path, at: float) -> dict[str, float]:
    """The record of an agent acting now, before its "active" is set: an agent first seen takes up one of the
    launches noted for it; one acting after it stopped was resumed by a message and starts its count anew,
    its budget read again from that message, and the calls of its runs before kept as "spent" for the ceiling
    (on_tool_done.py)."""
    if "active" not in recorded:
        _take_launch(directory, at)
    elif "stopped" in recorded and recorded["stopped"] >= recorded["active"]:
        spent = int(recorded.get("spent", 0)) + int(recorded.get("calls", 0))
        recorded = {key: value for key, value in recorded.items()
                    if key not in ("calls", "reminded", "warned", "budget")}
        recorded["spent"] = spent
    return recorded


def _forget_old(directory: Path, now: float) -> None:
    """Deletes the records last written more than RECORD_KEPT_SECONDS before now, and a helper's record of a
    stop alone after STOP_ALONE_KEPT_SECONDS. Looked at once in FORGET_EVERY_SECONDS: every subagent call
    writes a record, and each look reads every record past an hour old."""
    marker = directory / ".forgotten"
    try:
        if 0 <= now - float(marker.read_text(encoding="utf-8")) < FORGET_EVERY_SECONDS:
            return
    except (OSError, ValueError):
        pass
    marker.write_text(str(now), encoding="utf-8")
    for record in directory.glob("*.json"):
        try:
            age = now - record.stat().st_mtime
            if age > RECORD_KEPT_SECONDS or (age > STOP_ALONE_KEPT_SECONDS and "active" not in _read(record)):
                record.unlink()
        except OSError:
            continue


def mark_active(agent_id: str, root: Path | None = None, now: float | None = None) -> None:
    _record(agent_id, "active", root, now)


def mark_stopped(agent_id: str, root: Path | None = None, now: float | None = None) -> None:
    _record(agent_id, "stopped", root, now)


def count_call(agent_id: str, root: Path | None = None, now: float | None = None) -> tuple[int, int]:
    """Records one more tool call of the agent, as an action: how many it has made, and at which call it
    was last reminded of its brief's size (0: never)."""
    if not is_agent_id(agent_id):
        return 0, 0
    at = time.time() if now is None else now
    path = _state_path(agent_id, root)
    with _locked(path.parent):
        recorded = _acting(_read(path), path.parent, at)
        calls = int(recorded.get("calls", 0)) + 1
        _write(path, {**recorded, "active": at, "calls": calls})
    return calls, int(recorded.get("reminded", 0))


def mark_reminded(agent_id: str, calls: int, root: Path | None = None) -> None:
    note(agent_id, root, reminded=calls)


def note(agent_id: str, root: Path | None = None, **values: float) -> None:
    """Sets the values in the agent's record: what the call-budget hook keeps between calls."""
    if not is_agent_id(agent_id):
        return
    path = _state_path(agent_id, root)
    with _locked(path.parent):
        _write(path, {**_read(path), **values})


# Launches let through whose agents have not yet acted. Agents launched in one message each passed the
# limit before any had a record, so every one saw none running (the token audit, 2026-10-31); a launch
# counts until its agent first acts, or LAUNCH_PENDING_SECONDS pass (one refused after the check).
LAUNCHES = ".launches"
LAUNCH_PENDING_SECONDS = 2 * 60


def _launches(directory: Path, at: float) -> list[float]:
    try:
        noted = json.loads((directory / LAUNCHES).read_text(encoding="utf-8"))
    except (OSError, ValueError):
        return []
    return sorted(t for t in noted if isinstance(t, (int, float)) and at - t <= LAUNCH_PENDING_SECONDS) \
        if isinstance(noted, list) else []


def _take_launch(directory: Path, at: float) -> None:
    pending = _launches(directory, at)
    if pending:
        (directory / LAUNCHES).write_text(json.dumps(pending[1:]), encoding="utf-8")


def launch_if_room(most: int, root: Path | None = None, now: float | None = None) -> list[str] | None:
    """Notes a launch if fewer than most agents run or wait to start, and returns None; else returns them,
    each with how long since it last acted. Checked and noted under the lock, so launches in one message
    are counted one after another."""
    at = time.time() if now is None else now
    directory = state_dir(root)
    with _locked(directory):
        pending = _launches(directory, at)
        busy = [f"{name} ({int(at - active) // 60} min since it acted)" for name, active in _running(directory, at)]
        busy += ["one launched, not yet started"] * len(pending)
        if len(busy) >= most:
            return busy
        (directory / LAUNCHES).write_text(json.dumps([*pending, at]), encoding="utf-8")
    return None


# An agent that acted within this long and has not stopped since counts as running. A working agent
# makes a call every few seconds; one silent for longer is waiting on a command, which its tagged
# processes show, or died unseen, and then blocks a launch only this long (the token audit, 2026-10-31).
RUNNING_WITHIN_SECONDS = 5 * 60
# The longest an agent waits on its commands: BACKGROUND_LIMIT_SECONDS ends any of them.
WAITING_WITHIN_SECONDS = BACKGROUND_LIMIT_SECONDS + 5 * 60


def running(root: Path | None = None, now: float | None = None) -> list[str]:
    """The agents running now, by their records: not stopped since they last acted, and acted within
    RUNNING_WITHIN_SECONDS, or within WAITING_WITHIN_SECONDS with a process of theirs still running."""
    at = time.time() if now is None else now
    try:
        return [name for name, _ in _running(state_dir(root), at)]
    except (OSError, subprocess.SubprocessError):
        return []


def _running(directory: Path, at: float) -> list[tuple[str, float]]:
    found = []
    waiting: set[str] | None = None
    for record in sorted(directory.glob("*.json")):
        try:
            if at - record.stat().st_mtime > WAITING_WITHIN_SECONDS:
                continue
        except OSError:
            continue
        recorded = _read(record)
        active = float(recorded.get("active", 0.0))
        if "stopped" in recorded and recorded["stopped"] >= active:
            continue
        if at - active > RUNNING_WITHIN_SECONDS:
            if at - active > WAITING_WITHIN_SECONDS:
                continue
            waiting = set(tagged_processes().values()) if waiting is None else waiting
            if record.stem not in waiting:
                continue
        found.append((record.stem, active))
    return found


def has_stopped(agent_id: str, root: Path | None = None) -> bool:
    """Whether the agent stopped after its last action: an agent never seen stopping has not."""
    recorded = state(agent_id, root)
    return "stopped" in recorded and recorded["stopped"] >= recorded.get("active", 0.0)


def tagged_processes(proc: Path = Path("/proc")) -> dict[int, str]:
    """Every process carrying TAG, by pid, with the agent it names. Another user's processes, and
    any that ends while read, are passed over."""
    found: dict[int, str] = {}
    prefix = f"{TAG}=".encode()
    for entry in proc.iterdir():
        if not entry.name.isdigit() or int(entry.name) == os.getpid():
            continue
        try:
            environment = (entry / "environ").read_bytes()
        except OSError:
            continue
        for variable in environment.split(b"\0"):
            if variable.startswith(prefix):
                found[int(entry.name)] = variable[len(prefix):].decode(errors="replace")
                break
    return found


def end(pids: list[int]) -> list[int]:
    """Asks each process to end, kills those still there after GRACE_SECONDS; the pids it ended."""
    asked = []
    for pid in pids:
        try:
            os.kill(pid, signal.SIGTERM)
            asked.append(pid)
        except (ProcessLookupError, PermissionError):
            continue
    deadline = time.monotonic() + GRACE_SECONDS
    living = list(asked)
    while living and time.monotonic() < deadline:
        time.sleep(0.1)
        living = [pid for pid in living if _is_alive(pid)]
    for pid in living:
        try:
            os.kill(pid, signal.SIGKILL)
        except (ProcessLookupError, PermissionError):
            continue
    return asked


def _is_alive(pid: int) -> bool:
    try:
        os.kill(pid, 0)
    except ProcessLookupError:
        return False
    except PermissionError:
        return True
    # A zombie is ended; only its parent can reap it.
    try:
        return Path(f"/proc/{pid}/stat").read_text().split(") ", 1)[1][:1] != "Z"
    except (OSError, IndexError):
        return False


def end_left_by(agent_id: str, proc: Path = Path("/proc")) -> list[int]:
    """Ends every process the agent's commands left; the pids ended."""
    return end([pid for pid, owner in tagged_processes(proc).items() if owner == agent_id])


def end_left_by_stopped(root: Path | None = None, proc: Path = Path("/proc")) -> list[int]:
    """Ends every tagged process whose agent has stopped (the sweep at each prompt); the pids ended."""
    return end([pid for pid, owner in tagged_processes(proc).items() if has_stopped(owner, root)])


# How old a shell of Claude Code's own must be for the sweep to name it: a gate or a deploy ends in
# minutes; the review agent's `rg` was found at twelve hours (2026-10-27).
OLD_SHELL_SECONDS = 4 * 60 * 60
# A shell Claude Code's Bash tool starts, as its command line begins.
CLAUDE_SHELL = re.compile(r"^/bin/bash -c source \S*shell-snapshots/")


def _age_seconds(stat: str, uptime: float, ticks: int) -> float:
    started = int(stat.split(") ", 1)[1].split()[19]) / ticks
    return uptime - started


def old_untagged_shells(proc: Path = Path("/proc"), older_than: float = OLD_SHELL_SECONDS) -> list[str]:
    """Claude Code's shells older than older_than that carry no agent's tag: the session's own, which
    no agent's stop ends. Named for the session to look at, never ended: one may be a run it means
    to keep."""
    uptime = float((proc / "uptime").read_text().split()[0])
    ticks = os.sysconf("SC_CLK_TCK")
    tagged = tagged_processes(proc)
    found = []
    for entry in proc.iterdir():
        if not entry.name.isdigit() or int(entry.name) in tagged:
            continue
        try:
            arguments = (entry / "cmdline").read_bytes().replace(b"\0", b" ").decode(errors="replace").strip()
            age = _age_seconds((entry / "stat").read_text(), uptime, ticks)
        except (OSError, IndexError, ValueError):
            continue
        if CLAUDE_SHELL.match(arguments) and age > older_than:
            command = arguments.split("eval ", 1)[-1][:160]
            found.append(f"pid {entry.name}, {age / 3600:.1f} h: {command}")
    return found


if __name__ == "__main__":
    import sys

    # An agent that died unseen still counts as running for RUNNING_WITHIN_SECONDS; the lead, told so by a
    # refused launch, marks it stopped: `python3 .claude/hooks/agents.py stopped <agent id>`.
    if len(sys.argv) == 3 and sys.argv[1] == "stopped" and is_agent_id(sys.argv[2]):
        mark_stopped(sys.argv[2])
    else:
        sys.exit("usage: agents.py stopped <agent id>")
