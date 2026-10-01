#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# ABOUTME: Runs a make target with its full output kept in a log, and prints only what needs
# ABOUTME: reading: one line when it passes, the failures and no passing noise when it does not.
"""Run a gate quietly.

A passing `make check` printed about 150 lines (each package's `ok`, the file-size warnings, the
test runners' summaries), and agents read them every run; the owner asked for less (2026-09-30).
This runs `make <target>` with the full output written to a log under the git directory, then
prints one line on a pass, or on a failure the lines that are not known passing noise, capped,
with the log's path. `VERBOSE=1` in the environment prints everything, as before.
"""

from __future__ import annotations

import os
import re
import subprocess
import sys
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
)]
MAX_LINES = 200


def is_noise(line: str) -> bool:
    return not line.strip() or any(pattern.search(line) for pattern in NOISE)


def summary(output: str, log: Path) -> str:
    """The lines of a failed run worth reading, capped, with where the rest is."""
    kept = [line for line in output.splitlines() if not is_noise(line)]
    shown = kept[:MAX_LINES]
    if len(kept) > MAX_LINES:
        shown.append(f"… {len(kept) - MAX_LINES} more lines")
    shown.append(f"(the full output is in {log})")
    return "\n".join(shown)


def log_path(target: str) -> Path:
    git_dir = subprocess.run(
        ["git", "rev-parse", "--absolute-git-dir"], capture_output=True, text=True, check=False
    ).stdout.strip() or "."
    directory = Path(git_dir) / "gate-logs"
    directory.mkdir(parents=True, exist_ok=True)
    return directory / f"{target}.log"


def main(argv: list[str]) -> int:
    if len(argv) < 2:
        print("usage: quiet_gate.py <label> <make target> [make arguments...]", file=sys.stderr)
        return 2
    label, make_args = argv[0], argv[1:]
    if os.environ.get("VERBOSE") == "1":
        return subprocess.run(["make", "--no-print-directory", *make_args], check=False).returncode
    log = log_path(make_args[0])
    done = subprocess.run(
        ["make", "--no-print-directory", *make_args], capture_output=True, text=True, check=False
    )
    output = done.stdout + done.stderr
    log.write_text(output, encoding="utf-8")
    if done.returncode == 0:
        last = next((line for line in reversed(output.splitlines()) if line.strip()), "")
        print(last if last.startswith(label) else f"{label}: passed")
        return 0
    print(summary(output, log))
    print(f"{label}: FAILED")
    return done.returncode


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
