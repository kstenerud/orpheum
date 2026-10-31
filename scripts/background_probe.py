#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Revert-red's code part run in the background after each commit, niced, so check-pr finds
# ABOUTME: most files' probes already recorded (the test-regime audit of 2026-10-31, R6).
"""Probes the batch so far in the background, after each commit.

check-pr's revert-red probed every file of a batch at its end: 2480 s of one check-pr's 2788
(research/test-regime-audit-2026-10-31.md), most of it on files committed hours before. The probes
are kept by revert, by the code each run read (revert_red_known), so a probe made after the commit
that settled a file serves check-pr later, when nothing it read has changed. The post-commit hook
runs `background_probe.py launch`; it starts this, detached, and returns at once.

- **One at a time.** A run holds `<git-common-dir>/revert-red/background.lock`; a commit made while
  it runs marks `again`, and the run, ending, starts over at the new head. A cherry-pick of ten
  commits makes one run, then perhaps a second.
- **Out of the way.** It runs the code part alone (no browser), on two probe trees, at nice 19,
  holding the probe trees' lock as any revert-red run does. A run of the gate's own (check-pr, a
  targeted revert-red) stops it first (`stop`), taking the trees; what it had probed is kept.
- **Never while check-pr runs**, and never in check-pr's own tree.
- **Off** while `<git-common-dir>/revert-red/background-off` exists: `touch` it while the owner
  tests on staging, which shares this machine (memory: no gates during live tests).

It records no failures (`--unrecorded`): the batch's record of what failed is check-pr's.
"""

from __future__ import annotations

import contextlib
import fcntl
import os
import signal
import subprocess
import sys
from pathlib import Path

SCRIPTS = Path(__file__).resolve().parent
OFF = "background-off"
AGAIN = "again"
LOCK = "background.lock"
PID = "background.pid"
LOG = "background-probe.log"
# Two probe trees and the lowest priority: the dev loop and the owner's staging come first.
JOBS = 2
NICENESS = 19
# Set in the run's environment, so its own revert-red does not stop it.
IN_BACKGROUND = "ORPHEUM_BACKGROUND_PROBE"


def folder(root: Path | None = None) -> Path | None:
    """`<git-common-dir>/revert-red`, or None outside a repository."""
    done = subprocess.run(["git", "rev-parse", "--path-format=absolute", "--git-common-dir"], cwd=root,
                          capture_output=True, text=True, check=False)
    return Path(done.stdout.strip()) / "revert-red" if done.returncode == 0 else None


def is_wanted(here: Path) -> bool:
    """Whether a run may start: not switched off, not in check-pr's own tree, no check-pr running."""
    if (here / OFF).exists() or "/.git/check-pr/" in str(Path.cwd()):
        return False
    sys.path.insert(0, str(SCRIPTS))
    import check_pr_snapshot  # noqa: PLC0415 (only a run that may start needs it)

    return check_pr_snapshot.running(Path.cwd()) is None


def launch() -> int:
    """Starts a run, detached, and returns at once; a run already going is told to go again."""
    here = folder()
    if here is None or not is_wanted(here):
        return 0
    here.mkdir(parents=True, exist_ok=True)
    with open(here / LOG, "a", encoding="utf-8") as log:
        subprocess.Popen([sys.executable, str(Path(__file__).resolve()), "run"], stdin=subprocess.DEVNULL,  # noqa: S603
                         stdout=log, stderr=log, start_new_session=True, env={**os.environ, IN_BACKGROUND: "1"})
    return 0


def stop(root: Path | None = None) -> None:
    """Stops a background run, if one is going, and waits until it has let the probe trees go."""
    here = folder(root)
    if here is None or os.environ.get(IN_BACKGROUND) == "1":
        return
    with contextlib.suppress(OSError, ValueError):
        os.killpg(int((here / PID).read_text().strip()), signal.SIGTERM)
    with contextlib.suppress(OSError), open(here / LOCK, "a", encoding="utf-8") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)  # held until the run has ended


def base() -> str | None:
    """The batch's base, as check-pr resolves it when given none, or None when there is none."""
    done = subprocess.run([sys.executable, str(SCRIPTS / "branch_base.py")], capture_output=True, text=True,
                          check=False)
    return done.stdout.strip() or None if done.returncode == 0 else None


def run() -> int:
    """Probes base..HEAD's code, again while commits come, then ends."""
    here = folder()
    if here is None:
        return 0
    with open(here / LOCK, "a", encoding="utf-8") as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            (here / AGAIN).touch()
            return 0
        (here / PID).write_text(f"{os.getpgid(0)}\n", encoding="utf-8")
        try:
            while is_wanted(here):
                (here / AGAIN).unlink(missing_ok=True)
                start = base()
                if start is None:
                    break
                head = subprocess.run(["git", "rev-parse", "HEAD"], capture_output=True, text=True,
                                      check=False).stdout.strip()
                print(f"background probe: {start[:12]}..{head[:12]}", flush=True)
                subprocess.run(["nice", "-n", str(NICENESS), sys.executable, str(SCRIPTS / "check_revert_red.py"),  # noqa: S603
                                "--base", start, "--head", head, "--part", "code", "--unrecorded",
                                "--jobs", str(JOBS)], check=False)
                if not (here / AGAIN).exists():
                    break
        finally:
            (here / PID).unlink(missing_ok=True)
    return 0


def main(argv: list[str]) -> int:
    commands = {"launch": launch, "run": run}
    if len(argv) != 1 or argv[0] not in commands:
        print("usage: background_probe.py launch|run", file=sys.stderr)
        return 2
    return commands[argv[0]]()


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
