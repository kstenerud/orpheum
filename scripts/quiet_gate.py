#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Runs a make target with its full output kept in a log, and prints only what needs
# ABOUTME: reading: one line when it passes, the failures and no passing noise when it does not.
"""Run a gate quietly.

A passing `make check` printed about 150 lines (each package's `ok`, the file-size warnings, the
test runners' summaries), and agents read them every run; the owner asked for less (2026-09-30).
This runs `make <target>` with the full output written to a log under the git directory, then
prints one line on a pass, or on a failure the lines that are not known passing noise, capped,
with the log's path. `VERBOSE=1` in the environment prints everything, as before.

With `--fail-fast` (make check), the first step that fails stops the run: make itself starts no
new step after a failure but waits for the running ones, so a lint error found in seconds was
reported only once the Go linter and the test suites had finished (the owner, 2026-10-06). The
steps still running are stopped, and what they printed after the failure is left out of the
summary, kept in the log. `make check-pr` runs without it: it records every step's outcome.

A failed run's output is also kept as `<target>.failed.log`, which only the next failure
replaces: a hang's diagnostic once went into the log that the passing re-run then overwrote, and
the evidence was lost (audit-2026-09-30 §5, settled 2026-10-01).

Each run says how long each of its steps took, in `<target>.steps` beside the log, the longest
first: the run names a file in STEP_CLOCK, the Makefile then runs every recipe line through
`step_clock.sh`, which records its target's start and end there, and a step's time is its first
line's start to its last line's end. Without it no cut to the gate could be ranked by measurement
(gate-throughput G0; the owner, 2026-11-13).

Each run is also recorded, its time and its longest steps, in the git common directory's
`gate-logs/runs.jsonl` (scripts/gate_runs.py), and its line says how long it took, and when that was
over its budget, which steps took it there: make check took 92 to 236 s a day's median over the week
to 2026-11-21, against its 90 s, and nothing said so (the owner, 2026-11-21: "we should also be
timing make check and make spot").
"""

from __future__ import annotations

import os
import re
import signal
import subprocess
import sys
import threading
import time
from pathlib import Path

import gate_runs

# Lines a passing step prints. Anything else is kept, so an unknown line is never hidden.
NOISE = [re.compile(pattern) for pattern in (
    r"^ok\s+\S+\s+",                                  # a Go package passed
    r"^\?\s+\S+\s+\[no test files\]",
    r"^warning: .* lines \(over \d+; consider splitting\)",
    r"^check_\w+: .*\b0 problems\b",
    r"^check_generated: \d+ generated files match",
    r"^check_deps: every dependency is allowlisted",
    r"^check_architecture_map: every package has a row",
    r"^check_claims: architecture citations, .* resolve",
    r"^>> unused ",
    r"^lint-speculative-api: no speculative API",
    r"^0 issues\.$",
    r"^Checking formatting\.\.\.$",
    r"^All matched files use Prettier",
    r"^✔ no dependency violations",
    r"^\s*(RUN|Test Files|Tests|Start at|Duration|Transform|Isolate|persist|learn more)\b",
    r"^\s+at least ~.* faster with",
    r"^no pinned MKVToolNix for",
    r"^Ran \d+ tests? in ",
    r"^py_tests: \d+ modules, 0 failed$",
    r"^OK$",
    r"^-{20,}$",
    r"^ORPHEUM_TEST_UNCONTROLLED=",
    r"^make\[\d+\]: \*\*\* Waiting for unfinished jobs",
    r"^make check: already passed on this exact tree",
    r"^check-pr step \S+: (started|passed) at ",          # check_pr_steps.py's clock; a failure stays shown
)]
MAX_LINES = 200

# The line make prints when a step's recipe fails: with --fail-fast, the first ends the run.
STEP_FAILED = re.compile(r"^make(\[\d+\])?: \*\*\* \[[^\]]*\] Error \d+")
# How long the stopped steps have to end on SIGTERM before they are killed.
STOP_GRACE_SECONDS = 10
STOPPED_NOTE = "quiet_gate: a step failed, so the steps still running were stopped (--fail-fast)"


# Lines a run prints for the reader whether it passes or fails: check_vulns.py's report, a prompt to
# upgrade that never fails the run (the owner, 2026-10-30).
ALWAYS_SHOWN = "check-vulns: "


def shown_on_a_pass(output: str) -> list[str]:
    """The lines a passing run shows besides its last: the reports that never fail it."""
    return [line for line in output.splitlines() if line.startswith(ALWAYS_SHOWN)]


def is_noise(line: str) -> bool:
    return not line.strip() or any(pattern.search(line) for pattern in NOISE)


