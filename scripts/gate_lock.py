#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: One test run at a time on the repository: the gates, make spot, a test run by hand and the
# ABOUTME: background probe hold one lock, shared by every worktree; a second queues, a sub-run passes.
"""The gate lock (process review F17, gate-throughput G5; every test run, the owner, 2026-11-04).

`gate_lock.py [--as KIND] [--lease SECONDS] <command…>` runs the command holding an exclusive `flock`
on `<git-common-dir>/gate.lock`, which every worktree of the repository shares. A run that finds it
held waits, saying so, rather than running beside it: two at once each claim the whole machine and
failed each other's tests on the load (2026-10-20, 2026-10-26). Gates alone took it at first; agents'
spot checks and test runs by hand beside them loaded the machine to 70 on 8 cores and failed each
other's browser tests (the UI revamp's first wave, 2026-11-04), so every run of tests takes it: `make
check`, `make spot`, check-pr, revert-red, the background probe, and a test command typed in a
session (`.claude/hooks/pre_bash.py` runs it through this).

- **Its holder says what it is.** The holder writes its KIND (check, check-pr, revert-red, spot, test,
  probe), its process and the time into the lock file; `--held` prints it. The Stop hook leaves its
  `make check` for a later turn only behind a long gate (LONG), and waits behind the rest.
- **The background probe gives way.** A run finding the probe holding the lock asks it to stop
  (background_probe.stop) and takes the lock once it has: the probe's work is kept, and it runs again
  after the next commit.
- **A run's own sub-runs pass through.** The holder marks its environment (GATE_LOCK_HELD), so a
  gate's steps, revert-red's probes, and a commit's spot check inside a locked command do not wait on
  their own lock.
- **The lock goes with the run, and the run with its wrapper** (the lock's wargaming, 2026-11-04: a
  wrapper killed by a tool's timeout let the lock go while its tests ran on outside it). The command
  holds the lock too, so it is let go only when the wrapper and the command have both ended; the
  command runs in a process group of its own, and a wrapper asked to end (TERM, INT, HUP) ends that
  whole group first.
- **A hold has a lease** (LEASE_SECONDS by kind, or `--lease`). The wrapper ends its command at the
  lease's end and says so; a run waiting behind a holder past its lease (one whose wrapper was killed
  outright) ends the holder's group, when it can see it, and otherwise names what holds the lock.
- **A run started any other way holds it too** (the owner, 2026-11-07: "make each of the direct
  methods of invoking tests ... invoke a common script that checks the lock"). `--for-pid PID` takes a
  hold that lasts while process PID lives and returns once it is held, the hold going on in the
  background (hold_for); a second asking for the same PID joins it. `go test` reaches it through
  `scripts/go_test_exec`, the `-exec` that `make setup` adds to GOFLAGS, for the `go` command's PID
  (its packages' binaries share one hold); vitest and Playwright through `web/e2e/test-lock.ts`, their
  global setup, for their own.
- **The queue is visible.** Each waiting run registers itself (WAITING) and each finished hold records
  its length (HOLDS), so `expected_wait` can say how long a run arriving now would wait; the shell
  hook refuses a command that would spend its timeout waiting, rather than letting the tool kill it.
"""

from __future__ import annotations

import contextlib
import datetime
import fcntl
import io
import json
import os
import signal
import statistics
import subprocess
import sys
import threading
import time
from collections.abc import Iterator
from pathlib import Path
from typing import IO, Any

GATE_LOCK_HELD = "ORPHEUM_GATE_LOCK_HELD"
# What holds the lock when a holder does not say.
GATE = "gate"
PROBE = "probe"
# Holders the Stop hook does not wait behind: each can take longer than its ten minutes. A holder
# that wrote nothing (one started before holders said) counts as long, so the hook never waits on it.
LONG = frozenset({"check", "check-pr", "revert-red", GATE, ""})

MINUTE = 60
HOUR = 60 * MINUTE
# The longest a run of each kind holds the lock before it is ended. Each is well past the longest seen
# (gate-runs.jsonl, 2026-11-04: make check at most 313 s; check-pr 2788 s; revert-red 2480 s of it), so
# only a hung run reaches it. The probe gives way to every other run and needs none.
LEASE_SECONDS = {"spot": 20 * MINUTE, "test": 30 * MINUTE, "check": 30 * MINUTE,
                 "check-pr": 3 * HOUR, "revert-red": 3 * HOUR, GATE: 3 * HOUR}
