#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Starts each browser the browser tier drives once, and on Linux installs the system
# ABOUTME: libraries Playwright names when one cannot start, so a new machine needs no hand setup.
"""Make sure the pinned Playwright's Chromium and Firefox start on this machine.

    ensure_browsers.py

`make browsers` runs it after Playwright has fetched the browsers. Each is started headless once
(about 1.5 s for both); one that cannot start is, on Linux, given the system libraries Playwright
lists for it (`playwright install-deps`, as root or through `sudo -n`), and tried again. A
rebuilt sandbox kept its clone but none of those libraries, and every browser test failed in
milliseconds on "libnspr4.so: cannot open shared object file" (2026-10-23; the owner: "Prerequisites
should be automatically installed"). A record of a past success would survive exactly that
rebuild, so the start itself is the check. What still fails is reported with the command to run.
"""

from __future__ import annotations

import os
import platform
import subprocess
import sys
from collections.abc import Callable
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
BROWSERS = ("chromium", "firefox")

# Starts each browser named in argv and closes it; prints "<name> FAILED <why>" for each that
# cannot start, and exits non-zero if any could not.
PROBE = """
import * as playwright from "playwright";
for (const name of process.argv.slice(1)) {
  try { const browser = await playwright[name].launch(); await browser.close(); }
  catch (error) {
    const lines = String(error.message).split("\\n");
    console.log(name, "FAILED", lines.find((l) => l.includes("error while loading")) ?? lines[0]);
    process.exitCode = 1;
  }
}
"""

Run = Callable[[list[str]], subprocess.CompletedProcess]


def run(argv: list[str]) -> subprocess.CompletedProcess:
    return subprocess.run(argv, cwd=ROOT / "web", capture_output=True, text=True, check=False)


def failures(runner: Run) -> list[str]:
    """The browsers that cannot start, each with why."""
    done = runner(["node", "--input-type=module", "-e", PROBE, *BROWSERS])
    found = [line for line in done.stdout.splitlines() if " FAILED " in line]
    if done.returncode != 0 and not found:
        found = [f"the probe failed: {(done.stderr or done.stdout).strip()[:400]}"]
    return found


def install_command(is_root: bool) -> list[str]:
    command = ["npx", "--no-install", "playwright", "install-deps", *BROWSERS]
    return command if is_root else ["sudo", "-n", *command]


def ensure(runner: Run, system: str, is_root: bool, say: Callable[[str], None]) -> int:
    failed = failures(runner)
    if not failed:
        return 0
    command = install_command(is_root)
    if system != "Linux":
        say("browsers: these cannot start, and only Linux has their libraries installed for it:")
        for line in failed:
            say(f"  {line}")
        return 1
    say("browsers: installing the system libraries they need: " + " ".join(command))
    for line in failed:
        say(f"  {line}")
    installed = runner(command)
    if installed.returncode != 0:
        say(f"browsers: the install failed ({installed.returncode}): "
            f"{(installed.stderr or installed.stdout).strip()[-400:]}")
        say("  run it yourself as root: npx --prefix web playwright install-deps " + " ".join(BROWSERS))
        return 1
    still = failures(runner)
    if still:
        say("browsers: still cannot start after the install:")
        for line in still:
            say(f"  {line}")
        return 1
    say("browsers: both start now")
    return 0


def main() -> int:
    return ensure(run, platform.system(), os.geteuid() == 0, lambda line: print(line, file=sys.stderr))


if __name__ == "__main__":
    sys.exit(main())