def summary(output: str, log: Path, failed_log: Path | None = None) -> str:
    """The lines of a failed run worth reading, capped, with where the rest is."""
    kept = [line for line in output.splitlines() if not is_noise(line)]
    shown = kept[:MAX_LINES]
    if len(kept) > MAX_LINES:
        shown.append(f"… {len(kept) - MAX_LINES} more lines")
    kept_note = f"; it is kept in {failed_log} until the next failure" if failed_log else ""
    shown.append(f"(the full output is in {log}{kept_note})")
    return "\n".join(shown)


def log_path(target: str) -> Path:
    git_dir = subprocess.run(
        ["git", "rev-parse", "--absolute-git-dir"], capture_output=True, text=True, check=False
    ).stdout.strip() or "."
    directory = Path(git_dir) / "gate-logs"
    directory.mkdir(parents=True, exist_ok=True)
    return directory / f"{target}.log"


# A target's lines further apart than this are two runs of it: web-deps runs before the parallel
# steps and again before the browser run, and one span from the first to the last read as 216 s.
RUN_GAP_S = 1.0

Span = tuple[str, float, float, bool, int]


def parents(clock: str) -> set[str]:
    """The steps whose recipe started a sub-make: the PARENT field step_clock.sh writes (since
    2026-11-21), empty when the clock is older and names none."""
    return {fields[5] for fields in (line.split("\t") for line in clock.splitlines())
            if len(fields) == 6 and fields[5]}


def step_spans(clock: str) -> list[Span]:
    """Each run of a target step_clock.sh recorded, as (target, seconds from the run's first start,
    its seconds, whether a line of it failed, make's level), the longest first: a run is the target's
    consecutive lines, its first line's start to its last line's end. A malformed line is left out; a
    line without a level (before 2026-11-21) is level 0."""
    lines: dict[str, list[tuple[float, float, bool, int]]] = {}
    for line in clock.splitlines():
        fields = line.split("\t")
        if len(fields) not in (4, 5, 6):
            continue
        target, start_text, end_text, code = fields[:4]
        try:
            start, end = float(start_text), float(end_text)
            level = int(fields[4]) if len(fields) >= 5 else 0
        except ValueError:
            continue
        lines.setdefault(target, []).append((start, end, code != "0", level))
    runs: list[tuple[str, float, float, bool, int]] = []
    for target, recorded in lines.items():
        recorded.sort()
        start, end, failed, level = recorded[0]
        for next_start, next_end, next_failed, next_level in recorded[1:]:
            if next_start - end > RUN_GAP_S or next_level != level:
                runs.append((target, start, end, failed, level))
                start, end, failed, level = next_start, next_end, next_failed, next_level
            else:
                end, failed = max(end, next_end), failed or next_failed
        runs.append((target, start, end, failed, level))
    if not runs:
        return []
    first = min(run[1] for run in runs)
    spans = [(target, start - first, end - start, failed, level) for target, start, end, failed, level in runs]
    return sorted(spans, key=lambda span: (-span[2], span[0], span[1]))


def step_times(clock: str) -> list[tuple[str, float, float, bool]]:
    """step_spans without make's level: (target, seconds from the run's start, its seconds, failed)."""
    return [span[:4] for span in step_spans(clock)]


def leaf_steps(spans: list[Span], ran_others: set[str] | None = None) -> list[tuple[str, float]]:
    """The steps that ran no sub-make, as (target, seconds), the longest first: a target that runs
    others through `$(MAKE)` (check-parallel, check-generated) spans them, and naming it would hide
    which of them took the time. `ran_others` names those steps (parents); without it, from a clock
    that does not, a step holds another that ran at a deeper make level within its span."""
    if ran_others:
        return [(span[0], span[2]) for span in spans if span[0] not in ran_others]
    def runs_within(outer: Span, inner: Span) -> bool:
        return inner[4] > outer[4] and outer[1] <= inner[1] and inner[1] + inner[2] <= outer[1] + outer[2] + 0.05
    return [(span[0], span[2]) for span in spans if not any(runs_within(span, other) for other in spans)]


def pass_line(label: str, seconds: float, leaves: list[tuple[str, float]]) -> str:
    """A pass's one line: its time, and over its budget, the three longest steps."""
    over = gate_runs.over_budget(label.removeprefix("make "), seconds)
    longest = ""
    if over:
        longest = ": the longest steps " + ", ".join(f"{target} {took:.0f} s" for target, took in leaves[:3])
    return f"{label}: passed in {seconds:.0f} s{over}{longest}"