DEFAULT_LEASE_SECONDS = 30 * MINUTE
# How long a group asked to end has before it is killed.
GRACE_SECONDS = 10.0
# What a run of each kind takes when no hold of it is recorded yet, in seconds.
TYPICAL_SECONDS = {"spot": 60, "test": 60, "check": 120, "check-pr": 3000, "revert-red": 2500, GATE: 300,
                   PROBE: 15}
# A run held past every recorded hold of its kind is still expected to take this long again.
OVERDUE_SECONDS = 60
WAITING = "gate-waiting"
HOLDS = "gate-holds.jsonl"
HOLDS_KEPT = 400


# Where the lock and its records are kept in place of the git common directory: a test's own folder,
# so a hook's test never reads the lock of the clone it runs in.
LOCK_DIR_VARIABLE = "ORPHEUM_GATE_LOCK_DIR"


def common_dir(root: Path | None = None) -> Path:
    if os.environ.get(LOCK_DIR_VARIABLE):
        return Path(os.environ[LOCK_DIR_VARIABLE])
    common = subprocess.run(["git", "rev-parse", "--path-format=absolute", "--git-common-dir"],
                            capture_output=True, text=True, check=True, cwd=root).stdout.strip()
    return Path(common)


def lock_path(root: Path | None = None) -> Path:
    return common_dir(root) / "gate.lock"


def _read_holder(root: Path | None) -> list[str] | None:
    """The lock file's lines as its holder wrote them, None when nothing holds it; asked without waiting."""
    path = lock_path(root)
    if not path.exists():
        return None
    with open(path, "a+", encoding="utf-8") as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_SH | fcntl.LOCK_NB)
        except BlockingIOError:
            lock.seek(0)
            return lock.read().splitlines()
        fcntl.flock(lock, fcntl.LOCK_UN)
    return None


def holder(root: Path | None = None) -> str | None:
    """What holds the lock now (its KIND, process and time, as it wrote them), "" for a holder that
    wrote nothing, None when nothing does; asked without waiting."""
    lines = _read_holder(root)
    if lines is None:
        return None
    return lines[0].strip() if lines else ""


def holder_record(root: Path | None = None) -> dict[str, Any] | None:
    """The holder's record (kind, pid, started, until, group, ns), {} for a holder that wrote none,
    None when nothing holds the lock."""
    lines = _read_holder(root)
    if lines is None:
        return None
    try:
        record = json.loads(lines[1]) if len(lines) > 1 else {}
    except json.JSONDecodeError:
        record = {}
    return record if isinstance(record, dict) else {}


def holder_kind(said: str | None) -> str | None:
    """The KIND of what holder() said, None when nothing holds it."""
    return None if said is None else (said.split() or [""])[0]


def is_held(root: Path | None = None) -> bool:
    """Whether something holds the lock now, asked without waiting."""
    return holder(root) is not None


def pid_namespace() -> str:
    """This process's PID namespace: a process number means the same process only within one, and the
    gate's container has its own (2026-11-03: check-pr signalled a host's process group from it)."""
    try:
        return os.readlink("/proc/self/ns/pid")
    except OSError:
        return ""


def _clock(seconds: float) -> str:
    return datetime.datetime.fromtimestamp(seconds, datetime.timezone.utc).strftime("%H:%M:%S")


