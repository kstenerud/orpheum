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

import json
import os
import re
import shlex
import signal
import subprocess
import time
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
    done = subprocess.run(["git", "rev-parse", "--path-format=absolute", "--git-common-dir"],
                          cwd=root or hooklib.ROOT, capture_output=True, text=True, check=True)
    return Path(done.stdout.strip()) / "agents"


def _state_path(agent_id: str, root: Path | None) -> Path:
    return state_dir(root) / f"{agent_id}.json"


def state(agent_id: str, root: Path | None = None) -> dict[str, float]:
    path = _state_path(agent_id, root)
    try:
        found = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, ValueError):
        return {}
    return found if isinstance(found, dict) else {}


# How long an agent's record is kept after it was last written. Claude Code fires SubagentStop for its
# own helpers too, about four a minute, and 231 records had gathered in an afternoon (2026-10-27); a
# worktree's agent stopped long before is judged stopped as one never recorded would not be, so a week
# keeps every record that can still matter.
RECORD_KEPT_SECONDS = 7 * 24 * 60 * 60


def _record(agent_id: str, key: str, root: Path | None, now: float | None) -> None:
    if not is_agent_id(agent_id):
        return
    path = _state_path(agent_id, root)
    path.parent.mkdir(parents=True, exist_ok=True)
    at = time.time() if now is None else now
    recorded = {**state(agent_id, root), key: at}
    temporary = path.with_suffix(".tmp")
    temporary.write_text(json.dumps(recorded), encoding="utf-8")
    temporary.replace(path)
    _forget_old(path.parent, at)


def _forget_old(directory: Path, now: float) -> None:
    """Deletes the records last written more than RECORD_KEPT_SECONDS before now."""
    for record in directory.glob("*.json"):
        try:
            if now - record.stat().st_mtime > RECORD_KEPT_SECONDS:
                record.unlink()
        except OSError:
            continue


def mark_active(agent_id: str, root: Path | None = None, now: float | None = None) -> None:
    _record(agent_id, "active", root, now)


def mark_stopped(agent_id: str, root: Path | None = None, now: float | None = None) -> None:
    _record(agent_id, "stopped", root, now)


def count_call(agent_id: str, root: Path | None = None, now: float | None = None) -> int:
    """Records one more tool call of the agent, as an action, and returns how many it has made."""
    if not is_agent_id(agent_id):
        return 0
    calls = int(state(agent_id, root).get("calls", 0)) + 1
    _record(agent_id, "active", root, now)
    path = _state_path(agent_id, root)
    temporary = path.with_suffix(".tmp")
    temporary.write_text(json.dumps({**state(agent_id, root), "calls": calls}), encoding="utf-8")
    temporary.replace(path)
    return calls


# An agent that acted within this long and has not stopped since counts as running. A working agent
# makes a call every few seconds; one silent for longer waits for something, or was never seen to stop.
RUNNING_WITHIN_SECONDS = 15 * 60


def running(root: Path | None = None, now: float | None = None) -> list[str]:
    """The agents running now, by their records: acted within RUNNING_WITHIN_SECONDS, not stopped since."""
    at = time.time() if now is None else now
    try:
        records = sorted(state_dir(root).glob("*.json"))
    except (OSError, subprocess.SubprocessError):
        return []
    found = []
    for record in records:
        recorded = state(record.stem, root)
        active = float(recorded.get("active", 0.0))
        if at - active <= RUNNING_WITHIN_SECONDS and not has_stopped(record.stem, root):
            found.append(record.stem)
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
