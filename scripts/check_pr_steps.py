#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Records each `make check-pr` step's pass or fail with the commit it ran on, and runs
# ABOUTME: `make check-pr-failed`: only the steps that did not pass, revert-red on its failed files.
"""check-pr's steps, recorded one by one so a fix-and-re-run loop repeats only what failed.

`make check-pr` takes minutes, most of them in steps that passed. It records each step at
`<git-common-dir>/check-pr/last-results`:

* `start --base B` begins an empty record for a run against B.
* `record STEP -- COMMAND...` runs one step's command, records whether it passed with the commit
  HEAD was at, and exits with the command's status.
* `durations STEP...` prints how long each step of the run took, the longest first, with the clock
  times it started and ended, so a run over check-pr's budget names the step that grew (D279) and
  the log can be read against the clock (the owner, 2026-11-03: "make the testing process more
  observable"; an overrun's timeline was found from files' modification times).
* `rerun --base B --last LAST STEP...` is `make check-pr-failed`. The Makefile names the steps
  (PR_STEPS, and PR_LAST_STEP, which runs alone after them). It re-runs, the same way, each step
  that failed or has no result (it never ran: the last step runs only after the others pass), and
  names each one it skipped with the commit it passed on. Revert-red, when it ran and failed,
  re-runs only its failed files (`check_revert_red.py --rerun-failed`).

A re-run's green is not check-pr's green: the skipped steps passed on an older commit, and the
fixes since may have broken them. So `rerun` always ends by saying the full `make check-pr` is
still required, and nothing here writes the check stamp or anything else a hook trusts.

**A step whose inputs are unchanged** (the owner, 2026-10-28, the caching audit's finding 10): the
pure steps (STEP_INPUTS) are each a function of the files they read, the base and the toolchain, so
`record` keeps each pass under a key of those, at `<git-common-dir>/check-pr/step-inputs`, and a
later run of the step whose key was seen passing takes the pass without running, saying so. That is
not a targeted green: the inputs are the same, as Go's test cache says inside make check. A full
check-pr after a Go-only fix no longer runs web-lint-full; after a VQ tick, none of them. The commit
steps (lint-commits, check-test-weakening, check-decision-status) read the commits, not files, and
revert-red keeps records of its own; neither is here. `make check-everything` runs the steps'
targets directly, never through `record`, so it trusts no kept pass.
"""

from __future__ import annotations

import argparse
import datetime
import hashlib
import subprocess
import sys
import time
from typing import Any

import last_run
import revert_red_reruns

GATE = "check-pr"
NAME = "last-results"
INPUTS = "step-inputs"
# The pure steps and the files each reads, as the directories it reads ("" every one) and those it
# does not: the web lint the web client; the integration tier everything but it and the docs; the
# browser tier everything but the docs.
STEP_INPUTS = {
    "web-lint-full": (("web/",), ()),
    "test-integration": (("",), ("web/", "docs/")),
    "test-browser": (("",), ("docs/",)),
}
# The passes kept per step, the newest last.
KEPT_INPUTS = 20


def toolchain() -> str:
    """Go's and Node's versions here, which a kept pass holds for."""
    found = []
    for command in (["go", "env", "GOVERSION"], ["node", "--version"]):
        try:
            found.append(subprocess.run(command, capture_output=True, text=True, check=False).stdout.strip())
        except OSError:
            found.append("")
    return " ".join(found)


def inputs_key(step: str, base: str, head: str = "HEAD") -> str | None:
    """The key of what step reads at head against base: its files' blobs, the base, the toolchain;
    None for a step not in STEP_INPUTS, or when git cannot say."""
    reads = STEP_INPUTS.get(step)
    if reads is None or not base:
        return None
    listed = subprocess.run(["git", "ls-tree", "-r", "--full-tree", head], capture_output=True, text=True,
                            check=False)
    if listed.returncode != 0:
        return None
    included, excluded = reads
    files = [line for line in listed.stdout.splitlines()
             if (path := line.split("\t", 1)[-1]).startswith(included) and not path.startswith(excluded)]
    return hashlib.sha256("\0".join((step, base, toolchain(), *files)).encode()).hexdigest()


def kept_pass(step: str, key: str | None) -> bool:
    """Whether step passed before under key."""
    if key is None:
        return False
    return key in ((last_run.read(GATE, INPUTS) or {}).get(step) or [])


def keep_pass(step: str, key: str | None) -> None:
    if key is None:
        return

    def change(old: dict[str, Any] | None) -> dict[str, Any]:
        new = dict(old or {})
        keys = [each for each in new.get(step, []) if each != key]
        new[step] = [*keys, key][-KEPT_INPUTS:]
        return new
    last_run.update(GATE, INPUTS, change)
REVERT_RED = "check-revert-red"
STEP_TARGET = "pr-step-"  # the Makefile's wrapper that runs a step and records it


def start(base: str) -> None:
    last_run.update(GATE, NAME, lambda _old: {"base": base, "results": {}})


def clock() -> float:
    """The wall clock, as seconds since the epoch: what a step's start and end are recorded in."""
    return time.time()


def utc(seconds: float) -> str:
    """A clock time as the log prints it, HH:MM:SS in UTC."""
    return datetime.datetime.fromtimestamp(seconds, datetime.timezone.utc).strftime("%H:%M:%S")


