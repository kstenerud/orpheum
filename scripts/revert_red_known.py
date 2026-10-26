# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Revert-red's record of the reverts it has seen turn a test red, by the file's two versions and
# ABOUTME: what the red run read, so a later probe of the same revert over the same code takes the red.
"""Reverts known red (check_revert_red.probe_unclaimed).

check-commit-red probes each commit's files as it is made, and check-pr probed them all again: a
batch of 24 commits took 754 s, most of it repeating reds already found (2026-10-21). A probe
that goes red records, at `<git-common-dir>/revert-red/known-red`, the file's path and the blobs
its revert went between, those of the files undone with it, and the git tree of every directory
the red run tested. A later probe of the same revert, the same blobs, where every one of those
trees is still the same at its head, takes the red without running: the code that turned red
and the tests that saw it are unchanged.

Only a red is taken. A green is never kept, so no file passes on a record; and the record answers
a revert only when the code the run read is what it was. What the red run read beyond those
directories (a package they import) may have changed, which is why `make check-everything`,
the nightly run, reads no record (`--trust-no-record`).
"""

from __future__ import annotations

import re
import subprocess
from typing import Any

import last_run
import revert_red_commands as commands

GATE = "revert-red"
NAME = "known-red"
# Enough reverts for a batch and the commits after it; the oldest go first.
KEPT = 2000


def blob(root: str, rev: str, path: str) -> str:
    """The blob path has at rev, or "" when it has none there."""
    return subprocess.run(["git", "rev-parse", "--verify", "--quiet", f"{rev}:{path}"], cwd=root,
                          capture_output=True, text=True, check=False).stdout.strip()


def tree(root: str, rev: str, directory: str) -> str:
    """The git tree of a directory at rev, or "" when it has none there."""
    return blob(root, rev, directory.removeprefix("./").rstrip("/") or ".")


def revert_key(root: str, base: str, head: str, paths: list[str]) -> str:
    """The revert of paths, the probed file first and its companions after, by their blobs."""
    return "|".join(f"{path}:{blob(root, base, path)}>{blob(root, head, path)}" for path in paths)


GO_PACKAGE = re.compile(r"^\./(.+?)(/\.\.\.)?$")


def dirs_read(command: list[str]) -> list[str]:
    """The directories a run's tests are: the Go packages it names, a pattern's root for `/...`;
    the web client for a vitest or npm run; the scripts for the Python tests. Another run's are
    unknown, and its red is not kept."""
    go = [match.group(1) for argument in command if (match := GO_PACKAGE.match(argument))]
    if commands.is_go_test(command) and go:
        return go
    if command and command[0] in ("npx", "npm"):
        return ["web"]
    if any(argument.endswith("py_tests.py") for argument in command):
        return ["scripts", ".claude/hooks"]
    return []


def remember(root: str, head: str, key: str, command: list[str], verdict: str, detail: str) -> None:
    """Records a red revert, with the trees at head of the directories the red run read."""
    dirs = dirs_read(command)
    if not dirs:
        return
    trees = {directory: tree(root, head, directory) for directory in dirs}

    def change(old: dict[str, Any] | None) -> dict[str, Any]:
        reverts = dict((old or {}).get("reverts", {}))
        reverts.pop(key, None)
        reverts[key] = {"trees": trees, "verdict": verdict, "detail": detail, "head": head}
        return {"reverts": dict(list(reverts.items())[-KEPT:])}
    last_run.update(GATE, NAME, change)


def known(root: str, head: str, key: str) -> dict[str, Any] | None:
    """The red recorded for this revert, if every directory its run read is the same at head."""
    record = last_run.read(GATE, NAME)
    entry = (record or {}).get("reverts", {}).get(key)
    if not entry:
        return None
    if any(tree(root, head, directory) != was for directory, was in entry["trees"].items()):
        return None
    return entry
