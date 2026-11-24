#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: The record of every make check, make check-pr and make spot run, kept by the runs themselves,
# ABOUTME: and its report: each target's times against its budget, and the steps that took them over.
"""How long the gates take, run by run (the owner, 2026-11-21: make check and make spot "are supposed
to be lightweight alternatives to check-pr that keep dev loop velocity high").

Each run of `make check` and `make check-pr` (scripts/quiet_gate.py) and of `make spot`
(scripts/spot_check.py) appends a line to `gate-logs/runs.jsonl` in the git common directory, so
every worktree's runs and every way of starting one (the Stop hook, the pre-commit hook, an agent, the
owner's terminal) land in one record: when it started, its seconds and exit, the worktree, the load
before and after, the CPU seconds the gate, the sandbox's other work and the host's other work used
over it (cpu_used), and its longest steps. The Stop hook's `.claude/.cache/gate-runs.jsonl` measures the
turn's wait instead, the lock and the image's start included.

`python3 scripts/gate_runs.py` (`make gate-times`) reports each target's runs over the last days:
the median, the slowest tenth, how many went over the budget, and the steps that led the runs over it.
"""

from __future__ import annotations

import argparse
import json
import os
import resource
import subprocess
import sys
import time
from collections import Counter, defaultdict
from pathlib import Path
from statistics import median

# Each gate's budget in seconds: make check's is makefile.md §4's, spot's D279's, check-pr's the owner's
# (the Makefile's CHECK_PR_BUDGET_S, which a test holds to this).
BUDGET_S = {"check": 90, "spot": 30, "check-pr": 300}
# The newest runs kept.
KEPT = 5000
# The longest steps a run's line keeps.
STEPS_KEPT = 6
RECORD = "runs.jsonl"


def record_path() -> Path:
    common = subprocess.run(["git", "rev-parse", "--path-format=absolute", "--git-common-dir"],
                            capture_output=True, text=True, check=False).stdout.strip() or ".git"
    return Path(common) / "gate-logs" / RECORD


def load() -> float:
    try:
        return round(os.getloadavg()[0], 1)
    except OSError:
        return -1.0


# The machine's CPU counters: /proc/stat's first line is the host's whole (a sandbox's included), and
# the cgroup's cpu.stat the sandbox's own, every process in it. A load average says only that the
# machine was busy; these say whose work it was (the owner, 2026-11-24).
HOST_STAT = Path("/proc/stat")
SANDBOX_STAT = Path("/sys/fs/cgroup/cpu.stat")


def host_busy_s(stat: Path = HOST_STAT) -> float | None:
    """The host's busy CPU seconds since it started: user, nice, system, irq and softirq, in clock
    ticks; idle, waiting on disk and stolen by a hypervisor are not work done here."""
    try:
        fields = stat.read_text(encoding="utf-8").splitlines()[0].split()
        ticks = [int(each) for each in fields[1:8]]
        return (ticks[0] + ticks[1] + ticks[2] + ticks[5] + ticks[6]) / os.sysconf("SC_CLK_TCK")
    except (OSError, IndexError, ValueError):
        return None


def sandbox_busy_s(stat: Path = SANDBOX_STAT) -> float | None:
    """The CPU seconds every process in this cgroup has used, or None where there is no cgroup v2."""
    try:
        for line in stat.read_text(encoding="utf-8").splitlines():
            name, _, value = line.partition(" ")
            if name == "usage_usec":
                return int(value) / 1e6
    except (OSError, ValueError):
        return None
    return None


def own_busy_s() -> float:
    """The CPU seconds this process and every child it has waited for have used: a gate's own work,
    its make and all that runs under it."""
    own, children = resource.getrusage(resource.RUSAGE_SELF), resource.getrusage(resource.RUSAGE_CHILDREN)
    return own.ru_utime + own.ru_stime + children.ru_utime + children.ru_stime


def cpu_now() -> dict:
    """The CPU counters as a run starts, for cpu_used at its end."""
    return {"host": host_busy_s(), "sandbox": sandbox_busy_s(), "gate": own_busy_s()}


def cpu_used(before: dict, after: dict | None = None) -> dict:
    """The CPU seconds used since before, by whom: the gate itself; the sandbox's other work (another
    agent's tests, a staging transcode); and the host's other work (another sandbox, the host's own).
    A counter not readable at both ends is left out."""
    after = after or cpu_now()
    used: dict[str, float] = {}
    gate = after["gate"] - before["gate"]
    used["gate"] = round(gate, 1)
    sandbox = None
    if before.get("sandbox") is not None and after.get("sandbox") is not None:
        sandbox = after["sandbox"] - before["sandbox"]
        used["sandboxOthers"] = round(max(0.0, sandbox - gate), 1)
    if before.get("host") is not None and after.get("host") is not None:
        host = after["host"] - before["host"]
        used["hostOthers"] = round(max(0.0, host - (sandbox if sandbox is not None else gate)), 1)
    return used