def record(step: str, command: list[str]) -> int:
    started = time.monotonic()
    began = clock()
    print(f"check-pr step {step}: started at {utc(began)} UTC", flush=True)
    key = inputs_key(step, str((last_run.read(GATE, NAME) or {}).get("base") or ""))
    if kept_pass(step, key):
        print(f"{step}: passed before on the same files, base and toolchain; not run again", flush=True)
        status = 0
    else:
        status = subprocess.run(command, check=False).returncode
        if status == 0:
            keep_pass(step, key)
    seconds = round(time.monotonic() - started)
    ended = clock()
    print(f"check-pr step {step}: {'passed' if status == 0 else 'failed'} at {utc(ended)} UTC, "
          f"after {seconds} s", flush=True)
    head = last_run.commit_of("HEAD") or ""

    def change(old: dict[str, Any] | None) -> dict[str, Any]:
        new = old or {"base": "", "results": {}}
        new["results"][step] = {"passed": status == 0, "commit": head, "seconds": seconds,
                                "started": round(began), "ended": round(ended)}
        return new
    last_run.update(GATE, NAME, change)
    return status


def durations(steps: list[str]) -> str:
    """How long each of `steps` took in the recorded run, the longest first, and which did not run."""
    record_ = last_run.read(GATE, NAME) or {}
    results = record_.get("results", {})
    timed = sorted(((results[s]["seconds"], s) for s in steps if "seconds" in results.get(s, {})), reverse=True)
    lines = ["check-pr's steps, the longest first (the first group runs in parallel):"]
    lines += [f"  {seconds:>5} s  {step}{span(results[step])}{'' if results[step]['passed'] else '  (failed)'}"
              for seconds, step in timed]
    missing = [s for s in steps if "seconds" not in results.get(s, {})]
    if missing:
        lines.append("  did not run: " + " ".join(missing))
    return "\n".join(lines)


def span(result: dict[str, Any]) -> str:
    """When a recorded step ran, by the clock, or nothing for a record older than its clock times."""
    if "started" not in result or "ended" not in result:
        return ""
    return f"  ({utc(result['started'])}–{utc(result['ended'])} UTC)"


def plan(results: dict[str, Any], steps: list[str], head: str) -> tuple[list[str], list[str]]:
    """The steps to re-run, in order, and a line for each one skipped."""
    again, skipped = [], []
    for step in steps:
        result = results.get(step)
        if result is None or not result["passed"]:
            again.append(step)
            continue
        when = "this commit" if result["commit"] == head else "an older commit"
        skipped.append(f"  {step}  — passed on {result['commit'][:12]}, {when}")
    return again, skipped


def rerun(base: str, steps: list[str], lasts: list[str], jobs: int) -> int:
    record_ = last_run.read(GATE, NAME)
    if record_ is None:
        print(f"No check-pr run is recorded ({last_run.record_path(GATE, NAME)}); run `make check-pr`.")
        return 1
    if record_.get("base") != base:
        print(f"The last check-pr ran against base {str(record_.get('base'))[:12]}, not {base[:12]}; "
              "its results answer a different question. Run `make check-pr` against this base.")
        return 1
    results = record_.get("results", {})
    again, skipped = plan(results, [*steps, *lasts], last_run.commit_of("HEAD") or "")
    if skipped:
        print("Skipped: these passed at the last check-pr, and the fixes since may have broken them:")
        print("\n".join(skipped))
    status = 0
    first = [s for s in again if s not in lasts]
    if first:
        print(f"Re-running: {' '.join(first)}", flush=True)
        status = subprocess.run(["make", "--no-print-directory", f"-j{jobs}", "-k", "--output-sync=target",
                                 f"BASE={base}", *[STEP_TARGET + s for s in first]], check=False).returncode
    # The revert-red steps in their order, each after the steps before it pass (the code's, then
    # the stylesheets'), each on its own failed files when its record of them is whole.
    for last in lasts:
        if last not in again:
            continue
        if status == 0:
            status = rerun_last(last, last in results, base)
        else:
            print(f"{last} did not run: it runs only after the steps above pass.")
    print("\nA re-run of what failed, not the gate: the full `make check-pr` is still required "
          "before the branch is done.")
    return status


def rerun_last(last: str, ran: bool, base: str) -> int:
    """The last step again: revert-red on its failed files alone when it ran and failed and its
    own record of them is whole, and all of it otherwise."""
    target = last
    if last.startswith(REVERT_RED) and ran:
        _, _, problem = revert_red_reruns.recorded_failures(base)
        if problem:
            print(f"{problem}\nSo all of it runs again.")
        else:
            target = f"{last}-failed"
    print(f"Re-running: {target}", flush=True)
    return record(last, ["make", "--no-print-directory", target, f"BASE={base}"])


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = ap.add_subparsers(dest="action", required=True)
    begin = sub.add_parser("start")
    begin.add_argument("--base", required=True)
    one = sub.add_parser("record")
    one.add_argument("step")
    one.add_argument("command", nargs=argparse.REMAINDER)
    timing = sub.add_parser("durations")
    timing.add_argument("steps", nargs="+")
    again = sub.add_parser("rerun")
    again.add_argument("--base", required=True)
    again.add_argument("--last", required=True, action="append", help="a revert-red step, run alone after "
                       "the steps before it, in the order given (the code's, then the stylesheets')")
    again.add_argument("--jobs", type=int, default=4)
    again.add_argument("steps", nargs="+")
    args = ap.parse_args()

    if args.action == "start":
        start(last_run.commit_of(args.base) or args.base)
        return 0
    if args.action == "durations":
        print(durations(args.steps))
        return 0
    if args.action == "record":
        command = args.command[1:] if args.command[:1] == ["--"] else args.command
        return record(args.step, command)
    return rerun(last_run.commit_of(args.base) or args.base, args.steps, args.last, args.jobs)


if __name__ == "__main__":
    sys.exit(main())
