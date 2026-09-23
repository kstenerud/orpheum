#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# ABOUTME: PreToolUse hook for shell commands: refuses adding dependencies, skipping git
# ABOUTME: hooks, force-pushing, and obvious shell writes to gate files. A speed bump, not a
# ABOUTME: wall: a determined command can write a file without these words. The wall is the
# ABOUTME: branch gate's Gate-Changed trailer (check_test_weakening.py) and review.
from __future__ import annotations

import re
import shlex

import hooklib
from gate_paths import GATE_PATTERNS

ADD_DEPENDENCY = re.compile(r"\bgo\s+get\b|\b(?:pnpm|yarn)\s+add\b|\bpip3?\s+install\b")
NPM_INSTALL = frozenset({"i", "install", "add", "isntall"})
NPM_VALUE_FLAGS = frozenset({"--prefix", "-w", "--workspace", "-C", "--registry"})


def npm_adds_package(command: str) -> bool:
    """Whether any `npm … install <name>` in the command names a package to add.

    `npm install` with no name only installs what the lockfile already records.
    """
    for segment in re.split(r"[;&|]+", command):
        try:
            tokens = shlex.split(segment)
        except ValueError:
            tokens = segment.split()
        if not tokens or tokens[0] != "npm":
            continue
        seen_install = False
        skip_value = False
        for token in tokens[1:]:
            if skip_value:
                skip_value = False
                continue
            if token in NPM_VALUE_FLAGS:
                skip_value = True
                continue
            if token.startswith("-"):
                continue
            if not seen_install:
                if token in NPM_INSTALL:
                    seen_install = True
                    continue
                break  # some other subcommand: run, ci, test
            return True
    return False
SKIP_HOOKS = re.compile(r"--no-verify\b|\bcore\.hooksPath\b")
FORCE_PUSH = re.compile(r"\bgit\s+push\b.*(?:\s--force\b|\s-f\b|\s--force-with-lease\b)")
SHELL_WRITE = re.compile(r"(?:>>?|\btee\b|\bsed\s+-i|\bmv\b|\bcp\b|\brm\b|\btruncate\b|\bchmod\b)")
DEV_NULL = re.compile(r"\d?>>?\s*/dev/null|\d?>&\d")
HEREDOC = re.compile(r"<<-?\s*(['\"]?)(\w+)\1[^\n]*\n.*?\n\s*\2\s*(?:\n|$)", re.S)
QUOTED = re.compile(r"'[^']*'|\"(?:[^\"\\]|\\.)*\"")


def shell_text(command: str) -> str:
    """The command with heredoc bodies, quoted strings and /dev/null redirects blanked.

    A commit message is text, not a write: its `<noreply@anthropic.com>` is not a redirect,
    and its mention of `scripts/` is not a target.
    """
    return DEV_NULL.sub(" ", QUOTED.sub(" ", HEREDOC.sub(" ", command)))
GATE_WORDS = re.compile(
    r"(?:^|[\s'\"/])(?:Makefile|\.golangci\.ya?ml|sqlc\.ya?ml|scripts/|\.claude/|\.githooks/|"
    r"\.github/|deps/|internal/archtest/|eslint\.config|\.dependency-cruiser|tsconfig)"
)


def main() -> None:
    event = hooklib.read_event()
    command = str((event.get("tool_input") or {}).get("command") or "")
    if not command:
        return
    adds = ADD_DEPENDENCY.search(command) or npm_adds_package(command)
    if adds and not hooklib.owner_allows("ORPHEUM_ALLOW_DEPS"):
        hooklib.deny(
            "Adding a dependency is the owner's decision (AGENTS.md rule 3). Propose it: the "
            "name, the job it does, and why the standard library or an allowed package cannot. "
            "`npm ci` and `go mod download` are fine."
        )
    elif SKIP_HOOKS.search(command):
        hooklib.deny("Git hooks are part of the gate; don't bypass or repoint them.")
    elif FORCE_PUSH.search(command):
        hooklib.deny("No force-pushes. Add a commit that fixes the problem instead.")
    elif (
        SHELL_WRITE.search(shell_text(command))
        and GATE_WORDS.search(shell_text(command))
        and not hooklib.owner_allows("ORPHEUM_ALLOW_GATE_EDIT")
    ):
        hooklib.deny(
            "This command looks like it writes to a gate file (see scripts/gate_paths.py). "
            "Gates are the owner's to change; tell the owner what to change and why."
        )


assert GATE_PATTERNS  # the list GATE_WORDS approximates for shell text

if __name__ == "__main__":
    main()
