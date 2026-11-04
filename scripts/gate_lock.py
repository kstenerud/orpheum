#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: One test run at a time on the repository: the gates, make spot, a test run by hand and the
# ABOUTME: background probe hold one lock, shared by every worktree; a second queues, a sub-run passes.
"""The gate lock (process review F17, gate-throughput G5; every test run, the owner, 2026-11-04).

`gate_lock.py [--as KIND] <command…>` runs the command holding an exclusive `flock` on
`<git-common-dir>/gate.lock`, which every worktree of the repository shares. A run that finds it held
waits, saying so, rather than running beside it: two at once each claim the whole machine and failed
each other's tests on the load (2026-10-20, 2026-10-26). Gates alone took it at first; agents' spot
checks and test runs by hand beside them loaded the machine to 70 on 8 cores and failed each other's
browser tests (the UI revamp's first wave, 2026-11-04), so every run of tests takes it: `make check`,
`make spot`, check-pr, revert-red, the background probe, and a test command typed in a session
(`.claude/hooks/pre_bash.py` runs it through this).

- **Its holder says what it is.** The holder writes its KIND (check, check-pr, revert-red, spot, test,
  probe), its process and the time into the lock file; `--held` prints it. The Stop hook leaves its
  `make check` for a later turn only behind a long gate (LONG), and waits behind the rest.
- **The background probe gives way.** A run finding the probe holding the lock asks it to stop
  (background_probe.stop) and takes the lock once it has: the probe's work is kept, and it runs again
  after the next commit.
- **A run's own sub-runs pass through.** The holder marks its environment (GATE_LOCK_HELD), so a
  gate's steps, revert-red's probes, and a commit's spot check inside a locked command do not wait on
  their own lock. The kernel drops the lock when the holder ends, however it ends.
"""

from __future__ import annotations

import contextlib
import datetime
import fcntl
import os
import subprocess
import sys
from collections.abc import Iterator
from pathlib import Path
from typing import IO

GATE_LOCK_HELD = "ORPHEUM_GATE_LOCK_HELD"
# What holds the lock when a holder does not say.
GATE = "gate"
PROBE = "probe"
# Holders the Stop hook does not wait behind: each can take longer than its ten minutes. A holder
# that wrote nothing (one started before holders said) counts as long, so the hook never waits on it.
LONG = frozenset({"check", "check-pr", "revert-red", GATE, ""})


def lock_path(root: Path | None = None) -> Path:
    common = subprocess.run(["git", "rev-parse", "--path-format=absolute", "--git-common-dir"],
                            capture_output=True, text=True, check=True, cwd=root).stdout.strip()
    return Path(common) / "gate.lock"


def holder(root: Path | None = None) -> str | None:
    """What holds the lock now (its KIND, process and time, as it wrote them), "" for a holder that
    wrote nothing, None when nothing does; asked without waiting."""
    path = lock_path(root)
    if not path.exists():
        return None
    with open(path, "a+", encoding="utf-8") as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_SH | fcntl.LOCK_NB)
        except BlockingIOError:
            lock.seek(0)
            return lock.read().strip()
        fcntl.flock(lock, fcntl.LOCK_UN)
    return None


def holder_kind(said: str | None) -> str | None:
    """The KIND of what holder() said, None when nothing holds it."""
    return None if said is None else (said.split() or [""])[0]


def is_held(root: Path | None = None) -> bool:
    """Whether something holds the lock now, asked without waiting."""
    return holder(root) is not None


def mark(lock: IO[str], kind: str) -> None:
    """Writes what holds the lock into it, for holder() to read."""
    lock.seek(0)
    lock.truncate()
    started = datetime.datetime.now(datetime.timezone.utc).strftime("%H:%M:%S")
    lock.write(f"{kind} pid {os.getpid()} since {started} UTC\n")
    lock.flush()


def try_take(lock: IO[str]) -> bool:
    try:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
    except BlockingIOError:
        return False
    return True


@contextlib.contextmanager
def holding(root: Path | None = None, kind: str = GATE) -> Iterator[dict[str, str]]:
    """Holds the lock as kind, waiting for it when another run has it (asking the background probe to
    stop first, when it is the probe), and yields the environment a sub-run takes to pass through it.
    Held already (a run's own sub-run), it yields at once."""
    if os.environ.get(GATE_LOCK_HELD) == "1":
        yield dict(os.environ)
        return
    with open(lock_path(root), "a+", encoding="utf-8") as lock:
        if not try_take(lock):
            said = holder(root)
            if holder_kind(said) == PROBE:
                sys.path.insert(0, str(Path(__file__).resolve().parent))
                import background_probe  # noqa: PLC0415 (only a run behind the probe needs it)

                background_probe.stop(root)
            if not try_take(lock):
                print(f"Another test run holds this repository's lock ({said or 'unnamed'}); waiting for it "
                      "to finish.", flush=True)
                fcntl.flock(lock, fcntl.LOCK_EX)
        mark(lock, kind)
        yield {**os.environ, GATE_LOCK_HELD: "1"}


def run_holding(command: list[str], kind: str = GATE) -> int:
    with holding(kind=kind) as env:
        return subprocess.run(command, env=env, check=False).returncode


def main(argv: list[str]) -> int:
    if argv == ["--held"]:
        said = holder()
        if said is None:
            return 1
        print(said)
        return 0
    kind = GATE
    if argv[:1] == ["--as"] and len(argv) >= 2:
        kind, argv = argv[1], argv[2:]
    if not argv:
        print("usage: gate_lock.py [--as KIND] <command...> | --held", file=sys.stderr)
        return 2
    return run_holding(argv, kind)


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
