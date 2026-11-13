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
"""

from __future__ import annotations

import os
import re
import signal
import subprocess
import sys
import threading
from pathlib import Path

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


def step_times(clock: str) -> list[tuple[str, float, float, bool]]:
    """Each target step_clock.sh recorded, as (target, seconds from the run's first start, its
    seconds, whether a line of it failed), the longest first. A malformed line is left out."""
    spans: dict[str, list[float]] = {}
    failed: dict[str, bool] = {}
    for line in clock.splitlines():
        fields = line.split("\t")
        if len(fields) != 4:
            continue
        target, start_text, end_text, code = fields
        try:
            start, end = float(start_text), float(end_text)
        except ValueError:
            continue
        span = spans.setdefault(target, [start, end])
        span[0], span[1] = min(span[0], start), max(span[1], end)
        failed[target] = failed.get(target, False) or code != "0"
    if not spans:
        return []
    first = min(start for start, _ in spans.values())
    times = [(target, start - first, end - start, failed[target]) for target, (start, end) in spans.items()]
    return sorted(times, key=lambda time: (-time[2], time[0]))


def steps_report(label: str, clock: str) -> str:
    """How long each step took, for `<target>.steps`: seconds, when it started, and its name."""
    lines = [f"{label}: each step's time, the longest first (seconds; started, from the run's start)"]
    lines += [f"{seconds:8.1f}  +{began:6.1f}  {target}{'  (failed)' if failed else ''}"
              for target, began, seconds, failed in step_times(clock)]
    return "\n".join(lines) + "\n"


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

    lines: list[str] = []
    read_up_to: int | None = None
    try:
        assert process.stdout is not None
        for line in process.stdout:
            lines.append(line)
            if fail_fast and read_up_to is None and STEP_FAILED.search(line):
                read_up_to = len(lines)
                stop(signal.SIGTERM)
                threading.Timer(STOP_GRACE_SECONDS, stop, (signal.SIGKILL,)).start()
    except KeyboardInterrupt:
        stop(signal.SIGTERM)
        raise
    code = process.wait()
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
    code, output, read = run_make(make_args, fail_fast, {**os.environ, "STEP_CLOCK": str(clock)})
    log.write_text(output, encoding="utf-8")
    log.with_name(f"{make_args[0]}.steps").write_text(
        steps_report(label, clock.read_text(encoding="utf-8")), encoding="utf-8")
    if code == 0:
        last = next((line for line in reversed(output.splitlines()) if line.strip()), "")
        for line in shown_on_a_pass(output):
            print(line)
        print(last if last.startswith(label) else f"{label}: passed")
        return 0
    failed_log = log.with_name(f"{make_args[0]}.failed.log")
    failed_log.write_text(output, encoding="utf-8")
    print(summary(read, log, failed_log))
    print(f"{label}: FAILED")
    return code


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