def worktree() -> str:
    top = subprocess.run(["git", "rev-parse", "--show-toplevel"], capture_output=True, text=True,
                         check=False).stdout.strip()
    return Path(top).name if top else ""


def over_budget(target: str, seconds: float) -> str:
    """", over its N s budget" when the run took longer than its target's budget, else ""."""
    budget = BUDGET_S.get(target)
    return f", over its {budget} s budget" if budget is not None and seconds > budget else ""


def record(run: dict, path: Path | None = None) -> None:
    """Appends a run, keeping the newest KEPT; a record that cannot be written is let go, never failing
    the gate it measures."""
    path = path or record_path()
    try:
        path.parent.mkdir(parents=True, exist_ok=True)
        with path.open("a", encoding="utf-8") as out:
            out.write(json.dumps(run) + "\n")
        lines = path.read_text(encoding="utf-8").splitlines()
        if len(lines) > KEPT + KEPT // 10:
            path.write_text("\n".join(lines[-KEPT:]) + "\n", encoding="utf-8")
    except OSError:
        pass


def started_at(started: float) -> str:
    return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(started))


def read(path: Path | None = None) -> list[dict]:
    path = path or record_path()
    if not path.exists():
        return []
    runs = []
    for line in path.read_text(encoding="utf-8").splitlines():
        try:
            runs.append(json.loads(line))
        except json.JSONDecodeError:
            continue
    return runs


def tenth_slowest(values: list[float]) -> float:
    ordered = sorted(values)
    return ordered[min(len(ordered) - 1, int(len(ordered) * 0.9))]


def report(runs: list[dict], since: str) -> str:
    """Each target's runs that started at or after `since` (an ISO time): runs, failures, the median and
    slowest tenth of the passing runs, how many went over budget, and the steps that led those."""
    by_target: dict[str, list[dict]] = defaultdict(list)
    for run in runs:
        if run.get("start", "") >= since:
            by_target[run.get("target", "?")].append(run)
    if not by_target:
        return f"gate-times: no run recorded since {since}"
    lines = [f"gate-times: runs since {since} (seconds; passing runs)"]
    for target in sorted(by_target):
        all_runs = by_target[target]
        passed = [run["seconds"] for run in all_runs if run.get("exit") == 0]
        budget = BUDGET_S.get(target)
        line = f"  {target}: {len(all_runs)} run(s), {len(all_runs) - len(passed)} failed"
        if passed:
            line += f"; median {median(passed):.0f}, slowest tenth {tenth_slowest(passed):.0f}, max {max(passed):.0f}"
        if budget is not None and passed:
            over = [run for run in all_runs if run.get("exit") == 0 and run["seconds"] > budget]
            line += f"; {len(over)} over its {budget} s budget"
            lines.append(line)
            leaders = Counter(run["steps"][0][0] for run in over if run.get("steps"))
            if leaders:
                led = ", ".join(f"{step} {count}" for step, count in leaders.most_common(4))
                lines.append(f"    the longest step of the runs over it: {led}")
        else:
            lines.append(line)
        shares = cpu_shares(all_runs)
        if shares:
            lines.append(f"    {shares}")
    return "\n".join(lines)


def cpu_shares(runs: list[dict]) -> str:
    """The median cores the runs' gates used, and the median the sandbox's and the host's other work
    used beside them, over the runs that recorded their CPU: "" where none did."""
    measured = [run for run in runs if isinstance(run.get("cpu"), dict) and run.get("seconds", 0) > 0]
    if not measured:
        return ""
    parts = []
    for key, name in (("gate", "the gate"), ("sandboxOthers", "the sandbox's other work"),
                      ("hostOthers", "the host's other work")):
        cores = [run["cpu"][key] / run["seconds"] for run in measured if key in run["cpu"]]
        if cores:
            parts.append(f"{name} {median(cores):.1f}")
    return f"cores used, the median of {len(measured)} run(s): " + ", ".join(parts)


def main(argv: list[str]) -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--days", type=float, default=7, help="the days to report (default 7)")
    args = ap.parse_args(argv)
    print(report(read(), started_at(time.time() - args.days * 86400)))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