def mark(lock: IO[str], kind: str, lease: float | None = None, group: int | None = None,
         started: float | None = None) -> None:
    """Writes what holds the lock into it: a line for people (holder()), and a record (holder_record())."""
    started = time.time() if started is None else started
    record: dict[str, Any] = {"kind": kind, "pid": os.getpid(), "started": started, "ns": pid_namespace()}
    said = f"{kind} pid {os.getpid()} since {_clock(started)} UTC"
    if lease is not None:
        record["until"] = started + lease
        said += f", its lease until {_clock(started + lease)} UTC"
    if group is not None:
        record["group"] = group
    # Written over the old record, then cut to length: a reader never finds the file empty while it is
    # held (2026-11-08: CI read the holder between a truncate and its write, and saw no kind). The lock
    # is opened to append, which would put every write at the end, so appending is off while it writes.
    try:
        fd: int | None = lock.fileno()
    except (OSError, ValueError, io.UnsupportedOperation):
        fd = None
    flags = fcntl.fcntl(fd, fcntl.F_GETFL) if fd is not None else 0
    if fd is not None:
        fcntl.fcntl(fd, fcntl.F_SETFL, flags & ~os.O_APPEND)
    try:
        lock.seek(0)
        lock.write(f"{said}\n{json.dumps(record)}\n")
        lock.truncate()
        lock.flush()
    finally:
        if fd is not None:
            fcntl.fcntl(fd, fcntl.F_SETFL, flags)


def try_take(lock: IO[str]) -> bool:
    try:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
    except BlockingIOError:
        return False
    return True


def lease_for(kind: str) -> float:
    return LEASE_SECONDS.get(kind, DEFAULT_LEASE_SECONDS)


# --- The queue: who waits, and how long holds take ---------------------------------------------


def _waiting_dir(root: Path | None) -> Path:
    return common_dir(root) / WAITING


@contextlib.contextmanager
def _registered_waiting(root: Path | None, kind: str) -> Iterator[None]:
    folder = _waiting_dir(root)
    folder.mkdir(exist_ok=True)
    entry = folder / f"{os.getpid()}-{''.join(filter(str.isdigit, pid_namespace()))}"
    entry.write_text(json.dumps({"kind": kind, "pid": os.getpid(), "ns": pid_namespace(), "since": time.time()}))
    try:
        yield
    finally:
        entry.unlink(missing_ok=True)


def _alive(pid: int) -> bool:
    try:
        os.kill(pid, 0)
    except ProcessLookupError:
        return False
    except PermissionError:
        return True
    return True


def _running(pid: int) -> bool:
    """Whether process pid runs: alive and not a zombie waiting for its parent to read its end."""
    if not _alive(pid):
        return False
    try:
        stat = Path(f"/proc/{pid}/stat").read_text(encoding="utf-8")
    except OSError:
        return True
    return stat.rpartition(")")[2].split()[:1] != ["Z"]


def _hold_marker(pid: int, root: Path | None) -> Path:
    """Where the hold for process pid says who keeps it: one per process, in this PID namespace."""
    namespace = "".join(c for c in pid_namespace() if c.isdigit())
    return common_dir(root) / f"gate-for-{namespace}-{pid}"


def _read_hold(marker: Path) -> tuple[str, int] | None:
    """The hold's state ("asking" or "held") and the process keeping it, None when there is none."""
    try:
        state, _, keeper = marker.read_text(encoding="utf-8").strip().partition(" ")
        return state, int(keeper)
    except (OSError, ValueError):
        return None


def hold_for(pid: int, kind: str = "test", root: Path | None = None) -> int | None:
    """Holds the lock while process pid runs, returning once it is held: the wrapper keeping the hold,
    when this call started it (ending that wrapper lets the lock go at once), None when it joined a
    hold already kept for pid, or the lock is held already (GATE_LOCK_HELD). The hold is a run_holding
    of await_exit in the background, so it waits, queues and keeps its lease as any run does."""
    if os.environ.get(GATE_LOCK_HELD) == "1":
        return None
    marker = _hold_marker(pid, root)
    while True:
        try:
            created = os.open(marker, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o644)
        except FileExistsError:
            said = _read_hold(marker)
            if said is not None and _running(said[1]):
                if said[0] == "held":
                    return None
                time.sleep(0.05)
                continue
            marker.unlink(missing_ok=True)  # its keeper is gone: a hold that ended without its marker
            continue
        with os.fdopen(created, "w", encoding="utf-8") as handle:
            handle.write(f"asking {os.getpid()}\n")
        if (said := holder(root)) is not None:
            print(f"Another test run holds this repository's lock ({said or 'unnamed'}); waiting for it to "
                  "finish.", file=sys.stderr, flush=True)
        # Its output goes nowhere: a test binary's pipes, given to a keeper that outlives the binary,
        # would keep `go test` reading them until the keeper ends, and the keeper waits for `go test`.
        script = str(Path(__file__).resolve())
        keeper = subprocess.Popen([sys.executable, script, "--as", kind, sys.executable, script, "--await-exit",
                                   str(pid), str(marker)], cwd=root, stdin=subprocess.DEVNULL,
                                  stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, start_new_session=True)
        while (said := _read_hold(marker)) is None or said[0] != "held":
            if keeper.poll() is not None:
                marker.unlink(missing_ok=True)
                raise RuntimeError(f"gate lock: the hold for process {pid} ended before it was held")
            time.sleep(0.05)
        return keeper.pid