def steps_report(label: str, clock: str) -> str:
    """How long each step took, for `<target>.steps`: seconds, when it started, and its name."""
    lines = [f"{label}: each step's time, the longest first (seconds; started, from the run's start)"]
    lines += [f"{seconds:8.1f}  +{began:6.1f}  {target}{'  (failed)' if failed else ''}"
              for target, began, seconds, failed in step_times(clock)]
    return "\n".join(lines) + "\n"


def _group_alive(group: int) -> bool:
    try:
        os.killpg(group, 0)
    except (ProcessLookupError, PermissionError):
        return False
    return True


def run_make(make_args: list[str], fail_fast: bool, env: dict[str, str] | None = None) -> tuple[int, str, str]:
    """Runs make: its exit code, its whole output, and the part a summary reads, which with
    fail_fast ends at the first failed step, whose stopping ends the run."""
    process = subprocess.Popen(
        ["make", "--no-print-directory", *make_args], stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT, text=True, start_new_session=True, env=env,
    )

    def stop(sig: int) -> None:
        try:
            os.killpg(process.pid, sig)
        except ProcessLookupError:
            pass

    # Stopped from outside, the gate stops its make, which runs in a session of its own: a check-pr
    # stopped by its process group left its steps running, revert-red among them (the owner,
    # 2026-11-24, the test-time stock-take's item 9).
    def ended(signum: int, _frame: object) -> None:
        stop(signal.SIGTERM)
        raise SystemExit(128 + signum)

    previous = {signum: signal.signal(signum, ended) for signum in (signal.SIGTERM, signal.SIGHUP)}
    lines: list[str] = []
    read_up_to: int | None = None
    killer: list[threading.Timer] = []
    try:
        assert process.stdout is not None
        for line in process.stdout:
            lines.append(line)
            if fail_fast and read_up_to is None and STEP_FAILED.search(line):
                read_up_to = len(lines)
                stop(signal.SIGTERM)
                killer.append(threading.Timer(STOP_GRACE_SECONDS, stop, (signal.SIGKILL,)))
                killer[0].start()
    except KeyboardInterrupt:
        stop(signal.SIGTERM)
        raise
    finally:
        for signum, handler in previous.items():
            signal.signal(signum, handler)
    code = process.wait()
    # The killer is for steps that outlive the asking; when none did, it would only keep this process
    # alive for the whole grace, which held every failed make check 10 s after it said so (2026-11-20).
    if killer and not _group_alive(process.pid):
        killer[0].cancel()
        killer[0].join()
    output = "".join(lines)
    if read_up_to is None:
        return code, output, output
    # make itself was stopped by the signal: the run failed as make's own failure does, with 2.
    return 2, output, "".join(lines[:read_up_to]) + STOPPED_NOTE + "\n"


def main(argv: list[str]) -> int:
    fail_fast = bool(argv) and argv[0] == "--fail-fast"
    if fail_fast:
        argv = argv[1:]
    if len(argv) < 2:
        print("usage: quiet_gate.py [--fail-fast] <label> <make target> [make arguments...]",
              file=sys.stderr)
        return 2
    label, make_args = argv[0], argv[1:]
    if os.environ.get("VERBOSE") == "1":
        return subprocess.run(["make", "--no-print-directory", *make_args], check=False).returncode
    log = log_path(make_args[0])
    clock = log.with_name(f"{make_args[0]}.clock")
    clock.write_text("", encoding="utf-8")
    started, load_before = time.time(), gate_runs.load()
    code, output, read = run_make(make_args, fail_fast, {**os.environ, "STEP_CLOCK": str(clock)})
    seconds = time.time() - started
    log.write_text(output, encoding="utf-8")
    clocked = clock.read_text(encoding="utf-8")
    log.with_name(f"{make_args[0]}.steps").write_text(steps_report(label, clocked), encoding="utf-8")
    leaves = leaf_steps(step_spans(clocked), parents(clocked))
    gate_runs.record({"start": gate_runs.started_at(started), "target": label.removeprefix("make "),
                      "seconds": round(seconds), "exit": code, "worktree": gate_runs.worktree(),
                      "loadBefore": load_before, "loadAfter": gate_runs.load(),
                      "steps": [[target, round(took, 1)] for target, took in leaves[:gate_runs.STEPS_KEPT]]})
    if code == 0:
        last = next((line for line in reversed(output.splitlines()) if line.strip()), "")
        for line in shown_on_a_pass(output):
            print(line)
        print(last if last.startswith(label) else pass_line(label, seconds, leaves))
        return 0
    failed_log = log.with_name(f"{make_args[0]}.failed.log")
    failed_log.write_text(output, encoding="utf-8")
    print(summary(read, log, failed_log))
    print(f"{label}: FAILED in {seconds:.0f} s")
    return code


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
