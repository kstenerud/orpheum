# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: What the last run of a slow branch gate found, kept in the git common directory so a
# ABOUTME: targeted re-run can repeat only what failed (revert-red's files, check-pr's steps).
"""The record a slow gate leaves of its last run, for the next run to re-run only what failed.

A record lives at `<git-common-dir>/<gate>/<name>` as JSON: in the common directory, so every
worktree of the repository sees the same one, and outside the tree, so nothing commits it. A
record is a convenience for the fix-and-re-run loop and nothing more: no hook reads one, and a
green targeted re-run never stands for the full gate.
"""

from __future__ import annotations

import contextlib
import fcntl
import json
import subprocess
from pathlib import Path
from typing import Any, Callable, Iterator


def _git(*args: str) -> str:
    return subprocess.run(["git", *args], capture_output=True, text=True, check=False).stdout.strip()


def commit_of(ref: str) -> str | None:
    """The commit `ref` names, or None when it names none."""
    return _git("rev-parse", "--verify", "--quiet", f"{ref}^{{commit}}") or None


def record_path(gate: str, name: str) -> Path:
    common = Path(_git("rev-parse", "--path-format=absolute", "--git-common-dir"))
    return common / gate / name


def read(gate: str, name: str) -> dict[str, Any] | None:
    """The record, or None when there is none or it cannot be read."""
    try:
        return json.loads(record_path(gate, name).read_text())
    except (OSError, ValueError):
        return None


@contextlib.contextmanager
def _locked(path: Path) -> Iterator[None]:
    """One writer at a time: check-pr's steps finish in parallel, each updating the one record."""
    path.parent.mkdir(parents=True, exist_ok=True)
    with open(path.with_name(path.name + ".lock"), "w", encoding="utf-8") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        yield


def update(gate: str, name: str, change: Callable[[dict[str, Any] | None], dict[str, Any]]) -> dict[str, Any]:
    """Replaces the record with `change(old record)`, under a lock, and returns the new one."""
    path = record_path(gate, name)
    with _locked(path):
        new = change(read(gate, name))
        scratch = path.with_name(path.name + ".tmp")
        scratch.write_text(json.dumps(new, indent=2, sort_keys=True) + "\n")
        scratch.replace(path)
        return new