def await_exit(pid: int, marker: Path) -> int:
    """The command of a hold_for: says the hold is held, by its wrapper, then waits for process pid to
    end, and removes the hold's marker as it ends, however it is ended."""
    def ended(signum, _frame):
        sys.exit(128 + signum)

    for signum in (signal.SIGTERM, signal.SIGINT, signal.SIGHUP):
        signal.signal(signum, ended)
    try:
        written = marker.with_name(marker.name + ".new")
        written.write_text(f"held {os.getppid()}\n", encoding="utf-8")
        os.replace(written, marker)
        while _running(pid):
            time.sleep(0.2)
    finally:
        marker.unlink(missing_ok=True)
    return 0


def waiting(root: Path | None = None) -> list[dict[str, Any]]:
    """The runs waiting for the lock now, oldest first; an entry whose process has gone is dropped."""
    folder = _waiting_dir(root)
    if not folder.is_dir():
        return []
    found = []
    for entry in folder.iterdir():
        try:
            record = json.loads(entry.read_text())
        except (OSError, json.JSONDecodeError):
            continue
        here = record.get("ns") == pid_namespace()
        gone = here and not _alive(int(record.get("pid", 0)))
        stale = time.time() - float(record.get("since", 0)) > lease_for(str(record.get("kind")))
        if gone or stale:
            if here:
                entry.unlink(missing_ok=True)
            continue
        found.append(record)
    return sorted(found, key=lambda record: float(record.get("since", 0)))


def record_hold(root: Path | None, kind: str, seconds: float) -> None:
    """Adds a finished hold to HOLDS, keeping the last HOLDS_KEPT."""
    path = common_dir(root) / HOLDS
    with open(path, "a+", encoding="utf-8") as holds:
        fcntl.flock(holds, fcntl.LOCK_EX)
        holds.seek(0)
        lines = holds.read().splitlines()[-(HOLDS_KEPT - 1):]
        lines.append(json.dumps({"kind": kind, "seconds": round(seconds, 1), "ended": round(time.time())}))
        holds.seek(0)
        holds.truncate()
        holds.write("\n".join(lines) + "\n")


def hold_lengths(root: Path | None = None) -> dict[str, list[float]]:
    """The recorded holds' lengths in seconds, by kind."""
    path = common_dir(root) / HOLDS
    lengths: dict[str, list[float]] = {}
    if not path.exists():
        return lengths
    for line in path.read_text(encoding="utf-8").splitlines():
        try:
            hold = json.loads(line)
            lengths.setdefault(str(hold["kind"]), []).append(float(hold["seconds"]))
        except (json.JSONDecodeError, KeyError, TypeError, ValueError):
            continue
    return lengths


def typical(kind: str, lengths: dict[str, list[float]]) -> float:
    """A run of kind's usual length: the median of its last 20 recorded holds, else TYPICAL_SECONDS."""
    seen = lengths.get(kind, [])[-20:]
    return statistics.median(seen) if seen else float(TYPICAL_SECONDS.get(kind, TYPICAL_SECONDS["test"]))


def remaining(kind: str, elapsed: float, lengths: dict[str, list[float]]) -> float:
    """How much longer a run of kind, held for elapsed, is expected to hold the lock."""
    if kind == PROBE:
        return float(TYPICAL_SECONDS[PROBE])
    seen = sorted(lengths.get(kind, [])[-20:]) or [typical(kind, lengths)]
    for length in (statistics.median(seen), seen[int(len(seen) * 0.9)], seen[-1]):
        if elapsed < length:
            return length - elapsed
    return float(OVERDUE_SECONDS)


