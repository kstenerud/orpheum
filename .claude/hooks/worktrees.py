#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Removes a stopped subagent's worktree and branch once main holds every change it made and it
# ABOUTME: holds nothing uncommitted; lists any other, never removing it. Run by the Stop hook.
"""Agents' worktrees, cleaned up when their work is in main (the owner, 2026-10-27).

Nine merged worktrees, with their branches, stayed under .claude/worktrees/ after their work was
merged (2026-10-27). A worktree here is removed, with its branch, only when all of these hold:
its agent has stopped since it last acted (agents.has_stopped); every commit on its branch is in
main as a change, by patch id, so a rebased commit counts (`git cherry`); and its tree holds no
change, tracked or untracked. Any other is listed with what holds it, for the session or
`make handoff` to show, and left as it is. `--also-untracked` also takes worktrees whose agent was
never tracked (made before this), on the same two other conditions: for the session to run once,
by hand.
"""

from __future__ import annotations

import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path

import agents
import hooklib

WORKTREES = Path(".claude") / "worktrees"
MAIN = "main"


@dataclass(frozen=True)
class Worktree:
    path: Path
    branch: str  # empty for a detached HEAD
    is_locked: bool

    @property
    def agent_id(self) -> str:
        return self.path.name.removeprefix("agent-")


def git(*args: str, cwd: Path | None = None) -> subprocess.CompletedProcess[str]:
    return subprocess.run(["git", *args], cwd=cwd or hooklib.ROOT, capture_output=True, text=True, check=False)


def agents_worktrees(root: Path) -> list[Worktree]:
    """The worktrees Claude Code made for agents: .claude/worktrees/agent-<id>/."""
    found, path, branch, is_locked = [], None, "", False
    for line in [*git("worktree", "list", "--porcelain", cwd=root).stdout.splitlines(), ""]:
        if line.startswith("worktree "):
            path, branch, is_locked = Path(line.removeprefix("worktree ")), "", False
        elif line.startswith("branch "):
            branch = line.removeprefix("branch refs/heads/")
        elif line.startswith("locked"):
            is_locked = True
        elif not line and path is not None:
            if path.parent == (root / WORKTREES).resolve() and path.name.startswith("agent-"):
                found.append(Worktree(path, branch, is_locked))
            path = None
    return found


def what_holds(tree: Worktree, root: Path, *, also_untracked: bool) -> str:
    """Why the worktree must stay, or "" when it may go."""
    if not agents.has_stopped(tree.agent_id, root) and not (also_untracked and not agents.state(tree.agent_id, root)):
        return "its agent has not stopped"
    if not tree.branch:
        return "its HEAD is detached"
    changed = git("status", "--porcelain", cwd=tree.path)
    if changed.returncode != 0:
        return "its status cannot be read"
    if changed.stdout.strip():
        return f"{len(changed.stdout.splitlines())} uncommitted file(s)"
    cherry = git("cherry", MAIN, tree.branch, cwd=root)
    if cherry.returncode != 0:
        return "its commits cannot be compared with main"
    missing = [line for line in cherry.stdout.splitlines() if line.startswith("+")]
    if missing:
        return f"{len(missing)} commit(s) main does not hold"
    return ""


def remove(tree: Worktree, root: Path) -> str:
    """Removes the worktree and its branch; "" when done, else what failed."""
    if tree.is_locked:
        git("worktree", "unlock", str(tree.path), cwd=root)
    removed = git("worktree", "remove", str(tree.path), cwd=root)
    if removed.returncode != 0:
        return removed.stderr.strip() or "git worktree remove failed"
    deleted = git("branch", "-D", tree.branch, cwd=root)
    return "" if deleted.returncode == 0 else deleted.stderr.strip()


def clean(root: Path, *, also_untracked: bool = False) -> tuple[list[str], list[str]]:
    """Removes every agent worktree that may go; what was removed, and what stays and why."""
    removed, kept = [], []
    for tree in agents_worktrees(root):
        holds = what_holds(tree, root, also_untracked=also_untracked)
        if not holds:
            failed = remove(tree, root)
            holds = f"removing it failed: {failed}" if failed else ""
        if holds:
            kept.append(f"{tree.path.name}: {holds}")
        else:
            removed.append(f"{tree.path.name} ({tree.branch})")
    return removed, kept


def main(argv: list[str]) -> int:
    removed, kept = clean(hooklib.ROOT, also_untracked="--also-untracked" in argv)
    for name in removed:
        print(f"removed {name}")
    for line in kept:
        print(f"kept {line}")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
