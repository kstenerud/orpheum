#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: One gate at a time on the repository: make check and check-pr run holding one lock, shared by
# ABOUTME: every worktree; a second queues behind it, and a gate's own sub-runs pass through.
"""The gate lock (process review F17, gate-throughput G5).

`gate_lock.py <command…>` runs the command holding an exclusive `flock` on
`<git-common-dir>/gate.lock`, which every worktree of the repository shares. A gate that finds it
held waits, saying so, rather than running beside it: two gates at once each claim the whole machine
and failed each other's tests on the load (2026-10-20, 2026-10-26). The holder marks its environment
(GATE_LOCK_HELD), so the gate's own sub-runs (check-pr's steps, revert-red's probes) pass straight
through instead of waiting on themselves. The kernel drops the lock when the holder ends, however it
ends. `gate_lock.py --held` says, without waiting, whether some gate holds it: the Stop hook's
question, which must never wait for a gate (it has ten minutes; check-pr takes longer).
"""

from __future__ import annotations

import contextlib
import fcntl
import os
import subprocess
import sys
from collections.abc import Iterator
from pathlib import Path

GATE_LOCK_HELD = "ORPHEUM_GATE_LOCK_HELD"


def lock_path(root: Path | None = None) -> Path:
    common = subprocess.run(["git", "rev-parse", "--path-format=absolute", "--git-common-dir"],
                            capture_output=True, text=True, check=True, cwd=root).stdout.strip()
    return Path(common) / "gate.lock"


def is_held(root: Path | None = None) -> bool:
    """Whether a gate holds the lock now, asked without waiting."""
    path = lock_path(root)
    if not path.exists():
        return False
    with open(path, "a", encoding="utf-8") as held:
        try:
            fcntl.flock(held, fcntl.LOCK_SH | fcntl.LOCK_NB)
        except BlockingIOError:
            return True
        fcntl.flock(held, fcntl.LOCK_UN)
    return False


@contextlib.contextmanager
def holding(root: Path | None = None) -> Iterator[dict[str, str]]:
    """Holds the gate lock, waiting for it when another gate has it, and yields the environment a
    sub-run takes to pass through it. Held already (a gate's own sub-run), it yields at once."""
    if os.environ.get(GATE_LOCK_HELD) == "1":
        yield dict(os.environ)
        return
    with open(lock_path(root), "a", encoding="utf-8") as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            print("Another gate is running on this repository; waiting for it to finish.", flush=True)
            fcntl.flock(lock, fcntl.LOCK_EX)
        yield {**os.environ, GATE_LOCK_HELD: "1"}


def run_holding(command: list[str]) -> int:
    with holding() as env:
        return subprocess.run(command, env=env, check=False).returncode


def main(argv: list[str]) -> int:
    if argv == ["--held"]:
        return 0 if is_held() else 1
    if not argv:
        print("usage: gate_lock.py <command...> | --held", file=sys.stderr)
        return 2
    return run_holding(argv)


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
