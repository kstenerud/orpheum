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
- **One test run at a time.** Each pass holds the repository's test lock as `probe` (gate_lock.py),
  waiting for it while anything else runs tests, and any run wanting the lock stops the pass first
  (the owner, 2026-11-04: the probe beside agents' runs loaded the machine and failed their tests).
- **Stopped by a file, not a signal.** The post-commit hook starts it on the host, and check-pr runs
  in the gate's container, whose processes are numbered apart: its signal to the recorded process
  group reached nothing, said nothing, and check-pr waited eight minutes for the run to end on its
  own (2026-11-03). `stop` writes `stop` in the shared git directory, which both see; the run,
  watching for it, ends its revert-red (SIGTERM, which ends every test run going and records none
  of them: revert_red_run), and `stop` says how long it waited.
- **Its log says when.** Each run's start and end, and a stop, are in the log with the clock time.
- **Never while check-pr runs**, and never in check-pr's own tree.
- **Off** while `<git-common-dir>/revert-red/background-off` exists: `touch` it while the owner
  tests on staging, which shares this machine (memory: no gates during live tests).

It records no failures (`--unrecorded`): the batch's record of what failed is check-pr's.
"""

from __future__ import annotations

import datetime
import fcntl
import os
import signal
import subprocess
import sys
import time
from pathlib import Path
from typing import IO

SCRIPTS = Path(__file__).resolve().parent
sys.path.insert(0, str(SCRIPTS))

import gate_lock  # noqa: E402

OFF = "background-off"
AGAIN = "again"
LOCK = "background.lock"
STOP = "stop"
# How often a run looks for the stop file, in seconds.
STOP_POLL_SECONDS = 1.0
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
                         stdout=log, stderr=log, start_new_session=True, env=run_environment(os.environ))
    return 0


def run_environment(hook: dict[str, str] | os._Environ[str]) -> dict[str, str]:
    """The run's environment: the hook's, but for git's own variables, which a hook runs with
    (GIT_DIR, GIT_INDEX_FILE): kept, every git command in the probe trees used the commit's index
    and failed (its first run, 2026-10-31)."""
    return {**{name: value for name, value in hook.items() if not name.startswith("GIT_")}, IN_BACKGROUND: "1"}


def stop(root: Path | None = None) -> float:
    """Stops a background run, if one is going, and waits until it has let the probe trees go;
    returns the seconds it waited. It asks by the stop file, which a run in another process
    namespace (the host's, from check-pr's container) sees as well as one beside it."""
    here = folder(root)
    if here is None or os.environ.get(IN_BACKGROUND) == "1":
        return 0.0
    here.mkdir(parents=True, exist_ok=True)
    with open(here / LOCK, "a", encoding="utf-8") as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            return 0.0  # none was going
        except BlockingIOError:
            pass
        (here / STOP).touch()
        asked = time.monotonic()
        print(f"background probe: asked the run going to stop at {clock_text()} UTC; waiting for it to end",
              flush=True)
        try:
            fcntl.flock(lock, fcntl.LOCK_EX)  # held until the run has ended
        finally:
            (here / STOP).unlink(missing_ok=True)
        waited = time.monotonic() - asked
        print(f"background probe: it ended {waited:.0f} s after it was asked", flush=True)
        return waited


def clock_text() -> str:
    """The clock time now, HH:MM:SS in UTC, as the log prints it."""
    return datetime.datetime.now(datetime.timezone.utc).strftime("%H:%M:%S")


def take_test_lock(here: Path, lock: IO[str]) -> bool:
    """Takes the repository's test lock (gate_lock.py) as the probe, waiting while another run holds
    it; False, without it, once the run is asked to stop or is no longer wanted. It never blocks on the
    lock: a gate holding it may be waiting in `stop` for this run to end."""
    while is_wanted(here) and not (here / STOP).exists():
        if gate_lock.try_take(lock):
            gate_lock.mark(lock, gate_lock.PROBE)
            return True
        time.sleep(STOP_POLL_SECONDS)
    return False


def probe_until_stopped(here: Path, command: list[str]) -> int:
    """Runs revert-red, ending it with SIGTERM once the stop file appears; its exit status."""
    env = {**os.environ, gate_lock.GATE_LOCK_HELD: "1"}
    with subprocess.Popen(command, env=env) as running:  # noqa: S603
        while running.poll() is None:
            if (here / STOP).exists():
                print(f"background probe: stopped at {clock_text()} UTC, for a run of the gate's own", flush=True)
                running.send_signal(signal.SIGTERM)
                return running.wait()
            time.sleep(STOP_POLL_SECONDS)
        return running.returncode


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
        while is_wanted(here) and not (here / STOP).exists():
            (here / AGAIN).unlink(missing_ok=True)
            start = base()
            if start is None:
                break
            head = subprocess.run(["git", "rev-parse", "HEAD"], capture_output=True, text=True,
                                  check=False).stdout.strip()
            with open(gate_lock.lock_path(), "a+", encoding="utf-8") as test_lock:
                if not take_test_lock(here, test_lock):
                    break
                print(f"background probe: {start[:12]}..{head[:12]}, started at {clock_text()} UTC", flush=True)
                began = time.monotonic()
                status = probe_until_stopped(here, [
                    "nice", "-n", str(NICENESS), sys.executable, str(SCRIPTS / "check_revert_red.py"),
                    "--base", start, "--head", head, "--part", "code", "--unrecorded", "--jobs", str(JOBS)])
                print(f"background probe: ended at {clock_text()} UTC, after {time.monotonic() - began:.0f} s "
                      f"(exit {status})", flush=True)
            if not (here / AGAIN).exists():
                break
    return 0


def main(argv: list[str]) -> int:
    commands = {"launch": launch, "run": run}
    if len(argv) != 1 or argv[0] not in commands:
        print("usage: background_probe.py launch|run", file=sys.stderr)
        return 2
    return commands[argv[0]]()


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
