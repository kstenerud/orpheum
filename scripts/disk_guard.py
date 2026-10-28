#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Before a gate or a staging build: trims the persistent caches to their caps, and stops with
# ABOUTME: what holds the space when the disk is still nearly full, rather than failing mid-run.
"""The disk guard (the disk-full review, 2026-10-28).

    disk_guard.py            trim the caches if due, then check the free space
    disk_guard.py --trim     trim the caches now, whatever the last trim

The sandbox's disk reached 97%: Go's build cache had grown to 44 GB, runs killed mid-way had left
their temporary files behind, the gate scripts' tests theirs at every run, and a full disk
fails a gate in ways that look like anything but a full disk. So `make check`, `make check-pr` and
`./staging` call this first:

1. **Caps.** Each persistent cache is trimmed to its cap, least recently used first, at most once
   an hour (a stamp in the caches' own directory): Go's build cache, whose entries Go marks used by
   their modification time and rebuilds when missing, as its own five-day trim does; Node's compile
   cache, which the Makefile keeps beside it rather than in a run's TMPDIR; and Playwright's
   browsers, of which only the pinned revisions are kept.
2. **Test leftovers.** What tests leave in the temporary directory when their run is killed part-way
   (Go's t.TempDir()s and build work, browser profiles, the gate scripts' tests' directories), by
   their names, two hours old. A run's own temporary directory would hold them better, but dozens
   of Go tests' sockets fail under any TMPDIR longer than /tmp, and Go 1.26 puts t.TempDir() under
   GOTMPDIR too (code-debt.md).
3. **Free space.** Below MIN_FREE_BYTES free after the trim, it fails, naming the largest caches and
   the command to look further, before a run that would fail obscurely part-way.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import time
from dataclasses import dataclass
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
GiB = 1 << 30
# The least free space a gate starts with: check-pr's probe trees, films and browsers take several
# gigabytes at their height, and the Go build of a probe tree several more.
MIN_FREE_BYTES = 12 * GiB
# A trim at most this often: walking a large Go cache takes seconds.
TRIM_EVERY_SECONDS = 3600
# What a cache is trimmed down to once over its cap, so the next run does not trim again at once.
TRIM_TO = 0.8


# The names tests give what they make in the temporary directory: Go's t.TempDir() (the test's name
# and a number), Go's build work, Playwright's browser profiles, and the gate scripts' tests'.
LEFTOVER = re.compile(r"Test\w*?\d+|go-build\d+|playwright[\w-]*profile[\w-]*|py-tests(-home)?-\w+|"
                      r"(branch-)?gate-test-\w+|quiet-gate-\w+|format-files-bin\w*|post-edit-format\w*|"
                      r"gen-test\w*|gate-lock-\w+|disk-guard-\w+")
# How old a leftover is before it is swept: older than any one test binary's run. The guard runs as
# a gate starts, holding the gate lock, so no other gate's tests are running in this environment.
LEFTOVER_AGE_SECONDS = 2 * 3600


@dataclass(frozen=True)
class Cap:
    """A cache trimmed oldest first to cap_bytes."""

    name: str
    path: Path
    cap_bytes: int


def go_cache() -> Path | None:
    """Go's build cache, as `go env` names it; none without Go."""
    if not shutil.which("go"):
        return None
    done = subprocess.run(["go", "env", "GOCACHE"], capture_output=True, text=True, check=False)
    value = done.stdout.strip()
    return Path(value) if done.returncode == 0 and value and value != "off" else None


def caps(env: dict[str, str]) -> list[Cap]:
    """The caches with a cap, those present on this machine."""
    found = []
    go = go_cache()
    if go is not None:
        # Every tree's builds share it (-trimpath): the working tree, check-pr's and revert-red's.
        found.append(Cap("Go's build cache", go, 25 * GiB))
    compile_cache = env.get("NODE_COMPILE_CACHE")
    if compile_cache:
        found.append(Cap("Node's compile cache", Path(compile_cache), 1 * GiB))
    return found


def files_by_age(path: Path) -> tuple[list[tuple[float, int, Path]], int]:
    """Every file under path with its modification time and size, oldest first, and their total."""
    entries = []
    total = 0
    for directory, _, names in os.walk(path):
        for name in names:
            file = Path(directory) / name
            try:
                stat = file.lstat()
            except OSError:
                continue
            entries.append((stat.st_mtime, stat.st_size, file))
            total += stat.st_size
    entries.sort()
    return entries, total