def expected_wait(root: Path | None = None) -> tuple[float, str]:
    """How long a run arriving now would wait for the lock, and what for, in words: the holder's
    expected rest and each waiting run's usual length. (0, "") when the lock is free."""
    record = holder_record(root)
    if record is None:
        return 0.0, ""
    lengths = hold_lengths(root)
    # A holder started before holders kept a record says only its kind, in its line.
    kind = str(record.get("kind") or holder_kind(holder(root)) or GATE)
    elapsed = max(time.time() - float(record.get("started") or time.time()), 0.0)
    wait = remaining(kind, elapsed, lengths)
    said = f"{kind} has held it {elapsed / MINUTE:.0f} min" if elapsed >= MINUTE else f"{kind} has held it {elapsed:.0f} s"
    if kind != PROBE:
        said += f" (its kind usually takes {typical(kind, lengths):.0f} s)"
    queued = waiting(root)
    for each in queued:
        wait += typical(str(each.get("kind")), lengths)
    if queued:
        kinds = ", ".join(str(each.get("kind")) for each in queued)
        said += f", and {len(queued)} more wait{'s' if len(queued) == 1 else ''} before it ({kinds})"
    return wait, said


# --- Waiting, and ending what overstays ---------------------------------------------------------


class _Woken(Exception):
    pass


def _flock_until(lock: IO[str], deadline: float | None) -> bool:
    """Blocks for the lock, keeping this run's place in the kernel's queue; False when deadline came
    first. A wake-up needs a signal, so off the main thread it waits without one."""
    if deadline is None or threading.current_thread() is not threading.main_thread():
        fcntl.flock(lock, fcntl.LOCK_EX)
        return True

    def wake(_signum, _frame):
        raise _Woken

    previous = signal.signal(signal.SIGALRM, wake)
    try:
        signal.setitimer(signal.ITIMER_REAL, max(deadline - time.time(), 0.01))
        fcntl.flock(lock, fcntl.LOCK_EX)
        return True
    except _Woken:
        return False
    finally:
        signal.setitimer(signal.ITIMER_REAL, 0)
        signal.signal(signal.SIGALRM, previous)


def end_group(group: int, first: int = signal.SIGTERM, grace: float = GRACE_SECONDS) -> None:
    """Asks a process group to end (first), and kills what is left of it after grace."""
    try:
        os.killpg(group, first)
    except (ProcessLookupError, PermissionError):
        return
    given_up_at = time.monotonic() + grace
    while time.monotonic() < given_up_at:
        try:
            os.killpg(group, 0)
        except (ProcessLookupError, PermissionError):
            return
        time.sleep(0.1)
    with contextlib.suppress(ProcessLookupError, PermissionError):
        os.killpg(group, signal.SIGKILL)


def holders_of(path: Path) -> list[int]:
    """The processes of this PID namespace with path open, read from /proc."""
    target = str(path.resolve())
    found = []
    for proc in Path("/proc").iterdir():
        if not proc.name.isdigit() or int(proc.name) == os.getpid():
            continue
        try:
            if any(os.readlink(fd) == target for fd in (proc / "fd").iterdir()):
                found.append(int(proc.name))
        except OSError:
            continue
    return found


def _end_overstayed(record: dict[str, Any], root: Path | None) -> None:
    said = f"{record.get('kind')} pid {record.get('pid')}"
    group = record.get("group")
    if group is not None and record.get("ns") == pid_namespace():
        print(f"gate lock: {said} held the test lock past its lease ({_clock(float(record['until']))} UTC), "
              "its wrapper gone; ending its process group.", flush=True)
        end_group(int(group))
        return
    users = ", ".join(str(pid) for pid in holders_of(lock_path(root))) or "none this namespace can see"
    print(f"gate lock: {said} holds the test lock past its lease ({_clock(float(record['until']))} UTC), and this "
          f"run cannot end it (no group recorded, or another PID namespace). Processes holding the lock file: "
          f"{users}. Waiting on.", flush=True)


