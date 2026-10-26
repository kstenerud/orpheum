# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: How revert-red runs one probe's tests and reads how they failed: in a process group of its own,
# ABOUTME: ended whole when the run ends or outlasts its timeout, so no server or browser it started stays.
"""One probe's test run, and everything it started, ended with it.

`subprocess.run`'s timeout kills only the process it started. A browser tier's run is `make`,
which starts npm, Playwright, the browsers and a core and node of their own: killed at its timeout,
the rest ran on, holding ports and CPU, and the next probe's browser run shared the machine with a
server nothing would stop (check-pr, 2026-10-17). The run here is a session of its own, and its
whole group is killed once it is over, however it ended.
"""

from __future__ import annotations

import os
import signal
import subprocess
import threading
from dataclasses import dataclass
from typing import IO

import revert_red_commands as commands


def run_whole_group(command: list[str], cwd: str, timeout: int,
                    env: dict[str, str] | None = None) -> subprocess.CompletedProcess[str]:
    """Runs command as subprocess.run with capture_output and text would, in a process group of its
    own, which is killed whole when the command ends; past timeout, it is killed and TimeoutExpired
    raised, as subprocess.run raises it. The command's end is its own exit, not its output's: a
    process it left behind holds the pipes open, and reading them to their end waited for it. The
    run keeps within the probe's share of the CPUs (revert_red_commands); the result names the command
    as given."""
    given = command
    command, env = commands.shared(command, env, commands.CPU_SHARE)
    with subprocess.Popen(command, cwd=cwd, env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                          text=True, start_new_session=True) as running:
        stdout, stderr = _read_aside(running.stdout), _read_aside(running.stderr)
        try:
            running.wait(timeout=timeout)
        finally:
            end_group(running.pid)
            stdout.thread.join()
            stderr.thread.join()
    return subprocess.CompletedProcess(given, running.returncode, stdout.text(), stderr.text())


@dataclass
class _Read:
    """A pipe read to its end on a thread of its own."""

    thread: threading.Thread
    parts: list[str]

    def text(self) -> str:
        return "".join(self.parts)


def _read_aside(pipe: IO[str] | None) -> _Read:
    parts: list[str] = []

    def read() -> None:
        if pipe is not None:
            parts.extend(iter(pipe.read, ""))

    thread = threading.Thread(target=read, daemon=True)
    thread.start()
    return _Read(thread, parts)


def end_group(leader: int) -> None:
    """Kills every process left in the group leader started, none of which may outlive its probe."""
    try:
        os.killpg(leader, signal.SIGKILL)
    except (ProcessLookupError, PermissionError):
        pass


# The shell's exit status for a command it could not find: `npm run test` in a tree with no
# installed packages, which once passed every TypeScript file as red because vitest was absent.
RUNNER_MISSING = 127

def is_build_failure(output: str) -> bool:
    """Whether a failing run failed to compile rather than to assert.

    Keyed on the toolchains' own markers rather than on error text: `go test` prints
    a `[build failed]` suffix on the FAIL line, and pytest reports an import-time
    problem as a collection error. Matching compiler prose ("undefined:", "cannot
    use") would misread a test that legitimately asserts on such a message.
    """
    markers = ("[build failed]", "ERROR collecting", "ImportError", "SyntaxError", "error TS",
               "error during build")
    return any(m in output for m in markers)