def trim(cap: Cap) -> int:
    """Deletes cap's oldest files until it holds TRIM_TO of its cap, once over it; the bytes freed."""
    if not cap.path.is_dir():
        return 0
    entries, total = files_by_age(cap.path)
    if total <= cap.cap_bytes:
        return 0
    goal = int(cap.cap_bytes * TRIM_TO)
    freed = 0
    for _, size, file in entries:
        if total - freed <= goal:
            break
        # Go's own bookkeeping, kept: the cache's README and its trim stamp.
        if file.parent == cap.path:
            continue
        try:
            file.unlink()
        except OSError:
            continue
        freed += size
    return freed


def pinned_browsers(lock: Path) -> set[str] | None:
    """The directory names of the browsers the pinned Playwright uses, `chromium-1243` and so on."""
    try:
        browsers = json.loads(lock.read_text(encoding="utf-8"))["browsers"]
    except (OSError, ValueError, KeyError):
        return None
    return {f"{browser['name'].replace('-', '_')}-{browser['revision']}" for browser in browsers}


def trim_browsers(store: Path, pinned: set[str] | None) -> list[str]:
    """Deletes the browsers in store no pinned Playwright uses; the names deleted."""
    if pinned is None or not store.is_dir():
        return []
    gone = []
    for entry in sorted(store.iterdir()):
        if entry.is_dir() and "-" in entry.name and not entry.name.startswith(".") and entry.name not in pinned:
            shutil.rmtree(entry, ignore_errors=True)
            gone.append(entry.name)
    return gone


def sweep_leftovers(temporary: Path, now: float) -> int:
    """Deletes the tests' leftovers in temporary a day old or more; how many."""
    swept = 0
    try:
        entries = list(temporary.iterdir())
    except OSError:
        return 0
    for entry in entries:
        if not LEFTOVER.fullmatch(entry.name):
            continue
        try:
            if now - entry.lstat().st_mtime < LEFTOVER_AGE_SECONDS:
                continue
        except OSError:
            continue
        if entry.is_dir() and not entry.is_symlink():
            shutil.rmtree(entry, ignore_errors=True)
        else:
            entry.unlink(missing_ok=True)
        swept += 1
    return swept


def browser_store(env: dict[str, str]) -> Path:
    """Where Playwright keeps its browsers on Linux."""
    named = env.get("PLAYWRIGHT_BROWSERS_PATH")
    return Path(named) if named and named != "0" else Path.home() / ".cache" / "ms-playwright"


def stamp_path() -> Path:
    return Path.home() / ".cache" / "orpheum" / "disk-guard-trimmed"


def is_due(stamp: Path, now: float) -> bool:
    try:
        return now - stamp.stat().st_mtime >= TRIM_EVERY_SECONDS
    except OSError:
        return True


def trim_all(env: dict[str, str]) -> None:
    for cap in caps(env):
        freed = trim(cap)
        if freed:
            print(f"disk guard: trimmed {cap.name} by {freed / GiB:.1f} GiB, to under "
                  f"{cap.cap_bytes * TRIM_TO / GiB:.0f} GiB ({cap.path})", flush=True)
    lock = ROOT / "web" / "node_modules" / "playwright-core" / "browsers.json"
    for name in trim_browsers(browser_store(env), pinned_browsers(lock)):
        print(f"disk guard: deleted Playwright's {name}, which the pinned Playwright does not use", flush=True)
    temporary = Path(tempfile.gettempdir())
    if swept := sweep_leftovers(temporary, time.time()):
        print(f"disk guard: swept {swept} tests' leftovers two hours old from {temporary}", flush=True)
    stamp = stamp_path()
    stamp.parent.mkdir(parents=True, exist_ok=True)
    stamp.touch()


def free_bytes(path: Path) -> int:
    return shutil.disk_usage(path).free


def too_full(free: int, env: dict[str, str]) -> str:
    """What to tell a person whose disk has free bytes left."""
    named = "\n".join(f"  {cap.name}: {cap.path} (capped at {cap.cap_bytes / GiB:.0f} GiB)" for cap in caps(env))
    return (f"disk guard: only {free / GiB:.1f} GiB free, under the {MIN_FREE_BYTES / GiB:.0f} GiB a gate "
            f"needs; stopping now rather than failing part-way.\nThe capped caches:\n{named}\n"
            "Look further with `du -xh --max-depth=2 / | sort -h | tail` and `docker system df`.")


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--trim", action="store_true", help="trim the caches now, whatever the last trim")
    arguments = parser.parse_args(argv)
    env = dict(os.environ)
    if arguments.trim or is_due(stamp_path(), time.time()):
        trim_all(env)
    free = free_bytes(ROOT)
    if free < MIN_FREE_BYTES:
        trim_all(env)
        free = free_bytes(ROOT)
    if free < MIN_FREE_BYTES:
        print(too_full(free, env), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