def wait_for(lock: IO[str], root: Path | None) -> None:
    """Blocks until this run holds the lock, ending once a holder that overstays its lease."""
    ended: set[tuple[Any, Any]] = set()
    while True:
        record = holder_record(root) or {}
        until = record.get("until")
        key = (record.get("pid"), record.get("started"))
        if until is not None and key in ended:
            until = None
        elif until is not None and time.time() >= float(until):
            ended.add(key)
            _end_overstayed(record, root)
            until = time.time() + GRACE_SECONDS + 1
        if _flock_until(lock, None if until is None else float(until)):
            return


@contextlib.contextmanager
def _held(root: Path | None, kind: str, lease: float | None) -> Iterator[tuple[IO[str] | None, dict[str, str]]]:
    """Holds the lock as kind (see holding), yielding the lock file, None for a sub-run, and the
    environment; records the hold's length when it ends."""
    if os.environ.get(GATE_LOCK_HELD) == "1":
        yield None, dict(os.environ)
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
                with _registered_waiting(root, kind):
                    wait_for(lock, root)
        started = time.time()
        mark(lock, kind, lease, started=started)
        try:
            yield lock, {**os.environ, GATE_LOCK_HELD: "1"}
        finally:
            if kind != PROBE:
                with contextlib.suppress(OSError):
                    record_hold(root, kind, time.time() - started)


@contextlib.contextmanager
def holding(root: Path | None = None, kind: str = GATE) -> Iterator[dict[str, str]]:
    """Holds the lock as kind, waiting for it when another run has it (asking the background probe to
    stop first, when it is the probe), and yields the environment a sub-run takes to pass through it.
    Held already (a run's own sub-run), it yields at once. Its lease is kind's, enforced by the runs
    waiting behind it: with no group recorded, they name it rather than end it."""
    with _held(root, kind, lease_for(kind)) as (_lock, env):
        yield env


def run_holding(command: list[str], kind: str = GATE, lease: float | None = None) -> int:
    """Runs command holding the lock as kind: the command holds it too, in a process group of its own
    that this wrapper ends when asked to end or when the lease runs out."""
    lease = lease_for(kind) if lease is None else lease
    with _held(None, kind, lease) as (lock, env):
        if lock is None:
            return subprocess.run(command, env=env, check=False).returncode
        group: list[int] = []

        def forward(signum, _frame):
            if group:
                end_group(group[0], first=signum)
            sys.exit(128 + signum)

        # Before the command starts, so no signal finds it running and the wrapper without its handler.
        for signum in (signal.SIGTERM, signal.SIGINT, signal.SIGHUP):
            signal.signal(signum, forward)
        child = subprocess.Popen(command, env=env, pass_fds=(lock.fileno(),), start_new_session=True)
        group.append(child.pid)
        record = holder_record() or {}
        mark(lock, kind, lease, group=child.pid, started=float(record.get("started") or time.time()))
        try:
            return child.wait(timeout=lease)
        except subprocess.TimeoutExpired:
            print(f"gate lock: the {kind} run held the test lock for its whole lease ({lease / MINUTE:.0f} min) "
                  "and is ended. A run that needs longer names it: gate_lock.py --as KIND --lease SECONDS.",
                  file=sys.stderr, flush=True)
            end_group(child.pid)
            return 124


def main(argv: list[str]) -> int:
    if argv == ["--held"]:
        said = holder()
        if said is None:
            return 1
        print(said)
        return 0
    if argv[:1] == ["--await-exit"] and len(argv) == 3:
        return await_exit(int(argv[1]), Path(argv[2]))
    kind, lease = GATE, None
    while argv[:1] in (["--as"], ["--lease"]) and len(argv) >= 2:
        if argv[0] == "--as":
            kind = argv[1]
        else:
            lease = float(argv[1])
        argv = argv[2:]
    if argv[:1] == ["--for-pid"] and len(argv) == 2:
        keeper = hold_for(int(argv[1]), kind if kind != GATE else "test")
        print(keeper or "", flush=True)
        return 0
    if not argv:
        print("usage: gate_lock.py [--as KIND] [--lease SECONDS] <command...> | --for-pid PID | --held",
              file=sys.stderr)
        return 2
    return run_holding(argv, kind, lease)


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
