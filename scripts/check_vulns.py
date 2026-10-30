#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Reports known vulnerabilities in the pinned dependencies (govulncheck for Go, npm audit for
# ABOUTME: the web client) as check-pr's first step: a prompt to upgrade, never a failure.
"""The vulnerability report (the owner, 2026-10-30).

    check_vulns.py

The owner: "Can we make it one of the first things to run on a check-pr so that it prompts us to
upgrade, but doesn't cause a later failure on CI if something new comes out?" A vulnerability
published overnight is no fault of the branch, and a gate that turned red for it would fail CI on
code nobody changed. So this always exits 0. It prints:

* **Go:** govulncheck, pinned (GOVULNCHECK), run with `go run` so it stays out of go.mod and the
  binary. It reports only the vulnerabilities whose code ours can reach, each with the version that
  fixes it (research/security-pins-2026-10-30.md: one such, pion/stun, reached from the TURN relay).
* **Web:** npm audit's high and critical advisories, by package.

Either one that cannot run (no network, the database unreachable) says it could not check, and
the run goes on. Each takes a few seconds (3.7 s and 2.4 s, 2026-10-30).
"""

from __future__ import annotations

import json
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
# The owner's choice (2026-10-30): Go's own checker, at one version, outside go.mod.
GOVULNCHECK = "golang.org/x/vuln/cmd/govulncheck@v1.9.0"
TIMEOUT_S = 180
# npm audit's levels that are reported; lower ones are noise for a prompt to upgrade.
REPORTED_LEVELS = ("high", "critical")


def run(command: list[str], cwd: Path) -> subprocess.CompletedProcess[str] | None:
    """command's result, or None when it could not run in time."""
    try:
        return subprocess.run(command, cwd=cwd, capture_output=True, text=True, timeout=TIMEOUT_S, check=False)
    except (OSError, subprocess.TimeoutExpired):
        return None


def go_findings(stream: str) -> list[str]:
    """The reachable vulnerabilities in govulncheck's JSON stream, one line each: its id, the
    module and version found, and the version that fixes it."""
    decoder = json.JSONDecoder()
    found: dict[str, str] = {}
    summaries: dict[str, str] = {}
    at = 0
    while at < len(stream):
        while at < len(stream) and stream[at].isspace():
            at += 1
        if at >= len(stream):
            break
        message, at = decoder.raw_decode(stream, at)
        if "osv" in message:
            summaries[message["osv"]["id"]] = message["osv"].get("summary", "")
        finding = message.get("finding")
        # A finding traced to a function of ours has a trace whose first frame names a function.
        if finding and finding.get("trace") and finding["trace"][0].get("function"):
            frame = finding["trace"][0]
            fixed = finding.get("fixed_version") or "no fix yet"
            found[finding["osv"]] = f"{frame.get('module', '?')} {frame.get('version', '')}, fixed in {fixed}"
    return [f"{osv}: {where} — {summaries.get(osv, '')}".rstrip(" —") for osv, where in sorted(found.items())]


def npm_findings(report: str) -> list[str]:
    """npm audit's high and critical advisories, one line a package: its severity and whether a fix
    is available."""
    vulnerabilities = json.loads(report).get("vulnerabilities", {})
    lines = []
    for name, entry in sorted(vulnerabilities.items()):
        if entry.get("severity") in REPORTED_LEVELS:
            fix = "a fix is available" if entry.get("fixAvailable") else "no fix yet"
            lines.append(f"{name}: {entry['severity']}, {fix}")
    return lines


def go_report() -> list[str]:
    done = run(["go", "run", GOVULNCHECK, "-format", "json", "./..."], ROOT)
    if done is None or (done.returncode != 0 and not done.stdout.strip()):
        return ["Go: could not check (govulncheck did not run; no network?)"]
    try:
        findings = go_findings(done.stdout)
    except (json.JSONDecodeError, KeyError, TypeError):
        return ["Go: could not check (govulncheck's output was not understood)"]
    if not findings:
        return ["Go: no known vulnerability our code reaches"]
    return [f"Go: {len(findings)} known vulnerabilities our code reaches — upgrade:", *(f"  {f}" for f in findings)]


def npm_report() -> list[str]:
    done = run(["npm", "--prefix", "web", "audit", "--json"], ROOT)
    if done is None or not done.stdout.strip():
        return ["Web: could not check (npm audit did not run; no network?)"]
    try:
        findings = npm_findings(done.stdout)
    except (json.JSONDecodeError, KeyError, TypeError, AttributeError):
        return ["Web: could not check (npm audit's output was not understood)"]
    if not findings:
        return ["Web: no high or critical advisory"]
    return [f"Web: {len(findings)} high or critical advisories — upgrade:", *(f"  {f}" for f in findings)]


# What starts each line of the report: the quiet gate shows these lines on a pass too
# (quiet_gate.py ALWAYS_SHOWN), or a passing check-pr would hide the prompt.
PREFIX = "check-vulns: "


def main() -> int:
    print(f"{PREFIX}known vulnerabilities, a prompt to upgrade, never a failure", flush=True)
    for line in [*go_report(), *npm_report()]:
        print(f"{PREFIX}{line}", flush=True)
    return 0


if __name__ == "__main__":
    sys.exit(main())
